package enum

import "testing"

func TestParseMoneyPayment_Valid(t *testing.T) {
	cases := map[string]MoneyPayment{
		"tunai": tunai, "hutang": hutang, "transfer": transfer, "qris": qris,
	}
	for in, want := range cases {
		got, err := ParseMoneyPayment(in)
		if err != nil || got != want {
			t.Fatalf("ParseMoneyPayment(%q) = %v, %v; want %v, nil", in, got, err, want)
		}
	}
}

func TestParseMoneyPayment_KartuRemoved(t *testing.T) {
	if _, err := ParseMoneyPayment("kartu"); err == nil {
		t.Fatal("expected error for removed 'kartu' payment type")
	}
}

func TestMoneyPaymentNumberingPreserved(t *testing.T) {
	if tunai != 0 || hutang != 1 || transfer != 2 || qris != 3 {
		t.Fatalf("numbering changed: tunai=%d hutang=%d transfer=%d qris=%d", tunai, hutang, transfer, qris)
	}
}
