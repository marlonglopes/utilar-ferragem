-- ============================================================================
-- A7-H3: Token hashing migration
-- ----------------------------------------------------------------------------
-- Substitui a coluna `token` (plaintext) por `token_hash` (SHA-256 hex) em
-- email_verification_tokens, password_reset_tokens e refresh_tokens.
--
-- Motivação:
-- - Se o DB for comprometido (backup roubado, SQLi, dump), atacante hoje teria
--   refresh tokens em plaintext e poderia usar diretamente sem precisar de
--   senha do usuário.
-- - Com hash, o token só é útil enquanto o usuário tem cópia (no cookie/app).
--
-- Pre-launch: não há prod com dados reais. Truncamos as 3 tabelas — usuários
-- com sessão ativa precisarão fazer login novamente. Aceitável dada a fase.
-- ============================================================================

TRUNCATE refresh_tokens, password_reset_tokens, email_verification_tokens;

-- email_verification_tokens --------------------------------------------------
ALTER TABLE email_verification_tokens DROP CONSTRAINT email_verification_tokens_pkey;
ALTER TABLE email_verification_tokens DROP COLUMN token;
ALTER TABLE email_verification_tokens ADD COLUMN token_hash TEXT NOT NULL;
ALTER TABLE email_verification_tokens ADD CONSTRAINT email_verification_tokens_pkey PRIMARY KEY (token_hash);

-- password_reset_tokens ------------------------------------------------------
ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_pkey;
ALTER TABLE password_reset_tokens DROP COLUMN token;
ALTER TABLE password_reset_tokens ADD COLUMN token_hash TEXT NOT NULL;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (token_hash);

-- refresh_tokens -------------------------------------------------------------
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_pkey;
ALTER TABLE refresh_tokens DROP COLUMN token;
ALTER TABLE refresh_tokens ADD COLUMN token_hash TEXT NOT NULL;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (token_hash);
