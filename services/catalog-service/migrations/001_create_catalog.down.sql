DROP TRIGGER IF EXISTS trg_products_updated  ON products;
DROP TRIGGER IF EXISTS trg_sellers_updated   ON sellers;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS product_images;
DROP TABLE IF EXISTS products;
DROP TYPE  IF EXISTS product_badge;
DROP TABLE IF EXISTS sellers;
DROP TABLE IF EXISTS categories;
