-- ============================================================================
-- Estorno no PSP — guarda de idempotência (nunca estornar em dobro)
-- ----------------------------------------------------------------------------
-- POR QUÊ: quando uma devolução é estornada, o order-service pede ao payment o
-- ESTORNO REAL no PSP (POST /internal/v1/refunds). Estornar duas vezes devolve
-- dinheiro em dobro — então a operação precisa ser idempotente, chaveada pela
-- DEVOLUÇÃO (return_id), NÃO pelo pedido (um pedido tem N devoluções parciais).
--
-- Fluxo do handler: RESERVA a linha (status='requested') ANTES de chamar o PSP;
-- um retry concorrente colide no PK e vira no-op. Se o PSP falhar, a reserva é
-- desfeita para o retry poder tentar. Confirmado (síncrono) ou pela chegada do
-- webhook, o status vira 'refunded'.
--
-- NÃO é lançamento contábil — o livro é lançado pelo order-service, keyed por
-- return_id, fonte única, para nunca contar o estorno duas vezes.
--
-- Reversível.
-- ============================================================================
CREATE TABLE psp_refunds (
    return_id      TEXT PRIMARY KEY,
    order_id       UUID NOT NULL,
    psp_payment_id TEXT NOT NULL,
    amount_cents   BIGINT NOT NULL CHECK (amount_cents > 0),
    total          BOOLEAN NOT NULL,
    status         TEXT NOT NULL,          -- requested | refunded
    psp_refund_id  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_psp_refunds_order ON psp_refunds(order_id);
