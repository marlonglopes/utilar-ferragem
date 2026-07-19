-- ============================================================================
-- Venda de balcão: canal, atribuição, desconto, aprovação e auditoria
-- ----------------------------------------------------------------------------
-- O pedido de balcão quebra três premissas do pedido web:
--   1. tem endereço de entrega   → balcão é retirada no ato, não tem endereço;
--   2. quem cria é quem compra   → no balcão quem cria é o VENDEDOR;
--   3. o preço é o do catálogo   → no balcão existe negociação (desconto).
--
-- Cada uma dessas premissas virou uma coluna aqui, e cada coluna tem uma regra
-- de autorização correspondente em internal/balcao/authz.go.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. channel
-- ---------------------------------------------------------------------------
-- DEFAULT 'web': todo pedido que já existe no banco é venda do site, e todo
-- request antigo (sem o campo) continua criando pedido web. Nenhum pedido
-- histórico vira balcão por acidente.
CREATE TYPE order_channel AS ENUM ('web', 'balcao');

ALTER TABLE orders ADD COLUMN channel order_channel NOT NULL DEFAULT 'web';

-- ---------------------------------------------------------------------------
-- 2. Atribuição — quem vendeu, onde, para quem
-- ---------------------------------------------------------------------------
-- Três identidades DIFERENTES, e a confusão entre elas é o bug de segurança
-- clássico deste domínio:
--   user_id     — dono do pedido para efeito de leitura pelo cliente (já existia)
--   operator_id — o vendedor que registrou a venda (responde pelo desconto)
--   store_id    — a filial (escopo de TODA autorização do balcão)
--   customer_id — o cliente de balcão (store_customers no auth-service)
--
-- IDs opacos (TEXT): bancos são separados por serviço, não há FK cross-DB.
ALTER TABLE orders ADD COLUMN operator_id TEXT;
ALTER TABLE orders ADD COLUMN store_id    TEXT;
ALTER TABLE orders ADD COLUMN customer_id TEXT;

-- Snapshot do cliente de balcão no momento da venda. O nome/telefone do cadastro
-- pode mudar depois; o cupom precisa refletir quem comprou naquele dia. Também
-- é o que a Appmax recebe como pagador.
ALTER TABLE orders ADD COLUMN customer_name     TEXT;
ALTER TABLE orders ADD COLUMN customer_document TEXT;
ALTER TABLE orders ADD COLUMN customer_phone    TEXT;

-- Invariante no banco, não só no handler: pedido de balcão SEM vendedor e SEM
-- loja é um pedido cujo desconto ninguém responde. Um INSERT direto no psql
-- também não consegue criar esse estado.
ALTER TABLE orders ADD CONSTRAINT orders_balcao_requires_attribution
    CHECK (channel <> 'balcao' OR (operator_id IS NOT NULL AND store_id IS NOT NULL));

-- ---------------------------------------------------------------------------
-- 3. Desconto
-- ---------------------------------------------------------------------------
-- Não existia NENHUM campo de desconto: o total era subtotal + frete puro.
-- Os dois campos (pct e amount) são gravados porque respondem perguntas
-- diferentes: "que desconto foi negociado" (pct, o que o vendedor prometeu) e
-- "quanto dinheiro saiu" (amount, o que o financeiro reconcilia). Derivar um do
-- outro depois esbarra em arredondamento.
--
-- Os dois são calculados NO SERVIDOR a partir do pct pedido e do teto do cargo.
ALTER TABLE orders ADD COLUMN discount_pct    NUMERIC(5,2)  NOT NULL DEFAULT 0
    CHECK (discount_pct >= 0 AND discount_pct <= 100);
ALTER TABLE orders ADD COLUMN discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0
    CHECK (discount_amount >= 0);

-- ---------------------------------------------------------------------------
-- 4. Fila de aprovação
-- ---------------------------------------------------------------------------
-- 'not_required' é o default e cobre 100% dos pedidos existentes e de todos os
-- pedidos web: desconto acima do teto só existe no balcão.
--
-- Desconto acima do teto NÃO bloqueia a venda (o cliente está no caixa, a
-- mercadoria vai sair) — marca o pedido como pendente e joga na fila do gerente.
CREATE TYPE approval_status AS ENUM ('not_required', 'pending', 'approved', 'rejected');

ALTER TABLE orders ADD COLUMN approval_status  approval_status NOT NULL DEFAULT 'not_required';
ALTER TABLE orders ADD COLUMN approved_by      TEXT;
ALTER TABLE orders ADD COLUMN approved_at      TIMESTAMPTZ;
ALTER TABLE orders ADD COLUMN approval_note    TEXT;

-- A regra "não pode aprovar o próprio desconto" é validada no handler (com
-- teste de regressão), mas fica TAMBÉM aqui: é a única barreira que sobrevive a
-- um bug no handler, a um script de manutenção e a um endpoint futuro.
ALTER TABLE orders ADD CONSTRAINT orders_no_self_approval
    CHECK (approved_by IS NULL OR approved_by IS DISTINCT FROM operator_id);

-- ---------------------------------------------------------------------------
-- 5. Endereço opcional
-- ---------------------------------------------------------------------------
-- shipping_addresses já é uma tabela 1-1 separada, então "sem endereço" é
-- simplesmente a ausência da linha — nenhuma coluna precisa virar NULL e o
-- pedido web continua com a mesma garantia de antes (o handler exige o
-- endereço quando channel='web').

-- ---------------------------------------------------------------------------
-- 6. Índices
-- ---------------------------------------------------------------------------
-- A consulta quente do PDV é "pedidos da MINHA loja", e a do gerente é "o que
-- está pendente na minha loja". Índices parciais: pedido web não entra neles.
CREATE INDEX idx_orders_store    ON orders(store_id, created_at DESC) WHERE channel = 'balcao';
CREATE INDEX idx_orders_operator ON orders(operator_id, created_at DESC) WHERE channel = 'balcao';
CREATE INDEX idx_orders_pending_approval
    ON orders(store_id, created_at DESC) WHERE approval_status = 'pending';

-- ---------------------------------------------------------------------------
-- 7. Trilha de auditoria do balcão
-- ---------------------------------------------------------------------------
-- PORQUÊ separado do tracking_events: tracking conta a jornada do PEDIDO para o
-- CLIENTE ("saiu para entrega"). Isto aqui registra o que uma PESSOA fez, para
-- o DONO ("fulano deu 18% de desconto às 14h32 do IP tal"). Misturar os dois
-- significaria mostrar ao cliente a discussão interna de margem.
--
-- Desconto é dinheiro saindo: cada linha precisa amarrar valor + pessoa + hora.
CREATE TABLE balcao_audit_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id     UUID REFERENCES orders(id) ON DELETE SET NULL,
    action       TEXT NOT NULL,        -- 'order.created' | 'discount.applied' |
                                       -- 'discount.approved' | 'discount.rejected' |
                                       -- 'order.cancelled' | 'discount.capped'
    actor_id     TEXT NOT NULL,        -- quem fez (user_id do JWT)
    actor_role   TEXT,
    store_id     TEXT,
    old_value    JSONB,                -- estado anterior (ex: {"discountPct": 0})
    new_value    JSONB,                -- estado novo    (ex: {"discountPct": 18})
    amount       NUMERIC(12,2),        -- dinheiro envolvido (valor do desconto)
    ip           TEXT,                 -- de onde partiu
    request_id   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_balcao_audit_order  ON balcao_audit_events(order_id, created_at DESC);
CREATE INDEX idx_balcao_audit_actor  ON balcao_audit_events(actor_id, created_at DESC);
CREATE INDEX idx_balcao_audit_store  ON balcao_audit_events(store_id, created_at DESC);
CREATE INDEX idx_balcao_audit_action ON balcao_audit_events(action, created_at DESC);
