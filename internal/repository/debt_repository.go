package repository

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type debtRepository struct {
	db *gorm.DB
}

func NewDebtRepository(db *gorm.DB) domain.DebtRepository {
	return &debtRepository{db: db}
}

// GetDebtByID tidak preload Transactions: satu hutang berjalan menampung
// transaksi tanpa batas. Ambil lewat GET /api/customers/:id/transactions.
func (d *debtRepository) GetDebtByID(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	return d.getDebt(ctx, id)
}

func (d *debtRepository) GetDebtWithPayments(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	return d.getDebt(ctx, id, "DebtPayments.User")
}

func (d *debtRepository) getDebt(ctx context.Context, id uuid.UUID, preloads ...string) (*domain.Debts, error) {
	var debt domain.Debts
	query := d.db.WithContext(ctx).Preload("Customer", unscoped)
	for _, p := range preloads {
		query = query.Preload(p)
	}
	result := query.Where("id = ?", id).First(&debt)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("debt with id %s not found", id))
		}
		return nil, fmt.Errorf("failed to get debt: %w", result.Error)
	}
	return &debt, nil
}

func (d *debtRepository) AddDebt(ctx context.Context, debt *domain.Debts, userID uuid.UUID) error {
	return runTxDB(ctx, d.db, func(tx *gorm.DB) error {
		if err := lockCustomerShared(tx, debt.CustomerID); err != nil {
			return err
		}
		var open domain.Debts
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ? AND status = ?", debt.CustomerID, enum.BELUM_LUNAS).
			First(&open).Error
		debtID := open.ID
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(debt).Error; err != nil {
				return internalErr(fmt.Errorf("failed to add debt: %w", err))
			}
			debtID = debt.ID
		case err != nil:
			return internalErr(fmt.Errorf("failed to get debt: %w", err))
		default:
			fields := map[string]interface{}{
				"total_debt":     open.TotalDebt + debt.TotalDebt,
				"remaining_debt": open.RemainingDebt + debt.RemainingDebt,
				"status":         enum.BELUM_LUNAS,
			}
			if !debt.DueDate.IsZero() {
				fields["due_date"] = debt.DueDate
			}
			if err := tx.Model(&domain.Debts{}).Where("id = ?", open.ID).Updates(fields).Error; err != nil {
				return internalErr(fmt.Errorf("failed to update debt: %w", err))
			}
		}
		adjustment := &domain.DebtAdjustments{DebtID: debtID, UserID: userID, Amount: debt.TotalDebt}
		if err := tx.Create(adjustment).Error; err != nil {
			return internalErr(fmt.Errorf("failed to record debt adjustment: %w", err))
		}
		return nil
	})
}

func countDebtPayments(tx *gorm.DB, debtID uuid.UUID) (int64, error) {
	var count int64
	if err := tx.Model(&domain.DebtPayments{}).
		Where("debt_id = ?", debtID).Count(&count).Error; err != nil {
		return 0, internalErr(fmt.Errorf("failed to count debt payments: %w", err))
	}
	return count, nil
}

func (d *debtRepository) PayDebt(ctx context.Context, debtID uuid.UUID, payment *domain.DebtPayments) (*domain.DebtPaymentResult, error) {
	var result domain.DebtPaymentResult
	err := runTxDB(ctx, d.db, func(tx *gorm.DB) error {
		if payment.IdempotencyKey != nil && *payment.IdempotencyKey != "" {
			var existing domain.DebtPayments
			err := tx.Preload("User").
				Where("debt_id = ? AND idempotency_key = ?", debtID, *payment.IdempotencyKey).
				First(&existing).Error
			switch {
			case err == nil:
				if existing.NominalBayar != payment.NominalBayar {
					return domain.Conflict("idempotency_key was already used for a payment of a different amount")
				}
				var debt domain.Debts
				if err := tx.Preload("Customer").Where("id = ?", debtID).First(&debt).Error; err != nil {
					return internalErr(fmt.Errorf("failed to load debt for replay: %w", err))
				}
				debt.RemainingDebt = existing.PreviousRemainingDebt - existing.NominalBayar
				debt.Status = enum.BELUM_LUNAS
				if debt.RemainingDebt <= 0 {
					debt.RemainingDebt, debt.Status = 0, enum.LUNAS
				}
				result = domain.DebtPaymentResult{
					Debt:                  &debt,
					PreviousRemainingDebt: existing.PreviousRemainingDebt,
					PaymentID:             existing.ID,
					PaidAt:                existing.TanggalBayar,
				}
				return nil
			case !errors.Is(err, gorm.ErrRecordNotFound):
				return internalErr(fmt.Errorf("failed to check idempotency key: %w", err))
			}
		}

		var debt domain.Debts
		if err := tx.Preload("Customer").Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", debtID).First(&debt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("debt with id %s not found", debtID))
			}
			return internalErr(fmt.Errorf("failed to lock debt: %w", err))
		}
		if debt.RemainingDebt <= 0 || debt.Status == enum.LUNAS {
			return domain.Conflict("debt has already been fully paid")
		}
		if payment.NominalBayar > debt.RemainingDebt {
			return domain.Validation(fmt.Sprintf("payment amount (%d) exceeds remaining debt (%d)", payment.NominalBayar, debt.RemainingDebt))
		}
		previousRemaining := debt.RemainingDebt

		newRemaining := debt.RemainingDebt - payment.NominalBayar
		status := enum.BELUM_LUNAS
		if newRemaining == 0 {
			status = enum.LUNAS
		}
		if err := tx.Model(&domain.Debts{}).Where("id = ?", debt.ID).
			Updates(map[string]interface{}{
				"remaining_debt": newRemaining,
				"status":         status,
			}).Error; err != nil {
			return internalErr(fmt.Errorf("failed to update debt: %w", err))
		}

		payment.DebtID = debt.ID
		payment.PreviousRemainingDebt = previousRemaining
		if err := tx.Create(payment).Error; err != nil {
			return internalErr(fmt.Errorf("failed to record debt payment: %w", err))
		}

		debt.RemainingDebt = newRemaining
		debt.Status = status
		result = domain.DebtPaymentResult{
			Debt:                  &debt,
			PreviousRemainingDebt: previousRemaining,
			PaymentID:             payment.ID,
			PaidAt:                payment.TanggalBayar,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *debtRepository) GetAllDebt(ctx context.Context, filter domain.FilterDebt) (*domain.DebtsPaginated, error) {
	limit, order := pageParams(filter.Limit, filter.Order)

	// Transaksi tidak di-preload: satu hutang terbuka terus menampung transaksi
	// hutang customer, jadi jumlahnya tak terbatas. Ambil per customer lewat
	// GET /api/customers/:id/transactions (berhalaman).
	query := d.db.WithContext(ctx).Preload("Customer", unscoped).Model(&domain.Debts{})

	if filter.Search != "" {
		query = query.Select("debts.*").
			Joins("JOIN customers ON customers.id = debts.customer_id").
			Where("customers.name ILIKE ? ESCAPE '\\'", likeContains(filter.Search))
	}

	if filter.CustomerID != uuid.Nil {
		query = query.Where("debts.customer_id = ?", filter.CustomerID)
	}
	if filter.Status != nil {
		query = query.Where("debts.status = ?", *filter.Status)
	}

	var items []*domain.Debts
	if err := keysetPage(query, "debts.", limit, order, filter.Cursor).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get debts: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(d *domain.Debts) (time.Time, uuid.UUID) { return d.CreatedAt, d.ID })
	return &domain.DebtsPaginated{
		Data:    items,
		HasNext: hasNext,
		Cursor:  next,
	}, nil
}
