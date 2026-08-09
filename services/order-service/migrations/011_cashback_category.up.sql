-- ============================================================================
-- Cashback por CATEGORIA (taxa de acúmulo diferente por linha de produto)
-- ----------------------------------------------------------------------------
-- Pra acumular por categoria, o acúmulo (que roda no consumer, ao pagar) precisa
-- saber a categoria de cada item — e o item do pedido não guardava isso. Duas
-- coisas:
--   1. order_items.category_id: capturado no Create a partir do produto
--      autoritativo do catálogo (mesma fonte do preço). '' quando desconhecido.
--   2. cashback_category_rates: override de taxa por categoria. Categoria sem
--      override usa a taxa base (ou a da campanha, se ativa).
--
-- Reversível.
-- ============================================================================
ALTER TABLE order_items ADD COLUMN category_id TEXT NOT NULL DEFAULT '';

CREATE TABLE cashback_category_rates (
    category_id TEXT PRIMARY KEY,
    rate_pct    NUMERIC(5,2) NOT NULL CHECK (rate_pct >= 0 AND rate_pct <= 100),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  TEXT NOT NULL DEFAULT ''
);
