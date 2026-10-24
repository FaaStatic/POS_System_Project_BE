package pdf

import (
	"fmt"
	"strconv"
	"time"
)

var reportDir = "storage/reports"

const urlPrefix = "/api/reports"

type MonthReportData struct {
	StoreName     string
	Cashier       string
	Month         int
	Year          int
	Daily         []MonthReportDailyRow
	ProductsSold  []MonthReportProductRow
	DailyProducts []MonthReportDailyProducts
	GeneratedAt   time.Time
}

// MonthReportDailyRow: Cost/DebtCost = total harga dasar (pengeluaran) barang
// terjual. DebtPaid hanya mengurangi saldo hutang, tidak terikat barang.
type MonthReportDailyRow struct {
	Date       time.Time
	CountSale  int64
	Revenue    int64
	Cost       int64
	CountDebt  int64
	DebtSales  int64
	DebtManual int64
	DebtPaid   int64
	DebtCost   int64
}

func sumDaily(rows []MonthReportDailyRow) MonthReportDailyRow {
	var total MonthReportDailyRow
	for _, r := range rows {
		total.CountSale += r.CountSale
		total.Revenue += r.Revenue
		total.Cost += r.Cost
		total.CountDebt += r.CountDebt
		total.DebtSales += r.DebtSales
		total.DebtManual += r.DebtManual
		total.DebtPaid += r.DebtPaid
		total.DebtCost += r.DebtCost
	}
	return total
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

	total := sumDaily(data.Daily)

	drawSummary(pdf, "Ringkasan Penjualan Non-Hutang", [][2]string{
		{"Jumlah Transaksi", strconv.FormatInt(total.CountSale, 10) + " transaksi"},
		{"Total Pendapatan", FormatRupiah(total.Revenue)},
		{"Total Harga Dasar (Pengeluaran)", FormatRupiah(total.Cost)},
		{"Pendapatan Bersih", FormatRupiah(total.Revenue - total.Cost)},
	})
	pdf.Ln(6)
	drawSummary(pdf, "Ringkasan Hutang", [][2]string{
		{"Jumlah Transaksi Hutang", strconv.FormatInt(total.CountDebt, 10) + " transaksi"},
		{"Hutang Baru (Transaksi)", FormatRupiah(total.DebtSales)},
		{"Hutang Baru (Manual)", FormatRupiah(total.DebtManual)},
		{"Hutang Dibayar", FormatRupiah(total.DebtPaid)},
		{"Total Harga Dasar Hutang (Pengeluaran)", FormatRupiah(total.DebtCost)},
		{"Pendapatan Bersih Hutang", FormatRupiah(total.DebtSales - total.DebtCost)},
	})

	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Rincian Harian Non-Hutang", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	drawSalesDailyHeader(pdf)
	if len(data.Daily) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(180, 8, "Tidak ada transaksi pada bulan ini.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "", 9)
		for _, d := range data.Daily {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawSalesDailyHeader(pdf)
				pdf.SetFont("Arial", "", 9)
			}
			drawSalesDailyRow(pdf, formatDateOnly(d.Date), d, 7)
		}
		pdf.SetFont("Arial", "B", 9)
		drawSalesDailyRow(pdf, "TOTAL", total, 8)
	}

	pdf.Ln(8)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Rincian Harian Hutang", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	drawDebtDailyHeader(pdf)
	if len(data.Daily) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(180, 8, "Tidak ada hutang pada bulan ini.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "", 8)
		for _, d := range data.Daily {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawDebtDailyHeader(pdf)
				pdf.SetFont("Arial", "", 8)
			}
			drawDebtDailyRow(pdf, formatDateOnly(d.Date), d, 7)
		}
		pdf.SetFont("Arial", "B", 8)
		drawDebtDailyRow(pdf, "TOTAL", total, 8)
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
		pdf.CellFormat(45, 8, FormatRupiah(total.Revenue+total.DebtSales), "1", 1, "R", false, 0, "")
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
