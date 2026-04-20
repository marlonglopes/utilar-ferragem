# Sprint 12 — Avaliações e notas

**Fase**: 5 — Crescimento. **Estimativa**: 7–9 dias.

## Escopo

Prova social é um fator determinante de conversão em marketplaces de ferragens — uma furadeira DeWalt com 120 avaliações vende mais do que uma idêntica sem nenhuma. Este sprint introduz avaliações de compradores verificados: nota (1–5) + corpo opcional + fotos opcionais, restrita a clientes com um pedido `delivered` para o produto. Vendedores podem publicar uma resposta pública por avaliação. O admin pode ocultar conteúdo abusivo a partir da fila de moderação no gifthy-hub.

Questão em aberto: separar em um `reviews-service` dedicado ou manter no product-service. Decisão padrão para este sprint: **manter no product-service**; extrair somente se o volume de avaliações crescer de forma significativa. Decidir no kickoff.

## Tarefas

### product-service
1. Migration `services/product-service/db/migrate/YYYYMMDD_create_reviews.rb`: tabela `reviews` (`id, product_id, customer_id, order_id, rating ∈ 1..5, body, photos_json, seller_reply, seller_replied_at, status ∈ pending|published|hidden, created_at, updated_at`); índice único em `(product_id, customer_id)` para garantir 1 avaliação por cliente por produto
2. Model `services/product-service/app/models/review.rb` com `belongs_to :product`, `validates :rating, inclusion: { in: 1..5 }`, escopo de rate limit no create
3. Filtro de palavrões `services/product-service/app/lib/profanity_filter.rb` — lista de palavras em BR-PT em `services/product-service/config/profanity_pt_br.yml`; avaliações com ocorrências vão para `status: pending` para revisão pelo admin
4. Endpoint `POST /api/v1/reviews` em `services/product-service/app/controllers/api/v1/reviews_controller.rb` — JWT obrigatório; valida se o solicitante tem um pedido `delivered` para este produto (chamada cross-service ao order-service `GET /api/v1/orders?product_id=X&status=delivered&customer_id=me`)
5. Endpoint `GET /api/v1/products/:id/reviews` — público, paginado, suporta `?sort=newest|helpful` e `?rating=5`
6. Endpoint `POST /api/v1/reviews/:id/reply` — JWT do vendedor; o vendedor deve ser dono do produto; uma única resposta por avaliação
7. Endpoint `PATCH /api/v1/reviews/:id/status` — JWT do admin; alterna entre `hidden` / `published`
8. Incluir `rating_avg` + `rating_count` no serializer de produto (`services/product-service/app/controllers/concerns/product_serializable.rb`); calcular via contador materializado (colunas `products.rating_avg` e `products.rating_count` atualizadas no insert/update de avaliações)
9. Reutilizar o caminho de upload de imagens existente para `photos_json` — limitar em 4 fotos por avaliação

### order-service
10. Expor um endpoint auxiliar mínimo `GET /api/v1/orders/verify-purchase?product_id=X` (JWT) retornando `{ verified: true|false, order_id }` para que o product-service possa verificar sem duplicar o estado de pedidos

### Gateway
11. Rotear `/api/v1/reviews*` para o product-service (nova família de rotas)

### SPA (utilar-ferragem)
12. Atualizar `ProductDetailPage`: nova seção "Avaliações" com média de estrelas, contagem, gráfico de barras de distribuição (5→1 estrelas), pills de filtro (`Todas`, `5`, `4`, `3`, `2`, `1`), dropdown de ordenação (`Mais recentes`, `Mais úteis`)
13. Componente `ReviewCard.tsx` — estrelas, primeiro nome + inicial do sobrenome do autor, data, corpo, miniaturas de fotos (lightbox ao clicar), bloco de resposta do vendedor abaixo quando presente
14. Página `/conta/pedidos/:id/avaliar` → `WriteReviewPage.tsx` — acessível apenas a partir da página de detalhe de um pedido `delivered`; seletor de estrelas, textarea, upload de fotos (máx. 4); envia para `POST /reviews`
15. Adicionar CTA "Avaliar" em `/conta/pedidos/:id` para cada item quando o status do pedido for `delivered` e não houver avaliação ainda

### gifthy-hub
16. Vendedor: nova aba "Avaliações" na página de detalhe do produto do vendedor; exibe as avaliações; modal "Responder" que posta a resposta
17. Admin: nova página `/admin/reviews` — fila de moderação filtrada por `status=pending`; ações "Publicar" / "Ocultar" chamam `PATCH /reviews/:id/status`

## Critérios de aceite
- [ ] Apenas compradores verificados (com pedido `delivered` para aquele produto) podem enviar uma avaliação — não compradores veem "Compre para avaliar"
- [ ] Média e contagem de notas aparecem na PDP, atualizadas em até 1 segundo após a publicação da avaliação
- [ ] Filtro por nota e ordenação por mais recentes/mais úteis funcionam corretamente
- [ ] A resposta do vendedor aparece inline abaixo da avaliação e está limitada a uma por avaliação
- [ ] O admin pode ocultar uma avaliação abusiva; avaliações ocultas desaparecem da PDP pública imediatamente
- [ ] Uma avaliação contendo palavra da lista de palavrões em pt-BR vai automaticamente para `pending`
- [ ] Upload de fotos chegam ao S3 e são servidas pelo mesmo caminho CDN das imagens de produto

## Dependências
- Sprint 09 (pedidos) concluído — o status `delivered` deve existir
- Pipeline de upload de imagens existente (S3 ou LocalStack)
- Decisão no kickoff: product-service vs. novo reviews-service

## Riscos
- Avaliações falsas / incentivadas — a restrição a comprador verificado é a principal mitigação; adicionar rate limit de avaliações por cliente (≤ 10/dia)
- Abuso de fotos — moderação manual para a Fase 5; reavaliar com moderação automática (Rekognition) se o volume justificar
- Latência da chamada cross-service de verificação — cachear o resultado de `verify-purchase` por par cliente-produto por 5 minutos
