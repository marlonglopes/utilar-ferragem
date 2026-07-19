-- ============================================================================
-- import_batches.missing_skus — o arquivamento por ausência tem que sobreviver
-- ao intervalo entre o dry-run e o commit
-- ----------------------------------------------------------------------------
-- O dry-run calcula quais SKUs do fornecedor existem no catálogo e NÃO vieram
-- na planilha (Plan.MissingSKUs) — é o que a tela de revisão mostra ao humano
-- como "estes serão arquivados". Mas o plano só era persistido em `import_rows`
-- (uma linha por linha do arquivo), e ausência é justamente o que NÃO tem
-- linha. O resultado: `loadPlan` reconstruía o plano sem MissingSKUs e o commit
-- arquivava zero produtos — a opção `archiveMissing` era um no-op silencioso.
--
-- Persistir aqui, e não recalcular no commit, é deliberado: o commit aplica o
-- plano APROVADO. Se recalculasse, um produto cadastrado entre a revisão e a
-- aprovação seria arquivado sem que ninguém tivesse visto — exatamente o tipo
-- de escrita não-aprovada que a separação dry-run/commit existe para impedir.
--
-- JSONB e não TEXT[]: o restante do plano já trafega como JSONB e a lista é
-- lida inteira, nunca consultada por elemento.
-- ============================================================================

ALTER TABLE import_batches ADD COLUMN IF NOT EXISTS missing_skus JSONB;

COMMENT ON COLUMN import_batches.missing_skus IS
    'SKUs do fornecedor ausentes desta planilha; serão ARQUIVADOS no commit (nunca deletados)';
