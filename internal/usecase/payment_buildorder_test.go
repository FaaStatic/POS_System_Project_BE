package usecase

import (
	"context"
	"strings"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeProductRepo struct {
	domain.ProductRepository
	product *domain.Products
}

func (f fakeProductRepo) GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*domain.Products, error) {
	found := make(map[uuid.UUID]*domain.Products, len(ids))
	for _, id := range ids {
		p := *f.product
		p.ID = id
		found[id] = &p
	}
	return found, nil
}

func TestBuildOrder_RejectsDigitalProduct(t *testing.T) {
	u := &paymentUsecase{
		productRepo: fakeProductRepo{product: &domain.Products{
			ProductType:  enum.Digital,
			SellingPrice: 10000,
			Stock:        0,
		}},
		log: zap.NewNop(),
	}

	_, _, err := u.buildOrder(context.Background(), []itemPair{{productID: uuid.NewString(), qty: 1}})
	if err == nil {
		t.Fatal("expected buildOrder to reject a digital product, got nil error")
	}
	if !strings.Contains(err.Error(), "digital") {
		t.Fatalf("expected error to mention digital product, got: %v", err)
	}
}

func TestBuildOrder_AllowsPhysicalProduct(t *testing.T) {
	u := &paymentUsecase{
		productRepo: fakeProductRepo{product: &domain.Products{
			ProductType:  enum.Physical,
			SellingPrice: 10000,
			Stock:        5,
		}},
		log: zap.NewNop(),
	}

	gross, items, err := u.buildOrder(context.Background(), []itemPair{{productID: uuid.NewString(), qty: 1}})
	if err != nil {
		t.Fatalf("expected no error for physical product, got: %v", err)
	}
	if gross != 10000 {
		t.Fatalf("expected gross 10000, got %d", gross)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 payment item, got %d", len(items))
	}
}

func TestSameItems(t *testing.T) {
	beras, gula := uuid.New(), uuid.New()
	items := []domain.PaymentItem{{ProductID: beras, Qty: 2}, {ProductID: gula, Qty: 1.5}}

	cases := []struct {
		name    string
		details []domain.TransactionsDetail
		want    bool
	}{
		{"same items, different order", []domain.TransactionsDetail{{ProductID: gula, Qty: 1.5}, {ProductID: beras, Qty: 2}}, true},
		{"different qty", []domain.TransactionsDetail{{ProductID: beras, Qty: 1}, {ProductID: gula, Qty: 1.5}}, false},
		{"different product", []domain.TransactionsDetail{{ProductID: beras, Qty: 2}, {ProductID: uuid.New(), Qty: 1.5}}, false},
		{"fewer lines", []domain.TransactionsDetail{{ProductID: beras, Qty: 2}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameItems(tc.details, items); got != tc.want {
				t.Errorf("sameItems = %v, want %v", got, tc.want)
			}
		})
	}
}
