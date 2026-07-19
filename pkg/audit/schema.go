package audit

// SchemaUp é a migration da trilha. Cada serviço aplica no SEU banco (não há
// banco compartilhado — ver CLAUDE.md). Copie para
// `services/<svc>/migrations/NNN_create_audit_log.up.sql` ou execute via
// db.Exec(audit.SchemaUp) num bootstrap.
//
// Decisões e o PORQUÊ:
//
//   - `seq BIGINT` atribuído por nós, não BIGSERIAL: sequence do Postgres tem
//     gap natural em rollback e a gente precisa que gap == deleção. Sem isso,
//     apagar uma linha do meio é indistinguível de um rollback legítimo.
//   - Triggers bloqueiam UPDATE/DELETE/TRUNCATE. Convenção de código não é
//     controle: o banco é quem tem que recusar.
//   - REVOKE explícito no role da aplicação: mesmo que alguém remova o trigger
//     (o que exige ser owner), a app não tem o grant.
//   - Sem ON DELETE CASCADE apontando pra cá de lugar nenhum, por construção.
const SchemaUp = `
CREATE TABLE IF NOT EXISTS audit_log (
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

    -- prev_hash/hash sempre hex minúsculo de 64 chars. Uma linha com hash
    -- fora do formato é adulteração grosseira e o banco recusa na hora.
    CONSTRAINT audit_log_hash_format      CHECK (hash      ~ '^[0-9a-f]{64}$'),
    CONSTRAINT audit_log_prev_hash_format CHECK (prev_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT audit_log_seq_positive     CHECK (seq > 0)
);

CREATE INDEX IF NOT EXISTS idx_audit_log_entity     ON audit_log(entity_type, entity_id, seq DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor      ON audit_log(actor_id, seq DESC) WHERE actor_id <> '';
CREATE INDEX IF NOT EXISTS idx_audit_log_request    ON audit_log(request_id) WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS idx_audit_log_occurred   ON audit_log(occurred_at DESC);

-- ============ append-only, garantido pelo banco ============
CREATE OR REPLACE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log é append-only: % é proibido (registro de auditoria não se edita nem se apaga)', TG_OP
        USING ERRCODE = '42501';
END;
$$;

DROP TRIGGER IF EXISTS trg_audit_log_no_update ON audit_log;
CREATE TRIGGER trg_audit_log_no_update
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

DROP TRIGGER IF EXISTS trg_audit_log_no_truncate ON audit_log;
CREATE TRIGGER trg_audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

-- Defesa em profundidade: sem grant, nem removendo o trigger a app consegue.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM PUBLIC;
`

// SchemaDown desfaz a migration. Reversível por contrato de migrations — mas
// note o óbvio: derrubar a tabela APAGA a trilha. Em produção isso só deve
// rodar depois de exportar (ver Recorder.List / export CSV do ledger).
const SchemaDown = `
DROP TRIGGER IF EXISTS trg_audit_log_no_truncate ON audit_log;
DROP TRIGGER IF EXISTS trg_audit_log_no_update ON audit_log;
DROP TABLE IF EXISTS audit_log;
DROP FUNCTION IF EXISTS audit_log_append_only();
`
