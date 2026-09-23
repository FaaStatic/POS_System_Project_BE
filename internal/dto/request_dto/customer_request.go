package requestdto

type AddCustomer struct {
	CustomerName string `json:"customer_name" validate:"required"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	Address      string `json:"address,omitempty"`
}
type UpdateCustomer struct {
	CustomerId   string `json:"customer_id" validate:"required,uuid"`
	CustomerName string `json:"customer_name,omitempty"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	Address      string `json:"address,omitempty"`
}

type DeleteCustomer struct {
	CustomerId string `json:"customer_id" validate:"required,uuid"`
}

type GetAllCustomer struct {
	Limit     int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Search    string  `query:"search"`
	Order     string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterID   *string `query:"after_id,omitempty"`
	AfterTime *string `query:"after_time,omitempty"`
}

type GetCustomer struct {
	CustomerId string `query:"customer_id" validate:"required,uuid"`
}
