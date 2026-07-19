-- Reverte 016. Os índices criados sobre `stock_reservations` (tabela da 004)
-- são derrubados aqui explicitamente — DROP TABLE não os leva junto porque a
-- tabela não é desta migration.

DROP TRIGGER IF EXISTS trg_product_complement_rules_updated ON product_complement_rules;
DROP TABLE IF EXISTS product_complement_rules;

DROP INDEX IF EXISTS idx_stock_reservations_committed_basket;
DROP INDEX IF EXISTS idx_stock_reservations_committed_window;

DROP TABLE IF EXISTS copurchase_refresh_state;
DROP INDEX IF EXISTS idx_copurchase_lookup;
DROP TABLE IF EXISTS product_copurchase;
