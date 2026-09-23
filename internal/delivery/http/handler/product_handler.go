package handler

import (
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

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
//	@Router			/api/products [post]
func (h *ProductHandler) Add(c fiber.Ctx) error {
	var req requestdto.AddProduct
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
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
//	@Router			/api/products/bulk [post]
func (h *ProductHandler) AddBulk(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file_upload")
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "file_upload is required", err)
	}
	result, err := h.usecase.AddBulkProductShopWithLock(c.Context(), &requestdto.AddBulkProduct{FileUpload: fileHeader})
	if err != nil {
		return writeError(c, fiber.StatusBadRequest, err)
	}
	if result.TotalInserted == 0 {
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
//	@Router			/api/products/{id} [get]
func (h *ProductHandler) Get(c fiber.Ctx) error {
	req := requestdto.GetProduct{ID: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
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
//	@Param			category	query		string	false	"Filter kategori"
//	@Param			search		query		string	false	"Search by product name/SKU"
//	@Param			limit		query		int		false	"Number of items per page"
//	@Param			last_id		query		string	false	"Last ID for cursor pagination"
//	@Param			after_time	query		string	false	"Time cursor for pagination"
//	@Param			order		query		string	false	"Sort order"
//	@Success		200			{object}	response.APIResponse
//	@Failure		400			{object}	response.APIResponse
//	@Failure		500			{object}	response.APIResponse
//	@Router			/api/products [get]
func (h *ProductHandler) List(c fiber.Ctx) error {
	var req requestdto.GetAllProduct
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	products, err := h.usecase.GetAllProductShop(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Paginated(c, fiber.StatusOK, "products fetched", products)
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
//	@Router			/api/products/{id} [put]
func (h *ProductHandler) Update(c fiber.Ctx) error {
	var req requestdto.UpdateProduct
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.ID = c.Params("id")
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.UpdateProductShopWithLock(c.Context(), &req, 0); err != nil {
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
//	@Router			/api/products/stock [patch]
func (h *ProductHandler) UpdateStock(c fiber.Ctx) error {
	var req requestdto.UpdateStock
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.UpdateStockWithLock(c.Context(), &req, req.Stock); err != nil {
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
//	@Router			/api/products/{id} [delete]
func (h *ProductHandler) Delete(c fiber.Ctx) error {
	req := requestdto.DeleteProduct{ID: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.DeleteProductShop(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "product deleted", nil)
}
