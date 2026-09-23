package requestdto

type PaymentItemRequest struct {
	ProductId string  `json:"product_id" validate:"required,uuid"`
	Qty       float64 `json:"qty" validate:"required,gt=0"`
}

type ChargeQrisRequest struct {
	UserId     string               `json:"user_id" validate:"required,uuid"`
	CustomerId *string              `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	Items      []PaymentItemRequest `json:"items" validate:"required,min=1,dive"`
}

type ChargeVARequest struct {
	UserId     string               `json:"user_id" validate:"required,uuid"`
	CustomerId *string              `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	Bank       string               `json:"bank" validate:"required,oneof=bca mandiri"`
	Items      []PaymentItemRequest `json:"items" validate:"required,min=1,dive"`
}

type MidtransNotificationRequest struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	PaymentType       string `json:"payment_type"`
	TransactionID     string `json:"transaction_id"`
}
