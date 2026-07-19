-- ============================================================================
-- PDV de balcão: lojas físicas, operadores de balcão e clientes de balcão
-- ----------------------------------------------------------------------------
-- PORQUÊ um papel NOVO em vez de reusar `seller`:
--   `seller` no enum atual significa *lojista do marketplace* — o CNPJ que
--   anuncia produtos no site. Reusá-lo para o vendedor do balcão daria acesso
--   ao PDV (e ao poder de dar desconto) para todo anunciante cadastrado. São
--   duas populações disjuntas com poderes disjuntos; o nome coincide, o
--   significado não.
--
-- PORQUÊ papel único + nível, e não três papéis:
--   `role` é a chave grossa de autorização — ela decide QUAIS SUPERFÍCIES do
--   sistema você toca, e é lida pelo RequireRole dos 4 serviços. Cargo
--   (operador/supervisor/gerente) decide QUANTO DINHEIRO você pode dar de
--   desconto dentro da mesma superfície. Se cargo virasse papel, cada lista de
--   RequireRole nos 4 serviços passaria a enumerar três valores e "gerente
--   também é operador" viraria hierarquia de papéis escrita à mão em cada
--   serviço. Com papel único + nível, a superfície é uma checagem de papel e o
--   teto é um número consultável — que muda numa promoção sem migration.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. Papel store_operator
-- ---------------------------------------------------------------------------
-- ALTER TYPE ... ADD VALUE é irreversível (Postgres não remove valor de enum),
-- e o requisito é migration reversível. Por isso recriamos o tipo: o down
-- desfaz de verdade.
ALTER TYPE user_role RENAME TO user_role_old;

CREATE TYPE user_role AS ENUM ('customer', 'seller', 'admin', 'store_operator');

ALTER TABLE users ALTER COLUMN role DROP DEFAULT;
ALTER TABLE users ALTER COLUMN role TYPE user_role USING role::text::user_role;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'customer';

DROP TYPE user_role_old;

-- ---------------------------------------------------------------------------
-- 2. stores — lojas/filiais
-- ---------------------------------------------------------------------------
-- O balcão é multi-loja por natureza: o mesmo backend atende a matriz e as
-- filiais, e TODA autorização do PDV é escopada por loja.
CREATE TABLE stores (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          TEXT NOT NULL UNIQUE,          -- 'MATRIZ', 'FIL-002' — o que o operador fala no rádio
    name          TEXT NOT NULL,
    cnpj          TEXT NOT NULL UNIQUE,          -- 14 dígitos limpos
    street        TEXT NOT NULL,
    number        TEXT NOT NULL,
    complement    TEXT,
    neighborhood  TEXT NOT NULL,
    city          TEXT NOT NULL,
    state         CHAR(2) NOT NULL,
    cep           TEXT NOT NULL,
    phone         TEXT,
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_stores_active ON stores(active) WHERE active;

-- ---------------------------------------------------------------------------
-- 3. store_operator_levels — teto de desconto por cargo
-- ---------------------------------------------------------------------------
-- O teto vive em tabela (e não em constante Go) porque é regra comercial: o
-- dono muda "supervisor pode 20%" para 18% numa terça-feira e isso não pode
-- pedir deploy. O frontend hoje tem 12/20/100 hardcoded — estes valores são a
-- fonte de verdade que substitui aquilo.
CREATE TYPE store_operator_level AS ENUM ('operator', 'supervisor', 'manager');

CREATE TABLE store_operator_levels (
    level                 store_operator_level PRIMARY KEY,
    label                 TEXT NOT NULL,
    discount_ceiling_pct  NUMERIC(5,2) NOT NULL CHECK (discount_ceiling_pct >= 0 AND discount_ceiling_pct <= 100),
    -- Só quem pode aprovar desconto acima do teto alheio. Gerente aprova;
    -- supervisor negocia mais alto mas não homologa o desconto de ninguém.
    can_approve_discount  BOOLEAN NOT NULL DEFAULT false,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO store_operator_levels (level, label, discount_ceiling_pct, can_approve_discount) VALUES
    ('operator',   'Operador de caixa', 12.00,  false),
    ('supervisor', 'Supervisor',        20.00,  false),
    ('manager',    'Gerente',          100.00,  true);

-- ---------------------------------------------------------------------------
-- 4. store_operators — vínculo usuário ↔ loja ↔ cargo
-- ---------------------------------------------------------------------------
-- PK = user_id: um usuário opera UMA loja. Multi-loja para a mesma pessoa
-- deliberadamente fora — permitir isso exigiria escolher a loja em cada
-- request, e "de qual loja é este pedido?" viraria input do cliente, não do
-- servidor. Transferência de filial é um UPDATE.
CREATE TABLE store_operators (
    user_id               UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    store_id              UUID NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    level                 store_operator_level NOT NULL REFERENCES store_operator_levels(level),
    -- Override individual do teto (NULL = usa o do cargo). Existe para o caso
    -- real de "esse vendedor específico pode 15%" sem inventar um cargo novo.
    discount_ceiling_pct  NUMERIC(5,2) CHECK (discount_ceiling_pct IS NULL OR (discount_ceiling_pct >= 0 AND discount_ceiling_pct <= 100)),
    active                BOOLEAN NOT NULL DEFAULT true,
    created_by            UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_store_operators_store  ON store_operators(store_id) WHERE active;
CREATE INDEX idx_store_operators_level  ON store_operators(store_id, level);

-- ---------------------------------------------------------------------------
-- 5. store_customers — cadastro leve do cliente de balcão
-- ---------------------------------------------------------------------------
-- O cliente do balcão quase nunca tem conta: ele entra, compra e sai. Exigir
-- e-mail + senha para emitir uma nota é fricção que o vendedor vai contornar
-- inventando dados. Aqui o mínimo: nome, documento e telefone.
--
-- `phone` é NOT NULL porque a Appmax recusa a cobrança sem celular do pagador
-- (403 confirmado em sandbox) — deixar opcional só empurra a falha pro caixa.
--
-- LGPD: `document` é chave EXATA e única. A busca do PDV faz lookup por
-- igualdade e devolve no máximo um registro — nunca uma lista. Sem índice de
-- prefixo/trigram de propósito: não queremos que exista o caminho técnico para
-- "me dá todo mundo cujo CPF começa com 123".
CREATE TABLE store_customers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document      TEXT NOT NULL UNIQUE,          -- CPF (11) ou CNPJ (14), só dígitos
    document_type TEXT NOT NULL CHECK (document_type IN ('cpf', 'cnpj')),
    name          TEXT NOT NULL,
    phone         TEXT NOT NULL,                 -- obrigatório: exigência da Appmax
    email         TEXT,
    segment       TEXT NOT NULL DEFAULT 'varejo' CHECK (segment IN ('varejo', 'atacado', 'construtora')),
    -- Loja que cadastrou. NÃO é escopo de leitura (o cliente é da rede, não da
    -- filial) — é rastreabilidade de origem.
    created_store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
    created_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    -- Vínculo opcional com uma conta do site, se o cliente virar usuário depois.
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_store_customers_user ON store_customers(user_id) WHERE user_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 6. store_audit_events — trilha de auditoria da administração de operadores
-- ---------------------------------------------------------------------------
-- Tabela própria (em vez de mais valores no enum `auth_event_type`) porque
-- ALTER TYPE ADD VALUE não é reversível, e porque estes eventos carregam
-- old→new de campos que valem dinheiro (teto de desconto, cargo, loja).
--
-- Os eventos do PEDIDO (criação de venda, desconto aplicado, aprovação,
-- cancelamento) ficam no order-service, que é onde o pedido mora — bancos são
-- separados por serviço, não há tabela compartilhada.
CREATE TABLE store_audit_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action       TEXT NOT NULL,                  -- 'operator.created', 'operator.level_changed', ...
    actor_id     UUID REFERENCES users(id) ON DELETE SET NULL,   -- quem fez
    target_id    UUID REFERENCES users(id) ON DELETE SET NULL,   -- sobre quem
    store_id     UUID REFERENCES stores(id) ON DELETE SET NULL,
    old_value    JSONB,
    new_value    JSONB,
    ip           TEXT,
    user_agent   TEXT,
    request_id   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_store_audit_target ON store_audit_events(target_id, created_at DESC);
CREATE INDEX idx_store_audit_actor  ON store_audit_events(actor_id, created_at DESC);
CREATE INDEX idx_store_audit_store  ON store_audit_events(store_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Triggers updated_at
-- ---------------------------------------------------------------------------
CREATE TRIGGER trg_stores_updated          BEFORE UPDATE ON stores          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_store_operators_updated BEFORE UPDATE ON store_operators FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_store_customers_updated BEFORE UPDATE ON store_customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_store_levels_updated    BEFORE UPDATE ON store_operator_levels FOR EACH ROW EXECUTE FUNCTION set_updated_at();
