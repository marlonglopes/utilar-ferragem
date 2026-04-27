-- ============================================================================
-- L-AUTH-1: audit_events table
-- ----------------------------------------------------------------------------
-- Trilha de auditoria de eventos sensíveis (registro, login OK/falho,
-- logout, password reset, etc.). Usado pra forensics em incidente e
-- detecção de comportamento anômalo (ex: 50 logins falhos da mesma IP).
--
-- Sem PII no payload — só user_id, IP e tipo de evento. Detalhes específicos
-- vão em metadata JSONB com campos seguros.
-- ============================================================================

CREATE TYPE auth_event_type AS ENUM (
    'register',
    'login_success',
    'login_failure',
    'logout',
    'password_reset_requested',
    'password_changed',
    'email_verified'
);

CREATE TABLE auth_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  auth_event_type NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL, -- NULL pra login_failure de email não cadastrado
    ip          TEXT,
    user_agent  TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_events_user      ON auth_events(user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX idx_auth_events_type_time ON auth_events(event_type, created_at DESC);
CREATE INDEX idx_auth_events_ip_time   ON auth_events(ip, created_at DESC) WHERE ip IS NOT NULL;
