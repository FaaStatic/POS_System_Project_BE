package responsedto

import "github.com/google/uuid"

type CustomerDtoResponse struct {
	ID     uuid.UUID         `json:"id"`
	Nama   string            `json:"nama"`
	NoHP   string            `json:"no_hp"`
	Alamat string            `json:"alamat"`
	Debts  []DebtResponseDto `json:"debts,omitempty"`
}

type ListCustomerDtoResponse struct {
	AfterId       string                `json:"after_id"`
	AfterTime     string                `json:"after_time"`
	HasNext       bool                  `json:"has_next"`
	CustomerLists []CustomerDtoResponse `json:"customer_lists"`
}
