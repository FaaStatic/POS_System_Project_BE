package requestdto

type AddCustomer struct {
	CustomerName string `json:"customer_name" validate:"required"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	Address      string `json:"address,omitempty"`
}
type UpdateCustomer struct {
	CustomerId string `json:"customer_id" validate:"required,uuid"`
	// Pointer: nil = tidak diubah, "" = dikosongkan.
	CustomerName *string `json:"customer_name,omitempty" validate:"omitempty,min=1"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
	Address      *string `json:"address,omitempty"`
}

type DeleteCustomer struct {
	CustomerId string `json:"customer_id" validate:"required,uuid"`
}

type GetAllCustomer struct {
	Status    string  `query:"status" validate:"omitempty,oneof=active inactive"`
	Limit     int     `query:"limit" validate:"omitempty,min=1,max=100"`
	Search    string  `query:"search"`
	Order     string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
	AfterID   *string `query:"after_id,omitempty"`
	AfterTime *string `query:"after_time,omitempty"`
}

type GetCustomer struct {
	CustomerId string `query:"customer_id" validate:"required,uuid"`
	// IncludeInactive diisi handler: hanya superadmin yang boleh membuka customer nonaktif.
	IncludeInactive bool `query:"-"`
}
