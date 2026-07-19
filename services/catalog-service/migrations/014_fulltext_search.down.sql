-- Reverte a busca por tsvector. A ordem importa: a coluna gerada DEPENDE da
-- configuração de busca `utilar_pt` (dependência registrada no catálogo), então
-- a config só pode cair depois da coluna.

DROP TRIGGER IF EXISTS trg_sellers_name_to_products ON sellers;
DROP FUNCTION IF EXISTS sellers_propagate_name_to_products();

DROP TRIGGER IF EXISTS trg_products_seller_name_cache ON products;
DROP FUNCTION IF EXISTS products_fill_seller_name_cache();

DROP INDEX IF EXISTS idx_products_search_vector;

ALTER TABLE products DROP COLUMN IF EXISTS search_vector;
ALTER TABLE products DROP COLUMN IF EXISTS seller_name_cache;

DROP TEXT SEARCH CONFIGURATION IF EXISTS utilar_pt;

-- A extensão `unaccent` FICA de propósito. Ela é instalada no banco inteiro e
-- pode ter outros usos; um `DROP EXTENSION` aqui reverteria mais do que esta
-- migration criou. Extensão instalada e sem uso não custa nada.
