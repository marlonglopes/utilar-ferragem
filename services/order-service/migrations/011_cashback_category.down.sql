DROP TABLE IF EXISTS cashback_category_rates;
ALTER TABLE order_items DROP COLUMN IF EXISTS category_id;
