# Sprint 19 — Recomendações (baseadas em regras)

**Fase**: 5 — Crescimento. **Estimativa**: 4–6 dias.

## Escopo

Entregar recomendações sem ML. Quatro slots cobrem 80% do ganho: **"Comprado junto"** (co-compra da mesma ordem), **"Visto recentemente"** (client-side via localStorage), **"Da mesma categoria"** (filtro simples), **"Em alta"** (mais visualizados nos últimos 7 dias, janela deslizante).

O rastreamento de eventos é a base: uma nova tabela `product_events` captura `product.viewed`, `product.added_to_cart`, `order.created` (este último duplica dados do order-service intencionalmente, para desacoplar o pipeline de analytics). O módulo de eventos fica dentro do order-service (não é um novo serviço — evitar expansão de escopo; reavaliar na Fase 6 se o volume exigir uma separação). Um job noturno atualiza a materialized view de co-compras.

## Tarefas

### order-service — módulo de eventos
1. Migration `create_product_events`: `id, product_id, user_id (nullable), session_id, event_type (enum: viewed|added_to_cart|order_created), metadata (jsonb), occurred_at`. Índices em `(product_id, event_type, occurred_at)` e `(user_id, occurred_at)`.
2. `POST /api/v1/events` (auth opcional — aceita `session_id` anônimo): aceita lotes de até 50 eventos por requisição; valida o enum `event_type`; escreve de forma assíncrona via background job (Sidekiq ou thread pool simples, já que o volume é baixo)
3. Rate limit de 60 req/min por IP no gateway

### order-service — endpoints de recomendação
4. Migration para a materialized view `co_purchase_pairs`: `product_id, co_product_id, pair_count`, atualizada toda noite via `rake recommendations:refresh_co_purchase` — faz join entre `orders` + `order_items` para contar quantas vezes dois produtos aparecem no mesmo pedido
5. `GET /api/v1/products/:id/recommendations?slot=co-purchase&limit=6` — lê de `co_purchase_pairs`, filtra por `status='active'`, retorna os top N
6. `GET /api/v1/products/:id/recommendations?slot=category&limit=6` — simples `WHERE category = ? AND id != ? AND status='active' ORDER BY views_30d DESC`
7. `GET /api/v1/products/recommendations?slot=trending&limit=12` — agrega `product_events` WHERE `event_type='viewed' AND occurred_at > NOW() - INTERVAL '7 days'` GROUP BY product_id ORDER BY COUNT DESC
8. Fallback para cold start: se `co-purchase` retornar < 3 resultados, complementa silenciosamente com `category`
9. Cache de respostas por 10 minutos no Redis (`recommendations:{product_id}:{slot}`) — invalidado na atualização noturna
10. Cron noturno em `infrastructure/prod/cron.yml` executa `rake recommendations:refresh_co_purchase` às 03:00 BRT; registra duração da execução + número de linhas atualizadas

### SPA — emissão de eventos
11. `utilar-web/src/lib/analytics.ts` — `trackEvent(type, productId, metadata)` — acumula eventos em lote, envia ao atingir 10 eventos OU 5s, usa `navigator.sendBeacon` ao fechar a página
12. Integrar com: mount do PDP (`product.viewed`), botão adicionar ao carrinho (`product.added_to_cart`), sucesso no checkout (`order.created`, espelhado — o backend tem sua própria fonte de verdade)
13. `utilar-web/src/lib/recentlyViewed.ts` — adiciona o id do produto a `localStorage.utilar.recentlyViewed` (máx. 20, sem duplicatas, FIFO)

### SPA — componentes de recomendação
14. Componente `RelatedProducts` no PDP (`utilar-web/src/pages/ProductDetailPage.tsx`) — renderiza duas fileiras: "Comprado junto" + "Da mesma categoria"
15. `TrendingRow` na home (`utilar-web/src/pages/HomePage.tsx`) — "Em alta"
16. `RecentlyViewedRow` na home — lê o localStorage; se autenticado, pode sincronizar com o servidor opcionalmente (Fase 6)
17. Skeleton loaders por fileira; oculta a fileira inteira se o endpoint retornar < 3 itens (evita fileiras vazias)

## Critérios de aceite

- [ ] A home exibe 4 fileiras de recomendações (ou as oculta graciosamente quando vazias)
- [ ] O PDP exibe as fileiras "Comprado junto" + "Da mesma categoria" com ≥ 3 produtos cada
- [ ] "Visto recentemente" persiste entre sessões do navegador (localStorage)
- [ ] O `rake recommendations:refresh_co_purchase` noturno registra duração e quantidade de linhas atualizadas
- [ ] O endpoint `/api/v1/events` aceita lotes de até 50; rate limit retorna 429 acima de 60/min
- [ ] Cold start (tabela `product_events` vazia) faz fallback silencioso para recomendações por categoria
- [ ] Cache hit do Redis é registrado em log; invalidado após a atualização noturna

## Dependências

- Sprint 04 (PDP) concluído — ponto de montagem para `RelatedProducts`
- Sprint 09 (pedidos) concluído — fonte para os pares de co-compra
- Redis disponível (já em produção)

## Riscos

- Cold start sem eventos — o fallback por categoria é a rede de segurança, mas uma instalação nova ficará com pouca variedade nos primeiros 7–14 dias. Comunique as expectativas no briefing de lançamento.
- Volume de eventos em escala — se o QPS ultrapassar o que um único processo Rails consegue absorver, mova as escritas para Kafka (tópico `product.events`) e consuma para o banco em lotes. Não é necessário no dia 1.
- Crescimento da tabela `product_events` — adicione particionamento mensal ou um job de arquivamento na Fase 6; por enquanto um `VACUUM` mensal é suficiente.
