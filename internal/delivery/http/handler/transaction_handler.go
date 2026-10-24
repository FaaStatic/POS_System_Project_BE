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
	if err := validate.Struct(&req); err != nil {
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
	return h.listTransactions(c, "")
}

// ListByCustomer godoc
//
//	@Summary		List customer transactions
//	@Description	Fetches a customer's transactions, newest first, with keyset pagination.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id				path		string	true	"Customer ID"
//	@Param			type_payment	query		int		false	"Payment type (0=tunai,1=hutang,2=transfer,3=qris)"
//	@Param			limit			query		int		false	"Page size (1-100, default 10)"
//	@Param			after_time		query		string	false	"Time cursor for pagination"
//	@Param			after_id		query		string	false	"ID cursor for pagination"
//	@Success		200				{object}	response.APIResponse
//	@Failure		400				{object}	response.APIResponse
//	@Failure		500				{object}	response.APIResponse
//	@Router			/api/customers/{id}/transactions [get]
func (h *TransactionHandler) ListByCustomer(c fiber.Ctx) error {
	return h.listTransactions(c, c.Params("id"))
}

func (h *TransactionHandler) listTransactions(c fiber.Ctx, customerID string) error {
	var req requestdto.FilterTransactionRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	req.UserId = middleware.GetUserID(c)
	req.CustomerID = customerID
	if err := validate.Struct(&req); err != nil {
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
	if err := validate.Struct(&req); err != nil {
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
//	@Summary		Cancel transaction
//	@Description	Cancels (soft-deletes) a transaction by ID; the reason is stored with it.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string								true	"Transaction ID"
//	@Param			request	body		requestdto.DeleteTransactionRequest	true	"Cancel reason"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/transactions/{id} [delete]
func (h *TransactionHandler) Delete(c fiber.Ctx) error {
	var req requestdto.DeleteTransactionRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	role, _ := c.Locals("role").(string)
	req.ID, req.UserID, req.Role = c.Params("id"), middleware.GetUserID(c), role
	if err := validate.Struct(&req); err != nil {
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
	if err := validate.Struct(&req); err != nil {
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
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.PrintReportTransaction(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "report generated", result)
}

// Sync godoc
//
//	@Summary		Sync offline transactions
//	@Description	Records up to 100 cash/debt transactions made while the cashier was offline. Each item carries a client_ref (UUID) so resending the queue is safe; items succeed or fail independently. Stock may go negative, which flags the transaction as a stock conflict for review.
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.SyncTransactionsRequest	true	"Offline queue"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Router			/api/transactions/sync [post]
func (h *TransactionHandler) Sync(c fiber.Ctx) error {
	var req requestdto.SyncTransactionsRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.trxUsecase.SyncOfflineTransactions(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "sync processed", result)
}

// ReviewConflict godoc
//
//	@Summary		Mark stock conflict reviewed
//	@Description	Marks an offline transaction that pushed stock below zero as reviewed. Superadmin only.
//	@Tags			Transactions
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Transaction ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/transactions/{id}/conflict-review [patch]
func (h *TransactionHandler) ReviewConflict(c fiber.Ctx) error {
	if err := h.trxUsecase.ReviewConflict(c.Context(), c.Params("id")); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "conflict marked as reviewed", nil)
}
