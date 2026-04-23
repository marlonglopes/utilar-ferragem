CREATE TYPE payment_method AS ENUM ('pix', 'boleto', 'card');
CREATE TYPE payment_status AS ENUM ('pending', 'confirmed', 'failed', 'expired', 'cancelled');

CREATE TABLE payments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID NOT NULL,
    user_id       UUID NOT NULL,
    method        payment_method NOT NULL,
    status        payment_status NOT NULL DEFAULT 'pending',
    amount        NUMERIC(12, 2) NOT NULL,
    currency      CHAR(3) NOT NULL DEFAULT 'BRL',
    psp_payment_id TEXT,
    psp_metadata   JSONB,
    psp_payload    JSONB,
    confirmed_at  TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_order_id  ON payments(order_id);
CREATE INDEX idx_payments_psp_id    ON payments(psp_payment_id) WHERE psp_payment_id IS NOT NULL;
CREATE INDEX idx_payments_user_id   ON payments(user_id);
