package sheet

import (
	"bytes"
	"slices"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteProductTemplate(t *testing.T) {
	data, err := WriteProductTemplate()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a valid xlsx: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 2 || sheets[0] != templateSheet || sheets[1] != templateGuideName {
		t.Fatalf("sheets = %v; the importer reads the first sheet, so products must come first", sheets)
	}
	rows, err := f.GetRows(templateSheet)
	if err != nil || len(rows) != templatePlaceholderRows+1 {
		t.Fatalf("rows = %d (err %v), want a header plus %d placeholder rows", len(rows), err, templatePlaceholderRows)
	}
	if !slices.Equal(rows[0][:len(templateHeaders)], templateHeaders) || len(rows[0]) != len(templateHeaders) {
		t.Fatalf("header = %v, want %v", rows[0], templateHeaders)
	}

	// sku, barcode, dan image berisi placeholder; kolom lain kosong dan tanpa rumus.
	for _, cell := range []string{"A2", "B2", "K2", "A1001", "B1001", "K1001"} {
		if got, _ := f.GetCellValue(templateSheet, cell); got != autoPlaceholder {
			t.Errorf("%s = %q, want the auto placeholder", cell, got)
		}
		if formula, _ := f.GetCellFormula(templateSheet, cell); formula != "" {
			t.Errorf("%s must hold plain text, not a formula", cell)
		}
	}
	for _, cell := range []string{"C2", "F2", "J2", "A1002", "K1002"} {
		if got, _ := f.GetCellValue(templateSheet, cell); got != "" {
			t.Errorf("%s = %q, want empty", cell, got)
		}
	}
	dvs, err := f.GetDataValidations(templateSheet)
	if err != nil || len(dvs) != 2 {
		t.Fatalf("data validations = %d (err %v), want unit and product_type drop lists", len(dvs), err)
	}
}

// Template yang belum diisi harus bisa dibaca importer tanpa galat: baris yang hanya
// berisi placeholder dianggap kosong, bukan "product_name is empty".
func TestProductTemplateIsAcceptedByImporter(t *testing.T) {
	data, err := WriteProductTemplate()
	if err != nil {
		t.Fatal(err)
	}
	rows, rowErrors, err := ParseProducts(bytes.NewReader(data), "template.xlsx")
	if err != nil || len(rows) != 0 || len(rowErrors) != 0 {
		t.Fatalf("rows=%d rowErrors=%v err=%v; want an empty, valid file", len(rows), rowErrors, err)
	}
}

func fillTemplate(t *testing.T, cells map[string]any) []byte {
	t.Helper()
	data, err := WriteProductTemplate()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for cell, v := range cells {
		if err := f.SetCellValue(templateSheet, cell, v); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProductTemplateFilledOnlyWithRequiredColumns(t *testing.T) {
	// Pengguna hanya mengisi kolom wajib; placeholder sku, barcode, dan image dibiarkan.
	file := fillTemplate(t, map[string]any{"C2": "Gula Pasir", "F2": 12000, "G2": 14000, "H2": 15000})
	rows, rowErrors, err := ParseProducts(bytes.NewReader(file), "filled.xlsx")
	if err != nil || len(rowErrors) != 0 || len(rows) != 1 {
		t.Fatalf("rows=%d rowErrors=%v err=%v; want 1 valid row", len(rows), rowErrors, err)
	}
	if rows[0].SKU != "" || rows[0].Barcode != "" || rows[0].Image != "" || rows[0].ImageFile != nil {
		t.Errorf("the placeholder must never be read as a value; auto columns must stay empty for the usecase: %+v", rows[0])
	}
}

func TestProductTemplateKeepsTypedValuesOverPlaceholder(t *testing.T) {
	file := fillTemplate(t, map[string]any{
		"A2": "MY-SKU-1", "C2": "Gula Pasir", "F2": 12000, "G2": 14000, "H2": 15000, // barcode tetap placeholder
		"B3": "8991234567890", "C3": "Kopi", "F3": 1000, "G3": 1500, "H3": 1600, "K3": "https://cdn.example.com/kopi.png",
	})
	rows, rowErrors, err := ParseProducts(bytes.NewReader(file), "filled.xlsx")
	if err != nil || len(rowErrors) != 0 || len(rows) != 2 {
		t.Fatalf("rows=%d rowErrors=%v err=%v", len(rows), rowErrors, err)
	}
	if rows[0].SKU != "MY-SKU-1" || rows[0].Barcode != "" {
		t.Errorf("row 2: sku=%q barcode=%q, want the typed SKU and an empty barcode", rows[0].SKU, rows[0].Barcode)
	}
	if rows[1].SKU != "" || rows[1].Barcode != "8991234567890" || rows[1].Image != "https://cdn.example.com/kopi.png" {
		t.Errorf("row 3: %+v, want the typed barcode and URL with an empty SKU", rows[1])
	}
}

func TestAutoPlaceholderMatching(t *testing.T) {
	for in, want := range map[string]bool{
		autoPlaceholder:                    true,
		"  (OTOMATIS SETELAH UPLOAD)  ":    true,
		"#VALUE!":                          true,
		"#N/A":                             false, // galat lain dibiarkan terlihat, bukan diam-diam dianggap kosong
		"(otomatis)":                       false, // hanya teks persis yang dikenali
		"SKU-1":                            false,
		"":                                 false,
		"otomatis setelah upload tanpa ()": false,
	} {
		if got := isAutoPlaceholder(in); got != want {
			t.Errorf("isAutoPlaceholder(%q) = %v, want %v", in, got, want)
		}
	}
}
