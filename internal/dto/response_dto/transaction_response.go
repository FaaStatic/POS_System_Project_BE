package responsedto

import "github.com/google/uuid"

type AddTransactionResponse struct {
	TransactionID    uuid.UUID            `json:"transaction_id"`
	NoInvoice        string               `json:"no_invoice"`
	TotalTransaction int64                `json:"total_transaction"`
	PaymentType      string               `json:"payment_type"`
	StockConflict    bool                 `json:"stock_conflict"`
	DebtInfo         *DebtTransactionInfo `json:"debt_info,omitempty"`
	AmountPaid       *int64               `json:"amount_paid,omitempty"`
	ChangeAmount     *int64               `json:"change_amount,omitempty"`
}

// ChangeLineResponse: satu bentuk kembalian (tunai, permen, atau produk lain).
type ChangeLineResponse struct {
	Kind        string     `json:"kind"`
	Amount      int64      `json:"amount"`
	ProductID   *uuid.UUID `json:"product_id,omitempty"`
	ProductName string     `json:"product_name,omitempty"`
	Qty         float64    `json:"qty,omitempty"`
}

type SyncTransactionsResponse struct {
	Results []SyncTransactionResult `json:"results"`
}

// Status: created | duplicate | failed. Retryable=false berarti item tidak akan
// berhasil walau dikirim ulang (mis. produk sudah dihapus).
type SyncTransactionResult struct {
	ClientRef     string     `json:"client_ref"`
	Status        string     `json:"status"`
	TransactionID *uuid.UUID `json:"transaction_id,omitempty"`
	NoInvoice     string     `json:"no_invoice,omitempty"`
	StockConflict bool       `json:"stock_conflict"`
	Retryable     bool       `json:"retryable"`
	Error         string     `json:"error,omitempty"`
	// DebtInfo hanya ada untuk penjualan hutang yang baru dibuat: saldo pelanggan
	// sebelum dan sesudah, supaya struk kasir bisa mengganti angka perkiraan offline.
	DebtInfo *DebtTransactionInfo `json:"debt_info,omitempty"`
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
	TransactionID    uuid.UUID            `json:"transaction_id"`
	InvoiceNumber    string               `json:"invoice_number"`
	PaymentType      int                  `json:"payment_type"`
	TotalTransaction int64                `json:"total_transaction"`
	TotalProfit      int64                `json:"total_profit"`
	AmountPaid       *int64               `json:"amount_paid,omitempty"`
	ChangeAmount     *int64               `json:"change_amount,omitempty"`
	Change           []ChangeLineResponse `json:"change,omitempty"`
	StockConflict    bool                 `json:"stock_conflict"`
	ConflictReviewed bool                 `json:"conflict_reviewed"`
	// Status: success or cancelled. CancelReason is set only when cancelled.
	Status             string                        `json:"status"`
	CancelReason       string                        `json:"cancel_reason,omitempty"`
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
