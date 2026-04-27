-- ============================================================================
-- CT1-M5 / CT1-L1: CHECK constraints sanity em products
-- ----------------------------------------------------------------------------
-- Hoje o app valida price/stock no Go, mas se um seed/migration manual gravar
-- valores inválidos (negative, original_price < price), a UI quebra silenciosa.
-- CHECK no DB = última linha de defesa.
-- ============================================================================

ALTER TABLE products
    ADD CONSTRAINT products_price_nonneg     CHECK (price >= 0),
    ADD CONSTRAINT products_orig_price_pos   CHECK (original_price IS NULL OR original_price >= 0),
    ADD CONSTRAINT products_stock_nonneg     CHECK (stock >= 0),
    ADD CONSTRAINT products_rating_range     CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5)),
    ADD CONSTRAINT products_reviews_nonneg   CHECK (review_count IS NULL OR review_count >= 0),
    ADD CONSTRAINT products_installments_pos CHECK (installments IS NULL OR installments > 0);
