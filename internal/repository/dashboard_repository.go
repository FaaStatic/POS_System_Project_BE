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
func salesByDayAndMethod(db *gorm.DB, from, until time.Time) *gorm.DB {
	perTransaction := db.Table("transactions AS t").
		Select("t.created_at, t.payment_type, t.total_transaction, COALESCE(SUM(ROUND((d.price - d.purchase_price) * d.qty)), 0) AS profit").
		Joins("LEFT JOIN transactions_detail d ON d.transaction_id = t.id").
		Where("t.deleted_at IS NULL AND t.created_at >= ? AND t.created_at < ?", from, until).
		Group("t.id")
	return db.Table("(?) AS s", perTransaction).
		Select("to_char(" + storeDate("s.created_at") + ", 'YYYY-MM-DD') AS date, s.payment_type, COUNT(*) AS transactions, " +
			"SUM(s.total_transaction)::bigint AS revenue, SUM(s.profit)::bigint AS gross_profit").
		Group("1, 2").Order("1, 2")
}

type salesRow struct {
	Date         string `gorm:"column:date"`
	PaymentType  int    `gorm:"column:payment_type"`
	Transactions int64  `gorm:"column:transactions"`
	Revenue      int64  `gorm:"column:revenue"`
	GrossProfit  int64  `gorm:"column:gross_profit"`
}

func (r *dashboardRepository) Summary(ctx context.Context, from, until time.Time) (*domain.DashboardSummary, error) {
	db := r.db.WithContext(ctx)
	summary := &domain.DashboardSummary{}

	var rows []salesRow
	if err := salesByDayAndMethod(db, from, until).Scan(&rows).Error; err != nil {
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

	if err := soldLinesBetween(db, from, until).
		Select(soldProductName + " AS product_name, SUM(transactions_detail.qty) AS qty, COALESCE(SUM(transactions_detail.subtotal), 0)::bigint AS total").
		Group("transactions_detail.product_id, " + soldProductName).
		Order("total DESC").Limit(5).
		Scan(&summary.TopProducts).Error; err != nil {
		return nil, fmt.Errorf("failed to get top products: %w", err)
	}

	// Hutang baru = penjualan hutang (dari rows di atas) + hutang manual; dibayar = cicilan masuk.
	if err := db.Model(&domain.Debts{}).Where("status = ?", enum.BELUM_LUNAS).
		Select("COALESCE(SUM(remaining_debt), 0)::bigint").Scan(&summary.Debt.Outstanding).Error; err != nil {
		return nil, fmt.Errorf("failed to get debt summary: %w", err)
	}
	if err := db.Model(&domain.DebtAdjustments{}).Where("created_at >= ? AND created_at < ?", from, until).
		Select("COALESCE(SUM(amount), 0)::bigint").Scan(&summary.Debt.New).Error; err != nil {
		return nil, fmt.Errorf("failed to get debt summary: %w", err)
	}
	if err := db.Model(&domain.DebtPayments{}).Where("tanggal_bayar >= ? AND tanggal_bayar < ?", from, until).
		Select("COALESCE(SUM(nominal_bayar), 0)::bigint").Scan(&summary.Debt.Paid).Error; err != nil {
		return nil, fmt.Errorf("failed to get debt summary: %w", err)
	}
	summary.Debt.New += debtSales

	// "Online" = token push yang masih terpasang ke user, artinya device sedang login.
	if err := db.Model(&domain.DeviceToken{}).Where("user_id IS NOT NULL").
		Select("platform, COUNT(*) AS count").Group("platform").Order("platform").
		Scan(&summary.DevicesOnline).Error; err != nil {
		return nil, fmt.Errorf("failed to count devices: %w", err)
	}

	if err := db.Model(&domain.Transactions{}).
		Where("stock_conflict AND conflict_reviewed_at IS NULL").
		Count(&summary.OpenStockConflicts).Error; err != nil {
		return nil, fmt.Errorf("failed to count stock conflicts: %w", err)
	}
	return summary, nil
}
