DROP TRIGGER IF EXISTS trg_shipping_rates_updated ON shipping_rates;
DROP TABLE IF EXISTS shipping_rates;
DROP TABLE IF EXISTS processed_payment_events;
ALTER TABLE orders DROP COLUMN IF EXISTS shipping_service;
ALTER TABLE orders DROP COLUMN IF EXISTS stock_reserved;
