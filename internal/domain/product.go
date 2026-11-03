package domain

import (
	"context"
	"math"
	"regexp"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/constant/paginated"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Products struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SKU              string           `gorm:"type:varchar(50)" json:"sku"`
	Barcode          *string          `gorm:"type:varchar(64)" json:"barcode"`
	ProductName      string           `gorm:"type:varchar(255)" json:"product_name"`
	Unit             enum.ProductUnit `gorm:"type:smallint" json:"unit"`
	ProductType      enum.ProductType `gorm:"column:product_type;type:smallint;default:0" json:"product_type"`
	PurchasePrice    int64            `gorm:"type:bigint" json:"purchase_price"`
	SellingPrice     int64            `gorm:"type:bigint" json:"selling_price"`
	SellingPriceDebt int64            `gorm:"type:bigint" json:"selling_price_debt"`
	Stock            float64          `gorm:"type:decimal(12,3);default:0" json:"stock"`
	CategoryID       *uuid.UUID       `gorm:"type:uuid" json:"category_id"`
	Image            string           `gorm:"type:text" json:"image"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-"`

	Category *Categories `gorm:"foreignKey:CategoryID" json:"-"`
}

func (p *Products) TableName() string {
	return "products"
}

var barcodeUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// AutoBarcode: "<SKU>-<8 hex pertama id>", dibatasi 64 karakter dan hanya berisi
// karakter yang lolos chk_products_barcode_format. Migrasi 00010 membuat nilai
// yang sama di SQL untuk produk lama; ubah keduanya bersamaan.
func AutoBarcode(sku string, id uuid.UUID) string {
	safe := barcodeUnsafe.ReplaceAllString(sku, "-")
	if len(safe) > 55 {
		safe = safe[:55]
	}
	return safe + "-" + id.String()[:8]
}

// CheckQty memvalidasi qty jual maupun stok: maksimal 3 desimal (numeric(12,3))
// dan bulat untuk pcs.
func CheckQty(unit enum.ProductUnit, qty float64) error {
	if unit.IsWhole() && qty != math.Trunc(qty) {
		return Validation("quantity of a pcs product must be a whole number")
	}
	if scaled := qty * 1000; math.Abs(scaled-math.Round(scaled)) > 1e-6 {
		return Validation("quantity supports at most 3 decimal places")
	}
	return nil
}

type Categories struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Key       string    `gorm:"type:varchar(100)" json:"key"`
	Name      string    `gorm:"type:varchar(100)" json:"name"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (c *Categories) TableName() string {
	return "categories"
}

// CategoryWithCount: ProductCount hanya menghitung produk aktif; kategori dengan
// ProductCount > 0 tidak bisa dihapus.
type CategoryWithCount struct {
	Categories   `gorm:"embedded"`
	ProductCount int64 `gorm:"column:product_count" json:"product_count"`
}

// CategoryKey adalah kunci duplikat kategori: "Sembako " dan "SEMBAKO" sama.
func CategoryKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

type FilterAllProduct struct {
	Search     string
	Cursor     *paginated.CursorMeta
	Limit      int
	CategoryID *uuid.UUID
	Order      string
	// Barcode: exact match untuk hasil scan kasir.
	Barcode string
}

type PaginatedItem struct {
	DataItem []*Products
	HasNext  bool
	Cursor   *paginated.CursorMeta
}

type ProductRepository interface {
	AddProduct(ctx context.Context, product *Products) error
	UpsertBulkProduct(ctx context.Context, products []*Products) error
	DeleteProduct(ctx context.Context, id uuid.UUID) error
	GetProduct(ctx context.Context, id uuid.UUID) (*Products, error)
	GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*Products, error)
	GetProductsBySKUs(ctx context.Context, skus []string) (map[string]*Products, error)
	GetProductsByBarcodes(ctx context.Context, barcodes []string) (map[string]*Products, error)
	GetAllProduct(ctx context.Context, filter FilterAllProduct) (*PaginatedItem, error)
	// ListForExport mengembalikan produk ber-barcode yang cocok dengan filter, urut
	// stabil (created_at, id), beserta total yang cocok untuk menghitung jumlah part.
	ListForExport(ctx context.Context, filter FilterAllProduct, limit, offset int) ([]*Products, int64, error)
	UpdateProductWithLock(ctx context.Context, id uuid.UUID, fields map[string]interface{}, stockDelta float64) error
	GetCategoryIDs(ctx context.Context, names []string, create bool) (map[string]uuid.UUID, error)
	ListCategories(ctx context.Context) ([]*CategoryWithCount, error)
	AddCategory(ctx context.Context, category *Categories) error
	RenameCategory(ctx context.Context, id uuid.UUID, name string) error
	DeleteCategory(ctx context.Context, id uuid.UUID) error
}

type ProductUsecase interface {
	AddProductShopWithLock(ctx context.Context, request *requestdto.AddProduct) error
	AddBulkProductShopWithLock(ctx context.Context, request *requestdto.AddBulkProduct) (*responsedto.ProductBulkImportResponse, error)
	DeleteProductShop(ctx context.Context, request *requestdto.DeleteProduct) error
	GetProductShop(ctx context.Context, request *requestdto.GetProduct) (*responsedto.ProductDtoResponse, error)
	GetAllProductShop(ctx context.Context, request *requestdto.GetAllProduct) (*responsedto.GetAllProductResponse, error)
	ExportBarcodes(ctx context.Context, request *requestdto.ExportBarcodes) (*responsedto.BarcodeExport, error)
	UpdateProductShopWithLock(ctx context.Context, request *requestdto.UpdateProduct) error
	UpdateStockWithLock(ctx context.Context, request *requestdto.UpdateStock) error
	ListCategories(ctx context.Context) ([]*CategoryWithCount, error)
	AddCategory(ctx context.Context, request *requestdto.CategoryRequest) (*Categories, error)
	RenameCategory(ctx context.Context, id string, request *requestdto.CategoryRequest) error
	DeleteCategory(ctx context.Context, id string) error
}
