package repository_test

import (
	"context"
	"math"
	"os"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_DSN to run this test against a real, migrated Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return db
}

func seedUserProductCustomer(t *testing.T, db *gorm.DB, stock float64) (*domain.Users, *domain.Products, *domain.Customers) {
	t.Helper()

	user := &domain.Users{Username: "cashier-" + uuid.NewString(), Password: "x", Role: enum.UserRole(0)}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	product := &domain.Products{
		SKU:              "SKU-" + uuid.NewString(),
		ProductName:      "Test Product",
		PurchasePrice:    5000,
		SellingPrice:     10000,
		SellingPriceDebt: 11000,
		Stock:            stock,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}

	customer := &domain.Customers{Name: "Test Customer " + uuid.NewString()}
	if err := db.Create(customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	t.Cleanup(func() {
		var trxIDs []uuid.UUID
		db.Unscoped().Model(&domain.Transactions{}).Where("user_id = ?", user.ID).Pluck("id", &trxIDs)
		if len(trxIDs) > 0 {
			db.Unscoped().Delete(&domain.TransactionsDetail{}, "transaction_id IN ?", trxIDs)
		}
		db.Unscoped().Delete(&domain.Transactions{}, "user_id = ?", user.ID)
		db.Unscoped().Delete(&domain.Debts{}, "customer_id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Products{}, "id = ?", product.ID)
		db.Unscoped().Delete(&domain.Customers{}, "id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	return user, product, customer
}

func newTrx(user *domain.Users, product *domain.Products, customer *domain.Customers, paymentType enum.MoneyPayment, qty float64, unitPrice int64) *domain.Transactions {
	var customerID *uuid.UUID
	if customer != nil {
		id := customer.ID
		customerID = &id
	}
	subtotal := int64(math.Round(qty * float64(unitPrice)))
	return &domain.Transactions{
		NoInvoice:        "INV-" + uuid.NewString(),
		UserID:           user.ID,
		CustomerID:       customerID,
		PaymentType:      paymentType,
		TotalTransaction: subtotal,
		TransactionDetail: []domain.TransactionsDetail{
			{ProductID: product.ID, Price: unitPrice, PriceDebt: unitPrice, Qty: qty, Subtotal: subtotal},
		},
	}
}

func TestCreateTransaction_HutangIncreasesDebt(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 10)

	first := newTrx(user, product, customer, enum.MoneyPayment(1), 2, 11000)
	firstSnap, err := repo.CreateTransaction(context.Background(), first, true, true)
	if err != nil {
		t.Fatalf("first hutang transaction failed: %v", err)
	}
	if first.DebtID == nil {
		t.Fatal("expected the transaction to be linked to a newly created debt")
	}
	if firstSnap == nil {
		t.Fatal("expected a non-nil TransactionDebtSnapshot for a hutang sale")
	}
	if firstSnap.PreviousRemainingDebt != 0 {
		t.Errorf("previous remaining debt = %v, want 0 (first debt for this customer)", firstSnap.PreviousRemainingDebt)
	}
	if firstSnap.AmountAdded != 22000 || firstSnap.TotalDebt != 22000 || firstSnap.RemainingDebt != 22000 {
		t.Errorf("unexpected first snapshot: %+v", firstSnap)
	}

	var debt domain.Debts
	if err := db.First(&debt, "id = ?", *first.DebtID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if debt.TotalDebt != 22000 || debt.RemainingDebt != 22000 {
		t.Errorf("after first hutang sale: total=%v remaining=%v, want both 22000", debt.TotalDebt, debt.RemainingDebt)
	}
	if debt.Status != enum.BELUM_LUNAS {
		t.Errorf("expected status BELUM_LUNAS, got %v", debt.Status)
	}

	second := newTrx(user, product, customer, enum.MoneyPayment(1), 1, 11000)
	secondSnap, err := repo.CreateTransaction(context.Background(), second, true, true)
	if err != nil {
		t.Fatalf("second hutang transaction failed: %v", err)
	}
	if second.DebtID == nil || *second.DebtID != *first.DebtID {
		t.Fatal("expected the second hutang sale to reuse the same customer's debt row")
	}
	if secondSnap == nil {
		t.Fatal("expected a non-nil TransactionDebtSnapshot for the second hutang sale")
	}
	if secondSnap.PreviousRemainingDebt != 22000 {
		t.Errorf("previous remaining debt = %v, want 22000 (balance before this sale)", secondSnap.PreviousRemainingDebt)
	}
	if secondSnap.AmountAdded != 11000 || secondSnap.TotalDebt != 33000 || secondSnap.RemainingDebt != 33000 {
		t.Errorf("unexpected second snapshot: %+v", secondSnap)
	}

	if err := db.First(&debt, "id = ?", *first.DebtID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if debt.TotalDebt != 33000 || debt.RemainingDebt != 33000 {
		t.Errorf("after second hutang sale: total=%v remaining=%v, want both 33000 (22000+11000)", debt.TotalDebt, debt.RemainingDebt)
	}

	var reloadedProduct domain.Products
	if err := db.First(&reloadedProduct, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if reloadedProduct.Stock != 7 {
		t.Errorf("stock = %v, want 7 (must be decremented on a hutang sale too)", reloadedProduct.Stock)
	}
}

func TestCreateTransaction_CashDoesNotCreateDebt(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 5)

	trx := newTrx(user, product, customer, enum.MoneyPayment(0), 1, 10000)
	snap, err := repo.CreateTransaction(context.Background(), trx, false, true)
	if err != nil {
		t.Fatalf("cash transaction failed: %v", err)
	}
	if trx.DebtID != nil {
		t.Error("a cash sale must not create/link a debt")
	}
	if snap != nil {
		t.Errorf("a cash sale must return a nil TransactionDebtSnapshot, got: %+v", snap)
	}

	var count int64
	db.Model(&domain.Debts{}).Where("customer_id = ?", customer.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected no debt rows for a cash sale, found %d", count)
	}

	var reloadedProduct domain.Products
	if err := db.First(&reloadedProduct, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if reloadedProduct.Stock != 4 {
		t.Errorf("stock = %v, want 4", reloadedProduct.Stock)
	}
}

func TestDeleteTransaction_ReversesDebtAndRestoresStock(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 10)

	trx := newTrx(user, product, customer, enum.MoneyPayment(1), 2, 11000)
	if _, err := repo.CreateTransaction(context.Background(), trx, true, true); err != nil {
		t.Fatalf("create hutang transaction failed: %v", err)
	}

	if err := repo.DeleteTransaction(context.Background(), trx.ID); err != nil {
		t.Fatalf("delete transaction failed: %v", err)
	}

	var debt domain.Debts
	if err := db.First(&debt, "id = ?", *trx.DebtID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if debt.TotalDebt != 0 || debt.RemainingDebt != 0 {
		t.Errorf("after deleting the only hutang transaction: total=%v remaining=%v, want both 0", debt.TotalDebt, debt.RemainingDebt)
	}
	if debt.Status != enum.LUNAS {
		t.Errorf("expected status LUNAS once RemainingDebt reaches 0, got %v", debt.Status)
	}

	var reloadedProduct domain.Products
	if err := db.First(&reloadedProduct, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if reloadedProduct.Stock != 10 {
		t.Errorf("stock = %v, want 10 (fully restored)", reloadedProduct.Stock)
	}

	var deleted domain.Transactions
	err := db.Where("id = ?", trx.ID).First(&deleted).Error
	if err == nil {
		t.Error("expected the transaction to be soft-deleted (not visible to a plain query)")
	}
}

func TestDeleteTransaction_PartialReversalKeepsRemainderOwed(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 10)

	first := newTrx(user, product, customer, enum.MoneyPayment(1), 1, 11000)
	second := newTrx(user, product, customer, enum.MoneyPayment(1), 1, 11000)
	if _, err := repo.CreateTransaction(context.Background(), first, true, true); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := repo.CreateTransaction(context.Background(), second, true, true); err != nil {
		t.Fatalf("create second: %v", err)
	}

	if err := repo.DeleteTransaction(context.Background(), first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}

	var debt domain.Debts
	if err := db.First(&debt, "id = ?", *first.DebtID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if debt.RemainingDebt != 11000 {
		t.Errorf("remaining debt = %v, want 11000 (only the deleted transaction's amount reversed)", debt.RemainingDebt)
	}
	if debt.Status != enum.BELUM_LUNAS {
		t.Errorf("expected status to remain BELUM_LUNAS while 11000 is still owed, got %v", debt.Status)
	}
}
