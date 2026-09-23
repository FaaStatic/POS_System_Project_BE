package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"shop_project_be/pkg/sheet"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type productUsecase struct {
	productRepo domain.ProductRepository
	log         *zap.Logger
}

func NewProductUsecase(productRepo domain.ProductRepository, log *zap.Logger) domain.ProductUsecase {
	return &productUsecase{
		productRepo: productRepo,
		log:         log,
	}
}

func (p *productUsecase) GetProductShop(ctx context.Context, request *requestdto.GetProduct) (*domain.Products, error) {
	productUid, errUid := uuid.Parse(request.ID)
	if errUid != nil {
		p.log.Error("failed to parse product id", zap.Error(errUid))
		return nil, domain.InvalidID("invalid product id format")
	}
	products, err := p.productRepo.GetProduct(ctx, productUid)
	if err != nil {
		p.log.Error("failed to get product", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get product: %w", domain.ErrInternal)
	}
	return products, nil
}

func (p *productUsecase) AddBulkProductShopWithLock(ctx context.Context, request *requestdto.AddBulkProduct) (*responsedto.ProductBulkImportResponse, error) {
	if request.FileUpload == nil {
		return nil, domain.Validation("file upload is required")
	}

	file, err := request.FileUpload.Open()
	if err != nil {
		p.log.Error("failed to open uploaded file", zap.Error(err))
		return nil, fmt.Errorf("failed to open uploaded file: %w", domain.ErrInternal)
	}
	defer file.Close()

	rows, rowErrors, err := sheet.ParseProducts(file, request.FileUpload.Filename)
	if err != nil {
		p.log.Warn("failed to parse uploaded file", zap.Error(err))
		return nil, domain.Validation(fmt.Sprintf("failed to parse file: %v", err))
	}

	report := &responsedto.ProductBulkImportResponse{
		SkippedSKUs:     []string{},
		DuplicateInFile: []string{},
	}
	seen := make(map[string]struct{}, len(rows))
	products := make([]*domain.Products, 0, len(rows))
	for _, row := range rows {
		if _, exists := seen[row.SKU]; exists {
			report.DuplicateInFile = append(report.DuplicateInFile, row.SKU)
			continue
		}
		seen[row.SKU] = struct{}{}

		unit, err := enum.ParseProductUnit(row.Unit)
		if err != nil {
			rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: "invalid unit"})
			continue
		}
		productType, err := enum.ParseProductType(row.ProductType)
		if err != nil {
			rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: "invalid product_type"})
			continue
		}
		products = append(products, &domain.Products{
			SKU:              row.SKU,
			ProductName:      row.ProductName,
			Unit:             unit,
			ProductType:      productType,
			PurchasePrice:    int64(math.Round(row.PurchasePrice)),
			SellingPrice:     int64(math.Round(row.SellingPrice)),
			SellingPriceDebt: int64(math.Round(row.SellingPriceDebt)),
			Stock:            row.Stock,
			Category:         row.Category,
			Image:            row.Image,
		})
	}
	report.RowErrors = make([]responsedto.BulkImportRowError, 0, len(rowErrors))
	for _, re := range rowErrors {
		report.RowErrors = append(report.RowErrors, responsedto.BulkImportRowError{Line: re.Line, Message: re.Message})
	}

	if len(products) == 0 {
		return report, nil
	}

	result, err := p.productRepo.AddBulkProduct(ctx, products)
	if err != nil {
		p.log.Error("failed to bulk insert products", zap.Error(err))
		return nil, fmt.Errorf("failed to import products: %w", domain.ErrInternal)
	}
	report.TotalInserted = result.TotalInserted
	report.TotalSkipped = result.TotalSkipped
	report.SkippedSKUs = result.SkippedSKUs

	p.log.Info("bulk product import finished",
		zap.Int("inserted", result.TotalInserted),
		zap.Int("skipped_existing", result.TotalSkipped),
		zap.Int("duplicate_in_file", len(report.DuplicateInFile)),
		zap.Int("row_errors", len(report.RowErrors)),
	)
	return report, nil
}

func (p *productUsecase) AddProductShopWithLock(ctx context.Context, request *requestdto.AddProduct) error {
	err := p.productRepo.AddProduct(ctx, &domain.Products{
		SKU:              request.SKU,
		ProductName:      request.ProductName,
		Unit:             enum.ProductUnit(request.Unit),
		ProductType:      enum.ProductType(request.ProductType),
		PurchasePrice:    *request.PurchasePrice,
		SellingPrice:     *request.SellingPrice,
		SellingPriceDebt: *request.SellingPriceDebt,
		Stock:            *request.Stock,
		Category:         request.Category,
		Image:            request.Image,
	})
	if err != nil {
		p.log.Error("failed to add product", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to add product: %w", domain.ErrInternal)
		}
		return err
	}
	return nil
}

func (p *productUsecase) DeleteProductShop(ctx context.Context, request *requestdto.DeleteProduct) error {
	id, errId := uuid.Parse(request.ID)
	if errId != nil {
		p.log.Error("failed to parse product id", zap.Error(errId))
		return domain.InvalidID("invalid product id format")
	}
	err := p.productRepo.DeleteProduct(ctx, id)
	if err != nil {
		p.log.Error("failed to delete product", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("failed to delete product: %w", domain.ErrInternal)
	}
	return nil
}

func (p *productUsecase) GetAllProductShop(ctx context.Context, request *requestdto.GetAllProduct) (*responsedto.GetAllProductResponse, error) {
	cursor, err := parseCursor(request.LastId, request.AfterTime)
	if err != nil {
		return nil, err
	}

	filter := domain.FilterAllProduct{
		Search:   request.Search,
		Category: request.Category,
		Cursor:   cursor,
		Limit:    request.Limit,
		Order:    request.Order,
	}

	result, err := p.productRepo.GetAllProduct(ctx, filter)
	if err != nil {
		p.log.Error("failed to get all products", zap.Error(err))
		return nil, fmt.Errorf("failed to get all products: %w", domain.ErrInternal)
	}

	products := make([]responsedto.ProductDtoResponse, 0, len(result.DataItem))
	for _, item := range result.DataItem {
		products = append(products, responsedto.ProductDtoResponse{
			ID:               item.ID,
			SKU:              item.SKU,
			ProductName:      item.ProductName,
			Unit:             int(item.Unit),
			PurchasePrice:    item.PurchasePrice,
			SellingPrice:     item.SellingPrice,
			SellingPriceDebt: item.SellingPriceDebt,
			Stock:            item.Stock,
			Category:         item.Category,
			Image:            item.Image,
			UpdatedAt:        item.UpdatedAt,
		})
	}

	nextId, nextTime := result.Cursor.Encode()

	responses := responsedto.GetAllProductResponse{
		UserId:      request.UserId,
		NextId:      nextId,
		NextTime:    nextTime,
		HasNext:     result.HasNext,
		ProductList: products,
	}
	return &responses, nil
}

func (p *productUsecase) UpdateProductShopWithLock(ctx context.Context, request *requestdto.UpdateProduct, delta float64) error {
	id, err := uuid.Parse(request.ID)
	if err != nil {
		p.log.Error("failed to parse product id", zap.Error(err))
		return domain.InvalidID("invalid product id format")
	}

	fields := make(map[string]interface{})
	if request.SKU != nil {
		fields["sku"] = *request.SKU
	}
	if request.ProductName != nil {
		fields["product_name"] = *request.ProductName
	}
	if request.Unit != nil {
		fields["unit"] = enum.ProductUnit(*request.Unit)
	}
	if request.ProductType != nil {
		fields["product_type"] = enum.ProductType(*request.ProductType)
	}
	if request.PurchasePrice != nil {
		fields["purchase_price"] = *request.PurchasePrice
	}
	if request.SellingPrice != nil {
		fields["selling_price"] = *request.SellingPrice
	}
	if request.SellingPriceDebt != nil {
		fields["selling_price_debt"] = *request.SellingPriceDebt
	}
	if request.Category != nil {
		fields["category"] = *request.Category
	}
	if request.Image != nil {
		fields["image"] = *request.Image
	}

	if len(fields) == 0 && delta == 0 {
		return domain.Validation("no fields to update")
	}

	if err := p.productRepo.UpdateProductWithLock(ctx, id, fields, delta); err != nil {
		p.log.Error("failed to update product", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to update product: %w", domain.ErrInternal)
		}
		return fmt.Errorf("failed to update product: %w", err)
	}
	return nil
}

func (p *productUsecase) UpdateStockWithLock(ctx context.Context, request *requestdto.UpdateStock, delta float64) error {
	id, err := uuid.Parse(request.ID)
	if err != nil {
		p.log.Error("failed to parse product id", zap.Error(err))
		return domain.InvalidID("invalid product id format")
	}

	if err := p.productRepo.UpdateProductWithLock(ctx, id, map[string]interface{}{}, delta); err != nil {
		p.log.Error("failed to update stock", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to update stock: %w", domain.ErrInternal)
		}
		return fmt.Errorf("failed to update stock: %w", err)
	}
	return nil
}
