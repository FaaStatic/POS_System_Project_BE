package handler

import (
	"errors"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	"shop_project_be/pkg/response"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// isSuperadmin dipakai untuk aturan per-field yang tidak bisa diekspresikan
// lewat RequireRole di route (mis. staff boleh PATCH produk, tapi bukan harga dasar).
func isSuperadmin(c fiber.Ctx) bool {
	role, _ := c.Locals("role").(string)
	return role == enum.Superadmin.String()
}

func bindBody(c fiber.Ctx, out any) error {
	return c.Bind().SkipValidation(true).Body(out)
}

func bindQuery(c fiber.Ctx, out any) error {
	return c.Bind().SkipValidation(true).Query(out)
}

func writeError(c fiber.Ctx, fallback int, err error) error {
	switch {
	case errors.Is(err, domain.ErrInternal):
		return response.Error(c, fiber.StatusInternalServerError, err.Error(), err)
	case errors.Is(err, domain.ErrNotFound):
		return response.Error(c, fiber.StatusNotFound, err.Error(), err)
	case errors.Is(err, domain.ErrInvalidID):
		return response.Error(c, fiber.StatusBadRequest, err.Error(), err)
	case errors.Is(err, domain.ErrValidation):
		return response.Error(c, fiber.StatusBadRequest, err.Error(), err)
	case errors.Is(err, domain.ErrConflict):
		return response.Error(c, fiber.StatusConflict, err.Error(), err)
	case errors.Is(err, domain.ErrDuplicate):
		return response.Error(c, fiber.StatusConflict, err.Error(), err)
	case errors.Is(err, domain.ErrInvalidSignature):
		return response.Error(c, fiber.StatusForbidden, err.Error(), err)
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrPaymentAccessDenied):
		return response.Error(c, fiber.StatusForbidden, err.Error(), err)
	default:
		return response.Error(c, fallback, err.Error(), err)
	}
}
