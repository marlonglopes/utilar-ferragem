-- ============================================================================
-- Seed do catalog-service
-- ----------------------------------------------------------------------------
-- Carrega:
--   * 8 categorias (taxonomia fixa)
--   * 11 sellers reais dos mocks
--   * 31 produtos reais portados de app/src/lib/mockProducts.ts
--   * ~70 produtos sintéticos extras para chegar a 100+ linhas (testes de paginação)
-- ============================================================================

BEGIN;

-- CASCADE derruba junto product_price_tiers, product_attributes,
-- product_price_history e category_attributes (todos com FK para
-- products/categories). `category_attributes` é recriada abaixo porque o seed
-- precisa ser reprodutível sozinho, sem depender de reaplicar a migration 008.
TRUNCATE TABLE product_images, products, sellers, categories RESTART IDENTITY CASCADE;

-- ---------------------------------------------------------------------------
-- 1) Categorias (taxonomia fixa — 8 top-level)
-- ---------------------------------------------------------------------------
INSERT INTO categories (id, name, icon, sort_order) VALUES
    ('ferramentas', 'Ferramentas',        '⚒', 1),
    ('construcao',  'Construção',         '◫', 2),
    ('eletrica',    'Elétrica',           '⚡', 3),
    ('hidraulica',  'Hidráulica',         '◡', 4),
    ('pintura',     'Pintura',            '▥', 5),
    ('jardim',      'Jardim',             '❀', 6),
    ('seguranca',   'Segurança',          '⚠', 7),
    ('fixacao',     'Fixação',            '▣', 8);

-- ---------------------------------------------------------------------------
-- 2) Sellers (extraídos dos mocks)
-- ---------------------------------------------------------------------------
INSERT INTO sellers (id, name, rating, review_count, verified) VALUES
    ('ferragem-silva',  'Ferragem Silva',  4.8, 1240, true),
    ('pro-tools-br',    'Pro Tools BR',    4.6, 870,  true),
    ('casa-obra',       'Casa & Obra',     4.5, 620,  true),
    ('material-braz',   'Material Braz',   4.4, 380,  false),
    ('eletrica-costa',  'Elétrica Costa',  4.9, 2100, true),
    ('materiais-sp',    'Materiais SP',    4.3, 510,  false),
    ('hidro-total',     'Hidro Total',     4.7, 940,  true),
    ('tintas-rio',      'Tintas Rio',      4.8, 760,  true),
    ('verde-vida',      'Verde Vida',      4.6, 430,  true),
    ('epi-pro',         'EPI Pro',         4.9, 1850, true),
    ('parafusos-sp',    'Parafusos SP',    4.7, 690,  false);

-- ---------------------------------------------------------------------------
-- 3) Produtos reais (31) — portados de mockProducts.ts
-- ---------------------------------------------------------------------------
INSERT INTO products (slug, name, category_id, seller_id, price, original_price, icon, brand, stock, rating, review_count, cashback_amount, badge, badge_label, installments, description, specs) VALUES
-- ferramentas
('furadeira-bosch-gsb-13-re',    'Furadeira de Impacto Bosch GSB 13 RE 650W 127V',            'ferramentas', 'ferragem-silva', 329.00, 389.00, '⚒', 'Bosch',       42, 5, 142, 24.90, 'discount'::product_badge,      '-15%',          12, 'A Furadeira de Impacto Bosch GSB 13 RE é ideal para furar concreto, madeira e metal. Com 650W de potência e mandril de 13mm, oferece alta performance para obras e reformas. Bivolt automático, incluindo 2 brocas.', '{"Potência":"650 W","Tensão":"127 V","Mandril":"13 mm","Velocidade":"0 – 2.800 rpm","Peso":"1,7 kg","Garantia":"12 meses"}'::jsonb),
('parafusadeira-makita-df333',   'Parafusadeira Makita DF333DWYE 12V Bivolt',                 'ferramentas', 'ferragem-silva', 459.00, NULL,   '⚒', 'Makita',      18, 5, 201, NULL,  NULL,                           NULL,            12, 'Parafusadeira/furadeira a bateria Makita DF333DWYE 12V, leve e compacta para trabalhos em locais de difícil acesso. Inclui 2 baterias BL1015 e carregador.', '{"Tensão da Bateria":"12 V","Torque máx.":"30 Nm","Velocidades":"2","Mandril":"10 mm","Peso (com bateria)":"1,0 kg","Garantia":"12 meses"}'::jsonb),
('furadeira-bosch-gsb-16-re',    'Furadeira de Impacto Bosch GSB 16 RE 750W Bivolt',          'ferramentas', 'pro-tools-br',   419.00, NULL,   '⚒', 'Bosch',       27, 5, 214, NULL,  'free_shipping'::product_badge, 'Frete grátis',  NULL, NULL, '{"Potência":"750 W","Tensão":"Bivolt","Mandril":"13 mm","Velocidade":"0 – 3.000 rpm","Peso":"1,9 kg"}'::jsonb),
('martelete-bosch-gbh-2-24',     'Martelete Bosch GBH 2-24 D SDS Plus 790W Bivolt',           'ferramentas', 'ferragem-silva',1089.00, NULL,   '⚒', 'Bosch',        9, 5,  87, 32.67, NULL,                           NULL,            12, NULL, '{"Potência":"790 W","Energia de impacto":"2,7 J","Sistema":"SDS Plus","Peso":"2,7 kg","Tensão":"Bivolt"}'::jsonb),
('esmerilhadeira-bosch-gws-700', 'Esmerilhadeira Bosch GWS 700 4.1/2" 127V',                  'ferramentas', 'casa-obra',      289.00, NULL,   '⚒', 'Bosch',        3, 4,  63, NULL,  'last_units'::product_badge,    'Últimas 3',     NULL, NULL, '{"Potência":"700 W","Disco":"4.1/2\"","Tensão":"127 V","Velocidade":"11.000 rpm","Peso":"1,6 kg"}'::jsonb),
('lixadeira-bosch-gss-140',      'Lixadeira Orbital Bosch GSS 140 180W Bivolt',               'ferramentas', 'pro-tools-br',   499.00, NULL,   '⚒', 'Bosch',       15, 4,  54, 24.95, NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('serra-tico-tico-bosch-gst-650','Serra Tico-Tico Bosch GST 650 500W Bivolt',                 'ferramentas', 'ferragem-silva', 389.00, NULL,   '⚒', 'Bosch',       21, 5, 112, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('rompedor-bosch-gsh-5-ce',      'Rompedor Bosch GSH 5 CE SDS Max 1100W Bivolt',              'ferramentas', 'pro-tools-br',  2799.00, 3499.00,'⚒', 'Bosch',        6, 5,  29, 83.97, 'discount'::product_badge,      '-20%',          12, NULL, '{}'::jsonb),
-- construcao
('cimento-votoran-50kg',         'Cimento CP II-E-32 Votoran 50kg',                           'construcao',  'casa-obra',       42.90, NULL,   '◫', 'Votoran',    200, 4,  87, NULL,  NULL,                           NULL,            NULL, NULL, '{"Tipo":"CP II-E-32","Peso":"50 kg","Marca":"Votoran","Aplicação":"Uso geral"}'::jsonb),
('argamassa-quartzolit-20kg',    'Argamassa AC-II Quartzolit 20kg',                           'construcao',  'casa-obra',       28.50, NULL,   '◫', 'Quartzolit', 150, 4,  43, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('tijolo-ceramico-9-furos',      'Tijolo Cerâmico 9 Furos 9x14x19cm (cento)',                 'construcao',  'material-braz',   89.00, NULL,   '◫', NULL,          80, 4,  31, NULL,  'free_shipping'::product_badge, 'Frete grátis',  NULL, NULL, '{}'::jsonb),
('tela-soldada-galvanizada',     'Tela Soldada Galvanizada 1x25m Fio 1,5mm',                  'construcao',  'material-braz',  149.00, NULL,   '◫', NULL,          35, 4,  19, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
-- eletrica
('cabo-flexivel-2-5mm-100m',     'Cabo Flexível 2,5mm² 100m Rolo Azul Sil',                   'eletrica',    'eletrica-costa', 249.00, NULL,   '⚡', 'Sil',         60, 5, 203, 12.50, 'free_shipping'::product_badge, 'Frete grátis',  NULL, NULL, '{"Seção":"2,5 mm²","Comprimento":"100 m","Cor":"Azul","Marca":"Sil","Norma":"NBR NM 247-3"}'::jsonb),
('disjuntor-bipolar-20a-schneider','Disjuntor Bipolar 20A Schneider Domae',                   'eletrica',    'eletrica-costa',  34.90, NULL,   '⚡', 'Schneider',  120, 5,  88, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('tomada-tramontina-liz-10a',    'Tomada 2P+T 10A Tramontina Liz Branca',                     'eletrica',    'materiais-sp',    12.90, NULL,   '⚡', 'Tramontina', 300, 4, 162, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('quadro-distribuicao-12-disjuntores','Quadro de Distribuição 12 Disjuntores Embutir',        'eletrica',    'eletrica-costa',  89.90, NULL,   '⚡', NULL,          44, 4,  57, NULL,  NULL,                           NULL,            3,    NULL, '{}'::jsonb),
-- hidraulica
('kit-pvc-soldavel-25mm',        'Kit Tubo PVC Soldável 25mm 6m + 10 Conexões',               'hidraulica',  'hidro-total',     89.90, NULL,   '◡', NULL,          55, 4,  54, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('registro-gaveta-deca-3-4',     'Registro de Gaveta Bronze 3/4" Deca',                       'hidraulica',  'hidro-total',     38.50, NULL,   '◡', 'Deca',        90, 5,  76, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('caixa-dagua-500l-fortlev',     'Caixa d''Água Polietileno 500L Fortlev',                    'hidraulica',  'hidro-total',    379.00, NULL,   '◡', 'Fortlev',     12, 5,  34, NULL,  'free_shipping'::product_badge, 'Frete grátis',  10,   NULL, '{}'::jsonb),
-- pintura
('tinta-suvinil-fosco-18l',      'Tinta Acrílica Suvinil Fosco Premium 18L Branco',           'pintura',     'tintas-rio',     279.00, NULL,   '▥', 'Suvinil',     38, 5, 318, 25.00, NULL,                           NULL,            NULL, NULL, '{"Volume":"18 L","Acabamento":"Fosco","Tipo":"Acrílica","Marca":"Suvinil","Rendimento":"≈ 360 m²/demão","Secagem":"1 hora"}'::jsonb),
('rolo-la-23cm-tigre',           'Rolo de Lã 23cm Tigre Cabo 15cm',                           'pintura',     'tintas-rio',      18.90, NULL,   '▥', 'Tigre',      200, 4,  94, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('massa-corrida-pva-25kg-suvinil','Massa Corrida PVA 25kg Suvinil',                           'pintura',     'tintas-rio',      89.00, NULL,   '▥', 'Suvinil',     50, 4,  47, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
-- jardim
('mangueira-jardim-50m-tramontina','Mangueira de Jardim 50m 1/2" Tramontina',                 'jardim',      'verde-vida',      79.90, NULL,   '❀', 'Tramontina',  45, 4,  83, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('enxada-reta-tramontina-1400g', 'Enxada Reta 1400g Tramontina com Cabo',                     'jardim',      'verde-vida',      49.90, NULL,   '❀', 'Tramontina',  70, 5,  61, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
-- seguranca
('capacete-seguranca-classe-b',  'Capacete de Segurança Classe B Branco CA 31469',            'seguranca',   'epi-pro',         29.90, NULL,   '⚠', NULL,         150, 5,  72, 6.00,  'last_units'::product_badge,    'Últimas',       NULL, NULL, '{"Classe":"B","Cor":"Branco","CA":"31469","Material":"PEAD","Norma":"ABNT NBR 8221"}'::jsonb),
('luva-seguranca-vaqueta-m',     'Luva de Segurança Vaqueta Tamanho M Par',                   'seguranca',   'epi-pro',         14.90, NULL,   '⚠', NULL,         300, 4, 108, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('oculos-protecao-3m-incolor',   'Óculos de Proteção Ampla Visão Incolor 3M',                 'seguranca',   'epi-pro',         19.90, NULL,   '⚠', '3M',         200, 5,  95, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
-- fixacao
('kit-parafusos-autoatarraxantes-500','Kit Parafusos Autoatarraxantes Sortidos 500pç',        'fixacao',     'parafusos-sp',    34.90, NULL,   '▣', NULL,         120, 4,  96, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('bucha-nylon-s8-kit-100',       'Bucha Nylon S8 com Parafuso Kit 100pç',                     'fixacao',     'parafusos-sp',    22.90, NULL,   '▣', NULL,         400, 5, 134, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('prego-com-cabeca-17x27-1kg',   'Prego com Cabeça 17x27 1kg Gerdau',                         'fixacao',     'parafusos-sp',     9.90, NULL,   '▣', 'Gerdau',     500, 4,  52, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb),
('fita-veda-rosca-18x50m',       'Fita Veda Rosca 18mm x 50m Tigre',                          'fixacao',     'materiais-sp',     7.50, NULL,   '▣', 'Tigre',      600, 5, 187, NULL,  NULL,                           NULL,            NULL, NULL, '{}'::jsonb);

-- ---------------------------------------------------------------------------
-- 4) Produtos sintéticos (70+ extras) — para chegar a 100+ linhas totais
--     Gerados por CTE com variação por categoria, seller, preço e brand.
-- ---------------------------------------------------------------------------
WITH
cat AS (SELECT id, icon, row_number() OVER (ORDER BY sort_order) AS rn FROM categories),
sel AS (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM sellers),
brand_list AS (
    SELECT unnest(ARRAY['Bosch','Makita','DeWalt','Black+Decker','Tramontina','Suvinil','Quartzolit','Schneider','Deca','Fortlev','Gerdau','Tigre','3M','Sil','Votoran']) AS b,
           generate_series(1, 15) AS bn
),
gen AS (
    SELECT n,
           (n % 8) + 1  AS cat_rn,
           (n % 11) + 1 AS sel_rn,
           ((n * 7) % 15) + 1 AS brand_rn
    FROM generate_series(1, 80) AS n
)
INSERT INTO products (slug, name, category_id, seller_id, price, original_price, icon, brand, stock, rating, review_count, cashback_amount, badge, badge_label, installments, description, specs)
SELECT
    'seed-item-' || lpad(gen.n::text, 3, '0'),
    'Produto de Teste ' || gen.n || ' — ' || c.id,
    c.id,
    s.id,
    round((19.90 + (gen.n * 17) % 3980)::numeric, 2),
    CASE WHEN gen.n % 6 = 0 THEN round((49.90 + (gen.n * 23) % 4999)::numeric, 2) ELSE NULL END,
    c.icon,
    bl.b,
    (gen.n * 13) % 500,
    ((gen.n % 5) + 1),
    (gen.n * 3) % 250,
    CASE WHEN gen.n % 4 = 0 THEN round((1 + (gen.n % 30))::numeric, 2) ELSE NULL END,
    (ARRAY[NULL, 'discount'::product_badge, 'free_shipping'::product_badge, 'last_units'::product_badge])[((gen.n % 4) + 1)],
    (ARRAY[NULL, '-10%', 'Frete grátis', 'Últimas unidades'])[((gen.n % 4) + 1)],
    CASE WHEN gen.n % 3 = 0 THEN 12 ELSE NULL END,
    'Produto sintético gerado por seed para testes de paginação e filtros.',
    jsonb_build_object(
        'seed', true,
        'variante', gen.n,
        'cor', (ARRAY['Preto','Branco','Azul','Vermelho','Verde','Amarelo'])[((gen.n % 6) + 1)]
    )
FROM gen
JOIN cat c  ON c.rn  = gen.cat_rn
JOIN sel s  ON s.rn  = gen.sel_rn
JOIN brand_list bl ON bl.bn = gen.brand_rn;

-- ---------------------------------------------------------------------------
-- 5) Product images — 2 imagens por produto (picsum placeholder)
-- ---------------------------------------------------------------------------
INSERT INTO product_images (product_id, url, alt, sort_order)
SELECT p.id,
       'https://picsum.photos/seed/' || p.slug || '-1/800/800',
       p.name || ' — imagem principal',
       0
FROM products p;

INSERT INTO product_images (product_id, url, alt, sort_order)
SELECT p.id,
       'https://picsum.photos/seed/' || p.slug || '-2/800/800',
       p.name || ' — detalhe',
       1
FROM products p;

-- ---------------------------------------------------------------------------
-- 6) Domínio de ferragem (migration 005): custo, unidade, código de barras,
--    peso e fiscal.
-- ---------------------------------------------------------------------------
-- PORQUÊ um UPDATE em vez de 14 colunas a mais em cada INSERT acima: as linhas
-- de produto já são largas demais pra revisar, e a regra "qual unidade cada
-- categoria usa" fica legível como regra, não diluída em 111 literais.
--
-- Custo: margem plausível de ferragem — 28% em ferramenta de marca (giro
-- lento, preço tabelado), 18% em material básico (cimento, areia: commodity
-- de giro alto e margem apertada), 35% em elétrica/fixação (item pequeno,
-- margem maior).
UPDATE products SET
    cost = round(price * CASE category_id
                            WHEN 'construcao'  THEN 0.82   -- commodity, margem apertada
                            WHEN 'ferramentas' THEN 0.72
                            WHEN 'eletrica'    THEN 0.65
                            WHEN 'fixacao'     THEN 0.65
                            WHEN 'hidraulica'  THEN 0.70
                            WHEN 'pintura'     THEN 0.74
                            ELSE 0.70
                         END, 2),
    unit_of_measure = 'un',
    qty_step = 1,
    -- Código de barras NÃO é atribuído aqui. Ver a seção 7.1 — e o PORQUÊ de
    -- não gerar EAN com prefixo brasileiro (789) está documentado lá.
    ncm = '00000000',
    cfop = '5102',
    origem = 0,
    supplier_id = 'fornecedor-padrao',
    supplier_sku = 'F-' || upper(substr(md5(slug), 1, 8)),
    weight_kg = 1.0
WHERE cost IS NULL;

-- Unidade real por produto. É aqui que a loja deixa de ser marketplace
-- genérico: saco, barra, metro e metro cúbico são o vocabulário do balcão.
UPDATE products SET unit_of_measure = 'sc',  qty_step = 1,    weight_kg = 50.000, length_cm = 60, width_cm = 40, height_cm = 12, ncm = '25232990'
    WHERE slug = 'cimento-votoran-50kg';
UPDATE products SET unit_of_measure = 'sc',  qty_step = 1,    weight_kg = 20.000, ncm = '38245000'
    WHERE slug = 'argamassa-quartzolit-20kg';
UPDATE products SET unit_of_measure = 'cto', qty_step = 1,    weight_kg = 250.000, ncm = '69041000'
    WHERE slug = 'tijolo-ceramico-9-furos';
UPDATE products SET unit_of_measure = 'rl',  qty_step = 1,    weight_kg = 14.000, ncm = '73143100'
    WHERE slug = 'tela-soldada-galvanizada';
UPDATE products SET unit_of_measure = 'rl',  qty_step = 1,    weight_kg = 9.500,  ncm = '85444900'
    WHERE slug = 'cabo-flexivel-2-5mm-100m';
UPDATE products SET unit_of_measure = 'l',   qty_step = 1,    weight_kg = 24.000, ncm = '32091010'
    WHERE slug = 'tinta-suvinil-fosco-18l';
UPDATE products SET unit_of_measure = 'sc',  qty_step = 1,    weight_kg = 25.000, ncm = '32141000'
    WHERE slug = 'massa-corrida-pva-25kg-suvinil';
UPDATE products SET unit_of_measure = 'kg',  qty_step = 0.5,  weight_kg = 1.000,  ncm = '73170090'
    WHERE slug = 'prego-com-cabeca-17x27-1kg';
UPDATE products SET unit_of_measure = 'par', qty_step = 1,    weight_kg = 0.150
    WHERE slug = 'luva-seguranca-vaqueta-m';
UPDATE products SET unit_of_measure = 'un',  qty_step = 1,    weight_kg = 45.000, ncm = '39251000'
    WHERE slug = 'caixa-dagua-500l-fortlev';

-- Peso real das ferramentas (vinha só como texto dentro de `specs`).
UPDATE products SET weight_kg = 1.700 WHERE slug = 'furadeira-bosch-gsb-13-re';
UPDATE products SET weight_kg = 1.000 WHERE slug = 'parafusadeira-makita-df333';
UPDATE products SET weight_kg = 1.900 WHERE slug = 'furadeira-bosch-gsb-16-re';
UPDATE products SET weight_kg = 2.700 WHERE slug = 'martelete-bosch-gbh-2-24';
UPDATE products SET weight_kg = 1.600 WHERE slug = 'esmerilhadeira-bosch-gws-700';
UPDATE products SET weight_kg = 5.800 WHERE slug = 'rompedor-bosch-gsh-5-ce';

-- ---------------------------------------------------------------------------
-- 7) Itens de venda FRACIONADA — a razão de `stock` ter virado NUMERIC
-- ---------------------------------------------------------------------------
-- Sem estes o catálogo inteiro é venda por unidade e a mudança de tipo parece
-- gratuita. Areia sai por m³ com passo de 0,5; cabo e vergalhão por metro.
INSERT INTO products (sku, slug, name, category_id, seller_id, price, cost, icon, brand, stock,
                      unit_of_measure, qty_step, weight_kg, ncm, cfop, origem, rating, review_count,
                      description, specs, status)
VALUES
    ('AREIA-M3', 'areia-media-lavada-m3', 'Areia Média Lavada (m³)', 'construcao', 'material-braz',
     129.00, 98.00, '◫', NULL, 42.500, 'm3', 0.5, 1500.000, '25051000', '5102', 0, 4, 23,
     'Areia média lavada para concreto e assentamento. Venda mínima de 0,5 m³, entrega por caçamba.',
     '{"Tipo":"Média lavada","Aplicação":"Concreto e assentamento"}'::jsonb, 'published'),

    ('BRITA1-M3', 'brita-1-m3', 'Brita nº 1 (m³)', 'construcao', 'material-braz',
     139.00, 106.00, '◫', NULL, 28.000, 'm3', 0.5, 1600.000, '25171000', '5102', 0, 4, 17,
     'Brita nº 1 granítica para concreto estrutural. Venda mínima de 0,5 m³.',
     '{"Tipo":"Granítica nº 1","Aplicação":"Concreto estrutural"}'::jsonb, 'published'),

    ('VERG-10-BR', 'vergalhao-ca50-10mm-barra', 'Vergalhão CA-50 10mm Barra 12m Gerdau', 'construcao', 'material-braz',
     68.90, 56.50, '◫', 'Gerdau', 320.000, 'br', 1, 7.400, '72142000', '5102', 0, 5, 41,
     'Vergalhão nervurado CA-50 bitola 10mm, barra de 12 metros. Aço Gerdau.',
     '{"Bitola":"10 mm","Comprimento":"12 m","Tipo":"CA-50"}'::jsonb, 'published'),

    ('CABO-25-M', 'cabo-flexivel-2-5mm-metro', 'Cabo Flexível 2,5mm² Azul (por metro)', 'eletrica', 'eletrica-costa',
     2.90, 1.95, '⚡', 'Sil', 847.500, 'm', 0.5, 0.024, '85444900', '5102', 0, 5, 96,
     'Cabo flexível 2,5mm² 750V, cortado na medida. Venda a partir de 0,5 m.',
     '{"Seção":"2,5 mm²","Cor":"Azul","Norma":"NBR NM 247-3"}'::jsonb, 'published');

-- ---------------------------------------------------------------------------
-- 7.1) Identificação de balcão — SKU e código de barras
-- ---------------------------------------------------------------------------
-- PORQUÊ aqui e não junto de cada INSERT: tem que rodar DEPOIS da seção 7, se
-- não os itens de venda fracionada ficariam sem código de barras.
--
-- ⚠️ ESPELHO de migrations/balcao_ids.sql. O arquivo separado existe porque o
-- catálogo curado é importado DEPOIS do seed e também precisa ser rotulado.
-- Mantenha os dois idênticos — o teste
-- TestSeed_BlocoDeIdentificacaoDeBalcaoNaoDivergiu falha se divergirem.
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

-- ---------------------------------------------------------------------------
-- 8) Faixas de atacado (migration 006) — o cliente profissional
-- ---------------------------------------------------------------------------
-- Modelo "a partir de N": é como o balcão negocia ("de 10 pra cima sai por X").
INSERT INTO product_price_tiers (product_id, min_qty, price)
SELECT p.id, t.min_qty, t.price
FROM (VALUES
    ('cimento-votoran-50kg',          10.0,  39.90),  -- 10 sacos
    ('cimento-votoran-50kg',          50.0,  36.90),  -- palete
    ('argamassa-quartzolit-20kg',     20.0,  26.50),
    ('tijolo-ceramico-9-furos',        5.0,  82.00),  -- 5 centos
    ('vergalhao-ca50-10mm-barra',     10.0,  64.90),
    ('vergalhao-ca50-10mm-barra',     50.0,  61.50),
    ('cabo-flexivel-2-5mm-metro',    100.0,   2.45),  -- rolo fechado sai mais barato
    ('areia-media-lavada-m3',          5.0, 118.00),
    ('prego-com-cabeca-17x27-1kg',    10.0,   8.90),
    ('bucha-nylon-s8-kit-100',        10.0,  19.90),
    ('fita-veda-rosca-18x50m',        24.0,   6.20),  -- caixa fechada
    ('tomada-tramontina-liz-10a',     20.0,  10.90)
) AS t(slug, min_qty, price)
JOIN products p ON p.slug = t.slug;

-- ---------------------------------------------------------------------------
-- 9) Registry de atributos por categoria (migration 008) + backfill
-- ---------------------------------------------------------------------------
-- Recriado aqui porque o TRUNCATE ... CASCADE acima apaga category_attributes
-- (FK para categories). Sem isto, `make catalog-db-seed` deixaria o catálogo
-- sem facetas técnicas até alguém reaplicar a migration.
INSERT INTO category_attributes (category_id, key, label, data_type, unit, filterable, sort_order, spec_key) VALUES
    ('ferramentas', 'potencia_w',    'Potência',    'number', 'W',   true,  1, 'Potência'),
    ('ferramentas', 'tensao',        'Tensão',      'text',   NULL,  true,  2, 'Tensão'),
    ('ferramentas', 'mandril_mm',    'Mandril',     'number', 'mm',  true,  3, 'Mandril'),
    ('ferramentas', 'peso_kg',       'Peso',        'number', 'kg',  true,  4, 'Peso'),
    ('ferramentas', 'garantia',      'Garantia',    'text',   NULL,  false, 5, 'Garantia'),
    ('eletrica',    'secao_mm2',     'Seção',       'number', 'mm²', true,  1, 'Seção'),
    ('eletrica',    'tensao',        'Tensão',      'text',   NULL,  true,  2, 'Tensão'),
    ('eletrica',    'comprimento_m', 'Comprimento', 'number', 'm',   true,  3, 'Comprimento'),
    ('eletrica',    'cor',           'Cor',         'text',   NULL,  true,  4, 'Cor'),
    ('construcao',  'peso_kg',       'Peso',        'number', 'kg',  true,  1, 'Peso'),
    ('construcao',  'tipo',          'Tipo',        'text',   NULL,  true,  2, 'Tipo'),
    ('construcao',  'bitola_mm',     'Bitola',      'number', 'mm',  true,  3, 'Bitola'),
    ('construcao',  'aplicacao',     'Aplicação',   'text',   NULL,  false, 4, 'Aplicação'),
    ('pintura',     'volume_l',      'Volume',      'number', 'L',   true,  1, 'Volume'),
    ('pintura',     'acabamento',    'Acabamento',  'text',   NULL,  true,  2, 'Acabamento'),
    ('pintura',     'tipo',          'Tipo',        'text',   NULL,  true,  3, 'Tipo'),
    ('hidraulica',  'diametro_mm',   'Diâmetro',    'number', 'mm',  true,  1, 'Diâmetro'),
    ('hidraulica',  'material',      'Material',    'text',   NULL,  true,  2, 'Material'),
    ('fixacao',     'bitola_mm',     'Bitola',      'number', 'mm',  true,  1, 'Bitola'),
    ('fixacao',     'comprimento_mm','Comprimento', 'number', 'mm',  true,  2, 'Comprimento'),
    ('fixacao',     'material',      'Material',    'text',   NULL,  true,  3, 'Material'),
    ('seguranca',   'ca',            'CA',          'text',   NULL,  true,  1, 'CA'),
    ('seguranca',   'classe',        'Classe',      'text',   NULL,  true,  2, 'Classe'),
    ('seguranca',   'material',      'Material',    'text',   NULL,  true,  3, 'Material'),
    ('jardim',      'peso_kg',       'Peso',        'number', 'kg',  true,  1, 'Peso'),
    ('jardim',      'material',      'Material',    'text',   NULL,  true,  2, 'Material')
ON CONFLICT (category_id, key) DO NOTHING;

-- Backfill dos valores tipados a partir de `specs` (mesma lógica da 008).
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

-- O peso já está tipado na coluna `weight_kg`; espelhar no atributo mantém a
-- faceta "peso" funcionando pra quem não tem a grandeza dentro de `specs`.
INSERT INTO product_attributes (product_id, key, value_num)
SELECT p.id, 'peso_kg', p.weight_kg
FROM products p
JOIN category_attributes ca ON ca.category_id = p.category_id AND ca.key = 'peso_kg'
WHERE p.weight_kg IS NOT NULL
ON CONFLICT (product_id, key) DO NOTHING;

-- Preço inicial no histórico: sem linha de partida, a primeira alteração
-- aparece como "veio do nada" na auditoria.
INSERT INTO product_price_history (product_id, price, cost, source)
SELECT id, price, cost, 'seed' FROM products;

COMMIT;

-- Relatório
SELECT 'categories'     AS table_name, count(*) AS rows FROM categories
UNION ALL
SELECT 'sellers',        count(*) FROM sellers
UNION ALL
SELECT 'products',       count(*) FROM products
UNION ALL
SELECT 'product_images', count(*) FROM product_images;
