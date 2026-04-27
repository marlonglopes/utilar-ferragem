-- Rollback A7-H3. Volta plaintext token. Tokens armazenados são perdidos —
-- usuários precisam logar de novo. Use só em emergência (rollback de incident).

TRUNCATE refresh_tokens, password_reset_tokens, email_verification_tokens;

ALTER TABLE email_verification_tokens DROP CONSTRAINT email_verification_tokens_pkey;
ALTER TABLE email_verification_tokens DROP COLUMN token_hash;
ALTER TABLE email_verification_tokens ADD COLUMN token TEXT NOT NULL;
ALTER TABLE email_verification_tokens ADD CONSTRAINT email_verification_tokens_pkey PRIMARY KEY (token);

ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_pkey;
ALTER TABLE password_reset_tokens DROP COLUMN token_hash;
ALTER TABLE password_reset_tokens ADD COLUMN token TEXT NOT NULL;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (token);

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_pkey;
ALTER TABLE refresh_tokens DROP COLUMN token_hash;
ALTER TABLE refresh_tokens ADD COLUMN token TEXT NOT NULL;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (token);
