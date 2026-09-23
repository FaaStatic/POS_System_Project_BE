package requestdto

import (
	"mime/multipart"
)

type SearchProduct struct {
	ID          string `query:"id,omitempty"`
	Sku         string `query:"sku,omitempty"`
	ProductName string `query:"product_name,omitempty"`
}

type GetProduct struct {
	ID string `query:"id" validate:"required,uuid"`
}

type AddProduct struct {
	SKU              string   `json:"sku" validate:"required"`
	ProductName      string   `json:"product_name" validate:"required"`
	Unit             int      `json:"unit,omitempty" validate:"omitempty,oneof=0 1 2 3 4 5"`
	ProductType      int      `json:"product_type,omitempty" validate:"omitempty,oneof=0 1"`
	PurchasePrice    *int64   `json:"purchase_price" validate:"required,gte=0"`
	SellingPrice     *int64   `json:"selling_price" validate:"required,gte=0"`
	SellingPriceDebt *int64   `json:"selling_price_debt" validate:"required,gte=0"`
	Stock            *float64 `json:"stock" validate:"required,gte=0"`
	Category         string   `json:"category" validate:"required"`
	Image            string   `json:"image,omitempty"`
}

type AddBulkProduct struct {
	FileUpload *multipart.FileHeader `form:"file_upload"`
}

type DeleteProduct struct {
	ID string `json:"id" validate:"required,uuid"`
}

type UpdateProduct struct {
	ID               string  `json:"id" validate:"required,uuid"`
	SKU              *string `json:"sku,omitempty"`
	ProductName      *string `json:"product_name,omitempty"`
	Unit             *int    `json:"unit,omitempty" validate:"omitempty,oneof=0 1 2 3 4 5"`
	ProductType      *int    `json:"product_type,omitempty" validate:"omitempty,oneof=0 1"`
	PurchasePrice    *int64  `json:"purchase_price,omitempty" validate:"omitempty,gte=0"`
	SellingPrice     *int64  `json:"selling_price,omitempty" validate:"omitempty,gte=0"`
	SellingPriceDebt *int64  `json:"selling_price_debt,omitempty" validate:"omitempty,gte=0"`
	Category         *string `json:"category,omitempty"`
	Image            *string `json:"image,omitempty"`
}

type UpdateStock struct {
	ID    string  `json:"id" validate:"required,uuid"`
	Stock float64 `json:"stock" validate:"required,ne=0"`
}

type GetAllProduct struct {
	UserId    string  `query:"user_id" validate:"required"`
	Category  string  `query:"category"`
	Search    string  `query:"search"`
	Limit     int     `query:"limit" validate:"omitempty,min=1,max=100"`
	LastId    *string `query:"last_id"`
	AfterTime *string `query:"after_time"`
	Order     string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
}
