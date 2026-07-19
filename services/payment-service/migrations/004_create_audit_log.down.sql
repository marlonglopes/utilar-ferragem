-- ATENÇÃO: derrubar esta tabela APAGA a trilha de auditoria. Reversível por
-- contrato de migrations, mas em produção só rode depois de exportar
-- (GET /api/v1/ledger/audit/export ou pg_dump da tabela).
DROP TRIGGER IF EXISTS trg_audit_log_no_truncate ON audit_log;
DROP TRIGGER IF EXISTS trg_audit_log_no_update ON audit_log;
DROP TABLE IF EXISTS audit_log;
DROP FUNCTION IF EXISTS audit_log_append_only();
