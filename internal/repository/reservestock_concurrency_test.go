package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	"shop_project_be/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCreateWithReservation_Concurrent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_DSN to run the concurrency test against a real Postgres")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(30)
	defer sqlDB.Close()

	const (
		stock   = 20
		workers = 100
	)

	product := &domain.Products{
		SKU:         "CONC-" + uuid.NewString()[:8],
		ProductName: "concurrency test product",
		Stock:       stock,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	user := &domain.Users{Username: "conc-" + uuid.NewString(), Password: "x"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&domain.Payment{}, "user_id = ?", user.ID)
		db.Unscoped().Delete(&domain.Products{}, "id = ?", product.ID)
		db.Unscoped().Delete(&domain.Users{}, "id = ?", user.ID)
	})

	repo := repository.NewPaymentRepository(db)

	var success, insufficient, other int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := repo.CreateWithReservation(ctx, &domain.Payment{
				OrderID:       "CONC-" + uuid.NewString(),
				UserID:        user.ID,
				Method:        "qris",
				Status:        domain.PaymentPending,
				StockReserved: true,
				Items:         []domain.PaymentItem{{ProductID: product.ID, Qty: 1}},
			})
			switch {
			case err == nil:
				atomic.AddInt64(&success, 1)
			case strings.Contains(err.Error(), "insufficient stock"):
				atomic.AddInt64(&insufficient, 1)
			default:
				atomic.AddInt64(&other, 1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if other != 0 {
		t.Fatalf("got %d unexpected errors (deadlock/serialization should have been retried, not surfaced)", other)
	}
	if success != stock {
		t.Errorf("successful reservations = %d, want exactly %d (oversell or lost reservation)", success, stock)
	}
	if insufficient != workers-stock {
		t.Errorf("insufficient-stock rejections = %d, want %d", insufficient, workers-stock)
	}

	var final domain.Products
	if err := db.First(&final, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if final.Stock != 0 {
		t.Errorf("final stock = %v, want 0 (must never oversell or go negative)", final.Stock)
	}
	fmt.Printf("reserved=%d rejected=%d final_stock=%v\n", success, insufficient, final.Stock)

	var orders []string
	db.Model(&domain.Payment{}).Where("user_id = ?", user.ID).Limit(1).Pluck("order_id", &orders)
	if len(orders) != 1 {
		t.Fatalf("expected a reserved payment to fail, found %d", len(orders))
	}
	if err := repo.UpdateWithLock(context.Background(), orders[0], func(p *domain.Payment) (bool, error) {
		p.Status = domain.PaymentFailed
		p.StockReserved = false
		return true, nil
	}); err != nil {
		t.Fatalf("fail payment: %v", err)
	}
	if err := db.First(&final, "id = ?", product.ID).Error; err != nil {
		t.Fatalf("reload product: %v", err)
	}
	if final.Stock != 1 {
		t.Errorf("stock after failing one payment = %v, want 1 (its reservation returned)", final.Stock)
	}
}
