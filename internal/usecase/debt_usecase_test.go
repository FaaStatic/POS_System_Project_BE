package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeDebtRepo struct {
	domain.DebtRepository

	addErr    error
	added     *domain.Debts
	deleteErr error
	deletedID uuid.UUID

	getByIDResult *domain.Debts
	getByIDErr    error

	getAllResult *domain.DebtsPaginated
	getAllErr    error

	payErr     error
	payResult  *domain.DebtPaymentResult
	payDebtID  uuid.UUID
	payPayment *domain.DebtPayments
}

func (f *fakeDebtRepo) AddDebt(ctx context.Context, debt *domain.Debts) error {
	f.added = debt
	return f.addErr
}

func (f *fakeDebtRepo) DeleteDebt(ctx context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeDebtRepo) GetDebtByID(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	return f.getByIDResult, f.getByIDErr
}

func (f *fakeDebtRepo) GetDebtWithPayments(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	return f.getByIDResult, f.getByIDErr
}

func (f *fakeDebtRepo) GetAllDebt(ctx context.Context, filter domain.FilterDebt) (*domain.DebtsPaginated, error) {
	return f.getAllResult, f.getAllErr
}

func (f *fakeDebtRepo) PayDebt(ctx context.Context, debtID uuid.UUID, payment *domain.DebtPayments) (*domain.DebtPaymentResult, error) {
	f.payDebtID = debtID
	f.payPayment = payment
	return f.payResult, f.payErr
}

func newTestDebtUsecase(repo *fakeDebtRepo) *debtUsecase {
	return &debtUsecase{debtRepo: repo, log: zap.NewNop()}
}

func TestAddingDebtCustomer_Success(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	customerID := uuid.New().String()
	req := &requestdto.AddDebtRequest{
		CustomerID:     customerID,
		TotalTransaksi: 50000,
		JatuhTempo:     "2026-08-01",
	}
	if err := u.AddingDebtCustomer(context.Background(), req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.added == nil {
		t.Fatal("expected AddDebt to be called")
	}
	if repo.added.TotalDebt != 50000 || repo.added.RemainingDebt != 50000 {
		t.Errorf("total=%v remaining=%v, want both 50000", repo.added.TotalDebt, repo.added.RemainingDebt)
	}
	if repo.added.Status != enum.BELUM_LUNAS {
		t.Errorf("expected status BELUM_LUNAS on creation, got %v", repo.added.Status)
	}
}

func TestAddingDebtCustomer_InvalidCustomerID(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	err := u.AddingDebtCustomer(context.Background(), &requestdto.AddDebtRequest{
		CustomerID: "not-a-uuid", TotalTransaksi: 1000, JatuhTempo: "2026-08-01",
	})
	if err == nil {
		t.Fatal("expected error for invalid customer id")
	}
	if repo.added != nil {
		t.Error("AddDebt must not be called when validation fails")
	}
}

func TestAddingDebtCustomer_InvalidDueDate(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	err := u.AddingDebtCustomer(context.Background(), &requestdto.AddDebtRequest{
		CustomerID: uuid.New().String(), TotalTransaksi: 1000, JatuhTempo: "01/08/2026",
	})
	if err == nil || !strings.Contains(err.Error(), "jatuh_tempo") {
		t.Fatalf("expected a jatuh_tempo format error, got: %v", err)
	}
}

func TestDeleteDebtCustomer_Success(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	id := uuid.New()
	if err := u.DeleteDebtCustomer(context.Background(), &requestdto.DeleteDebtRequest{DebtId: id.String()}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.deletedID != id {
		t.Errorf("expected DeleteDebt called with %s, got %s", id, repo.deletedID)
	}
}

func TestDeleteDebtCustomer_InvalidID(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	err := u.DeleteDebtCustomer(context.Background(), &requestdto.DeleteDebtRequest{DebtId: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid debt id")
	}
}

func TestGetDebtCustomer_NotFound(t *testing.T) {
	repo := &fakeDebtRepo{getByIDErr: domain.NotFound("debt not found")}
	u := newTestDebtUsecase(repo)

	_, err := u.GetDebtCustomer(context.Background(), &requestdto.GetDebtRequest{DebtId: uuid.New().String()})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestGetDebtCustomer_Success(t *testing.T) {
	repo := &fakeDebtRepo{getByIDResult: &domain.Debts{
		TotalDebt:     30000,
		RemainingDebt: 12000,
		Customer:      domain.Customers{Name: "Budi"},
	}}
	u := newTestDebtUsecase(repo)

	resp, err := u.GetDebtCustomer(context.Background(), &requestdto.GetDebtRequest{DebtId: uuid.New().String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.NameCustomer != "Budi" {
		t.Errorf("expected customer name Budi, got %s", resp.NameCustomer)
	}
	if resp.TotalDebt != 30000 || resp.RemainingDebt != 12000 {
		t.Errorf("unexpected formatted amounts: total=%d remaining=%d", resp.TotalDebt, resp.RemainingDebt)
	}
}

func TestGetAllDebtCustomerList_Success(t *testing.T) {
	repo := &fakeDebtRepo{getAllResult: &domain.DebtsPaginated{
		Data: []*domain.Debts{
			{TotalDebt: 1000, RemainingDebt: 1000, Customer: domain.Customers{Name: "A"}},
			{TotalDebt: 2000, RemainingDebt: 0, Status: enum.LUNAS, Customer: domain.Customers{Name: "B"}},
		},
		HasNext: false,
	}}
	u := newTestDebtUsecase(repo)

	resp, err := u.GetAllDebtCustomerList(context.Background(), &requestdto.FilterDebtRequest{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(resp.TransactionList) != 2 {
		t.Fatalf("expected 2 debt entries, got %d", len(resp.TransactionList))
	}
	if resp.HasNext {
		t.Error("expected HasNext=false")
	}
}

func TestPayDebtCash_Success(t *testing.T) {
	debtID := uuid.New()
	userID := uuid.New()
	paidAt := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	repo := &fakeDebtRepo{payResult: &domain.DebtPaymentResult{
		Debt: &domain.Debts{
			ID:            debtID,
			TotalDebt:     50000,
			RemainingDebt: 5000,
			Status:        enum.BELUM_LUNAS,
			Customer:      domain.Customers{Name: "Budi"},
		},
		PreviousRemainingDebt: 20000,
		PaidAt:                paidAt,
	}}
	u := newTestDebtUsecase(repo)

	req := &requestdto.DebtPayment{
		DebtID:       debtID.String(),
		UserID:       userID.String(),
		NominalBayar: 15000,
	}
	resp, err := u.PayDebtCash(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.payDebtID != debtID {
		t.Errorf("expected PayDebt called with debt id %s, got %s", debtID, repo.payDebtID)
	}
	if repo.payPayment == nil || repo.payPayment.UserID != userID || repo.payPayment.NominalBayar != 15000 {
		t.Errorf("unexpected payment passed to repository: %+v", repo.payPayment)
	}
	if resp.PreviousRemainingDebt != 20000 {
		t.Errorf("expected previous remaining debt 20000, got %d", resp.PreviousRemainingDebt)
	}
	if resp.RemainingDebt != 5000 {
		t.Errorf("expected formatted remaining debt 5000, got %d", resp.RemainingDebt)
	}
	if resp.TotalDebt != 50000 {
		t.Errorf("expected formatted total debt 50000, got %d", resp.TotalDebt)
	}
	if resp.NominalBayar != 15000 {
		t.Errorf("expected formatted nominal_bayar 15000, got %d", resp.NominalBayar)
	}
	if resp.CustomerName != "Budi" {
		t.Errorf("expected customer name Budi on the receipt, got %s", resp.CustomerName)
	}
	if resp.PaidAt != paidAt.Format(time.RFC3339) {
		t.Errorf("expected paid_at %s, got %s", paidAt.Format(time.RFC3339), resp.PaidAt)
	}
	if resp.Status != enum.BELUM_LUNAS.String() {
		t.Errorf("expected status %s, got %s", enum.BELUM_LUNAS.String(), resp.Status)
	}
}

func TestPayDebtCash_FullyPaidFlipsStatusToLunas(t *testing.T) {
	debtID := uuid.New()
	repo := &fakeDebtRepo{payResult: &domain.DebtPaymentResult{
		Debt: &domain.Debts{
			ID:            debtID,
			RemainingDebt: 0,
			Status:        enum.LUNAS,
		},
		PreviousRemainingDebt: 20000,
	}}
	u := newTestDebtUsecase(repo)

	resp, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
		DebtID: debtID.String(), UserID: uuid.New().String(), NominalBayar: 20000,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Status != enum.LUNAS.String() {
		t.Errorf("expected status LUNAS once remaining debt reaches 0, got %s", resp.Status)
	}
	if resp.RemainingDebt != 0 {
		t.Errorf("expected remaining debt 0, got %d", resp.RemainingDebt)
	}
	if resp.PreviousRemainingDebt != 20000 {
		t.Errorf("expected previous remaining debt 20000, got %d", resp.PreviousRemainingDebt)
	}
}

func TestPayDebtCash_InvalidDebtID(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	_, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
		DebtID: "not-a-uuid", UserID: uuid.New().String(), NominalBayar: 1000,
	})
	if err == nil {
		t.Fatal("expected error for invalid debt id")
	}
	if repo.payPayment != nil {
		t.Error("PayDebt must not be called when validation fails")
	}
}

func TestPayDebtCash_InvalidUserID(t *testing.T) {
	repo := &fakeDebtRepo{}
	u := newTestDebtUsecase(repo)

	_, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
		DebtID: uuid.New().String(), UserID: "not-a-uuid", NominalBayar: 1000,
	})
	if err == nil {
		t.Fatal("expected error for invalid user id")
	}
}

func TestPayDebtCash_RejectsNonPositiveNominal(t *testing.T) {
	tests := []int64{0, -100}
	for _, nominal := range tests {
		repo := &fakeDebtRepo{}
		u := newTestDebtUsecase(repo)

		_, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
			DebtID: uuid.New().String(), UserID: uuid.New().String(), NominalBayar: nominal,
		})
		if err == nil {
			t.Errorf("expected error for nominal_bayar=%v", nominal)
		}
		if repo.payPayment != nil {
			t.Errorf("PayDebt must not be called for nominal_bayar=%v", nominal)
		}
	}
}

func TestPayDebtCash_OverpaymentRejectedByRepository(t *testing.T) {
	repo := &fakeDebtRepo{payErr: errOverpaymentFixture}
	u := newTestDebtUsecase(repo)

	_, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
		DebtID: uuid.New().String(), UserID: uuid.New().String(), NominalBayar: 999999,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds remaining debt") {
		t.Fatalf("expected the overpayment error to pass through, got: %v", err)
	}
}

func TestPayDebtCash_InternalErrorIsHidden(t *testing.T) {
	repo := &fakeDebtRepo{payErr: wrapInternal(errBoomFixture)}
	u := newTestDebtUsecase(repo)

	_, err := u.PayDebtCash(context.Background(), &requestdto.DebtPayment{
		DebtID: uuid.New().String(), UserID: uuid.New().String(), NominalBayar: 1000,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "boom") {
		t.Errorf("driver detail must not leak to the caller, got: %v", err)
	}
}

var errOverpaymentFixture = errors.New("payment amount (999999.00) exceeds remaining debt (5000.00)")
