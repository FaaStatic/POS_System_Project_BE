package sheet

import (
	"fmt"
	"strings"
	"testing"
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

	if len(rows) != 3 {
		t.Fatalf("expected 3 valid rows, got %d (%+v)", len(rows), rows)
	}
	if len(rowErrors) != 2 {
		t.Fatalf("expected 2 row errors, got %d (%+v)", len(rowErrors), rowErrors)
	}

	if rows[0].SKU != "SKU-1" || rows[0].ProductName != "Beras 5kg" || rows[0].SellingPrice != 65000 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[2].SKU != "SKU-5" || rows[2].Stock != 0 {
		t.Fatalf("unexpected last row: %+v", rows[2])
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
