package repository

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/domain"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type paymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) domain.PaymentRepository {
	return &paymentRepository{db: db}
}

func (p *paymentRepository) CreateWithReservation(ctx context.Context, payment *domain.Payment) error {
	return runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		if payment.StockReserved {
			if err := decrementStock(tx, paymentLines(payment.Items)); err != nil {
				return err
			}
		}
		if err := tx.Create(payment).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return domain.Duplicate("payment with invoice " + payment.OrderID + " already exists")
			}
			return internalErr(fmt.Errorf("failed to create payment: %w", err))
		}
		return nil
	})
}

func (p *paymentRepository) GetByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	var payment domain.Payment
	err := p.db.WithContext(ctx).Where("order_id = ?", orderID).First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}
	return &payment, nil
}

func (p *paymentRepository) ListStalePending(ctx context.Context, limit int) ([]*domain.Payment, error) {
	now := time.Now()
	var payments []*domain.Payment
	err := p.db.WithContext(ctx).
		Where("status = ?", domain.PaymentPending).
		Where("(expiry_time IS NOT NULL AND expiry_time < ?) OR (expiry_time IS NULL AND created_at < ?)",
			now.Add(-10*time.Minute), now.Add(-24*time.Hour)).
		Order("updated_at ASC").
		Limit(limit).
		Find(&payments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list stale pending payments: %w", err)
	}
	return payments, nil
}

func (p *paymentRepository) ListPendingFinalization(ctx context.Context, limit int) ([]*domain.Payment, error) {
	var payments []*domain.Payment
	err := p.db.WithContext(ctx).
		Where("status = ?", domain.PaymentSettling).
		Order("updated_at ASC").
		Limit(limit).
		Find(&payments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list payments pending finalization: %w", err)
	}
	return payments, nil
}

func (p *paymentRepository) Touch(ctx context.Context, orderID string) error {
	return p.db.WithContext(ctx).Model(&domain.Payment{}).
		Where("order_id = ?", orderID).
		UpdateColumn("updated_at", time.Now()).Error
}

func (p *paymentRepository) UpdateWithLock(ctx context.Context, orderID string, fn func(payment *domain.Payment) (bool, error)) error {
	return runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		var payment domain.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_id = ?", orderID).First(&payment).Error; err != nil {
			return internalErr(fmt.Errorf("failed to lock payment: %w", err))
		}
		wasReserved := payment.StockReserved
		save, err := fn(&payment)
		if err != nil {
			return err
		}
		if !save {
			return nil
		}
		if wasReserved && !payment.StockReserved && payment.TransactionID == nil {
			if err := incrementStock(tx, paymentLines(payment.Items)); err != nil {
				return err
			}
		}
		if err := tx.Save(&payment).Error; err != nil {
			return internalErr(fmt.Errorf("failed to update payment: %w", err))
		}
		return nil
	})
}
