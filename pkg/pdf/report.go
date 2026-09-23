package pdf

import (
	"fmt"
	"strconv"
	"time"
)

var reportDir = "storage/reports"

const urlPrefix = "/api/reports"

type MonthReportData struct {
	StoreName        string
	Cashier          string
	Month            int
	Year             int
	TotalTransaction int64
	TotalRevenue     int64
	TotalDebt        int64
	GrandTotal       int64
	Daily            []MonthReportDailyRow
	ProductsSold     []MonthReportProductRow
	DailyProducts    []MonthReportDailyProducts
	GeneratedAt      time.Time
}

type MonthReportDailyRow struct {
	Date             time.Time
	TotalTransaction int64
	Revenue          int64
	Debt             int64
	Total            int64
}

type MonthReportProductRow struct {
	ProductName string
	Qty         float64
	Total       int64
}

type MonthReportDailyProducts struct {
	Date     time.Time
	Products []MonthReportProductRow
}

type TransactionReportItem struct {
	ProductName string
	Qty         float64
	Price       int64
	Subtotal    int64
}

type TransactionReportData struct {
	StoreName   string
	NoInvoice   string
	Cashier     string
	Customer    string
	PaymentType string
	CreatedAt   time.Time
	Items       []TransactionReportItem
	Total       int64
	GeneratedAt time.Time
}

func GenerateMonthReport(data MonthReportData) (string, error) {
	pdf := newDocument()
	pdf.AddPage()

	storeName := data.StoreName
	if storeName == "" {
		storeName = "Shop Project"
	}

	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 10, storeName, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, "Laporan Bulanan Transaksi", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("Periode: %s %d", monthNameID(data.Month), data.Year), "", 1, "C", false, 0, "")
	pdf.Ln(2)
	drawLine(pdf)
	pdf.Ln(4)

	pdf.SetFont("Arial", "", 10)
	if data.Cashier != "" {
		pdf.CellFormat(0, 6, "Dibuat oleh : "+data.Cashier, "", 1, "L", false, 0, "")
	}
	pdf.CellFormat(0, 6, "Dicetak pada : "+formatDateTime(data.GeneratedAt), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Ringkasan", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	rows := [][2]string{
		{"Jumlah Transaksi", strconv.FormatInt(data.TotalTransaction, 10) + " transaksi"},
		{"Total Pendapatan (Lunas)", FormatRupiah(data.TotalRevenue)},
		{"Total Hutang (Belum Lunas)", FormatRupiah(data.TotalDebt)},
		{"Total Nilai Transaksi", FormatRupiah(data.GrandTotal)},
	}
	for i, row := range rows {
		if i == len(rows)-1 {
			pdf.SetFont("Arial", "B", 11)
		} else {
			pdf.SetFont("Arial", "", 11)
		}
		pdf.CellFormat(110, 8, row[0], "1", 0, "L", false, 0, "")
		pdf.CellFormat(70, 8, row[1], "1", 1, "R", false, 0, "")
	}

	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Rincian Harian", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	drawDailyHeader(pdf)
	if len(data.Daily) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(180, 8, "Tidak ada transaksi pada bulan ini.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "", 9)
		for _, d := range data.Daily {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawDailyHeader(pdf)
				pdf.SetFont("Arial", "", 9)
			}
			pdf.CellFormat(30, 7, formatDateOnly(d.Date), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 7, strconv.FormatInt(d.TotalTransaction, 10), "1", 0, "C", false, 0, "")
			pdf.CellFormat(45, 7, FormatRupiah(d.Revenue), "1", 0, "R", false, 0, "")
			pdf.CellFormat(35, 7, FormatRupiah(d.Debt), "1", 0, "R", false, 0, "")
			pdf.CellFormat(45, 7, FormatRupiah(d.Total), "1", 1, "R", false, 0, "")
		}
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(30, 8, "TOTAL", "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 8, strconv.FormatInt(data.TotalTransaction, 10), "1", 0, "C", false, 0, "")
		pdf.CellFormat(45, 8, FormatRupiah(data.TotalRevenue), "1", 0, "R", false, 0, "")
		pdf.CellFormat(35, 8, FormatRupiah(data.TotalDebt), "1", 0, "R", false, 0, "")
		pdf.CellFormat(45, 8, FormatRupiah(data.GrandTotal), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Daftar Barang Terjual (Rekap Bulan)", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	drawProductHeader(pdf)
	if len(data.ProductsSold) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(180, 8, "Tidak ada barang terjual pada bulan ini.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "", 9)
		var totalQty float64
		for _, p := range data.ProductsSold {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawProductHeader(pdf)
				pdf.SetFont("Arial", "", 9)
			}
			pdf.CellFormat(100, 7, truncate(p.ProductName, 55), "1", 0, "L", false, 0, "")
			pdf.CellFormat(35, 7, formatQty(p.Qty), "1", 0, "C", false, 0, "")
			pdf.CellFormat(45, 7, FormatRupiah(p.Total), "1", 1, "R", false, 0, "")
			totalQty += p.Qty
		}
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(100, 8, "TOTAL", "1", 0, "L", false, 0, "")
		pdf.CellFormat(35, 8, formatQty(totalQty), "1", 0, "C", false, 0, "")
		pdf.CellFormat(45, 8, FormatRupiah(data.TotalRevenue+data.TotalDebt), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Rincian Barang Terjual per Hari", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	if len(data.DailyProducts) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(0, 6, "Tidak ada barang terjual pada bulan ini.", "", 1, "L", false, 0, "")
	} else {
		for _, day := range data.DailyProducts {
			if pdf.GetY() > 260 {
				pdf.AddPage()
			}
			pdf.SetFont("Arial", "B", 10)
			pdf.SetFillColor(245, 245, 245)
			pdf.CellFormat(180, 7, formatDateOnly(day.Date), "1", 1, "L", true, 0, "")
			drawProductHeader(pdf)
			pdf.SetFont("Arial", "", 9)
			for _, p := range day.Products {
				if pdf.GetY() > 270 {
					pdf.AddPage()
					drawProductHeader(pdf)
					pdf.SetFont("Arial", "", 9)
				}
				pdf.CellFormat(100, 7, truncate(p.ProductName, 55), "1", 0, "L", false, 0, "")
				pdf.CellFormat(35, 7, formatQty(p.Qty), "1", 0, "C", false, 0, "")
				pdf.CellFormat(45, 7, FormatRupiah(p.Total), "1", 1, "R", false, 0, "")
			}
			pdf.Ln(3)
		}
	}

	pdf.Ln(8)
	pdf.SetFont("Arial", "I", 9)
	pdf.CellFormat(0, 5, "Dokumen ini dibuat otomatis oleh sistem.", "", 1, "C", false, 0, "")

	return saveDocument(pdf, fmt.Sprintf("laporan-bulanan-%04d-%02d", data.Year, data.Month))
}

func GenerateTransactionReport(data TransactionReportData) (string, error) {
	pdf := newDocument()
	pdf.AddPage()

	storeName := data.StoreName
	if storeName == "" {
		storeName = "Shop Project"
	}

	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 10, storeName, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, "Struk Transaksi", "", 1, "C", false, 0, "")
	pdf.Ln(2)
	drawLine(pdf)
	pdf.Ln(4)

	pdf.SetFont("Arial", "", 10)
	metaInfo := [][2]string{
		{"No. Invoice", data.NoInvoice},
		{"Tanggal", formatDateTime(data.CreatedAt)},
		{"Kasir", data.Cashier},
		{"Pembayaran", titleCase(data.PaymentType)},
	}
	if data.Customer != "" {
		metaInfo = append(metaInfo, [2]string{"Customer", data.Customer})
	}
	for _, m := range metaInfo {
		if m[1] == "" {
			continue
		}
		pdf.CellFormat(40, 6, m[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, ": "+m[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(70, 8, "Produk", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 8, "Qty", "1", 0, "C", true, 0, "")
	pdf.CellFormat(40, 8, "Harga", "1", 0, "R", true, 0, "")
	pdf.CellFormat(45, 8, "Subtotal", "1", 1, "R", true, 0, "")

	pdf.SetFont("Arial", "", 10)
	for _, item := range data.Items {
		pdf.CellFormat(70, 8, truncate(item.ProductName, 40), "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 8, formatQty(item.Qty), "1", 0, "C", false, 0, "")
		pdf.CellFormat(40, 8, FormatRupiah(item.Price), "1", 0, "R", false, 0, "")
		pdf.CellFormat(45, 8, FormatRupiah(item.Subtotal), "1", 1, "R", false, 0, "")
	}

	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(135, 9, "TOTAL", "1", 0, "R", false, 0, "")
	pdf.CellFormat(45, 9, FormatRupiah(data.Total), "1", 1, "R", false, 0, "")

	pdf.Ln(12)
	pdf.SetFont("Arial", "I", 10)
	pdf.CellFormat(0, 5, "Terima kasih atas kunjungan Anda.", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "I", 8)
	pdf.CellFormat(0, 5, "Dicetak pada "+formatDateTime(data.GeneratedAt), "", 1, "C", false, 0, "")

	return saveDocument(pdf, "struk-"+sanitizeFilename(data.NoInvoice))
}
