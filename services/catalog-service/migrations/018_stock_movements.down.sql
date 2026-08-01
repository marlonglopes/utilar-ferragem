-- Reverte 018. O histórico de movimento é PERDIDO (é o ponto de um down).
DROP TABLE IF EXISTS stock_movements;
ALTER TABLE products DROP COLUMN IF EXISTS low_stock_threshold;
