-- Reverte 004_store_operators.
--
-- Ordem importa: os usuários com papel `store_operator` viram `customer` ANTES
-- do enum voltar a não ter esse valor, senão o cast do tipo falha e a migration
-- de down trava com o banco no meio do caminho.

DROP TABLE IF EXISTS store_audit_events;
DROP TABLE IF EXISTS store_customers;
DROP TABLE IF EXISTS store_operators;
DROP TABLE IF EXISTS store_operator_levels;
DROP TYPE  IF EXISTS store_operator_level;
DROP TABLE IF EXISTS stores;

-- Demote antes do cast (ver comentário acima).
UPDATE users SET role = 'customer' WHERE role = 'store_operator';

ALTER TYPE user_role RENAME TO user_role_new;
CREATE TYPE user_role AS ENUM ('customer', 'seller', 'admin');
ALTER TABLE users ALTER COLUMN role DROP DEFAULT;
ALTER TABLE users ALTER COLUMN role TYPE user_role USING role::text::user_role;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'customer';
DROP TYPE user_role_new;
