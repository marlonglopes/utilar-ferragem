-- ============================================================================
-- Atributos tipados por categoria (registry) + valores por produto
-- ----------------------------------------------------------------------------
-- PORQUÊ: hoje `products.specs` é JSONB livre com TUDO string —
-- `{"Peso":"1,7 kg","Potência":"650 W"}`. Isso é bom pra exibir e péssimo pra
-- tudo mais: não filtra ("furadeira acima de 700 W"), não ordena, não compara,
-- e a mesma grandeza aparece com cinco grafias diferentes ("Potência",
-- "potencia", "Potência (W)"). As facetas da vitrine hoje são só marca + preço
-- porque não há como facetar cima de string livre.
--
-- Desenho: DOIS objetos separados.
--   1. `category_attributes` — o REGISTRY: quais grandezas existem em cada
--      categoria, de que tipo, em que unidade, e se entram nos filtros.
--      É configuração, editável sem deploy.
--   2. `product_attributes` — o VALOR tipado por produto, em coluna própria
--      por tipo (num/text/bool) pra que o índice e o operador de comparação
--      sejam os do Postgres, não string.
--
-- `specs` NÃO é removido: continua sendo a ficha técnica de exibição livre.
-- O registry é o subconjunto que a máquina entende.
-- ============================================================================

CREATE TABLE IF NOT EXISTS category_attributes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,   -- identificador estável, snake_case: 'potencia_w'
    label       TEXT NOT NULL,   -- rótulo pt-BR pra UI: 'Potência'
    data_type   TEXT NOT NULL CHECK (data_type IN ('number', 'text', 'bool')),
    unit        TEXT,            -- 'W', 'mm', 'V' — NULL para text/bool
    filterable  BOOLEAN NOT NULL DEFAULT false,
    sort_order  INT NOT NULL DEFAULT 0,
    -- spec_key: de qual chave do `specs` legado este atributo foi extraído.
    -- Guardado pra permitir re-backfill quando a ingestão trouxer dados novos.
    spec_key    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (category_id, key)
);

CREATE INDEX IF NOT EXISTS idx_category_attributes_cat
    ON category_attributes(category_id, sort_order);

CREATE TABLE IF NOT EXISTS product_attributes (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value_num  NUMERIC(18,6),
    value_text TEXT,
    value_bool BOOLEAN,
    PRIMARY KEY (product_id, key),
    -- Exatamente um dos três preenchido. Sem isso, um valor "1,7" gravado em
    -- value_text some do filtro numérico e ninguém percebe.
    CONSTRAINT product_attributes_one_value CHECK (
        (value_num  IS NOT NULL)::int +
        (value_text IS NOT NULL)::int +
        (value_bool IS NOT NULL)::int = 1
    )
);

-- Faceta numérica (min/max, histograma) e faceta de valores discretos.
CREATE INDEX IF NOT EXISTS idx_product_attributes_num
    ON product_attributes(key, value_num) WHERE value_num IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_product_attributes_text
    ON product_attributes(key, value_text) WHERE value_text IS NOT NULL;

-- ----------------------------------------------------------------------------
-- catalog_num_from_spec — extrai número de string de ficha técnica brasileira
-- ----------------------------------------------------------------------------
-- "650 W" → 650 | "1,7 kg" → 1.7 | "11.000 rpm" → 11000 | "≈ 360 m²" → 360
-- Retorna NULL quando não há número reconhecível, em vez de estourar — uma
-- célula ruim não pode abortar o backfill de 4.000 linhas.
CREATE OR REPLACE FUNCTION catalog_num_from_spec(v TEXT) RETURNS NUMERIC AS $$
DECLARE
    tok TEXT;
BEGIN
    IF v IS NULL THEN RETURN NULL; END IF;
    -- Primeiro token numérico da string (ignora prefixos como "≈ " e "0 – ").
    tok := substring(v from '[0-9][0-9.,]*');
    IF tok IS NULL THEN RETURN NULL; END IF;
    tok := rtrim(tok, '.,');

    IF position(',' in tok) > 0 THEN
        -- Formato BR: ponto é milhar, vírgula é decimal.
        tok := replace(replace(tok, '.', ''), ',', '.');
    ELSIF tok ~ '^[0-9]{1,3}(\.[0-9]{3})+$' THEN
        -- "11.000" sem vírgula: ponto é separador de milhar, não decimal.
        tok := replace(tok, '.', '');
    END IF;

    RETURN tok::numeric;
EXCEPTION WHEN others THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ----------------------------------------------------------------------------
-- Registry inicial + backfill a partir de `specs`
-- ----------------------------------------------------------------------------
-- O registry começa com as grandezas que o catálogo atual já carrega em specs,
-- pra que o backfill tenha o que migrar. Categorias sem specs estruturadas
-- ganham só o esqueleto (peso), que a ingestão preenche depois.
-- JOIN com `categories` em vez de INSERT direto: numa base recém-criada as
-- migrations rodam ANTES do seed, e as categorias ainda não existem. Sem o
-- JOIN a migration falha na FK e o serviço não sobe do zero. Categoria
-- ausente simplesmente não gera registry — o seed cria os dois juntos.
INSERT INTO category_attributes (category_id, key, label, data_type, unit, filterable, sort_order, spec_key)
SELECT v.category_id, v.key, v.label, v.data_type, v.unit, v.filterable, v.sort_order, v.spec_key
FROM (VALUES
-- ferramentas
    ('ferramentas', 'potencia_w',  'Potência',      'number', 'W',  true,  1, 'Potência'),
    ('ferramentas', 'tensao',      'Tensão',        'text',   NULL, true,  2, 'Tensão'),
    ('ferramentas', 'mandril_mm',  'Mandril',       'number', 'mm', true,  3, 'Mandril'),
    ('ferramentas', 'peso_kg',     'Peso',          'number', 'kg', true,  4, 'Peso'),
    ('ferramentas', 'garantia',    'Garantia',      'text',   NULL, false, 5, 'Garantia'),
    -- eletrica
    ('eletrica',    'secao_mm2',   'Seção',         'number', 'mm²', true, 1, 'Seção'),
    ('eletrica',    'tensao',      'Tensão',        'text',   NULL,  true, 2, 'Tensão'),
    ('eletrica',    'comprimento_m','Comprimento',  'number', 'm',   true, 3, 'Comprimento'),
    ('eletrica',    'cor',         'Cor',           'text',   NULL,  true, 4, 'Cor'),
    -- construcao
    ('construcao',  'peso_kg',     'Peso',          'number', 'kg', true,  1, 'Peso'),
    ('construcao',  'tipo',        'Tipo',          'text',   NULL, true,  2, 'Tipo'),
    ('construcao',  'aplicacao',   'Aplicação',     'text',   NULL, false, 3, 'Aplicação'),
    -- pintura
    ('pintura',     'volume_l',    'Volume',        'number', 'L',  true,  1, 'Volume'),
    ('pintura',     'acabamento',  'Acabamento',    'text',   NULL, true,  2, 'Acabamento'),
    ('pintura',     'tipo',        'Tipo',          'text',   NULL, true,  3, 'Tipo'),
    -- hidraulica
    ('hidraulica',  'diametro_mm', 'Diâmetro',      'number', 'mm', true,  1, 'Diâmetro'),
    ('hidraulica',  'material',    'Material',      'text',   NULL, true,  2, 'Material'),
    -- fixacao
    ('fixacao',     'bitola_mm',   'Bitola',        'number', 'mm', true,  1, 'Bitola'),
    ('fixacao',     'comprimento_mm','Comprimento', 'number', 'mm', true,  2, 'Comprimento'),
    ('fixacao',     'material',    'Material',      'text',   NULL, true,  3, 'Material'),
    -- seguranca
    ('seguranca',   'ca',          'CA',            'text',   NULL, true,  1, 'CA'),
    ('seguranca',   'classe',      'Classe',        'text',   NULL, true,  2, 'Classe'),
    ('seguranca',   'material',    'Material',      'text',   NULL, true,  3, 'Material'),
    -- jardim
    ('jardim',      'peso_kg',     'Peso',          'number', 'kg', true,  1, 'Peso'),
    ('jardim',      'material',    'Material',      'text',   NULL, true,  2, 'Material')
) AS v(category_id, key, label, data_type, unit, filterable, sort_order, spec_key)
JOIN categories c ON c.id = v.category_id
ON CONFLICT (category_id, key) DO NOTHING;

-- Backfill: para cada atributo do registry com spec_key, extrai o valor do
-- specs do produto daquela categoria. Numéricos passam pelo parser BR; textos
-- entram normalizados (trim). Valor vazio ou não-parseável é simplesmente
-- ignorado — atributo ausente é informação válida, atributo errado não é.
INSERT INTO product_attributes (product_id, key, value_num, value_text)
SELECT p.id,
       ca.key,
       CASE WHEN ca.data_type = 'number' THEN catalog_num_from_spec(p.specs->>ca.spec_key) END,
       CASE WHEN ca.data_type = 'text'   THEN nullif(btrim(p.specs->>ca.spec_key), '') END
FROM products p
JOIN category_attributes ca ON ca.category_id = p.category_id AND ca.spec_key IS NOT NULL
WHERE p.specs ? ca.spec_key
  AND CASE
        WHEN ca.data_type = 'number' THEN catalog_num_from_spec(p.specs->>ca.spec_key) IS NOT NULL
        WHEN ca.data_type = 'text'   THEN nullif(btrim(p.specs->>ca.spec_key), '') IS NOT NULL
        ELSE false
      END
ON CONFLICT (product_id, key) DO NOTHING;
