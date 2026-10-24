package requestdto

import (
	"mime/multipart"
)

type GetProduct struct {
	ID string `query:"id" validate:"required,uuid"`
}

type AddProduct struct {
	SKU              string   `json:"sku" validate:"required"`
	ProductName      string   `json:"product_name" validate:"required"`
	Unit             int      `json:"unit,omitempty" validate:"omitempty,oneof=0 1 2 3 4 5"`
	ProductType      int      `json:"product_type,omitempty" validate:"omitempty,oneof=0 1"`
	PurchasePrice    *int64   `json:"purchase_price" validate:"required,gt=0"`
	SellingPrice     *int64   `json:"selling_price" validate:"required,gt=0"`
	SellingPriceDebt *int64   `json:"selling_price_debt" validate:"required,gt=0"`
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
	PurchasePrice    *int64  `json:"purchase_price,omitempty" validate:"omitempty,gt=0"`
	SellingPrice     *int64  `json:"selling_price,omitempty" validate:"omitempty,gt=0"`
	SellingPriceDebt *int64  `json:"selling_price_debt,omitempty" validate:"omitempty,gt=0"`
	Category         *string `json:"category,omitempty"`
	Image            *string `json:"image,omitempty"`
	// AllowNewCategory diisi handler: hanya superadmin yang boleh membuat kategori baru.
	AllowNewCategory bool `json:"-"`
}

type UpdateStock struct {
	ID    string  `json:"id" validate:"required,uuid"`
	Stock float64 `json:"stock" validate:"required,ne=0"`
}

type GetAllProduct struct {
	UserId     string  `query:"user_id" validate:"required"`
	CategoryID string  `query:"category_id" validate:"omitempty,uuid"`
	Search     string  `query:"search"`
	Limit      int     `query:"limit" validate:"omitempty,min=1,max=100"`
	LastId     *string `query:"last_id"`
	AfterTime  *string `query:"after_time"`
	Order      string  `query:"order" validate:"omitempty,oneof=asc desc ASC DESC"`
}

type CategoryRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}
