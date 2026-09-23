package handler

import (
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type CustomerHandler struct {
	usecase domain.CustomerUsecase
	log     *zap.Logger
}

func NewCustomerHandler(usecase domain.CustomerUsecase, log *zap.Logger) *CustomerHandler {
	return &CustomerHandler{usecase: usecase, log: log}
}

// Add godoc
//
//	@Summary		Add customer
//	@Description	Adds a new customer.
//	@Tags			Customers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.AddCustomer	true	"Customer data"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/customers [post]
func (h *CustomerHandler) Add(c fiber.Ctx) error {
	var req requestdto.AddCustomer
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.AddCustomerShop(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "customer created", nil)
}

// Update godoc
//
//	@Summary		Update customer
//	@Description	Update customer data.
//	@Tags			Customers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Customer ID"
//	@Param			request	body		requestdto.UpdateCustomer	true	"Updated customer data"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/customers/{id} [put]
func (h *CustomerHandler) Update(c fiber.Ctx) error {
	var req requestdto.UpdateCustomer
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.CustomerId = c.Params("id")
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.UpdateCustomerShop(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "customer updated", nil)
}

// Delete godoc
//
//	@Summary		Delete customer
//	@Description	Deletes a customer by ID.
//	@Tags			Customers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Customer ID"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/customers/{id} [delete]
func (h *CustomerHandler) Delete(c fiber.Ctx) error {
	req := requestdto.DeleteCustomer{CustomerId: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.DeleteCustomerShop(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "customer deleted", nil)
}

// Get godoc
//
//	@Summary		Detail customer
//	@Description	Fetches customer details by ID.
//	@Tags			Customers
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Customer ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		400	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/customers/{id} [get]
func (h *CustomerHandler) Get(c fiber.Ctx) error {
	req := requestdto.GetCustomer{CustomerId: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	customer, err := h.usecase.GetCustomerShop(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err)
	}
	return response.Success(c, fiber.StatusOK, "customer found", customer)
}

// List godoc
//
//	@Summary		List customer
//	@Description	Fetches the customer list belonging to the logged-in user.
//	@Tags			Customers
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit		query		int		false	"Number of items per page"
//	@Param			search		query		string	false	"Pencarian nama customer"
//	@Param			order		query		string	false	"Sort: ASC or DESC (default DESC)"
//	@Param			after_id	query		string	false	"Cursor: ID of the last row of the previous page"
//	@Param			after_time	query		string	false	"Cursor: created_at of the last row (RFC3339Nano)"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/customers [get]
func (h *CustomerHandler) List(c fiber.Ctx) error {
	var req requestdto.GetAllCustomer
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	customers, err := h.usecase.GetListCustomerShop(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "customers fetched", customers)
}
