-- ============================================================================
-- Personas de operação do backoffice — contador, vendas, almoxarife
-- ----------------------------------------------------------------------------
-- POR QUÊ: o painel /admin era tudo-ou-nada (só `admin`). O dono quer que o
-- contador, o vendedor e o almoxarife entrem, cada um vendo só o seu. O papel
-- é a fronteira: cada serviço recusa (403) o que está fora do escopo da persona
-- (o menu filtrado é só conforto). Ver docs/backoffice-personas.md.
--
-- Estende o enum user_role. ALTER TYPE ... ADD VALUE é IRREVERSÍVEL (Postgres
-- não remove valor de enum) e o requisito é migration reversível — então
-- recriamos o tipo, igual a migration 004. O down desfaz de verdade.
--
-- ⚠️ `vendas` é o vendedor INTERNO (PDV+pedidos+catálogo), NÃO o `seller`
-- (lojista anunciante do marketplace) nem o `store_operator` (papel do balcão).
-- ============================================================================

ALTER TYPE user_role RENAME TO user_role_old;

CREATE TYPE user_role AS ENUM (
    'customer', 'seller', 'admin', 'store_operator',
    'contador', 'vendas', 'almoxarife'
);

ALTER TABLE users ALTER COLUMN role DROP DEFAULT;
ALTER TABLE users ALTER COLUMN role TYPE user_role USING role::text::user_role;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'customer';

DROP TYPE user_role_old;
