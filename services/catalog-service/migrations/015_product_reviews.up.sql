-- ============================================================================
-- Avaliações de produto — com conteúdo por trás do número
-- ============================================================================
--
-- O QUE HAVIA ANTES: `products.rating` e `products.review_count` eram dois
-- números soltos, preenchidos pelo seed, sem NENHUMA linha de avaliação por
-- trás. A aba de avaliações dizia "disponível em breve" e `sort=top_rated`
-- ordenava a vitrine por um número inventado. Isto é: a ordenação mais
-- confiável da loja ("mais bem avaliado") era a menos confiável de todas.
--
-- Esta migration cria a tabela que dá lastro ao número e liga o agregado a ela
-- por GATILHO, para que os dois nunca mais possam divergir.
--
-- ----------------------------------------------------------------------------
-- COMPRA VERIFICADA — por que a coluna `order_id` é NOT NULL
-- ----------------------------------------------------------------------------
--
-- Avaliação sem compra é caixa de comentários. O que separa avaliação de spam é
-- custar dinheiro para o autor. Por isso `order_id` é obrigatório: NÃO EXISTE
-- linha nesta tabela sem um pedido associado. A verificação de que aquele
-- pedido é REAL e é DAQUELE usuário é feita na aplicação, em duas provas
-- independentes (ver internal/review/grant.go e docs/reviews-e-recomendacao.md):
--
--   1. um "purchase grant" assinado pelo order-service com o SERVICE_JWT_SECRET,
--      amarrando usuário + produto + pedido; e
--   2. a existência local, NESTE banco, de uma `stock_reservations` com
--      status='committed' para aquele (order_id, product_id).
--
-- As duas são exigidas juntas. O order-service tem o banco de pedidos mas não
-- tem este; o catalog tem a reserva confirmada mas não sabe de quem é o pedido.
-- Cada um sabe metade, e é por isso que as duas metades são pedidas.
--
-- ----------------------------------------------------------------------------
-- UMA AVALIAÇÃO POR PESSOA POR PRODUTO
-- ----------------------------------------------------------------------------
--
-- Garantido por índice ÚNICO, não por checagem na aplicação: comprar duas vezes
-- o mesmo produto (o que é comum em ferragem — parafuso, cimento, fita) não
-- pode virar dois votos. Quem comprou de novo EDITA a avaliação que já tem.
-- ============================================================================

CREATE TABLE IF NOT EXISTS product_reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,

    -- Identidade do autor: o `sub` do JWT do auth-service. Opaco aqui, sem FK
    -- cross-DB (mesma convenção de stock_reservations.order_id).
    author_user_id  TEXT NOT NULL,

    -- Nome de EXIBIÇÃO, já minimizado pela aplicação ("Marlon G."). LGPD:
    -- guardamos o que vai ser exibido, não o cadastro completo do cliente —
    -- o catálogo não é o lugar de replicar dado pessoal do auth-service.
    author_name     TEXT NOT NULL,

    -- Pedido que autorizou a avaliação. NOT NULL por desenho (ver cabeçalho).
    order_id        TEXT NOT NULL,

    rating          SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    title           TEXT CHECK (title IS NULL OR length(title) <= 120),
    body            TEXT CHECK (body  IS NULL OR length(body)  <= 2000),

    -- MODERAÇÃO: 'published' é o default deliberado. Ver a justificativa longa
    -- em docs/reviews-e-recomendacao.md — resumo: a barreira de spam já foi
    -- paga na compra verificada, e uma loja sem equipe de moderação que exige
    -- aprovação manual acaba com uma fila que ninguém lê e zero avaliação no
    -- ar. O que vai para 'pending' é só o que a heurística de
    -- internal/review/moderation.go marca (link, contato, caixa alta, spam
    -- repetitivo) — o subconjunto pequeno que vale o tempo de um humano.
    status          TEXT NOT NULL DEFAULT 'published'
                    CHECK (status IN ('published', 'pending', 'rejected')),
    -- Por que foi parar na fila / por que foi recusada. Aparece para o admin.
    moderation_note TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Uma por pessoa por produto. É o índice que sustenta a credibilidade do
-- agregado; sem ele, uma pessoa determinada vira "muitas avaliações".
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_reviews_one_per_author
    ON product_reviews (product_id, author_user_id);

-- Leitura pública: sempre por produto, sempre só as publicadas, quase sempre
-- da mais recente para a mais antiga. Índice PARCIAL pelo mesmo motivo dos da
-- migration 013 — 'published' é o filtro fixo da rota pública.
CREATE INDEX IF NOT EXISTS idx_product_reviews_public
    ON product_reviews (product_id, created_at DESC)
    WHERE status = 'published';

-- Fila de moderação: poucas linhas, varridas por data. Parcial para não pagar
-- índice sobre o corpo inteiro da tabela por causa de uma tela de admin.
CREATE INDEX IF NOT EXISTS idx_product_reviews_pending
    ON product_reviews (created_at)
    WHERE status = 'pending';

CREATE TRIGGER trg_product_reviews_updated
    BEFORE UPDATE ON product_reviews
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================================
-- ORDENAÇÃO POR RELEVÂNCIA — `rating_bayes`
-- ============================================================================
--
-- Média pura é uma ordenação ruim e todo mundo já viu por quê: um produto com
-- UMA avaliação 5★ passa na frente de um com 4,8★ e 400 avaliações. Quem
-- ordena a vitrine por média pura está ordenando por "quem teve menos
-- avaliações", que é exatamente o contrário do que o usuário quis pedir.
--
-- A correção é uma média BAYESIANA (o mesmo truque do ranking do IMDb):
--
--     score = (v * R + m * C) / (v + m)
--
--   v = review_count do produto
--   R = média do produto
--   m = PRIOR_COUNT — "peso" do palpite inicial, em número de avaliações
--   C = PRIOR_MEAN  — a nota que assumimos antes de saber qualquer coisa
--
-- Com m = 5 e C = 4.0: um produto com uma única 5★ pontua
-- (1*5 + 5*4)/6 = 4,17 — abaixo de um 4,8★ com 400 avaliações (4,79). O item
-- novo não é punido para sempre: bastam ~10 avaliações boas para ele passar.
--
-- POR QUE `C` É UMA CONSTANTE E NÃO A MÉDIA GLOBAL DA LOJA: a média global
-- muda a cada avaliação inserida, e usá-la obrigaria a REESCREVER o score de
-- TODOS os produtos a cada review — um UPDATE de tabela inteira no caminho de
-- escrita do cliente. Constante fixa custa zero e erra pouco; mudá-la é uma
-- migration, que é honesto: é uma decisão de produto, não um efeito colateral.
--
-- NUMERIC(4,3) e não (2,1) como `rating`: o score precisa desempatar. Com uma
-- casa decimal, metade do catálogo empataria em 4,0 e a "ordenação por
-- relevância" viraria ordenação por ordem física no disco.
-- ============================================================================

ALTER TABLE products ADD COLUMN IF NOT EXISTS rating_bayes NUMERIC(4,3) NOT NULL DEFAULT 0;

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_rating_bayes_range;
ALTER TABLE products ADD CONSTRAINT products_rating_bayes_range
    CHECK (rating_bayes >= 0 AND rating_bayes <= 5);

CREATE INDEX IF NOT EXISTS idx_products_published_bayes
    ON products (rating_bayes DESC, review_count DESC)
    WHERE status = 'published';

-- ============================================================================
-- AGREGADO CONSISTENTE — GATILHO, não recálculo em lote
-- ============================================================================
--
-- A escolha entre gatilho e job de recálculo é decidida pelo CONSUMIDOR do
-- agregado, não pelo custo de escrita:
--
--   • `rating`/`review_count`/`rating_bayes` alimentam a ORDENAÇÃO da vitrine
--     (`sort=top_rated`) e a nota do card. Um job de 5 em 5 minutos significa
--     uma janela de até 5 minutos em que "mais bem avaliado" está mentindo, e
--     em que o cliente que acabou de avaliar não vê a própria nota no produto.
--     Defasagem em agregado que ordena não é atraso, é resultado errado.
--
--   • O custo do gatilho é irrelevante AQUI, e isso é particularidade deste
--     agregado: escrita de avaliação é rara (uma por pessoa por produto, e só
--     de quem comprou) e a leitura de produto é o caminho mais quente da loja.
--     Pagar na escrita rara para não pagar na leitura quente é a troca certa.
--     Fosse um contador de visualizações, a resposta seria a oposta.
--
--   • Um gatilho não pode ser esquecido por um caminho de escrita novo. Um job
--     que roda "depois do INSERT" pode — e o modo de falha é silencioso.
--
-- Só linhas 'published' entram na conta: uma avaliação na fila de moderação
-- ainda não é opinião pública, e deixá-la contar permitiria mover a nota do
-- produto com um texto que ninguém aprovou.
-- ============================================================================

CREATE OR REPLACE FUNCTION product_reviews_recalc(p_product_id UUID) RETURNS VOID
LANGUAGE plpgsql
-- SET search_path: mesma lição da migration 014 — sem isto a função quebra o
-- pg_restore, que roda com search_path vazio.
SET search_path = pg_catalog, public
AS $$
DECLARE
    v_count INT;
    v_avg   NUMERIC;
    prior_count CONSTANT NUMERIC := 5;    -- m
    prior_mean  CONSTANT NUMERIC := 4.0;  -- C
BEGIN
    SELECT count(*), coalesce(avg(rating), 0)
      INTO v_count, v_avg
      FROM product_reviews
     WHERE product_id = p_product_id AND status = 'published';

    UPDATE products
       SET rating       = round(v_avg, 1),
           review_count = v_count,
           rating_bayes = CASE
               WHEN v_count = 0 THEN 0
               ELSE round((v_count * v_avg + prior_count * prior_mean)
                          / (v_count + prior_count), 3)
           END
     WHERE id = p_product_id;
END;
$$;

CREATE OR REPLACE FUNCTION product_reviews_sync_aggregate() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    -- UPDATE que move a avaliação de produto (não acontece hoje, mas o gatilho
    -- não pode depender disso) precisa recalcular OS DOIS produtos.
    IF (TG_OP = 'DELETE' OR TG_OP = 'UPDATE') THEN
        PERFORM product_reviews_recalc(OLD.product_id);
    END IF;
    IF (TG_OP = 'INSERT' OR TG_OP = 'UPDATE') THEN
        PERFORM product_reviews_recalc(NEW.product_id);
    END IF;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_product_reviews_aggregate ON product_reviews;
CREATE TRIGGER trg_product_reviews_aggregate
    AFTER INSERT OR UPDATE OR DELETE ON product_reviews
    FOR EACH ROW EXECUTE FUNCTION product_reviews_sync_aggregate();

-- ============================================================================
-- ⚠️ BACKFILL DESTRUTIVO (e o backup que o torna reversível)
-- ============================================================================
--
-- Os `rating`/`review_count` que existem hoje são FICÇÃO do seed: 111 produtos
-- com nota e contagem sem uma única avaliação por trás. Manter esses números
-- depois desta migration seria pior do que antes — agora existe uma tabela de
-- avaliações, e a nota do produto continuaria não vindo dela. O "4,7 (128
-- avaliações)" viraria uma afirmação que o próprio banco desmente.
--
-- Então recalculamos TUDO a partir de `product_reviews` (que está vazia): todo
-- produto passa a 0 avaliações. A loja começa dizendo a verdade — "ainda sem
-- avaliações" — em vez de continuar exibindo número inventado.
--
-- Para a migration continuar REVERSÍVEL, os valores antigos são guardados
-- antes. O `.down.sql` restaura a partir daqui.
CREATE TABLE IF NOT EXISTS products_rating_pre_reviews_backup (
    product_id   UUID PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    rating       NUMERIC(2,1) NOT NULL,
    review_count INT NOT NULL,
    saved_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO products_rating_pre_reviews_backup (product_id, rating, review_count)
SELECT id, rating, review_count FROM products
ON CONFLICT (product_id) DO NOTHING;

UPDATE products p
   SET rating       = coalesce(r.avg_rating, 0),
       review_count = coalesce(r.n, 0),
       rating_bayes = CASE
           WHEN coalesce(r.n, 0) = 0 THEN 0
           ELSE round((r.n * r.avg_rating + 5 * 4.0) / (r.n + 5), 3)
       END
  FROM (
      SELECT p2.id AS product_id,
             count(pr.id)                    AS n,
             round(avg(pr.rating), 1)        AS avg_rating
        FROM products p2
        LEFT JOIN product_reviews pr
               ON pr.product_id = p2.id AND pr.status = 'published'
       GROUP BY p2.id
  ) r
 WHERE r.product_id = p.id;

ANALYZE products;
