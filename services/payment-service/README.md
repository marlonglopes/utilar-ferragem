# payment-service

Serviço de pagamentos do marketplace Utilar Ferragem — integra Mercado Pago (Pix, boleto, cartão), processa webhooks do PSP com idempotência e publica eventos confirmados via outbox → Redpanda.

| | |
|---|---|
| **Stack** | Go 1.26 + Gin 1.12 + Postgres 17 + Mercado Pago SDK + Redpanda (franz-go) |
| **Porta** | `:8090` |
| **DB** | `utilar_payment_db` (Postgres em `localhost:5435`) |
| **Status** | Sprint 08 ✅ em operação em dev/sandbox. ⛔ **BLOQUEADO para produção** — ver [audit 2026-04-24](../../docs/security/payment-service-audit-2026-04-24.md) (Sprint 8.5 pendente) |

Documentação transversal:
- [README raiz](../../README.md)
- [Database maintenance](../../docs/maintenance/database.md)
- [auth-service](../auth-service/README.md) — emite o JWT que este serviço valida

---

## Estrutura

```
payment-service/
  cmd/server/main.go         ← bootstrap + outbox drainer em goroutine
  internal/
    config/                  ← PORT, PAYMENT_DB_URL, MP_ACCESS_TOKEN, MP_PUBLIC_KEY,
                               MP_WEBHOOK_SECRET, JWT_SECRET, REDPANDA_BROKERS
    db/                      ← golang-migrate no startup
    handler/                 ← payment + webhook + auth middleware + errors + middleware
    mercadopago/             ← wrapper cliente Mercado Pago (pix/boleto/preference)
    model/                   ← Payment struct + Request types
    outbox/                  ← drainer: lê payments_outbox unpublished → publica em Redpanda
  migrations/                ← 001 payments, 002 webhook_events, 003 payments_outbox + seed.sql
  Makefile                   ← run / build / test / test-integration / infra-up
```

---

## Modelo de dados

3 tabelas:

| Tabela | Propósito |
|---|---|
| `payments` | pagamento criado pela SPA (method, status, amount, psp_payment_id, psp_payload) |
| `webhook_events` | eventos recebidos do Mercado Pago, com unique `(psp_id, psp_payment_id, event_type)` para idempotência |
| `payments_outbox` | transactional outbox pattern — eventos a publicar em Redpanda, com retry backoff |

**ENUMs:** `payment_method` (pix, boleto, card), `payment_status` (pending, confirmed, failed, expired, cancelled).

**Padrão transactional outbox:** quando um webhook confirma um pagamento, o handler atualiza `payments` **e** insere em `payments_outbox` na mesma transação. O drainer roda em goroutine lendo `WHERE published_at IS NULL`, publica em Redpanda, marca `published_at`. Garante at-least-once delivery mesmo se o serviço cair depois do commit mas antes do publish.

## API

Base URL em dev: `http://localhost:8090`.

### Webhook (público — assinatura HMAC valida o caller)

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/webhooks/mp` | recebe evento do Mercado Pago; valida HMAC-SHA256 contra `MP_WEBHOOK_SECRET`; idempotente via unique `(psp_id, psp_payment_id, event_type)` |

### API (protegida por JWT)

| Método | Rota | Descrição |
|---|---|---|
| `GET`  | `/health` | liveness probe |
| `POST` | `/api/v1/payments` | cria pagamento pending + chama Mercado Pago + retorna psp_payload (QR code / barcode / init_point) |
| `GET`  | `/api/v1/payments/:id` | consulta status (scoped ao `user_id` do JWT) |

Middleware: `JWTMiddleware(JWT_SECRET)` decodifica `Authorization: Bearer <JWT>` emitido pelo [auth-service](../auth-service/README.md). Extrai `user_id` e `email` para injetar no contexto.

---

## Configuração

| Var | Default | Descrição |
|---|---|---|
| `PORT` | `8090` | porta HTTP |
| `PAYMENT_DB_URL` | `postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable` | DSN Postgres |
| `JWT_SECRET` | `change-me` | **precisa ser o mesmo** em auth + order + payment |
| `MP_ACCESS_TOKEN` | — | **obrigatório** — token sandbox/prod do Mercado Pago |
| `MP_PUBLIC_KEY` | — | chave pública MP (usada pelo frontend; serviço apenas propaga) |
| `MP_WEBHOOK_SECRET` | — | segredo para verificar HMAC dos webhooks |
| `REDPANDA_BROKERS` | `localhost:19092` | brokers do Redpanda para publicar outbox |

Em dev, o Makefile do serviço lê `MP_ACCESS_TOKEN` e `MP_PUBLIC_KEY` de `.env.local` na raiz do repo. Em produção, virá de AWS Secrets Manager.

---

## Rodar

```bash
make infra-up          # Postgres + Redpanda
make db-reset          # schema + seed (150 payments, 270 webhook events, 110 outbox entries)
make svc-run           # servidor :8090 (precisa de MP_ACCESS_TOKEN em .env.local)
```

Atalho completo: `make dev-full` sobe infra + 4 serviços + SPA (ver [Makefile](../../Makefile)).

### Comandos do Makefile (root)

```bash
make svc-run           # roda o servidor
make svc-build         # compila binário
make svc-test          # unit tests (HMAC, status resolution)

make db-migrate        # aplica *.up.sql
make db-migrate-down   # reverte
make db-seed           # popula 150 payments
make db-reset          # down + up + seed
make db-status         # \dt + contagens
make db-psql           # shell interativo
make db-dump           # backups/payment_service_<ts>.sql
make db-restore FILE=<path>
```

---

## Testes

### Unit (`make svc-test`)

- `webhook_unit_test.go`: HMAC verification (valid/tampered/wrong-secret/empty); `resolveStatus` (action → status mapping)
- `mercadopago/`: integration com SDK do MP (wrapper, mocks)

### Integration (`make -C services/payment-service test-integration`)

- `webhook_test.go`: idempotency (webhook duplicado não gera duplicação); DB state após confirmed
- Requer Postgres em `localhost:5435` com schema aplicado. Skipa se não disponível.

---

## Seed

150 payments distribuídos entre 100 user UUIDs × 150 orders:

- 50 pix + 50 boleto + 50 card
- Status: ~60% confirmed, 20% pending, 10% expired, 5% failed, 5% cancelled
- 270 webhook_events (1 `payment.created` + 1 final event por pagamento não-pending)
- 110 payments_outbox (90 published from confirmed + 20 pending retry)

Todos os dados de seed têm `psp_metadata->>'seed' = 'true'` para filtrar se necessário.

```bash
make db-seed     # repopula (idempotente via TRUNCATE CASCADE)
```

---

## Fluxos

### 1. Criar pagamento (SPA → payment-service → Mercado Pago)

```
SPA           payment-service         Mercado Pago
 │  POST /api/v1/payments (Bearer JWT)    │
 ├────────────────────────────────────────>
 │                                         │
 │       INSERT payments (pending)         │
 │  ── call MP API (method-specific) ─────>
 │  <── psp_payment_id + psp_payload ──────
 │       UPDATE payments SET psp_*         │
 │  <── 201 { psp_payload } ───────────────┤
 <────────────────────────────────────────
```

### 2. Webhook de confirmação (Mercado Pago → payment-service → outbox → Redpanda)

```
Mercado Pago   payment-service             payments_outbox   Redpanda   order-service
 │  POST /webhooks/mp + HMAC                      │              │             │
 ├──────────────────────────────────────────────> │              │             │
 │  verify HMAC; check idempotency                │              │             │
 │  BEGIN                                         │              │             │
 │    INSERT webhook_events                       │              │             │
 │    UPDATE payments SET status='confirmed'      │              │             │
 │    INSERT payments_outbox (payment.confirmed)  │              │             │
 │  COMMIT                                        │              │             │
 │  <── 200 ─────────────────────────────────────                 │             │
 │                                                                             │
 │       (goroutine) drainer loop:                                             │
 │       SELECT FROM payments_outbox WHERE published_at IS NULL ──> publish ──>│
 │       UPDATE payments_outbox SET published_at = now()                       │
 │                                                                             │
 │                                                              (futuro: order consome
 │                                                               e avança status para 'paid')
```

---

## Próximos passos

- **Sprint 15** — disputas/estornos: endpoints `POST /payments/:id/refund`, webhook `chargeback.*`.
- **Sprint 22** — métricas Prometheus em `/metrics`, Sentry SDK, alertas payment_success_rate < 95%.
- **order-service integration** — quando `payment.confirmed` chega no Redpanda, order-service consome e atualiza `orders.status` + `paid_at`. (hoje o drainer publica mas ninguém consome ainda.)
