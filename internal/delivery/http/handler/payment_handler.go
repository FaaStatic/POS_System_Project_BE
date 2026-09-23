package handler

import (
	"errors"

	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type PaymentHandler struct {
	usecase domain.PaymentUsecase
	log     *zap.Logger
}

func NewPaymentHandler(usecase domain.PaymentUsecase, log *zap.Logger) *PaymentHandler {
	return &PaymentHandler{usecase: usecase, log: log}
}

// ChargeQris godoc
//
//	@Summary		Create QRIS payment
//	@Description	Creates a QRIS charge via Midtrans. The response contains qr_url/qr_string to display as a QR code.
//	@Tags			Payments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.ChargeQrisRequest	true	"QRIS payment cart"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/payments/qris [post]
func (h *PaymentHandler) ChargeQris(c fiber.Ctx) error {
	var req requestdto.ChargeQrisRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	res, err := h.usecase.ChargeQris(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "qris payment created", res)
}

// ChargeVA godoc
//
//	@Summary		Create Virtual Account payment
//	@Description	Creates a VA charge via Midtrans. bank = "bca" (bank_transfer) or "mandiri" (echannel). Response contains va_number or bill_key/biller_code.
//	@Tags			Payments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.ChargeVARequest	true	"VA payment cart"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/payments/va [post]
func (h *PaymentHandler) ChargeVA(c fiber.Ctx) error {
	var req requestdto.ChargeVARequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.UserId = middleware.GetUserID(c)
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	res, err := h.usecase.ChargeVA(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "va payment created", res)
}

// Status godoc
//
//	@Summary		Check payment status
//	@Description	Returns the current payment status from the DB. Used by Flutter to poll after showing the QR / completing 3DS.
//	@Tags			Payments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			order_id	path		string	true	"Order ID (= no_invoice)"
//	@Success		200			{object}	response.APIResponse
//	@Failure		403			{object}	response.APIResponse
//	@Failure		404			{object}	response.APIResponse
//	@Router			/api/payments/{order_id}/status [get]
func (h *PaymentHandler) Status(c fiber.Ctx) error {
	orderID := c.Params("order_id")
	role, _ := c.Locals("role").(string)
	res, err := h.usecase.GetStatus(c.Context(), orderID, middleware.GetUserID(c), role)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "payment status fetched", res)
}

// Notification godoc
//
//	@Summary		Midtrans notification webhook
//	@Description	Midtrans HTTP(S) Notification endpoint. PUBLIC (no JWT) — authenticity is validated via signature_key. Register this URL in the Midtrans dashboard.
//	@Tags			Payments
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requestdto.MidtransNotificationRequest	true	"Midtrans notification payload"
//	@Success		200		{object}	response.APIResponse
//	@Failure		403		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/payments/notification [post]
func (h *PaymentHandler) Notification(c fiber.Ctx) error {
	var req requestdto.MidtransNotificationRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid notification body", err)
	}
	if err := h.usecase.HandleNotification(c.Context(), &req); err != nil {
		if errors.Is(err, domain.ErrInvalidSignature) {
			return response.Error(c, fiber.StatusForbidden, "invalid signature", err)
		}
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "notification processed", nil)
}
