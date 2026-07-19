-- ATENÇÃO: isto APAGA o livro contábil. Só rode com dump prévio.
-- Ordem: triggers → tabelas dependentes → tabelas base → funções → tipos.
DROP TRIGGER IF EXISTS trg_ledger_entries_balance   ON ledger_entries;
DROP TRIGGER IF EXISTS trg_ledger_tx_balance        ON ledger_transactions;
DROP TRIGGER IF EXISTS trg_ledger_entries_no_truncate ON ledger_entries;
DROP TRIGGER IF EXISTS trg_ledger_tx_no_truncate      ON ledger_transactions;
DROP TRIGGER IF EXISTS trg_ledger_entries_immutable ON ledger_entries;
DROP TRIGGER IF EXISTS trg_ledger_tx_immutable      ON ledger_transactions;
DROP TRIGGER IF EXISTS trg_ledger_tx_set_period     ON ledger_transactions;
DROP TRIGGER IF EXISTS trg_ledger_period_no_reopen  ON ledger_periods;

DROP TABLE IF EXISTS reconciliation_discrepancies;
DROP TABLE IF EXISTS reconciliation_runs;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS ledger_transactions;
DROP TABLE IF EXISTS ledger_periods;
DROP TABLE IF EXISTS ledger_accounts;

DROP FUNCTION IF EXISTS ledger_tx_must_balance();
DROP FUNCTION IF EXISTS ledger_tx_set_period();
DROP FUNCTION IF EXISTS ledger_immutable();
DROP FUNCTION IF EXISTS ledger_period_no_reopen();

DROP TYPE IF EXISTS ledger_period_status;
DROP TYPE IF EXISTS ledger_side;
DROP TYPE IF EXISTS ledger_account_type;
