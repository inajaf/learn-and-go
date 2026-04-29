-- Migration 1: creating tables
--
-- UP: apply (create tables)
-- DOWN: roll back (drop tables) - file 000001_init.down.sql

-- ─────────────────────────────────────────────────────────────
-- customers - clients
-- Use TEXT for ID instead of UUID - flexible for any format
-- (UUID, ULID, Snowflake, "cust-123", etc.)
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS customers (
    id         TEXT         PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    email      VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────
-- orders - orders
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS orders (
    id           TEXT          PRIMARY KEY,
    customer_id  TEXT          NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    status       VARCHAR(50)   NOT NULL DEFAULT 'pending',
    total_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    notes        TEXT,
    -- Soft delete: запись не удаляется, только помечается
    deleted_at   TIMESTAMPTZ,
    -- Оптимистичная блокировка: обновление только если version совпадает
    version      INT           NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT orders_status_check CHECK (status IN ('pending','confirmed','cancelled'))
);

-- ─────────────────────────────────────────────────────────────
-- order_items — order items
-- ─────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS order_items (
    id          TEXT          PRIMARY KEY,
    order_id    TEXT          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id  VARCHAR(100)  NOT NULL,
    name        VARCHAR(255)  NOT NULL,
    quantity    INT           NOT NULL CHECK (quantity > 0),
    unit_price  NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────
-- Indexes - speed up searching
-- ─────────────────────────────────────────────────────────────

-- Frequently requested: specific customer orders
CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id);

-- Frequently requested: orders by status (active only)
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status) WHERE deleted_at IS NULL;

-- Frequently requested: specific order items
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items(order_id);

