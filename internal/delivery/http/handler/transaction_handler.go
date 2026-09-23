package handler

import (
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type TransactionHandler struct {
	trxUsecase domain.TransactionUsecase
	log        *zap.Logger
}

func NewTransactionHandler(trxUsecase domain.TransactionUsecase, log *zap.Logger) *TransactionHandler {
	return &TransactionHandler{
		trxUsecase: trxUsecase,
		log:        log,
	}
}

// Add godoc
//
//	@Summary		Add transaction
//	@Description	Creates a new sales transaction along with its item details.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.AddTransactionRequest	true	"Transaction data"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/transactions [post]
func (h *TransactionHandler) Add(c fiber.Ctx) error {
	var req requestdto.AddTransactionRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.AddTransaction(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "transaction created", result)
}

// List godoc
//
//	@Summary		List transactions
//	@Description	Fetches the transaction list belonging to the logged-in user with filters.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			date_start		query		string	false	"Start date (YYYY-MM-DD)"
//	@Param			date_end		query		string	false	"End date (YYYY-MM-DD)"
//	@Param			type_payment	query		int		true	"Payment type (0=tunai,1=hutang,2=transfer,3=qris)"
//	@Param			number_invoices	query		string	false	"Invoice number filter"
//	@Param			after_time		query		string	false	"Time cursor for pagination"
//	@Param			after_id		query		string	false	"ID cursor for pagination"
//	@Success		200				{object}	response.APIResponse
//	@Failure		400				{object}	response.APIResponse
//	@Failure		500				{object}	response.APIResponse
//	@Router			/api/transactions [get]
func (h *TransactionHandler) List(c fiber.Ctx) error {
	var req requestdto.FilterTransactionRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.GetAllTransaction(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "transactions fetched", result)
}

// Get godoc
//
//	@Summary		Transaction detail
//	@Description	Fetches transaction details by ID.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Transaction ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		400	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/transactions/{id} [get]
func (h *TransactionHandler) Get(c fiber.Ctx) error {
	req := requestdto.GetTransactionRequest{ID: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.GetTransaction(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusNotFound, err)
	}
	return response.Success(c, fiber.StatusOK, "transaction found", result)
}

// Delete godoc
//
//	@Summary		Delete transaction
//	@Description	Deletes a transaction by ID.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Transaction ID"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/transactions/{id} [delete]
func (h *TransactionHandler) Delete(c fiber.Ctx) error {
	req := requestdto.DeleteTransactionRequest{ID: c.Params("id")}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.trxUsecase.DeleteTransaction(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "transaction deleted", nil)
}

// ReportMonth godoc
//
//	@Summary		Monthly transaction report
//	@Description	Generates a PDF monthly transaction recap report.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			month	query		int	false	"Bulan (1-12)"
//	@Param			year	query		int	false	"Tahun"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/transactions/report/month [get]
func (h *TransactionHandler) ReportMonth(c fiber.Ctx) error {
	var req requestdto.PrintReportMonthRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.PrintReportMonth(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "report generated", result)
}

// ReportTransaction godoc
//
//	@Summary		Transaction detail report
//	@Description	Generates a PDF report for a single transaction.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			trx_id			query		string	false	"Transaction ID"
//	@Param			number_invoice	query		string	false	"Invoice number"
//	@Success		200				{object}	response.APIResponse
//	@Failure		400				{object}	response.APIResponse
//	@Failure		500				{object}	response.APIResponse
//	@Router			/api/transactions/report/transaction [get]
func (h *TransactionHandler) ReportTransaction(c fiber.Ctx) error {
	var req requestdto.PrintReportTransactionRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.PrintReportTransaction(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "report generated", result)
}
