-- ============================================================================
-- MFA (TOTP) — segundo fator para o login de admin
-- ----------------------------------------------------------------------------
-- POR QUÊ: o painel /admin reescreve preço, marca pedido entregue, aprova
-- desconto e (agora) estorna dinheiro. Senha só não basta para uma conta com
-- esse poder — MFA é a diferença entre "vazou a senha do admin" e "perderam a
-- loja". O enrollment é opt-in por conta (mfa_enabled), começando pelo admin.
--
--   totp_secret          — segredo ATIVO (Base32), usado na verificação do login.
--   totp_pending_secret  — segredo do enrollment ainda NÃO confirmado. Separado
--                          para que reconfigurar o MFA não desligue o que já vale
--                          até o novo código ser confirmado.
--   mfa_enabled          — se o 2º fator é exigido no login desta conta.
--   mfa_enrolled_at      — quando foi confirmado (trilha).
--
-- ⚠️ O segredo fica em texto no banco (como o hash de senha e os refresh tokens):
-- o banco JÁ é a fronteira de confiança. Cifrar em repouso exigiria um KMS e é
-- melhoria futura, não pré-requisito. Colunas omitidas do INSERT de Register →
-- DEFAULT vale (ver CLAUDE.md). Reversível.
-- ============================================================================
ALTER TABLE users
    ADD COLUMN totp_secret         TEXT,
    ADD COLUMN totp_pending_secret TEXT,
    ADD COLUMN mfa_enabled         BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN mfa_enrolled_at     TIMESTAMPTZ;
