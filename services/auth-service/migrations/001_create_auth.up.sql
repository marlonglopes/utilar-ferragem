-- ============================================================================
-- Auth service schema
-- ----------------------------------------------------------------------------
-- users: identidade do comprador/vendedor/admin
-- addresses: endereços salvos para checkout rápido
-- email_verification_tokens: confirmação de email via link (TTL 24h)
-- password_reset_tokens: reset de senha (TTL 1h)
-- refresh_tokens: sessões revogáveis (TTL 30 dias, 1 por device)
-- ============================================================================

CREATE TYPE user_role AS ENUM ('customer', 'seller', 'admin');

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,                  -- argon2id encoded
    name            TEXT NOT NULL,
    cpf             TEXT,                           -- 11 dígitos limpos; unique quando presente (ver índice parcial)
    phone           TEXT,
    role            user_role NOT NULL DEFAULT 'customer',
    email_verified  BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_users_cpf  ON users(cpf) WHERE cpf IS NOT NULL;
CREATE INDEX        idx_users_role ON users(role);

-- Endereços salvos ------------------------------------------------------------
CREATE TABLE addresses (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label         TEXT NOT NULL DEFAULT 'Principal',   -- 'Casa', 'Trabalho', ...
    street        TEXT NOT NULL,
    number        TEXT NOT NULL,
    complement    TEXT,
    neighborhood  TEXT NOT NULL,
    city          TEXT NOT NULL,
    state         CHAR(2) NOT NULL,
    cep           TEXT NOT NULL,
    is_default    BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_addresses_user    ON addresses(user_id);
CREATE UNIQUE INDEX idx_addresses_default ON addresses(user_id) WHERE is_default;

-- Email verification ---------------------------------------------------------
CREATE TABLE email_verification_tokens (
    token       TEXT PRIMARY KEY,                   -- UUID v4 gerado no server
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_tokens_user ON email_verification_tokens(user_id);

-- Password reset -------------------------------------------------------------
CREATE TABLE password_reset_tokens (
    token       TEXT PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_reset_user ON password_reset_tokens(user_id);

-- Refresh tokens (sessões) ---------------------------------------------------
-- Cada login emite um refresh token (UUID). Revogar = setar revoked_at.
-- access_token (JWT) não é armazenado — dura 15 min e é sem estado.
CREATE TABLE refresh_tokens (
    token        TEXT PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent   TEXT,
    ip           TEXT,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_user       ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_active     ON refresh_tokens(user_id) WHERE revoked_at IS NULL;

-- Triggers updated_at --------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated     BEFORE UPDATE ON users     FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_addresses_updated BEFORE UPDATE ON addresses FOR EACH ROW EXECUTE FUNCTION set_updated_at();
