package handler

import (
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type DebtHandler struct {
	usecase domain.DebtUseCase
	log     *zap.Logger
}

func NewDebtHandler(usecase domain.DebtUseCase, log *zap.Logger) *DebtHandler {
	return &DebtHandler{usecase: usecase, log: log}
}

// Add godoc
//
//	@Summary		Add customer debt
//	@Description	Records a new debt for a customer.
//	@Tags			Debts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.AddDebtRequest	true	"Debt data"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/debts [post]
func (h *DebtHandler) Add(c fiber.Ctx) error {
	var req requestdto.AddDebtRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.AddingDebtCustomer(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	h.log.Info("audit: manual debt added",
		zap.String("user_id", middleware.GetUserID(c)),
		zap.String("customer_id", req.CustomerID),
		zap.Int64("amount", req.TotalTransaksi),
		zap.String("jatuh_tempo", req.JatuhTempo),
	)
	return response.Success(c, fiber.StatusCreated, "debt created", nil)
}

// Delete godoc
//
//	@Summary		Delete debt
//	@Description	Deletes a debt record by ID.
//	@Tags			Debts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Debt ID"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/debts/{id} [delete]
func (h *DebtHandler) Delete(c fiber.Ctx) error {
	req := requestdto.DeleteDebtRequest{DebtId: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.DeleteDebtCustomer(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "debt deleted", nil)
}

// List godoc
//
//	@Summary		List debts
//	@Description	Fetches the debt list with customer and period filters.
//	@Tags			Debts
//	@Produce		json
//	@Security		BearerAuth
//	@Param			customer_id	query		string	false	"Filter customer ID"
//	@Param			month		query		string	false	"Filter bulan"
//	@Param			year		query		string	false	"Filter tahun"
//	@Success		200			{object}	response.APIResponse
//	@Failure		400			{object}	response.APIResponse
//	@Failure		500			{object}	response.APIResponse
//	@Router			/api/debts [get]
func (h *DebtHandler) List(c fiber.Ctx) error {
	var req requestdto.FilterDebtRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.GetAllDebtCustomerList(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "debts fetched", result)
}

// Get godoc
//
//	@Summary		Debt detail
//	@Description	Fetches debt details by ID.
//	@Tags			Debts
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Debt ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		400	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/debts/{id} [get]
func (h *DebtHandler) Get(c fiber.Ctx) error {
	req := requestdto.GetDebtRequest{DebtId: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.GetDebtCustomer(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err)
	}
	return response.Success(c, fiber.StatusOK, "debt found", result)
}

// Pay godoc
//
//	@Summary		Pay debt in cash
//	@Description	Records a cash payment made by a customer at the register toward an existing debt. The nominal does not have to cover the full remaining balance.
//	@Tags			Debts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.DebtPayment	true	"Cash payment data"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/debts/pay [post]
func (h *DebtHandler) Pay(c fiber.Ctx) error {
	var req requestdto.DebtPayment
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.UserID = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.PayDebtCash(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusBadRequest, err)
	}
	return response.Success(c, fiber.StatusOK, "debt payment recorded", result)
}

// Report godoc
//
//	@Summary		Customer debt report
//	@Description	Generates a PDF customer debt report based on filters.
//	@Tags			Debts
//	@Produce		json
//	@Security		BearerAuth
//	@Param			debt_id			query		string	false	"Debt ID"
//	@Param			name_customer	query		string	false	"Nama customer"
//	@Param			month			query		string	false	"Bulan"
//	@Param			year			query		string	false	"Tahun"
//	@Success		200				{object}	response.APIResponse
//	@Failure		500				{object}	response.APIResponse
//	@Router			/api/debts/report [get]
func (h *DebtHandler) Report(c fiber.Ctx) error {
	var req requestdto.PrintDebtReport
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.PrintReportDebtCustomer(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "report generated", result)
}
