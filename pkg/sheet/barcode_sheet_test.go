package sheet

import (
	"bytes"
	"fmt"
	"testing"

	"shop_project_be/pkg/barcodeimg"

	"github.com/xuri/excelize/v2"
)

func TestWriteBarcodeSheet(t *testing.T) {
	codes := []string{"GULA-1a2b3c4d", "KOPI-9f8e7d6c"}
	rows := make([]BarcodeRow, 0, len(codes))
	for i, code := range codes {
		img, err := barcodeimg.PNG(code)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, BarcodeRow{Name: []string{"Gula", "Kopi"}[i], Code: code, PNG: img})
	}

	data, err := WriteBarcodeSheet(rows)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output is not a valid xlsx: %v", err)
	}
	defer f.Close()

	if got, _ := f.GetCellValue(barcodeSheet, "A3"); got != "Kopi" {
		t.Errorf("A3 = %q, want Kopi", got)
	}
	for _, cell := range []string{"B2", "B3"} {
		pics, err := f.GetPictures(barcodeSheet, cell)
		if err != nil || len(pics) != 1 {
			t.Errorf("%s pictures = %d (err %v), want 1", cell, len(pics), err)
		}
	}
}

// Satu part ekspor berisi sampai 500 produk. Kasus terburuk: semua barcode berbeda
// dan sepanjang batas 64 karakter (tidak ada gambar yang bisa dipakai ulang).
func TestWriteBarcodeSheetFullPartStaysUnder500KB(t *testing.T) {
	rows := make([]BarcodeRow, 0, 500)
	for i := 0; i < 500; i++ {
		code := fmt.Sprintf("Zz9.-_%058d", i+1)
		img, err := barcodeimg.PNG(code)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, BarcodeRow{Name: "Produk", Code: code, PNG: img})
	}
	data, err := WriteBarcodeSheet(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 500<<10 {
		t.Errorf("xlsx = %d KB, want <= 500 KB", len(data)>>10)
	}
}

func TestWriteBarcodeSheetEmpty(t *testing.T) {
	data, err := WriteBarcodeSheet(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := excelize.OpenReader(bytes.NewReader(data)); err != nil {
		t.Fatalf("empty export must still be a valid xlsx: %v", err)
	}
}
