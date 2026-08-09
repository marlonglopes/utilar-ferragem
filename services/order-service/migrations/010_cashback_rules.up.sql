-- ============================================================================
-- Regras extras de cashback (config-level): pedidos mínimos + campanha
-- ----------------------------------------------------------------------------
-- Tudo no singleton cashback_config, editável no admin sem deploy:
--   - min_earn_subtotal   : só ACUMULA em compras de mercadoria acima deste valor
--   - min_redeem_subtotal : só deixa RESGATAR em pedidos acima deste valor
--   - campaign_*          : taxa turbinada entre datas (ex.: cashback dobrado na
--                           semana). Quando now está na janela e a taxa > 0, ela
--                           substitui earn_rate_pct no acúmulo.
--
-- Reversível.
-- ============================================================================
ALTER TABLE cashback_config
    ADD COLUMN min_earn_subtotal   NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (min_earn_subtotal   >= 0),
    ADD COLUMN min_redeem_subtotal NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (min_redeem_subtotal >= 0),
    ADD COLUMN campaign_rate_pct   NUMERIC(5,2) CHECK (campaign_rate_pct IS NULL OR (campaign_rate_pct >= 0 AND campaign_rate_pct <= 100)),
    ADD COLUMN campaign_starts_at  TIMESTAMPTZ,
    ADD COLUMN campaign_ends_at    TIMESTAMPTZ;
