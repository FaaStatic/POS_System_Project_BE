package handler

import (
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type FcmHandler struct {
	fcmUsecase domain.DeviceTokenUsecase
	log        *zap.Logger
}

func NewFcmHandler(fcmUsecase domain.DeviceTokenUsecase, log *zap.Logger) *FcmHandler {
	return &FcmHandler{
		fcmUsecase: fcmUsecase,
		log:        log,
	}
}

func (h *FcmHandler) Register(c fiber.Ctx) error {
	var req requestdto.RegisterDeviceRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}

	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	userId, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Error(c, fiber.StatusUnauthorized, "user_id not found in context", nil)
	}

	if err := h.fcmUsecase.RegisterDevice(c.Context(), userId, req.Token, req.Platform, req.DeviceID); err != nil {
		h.log.Error("failed to register device token", zap.Error(err))
		return response.Error(c, fiber.StatusInternalServerError, "failed to register device token", err)
	}

	return response.Success(c, fiber.StatusOK, "device registered successfully", nil)
}

func (h *FcmHandler) Logout(c fiber.Ctx) error {
	var req requestdto.LogoutDeviceRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}

	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	if err := h.fcmUsecase.HandleLogout(c.Context(), req.Token); err != nil {
		h.log.Error("failed to handle logout", zap.Error(err))
		return response.Error(c, fiber.StatusInternalServerError, "failed to handle logout", err)
	}

	return response.Success(c, fiber.StatusOK, "logout successful", nil)
}
