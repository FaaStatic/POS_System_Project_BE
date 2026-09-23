package responsedto

import "github.com/google/uuid"

type AddTransactionResponse struct {
	TransactionID    uuid.UUID            `json:"transaction_id"`
	NoInvoice        string               `json:"no_invoice"`
	TotalTransaction int64                `json:"total_transaction"`
	PaymentType      string               `json:"payment_type"`
	DebtInfo         *DebtTransactionInfo `json:"debt_info,omitempty"`
}

type DebtTransactionInfo struct {
	DebtID                string `json:"debt_id"`
	PreviousRemainingDebt int64  `json:"previous_remaining_debt"`
	AmountAdded           int64  `json:"amount_added"`
	TotalDebt             int64  `json:"total_debt"`
	RemainingDebt         int64  `json:"remaining_debt"`
	Status                string `json:"status"`
}

type ProductTransactionResponse struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       int64     `json:"price"`
	Qty         float64   `json:"qty"`
	Subtotal    int64     `json:"subtotal"`
}

type TransactionResponse struct {
	TransactionID      uuid.UUID                     `json:"transaction_id"`
	InvoiceNumber      string                        `json:"invoice_number"`
	PaymentType        int                           `json:"payment_type"`
	TotalTransaction   int64                         `json:"total_transaction"`
	TotalProfit        int64                         `json:"total_profit"`
	CreatedAt          string                        `json:"created_at"`
	TransactionDetails []*ProductTransactionResponse `json:"transaction_details"`
}

type GetAllTransactionResponse struct {
	UserID          string                 `json:"user_id"`
	AfterId         string                 `json:"after_id"`
	AfterTime       string                 `json:"after_time"`
	HasNext         bool                   `json:"has_next"`
	TransactionList []*TransactionResponse `json:"list_transaction"`
}

type PrintReportTransactionResponse struct {
	ID        uuid.UUID `json:"id"`
	NoInvoice string    `json:"number_invoice"`
	UrlPdf    string    `json:"url_pdf"`
}

type PrintReportMonthTransactionResponse struct {
	ID     uuid.UUID `json:"id"`
	Month  string    `json:"month"`
	Year   string    `json:"year"`
	UrlPdf string    `json:"url_pdf"`
}
