package requestdto

type AddTransactionRequest struct {
	NoInvoice   string                        `json:"no_invoice" validate:"required"`
	TypePayment string                        `json:"type_payment" validate:"required,oneof=tunai hutang transfer qris"`
	UserId      string                        `json:"user_id" validate:"required,uuid"`
	CustomerId  *string                       `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	Bank        *string                       `json:"bank,omitempty" validate:"omitempty,oneof=bca mandiri"`
	Details     []AddTransactionDetailRequest `json:"details" validate:"required,min=1,dive"`
}

type AddTransactionDetailRequest struct {
	ProductId   string  `json:"product_id" validate:"required,uuid"`
	Qty         float64 `json:"qty" validate:"required,gt=0"`
	Destination *string `json:"destination,omitempty"`
	UnitPrice   *int64  `json:"-"`
}

type GetTransactionRequest struct {
	ID string `query:"id" validate:"required,uuid"`
}

type FilterTransactionRequest struct {
	UserId        string  `query:"user_id" validate:"required"`
	CustomerID    string  `query:"-" validate:"omitempty,uuid"`
	DateStart     *string `query:"date_start,omitempty"`
	DateEnd       *string `query:"date_end,omitempty"`
	TypePayment   *int    `query:"type_payment" validate:"omitempty,oneof=0 1 2 3"`
	InvoiceNumber string  `query:"number_invoices,omitempty"`
	Limit         int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Order         string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterTime     *string `query:"after_time,omitempty"`
	AfterID       *string `query:"after_id,omitempty"`
	OpenConflict  bool    `query:"open_conflict"`
}

type DeleteTransactionRequest struct {
	ID     string `json:"trx_id" validate:"required,uuid"`
	Reason string `json:"reason" validate:"required,min=3,max=500"`
	UserID string `json:"-"`
	Role   string `json:"-"`
}

// SyncTransactionsRequest sengaja tidak memakai `dive`: satu item rusak tidak
// boleh menahan seluruh antrean, jadi tiap item divalidasi di usecase dan
// gagal sendiri-sendiri.
type SyncTransactionsRequest struct {
	UserId       string               `json:"-"`
	Transactions []OfflineTransaction `json:"transactions" validate:"required,min=1,max=100"`
}

type OfflineTransaction struct {
	ClientRef       string                     `json:"client_ref"`
	CreatedAtClient string                     `json:"created_at_client"`
	TypePayment     string                     `json:"type_payment"`
	CustomerId      *string                    `json:"customer_id,omitempty"`
	Details         []OfflineTransactionDetail `json:"details"`
}

type OfflineTransactionDetail struct {
	ProductId   string  `json:"product_id"`
	Qty         float64 `json:"qty"`
	UnitPrice   int64   `json:"unit_price"`
	Destination *string `json:"destination,omitempty"`
}

type PrintReportTransactionRequest struct {
	TrxId     string `query:"trx_id,omitempty"`
	NoInvoice string `query:"number_invoice,omitempty"`
}

type PrintReportMonthRequest struct {
	UserId string `query:"user_id" validate:"required"`
	Month  int    `query:"month"`
	Year   int    `query:"year"`
}
