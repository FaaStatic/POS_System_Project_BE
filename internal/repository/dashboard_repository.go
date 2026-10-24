package repository

import (
	"cmp"
	"context"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"slices"
	"time"

	"gorm.io/gorm"
)

type dashboardRepository struct {
	db *gorm.DB
}

func NewDashboardRepository(db *gorm.DB) domain.DashboardRepository {
	return &dashboardRepository{db: db}
}

// salesByDayAndMethod: satu baris per (hari WIB, payment_type) untuk transaksi
// aktif di [from, until). Total, grafik harian, dan pembagian metode bayar
// semuanya diturunkan dari hasil ini, jadi transaksi cukup dipindai sekali.
// Laba kotor dari snapshot harga di transactions_detail; JOIN + GROUP BY t.id
// ~4x lebih cepat daripada subquery per transaksi untuk rentang setahun.
var salesByDayAndMethod = `SELECT to_char(` + storeDate("s.created_at") + `, 'YYYY-MM-DD') AS date,
	s.payment_type, COUNT(*) AS transactions,
	SUM(s.total_transaction)::bigint AS revenue, SUM(s.profit)::bigint AS gross_profit
FROM (
	SELECT t.created_at, t.payment_type, t.total_transaction,
		COALESCE(SUM(ROUND((d.price - d.purchase_price) * d.qty)), 0) AS profit
	FROM transactions t
	LEFT JOIN transactions_detail d ON d.transaction_id = t.id
	WHERE t.deleted_at IS NULL AND t.created_at >= @from AND t.created_at < @until
	GROUP BY t.id
) s
GROUP BY 1, 2 ORDER BY 1, 2`

type salesRow struct {
	Date         string `gorm:"column:date"`
	PaymentType  int    `gorm:"column:payment_type"`
	Transactions int64  `gorm:"column:transactions"`
	Revenue      int64  `gorm:"column:revenue"`
	GrossProfit  int64  `gorm:"column:gross_profit"`
}

func (r *dashboardRepository) Summary(ctx context.Context, from, until time.Time) (*domain.DashboardSummary, error) {
	db := r.db.WithContext(ctx)
	args := map[string]any{"from": from, "until": until, "open": int(enum.BELUM_LUNAS)}
	summary := &domain.DashboardSummary{}

	var rows []salesRow
	if err := db.Raw(salesByDayAndMethod, args).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get sales: %w", err)
	}
	var debtSales int64
	methods := map[int]*domain.DashboardPaymentMethod{}
	for _, row := range rows {
		summary.TotalTransactions += row.Transactions
		summary.Revenue += row.Revenue
		summary.GrossProfit += row.GrossProfit
		if row.PaymentType == int(enum.Hutang) {
			debtSales += row.Revenue
		}
		if n := len(summary.Daily); n == 0 || summary.Daily[n-1].Date != row.Date {
			summary.Daily = append(summary.Daily, domain.DashboardDay{Date: row.Date})
		}
		day := &summary.Daily[len(summary.Daily)-1]
		day.Transactions += row.Transactions
		day.Revenue += row.Revenue
		day.GrossProfit += row.GrossProfit

		m, ok := methods[row.PaymentType]
		if !ok {
			m = &domain.DashboardPaymentMethod{PaymentType: row.PaymentType}
			methods[row.PaymentType] = m
		}
		m.Count += row.Transactions
		m.Total += row.Revenue
	}
	for _, m := range methods {
		summary.PaymentMethods = append(summary.PaymentMethods, *m)
	}
	slices.SortFunc(summary.PaymentMethods, func(a, b domain.DashboardPaymentMethod) int { return cmp.Compare(b.Total, a.Total) })

	if err := db.Raw(`SELECT `+soldProductName+` AS product_name,
		SUM(transactions_detail.qty) AS qty, COALESCE(SUM(transactions_detail.subtotal), 0)::bigint AS total
		FROM transactions
		JOIN transactions_detail ON transactions_detail.transaction_id = transactions.id
		LEFT JOIN products ON products.id = transactions_detail.product_id
		WHERE transactions.deleted_at IS NULL AND transactions.created_at >= @from AND transactions.created_at < @until
		GROUP BY transactions_detail.product_id, `+soldProductName+`
		ORDER BY total DESC LIMIT 5`, args).Scan(&summary.TopProducts).Error; err != nil {
		return nil, fmt.Errorf("failed to get top products: %w", err)
	}

	// Hutang baru = penjualan hutang (dari rows di atas) + hutang manual; dibayar = cicilan masuk.
	if err := db.Raw(`SELECT
		(SELECT COALESCE(SUM(remaining_debt), 0) FROM debts WHERE deleted_at IS NULL AND status = @open)::bigint AS outstanding,
		(SELECT COALESCE(SUM(amount), 0) FROM debt_adjustments WHERE created_at >= @from AND created_at < @until)::bigint AS new_debt,
		(SELECT COALESCE(SUM(nominal_bayar), 0) FROM debt_payments
			WHERE tanggal_bayar >= @from AND tanggal_bayar < @until)::bigint AS paid`, args).
		Scan(&summary.Debt).Error; err != nil {
		return nil, fmt.Errorf("failed to get debt summary: %w", err)
	}
	summary.Debt.New += debtSales

	// "Online" = token push yang masih terpasang ke user, artinya device sedang login.
	if err := db.Raw(`SELECT platform, COUNT(*) AS count FROM device_tokens
		WHERE user_id IS NOT NULL GROUP BY platform ORDER BY platform`).Scan(&summary.DevicesOnline).Error; err != nil {
		return nil, fmt.Errorf("failed to count devices: %w", err)
	}

	if err := db.Model(&domain.Transactions{}).
		Where("stock_conflict AND conflict_reviewed_at IS NULL").
		Count(&summary.OpenStockConflicts).Error; err != nil {
		return nil, fmt.Errorf("failed to count stock conflicts: %w", err)
	}
	return summary, nil
}
