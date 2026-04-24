-- ============================================================================
-- Order service schema
-- ----------------------------------------------------------------------------
-- Referência para produtos e pagamentos é feita via IDs opacos (TEXT/UUID),
-- sem FK cross-DB. A integridade é verificada no nível da aplicação quando
-- a ordem é criada (consulta HTTP ao catalog-service).
-- ============================================================================

CREATE TYPE order_status AS ENUM (
    'pending_payment',
    'paid',
    'picking',
    'shipped',
    'delivered',
    'cancelled'
);

CREATE TYPE payment_method AS ENUM ('pix', 'boleto', 'card');

-- ---------------------------------------------------------------------------
-- orders
-- ---------------------------------------------------------------------------
CREATE TABLE orders (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number         TEXT NOT NULL UNIQUE,               -- humano: '2026-0042'
    user_id        TEXT NOT NULL,                       -- opaco, vem do auth-service (futuro). TEXT para permitir IDs de dev/mock (não é FK)
    status         order_status NOT NULL DEFAULT 'pending_payment',
    payment_method payment_method NOT NULL,
    payment_id     UUID,                                -- opaco, vem do payment-service (set após confirmação)
    payment_info   TEXT,                                -- human-readable: "Pix · pago em 10/04 14:32"
    subtotal       NUMERIC(12,2) NOT NULL,
    shipping_cost  NUMERIC(12,2) NOT NULL DEFAULT 0,
    total          NUMERIC(12,2) NOT NULL,
    tracking_code  TEXT,                                -- código dos Correios
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at        TIMESTAMPTZ,
    picked_at      TIMESTAMPTZ,
    shipped_at     TIMESTAMPTZ,
    delivered_at   TIMESTAMPTZ,
    cancelled_at   TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id    ON orders(user_id);
CREATE INDEX idx_orders_status     ON orders(status);
CREATE INDEX idx_orders_payment_id ON orders(payment_id) WHERE payment_id IS NOT NULL;
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);

-- ---------------------------------------------------------------------------
-- order_items — snapshot de preço/nome no momento da compra
-- ---------------------------------------------------------------------------
CREATE TABLE order_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   UUID NOT NULL,                         -- opaco, ref ao catalog-service
    name         TEXT NOT NULL,                          -- snapshot para o caso do produto ser alterado depois
    icon         TEXT NOT NULL,
    seller_id    TEXT NOT NULL,
    seller_name  TEXT NOT NULL,
    quantity     INT NOT NULL CHECK (quantity > 0),
    unit_price   NUMERIC(12,2) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_items_order ON order_items(order_id);

-- ---------------------------------------------------------------------------
-- shipping_addresses — 1-1 com orders (embedded, mas em tabela própria
-- para permitir histórico quando usuário editar endereço sem afetar pedidos)
-- ---------------------------------------------------------------------------
CREATE TABLE shipping_addresses (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    street        TEXT NOT NULL,
    number        TEXT NOT NULL,
    complement    TEXT,
    neighborhood  TEXT NOT NULL,
    city          TEXT NOT NULL,
    state         CHAR(2) NOT NULL,
    cep           TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- tracking_events — histórico da jornada do pedido
-- ---------------------------------------------------------------------------
CREATE TABLE tracking_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    status      order_status NOT NULL,
    location    TEXT,                                  -- "Centro de distribuição SP"
    description TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tracking_events_order ON tracking_events(order_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Triggers — manter updated_at consistente
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_orders_updated BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();
