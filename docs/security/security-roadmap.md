# Security Roadmap — pós Sprint 8.5 + MEDIUMs + LOWs

**Última atualização**: 2026-04-28 (checkout flow validado end-to-end)

Documento vivo do trabalho de segurança. **Todos os achados de auditoria fechados.** Manutenção a partir daqui é orgânica via dependabot, govulncheck no CI e novos audits.

---

## Status atual (snapshot)

| Categoria | Aberto | Fechado |
|---|---:|---:|
| CRITICAL | **0** ✅ | 14 |
| HIGH     | **0** ✅ | 19 |
| MEDIUM   | **0** ✅ | 18 |
| LOW      | **0** ✅ | 14 |
| **Total** | **0** ✅ | **65** |

**Suite de testes**: 238 Go (race) + 154 frontend = **392 PASS, 0 FAIL, 0 SKIP**

Ver detalhes em:
- [full-audit-2026-04-26.md](full-audit-2026-04-26.md) — auth/order/catalog
- [payment-service-audit-2026-04-24.md](payment-service-audit-2026-04-24.md) — payment
- [verification-2026-04-27.md](verification-2026-04-27.md) — sweep pós-remediation
- [checkout-flow-validation-2026-04-28.md](checkout-flow-validation-2026-04-28.md) — E2E das 3 formas de pagamento

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

## MEDIUMs ✅ FECHADOS (2026-04-27)

Executados em 3 fases após bundles de HIGHs:

### Fase 1 — Quick wins (~2.5h)

| ID | Serviço | Fix |
|---|---|---|
| **A16-M7** | auth+order+payment | JWT alg lock estrito em HS256 (`t.Method.Alg() != HS256`) — anti algorithm confusion |
| **O3-M1** | order | Items array já tinha `max=100,dive` (regressão test adicionado) |
| **O3-M3** | order | Rate limit `POST /orders` 20/min/user via Redis |
| **CT1-M5/L1** | catalog | Migration 002 com CHECK constraints (`price>=0`, `stock>=0`, etc.) |
| **M6** | payment | Novo `authclient` busca CPF/Name de `/api/v1/me` pro boleto (não confia no body) |

### Fase 2 — Hardening operacional (~4h)

| ID | Serviço | Fix |
|---|---|---|
| **A10-M1** | auth | Validação CPF (mod-11 check digit) em Register; rejeita inválidos e CPFs com dígitos iguais |
| **A15-M6** | auth | Complexidade de senha: 10+ chars, 3 de 4 categorias, blacklist top-passwords (incl. pt-BR) |
| **A14-M5** | auth | `StartTokenCleanup()` goroutine apaga refresh/reset/verify tokens expirados a cada 1h |
| **CT1-M1** | catalog | Truncate de filtros (`q`/`brand`/`category`) com `truncateRunes` (UTF-8 safe) — anti-DoS via query |
| **O3-M4** | order | `SELECT ... FOR UPDATE` em Cancel — pessimistic lock previne TOCTOU |

### Fase 3 — Compliance/PII + cleanup (~3h)

| ID | Serviço | Fix |
|---|---|---|
| **M1** | payment | Removido código morto `verifyHMAC` + `resolveStatus` (legacy MP); `webhook_unit_test.go` deletado |
| **M2** | payment | `redactPSPPayload()` mascara PAN/CVC/CPF/identification em `psp_payload` antes do INSERT (mantém last4 + IDs) |
| **M5** | payment | `redactLogValue()` aplicado em `Respond()` — mascara emails, CPFs e PANs em mensagens de erro |
| **M11+CT1-M4** | transversal | Novo `pkg/requestid` com ULID (k-sortable, 26 chars) substitui `newRequestID()` em 4 serviços |

**Testes adicionados nesta sessão**:
- `cpf_test.go`, `password_test.go`, `cleanup_test.go` (auth)
- `redact_test.go`, `redactlog_test.go`, `authclient/client_test.go` (payment)
- `order_itemscap_test.go` (order — regressão)
- `pkg/requestid/requestid_test.go` (4 casos: formato, distinct, k-sortable, concorrência)
- Migration `002_check_constraints.up.sql` (catalog)
- Migration `002` validada no DB local (rejeita `price=-10`)

---

## LOWs ✅ FECHADOS (2026-04-27)

| ID | Serviço | Fix |
|---|---|---|
| **L-AUTH-1** | auth | Migration 003 + tabela `auth_events` + log de register/login_success/login_failure/logout/email_verified/password_reset_requested/password_changed |
| **L-AUTH-2** | auth | Deny-list de access tokens via Redis (`auth:revoked:<userID>` com TTL=AccessTokenTTL); JWTAuth middleware checa antes de aceitar |
| **L-AUTH-3** | auth | Rate limit em `POST /auth/register` (5/h por IP — registro é raro no funil) |
| **L-ORDER-1** | order | Regression test confirma que `?status=arbitrary` é tratado como "all" sem SQL injection (já era safe via switch) |
| **L-ORDER-2** | order | Idempotency-Key middleware em `POST /orders` (TTL 24h, mesma key replays response) |
| **L-CATALOG-1** | catalog | `Cache-Control: public, max-age=N` em GETs — listings 60s, detail 300s |
| **L-CATALOG-2** | catalog | ETag weak (`W/"<sha256-prefix>"`) baseado em `updated_at`; `If-None-Match` → 304 |
| **L-PAYMENT-1** | transversal | Novo `pkg/httpclient` com transport defensivo (dial 1s, TLS 2s, 10 conns/host, 30s idle) usado em orderclient/authclient/catalogclient |
| **L-CI-1** | infra | Workflow `go-ci.yml` com `govulncheck ./...` por módulo |
| **L-CI-2** | infra | Workflow `go-ci.yml` com `gosec` (SARIF upload pra Code Scanning) |
| **L-CI-3** | infra | `dependabot.yml` para 6 ecossistemas (npm + 5 Go modules + GitHub Actions) |
| **L-CI-4** | infra | Workflow `go-ci.yml` matriz: build/vet/test em pkg + 4 services |
| **L-MISC-1** | infra | Trivy container scan — adiar até existir Dockerfile (skip por enquanto) |
| **L-MISC-2** | infra | Hook `.githooks/pre-commit` (vet + build nos módulos staged); ativar via `git config core.hooksPath .githooks` |

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
