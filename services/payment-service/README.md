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
| `POST` | `/api/v1/payments/:id/sync` | chama `mp.GetPayment` e atualiza status local — **workaround de webhook em dev** (sem ngrok). Em produção o webhook real substitui. Inclui comparação de `amount` MP vs DB (foundation da issue C3 do audit). |

Middleware: `JWTMiddleware(JWT_SECRET)` decodifica `Authorization: Bearer <JWT>` emitido pelo [auth-service](../auth-service/README.md). Extrai `user_id` e `email` para injetar no contexto.

### Payload do `POST /api/v1/payments`

```json
{
  "order_id": "uuid-do-pedido",
  "method": "pix" | "boleto" | "card",
  "amount": 199.90,
  "payer_cpf": "12345678901",   // opcional — OBRIGATÓRIO para method=boleto
  "payer_name": "Nome completo" // opcional — OBRIGATÓRIO para method=boleto
}
```

### Resposta `201 Created`

```json
{
  "id": "uuid-pagamento-local",
  "method": "card",
  "status": "pending",
  "provider": "stripe",                      // stripe | mercadopago
  "psp_id": "pi_3TQa8WLQCtijFcSY12pJyBht",
  "clientSecret": "pi_..._secret_...",       // Stripe Elements (frontend confirma sem PCI scope)
  "psp_payload": {                           // dados normalizados pro frontend
    "type": "card" | "pix" | "boleto",
    "client_secret": "...",
    "next_action": { "pix_display_qr_code": {...} | "boleto_display_details": {...} }
  }
}
```

- **Card (Stripe):** frontend usa `clientSecret` em `<PaymentElement>` + `stripe.confirmPayment()`.
- **Pix (Stripe):** `next_action.pix_display_qr_code.data` (copy-paste) + `image_url_png`.
- **Boleto (Stripe):** `next_action.boleto_display_details.hosted_voucher_url` (HTML imprimível) + `pdf` + `number` (linha digitável).
- **Card (MP):** `psp_payload.init_point` (URL do Checkout Pro — redirect).

> **Nota (audit C1/C2 — fechado em 2026-04-27):** `amount` e `order_id` agora são validados via order-service. O backend chama `GET /api/v1/orders/:id` propagando o JWT do cliente; usa `order.total` autoritativamente (body amount vira hint), e o order-service responde 404 se o pedido não pertence ao user. Detalhes: [§Segurança abaixo](#segurança--sprint-85-fase-1--2026-04-27). `payer_cpf`/`payer_name` ainda vêm do cliente (backlog: buscar do auth-service). Ver [audit](../../docs/security/payment-service-audit-2026-04-24.md).

---

## Configuração

| Var | Default | Descrição |
|---|---|---|
| `PORT` | `8090` | porta HTTP |
| `PAYMENT_DB_URL` | `postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable` | DSN Postgres |
| `JWT_SECRET` | (obrigatório em prod) | **mesmo valor** em auth + order + payment; em prod precisa ≥ 32 chars não-default; em dev (`DEV_MODE=true`) aceita qualquer coisa |
| `DEV_MODE` | `false` | `true` libera fallbacks de dev (X-User-Id, secret curto, webhook sem secret). NUNCA em prod |
| `ORDER_SERVICE_URL` | `http://localhost:8092` | base URL pro `GET /orders/:id` (audit C1/C2) |
| `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` | (obrigatórios em prod se `PSP_PROVIDER=stripe`) | secret + webhook signing key da Stripe |
| `MP_ACCESS_TOKEN` / `MP_WEBHOOK_SECRET` | (obrigatórios em prod se `PSP_PROVIDER=mercadopago`) | token + webhook signing key do MP |
| `MP_PUBLIC_KEY` | — | chave pública MP (usada pelo frontend; serviço apenas propaga) |
| `MP_WEBHOOK_SECRET` | — | segredo para verificar HMAC dos webhooks |
| `REDPANDA_BROKERS` | `localhost:19092` | brokers do Redpanda para publicar outbox |

Em dev, o Makefile do serviço faz `include ../../.env.local` automaticamente — só criar o arquivo e rodar `make run`. Em produção, virá de AWS Secrets Manager.

**Atenção ao sincronizar secrets em dev:** `JWT_SECRET` precisa ser idêntico em auth-service, order-service e payment-service — senão o JWT emitido pelo auth não valida nos outros. Exemplo funcional:

```bash
# Terminal 1: auth
cd services/auth-service && JWT_SECRET="change-me-in-production" ./bin/auth-service

# Terminal 2: payment
cd services/payment-service && \
  MP_ACCESS_TOKEN=... MP_PUBLIC_KEY=... JWT_SECRET="change-me-in-production" \
  REDPANDA_BROKERS="localhost:19092" ./bin/payment-service
```

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

## Testando integração MP em dev

**Status atual (2026-04-24):** Integração parcialmente validada em sandbox. Ver [docs/security/mp-integration-test-2026-04-24.md](../../docs/security/mp-integration-test-2026-04-24.md) para detalhes completos.

| Método | Status sandbox | Como testar |
|---|---|---|
| **Cartão** | ✅ Funciona | Retorna `sandbox_init_point` do Checkout Pro |
| **Pix direto** | 🟡 Bloqueado | MP requer onboarding da conta seller (não disponível em test users); workaround: migrar pra Preferences API |
| **Boleto direto** | 🟡 Bloqueado | Mesma causa |

### Smoke test rápido — Cartão

```bash
# 1. Login e pegar JWT
TOKEN=$(curl -s -X POST http://localhost:8093/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test1@utilar.com.br","password":"utilar123"}' \
  | jq -r .accessToken)

# 2. Criar payment cartão
RESP=$(curl -s -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"55555555-5555-4555-8555-555555555555","method":"card","amount":99.90}')

echo "$RESP" | jq '.id, .status, .psp_payload.sandbox_init_point'
```

### Completar o checkout no navegador

1. Abrir `sandbox_init_point` em **janela anônima** (evita conflito com sessão MP pessoal)
2. Se pedir login, usar um **test user buyer** (criar via `POST https://api.mercadopago.com/users/test_user`)
3. Ou continuar como convidado e preencher cartão de teste:

| Bandeira | Número | CVV | Validade | Titular |
|---|---|---|---|---|
| Mastercard | `5031 4332 1540 6351` | `123` | `11/30` | `APRO` |
| Visa | `4235 6477 2802 5682` | `123` | `11/30` | `APRO` |
| Amex | `3753 651535 56885` | `1234` | `11/30` | `APRO` |

Titular `APRO` = aprovado. Outras flags: `OTHE` (rejeitado genérico), `CONT` (pendente), `FUND` (saldo insuficiente), `SECU` (CVV inválido).

### Erro comum: "Uma das partes é de teste"

Aparece se você está logado no navegador com sua conta MP pessoal/produção + abrindo checkout de seller teste. Solução: **janela anônima** sem login, ou logar com test user buyer criado via API.

### Sincronização de status (workaround webhook)

Após o pagamento ser processado no MP sandbox, chame sync para trazer o status:

```bash
PAYMENT_ID="<id retornado no POST>"
curl -X POST http://localhost:8090/api/v1/payments/$PAYMENT_ID/sync \
  -H "Authorization: Bearer $TOKEN"
# → { status: "confirmed", mp_status: "approved", mp_amount: 99.90, local_amount: 99.90, changed: true }
```

Em produção, webhook `POST /webhooks/mp` faz isso automaticamente quando MP notifica.

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

## Segurança — Sprint 8.5 Fase 1 ✅ (2026-04-27)

5 CRITICALs do [audit](../../docs/security/payment-service-audit-2026-04-24.md) fechados:

- **C1+C2 — cross-service amount/ownership**: `POST /api/v1/payments` agora chama `GET /api/v1/orders/:id` no order-service propagando o JWT do cliente. O `amount` que vai pro PSP vem de `order.total` (server-side); body amount vira hint logado. Se order não existe, não pertence ao user, ou está em status que não aceita pagamento → 4xx. Cliente: [`internal/orderclient`](internal/orderclient/).
- **C3 — webhook valida amount via PSP**: handler reescrito provider-agnostic em `/webhooks/:provider`. Antes de promover status, chama `gateway.GetPayment(pspID)` e compara `pspResult.Amount` com `payments.amount` local. Mismatch → não confirma + flag `psp_metadata.amount_mismatch=true` + outbox event `payment.fraud_suspect` pra revisão manual.
- **C4 — MP HMAC formato V2**: `parseMPSignatureHeader` parsa `ts=X,v1=Y`, manifest `id:<data.id>;request-id:<x-request-id>;ts:<ts>;`, HMAC-SHA256 + `hmac.Equal` constant-time, replay window de 5 minutos. Stripe já estava OK via SDK oficial.
- **C5 — fail-closed**: `config.Load` em produção (`DEV_MODE=false`) recusa subir sem `STRIPE_WEBHOOK_SECRET` (PSP=stripe) ou `MP_WEBHOOK_SECRET` (PSP=mp). Mesmo padrão pro `JWT_SECRET` (auth/order/payment).

50+ testes Go cobrindo os fixes — ver `internal/{config,handler,orderclient}/*_test.go` + `internal/psp/mercadopago/webhook_test.go`.

## Próximos passos

- **Sprint 8.5 Fase 2 — Hardening operacional** (~8h): Idempotency-Key, rate limit em `/payments`, CORS whitelist via env, `MaxBytesReader` no body, JWT claims tipadas.
- **order-service consumer** — quando `payment.confirmed` chega no Redpanda, order-service consome e avança status pra `paid` + `paid_at`. (Hoje drainer publica mas ninguém consome.)
- **Sprint 15** — disputas/estornos: `POST /payments/:id/refund`, webhook `chargeback.*`.
- **Sprint 22** — métricas Prometheus em `/metrics`, Sentry SDK, alertas `payment_success_rate < 95%`.

---

## Stripe Elements no frontend (SPA)

A SPA confirma cartão **sem redirect** usando Stripe Elements (`<PaymentElement>`).
- Backend cria `PaymentIntent` e retorna `clientSecret`.
- Frontend monta `<Elements stripe={stripePromise} options={{ clientSecret }}>` e chama `stripe.confirmPayment({ redirect: 'if_required' })`.
- PCI scope: SAQ-A — campos sensíveis vivem dentro do iframe Stripe.

Configuração rápida na SPA (`app/.env.local`):

```bash
VITE_API_URL=http://localhost:8090
VITE_STRIPE_PUBLISHABLE_KEY=pk_test_...   # mesma conta do STRIPE_SECRET_KEY do backend
```

Cartões de teste (qualquer data futura, qualquer CVC):
- `4242 4242 4242 4242` — sucesso
- `4000 0000 0000 0002` — recusado
- `4000 0025 0000 3155` — exige 3DS

Componentes:
- [`app/src/lib/stripe.ts`](../../app/src/lib/stripe.ts) — singleton `loadStripe()`.
- [`app/src/pages/checkout/CardPayment.tsx`](../../app/src/pages/checkout/CardPayment.tsx) — Elements + `confirmPayment`.
- [`app/src/hooks/usePayment.ts`](../../app/src/hooks/usePayment.ts) — parser branchando por `provider` (stripe|mercadopago|mock).
