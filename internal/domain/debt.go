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
	CustomerID    uuid.UUID       `gorm:"type:uuid;not null" json:"customer_id"`
	TotalDebt     int64           `gorm:"type:bigint;not null" json:"total_debt"`
	RemainingDebt int64           `gorm:"type:bigint;not null" json:"remaining_debt"`
	Status        enum.DebtStatus `gorm:"type:smallint;check:status IN (0,1);default:0" json:"status"`
	DueDate       time.Time       `gorm:"type:date" json:"due_date"`
	CreatedAt     time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt  `gorm:"index" json:"-"`

	Customer     Customers      `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Transactions []Transactions `gorm:"foreignKey:DebtID" json:"transactions,omitempty"`
	DebtPayments []DebtPayments `gorm:"foreignKey:DebtID" json:"payments"`
}

type DebtPayments struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DebtID                uuid.UUID `gorm:"type:uuid;not null" json:"debt_id"`
	UserID                uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	NominalBayar          int64     `gorm:"type:bigint;not null" json:"nominal_bayar"`
	TanggalBayar          time.Time `gorm:"autoCreateTime" json:"tanggal_bayar"`
	IdempotencyKey        *string   `gorm:"type:varchar(100)" json:"idempotency_key,omitempty"`
	PreviousRemainingDebt int64     `gorm:"type:bigint;not null;default:0" json:"previous_remaining_debt"`

	User *Users `gorm:"foreignKey:UserID" json:"user,omitempty"`
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

type DebtPaymentResult struct {
	Debt                  *Debts
	PreviousRemainingDebt int64
	PaymentID             uuid.UUID
	PaidAt                time.Time
}

type DebtRepository interface {
	AddDebt(ctx context.Context, debt *Debts) error
	DeleteDebt(ctx context.Context, id uuid.UUID) error
	GetAllDebt(ctx context.Context, filter FilterDebt) (*DebtsPaginated, error)
	GetDebtByID(ctx context.Context, id uuid.UUID) (*Debts, error)
	PayDebt(ctx context.Context, debtID uuid.UUID, payment *DebtPayments) (*DebtPaymentResult, error)
}

type DebtUseCase interface {
	AddingDebtCustomer(ctx context.Context, request *requestdto.AddDebtRequest) error
	DeleteDebtCustomer(ctx context.Context, request *requestdto.DeleteDebtRequest) error
	GetAllDebtCustomerList(ctx context.Context, request *requestdto.FilterDebtRequest) (*responsedto.DebtListResponseDto, error)
	GetDebtCustomer(ctx context.Context, request *requestdto.GetDebtRequest) (*responsedto.DebtResponseDto, error)
	PrintReportDebtCustomer(ctx context.Context, request *requestdto.PrintDebtReport) (*responsedto.PrintDebtCustomerResponse, error)
	PayDebtCash(ctx context.Context, request *requestdto.DebtPayment) (*responsedto.DebtPaymentResponse, error)
}
