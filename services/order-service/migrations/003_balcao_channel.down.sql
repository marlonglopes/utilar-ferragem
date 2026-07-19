-- Reverte 003_balcao_channel.
--
-- Os pedidos de balcão são apagados: sem `channel` eles virariam pedidos web
-- sem endereço de entrega, que é um estado que o resto do sistema trata como
-- corrompido. Rollback desta migration é operação destrutiva por natureza —
-- fazer o dump antes é responsabilidade de quem roda o down.

DROP TABLE IF EXISTS balcao_audit_events;

DELETE FROM orders WHERE channel = 'balcao';

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_no_self_approval;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_balcao_requires_attribution;

DROP INDEX IF EXISTS idx_orders_pending_approval;
DROP INDEX IF EXISTS idx_orders_operator;
DROP INDEX IF EXISTS idx_orders_store;

ALTER TABLE orders
    DROP COLUMN IF EXISTS approval_note,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approval_status,
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS discount_pct,
    DROP COLUMN IF EXISTS customer_phone,
    DROP COLUMN IF EXISTS customer_document,
    DROP COLUMN IF EXISTS customer_name,
    DROP COLUMN IF EXISTS customer_id,
    DROP COLUMN IF EXISTS store_id,
    DROP COLUMN IF EXISTS operator_id,
    DROP COLUMN IF EXISTS channel;

DROP TYPE IF EXISTS approval_status;
DROP TYPE IF EXISTS order_channel;
