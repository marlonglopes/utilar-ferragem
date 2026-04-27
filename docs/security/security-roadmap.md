# Security Roadmap — pós Sprint 8.5 Fase 2

**Última atualização**: 2026-04-27 (Bundles 1+2+3+4 fechados)

Documento vivo do trabalho de segurança. CRITICALs e HIGHs zerados; agora rastreia apenas MEDIUMs (hardening orgânico) e LOWs (backlog).

---

## Status atual (snapshot)

| Categoria | Aberto | Fechado |
|---|---:|---:|
| CRITICAL | **0** ✅ | 14 (audit completo + Sprint 8.5 Fase 1) |
| HIGH     | **0** ✅ | 19 (8 audit + 11 Sprint 8.5 Fase 2) |
| MEDIUM   | 18       | 4 |
| LOW      | 14       | 0 |

Ver detalhes em:
- [full-audit-2026-04-26.md](full-audit-2026-04-26.md) — auth/order/catalog
- [payment-service-audit-2026-04-24.md](payment-service-audit-2026-04-24.md) — payment

---

## Plano de execução — Sprint 8.5 Fase 2 (~19h, 2.5 dias)

Os 11 HIGHs em aberto agrupados em 4 bundles pelo eixo dependência/coesão. Cada bundle pode ser um PR independente, na ordem proposta abaixo (de menor pra maior risco de regressão).

### Bundle 1 — Quick wins isolados ✅ FECHADO (2026-04-27)

Pequenos fixes pontuais, baixo risco, alto sinal-pra-ruído. Concluídos em ~3h.

| ID | Serviço | Issue | Status | Fix |
|---|---|---|---|---|
| **O2-H4** | order | Order number sequencial enumerável | ✅ | `generateOrderNumber()` usa `crypto/rand` + 8 chars base32 (40 bits) — `internal/handler/ordernumber.go` |
| **CT1-H4** | catalog | Slug GET timing attack | ✅ | `padToMinElapsed(50ms)` em `GetBySlug` e `Related` — `internal/handler/product.go` |
| **A9-H5** | auth | Forgot-password timing-safe | ✅ | `padToMinElapsed(200ms)` em `ForgotPassword` — `internal/handler/timing.go` |
| **H2** | payment | JWT claims tipadas | ✅ | Novo `internal/auth/jwt.go` com `Claims` struct + `ParseAccessToken`; middleware migrado de `jwt.MapClaims` |
| **H5** | payment | MaxBytesReader em `POST /payments` | ✅ | `http.MaxBytesReader(16KB)` antes do bind — `internal/handler/payment.go` |

**Testes adicionados**: `ordernumber_test.go` (formato/entropia/colisão), `timing_test.go` (catalog + auth), `jwt_test.go` (assinatura/expiração/algoritmo none), `payment_bodylimit_test.go` (cap 16KB).

### Bundle 2 — Rate limiting + Idempotency-Key via Redis ✅ FECHADO (2026-04-27)

Adicionou Redis ao stack e helpers compartilhados em `pkg/` via Go workspace.

**Decisão de arquitetura**: criado `go.work` na raiz + módulo `github.com/utilar/pkg` com:
- `pkg/ratelimit/limiter.go` — janela fixa via `INCR + EXPIRE` (1 round-trip), fail-open quando Redis cai
- `pkg/idempotency/store.go` — `SETNX` reservation + replay (handshake protege contra concorrência)
- Tests com `miniredis` (sem dependência de Redis real)

**Setup**:
- `redis:7-alpine` em [docker-compose.yml](../../docker-compose.yml) (256MB, allkeys-lru)
- `REDIS_URL` env var em todos os 4 serviços; vazio = features desabilitadas + log warn

| ID | Serviço | Endpoint | Limite | Status |
|---|---|---|---|---|
| **A6-H2** | auth | `/auth/login` | 5/min/IP | ✅ |
|           | auth | `/auth/forgot-password` | 5/min/IP | ✅ |
|           | auth | `/auth/reset-password`  | 5/min/IP | ✅ |
|           | auth | `/auth/verify-email`    | 10/min/IP | ✅ |
| **CT1-H1** | catalog | `/products` + `/products/facets` | 100/min/IP | ✅ |
| **H4** | payment | `POST /payments` | 10/min/user | ✅ |
| **H1** | payment | `POST /payments` Idempotency-Key | TTL 24h | ✅ |

**Testes**: `pkg/ratelimit/limiter_test.go` (5 casos: bloqueio, reset de janela, IPs distintos, fail-open) + `pkg/idempotency/store_test.go` (4 casos: replay, keys distintas, key inválida, sem header).

### Bundle 3 — Cross-service price validation ✅ FECHADO (2026-04-27)

Resolveu **O2-H5**. Decidiu-se NÃO extrair orderclient pra shared (mantém isolamento entre serviços) — apenas adicionou catalogclient autocontido em order-service.

| ID | Serviço | Issue | Status |
|---|---|---|---|
| **O2-H5** | order | `unitPrice` autoritativo via catalog | ✅ |

**Mudanças**:
- Novo endpoint `GET /api/v1/products/by-id/:id` em [catalog-service](../../services/catalog-service/internal/handler/product.go)
- Novo cliente `services/order-service/internal/catalogclient/` (mesmo pattern do payment→order)
- `OrderHandler.Create` agora chama `catalog.GetByID()` por item, sobrescreve `unitPrice` com valor do catalog, loga warning se diverge >1¢
- Erro upstream → 502; produto não existe → 400
- Config nova: `CATALOG_SERVICE_URL` em order-service (default `http://localhost:8091`)

**Testes**: `order_pricing_test.go` cobre tamper bloqueado (0.01 → 599.90), produto não encontrado, upstream error, tolerância de 1 centavo.

### Bundle 4 — Token hashing migration ✅ FECHADO (2026-04-27)

Resolveu **A7-H3**. Migração single-shot (pre-launch, sem dados reais) — substituiu coluna `token` por `token_hash` em uma só transação, sem zero-downtime gymnastics.

| ID | Serviço | Issue | Status |
|---|---|---|---|
| **A7-H3** | auth | Tokens armazenados como SHA-256 | ✅ |

**Mudanças**:
- Migration [`002_token_hash.up.sql`](../../services/auth-service/migrations/002_token_hash.up.sql) (+ down): TRUNCATE + DROP `token` + ADD `token_hash` PRIMARY KEY nas 3 tabelas
- Helper `hashToken()` em [tokenhash.go](../../services/auth-service/internal/handler/tokenhash.go) — SHA-256 hex (sem salt: input já tem 128 bits de entropia via `randToken()`)
- Todos os handlers (Register, Login, Refresh, Logout, VerifyEmail, ForgotPassword, ResetPassword, issueTokens) inserem `hashToken(token)` e fazem lookup por `WHERE token_hash = $1`
- Plaintext só sai pro cliente (cookie/email), nunca volta pro DB

**Testes**:
- `tokenhash_test.go`: determinismo, vetor conhecido SHA-256("abc"), shape hex 64 chars
- `tokenhash_integration_test.go`: invariante "DB nunca tem plaintext" — login emite refresh, query confirma que `token_hash` ≠ plaintext mas == sha256(plaintext)

---

## Ordem de execução

1. ~~**Bundle 1** — Quick wins.~~ ✅ Fechado 2026-04-27.
2. ~~**Bundle 3** — Cross-service price validation.~~ ✅ Fechado 2026-04-27.
3. ~~**Bundle 4** — Token hashing migration.~~ ✅ Fechado 2026-04-27.
4. ~~**Bundle 2** — Redis: rate limit + Idempotency-Key.~~ ✅ Fechado 2026-04-27.

**Sprint 8.5 Fase 2 fechada**. payment-service e auth-service em estado **saudável pra produção**: 0 CRITICAL, 0 HIGH. Restam apenas MEDIUM/LOW (hardening orgânico, sem urgência).

---

## MEDIUM em aberto (~18, ~10h total)

Tratados após HIGHs. Ordem orgânica baseada em ROI:

- **auth A10-M1** — validação de CPF (algoritmo de check digit) — 30min
- **auth A14-M5** — cleanup automático de tokens expirados via cron/job — 1h
- **auth A15-M6** — complexidade mínima de senha (10 chars + blacklist top-passwords) — 1h
- **auth A16-M7** — JWT alg lock pra HS256 exato (evita confusão de algorítmos) — 15min
- **catalog CT1-M1** — limite no número de filtros simultâneos — 30min
- **catalog CT1-M4** — RequestID via UUID v4 (UUID lib ou ULID) — 1h
- **catalog CT1-M5, L1** — CHECK constraints no schema (`stock >= 0`, `price >= 0`) — 30min
- **order O3-M3** — rate limit em create order — engloba HIGH Bundle 2
- **order O3-M4** — pessimistic locking em cancel pra evitar TOCTOU — 1h
- **payment M2** — redactor PII em `psp_payload` — 2h
- **payment M5** — redactor PII em logs — 1h
- **payment M6** — buscar CPF do auth-service pro boleto — 30min (depois do auth client)
- **(transversal) M11** — request_id via ULID em todos os 4 services — 1h

---

## LOW em aberto (14)

Backlog orgânico, sem urgência: audit logging tables, slug enumeration, govulncheck no CI, circuit breaker, SAST (gosec), dependabot, container scanning, etc.

---

## Como abrir issues no GitHub (sugestão)

4 issues principais (1 por bundle):

```bash
gh issue create --title "[Sprint 8.5 Fase 2] Bundle 1: HIGH quick wins (5 fixes, ~3h)" \
  --label security,sprint-8.5,high \
  --body "Ver docs/security/security-roadmap.md#bundle-1"

gh issue create --title "[Sprint 8.5 Fase 2] Bundle 2: Redis — Rate limit + Idempotency-Key (~9h)" \
  --label security,sprint-8.5,high,infra \
  --body "Ver docs/security/security-roadmap.md#bundle-2"

gh issue create --title "[Sprint 8.5 Fase 2] Bundle 3: Cross-service price validation (~3h)" \
  --label security,sprint-8.5,high \
  --body "Ver docs/security/security-roadmap.md#bundle-3"

gh issue create --title "[Sprint 8.5 Fase 2] Bundle 4: Token hashing migration (~3h)" \
  --label security,sprint-8.5,high,migration \
  --body "Ver docs/security/security-roadmap.md#bundle-4"
```

Plus 1 issue agrupando MEDIUMs:

```bash
gh issue create --title "[Sprint 22] Hardening operacional — 18 MEDIUMs do audit" \
  --label security,sprint-22,medium \
  --body "Ver docs/security/security-roadmap.md#medium-em-aberto-18-10h-total"
```

---

## Critério pra reabrir produção

Já cumprido (CRITICAL block clear) — **payment-service tecnicamente desbloqueado** em 2026-04-27.

Recomendação prática:
- **Dev/staging**: pode rodar como está (CRITICALs fechados, transversais hardened).
- **Prod com tráfego < 10 reqs/min de pagamento real**: rodar Bundle 1 antes (3h).
- **Prod com tráfego significativo ou MP em produção real**: completar Bundles 1–4 (19h) antes do go-live.
