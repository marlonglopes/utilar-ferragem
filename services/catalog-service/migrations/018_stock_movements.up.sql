-- ============================================================================
-- Estoque — ajuste com motivo, histórico de movimento e alerta de baixo
-- ----------------------------------------------------------------------------
-- POR QUÊ: hoje só dá pra SOBRESCREVER o número absoluto do estoque (PATCH do
-- produto). Não há motivo, não há histórico, não há alerta. É a tela do
-- almoxarife (persona de backoffice): ele conta a prateleira, dá entrada de
-- recebimento e baixa de avaria — cada movimento com MOTIVO e rastro de quem/
-- quando. Ver docs/backoffice-personas.md e docs/estoque.md.
--
-- Reversível.
-- ============================================================================

-- Limite de "estoque baixo" por produto. Default 5: número conservador que a
-- loja ajusta por item depois (parafuso a granel tem limite diferente de
-- furadeira). NUMERIC(14,3) igual a `stock` — venda fracionada (2,5 m) existe.
ALTER TABLE products
    ADD COLUMN low_stock_threshold NUMERIC(14,3) NOT NULL DEFAULT 5;

-- Histórico de movimento. Cada linha é UM ajuste: quanto entrou/saiu (delta),
-- por quê (reason, obrigatório), e o estoque RESULTANTE (redundante de
-- propósito — permite reconciliar a série sem recomputar, e detecta divergência
-- se alguém mexeu no `stock` por fora deste caminho).
CREATE TABLE stock_movements (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id       UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    delta            NUMERIC(14,3) NOT NULL,        -- + entrada / - saída-ajuste
    reason           TEXT NOT NULL CHECK (btrim(reason) <> ''),
    resulting_stock  NUMERIC(14,3) NOT NULL CHECK (resulting_stock >= 0),
    actor_id         TEXT,
    actor_role       TEXT,
    request_id       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Consulta quente: histórico de um produto, do mais recente.
CREATE INDEX idx_stock_mov_product ON stock_movements(product_id, created_at DESC);
