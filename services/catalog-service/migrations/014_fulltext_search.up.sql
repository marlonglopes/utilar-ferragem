-- Busca textual por tsvector + GIN, com relevância em português.
--
-- ============================================================================
-- O PROBLEMA QUE ISSO RESOLVE
-- ============================================================================
--
-- A busca antiga era:
--
--   p.name ILIKE $1 OR p.description ILIKE $1 OR s.name ILIKE $1 OR ...
--
-- Os índices trigram (idx_products_name_trgm, idx_products_sku_trgm) EXISTIAM e
-- NUNCA eram usados. A causa é `s.name` dentro do mesmo OR: é coluna da tabela
-- do JOIN, e o planejador não consegue satisfazer o predicado por índice em
-- `products` — teria que provar que nenhum outro ramo do OR casa, e um deles
-- depende do JOIN. Resultado medido em 150k produtos: Parallel Seq Scan,
-- 6.110 buffers, custo proporcional ao catálogo INTEIRO.
--
-- Além de lento, era ruim: "eletrica" não achava "elétrica", não tinha plural,
-- e o resultado saía em ordem de cadastro — sem nenhuma noção de relevância.
--
-- ============================================================================
-- POR QUE UMA CONFIGURAÇÃO DE BUSCA PRÓPRIA (`utilar_pt`)
-- ============================================================================
--
-- Coluna `GENERATED ALWAYS AS (...) STORED` EXIGE expressão IMMUTABLE.
--   • `to_tsvector('portuguese', x)` (2 argumentos) é IMMUTABLE. ✅
--   • `unaccent(x)` NÃO é immutable — depende do dicionário instalado. ❌
--
-- Ou seja: `to_tsvector('portuguese', unaccent(name))` é REJEITADO pelo
-- Postgres numa coluna gerada. As três saídas possíveis eram:
--
--   1. Coluna mantida por TRIGGER — funciona, mas troca uma declaração
--      verificada pelo banco por código imperativo que precisa cobrir INSERT
--      *e* UPDATE, e que silenciosamente desatualiza o vetor se alguém
--      esquecer um caminho de escrita.
--   2. Wrapper `IMMUTABLE` sobre `unaccent` — funciona, mas é uma MENTIRA ao
--      planejador: se o dicionário mudar, o índice fica errado e ninguém fica
--      sabendo. Corrupção silenciosa de índice é o pior modo de falha possível.
--   3. Configuração de busca própria com `unaccent` no dicionário. ← ESCOLHIDA
--
-- A (3) é a única honesta: o `unaccent` vira parte do DICIONÁRIO da config, não
-- uma chamada de função na expressão. Aí `to_tsvector('utilar_pt', x)` é o
-- to_tsvector de 2 argumentos de sempre — immutable DE VERDADE, sem mentira —
-- e a coluna gerada é aceita. A dependência fica registrada no catálogo do
-- Postgres (a config não pode ser dropada com a coluna usando).

CREATE EXTENSION IF NOT EXISTS unaccent;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'utilar_pt') THEN
        EXECUTE 'CREATE TEXT SEARCH CONFIGURATION utilar_pt ( COPY = portuguese )';
        -- unaccent ANTES do stemmer: "elétrica" → "eletrica" → "eletr".
        -- A ordem importa — o stemmer português não sabe o que fazer com
        -- acento, então tirar depois não adiantaria nada.
        EXECUTE 'ALTER TEXT SEARCH CONFIGURATION utilar_pt
                 ALTER MAPPING FOR asciiword, asciihword, hword_asciipart,
                                   word, hword, hword_part,
                                   numword, numhword, hword_numpart
                 WITH unaccent, portuguese_stem';
    END IF;
END $$;

-- ============================================================================
-- NOME DO VENDEDOR DESNORMALIZADO — é isso que tira o JOIN do WHERE
-- ============================================================================
--
-- Coluna gerada só enxerga colunas da PRÓPRIA LINHA. `s.name` não podia entrar
-- direto. A denormalização em `seller_name_cache` é o que permite o vetor
-- inteiro (inclusive o vendedor) morar em `products`, e com isso o predicado de
-- busca vira uma coluna só, de uma tabela só — indexável por GIN.
--
-- A consistência é mantida por DOIS gatilhos (abaixo), não por convenção.

ALTER TABLE products ADD COLUMN IF NOT EXISTS seller_name_cache TEXT;

-- Backfill dos produtos existentes ANTES de criar a coluna gerada — a coluna
-- gerada é computada no momento do ADD COLUMN, e se o cache ainda estivesse
-- NULL o vetor nasceria sem o vendedor.
UPDATE products p
   SET seller_name_cache = s.name
  FROM sellers s
 WHERE s.id = p.seller_id
   AND p.seller_name_cache IS DISTINCT FROM s.name;

-- Gatilho 1: produto novo (ou que troca de vendedor) preenche o cache.
--
-- ⚠️ `SET search_path` NÃO É ENFEITE — sem ele esta função quebra o restore.
-- pg_dump/pg_restore rodam com `search_path = ''`, e aí o `FROM sellers` de
-- dentro do corpo não resolve: `ERROR: relation "sellers" does not exist`, e a
-- restauração do backup morre no meio da tabela de produtos. Descoberto na
-- prática ao carregar o dump real na base de medição. Vale lembrar que
-- "backup nunca restaurado" já é pendência aberta em CLAUDE.md — este seria
-- exatamente o tipo de surpresa que só aparece na hora do desastre.
CREATE OR REPLACE FUNCTION products_fill_seller_name_cache() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    SELECT s.name INTO NEW.seller_name_cache FROM sellers s WHERE s.id = NEW.seller_id;
    RETURN NEW;
END;
$$;

-- BEFORE: a coluna gerada é recalculada DEPOIS dos gatilhos BEFORE, então o
-- search_vector já nasce com o nome do vendedor correto na mesma escrita.
DROP TRIGGER IF EXISTS trg_products_seller_name_cache ON products;
CREATE TRIGGER trg_products_seller_name_cache
    BEFORE INSERT OR UPDATE OF seller_id ON products
    FOR EACH ROW EXECUTE FUNCTION products_fill_seller_name_cache();

-- Gatilho 2: vendedor renomeado propaga para os produtos dele.
--
-- Sem isso o vetor guardaria para sempre o nome ANTIGO da loja, e a busca por
-- fornecedor apontaria para um nome que não existe mais — o modo de falha
-- clássico de denormalização, que não dá erro, só dá resultado errado.
--
-- Custo: renomear vendedor faz UPDATE em todos os produtos dele (recalcula o
-- vetor de cada um). É operação rara, de admin, e o WHERE cai em
-- idx_products_seller. Não está no caminho da venda.
CREATE OR REPLACE FUNCTION sellers_propagate_name_to_products() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    -- IS DISTINCT FROM (e não <>) porque name é NOT NULL hoje, mas um UPDATE
    -- que não muda o nome não pode custar uma varredura nos produtos.
    IF NEW.name IS DISTINCT FROM OLD.name THEN
        UPDATE products SET seller_name_cache = NEW.name WHERE seller_id = NEW.id;
    END IF;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_sellers_name_to_products ON sellers;
CREATE TRIGGER trg_sellers_name_to_products
    AFTER UPDATE OF name ON sellers
    FOR EACH ROW EXECUTE FUNCTION sellers_propagate_name_to_products();

-- ============================================================================
-- O VETOR, COM PESOS
-- ============================================================================
--
-- Peso é o que transforma "presença" em "relevância". Sem ele, um produto que
-- cita "furadeira" de passagem na descrição empata com a furadeira de verdade.
--
--   A — nome do produto      (o que o usuário está procurando)
--   B — marca e SKU          (identificadores fortes)
--   C — descrição            (contexto)
--   D — nome do vendedor     (o mais fraco: quase nunca é a intenção da busca)
--
-- ts_rank_cd usa esses pesos com os multiplicadores default {0.1, 0.2, 0.4, 1.0}
-- para D,C,B,A — nome vale 10x o vendedor.
--
-- ⚠️ ADD COLUMN de coluna gerada REESCREVE A TABELA (lock ACCESS EXCLUSIVE).
-- Em 150k produtos levou ~2,5 s. Ver a nota de produção no fim do arquivo.
ALTER TABLE products ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('utilar_pt', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('utilar_pt', coalesce(brand, '') || ' ' || coalesce(sku, '')), 'B') ||
        setweight(to_tsvector('utilar_pt', coalesce(description, '')), 'C') ||
        setweight(to_tsvector('utilar_pt', coalesce(seller_name_cache, '')), 'D')
    ) STORED;

-- PARCIAL em status='published' pelo mesmo motivo dos índices da 013: é o
-- filtro fixo da rota pública, o único consumidor da busca textual. O admin
-- lista por outros status e não usa busca textual.
CREATE INDEX IF NOT EXISTS idx_products_search_vector
    ON products USING gin (search_vector)
    WHERE status = 'published';

-- ============================================================================
-- ⚠️ PRODUÇÃO — CREATE INDEX CONCURRENTLY e a reescrita de tabela
-- ============================================================================
--
-- O golang-migrate roda cada migration DENTRO de uma transação, e
-- CREATE INDEX CONCURRENTLY não pode rodar em transação. Além disso, o
-- ADD COLUMN da coluna gerada reescreve `products` com ACCESS EXCLUSIVE —
-- e para ESSE não existe versão concorrente.
--
-- Se o catálogo já estiver grande e a loja não puder parar de vender, aplique à
-- mão, em janela de manutenção curta, ANTES de subir o serviço:
--
--   -- 1) fora de transação, um comando por vez:
--   CREATE EXTENSION IF NOT EXISTS unaccent;
--   CREATE TEXT SEARCH CONFIGURATION utilar_pt ( COPY = portuguese );
--   ALTER TEXT SEARCH CONFIGURATION utilar_pt
--     ALTER MAPPING FOR asciiword, asciihword, hword_asciipart, word, hword,
--                       hword_part, numword, numhword, hword_numpart
--     WITH unaccent, portuguese_stem;
--   ALTER TABLE products ADD COLUMN seller_name_cache TEXT;
--   UPDATE products p SET seller_name_cache = s.name
--     FROM sellers s WHERE s.id = p.seller_id;     -- em lotes, se for enorme
--   -- (gatilhos: copie os dois CREATE FUNCTION/TRIGGER acima)
--   ALTER TABLE products ADD COLUMN search_vector tsvector
--     GENERATED ALWAYS AS (...) STORED;            -- ← a janela de lock
--   CREATE INDEX CONCURRENTLY idx_products_search_vector
--     ON products USING gin (search_vector) WHERE status = 'published';
--   ANALYZE products;
--
--   -- 2) marcar como aplicada, senão o migrate tenta de novo e falha:
--   INSERT INTO schema_migrations (version, dirty) VALUES (14, false)
--     ON CONFLICT (version) DO UPDATE SET dirty = false;
--
-- (com dirty = true o serviço se recusa a subir.)

-- Estatística do GIN novo: sem ANALYZE o planejador não sabe a seletividade da
-- coluna e pode escolher seq scan mesmo com o índice pronto.
ANALYZE products;
