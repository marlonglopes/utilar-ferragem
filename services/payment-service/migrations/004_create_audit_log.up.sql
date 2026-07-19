-- Trilha de auditoria append-only encadeada por hash (pkg/audit).
--
-- Este SQL é a cópia fiel de `audit.SchemaUp` do módulo compartilhado
-- github.com/utilar/pkg/audit. Cada serviço aplica no SEU banco — não há banco
-- compartilhado no Utilar (ver CLAUDE.md). Se mudar aqui, mude lá.
--
-- Decisões e o PORQUÊ:
--   * `seq BIGINT` atribuído pela aplicação, não BIGSERIAL: a sequence do
--     Postgres tem gap natural em rollback, e a gente PRECISA que gap signifique
--     deleção. Com BIGSERIAL, apagar a linha do meio seria indistinguível de um
--     rollback legítimo e a detecção de adulteração perderia o dente.
--   * UPDATE/DELETE/TRUNCATE bloqueados por trigger. Convenção de código não é
--     controle de segurança: quem tem que recusar é o banco.
--   * REVOKE explícito: defesa em profundidade caso alguém remova o trigger.

CREATE TABLE audit_log (
    seq              BIGINT PRIMARY KEY,
    occurred_at      TIMESTAMPTZ NOT NULL,
    service          TEXT        NOT NULL,
    actor_id         TEXT        NOT NULL DEFAULT '',
    actor_role       TEXT        NOT NULL DEFAULT '',
    actor_ip         TEXT        NOT NULL DEFAULT '',
    actor_user_agent TEXT        NOT NULL DEFAULT '',
    entity_type      TEXT        NOT NULL,
    entity_id        TEXT        NOT NULL DEFAULT '',
    action           TEXT        NOT NULL,
    old_value        JSONB,
    new_value        JSONB,
    request_id       TEXT        NOT NULL DEFAULT '',
    prev_hash        CHAR(64)    NOT NULL,
    hash             CHAR(64)    NOT NULL UNIQUE,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT audit_log_hash_format      CHECK (hash      ~ '^[0-9a-f]{64}$'),
    CONSTRAINT audit_log_prev_hash_format CHECK (prev_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT audit_log_seq_positive     CHECK (seq > 0)
);

CREATE INDEX idx_audit_log_entity   ON audit_log(entity_type, entity_id, seq DESC);
CREATE INDEX idx_audit_log_actor    ON audit_log(actor_id, seq DESC) WHERE actor_id <> '';
CREATE INDEX idx_audit_log_request  ON audit_log(request_id) WHERE request_id <> '';
CREATE INDEX idx_audit_log_occurred ON audit_log(occurred_at DESC);

CREATE OR REPLACE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log é append-only: % é proibido (registro de auditoria não se edita nem se apaga)', TG_OP
        USING ERRCODE = '42501';
END;
$$;

CREATE TRIGGER trg_audit_log_no_update
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

CREATE TRIGGER trg_audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM PUBLIC;
