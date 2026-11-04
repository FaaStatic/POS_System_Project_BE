-- +goose Up
-- products.image kini berisi base64 mentah dari WebP persegi; FE menambahkan prefix
-- "data:image/webp;base64," sendiri. Baris yang sempat tersimpan dengan prefix itu
-- (import berformat data URI) dibersihkan supaya tidak jadi prefix ganda di FE.
UPDATE products
SET image = substr(image, length('data:image/webp;base64,') + 1)
WHERE image LIKE 'data:image/webp;base64,%';

-- +goose Down
-- Tidak bisa dibedakan dari baris yang memang disimpan tanpa prefix, jadi tidak
-- dikembalikan.
SELECT 1;
