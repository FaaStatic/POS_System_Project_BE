package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"mime/multipart"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"shop_project_be/pkg/productimg"
	"shop_project_be/pkg/sheet"

	"github.com/gen2brain/webp"
	"github.com/xuri/excelize/v2"
)

func TestConvertRowImages_DedupesIdenticalImages(t *testing.T) {
	shot := gradientPNG(t, 64, 64)
	rows := []sheet.ProductRow{
		{ImageFile: shot},
		{ImageFile: shot},
		{},
		{Image: "https://cdn.example.com/a.png"},
		{Image: "https://cdn.example.com/a.png"},
	}
	got, err := convertRowImages(rows)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].err != nil || got[0].value == "" || got[1] != got[0] {
		t.Errorf("identical pictures must share one result, got %+v and %+v", got[0], got[1])
	}
	if got[2] != (rowImageResult{}) {
		t.Errorf("a row without image must stay empty, got %+v", got[2])
	}
	if got[3].value != "https://cdn.example.com/a.png" || got[4] != got[3] {
		t.Errorf("identical text must share one result, got %+v and %+v", got[3], got[4])
	}
}

func TestConvertRowImages_CapsDistinctImages(t *testing.T) {
	rows := make([]sheet.ProductRow, maxImportImages+1)
	for i := range rows {
		rows[i].Image = "https://cdn.example.com/" + strconv.Itoa(i) + ".png"
	}
	if _, err := convertRowImages(rows); err == nil {
		t.Fatalf("%d distinct images must be rejected", len(rows))
	}

	// Gambar yang sama berulang tidak menambah hitungan.
	for i := range rows {
		rows[i].Image = "https://cdn.example.com/same.png"
	}
	if _, err := convertRowImages(rows); err != nil {
		t.Fatalf("repeated image must not count against the cap: %v", err)
	}
}

func TestAutoSKUPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"Sembako":             "SEM",
		"mi & makanan instan": "MIM",
		"Rokok":               "ROK",
		"AB":                  "AB",
		"123 !":               "PRD",
		"":                    "PRD",
		"  Alat Tulis":        "ALA",
	} {
		if got := autoSKUPrefix(in); got != want {
			t.Errorf("autoSKUPrefix(%q) = %q, want %q", in, got, want)
		}
	}
	if !regexp.MustCompile(`^SEM-[0-9a-f]{8}$`).MatchString(newAutoSKU("Sembako")) {
		t.Errorf("newAutoSKU = %q, want SEM-<8 hex>", newAutoSKU("Sembako"))
	}
}

func TestAssignAutoSKUs_AvoidsCollisions(t *testing.T) {
	// "DB-1" sudah dipakai produk aktif, "FILE-1" dipakai baris lain di file ini.
	repo := &fakeProdRepo{existingBySKU: map[string]*domain.Products{"DB-1": {SKU: "DB-1"}}}
	u := newTestProductUsecase(repo)
	rows := []sheet.ProductRow{{SKU: "FILE-1"}, {SKU: ""}, {SKU: ""}, {SKU: "KEEP"}}

	script := []string{"DB-1", "FILE-1", "OK-1", "OK-2", "OK-3"}
	next := 0
	generate := func(string) string { v := script[next]; next++; return v }

	if err := u.assignAutoSKUs(context.Background(), rows, generate); err != nil {
		t.Fatal(err)
	}
	// Putaran 1: DB-1 (bentrok DB) dan FILE-1 (bentrok file) ditolak; putaran 2: OK-1, OK-2.
	if rows[1].SKU != "OK-1" || rows[2].SKU != "OK-2" {
		t.Errorf("blank rows got %q and %q, want OK-1 and OK-2", rows[1].SKU, rows[2].SKU)
	}
	if rows[0].SKU != "FILE-1" || rows[3].SKU != "KEEP" {
		t.Errorf("explicit SKUs must not change: %q, %q", rows[0].SKU, rows[3].SKU)
	}
}

func TestAssignAutoSKUs_GivesUp(t *testing.T) {
	repo := &fakeProdRepo{existingBySKU: map[string]*domain.Products{"SAME": {SKU: "SAME"}}}
	u := newTestProductUsecase(repo)
	rows := []sheet.ProductRow{{SKU: ""}}
	err := u.assignAutoSKUs(context.Background(), rows, func(string) string { return "SAME" })
	if err == nil {
		t.Fatal("expected an error when every candidate collides")
	}
}

func xlsxUpload(t *testing.T, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file_upload", "products.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	w.Close()
	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(32 << 20)
	if err != nil {
		t.Fatal(err)
	}
	return form.File["file_upload"][0]
}

func gradientPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAddBulkProduct_AutoSKUBarcodeAndPicture(t *testing.T) {
	pic := gradientPNG(t, 800, 400)

	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetRow("Sheet1", "A1", &[]any{"sku", "barcode", "product_name", "unit", "purchase_price", "selling_price", "selling_price_debt", "stock", "category", "image"})
	// baris 2: hanya kolom wajib + kategori, gambar ditempel di Excel
	f.SetSheetRow("Sheet1", "A2", &[]any{"", "", "Gula Pasir", "kg", 12000, 14000, 15000, 2.5, "Sembako"})
	// baris 3: gambar berupa data URI PNG
	f.SetSheetRow("Sheet1", "A3", &[]any{"", "", "Kopi", "pcs", 1000, 1500, 1600, 5, "Minuman", "data:image/png;base64," + base64.StdEncoding.EncodeToString(pic)})
	// baris 4: URL dibiarkan, SKU eksplisit dipakai apa adanya
	f.SetSheetRow("Sheet1", "A4", &[]any{"TEH-01", "", "Teh", "pcs", 2000, 3000, 3200, 1, "Minuman", "https://cdn.example.com/teh.png"})
	// baris 5: gambar rusak -> hanya baris ini yang ditolak
	f.SetSheetRow("Sheet1", "A5", &[]any{"", "", "Rusak", "pcs", 1000, 1500, 1600, 1, "Lain", base64.StdEncoding.EncodeToString([]byte("bukan gambar sama sekali"))})
	if err := f.AddPictureFromBytes("Sheet1", "J2", &excelize.Picture{Extension: ".png", File: pic}); err != nil {
		t.Fatal(err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	repo := &fakeProdRepo{}
	u := newTestProductUsecase(repo)
	report, err := u.AddBulkProductShopWithLock(context.Background(), &requestdto.AddBulkProduct{FileUpload: xlsxUpload(t, buf.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalInserted != 3 || len(report.RowErrors) != 1 || report.RowErrors[0].Line != 5 || !strings.HasPrefix(report.RowErrors[0].Message, "image:") {
		t.Fatalf("report = %+v; want 3 inserted and one 'image:' error on line 5", report)
	}

	byName := map[string]string{}
	for _, p := range repo.upserted {
		byName[p.ProductName] = p.SKU
		switch p.ProductName {
		case "Gula Pasir", "Kopi":
			if !regexp.MustCompile(`^(SEM|MIN)-[0-9a-f]{8}$`).MatchString(p.SKU) {
				t.Errorf("%s: sku = %q, want PREFIX-<8 hex>", p.ProductName, p.SKU)
			}
			raw, err := base64.StdEncoding.DecodeString(p.Image)
			if err != nil || strings.HasPrefix(p.Image, "data:") {
				t.Fatalf("%s: image must be raw base64: %v", p.ProductName, err)
			}
			img, err := webp.Decode(bytes.NewReader(raw))
			if err != nil || img.Bounds().Dx() != img.Bounds().Dy() || len(raw) > productimg.MaxBytes {
				t.Fatalf("%s: image must be a square webp under 500 KB (err %v, %d bytes)", p.ProductName, err, len(raw))
			}
			if img.Bounds().Dx() != 256 {
				t.Errorf("%s: side = %d, want 256 (800x400 cropped to a square, then capped)", p.ProductName, img.Bounds().Dx())
			}
		case "Teh":
			if p.SKU != "TEH-01" || p.Image != "https://cdn.example.com/teh.png" {
				t.Errorf("Teh: sku/image = %q / %q, want them unchanged", p.SKU, p.Image)
			}
		}
		if p.Barcode == nil || *p.Barcode != domain.AutoBarcode(p.SKU, p.ID) {
			t.Errorf("%s: barcode = %v, want auto SKU-<id hex>", p.ProductName, p.Barcode)
		}
	}
	if _, rejected := byName["Rusak"]; rejected {
		t.Error("the row with a broken image must not be imported")
	}
}
