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

type Debts struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CustomerID    uuid.UUID       `gorm:"type:uuid" json:"customer_id"`
	TotalDebt     int64           `gorm:"type:bigint" json:"total_debt"`
	RemainingDebt int64           `gorm:"type:bigint" json:"remaining_debt"`
	Status        enum.DebtStatus `gorm:"type:smallint;default:0" json:"status"`
	DueDate       time.Time       `gorm:"type:date" json:"due_date"`
	CreatedAt     time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt  `json:"-"`

	Customer     Customers      `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Transactions []Transactions `gorm:"foreignKey:DebtID" json:"transactions,omitempty"`
	DebtPayments []DebtPayments `gorm:"foreignKey:DebtID" json:"payments"`
}

type DebtPayments struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DebtID                uuid.UUID `gorm:"type:uuid" json:"debt_id"`
	UserID                uuid.UUID `gorm:"type:uuid" json:"user_id"`
	NominalBayar          int64     `gorm:"type:bigint" json:"nominal_bayar"`
	TanggalBayar          time.Time `gorm:"autoCreateTime" json:"tanggal_bayar"`
	IdempotencyKey        *string   `gorm:"type:varchar(100)" json:"idempotency_key,omitempty"`
	PreviousRemainingDebt int64     `gorm:"type:bigint;default:0" json:"previous_remaining_debt"`

	User *Users `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Debt *Debts `gorm:"foreignKey:DebtID" json:"-"`
}

// DebtAdjustments mencatat hutang manual (POST /api/v1/sales/debts) yang tidak punya
// transaksi: siapa input, berapa, kapan.
type DebtAdjustments struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DebtID    uuid.UUID `gorm:"type:uuid" json:"debt_id"`
	UserID    uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Amount    int64     `gorm:"type:bigint" json:"amount"`
	Note      *string   `gorm:"type:varchar(200)" json:"note,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (d *DebtAdjustments) TableName() string {
	return "debt_adjustments"
}

func (d *Debts) TableName() string {
	return "debts"
}

func (d *DebtPayments) TableName() string {
	return "debt_payments"
}

type DebtsPaginated struct {
	Data    []*Debts              `json:"data"`
	HasNext bool                  `json:"has_next"`
	Cursor  *paginated.CursorMeta `json:"cursor,omitempty"`
}

type FilterDebt struct {
	Cursor     *paginated.CursorMeta `json:"cursor,omitempty"`
	Limit      int                   `json:"limit"`
	CustomerID uuid.UUID             `json:"customer_id"`
	Order      string                `json:"order"`
	Status     *enum.DebtStatus      `json:"status"`
	Search     string                `json:"search"`
}

// FilterDebtPayment menyaring riwayat cicilan; CustomerID kosong berarti semua pelanggan.
type FilterDebtPayment struct {
	Cursor     *paginated.CursorMeta
	Limit      int
	CustomerID uuid.UUID
	Order      string
	Search     string
}

type DebtPaymentsPaginated struct {
	Data    []*DebtPayments
	HasNext bool
	Cursor  *paginated.CursorMeta
}

const (
	DebtEntrySale   = "sale"
	DebtEntryManual = "manual"
)

// DebtEntry satu baris yang membentuk saldo hutang terbuka: penjualan hutang
// (DebtEntrySale) atau hutang manual (DebtEntryManual).
type DebtEntry struct {
	Type    string
	Amount  int64
	At      time.Time
	Invoice string
	Note    string
}

type DebtPaymentResult struct {
	Debt                  *Debts
	PreviousRemainingDebt int64
	PaymentID             uuid.UUID
	PaidAt                time.Time
}

type DebtRepository interface {
	AddDebt(ctx context.Context, debt *Debts, userID uuid.UUID, note string) error
	GetAllDebt(ctx context.Context, filter FilterDebt) (*DebtsPaginated, error)
	GetDebtByID(ctx context.Context, id uuid.UUID) (*Debts, error)
	GetDebtWithPayments(ctx context.Context, id uuid.UUID) (*Debts, error)
	PayDebt(ctx context.Context, debtID uuid.UUID, payment *DebtPayments) (*DebtPaymentResult, error)
	GetAllDebtPayments(ctx context.Context, filter FilterDebtPayment) (*DebtPaymentsPaginated, error)
	GetDebtPaymentByID(ctx context.Context, id uuid.UUID) (*DebtPayments, error)
	// GetDebtPaymentsInRange: cicilan dengan tanggal bayar di [from, until), terlama dulu.
	GetDebtPaymentsInRange(ctx context.Context, from, until time.Time) ([]*DebtPayments, error)
	GetDebtEntries(ctx context.Context, debtID uuid.UUID, limit int) ([]DebtEntry, error)
}

type DebtUseCase interface {
	AddingDebtCustomer(ctx context.Context, request *requestdto.AddDebtRequest) error
	GetAllDebtCustomerList(ctx context.Context, request *requestdto.FilterDebtRequest) (*responsedto.DebtListResponseDto, error)
	GetDebtCustomer(ctx context.Context, request *requestdto.GetDebtRequest) (*responsedto.DebtResponseDto, error)
	PrintReportDebtCustomer(ctx context.Context, request *requestdto.PrintDebtReport) (*responsedto.PrintDebtCustomerResponse, error)
	PayDebtCash(ctx context.Context, request *requestdto.DebtPayment) (*responsedto.DebtPaymentResponse, error)
	GetAllDebtPaymentList(ctx context.Context, request *requestdto.FilterDebtPaymentRequest) (*responsedto.DebtPaymentListResponseDto, error)
	PrintDebtPaymentReceipt(ctx context.Context, request *requestdto.PrintDebtPaymentReceipt) (*responsedto.PrintDebtPaymentResponse, error)
	PrintDebtPaymentsMonth(ctx context.Context, request *requestdto.PrintDebtPaymentsMonth) (*responsedto.PrintDebtPaymentsMonthResponse, error)
}
