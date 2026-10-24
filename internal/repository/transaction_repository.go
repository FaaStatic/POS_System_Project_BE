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

type transactionRepository struct {
	db *gorm.DB
}

func NewTransactionRepository(db *gorm.DB) domain.TransactionRepository {
	return &transactionRepository{db: db}
}

func (t *transactionRepository) CheckTransactionByNoInvoice(ctx context.Context, noInvoice string) (*domain.Transactions, error) {
	var item domain.Transactions
	result := t.db.WithContext(ctx).Where("no_invoice = ?", noInvoice).First(&item)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get transaction: %w", result.Error)
	}
	return &item, nil
}

func detailLines(details []domain.TransactionsDetail) []stockLine {
	lines := make([]stockLine, 0, len(details))
	for _, d := range details {
		lines = append(lines, stockLine{productID: d.ProductID, qty: d.Qty})
	}
	return lines
}

func (t *transactionRepository) CreateTransaction(ctx context.Context, transaction *domain.Transactions, isHutang bool, deductStock bool) (*domain.TransactionDebtSnapshot, error) {
	var debtSnapshot *domain.TransactionDebtSnapshot
	err := runTxDB(ctx, t.db, func(tx *gorm.DB) error {
		if deductStock {
			short, err := decrementStockLines(tx, detailLines(transaction.TransactionDetail), transaction.ClientRef != nil)
			if err != nil {
				return err
			}
			transaction.StockConflict = short
		}

		if isHutang && transaction.CustomerID != nil {
			if err := lockCustomerShared(tx, *transaction.CustomerID); err != nil {
				return err
			}
			var debt domain.Debts
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("customer_id = ? AND status = ?", *transaction.CustomerID, enum.BELUM_LUNAS).
				First(&debt).Error
			var previousRemaining int64
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				previousRemaining = 0

				totalDebt := transaction.TotalTransaction
				debt = domain.Debts{
					CustomerID:    *transaction.CustomerID,
					TotalDebt:     totalDebt,
					RemainingDebt: totalDebt,
					Status:        enum.BELUM_LUNAS,
				}
				if err := tx.Create(&debt).Error; err != nil {
					return internalErr(fmt.Errorf("failed to create debt: %w", err))
				}
			case err != nil:
				return internalErr(fmt.Errorf("failed to get debt: %w", err))
			default:
				previousRemaining = debt.RemainingDebt
				if err := tx.Model(&domain.Debts{}).Where("id = ?", debt.ID).
					Updates(map[string]interface{}{
						"total_debt":     debt.TotalDebt + transaction.TotalTransaction,
						"remaining_debt": debt.RemainingDebt + transaction.TotalTransaction,
						"status":         enum.BELUM_LUNAS,
					}).Error; err != nil {
					return internalErr(fmt.Errorf("failed to update debt: %w", err))
				}
				debt.TotalDebt += transaction.TotalTransaction
				debt.RemainingDebt += transaction.TotalTransaction
				debt.Status = enum.BELUM_LUNAS
			}
			transaction.DebtID = &debt.ID
			debtSnapshot = &domain.TransactionDebtSnapshot{
				DebtID:                debt.ID,
				PreviousRemainingDebt: previousRemaining,
				AmountAdded:           transaction.TotalTransaction,
				TotalDebt:             debt.TotalDebt,
				RemainingDebt:         debt.RemainingDebt,
				Status:                debt.Status,
			}
		}

		if err := tx.Session(&gorm.Session{FullSaveAssociations: true}).Create(transaction).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return domain.Duplicate(fmt.Sprintf("transaction with no invoice %s already exists", transaction.NoInvoice))
			}
			return internalErr(fmt.Errorf("failed to create transaction: %w", err))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return debtSnapshot, nil
}

func (t *transactionRepository) DeleteTransaction(ctx context.Context, id uuid.UUID, reason string) error {
	return runTxDB(ctx, t.db, func(tx *gorm.DB) error {
		var trx domain.Transactions
		if err := tx.Preload("TransactionDetail").Where("id = ?", id).First(&trx).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("transaction with id %s not found", id))
			}
			return internalErr(fmt.Errorf("failed to get transaction: %w", err))
		}

		if err := assertTransactionDeletable(tx, &trx); err != nil {
			return err
		}

		if err := incrementStock(tx, detailLines(trx.TransactionDetail)); err != nil {
			return err
		}

		if trx.DebtID != nil {
			var debt domain.Debts
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", *trx.DebtID).First(&debt).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
			case err != nil:
				return internalErr(fmt.Errorf("failed to lock debt: %w", err))
			default:
				// Dihitung setelah lock hutang: PayDebt juga lock baris ini, jadi
				// cicilan yang masuk bersamaan tidak bisa lolos dari cek ini.
				paidInstalments, err := countDebtPayments(tx, debt.ID)
				if err != nil {
					return err
				}
				if paidInstalments > 0 {
					return domain.Conflict(fmt.Sprintf(
						"transaction %s cannot be deleted: its debt already has %d recorded payment(s); void the payments first",
						trx.ID, paidInstalments))
				}
				newTotal := max(0, debt.TotalDebt-trx.TotalTransaction)
				newRemaining := max(0, debt.RemainingDebt-trx.TotalTransaction)
				status := enum.BELUM_LUNAS
				if newRemaining == 0 {
					status = enum.LUNAS
				}
				if err := tx.Model(&domain.Debts{}).Where("id = ?", debt.ID).
					Updates(map[string]interface{}{
						"total_debt":     newTotal,
						"remaining_debt": newRemaining,
						"status":         status,
					}).Error; err != nil {
					return internalErr(fmt.Errorf("failed to reverse debt: %w", err))
				}
			}
		}

		if err := tx.Model(&domain.Transactions{}).Where("id = ?", id).
			Update("cancel_reason", reason).Error; err != nil {
			return internalErr(fmt.Errorf("failed to save cancel reason: %w", err))
		}
		result := tx.Where("id = ?", id).Delete(&domain.Transactions{})
		if result.Error != nil {
			return internalErr(fmt.Errorf("failed to delete transaction: %w", result.Error))
		}
		if result.RowsAffected == 0 {
			return domain.NotFound(fmt.Sprintf("transaction with id %s not found", id))
		}
		return nil
	})
}

func assertTransactionDeletable(tx *gorm.DB, trx *domain.Transactions) error {
	var settledPayments int64
	if err := tx.Model(&domain.Payment{}).
		Where("status IN ?", []domain.PaymentStatus{domain.PaymentSuccess, domain.PaymentSettling}).
		Where("transaction_id = ? OR order_id = ?", trx.ID, trx.NoInvoice).
		Count(&settledPayments).Error; err != nil {
		return internalErr(fmt.Errorf("failed to count settled payments: %w", err))
	}
	if settledPayments > 0 {
		return domain.Conflict(fmt.Sprintf(
			"transaction %s cannot be deleted: it is backed by a settled online payment (order %s); refund it through the payment gateway first",
			trx.ID, trx.NoInvoice))
	}

	return nil
}

func (t *transactionRepository) GetAllTransaction(ctx context.Context, filter domain.FilterTransaction) (*domain.ResultTransaction, error) {
	limit, order := pageParams(filter.Limit, filter.Order)

	query := t.db.WithContext(ctx).Model(&domain.Transactions{}).Preload("TransactionDetail")

	if filter.CustomerID != nil {
		query = query.Where("customer_id = ?", *filter.CustomerID)
	}
	if filter.NoInvoices != "" {
		query = query.Where("no_invoice LIKE ? ESCAPE '\\'", likeContains(filter.NoInvoices))
	}
	if filter.TypeTrx != nil {
		query = query.Where("payment_type = ?", *filter.TypeTrx)
	}
	if filter.DateFrom != nil {
		query = query.Where("created_at >= ?", *filter.DateFrom)
	}
	if filter.DateUntil != nil {
		query = query.Where("created_at < ?", *filter.DateUntil)
	}
	if filter.OpenConflict {
		query = query.Where("stock_conflict AND conflict_reviewed_at IS NULL")
	}

	var items []*domain.Transactions
	if err := keysetPage(query, "", limit, order, filter.Cursor).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(t *domain.Transactions) (time.Time, uuid.UUID) { return t.CreatedAt, t.ID })
	return &domain.ResultTransaction{
		DataItem: items,
		HasNext:  hasNext,
		Cursor:   next,
	}, nil
}

func monthRange(month, year int) (time.Time, time.Time) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, domain.StoreLocation)
	return start, start.AddDate(0, 1, 0)
}

// security: col dan konstanta enum yang disisipkan ke SQL lewat storeDate/Sprintf
// di file ini selalu literal dari kode, bukan input user.
// storeDate harus sezona dengan domain.StoreLocation (WIB, UTC+7) yang dipakai
// monthRange; ubah keduanya bersamaan.
func storeDate(col string) string {
	return "(" + col + " AT TIME ZONE 'Asia/Jakarta')::date"
}

const soldProductName = "COALESCE(NULLIF(transactions_detail.product_name, ''), products.product_name)"

func (t *transactionRepository) GetDailyReport(ctx context.Context, month int, year int) ([]domain.DailyReport, error) {
	start, end := monthRange(month, year)
	day := storeDate("created_at")

	var rows []domain.DailyReport
	result := t.db.WithContext(ctx).
		Model(&domain.Transactions{}).
		Joins(`LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(ROUND(d.purchase_price * d.qty)), 0)::bigint AS cost
			FROM transactions_detail d WHERE d.transaction_id = transactions.id
		) AS line_cost ON true`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Select(fmt.Sprintf(`%[1]s AS date,
			COUNT(*) FILTER (WHERE payment_type <> %[2]d) AS count_sale,
			COALESCE(SUM(total_transaction) FILTER (WHERE payment_type <> %[2]d), 0)::bigint AS revenue,
			COALESCE(SUM(line_cost.cost) FILTER (WHERE payment_type <> %[2]d), 0)::bigint AS cost,
			COUNT(*) FILTER (WHERE payment_type = %[2]d) AS count_debt,
			COALESCE(SUM(total_transaction) FILTER (WHERE payment_type = %[2]d), 0)::bigint AS debt_sales,
			COALESCE(SUM(line_cost.cost) FILTER (WHERE payment_type = %[2]d), 0)::bigint AS debt_cost`, day, int(enum.Hutang))).
		Group(day).
		Order(day + " ASC").
		Scan(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get daily report: %w", result.Error)
	}
	return rows, nil
}

// GetDailyDebtFlow: hutang manual (debt_adjustments) dan pembayaran hutang
// (debt_payments) per hari. Pembayaran hanya mengurangi saldo, tidak terikat barang.
func (t *transactionRepository) GetDailyDebtFlow(ctx context.Context, month int, year int) ([]domain.DailyDebtFlow, error) {
	start, end := monthRange(month, year)

	var rows []domain.DailyDebtFlow
	result := t.db.WithContext(ctx).Raw(`SELECT date, SUM(manual)::bigint AS manual, SUM(paid)::bigint AS paid FROM (
			SELECT `+storeDate("created_at")+` AS date, amount AS manual, 0 AS paid
			FROM debt_adjustments WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT `+storeDate("tanggal_bayar")+`, 0, nominal_bayar
			FROM debt_payments WHERE tanggal_bayar >= ? AND tanggal_bayar < ?
		) AS flow GROUP BY date ORDER BY date`, start, end, start, end).
		Scan(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get daily debt flow: %w", result.Error)
	}
	return rows, nil
}

func (t *transactionRepository) soldLines(ctx context.Context, month, year int) *gorm.DB {
	start, end := monthRange(month, year)
	return t.db.WithContext(ctx).
		Model(&domain.Transactions{}).
		Joins("JOIN transactions_detail ON transactions_detail.transaction_id = transactions.id").
		Joins("LEFT JOIN products ON products.id = transactions_detail.product_id").
		Where("transactions.created_at >= ? AND transactions.created_at < ?", start, end)
}

func (t *transactionRepository) GetMonthlyProductSold(ctx context.Context, month int, year int) ([]domain.ProductSoldReport, error) {
	var rows []domain.ProductSoldReport
	result := t.soldLines(ctx, month, year).
		Select(soldProductName + " AS product_name, SUM(transactions_detail.qty) AS qty, COALESCE(SUM(transactions_detail.subtotal), 0)::bigint AS total").
		Group("transactions_detail.product_id, " + soldProductName).
		Order("qty DESC").
		Scan(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get monthly product sold: %w", result.Error)
	}
	return rows, nil
}

func (t *transactionRepository) GetDailyProductSold(ctx context.Context, month int, year int) ([]domain.DailyProductSoldReport, error) {
	day := storeDate("transactions.created_at")

	var rows []domain.DailyProductSoldReport
	result := t.soldLines(ctx, month, year).
		Select(day + " AS date, " + soldProductName + " AS product_name, SUM(transactions_detail.qty) AS qty, COALESCE(SUM(transactions_detail.subtotal), 0)::bigint AS total").
		Group(day + ", transactions_detail.product_id, " + soldProductName).
		Order(day + " ASC, qty DESC").
		Scan(&rows)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get daily product sold: %w", result.Error)
	}
	return rows, nil
}

func (t *transactionRepository) GetTransactionByID(ctx context.Context, id uuid.UUID) (*domain.Transactions, error) {
	var item domain.Transactions
	// Kasir/customer yang sudah dinonaktifkan tetap tampil di detail dan struk lama.
	result := t.db.WithContext(ctx).Preload("User", unscoped).
		Preload("Customer", unscoped).
		Preload("TransactionDetail").Where("id = ?", id).First(&item)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("transaction with id %s not found", id))
		}
		return nil, fmt.Errorf("failed to get transaction: %w", result.Error)
	}
	return &item, nil
}

// GetTransactionByClientRef ikut membaca transaksi yang sudah dibatalkan, supaya
// antrean offline yang dikirim ulang tidak menghidupkannya kembali. nil = belum ada.
func (t *transactionRepository) GetTransactionByClientRef(ctx context.Context, ref uuid.UUID) (*domain.Transactions, error) {
	var item domain.Transactions
	result := t.db.WithContext(ctx).Unscoped().Where("client_ref = ?", ref).First(&item)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get transaction by client ref: %w", result.Error)
	}
	return &item, nil
}

func (t *transactionRepository) MarkConflictReviewed(ctx context.Context, id uuid.UUID) error {
	result := t.db.WithContext(ctx).Model(&domain.Transactions{}).
		Where("id = ? AND stock_conflict", id).
		Update("conflict_reviewed_at", time.Now())
	if result.Error != nil {
		return fmt.Errorf("failed to mark conflict reviewed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound(fmt.Sprintf("transaction %s with a stock conflict not found", id))
	}
	return nil
}
