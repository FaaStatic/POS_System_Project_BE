-- +goose Up
-- GET /sales/debts/payments membaca riwayat cicilan per halaman (kursor tanggal_bayar, id)
-- dan menyaring per hutang; tanpa indeks ini setiap halaman memindai seluruh tabel.
CREATE INDEX IF NOT EXISTS idx_debt_payments_paid ON debt_payments (tanggal_bayar DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_debt_payments_paid;
