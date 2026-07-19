-- ============================================================================
-- Identificação de balcão — SKU e código de barras
-- ----------------------------------------------------------------------------
-- PORQUÊ este arquivo existe SEPARADO do seed: o catálogo do ambiente de
-- desenvolvimento é montado em dois passos — `make catalog-db-seed` carrega os
-- produtos base, e o importador do catálogo curado
-- (`scripts/ingestao/importar_curado.py`) traz os outros ~285. Os produtos
-- importados chegam SEM código de barras (decisão de curadoria, explicada
-- abaixo), então o PDV continuaria com um leitor que não casa com nada se a
-- atribuição só existisse dentro do seed.
--
-- Rode DEPOIS da importação:
--     docker exec -i utilar_catalog_db psql -U utilar -d catalog_service \
--         < services/catalog-service/migrations/balcao_ids.sql
--
-- É IDEMPOTENTE: só toca em linhas onde o campo está NULL. Rodar duas vezes não
-- reetiqueta nada — reetiquetar um produto já rotulado fisicamente na
-- prateleira seria pior que não ter código nenhum.
--
-- ⚠️ O bloco entre os marcadores é ESPELHADO em seed.sql (seção 7.1), para que
-- `make catalog-db-seed` sozinho já entregue um catálogo escaneável. O teste
-- TestSeed_BlocoDeIdentificacaoDeBalcaoNaoDivergiu falha se os dois saírem de
-- sincronia.
-- ============================================================================

-- >>> BALCAO_IDS_BEGIN
-- ----------------------------------------------------------------------------
-- SKU
-- ----------------------------------------------------------------------------
-- Formato: UTL-<CAT>-<NNNN> (UTL-FER-0007, UTL-CON-0031).
--
-- PORQUÊ derivar do id do produto não serve: era exatamente o que o PDV fazia,
-- e um UUID não cabe numa etiqueta nem é ditável por telefone. SKU é código
-- HUMANO — o vendedor lê em voz alta pro depósito e digita no balcão.
--
-- Prefixo `UTL-` distingue do `CUR-` do catálogo curado: dá pra saber a
-- procedência de um item olhando só o código, e garante que os dois conjuntos
-- não colidam no índice único.
--
-- Numeração por row_number (não por hash): determinística, estável entre
-- reseeds e SEM chance de colisão. Hash de 400 linhas colidiria raramente — e
-- "raramente" aqui significa um seed que falha sem explicação no índice único.
--
-- CONTINUA de onde parou (`ultimos`), em vez de reiniciar em 0001: este bloco
-- roda de novo depois da importação do catálogo curado, e reiniciar a
-- contagem daria a um produto novo o código de um já rotulado na prateleira.
WITH ultimos AS (
    SELECT upper(substr(category_id, 1, 3)) AS cat,
           MAX(substr(sku, 9, 4)::int)      AS ultimo
      FROM products
     WHERE sku ~ '^UTL-[A-Z]{3}-[0-9]{4}$'
     GROUP BY 1
),
numerados AS (
    SELECT id,
           upper(substr(category_id, 1, 3)) AS cat,
           row_number() OVER (PARTITION BY upper(substr(category_id, 1, 3)) ORDER BY slug) AS n
      FROM products
     WHERE sku IS NULL
)
UPDATE products p
   SET sku = 'UTL-' || x.cat || '-' || lpad((COALESCE(u.ultimo, 0) + x.n)::text, 4, '0')
  FROM numerados x
  LEFT JOIN ultimos u ON u.cat = x.cat
 WHERE p.id = x.id;

-- ----------------------------------------------------------------------------
-- Código de barras — FAIXA RESERVADA, nunca EAN de fabricante
-- ----------------------------------------------------------------------------
-- DECISÃO (e o porquê dela):
--
-- O catálogo curado omitiu códigos de barras DE PROPÓSITO. Inventar um EAN com
-- prefixo brasileiro (789…) cria identidade física falsa: aquele número
-- pertence, ou vai pertencer, a um produto real de um fabricante real. No dia
-- em que a loja passar o leitor num item de verdade, o código casaria com o
-- produto errado no sistema — e a venda sai com o preço, o custo e o estoque
-- de outra coisa. O seed anterior fazia isso (`'789' || hash(slug)`), e é o
-- que este bloco corrige.
--
-- O que fazemos em vez disso: gerar na FAIXA DE CIRCULAÇÃO RESTRITA do GS1.
-- GTINs que começam com "2" são reservados pela norma para uso interno de uma
-- loja e NUNCA são alocados a fabricante nenhum. É o mesmo mecanismo do código
-- que o supermercado imprime na etiqueta da balança. Portanto:
--
--   * não colide com produto real no leitor — por definição da norma;
--   * é um EAN-13 estruturalmente válido (13 dígitos, dígito verificador
--     correto), então o leitor físico lê e o CHECK do banco aceita;
--   * é reconhecível como interno a olho nu pelo prefixo 200.
--
-- Quando a planilha real do fornecedor chegar com os EANs verdadeiros, eles
-- substituem estes — o UPDATE só preenche NULL, então basta apagar o interno
-- do item que ganhou código de fábrica.
--
-- Formato: 200 + 9 dígitos sequenciais + dígito verificador = 13.
WITH proximo AS (
    -- Continua a sequência (mesmo motivo do SKU): reetiquetar um item já
    -- rotulado fisicamente é pior que não ter código nenhum.
    SELECT COALESCE(MAX(substr(barcode, 4, 9)::bigint), 0) AS ultimo
      FROM products
     WHERE barcode ~ '^200[0-9]{10}$'
),
numerados AS (
    SELECT id, row_number() OVER (ORDER BY slug) AS n
      FROM products
     WHERE barcode IS NULL
),
base AS (
    SELECT nu.id, '200' || lpad((pr.ultimo + nu.n)::text, 9, '0') AS d12
      FROM numerados nu CROSS JOIN proximo pr
),
com_dv AS (
    -- Dígito verificador do EAN-13: soma ponderada 1,3,1,3… das 12 primeiras
    -- posições; o DV é o que falta pro próximo múltiplo de 10.
    SELECT b.id,
           b.d12 || ((10 - (SUM(substr(b.d12, g, 1)::int
                                * CASE WHEN g % 2 = 0 THEN 3 ELSE 1 END) % 10)) % 10)::text AS ean
      FROM base b
      CROSS JOIN generate_series(1, 12) AS g
     GROUP BY b.id, b.d12
)
UPDATE products p
   SET barcode = c.ean
  FROM com_dv c
 WHERE p.id = c.id;
-- <<< BALCAO_IDS_END
