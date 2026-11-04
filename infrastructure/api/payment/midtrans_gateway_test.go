package payment

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"shop_project_be/internal/domain"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/coreapi"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// chargeHeaders menjalankan satu charge QRIS lewat gateway sungguhan dengan transport
// palsu, lalu mengembalikan header request yang sampai ke "Midtrans".
func chargeHeaders(t *testing.T, notificationURL string) http.Header {
	t.Helper()
	g := NewMidtransGateway("server-key", "sandbox", notificationURL).(*midtransGateway)
	var got http.Header
	g.client.HttpClient = &midtrans.HttpClientImplementation{
		HttpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			got = r.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"status_code":"201","order_id":"INV-1","transaction_status":"pending"}`)),
				Request:    r,
			}, nil
		})},
		Logger: midtrans.GetDefaultLogger(midtrans.Sandbox),
	}
	if _, err := g.ChargeQris(context.Background(), domain.GatewayChargeInput{OrderID: "INV-1", GrossAmount: 1000}); err != nil {
		t.Fatalf("charge failed: %v", err)
	}
	if got == nil {
		t.Fatal("no request reached the transport")
	}
	return got
}

func TestCharge_SendsOverrideNotificationHeader(t *testing.T) {
	const url = "https://api.tokoku.id/api/v2/sales/payments/notification"
	if got := chargeHeaders(t, url).Get("X-Override-Notification"); got != url {
		t.Errorf("X-Override-Notification = %q, want %q", got, url)
	}
}

func TestCharge_WithoutNotificationURLKeepsDashboardSetting(t *testing.T) {
	if got := chargeHeaders(t, "").Get("X-Override-Notification"); got != "" {
		t.Errorf("X-Override-Notification = %q, want none so the dashboard URL applies", got)
	}
}

func TestMapChargeResponse_BCAVirtualAccount(t *testing.T) {
	res := &coreapi.ChargeResponse{
		TransactionID:     "txn-1",
		OrderID:           "INV-1",
		PaymentType:       "bank_transfer",
		TransactionStatus: "pending",
		StatusCode:        "201",
		VaNumbers:         []coreapi.VANumber{{Bank: "bca", VANumber: "12345678"}},
	}
	out := mapChargeResponse(res)
	if out.VANumber != "12345678" || out.Bank != "bca" {
		t.Fatalf("VA mapping failed: %+v", out)
	}
}

func TestMapChargeResponse_MandiriEChannel(t *testing.T) {
	res := &coreapi.ChargeResponse{
		TransactionID:     "txn-2",
		OrderID:           "INV-2",
		PaymentType:       "echannel",
		TransactionStatus: "pending",
		StatusCode:        "201",
		BillKey:           "BK-9",
		BillerCode:        "BC-7",
	}
	out := mapChargeResponse(res)
	if out.BillKey != "BK-9" || out.BillerCode != "BC-7" {
		t.Fatalf("echannel mapping failed: %+v", out)
	}
}
