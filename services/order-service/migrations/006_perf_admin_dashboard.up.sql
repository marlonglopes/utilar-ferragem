-- ============================================================================
-- Índices do painel administrativo (/api/v1/admin/overview e /sellers/performance)
-- ----------------------------------------------------------------------------
-- PORQUÊ: todas as agregações do painel são ancoradas em `paid_at` (receita é
-- reconhecida quando o dinheiro entra, não quando o pedido é criado), e NÃO
-- HAVIA NENHUM índice em paid_at. Todo cálculo de faturamento fazia seq scan
-- da tabela inteira de pedidos.
--
-- Isso é pior do que parece: a tela de observabilidade chama o painel a cada
-- 30s enquanto o dono a deixa aberta, e o overview é a primeira coisa que
-- carrega em /admin. Um painel que varre 208 mil pedidos a cada refresh compete
-- com a venda pelo pool de conexões (10 por serviço) — o painel derrubaria a
-- loja exatamente no momento em que o dono abre o painel para entender por que
-- a loja está lenta.
--
-- MEDIDO em base de 208.000 pedidos + 624.000 itens, espalhados por 2 anos
-- (método em docs/performance-banco.md; VACUUM ANALYZE antes de cada medição):
--
--   1. Série diária de faturamento (30 dias) — o gráfico da visão geral:
--      ANTES:  Parallel Seq Scan on orders
--              24,6-34,9 ms / 4.245 buffers
--      DEPOIS: Bitmap Index Scan on idx_orders_paid_at
--              4,6 ms /   470 buffers        → ~6x mais rápido, 9x menos I/O
--
--   2. Pedidos travados (status='paid' há mais de 4h) — a pior das cinco:
--      ANTES:  Bitmap Index Scan on idx_orders_status, materializando
--              34.667 linhas em 'paid' só para descartar quase todas no
--              filtro de paid_at e ordenar o que sobrou
--              49,0-58,8 ms / 4.279 buffers
--      DEPOIS: Index Scan on idx_orders_stuck (parcial, já na ordem certa)
--              0,21 ms /   104 buffers       → ~230x mais rápido, 41x menos I/O
--
--   3. KPIs rolantes (hoje/7d/30d + anteriores, uma varredura com FILTER):
--      ANTES:  Parallel Seq Scan   24,4-31,6 ms / 4.245 buffers
--      DEPOIS: idx_orders_paid_at   7,8 ms /   662 buffers  → ~4x
--
--   4. Desempenho por vendedor (30 dias, 1 loja):
--      ANTES:  Bitmap Index Scan on idx_orders_store lendo 10.400 linhas
--              (o índice ordena por created_at, não por paid_at)
--              7,4-7,7 ms / 4.289 buffers
--      DEPOIS: idx_orders_balcao_paid, 284 linhas
--              0,51 ms /   262 buffers       → ~15x mais rápido, 16x menos I/O
--
--   5. Margem (join com order_items, 30 dias, 1 loja):
--      ANTES:  13,7 ms / 5.425 buffers
--      DEPOIS:  4,6 ms / 1.426 buffers       → ~3x
--
-- O ganho de I/O é o número que mais importa aqui, não o de tempo: o plano
-- ANTIGO lê ~4.245 buffers INDEPENDENTEMENTE do recorte pedido, porque varre a
-- tabela inteira. Esse custo cresce com o histórico da loja, não com o tamanho
-- da janela — com 2 milhões de pedidos a mesma consulta lê ~10x mais páginas,
-- enquanto o plano NOVO continua lendo o que a janela realmente contém.
--
-- ⚠️ PRODUÇÃO: CREATE INDEX (não CONCURRENTLY) porque golang-migrate roda a
-- migration dentro de uma transação — trava escrita em orders enquanto
-- constrói. Mesmo procedimento e mesma ressalva da migration 005.
-- ============================================================================

-- 1. Receita por período. Parcial em `paid_at IS NOT NULL`: pedido não pago
--    nunca entra em nenhuma conta de faturamento, e mantê-lo fora deixa o
--    índice do tamanho do que realmente vendeu.
CREATE INDEX IF NOT EXISTS idx_orders_paid_at
  ON orders (paid_at)
  WHERE paid_at IS NOT NULL;

-- 2. Pedidos travados. Índice parcial estreito: só o que está em 'paid'.
--    A lista de travados é minúscula em operação saudável (zero linhas), e é
--    justamente por isso que ela não pode custar uma varredura para provar que
--    está vazia — o painel checa isso o tempo todo.
CREATE INDEX IF NOT EXISTS idx_orders_stuck
  ON orders (paid_at)
  WHERE status = 'paid' AND paid_at IS NOT NULL;

-- 3. Desempenho de vendedores. A consulta agrupa por (store_id, operator_id)
--    dentro de uma janela de paid_at, só no canal balcão.
--
--    A ordem das colunas segue o formato do filtro: store_id é igualdade
--    (filtro opcional por loja), paid_at é intervalo. Coluna de igualdade
--    ANTES da de intervalo — invertido, o Postgres só consegue usar a primeira
--    coluna e volta a filtrar o resto na heap.
--
--    idx_orders_store (store_id, created_at DESC) NÃO serve: ordena por
--    created_at, e a janela do painel é sobre paid_at.
CREATE INDEX IF NOT EXISTS idx_orders_balcao_paid
  ON orders (store_id, paid_at, operator_id)
  WHERE channel = 'balcao' AND paid_at IS NOT NULL;

-- 4. Funil e byStatus são ancorados em created_at e já são atendidos por
--    idx_orders_created_at (migration 001). Nenhum índice novo aqui de
--    propósito: índice que não muda plano só custa escrita.
