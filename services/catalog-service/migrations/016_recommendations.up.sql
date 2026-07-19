-- ============================================================================
-- Recomendação de produto — co-compra agregada + regra técnica
-- ============================================================================
--
-- O QUE HAVIA ANTES:
--
--     SELECT ... WHERE category_id = (do produto atual) AND slug != $1
--     ORDER BY rating DESC LIMIT 4
--
-- Numa categoria como `ferramentas` (dezenas de produtos) isso devolve OS
-- MESMOS 4 ITENS para TODO produto da categoria — os 4 de maior `rating`, que
-- por sua vez era um número inventado pelo seed (ver migration 015). Era uma
-- lista fixa fantasiada de recomendação, e uma que ficava idêntica em 100% das
-- páginas de produto daquela categoria.
--
-- Esta migration cria as duas fontes que substituem aquilo, mais o estado do
-- job que mantém a primeira.
-- ============================================================================


-- ============================================================================
-- 1. CO-COMPRA — "quem levou isto levou também"
-- ============================================================================
--
-- FONTE DO DADO: `stock_reservations` com status='committed', que já vive
-- NESTE banco. Toda compra online passa por reserva (POST /internal/reservations)
-- e por confirmação (POST /internal/reservations/:orderId/commit), ambas
-- chamadas pelo order-service. Ou seja: o catálogo já tem a cesta de cada
-- pedido, sem nenhum acesso ao banco de pedidos e sem nenhuma chamada de rede.
--
-- ⚠️ LGPD — POR QUE ESTA TABELA NÃO TEM NEM `order_id` NEM USUÁRIO
--
-- O sinal usado é AGREGADO por construção: a tabela guarda um par de produtos e
-- QUANTOS pedidos distintos contiveram os dois. Não há como reconstruir a cesta
-- de ninguém a partir daqui, e a recomendação exibida NÃO depende de quem está
-- olhando — dois visitantes na mesma página de produto veem a mesma lista.
-- Isso é deliberado: perfil de compra individual é dado pessoal, e recomendação
-- personalizada por histórico é tratamento que exigiria base legal, aviso e
-- controle de oposição que a loja não tem. O agregado entrega quase todo o
-- valor sem entrar nesse território.
--
-- ⚠️ POR QUE UMA TABELA MATERIALIZADA E NÃO UM JOIN NA HORA DA LEITURA
--
-- A consulta "honesta" seria um self-join em stock_reservations por order_id a
-- cada visualização de produto. Isso varre o histórico de pedidos INTEIRO para
-- desenhar 4 cards, e o custo cresce com o número de pedidos já feitos — ou
-- seja, quanto mais a loja vende, mais lenta fica a página de produto. É o
-- mesmo modo de falha do item 1 de docs/performance-banco.md (custo
-- proporcional ao catálogo inteiro para entregar 24 linhas). A medição está em
-- docs/reviews-e-recomendacao.md.
--
-- Aqui a leitura é um index scan por `product_id` numa tabela de pares, e o
-- trabalho pesado é feito FORA do caminho da requisição, incrementalmente, pelo
-- job de internal/reco/copurchase.go.
-- ============================================================================

CREATE TABLE IF NOT EXISTS product_copurchase (
    product_id         UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    related_product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,

    -- Número de PEDIDOS DISTINTOS que continham os dois produtos. É este número
    -- que o mínimo de ocorrências filtra: com o limiar em 1, duas compras
    -- coincidentes viram "recomendação", e a vitrine passa a sugerir ruído com
    -- cara de estatística.
    order_count        INT NOT NULL DEFAULT 0 CHECK (order_count >= 0),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (product_id, related_product_id),
    CONSTRAINT product_copurchase_no_self CHECK (product_id <> related_product_id)
);

-- O índice que faz a leitura ser O(top-N) e não O(pares do produto): a consulta
-- é sempre `WHERE product_id = $1 AND order_count >= $2 ORDER BY order_count DESC LIMIT n`.
CREATE INDEX IF NOT EXISTS idx_copurchase_lookup
    ON product_copurchase (product_id, order_count DESC);

-- Estado do job incremental. Uma linha só — o CHECK(id) trava isso no banco em
-- vez de confiar em quem escrever o próximo INSERT.
CREATE TABLE IF NOT EXISTS copurchase_refresh_state (
    id          BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    -- Marca d'água: até que instante os pedidos confirmados já foram contados.
    -- O job processa a janela (watermark, cutoff] e avança. É isso que impede
    -- o refresh de recontar o histórico inteiro a cada rodada.
    -- ⚠️ 'epoch' e NÃO '-infinity'. O lib/pq não converte os infinitos do
    -- Postgres para time.Time — devolve []byte cru, e o Scan falha com
    -- "unsupported Scan, storing driver.Value type []uint8 into type
    -- *time.Time". O job quebraria na primeira execução de uma instalação
    -- nova, que é o pior momento possível. 1970 é anterior a qualquer reserva.
    watermark   TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    last_run_at TIMESTAMPTZ,
    last_pairs  INT NOT NULL DEFAULT 0
);
INSERT INTO copurchase_refresh_state (id) VALUES (true) ON CONFLICT (id) DO NOTHING;

-- Índice da JANELA do job. Sem ele o refresh incremental faz seq scan em
-- stock_reservations só para descobrir o que mudou desde a última rodada — o
-- job barato viraria o job caro. Parcial em 'committed': é o único status que
-- interessa, e reservas ativas/liberadas são a maioria das linhas.
CREATE INDEX IF NOT EXISTS idx_stock_reservations_committed_window
    ON stock_reservations (updated_at)
    WHERE status = 'committed';

-- Montagem da cesta a partir dos order_id da janela. `(order_id, product_id)`
-- porque o self-join do job agrupa por pedido e projeta o produto.
CREATE INDEX IF NOT EXISTS idx_stock_reservations_committed_basket
    ON stock_reservations (order_id, product_id)
    WHERE status = 'committed';


-- ============================================================================
-- 2. COMPLEMENTAR POR REGRA TÉCNICA
-- ============================================================================
--
-- Co-compra só funciona depois que houve compra. No dia 1 — e para todo produto
-- da cauda longa, que é a maioria do catálogo — a tabela acima está vazia, e
-- ficar sem recomendação nenhuma seria desperdiçar conhecimento que a loja já
-- tem: quem leva porcelanato PRECISA de argamassa AC-III, espaçador e rejunte.
-- Isso é fato técnico de aplicação, não estatística — não depende de ninguém
-- ter comprado antes, e é justamente o que um vendedor de balcão diria.
--
-- FORMATO DA REGRA: (categoria de origem + termo de origem) → (categoria de
-- destino + termo de destino). Os termos são casados contra `search_vector`,
-- a coluna tsvector da migration 014 — então a regra pega "Porcelanato
-- Acetinado 60x60", "Piso Cerâmico..." e o que mais entrar no catálogo depois,
-- sem virar uma lista de SKU que envelhece.
--
-- `websearch_to_tsquery` e não `to_tsquery`: aceita `OR` e NUNCA levanta erro de
-- sintaxe. Uma regra mal escrita deixa de casar; não derruba a página de
-- produto com 500. Numa tabela de conteúdo editável, tolerar o texto ruim é
-- requisito, não conveniência.
--
-- `note` é OBRIGATÓRIA e é exibível: a razão técnica ("Porcelanato exige
-- argamassa AC-III") é o que transforma um card genérico em recomendação que o
-- cliente entende — e é o que permite ao frontend não chamar de "quem comprou
-- também levou" algo que ninguém comprou.
-- ============================================================================

CREATE TABLE IF NOT EXISTS product_complement_rules (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    source_category_id TEXT REFERENCES categories(id) ON DELETE CASCADE,
    source_query       TEXT,
    target_category_id TEXT REFERENCES categories(id) ON DELETE CASCADE,
    target_query       TEXT,

    note               TEXT NOT NULL CHECK (length(note) BETWEEN 3 AND 240),
    -- Menor primeiro. Empate resolvido por `id` para a lista ser estável entre
    -- requisições (paginação sem ordem total é bug de resultado — ver a nota
    -- equivalente em productOrderBy).
    priority           INT NOT NULL DEFAULT 100,
    active             BOOLEAN NOT NULL DEFAULT true,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Regra sem NENHUM critério de origem casaria com o catálogo inteiro;
    -- sem critério de destino, recomendaria o catálogo inteiro. As duas pontas
    -- precisam de pelo menos um lado.
    CONSTRAINT complement_rule_has_source CHECK (source_category_id IS NOT NULL OR source_query IS NOT NULL),
    CONSTRAINT complement_rule_has_target CHECK (target_category_id IS NOT NULL OR target_query IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_complement_rules_source
    ON product_complement_rules (source_category_id, priority)
    WHERE active;

CREATE TRIGGER trg_product_complement_rules_updated
    BEFORE UPDATE ON product_complement_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ----------------------------------------------------------------------------
-- Regras iniciais. Todas são fatos de aplicação de ferragem/construção — o tipo
-- de coisa que o cliente descobre que faltava quando já está na obra.
-- ----------------------------------------------------------------------------
INSERT INTO product_complement_rules (source_category_id, source_query, target_category_id, target_query, note, priority)
VALUES
    -- Piso / porcelanato: o trio clássico do assentamento.
    ('construcao', 'piso OR porcelanato OR ceramica', 'construcao', 'argamassa',
     'Porcelanato e piso cerâmico exigem argamassa colante — AC-III para porcelanato.', 10),
    ('construcao', 'piso OR porcelanato OR ceramica', 'construcao', 'rejunte',
     'O rejunte fecha as juntas depois do assentamento.', 20),
    ('construcao', 'piso OR porcelanato OR ceramica', 'ferramentas', 'espacador OR espaçador',
     'Espaçadores mantêm a junta uniforme durante o assentamento.', 30),
    ('construcao', 'argamassa', 'ferramentas', 'desempenadeira OR colher',
     'A argamassa colante é aplicada com desempenadeira dentada.', 40),

    -- Hidráulica: PVC soldável não cola sem adesivo, e não adere sem lixar.
    ('hidraulica', 'tubo OR pvc', 'hidraulica', 'adesivo OR cola',
     'Tubo de PVC soldável só veda com adesivo plástico específico.', 10),
    ('hidraulica', 'tubo OR pvc', 'hidraulica', 'conexao OR joelho OR luva',
     'Conexões (joelhos e luvas) fecham o trecho da tubulação.', 20),
    ('hidraulica', 'tubo OR rosca', 'hidraulica', 'veda OR fita',
     'Rosca de tubulação precisa de fita veda-rosca para não vazar.', 30),

    -- Elétrica: cabo passa dentro de eletroduto; circuito termina em disjuntor.
    ('eletrica', 'cabo OR fio', 'eletrica', 'eletroduto',
     'O cabo deve correr dentro de eletroduto na infraestrutura embutida.', 10),
    ('eletrica', 'cabo OR fio', 'eletrica', 'disjuntor',
     'Cada circuito precisa do disjuntor dimensionado para a bitola do cabo.', 20),
    ('eletrica', 'tomada OR interruptor', 'eletrica', 'caixa',
     'Tomadas e interruptores são instalados sobre caixa de embutir.', 30),

    -- Fixação: parafuso em alvenaria sem bucha não segura.
    ('fixacao', 'parafuso', 'fixacao', 'bucha',
     'Parafuso em alvenaria precisa de bucha compatível com o diâmetro.', 10),
    ('fixacao', 'parafuso OR bucha', 'ferramentas', 'broca',
     'A bucha exige furo do diâmetro correto — broca compatível.', 20),

    -- Ferramentas: furadeira sem broca não fura; corte/desbaste pede EPI.
    ('ferramentas', 'furadeira OR parafusadeira', 'ferramentas', 'broca',
     'A furadeira não acompanha brocas — escolha o jogo pelo material.', 10),
    ('ferramentas', 'esmerilhadeira OR lixadeira OR serra', 'seguranca', 'oculos OR protecao',
     'Corte e desbaste projetam partículas: óculos de proteção é obrigatório (NR-6).', 10),

    -- Pintura: tinta sozinha não pinta parede.
    ('pintura', 'tinta', 'pintura', 'rolo OR pincel',
     'A aplicação pede rolo para área ampla e pincel para recorte.', 10),
    ('pintura', 'tinta', 'pintura', 'fita OR crepe',
     'Fita crepe delimita rodapé e batente antes de pintar.', 20),
    ('pintura', 'tinta OR massa', 'pintura', 'lixa OR massa',
     'Massa corrida e lixa preparam a superfície antes da tinta.', 30)
ON CONFLICT DO NOTHING;

ANALYZE product_complement_rules;
