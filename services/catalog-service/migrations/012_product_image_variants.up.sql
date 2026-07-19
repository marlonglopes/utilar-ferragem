-- Upload e processamento de imagem de produto.
--
-- Até aqui `product_images` era só (url, alt, sort_order): imagem de terceiro,
-- apontada por link. Agora convivem DOIS tipos de imagem na mesma tabela:
--
--   1. EXTERNA — as 288 fotos CC0 do Wikimedia já cadastradas. `url` é uma URL
--      absoluta de terceiro, `storage_key` é NULL, `variants` fica '{}'.
--   2. PRÓPRIA — subida pelo admin e normalizada pelo backend. `storage_key`
--      tem a chave lógica do objeto e `variants` tem as três resoluções.
--
-- PORQUÊ não uma tabela nova: é a MESMA galeria, na mesma ordem, no mesmo
-- carrossel. Separar obrigaria todo leitor (loadImages, loadThumbnails, o
-- commit da ingestão) a fazer UNION e reordenar em memória — e a capa passaria
-- a depender de duas fontes de sort_order.
--
-- PORQUÊ as variantes são JSONB numa coluna, e não linhas com `variant`:
-- thumb/medium/large são A MESMA FOTO em três tamanhos, não três fotos. Com
-- linhas separadas, o carrossel precisaria agrupar por algum id lógico pra não
-- exibir a mesma peça três vezes, e `sort_order` teria que ser replicado
-- idêntico em cada linha (com o risco óbvio de divergir e a capa virar a
-- miniatura de outra foto). Uma linha por foto mantém capa e reordenação
-- funcionando exatamente como já funcionam.
ALTER TABLE product_images
    ADD COLUMN storage_key       TEXT,
    ADD COLUMN content_type      TEXT,
    ADD COLUMN width             INT,
    ADD COLUMN height            INT,
    ADD COLUMN bytes             BIGINT,
    ADD COLUMN original_bytes    BIGINT,
    ADD COLUMN original_filename TEXT,
    ADD COLUMN checksum          TEXT,
    ADD COLUMN variants          JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN product_images.storage_key IS
    'Chave LÓGICA do objeto (produtos/<id>/<hash>-<variante>.jpg). NUNCA caminho de disco nem URL absoluta: a URL pública é derivada em tempo de resposta a partir de MEDIA_BASE_URL, para que migrar disco->S3/CDN não exija reescrever a tabela. NULL = imagem externa por URL.';
COMMENT ON COLUMN product_images.variants IS
    'Mapa variante -> chave lógica, ex.: {"thumb":"produtos/x/ab-thumb.jpg",...}. Vazio = imagem externa sem variantes.';
COMMENT ON COLUMN product_images.checksum IS
    'SHA-256 do arquivo ORIGINAL enviado. Deduplica e torna a reprocessagem idempotente.';
COMMENT ON COLUMN product_images.original_bytes IS
    'Peso do arquivo enviado, antes da normalizacao. Com `bytes` da o ganho real de cada upload.';

-- Dedupe: a mesma foto subida duas vezes no mesmo produto é uma linha só.
--
-- ⚠️ Índice PARCIAL (`WHERE checksum IS NOT NULL`) porque as 288 linhas
-- legadas têm checksum NULL — sem o filtro, o índice único trataria... nada,
-- já que NULL nunca colide; mas o parcial deixa a intenção explícita e mantém
-- o índice pequeno. Quem for usar ON CONFLICT com ele PRECISA repetir o
-- predicado: `ON CONFLICT (product_id, checksum) WHERE checksum IS NOT NULL`.
CREATE UNIQUE INDEX idx_product_images_dedupe
    ON product_images (product_id, checksum)
    WHERE checksum IS NOT NULL;

-- Busca por conteúdo entre produtos: encontra a mesma foto reaproveitada e é
-- o que permitirá reprocessar em lote sem redecidir a chave.
CREATE INDEX idx_product_images_checksum
    ON product_images (checksum)
    WHERE checksum IS NOT NULL;
