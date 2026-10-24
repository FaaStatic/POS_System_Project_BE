package domain

import (
	"context"
	requestdto "shop_project_be/internal/dto/request_dto"
	"time"
)

// DashboardSummary: Revenue dan GrossProfit mencakup penjualan hutang, karena
// barangnya sudah keluar. Debt.Outstanding adalah saldo saat ini, bukan per rentang.
type DashboardSummary struct {
	DateStart          string                   `json:"date_start"`
	DateEnd            string                   `json:"date_end"`
	TotalTransactions  int64                    `json:"total_transactions"`
	Revenue            int64                    `json:"revenue"`
	GrossProfit        int64                    `json:"gross_profit"`
	Debt               DashboardDebt            `json:"debt"`
	DevicesOnline      []DashboardPlatformCount `json:"devices_online"`
	OpenStockConflicts int64                    `json:"open_stock_conflicts"`
	Daily              []DashboardDay           `json:"daily"`
	TopProducts        []DashboardProduct       `json:"top_products"`
	PaymentMethods     []DashboardPaymentMethod `json:"payment_methods"`
}

type DashboardDebt struct {
	Outstanding int64 `gorm:"column:outstanding" json:"outstanding"`
	New         int64 `gorm:"column:new_debt" json:"new"`
	Paid        int64 `gorm:"column:paid" json:"paid"`
}

type DashboardPlatformCount struct {
	Platform string `gorm:"column:platform" json:"platform"`
	Count    int64  `gorm:"column:count" json:"count"`
}

type DashboardDay struct {
	Date         string `gorm:"column:date" json:"date"`
	Transactions int64  `gorm:"column:transactions" json:"transactions"`
	Revenue      int64  `gorm:"column:revenue" json:"revenue"`
	GrossProfit  int64  `gorm:"column:gross_profit" json:"gross_profit"`
}

type DashboardProduct struct {
	ProductName string  `gorm:"column:product_name" json:"product_name"`
	Qty         float64 `gorm:"column:qty" json:"qty"`
	Total       int64   `gorm:"column:total" json:"total"`
}

type DashboardPaymentMethod struct {
	PaymentType int    `gorm:"column:payment_type" json:"-"`
	Method      string `gorm:"-" json:"method"`
	Count       int64  `gorm:"column:count" json:"count"`
	Total       int64  `gorm:"column:total" json:"total"`
}

type DashboardRepository interface {
	Summary(ctx context.Context, from, until time.Time) (*DashboardSummary, error)
}

type DashboardUsecase interface {
	Summary(ctx context.Context, request *requestdto.DashboardRequest) (*DashboardSummary, error)
}
