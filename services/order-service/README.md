# order-service

Serviço de pedidos do marketplace Utilar Ferragem — cria, lista, consulta e cancela pedidos. Requer identificação via header `X-User-Id` (temporário até o `auth-service` da Phase B3 entrar em produção com JWT).

| | |
|---|---|
| **Stack** | Go 1.26 + Gin 1.12 + Postgres 17 |
| **Porta** | `:8092` |
| **DB** | `utilar_order_db` (Postgres em `localhost:5437`) |
| **Status** | Phase B2 ✅ em dev, plugado no frontend via `VITE_ORDER_URL` |

Documentação transversal:
- [README raiz](../../README.md)
- [Database maintenance](../../docs/maintenance/database.md)
- [catalog-service](../catalog-service/README.md) — produtos referenciados como `productId`

---

## Estrutura

```
order-service/
  cmd/server/main.go           ← bootstrap
  internal/
    config/                    ← PORT, ORDER_DB_URL
    db/                        ← golang-migrate no startup
    handler/                   ← order + middleware (RequestID/AccessLog/CORS/RequireUser) + errors
    model/                     ← Order, OrderItem, OrderAddress, TrackingEvent
  migrations/
    001_create_orders.up.sql   ← 4 tabelas + 2 ENUMs + triggers
    001_create_orders.down.sql
    seed.sql                   ← 60 pedidos de teste (20 usuários × 3 pedidos em vários estados)
  Makefile                     ← run / build / test / tidy
```

---

## Modelo de dados

Quatro tabelas, todas criadas em uma única migration (001).

| Tabela | Papel | Chave externa |
|---|---|---|
| `orders` | cabeçalho do pedido (número, status, user_id, totais, datas de cada transição) | — |
| `order_items` | snapshot de produto no momento da compra (name, icon, seller_name preservados mesmo se catalog mudar) | `order_id FK → orders` |
| `shipping_addresses` | endereço em tabela 1-1 (permite histórico quando user editar endereço depois) | `order_id FK → orders UNIQUE` |
| `tracking_events` | linha do tempo de transições (pending_payment → paid → picking → shipped → delivered) | `order_id FK → orders` |

**ENUMs:**
- `order_status`: `pending_payment`, `paid`, `picking`, `shipped`, `delivered`, `cancelled`
- `payment_method`: `pix`, `boleto`, `card`

**Decisões:**
- **`user_id` é TEXT** (não UUID) — opaco, sem FK cross-DB, aceita IDs de dev/mock (ex: `mock-1`, `user-001`). Quando o `auth-service` entrar em produção, continuará emitindo strings — só que virarão UUIDs.
- **`product_id` é UUID** — referência opaca ao `catalog-service`. Sem FK — integridade verificada via HTTP quando o pedido é criado.
- **`payment_id` é UUID opcional** — set pelo `payment-service` via webhook após confirmação (futuro).
- **Totais calculados no servidor** — `Create` recalcula `subtotal + shippingCost = total`, ignora o que o cliente enviar.
- **Cancelamento é idempotente-ish** — status final vira `cancelled` via transação (UPDATE orders + INSERT tracking_events). Tentar cancelar um pedido `shipped`/`delivered`/`cancelled` → 409 Conflict.

---

## API

Base URL em dev: `http://localhost:8092`. Todos os endpoints `api/v1` requerem `X-User-Id` (401 se faltar).

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/health` | liveness probe (verifica DB) |
| `POST` | `/api/v1/orders` | cria pedido + items + endereço + evento inicial |
| `GET` | `/api/v1/orders` | lista pedidos do usuário (suporta `?status=active\|done\|all`, `?page`, `?per_page`) |
| `GET` | `/api/v1/orders/:id` | detalhe + items + endereço + tracking events |
| `PATCH` | `/api/v1/orders/:id/cancel` | transição para `cancelled` (apenas se não shipped/delivered) |

**Headers comuns:**
- `X-User-Id: <opaque-id>` — identifica o dono do pedido (RLS implementado em SQL via `WHERE user_id = $1`)
- `X-Request-Id: <id>` — correlação; se ausente, o serviço gera e devolve no header da resposta

**Error envelope** (padrão compartilhado com catalog + payment):
```json
{ "error": "human readable", "code": "not_found", "requestId": "18a94d..." }
```
Códigos: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `db_error`, `internal`.

### Exemplo: criar pedido

```bash
curl -X POST http://localhost:8092/api/v1/orders \
  -H "Content-Type: application/json" \
  -H "X-User-Id: user-001" \
  -d '{
    "paymentMethod": "pix",
    "shippingCost": 19.90,
    "items": [
      {"productId":"00000000-0000-0000-0000-000000000001",
       "name":"Furadeira Bosch","icon":"⚒",
       "sellerId":"ferragem-silva","sellerName":"Ferragem Silva",
       "quantity":1,"unitPrice":329}
    ],
    "address": {
      "street":"Rua X","number":"42","neighborhood":"Centro",
      "city":"São Paulo","state":"SP","cep":"01310-000"
    }
  }'
```

Response 201:
```json
{
  "id": "a069ce01-72af-4aa1-aeca-adb919d4f020",
  "number": "2026-79894",
  "userId": "user-001",
  "status": "pending_payment",
  "paymentMethod": "pix",
  "paymentId": null,
  "subtotal": 329,
  "shippingCost": 19.9,
  "total": 348.9,
  "items": [...],
  "address": {...},
  "trackingEvents": [
    { "status": "pending_payment", "description": "Pedido criado. Aguardando pagamento.", "occurredAt": "..." }
  ],
  "createdAt": "..."
}
```

---

## Configuração

| Var | Default | Descrição |
|---|---|---|
| `PORT` | `8092` | porta HTTP |
| `ORDER_DB_URL` | `postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable` | DSN Postgres |

---

## Rodar

```bash
# do diretório raiz
make infra-up              # Postgres + Redpanda
make order-db-reset        # schema + 60 pedidos
make order-run             # servidor em :8092

# atalho completo (infra + payment + catalog + order + SPA)
make dev-full              # todos em paralelo
```

### Comandos do Makefile

```bash
make order-run
make order-build
make order-test             # roda todos os tests (unit + integration)

make order-db-migrate       # aplica *.up.sql
make order-db-migrate-down  # reverte
make order-db-seed          # popula seed.sql
make order-db-reset         # down + up + seed
make order-db-clean         # TRUNCATE sem perder schema
make order-db-status        # \dt + row counts
make order-db-psql          # shell interativo
make order-db-dump          # backups/order_<ts>.sql
make order-db-restore FILE=<path>
```

---

## Testes

8 testes de integração cobrindo:

| Teste | Cenário |
|---|---|
| `TestOrders_Unauthorized` | 401 quando `X-User-Id` ausente |
| `TestOrders_List` | listagem retorna ≥ 1 pedido seed |
| `TestOrders_List_FilterActive` | `?status=active` exclui delivered/cancelled |
| `TestOrders_List_FilterDone` | `?status=done` só devolve delivered/cancelled |
| `TestOrders_Get_NotFound` | 404 para ID inexistente |
| `TestOrders_Get_WrongUser` | 404 para ordem de outro usuário (isolamento) |
| `TestOrders_CreateAndCancel` | POST retorna 201 com total calculado; PATCH cancela; segundo PATCH retorna 409 |
| `TestOrders_Create_BadRequest` | items vazios e paymentMethod inválido → 400 |

```bash
make order-test               # roda tudo
# ou, dentro de services/order-service:
go test ./... -v
```

Os testes skipam se `localhost:5437` não estiver acessível (CI sem DB passa). Também skipam se a tabela `orders` tiver 0 linhas (pedem `make order-db-seed` antes).

---

## Integração com o frontend

Os hooks [app/src/hooks/useOrders.ts](../../app/src/hooks/useOrders.ts) detectam `VITE_ORDER_URL` via `isOrderEnabled`. Quando setado, as chamadas vão para o backend real usando `orderGet()`/`orderPatch()` em [app/src/lib/api.ts](../../app/src/lib/api.ts). Sem a var, usa `MOCK_ORDERS`.

**Fluxo end-to-end (dev):**
1. Usuário loga no SPA (auth mock aceita qualquer credencial) — `authStore.user.id` fica como string opaca (`mock-1`, `mock-2`, ...)
2. SPA lista pedidos: `GET /api/v1/orders?per_page=50` com `X-User-Id: mock-1`
3. Order-service busca `WHERE user_id = 'mock-1'` — seed tem `user-001..user-020`, então mock users retornam lista vazia
4. Usuário faz checkout → SPA cria pedido real com `X-User-Id: mock-1`
5. Pedido aparece na aba "Pedidos" desse mesmo user

Para ver pedidos do seed, o mock auth precisaria gerar um dos `user-001..user-020`. Pequeno ajuste futuro: mapear email do login para um user_id determinístico.

---

## Próximos passos

- **Phase B3 — `auth-service`**: `users` + `addresses` + JWT. Remove `RequireUser()` middleware, substitui por `JWTMiddleware`.
- **Webhook integration com payment-service**: quando `payment.confirmed` chega, o order-service atualiza `payment_id`, `paid_at`, `status = 'paid'` e insere evento de tracking. Isso é parte da Sprint 15 (disputas) ou pode ser antecipado.
- **Status manual transitions por admin**: endpoints `PATCH /orders/:id/status` com role admin (Sprint 20 — admin console).
