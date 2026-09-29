CREATE DATABASE shop;
\connect shop
CREATE TABLE IF NOT EXISTS products (
    sku TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    price_cents INT NOT NULL,
    stock INT NOT NULL
);
INSERT INTO products (sku, name, price_cents, stock) VALUES
    ('sku-100', 'Field notebook', 1800, 50),
    ('sku-200', 'Ink cartridge', 900, 40),
    ('sku-300', 'Desk lamp', 4200, 15)
ON CONFLICT (sku) DO NOTHING;
CREATE TABLE IF NOT EXISTS orders (
    id TEXT PRIMARY KEY,
    sku TEXT NOT NULL,
    qty INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
