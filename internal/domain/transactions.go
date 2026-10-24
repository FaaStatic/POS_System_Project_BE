package domain

import (
	"context"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/constant/paginated"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Transactions struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	NoInvoice  string     `gorm:"type:varchar(50)" json:"no_invoice"`
	UserID     uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	CustomerID *uuid.UUID `gorm:"column:customer_id" json:"customer_id"`
	DebtID     *uuid.UUID `gorm:"column:debt_id" json:"debt_id"`

	PaymentType      enum.MoneyPayment `gorm:"type:smallint" json:"payment_type"`
	Bank             *string           `gorm:"type:varchar(20)" json:"bank,omitempty"`
	TotalTransaction int64             `gorm:"type:bigint" json:"total_transaction"`

	// Diisi hanya untuk transaksi offline dari kasir (POST /transactions/sync).
	ClientRef          *uuid.UUID `gorm:"type:uuid" json:"client_ref,omitempty"`
	StockConflict      bool       `gorm:"default:false" json:"stock_conflict"`
	ConflictReviewedAt *time.Time `json:"conflict_reviewed_at,omitempty"`
	CancelReason       *string    `gorm:"type:text" json:"cancel_reason,omitempty"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-"`

	User              Users                `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Customer          Customers            `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	TransactionDetail []TransactionsDetail `gorm:"foreignKey:TransactionID" json:"details"`
}

type TransactionsDetail struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TransactionID uuid.UUID `gorm:"type:uuid" json:"transaction_id"`
	ProductID     uuid.UUID `gorm:"type:uuid" json:"product_id"`
	ProductName   string    `gorm:"type:varchar(255);default:''" json:"product_name"`
	Price         int64     `gorm:"type:bigint" json:"price"`
	PriceDebt     int64     `gorm:"type:bigint" json:"price_debt"`
	PurchasePrice int64     `gorm:"type:bigint;default:0" json:"purchase_price"`
	Qty           float64   `gorm:"type:decimal(8,2)" json:"qty"`
	Subtotal      int64     `gorm:"type:bigint" json:"subtotal"`
	Destination   *string   `gorm:"type:varchar(50)" json:"destination,omitempty"`

	Product Products `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (t *Transactions) TableName() string {
	return "transactions"
}

func (td *TransactionsDetail) TableName() string {
	return "transactions_detail"
}

var StoreLocation = time.FixedZone("WIB", 7*60*60)

type FilterTransaction struct {
	NoInvoices string
	CustomerID *uuid.UUID
	Cursor     *paginated.CursorMeta
	DateFrom   *time.Time
	DateUntil  *time.Time
	Limit      int
	TypeTrx    *int
	Order      string
	// OpenConflict: hanya transaksi offline dengan stok minus yang belum ditinjau.
	OpenConflict bool
}

type ResultTransaction struct {
	DataItem []*Transactions
	HasNext  bool
	Cursor   *paginated.CursorMeta
}

// DailyReport memisahkan penjualan non-hutang dan hutang; Cost/DebtCost adalah
// HPP dari snapshot purchase_price di transactions_detail.
type DailyReport struct {
	Date      time.Time `gorm:"column:date"`
	CountSale int64     `gorm:"column:count_sale"`
	Revenue   int64     `gorm:"column:revenue"`
	Cost      int64     `gorm:"column:cost"`
	CountDebt int64     `gorm:"column:count_debt"`
	DebtSales int64     `gorm:"column:debt_sales"`
	DebtCost  int64     `gorm:"column:debt_cost"`
}

type DailyDebtFlow struct {
	Date   time.Time `gorm:"column:date"`
	Manual int64     `gorm:"column:manual"`
	Paid   int64     `gorm:"column:paid"`
}

type ProductSoldReport struct {
	ProductName string  `gorm:"column:product_name"`
	Qty         float64 `gorm:"column:qty"`
	Total       int64   `gorm:"column:total"`
}

type DailyProductSoldReport struct {
	Date        time.Time `gorm:"column:date"`
	ProductName string    `gorm:"column:product_name"`
	Qty         float64   `gorm:"column:qty"`
	Total       int64     `gorm:"column:total"`
}

type TransactionDebtSnapshot struct {
	DebtID                uuid.UUID
	PreviousRemainingDebt int64
	AmountAdded           int64
	TotalDebt             int64
	RemainingDebt         int64
	Status                enum.DebtStatus
}

type TransactionRepository interface {
	CreateTransaction(ctx context.Context, transaction *Transactions, isHutang bool, deductStock bool) (*TransactionDebtSnapshot, error)
	GetTransactionByID(ctx context.Context, id uuid.UUID) (*Transactions, error)
	GetTransactionByClientRef(ctx context.Context, ref uuid.UUID) (*Transactions, error)
	GetAllTransaction(ctx context.Context, filter FilterTransaction) (*ResultTransaction, error)
	DeleteTransaction(ctx context.Context, id uuid.UUID, reason string) error
	MarkConflictReviewed(ctx context.Context, id uuid.UUID) error
	CheckTransactionByNoInvoice(ctx context.Context, noInvoice string) (*Transactions, error)
	GetDailyReport(ctx context.Context, month int, year int) ([]DailyReport, error)
	GetDailyDebtFlow(ctx context.Context, month int, year int) ([]DailyDebtFlow, error)
	GetMonthlyProductSold(ctx context.Context, month int, year int) ([]ProductSoldReport, error)
	GetDailyProductSold(ctx context.Context, month int, year int) ([]DailyProductSoldReport, error)
}

type TransactionUsecase interface {
	AddTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest) (*responsedto.AddTransactionResponse, error)
	AddPrepaidTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest) (*responsedto.AddTransactionResponse, error)
	SyncOfflineTransactions(ctx context.Context, dto *requestdto.SyncTransactionsRequest) (*responsedto.SyncTransactionsResponse, error)
	ReviewConflict(ctx context.Context, id string) error
	GetTransaction(ctx context.Context, dto *requestdto.GetTransactionRequest) (*responsedto.TransactionResponse, error)
	GetAllTransaction(ctx context.Context, dto *requestdto.FilterTransactionRequest) (*responsedto.GetAllTransactionResponse, error)
	DeleteTransaction(ctx context.Context, dto *requestdto.DeleteTransactionRequest) error
	PrintReportTransaction(ctx context.Context, dto *requestdto.PrintReportTransactionRequest) (*responsedto.PrintReportTransactionResponse, error)
	PrintReportMonth(ctx context.Context, dto *requestdto.PrintReportMonthRequest) (*responsedto.PrintReportMonthTransactionResponse, error)
}
