package repository

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"slices"
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
// transaksi tanpa batas. Ambil lewat GET /api/v1/sales/customers/:id/transactions.
func (d *debtRepository) GetDebtByID(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	return d.getDebt(ctx, id)
}

// maxReportPayments: satu hutang berjalan menampung cicilan tanpa batas; laporan
// cukup memuat yang terbaru. Total dan sisa tetap dari baris debts.
const maxReportPayments = 500

// GetDebtWithPayments memuat maxReportPayments cicilan terbaru, urut lama ke baru.
func (d *debtRepository) GetDebtWithPayments(ctx context.Context, id uuid.UUID) (*domain.Debts, error) {
	debt, err := d.getDebt(ctx, id, func(q *gorm.DB) *gorm.DB {
		return q.Preload("DebtPayments", func(db *gorm.DB) *gorm.DB {
			return db.Order("tanggal_bayar DESC, id DESC").Limit(maxReportPayments)
		}).Preload("DebtPayments.User")
	})
	if err != nil {
		return nil, err
	}
	slices.Reverse(debt.DebtPayments)
	return debt, nil
}

func (d *debtRepository) getDebt(ctx context.Context, id uuid.UUID, scopes ...func(*gorm.DB) *gorm.DB) (*domain.Debts, error) {
	var debt domain.Debts
	query := d.db.WithContext(ctx).Preload("Customer", unscoped).Scopes(scopes...)
	result := query.Where("id = ?", id).First(&debt)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("debt with id %s not found", id))
		}
		return nil, fmt.Errorf("failed to get debt: %w", result.Error)
	}
	return &debt, nil
}

// addToOpenDebt adds amount to the customer's open debt, or opens a new one when every earlier
// debt is paid off; a customer has at most one open debt. The caller already holds
// lockCustomerShared. A zero dueDate keeps the current due date.
func addToOpenDebt(tx *gorm.DB, customerID uuid.UUID, amount int64, dueDate time.Time) (debt domain.Debts, err error) {
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("customer_id = ? AND status = ?", customerID, enum.BELUM_LUNAS).
		First(&debt).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		debt = domain.Debts{CustomerID: customerID, TotalDebt: amount, RemainingDebt: amount, Status: enum.BELUM_LUNAS, DueDate: dueDate}
		if err := tx.Create(&debt).Error; err != nil {
			return debt, internalErr(fmt.Errorf("failed to create debt: %w", err))
		}
		return debt, nil
	case err != nil:
		return debt, internalErr(fmt.Errorf("failed to get debt: %w", err))
	}
	fields := map[string]interface{}{
		"total_debt":     debt.TotalDebt + amount,
		"remaining_debt": debt.RemainingDebt + amount,
		"status":         enum.BELUM_LUNAS,
	}
	if !dueDate.IsZero() {
		fields["due_date"] = dueDate
	}
	if err := tx.Model(&domain.Debts{}).Where("id = ?", debt.ID).Updates(fields).Error; err != nil {
		return debt, internalErr(fmt.Errorf("failed to update debt: %w", err))
	}
	debt.TotalDebt += amount
	debt.RemainingDebt += amount
	debt.Status = enum.BELUM_LUNAS
	return debt, nil
}

func (d *debtRepository) AddDebt(ctx context.Context, debt *domain.Debts, userID uuid.UUID, note string) error {
	return runTxDB(ctx, d.db, func(tx *gorm.DB) error {
		if err := lockCustomerShared(tx, debt.CustomerID); err != nil {
			return err
		}
		open, err := addToOpenDebt(tx, debt.CustomerID, debt.TotalDebt, debt.DueDate)
		if err != nil {
			return err
		}
		adjustment := &domain.DebtAdjustments{DebtID: open.ID, UserID: userID, Amount: debt.TotalDebt}
		if note != "" {
			adjustment.Note = &note
		}
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
	// GET /api/v1/sales/customers/:id/transactions (berhalaman).
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

// GetAllDebtPayments memuat cicilan terbaru lebih dulu, per halaman dengan kursor
// (tanggal_bayar, id). Cicilan dari hutang yang sudah lunas ikut tampil.
func (d *debtRepository) GetAllDebtPayments(ctx context.Context, filter domain.FilterDebtPayment) (*domain.DebtPaymentsPaginated, error) {
	limit, order := pageParams(filter.Limit, filter.Order)

	query := d.db.WithContext(ctx).Model(&domain.DebtPayments{}).
		Preload("User").
		Preload("Debt", unscoped).
		Preload("Debt.Customer", unscoped).
		Joins("JOIN debts ON debts.id = debt_payments.debt_id")
	if filter.CustomerID != uuid.Nil {
		query = query.Where("debts.customer_id = ?", filter.CustomerID)
	}
	if filter.Search != "" {
		query = query.Joins("JOIN customers ON customers.id = debts.customer_id").
			Where("customers.name ILIKE ? ESCAPE '\\'", likeContains(filter.Search))
	}
	if filter.Cursor != nil {
		op := "<"
		if order == "ASC" {
			op = ">"
		}
		// security: op hanya "<" atau ">" dari pageParams; nilai kursor lewat placeholder.
		query = query.Where(fmt.Sprintf("(debt_payments.tanggal_bayar, debt_payments.id) %s (?, ?)", op),
			filter.Cursor.AfterTime, filter.Cursor.AfterID)
	}
	query = query.Select("debt_payments.*").
		Order(fmt.Sprintf("debt_payments.tanggal_bayar %[1]s, debt_payments.id %[1]s", order)).
		Limit(limit + 1)

	var items []*domain.DebtPayments
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get debt payments: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(p *domain.DebtPayments) (time.Time, uuid.UUID) { return p.TanggalBayar, p.ID })
	return &domain.DebtPaymentsPaginated{Data: items, HasNext: hasNext, Cursor: next}, nil
}

func (d *debtRepository) GetDebtPaymentByID(ctx context.Context, id uuid.UUID) (*domain.DebtPayments, error) {
	var payment domain.DebtPayments
	err := d.db.WithContext(ctx).
		Preload("User").
		Preload("Debt", unscoped).
		Preload("Debt.Customer", unscoped).
		Where("id = ?", id).First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("debt payment with id %s not found", id))
		}
		return nil, internalErr(fmt.Errorf("failed to get debt payment: %w", err))
	}
	return &payment, nil
}

// maxMonthPayments membatasi satu rekap bulanan; toko kecil jauh di bawahnya.
const maxMonthPayments = 5000

func (d *debtRepository) GetDebtPaymentsInRange(ctx context.Context, from, until time.Time) ([]*domain.DebtPayments, error) {
	var items []*domain.DebtPayments
	err := d.db.WithContext(ctx).
		Preload("User").
		Preload("Debt", unscoped).
		Preload("Debt.Customer", unscoped).
		Where("tanggal_bayar >= ? AND tanggal_bayar < ?", from, until).
		Order("tanggal_bayar ASC, id ASC").
		Limit(maxMonthPayments + 1).
		Find(&items).Error
	if err != nil {
		return nil, internalErr(fmt.Errorf("failed to get debt payments: %w", err))
	}
	if len(items) > maxMonthPayments {
		return nil, domain.Validation(fmt.Sprintf("more than %d debt payments in this month, too many for one report", maxMonthPayments))
	}
	return items, nil
}

// GetDebtEntries menggabungkan penjualan hutang (yang belum dibatalkan) dan hutang manual
// untuk satu hutang, terbaru dulu, paling banyak limit baris. Keduanya diambil sebanyak
// limit lalu digabung, jadi hasil gabungan selalu cukup.
func (d *debtRepository) GetDebtEntries(ctx context.Context, debtID uuid.UUID, limit int) ([]domain.DebtEntry, error) {
	var sales []domain.Transactions
	if err := d.db.WithContext(ctx).Select("id", "no_invoice", "total_transaction", "created_at").
		Where("debt_id = ?", debtID).
		Order("created_at DESC, id DESC").Limit(limit).Find(&sales).Error; err != nil {
		return nil, internalErr(fmt.Errorf("failed to get debt sales: %w", err))
	}
	var manual []domain.DebtAdjustments
	if err := d.db.WithContext(ctx).Where("debt_id = ?", debtID).
		Order("created_at DESC, id DESC").Limit(limit).Find(&manual).Error; err != nil {
		return nil, internalErr(fmt.Errorf("failed to get debt adjustments: %w", err))
	}

	entries := make([]domain.DebtEntry, 0, len(sales)+len(manual))
	for _, s := range sales {
		entries = append(entries, domain.DebtEntry{Type: domain.DebtEntrySale, Amount: s.TotalTransaction, At: s.CreatedAt, Invoice: s.NoInvoice})
	}
	for _, m := range manual {
		entry := domain.DebtEntry{Type: domain.DebtEntryManual, Amount: m.Amount, At: m.CreatedAt}
		if m.Note != nil {
			entry.Note = *m.Note
		}
		entries = append(entries, entry)
	}
	slices.SortStableFunc(entries, func(a, b domain.DebtEntry) int { return b.At.Compare(a.At) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
