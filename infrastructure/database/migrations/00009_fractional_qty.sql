-- +goose Up
-- Toko menjual per kg/gram/liter/kardus dalam pecahan (0.125 kg), jadi stok dan qty
-- naik dari 2 ke 3 desimal. Hanya produk berunit pcs yang wajib bulat; itu dicek di
-- usecase karena unit ada di baris produk, bukan di kolom qty.
ALTER TABLE products ALTER COLUMN stock TYPE numeric(12,3);
ALTER TABLE transactions_detail ALTER COLUMN qty TYPE numeric(12,3);

-- +goose Down
-- Turun ke 2 desimal membulatkan nilai yang sudah tersimpan.
ALTER TABLE transactions_detail ALTER COLUMN qty TYPE numeric(8,2);
ALTER TABLE products ALTER COLUMN stock TYPE numeric(10,2);
