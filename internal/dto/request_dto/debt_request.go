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

type AddDebtRequest struct {
	CustomerID     string `json:"customer_id" validate:"required,uuid"`
	TotalTransaksi int64  `json:"total_transaksi" validate:"required,gt=0"`
	JatuhTempo     string `json:"jatuh_tempo" validate:"required"`
}

type DeleteDebtRequest struct {
	DebtId string `json:"debt_id" validate:"required,uuid"`
}

type PrintDebtReport struct {
	DebtId string `query:"debt_id" validate:"required,uuid"`
}
