package repository_test

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedExportProduct(t *testing.T, db *gorm.DB, prefix, name string, barcode *string) *domain.Products {
	t.Helper()
	p := &domain.Products{
		SKU:              prefix + name,
		Barcode:          barcode,
		ProductName:      prefix + name,
		PurchasePrice:    1000,
		SellingPrice:     1500,
		SellingPriceDebt: 1600,
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed product %s: %v", name, err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&domain.Products{}, "id = ?", p.ID) })
	return p
}

func TestListForExport_PagesWithStableOrderAndSkipsMissingBarcode(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewProductRepository(db)
	prefix := "exp-" + uuid.NewString()[:8] + "-"
	code := func(s string) *string { c := prefix + s; return &c }

	seedExportProduct(t, db, prefix, "a", code("a"))
	seedExportProduct(t, db, prefix, "b", code("b"))
	seedExportProduct(t, db, prefix, "c", code("c"))
	seedExportProduct(t, db, prefix, "nobarcode", nil)

	filter := domain.FilterAllProduct{Search: prefix}
	first, total, err := repo.ListForExport(context.Background(), filter, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := repo.ListForExport(context.Background(), filter, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (product without barcode excluded)", total)
	}
	if len(first) != 2 || len(second) != 1 {
		t.Fatalf("page sizes = %d, %d; want 2, 1", len(first), len(second))
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range append(first, second...) {
		if seen[p.ID] {
			t.Errorf("product %s appears on two pages", p.SKU)
		}
		seen[p.ID] = true
	}
}

func TestUpdateStock_PcsRejectsFractionButKgAccepts(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewProductRepository(db)
	pcs := seedExportProduct(t, db, "pcs-"+uuid.NewString()[:8]+"-", "a", nil)
	kg := seedExportProduct(t, db, "kg-"+uuid.NewString()[:8]+"-", "a", nil)
	if err := db.Model(kg).Update("unit", 2).Error; err != nil { // 2 = kg
		t.Fatal(err)
	}

	if err := repo.UpdateProductWithLock(context.Background(), pcs.ID, map[string]any{}, 0.5); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("pcs +0.5: err = %v, want validation error", err)
	}
	assertStock(t, db, pcs.ID, 0)

	if err := repo.UpdateProductWithLock(context.Background(), kg.ID, map[string]any{}, 0.5); err != nil {
		t.Errorf("kg +0.5: %v", err)
	}
	assertStock(t, db, kg.ID, 0.5)
}

func TestFractionalStockKeepsThreeDecimals(t *testing.T) {
	db := openTestDB(t)
	p := seedExportProduct(t, db, "frac-"+uuid.NewString()[:8]+"-", "kg", nil)
	if err := db.Model(p).Updates(map[string]any{"stock": 2.5, "unit": 2}).Error; err != nil { // 2 = kg
		t.Fatal(err)
	}

	if err := repository.NewProductRepository(db).UpdateProductWithLock(context.Background(), p.ID, map[string]interface{}{}, -0.125); err != nil {
		t.Fatal(err)
	}
	var got domain.Products
	if err := db.First(&got, "id = ?", p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Stock != 2.375 {
		t.Errorf("stock = %v, want 2.375", got.Stock)
	}
}
