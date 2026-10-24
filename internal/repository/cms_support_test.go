package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
)

func TestCreateTransaction_OfflineSaleMayOversellStock(t *testing.T) {
	db := openTestDB(t)
	user, product, _ := seedUserProductCustomer(t, db, 1)
	repo := repository.NewTransactionRepository(db)

	online := newTrx(user, product, nil, enum.MoneyPayment(0), 3, 10000)
	if _, err := repo.CreateTransaction(context.Background(), online, false, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("online oversell: err = %v; want conflict", err)
	}

	offline := newTrx(user, product, nil, enum.MoneyPayment(0), 3, 10000)
	ref := uuid.New()
	offline.ClientRef = &ref
	if _, err := repo.CreateTransaction(context.Background(), offline, false, true); err != nil {
		t.Fatalf("offline oversell rejected: %v", err)
	}
	if !offline.StockConflict {
		t.Error("offline oversell not flagged as stock conflict")
	}
	var stock float64
	db.Model(&domain.Products{}).Where("id = ?", product.ID).Pluck("stock", &stock)
	if stock != 0 {
		t.Errorf("stock = %v; want 0, the floor kept by chk_products_stock_non_negative", stock)
	}

	page, err := repo.GetAllTransaction(context.Background(), domain.FilterTransaction{OpenConflict: true, Limit: 100})
	if err != nil || !containsTrx(page.DataItem, offline.ID) {
		t.Fatalf("open conflict filter missed the transaction (err %v)", err)
	}
	if err := repo.MarkConflictReviewed(context.Background(), offline.ID); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	page, _ = repo.GetAllTransaction(context.Background(), domain.FilterTransaction{OpenConflict: true, Limit: 100})
	if containsTrx(page.DataItem, offline.ID) {
		t.Error("reviewed conflict still listed as open")
	}
}

func containsTrx(items []*domain.Transactions, id uuid.UUID) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func TestClientRef_SurvivesCancelAndBlocksResend(t *testing.T) {
	db := openTestDB(t)
	user, product, _ := seedUserProductCustomer(t, db, 10)
	repo := repository.NewTransactionRepository(db)
	ref := uuid.New()

	first := newTrx(user, product, nil, enum.MoneyPayment(0), 1, 10000)
	first.ClientRef = &ref
	if _, err := repo.CreateTransaction(context.Background(), first, false, true); err != nil {
		t.Fatalf("create: %v", err)
	}
	resend := newTrx(user, product, nil, enum.MoneyPayment(0), 1, 10000)
	resend.ClientRef = &ref
	if _, err := repo.CreateTransaction(context.Background(), resend, false, true); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("resend: err = %v; want duplicate", err)
	}

	if err := repo.DeleteTransaction(context.Background(), first.ID, "salah input"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	found, err := repo.GetTransactionByClientRef(context.Background(), ref)
	if err != nil || found == nil || found.ID != first.ID {
		t.Fatalf("cancelled transaction not found by client_ref: %+v, %v", found, err)
	}
	if found.CancelReason == nil || *found.CancelReason != "salah input" {
		t.Errorf("cancel reason = %v; want stored", found.CancelReason)
	}
}

func TestPurgeUser_DeletesOrAnonymizes(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewUserRepository(db)
	ctx := context.Background()

	idle := &domain.Users{Username: "idle-" + uuid.NewString(), Password: "x", Role: enum.Staff}
	if err := db.Create(idle).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&domain.Users{}, "id = ?", idle.ID) })

	if _, err := repo.PurgeUser(ctx, idle.ID, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("purge active user: err = %v; want conflict", err)
	}
	if err := repo.DeactivateUser(ctx, idle.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	anonymized, err := repo.PurgeUser(ctx, idle.ID, map[string]any{"username": "x-" + uuid.NewString()})
	if err != nil || anonymized {
		t.Fatalf("purge idle user: anonymized=%v err=%v; want hard delete", anonymized, err)
	}
	if _, err := repo.GetUserAnyState(ctx, idle.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("idle user still present: %v", err)
	}

	seller, product, _ := seedUserProductCustomer(t, db, 5)
	if _, err := repository.NewTransactionRepository(db).CreateTransaction(ctx, newTrx(seller, product, nil, enum.MoneyPayment(0), 1, 10000), false, true); err != nil {
		t.Fatalf("seed sale: %v", err)
	}
	if err := repo.DeactivateUser(ctx, seller.ID); err != nil {
		t.Fatalf("deactivate seller: %v", err)
	}
	anonName := "staff-terhapus-" + uuid.NewString()[:8]
	anonymized, err = repo.PurgeUser(ctx, seller.ID, map[string]any{"username": anonName})
	if err != nil || !anonymized {
		t.Fatalf("purge seller: anonymized=%v err=%v; want anonymized", anonymized, err)
	}
	kept, err := repo.GetUserAnyState(ctx, seller.ID)
	if err != nil || kept.Username != anonName {
		t.Errorf("seller row = %+v, %v; want kept with anonymized username", kept, err)
	}
}

func TestDeleteCategory_RefusedWhileUsed(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewProductRepository(db)
	ctx := context.Background()
	_, product, _ := seedUserProductCustomer(t, db, 1)

	category := &domain.Categories{Key: "cat-" + uuid.NewString(), Name: "Cat"}
	if err := repo.AddCategory(ctx, category); err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Model(&domain.Products{}).Where("id = ?", product.ID).Update("category_id", nil)
		db.Delete(&domain.Categories{}, "id = ?", category.ID)
	})
	if err := repo.AddCategory(ctx, &domain.Categories{Key: category.Key, Name: "dup"}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate key: err = %v; want duplicate", err)
	}
	db.Model(&domain.Products{}).Where("id = ?", product.ID).Update("category_id", category.ID)

	if err := repo.DeleteCategory(ctx, category.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete used category: err = %v; want conflict", err)
	}
	rows, err := repo.ListCategories(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range rows {
		if row.ID == category.ID && row.ProductCount != 1 {
			t.Errorf("product_count = %d; want 1", row.ProductCount)
		}
	}

	db.Delete(&domain.Products{}, "id = ?", product.ID)
	if err := repo.DeleteCategory(ctx, category.ID); err != nil {
		t.Errorf("delete category of soft-deleted product: %v", err)
	}
}

func TestDashboardSummary_CountsTodaySales(t *testing.T) {
	db := openTestDB(t)
	user, product, _ := seedUserProductCustomer(t, db, 5)
	if _, err := repository.NewTransactionRepository(db).CreateTransaction(context.Background(),
		newTrx(user, product, nil, enum.MoneyPayment(0), 2, 10000), false, true); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	now := time.Now().In(domain.StoreLocation)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, domain.StoreLocation)
	summary, err := repository.NewDashboardRepository(db).Summary(context.Background(), from, from.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalTransactions < 1 || summary.Revenue < 20000 || summary.GrossProfit < 10000 {
		t.Errorf("summary = %+v; want today's 20000 sale with 10000 profit counted", summary)
	}
	if len(summary.Daily) == 0 || len(summary.TopProducts) == 0 || len(summary.PaymentMethods) == 0 {
		t.Errorf("breakdowns empty: daily=%d top=%d methods=%d", len(summary.Daily), len(summary.TopProducts), len(summary.PaymentMethods))
	}
	var dayRevenue, methodRevenue, dayCount, methodCount int64
	for _, d := range summary.Daily {
		dayRevenue, dayCount = dayRevenue+d.Revenue, dayCount+d.Transactions
	}
	for _, m := range summary.PaymentMethods {
		methodRevenue, methodCount = methodRevenue+m.Total, methodCount+m.Count
	}
	if dayRevenue != summary.Revenue || methodRevenue != summary.Revenue || dayCount != summary.TotalTransactions || methodCount != summary.TotalTransactions {
		t.Errorf("breakdowns disagree with totals: daily %d/%d, methods %d/%d, totals %d/%d",
			dayRevenue, dayCount, methodRevenue, methodCount, summary.Revenue, summary.TotalTransactions)
	}
}

func TestDebts_ShowDeactivatedCustomer(t *testing.T) {
	db := openTestDB(t)
	_, _, customer := seedUserProductCustomer(t, db, 1)
	debt := &domain.Debts{CustomerID: customer.ID, TotalDebt: 1000, RemainingDebt: 0, Status: enum.LUNAS}
	if err := db.Create(debt).Error; err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	if err := repository.NewCustomerRepository(db).DeleteCustomer(context.Background(), customer.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	repo := repository.NewDebtRepository(db)
	got, err := repo.GetDebtByID(context.Background(), debt.ID)
	if err != nil || got.Customer.Name != customer.Name {
		t.Errorf("debt detail customer = %q, %v; want %q", got.Customer.Name, err, customer.Name)
	}
	page, err := repo.GetAllDebt(context.Background(), domain.FilterDebt{Search: customer.Name, Limit: 10})
	if err != nil || len(page.Data) != 1 || page.Data[0].Customer.Name != customer.Name {
		t.Errorf("search by deactivated customer: %+v, %v; want the paid debt with its customer", page, err)
	}
}
