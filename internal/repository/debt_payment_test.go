package repository_test

import (
	"context"
	"strings"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
)

func TestPayDebt_PartialPaymentReducesRemainingDebt(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewDebtRepository(db)

	customer := &domain.Customers{Name: "Debt Payer " + uuid.NewString()}
	if err := db.Create(customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	debt := &domain.Debts{CustomerID: customer.ID, TotalDebt: 50000, RemainingDebt: 50000, Status: enum.BELUM_LUNAS}
	if err := db.Create(debt).Error; err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	user := &domain.Users{Username: "cashier-" + uuid.NewString(), Password: "x"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&domain.DebtPayments{}, "debt_id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Debts{}, "id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Customers{}, "id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	result, err := repo.PayDebt(context.Background(), debt.ID, &domain.DebtPayments{UserID: user.ID, NominalBayar: 20000})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Debt.RemainingDebt != 30000 {
		t.Errorf("remaining debt = %v, want 30000 (50000-20000)", result.Debt.RemainingDebt)
	}
	if result.Debt.Status != enum.BELUM_LUNAS {
		t.Errorf("expected status BELUM_LUNAS while 30000 is still owed, got %v", result.Debt.Status)
	}
	if result.PreviousRemainingDebt != 50000 {
		t.Errorf("previous remaining debt = %v, want 50000 (the balance before this payment)", result.PreviousRemainingDebt)
	}
	if result.Debt.Customer.Name != customer.Name {
		t.Errorf("expected the debt's customer to be preloaded for the receipt, got: %+v", result.Debt.Customer)
	}
	if result.PaymentID == uuid.Nil {
		t.Error("expected a non-zero PaymentID for the recorded payment")
	}
	if result.PaidAt.IsZero() {
		t.Error("expected a non-zero PaidAt timestamp")
	}

	var reloaded domain.Debts
	if err := db.First(&reloaded, "id = ?", debt.ID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if reloaded.TotalDebt != 50000 {
		t.Errorf("total debt = %v, want unchanged 50000 (only remaining_debt should move on payment)", reloaded.TotalDebt)
	}
	if reloaded.RemainingDebt != 30000 {
		t.Errorf("persisted remaining debt = %v, want 30000", reloaded.RemainingDebt)
	}

	var payments []domain.DebtPayments
	if err := db.Where("debt_id = ?", debt.ID).Find(&payments).Error; err != nil {
		t.Fatalf("query debt payments: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("expected 1 debt payment row, got %d", len(payments))
	}
	if payments[0].NominalBayar != 20000 || payments[0].UserID != user.ID {
		t.Errorf("unexpected debt payment row: %+v", payments[0])
	}
}

func TestPayDebt_FullPaymentFlipsStatusToLunas(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewDebtRepository(db)

	customer := &domain.Customers{Name: "Debt Payer " + uuid.NewString()}
	if err := db.Create(customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	debt := &domain.Debts{CustomerID: customer.ID, TotalDebt: 10000, RemainingDebt: 10000, Status: enum.BELUM_LUNAS}
	if err := db.Create(debt).Error; err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	user := &domain.Users{Username: "cashier-" + uuid.NewString(), Password: "x"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&domain.DebtPayments{}, "debt_id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Debts{}, "id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Customers{}, "id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	result, err := repo.PayDebt(context.Background(), debt.ID, &domain.DebtPayments{UserID: user.ID, NominalBayar: 10000})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Debt.RemainingDebt != 0 {
		t.Errorf("remaining debt = %v, want 0", result.Debt.RemainingDebt)
	}
	if result.Debt.Status != enum.LUNAS {
		t.Errorf("expected status LUNAS after paying off the full balance, got %v", result.Debt.Status)
	}
	if result.PreviousRemainingDebt != 10000 {
		t.Errorf("previous remaining debt = %v, want 10000", result.PreviousRemainingDebt)
	}
}

func TestPayDebt_RejectsOverpayment(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewDebtRepository(db)

	customer := &domain.Customers{Name: "Debt Payer " + uuid.NewString()}
	if err := db.Create(customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	debt := &domain.Debts{CustomerID: customer.ID, TotalDebt: 10000, RemainingDebt: 10000, Status: enum.BELUM_LUNAS}
	if err := db.Create(debt).Error; err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	user := &domain.Users{Username: "cashier-" + uuid.NewString(), Password: "x"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&domain.DebtPayments{}, "debt_id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Debts{}, "id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Customers{}, "id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	_, err := repo.PayDebt(context.Background(), debt.ID, &domain.DebtPayments{UserID: user.ID, NominalBayar: 99999})
	if err == nil || !strings.Contains(err.Error(), "exceeds remaining debt") {
		t.Fatalf("expected an overpayment error, got: %v", err)
	}

	var reloaded domain.Debts
	if err := db.First(&reloaded, "id = ?", debt.ID).Error; err != nil {
		t.Fatalf("reload debt: %v", err)
	}
	if reloaded.RemainingDebt != 10000 {
		t.Errorf("remaining debt = %v, want unchanged 10000 after a rejected overpayment", reloaded.RemainingDebt)
	}

	var count int64
	db.Model(&domain.DebtPayments{}).Where("debt_id = ?", debt.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected no debt_payments row for a rejected payment, found %d", count)
	}
}

func TestPayDebt_RejectsPaymentOnAlreadyLunasDebt(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewDebtRepository(db)

	customer := &domain.Customers{Name: "Debt Payer " + uuid.NewString()}
	if err := db.Create(customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	debt := &domain.Debts{CustomerID: customer.ID, TotalDebt: 10000, RemainingDebt: 0, Status: enum.LUNAS}
	if err := db.Create(debt).Error; err != nil {
		t.Fatalf("seed debt: %v", err)
	}
	user := &domain.Users{Username: "cashier-" + uuid.NewString(), Password: "x"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&domain.DebtPayments{}, "debt_id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Debts{}, "id = ?", debt.ID)
		db.Unscoped().Delete(&domain.Customers{}, "id = ?", customer.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	_, err := repo.PayDebt(context.Background(), debt.ID, &domain.DebtPayments{UserID: user.ID, NominalBayar: 5000})
	if err == nil || !strings.Contains(err.Error(), "already been fully paid") {
		t.Fatalf("expected an already-paid error, got: %v", err)
	}
}

func TestPayDebt_NotFound(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewDebtRepository(db)

	_, err := repo.PayDebt(context.Background(), uuid.New(), &domain.DebtPayments{UserID: uuid.New(), NominalBayar: 1000})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found error, got: %v", err)
	}
}
