-- ============================================================================
-- Programa de CASHBACK (dinheiro que a loja devolve ao cliente)
-- ----------------------------------------------------------------------------
-- Cashback é PASSIVO da loja (dívida com o cliente), então o modelo trata como
-- dinheiro de verdade: valores autoritativos no servidor, idempotência por
-- pedido, histórico append-only e saldo derivado de LOTES com validade — não de
-- um contador solto que pode divergir.
--
-- Regras (decisão do dono, 2026-08): acúmulo = % sobre o valor de mercadoria
-- pago; credita ao pagar (reverte na devolução); validade 90 dias; resgate até
-- 50% do pedido. Tudo isso vive em `cashback_config` (singleton), editável no
-- admin sem deploy.
--
-- Reversível.
-- ============================================================================

-- Config singleton (id travado em 1, como store_settings do catalog).
CREATE TABLE cashback_config (
    id             INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    active         BOOLEAN NOT NULL DEFAULT false,
    earn_rate_pct  NUMERIC(5,2) NOT NULL DEFAULT 5   CHECK (earn_rate_pct  >= 0 AND earn_rate_pct  <= 100),
    redeem_max_pct NUMERIC(5,2) NOT NULL DEFAULT 50  CHECK (redeem_max_pct >= 0 AND redeem_max_pct <= 100),
    validity_days  INTEGER NOT NULL DEFAULT 90 CHECK (validity_days > 0),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by     TEXT NOT NULL DEFAULT ''
);
-- Semeia já LIGADO com os padrões escolhidos: 5% acumula, resgate até 50%, 90d.
-- (O dono liga/desliga e ajusta as taxas no admin.)
INSERT INTO cashback_config (id, active) VALUES (1, true) ON CONFLICT (id) DO NOTHING;

-- LOTES: cada acúmulo é um lote com validade e saldo restante. O saldo do cliente
-- é a soma do `remaining` dos lotes vivos (não vencidos). Um lote por pedido
-- (UNIQUE em order_id) — é isso que torna o acúmulo idempotente: replay do evento
-- de pagamento bate na constraint e não credita duas vezes.
CREATE TABLE cashback_lots (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT NOT NULL,
    order_id    TEXT NOT NULL UNIQUE,
    earned      NUMERIC(12,2) NOT NULL CHECK (earned >= 0),
    remaining   NUMERIC(12,2) NOT NULL CHECK (remaining >= 0 AND remaining <= earned),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Saldo por cliente: soma de remaining dos lotes vivos. Índice pro cálculo e pra
-- consumo FIFO (vence primeiro, gasta primeiro).
CREATE INDEX idx_cashback_lots_customer_live ON cashback_lots (customer_id, expires_at)
    WHERE remaining > 0;

-- HISTÓRICO append-only (o que o cliente vê em /conta/cashback e a auditoria).
-- Valor ASSINADO: earn positivo; redeem/reverse/expire negativos.
CREATE TABLE cashback_entries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT NOT NULL,
    order_id    TEXT,
    kind        TEXT NOT NULL CHECK (kind IN ('earn', 'redeem', 'reverse', 'expire')),
    amount      NUMERIC(12,2) NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_cashback_entries_customer ON cashback_entries (customer_id, created_at DESC);

-- Quanto de cashback o pedido resgatou (pra reverter certo e pra mostrar na nota).
ALTER TABLE orders ADD COLUMN cashback_redeemed NUMERIC(12,2) NOT NULL DEFAULT 0;
