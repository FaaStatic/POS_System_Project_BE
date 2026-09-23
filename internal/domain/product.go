package domain

import (
	"context"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/constant/paginated"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Products struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SKU              string           `gorm:"type:varchar(50);uniqueIndex" json:"sku"`
	ProductName      string           `gorm:"type:varchar(255);not null" json:"product_name"`
	Unit             enum.ProductUnit `gorm:"type:smallint;check:unit IN (0,1,2,3,4,5);not null" json:"unit"`
	ProductType      enum.ProductType `gorm:"column:product_type;type:smallint;check:product_type IN (0,1);not null;default:0" json:"product_type"`
	PurchasePrice    int64            `gorm:"type:bigint;not null" json:"purchase_price"`
	SellingPrice     int64            `gorm:"type:bigint;not null" json:"selling_price"`
	SellingPriceDebt int64            `gorm:"type:bigint;not null" json:"selling_price_debt"`
	Stock            float64          `gorm:"type:decimal(10,2);default:0" json:"stock"`
	Category         string           `gorm:"type:varchar(100);index" json:"category"`
	Image            string           `gorm:"type:text" json:"image"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (p *Products) TableName() string {
	return "products"
}

type BulkInsertResult struct {
	TotalInserted int
	TotalSkipped  int
	SkippedSKUs   []string
}

type FilterAllProduct struct {
	Search   string
	Cursor   *paginated.CursorMeta
	Limit    int
	Category string
	Order    string
}

type PaginatedItem struct {
	DataItem []*Products
	HasNext  bool
	Cursor   *paginated.CursorMeta
}

type ProductRepository interface {
	AddProduct(ctx context.Context, product *Products) error
	AddBulkProduct(ctx context.Context, products []*Products) (*BulkInsertResult, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) error
	GetProduct(ctx context.Context, id uuid.UUID) (*Products, error)
	GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*Products, error)
	GetAllProduct(ctx context.Context, filter FilterAllProduct) (*PaginatedItem, error)
	UpdateProductWithLock(ctx context.Context, id uuid.UUID, fields map[string]interface{}, stockDelta float64) error
}

type ProductUsecase interface {
	AddProductShopWithLock(ctx context.Context, request *requestdto.AddProduct) error
	AddBulkProductShopWithLock(ctx context.Context, request *requestdto.AddBulkProduct) (*responsedto.ProductBulkImportResponse, error)
	DeleteProductShop(ctx context.Context, request *requestdto.DeleteProduct) error
	GetProductShop(ctx context.Context, request *requestdto.GetProduct) (*Products, error)
	GetAllProductShop(ctx context.Context, request *requestdto.GetAllProduct) (*responsedto.GetAllProductResponse, error)
	UpdateProductShopWithLock(ctx context.Context, request *requestdto.UpdateProduct, delta float64) error
	UpdateStockWithLock(ctx context.Context, request *requestdto.UpdateStock, delta float64) error
}
