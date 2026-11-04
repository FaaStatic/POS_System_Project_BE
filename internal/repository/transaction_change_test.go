package repository_test

import (
	"context"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedCandy(t *testing.T, db *gorm.DB, stock float64) *domain.Products {
	t.Helper()
	candy := &domain.Products{
		SKU: "CANDY-" + uuid.NewString(), ProductName: "Permen", PurchasePrice: 300,
		SellingPrice: 1000, SellingPriceDebt: 1100, Stock: stock,
	}
	if err := db.Create(candy).Error; err != nil {
		t.Fatalf("seed candy: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&domain.Products{}, "id = ?", candy.ID) })
	return candy
}

func assertStock(t *testing.T, db *gorm.DB, id uuid.UUID, want float64) {
	t.Helper()
	var p domain.Products
	if err := db.First(&p, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if p.Stock != want {
		t.Errorf("stock of %s = %v, want %v", id, p.Stock, want)
	}
}

func TestCreateTransaction_ChangeProductsLeaveStockAndComeBackOnCancel(t *testing.T) {
	db := openTestDB(t)
	trxRepo := repository.NewTransactionRepository(db)
	user, product, _ := seedUserProductCustomer(t, db, 10)
	candy := seedCandy(t, db, 5)

	trx := newTrx(user, product, nil, enum.MoneyPayment(0), 2, 10000) // total 20000
	paid, change := int64(25000), int64(5000)
	trx.AmountPaid, trx.ChangeAmount = &paid, &change
	trx.Change = []domain.TransactionChange{
		{Kind: domain.ChangeCash, Amount: 2000},
		{Kind: domain.ChangeCandy, Amount: 3000, ProductID: &candy.ID, ProductName: candy.ProductName, Qty: 3, UnitPrice: 1000, PurchasePrice: 300},
	}
	// Didaftarkan setelah cleanup seed, jadi jalan lebih dulu: baris kembalian
	// memegang FK ke transaksi yang dihapus cleanup seed.
	t.Cleanup(func() { db.Unscoped().Delete(&domain.TransactionChange{}, "transaction_id = ?", trx.ID) })

	if _, err := trxRepo.CreateTransaction(context.Background(), trx, false, true); err != nil {
		t.Fatalf("create: %v", err)
	}
	assertStock(t, db, product.ID, 8)
	assertStock(t, db, candy.ID, 2)

	got, err := trxRepo.GetTransactionByID(context.Background(), trx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountPaid == nil || *got.AmountPaid != 25000 || got.ChangeAmount == nil || *got.ChangeAmount != 5000 {
		t.Errorf("paid/change = %v/%v, want 25000/5000", got.AmountPaid, got.ChangeAmount)
	}
	if len(got.Change) != 2 || got.TotalTransaction != 20000 {
		t.Errorf("change lines = %d, total = %d; want 2 lines and total unchanged at 20000", len(got.Change), got.TotalTransaction)
	}

	if err := trxRepo.DeleteTransaction(context.Background(), trx.ID, "salah input"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	assertStock(t, db, product.ID, 10)
	assertStock(t, db, candy.ID, 5)
}

func TestCreateTransaction_ShortChangeProductRollsBackTheWholeSale(t *testing.T) {
	db := openTestDB(t)
	trxRepo := repository.NewTransactionRepository(db)
	user, product, _ := seedUserProductCustomer(t, db, 10)
	candy := seedCandy(t, db, 1)

	trx := newTrx(user, product, nil, enum.MoneyPayment(0), 2, 10000)
	paid, change := int64(23000), int64(3000)
	trx.AmountPaid, trx.ChangeAmount = &paid, &change
	trx.Change = []domain.TransactionChange{
		{Kind: domain.ChangeCandy, Amount: 3000, ProductID: &candy.ID, ProductName: candy.ProductName, Qty: 3, UnitPrice: 1000},
	}

	if _, err := trxRepo.CreateTransaction(context.Background(), trx, false, true); err == nil {
		t.Fatal("expected insufficient stock error for the candy given as change")
	}
	assertStock(t, db, product.ID, 10) // barang belanja tidak boleh ikut berkurang
	assertStock(t, db, candy.ID, 1)
}
