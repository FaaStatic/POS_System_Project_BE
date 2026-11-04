// Package productimg mengubah gambar produk apa pun menjadi base64 mentah (tanpa
// prefix data URI) dari WebP persegi 1:1 di bawah 500 KB: format yang disimpan di
// products.image. FE yang menambahkan "data:image/webp;base64,".
package productimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // mendaftarkan decoder untuk image.Decode
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/gen2brain/webp" // juga mendaftarkan decoder WebP
	"golang.org/x/image/draw"
)

const (
	MaxBytes = 500 << 10 // batas ukuran file WebP hasil
	// maxSide: gambar produk tampil sebagai thumbnail di kasir dan CMS, dan ikut
	// terkirim di setiap baris list produk, jadi dijaga kecil (±10 KB per gambar).
	maxSide = 256
	// maxPixels: batas piksel gambar sumber, mencegah decompression bomb. 16 MP
	// berarti ±64 MB per decode; dengan 4 worker paralel puncaknya ±256 MB.
	maxPixels = 16_000_000

	maxImageRef   = 2048    // panjang maksimum URL atau path di sel image
	maxImageText  = 8 << 20 // panjang maksimum base64 di sel image
	minBase64Text = 16      // teks tanpa prefix lebih pendek dari ini dianggap nama, bukan base64
)

var base64Cleaner = strings.NewReplacer("\r", "", "\n", "", " ", "", "\t", "")

// FromBytes memotong bagian tengah gambar menjadi persegi, memperkecilnya bila
// lebih dari maxSide, lalu mengkodekannya sebagai WebP lossy. Mengembalikan base64.
func FromBytes(src []byte) (string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return "", errors.New("unsupported image format (use PNG, JPEG, GIF or WebP)")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return "", errors.New("image dimensions are too large")
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return "", fmt.Errorf("image cannot be decoded: %w", err)
	}
	// Sudah WebP persegi yang cukup kecil: dipakai apa adanya. Mengkodekan ulang WebP
	// lossy hanya menurunkan kualitas dan memakan CPU.
	if format == "webp" && cfg.Width == cfg.Height && cfg.Width <= maxSide && len(src) <= MaxBytes {
		return base64.StdEncoding.EncodeToString(src), nil
	}

	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)

	out := min(side, maxSide)
	dst := image.NewRGBA(image.Rect(0, 0, out, out))
	// Latar putih: PNG/GIF transparan tidak jadi hitam saat dikonversi ke WebP.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, draw.Over, nil)

	// Noise acak murni 256x256 (kasus terburuk) hanya 44 KB pada q80, jauh di bawah
	// MaxBytes, jadi ukuran hasil tidak perlu dicek lagi.
	var buf bytes.Buffer
	if err := webp.Encode(&buf, dst, webp.Options{Quality: 80, Method: 4}); err != nil {
		return "", fmt.Errorf("encode webp: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// FromText menormalkan isi teks sel image:
//   - kosong, URL (http/https), atau teks yang bukan base64 (mis. path) dibiarkan;
//   - data URI atau base64 mentah dikonversi lewat FromBytes.
//
// Teks yang valid base64 tapi bukan gambar ditolak.
func FromText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if len(text) > maxImageText {
		return "", errors.New("image text is too long")
	}
	lower := strings.ToLower(text)
	if text == "" || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if len(text) > maxImageRef {
			return "", errors.New("image url is too long")
		}
		return text, nil
	}
	payload := text
	isDataURI := strings.HasPrefix(lower, "data:")
	if isDataURI {
		head, body, ok := strings.Cut(text, ",")
		if !ok || !strings.Contains(strings.ToLower(head), ";base64") {
			return "", errors.New("image data URI must be base64 encoded")
		}
		payload = body
	}
	cleaned := base64Cleaner.Replace(payload)
	raw, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(cleaned, "="))
	}
	if err != nil {
		if isDataURI {
			return "", errors.New("image base64 is not valid")
		}
		if len(text) > maxImageRef {
			return "", errors.New("image must be a url, a path or base64 image data")
		}
		return text, nil
	}
	if !isDataURI && len(cleaned) < minBase64Text {
		return text, nil
	}
	return FromBytes(raw)
}
