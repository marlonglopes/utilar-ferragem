-- ============================================================================
-- Fulfillment (consumo de eventos de pagamento) + frete server-side
-- ============================================================================

-- ---------------------------------------------------------------------------
-- processed_payment_events — idempotência do consumer Kafka
-- ---------------------------------------------------------------------------
-- PORQUÊ: o outbox do payment-service é at-least-once por construção (publica,
-- depois marca published_at; se cair no meio, republica). O mesmo
-- `payment.confirmed` chega várias vezes. Sem esta tabela, cada reentrega
-- reescreveria paid_at e inseriria outro tracking_event — a timeline do cliente
-- viraria "Pagamento confirmado" cinco vezes.
--
-- A chave é (payment_id, event_type): um pagamento pode legitimamente gerar um
-- 'payment.confirmed' e depois um 'payment.cancelled' (estorno), e queremos
-- processar os dois. O que não pode é processar o MESMO tipo duas vezes.
--
-- O INSERT desta linha acontece na MESMA transação que muda o status do pedido.
-- Ou os dois acontecem, ou nenhum: não existe "marquei como processado mas não
-- apliquei" nem o contrário.
CREATE TABLE processed_payment_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id    TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    order_id      UUID REFERENCES orders(id) ON DELETE SET NULL,
    outcome       TEXT NOT NULL,            -- 'applied' | 'ignored' | 'order_not_found'
    detail        TEXT,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_processed_payment_events_key
    ON processed_payment_events(payment_id, event_type);

CREATE INDEX idx_processed_payment_events_order
    ON processed_payment_events(order_id) WHERE order_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- shipping_rates — tabela de frete server-side
-- ---------------------------------------------------------------------------
-- PORQUÊ: o total era `subtotal + req.ShippingCost`, com ShippingCost vindo do
-- cliente. Frete agora sai daqui.
--
-- Faixa de CEP × valor do carrinho. Sem peso porque o catálogo não tem coluna
-- de peso; `cost_per_item` aproxima o volume até lá.
CREATE TABLE shipping_rates (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_name      TEXT NOT NULL,
    cep_start      INT  NOT NULL CHECK (cep_start BETWEEN 0 AND 99999999),
    cep_end        INT  NOT NULL CHECK (cep_end   BETWEEN 0 AND 99999999),
    service_code   TEXT NOT NULL,           -- 'standard' | 'express'
    service_name   TEXT NOT NULL,
    base_cost      NUMERIC(12,2) NOT NULL CHECK (base_cost >= 0),
    cost_per_item  NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (cost_per_item >= 0),
    delivery_days  INT  NOT NULL CHECK (delivery_days > 0),
    free_above     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (free_above >= 0),
    active         BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT shipping_rates_range CHECK (cep_end >= cep_start)
);

CREATE INDEX idx_shipping_rates_lookup ON shipping_rates(cep_start, cep_end) WHERE active;

CREATE TRIGGER trg_shipping_rates_updated
    BEFORE UPDATE ON shipping_rates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- orders: colunas de frete e reserva
-- ---------------------------------------------------------------------------
-- shipping_service: qual opção o cliente escolheu (para recotar e auditar).
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_service TEXT NOT NULL DEFAULT 'standard';
-- stock_reserved: o catalog-service confirmou a reserva deste pedido? Serve pra
-- não tentar liberar/commitar reserva de pedido que nunca reservou (pedidos
-- criados antes desta migration, ou criados com catalog fora do ar).
ALTER TABLE orders ADD COLUMN IF NOT EXISTS stock_reserved BOOLEAN NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- Seed de faixas — loja de ferragem sediada em São Paulo capital
-- ---------------------------------------------------------------------------
-- Faixas de CEP conforme a divisão dos Correios:
--   01000-000..05999-999  São Paulo capital
--   06000-000..09999-999  Grande SP / ABC / Osasco / Guarulhos
--   10000-000..19999-999  Interior de SP
--   20000-000..39999-999  Sudeste (RJ/ES/MG)
--   40000-000..99999-999  Demais regiões
--
-- Frete grátis acima de R$ 299 na capital e Grande SP (o limiar é por faixa —
-- o operador ajusta na tabela sem deploy). Regiões distantes não têm grátis:
-- o custo de mandar 30kg de cimento pro Norte não fecha.
INSERT INTO shipping_rates
    (zone_name, cep_start, cep_end, service_code, service_name, base_cost, cost_per_item, delivery_days, free_above)
VALUES
    ('São Paulo - Capital',  1000000,  5999999, 'standard', 'Entrega padrão',   19.90,  2.50,  2, 299.00),
    ('São Paulo - Capital',  1000000,  5999999, 'express',  'Entrega expressa', 39.90,  4.00,  1,      0),
    ('Grande São Paulo',     6000000,  9999999, 'standard', 'Entrega padrão',   24.90,  3.00,  3, 299.00),
    ('Grande São Paulo',     6000000,  9999999, 'express',  'Entrega expressa', 49.90,  5.00,  2,      0),
    ('Interior de SP',      10000000, 19999999, 'standard', 'Entrega padrão',   34.90,  3.50,  5, 499.00),
    ('Interior de SP',      10000000, 19999999, 'express',  'Entrega expressa', 69.90,  6.00,  3,      0),
    ('Sudeste',             20000000, 39999999, 'standard', 'Entrega padrão',   49.90,  4.50,  7, 799.00),
    ('Sul e Centro-Oeste',  70000000, 89999999, 'standard', 'Entrega padrão',   64.90,  5.50, 10,      0),
    ('Nordeste',            40000000, 65999999, 'standard', 'Entrega padrão',   79.90,  6.50, 12,      0),
    ('Norte',               66000000, 69999999, 'standard', 'Entrega padrão',   99.90,  8.00, 15,      0),
    ('Sul',                 90000000, 99999999, 'standard', 'Entrega padrão',   64.90,  5.50, 10,      0);
