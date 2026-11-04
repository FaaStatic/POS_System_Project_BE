package sheet

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseProductsCSV(t *testing.T) {
	csvData := `sku,product_name,unit,purchase_price,selling_price,selling_price_debt,stock,category
SKU-1,Beras 5kg,kg,60000,65000,67000,10,sembako
SKU-2,Minyak 1L,2,15000,18000,19000,20,sembako
,No SKU,pcs,1000,2000,2100,5,lain
SKU-4,Bad Price,pcs,abc,2000,2100,5,lain

SKU-5,Gula 1kg,,10000,12000,12500,,sembako`

	rows, rowErrors, err := ParseProducts(strings.NewReader(csvData), "products.csv")
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}

	// Baris tanpa SKU valid: SKU-nya dibuat otomatis oleh usecase, bukan ditolak parser.
	if len(rows) != 4 {
		t.Fatalf("expected 4 valid rows, got %d (%+v)", len(rows), rows)
	}
	if len(rowErrors) != 1 || rowErrors[0].Line != 5 {
		t.Fatalf("expected 1 row error (line 5, bad price), got %d (%+v)", len(rowErrors), rowErrors)
	}

	if rows[0].SKU != "SKU-1" || rows[0].ProductName != "Beras 5kg" || rows[0].SellingPrice != 65000 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[2].SKU != "" || rows[2].ProductName != "No SKU" {
		t.Fatalf("blank-sku row must be kept with an empty SKU: %+v", rows[2])
	}
	if rows[3].SKU != "SKU-5" || rows[3].Stock != 0 {
		t.Fatalf("unexpected last row: %+v", rows[3])
	}
}

// withInCellPicture menyisipkan gambar "Place in Cell" ke sel baris 2 (metadata + richData
// + relasi gambar) dengan struktur yang ditulis Excel 365; excelize hanya bisa
// menulis gambar "di atas sel", jadi file ini dirakit manual.
func withInCellPicture(t *testing.T, xlsx, pic []byte, cell string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "xl/worksheets/sheet1.xml" {
			re := regexp.MustCompile(`(<row r="2"[^>]*>.*?)(</row>)`)
			data = re.ReplaceAll(data, []byte(`${1}<c r="`+cell+`" t="e" vm="1"><v>#VALUE!</v></c>${2}`))
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	for name, body := range map[string]string{
		"xl/metadata.xml":                         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><metadata xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:xlrd="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata"><metadataTypes count="1"><metadataType name="XLRICHVALUE" minSupportedVersion="120000"/></metadataTypes><futureMetadata name="XLRICHVALUE" count="1"><bk><extLst><ext uri="{3e2802c4-a4d2-4d8b-9148-e3be6c30e623}"><xlrd:rvb i="0"/></ext></extLst></bk></futureMetadata><valueMetadata count="1"><bk><rc t="1" v="0"/></bk></valueMetadata></metadata>`,
		"xl/richData/rdrichvalue.xml":             `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><rvData xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata" count="1"><rv s="0"><v>0</v><v>5</v></rv></rvData>`,
		"xl/richData/rdrichvaluestructure.xml":    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><rvStructures xmlns="http://schemas.microsoft.com/office/spreadsheetml/2017/richdata" count="1"><s t="_localImage"><k n="_rvRel:LocalImageIdentifier" t="i"/><k n="CalcOrigin" t="i"/></s></rvStructures>`,
		"xl/richData/richValueRel.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><richValueRels xmlns="http://schemas.microsoft.com/office/spreadsheetml/2022/richvaluerel" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><rel r:id="rId1"/></richValueRels>`,
		"xl/richData/_rels/richValueRel.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image9.png"/></Relationships>`,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	w, _ := zw.Create("xl/media/image9.png")
	w.Write(pic)
	zw.Close()
	return out.Bytes()
}

func TestParseProductsXLSXPictureInCell(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetRow("Sheet1", "A1", &[]any{"product_name", "purchase_price", "selling_price", "selling_price_debt", "stock", "category", "image"})
	f.SetSheetRow("Sheet1", "A2", &[]any{"Gula", 10000, 12000, 12600, 3, "Sembako"})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	file := withInCellPicture(t, buf.Bytes(), pngData.Bytes(), "G2") // G = kolom image di sheet ini
	rows, rowErrors, err := ParseProducts(bytes.NewReader(file), "products.xlsx")
	if err != nil || len(rowErrors) != 0 || len(rows) != 1 {
		t.Fatalf("rows=%d rowErrors=%v err=%v", len(rows), rowErrors, err)
	}
	if !bytes.Equal(rows[0].ImageFile, pngData.Bytes()) {
		t.Errorf("a Place-in-Cell picture must be read like a floating one, got %d bytes", len(rows[0].ImageFile))
	}
	if rows[0].Image != "" {
		t.Errorf("the #VALUE! Excel keeps in a Place-in-Cell cell must not become the image text, got %q", rows[0].Image)
	}
}

func TestParseProductsXLSXPictures(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetRow("Sheet1", "A1", &[]any{"product_name", "purchase_price", "selling_price", "selling_price_debt", "image"})
	f.SetSheetRow("Sheet1", "A2", &[]any{"Gula", 10000, 12000, 12600})
	f.SetSheetRow("Sheet1", "A3", &[]any{"Kopi", 1000, 1500, 1600, "https://cdn.example.com/kopi.png"})
	// baris 4 sengaja kosong, gambar di baris 5 tidak boleh bergeser ke baris 3
	f.SetSheetRow("Sheet1", "A5", &[]any{"Teh", 2000, 3000, 3200})

	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	add := func(cell string) {
		t.Helper()
		if err := f.AddPictureFromBytes("Sheet1", cell, &excelize.Picture{Extension: ".png", File: pngData.Bytes()}); err != nil {
			t.Fatal(err)
		}
	}
	add("E2") // kolom image, baris Gula
	add("E5") // kolom image, baris Teh
	add("A3") // di kolom lain: harus diabaikan
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	rows, rowErrors, err := ParseProducts(buf, "products.xlsx")
	if err != nil || len(rowErrors) != 0 {
		t.Fatalf("err = %v, rowErrors = %+v", err, rowErrors)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	got := map[string][]byte{}
	for _, r := range rows {
		got[r.ProductName] = r.ImageFile
	}
	if !bytes.Equal(got["Gula"], pngData.Bytes()) || !bytes.Equal(got["Teh"], pngData.Bytes()) {
		t.Errorf("Gula and Teh must carry the picture anchored on their own row")
	}
	if got["Kopi"] != nil {
		t.Errorf("Kopi has no picture on its row (only a picture in column A and a URL text), got %d bytes", len(got["Kopi"]))
	}
	if rows[1].Image != "https://cdn.example.com/kopi.png" {
		t.Errorf("text in the image cell must still be read, got %q", rows[1].Image)
	}
}

func TestParseProducts_ProductTypeColumn(t *testing.T) {
	csvData := "sku,product_name,unit,purchase_price,selling_price,selling_price_debt,stock,category,product_type\n" +
		"SKU-D,Pulsa 10k,pcs,10000,11000,11000,0,pulsa,digital\n" +
		"SKU-P,Beras,kg,60000,65000,67000,10,sembako,\n"
	rows, _, err := ParseProducts(strings.NewReader(csvData), "products.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].ProductType != "digital" {
		t.Fatalf("want digital, got %q", rows[0].ProductType)
	}
	if rows[1].ProductType != "" {
		t.Fatalf("want empty (defaults later), got %q", rows[1].ProductType)
	}
}

func TestParseProductsRejectsNonFiniteNumbers(t *testing.T) {
	csvData := `sku,product_name,purchase_price,selling_price,selling_price_debt,stock
A,Nan stock,1000,2000,2100,NaN
B,Inf stock,1000,2000,2100,Inf
C,Inf price,Inf,2000,2100,1
D,Ok,1000,2000,2100,1`

	rows, rowErrors, err := ParseProducts(strings.NewReader(csvData), "p.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SKU != "D" || len(rowErrors) != 3 {
		t.Fatalf("rows = %+v, errors = %+v; want only D accepted and 3 row errors", rows, rowErrors)
	}
}

func TestParseProductsMissingHeader(t *testing.T) {
	csvData := "sku,product_name,purchase_price\nSKU-1,Beras,60000"
	_, _, err := ParseProducts(strings.NewReader(csvData), "p.csv")
	if err == nil {
		t.Fatal("expected error for missing required header, got nil")
	}
}

func TestParseProductsRejectsTooManyRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("sku,product_name,purchase_price,selling_price,selling_price_debt\n")
	for i := 0; i <= maxImportRows; i++ {
		fmt.Fprintf(&b, "SKU-%d,P,1,2,3\n", i)
	}
	if _, _, err := ParseProducts(strings.NewReader(b.String()), "p.csv"); err == nil {
		t.Fatalf("expected an error for more than %d data rows", maxImportRows)
	}
}

func TestParseProductsUnsupportedFormat(t *testing.T) {
	_, _, err := ParseProducts(strings.NewReader("x"), "p.txt")
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
}
