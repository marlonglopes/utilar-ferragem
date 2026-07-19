-- ============================================================================
-- Auditoria de escrita administrativa
-- ----------------------------------------------------------------------------
-- PORQUÊ: hoje qualquer admin pode zerar o preço de 4.000 SKUs e não sobra
-- registro de quem foi nem do que havia antes. Catálogo é a superfície de
-- escrita mais destrutiva da loja; sem trilha, um erro (ou um abuso) é
-- indistinguível de comportamento normal e irrecuperável.
--
-- Escopo deliberadamente pequeno: tabela própria do catalog-service, sem
-- framework. Se `pkg/` ganhar um pacote de auditoria compartilhado, esta tabela
-- vira o sink dele — mas o registro NÃO pode depender disso existir.
-- ============================================================================

CREATE TABLE IF NOT EXISTS catalog_audit_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- actor_id é TEXT (não UUID) porque em DEV_MODE o ator vem do header
    -- X-User-Id, que é livre. Auditoria que rejeita a linha por formato do ator
    -- perde justamente o evento que se queria registrar.
    actor_id   TEXT,
    actor_role TEXT,
    action     TEXT NOT NULL,   -- product.create | product.update | product.archive | product.import | ...
    entity     TEXT NOT NULL,   -- product | product_image | price_tier
    entity_id  TEXT,
    -- changes: {"price": {"old": 10.00, "new": 12.50}} — valor antigo → novo.
    changes    JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT,            -- cruza com o access log e com o Sentry
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "o que mudou nesse produto?" e "o que esse admin fez ontem?" são as duas
-- perguntas reais da auditoria.
CREATE INDEX IF NOT EXISTS idx_audit_entity ON catalog_audit_log(entity, entity_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_actor  ON catalog_audit_log(actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_time   ON catalog_audit_log(created_at DESC);
