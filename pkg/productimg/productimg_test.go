package productimg

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/gen2brain/webp"
)

func pngBytes(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 120, A: alpha})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decodeResult memeriksa hasil: base64 mentah (tanpa prefix) dari WebP persegi < 500 KB.
func decodeResult(t *testing.T, got string, wantSide int) {
	t.Helper()
	if strings.HasPrefix(got, "data:") {
		t.Fatal("result must be raw base64, without a data URI prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("result is not base64: %v", err)
	}
	if len(raw) > MaxBytes {
		t.Errorf("webp = %d bytes, want <= %d", len(raw), MaxBytes)
	}
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		t.Fatal("result is not a webp container")
	}
	img, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("webp does not decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != b.Dy() || b.Dx() != wantSide {
		t.Errorf("size = %v, want %dx%d (1:1)", b, wantSide, wantSide)
	}
}

func TestFromBytes(t *testing.T) {
	var jpg bytes.Buffer
	src := image.NewRGBA(image.Rect(0, 0, 300, 200))
	if err := jpeg.Encode(&jpg, src, nil); err != nil {
		t.Fatal(err)
	}
	var gifData bytes.Buffer
	if err := gif.Encode(&gifData, image.NewPaletted(image.Rect(0, 0, 90, 60), color.Palette{color.White, color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	var webpData bytes.Buffer
	if err := webp.Encode(&webpData, image.NewRGBA(image.Rect(0, 0, 100, 140)), webp.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		data     []byte
		wantSide int
	}{
		{"landscape png is cropped to a square and capped at 256", pngBytes(t, 800, 400, 255), 256},
		{"portrait png is cropped to a square and capped at 256", pngBytes(t, 400, 800, 255), 256},
		{"large square png is shrunk to 256", pngBytes(t, 1600, 1600, 255), 256},
		{"small square stays as it is (never upscaled)", pngBytes(t, 64, 64, 255), 64},
		{"short side below the cap decides the size", pngBytes(t, 500, 200, 255), 200},
		{"transparent png", pngBytes(t, 300, 300, 0), 256},
		{"jpeg", jpg.Bytes(), 200},
		{"gif", gifData.Bytes(), 60},
		{"webp input", webpData.Bytes(), 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromBytes(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			decodeResult(t, got, tt.wantSide)
		})
	}
}

func TestFromBytesKeepsValidWebPUntouched(t *testing.T) {
	var square, wide bytes.Buffer
	if err := webp.Encode(&square, image.NewRGBA(image.Rect(0, 0, 200, 200)), webp.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}
	if err := webp.Encode(&wide, image.NewRGBA(image.Rect(0, 0, 300, 200)), webp.Options{Quality: 60}); err != nil {
		t.Fatal(err)
	}

	got, err := FromBytes(square.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got != base64.StdEncoding.EncodeToString(square.Bytes()) {
		t.Error("a square WebP within the size cap must be kept byte for byte, not re-encoded")
	}

	got, err = FromBytes(wide.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got == base64.StdEncoding.EncodeToString(wide.Bytes()) {
		t.Error("a non-square WebP must be cropped and re-encoded")
	}
	decodeResult(t, got, 200)
}

func TestFromBytesRejects(t *testing.T) {
	// Header PNG sah dengan dimensi 20000x20000 (400 MP): harus ditolak tanpa mendekode pikselnya.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 20000)
	binary.BigEndian.PutUint32(ihdr[4:], 20000)
	ihdr[8], ihdr[9] = 8, 0 // 8-bit grayscale
	chunk := append([]byte("IHDR"), ihdr...)
	huge := []byte("\x89PNG\r\n\x1a\n")
	huge = binary.BigEndian.AppendUint32(huge, 13)
	huge = append(huge, chunk...)
	huge = binary.BigEndian.AppendUint32(huge, crc32.ChecksumIEEE(chunk))

	for name, data := range map[string][]byte{
		"not an image": []byte("hello world"),
		"empty":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := FromBytes(data); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	// DecodeConfig menerima header ini, jadi yang menolaknya harus cek ukuran kita.
	if _, err := FromBytes(huge); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v, want the 'too large' dimension error", err)
	}
}

func TestFromText(t *testing.T) {
	raw := pngBytes(t, 200, 100, 255)
	b64 := base64.StdEncoding.EncodeToString(raw)

	for name, in := range map[string]string{
		"raw base64":          b64,
		"data uri png":        "data:image/png;base64," + b64,
		"data uri uppercase":  "DATA:IMAGE/PNG;BASE64," + b64,
		"base64 with newline": b64[:40] + "\n" + b64[40:],
	} {
		t.Run(name, func(t *testing.T) {
			got, err := FromText(in)
			if err != nil {
				t.Fatal(err)
			}
			decodeResult(t, got, 100)
		})
	}

	for name, in := range map[string]string{
		"empty":     "",
		"blank":     "   ",
		"https url": "https://cdn.example.com/a.png",
		"http url":  "HTTP://cdn.example.com/a.png",
		"path":      "images/gula.webp",
	} {
		t.Run("kept "+name, func(t *testing.T) {
			got, err := FromText(in)
			if err != nil || got != strings.TrimSpace(in) {
				t.Fatalf("got %q, %v; want the input unchanged", got, err)
			}
		})
	}

	for name, in := range map[string]string{
		"base64 but not an image":  base64.StdEncoding.EncodeToString([]byte("just some text bytes")),
		"data uri without base64":  "data:image/png,abc",
		"data uri invalid payload": "data:image/png;base64,@@@",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			if _, err := FromText(in); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
