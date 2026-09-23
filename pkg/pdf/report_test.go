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
		Cashier:          "admin",
		Month:            6,
		Year:             2026,
		TotalTransaction: 42,
		TotalRevenue:     12500000,
		TotalDebt:        750000,
		GrandTotal:       13250000,
		Daily: []MonthReportDailyRow{
			{Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), TotalTransaction: 5, Revenue: 1500000, Debt: 0, Total: 1500000},
			{Date: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), TotalTransaction: 3, Revenue: 800000, Debt: 250000, Total: 1050000},
			{Date: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), TotalTransaction: 8, Revenue: 3200000, Debt: 500000, Total: 3700000},
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
