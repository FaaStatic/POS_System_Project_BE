package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	reportDir = "../../storage/reports_test"
	os.Exit(m.Run())
}

func TestGenerateMonthReport(t *testing.T) {
	url, err := GenerateMonthReport(MonthReportData{
		Cashier: "admin",
		Month:   6,
		Year:    2026,
		Daily: []MonthReportDailyRow{
			{Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), CountSale: 5, Revenue: 1500000, Cost: 1200000},
			{Date: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), CountSale: 3, Revenue: 800000, Cost: 600000, CountDebt: 1, DebtSales: 250000, DebtCost: 200000, DebtManual: 100000},
			{Date: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), DebtPaid: 150000},
		},
		ProductsSold: []MonthReportProductRow{
			{ProductName: "Beras 5kg", Qty: 40, Total: 2600000},
			{ProductName: "Minyak Goreng 1L", Qty: 75, Total: 1350000},
			{ProductName: "Gula 1kg", Qty: 30, Total: 420000},
		},
		DailyProducts: []MonthReportDailyProducts{
			{Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Products: []MonthReportProductRow{
				{ProductName: "Beras 5kg", Qty: 5, Total: 325000},
				{ProductName: "Minyak Goreng 1L", Qty: 8, Total: 144000},
			}},
			{Date: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), Products: []MonthReportProductRow{
				{ProductName: "Gula 1kg", Qty: 3, Total: 42000},
			}},
		},
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("GenerateMonthReport error: %v", err)
	}
	if !strings.HasPrefix(url, urlPrefix+"/") {
		t.Fatalf("unexpected url: %s", url)
	}
	assertFileExists(t, url)
}

func TestGenerateTransactionReport(t *testing.T) {
	url, err := GenerateTransactionReport(TransactionReportData{
		NoInvoice:   "INV-001",
		Cashier:     "admin",
		Customer:    "Budi",
		PaymentType: "tunai",
		CreatedAt:   time.Now(),
		Items: []TransactionReportItem{
			{ProductName: "Beras 5kg", Qty: 2, Price: 65000, Subtotal: 130000},
			{ProductName: "Minyak Goreng 1L", Qty: 3, Price: 18000, Subtotal: 54000},
		},
		Total:       184000,
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("GenerateTransactionReport error: %v", err)
	}
	assertFileExists(t, url)
}

func TestGenerateTransactionReportWithChange(t *testing.T) {
	url, err := GenerateTransactionReport(TransactionReportData{
		NoInvoice:   "INV-002",
		Cashier:     "admin",
		PaymentType: "tunai",
		CreatedAt:   time.Now(),
		Items:       []TransactionReportItem{{ProductName: "Beras", Qty: 2.5, Price: 16000, Subtotal: 40000}},
		Total:       40000,
		AmountPaid:  50000, ChangeAmount: 10000,
		Change: []TransactionReportChange{
			{Kind: "tunai", Amount: 5000},
			{Kind: "permen", ProductName: "Permen Kopiko", Qty: 4, Amount: 2000},
			{Kind: "produk_lain", ProductName: "Gula", Qty: 0.25, Amount: 3000},
		},
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("GenerateTransactionReport error: %v", err)
	}
	assertFileExists(t, url)
}

func TestGenerateDebtReport(t *testing.T) {
	url, err := GenerateDebtReport(DebtReportData{
		DebtID:          "DEBT-001",
		CustomerName:    "Budi",
		CustomerPhone:   "081234567890",
		CustomerAddress: "Jl. Mawar No. 1",
		TotalDebt:       500000,
		RemainingDebt:   200000,
		Status:          "Belum Lunas",
		DueDate:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Payments: []DebtPaymentRow{
			{Date: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), Cashier: "admin", Nominal: 150000},
			{Date: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), Cashier: "admin", Nominal: 150000},
		},
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("GenerateDebtReport error: %v", err)
	}
	assertFileExists(t, url)
}

func TestReportPath(t *testing.T) {
	url, err := GenerateTransactionReport(TransactionReportData{NoInvoice: "INV-9/../x", GeneratedAt: time.Now()})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(url, urlPrefix+"/struk-INV-9----x-") {
		t.Fatalf("unexpected url %s", url)
	}
	if _, ok := ReportPath(filepath.Base(url)); !ok {
		t.Fatalf("generated report %s is not resolvable", url)
	}
	for _, bad := range []string{"", "../report.pdf", "a/b.pdf", `a\b.pdf`, ".hidden.pdf", "missing.pdf", "report.txt"} {
		if _, ok := ReportPath(bad); ok {
			t.Errorf("ReportPath(%q) accepted", bad)
		}
	}
}

func assertFileExists(t *testing.T, url string) {
	t.Helper()
	path := filepath.Join(reportDir, filepath.Base(url))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file not found %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("file %s empty", path)
	}
}

func TestGenerateDebtPaymentReceipt(t *testing.T) {
	for _, remaining := range []int64{5000, 0} {
		url, err := GenerateDebtPaymentReceipt(DebtPaymentReceiptData{
			Number: "HTG-A1-0007", Customer: "Budi", Cashier: "kasir1", PaidAt: time.Now(),
			PreviousRemaining: 20000, Paid: 15000, Remaining: remaining, GeneratedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("GenerateDebtPaymentReceipt error: %v", err)
		}
		assertFileExists(t, url)
	}
}

func TestGenerateDebtPaymentsMonthSpansPagesAndHandlesEmpty(t *testing.T) {
	rows := make([]DebtPaymentsMonthRow, 80)
	for i := range rows {
		rows[i] = DebtPaymentsMonthRow{
			Customer: "Toko Sembako Berkah Jaya Makmur Sentosa Abadi Cabang Pasar Minggu", Cashier: "kasir-dengan-nama-panjang-sekali",
			PaidAt: time.Date(2026, 11, 1+i%28, 9, 0, 0, 0, time.UTC), PreviousRemaining: 987654321, Paid: 100000, Remaining: int64(i%2) * 5000,
		}
	}
	for _, r := range [][]DebtPaymentsMonthRow{rows, nil} {
		url, err := GenerateDebtPaymentsMonth(DebtPaymentsMonthData{Month: 11, Year: 2026, Rows: r, GeneratedAt: time.Now()})
		if err != nil {
			t.Fatalf("GenerateDebtPaymentsMonth error: %v", err)
		}
		assertFileExists(t, url)
	}
}
