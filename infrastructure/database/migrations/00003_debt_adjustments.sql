-- +goose Up
-- Hutang manual (POST /api/debts) tidak punya transaksi, jadi tanpa tabel ini
-- nominalnya hanya menambah saldo debts tanpa jejak. Laporan bulanan
-- menjumlahkan baris ini sebagai "hutang baru (manual)".
CREATE TABLE IF NOT EXISTS debt_adjustments (
    id         uuid DEFAULT gen_random_uuid() NOT NULL,
    debt_id    uuid NOT NULL,
    user_id    uuid NOT NULL,
    amount     bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT debt_adjustments_pkey PRIMARY KEY (id),
    CONSTRAINT chk_debt_adjustments_amount CHECK (amount > 0),
    CONSTRAINT fk_debts_debt_adjustments FOREIGN KEY (debt_id) REFERENCES debts (id),
    CONSTRAINT fk_debt_adjustments_user FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_debt_adjustments_debt_id ON debt_adjustments (debt_id);
CREATE INDEX IF NOT EXISTS idx_debt_adjustments_created_at ON debt_adjustments (created_at);

-- +goose Down
DROP TABLE IF EXISTS debt_adjustments;
