-- +goose Up
-- client_ref: id transaksi yang dibuat kasir saat offline. Unique tanpa filter
-- deleted_at supaya transaksi offline yang sudah dibatalkan tidak tercipta lagi
-- ketika device mengirim ulang antrean yang sama.
-- stock_conflict: transaksi offline diterima walau stok kurang (barang sudah
-- diserahkan); stok berhenti di 0, admin meninjaunya di CMS lalu mengisi
-- conflict_reviewed_at.
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS client_ref           uuid,
    ADD COLUMN IF NOT EXISTS stock_conflict       boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS conflict_reviewed_at timestamptz,
    ADD COLUMN IF NOT EXISTS cancel_reason        text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_client_ref
    ON transactions (client_ref) WHERE client_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_open_conflict
    ON transactions (created_at, id)
    WHERE stock_conflict AND conflict_reviewed_at IS NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_open_conflict;
DROP INDEX IF EXISTS idx_transactions_client_ref;
ALTER TABLE transactions
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS conflict_reviewed_at,
    DROP COLUMN IF EXISTS stock_conflict,
    DROP COLUMN IF EXISTS client_ref;
