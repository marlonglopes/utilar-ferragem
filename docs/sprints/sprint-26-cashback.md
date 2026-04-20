# Sprint 26 — Programa de cashback

**Fase**: 5 — Crescimento. **Estimativa**: 8–10 dias.

## Escopo

Entregar o mecanismo de cashback conforme projetado em [ADR 011](../adr/011-cashback-mechanism.md). `cashback_percent` por produto definido pelos vendedores (0–10%), ganho em `paid`, creditado em `delivered`, expira em 12 meses, resgatável no checkout em qualquer valor inteiro de real entre R$ 5 e `min(saldo_disponível, 50% * subtotal_do_pedido)`. O ledger fica como módulo dentro do user-service. A UI do cliente aparece em cards de produto, PDP, carrinho, checkout, confirmação e na nova página `/conta/cashback`; a UI do vendedor ganha o campo `cashback_percent` em Adicionar/Editar produto; a UI do admin ganha uma ferramenta de ajuste de saldo.

Não existem cupons ainda, então o empilhamento de resgates não é um problema no MVP. Contas Pro/B2B são excluídas — têm precificação separada de qualquer forma.

## Tarefas

### product-service
1. Migration `services/product-service/db/migrate/YYYYMMDD_add_cashback_percent_to_products.rb`: `add_column :products, :cashback_percent, :decimal, precision: 4, scale: 2, default: 0.00, null: false` + CHECK `cashback_percent >= 0 AND cashback_percent <= 10`
2. Validação no model `services/product-service/app/models/product.rb`: `validates :cashback_percent, numericality: { greater_than_or_equal_to: 0, less_than_or_equal_to: 10 }`
3. Incluir `cashback_percent` em `services/product-service/app/controllers/concerns/product_serializable.rb`
4. Permitir `cashback_percent` nos strong params de `services/product-service/app/controllers/api/v1/products_controller.rb`

### user-service (novo módulo de cashback)
5. Migration `services/user-service/db/migrate/YYYYMMDD_create_cashback_ledger.rb` — tabela completa conforme [07-data-model.md §N](../07-data-model.md#n-cashback): colunas `id, user_id, order_id, amount_cents, currency, kind, status, earned_at, available_at, expires_at, metadata_json, created_at`; índices: unique parcial `(user_id, order_id)` onde `kind='earned'`, btree `(user_id, status)`, btree `(expires_at)` onde `status='available'`, btree `(order_id)`
6. Model `services/user-service/app/models/cashback_ledger_entry.rb` com enums para `kind` e `status`; scopes `.pending`, `.available`, `.expired`, `.for_user(id)`
7. Service objects em `services/user-service/app/services/cashback/`:
   - `earn.rb` — input `{ order_id, user_id, line_items: [{product_id, qty, unit_price_cents, cashback_percent_snapshot}] }` → insere linhas `pending`
   - `credit.rb` — input `{ order_id }` → `pending → available`, define `available_at=now, expires_at=now+12mo`
   - `redeem.rb` — input `{ user_id, order_id, amount_cents }` → pg_advisory_xact_lock no `user_id`, verifica saldo + regras, insere linha `kind=redeemed, status=used`, retorna `redemption_id`
   - `expire.rb` — encontra linhas `available` com `expires_at < now`, transita para `expired`, insere linha de auditoria espelhada com `kind=expired`
   - `reverse.rb` — input `{ order_id, scope: :full | :partial, refunded_line_items: [...] }` → `pending` → `reversed`, proporcional se parcial
   - `adjust.rb` — input `{ user_id, amount_cents, direction, reason, actor_id }` → ajuste manual com log de auditoria
8. Consumer Kafka `services/user-service/app/consumers/cashback_consumer.rb` — assina `cashback.earned`, `cashback.credited`, `cashback.reversed`, `cashback.redeemed`, `cashback.expired`; idempotente em `(order_id, kind)`
9. Endpoints em `services/user-service/app/controllers/api/v1/cashback_controller.rb`:
   - `GET /api/v1/me/cashback` → `{ available_balance_cents, pending_balance_cents, next_expiration: {amount_cents, expires_at} | null }`
   - `GET /api/v1/me/cashback/history?page=N&per=20` → linhas do ledger paginadas com kind/status/amount/order_id
   - `POST /api/v1/me/cashback/redeem` → `{ redemption_id }`
10. Endpoint de admin em `services/user-service/app/controllers/api/v1/admin/cashback_controller.rb`:
    - `POST /api/v1/admin/cashback/adjust` — JWT admin, body `{ user_id, amount_cents, reason, direction }`
11. Worker Sidekiq `services/user-service/app/workers/cashback_expiry_worker.rb` agendado para 02:00 BRT todo dia; em lotes (1000 linhas/passagem); emite `cashback.expired`

### order-service
12. Migration `services/order-service/db/migrate/YYYYMMDD_add_cashback_snapshot_to_order_items.rb`: `add_column :order_items, :cashback_percent_snapshot, :decimal, precision: 4, scale: 2, default: 0.00, null: false`
13. Na criação do pedido em `services/order-service/app/services/orders/create.rb`: copiar `product.cashback_percent` para cada `order_items.cashback_percent_snapshot` no momento da compra
14. Hook de produtor em `services/order-service/app/services/orders/transition_status.rb`:
    - `pending_payment → paid` → publicar `cashback.earned { order_id, user_id, line_items }`
    - `shipped → delivered` → publicar `cashback.credited { order_id }`
    - `* → cancelled` ou `* → refunded` (pré-entrega) → publicar `cashback.reversed { order_id, scope: :full }`
    - reembolso parcial pós-entrega → `cashback.reversed { order_id, scope: :partial, refunded_line_items }`

### payment-service
15. Aceitar `redemption_id` opcional no body de `POST /api/v1/payments`
16. Na criação da cobrança, chamar user-service `GET /api/v1/me/cashback/redemptions/:id` (rota apenas interna com token de serviço) para verificar + obter `amount_cents`
17. Reduzir o valor cobrado pelo resgate; armazenar `cashback_redemption_id` + `cashback_discount_cents` na linha de `payments` (novas colunas, migration aditiva)
18. Em caso de falha/estorno do pagamento, chamar user-service `POST /api/v1/internal/cashback/release` para restaurar o saldo

### gateway
19. Rotear `/api/v1/me/cashback` + `/api/v1/me/cashback/*` → user-service (com gate JWT)
20. Rotear `/api/v1/admin/cashback/*` → user-service (JWT + verificação de papel admin)

### SPA (utilar-ferragem)
21. `src/lib/cashbackApi.ts` — hooks `useMyCashback()`, `useCashbackHistory(page)`, `useRedeemCashback()`
22. Componente `CashbackBadge.tsx` — pílula "Ganhe R$ X"; renderiza em `ProductCard` e no hero do PDP quando `cashback_percent > 0`
23. Atualizar `ProductCard.tsx` e `ProductDetailPage.tsx` para renderizar `CashbackBadge`
24. Atualizar `CartPage.tsx` — linha de resumo "Você vai ganhar R$ X em cashback" calculado a partir dos itens do carrinho × seus `cashback_percent`
25. Atualizar `CheckoutPage.tsx` passo 3 (pagamento) — novo campo "Usar meu cashback" com slider/input:
    - Exibe saldo disponível
    - Limita a `min(saldo_disponível, 50% * subtotal)`, mínimo R$ 5,00, granularidade de 100 centavos
    - Chama `POST /me/cashback/redeem` ao aplicar → armazena `redemption_id` no estado do checkout
    - No envio do pagamento, inclui `redemption_id` em `POST /payments`
26. Atualizar `OrderConfirmationPage.tsx` — exibir valor ganho como "Você ganhará R$ X em cashback quando o pedido for entregue"
27. Nova página `src/pages/AccountCashbackPage.tsx` em `/conta/cashback`:
    - Card de saldo: disponível R$ X, pendente R$ Y
    - Card de próxima expiração: "R$ Z expira em DD/MM/AAAA" (ou "Nenhum cashback prestes a expirar")
    - Tabela de histórico com filtros (`Todos`, `Ganhos`, `Usados`, `Expirados`, `Estornados`), paginada, colunas: data, tipo, valor, pedido, status
    - Estado vazio com CTA para `/categoria`
28. Entrada no router `src/router/index.tsx` para `/conta/cashback` (autenticação obrigatória)
29. `Topbar.tsx` — ícone de carteira com chip de saldo quando autenticado; clique → `/conta/cashback`

### i18n (ambas as localidades)
30. Adicionar namespace `cashback` em `src/i18n/pt-BR/cashback.json` e `src/i18n/en/cashback.json`. As chaves incluem:
    - `cashback.earn.badge` — "Ganhe R$ {{amount}}" / "Earn R$ {{amount}}"
    - `cashback.pending.label` — "Pendente" / "Pending"
    - `cashback.available.label` — "Disponível" / "Available"
    - `cashback.used.label` — "Usado" / "Used"
    - `cashback.expired.label` — "Expirado" / "Expired"
    - `cashback.reversed.label` — "Estornado" / "Reversed"
    - `cashback.redeem.title` — "Usar meu cashback" / "Use my cashback"
    - `cashback.redeem.min` — "Mínimo R$ 5,00" / "Minimum R$ 5.00"
    - `cashback.redeem.maxHalf` — "Até 50% do subtotal" / "Up to 50% of subtotal"
    - `cashback.earn.pendingDelivery` — "Creditado após entrega" / "Credited after delivery"
    - `cashback.history.title` — "Histórico de cashback" / "Cashback history"
    - `cashback.balance.available` / `cashback.balance.pending` / `cashback.nextExpiration` / `cashback.empty.cta` etc.

### gifthy-hub (admin + seller)
31. Seller: adicionar input `cashback_percent` (0–10, decimal) nas seções de precificação de `AddProductPage` e `EditProductPage`; tooltip explica "É debitado do repasse, não da margem"
32. Admin: nova página `src/pages/admin/AdminCashbackPage.tsx` em `/admin/utilar/cashback`:
    - Busca por e-mail ou ID do usuário
    - Exibe saldo atual (disponível + pendente)
    - Exibe últimas 20 linhas do ledger
    - Formulário de ajuste: direção (crédito/débito), valor em R$, motivo (textarea obrigatório) → `POST /api/v1/admin/cashback/adjust`
    - Cada ajuste emite uma linha no log de auditoria

## Critérios de aceite

- [ ] Produto com `cashback_percent=5` em um pedido de R$ 200 cria uma linha `pending` no ledger de R$ 10 quando `order.status=paid`
- [ ] A mesma linha transita para `available` com `expires_at = now + 12 meses` quando `order.status=delivered`
- [ ] Resgate de R$ 20 em um pedido com subtotal de R$ 100 é aplicado ao payment-service, a cobrança fica em R$ 80, e o saldo disponível do cliente cai R$ 20
- [ ] Falha no pagamento após resgate libera o resgate — saldo restaurado, linha de auditoria escrita no ledger
- [ ] Pedido cancelado antes da entrega estorna todas as linhas `pending` daquele pedido (status → `reversed`)
- [ ] Reembolso parcial (1 de 3 linhas) pós-entrega estorna o cashback proporcional apenas da linha reembolsada
- [ ] O job noturno de expiração transita uma linha `available` com data fixada em 12 meses atrás para `expired` e emite `cashback.expired`
- [ ] Admin consegue creditar ou debitar o saldo de um cliente com um motivo; a linha de ajuste aparece no histórico do cliente com `kind=adjusted` e o motivo em `metadata_json`
- [ ] Saldo é consistente no chip da Topbar, CartPage, CheckoutPage e `/conta/cashback` — todos lendo da mesma chamada `GET /me/cashback` com cache de query de 30s
- [ ] Todas as chaves i18n `cashback.*` resolvem em `pt-BR` e `en`; nenhuma string hardcoded no JSX
- [ ] Alteração de `cashback_percent` pelo vendedor em um produto **não** altera o cashback de pedidos já realizados (snapshot preservado em `order_items.cashback_percent_snapshot`)
- [ ] Eventos Kafka relacionados a cashback aparecem no console do Redpanda com os payloads esperados
- [ ] Latência p95 para `GET /api/v1/me/cashback` < 150ms; para `POST /me/cashback/redeem` < 200ms

## Dependências

- Sprint 08 (checkout + payment-service) — o resgate se integra com `POST /api/v1/payments`
- Sprint 09 (pedidos) — as transições de status `paid` e `delivered` devem estar ativas e publicando no Kafka
- Sprint 22 (observabilidade) — os eventos Kafka de cashback devem ser observáveis; dashboards rastreiam taxa de resgate, volume de expiração e taxa de estorno
- `users.cpf` (Sprint 07) — a ferramenta de ajuste do admin também busca por CPF
- [ADR 010](../adr/010-notification-architecture.md) — e-mail de aviso de expiração usa a mini-lib `notification-core`

## Riscos

- **Condições de corrida no ledger** — duas chamadas simultâneas de resgate para o mesmo usuário podem causar duplo gasto. Mitigação: `pg_advisory_xact_lock(user_id)` dentro da transação `Cashback::Redeem`.
- **Replay de eventos / entrega duplicada** — consumers Kafka podem reprocessar. Mitigação: cada inserção tem chave em `(order_id, kind)` com índice unique parcial que rejeita duplicatas; o consumer captura `PG::UniqueViolation` e trata como já processado.
- **Vendedor altera `cashback_percent` durante a compra** — o snapshot na criação do item de pedido (`order_items.cashback_percent_snapshot`) garante que o comprador recebe o que foi anunciado quando adicionou ao carrinho. Documentar no texto de ajuda do vendedor.
- **Cliente espera cashback em pedido cancelado** — não verá nenhum crédito; o texto da UI na OrderConfirmationPage deve exibir "Creditado após entrega" (não "Você ganhou") para gerenciar expectativas.
- **Deriva na reconciliação de repasses** — a plataforma deve aos vendedores o valor líquido menos o passivo de cashback; se o ledger e os cálculos de repasse divergirem, haverá super ou sub-pagamento. Mitigação: relatório de reconciliação diário (o sprint de liquidação da Fase 6 fica responsável pelo cron).
- **Float de 12 meses crescendo sem limite** — o volume de lançamento é controlado; rastrear o saldo total `available` como gauge do Prometheus para ver a tendência do passivo antes que se torne um problema.
- **Fraude em anel** — vendedor comprando de si mesmo para gerar cashback. Fora do escopo no MVP; o KYC de vendedores do Sprint 15 + monitoramento de anomalias da Fase 6 cobrem isso.
- **Exclusão LGPD** — o saldo do usuário excluído deve ser zerado corretamente. Coberto pelo §6 do ADR 011; testado no sprint de LGPD (Sprint 24).
