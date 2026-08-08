-- ============================================================================
-- Cupons de desconto (WEB) — alavanca de conversão
-- ----------------------------------------------------------------------------
-- POR QUÊ: hoje o único desconto é o negociado no balcão (com teto). Não há
-- promoção nenhuma no site. Cupom é a alavanca clássica de conversão — e o valor
-- é SEMPRE resolvido no servidor a partir do subtotal autoritativo (o cliente
-- manda só o código), nunca o valor. O total do pedido cai, e como o
-- payment-service deriva o valor cobrado de orders.total, a cobrança cai junto —
-- sem tocar no payment.
--
-- Modelo: percentual (0-100, com teto no CHECK) OU valor fixo em reais. Piso de
-- pedido (min_subtotal), validade e limite de usos opcionais. Reversível.
-- ============================================================================
CREATE TYPE coupon_type AS ENUM ('percent', 'fixed');

CREATE TABLE coupons (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code         TEXT NOT NULL UNIQUE,
    type         coupon_type NOT NULL,
    -- percent: 0-100 (o CHECK abaixo trava >100); fixed: valor em reais.
    value        NUMERIC(12,2) NOT NULL CHECK (value >= 0),
    min_subtotal NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (min_subtotal >= 0),
    max_uses     INTEGER,                              -- NULL = ilimitado
    uses         INTEGER NOT NULL DEFAULT 0 CHECK (uses >= 0),
    active       BOOLEAN NOT NULL DEFAULT true,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (type <> 'percent' OR value <= 100)
);

CREATE TRIGGER trg_coupons_updated
    BEFORE UPDATE ON coupons
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Qual cupom deu o desconto neste pedido — rastro para conciliação e marketing.
-- O valor em reais já vai em orders.discount_amount (coluna genérica "dinheiro
-- que saiu", compartilhada com o desconto de balcão).
ALTER TABLE orders ADD COLUMN coupon_code TEXT;
