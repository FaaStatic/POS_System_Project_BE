package handler

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

func TestDownloadTemplate(t *testing.T) {
	app := fiber.New()
	h := NewProductHandler(nil, zap.NewNop()) // template tidak memakai usecase
	app.Get("/api/v1/catalog/products/template", h.DownloadTemplate)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/catalog/products/template", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != xlsxContentType {
		t.Errorf("Content-Type = %q, want the xlsx type", got)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".xlsx") {
		t.Errorf("Content-Disposition = %q, want an .xlsx attachment", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not a valid xlsx: %v", err)
	}
	defer f.Close()
	if sheets := f.GetSheetList(); len(sheets) == 0 || sheets[0] != "products" {
		t.Errorf("sheets = %v, want products first", sheets)
	}
}
