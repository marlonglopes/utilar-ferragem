-- Reverte as personas de backoffice. Recria user_role sem contador/vendas/
-- almoxarife. FALHA de propósito se algum usuário ainda estiver com um desses
-- papéis (o USING não consegue converter) — reetiquetar essas pessoas para um
-- papel válido é decisão humana, não algo que o down deva silenciar.
ALTER TYPE user_role RENAME TO user_role_old;

CREATE TYPE user_role AS ENUM ('customer', 'seller', 'admin', 'store_operator');

ALTER TABLE users ALTER COLUMN role DROP DEFAULT;
ALTER TABLE users ALTER COLUMN role TYPE user_role USING role::text::user_role;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'customer';

DROP TYPE user_role_old;
