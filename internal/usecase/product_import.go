package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"shop_project_be/internal/domain"
	"shop_project_be/pkg/productimg"
	"shop_project_be/pkg/sheet"

	"github.com/google/uuid"
)

// autoSKUPrefix: tiga huruf pertama nama kategori dalam huruf kapital ("Sembako"
// -> "SEM"); "PRD" bila kategori kosong atau tanpa huruf.
func autoSKUPrefix(category string) string {
	var prefix strings.Builder
	for _, r := range category {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		if r >= 'A' && r <= 'Z' {
			prefix.WriteRune(r)
			if prefix.Len() == 3 {
				break
			}
		}
	}
	if prefix.Len() == 0 {
		return "PRD"
	}
	return prefix.String()
}

// newAutoSKU: "<PREFIX>-<8 hex acak>", mis. SEM-1a2b3c4d.
func newAutoSKU(category string) string {
	return autoSKUPrefix(category) + "-" + uuid.NewString()[:8]
}

// assignAutoSKUs mengisi SKU baris yang kosong. SKU yang bentrok dengan SKU di file
// atau produk aktif diganti: tanpa pengecekan ini upsert akan menimpa produk lain
// yang kebetulan memakai SKU yang sama.
func (p *productUsecase) assignAutoSKUs(ctx context.Context, rows []sheet.ProductRow, generate func(category string) string) error {
	taken := make(map[string]struct{}, len(rows))
	var pending []int
	for i, row := range rows {
		if row.SKU == "" {
			pending = append(pending, i)
			continue
		}
		taken[row.SKU] = struct{}{}
	}

	for attempt := 0; len(pending) > 0 && attempt < 5; attempt++ {
		candidates := make([]string, len(pending))
		for k, i := range pending {
			candidates[k] = generate(rows[i].Category)
		}
		inUse, err := p.productRepo.GetProductsBySKUs(ctx, candidates)
		if err != nil {
			return err
		}
		var retry []int
		for k, i := range pending {
			_, dup := taken[candidates[k]]
			if _, exists := inUse[candidates[k]]; exists || dup {
				retry = append(retry, i)
				continue
			}
			rows[i].SKU = candidates[k]
			taken[candidates[k]] = struct{}{}
		}
		pending = retry
	}
	if len(pending) > 0 {
		return fmt.Errorf("could not generate a unique sku for %d row(s): %w", len(pending), domain.ErrInternal)
	}
	return nil
}

// rowImage mengubah gambar satu baris menjadi base64 WebP 1:1 di bawah 500 KB.
// Gambar yang ditempel di Excel didahulukan dari teks di sel image.
func rowImage(row sheet.ProductRow) (string, error) {
	if len(row.ImageFile) > 0 {
		return productimg.FromBytes(row.ImageFile)
	}
	return productimg.FromText(row.Image)
}

// imageWorkers membatasi konversi paralel: satu konversi memakai ±0,1 detik CPU dan
// puluhan MB memori untuk foto besar, jadi tanpa batas 500 foto bisa menghabiskan RAM.
const imageWorkers = 4

type rowImageResult struct {
	value string
	err   error
}

// convertRowImages mengonversi gambar semua baris secara paralel terbatas; hasilnya
// sejajar dengan rows. Berurutan, 500 foto butuh sekitar satu menit lebih, terlalu
// lama untuk satu request.
func convertRowImages(rows []sheet.ProductRow) []rowImageResult {
	results := make([]rowImageResult, len(rows))
	sem := make(chan struct{}, imageWorkers)
	var wg sync.WaitGroup
	for i := range rows {
		if len(rows[i].ImageFile) == 0 && rows[i].Image == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			// Input tak tepercaya diproses di goroutine sendiri, di luar recover Fiber:
			// satu gambar yang membuat decoder panik tidak boleh menjatuhkan server.
			defer func() {
				if r := recover(); r != nil {
					results[i].err = errors.New("image could not be processed")
				}
			}()
			value, err := rowImage(rows[i])
			results[i] = rowImageResult{value: value, err: err}
		}()
	}
	wg.Wait()
	return results
}
