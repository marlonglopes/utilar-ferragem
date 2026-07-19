-- Reverte 007_returns.
--
-- ⚠️ APAGA REGISTRO DE DEVOLUÇÃO E DE ESTORNO. Devolução é documento fiscal e
-- contábil: o lançamento correspondente no livro do payment-service continua
-- lá, imutável (o livro é append-only por construção), e passa a não ter par
-- no order-service. A trilha de quem aprovou cada estorno se perde junto.
--
-- Faça o dump ANTES. Em produção com devoluções já processadas, esta migration
-- destrói a prova de que a loja cumpriu o CDC — que é exatamente a prova que o
-- Procon pede.

DROP TABLE IF EXISTS return_audit_events;
DROP TABLE IF EXISTS order_return_items;

DROP TRIGGER IF EXISTS trg_returns_updated ON order_returns;
DROP TABLE IF EXISTS order_returns;

DROP TYPE IF EXISTS return_status;
DROP TYPE IF EXISTS return_kind;

ALTER TABLE orders DROP COLUMN IF EXISTS payment_split;
