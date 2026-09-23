package responsedto

type ChargePaymentResponse struct {
	OrderID     string `json:"order_id"`
	Method      string `json:"method"`
	Status      string `json:"status"`
	GrossAmount int64  `json:"gross_amount"`

	MidtransStatus string `json:"midtrans_status"`
	QrString       string `json:"qr_string,omitempty"`
	QrUrl          string `json:"qr_url,omitempty"`
	RedirectUrl    string `json:"redirect_url,omitempty"`
	VaNumber       string `json:"va_number,omitempty"`
	Bank           string `json:"bank,omitempty"`
	BillKey        string `json:"bill_key,omitempty"`
	BillerCode     string `json:"biller_code,omitempty"`
	ExpiryTime     string `json:"expiry_time,omitempty"`
}

type PaymentStatusResponse struct {
	OrderID        string `json:"order_id"`
	Method         string `json:"method"`
	Status         string `json:"status"`
	MidtransStatus string `json:"midtrans_status"`
	GrossAmount    int64  `json:"gross_amount"`
	TransactionID  string `json:"transaction_id,omitempty"`
	PaidAt         string `json:"paid_at,omitempty"`
}
