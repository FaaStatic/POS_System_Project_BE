package usecase

import "testing"

func TestApplyFee(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		subtotal int64
		want     int64
	}{
		{"qris passes the 0.7% VAT-inclusive MDR to the buyer", "qris", 100000, 100705},
		{"qris rounds up so the merchant never absorbs a fraction", "qris", 10001, 10072},
		{"va adds the flat Rp4.000 + 11% VAT", "va", 100000, 104440},
		{"unknown method charges no fee", "cash", 100000, 100000},
		{"zero subtotal stays zero", "qris", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := applyFee(c.method, c.subtotal); got != c.want {
				t.Errorf("applyFee(%q,%d) = %d; want %d", c.method, c.subtotal, got, c.want)
			}
		})
	}
}

func TestApplyFee_QrisNetsTheSubtotal(t *testing.T) {
	for _, subtotal := range []int64{1000, 9999, 100000, 1000000, 12345678} {
		gross := applyFee("qris", subtotal)
		netted := gross - int64(float64(gross)*feeQrisRate)
		if netted < subtotal {
			t.Errorf("subtotal %d: gross %d nets %d after the MDR; merchant is short %d",
				subtotal, gross, netted, subtotal-netted)
		}
	}
}
