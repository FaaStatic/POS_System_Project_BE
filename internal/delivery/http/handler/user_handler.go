package handler

import (
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type UserHandler struct {
	usecase    domain.UserUsecase
	fcmUsecase domain.DeviceTokenUsecase
	log        *zap.Logger
}

func NewUserHandler(usecase domain.UserUsecase, fcmUsecase domain.DeviceTokenUsecase, log *zap.Logger) *UserHandler {
	return &UserHandler{usecase: usecase, fcmUsecase: fcmUsecase, log: log}
}

// Register godoc
//
//	@Summary		Register new staff
//	@Description	Staff account registration, superadmin only. The role is always forced to "staff"; a superadmin is created out of band with the create-admin command.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.UserRegisterRequest	true	"New staff data"
//	@Success		201		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		409		{object}	response.APIResponse
//	@Router			/api/auth/register [post]
func (h *UserHandler) Register(c fiber.Ctx) error {
	var req requestdto.UserRegisterRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	result, err := h.usecase.RegisterUser(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusCreated, "register success", result)
}

// Login godoc
//
//	@Summary		Login user
//	@Description	Authenticates the user and produces access & refresh tokens.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requestdto.UserLoginRequest	true	"Login credentials"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		401		{object}	response.APIResponse
//	@Router			/auth/login [post]
func (h *UserHandler) Login(c fiber.Ctx) error {
	var req requestdto.UserLoginRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	result, err := h.usecase.UserLogin(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusUnauthorized, err)
	}
	return response.Success(c, fiber.StatusOK, "login success", result)
}

// Logout godoc
//
//	@Summary		Logout
//	@Description	Revokes the caller's session: deletes the access session, the refresh session when refresh_token is supplied, and clears the online marker. Send fcm_token to also detach this device so push notifications stop.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		requestdto.UserLogoutRequest	false	"Optional refresh and FCM tokens to revoke alongside the session"
//	@Success		200		{object}	response.APIResponse
//	@Failure		401		{object}	response.APIResponse
//	@Failure		500		{object}	response.APIResponse
//	@Router			/api/auth/logout [post]
func (h *UserHandler) Logout(c fiber.Ctx) error {
	var req requestdto.UserLogoutRequest
	if err := bindBody(c, &req); err != nil {
		h.log.Debug("logout called without a parsable body", zap.Error(err))
	}

	accessToken, _ := c.Locals("access_token").(string)
	userID := middleware.GetUserID(c)

	if req.FcmToken != "" {
		if err := h.fcmUsecase.HandleLogout(c.Context(), req.FcmToken); err != nil {
			h.log.Warn("failed to detach device token on logout",
				zap.Error(err), zap.String("user_id", userID))
		}
	}

	if err := h.usecase.Logout(c.Context(), accessToken, &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "logout success", nil)
}

// Refresh godoc
//
//	@Summary		Refresh access token
//	@Description	Exchanges a valid refresh token for a fresh access/refresh token pair.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		requestdto.UserRefreshTokenRequest	true	"Refresh token"
//	@Success		200		{object}	response.APIResponse
//	@Failure		400		{object}	response.APIResponse
//	@Failure		401		{object}	response.APIResponse
//	@Router			/auth/refresh [post]
func (h *UserHandler) Refresh(c fiber.Ctx) error {
	var req requestdto.UserRefreshTokenRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if err := validate.Validate(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	result, err := h.usecase.RefreshToken(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusUnauthorized, err)
	}
	return response.Success(c, fiber.StatusOK, "refresh success", result)
}
