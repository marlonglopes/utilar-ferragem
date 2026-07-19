-- Reverte 015. Ordem inversa: gatilhos → funções → restauração do agregado →
-- coluna → tabelas.

DROP TRIGGER IF EXISTS trg_product_reviews_aggregate ON product_reviews;
DROP TRIGGER IF EXISTS trg_product_reviews_updated ON product_reviews;
DROP FUNCTION IF EXISTS product_reviews_sync_aggregate();
DROP FUNCTION IF EXISTS product_reviews_recalc(UUID);

-- Restaura os agregados que existiam ANTES do backfill destrutivo da .up.
-- Sem isto o rollback deixaria a loja com todo produto em 0 avaliações e
-- nenhuma tabela de avaliações — o pior dos dois mundos.
UPDATE products p
   SET rating       = b.rating,
       review_count = b.review_count
  FROM products_rating_pre_reviews_backup b
 WHERE b.product_id = p.id;

DROP INDEX IF EXISTS idx_products_published_bayes;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_rating_bayes_range;
ALTER TABLE products DROP COLUMN IF EXISTS rating_bayes;

DROP TABLE IF EXISTS products_rating_pre_reviews_backup;
DROP TABLE IF EXISTS product_reviews;
