package usecase

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"shop_project_be/internal/constant/enum"
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

	current *domain.Products

	categories    map[string]uuid.UUID // key -> id, kategori yang sudah ada
	createdKeys   []string
	existingBySKU map[string]*domain.Products
	upserted      []*domain.Products
}

func (f *fakeProdRepo) GetCategoryIDs(ctx context.Context, names []string, create bool) (map[string]uuid.UUID, error) {
	if f.categories == nil {
		f.categories = map[string]uuid.UUID{}
	}
	ids := map[string]uuid.UUID{}
	for _, n := range names {
		key := domain.CategoryKey(n)
		if key == "" {
			continue
		}
		if _, ok := f.categories[key]; !ok && create {
			f.categories[key] = uuid.New()
			f.createdKeys = append(f.createdKeys, key)
		}
		if id, ok := f.categories[key]; ok {
			ids[key] = id
		}
	}
	return ids, nil
}

func (f *fakeProdRepo) GetProductsBySKUs(ctx context.Context, skus []string) (map[string]*domain.Products, error) {
	return f.existingBySKU, nil
}

func (f *fakeProdRepo) UpsertBulkProduct(ctx context.Context, products []*domain.Products) error {
	f.upserted = products
	return nil
}

func (f *fakeProdRepo) GetProduct(ctx context.Context, id uuid.UUID) (*domain.Products, error) {
	if f.current == nil {
		return nil, domain.NotFound("product not found")
	}
	return f.current, nil
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
	repo := &fakeProdRepo{current: &domain.Products{PurchasePrice: 10000, SellingPrice: 13000, SellingPriceDebt: 16000}}
	u := newTestProductUsecase(repo)

	id := uuid.New()
	name := "Gula Baru"
	price := int64(15000)
	req := &requestdto.UpdateProduct{ID: id.String(), ProductName: &name, SellingPrice: &price}

	if err := u.UpdateProductShopWithLock(context.Background(), req); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.updatedID != id {
		t.Errorf("expected update called with id %s, got %s", id, repo.updatedID)
	}
	if repo.updatedFields["product_name"] != name || repo.updatedFields["selling_price"] != price {
		t.Errorf("unexpected fields map: %+v", repo.updatedFields)
	}
	if repo.updatedDelta != 0 {
		t.Errorf("product update must not change stock (that is PATCH /products/stock), got delta %v", repo.updatedDelta)
	}
	if _, ok := repo.updatedFields["sku"]; ok {
		t.Error("sku was not provided in the request and must be absent from the update map")
	}
}

func TestUpdateProductShopWithLock_NoFieldsNoDeltaIsError(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	err := u.UpdateProductShopWithLock(context.Background(), &requestdto.UpdateProduct{ID: uuid.New().String()})
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

			if err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: id.String(), Stock: tt.delta}); err != nil {
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

	err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: uuid.New().String(), Stock: -100})
	if err == nil || !strings.Contains(err.Error(), "insufficient stock") {
		t.Fatalf("expected the business error to pass through unwrapped, got: %v", err)
	}
}

func TestUpdateStockWithLock_InvalidID(t *testing.T) {
	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)

	err := u.UpdateStockWithLock(context.Background(), &requestdto.UpdateStock{ID: "not-a-uuid", Stock: 1})
	if err == nil {
		t.Fatal("expected error for invalid product id")
	}
}

func TestValidatePrices(t *testing.T) {
	cases := []struct {
		name                        string
		purchase, selling, sellDebt int64
		wantErr                     bool
	}{
		{"valid, debt exactly +5%", 10000, 12000, 12600, false},
		{"debt rounds up: 1010*1.05=1060.5 needs 1061", 1000, 1010, 1061, false},
		{"debt 1060 below rounded-up minimum", 1000, 1010, 1060, true},
		{"debt below +5%", 10000, 12000, 12599, true},
		{"selling equal to purchase", 10000, 10000, 10500, true},
		{"zero purchase", 0, 12000, 12600, true},
		{"zero debt price", 10000, 12000, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePrices(tc.purchase, tc.selling, tc.sellDebt)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validatePrices(%d, %d, %d) err = %v, wantErr %v", tc.purchase, tc.selling, tc.sellDebt, err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrValidation) {
				t.Errorf("want ErrValidation, got %v", err)
			}
		})
	}
}

func TestUpdateProductShopWithLock_RejectsDebtPriceBelowMinimumAfterMerge(t *testing.T) {
	repo := &fakeProdRepo{current: &domain.Products{PurchasePrice: 10000, SellingPrice: 12000, SellingPriceDebt: 12600}}
	u := newTestProductUsecase(repo)

	raised := int64(13000)
	err := u.UpdateProductShopWithLock(context.Background(), &requestdto.UpdateProduct{ID: uuid.New().String(), SellingPrice: &raised})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("raising selling_price above the stored debt price must fail validation, got: %v", err)
	}
	if repo.updatedFields != nil {
		t.Error("repository must not be called when validation fails")
	}
}

func TestUpdateProductShopWithLock_StaffCannotCreateCategory(t *testing.T) {
	repo := &fakeProdRepo{categories: map[string]uuid.UUID{"sembako": uuid.New()}}
	u := newTestProductUsecase(repo)

	existing := " SEMBAKO "
	if err := u.UpdateProductShopWithLock(context.Background(), &requestdto.UpdateProduct{ID: uuid.New().String(), Category: &existing}); err != nil {
		t.Fatalf("existing category (case/space-insensitive) must be accepted, got: %v", err)
	}
	if repo.updatedFields["category_id"] != repo.categories["sembako"] {
		t.Errorf("category_id = %v, want the existing sembako id", repo.updatedFields["category_id"])
	}

	brandNew := "Minuman"
	err := u.UpdateProductShopWithLock(context.Background(), &requestdto.UpdateProduct{ID: uuid.New().String(), Category: &brandNew})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("staff creating a new category must fail validation, got: %v", err)
	}
	if len(repo.createdKeys) != 0 {
		t.Errorf("no category may be created for staff, created: %v", repo.createdKeys)
	}
}

func bulkUpload(t *testing.T, csv string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file_upload", "products.csv")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte(csv))
	w.Close()
	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	return form.File["file_upload"][0]
}

func TestAddBulkProduct_UpsertRules(t *testing.T) {
	pulsaID := uuid.New()
	repo := &fakeProdRepo{existingBySKU: map[string]*domain.Products{
		"GULA":  {SKU: "GULA", Unit: enum.ProductUnit(2), ProductType: enum.Physical},
		"PULSA": {ID: pulsaID, SKU: "PULSA", ProductType: enum.Digital},
	}}
	u := newTestProductUsecase(repo)

	csv := "sku,product_name,unit,product_type,purchase_price,selling_price,selling_price_debt,stock,category\n" +
		"GULA,Gula 1kg,,,10000,12000,12600,5,Sembako\n" +
		"PULSA,Pulsa 10k,,physical,10000,11000,11550,0,\n" +
		"BERAS,Beras 5kg,kg,,60000,65000,68250,10, sembako \n"
	report, err := u.AddBulkProductShopWithLock(context.Background(), &requestdto.AddBulkProduct{FileUpload: bulkUpload(t, csv)})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if report.TotalInserted != 1 || report.TotalUpdated != 1 || len(report.UpdatedSKUs) != 1 || report.UpdatedSKUs[0] != "GULA" {
		t.Errorf("inserted=%d updated=%d skus=%v, want 1/1/[GULA]", report.TotalInserted, report.TotalUpdated, report.UpdatedSKUs)
	}
	if len(report.RowErrors) != 1 || report.RowErrors[0].Line != 3 {
		t.Errorf("digital PULSA turned physical must be the only row error, got %+v", report.RowErrors)
	}
	var gula *domain.Products
	for _, p := range repo.upserted {
		if p.SKU == "GULA" {
			gula = p
		}
	}
	if gula == nil || gula.Unit != enum.ProductUnit(2) {
		t.Fatalf("empty unit cell must keep the stored unit, got %+v", gula)
	}
	if len(repo.createdKeys) != 1 || repo.createdKeys[0] != "sembako" {
		t.Errorf("Sembako and ' sembako ' must create one category, created %v", repo.createdKeys)
	}
}
