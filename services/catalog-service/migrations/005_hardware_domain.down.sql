-- Reverte 005. ATENÇÃO: `stock` volta a INT e frações são ARREDONDADAS —
-- 2.5 m de cabo vira 3. Perda de dado inevitável ao estreitar o tipo; por isso
-- o down existe para rollback imediato de deploy, não para uso rotineiro.

DROP INDEX IF EXISTS idx_products_supplier;
DROP INDEX IF EXISTS idx_products_sku_trgm;
DROP INDEX IF EXISTS idx_products_barcode;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_cost_nonneg,
    DROP CONSTRAINT IF EXISTS products_qty_step_pos,
    DROP CONSTRAINT IF EXISTS products_uom_sane,
    DROP CONSTRAINT IF EXISTS products_weight_nonneg,
    DROP CONSTRAINT IF EXISTS products_dims_nonneg,
    DROP CONSTRAINT IF EXISTS products_barcode_format,
    DROP CONSTRAINT IF EXISTS products_ncm_format,
    DROP CONSTRAINT IF EXISTS products_cfop_format,
    DROP CONSTRAINT IF EXISTS products_cest_format,
    DROP CONSTRAINT IF EXISTS products_origem_range;

ALTER TABLE products ALTER COLUMN stock TYPE INT USING round(stock)::int;
ALTER TABLE products ALTER COLUMN stock SET DEFAULT 0;

ALTER TABLE products
    DROP COLUMN IF EXISTS cost,
    DROP COLUMN IF EXISTS unit_of_measure,
    DROP COLUMN IF EXISTS qty_step,
    DROP COLUMN IF EXISTS barcode,
    DROP COLUMN IF EXISTS weight_kg,
    DROP COLUMN IF EXISTS length_cm,
    DROP COLUMN IF EXISTS width_cm,
    DROP COLUMN IF EXISTS height_cm,
    DROP COLUMN IF EXISTS supplier_id,
    DROP COLUMN IF EXISTS supplier_sku,
    DROP COLUMN IF EXISTS ncm,
    DROP COLUMN IF EXISTS cfop,
    DROP COLUMN IF EXISTS cest,
    DROP COLUMN IF EXISTS origem;
