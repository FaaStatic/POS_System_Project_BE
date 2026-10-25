-- +goose NO TRANSACTION
-- +goose Up
-- Barcode opsional, terpisah dari SKU: toko sering memakai barcode pabrik atau
-- buatan sendiri. NULL = belum ada barcode. Unique hanya di antara produk aktif
-- supaya satu scan tidak pernah cocok ke dua produk, sama seperti idx_products_sku.
-- Format juga dicek di usecase (normalizeBarcode); CHECK ini pengaman terakhir.
-- Tanpa transaksi karena index dibuat CONCURRENTLY (lihat 00007).
ALTER TABLE products ADD COLUMN IF NOT EXISTS barcode varchar(64);
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_barcode_format;
ALTER TABLE products ADD CONSTRAINT chk_products_barcode_format
    CHECK (barcode ~ '^[A-Za-z0-9._-]{1,64}$');
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_products_barcode
    ON products (barcode) WHERE deleted_at IS NULL AND barcode IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_products_barcode;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_barcode_format;
ALTER TABLE products DROP COLUMN IF EXISTS barcode;
