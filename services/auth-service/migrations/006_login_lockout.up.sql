-- ============================================================================
-- Bloqueio de conta após N tentativas de login falhas (anti brute-force)
-- ----------------------------------------------------------------------------
-- POR QUÊ: hoje o único freio ao brute-force é o rate-limit POR IP (5/min). Ele
-- NÃO protege contra IP-hopping (botnet distribuída tentando a mesma conta de um
-- IP diferente a cada request). O alvo óbvio é admin@utilar.com.br — com o painel
-- inteiro atrás. O lockout POR CONTA persiste na própria linha do usuário, então
-- vale independentemente do IP de origem.
--
-- Colunas omitidas do INSERT de Register → o DEFAULT vale (ver CLAUDE.md sobre
-- NOT NULL DEFAULT). Reversível.
-- ============================================================================
ALTER TABLE users
    ADD COLUMN failed_login_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN locked_until          TIMESTAMPTZ,
    ADD COLUMN last_failed_login_at  TIMESTAMPTZ;
