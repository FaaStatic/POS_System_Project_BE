package requestdto

type DebtPayment struct {
	DebtID         string  `json:"debt_id" validate:"required,uuid"`
	UserID         string  `json:"user_id" validate:"required,uuid"`
	NominalBayar   int64   `json:"nominal_bayar" validate:"required,gt=0"`
	IdempotencyKey *string `json:"idempotency_key,omitempty" validate:"omitempty,max=100"`
}

type GetDebtRequest struct {
	DebtId string `query:"debt_id" validate:"required,uuid"`
}

type FilterDebtRequest struct {
	CustomerId string  `query:"customer_id"`
	Search     string  `query:"search"`
	Status     *int    `query:"status" validate:"omitempty,oneof=0 1"`
	Limit      int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Order      string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterID    *string `query:"after_id,omitempty"`
	AfterTime  *string `query:"after_time,omitempty"`
}

type FilterDebtPaymentRequest struct {
	CustomerId string  `query:"customer_id" validate:"omitempty,uuid"`
	Search     string  `query:"search"`
	Limit      int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Order      string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterID    *string `query:"after_id,omitempty"`
	AfterTime  *string `query:"after_time,omitempty"`
}

type AddDebtRequest struct {
	CustomerID     string `json:"customer_id" validate:"required,uuid"`
	TotalTransaksi int64  `json:"total_transaksi" validate:"required,gt=0"`
	JatuhTempo     string `json:"jatuh_tempo,omitempty"`
	Note           string `json:"note,omitempty" validate:"omitempty,max=200"`
	UserID         string `json:"-" validate:"required,uuid"`
}

type PrintDebtPaymentReceipt struct {
	ID string `validate:"required,uuid"`
}

// PrintDebtPaymentsMonth: bulan dan tahun kosong berarti bulan ini.
type PrintDebtPaymentsMonth struct {
	Month int `query:"month" validate:"omitempty,min=1,max=12"`
	Year  int `query:"year" validate:"omitempty,min=2000,max=2100"`
}

type PrintDebtReport struct {
	DebtId string `query:"debt_id" validate:"required,uuid"`
}
