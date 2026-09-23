package handler

import (
	"strings"

	"shop_project_be/pkg/pdf"
	"shop_project_be/pkg/response"

	"github.com/gofiber/fiber/v3"
)

// DownloadReport godoc
//
//	@Summary		Download a generated PDF report
//	@Description	Serves a file returned as url_pdf by the report endpoints. Receipts (struk-*) are open to any signed-in user; monthly and debt reports need superadmin, like the endpoints that create them. Files expire an hour after they are generated.
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			file	path	string	true	"File name from url_pdf"
//	@Success		200
//	@Failure		403	{object}	response.APIResponse
//	@Failure		404	{object}	response.APIResponse
//	@Router			/api/reports/{file} [get]
func DownloadReport(c fiber.Ctx) error {
	name := c.Params("file")
	if role, _ := c.Locals("role").(string); role != "superadmin" && !strings.HasPrefix(name, "struk-") {
		return response.Error(c, fiber.StatusForbidden, "akses ditolak", nil)
	}
	path, ok := pdf.ReportPath(name)
	if !ok {
		return response.Error(c, fiber.StatusNotFound, "report not found or expired", nil)
	}
	return c.Download(path, name)
}
