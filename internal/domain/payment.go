package domain

import (
	"context"
	"errors"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentSettling PaymentStatus = "settling"
	PaymentSuccess  PaymentStatus = "success"
	PaymentFailed   PaymentStatus = "failed"
	PaymentExpired  PaymentStatus = "expired"
)

var ErrPaymentAccessDenied = errors.New("payment access denied")

type PaymentItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Qty       float64   `json:"qty"`
	UnitPrice int64     `json:"unit_price"`
}

type Payment struct {
	ID             uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID        string        `gorm:"type:varchar(50);uniqueIndex;not null" json:"order_id"`
	UserID         uuid.UUID     `gorm:"type:uuid;not null" json:"user_id"`
	CustomerID     *uuid.UUID    `gorm:"type:uuid" json:"customer_id,omitempty"`
	Method         string        `gorm:"type:varchar(20);not null" json:"method"`
	SubtotalAmount int64         `gorm:"type:bigint;not null;default:0" json:"subtotal_amount"`
	FeeAmount      int64         `gorm:"type:bigint;not null;default:0" json:"fee_amount"`
	GrossAmount    int64         `gorm:"type:bigint;not null" json:"gross_amount"`
	Status         PaymentStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`

	MidtransTrxID  string `gorm:"column:midtrans_trx_id;type:varchar(100)" json:"midtrans_transaction_id"`
	MidtransStatus string `gorm:"column:midtrans_status;type:varchar(30)" json:"midtrans_status"`
	FraudStatus    string `gorm:"column:fraud_status;type:varchar(30)" json:"fraud_status"`
	QRString       string `gorm:"column:qr_string;type:text" json:"qr_string"`
	QRURL          string `gorm:"column:qr_url;type:text" json:"qr_url"`
	RedirectURL    string `gorm:"column:redirect_url;type:text" json:"redirect_url"`

	VABank     string `gorm:"column:va_bank;type:varchar(20)" json:"va_bank,omitempty"`
	VANumber   string `gorm:"column:va_number;type:varchar(50)" json:"va_number,omitempty"`
	BillKey    string `gorm:"column:bill_key;type:varchar(50)" json:"bill_key,omitempty"`
	BillerCode string `gorm:"column:biller_code;type:varchar(20)" json:"biller_code,omitempty"`

	Items         []PaymentItem `gorm:"serializer:json;type:jsonb" json:"items"`
	ExpiryTime    *time.Time    `json:"expiry_time,omitempty"`
	PaidAt        *time.Time    `json:"paid_at,omitempty"`
	TransactionID *uuid.UUID    `gorm:"type:uuid" json:"transaction_id,omitempty"`

	StockReserved bool `gorm:"column:stock_reserved;not null;default:false" json:"stock_reserved"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (p *Payment) TableName() string {
	return "payments"
}

type GatewayChargeInput struct {
	OrderID     string
	GrossAmount int64
	Bank        string
}

type GatewayChargeResult struct {
	TransactionID     string
	OrderID           string
	PaymentType       string
	TransactionStatus string
	FraudStatus       string
	StatusCode        string
	QRString          string
	QRURL             string
	RedirectURL       string
	ExpiryTime        string

	VANumber   string
	Bank       string
	BillKey    string
	BillerCode string
}

type PaymentGateway interface {
	ChargeQris(ctx context.Context, in GatewayChargeInput) (*GatewayChargeResult, error)
	ChargeVA(ctx context.Context, in GatewayChargeInput) (*GatewayChargeResult, error)
	CheckStatus(ctx context.Context, orderID string) (*GatewayChargeResult, error)
	VerifySignature(orderID, statusCode, grossAmount, signatureKey string) bool
}

type PaymentRepository interface {
	CreateWithReservation(ctx context.Context, payment *Payment) error
	GetByOrderID(ctx context.Context, orderID string) (*Payment, error)
	UpdateWithLock(ctx context.Context, orderID string, fn func(payment *Payment) (save bool, err error)) error
	ListStalePending(ctx context.Context, limit int) ([]*Payment, error)
	ListPendingFinalization(ctx context.Context, limit int) ([]*Payment, error)
	Touch(ctx context.Context, orderID string) error
}

type PaymentUsecase interface {
	ChargeQris(ctx context.Context, request *requestdto.ChargeQrisRequest) (*responsedto.ChargePaymentResponse, error)
	ChargeVA(ctx context.Context, request *requestdto.ChargeVARequest) (*responsedto.ChargePaymentResponse, error)
	HandleNotification(ctx context.Context, notif *requestdto.MidtransNotificationRequest) error
	GetStatus(ctx context.Context, orderID, requesterID, requesterRole string) (*responsedto.PaymentStatusResponse, error)
	ReconcileStalePayments(ctx context.Context) error
}
