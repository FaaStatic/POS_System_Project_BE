package sheet

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

const (
	templateSheet     = "products"
	templateGuideName = "Petunjuk"
)

// autoPlaceholder diisi di sel sku, barcode, dan image pada template sebagai petunjuk
// bahwa kolom itu dibuat otomatis oleh backend setelah file diunggah. Excel tidak
// punya placeholder sungguhan untuk sel, jadi ini teks biasa; parser menganggapnya
// kosong (lihat isAutoPlaceholder), sehingga tidak pernah tersimpan sebagai SKU,
// barcode, atau gambar.
const autoPlaceholder = "(otomatis setelah upload)"

// templatePlaceholderRows: jumlah baris template yang sel otomatisnya sudah berisi placeholder.
const templatePlaceholderRows = 1000

// templateHeaders harus mengikuti kolom yang dibaca parseRecords.
var templateHeaders = []string{
	"sku", "barcode", "product_name", "unit", "product_type",
	"purchase_price", "selling_price", "selling_price_debt", "stock", "category", "image",
}

// autoColumns: kolom yang diisi otomatis saat import; dipakai untuk warna header dan
// pemasangan placeholder.
var autoColumns = map[string]bool{"sku": true, "barcode": true, "image": true}

var templateGuide = [][3]string{
	{"Kolom", "Wajib?", "Keterangan"},
	{"sku", "Otomatis", "Biarkan teks abu-abu '(otomatis setelah upload)'. Setelah file diunggah, sistem membuat SKU: 3 huruf pertama kategori + '-' + 8 karakter acak, mis. SEM-1a2b3c4d (PRD bila kategori kosong). Isi sendiri bila ingin SKU tertentu; SKU yang sudah terdaftar memperbarui produk itu dan stoknya ditambahkan."},
	{"barcode", "Otomatis", "Biarkan teks abu-abu '(otomatis setelah upload)'. Setelah file diunggah, sistem membuat barcode: SKU + '-' + 8 karakter dari ID produk, beserta gambar barcodenya. Isi sendiri bila memakai barcode pabrik. Produk lama yang sudah punya barcode tidak diubah bila sel dibiarkan."},
	{"product_name", "Wajib", "Nama produk."},
	{"unit", "Boleh kosong", "pcs, gram, kg, liter, kardus, atau ikat (pilih dari daftar). Kosong = pcs. Hanya pcs yang harus bilangan bulat; satuan lain boleh pecahan sampai 3 desimal (mis. 0,5 kardus, 2,125 kg)."},
	{"product_type", "Boleh kosong", "physical atau digital (pilih dari daftar). Kosong = physical."},
	{"purchase_price", "Wajib", "Harga beli dalam rupiah, lebih dari 0."},
	{"selling_price", "Wajib", "Harga jual dalam rupiah, harus lebih besar dari harga beli."},
	{"selling_price_debt", "Wajib", "Harga jual untuk hutang, minimal 5% di atas harga jual."},
	{"stock", "Boleh kosong", "Jumlah stok. Kosong = 0. Pcs bulat; satuan lain maksimal 3 desimal."},
	{"category", "Boleh kosong", "Nama kategori. Kategori yang belum ada dibuat otomatis."},
	{"image", "Otomatis", "Tidak perlu diisi. Bila ingin ada gambar, masukkan gambar pada sel kolom ini di baris produknya: Insert > Pictures (boleh 'Place in Cell' maupun 'Place Over Cells'). Gambar TIDAK berubah menjadi teks di dalam Excel; setelah file diunggah, sistem mengubahnya menjadi WebP persegi 1:1 di bawah 500 KB dan menyimpannya sebagai base64. Boleh juga mengisi URL (http/https). Format gambar: PNG, JPEG, GIF, WebP; hanya terbaca dari file .xlsx."},
	{"", "", ""},
	{"Batas", "", "Maksimal 10.000 baris dan 25 MB per file. Baris yang gagal dilaporkan per baris; baris lain tetap masuk."},
}

// WriteProductTemplate membuat template import produk (.xlsx). Sheet pertama
// ("products") adalah yang dibaca importer, jadi petunjuk ada di sheet kedua.
func WriteProductTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", templateSheet); err != nil {
		return nil, fmt.Errorf("rename sheet: %w", err)
	}

	required, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#DDDDDD"}},
	})
	if err != nil {
		return nil, fmt.Errorf("create style: %w", err)
	}
	auto, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#C6EFCE"}},
	})
	if err != nil {
		return nil, fmt.Errorf("create style: %w", err)
	}
	placeholder, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Color: "#999999"}})
	if err != nil {
		return nil, fmt.Errorf("create style: %w", err)
	}

	for i, h := range templateHeaders {
		col, _ := excelize.ColumnNumberToName(i + 1)
		cell := col + "1"
		style := required
		if autoColumns[h] {
			style = auto
		}
		if err := f.SetCellValue(templateSheet, cell, h); err != nil {
			return nil, fmt.Errorf("write header: %w", err)
		}
		if err := f.SetCellStyle(templateSheet, cell, cell, style); err != nil {
			return nil, fmt.Errorf("style header: %w", err)
		}
		width := 16.0
		switch h {
		case "product_name":
			width = 40
		case "sku", "barcode", "image":
			width = 26
		}
		if err := f.SetColWidth(templateSheet, col, col, width); err != nil {
			return nil, fmt.Errorf("set column width: %w", err)
		}

		if !autoColumns[h] {
			continue
		}
		for row := 2; row <= templatePlaceholderRows+1; row++ {
			if err := f.SetCellValue(templateSheet, fmt.Sprintf("%s%d", col, row), autoPlaceholder); err != nil {
				return nil, fmt.Errorf("write placeholder: %w", err)
			}
		}
		if err := f.SetCellStyle(templateSheet, col+"2", fmt.Sprintf("%s%d", col, templatePlaceholderRows+1), placeholder); err != nil {
			return nil, fmt.Errorf("style placeholder: %w", err)
		}
	}
	if err := f.SetPanes(templateSheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, fmt.Errorf("freeze header: %w", err)
	}

	lastRow := maxImportRows + 1
	for _, list := range []struct {
		column string
		items  []string
	}{
		{"D", []string{"pcs", "gram", "kg", "liter", "kardus", "ikat"}},
		{"E", []string{"physical", "digital"}},
	} {
		dv := excelize.NewDataValidation(true)
		dv.Sqref = fmt.Sprintf("%s2:%s%d", list.column, list.column, lastRow)
		if err := dv.SetDropList(list.items); err != nil {
			return nil, fmt.Errorf("drop list: %w", err)
		}
		if err := f.AddDataValidation(templateSheet, dv); err != nil {
			return nil, fmt.Errorf("add validation: %w", err)
		}
	}

	if _, err := f.NewSheet(templateGuideName); err != nil {
		return nil, fmt.Errorf("add guide sheet: %w", err)
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	if err != nil {
		return nil, fmt.Errorf("create style: %w", err)
	}
	for i, row := range templateGuide {
		for j, text := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellValue(templateGuideName, cell, text); err != nil {
				return nil, fmt.Errorf("write guide: %w", err)
			}
		}
	}
	end := fmt.Sprintf("C%d", len(templateGuide))
	if err := f.SetCellStyle(templateGuideName, "A1", end, wrap); err != nil {
		return nil, fmt.Errorf("style guide: %w", err)
	}
	if err := f.SetCellStyle(templateGuideName, "A1", "C1", required); err != nil {
		return nil, fmt.Errorf("style guide header: %w", err)
	}
	for col, width := range map[string]float64{"A": 20, "B": 14, "C": 90} {
		if err := f.SetColWidth(templateGuideName, col, col, width); err != nil {
			return nil, fmt.Errorf("set column width: %w", err)
		}
	}
	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}
	return buf.Bytes(), nil
}
