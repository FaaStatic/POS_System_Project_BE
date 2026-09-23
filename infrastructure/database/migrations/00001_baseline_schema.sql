-- +goose Up
-- Baseline schema, consolidated to match internal/domain/ as of this migration.
-- Perubahan skema selanjutnya WAJIB lewat file migration baru, bukan AutoMigrate.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
    id         uuid DEFAULT gen_random_uuid() NOT NULL,
    username   varchar(100) NOT NULL,
    password   varchar(255) NOT NULL,
    role       smallint DEFAULT 1,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT users_pkey PRIMARY KEY (id),
    CONSTRAINT chk_users_role CHECK (role IN (0, 1))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

CREATE TABLE IF NOT EXISTS customers (
    id         uuid DEFAULT gen_random_uuid() NOT NULL,
    name       varchar(150) NOT NULL,
    phone      varchar(15),
    address    text,
    created_at timestamptz,
    updated_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT customers_pkey PRIMARY KEY (id)
);
CREATE INDEX IF NOT EXISTS idx_customers_deleted_at ON customers (deleted_at);
CREATE INDEX IF NOT EXISTS idx_customers_keyset ON customers (created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_customers_name_trgm ON customers USING GIN (name gin_trgm_ops) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS products (
    id                 uuid DEFAULT gen_random_uuid() NOT NULL,
    sku                varchar(50),
    product_name       varchar(255) NOT NULL,
    unit               smallint NOT NULL,
    product_type       smallint NOT NULL DEFAULT 0,
    purchase_price     bigint NOT NULL,
    selling_price      bigint NOT NULL,
    selling_price_debt bigint NOT NULL,
    stock              numeric(10,2) DEFAULT 0,
    category           varchar(100),
    image              text,
    created_at         timestamptz,
    updated_at         timestamptz,
    deleted_at         timestamptz,
    CONSTRAINT products_pkey PRIMARY KEY (id),
    CONSTRAINT chk_products_unit CHECK (unit IN (0, 1, 2, 3, 4, 5)),
    CONSTRAINT chk_products_product_type CHECK (product_type IN (0, 1)),
    -- Backstop for the invariant the stock paths enforce under row locks.
    CONSTRAINT chk_products_stock_non_negative CHECK (stock >= 0)
);
-- Partial, like idx_transactions_no_invoice: a soft-deleted product must not
-- reserve its SKU forever. AddBulkProduct's ON CONFLICT repeats the predicate
-- so Postgres can infer this index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_sku ON products (sku) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_products_category ON products (category);
CREATE INDEX IF NOT EXISTS idx_products_deleted_at ON products (deleted_at);
CREATE INDEX IF NOT EXISTS idx_products_keyset ON products (created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_products_name_trgm ON products USING GIN (product_name gin_trgm_ops) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS debts (
    id             uuid DEFAULT gen_random_uuid() NOT NULL,
    customer_id    uuid NOT NULL,
    total_debt     bigint NOT NULL,
    remaining_debt bigint NOT NULL,
    status         smallint DEFAULT 0,
    due_date       date,
    created_at     timestamptz,
    updated_at     timestamptz,
    deleted_at     timestamptz,
    CONSTRAINT debts_pkey PRIMARY KEY (id),
    CONSTRAINT chk_debts_status CHECK (status IN (0, 1)),
    CONSTRAINT chk_debts_remaining_range CHECK (remaining_debt >= 0 AND remaining_debt <= total_debt),
    CONSTRAINT fk_customers_debts FOREIGN KEY (customer_id) REFERENCES customers (id)
);
CREATE INDEX IF NOT EXISTS idx_debts_deleted_at ON debts (deleted_at);
CREATE INDEX IF NOT EXISTS idx_debts_customer_id ON debts (customer_id);
CREATE INDEX IF NOT EXISTS idx_debts_status ON debts (status);
CREATE INDEX IF NOT EXISTS idx_debts_keyset ON debts (created_at, id) WHERE deleted_at IS NULL;
-- A SELECT ... FOR UPDATE in AddDebt/CreateTransaction only locks rows that
-- already exist, so two concurrent hutang sales for the same customer can
-- both see "no open debt" and both insert, leaving two independent open
-- debts. This partial unique index is the actual guard: the second concurrent
-- insert fails with a unique_violation, which the repository's retry layer
-- (see isRetryableTxError) retries — on retry it finds the row the first
-- transaction committed and updates it instead.
CREATE UNIQUE INDEX IF NOT EXISTS idx_debts_customer_open ON debts (customer_id) WHERE status = 0 AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS debt_payments (
    id            uuid DEFAULT gen_random_uuid() NOT NULL,
    debt_id       uuid NOT NULL,
    user_id       uuid NOT NULL,
    nominal_bayar bigint NOT NULL,
    tanggal_bayar timestamptz,
    -- idempotency_key, when set by the client, lets a retried PayDebt request
    -- (lost response, app crash) replay the original result instead of
    -- double-recording the payment.
    idempotency_key varchar(100),
    -- previous_remaining_debt is remaining_debt just before this payment was
    -- applied, so a replayed (idempotent) request can return the exact
    -- original receipt numbers instead of recomputing them from current state.
    previous_remaining_debt bigint NOT NULL DEFAULT 0,
    CONSTRAINT debt_payments_pkey PRIMARY KEY (id),
    CONSTRAINT fk_debts_debt_payments FOREIGN KEY (debt_id) REFERENCES debts (id),
    CONSTRAINT fk_debt_payments_user FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_debt_payments_debt_id ON debt_payments (debt_id);
CREATE INDEX IF NOT EXISTS idx_debt_payments_user_id ON debt_payments (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_debt_payments_idempotency
    ON debt_payments (debt_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS transactions (
    id                uuid DEFAULT gen_random_uuid() NOT NULL,
    no_invoice        varchar(50) NOT NULL,
    user_id           uuid NOT NULL,
    customer_id       uuid,
    debt_id           uuid,
    payment_type      smallint NOT NULL,
    bank              varchar(20),
    total_transaction bigint NOT NULL,
    created_at        timestamptz,
    updated_at        timestamptz,
    deleted_at        timestamptz,
    CONSTRAINT transactions_pkey PRIMARY KEY (id),
    CONSTRAINT chk_transactions_payment_type CHECK (payment_type IN (0, 1, 2, 3)),
    CONSTRAINT fk_customers_transactions FOREIGN KEY (customer_id) REFERENCES customers (id),
    CONSTRAINT fk_debts_transactions FOREIGN KEY (debt_id) REFERENCES debts (id),
    CONSTRAINT fk_users_transactions FOREIGN KEY (user_id) REFERENCES users (id)
);
-- Partial on purpose: transactions are soft-deleted, and a plain unique index
-- reserves an invoice number forever once its transaction is deleted — the
-- pre-insert check (which excludes soft-deleted rows) passes, then the INSERT
-- fails with a confusing "already exists" for an invoice the operator can no
-- longer see. Scoping it to live rows lets a cancelled invoice number be reused
-- while still stopping two live transactions from sharing one.
CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_no_invoice
    ON transactions (no_invoice) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_debt_id ON transactions (debt_id);
CREATE INDEX IF NOT EXISTS idx_transactions_deleted_at ON transactions (deleted_at);
CREATE INDEX IF NOT EXISTS idx_transactions_customer_id ON transactions (customer_id);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions (user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_keyset ON transactions (created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_invoice_trgm ON transactions USING GIN (no_invoice gin_trgm_ops) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transactions_detail (
    id             uuid DEFAULT gen_random_uuid() NOT NULL,
    transaction_id uuid NOT NULL,
    product_id     uuid NOT NULL,
    product_name   varchar(255) NOT NULL DEFAULT '',
    price          bigint NOT NULL,
    price_debt     bigint NOT NULL,
    qty            numeric(8,2) NOT NULL,
    subtotal       bigint NOT NULL,
    purchase_price bigint NOT NULL DEFAULT 0,
    destination    varchar(50),
    CONSTRAINT transactions_detail_pkey PRIMARY KEY (id),
    CONSTRAINT fk_transactions_transaction_detail FOREIGN KEY (transaction_id) REFERENCES transactions (id),
    CONSTRAINT fk_transactions_detail_product FOREIGN KEY (product_id) REFERENCES products (id)
);
CREATE INDEX IF NOT EXISTS idx_transactions_detail_transaction_id ON transactions_detail (transaction_id);
CREATE INDEX IF NOT EXISTS idx_transactions_detail_product_id ON transactions_detail (product_id);

CREATE TABLE IF NOT EXISTS payments (
    id               uuid DEFAULT gen_random_uuid() NOT NULL,
    order_id         varchar(50) NOT NULL,
    user_id          uuid NOT NULL,
    customer_id      uuid,
    method           varchar(20) NOT NULL,
    -- gross_amount is what the buyer pays = subtotal_amount + fee_amount. The
    -- parts are stored separately so the books reconcile: the sale is worth the
    -- subtotal, the channel fee is only passed through, and drift between them
    -- is detectable instead of silently folded into one number.
    subtotal_amount  bigint NOT NULL DEFAULT 0,
    fee_amount       bigint NOT NULL DEFAULT 0,
    gross_amount     bigint NOT NULL,
    status           varchar(20) NOT NULL DEFAULT 'pending',
    midtrans_trx_id  varchar(100),
    midtrans_status  varchar(30),
    fraud_status     varchar(30),
    qr_string        text,
    qr_url           text,
    redirect_url     text,
    items            jsonb,
    expiry_time      timestamptz,
    paid_at          timestamptz,
    transaction_id   uuid,
    stock_reserved   boolean NOT NULL DEFAULT false,
    va_bank          varchar(20),
    va_number        varchar(50),
    bill_key         varchar(50),
    biller_code      varchar(20),
    created_at       timestamptz,
    updated_at       timestamptz,
    deleted_at       timestamptz,
    CONSTRAINT payments_pkey PRIMARY KEY (id),
    -- 'settling' is the hand-off state between claiming a settled payment
    -- (phase 1, under a row lock) and creating its sales transaction (phase 2,
    -- on its own connection). Without it, a crash between the phases is
    -- indistinguishable from a payment that was never paid at all.
    CONSTRAINT chk_payments_status CHECK (status IN ('pending', 'settling', 'success', 'failed', 'expired')),
    CONSTRAINT fk_payments_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_payments_customer FOREIGN KEY (customer_id) REFERENCES customers (id),
    CONSTRAINT fk_payments_transaction FOREIGN KEY (transaction_id) REFERENCES transactions (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_order_id ON payments (order_id);
-- The reconciliation sweep scans by status on every tick, least recently
-- touched first: a payment it fails on is touched and rotates to the back.
CREATE INDEX IF NOT EXISTS idx_payments_status_updated_at ON payments (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_payments_deleted_at ON payments (deleted_at);
CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments (user_id);
CREATE INDEX IF NOT EXISTS idx_payments_customer_id ON payments (customer_id);
CREATE INDEX IF NOT EXISTS idx_payments_transaction_id ON payments (transaction_id);

CREATE TABLE IF NOT EXISTS device_tokens (
    id           uuid DEFAULT gen_random_uuid() NOT NULL,
    token        text NOT NULL,
    user_id      uuid,
    device_id    text NOT NULL,
    platform     varchar(16) NOT NULL,
    created_at   timestamptz,
    updated_at   timestamptz,
    last_used_at timestamptz,
    CONSTRAINT device_tokens_pkey PRIMARY KEY (id),
    CONSTRAINT fk_device_tokens_user FOREIGN KEY (user_id) REFERENCES users (id)
);
-- Unique index pada token diperlukan oleh upsert ON CONFLICT (token) di repository.
CREATE UNIQUE INDEX IF NOT EXISTS idx_device_tokens_token ON device_tokens (token);
CREATE INDEX IF NOT EXISTS idx_device_tokens_user_id ON device_tokens (user_id);

-- +goose Down
DROP TABLE IF EXISTS device_tokens;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS transactions_detail;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS debt_payments;
DROP TABLE IF EXISTS debts;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS users;
