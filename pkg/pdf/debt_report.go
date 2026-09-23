package pdf

import "time"

type DebtReportData struct {
	StoreName       string
	DebtID          string
	CustomerName    string
	CustomerPhone   string
	CustomerAddress string
	TotalDebt       int64
	RemainingDebt   int64
	Status          string
	DueDate         time.Time
	Payments        []DebtPaymentRow
	GeneratedAt     time.Time
}

type DebtPaymentRow struct {
	Date    time.Time
	Cashier string
	Nominal int64
}

func GenerateDebtReport(data DebtReportData) (string, error) {
	pdf := newDocument()
	pdf.AddPage()

	storeName := data.StoreName
	if storeName == "" {
		storeName = "Shop Project"
	}

	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 10, storeName, "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(0, 8, "Laporan Hutang Customer", "", 1, "C", false, 0, "")
	pdf.Ln(2)
	drawLine(pdf)
	pdf.Ln(4)

	pdf.SetFont("Arial", "", 10)
	info := [][2]string{
		{"Nama", data.CustomerName},
		{"No. HP", data.CustomerPhone},
		{"Alamat", data.CustomerAddress},
		{"Jatuh Tempo", formatDueDate(data.DueDate)},
		{"Status", data.Status},
	}
	for _, m := range info {
		if m[1] == "" {
			continue
		}
		pdf.CellFormat(35, 6, m[0], "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, ": "+m[1], "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Ringkasan", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	paid := data.TotalDebt - data.RemainingDebt
	if paid < 0 {
		paid = 0
	}
	rows := [][2]string{
		{"Total Hutang", FormatRupiah(data.TotalDebt)},
		{"Sudah Dibayar", FormatRupiah(paid)},
		{"Sisa Hutang", FormatRupiah(data.RemainingDebt)},
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
	pdf.Ln(6)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Riwayat Pembayaran", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	drawDebtPaymentHeader(pdf)
	if len(data.Payments) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(180, 8, "Belum ada pembayaran.", "1", 1, "C", false, 0, "")
	} else {
		pdf.SetFont("Arial", "", 9)
		for _, p := range data.Payments {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawDebtPaymentHeader(pdf)
				pdf.SetFont("Arial", "", 9)
			}
			pdf.CellFormat(50, 7, formatDateTime(p.Date), "1", 0, "L", false, 0, "")
			pdf.CellFormat(80, 7, truncate(p.Cashier, 45), "1", 0, "L", false, 0, "")
			pdf.CellFormat(50, 7, FormatRupiah(p.Nominal), "1", 1, "R", false, 0, "")
		}
		pdf.SetFont("Arial", "B", 9)
		pdf.CellFormat(130, 8, "TOTAL DIBAYAR", "1", 0, "R", false, 0, "")
		pdf.CellFormat(50, 8, FormatRupiah(paid), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(10)
	pdf.SetFont("Arial", "I", 9)
	pdf.CellFormat(0, 5, "Dicetak pada "+formatDateTime(data.GeneratedAt), "", 1, "C", false, 0, "")

	return saveDocument(pdf, "hutang-"+sanitizeFilename(data.DebtID))
}

func formatDueDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return formatDateOnly(t)
}
