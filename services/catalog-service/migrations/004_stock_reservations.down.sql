DROP TRIGGER IF EXISTS trg_stock_reservations_updated ON stock_reservations;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_stock_non_negative;
DROP TABLE IF EXISTS stock_reservations;
