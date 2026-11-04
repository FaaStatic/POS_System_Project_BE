// Package barcodeimg merender barcode Code128 sebagai gambar: WebP (base64) untuk
// API produk dan PNG untuk Excel, karena Excel tidak menampilkan WebP.
package barcodeimg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/gen2brain/webp"
	"golang.org/x/sync/singleflight"
)

const (
	moduleWidth = 3  // piksel per modul batang; kelipatan bulat menjaga batang tetap tajam
	barHeight   = 90 // tinggi batang
	quietZone   = 10 * moduleWidth
	padTop      = 8
)

func render(code string) (image.Image, error) {
	encoded, err := code128.Encode(code)
	if err != nil {
		return nil, fmt.Errorf("encode code128 %q: %w", code, err)
	}
	bars, err := barcode.Scale(encoded, encoded.Bounds().Dx()*moduleWidth, barHeight)
	if err != nil {
		return nil, fmt.Errorf("scale barcode: %w", err)
	}
	// Scanner butuh margin putih minimal 10 modul di kiri-kanan; library tidak menambahkannya.
	// Dua warna: PNG-nya 1-bit, jadi file Excel berisi 500 barcode tetap kecil.
	canvas := image.NewPaletted(image.Rect(0, 0, bars.Bounds().Dx()+2*quietZone, barHeight+2*padTop), color.Palette{color.White, color.Black})
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, bars.Bounds().Add(image.Pt(quietZone, padTop)), bars, bars.Bounds().Min, draw.Src)
	return canvas, nil
}

func PNG(code string) ([]byte, error) {
	img, err := render(code)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// webpCacheMax membatasi cache: barcode yang sudah tidak dipakai (SKU diganti,
// produk dihapus) tidak pernah dibuang satu per satu, jadi cache dikosongkan penuh
// saat mencapai batas. ponytail: kosongkan penuh; ganti LRU bila churn barcode tinggi.
const webpCacheMax = 5000

// webpCache menyimpan hasil per kode: gambar murni fungsi dari kodenya, dan list
// produk memanggilnya sampai 100x per halaman.
var (
	webpMu    sync.RWMutex
	webpCache = make(map[string]string)
	webpGroup singleflight.Group
)

// WebPBase64 mengembalikan base64 mentah (tanpa prefix data URI, sama seperti
// products.image; FE yang menambahkan "data:image/webp;base64,"). Lossless supaya
// tepi batang tidak kabur dan tetap terbaca scanner. Permintaan bersamaan untuk
// kode yang sama (cache dingin) dikodekan sekali.
func WebPBase64(code string) (string, error) {
	webpMu.RLock()
	cached, ok := webpCache[code]
	webpMu.RUnlock()
	if ok {
		return cached, nil
	}
	encoded, err, _ := webpGroup.Do(code, func() (any, error) {
		img, err := render(code)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		if err := webp.Encode(&buf, img, webp.Options{Lossless: true}); err != nil {
			return "", fmt.Errorf("encode webp: %w", err)
		}
		value := base64.StdEncoding.EncodeToString(buf.Bytes())
		webpMu.Lock()
		if len(webpCache) >= webpCacheMax {
			clear(webpCache)
		}
		webpCache[code] = value
		webpMu.Unlock()
		return value, nil
	})
	if err != nil {
		return "", err
	}
	return encoded.(string), nil
}
