package usecase

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeFilterTrxRepo struct {
	domain.TransactionRepository
	gotFilter domain.FilterTransaction
}

func (f *fakeFilterTrxRepo) GetAllTransaction(ctx context.Context, filter domain.FilterTransaction) (*domain.ResultTransaction, error) {
	f.gotFilter = filter
	return &domain.ResultTransaction{DataItem: nil, HasNext: false}, nil
}

func TestGetAllTransaction_PaymentTypeFilterIsOptional(t *testing.T) {
	tunai, hutang := 0, 1

	tests := []struct {
		name        string
		typePayment *int
		wantApplied bool
		wantValue   int
	}{
		{name: "omitted means all payment types", typePayment: nil, wantApplied: false},
		{name: "explicit tunai (0) still filters", typePayment: &tunai, wantApplied: true, wantValue: 0},
		{name: "explicit hutang (1) filters", typePayment: &hutang, wantApplied: true, wantValue: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeFilterTrxRepo{}
			u := &transactionUsecase{trxRepo: repo, log: zap.NewNop()}

			_, err := u.GetAllTransaction(context.Background(), &requestdto.FilterTransactionRequest{
				UserId:      uuid.NewString(),
				TypePayment: tt.typePayment,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			applied := repo.gotFilter.TypeTrx != nil
			if applied != tt.wantApplied {
				t.Fatalf("filter applied = %v; want %v", applied, tt.wantApplied)
			}
			if tt.wantApplied && *repo.gotFilter.TypeTrx != tt.wantValue {
				t.Errorf("filter value = %d; want %d", *repo.gotFilter.TypeTrx, tt.wantValue)
			}
		})
	}
}

func TestAddTransaction_PrepaidUsesLockedUnitPrice(t *testing.T) {
	productID := uuid.New()
	lockedPrice := int64(10000)

	newProductRepo := func() *fakeTrxProductRepo {
		return &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
			productID: {
				ID:               productID,
				ProductName:      "Gula",
				SellingPrice:     15000,
				SellingPriceDebt: 16000,
				PurchasePrice:    8000,
				Stock:            50,
			},
		}}
	}

	tests := []struct {
		name      string
		prepaid   bool
		unitPrice *int64
		wantTotal int64
	}{
		{
			name:      "prepaid honours the price locked at charge time",
			prepaid:   true,
			unitPrice: &lockedPrice,
			wantTotal: 20000,
		},
		{
			name:      "prepaid without a snapshot falls back to the catalogue",
			prepaid:   true,
			unitPrice: nil,
			wantTotal: 30000,
		},
		{
			name:      "cash sale ignores a unit price it was somehow handed",
			prepaid:   false,
			unitPrice: &lockedPrice,
			wantTotal: 30000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trxRepo := &fakeTrxRepo{}
			userID := uuid.New()
			u := newTestTransactionUsecase(
				trxRepo,
				newProductRepo(),
				&fakeTrxUserRepo{user: &domain.Users{ID: userID, Username: "kasir"}},
			)

			req := &requestdto.AddTransactionRequest{
				NoInvoice:   "INV-PRICE-TEST",
				TypePayment: "qris",
				UserId:      userID.String(),
				Details: []requestdto.AddTransactionDetailRequest{{
					ProductId: productID.String(),
					Qty:       2,
					UnitPrice: tt.unitPrice,
				}},
			}

			var err error
			if tt.prepaid {
				_, err = u.AddPrepaidTransaction(context.Background(), req)
			} else {
				_, err = u.AddTransaction(context.Background(), req)
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if trxRepo.created == nil {
				t.Fatal("expected a transaction to be created")
			}
			if got := trxRepo.created.TotalTransaction; got != tt.wantTotal {
				t.Errorf("total = %d; want %d", got, tt.wantTotal)
			}
			if trxRepo.createdDeductStock == tt.prepaid {
				t.Errorf("deductStock = %v for prepaid=%v; prepaid stock must not be deducted twice",
					trxRepo.createdDeductStock, tt.prepaid)
			}
		})
	}
}

func TestBuildOrder_FreezesUnitPriceAndReturnsSubtotal(t *testing.T) {
	u := &paymentUsecase{
		productRepo: fakeProductRepo{product: &domain.Products{
			ProductName:  "Beras",
			SellingPrice: 12500,
			Stock:        100,
		}},
		log: zap.NewNop(),
	}

	subtotal, items, err := u.buildOrder(context.Background(), []itemPair{
		{productID: uuid.NewString(), qty: 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subtotal != 37500 {
		t.Errorf("subtotal = %d; want 37500 (12500 x 3, fee excluded)", subtotal)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 payment item, got %d", len(items))
	}
	if items[0].UnitPrice != 12500 {
		t.Errorf("UnitPrice = %d; want the price frozen at charge time (12500)", items[0].UnitPrice)
	}
}

func TestBuildOrder_InsufficientStockIsAConflict(t *testing.T) {
	u := &paymentUsecase{
		productRepo: fakeProductRepo{product: &domain.Products{
			ProductName:  "Beras",
			SellingPrice: 12500,
			Stock:        1,
		}},
		log: zap.NewNop(),
	}

	_, _, err := u.buildOrder(context.Background(), []itemPair{
		{productID: uuid.NewString(), qty: 5},
	})
	if err == nil {
		t.Fatal("expected an error for insufficient stock")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("error = %v; want it to match domain.ErrConflict", err)
	}
}

func TestIsTerminalPaymentStatus(t *testing.T) {
	tests := []struct {
		status domain.PaymentStatus
		want   bool
	}{
		{domain.PaymentPending, false},
		{domain.PaymentSettling, false},
		{domain.PaymentSuccess, true},
		{domain.PaymentFailed, true},
		{domain.PaymentExpired, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := isTerminalPaymentStatus(tt.status); got != tt.want {
				t.Errorf("isTerminalPaymentStatus(%q) = %v; want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestReserveStockError(t *testing.T) {
	tests := []struct {
		name        string
		in          error
		wantMatches error
	}{
		{
			name:        "db failure stays internal",
			in:          wrapInternal(errBoomFixture),
			wantMatches: domain.ErrInternal,
		},
		{
			name:        "genuine shortage stays a conflict",
			in:          domain.Conflict("insufficient stock for product X"),
			wantMatches: domain.ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reserveStockError(tt.in)
			if !errors.Is(got, tt.wantMatches) {
				t.Errorf("reserveStockError(%v) = %v; want it to match %v", tt.in, got, tt.wantMatches)
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"infrastructure failure", wrapInternal(errBoomFixture), true},
		{"validation", domain.Validation("bank is required"), false},
		{"conflict", domain.Conflict("insufficient stock"), false},
		{"not found", domain.NotFound("product not found"), false},
		{"duplicate", domain.Duplicate("invoice exists"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.IsTransient(tt.err); got != tt.want {
				t.Errorf("IsTransient(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}
