package payment

import (
	"testing"

	"github.com/midtrans/midtrans-go/coreapi"
)

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
