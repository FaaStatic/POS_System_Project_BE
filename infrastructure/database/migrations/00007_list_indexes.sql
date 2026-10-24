-- +goose NO TRANSACTION
-- +goose Up
-- CONCURRENTLY supaya tabel tetap bisa ditulis (webhook Midtrans, pembayaran
-- hutang) selama index dibangun; karena itu migration ini tanpa transaksi.
--
-- payments: GET /api/payments memakai keyset (created_at, id); tanpa index ini
-- setiap halaman seq scan + sort seluruh tabel.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payments_keyset
    ON payments (created_at, id) WHERE deleted_at IS NULL;
-- debt_payments: dashboard dan laporan bulanan memfilter rentang tanggal_bayar.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_debt_payments_tanggal_bayar
    ON debt_payments (tanggal_bayar);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_debt_payments_tanggal_bayar;
DROP INDEX CONCURRENTLY IF EXISTS idx_payments_keyset;
