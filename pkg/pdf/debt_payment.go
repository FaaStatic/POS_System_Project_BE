package pdf

import (
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

// DebtPaymentReceiptData satu cicilan hutang. PreviousRemaining adalah sisa hutang
// sebelum cicilan ini; Remaining sisanya sesudahnya.
type DebtPaymentReceiptData struct {
	StoreName         string
	Number            string
	Customer          string
	Cashier           string
	PaidAt            time.Time
	PreviousRemaining int64
	Paid              int64
	Remaining         int64
	GeneratedAt       time.Time
}

// drawDebtHeader: nama toko, judul, dan baris periode opsional, ditutup garis.
func drawDebtHeader(pdf *fpdf.Fpdf, storeName, title, subtitle string) {
	if storeName == "" {
		storeName = "Shop Project"
	}
	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 10, storeName, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, title, "", 1, "C", false, 0, "")
	if subtitle != "" {
		pdf.SetFont("Arial", "", 11)
		pdf.CellFormat(0, 7, subtitle, "", 1, "C", false, 0, "")
	}
	pdf.Ln(2)
	drawLine(pdf)
}

func GenerateDebtPaymentReceipt(data DebtPaymentReceiptData) (string, error) {
	pdf := newDocument()
	pdf.AddPage()

	drawDebtHeader(pdf, data.StoreName, "Struk Pembayaran Hutang", "")
	pdf.Ln(4)

	pdf.SetFont("Arial", "", 10)
	info := [][2]string{
		{"No. Struk", data.Number},
		{"Tanggal", formatDateTime(data.PaidAt)},
		{"Customer", data.Customer},
		{"Kasir", data.Cashier},
	}
	for _, m := range info {
		if m[1] == "" {
			continue
		}
		pdf.CellFormat(35, 6, m[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, ": "+m[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	rows := [][2]string{
		{"Hutang Sebelum Bayar", FormatRupiah(data.PreviousRemaining)},
		{"Dibayar Sekarang", FormatRupiah(data.Paid)},
		{"Saldo Terhutang", FormatRupiah(data.Remaining)},
	}
	for i, row := range rows {
		style := ""
		if i > 0 {
			style = "B"
		}
		pdf.SetFont("Arial", style, 11)
		pdf.CellFormat(110, 8, row[0], "1", 0, "L", false, 0, "")
		pdf.CellFormat(70, 8, row[1], "1", 1, "R", false, 0, "")
	}
	pdf.Ln(6)

	status := "BELUM LUNAS"
	if data.Remaining <= 0 {
		status = "LUNAS"
	}
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "*** "+status+" ***", "", 1, "C", false, 0, "")

	pdf.Ln(8)
	pdf.SetFont("Arial", "I", 9)
	pdf.CellFormat(0, 5, "Dicetak pada "+formatDateTime(data.GeneratedAt), "", 1, "C", false, 0, "")

	return saveDocument(pdf, "struk-hutang")
}

// DebtPaymentsMonthRow satu cicilan dalam rekap bulanan semua customer.
type DebtPaymentsMonthRow struct {
	Customer          string
	Cashier           string
	PaidAt            time.Time
	PreviousRemaining int64
	Paid              int64
	Remaining         int64
}

type DebtPaymentsMonthData struct {
	StoreName   string
	Month       int
	Year        int
	Rows        []DebtPaymentsMonthRow
	GeneratedAt time.Time
}

// Lebar kolom (mm) di A4 landscape dengan margin 15: 267 total.
var debtMonthCols = []struct {
	title string
	width float64
	align string
}{
	{"Customer", 55, "L"},
	{"Kasir", 35, "L"},
	{"Tanggal Bayar", 40, "L"},
	{"Saldo Total", 37, "R"},
	{"Dibayarkan", 37, "R"},
	{"Sisa Saldo", 37, "R"},
	{"Status", 26, "C"},
}

func drawDebtMonthHeader(pdf *fpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	for i, c := range debtMonthCols {
		ln := 0
		if i == len(debtMonthCols)-1 {
			ln = 1
		}
		pdf.CellFormat(c.width, 8, c.title, "1", ln, c.align, true, 0, "")
	}
}

// GenerateDebtPaymentsMonth: satu tabel rekap cicilan hutang semua customer dalam satu bulan.
func GenerateDebtPaymentsMonth(data DebtPaymentsMonthData) (string, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	drawDebtHeader(pdf, data.StoreName, "Rekap Pembayaran Hutang Customer", fmt.Sprintf("Periode: %s %d", monthNameID(data.Month), data.Year))
	pdf.Ln(4)

	drawDebtMonthHeader(pdf)
	if len(data.Rows) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(267, 8, "Belum ada pembayaran hutang pada bulan ini.", "1", 1, "C", false, 0, "")
	}
	var totalPaid int64
	pdf.SetFont("Arial", "", 9)
	for _, r := range data.Rows {
		if pdf.GetY() > 185 {
			pdf.AddPage()
			drawDebtMonthHeader(pdf)
			pdf.SetFont("Arial", "", 9)
		}
		status := "Belum Lunas"
		if r.Remaining <= 0 {
			status = "Lunas"
		}
		cells := []string{
			truncate(r.Customer, 32),
			truncate(r.Cashier, 20),
			formatDateTime(r.PaidAt),
			FormatRupiah(r.PreviousRemaining),
			FormatRupiah(r.Paid),
			FormatRupiah(r.Remaining),
			status,
		}
		for i, c := range debtMonthCols {
			ln := 0
			if i == len(debtMonthCols)-1 {
				ln = 1
			}
			pdf.CellFormat(c.width, 7, cells[i], "1", ln, c.align, false, 0, "")
		}
		totalPaid += r.Paid
	}
	if len(data.Rows) > 0 {
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(167, 8, "TOTAL DIBAYARKAN", "1", 0, "R", false, 0, "")
		pdf.CellFormat(37, 8, FormatRupiah(totalPaid), "1", 0, "R", false, 0, "")
		pdf.CellFormat(63, 8, fmt.Sprintf("%d pembayaran", len(data.Rows)), "1", 1, "C", false, 0, "")
	}

	pdf.Ln(6)
	pdf.SetFont("Arial", "I", 9)
	pdf.CellFormat(0, 5, "Dicetak pada "+formatDateTime(data.GeneratedAt), "", 1, "C", false, 0, "")

	return saveDocument(pdf, fmt.Sprintf("rekap-hutang-%d-%02d", data.Year, data.Month))
}
