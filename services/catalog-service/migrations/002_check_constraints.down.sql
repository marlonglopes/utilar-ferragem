ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_price_nonneg,
    DROP CONSTRAINT IF EXISTS products_orig_price_pos,
    DROP CONSTRAINT IF EXISTS products_stock_nonneg,
    DROP CONSTRAINT IF EXISTS products_rating_range,
    DROP CONSTRAINT IF EXISTS products_reviews_nonneg,
    DROP CONSTRAINT IF EXISTS products_installments_pos;
