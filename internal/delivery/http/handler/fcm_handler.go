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

	if err := validate.Struct(&req); err != nil {
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

	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	if err := h.fcmUsecase.HandleLogout(c.Context(), req.Token); err != nil {
		h.log.Error("failed to handle logout", zap.Error(err))
		return response.Error(c, fiber.StatusInternalServerError, "failed to handle logout", err)
	}

	return response.Success(c, fiber.StatusOK, "logout successful", nil)
}

// ListDevices godoc
//
//	@Summary		List devices
//	@Description	Lists registered Android/iOS devices with the user currently signed in on each. Superadmin only.
//	@Tags			Devices
//	@Produce		json
//	@Security		BearerAuth
//	@Param			platform	query		string	false	"android or ios"
//	@Success		200			{object}	response.APIResponse
//	@Router			/api/fcm/devices [get]
func (h *FcmHandler) ListDevices(c fiber.Ctx) error {
	platform := c.Query("platform")
	if platform != "" && platform != "android" && platform != "ios" {
		return response.Error(c, fiber.StatusBadRequest, "platform must be android or ios", nil)
	}
	devices, err := h.fcmUsecase.ListDevices(c.Context(), platform)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "devices fetched", devices)
}

// RevokeDevice godoc
//
//	@Summary		Revoke device
//	@Description	Deletes a device token so the device stops receiving push notifications until it registers again. Superadmin only.
//	@Tags			Devices
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Device row ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/fcm/devices/{id} [delete]
func (h *FcmHandler) RevokeDevice(c fiber.Ctx) error {
	if err := h.fcmUsecase.RevokeDevice(c.Context(), c.Params("id")); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "device revoked", nil)
}
