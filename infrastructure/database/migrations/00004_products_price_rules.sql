-- +goose Up
-- Backstop untuk validatePrices di product_usecase: usecase membaca harga lama
-- di luar lock baris, jadi dua update harga bersamaan bisa lolos kombinasi tak
-- valid. CHECK ini dievaluasi atomik per baris. NOT VALID: baris lama tidak
-- dicek ulang, hanya INSERT/UPDATE baru.
ALTER TABLE products ADD CONSTRAINT chk_products_price_rules CHECK (
    purchase_price > 0
    AND selling_price > purchase_price
    AND selling_price_debt * 100 >= selling_price * 105
) NOT VALID;

-- +goose Down
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_price_rules;
