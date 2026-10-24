package pdf

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

func newDocument() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	return pdf
}

const reportTTL = time.Hour

func saveDocument(pdf *fpdf.Fpdf, prefix string) (string, error) {
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create report folder: %w", err)
	}
	filename := prefix + "-" + strings.ToLower(rand.Text()) + ".pdf"
	if err := pdf.OutputFileAndClose(filepath.Join(reportDir, filename)); err != nil {
		return "", fmt.Errorf("failed to save pdf file: %w", err)
	}
	purgeExpiredReports()
	return urlPrefix + "/" + filename, nil
}

func purgeExpiredReports() {
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-reportTTL)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(reportDir, e.Name()))
		}
	}
}

func ReportPath(name string) (string, bool) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".pdf") {
		return "", false
	}
	path := filepath.Join(reportDir, name)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) > reportTTL {
		return "", false
	}
	return path, true
}

func drawLine(pdf *fpdf.Fpdf) {
	x := pdf.GetX()
	y := pdf.GetY()
	pdf.Line(x, y, 195, y)
}

// drawSummary menggambar tabel label-nilai; baris terakhir ditebalkan.
func drawSummary(pdf *fpdf.Fpdf, title string, rows [][2]string) {
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	pdf.Ln(1)
	for i, row := range rows {
		if i == len(rows)-1 {
			pdf.SetFont("Arial", "B", 11)
		} else {
			pdf.SetFont("Arial", "", 11)
		}
		pdf.CellFormat(110, 8, row[0], "1", 0, "L", false, 0, "")
		pdf.CellFormat(70, 8, row[1], "1", 1, "R", false, 0, "")
	}
}

func drawSalesDailyHeader(pdf *fpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(30, 8, "Tanggal", "1", 0, "L", true, 0, "")
	pdf.CellFormat(20, 8, "Transaksi", "1", 0, "C", true, 0, "")
	pdf.CellFormat(45, 8, "Pendapatan", "1", 0, "R", true, 0, "")
	pdf.CellFormat(40, 8, "Harga Dasar", "1", 0, "R", true, 0, "")
	pdf.CellFormat(45, 8, "Pendapatan Bersih", "1", 1, "R", true, 0, "")
}

func drawSalesDailyRow(pdf *fpdf.Fpdf, label string, r MonthReportDailyRow, h float64) {
	pdf.CellFormat(30, h, label, "1", 0, "L", false, 0, "")
	pdf.CellFormat(20, h, strconv.FormatInt(r.CountSale, 10), "1", 0, "C", false, 0, "")
	pdf.CellFormat(45, h, FormatRupiah(r.Revenue), "1", 0, "R", false, 0, "")
	pdf.CellFormat(40, h, FormatRupiah(r.Cost), "1", 0, "R", false, 0, "")
	pdf.CellFormat(45, h, FormatRupiah(r.Revenue-r.Cost), "1", 1, "R", false, 0, "")
}

func drawDebtDailyHeader(pdf *fpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(26, 8, "Tanggal", "1", 0, "L", true, 0, "")
	pdf.CellFormat(14, 8, "Trx", "1", 0, "C", true, 0, "")
	pdf.CellFormat(28, 8, "Hutang Baru", "1", 0, "R", true, 0, "")
	pdf.CellFormat(27, 8, "Manual", "1", 0, "R", true, 0, "")
	pdf.CellFormat(28, 8, "Dibayar", "1", 0, "R", true, 0, "")
	pdf.CellFormat(28, 8, "Harga Dasar", "1", 0, "R", true, 0, "")
	pdf.CellFormat(29, 8, "Pend. Bersih", "1", 1, "R", true, 0, "")
}

func drawDebtDailyRow(pdf *fpdf.Fpdf, label string, r MonthReportDailyRow, h float64) {
	pdf.CellFormat(26, h, label, "1", 0, "L", false, 0, "")
	pdf.CellFormat(14, h, strconv.FormatInt(r.CountDebt, 10), "1", 0, "C", false, 0, "")
	pdf.CellFormat(28, h, FormatRupiah(r.DebtSales), "1", 0, "R", false, 0, "")
	pdf.CellFormat(27, h, FormatRupiah(r.DebtManual), "1", 0, "R", false, 0, "")
	pdf.CellFormat(28, h, FormatRupiah(r.DebtPaid), "1", 0, "R", false, 0, "")
	pdf.CellFormat(28, h, FormatRupiah(r.DebtCost), "1", 0, "R", false, 0, "")
	pdf.CellFormat(29, h, FormatRupiah(r.DebtSales-r.DebtCost), "1", 1, "R", false, 0, "")
}

func FormatRupiah(v int64) string {
	n := v
	negative := n < 0
	if negative {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)

	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	result := "Rp " + b.String()
	if negative {
		result = "-" + result
	}
	return result
}

func formatQty(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return fmt.Sprintf("%02d %s %d %02d:%02d",
		t.Day(), monthNameShortID(int(t.Month())), t.Year(), t.Hour(), t.Minute())
}

func drawProductHeader(pdf *fpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(100, 8, "Produk", "1", 0, "L", true, 0, "")
	pdf.CellFormat(35, 8, "Qty", "1", 0, "C", true, 0, "")
	pdf.CellFormat(45, 8, "Total", "1", 1, "R", true, 0, "")
}

func drawDebtPaymentHeader(pdf *fpdf.Fpdf) {
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(50, 8, "Tanggal", "1", 0, "L", true, 0, "")
	pdf.CellFormat(80, 8, "Kasir", "1", 0, "L", true, 0, "")
	pdf.CellFormat(50, 8, "Nominal", "1", 1, "R", true, 0, "")
}

func formatDateOnly(t time.Time) string {
	return fmt.Sprintf("%02d %s %d", t.Day(), monthNameShortID(int(t.Month())), t.Year())
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

func sanitizeFilename(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.TrimSpace(s))
	if cleaned == "" {
		return "transaksi"
	}
	return cleaned
}

var monthNamesID = [...]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

var monthNamesShortID = [...]string{
	"Jan", "Feb", "Mar", "Apr", "Mei", "Jun",
	"Jul", "Agu", "Sep", "Okt", "Nov", "Des",
}

func monthNameID(month int) string {
	if month < 1 || month > 12 {
		return ""
	}
	return monthNamesID[month-1]
}

func monthNameShortID(month int) string {
	if month < 1 || month > 12 {
		return ""
	}
	return monthNamesShortID[month-1]
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
