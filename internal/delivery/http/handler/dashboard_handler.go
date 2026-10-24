package handler

import (
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
)

type DashboardHandler struct {
	usecase domain.DashboardUsecase
}

func NewDashboardHandler(usecase domain.DashboardUsecase) *DashboardHandler {
	return &DashboardHandler{usecase: usecase}
}

// Summary godoc
//
//	@Summary		Dashboard summary
//	@Description	Sales, gross profit, debt flow, top products, payment method split and signed-in devices for a date range (WIB). Defaults to today; at most 366 days. Superadmin only.
//	@Tags			Dashboard
//	@Produce		json
//	@Security		BearerAuth
//	@Param			date_start	query		string	false	"YYYY-MM-DD"
//	@Param			date_end	query		string	false	"YYYY-MM-DD, inclusive"
//	@Success		200			{object}	response.APIResponse
//	@Failure		400			{object}	response.APIResponse
//	@Router			/api/dashboard/summary [get]
func (h *DashboardHandler) Summary(c fiber.Ctx) error {
	var req requestdto.DashboardRequest
	if err := bindQuery(c, &req); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "invalid query", err)
	}
	result, err := h.usecase.Summary(c.Context(), &req)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err)
	}
	return response.Success(c, fiber.StatusOK, "dashboard fetched", result)
}
