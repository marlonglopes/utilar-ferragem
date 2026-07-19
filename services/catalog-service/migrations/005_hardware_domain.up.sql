-- ============================================================================
-- Domínio de loja de ferragem / material de construção
-- ----------------------------------------------------------------------------
-- PORQUÊ: até aqui `products` descrevia um marketplace genérico (nome, preço,
-- marca, specs livres). Uma ferragem vende por unidade de medida, precisa de
-- custo pra negociar no balcão, de código de barras pra ler no PDV, de peso e
-- dimensões pra calcular frete real, e de dados fiscais pra emitir NFC-e.
-- Tudo isso hoje ou não existe, ou está escondido dentro do NOME do produto
-- ("Tijolo Cerâmico (cento)"), onde nenhum código consegue usar.
-- ============================================================================

ALTER TABLE products
    -- cost: SENSÍVEL. É a base da trava de margem do PDV. Hoje o balcão estima
    -- custo como preço × 0,72 — um chute que erra feio em linha branca vs
    -- cimento. NUNCA pode sair em rota pública (ver handler/product.go).
    ADD COLUMN IF NOT EXISTS cost            NUMERIC(12,2),

    -- unit_of_measure: "sc" (saco), "br" (barra), "m", "m3", "un"... Sem isso
    -- não dá pra exibir "R$ 34,90 / saco" nem comparar preço entre fornecedores.
    -- Default 'un' porque o catálogo legado inteiro é venda por unidade.
    ADD COLUMN IF NOT EXISTS unit_of_measure TEXT NOT NULL DEFAULT 'un',

    -- qty_step: passo de venda. 1 para unidade, 0.5 para meio metro de cabo,
    -- 0.25 para m³ de areia. O frontend usa pra montar o stepper de quantidade.
    ADD COLUMN IF NOT EXISTS qty_step        NUMERIC(12,3) NOT NULL DEFAULT 1,

    -- barcode: EAN-8/12/13/14 (GTIN). Leitura de scanner no balcão.
    ADD COLUMN IF NOT EXISTS barcode         TEXT,

    -- Frete real: saco de cimento (50 kg) e parafuso (0,01 kg) não custam o
    -- mesmo pra entregar. O order-service hoje aproxima por item.
    ADD COLUMN IF NOT EXISTS weight_kg       NUMERIC(10,3),
    ADD COLUMN IF NOT EXISTS length_cm       NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS width_cm        NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS height_cm       NUMERIC(10,2),

    -- Origem do item: de qual fornecedor veio e com qual código lá. Permite
    -- reimportar planilha do mesmo fornecedor e rastrear "de onde veio isso".
    ADD COLUMN IF NOT EXISTS supplier_id     TEXT,
    ADD COLUMN IF NOT EXISTS supplier_sku    TEXT,

    -- Fiscais: obrigatórios para NF-e/NFC-e. Aqui só guardamos os campos —
    -- a emissão de nota NÃO é escopo deste serviço.
    ADD COLUMN IF NOT EXISTS ncm             TEXT,   -- 8 dígitos
    ADD COLUMN IF NOT EXISTS cfop            TEXT,   -- 4 dígitos
    ADD COLUMN IF NOT EXISTS cest            TEXT,   -- 7 dígitos
    ADD COLUMN IF NOT EXISTS origem          SMALLINT; -- 0..8 (tabela ICMS)

-- ----------------------------------------------------------------------------
-- stock INT → NUMERIC
-- ----------------------------------------------------------------------------
-- PORQUÊ: não dá pra vender 2,5 m de cabo nem 1,5 m³ de areia com estoque
-- inteiro. `INT` era o bloqueio real da venda fracionada.
--
-- A ATOMICIDADE DA RESERVA CONTINUA INTACTA: o `UPDATE products SET stock =
-- stock - $n WHERE id = $id AND stock >= $n` de handler/reservation.go depende
-- do row lock + reavaliação do predicado (EvalPlanQual) do Postgres, não do
-- tipo da coluna. NUMERIC compara e subtrai com a mesma semântica — e com
-- exatidão decimal, ao contrário de float. O CHECK stock >= 0 sobrevive ao
-- ALTER TYPE e segue sendo a última linha de defesa.
--
-- Escala 3 casas: cobre m³/kg/m com folga e mantém o valor exato (NUMERIC não
-- é binário, então 0.1 + 0.2 = 0.3 de verdade).
ALTER TABLE products ALTER COLUMN stock TYPE NUMERIC(14,3) USING stock::numeric;
ALTER TABLE products ALTER COLUMN stock SET DEFAULT 0;

-- ----------------------------------------------------------------------------
-- Validação no banco — última linha de defesa contra seed/migration manual
-- ----------------------------------------------------------------------------
ALTER TABLE products
    ADD CONSTRAINT products_cost_nonneg      CHECK (cost IS NULL OR cost >= 0),
    -- qty_step 0 travaria o stepper do frontend em loop infinito.
    ADD CONSTRAINT products_qty_step_pos     CHECK (qty_step > 0),
    ADD CONSTRAINT products_uom_sane         CHECK (unit_of_measure <> '' AND length(unit_of_measure) <= 10),
    ADD CONSTRAINT products_weight_nonneg    CHECK (weight_kg IS NULL OR weight_kg >= 0),
    ADD CONSTRAINT products_dims_nonneg      CHECK (
        (length_cm IS NULL OR length_cm >= 0) AND
        (width_cm  IS NULL OR width_cm  >= 0) AND
        (height_cm IS NULL OR height_cm >= 0)),
    -- GTIN-8/12/13/14. Rejeitar aqui evita o clássico "7.89123E+12" que o Excel
    -- produz ao tratar EAN como número.
    ADD CONSTRAINT products_barcode_format   CHECK (barcode IS NULL OR barcode ~ '^[0-9]{8,14}$'),
    ADD CONSTRAINT products_ncm_format       CHECK (ncm  IS NULL OR ncm  ~ '^[0-9]{8}$'),
    ADD CONSTRAINT products_cfop_format      CHECK (cfop IS NULL OR cfop ~ '^[0-9]{4}$'),
    ADD CONSTRAINT products_cest_format      CHECK (cest IS NULL OR cest ~ '^[0-9]{7}$'),
    ADD CONSTRAINT products_origem_range     CHECK (origem IS NULL OR (origem >= 0 AND origem <= 8));

-- Código de barras é identidade física do item: não pode repetir. Parcial
-- porque a maioria do catálogo legado não tem EAN cadastrado.
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_barcode
    ON products(barcode) WHERE barcode IS NOT NULL;

-- Busca por SKU no balcão: o vendedor digita o começo do código. Trigram
-- porque `ILIKE 'ABC%'` não usa índice B-tree com collation não-C — mesmo
-- padrão já usado em idx_products_name_trgm.
CREATE INDEX IF NOT EXISTS idx_products_sku_trgm
    ON products USING gin (sku gin_trgm_ops);

-- Reimportação: "todos os itens do fornecedor X".
CREATE INDEX IF NOT EXISTS idx_products_supplier
    ON products(supplier_id, supplier_sku) WHERE supplier_id IS NOT NULL;
