-- ============================================================================
-- Devolução e troca — OBRIGAÇÃO LEGAL, não feature
-- ----------------------------------------------------------------------------
-- Até aqui não existia fluxo NENHUM: o cliente não tinha como pedir devolução e
-- a loja não tinha como registrar. Isso é descumprimento direto do Código de
-- Defesa do Consumidor, não um item de backlog.
--
-- As DUAS bases legais são diferentes e não podem virar um campo só:
--
--   CDC art. 49 — ARREPENDIMENTO. Compra fora do estabelecimento (toda venda
--     online é) dá 7 dias CORRIDOS para desistir. É DIREITO INCONDICIONAL:
--     não se pergunta o motivo, não se avalia, não se recusa. O prazo conta do
--     RECEBIMENTO ("da assinatura ou do ato de recebimento"), não da compra.
--     O frete de devolução é da loja.
--
--   CDC art. 26 — VÍCIO DO PRODUTO. Defeito. 30 dias (não durável) ou 90 dias
--     (durável) a contar de quando o vício ficou EVIDENTE. Aqui sim existe
--     análise, e a loja tem 30 dias para sanar antes de dever troca/devolução.
--
-- Modelar as duas como "solicitação de devolução com um motivo" faria o
-- atendente avaliar um arrependimento — que não se avalia — e é assim que a
-- loja toma multa do Procon. Por isso `kind` é uma coluna própria, e é ela que
-- decide se existe etapa de análise.
--
-- ⚠️ QUEM RESPONDE PELA DEVOLUÇÃO depende de a Utilar ser LOJISTA (vende o
-- próprio produto) ou MARKETPLACE (intermedeia a venda de terceiros). Este
-- schema implementa o caminho do LOJISTA. Ver docs/devolucao-e-troca.md §
-- "Lojista ou marketplace" para o que mudaria.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- 1. Tipos
-- ---------------------------------------------------------------------------

-- A base legal. NÃO é "o motivo" — é qual artigo do CDC rege o pedido, e isso
-- muda o prazo, se há análise e quem paga o frete.
CREATE TYPE return_kind AS ENUM (
    'regret',  -- art. 49: arrependimento, 7 dias, INCONDICIONAL, sem análise
    'defect'   -- art. 26: vício do produto, com análise
);

-- Estados. A ordem importa: estoque só volta em 'received', dinheiro só sai em
-- 'refunded'. Ver internal/returns/transition.go.
CREATE TYPE return_status AS ENUM (
    'requested',   -- cliente pediu
    'approved',    -- deferido (automático no arrependimento; humano no vício)
    'rejected',    -- indeferido (só é alcançável a partir de 'defect')
    'in_transit',  -- mercadoria a caminho de volta
    'received',    -- mercadoria CONFERIDA na loja  → estoque volta AQUI
    'refunded',    -- dinheiro devolvido            → lançamento contábil AQUI
    'cancelled'    -- o próprio cliente desistiu da devolução
);

-- ---------------------------------------------------------------------------
-- 2. orders — o que faltava para conseguir decidir uma devolução
-- ---------------------------------------------------------------------------

-- PORQUÊ esta coluna existe: a Appmax só aceita estorno TOTAL em pedido com
-- Payment Split (docs/appmax-v1-appstore.md § 5). Sem saber disso ANTES, uma
-- devolução parcial legítima seria aceita aqui, o cliente seria avisado de que
-- o dinheiro está voltando, e a chamada só falharia lá no PSP — com o produto
-- já devolvido e o estoque já reposto. O erro precisa acontecer na hora do
-- pedido de devolução, não na hora do estorno.
--
-- DEFAULT false é CORRETO no modelo LOJISTA: split só existe quando há um
-- terceiro recebendo parte do dinheiro, o que só acontece em marketplace.
-- Se a Utilar for marketplace, esta coluna passa a ter um produtor de verdade
-- (o evento payment.confirmed teria que carregá-la) e a trava fica ativa.
ALTER TABLE orders ADD COLUMN payment_split BOOLEAN NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- 3. order_returns — o pedido de devolução
-- ---------------------------------------------------------------------------
CREATE TABLE order_returns (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    -- ON DELETE RESTRICT e não CASCADE: devolução é documento fiscal e
    -- contábil. Apagar o pedido não pode apagar a prova de que houve estorno.

    -- user_id DUPLICA orders.user_id de propósito. A autorização de leitura
    -- ("o cliente só vê a PRÓPRIA devolução") não pode depender de um JOIN que
    -- alguém esqueça de escrever numa query nova.
    user_id     TEXT NOT NULL,

    kind        return_kind   NOT NULL,
    status      return_status NOT NULL DEFAULT 'requested',

    -- reason_code é taxonomia fechada (validada na aplicação); reason_text é o
    -- texto livre do cliente.
    --
    -- No ARREPENDIMENTO os dois são OPCIONAIS e ficam apenas como informação de
    -- negócio. Exigir motivo em direito incondicional é criar atrito onde a lei
    -- proíbe atrito. O CHECK abaixo garante que a exigência só vale no vício.
    reason_code TEXT,
    reason_text TEXT,

    -- deadline_basis é a data DA QUAL o prazo foi contado, congelada no ato do
    -- pedido. Sem congelar, um `delivered_at` corrigido meses depois mudaria
    -- retroativamente se a devolução foi tempestiva — e a prova de que a loja
    -- agiu certo evaporaria.
    deadline_basis    TIMESTAMPTZ,
    deadline_at       TIMESTAMPTZ,
    -- basis_source registra DE ONDE saiu a data: 'delivered_at', 'shipped_at',
    -- 'not_delivered' (pedido ainda não entregue) ou 'unknown'.
    --
    -- 'unknown' é o caso real e chato: o pedido não tem data de entrega
    -- registrada. Ver a política em internal/returns/deadline.go — resumo: sem
    -- data de recebimento a loja NÃO PODE alegar prazo vencido, porque o ônus
    -- de provar a data da entrega é dela.
    basis_source      TEXT NOT NULL DEFAULT 'unknown',

    -- Valor a estornar, congelado na aprovação a partir dos itens + rateio.
    refund_amount     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (refund_amount >= 0),
    -- Frete devolvido junto. No arrependimento a loja arca com o frete de ida
    -- e volta (art. 49, parágrafo único: valores pagos são devolvidos
    -- "monetariamente atualizados" — a jurisprudência consolidada inclui o
    -- frete). Coluna separada porque o contador precisa ver os dois.
    refund_shipping   NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (refund_shipping >= 0),

    requested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_by    TEXT,
    decided_at    TIMESTAMPTZ,
    decision_note TEXT,
    shipped_at    TIMESTAMPTZ,  -- cliente postou a mercadoria de volta
    received_at   TIMESTAMPTZ,  -- CONFERIDA na loja
    refunded_at   TIMESTAMPTZ,

    -- stock_returned é o flag que impede a devolução de estoque em duplicidade
    -- e permite ao financeiro achar o que ficou pendente quando o
    -- catalog-service estava fora na hora do recebimento.
    stock_returned    BOOLEAN NOT NULL DEFAULT false,
    -- ledger_posted idem, para o lançamento contábil.
    ledger_posted     BOOLEAN NOT NULL DEFAULT false,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Invariantes no BANCO, não só no handler. Estas sobrevivem a um bug no
    -- handler, a um script de manutenção e a um endpoint futuro.

    -- Vício exige motivo. Arrependimento NÃO pode exigir.
    CONSTRAINT returns_defect_requires_reason
        CHECK (kind <> 'defect' OR (reason_code IS NOT NULL AND length(trim(reason_code)) > 0)),

    -- Recusa exige justificativa. "Recusado" sem motivo deixa o cliente sem
    -- saber o que fazer e a loja sem defesa no Procon.
    CONSTRAINT returns_rejection_requires_note
        CHECK (status <> 'rejected' OR (decision_note IS NOT NULL AND length(trim(decision_note)) > 0)),

    -- ⚠️ ARREPENDIMENTO NÃO SE RECUSA. Direito incondicional do art. 49.
    -- Esta é a barreira que impede um atendente (ou um endpoint futuro) de
    -- "indeferir" uma desistência dentro do prazo.
    CONSTRAINT returns_regret_cannot_be_rejected
        CHECK (NOT (kind = 'regret' AND status = 'rejected')),

    -- Quem decidiu e quando andam juntos: decisão sem pessoa é decisão sem
    -- responsável, e estorno é dinheiro saindo.
    CONSTRAINT returns_decision_complete
        CHECK ((decided_by IS NULL AND decided_at IS NULL)
            OR (decided_by IS NOT NULL AND decided_at IS NOT NULL)),

    -- Dinheiro devolvido tem que ter hora e valor.
    CONSTRAINT returns_refund_complete
        CHECK (status <> 'refunded' OR (refunded_at IS NOT NULL AND refund_amount > 0))
);

CREATE INDEX idx_returns_order  ON order_returns(order_id, requested_at DESC);
CREATE INDEX idx_returns_user   ON order_returns(user_id, requested_at DESC);
-- Fila do atendimento: o que está aberto, mais antigo primeiro.
CREATE INDEX idx_returns_open   ON order_returns(status, requested_at)
    WHERE status IN ('requested', 'approved', 'in_transit', 'received');
-- Relatório de pendências: recebido mas com estoque ou livro em atraso.
CREATE INDEX idx_returns_pendencias ON order_returns(received_at)
    WHERE status IN ('received','refunded') AND (stock_returned = false OR ledger_posted = false);

CREATE TRIGGER trg_returns_updated BEFORE UPDATE ON order_returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- 4. order_return_items — DEVOLUÇÃO PARCIAL
-- ---------------------------------------------------------------------------
-- O cliente compra 10 itens e devolve 1. Uma coluna `order_id` no
-- order_returns não resolve isso: precisa dizer QUAL item e QUANTOS.
CREATE TABLE order_return_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id     UUID NOT NULL REFERENCES order_returns(id) ON DELETE CASCADE,
    order_item_id UUID NOT NULL REFERENCES order_items(id) ON DELETE RESTRICT,
    product_id    UUID NOT NULL,
    quantity      INT  NOT NULL CHECK (quantity > 0),
    -- unit_price é SNAPSHOT do preço pago, copiado de order_items no ato.
    -- Estornar pelo preço atual do catálogo devolveria o valor errado quando o
    -- produto tivesse mudado de preço — a favor ou contra a loja.
    unit_price    NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    line_amount   NUMERIC(12,2) NOT NULL CHECK (line_amount >= 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- O mesmo item do pedido não pode aparecer duas vezes NA MESMA devolução.
    -- (Duas devoluções distintas do mesmo item são legítimas: devolveu 1 de 10
    -- hoje, mais 1 amanhã. O limite de quantidade acumulada é validado na
    -- aplicação, em internal/returns.)
    CONSTRAINT return_items_unicos UNIQUE (return_id, order_item_id)
);

CREATE INDEX idx_return_items_return ON order_return_items(return_id);
-- Consulta quente da validação: "quanto deste item do pedido já foi devolvido".
CREATE INDEX idx_return_items_order_item ON order_return_items(order_item_id);

-- ---------------------------------------------------------------------------
-- 5. return_audit_events — a trilha do dinheiro saindo
-- ---------------------------------------------------------------------------
-- ⚠️ ESTORNO É DINHEIRO SAINDO. Precisa da mesma qualidade de rastro que a
-- liquidação externa (a outra operação do sistema que move dinheiro por decisão
-- humana): quem, quando, quanto, de onde.
--
-- Tabela PRÓPRIA e não balcao_audit_events: aquela é escopada por loja e por
-- operador de PDV. Devolução é do canal web também, e o ator costuma ser o
-- atendente ou o próprio cliente. Misturar as duas faria o índice por store_id
-- ficar cheio de NULL e a consulta do balcão varrer devoluções web.
--
-- A gravação é FAIL-CLOSED na mesma transação: se a trilha não grava, a
-- decisão não acontece. Mesmo princípio de auditTx.
CREATE TABLE return_audit_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id  UUID REFERENCES order_returns(id) ON DELETE SET NULL,
    order_id   UUID REFERENCES orders(id) ON DELETE SET NULL,
    action     TEXT NOT NULL,   -- 'return.requested' | 'return.approved' |
                                -- 'return.rejected'  | 'return.received' |
                                -- 'return.refunded'  | 'return.stock_restored' |
                                -- 'return.stock_restore_failed' |
                                -- 'return.ledger_post_failed'
    actor_id   TEXT NOT NULL,
    actor_role TEXT,
    old_value  JSONB,
    new_value  JSONB,
    amount     NUMERIC(12,2),
    ip         TEXT,
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_return_audit_return ON return_audit_events(return_id, created_at DESC);
CREATE INDEX idx_return_audit_order  ON return_audit_events(order_id, created_at DESC);
CREATE INDEX idx_return_audit_actor  ON return_audit_events(actor_id, created_at DESC);
CREATE INDEX idx_return_audit_action ON return_audit_events(action, created_at DESC);
