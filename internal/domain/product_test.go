package domain

import (
	"math"
	"strings"
	"testing"

	"shop_project_be/internal/constant/enum"

	"github.com/google/uuid"
)

// Nilai harus sama persis dengan hasil migrasi 00010 untuk SKU yang sama; kalau
// rumus salah satunya berubah, barcode produk lama dan baru jadi tidak seragam.
func TestAutoBarcodeMatchesBackfillMigration(t *testing.T) {
	id := uuid.MustParse("1a2b3c4d-0000-0000-0000-000000000001")
	if got, want := AutoBarcode("GULA 1/2 é", id), "GULA-1-2---1a2b3c4d"; got != want {
		t.Errorf("AutoBarcode = %q, want %q", got, want)
	}
	long := AutoBarcode(strings.Repeat("A", 70), id)
	if len(long) != 64 {
		t.Errorf("long sku barcode length = %d, want 64", len(long))
	}
}

func TestCheckQty(t *testing.T) {
	tests := []struct {
		name    string
		unit    string
		qty     float64
		wantErr bool
	}{
		{"pcs whole", "pcs", 3, false},
		{"pcs fractional", "pcs", 0.5, true},
		{"kg half", "kg", 0.5, false},
		{"kg eighth", "kg", 0.125, false},
		{"kg float noise", "kg", 0.1 + 0.2, false},
		{"kg four decimals", "kg", 0.1234, true},
		{"kardus quarter", "kardus", 0.25, false},
		{"gram fractional", "gram", 12.5, false},
		{"kg NaN", "kg", math.NaN(), true},
		{"pcs NaN", "pcs", math.NaN(), true},
		{"pcs Inf", "pcs", math.Inf(1), true},
		{"kg -Inf", "kg", math.Inf(-1), true},
		{"over numeric(12,3)", "kg", 1e10, true},
		{"negative delta in range", "kg", -2.5, false},
		{"upper bound", "kg", maxQty, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unit, err := enum.ParseProductUnit(tt.unit)
			if err != nil {
				t.Fatal(err)
			}
			if got := CheckQty(unit, tt.qty); (got != nil) != tt.wantErr {
				t.Fatalf("CheckQty(%s, %v) err = %v, wantErr %v", tt.unit, tt.qty, got, tt.wantErr)
			}
		})
	}
}
