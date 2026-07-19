-- Livro contábil de partidas dobradas — imutável, em centavos inteiros.
--
-- POR QUE ISTO EXISTE
-- A tabela `payments` responde "esse pagamento foi confirmado?". Ela NÃO
-- responde "quanto entrou de receita em junho, quanto foi taxa de gateway,
-- quanto foi estornado e quanto devemos aos vendedores". Essas perguntas são
-- contábeis e exigem um livro: cada movimento de dinheiro registrado em
-- partidas dobradas, imutável, com o documento de origem apontado.
--
-- DECISÕES DE PROJETO E O PORQUÊ
--
-- 1. BIGINT CENTAVOS, NUNCA float nem NUMERIC-com-float-no-Go.
--    `payments.amount` é NUMERIC(12,2) e o Go lê como float64 — aceitável pra
--    exibir, inaceitável pra somar milhares de linhas: 0.1+0.2 != 0.3 e o erro
--    ACUMULA. Ver internal/psp/appmaxv1/money_test.go, que documenta o mesmo
--    problema no caminho do PSP. Aqui é BIGINT e ponto.
--
-- 2. IMUTÁVEL DE VERDADE. UPDATE e DELETE bloqueados por trigger em
--    ledger_transactions e ledger_entries. Corrigir um lançamento errado é
--    LANÇAR O ESTORNO (kind='reversal', reverses_id apontando pro original) —
--    é assim que contabilidade funciona e é a única forma de a trilha continuar
--    fazendo sentido depois da correção.
--
-- 3. TODA TRANSAÇÃO SOMA ZERO, garantido pelo banco. O trigger de balanço é
--    CONSTRAINT TRIGGER DEFERRABLE INITIALLY DEFERRED: as partidas entram uma a
--    uma e a checagem roda no COMMIT, quando a transação está completa. Sem o
--    DEFERRABLE, o primeiro INSERT já falharia (débito sem crédito ainda).
--
-- 4. FECHAMENTO DE PERÍODO trava o mês no banco, não no código. Depois de
--    fechado, INSERT com occurred_at naquele mês é recusado — inclusive por um
--    job antigo que ficou rodando, ou por alguém com psql aberto.

CREATE TYPE ledger_account_type AS ENUM ('asset', 'liability', 'equity', 'revenue', 'expense');
CREATE TYPE ledger_side         AS ENUM ('debit', 'credit');
CREATE TYPE ledger_period_status AS ENUM ('open', 'closed');

-- ===================== Plano de contas =====================
CREATE TABLE ledger_accounts (
    code        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        ledger_account_type NOT NULL,
    normal_side ledger_side NOT NULL,
    description TEXT NOT NULL DEFAULT ''
);

INSERT INTO ledger_accounts (code, name, type, normal_side, description) VALUES
    ('1.1.1', 'Caixa em trânsito no PSP', 'asset',     'debit',
     'Dinheiro que o PSP já capturou e ainda não repassou pra nossa conta bancária.'),
    ('1.1.2', 'Banco - conta movimento',  'asset',     'debit',
     'Saldo já liquidado na conta da empresa (saque do PSP concluído).'),
    ('2.1.1', 'Repasses a vendedores',    'liability', 'credit',
     'Obrigação com o vendedor pelo split do pedido, até o saque dele.'),
    ('3.1.1', 'Receita bruta de vendas',  'revenue',   'credit',
     'Valor cheio do pedido no momento da captura. Bruto: taxas e repasses são contas próprias.'),
    ('3.1.8', 'Estornos e devoluções',    'revenue',   'debit',
     'Conta REDUTORA de receita. Saldo devedor é o esperado.'),
    ('3.1.9', 'Chargebacks',              'revenue',   'debit',
     'Conta REDUTORA de receita. Separada de estorno: a causa e o tratamento são outros.'),
    ('4.1.1', 'Taxas do gateway (PSP)',   'expense',   'debit',
     'MDR, taxa fixa por transação e afins, cobrados pela Appmax/PSP.'),
    ('4.1.2', 'Taxa de antecipação',      'expense',   'debit',
     'Custo de antecipar recebíveis.'),
    ('4.2.1', 'Custo de repasse a vendedor', 'expense', 'debit',
     'Contrapartida do split: o que do pedido pertence ao vendedor.');

-- ===================== Períodos contábeis =====================
CREATE TABLE ledger_periods (
    period           DATE PRIMARY KEY,   -- sempre o dia 1º do mês
    status           ledger_period_status NOT NULL DEFAULT 'open',
    closed_at        TIMESTAMPTZ,
    closed_by        TEXT NOT NULL DEFAULT '',
    closing_balances JSONB,              -- saldo por conta no fechamento (centavos)
    totals_cents     JSONB,              -- receita bruta / taxas / estornos / líquido
    entries_count    BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT ledger_periods_first_of_month CHECK (EXTRACT(DAY FROM period) = 1),
    CONSTRAINT ledger_periods_closed_has_ts  CHECK (status <> 'closed' OR closed_at IS NOT NULL)
);

-- Reabrir um mês fechado invalidaria todo balanço já entregue ao contador.
-- Se for MESMO necessário, é operação de DBA com trigger desabilitado e
-- registro na trilha de auditoria — nunca pela aplicação.
CREATE OR REPLACE FUNCTION ledger_period_no_reopen() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status = 'closed' THEN
        RAISE EXCEPTION 'período %-% já está fechado: reabertura é proibida pela aplicação',
            EXTRACT(YEAR FROM OLD.period), LPAD(EXTRACT(MONTH FROM OLD.period)::text, 2, '0')
            USING ERRCODE = '42501';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_ledger_period_no_reopen
    BEFORE UPDATE OR DELETE ON ledger_periods
    FOR EACH ROW EXECUTE FUNCTION ledger_period_no_reopen();

-- ===================== Transações (documentos) =====================
CREATE TABLE ledger_transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at TIMESTAMPTZ NOT NULL,
    period      DATE NOT NULL,   -- derivado de occurred_at pelo trigger; nunca informado à mão
    kind        TEXT NOT NULL,   -- sale | psp_fee | refund | chargeback | seller_split | seller_withdrawal | payout | anticipation_fee | reversal | adjustment
    source_type TEXT NOT NULL,   -- payment | webhook_event | order | recipient | manual
    source_id   TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    currency    CHAR(3) NOT NULL DEFAULT 'BRL',
    request_id  TEXT NOT NULL DEFAULT '',
    reverses_id UUID REFERENCES ledger_transactions(id),
    created_by  TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Idempotência do lançamento: um webhook reentregue não pode dobrar a
    -- receita. Reentrega vira violação de UNIQUE, que o Go trata como no-op.
    CONSTRAINT ledger_tx_source_unique UNIQUE (kind, source_type, source_id)
);

CREATE INDEX idx_ledger_tx_period   ON ledger_transactions(period, occurred_at);
CREATE INDEX idx_ledger_tx_occurred ON ledger_transactions(occurred_at);
CREATE INDEX idx_ledger_tx_source   ON ledger_transactions(source_type, source_id);
CREATE INDEX idx_ledger_tx_request  ON ledger_transactions(request_id) WHERE request_id <> '';

-- ===================== Partidas =====================
CREATE TABLE ledger_entries (
    id             BIGSERIAL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES ledger_transactions(id),
    account_code   TEXT NOT NULL REFERENCES ledger_accounts(code),
    side           ledger_side NOT NULL,
    -- CENTAVOS. Sempre positivo: o sinal é o `side`, não o valor. Valor
    -- negativo com side='debit' seria um crédito disfarçado e furaria todo
    -- relatório que agrupa por side.
    amount_cents   BIGINT NOT NULL CHECK (amount_cents > 0),
    payment_method TEXT NOT NULL DEFAULT '',  -- pix | boleto | card | '' (label de relatório)
    seller_id      TEXT NOT NULL DEFAULT '',
    memo           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_ledger_entries_tx      ON ledger_entries(transaction_id);
CREATE INDEX idx_ledger_entries_account ON ledger_entries(account_code);
CREATE INDEX idx_ledger_entries_seller  ON ledger_entries(seller_id) WHERE seller_id <> '';

-- ===================== Imutabilidade =====================
CREATE OR REPLACE FUNCTION ledger_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ledger é imutável: % em % é proibido — corrija com um lançamento de estorno (kind=reversal)',
        TG_OP, TG_TABLE_NAME
        USING ERRCODE = '42501';
END;
$$;

CREATE TRIGGER trg_ledger_tx_immutable
    BEFORE UPDATE OR DELETE ON ledger_transactions
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER trg_ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER trg_ledger_tx_no_truncate
    BEFORE TRUNCATE ON ledger_transactions
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_immutable();

CREATE TRIGGER trg_ledger_entries_no_truncate
    BEFORE TRUNCATE ON ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_immutable();

REVOKE UPDATE, DELETE, TRUNCATE ON ledger_transactions FROM PUBLIC;
REVOKE UPDATE, DELETE, TRUNCATE ON ledger_entries      FROM PUBLIC;

-- ===================== Período: derivação + trava =====================
CREATE OR REPLACE FUNCTION ledger_tx_set_period() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    p DATE;
    st ledger_period_status;
BEGIN
    -- O período NUNCA vem do chamador: é derivado de occurred_at. Deixar a
    -- aplicação escolher o período permitiria empurrar um lançamento de julho
    -- pra junho e furar o fechamento.
    p := date_trunc('month', NEW.occurred_at AT TIME ZONE 'UTC')::date;
    NEW.period := p;

    SELECT status INTO st FROM ledger_periods WHERE period = p;
    IF st = 'closed' THEN
        RAISE EXCEPTION 'período % está FECHADO: lançamento retroativo proibido (use um lançamento no período aberto corrente)', to_char(p, 'YYYY-MM')
            USING ERRCODE = '42501';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_ledger_tx_set_period
    BEFORE INSERT ON ledger_transactions
    FOR EACH ROW EXECUTE FUNCTION ledger_tx_set_period();

-- ===================== Partidas dobradas: soma zero =====================
-- CONSTRAINT TRIGGER DEFERRABLE: roda no COMMIT, quando a transação já tem
-- todas as partidas. Se fosse imediato, o primeiro INSERT (débito, ainda sem o
-- crédito correspondente) falharia sempre.
CREATE OR REPLACE FUNCTION ledger_tx_must_balance() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    tx_id UUID;
    dr BIGINT;
    cr BIGINT;
    n  INT;
BEGIN
    -- A MESMA função é usada nos dois gatilhos (partidas e documento), e o
    -- plpgsql não permite referenciar um campo que não existe no record —
    -- `NEW.transaction_id` explodiria quando disparado por ledger_transactions.
    -- Daí o dispatch explícito por TG_TABLE_NAME.
    IF TG_TABLE_NAME = 'ledger_entries' THEN
        tx_id := NEW.transaction_id;
    ELSE
        tx_id := NEW.id;
    END IF;

    SELECT COALESCE(SUM(amount_cents) FILTER (WHERE side = 'debit'), 0),
           COALESCE(SUM(amount_cents) FILTER (WHERE side = 'credit'), 0),
           COUNT(*)
      INTO dr, cr, n
      FROM ledger_entries WHERE transaction_id = tx_id;

    IF n = 0 THEN
        RAISE EXCEPTION 'lançamento % não tem nenhuma partida — documento contábil vazio é erro', tx_id
            USING ERRCODE = '23514';
    END IF;
    IF n < 2 THEN
        RAISE EXCEPTION 'lançamento % tem só % partida: partida dobrada exige ao menos débito e crédito', tx_id, n
            USING ERRCODE = '23514';
    END IF;
    IF dr <> cr THEN
        RAISE EXCEPTION 'lançamento % NÃO FECHA: débitos=% centavos, créditos=% centavos, diferença=% centavos',
            tx_id, dr, cr, dr - cr
            USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER trg_ledger_entries_balance
    AFTER INSERT ON ledger_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger_tx_must_balance();

-- Também pelo lado da transação: um documento inserido sem NENHUMA partida
-- passaria despercebido se só o gatilho de ledger_entries existisse.
CREATE CONSTRAINT TRIGGER trg_ledger_tx_balance
    AFTER INSERT ON ledger_transactions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger_tx_must_balance();

-- ===================== Reconciliação =====================
-- Divergência de dinheiro NÃO é corrigida automaticamente: é registrada e
-- espera humano. Por isso estas duas tabelas ficam FORA do regime de
-- imutabilidade — `resolved_at`/`resolution_note` são preenchidos depois.
CREATE TABLE reconciliation_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          TEXT NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ,
    window_from       TIMESTAMPTZ NOT NULL,
    window_to         TIMESTAMPTZ NOT NULL,
    checked_count     INT NOT NULL DEFAULT 0,
    discrepancy_count INT NOT NULL DEFAULT 0,
    error_count       INT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'running',  -- running | ok | discrepancies | failed
    request_id        TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_recon_runs_started ON reconciliation_runs(started_at DESC);

CREATE TABLE reconciliation_discrepancies (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id             UUID NOT NULL REFERENCES reconciliation_runs(id) ON DELETE CASCADE,
    payment_id         UUID,
    psp_payment_id     TEXT NOT NULL DEFAULT '',
    kind               TEXT NOT NULL,  -- amount_mismatch | status_mismatch | missing_at_psp | psp_error | ledger_missing
    severity           TEXT NOT NULL DEFAULT 'high',
    local_value        TEXT NOT NULL DEFAULT '',
    psp_value          TEXT NOT NULL DEFAULT '',
    amount_delta_cents BIGINT NOT NULL DEFAULT 0,
    detail             TEXT NOT NULL DEFAULT '',
    detected_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at        TIMESTAMPTZ,
    resolved_by        TEXT NOT NULL DEFAULT '',
    resolution_note    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_recon_disc_run    ON reconciliation_discrepancies(run_id);
CREATE INDEX idx_recon_disc_open   ON reconciliation_discrepancies(detected_at DESC) WHERE resolved_at IS NULL;
CREATE INDEX idx_recon_disc_payment ON reconciliation_discrepancies(payment_id) WHERE payment_id IS NOT NULL;
