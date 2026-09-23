package responsedto

import (
	"time"

	"github.com/google/uuid"
)

type ProductDtoResponse struct {
	ID               uuid.UUID `json:"id"`
	SKU              string    `json:"sku"`
	ProductName      string    `json:"product_name"`
	Unit             int       `json:"unit"`
	PurchasePrice    int64     `json:"purchase_price"`
	SellingPrice     int64     `json:"selling_price"`
	SellingPriceDebt int64     `json:"selling_price_debt"`
	Stock            float64   `json:"stock"`
	Category         string    `json:"category"`
	Image            string    `json:"image"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ProductBulkImportResponse struct {
	TotalInserted   int                  `json:"total_inserted"`
	TotalSkipped    int                  `json:"total_skipped"`
	SkippedSKUs     []string             `json:"skipped_skus"`
	DuplicateInFile []string             `json:"duplicate_in_file"`
	RowErrors       []BulkImportRowError `json:"row_errors"`
}

type BulkImportRowError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type GetAllProductResponse struct {
	UserId      string               `json:"user_id"`
	NextId      string               `json:"next_id"`
	NextTime    string               `json:"next_time"`
	HasNext     bool                 `json:"has_next"`
	ProductList []ProductDtoResponse `json:"product_list"`
}
