-- Reverte 009_import_pipeline.
--
-- Ordem: dependentes primeiro. As constraints/colunas adicionadas em `products`
-- saem antes das tabelas porque a FK de price_history aponta pra import_batches.

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_published_needs_review;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_source_check;
DROP INDEX IF EXISTS idx_products_source;
ALTER TABLE products DROP COLUMN IF EXISTS price_reviewed;
ALTER TABLE products DROP COLUMN IF EXISTS source;

-- Devolve batch_id ao estado da 006 (coluna sem FK), preservando os dados de
-- histórico — que não devem sumir só porque a ingestão foi revertida.
ALTER TABLE product_price_history DROP CONSTRAINT IF EXISTS price_history_batch_fk;

DROP TABLE IF EXISTS sinapi_composition_items;
DROP TABLE IF EXISTS sinapi_compositions;
DROP TABLE IF EXISTS import_rows;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS import_profiles;
