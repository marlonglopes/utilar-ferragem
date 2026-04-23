CREATE TABLE payments_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type      TEXT NOT NULL,
    payload_json    JSONB NOT NULL,
    attempts        INT NOT NULL DEFAULT 0,
    published_at    TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_outbox_unpublished
    ON payments_outbox(next_attempt_at)
    WHERE published_at IS NULL;
