package barcodeimg

import (
	"bytes"
	"encoding/base64"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/gen2brain/webp"
)

func TestPNGDecodesWithQuietZone(t *testing.T) {
	data, err := PNG("SKU-1-1a2b3c4d")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a valid png: %v", err)
	}
	// Piksel paling kiri ada di quiet zone, jadi harus putih.
	if r, g, b, _ := img.At(0, img.Bounds().Dy()/2).RGBA(); r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatal("quiet zone must be white")
	}
}

func TestWebPBase64(t *testing.T) {
	encoded, err := WebPBase64("SKU-1-1a2b3c4d")
	if err != nil {
		t.Fatal(err)
	}
	// Sama seperti products.image: base64 mentah, FE yang menambah prefix data URI.
	if strings.HasPrefix(encoded, "data:") {
		t.Fatalf("must be raw base64 without a data URI prefix: %.30s", encoded)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		t.Fatal("payload is not a webp container")
	}

	if string(raw[12:16]) != "VP8L" {
		t.Fatalf("chunk = %q, want VP8L (lossless)", raw[12:16])
	}

	// Posisi batang hitam/putih harus sama dengan hasil render; bila bergeser,
	// barcode tidak terbaca scanner. Dibandingkan sebagai hitam/putih karena
	// decoder library ini mengembalikan YCbCr berskala terbatas (putih = 235),
	// bukan 255, walaupun bitstream-nya lossless.
	decoded, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("webp does not decode: %v", err)
	}
	want, err := render("SKU-1-1a2b3c4d")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != want.Bounds() {
		t.Fatalf("size = %v, want %v", decoded.Bounds(), want.Bounds())
	}
	isBar := func(c color.Color) bool { r, _, _, _ := c.RGBA(); return r < 0x8000 }
	b := want.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isBar(want.At(x, y)) != isBar(decoded.At(x, y)) {
				t.Fatalf("bar/space mismatch at (%d,%d)", x, y)
			}
		}
	}
	if again, _ := WebPBase64("SKU-1-1a2b3c4d"); again != encoded {
		t.Fatal("cached value must equal first render")
	}
}

// maxImageBytes: batas hasil gambar barcode per produk.
const maxImageBytes = 500 << 10

// Kode terpanjang yang diterima API (64 karakter, batas barcodePattern dan kolom
// DB) menghasilkan gambar terlebar; kalau ini lolos, semua barcode lolos.
func TestWorstCaseImagesStayUnder500KB(t *testing.T) {
	worst := strings.Repeat("Zz9.-_", 10) + "Zz9." // 64 karakter, huruf campur angka (Code128 subset B)
	if len(worst) != 64 {
		t.Fatalf("fixture length = %d, want 64", len(worst))
	}

	pngData, err := PNG(worst)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := WebPBase64(worst)
	if err != nil {
		t.Fatal(err)
	}
	if len(pngData) > maxImageBytes {
		t.Errorf("png = %d bytes, want <= %d", len(pngData), maxImageBytes)
	}
	if len(encoded) > maxImageBytes {
		t.Errorf("webp base64 = %d bytes, want <= %d", len(encoded), maxImageBytes)
	}
	t.Logf("worst case: png %d bytes, webp base64 %d bytes", len(pngData), len(encoded))
}

func TestRejectsUnencodable(t *testing.T) {
	if _, err := PNG("kopi-é"); err == nil {
		t.Fatal("expected error for non-encodable content")
	}
}
