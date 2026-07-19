-- Reversão. Os OBJETOS no storage não são apagados de propósito: derrubar a
-- migration é operação de schema, e apagar arquivo é irreversível. Órfão em
-- disco custa espaço; foto de catálogo apagada por um rollback custa a venda.
DROP INDEX IF EXISTS idx_product_images_checksum;
DROP INDEX IF EXISTS idx_product_images_dedupe;

ALTER TABLE product_images
    DROP COLUMN IF EXISTS variants,
    DROP COLUMN IF EXISTS checksum,
    DROP COLUMN IF EXISTS original_filename,
    DROP COLUMN IF EXISTS original_bytes,
    DROP COLUMN IF EXISTS bytes,
    DROP COLUMN IF EXISTS height,
    DROP COLUMN IF EXISTS width,
    DROP COLUMN IF EXISTS content_type,
    DROP COLUMN IF EXISTS storage_key;
