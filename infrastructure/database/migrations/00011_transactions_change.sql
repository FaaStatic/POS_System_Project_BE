-- +goose Up
-- Kembalian tunai: amount_paid = uang yang diterima kasir, change_amount = selisihnya
-- dengan total_transaction. NULL = transaksi lama atau kasir tidak mengirim jumlah bayar.
-- Laporan omzet/laba tetap memakai total_transaction; kembalian hanya dicatat.
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS amount_paid   bigint,
    ADD COLUMN IF NOT EXISTS change_amount bigint;

-- Satu baris per bentuk kembalian. kind tunai = uang; permen/produk_lain = barang
-- yang stoknya ikut berkurang, dengan harga jual dan harga pokok saat itu.
-- Mengikuti transactions_detail: tanpa soft delete sendiri, ikut transaksinya.
CREATE TABLE IF NOT EXISTS transactions_change (
    id             uuid DEFAULT gen_random_uuid() NOT NULL,
    transaction_id uuid NOT NULL,
    kind           varchar(20) NOT NULL,
    amount         bigint NOT NULL,
    product_id     uuid,
    product_name   varchar(255) NOT NULL DEFAULT '',
    qty            numeric(12,3) NOT NULL DEFAULT 0,
    unit_price     bigint NOT NULL DEFAULT 0,
    purchase_price bigint NOT NULL DEFAULT 0,
    CONSTRAINT transactions_change_pkey PRIMARY KEY (id),
    CONSTRAINT fk_transactions_change_transaction FOREIGN KEY (transaction_id) REFERENCES transactions (id),
    CONSTRAINT fk_transactions_change_product FOREIGN KEY (product_id) REFERENCES products (id),
    CONSTRAINT chk_transactions_change_kind CHECK (kind IN ('tunai', 'permen', 'produk_lain')),
    CONSTRAINT chk_transactions_change_amount CHECK (amount > 0),
    CONSTRAINT chk_transactions_change_product CHECK ((kind = 'tunai') = (product_id IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_transactions_change_transaction_id ON transactions_change (transaction_id);

-- +goose Down
DROP TABLE IF EXISTS transactions_change;
ALTER TABLE transactions
    DROP COLUMN IF EXISTS change_amount,
    DROP COLUMN IF EXISTS amount_paid;
