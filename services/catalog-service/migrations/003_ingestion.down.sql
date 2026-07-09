DROP INDEX IF EXISTS idx_products_status;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_status_check;
ALTER TABLE products DROP COLUMN IF EXISTS status;
DROP INDEX IF EXISTS idx_products_sku;
ALTER TABLE products DROP COLUMN IF EXISTS sku;
