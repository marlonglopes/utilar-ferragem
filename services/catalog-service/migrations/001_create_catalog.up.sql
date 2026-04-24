CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Categorias do marketplace (8 fixas da taxonomia; parent_id reservado para futuro)
CREATE TABLE categories (
    id         TEXT PRIMARY KEY,           -- slug: 'ferramentas', 'construcao', ...
    name       TEXT NOT NULL,
    icon       TEXT NOT NULL,              -- glyph unicode
    parent_id  TEXT REFERENCES categories(id),
    sort_order INT  NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_categories_parent ON categories(parent_id) WHERE parent_id IS NOT NULL;

-- Vendedores (lojas) do marketplace
CREATE TABLE sellers (
    id            TEXT PRIMARY KEY,        -- slug: 'ferragem-silva', ...
    name          TEXT NOT NULL,
    rating        NUMERIC(3,2) NOT NULL DEFAULT 0,
    review_count  INT NOT NULL DEFAULT 0,
    verified      BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Produtos
CREATE TYPE product_badge AS ENUM ('discount', 'free_shipping', 'last_units');

CREATE TABLE products (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    category_id     TEXT NOT NULL REFERENCES categories(id),
    seller_id       TEXT NOT NULL REFERENCES sellers(id),
    price           NUMERIC(12,2) NOT NULL,
    original_price  NUMERIC(12,2),
    currency        CHAR(3) NOT NULL DEFAULT 'BRL',
    icon            TEXT NOT NULL,
    brand           TEXT,
    stock           INT NOT NULL DEFAULT 0,
    rating          NUMERIC(2,1) NOT NULL DEFAULT 0,
    review_count    INT NOT NULL DEFAULT 0,
    cashback_amount NUMERIC(12,2),
    badge           product_badge,
    badge_label     TEXT,
    installments    INT,
    description     TEXT,
    specs           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_category  ON products(category_id);
CREATE INDEX idx_products_seller    ON products(seller_id);
CREATE INDEX idx_products_brand     ON products(brand) WHERE brand IS NOT NULL;
CREATE INDEX idx_products_price     ON products(price);
-- Busca textual simples (nome + descrição) — upgrade para tsvector vem na Sprint 17
CREATE INDEX idx_products_name_trgm ON products USING gin (name gin_trgm_ops);

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Imagens de produto (ordem controlada por sort_order; primeira = hero)
CREATE TABLE product_images (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    alt        TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_product_images_product ON product_images(product_id, sort_order);

-- Trigger genérico para manter updated_at em products/sellers
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_products_updated  BEFORE UPDATE ON products  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_sellers_updated   BEFORE UPDATE ON sellers   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
