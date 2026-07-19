-- Índices da listagem de produtos: `ORDER BY created_at DESC` é o default de
-- TODA listagem pública (product.go, `orderBy` cai em "p.created_at DESC"
-- quando sort é vazio ou "newest") e não tinha NENHUM índice que o servisse.
--
-- Medido em base de 150.000 produtos (92% published), não nos ~400 de dev —
-- com 400 linhas o planejador escolhe seq scan mesmo com o índice certo, e a
-- conclusão não valeria nada.
--
--   Listagem default (published + created_at DESC LIMIT 24):
--     ANTES:  Parallel Seq Scan + top-N heapsort de 138.000 linhas
--             33,9 ms / 6.110 buffers
--     DEPOIS: Index Scan em idx_products_published_created
--              0,32 ms /    67 buffers      → 106x mais rápido, 91x menos I/O
--
--   Listagem por categoria (published + category_id + created_at DESC):
--     ANTES:  Bitmap Heap Scan de 17.250 linhas + top-N heapsort
--             20,2 ms / 5.949 buffers
--     DEPOIS: Index Scan em idx_products_published_cat_created
--              0,34 ms /    65 buffers      →  59x mais rápido, 91x menos I/O
--
-- Os dois são PARCIAIS em status='published' porque é o filtro fixo da rota
-- pública (`where := []string{"p.status = 'published'"}`). Parcial deixa o
-- índice ~8% menor e não penaliza o admin, que lista por outros status e
-- continua caindo em idx_products_status.
--
-- O índice de categoria não torna o primeiro redundante: a home lista sem
-- filtro de categoria, e aí (category_id, created_at) não pode ser usado pra
-- ordenar — a primeira coluna não está no WHERE.
--
-- ⚠️ PRODUÇÃO: golang-migrate roda cada migration DENTRO de uma transação, e
-- CREATE INDEX CONCURRENTLY não pode rodar em transação. Aqui vai CREATE INDEX
-- normal, que trava ESCRITA em products enquanto constrói (leitura continua).
-- Em 150k linhas levou ~400 ms. Se a tabela já estiver grande e a loja não
-- puder parar de vender, aplique à mão ANTES de subir o serviço:
--
--   CREATE INDEX CONCURRENTLY idx_products_published_created
--     ON products (created_at DESC) WHERE status = 'published';
--   CREATE INDEX CONCURRENTLY idx_products_published_cat_created
--     ON products (category_id, created_at DESC) WHERE status = 'published';
--   INSERT INTO schema_migrations (version, dirty) VALUES (13, false)
--     ON CONFLICT (version) DO UPDATE SET dirty = false;
--
-- (sem o INSERT o migrate tenta criar de novo e falha; com dirty=true o
-- serviço se recusa a subir.)

CREATE INDEX IF NOT EXISTS idx_products_published_created
  ON products (created_at DESC)
  WHERE status = 'published';

CREATE INDEX IF NOT EXISTS idx_products_published_cat_created
  ON products (category_id, created_at DESC)
  WHERE status = 'published';
