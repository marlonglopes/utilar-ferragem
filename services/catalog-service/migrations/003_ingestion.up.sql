-- Ingestão de produtos (Fase A): SKU como chave de negócio para upsert +
-- workflow de publicação (draft → published → archived).

-- SKU: chave estável de importação. Nullable (produtos legados/seed não têm),
-- único quando presente. O importador CSV faz upsert por SKU.
ALTER TABLE products ADD COLUMN IF NOT EXISTS sku TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_sku ON products(sku) WHERE sku IS NOT NULL;

-- status: só 'published' aparece na loja. Produtos existentes (seed) já entram
-- publicados pra não sumir da vitrine. Rascunhos e arquivados ficam ocultos.
ALTER TABLE products ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'published';
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_status_check;
ALTER TABLE products ADD CONSTRAINT products_status_check
    CHECK (status IN ('draft', 'published', 'archived'));

-- Índice para o filtro da vitrine (status='published' em quase toda query pública).
CREATE INDEX IF NOT EXISTS idx_products_status ON products(status);
