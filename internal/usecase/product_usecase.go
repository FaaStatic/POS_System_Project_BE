package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"shop_project_be/pkg/sheet"
	"strings"

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

// validatePrices: harga hutang minimal harga jual +5%, dihitung integer supaya
// setara pembulatan ke atas tanpa float.
func validatePrices(purchase, selling, sellingDebt int64) error {
	switch {
	case purchase <= 0 || selling <= 0 || sellingDebt <= 0:
		return domain.Validation("purchase_price, selling_price and selling_price_debt must be greater than 0")
	case selling <= purchase:
		return domain.Validation("selling_price must be greater than purchase_price")
	case sellingDebt*100 < selling*105:
		return domain.Validation("selling_price_debt must be at least 5% above selling_price")
	}
	return nil
}

var barcodePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// normalizeBarcode: string kosong berarti produk tanpa barcode (NULL). Tanpa cek
// checksum EAN/UPC karena toko juga memakai barcode buatan sendiri. Format sama
// dengan chk_products_barcode_format (migrasi 00008).
func normalizeBarcode(raw string) (*string, error) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return nil, nil
	}
	if !barcodePattern.MatchString(code) {
		return nil, domain.Validation("barcode must be 1-64 characters of letters, digits, '.', '_' or '-'")
	}
	return &code, nil
}

func toProductResponse(item *domain.Products) responsedto.ProductDtoResponse {
	var category string
	if item.Category != nil {
		category = item.Category.Name
	}
	return responsedto.ProductDtoResponse{
		ID:               item.ID,
		SKU:              item.SKU,
		Barcode:          item.Barcode,
		ProductName:      item.ProductName,
		Unit:             int(item.Unit),
		ProductType:      int(item.ProductType),
		PurchasePrice:    item.PurchasePrice,
		SellingPrice:     item.SellingPrice,
		SellingPriceDebt: item.SellingPriceDebt,
		Stock:            item.Stock,
		CategoryID:       item.CategoryID,
		Category:         category,
		Image:            item.Image,
		UpdatedAt:        item.UpdatedAt,
	}
}

// categoryID memetakan nama kategori ke id; nama kosong berarti tanpa kategori.
// create=false (staff) menolak kategori yang belum ada.
func (p *productUsecase) categoryID(ctx context.Context, name string, create bool) (*uuid.UUID, error) {
	key := domain.CategoryKey(name)
	if key == "" {
		return nil, nil
	}
	ids, err := p.productRepo.GetCategoryIDs(ctx, []string{name}, create)
	if err != nil {
		p.log.Error("failed to resolve category", zap.Error(err))
		return nil, fmt.Errorf("failed to resolve category: %w", domain.ErrInternal)
	}
	id, ok := ids[key]
	if !ok {
		return nil, domain.Validation(fmt.Sprintf("category %q does not exist yet; ask a superadmin to add it", strings.TrimSpace(name)))
	}
	return &id, nil
}

func (p *productUsecase) ListCategories(ctx context.Context) ([]*domain.CategoryWithCount, error) {
	categories, err := p.productRepo.ListCategories(ctx)
	if err != nil {
		p.log.Error("failed to list categories", zap.Error(err))
		return nil, fmt.Errorf("failed to list categories: %w", domain.ErrInternal)
	}
	return categories, nil
}

func (p *productUsecase) GetProductShop(ctx context.Context, request *requestdto.GetProduct) (*responsedto.ProductDtoResponse, error) {
	productUid, errUid := uuid.Parse(request.ID)
	if errUid != nil {
		p.log.Error("failed to parse product id", zap.Error(errUid))
		return nil, domain.InvalidID("invalid product id format")
	}
	products, err := p.productRepo.GetProduct(ctx, productUid)
	if err != nil {
		return nil, passOrInternal(p.log, err, "failed to get product", domain.ErrNotFound)
	}
	response := toProductResponse(products)
	return &response, nil
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
		UpdatedSKUs:     []string{},
		DuplicateInFile: []string{},
	}
	seen := make(map[string]struct{}, len(rows))
	unique := make([]sheet.ProductRow, 0, len(rows))
	for _, row := range rows {
		if _, exists := seen[row.SKU]; exists {
			report.DuplicateInFile = append(report.DuplicateInFile, row.SKU)
			continue
		}
		seen[row.SKU] = struct{}{}
		unique = append(unique, row)
	}

	skus := make([]string, 0, len(unique))
	for _, row := range unique {
		skus = append(skus, row.SKU)
	}
	existing, err := p.productRepo.GetProductsBySKUs(ctx, skus)
	if err != nil {
		p.log.Error("failed to get existing products", zap.Error(err))
		return nil, fmt.Errorf("failed to import products: %w", domain.ErrInternal)
	}

	// Barcode yang sudah dipakai SKU lain ditolak per baris; tanpa cek ini satu
	// baris bentrok membuat seluruh upsert gagal di idx_products_barcode.
	var barcodes []string
	for _, row := range unique {
		if code, err := normalizeBarcode(row.Barcode); err == nil && code != nil {
			barcodes = append(barcodes, *code)
		}
	}
	barcodeOwners, err := p.productRepo.GetProductsByBarcodes(ctx, barcodes)
	if err != nil {
		p.log.Error("failed to get products by barcode", zap.Error(err))
		return nil, fmt.Errorf("failed to import products: %w", domain.ErrInternal)
	}
	barcodeRows := make(map[string]int, len(barcodes))

	products := make([]*domain.Products, 0, len(unique))
	categories := make([]string, 0, len(unique))
	for _, row := range unique {
		current := existing[row.SKU]

		// Sel unit/product_type kosong pada SKU lama mempertahankan nilai lama;
		// kolom lain selalu ditimpa isi file.
		unit, productType := enum.ProductUnit(0), enum.ProductType(0)
		if current != nil {
			unit, productType = current.Unit, current.ProductType
		}
		if row.Unit != "" || current == nil {
			if unit, err = enum.ParseProductUnit(row.Unit); err != nil {
				rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: "invalid unit"})
				continue
			}
		}
		if row.ProductType != "" || current == nil {
			if productType, err = enum.ParseProductType(row.ProductType); err != nil {
				rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: "invalid product_type"})
				continue
			}
		}
		if current != nil && productType != current.ProductType {
			rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: "product_type of an existing product cannot change between digital and physical"})
			continue
		}

		barcode, err := normalizeBarcode(row.Barcode)
		if err != nil {
			rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: err.Error()})
			continue
		}
		barcodeFromFile := barcode != nil
		if barcode == nil && current != nil {
			// Sel kosong mempertahankan barcode lama, supaya file tanpa kolom
			// barcode tidak menghapus barcode yang sudah terdaftar.
			barcode = current.Barcode
		} else if barcodeFromFile {
			if owner := barcodeOwners[*barcode]; owner != nil && owner.SKU != row.SKU {
				rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: fmt.Sprintf("barcode %s is already used by sku %s", *barcode, owner.SKU)})
				continue
			}
			if line, dup := barcodeRows[*barcode]; dup {
				rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: fmt.Sprintf("barcode %s is also used on line %d", *barcode, line)})
				continue
			}
		}

		purchase := int64(math.Round(row.PurchasePrice))
		selling := int64(math.Round(row.SellingPrice))
		sellingDebt := int64(math.Round(row.SellingPriceDebt))
		if err := validatePrices(purchase, selling, sellingDebt); err != nil {
			rowErrors = append(rowErrors, sheet.RowError{Line: row.Line, Message: err.Error()})
			continue
		}
		// Dicatat setelah semua validasi: baris yang ditolak tidak boleh
		// mengunci barcode untuk baris berikutnya.
		if barcodeFromFile {
			barcodeRows[*barcode] = row.Line
		}
		products = append(products, &domain.Products{
			SKU:              row.SKU,
			Barcode:          barcode,
			ProductName:      row.ProductName,
			Unit:             unit,
			ProductType:      productType,
			PurchasePrice:    purchase,
			SellingPrice:     selling,
			SellingPriceDebt: sellingDebt,
			Stock:            row.Stock,
			Image:            row.Image,
		})
		categories = append(categories, row.Category)
		if current != nil {
			report.UpdatedSKUs = append(report.UpdatedSKUs, row.SKU)
		}
	}
	report.RowErrors = make([]responsedto.BulkImportRowError, 0, len(rowErrors))
	for _, re := range rowErrors {
		report.RowErrors = append(report.RowErrors, responsedto.BulkImportRowError{Line: re.Line, Message: re.Message})
	}

	if len(products) == 0 {
		return report, nil
	}

	categoryIDs, err := p.productRepo.GetCategoryIDs(ctx, categories, true)
	if err != nil {
		p.log.Error("failed to resolve categories", zap.Error(err))
		return nil, fmt.Errorf("failed to import products: %w", domain.ErrInternal)
	}
	for i, product := range products {
		if id, ok := categoryIDs[domain.CategoryKey(categories[i])]; ok {
			product.CategoryID = &id
		}
	}

	if err := p.productRepo.UpsertBulkProduct(ctx, products); err != nil {
		p.log.Error("failed to bulk upsert products", zap.Error(err))
		return nil, fmt.Errorf("failed to import products: %w", domain.ErrInternal)
	}
	report.TotalUpdated = len(report.UpdatedSKUs)
	report.TotalInserted = len(products) - report.TotalUpdated

	p.log.Info("bulk product import finished",
		zap.Int("inserted", report.TotalInserted),
		zap.Int("updated", report.TotalUpdated),
		zap.Int("duplicate_in_file", len(report.DuplicateInFile)),
		zap.Int("row_errors", len(report.RowErrors)),
	)
	return report, nil
}

func (p *productUsecase) AddProductShopWithLock(ctx context.Context, request *requestdto.AddProduct) error {
	if err := validatePrices(*request.PurchasePrice, *request.SellingPrice, *request.SellingPriceDebt); err != nil {
		return err
	}
	barcode, err := normalizeBarcode(request.Barcode)
	if err != nil {
		return err
	}
	categoryID, err := p.categoryID(ctx, request.Category, true)
	if err != nil {
		return err
	}
	err = p.productRepo.AddProduct(ctx, &domain.Products{
		SKU:              request.SKU,
		Barcode:          barcode,
		ProductName:      request.ProductName,
		Unit:             enum.ProductUnit(request.Unit),
		ProductType:      enum.ProductType(request.ProductType),
		PurchasePrice:    *request.PurchasePrice,
		SellingPrice:     *request.SellingPrice,
		SellingPriceDebt: *request.SellingPriceDebt,
		Stock:            *request.Stock,
		CategoryID:       categoryID,
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
		return passOrInternal(p.log, err, "failed to delete product", domain.ErrNotFound)
	}
	return nil
}

func (p *productUsecase) GetAllProductShop(ctx context.Context, request *requestdto.GetAllProduct) (*responsedto.GetAllProductResponse, error) {
	cursor, err := parseCursor(request.LastId, request.AfterTime)
	if err != nil {
		return nil, err
	}

	filter := domain.FilterAllProduct{
		Search:  request.Search,
		Barcode: strings.TrimSpace(request.Barcode),
		Cursor:  cursor,
		Limit:   request.Limit,
		Order:   request.Order,
	}
	if request.CategoryID != "" {
		id, err := uuid.Parse(request.CategoryID)
		if err != nil {
			return nil, domain.InvalidID("invalid category id format")
		}
		filter.CategoryID = &id
	}

	result, err := p.productRepo.GetAllProduct(ctx, filter)
	if err != nil {
		p.log.Error("failed to get all products", zap.Error(err))
		return nil, fmt.Errorf("failed to get all products: %w", domain.ErrInternal)
	}

	products := make([]responsedto.ProductDtoResponse, 0, len(result.DataItem))
	for _, item := range result.DataItem {
		products = append(products, toProductResponse(item))
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

func (p *productUsecase) UpdateProductShopWithLock(ctx context.Context, request *requestdto.UpdateProduct) error {
	id, err := uuid.Parse(request.ID)
	if err != nil {
		p.log.Error("failed to parse product id", zap.Error(err))
		return domain.InvalidID("invalid product id format")
	}

	fields := make(map[string]interface{})
	if request.SKU != nil {
		fields["sku"] = *request.SKU
	}
	if request.Barcode != nil {
		barcode, err := normalizeBarcode(*request.Barcode)
		if err != nil {
			return err
		}
		var value any // nil = kolom dikosongkan (NULL)
		if barcode != nil {
			value = *barcode
		}
		fields["barcode"] = value
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
		categoryID, err := p.categoryID(ctx, *request.Category, request.AllowNewCategory)
		if err != nil {
			return err
		}
		var value any
		if categoryID != nil {
			value = *categoryID
		}
		fields["category_id"] = value
	}
	if request.Image != nil {
		fields["image"] = *request.Image
	}

	if len(fields) == 0 {
		return domain.Validation("no fields to update")
	}

	if request.PurchasePrice != nil || request.SellingPrice != nil || request.SellingPriceDebt != nil {
		// Dibaca di luar lock repo; race dua PATCH bersamaan ditahan oleh
		// chk_products_price_rules (migrasi 00004).
		current, err := p.productRepo.GetProduct(ctx, id)
		if err != nil {
			return passOrInternal(p.log, err, "failed to update product", domain.ErrNotFound)
		}
		purchase, selling, sellingDebt := current.PurchasePrice, current.SellingPrice, current.SellingPriceDebt
		if request.PurchasePrice != nil {
			purchase = *request.PurchasePrice
		}
		if request.SellingPrice != nil {
			selling = *request.SellingPrice
		}
		if request.SellingPriceDebt != nil {
			sellingDebt = *request.SellingPriceDebt
		}
		if err := validatePrices(purchase, selling, sellingDebt); err != nil {
			return err
		}
	}

	if err := p.productRepo.UpdateProductWithLock(ctx, id, fields, 0); err != nil {
		p.log.Error("failed to update product", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to update product: %w", domain.ErrInternal)
		}
		return fmt.Errorf("failed to update product: %w", err)
	}
	return nil
}

func (p *productUsecase) UpdateStockWithLock(ctx context.Context, request *requestdto.UpdateStock) error {
	id, err := uuid.Parse(request.ID)
	if err != nil {
		p.log.Error("failed to parse product id", zap.Error(err))
		return domain.InvalidID("invalid product id format")
	}

	if err := p.productRepo.UpdateProductWithLock(ctx, id, map[string]interface{}{}, request.Stock); err != nil {
		p.log.Error("failed to update stock", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to update stock: %w", domain.ErrInternal)
		}
		return fmt.Errorf("failed to update stock: %w", err)
	}
	return nil
}

func (p *productUsecase) AddCategory(ctx context.Context, request *requestdto.CategoryRequest) (*domain.Categories, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, domain.Validation("category name must not be empty")
	}
	category := &domain.Categories{Key: domain.CategoryKey(name), Name: name}
	if err := p.productRepo.AddCategory(ctx, category); err != nil {
		return nil, passOrInternal(p.log, err, "failed to add category", domain.ErrDuplicate)
	}
	return category, nil
}

func (p *productUsecase) RenameCategory(ctx context.Context, rawID string, request *requestdto.CategoryRequest) error {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return domain.InvalidID("invalid category id format")
	}
	if strings.TrimSpace(request.Name) == "" {
		return domain.Validation("category name must not be empty")
	}
	if err := p.productRepo.RenameCategory(ctx, id, request.Name); err != nil {
		return passOrInternal(p.log, err, "failed to rename category", domain.ErrDuplicate, domain.ErrNotFound)
	}
	return nil
}

func (p *productUsecase) DeleteCategory(ctx context.Context, rawID string) error {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return domain.InvalidID("invalid category id format")
	}
	if err := p.productRepo.DeleteCategory(ctx, id); err != nil {
		return passOrInternal(p.log, err, "failed to delete category", domain.ErrNotFound, domain.ErrConflict)
	}
	return nil
}
