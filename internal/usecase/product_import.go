package usecase

import (
	"context"
	"crypto/sha256"
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

const (
	// imageWorkers membatasi konversi paralel: satu konversi memakai ±0,1 detik CPU
	// dan puluhan MB memori untuk foto besar.
	imageWorkers = 4
	// maxImportImages membatasi gambar unik per import; berurutan, 500 foto sudah
	// butuh sekitar satu menit, terlalu lama untuk satu request.
	maxImportImages = 500
)

type rowImageResult struct {
	value string
	err   error
}

// imageKey: hash sumber gambar satu baris (gambar tertanam atau teks sel), supaya
// gambar yang sama di banyak baris cukup dikonversi sekali. Baris tanpa gambar ok=false.
func imageKey(row sheet.ProductRow) (key [sha256.Size]byte, ok bool) {
	switch {
	case len(row.ImageFile) > 0:
		return sha256.Sum256(append([]byte{'f'}, row.ImageFile...)), true
	case row.Image != "":
		return sha256.Sum256([]byte("t" + row.Image)), true
	}
	return key, false
}

// convertRowImages mengonversi gambar semua baris secara paralel terbatas; hasilnya
// sejajar dengan rows. Gambar yang ditempel di Excel didahulukan dari teks di sel
// image. Gagal bila gambar unik lebih dari maxImportImages.
func convertRowImages(rows []sheet.ProductRow) ([]rowImageResult, error) {
	results := make([]rowImageResult, len(rows))
	firstByKey := make(map[[sha256.Size]byte]int)
	duplicateOf := make(map[int]int)
	var jobs []int
	for i, row := range rows {
		key, ok := imageKey(row)
		if !ok {
			continue
		}
		if first, seen := firstByKey[key]; seen {
			duplicateOf[i] = first
			continue
		}
		firstByKey[key] = i
		jobs = append(jobs, i)
	}
	if len(jobs) > maxImportImages {
		return nil, domain.Validation(fmt.Sprintf("too many images: %d distinct images in one import, the limit is %d", len(jobs), maxImportImages))
	}

	sem := make(chan struct{}, imageWorkers)
	var wg sync.WaitGroup
	for _, i := range jobs {
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
			var value string
			var err error
			if len(rows[i].ImageFile) > 0 {
				value, err = productimg.FromBytes(rows[i].ImageFile)
			} else {
				value, err = productimg.FromText(rows[i].Image)
			}
			results[i] = rowImageResult{value: value, err: err}
		}()
	}
	wg.Wait()
	for i, first := range duplicateOf {
		results[i] = results[first]
	}
	return results, nil
}
