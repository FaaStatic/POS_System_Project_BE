package enum

import "testing"

func TestParseProductType(t *testing.T) {
	cases := map[string]ProductType{
		"": Physical, "physical": Physical, "0": Physical,
		"digital": Digital, "1": Digital, "DIGITAL": Digital,
	}
	for in, want := range cases {
		got, err := ParseProductType(in)
		if err != nil || got != want {
			t.Fatalf("ParseProductType(%q) = %v, %v; want %v, nil", in, got, err, want)
		}
	}
}

func TestParseProductType_Invalid(t *testing.T) {
	if _, err := ParseProductType("gas"); err == nil {
		t.Fatal("expected error for invalid product type")
	}
}

func TestProductTypeIsDigital(t *testing.T) {
	if !Digital.IsDigital() || Physical.IsDigital() {
		t.Fatal("IsDigital mismatch")
	}
	if Physical.String() != "physical" || Digital.String() != "digital" {
		t.Fatal("String mismatch")
	}
}
