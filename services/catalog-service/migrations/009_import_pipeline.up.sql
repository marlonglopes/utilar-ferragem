-- ============================================================================
-- Pipeline de ingestão multi-formato (Fase B)
-- ----------------------------------------------------------------------------
-- Implementa o esquema desenhado em docs/ingestao-de-produtos.md. A ideia
-- central: separar TRÊS coisas que normalmente viram uma só —
--
--   1. o que chegou            → import_rows.raw   (linha crua, JSONB, imutável)
--   2. como aquilo se traduz   → import_profiles   (mapeamento como DADO)
--   3. o que virou produto     → products          (já existe)
--
-- Com essa separação, planilha de fornecedor novo é um PERFIL novo (linha de
-- tabela), não código novo + deploy. E "de onde veio esse preço?" — a pergunta
-- que sempre aparece três meses depois — é respondível sem pedir o arquivo de
-- volta ao fornecedor.
-- ============================================================================

-- ----------------------------------------------------------------------------
-- import_profiles — o mapeamento coluna→campo, versionado
-- ----------------------------------------------------------------------------
-- Criada ANTES de import_batches porque batches referencia profiles.
--
-- `mapping` é JSONB no formato:
--   {"columns": {"VLR VENDA": {"field": "price", "parser": "money_br"}, ...},
--    "defaults": {"category": "construcao"},
--    "options":  {"maxPriceDropPct": 30, "archiveMissing": false}}
--
-- Versionado porque a planilha do MESMO fornecedor muda com o tempo e é preciso
-- saber qual versão do mapeamento gerou qual importação — sem isso, reprocessar
-- um lote antigo produz resultado diferente e ninguém entende por quê.
CREATE TABLE IF NOT EXISTS import_profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    version     INT  NOT NULL DEFAULT 1 CHECK (version > 0),
    -- kind distingue o perfil genérico (planilha de fornecedor) do importador
    -- especializado do SINAPI, que tem regras próprias de preço (ver abaixo).
    kind        TEXT NOT NULL DEFAULT 'generic' CHECK (kind IN ('generic', 'sinapi')),
    mapping     JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT,
    created_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

-- ----------------------------------------------------------------------------
-- import_batches — um arquivo enviado
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS import_batches (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename     TEXT NOT NULL,
    -- file_hash (sha256): detecta reenvio do MESMO arquivo. Não bloqueia
    -- (reimportar de propósito é legítimo), mas a interface avisa — o modo de
    -- falha real é o operador subir o arquivo de ontem achando que é o de hoje.
    file_hash    TEXT NOT NULL,
    -- format é registrado porque o mesmo conteúdo lido de CSV e de XLSX pode
    -- divergir (o Excel "ajuda"), e a investigação começa por aqui.
    format       TEXT NOT NULL CHECK (format IN ('csv', 'xlsx', 'json', 'sinapi')),
    profile_id   UUID REFERENCES import_profiles(id) ON DELETE SET NULL,
    supplier_id  TEXT,
    -- Máquina de estados. `failed` é falha do LOTE (arquivo ilegível, tamanho
    -- estourado), não de linha — linha ruim vira import_rows.action='reject' e
    -- o lote continua `validated`.
    status       TEXT NOT NULL DEFAULT 'uploaded'
                 CHECK (status IN ('uploaded', 'staged', 'validated', 'committed', 'failed')),
    total_rows   INT NOT NULL DEFAULT 0,
    ok_rows      INT NOT NULL DEFAULT 0,
    error_rows   INT NOT NULL DEFAULT 0,
    -- Contadores do dry-run: o que o COMMIT faria. Persistidos pra que a tela
    -- de aprovação não precise recalcular (e pra que o que foi aprovado fique
    -- registrado como foi aprovado).
    create_count INT NOT NULL DEFAULT 0,
    update_count INT NOT NULL DEFAULT 0,
    reject_count INT NOT NULL DEFAULT 0,
    review_count INT NOT NULL DEFAULT 0,
    -- created_by é TEXT (não UUID) pelo mesmo motivo de catalog_audit_log:
    -- em DEV_MODE o ator vem do header X-User-Id, que é livre. Rejeitar a
    -- linha por formato do ator perderia justo o registro que se queria ter.
    created_by   TEXT,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at TIMESTAMPTZ,
    committed_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_import_batches_created ON import_batches(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_import_batches_hash    ON import_batches(file_hash);
CREATE INDEX IF NOT EXISTS idx_import_batches_status  ON import_batches(status, created_at DESC);

-- ----------------------------------------------------------------------------
-- import_rows — a linha como veio + o que viramos dela
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS import_rows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id    UUID NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    -- row_number é a linha NA PLANILHA (1-based, contando o cabeçalho), não o
    -- índice do array. O operador precisa achar o erro no arquivo dele.
    row_number  INT NOT NULL,
    raw         JSONB NOT NULL,
    mapped      JSONB,
    sku         TEXT,
    -- action: o veredito do dry-run.
    --   create/update → vai escrever
    --   skip          → sem mudança (idempotência: 2ª rodada do mesmo arquivo)
    --   review        → precisa de olho humano (queda de preço acima do limite)
    --   reject        → inválida, NÃO aborta o lote
    action      TEXT CHECK (action IN ('create', 'update', 'skip', 'review', 'reject')),
    errors      JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- warnings ≠ errors: warning não impede o commit (ex.: "unidade
    -- desconhecida, assumindo 'un'"), error impede.
    warnings    JSONB NOT NULL DEFAULT '[]'::jsonb,
    product_id  UUID REFERENCES products(id) ON DELETE SET NULL,
    UNIQUE (batch_id, row_number)
);

CREATE INDEX IF NOT EXISTS idx_import_rows_batch_action ON import_rows(batch_id, action);
CREATE INDEX IF NOT EXISTS idx_import_rows_sku          ON import_rows(sku) WHERE sku IS NOT NULL;

-- ----------------------------------------------------------------------------
-- product_price_history.batch_id — fecha a FK que a 006 deixou aberta
-- ----------------------------------------------------------------------------
-- A migration 006 criou a coluna SEM foreign key porque import_batches ainda
-- não existia. Agora existe. ON DELETE SET NULL: apagar um lote de importação
-- (housekeeping) não pode apagar o histórico de preço do produto — perder o
-- rastro de "por que esse produto está R$ 12" é pior que ter um rastro órfão.
ALTER TABLE product_price_history DROP CONSTRAINT IF EXISTS price_history_batch_fk;
ALTER TABLE product_price_history
    ADD CONSTRAINT price_history_batch_fk
    FOREIGN KEY (batch_id) REFERENCES import_batches(id) ON DELETE SET NULL;

-- ----------------------------------------------------------------------------
-- products.source — de que fonte o produto veio
-- ----------------------------------------------------------------------------
-- PORQUÊ: sem isto não há como responder "quais produtos vieram do SINAPI?" —
-- e essa pergunta é a defesa operacional contra o erro grave: preço do SINAPI
-- é CUSTO DE REFERÊNCIA PARA OBRA PÚBLICA, não preço de varejo. Produto de
-- origem SINAPI precisa ser localizável em massa pra revisão de precificação
-- antes de qualquer publicação.
ALTER TABLE products ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_source_check;
ALTER TABLE products ADD CONSTRAINT products_source_check
    CHECK (source IN ('manual', 'seed', 'import', 'sinapi', 'curated'));

CREATE INDEX IF NOT EXISTS idx_products_source ON products(source);

-- price_reviewed: marca que um humano OLHOU o preço de venda. Produto de
-- origem SINAPI nasce com isto FALSE, e a regra de publicação exige TRUE.
-- É a trava explícita entre "custo oficial de obra pública" e "preço da Utilar".
ALTER TABLE products ADD COLUMN IF NOT EXISTS price_reviewed BOOLEAN NOT NULL DEFAULT true;

-- ----------------------------------------------------------------------------
-- TRAVA DE BANCO: produto sem preço revisado NÃO pode estar publicado
-- ----------------------------------------------------------------------------
-- Última linha de defesa, no mesmo espírito dos CHECKs da 005. Se um bug no
-- importador, um script manual ou um UPDATE apressado tentar publicar um item
-- importado do SINAPI sem revisão de preço, o banco recusa.
--
-- Produtos existentes (seed/legado) já entram com price_reviewed=true pelo
-- DEFAULT, então a constraint é satisfeita na criação e nada some da vitrine.
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_published_needs_review;
ALTER TABLE products ADD CONSTRAINT products_published_needs_review
    CHECK (status <> 'published' OR price_reviewed = true);

-- ----------------------------------------------------------------------------
-- sinapi_compositions / sinapi_composition_items — base de conhecimento
-- ----------------------------------------------------------------------------
-- PORQUÊ tabelas próprias e NÃO `products`: uma composição SINAPI é um SERVIÇO
-- de obra ("alvenaria de bloco cerâmico, por m²"), não um item de prateleira.
-- Enfiar isso em `products` colocaria serviço na vitrine da ferragem.
--
-- O VALOR está nos coeficientes: "1 m² de alvenaria consome 13,5 blocos +
-- 0,012 m³ de argamassa". É exatamente a base de conhecimento que a assistente
-- precisa pra responder "quantos blocos pra um muro de 20 m²?" — e vinda de
-- fonte OFICIAL e CITÁVEL, não de alucinação.
--
-- Este serviço apenas ARMAZENA e serve os coeficientes. O consumo é do
-- assistant-service (outro dono) — por isso nenhuma dependência daqui pra lá.
CREATE TABLE IF NOT EXISTS sinapi_compositions (
    code        TEXT PRIMARY KEY,           -- código SINAPI da composição
    description TEXT NOT NULL,
    unit        TEXT NOT NULL,              -- unidade do SERVIÇO: M2, M3, M, UN
    -- reference_cost: custo total de referência da composição, por unidade.
    -- MESMO AVISO do preço de insumo: é custo de obra pública, não preço.
    reference_cost NUMERIC(14,4),
    uf          TEXT,                       -- UF da tabela de referência
    reference_month TEXT,                   -- 'MM/AAAA' da tabela usada
    -- desonerado: a Caixa publica duas versões (com e sem desoneração da folha).
    -- Misturar as duas numa mesma base produz orçamento errado silenciosamente.
    desonerado  BOOLEAN NOT NULL DEFAULT false,
    batch_id    UUID REFERENCES import_batches(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sinapi_composition_items (
    composition_code TEXT NOT NULL REFERENCES sinapi_compositions(code) ON DELETE CASCADE,
    -- item_type: uma composição consome INSUMOS e outras COMPOSIÇÕES
    -- (recursivo). Sem distinguir, a resolução de "material total" entra em
    -- loop ou soma serviço como se fosse material.
    item_type   TEXT NOT NULL CHECK (item_type IN ('insumo', 'composicao')),
    item_code   TEXT NOT NULL,
    description TEXT NOT NULL,
    unit        TEXT NOT NULL,
    -- coefficient: O DADO QUE IMPORTA. Quantas unidades do item por 1 unidade
    -- da composição. Escala 7 porque coeficiente de SINAPI chega a 0,0000123
    -- e arredondar aqui vira erro de material multiplicado pela metragem.
    coefficient NUMERIC(18,7) NOT NULL,
    PRIMARY KEY (composition_code, item_type, item_code)
);

CREATE INDEX IF NOT EXISTS idx_sinapi_items_item ON sinapi_composition_items(item_code);
