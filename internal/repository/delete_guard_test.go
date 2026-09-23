package repository_test

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
)

func TestDeleteTransaction_RefusedWhenDebtHasInstalments(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	debtRepo := repository.NewDebtRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 10)

	trx := newTrx(user, product, customer, enum.Hutang, 2, 50000)
	if _, err := repo.CreateTransaction(context.Background(), trx, true, true); err != nil {
		t.Fatalf("create hutang transaction: %v", err)
	}
	if trx.DebtID == nil {
		t.Fatal("expected the hutang sale to be linked to a debt")
	}

	if _, err := debtRepo.PayDebt(context.Background(), *trx.DebtID, &domain.DebtPayments{
		UserID:       user.ID,
		NominalBayar: 30000,
	}); err != nil {
		t.Fatalf("record debt payment: %v", err)
	}

	err := repo.DeleteTransaction(context.Background(), trx.ID)
	if err == nil {
		t.Fatal("expected the delete to be refused once the debt has instalments")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("error = %v; want it to match domain.ErrConflict", err)
	}

	var stillThere domain.Transactions
	if err := db.Where("id = ?", trx.ID).First(&stillThere).Error; err != nil {
		t.Fatalf("transaction should still exist after a refused delete: %v", err)
	}
	var afterProduct domain.Products
	if err := db.Where("id = ?", product.ID).First(&afterProduct).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if afterProduct.Stock != 8 {
		t.Errorf("stock = %v; want 8 (unchanged — a refused delete must not restore stock)", afterProduct.Stock)
	}

	var debt domain.Debts
	if err := db.Where("id = ?", *trx.DebtID).First(&debt).Error; err != nil {
		t.Fatalf("load debt: %v", err)
	}
	if debt.RemainingDebt != 70000 || debt.Status == enum.LUNAS {
		t.Errorf("debt remaining = %d status = %v; want 70000 and still unpaid",
			debt.RemainingDebt, debt.Status)
	}
}

func TestDeleteTransaction_RefusedWhenBackedBySettledPayment(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, _ := seedUserProductCustomer(t, db, 10)

	trx := newTrx(user, product, nil, enum.MoneyPayment(3), 2, 50000)
	if _, err := repo.CreateTransaction(context.Background(), trx, false, true); err != nil {
		t.Fatalf("create qris transaction: %v", err)
	}

	payment := &domain.Payment{
		OrderID:        trx.NoInvoice,
		UserID:         user.ID,
		Method:         "qris",
		SubtotalAmount: trx.TotalTransaction,
		GrossAmount:    trx.TotalTransaction,
		Status:         domain.PaymentSuccess,
		TransactionID:  &trx.ID,
		Items:          []domain.PaymentItem{{ProductID: product.ID, Qty: 2, UnitPrice: 50000}},
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("seed settled payment: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&domain.Payment{}, "id = ?", payment.ID) })

	err := repo.DeleteTransaction(context.Background(), trx.ID)
	if err == nil {
		t.Fatal("expected the delete to be refused for a settled online payment")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("error = %v; want it to match domain.ErrConflict", err)
	}

	var afterProduct domain.Products
	if err := db.Where("id = ?", product.ID).First(&afterProduct).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if afterProduct.Stock != 8 {
		t.Errorf("stock = %v; want 8 (unchanged — the goods were paid for)", afterProduct.Stock)
	}
}

func TestDeleteTransaction_AllowedWhenNothingSettled(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	user, product, _ := seedUserProductCustomer(t, db, 10)

	trx := newTrx(user, product, nil, enum.MoneyPayment(0), 2, 50000)
	if _, err := repo.CreateTransaction(context.Background(), trx, false, true); err != nil {
		t.Fatalf("create cash transaction: %v", err)
	}

	if err := repo.DeleteTransaction(context.Background(), trx.ID); err != nil {
		t.Fatalf("a plain cash sale with no instalments and no payment must stay deletable: %v", err)
	}

	var afterProduct domain.Products
	if err := db.Where("id = ?", product.ID).First(&afterProduct).Error; err != nil {
		t.Fatalf("load product: %v", err)
	}
	if afterProduct.Stock != 10 {
		t.Errorf("stock = %v; want 10 (restored by the successful delete)", afterProduct.Stock)
	}
}

func TestDeleteTransaction_UnknownIDIsNotFound(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)

	err := repo.DeleteTransaction(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v; want it to match domain.ErrNotFound", err)
	}
}

func TestDeleteCustomer_RefusedWithOpenDebt(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewTransactionRepository(db)
	customers := repository.NewCustomerRepository(db)
	user, product, customer := seedUserProductCustomer(t, db, 10)

	trx := newTrx(user, product, customer, enum.Hutang, 1, 50000)
	if _, err := repo.CreateTransaction(context.Background(), trx, true, true); err != nil {
		t.Fatalf("create hutang transaction: %v", err)
	}

	err := customers.DeleteCustomer(context.Background(), customer.ID)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("error = %v; want it to match domain.ErrConflict while the debt is unpaid", err)
	}
}
