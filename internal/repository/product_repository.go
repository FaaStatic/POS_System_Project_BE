package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"shop_project_be/internal/domain"
	"sort"
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

func (p *productRepository) AddBulkProduct(ctx context.Context, products []*domain.Products) (*domain.BulkInsertResult, error) {
	if len(products) == 0 {
		return &domain.BulkInsertResult{}, nil
	}
	skus := make([]string, 0, len(products))
	for _, p := range products {
		skus = append(skus, p.SKU)
	}
	var existingSKUs []string
	if err := p.db.WithContext(ctx).
		Model(&domain.Products{}).
		Where("sku IN ?", skus).
		Pluck("sku", &existingSKUs).Error; err != nil {
		return nil, internalErr(fmt.Errorf("failed to check existing SKUs: %w", err))
	}

	existingMap := make(map[string]struct{}, len(existingSKUs))
	for _, sku := range existingSKUs {
		existingMap[sku] = struct{}{}
	}
	newProducts := make([]*domain.Products, 0, len(products))
	skippedSKUs := make([]string, 0)
	for _, product := range products {
		if _, isDuplicate := existingMap[product.SKU]; isDuplicate {
			skippedSKUs = append(skippedSKUs, product.SKU)
		} else {
			newProducts = append(newProducts, product)
		}
	}
	if len(newProducts) == 0 {
		return &domain.BulkInsertResult{
			TotalInserted: 0,
			TotalSkipped:  len(skippedSKUs),
			SkippedSKUs:   skippedSKUs,
		}, nil
	}

	sort.Slice(newProducts, func(i, j int) bool {
		return newProducts[i].SKU < newProducts[j].SKU
	})

	const batchSize = 100
	var inserted int64
	err := runTxDB(ctx, p.db, func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "sku"}},
			TargetWhere: liveSKU,
			DoNothing:   true,
		}).CreateInBatches(newProducts, batchSize)
		if result.Error != nil {
			return internalErr(fmt.Errorf("failed during batch insert: %w", result.Error))
		}
		inserted = result.RowsAffected
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &domain.BulkInsertResult{
		TotalInserted: int(inserted),
		TotalSkipped:  len(skippedSKUs) + len(newProducts) - int(inserted),
		SkippedSKUs:   skippedSKUs,
	}, nil
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
	query := p.db.WithContext(ctx).Model(&domain.Products{})

	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
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
	if err := p.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
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
	locked, err := lockProductsOrdered(tx, lines)
	if err != nil {
		return err
	}
	for _, l := range lines {
		product, ok := locked[l.productID]
		if !ok {
			return domain.NotFound(fmt.Sprintf("product with id %s not found", l.productID))
		}
		if product.ProductType.IsDigital() {
			continue
		}
		if product.Stock < l.qty {
			return domain.Conflict(fmt.Sprintf("insufficient stock for product %s (current: %v, requested: %v)", l.productID, product.Stock, l.qty))
		}
		product.Stock = roundStock(product.Stock - l.qty)
		if err := tx.Model(&domain.Products{}).Where("id = ?", l.productID).Update("stock", product.Stock).Error; err != nil {
			return internalErr(fmt.Errorf("failed to update product stock: %w", err))
		}
	}
	return nil
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
