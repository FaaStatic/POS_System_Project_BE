package sheet

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/xuri/excelize/v2"
)

type BarcodeRow struct {
	Name string
	Code string
	PNG  []byte
}

const (
	barcodeSheet    = "Barcode"
	barcodeRowPt    = 40.0  // tinggi baris (point), muat gambar skala 0.4
	barcodeMaxScale = 0.4   // skala gambar: ~250px lebar untuk SKU pendek, cocok ditempel di barang
	barcodeMaxWidth = 260.0 // lebar maksimum gambar (px); kode panjang diperkecil dengan rasio tetap
)

// WriteBarcodeSheet menulis satu baris per produk: nama dan gambar barcode (PNG,
// karena Excel tidak menampilkan WebP). Skala X dan Y selalu sama supaya batang
// tidak melar dan tetap terbaca scanner setelah dicetak.
func WriteBarcodeSheet(rows []BarcodeRow) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", barcodeSheet); err != nil {
		return nil, fmt.Errorf("rename sheet: %w", err)
	}
	if err := f.SetColWidth(barcodeSheet, "A", "A", 34); err != nil {
		return nil, fmt.Errorf("set column width: %w", err)
	}
	if err := f.SetColWidth(barcodeSheet, "B", "B", 38); err != nil {
		return nil, fmt.Errorf("set column width: %w", err)
	}
	if err := f.SetSheetRow(barcodeSheet, "A1", &[]any{"Nama Produk", "Barcode"}); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	nameStyle, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, fmt.Errorf("create style: %w", err)
	}

	for i, row := range rows {
		line := i + 2
		nameCell := fmt.Sprintf("A%d", line)
		if err := f.SetCellValue(barcodeSheet, nameCell, row.Name); err != nil {
			return nil, fmt.Errorf("write name: %w", err)
		}
		if err := f.SetCellStyle(barcodeSheet, nameCell, nameCell, nameStyle); err != nil {
			return nil, fmt.Errorf("style name: %w", err)
		}
		if err := f.SetRowHeight(barcodeSheet, line, barcodeRowPt); err != nil {
			return nil, fmt.Errorf("set row height: %w", err)
		}

		cfg, err := png.DecodeConfig(bytes.NewReader(row.PNG))
		if err != nil {
			return nil, fmt.Errorf("read barcode png for %s: %w", row.Code, err)
		}
		scale := min(barcodeMaxScale, barcodeMaxWidth/float64(cfg.Width))
		err = f.AddPictureFromBytes(barcodeSheet, fmt.Sprintf("B%d", line), &excelize.Picture{
			Extension: ".png",
			File:      row.PNG,
			Format: &excelize.GraphicOptions{
				AltText:     row.Code,
				ScaleX:      scale,
				ScaleY:      scale,
				OffsetX:     4,
				OffsetY:     4,
				Positioning: "oneCell",
			},
		})
		if err != nil {
			return nil, fmt.Errorf("add barcode picture for %s: %w", row.Code, err)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}
	return buf.Bytes(), nil
}
