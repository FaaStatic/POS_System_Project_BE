-- +goose Up
-- GET /api/customers/:id/transactions: filter customer_id lalu keyset
-- (created_at, id). Index ini melayani filter + urutan + cursor sekaligus, jadi
-- Postgres cukup scan range tanpa sort. idx_transactions_customer_id tetap
-- dipakai untuk query yang ikut membaca baris soft-deleted (index ini partial).
CREATE INDEX IF NOT EXISTS idx_transactions_customer_keyset
    ON transactions (customer_id, created_at, id) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_customer_keyset;
