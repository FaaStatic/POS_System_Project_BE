package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
)

type fakeSyncTrxRepo struct {
	fakeTrxRepo
	byRef   map[uuid.UUID]*domain.Transactions
	refErr  error
	creates int
}

func (f *fakeSyncTrxRepo) GetTransactionByClientRef(ctx context.Context, ref uuid.UUID) (*domain.Transactions, error) {
	if f.refErr != nil {
		return nil, f.refErr
	}
	return f.byRef[ref], nil
}

func (f *fakeSyncTrxRepo) CreateTransaction(ctx context.Context, trx *domain.Transactions, isHutang, deductStock bool) (*domain.TransactionDebtSnapshot, error) {
	f.creates++
	trx.ID = uuid.New()
	return f.fakeTrxRepo.CreateTransaction(ctx, trx, isHutang, deductStock)
}

func offlineItem(ref, productID string) requestdto.OfflineTransaction {
	return requestdto.OfflineTransaction{
		ClientRef:       ref,
		CreatedAtClient: "2026-10-20T09:15:00+07:00",
		TypePayment:     "tunai",
		Details:         []requestdto.OfflineTransactionDetail{{ProductId: productID, Qty: 2, UnitPrice: 11000}},
	}
}

func TestParseOfflineTransaction(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, domain.StoreLocation)
	ref := uuid.NewString()

	sale, req, err := parseOfflineTransaction("u1", offlineItem(ref, uuid.NewString()), now)
	if err != nil {
		t.Fatalf("valid item rejected: %v", err)
	}
	if sale.clientRef.String() != ref || sale.soldAt.Day() != 20 {
		t.Errorf("sale = %+v; want client_ref %s sold on the 20th", sale, ref)
	}
	if req.UserId != "u1" || *req.Details[0].UnitPrice != 11000 {
		t.Errorf("request lost user or snapshot price: %+v", req)
	}

	future := offlineItem(ref, uuid.NewString())
	future.CreatedAtClient = now.Add(time.Hour).Format(time.RFC3339)
	if sale, _, _ := parseOfflineTransaction("u1", future, now); !sale.soldAt.Equal(now) {
		t.Errorf("future device clock not clamped: got %v want %v", sale.soldAt, now)
	}
	stale := offlineItem(ref, uuid.NewString())
	stale.CreatedAtClient = "2000-01-01T08:00:00+07:00"
	if sale, _, _ := parseOfflineTransaction("u1", stale, now); !sale.soldAt.Equal(now) {
		t.Errorf("reset device clock not clamped: got %v want %v", sale.soldAt, now)
	}

	bad := map[string]func(*requestdto.OfflineTransaction){
		"client_ref not uuid": func(i *requestdto.OfflineTransaction) { i.ClientRef = "abc" },
		"qris offline":        func(i *requestdto.OfflineTransaction) { i.TypePayment = "qris" },
		"bad time":            func(i *requestdto.OfflineTransaction) { i.CreatedAtClient = "20/10/2026" },
		"no details":          func(i *requestdto.OfflineTransaction) { i.Details = nil },
		"zero qty":            func(i *requestdto.OfflineTransaction) { i.Details[0].Qty = 0 },
		"negative price":      func(i *requestdto.OfflineTransaction) { i.Details[0].UnitPrice = -1 },
	}
	for name, mutate := range bad {
		item := offlineItem(ref, uuid.NewString())
		mutate(&item)
		if _, _, err := parseOfflineTransaction("u1", item, now); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: err = %v; want validation error", name, err)
		}
	}
}

func TestSyncOfflineTransactions(t *testing.T) {
	productID := uuid.New()
	known := uuid.New()
	repo := &fakeSyncTrxRepo{byRef: map[uuid.UUID]*domain.Transactions{
		known: {ID: uuid.New(), NoInvoice: "INV-OLD", StockConflict: true},
	}}
	prodRepo := &fakeTrxProductRepo{products: map[uuid.UUID]*domain.Products{
		productID: {ID: productID, ProductName: "Gula", SellingPrice: 15000, SellingPriceDebt: 16000},
	}}
	user := validUser()
	u := newTestTransactionUsecase(&repo.fakeTrxRepo, prodRepo, &fakeTrxUserRepo{user: user})
	u.trxRepo = repo

	newRef := uuid.NewString()
	resp, err := u.SyncOfflineTransactions(context.Background(), &requestdto.SyncTransactionsRequest{
		UserId: user.ID.String(),
		Transactions: []requestdto.OfflineTransaction{
			offlineItem(newRef, productID.String()),
			offlineItem(known.String(), productID.String()),
			offlineItem(uuid.NewString(), uuid.NewString()),
		},
	})
	if err != nil {
		t.Fatalf("sync returned error: %v", err)
	}
	got := resp.Results

	if got[0].Status != syncCreated || repo.created.ClientRef == nil || repo.created.ClientRef.String() != newRef {
		t.Errorf("new item: %+v; want created with client_ref stored", got[0])
	}
	if repo.created.TransactionDetail[0].Price != 11000 {
		t.Errorf("offline sale priced at %d; want the snapshot 11000, not the current 15000", repo.created.TransactionDetail[0].Price)
	}
	if got[1].Status != syncDuplicate || got[1].NoInvoice != "INV-OLD" || !got[1].StockConflict {
		t.Errorf("known client_ref: %+v; want duplicate pointing at INV-OLD", got[1])
	}
	if got[2].Status != syncFailed || got[2].Retryable {
		t.Errorf("unknown product: %+v; want permanent failure", got[2])
	}
	if repo.creates != 1 {
		t.Errorf("CreateTransaction called %d times; only the new item may be created", repo.creates)
	}

	repo.refErr = errors.New("db down")
	resp, _ = u.SyncOfflineTransactions(context.Background(), &requestdto.SyncTransactionsRequest{
		UserId: user.ID.String(), Transactions: []requestdto.OfflineTransaction{offlineItem(uuid.NewString(), productID.String())},
	})
	if r := resp.Results[0]; r.Status != syncFailed || !r.Retryable {
		t.Errorf("lookup failure: %+v; want retryable failure", r)
	}
}
