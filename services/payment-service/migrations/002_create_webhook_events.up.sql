CREATE TABLE webhook_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    psp_id        TEXT NOT NULL,
    psp_payment_id TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    raw_payload   JSONB NOT NULL,
    received_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Idempotency: same PSP + payment + event_type can only be processed once
CREATE UNIQUE INDEX idx_webhook_events_idempotency
    ON webhook_events(psp_id, psp_payment_id, event_type);
