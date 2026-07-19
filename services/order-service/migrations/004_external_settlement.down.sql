-- Reverte 004_external_settlement.
--
-- Os pedidos liquidados na maquininha viram `card` de novo — que é exatamente
-- o bug de conciliação que esta migration corrigiu. É a única saída: o enum
-- antigo não tem 'external' e a coluna é NOT NULL.
--
-- Faça o dump ANTES. As colunas external_* são apagadas junto, então o NSU (a
-- única amarra com o extrato do adquirente) se perde. Reverter esta migration
-- em produção com vendas já liquidadas destrói rastro contábil; o lançamento
-- correspondente no livro do payment-service continua lá, imutável, e passa a
-- não ter par no order-service.

DROP INDEX IF EXISTS idx_orders_external_settled;
DROP INDEX IF EXISTS idx_orders_external_nsu_unico;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_external_settlement_complete;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_external_only_balcao;

-- Reclassifica antes de estreitar o tipo, senão o USING falha.
UPDATE orders SET payment_method = 'card' WHERE payment_method = 'external';

ALTER TYPE payment_method RENAME TO payment_method_ext;

CREATE TYPE payment_method AS ENUM ('pix', 'boleto', 'card');

ALTER TABLE orders
    ALTER COLUMN payment_method TYPE payment_method
    USING payment_method::text::payment_method;

DROP TYPE payment_method_ext;

ALTER TABLE orders
    DROP COLUMN IF EXISTS external_settled_at,
    DROP COLUMN IF EXISTS external_settled_by,
    DROP COLUMN IF EXISTS external_auth_code,
    DROP COLUMN IF EXISTS external_brand,
    DROP COLUMN IF EXISTS external_nsu;
