package repository

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"shop_project_be/internal/domain"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func roundStock(v float64) float64 {
	return math.Round(v*100) / 100
}

var liveSKU = clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}}

type productRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) domain.ProductRepository {
	return &productRepository{db: db}
}

// UpsertBulkProduct: SKU baru di-insert; SKU hidup yang sudah ada ditimpa
// datanya, dan stoknya dijumlahkan di DB supaya penjualan yang berjalan
// bersamaan tidak tertimpa. Diurutkan per SKU agar dua import bersamaan
// mengunci baris dengan urutan yang sama.
func (p *productRepository) UpsertBulkProduct(ctx context.Context, products []*domain.Products) error {
	if len(products) == 0 {
		return nil
	}
	sort.Slice(products, func(i, j int) bool {
		return products[i].SKU < products[j].SKU
	})

	updates := append(clause.AssignmentColumns([]string{
		"product_name", "unit", "product_type", "purchase_price", "selling_price",
		"selling_price_debt", "category_id", "image", "updated_at",
	}), clause.Assignment{Column: clause.Column{Name: "stock"}, Value: gorm.Expr("products.stock + EXCLUDED.stock")})

	const batchSize = 100
	return runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "sku"}},
			TargetWhere: liveSKU,
			DoUpdates:   updates,
		}).CreateInBatches(products, batchSize).Error
		if err != nil {
			return internalErr(fmt.Errorf("failed during bulk upsert: %w", err))
		}
		return nil
	})
}

func (p *productRepository) GetProductsBySKUs(ctx context.Context, skus []string) (map[string]*domain.Products, error) {
	bySKU := make(map[string]*domain.Products, len(skus))
	if len(skus) == 0 {
		return bySKU, nil
	}
	var rows []*domain.Products
	if err := p.db.WithContext(ctx).Where("sku IN ?", skus).Find(&rows).Error; err != nil {
		return nil, internalErr(fmt.Errorf("failed to get products by sku: %w", err))
	}
	for _, row := range rows {
		bySKU[row.SKU] = row
	}
	return bySKU, nil
}

// GetCategoryIDs memetakan nama kategori ke id, dengan kunci domain.CategoryKey.
// create=true menambahkan kategori yang belum ada; ON CONFLICT pada key membuat
// dua request bersamaan dengan kategori sama tetap menghasilkan satu baris.
func (p *productRepository) GetCategoryIDs(ctx context.Context, names []string, create bool) (map[string]uuid.UUID, error) {
	nameByKey := make(map[string]string, len(names))
	for _, n := range names {
		if key := domain.CategoryKey(n); key != "" {
			if _, ok := nameByKey[key]; !ok {
				nameByKey[key] = strings.TrimSpace(n)
			}
		}
	}
	ids := make(map[string]uuid.UUID, len(nameByKey))
	if len(nameByKey) == 0 {
		return ids, nil
	}
	keys := slices.Sorted(maps.Keys(nameByKey))

	db := p.db.WithContext(ctx)
	if create {
		rows := make([]domain.Categories, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, domain.Categories{Key: key, Name: nameByKey[key]})
		}
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).
			Create(&rows).Error; err != nil {
			return nil, internalErr(fmt.Errorf("failed to create categories: %w", err))
		}
	}

	var found []domain.Categories
	if err := db.Where("key IN ?", keys).Find(&found).Error; err != nil {
		return nil, internalErr(fmt.Errorf("failed to get categories: %w", err))
	}
	for _, c := range found {
		ids[c.Key] = c.ID
	}
	return ids, nil
}

func (p *productRepository) ListCategories(ctx context.Context) ([]*domain.CategoryWithCount, error) {
	var rows []*domain.CategoryWithCount
	if err := p.db.WithContext(ctx).Table("categories").
		Select(`categories.*, (SELECT COUNT(*) FROM products
			WHERE products.category_id = categories.id AND products.deleted_at IS NULL) AS product_count`).
		Order("name ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	return rows, nil
}

func (p *productRepository) AddCategory(ctx context.Context, category *domain.Categories) error {
	if err := p.db.WithContext(ctx).Create(category).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.Duplicate(fmt.Sprintf("category %q already exists", category.Name))
		}
		return fmt.Errorf("failed to add category: %w", err)
	}
	return nil
}

func (p *productRepository) RenameCategory(ctx context.Context, id uuid.UUID, name string) error {
	result := p.db.WithContext(ctx).Model(&domain.Categories{}).Where("id = ?", id).
		Updates(map[string]any{"key": domain.CategoryKey(name), "name": strings.TrimSpace(name)})
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return domain.Duplicate(fmt.Sprintf("category %q already exists", strings.TrimSpace(name)))
		}
		return fmt.Errorf("failed to rename category: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound(fmt.Sprintf("category with id %s not found", id))
	}
	return nil
}

// DeleteCategory menolak kategori yang masih dipakai produk aktif. Produk yang
// sudah dihapus (soft delete) masih memegang FK, jadi kategorinya dilepas dulu.
func (p *productRepository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	return runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		var category domain.Categories
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("category with id %s not found", id))
			}
			return internalErr(fmt.Errorf("failed to lock category: %w", err))
		}
		var inUse int64
		if err := tx.Model(&domain.Products{}).Where("category_id = ?", id).Count(&inUse).Error; err != nil {
			return internalErr(fmt.Errorf("failed to count products: %w", err))
		}
		if inUse > 0 {
			return domain.Conflict(fmt.Sprintf("category %q is still used by %d product(s); move them first", category.Name, inUse))
		}
		if err := tx.Unscoped().Model(&domain.Products{}).Where("category_id = ?", id).
			Update("category_id", nil).Error; err != nil {
			return internalErr(fmt.Errorf("failed to detach deleted products: %w", err))
		}
		if err := tx.Delete(&domain.Categories{}, "id = ?", id).Error; err != nil {
			return internalErr(fmt.Errorf("failed to delete category: %w", err))
		}
		return nil
	})
}

func (p *productRepository) AddProduct(ctx context.Context, product *domain.Products) error {
	if err := p.db.WithContext(ctx).Create(product).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domain.Duplicate(fmt.Sprintf("product with sku %s already exists", product.SKU))
		}
		return internalErr(fmt.Errorf("failed to add product: %w", err))
	}
	return nil
}

func (p *productRepository) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	result := p.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Products{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete product: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NotFound(fmt.Sprintf("product with id %s not found", id))
	}
	return nil
}

func (p *productRepository) GetAllProduct(ctx context.Context, filter domain.FilterAllProduct) (*domain.PaginatedItem, error) {
	limit, order := pageParams(filter.Limit, filter.Order)
	query := p.db.WithContext(ctx).Model(&domain.Products{}).Preload("Category")

	if filter.CategoryID != nil {
		query = query.Where("category_id = ?", *filter.CategoryID)
	}

	if filter.Search != "" {
		query = query.Where("product_name ILIKE ? ESCAPE '\\'", likeContains(filter.Search))
	}

	var items []*domain.Products
	if err := keysetPage(query, "", limit, order, filter.Cursor).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to get products: %w", err)
	}
	items, hasNext, next := trimPage(items, limit, func(p *domain.Products) (time.Time, uuid.UUID) { return p.CreatedAt, p.ID })
	return &domain.PaginatedItem{
		DataItem: items,
		HasNext:  hasNext,
		Cursor:   next,
	}, nil
}

func (p *productRepository) GetProduct(ctx context.Context, id uuid.UUID) (*domain.Products, error) {
	var item domain.Products
	if err := p.db.WithContext(ctx).Preload("Category").Where("id = ?", id).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NotFound(fmt.Sprintf("product with id %s not found", id))
		}
		return nil, fmt.Errorf("failed to get product: %w", err)
	}
	return &item, nil
}

func (p *productRepository) GetProductsByIDs(ctx context.Context, ids []uuid.UUID, includeDeleted bool) (map[uuid.UUID]*domain.Products, error) {
	db := p.db.WithContext(ctx)
	if includeDeleted {
		db = db.Unscoped()
	}
	var rows []*domain.Products
	if len(ids) > 0 {
		if err := db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, internalErr(fmt.Errorf("failed to get products: %w", err))
		}
	}
	byID := make(map[uuid.UUID]*domain.Products, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	return byID, nil
}

func (p *productRepository) UpdateProductWithLock(ctx context.Context, id uuid.UUID, fields map[string]interface{}, stockDelta float64) error {
	return runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		var product domain.Products
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&product).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.NotFound(fmt.Sprintf("product with id %s not found", id))
			}
			return internalErr(fmt.Errorf("failed to find product for update: %w", err))
		}

		if stockDelta != 0 {
			newStock := roundStock(product.Stock + stockDelta)
			if newStock < 0 {
				return domain.Conflict(fmt.Sprintf("insufficient stock for product %s (current: %v, requested change: %v)", id, product.Stock, stockDelta))
			}
			fields["stock"] = newStock
		}

		if len(fields) == 0 {
			return nil
		}

		if err := tx.Model(&domain.Products{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return domain.Duplicate(fmt.Sprintf("product with sku %v already exists", fields["sku"]))
			}
			return internalErr(fmt.Errorf("failed to update product: %w", err))
		}
		return nil
	})
}

func internalErr(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrInternal, err)
}

type stockLine struct {
	productID uuid.UUID
	qty       float64
}

func paymentLines(items []domain.PaymentItem) []stockLine {
	lines := make([]stockLine, 0, len(items))
	for _, it := range items {
		lines = append(lines, stockLine{productID: it.ProductID, qty: it.Qty})
	}
	return lines
}

func decrementStock(tx *gorm.DB, lines []stockLine) error {
	_, err := decrementStockLines(tx, lines, false)
	return err
}

// decrementStockLines dengan allowShortage=true tidak menolak stok yang kurang:
// stok berhenti di 0 (constraint chk_products_stock_non_negative) dan short=true.
// Dipakai transaksi offline yang barangnya sudah diserahkan ke pembeli sebelum
// server tahu stoknya kurang; admin meninjau selisihnya dari detail transaksi.
func decrementStockLines(tx *gorm.DB, lines []stockLine, allowShortage bool) (short bool, err error) {
	locked, err := lockProductsOrdered(tx, lines)
	if err != nil {
		return false, err
	}
	for _, l := range lines {
		product, ok := locked[l.productID]
		if !ok {
			return false, domain.NotFound(fmt.Sprintf("product with id %s not found", l.productID))
		}
		if product.ProductType.IsDigital() {
			continue
		}
		if product.Stock < l.qty {
			if !allowShortage {
				return false, domain.Conflict(fmt.Sprintf("insufficient stock for product %s (current: %v, requested: %v)", l.productID, product.Stock, l.qty))
			}
			short = true
		}
		product.Stock = max(0, roundStock(product.Stock-l.qty))
		if err := tx.Model(&domain.Products{}).Where("id = ?", l.productID).Update("stock", product.Stock).Error; err != nil {
			return false, internalErr(fmt.Errorf("failed to update product stock: %w", err))
		}
	}
	return short, nil
}

func incrementStock(tx *gorm.DB, lines []stockLine) error {
	locked, err := lockProductsOrdered(tx.Unscoped(), lines)
	if err != nil {
		return err
	}
	for _, l := range lines {
		product, ok := locked[l.productID]
		if !ok || product.ProductType.IsDigital() {
			continue
		}
		product.Stock = roundStock(product.Stock + l.qty)
		if err := tx.Unscoped().Model(&domain.Products{}).Where("id = ?", l.productID).Update("stock", product.Stock).Error; err != nil {
			return internalErr(fmt.Errorf("failed to restore product stock: %w", err))
		}
	}
	return nil
}

func lockProductsOrdered(tx *gorm.DB, lines []stockLine) (map[uuid.UUID]*domain.Products, error) {
	ids := make([]uuid.UUID, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.productID)
	}
	var rows []*domain.Products
	if len(ids) > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
			return nil, internalErr(fmt.Errorf("failed to lock products: %w", err))
		}
	}
	locked := make(map[uuid.UUID]*domain.Products, len(rows))
	for _, row := range rows {
		locked[row.ID] = row
	}
	return locked, nil
}
