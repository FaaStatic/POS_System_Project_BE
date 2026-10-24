package handler

import (
	"shop_project_be/internal/delivery/http/middleware"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"shop_project_be/pkg/response"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type UserHandler struct {
	usecase    domain.UserUsecase
	fcmUsecase domain.DeviceTokenUsecase
	log        *zap.Logger
	webCookie  WebCookieConfig
}

// WebCookieConfig: Secure dimatikan hanya di development (http://localhost).
type WebCookieConfig struct {
	Secure     bool
	RefreshTTL time.Duration
}

func NewUserHandler(usecase domain.UserUsecase, fcmUsecase domain.DeviceTokenUsecase, log *zap.Logger, webCookie WebCookieConfig) *UserHandler {
	return &UserHandler{usecase: usecase, fcmUsecase: fcmUsecase, log: log, webCookie: webCookie}
}

const refreshCookieName = "refresh_token"

// isWebClient: CMS web mengirim X-Client: web. Refresh token web hanya hidup di
// cookie HttpOnly. Header custom ini juga penjaga CSRF: form atau link dari situs
// lain tidak bisa menambahkan header tanpa lolos preflight CORS, jadi cookie
// tidak pernah dipakai untuk request yang tidak membawanya.
func isWebClient(c fiber.Ctx) bool {
	return c.Get("X-Client") == "web"
}

func (h *UserHandler) setRefreshCookie(c fiber.Ctx, token string, maxAge int) {
	c.Cookie(&fiber.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   h.webCookie.Secure,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

func (h *UserHandler) clearRefreshCookie(c fiber.Ctx) {
	h.setRefreshCookie(c, "", -1)
}

// moveRefreshToCookie memindahkan refresh token dari body ke cookie untuk klien web.
func (h *UserHandler) moveRefreshToCookie(c fiber.Ctx, result *responsedto.UserLoginResponse) {
	if !isWebClient(c) {
		return
	}
	h.setRefreshCookie(c, result.RefreshToken, int(h.webCookie.RefreshTTL.Seconds()))
	result.RefreshToken = ""
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
	if err := validate.Struct(&req); err != nil {
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
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	req.AdminOnly = isWebClient(c)

	result, err := h.usecase.UserLogin(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusUnauthorized, err)
	}
	h.moveRefreshToCookie(c, result)
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

	if isWebClient(c) {
		if req.RefreshToken == "" {
			req.RefreshToken = c.Cookies(refreshCookieName)
		}
		h.clearRefreshCookie(c)
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
	web := isWebClient(c)
	// Klien web boleh mengirim body kosong; tokennya ada di cookie.
	if err := bindBody(c, &req); err != nil && !web {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	if web && req.RefreshToken == "" {
		req.RefreshToken = c.Cookies(refreshCookieName)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}

	result, err := h.usecase.RefreshToken(c.Context(), &req)
	if err != nil {
		if web {
			h.clearRefreshCookie(c)
		}
		return writeError(c, fiber.StatusUnauthorized, err)
	}
	h.moveRefreshToCookie(c, result)
	return response.Success(c, fiber.StatusOK, "refresh success", result)
}

// ListStaff godoc
//
//	@Summary		List staff
//	@Description	Lists user accounts. status=inactive returns deactivated accounts. Superadmin only.
//	@Tags			Staff
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status		query		string	false	"active (default) or inactive"
//	@Param			search		query		string	false	"Username contains"
//	@Param			limit		query		int		false	"1-100"
//	@Param			after_id	query		string	false	"Cursor id"
//	@Param			after_time	query		string	false	"Cursor time"
//	@Success		200			{object}	response.APIResponse
//	@Router			/api/users [get]
func (h *UserHandler) ListStaff(c fiber.Ctx) error {
	var req requestdto.ListStaffRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.ListStaff(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "staff fetched", result)
}

// UpdateStaff godoc
//
//	@Summary		Update staff
//	@Description	Changes a staff username and/or password. A new password signs the staff out of every device. Superadmin only.
//	@Tags			Staff
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string							true	"User ID"
//	@Param			request	body		requestdto.UpdateStaffRequest	true	"Fields to change"
//	@Success		200		{object}	response.APIResponse
//	@Failure		403		{object}	response.APIResponse
//	@Failure		409		{object}	response.APIResponse
//	@Router			/api/users/{id} [patch]
func (h *UserHandler) UpdateStaff(c fiber.Ctx) error {
	var req requestdto.UpdateStaffRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.ID = c.Params("id")
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	if err := h.usecase.UpdateStaff(c.Context(), &req); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "staff updated", nil)
}

// DeactivateStaff godoc
//
//	@Summary		Deactivate staff
//	@Description	Deactivates a staff account, signs it out everywhere and removes its push tokens. History stays intact. Superadmin only.
//	@Tags			Staff
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/users/{id} [delete]
func (h *UserHandler) DeactivateStaff(c fiber.Ctx) error {
	if err := h.usecase.DeactivateStaff(c.Context(), c.Params("id")); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "staff deactivated", nil)
}

// RestoreStaff godoc
//
//	@Summary		Restore staff
//	@Description	Reactivates a deactivated staff account. Superadmin only.
//	@Tags			Staff
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/users/{id}/restore [post]
func (h *UserHandler) RestoreStaff(c fiber.Ctx) error {
	if err := h.usecase.RestoreStaff(c.Context(), c.Params("id")); err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "staff restored", nil)
}

// PurgeStaff godoc
//
//	@Summary		Delete staff permanently
//	@Description	Permanently removes a deactivated staff account. Accounts referenced by sales, payments or debts are anonymized instead (mode=anonymized) so reports stay intact. confirm must equal the username. Superadmin only.
//	@Tags			Staff
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"User ID"
//	@Param			request	body		requestdto.PurgeStaffRequest	true	"Username confirmation"
//	@Success		200		{object}	response.APIResponse
//	@Failure		409		{object}	response.APIResponse
//	@Router			/api/users/{id}/permanent [delete]
func (h *UserHandler) PurgeStaff(c fiber.Ctx) error {
	var req requestdto.PurgeStaffRequest
	if err := bindBody(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid request body", err)
	}
	req.ID = c.Params("id")
	if err := validate.Struct(&req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "validation failed", err)
	}
	result, err := h.usecase.PurgeStaff(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "staff deleted", result)
}
