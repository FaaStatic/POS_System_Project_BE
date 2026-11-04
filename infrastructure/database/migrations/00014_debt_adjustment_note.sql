-- +goose Up
-- Keterangan opsional untuk hutang manual (mis. "pinjam tunai"), ditampilkan di rincian hutang.
ALTER TABLE debt_adjustments ADD COLUMN IF NOT EXISTS note varchar(200);

-- +goose Down
ALTER TABLE debt_adjustments DROP COLUMN IF EXISTS note;
