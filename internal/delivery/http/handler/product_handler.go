package handler

import (
	"strconv"

	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"
	"shop_project_be/pkg/sheet"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type ProductHandler struct {
	usecase domain.ProductUsecase
	log     *zap.Logger
}

func NewProductHandler(usecase domain.ProductUsecase, log *zap.Logger) *ProductHandler {
	return &ProductHandler{usecase: usecase, log: log}
}

// Add godoc
//
//	@Summary		Add product
//	@Description	Adds a new product to the logged-in user's store.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.AddProduct	true	"Product data"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/v1/catalog/products [post]
func (h *ProductHandler) Add(c fiber.Ctx) error {
	var req requestdto.AddProduct
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.AddProductShopWithLock(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "product created", nil)
}

// AddBulk godoc
//
//	@Summary		Bulk product import
//	@Description	Imports many products at once from a CSV/Excel file.
//	@Tags			Products
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			file_upload	formData	file	true	"Product CSV/Excel file (max 10000 rows)"
//	@Success		201			{object}	response.APIResponse	"Some rows imported; data is the per-row report"
//	@Success		200			{object}	response.APIResponse	"Nothing imported; data says why"
//	@Failure		400			{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/bulk [post]
func (h *ProductHandler) AddBulk(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file_upload")
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "file_upload is required", err)
	}
	result, err := h.usecase.AddBulkProductShopWithLock(c.Context(), &requestdto.AddBulkProduct{FileUpload: fileHeader})
	if err != nil {
		return writeError(c, fiber.StatusBadRequest, err)
	}
	if result.TotalInserted == 0 && result.TotalUpdated == 0 {
		return response.Success(c, fiber.StatusOK, "no product imported", result)
	}
	return response.Success(c, fiber.StatusCreated, "bulk product imported", result)
}

// Get godoc
//
//	@Summary		Product detail
//	@Description	Fetches product details by ID.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Product ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		400	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/{id} [get]
func (h *ProductHandler) Get(c fiber.Ctx) error {
	req := requestdto.GetProduct{ID: c.Params("id")}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	product, err := h.usecase.GetProductShop(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err)
	}
	return response.Success(c, fiber.StatusOK, "product found", product)
}

// List godoc
//
//	@Summary		List products
//	@Description	Fetches the product list belonging to the logged-in user, with filters and pagination.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			category_id	query		string	false	"Filter category ID"
//	@Param			search		query		string	false	"Search by product name/SKU"
//	@Param			barcode		query		string	false	"Exact barcode match (cashier scan)"
//	@Param			limit		query		int		false	"Number of items per page"
//	@Param			last_id		query		string	false	"Last ID for cursor pagination"
//	@Param			after_time	query		string	false	"Time cursor for pagination"
//	@Param			order		query		string	false	"Sort order"
//	@Success		200			{object}	response.APIResponse
//	@Failure		400			{object}	response.APIResponse
//	@Failure		500			{object}	response.APIResponse
//	@Router			/api/v1/catalog/products [get]
func (h *ProductHandler) List(c fiber.Ctx) error {
	var req requestdto.GetAllProduct
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	products, err := h.usecase.GetAllProductShop(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "products fetched", products)
}

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// DownloadTemplate godoc
//
//	@Summary		Download the product import template (Excel)
//	@Description	Downloads an .xlsx template for POST /api/v1/catalog/products/bulk. The sku, barcode and image columns may be left empty: they are generated on import. A picture inserted in a cell of the image column (Insert > Pictures, Place in Cell or Place Over Cells) is converted automatically to a square 1:1 WebP under 500 KB and stored as base64. Unit and product_type have drop-down lists, and the Petunjuk sheet explains every column.
//	@Tags			Products
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200
//	@Failure		500	{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/template [get]
func (h *ProductHandler) DownloadTemplate(c fiber.Ctx) error {
	data, err := sheet.WriteProductTemplate()
	if err != nil {
		h.log.Error("failed to build product template", zap.Error(err))
		return response.Error(c, fiber.StatusInternalServerError, "failed to build template", err)
	}
	c.Set(fiber.HeaderContentType, xlsxContentType)
	c.Attachment("template-produk.xlsx")
	return c.Send(data)
}

// ExportBarcodes godoc
//
//	@Summary		Export product barcodes to Excel
//	@Description	Downloads an .xlsx of product names with Code128 barcode images, for printing and sticking on products. Uses the same search and category filters as the list; with no filter every product is exported. Each file holds at most 500 products: read X-Total-Parts and call again with part=2, 3, ... to download the rest.
//	@Tags			Products
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			category_id	query	string	false	"Filter category ID"
//	@Param			search		query	string	false	"Search by product name"
//	@Param			part		query	int		false	"File part, starting at 1 (default 1)"
//	@Success		200
//	@Header			200	{integer}	X-Total-Parts	"Number of files for this filter"
//	@Header			200	{integer}	X-Total-Items	"Number of products matching this filter"
//	@Failure		400	{object}	response.APIResponse
//	@Failure		500	{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/export/barcodes [get]
func (h *ProductHandler) ExportBarcodes(c fiber.Ctx) error {
	var req requestdto.ExportBarcodes
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	export, err := h.usecase.ExportBarcodes(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	c.Set("X-Total-Parts", strconv.Itoa(export.TotalParts))
	c.Set("X-Total-Items", strconv.Itoa(export.TotalItems))
	c.Set(fiber.HeaderContentType, xlsxContentType)
	c.Attachment(export.Name)
	return c.Send(export.File)
}

// Update godoc
//
//	@Summary		Update product
//	@Description	Updates product attributes (does not change stock).
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Product ID"
//	@Param			request	body		requestdto.UpdateProduct	true	"Updated product data"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/{id} [patch]
func (h *ProductHandler) Update(c fiber.Ctx) error {
	var req requestdto.UpdateProduct
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.ID = c.Params("id")
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	superadmin := isSuperadmin(c)
	if !superadmin && (req.PurchasePrice != nil || req.ProductType != nil) {
		return response.Error(c, fiber.StatusForbidden, "only superadmin can change purchase_price or product_type", nil)
	}
	req.AllowNewCategory = superadmin
	if err := h.usecase.UpdateProductShopWithLock(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "product updated", nil)
}

// UpdateStock godoc
//
//	@Summary		Update product stock
//	@Description	Changes product stock by a delta (positive adds, negative subtracts).
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.UpdateStock	true	"Product stock delta"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/stock [patch]
func (h *ProductHandler) UpdateStock(c fiber.Ctx) error {
	var req requestdto.UpdateStock
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.UpdateStockWithLock(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	h.log.Info("audit: stock adjusted",
		zap.String("user_id", middleware.GetUserID(c)),
		zap.String("product_id", req.ID),
		zap.Float64("delta", req.Stock),
	)
	return response.Success(c, fiber.StatusOK, "stock updated", nil)
}

// Delete godoc
//
//	@Summary		Delete product
//	@Description	Deletes a product by ID.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Product ID"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/v1/catalog/products/{id} [delete]
func (h *ProductHandler) Delete(c fiber.Ctx) error {
	req := requestdto.DeleteProduct{ID: c.Params("id")}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.DeleteProductShop(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "product deleted", nil)
}

// ListCategories godoc
//
//	@Summary		List categories
//	@Description	Fetches all product categories, sorted by name.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.APIResponse
//	@Failure		500	{object}	response.APIResponse
//	@Router			/api/v1/catalog/categories [get]
func (h *ProductHandler) ListCategories(c fiber.Ctx) error {
	categories, err := h.usecase.ListCategories(c.Context())
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "categories fetched", categories)
}

// AddCategory godoc
//
//	@Summary		Add category
//	@Description	Adds a product category. Names are unique ignoring case and surrounding spaces. Superadmin only.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.CategoryRequest	true	"Category name"
//	@Success		201		{object}	response.APIResponse
//	@Failure		409		{object}	response.APIResponse
//	@Router			/api/v1/catalog/categories [post]
func (h *ProductHandler) AddCategory(c fiber.Ctx) error {
	var req requestdto.CategoryRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	category, err := h.usecase.AddCategory(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "category added", category)
}

// RenameCategory godoc
//
//	@Summary		Rename category
//	@Description	Renames a product category. Superadmin only.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"Category ID"
//	@Param			request	body		requestdto.CategoryRequest	true	"New name"
//	@Success		200		{object}	response.APIResponse
//	@Failure		409		{object}	response.APIResponse
//	@Router			/api/v1/catalog/categories/{id} [patch]
func (h *ProductHandler) RenameCategory(c fiber.Ctx) error {
	var req requestdto.CategoryRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.RenameCategory(c.Context(), c.Params("id"), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "category renamed", nil)
}

// DeleteCategory godoc
//
//	@Summary		Delete category
//	@Description	Deletes a category that no active product uses (see product_count in the list). Superadmin only.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Category ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		409	{object}	response.APIResponse
//	@Router			/api/v1/catalog/categories/{id} [delete]
func (h *ProductHandler) DeleteCategory(c fiber.Ctx) error {
	if err := h.usecase.DeleteCategory(c.Context(), c.Params("id")); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "category deleted", nil)
}
