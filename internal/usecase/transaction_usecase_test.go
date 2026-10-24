package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	errNotFoundFixture = errors.New("transaction with id xxx not found")
	errBoomFixture     = errors.New("boom: connection reset by peer")
)

func wrapInternal(err error) error {
	return fmt.Errorf("%w: %v", domain.ErrInternal, err)
}

type fakeTrxRepo struct {
	domain.TransactionRepository

	createErr          error
	createDebtSnap     *domain.TransactionDebtSnapshot
	created            *domain.Transactions
	createdIsHutang    bool
	createdDeductStock bool

	deleteErr error
	deletedID uuid.UUID

	getByIDResult *domain.Transactions
	getByIDErr    error
}

func (f *fakeTrxRepo) CreateTransaction(ctx context.Context, transaction *domain.Transactions, isHutang bool, deductStock bool) (*domain.TransactionDebtSnapshot, error) {
	f.created = transaction
	f.createdIsHutang = isHutang
	f.createdDeductStock = deductStock
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createDebtSnap, nil
}

func (f *fakeTrxRepo) DeleteTransaction(ctx context.Context, id uuid.UUID, reason string) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeTrxRepo) GetTransactionByID(ctx context.Context, id uuid.UUID) (*domain.Transactions, error) {
	return f.getByIDResult, f.getByIDErr
}

type fakeTrxUserRepo struct {
	domain.UserRepository
	user *domain.Users
	err  error
}

func (f *fakeTrxUserRepo) GetUserById(ctx context.Context, id uuid.UUID) (*domain.Users, error) {
	return f.user, f.err
}

type fakeTrxProductRepo struct {
	domain.ProductRepository
	products map[uuid.UUID]*domain.Products
}

func (f *fakeTrxProductRepo) GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*domain.Products, error) {
	found := make(map[uuid.UUID]*domain.Products, len(ids))
	for _, id := range ids {
		if p, ok := f.products[id]; ok {
			found[id] = p
		}
	}
	return found, nil
}

func newTestTransactionUsecase(trx *fakeTrxRepo, prod *fakeTrxProductRepo, user *fakeTrxUserRepo) *transactionUsecase {
	return &transactionUsecase{
		trxRepo:     trx,
		productRepo: prod,
		userRepo:    user,
		storeName:   "Test Store",
		log:         zap.NewNop(),
	}
}

func validUser() *domain.Users {
	return &domain.Users{ID: uuid.New(), Username: "kasir1"}
}

func TestAddTransaction_CashSale_Success(t *testing.T) {
	productID := uuid.New()
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, ProductName: "Beras", SellingPrice: 12000, SellingPriceDebt: 13000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0001",
		TypePayment: "tunai",
		UserId:      userRepo.user.ID.String(),
		Details: []requestdto.AddTransactionDetailRequest{
			{ProductId: productID.String(), Qty: 2},
		},
	}

	resp, err := u.AddTransaction(context.Background(), dto)
	if err != nil {
		t.Fatalf("expected no error for cash sale, got: %v", err)
	}

	if trxRepo.created == nil {
		t.Fatal("expected CreateTransaction to be called")
	}
	if trxRepo.createdIsHutang {
		t.Error("cash sale must not be flagged as hutang")
	}
	if !trxRepo.createdDeductStock {
		t.Error("AddTransaction must deduct stock (deductStock=true)")
	}
	wantTotal := int64(24000)
	if trxRepo.created.TotalTransaction != wantTotal {
		t.Errorf("total = %v, want %v (must use SellingPrice for cash)", trxRepo.created.TotalTransaction, wantTotal)
	}
	if trxRepo.created.CustomerID != nil {
		t.Error("cash sale should not require/attach a customer")
	}
	if resp.DebtInfo != nil {
		t.Errorf("a cash sale must not include DebtInfo in the response, got: %+v", resp.DebtInfo)
	}
	if resp.TotalTransaction != wantTotal || resp.PaymentType != "tunai" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestAddTransaction_HutangSale_UsesDebtPriceAndRequiresCustomer(t *testing.T) {
	productID := uuid.New()
	customerID := uuid.New()
	debtID := uuid.New()
	trxRepo := &fakeTrxRepo{createDebtSnap: &domain.TransactionDebtSnapshot{
		DebtID:                debtID,
		PreviousRemainingDebt: 0,
		AmountAdded:           39000,
		TotalDebt:             39000,
		RemainingDebt:         39000,
		Status:                enum.BELUM_LUNAS,
	}}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, ProductName: "Beras", SellingPrice: 12000, SellingPriceDebt: 13000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	customerIDStr := customerID.String()
	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0002",
		TypePayment: "hutang",
		UserId:      userRepo.user.ID.String(),
		CustomerId:  &customerIDStr,
		Details: []requestdto.AddTransactionDetailRequest{
			{ProductId: productID.String(), Qty: 3},
		},
	}

	resp, err := u.AddTransaction(context.Background(), dto)
	if err != nil {
		t.Fatalf("expected no error for hutang sale, got: %v", err)
	}

	if !trxRepo.createdIsHutang {
		t.Error("expected isHutang=true to be passed to CreateTransaction")
	}
	if !trxRepo.createdDeductStock {
		t.Error("a hutang POS sale still deducts stock immediately")
	}
	wantTotal := int64(39000)
	if trxRepo.created.TotalTransaction != wantTotal {
		t.Errorf("total = %v, want %v (hutang must use SellingPriceDebt)", trxRepo.created.TotalTransaction, wantTotal)
	}
	if trxRepo.created.CustomerID == nil || *trxRepo.created.CustomerID != customerID {
		t.Error("hutang transaction must be linked to the given customer")
	}

	if resp.DebtInfo == nil {
		t.Fatal("expected DebtInfo to be populated for a hutang sale")
	}
	if resp.DebtInfo.DebtID != debtID.String() {
		t.Errorf("debt id = %s, want %s", resp.DebtInfo.DebtID, debtID)
	}
	if resp.DebtInfo.PreviousRemainingDebt != 0 {
		t.Errorf("previous remaining debt = %d, want 0 (first debt for this customer)", resp.DebtInfo.PreviousRemainingDebt)
	}
	if resp.DebtInfo.AmountAdded != 39000 || resp.DebtInfo.RemainingDebt != 39000 || resp.DebtInfo.TotalDebt != 39000 {
		t.Errorf("unexpected debt info: %+v", resp.DebtInfo)
	}
	if resp.DebtInfo.Status != enum.BELUM_LUNAS.String() {
		t.Errorf("debt info status = %s, want %s", resp.DebtInfo.Status, enum.BELUM_LUNAS.String())
	}
}

func TestAddTransaction_HutangSale_MissingCustomer(t *testing.T) {
	productID := uuid.New()
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, SellingPrice: 12000, SellingPriceDebt: 13000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0003",
		TypePayment: "hutang",
		UserId:      userRepo.user.ID.String(),
		Details: []requestdto.AddTransactionDetailRequest{
			{ProductId: productID.String(), Qty: 1},
		},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if err == nil {
		t.Fatal("expected error when hutang sale has no customer id")
	}
	if !strings.Contains(err.Error(), "customer id is required") {
		t.Errorf("unexpected error message: %v", err)
	}
	if trxRepo.created != nil {
		t.Error("CreateTransaction must not be called when validation fails")
	}
}

func TestAddTransaction_HutangSale_CustomerNotFound(t *testing.T) {
	productID := uuid.New()
	customerID := uuid.New().String()
	trxRepo := &fakeTrxRepo{createErr: domain.NotFound("customer with id " + customerID + " not found")}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, SellingPrice: 12000, SellingPriceDebt: 13000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0004",
		TypePayment: "hutang",
		UserId:      userRepo.user.ID.String(),
		CustomerId:  &customerID,
		Details: []requestdto.AddTransactionDetailRequest{
			{ProductId: productID.String(), Qty: 1},
		},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected customer not found error, got: %v", err)
	}
}

func TestAddTransaction_DuplicateInvoice(t *testing.T) {
	productID := uuid.New()
	trxRepo := &fakeTrxRepo{createErr: domain.Duplicate("transaction with no invoice INV-DUP-1 already exists")}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, SellingPrice: 12000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "DUP-1",
		TypePayment: "tunai",
		UserId:      userRepo.user.ID.String(),
		Details:     []requestdto.AddTransactionDetailRequest{{ProductId: productID.String(), Qty: 1}},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate invoice error, got: %v", err)
	}
}

func TestAddTransaction_ProductNotFound(t *testing.T) {
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0005",
		TypePayment: "tunai",
		UserId:      userRepo.user.ID.String(),
		Details:     []requestdto.AddTransactionDetailRequest{{ProductId: uuid.New().String(), Qty: 1}},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected product not found error, got: %v", err)
	}
}

func TestAddTransaction_DigitalProductRequiresDestination(t *testing.T) {
	productID := uuid.New()
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, ProductType: 1, SellingPrice: 5000},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0006",
		TypePayment: "tunai",
		UserId:      userRepo.user.ID.String(),
		Details:     []requestdto.AddTransactionDetailRequest{{ProductId: productID.String(), Qty: 1}},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if err == nil || !strings.Contains(err.Error(), "destination is required") {
		t.Fatalf("expected destination-required error for digital product, got: %v", err)
	}
}

func TestAddTransaction_InvalidPaymentType(t *testing.T) {
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "0007",
		TypePayment: "bitcoin",
		UserId:      userRepo.user.ID.String(),
		Details:     []requestdto.AddTransactionDetailRequest{{ProductId: uuid.New().String(), Qty: 1}},
	}

	_, err := u.AddTransaction(context.Background(), dto)
	if err == nil {
		t.Fatal("expected error for an unsupported payment type")
	}
}

func TestAddPrepaidTransaction_DoesNotDeductStockAgain(t *testing.T) {
	productID := uuid.New()
	trxRepo := &fakeTrxRepo{}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, SellingPrice: 12000, SellingPriceDebt: 13000, Stock: 10},
	}}
	userRepo := &fakeTrxUserRepo{user: validUser()}

	u := newTestTransactionUsecase(trxRepo, prodRepo, userRepo)

	dto := &requestdto.AddTransactionRequest{
		NoInvoice:   "INV-ORDER-1",
		TypePayment: "tunai",
		UserId:      userRepo.user.ID.String(),
		Details:     []requestdto.AddTransactionDetailRequest{{ProductId: productID.String(), Qty: 1}},
	}

	if _, err := u.AddPrepaidTransaction(context.Background(), dto); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if trxRepo.createdDeductStock {
		t.Error("AddPrepaidTransaction must pass deductStock=false: stock was already reserved at charge time")
	}
}

func TestDeleteTransaction_Success(t *testing.T) {
	trxRepo := &fakeTrxRepo{}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	id := uuid.New()
	if err := u.DeleteTransaction(context.Background(), &requestdto.DeleteTransactionRequest{Role: "superadmin", ID: id.String()}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if trxRepo.deletedID != id {
		t.Errorf("expected DeleteTransaction to be called with id %s, got %s", id, trxRepo.deletedID)
	}
}

func TestDeleteTransaction_InvalidID(t *testing.T) {
	trxRepo := &fakeTrxRepo{}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	err := u.DeleteTransaction(context.Background(), &requestdto.DeleteTransactionRequest{Role: "superadmin", ID: "not-a-uuid"})
	if err == nil {
		t.Fatal("expected error for invalid transaction id")
	}
}

func TestDeleteTransaction_NotFoundPropagatesRepoError(t *testing.T) {
	trxRepo := &fakeTrxRepo{deleteErr: errNotFoundFixture}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	err := u.DeleteTransaction(context.Background(), &requestdto.DeleteTransactionRequest{Role: "superadmin", ID: uuid.New().String()})
	if err == nil || err.Error() != errNotFoundFixture.Error() {
		t.Fatalf("expected the repository's not-found error to pass through, got: %v", err)
	}
}

func TestDeleteTransaction_InternalErrorIsHidden(t *testing.T) {
	trxRepo := &fakeTrxRepo{createErr: nil, deleteErr: wrapInternal(errBoomFixture)}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	err := u.DeleteTransaction(context.Background(), &requestdto.DeleteTransactionRequest{Role: "superadmin", ID: uuid.New().String()})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "boom") {
		t.Errorf("internal/driver error detail must not leak to the caller, got: %v", err)
	}
}

func TestGetTransaction_NotFound(t *testing.T) {
	trxRepo := &fakeTrxRepo{getByIDErr: domain.NotFound("transaction not found")}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	_, err := u.GetTransaction(context.Background(), &requestdto.GetTransactionRequest{ID: uuid.New().String()})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestGetTransaction_Success(t *testing.T) {
	trxID := uuid.New()
	trxRepo := &fakeTrxRepo{getByIDResult: &domain.Transactions{
		ID:               trxID,
		NoInvoice:        "INV-1",
		TotalTransaction: 5000,
		TransactionDetail: []domain.TransactionsDetail{
			{Qty: 1, Price: 5000, Subtotal: 5000, ProductName: "Air Mineral"},
		},
	}}
	u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	resp, err := u.GetTransaction(context.Background(), &requestdto.GetTransactionRequest{ID: trxID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.InvoiceNumber != "INV-1" || resp.TotalTransaction != 5000 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(resp.TransactionDetails) != 1 || resp.TransactionDetails[0].ProductName != "Air Mineral" {
		t.Errorf("unexpected transaction details: %+v", resp.TransactionDetails)
	}
}

func TestMergeDailyReport_KeepsPaymentOnlyDaysAndSortsByDate(t *testing.T) {
	day1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	daily := mergeDailyReport(
		[]domain.DailyReport{
			{Date: day3, CountSale: 1, Revenue: 5000, Cost: 4000},
			{Date: day1, CountDebt: 2, DebtSales: 21000, DebtCost: 16000},
		},
		[]domain.DailyDebtFlow{
			{Date: day1, Manual: 50000},
			{Date: day2, Paid: 10000},
		},
	)

	if len(daily) != 3 {
		t.Fatalf("got %d rows, want 3 (payment-only day must appear)", len(daily))
	}
	for i, want := range []time.Time{day1, day2, day3} {
		if !daily[i].Date.Equal(want) {
			t.Errorf("row %d date = %v, want %v", i, daily[i].Date, want)
		}
	}
	if daily[0].DebtSales != 21000 || daily[0].DebtManual != 50000 {
		t.Errorf("day1 must merge sales and manual debt: %+v", daily[0])
	}
	if daily[1].DebtPaid != 10000 || daily[1].Revenue != 0 {
		t.Errorf("day2 must hold only the payment: %+v", daily[1])
	}
}

func TestDeleteTransaction_StaffRules(t *testing.T) {
	staffID := uuid.New()
	now := time.Now()
	cases := []struct {
		name      string
		owner     uuid.UUID
		createdAt time.Time
		wantErr   error
	}{
		{"own transaction today", staffID, now, nil},
		{"someone else's transaction", uuid.New(), now, domain.ErrForbidden},
		{"own transaction from yesterday", staffID, now.AddDate(0, 0, -1), domain.ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trxID := uuid.New()
			trxRepo := &fakeTrxRepo{getByIDResult: &domain.Transactions{ID: trxID, UserID: tc.owner, CreatedAt: tc.createdAt}}
			u := newTestTransactionUsecase(trxRepo, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

			err := u.DeleteTransaction(context.Background(), &requestdto.DeleteTransactionRequest{
				ID: trxID.String(), UserID: staffID.String(), Role: "staff",
			})
			if tc.wantErr == nil {
				if err != nil || trxRepo.deletedID != trxID {
					t.Fatalf("staff must cancel own same-day transaction; err=%v deleted=%s", err, trxRepo.deletedID)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if trxRepo.deletedID != uuid.Nil {
				t.Error("repository delete must not run when the staff rule rejects")
			}
		})
	}
}
