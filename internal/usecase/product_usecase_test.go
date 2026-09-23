package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var errInsufficientStockFixture = errors.New("insufficient stock for product X")

type fakeProdRepo struct {
	domain.ProductRepository

	addErr        error
	added         *domain.Products
	deleteErr     error
	deletedID     uuid.UUID
	updateErr     error
	updatedID     uuid.UUID
	updatedFields map[string]interface{}
	updatedDelta  float64
}

func (f *fakeProdRepo) AddProduct(ctx context.Context, product *domain.Products) error {
	f.added = product
	return f.addErr
}

func (f *fakeProdRepo) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakeProdRepo) UpdateProductWithLock(ctx context.Context, id uuid.UUID, fields map[string]interface{}, updatedDelta float64) error {
	f.updatedID = id
	f.updatedFields = fields
	f.updatedDelta = updatedDelta
	return f.updateErr
}

func newTestProductUsecase(repo *fakeProdRepo) *productUsecase {
	return &productUsecase{productRepo: repo, log: zap.NewNop()}
}

func ptr[T any](v T) *T { return &v }

func TestAddProductShopWithLock_Success(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	req := &requestdto.AddProduct{
		SKU: "SKU-1", ProductName: "Gula",
		PurchasePrice: ptr(int64(10000)), SellingPrice: ptr(int64(13000)),
		SellingPriceDebt: ptr(int64(14000)), Stock: ptr(float64(20)), Category: "sembako",
	}
	if err := u.AddProductShopWithLock(context.Background(), req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.added == nil || repo.added.SKU != "SKU-1" || repo.added.Stock != 20 {
		t.Errorf("unexpected product passed to repository: %+v", repo.added)
	}
}

func TestAddProductShopWithLock_RepoErrorIsWrapped(t *testing.T) {
	repo := &fakeProdRepo{addErr: wrapInternal(errBoomFixture)}
	u := newTestProductUsecase(repo)

	err := u.AddProductShopWithLock(context.Background(), &requestdto.AddProduct{
		SKU: "X", PurchasePrice: ptr(int64(1)), SellingPrice: ptr(int64(2)),
		SellingPriceDebt: ptr(int64(3)), Stock: ptr(float64(1)),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "boom") {
		t.Errorf("driver detail must not leak: %v", err)
	}
}

func TestDeleteProductShop_Success(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	id := uuid.New()
	if err := u.DeleteProductShop(context.Background(), &requestdto.DeleteProduct{ID: id.String()}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.deletedID != id {
		t.Errorf("expected DeleteProduct called with %s, got %s", id, repo.deletedID)
	}
}

func TestDeleteProductShop_InvalidID(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	err := u.DeleteProductShop(context.Background(), &requestdto.DeleteProduct{ID: "bad-id"})
	if err == nil {
		t.Fatal("expected error for invalid product id")
	}
}

func TestUpdateProductShopWithLock_BuildsPartialFieldMap(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	id := uuid.New()
	name := "Gula Baru"
	price := int64(15000)
	req := &requestdto.UpdateProduct{ID: id.String(), ProductName: &name, SellingPrice: &price}

	if err := u.UpdateProductShopWithLock(context.Background(), req, 5); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.updatedID != id {
		t.Errorf("expected update called with id %s, got %s", id, repo.updatedID)
	}
	if repo.updatedFields["product_name"] != name || repo.updatedFields["selling_price"] != price {
		t.Errorf("unexpected fields map: %+v", repo.updatedFields)
	}
	if repo.updatedDelta != 5 {
		t.Errorf("expected stock delta 5, got %v", repo.updatedDelta)
	}
	if _, ok := repo.updatedFields["sku"]; ok {
		t.Error("sku was not provided in the request and must be absent from the update map")
	}
}

func TestUpdateProductShopWithLock_NoFieldsNoDeltaIsError(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	err := u.UpdateProductShopWithLock(context.Background(), &requestdto.UpdateProduct{ID: uuid.New().String()}, 0)
	if err == nil || !strings.Contains(err.Error(), "no fields to update") {
		t.Fatalf("expected 'no fields to update' error, got: %v", err)
	}
}

func TestUpdateStockWithLock_PositiveAndNegativeDelta(t *testing.T) {
	tests := []struct {
		name  string
		delta float64
	}{
		{"restock (positive delta)", 10},
		{"sale/deduction (negative delta)", -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProdRepo{}
			u := newTestProductUsecase(repo)
			id := uuid.New()

			if err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: id.String()}, tt.delta); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if repo.updatedID != id || repo.updatedDelta != tt.delta {
				t.Errorf("expected UpdateStockWithLock(%s, %v), got (%s, %v)", id, tt.delta, repo.updatedID, repo.updatedDelta)
			}
		})
	}
}

func TestUpdateStockWithLock_InsufficientStockPassesThrough(t *testing.T) {
	repo := &fakeProdRepo{updateErr: errInsufficientStockFixture}
	u := newTestProductUsecase(repo)

	err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: uuid.New().String()}, -100)
	if err == nil || !strings.Contains(err.Error(), "insufficient stock") {
		t.Fatalf("expected the business error to pass through unwrapped, got: %v", err)
	}
}

func TestUpdateStockWithLock_InvalidID(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: "not-a-uuid"}, 1)
	if err == nil {
		t.Fatal("expected error for invalid product id")
	}
}
