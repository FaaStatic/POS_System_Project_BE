-- +goose Up
-- Produk lama tanpa barcode diberi barcode otomatis "<SKU>-<8 hex id>", rumus yang
-- sama dengan domain.AutoBarcode. Hanya produk aktif: idx_products_barcode hanya
-- mencakup baris yang belum dihapus.
UPDATE products
SET barcode = left(regexp_replace(sku, '[^A-Za-z0-9._-]', '-', 'g'), 55) || '-' || left(id::text, 8)
WHERE barcode IS NULL AND deleted_at IS NULL;

-- +goose Down
-- Tidak bisa dibedakan dari barcode yang diisi manual dengan pola yang sama, jadi
-- backfill tidak dibatalkan.
SELECT 1;
