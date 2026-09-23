package handler

import (
	"errors"

	"shop_project_be/internal/domain"
	"shop_project_be/pkg/response"
	appvalidator "shop_project_be/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

var validate = appvalidator.New()

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
	case errors.Is(err, domain.ErrPaymentAccessDenied):
		return response.Error(c, fiber.StatusForbidden, err.Error(), err)
	default:
		return response.Error(c, fallback, err.Error(), err)
	}
}
