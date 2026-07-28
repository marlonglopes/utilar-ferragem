-- ============================================================================
-- scrape_queue — fila de URLs e estado de execução do scraper de catálogo.
-- ----------------------------------------------------------------------------
-- PORQUÊ existe: um scraping de catálogo é longo e pode cair no meio (rede,
-- 429, deploy). Sem persistir o que já foi feito, retomar significa recomeçar
-- do zero e martelar o fornecedor de novo. A fila deixa a execução RETOMÁVEL e
-- permite re-scraping incremental por cron (preço/estoque mudam; a estrutura do
-- catálogo, não).
--
-- Uma linha por (fonte, url). `status` caminha pending → claimed → done|error.
-- O índice parcial serve o "pegue o próximo pendente" barato.
-- ============================================================================
CREATE TABLE scrape_queue (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte       TEXT NOT NULL,                      -- nome do adapter (a fonte)
    url         TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',    -- pending | claimed | done | error
    attempts    INT  NOT NULL DEFAULT 0,
    last_error  TEXT,
    claimed_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scrape_queue_status_ck CHECK (status IN ('pending','claimed','done','error')),
    CONSTRAINT scrape_queue_fonte_url_uq UNIQUE (fonte, url)
);

-- "próximo pendente desta fonte" — índice parcial, só as linhas que interessam.
CREATE INDEX idx_scrape_queue_pending ON scrape_queue (fonte, created_at) WHERE status = 'pending';

CREATE TRIGGER trg_scrape_queue_updated
    BEFORE UPDATE ON scrape_queue
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
