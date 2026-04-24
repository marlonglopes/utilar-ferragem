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

COMMIT;

-- Relatório
SELECT 'categories'     AS table_name, count(*) AS rows FROM categories
UNION ALL
SELECT 'sellers',        count(*) FROM sellers
UNION ALL
SELECT 'products',       count(*) FROM products
UNION ALL
SELECT 'product_images', count(*) FROM product_images;
