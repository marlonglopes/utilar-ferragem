-- ============================================================================
-- Preço por faixa (atacado) + histórico de preço
-- ============================================================================

-- ----------------------------------------------------------------------------
-- product_price_tiers
-- ----------------------------------------------------------------------------
-- PORQUÊ: o cliente profissional (pedreiro, empreiteiro, instalador) é o de
-- ticket alto e hoje paga o mesmo preço de quem leva 1 unidade. Sem faixa de
-- atacado, esse cliente compra no concorrente que dá.
--
-- Modelo escolhido — "preço a partir de N": cada faixa diz "comprando min_qty
-- ou mais, o unitário é `price`". A resolução pega a MAIOR min_qty que a
-- quantidade pedida alcança. É o modelo que o balcão já usa verbalmente ("de
-- 10 pra cima sai por X") e evita faixas com fim (min/max) que sempre acabam
-- deixando buraco entre elas por erro de digitação.
CREATE TABLE IF NOT EXISTS product_price_tiers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    min_qty    NUMERIC(14,3) NOT NULL CHECK (min_qty > 0),
    price      NUMERIC(12,2) NOT NULL CHECK (price >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Duas faixas com o mesmo min_qty tornariam o preço não-determinístico.
    UNIQUE (product_id, min_qty)
);

-- A resolução de preço sempre varre as faixas de um produto em ordem de min_qty.
CREATE INDEX IF NOT EXISTS idx_price_tiers_product
    ON product_price_tiers(product_id, min_qty);

CREATE TRIGGER trg_price_tiers_updated
    BEFORE UPDATE ON product_price_tiers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ----------------------------------------------------------------------------
-- product_price_history
-- ----------------------------------------------------------------------------
-- PORQUÊ: hoje o preço é sobrescrito e não sobra rastro. Responde "por que
-- esse produto está R$ 12?" e — o motivo caro — detecta o erro de vírgula na
-- importação (`1.234,56` lido como `1,23`), que é o modo de falha mais caro do
-- catálogo. Com o histórico dá pra alertar em queda > X% e reverter.
--
-- Desenhada em docs/ingestao-de-produtos.md. `batch_id` fica SEM foreign key
-- porque `import_batches` ainda não existe (Fase B da ingestão) — a coluna já
-- entra pra não precisar de migration de dado depois.
CREATE TABLE IF NOT EXISTS product_price_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    price      NUMERIC(12,2) NOT NULL,
    cost       NUMERIC(12,2),
    -- old_* permite calcular a variação sem precisar de window function e sem
    -- depender de haver linha anterior (produto novo não tem).
    old_price  NUMERIC(12,2),
    old_cost   NUMERIC(12,2),
    source     TEXT NOT NULL CHECK (source IN ('import', 'admin', 'api', 'seed')),
    batch_id   UUID,
    changed_by TEXT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_price_history_product
    ON product_price_history(product_id, changed_at DESC);
