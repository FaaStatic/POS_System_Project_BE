package responsedto

type DebtResponseDto struct {
	DebtID        string  `json:"debt_id"`
	Status        string  `json:"status"`
	CustomerID    string  `json:"customer_id"`
	NameCustomer  string  `json:"name_customer"`
	TotalDebt     int64   `json:"total_debt"`
	RemainingDebt int64   `json:"remaining_debt"`
	DateDebt      *string `json:"date_debt"`
	// Entries hanya diisi oleh GET /debts/:id: rincian terbaru yang membentuk saldo,
	// penjualan hutang (sale) dan hutang manual (manual).
	Entries []DebtEntryDto `json:"entries,omitempty"`
}

type DebtEntryDto struct {
	Type    string `json:"type"`
	Amount  int64  `json:"amount"`
	Date    string `json:"date"`
	Invoice string `json:"invoice,omitempty"`
	Note    string `json:"note,omitempty"`
}

// DebtPaymentItemDto satu cicilan untuk riwayat. ClientRef adalah idempotency_key yang
// dikirim kasir (nomor struk), kosong untuk cicilan dari CMS.
type DebtPaymentItemDto struct {
	ID                    string `json:"id"`
	DebtID                string `json:"debt_id"`
	CustomerID            string `json:"customer_id"`
	CustomerName          string `json:"customer_name"`
	NominalBayar          int64  `json:"nominal_bayar"`
	PreviousRemainingDebt int64  `json:"previous_remaining_debt"`
	RemainingDebt         int64  `json:"remaining_debt"`
	Status                string `json:"status"`
	Cashier               string `json:"cashier"`
	ClientRef             string `json:"client_ref,omitempty"`
	PaidAt                string `json:"paid_at"`
}

type PrintDebtPaymentResponse struct {
	ID     string `json:"id"`
	UrlPdf string `json:"url_pdf"`
}

type PrintDebtPaymentsMonthResponse struct {
	Month  string `json:"month"`
	Year   string `json:"year"`
	Count  int    `json:"count"`
	UrlPdf string `json:"url_pdf"`
}

type DebtPaymentListResponseDto struct {
	AfterId     string               `json:"after_id"`
	AfterTime   string               `json:"after_time"`
	HasNext     bool                 `json:"has_next"`
	PaymentList []DebtPaymentItemDto `json:"payment_list"`
}

type DebtListResponseDto struct {
	AfterId         string            `json:"after_id"`
	AfterTime       string            `json:"after_time"`
	HasNext         bool              `json:"has_next"`
	TransactionList []DebtResponseDto `json:"transaction_list"`
}

type PrintDebtCustomerResponse struct {
	CustomerName string `json:"customer_name"`
	DebtId       string `json:"debt_id"`
	UrlPdf       string `json:"url_pdf"`
}

type DebtPaymentResponse struct {
	DebtId                string `json:"debt_id"`
	CustomerName          string `json:"customer_name"`
	NominalBayar          int64  `json:"nominal_bayar"`
	PreviousRemainingDebt int64  `json:"previous_remaining_debt"`
	RemainingDebt         int64  `json:"remaining_debt"`
	TotalDebt             int64  `json:"total_debt"`
	Status                string `json:"status"`
	PaidAt                string `json:"paid_at"`
}
