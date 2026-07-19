-- Reverte os índices do painel administrativo. As consultas continuam
-- funcionando sem eles — voltam a ser seq scan, que é lento, não incorreto.
DROP INDEX IF EXISTS idx_orders_balcao_paid;
DROP INDEX IF EXISTS idx_orders_stuck;
DROP INDEX IF EXISTS idx_orders_paid_at;
