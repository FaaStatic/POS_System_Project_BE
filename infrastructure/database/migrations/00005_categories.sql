-- +goose Up
-- Kategori jadi tabel sendiri supaya "Sembako", "sembako " dan "SEMBAKO" tidak
-- tercatat sebagai tiga kategori. key = lower(trim(name)) diisi aplikasi
-- (domain.CategoryKey) dan jadi penjaga duplikat; name = penulisan pertama.
CREATE TABLE IF NOT EXISTS categories (
    id         uuid DEFAULT gen_random_uuid() NOT NULL,
    key        varchar(100) NOT NULL,
    name       varchar(100) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT categories_pkey PRIMARY KEY (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_key ON categories (key);

ALTER TABLE products ADD COLUMN IF NOT EXISTS category_id uuid
    CONSTRAINT fk_products_category REFERENCES categories (id);

-- Pindahkan kategori teks lama, kalau DB tidak di-reset.
INSERT INTO categories (key, name)
SELECT DISTINCT ON (lower(btrim(category))) lower(btrim(category)), btrim(category)
FROM products
WHERE btrim(coalesce(category, '')) <> ''
ORDER BY lower(btrim(category)), created_at
ON CONFLICT (key) DO NOTHING;
UPDATE products SET category_id = c.id
FROM categories c
WHERE c.key = lower(btrim(products.category));

DROP INDEX IF EXISTS idx_products_category;
ALTER TABLE products DROP COLUMN IF EXISTS category;
CREATE INDEX IF NOT EXISTS idx_products_category_id ON products (category_id);

-- +goose Down
ALTER TABLE products ADD COLUMN IF NOT EXISTS category varchar(100);
UPDATE products SET category = c.name FROM categories c WHERE c.id = products.category_id;
DROP INDEX IF EXISTS idx_products_category_id;
ALTER TABLE products DROP COLUMN IF EXISTS category_id;
CREATE INDEX IF NOT EXISTS idx_products_category ON products (category);
DROP TABLE IF EXISTS categories;
