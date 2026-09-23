package responsedto

type DebtResponseDto struct {
	NameCustomer    string                `json:"name_customer"`
	TotalDebt       int64                 `json:"total_debt"`
	RemainingDebt   int64                 `json:"remaining_debt"`
	DateDebt        *string               `json:"date_debt"`
	TransactionList []TransactionResponse `json:"transaction_list"`
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
