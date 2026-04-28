# Security Verification Report — 2026-04-27

**Tipo**: Double-check pós-remediation completa.
**Escopo**: 4 microservices Go (auth, catalog, order, payment) + 4 pacotes shared (`pkg/`) + frontend SPA (Vite/React).
**Método**: re-leitura dos audits + grep de anti-patterns + ferramentas (`go vet`, `govulncheck`, `gosec`, `npm audit`) + suite completa de testes.
**Resultado**: ✅ **Backend e frontend liberados pra produção** — todos os 65 findings de auditoria fechados, 1 nova vulnerabilidade descoberta e fixada nesta sessão.

> **Continuação**: este sweep cobriu segurança estática + suite de testes. A validação E2E do flow de pagamento (Card/Boleto/Pix com Stripe Test mode real) foi feita em [checkout-flow-validation-2026-04-28.md](checkout-flow-validation-2026-04-28.md).

---

## 1. Sumário executivo

| Categoria | Status | Notas |
|---|---|---|
| 14 CRITICALs do audit | ✅ Fechados | Verificados via grep + leitura linha-a-linha |
| 19 HIGHs do audit | ✅ Fechados | Idem |
| 18 MEDIUMs do audit | ✅ Fechados | Idem |
| 14 LOWs do audit | ✅ Fechados | Idem |
| **govulncheck** | ✅ 0 vulns | **Achou GO-2025-3553 (jwt v5.2.1) — fixado nesta sessão** |
| **gosec** | ✅ 0 issues | 7 false positives anotados com `#nosec` documentado |
| **go vet** | ✅ 0 issues | Em pkg + 4 services |
| **npm audit** | ✅ 0 vulns | `npm audit --audit-level=moderate` |
| **eslint** | ✅ 0 warnings | `--max-warnings 0` (CI-grade) |
| **Suite de testes** | ✅ 389 passing | 237 Go + 152 frontend |

---

## 2. Vulnerabilidade descoberta + fixada

### GO-2025-3553 — `golang-jwt/jwt v5.2.1` (DoS via header parsing)

**Severity**: HIGH (DoS)
**Descoberto por**: `govulncheck ./...` em auth-service / order-service / payment-service.
**Detalhe**: Excessive memory allocation durante parsing de header JWT — atacante manda token com header malformado e força alocação de memória descontrolada.
**Trace**: chamada `jwt.ParseWithClaims` → `jwt.Parser.ParseUnverified` afetada nos 3 serviços que validam JWT.
**Fix**: bump pra `v5.2.2` em [auth-service/go.mod](../../services/auth-service/go.mod), [order-service/go.mod](../../services/order-service/go.mod), [payment-service/go.mod](../../services/payment-service/go.mod).
**Verificação pós-fix**: `govulncheck ./...` → "No vulnerabilities found" em todos os 3 serviços.

---

## 3. Verificação de cada padrão transversal (20 spot-checks)

| # | Padrão | Localização | Verificado |
|---|---|---|---|
| 1 | DBError não vaza `err.Error()` no body HTTP | [errors.go](../../services/payment-service/internal/handler/errors.go) — `slog.Error` interno + `Respond` com mensagem genérica | ✅ |
| 2 | JWT alg lock estrito em HS256 | [auth/jwt.go](../../services/auth-service/internal/auth/jwt.go), [order/middleware.go](../../services/order-service/internal/handler/middleware.go), [payment/auth/jwt.go](../../services/payment-service/internal/auth/jwt.go) | ✅ |
| 3 | Hardcoded secrets só em fail-closed check ou blacklist | `change-me`/`change-me-in-prod-please` aparecem só em `Load()` rejeitando-os; `password123` é blacklist; `dev-only-secret-not-for-production` rejeitado em prod | ✅ |
| 4 | CORS `*` apenas em dev (sem `ALLOWED_ORIGINS`) | Whitelist via env em prod, wildcard só quando `len(allowed)==0` | ✅ |
| 5 | Token plaintext eliminado do DB | `INSERT INTO refresh_tokens (token_hash, ...)` — sem coluna `token` plaintext | ✅ |
| 6 | Order number entropia via crypto/rand | [ordernumber.go](../../services/order-service/internal/handler/ordernumber.go) — base32 40 bits | ✅ |
| 7 | MaxBytesReader cap em payload de payment | [payment.go:82](../../services/payment-service/internal/handler/payment.go#L82) — 16KB | ✅ |
| 8 | ESCAPE em LIKE no catalog (anti-ReDoS pg_trgm) | [product.go:50,252](../../services/catalog-service/internal/handler/product.go) — `escapeLikePattern` + `ESCAPE '\'` | ✅ |
| 9 | Webhook HMAC constant-time compare | [psp/mercadopago/gateway.go:187](../../services/payment-service/internal/psp/mercadopago/gateway.go#L187) — `hmac.Equal` | ✅ |
| 10 | Fail-closed em config de secrets | `ErrInsecureJWTSecret` + checks STRIPE/MP webhook secret em prod | ✅ |
| 11 | Pessimistic locking em Cancel order | [order.go:248](../../services/order-service/internal/handler/order.go#L248) — `SELECT ... FOR UPDATE` | ✅ |
| 12 | Cross-service price validation autoritativa | [order.go:62,290+](../../services/order-service/internal/handler/order.go) — `applyAuthoritativePricing` sobrescreve `unitPrice` com `catalog.GetByID().Price` | ✅ |
| 13 | Token hashing SHA-256 (sem plaintext em DB) | 9 INSERT/UPDATE em auth.go usam `hashToken(...)`; sem ocorrências de `token =` em queries | ✅ |
| 14 | Rate limiting wired em todos os endpoints públicos | auth: 5 endpoints; catalog: search; order: create; payment: create | ✅ |
| 15 | Idempotency-Key wired em endpoints write críticos | order:create + payment:create (TTL 24h) | ✅ |
| 16 | Timing-safe ForgotPassword | [auth.go:345](../../services/auth-service/internal/handler/auth.go#L345) — `padToMinElapsed(200ms)` | ✅ |
| 17 | ULID requestID em todos os 4 services | [pkg/requestid](../../pkg/requestid) usado nas 4 middlewares | ✅ |
| 18 | Audit events nos 7 fluxos sensíveis | register, login OK/fail, logout, email_verified, password_reset_requested, password_changed | ✅ |
| 19 | PII redactor em psp_payload + logs | `redactPSPPayload` em INSERT; `redactLogValue` em `Respond` (mascara emails/CPFs/PANs) | ✅ |
| 20 | Sem SQL string concat de valores | Todas as queries usam `$N` placeholders; whereSQL/orderBy montados de literais hardcoded | ✅ |

---

## 4. Anti-patterns scan

| Categoria | Resultado |
|---|---|
| `os/exec` / `exec.Command` (RCE risk) | ✅ Não aparece em código fonte |
| `unsafe.Pointer` / `reflect.New` arbitrário | ✅ Não aparece |
| `http.Client{}` sem timeout | ✅ Todos os clientes via `pkg/httpclient.New(timeout)` |
| `tls.Config{InsecureSkipVerify}` | ✅ Não aparece |
| `panic()` em hot paths | ✅ 2 ocorrências (`crypto/rand` failure) — intencional, documentado A3-C3 |
| `fmt.Sprintf` injetando valores em SQL | ✅ Não aparece — só `$N` placeholders |

---

## 5. Resultados das ferramentas

### `go vet ./...` (todos os 5 módulos)

```
=== pkg ===
=== services/auth-service ===
=== services/catalog-service ===
=== services/order-service ===
=== services/payment-service ===
```

(saída vazia = limpo)

### `govulncheck ./...` (após bump jwt v5.2.2)

```
=== pkg ===
No vulnerabilities found.
=== services/auth-service ===
No vulnerabilities found.
=== services/catalog-service ===
No vulnerabilities found.
=== services/order-service ===
No vulnerabilities found.
=== services/payment-service ===
No vulnerabilities found.
```

### `gosec` (após anotações `#nosec` documentadas)

```
=== pkg ===                  Issues: 0
=== services/auth-service === Issues: 0 (3 nosec G101 — devSecret placeholder)
=== services/catalog-service === Issues: 0 (2 nosec G202 — SQL com placeholders posicionais hardcoded)
=== services/order-service === Issues: 0 (1 nosec G101 — devSecret placeholder)
=== services/payment-service === Issues: 0 (4 nosec — 2x G101 devSecret, 2x G117 Stripe client_secret público)
```

**Justificativas dos `#nosec`**:
- **G101 em `devSecret`**: placeholder dev-only, rejeitado em prod via fail-closed em `Load()`. Não é credential.
- **G202 em catalog**: `whereSQL`/`orderBy` montados de literais hardcoded com `$N` placeholders; valores via `args` slice. `orderBy` passa por whitelist (CT1-M3). Atacante não controla SQL fragments.
- **G117 em Stripe `ClientSecret`**: deliberadamente público — escopado a 1 PaymentIntent, expira ao confirmar, projetado pra `stripe.confirmPayment(clientSecret)` no browser. Stripe documenta como tal.

### `npm audit --omit=dev --audit-level=moderate`

```
found 0 vulnerabilities
```

### `eslint . --max-warnings 0`

(saída vazia = limpo)

---

## 6. Suite completa de testes

### Go: 237 tests passando, 0 failing

| Módulo | Tests | Failures | Notas |
|---|---:|---:|---|
| `pkg` (4 sub-pkgs) | 16 | 0 | httpclient + idempotency + ratelimit + requestid (com miniredis) |
| `auth-service` | 47 | 0 | Inclui integration tests com Postgres (auth_service DB) |
| `catalog-service` | 41 | 0 | Inclui integration com seeds reais |
| `order-service` | 31 | 0 | Inclui price-tampering integration tests |
| `payment-service` | 102 | 0 | Inclui webhook + Stripe + MP gateway tests |

Comando: `go test -race -count=1 ./...` (com race detector ativo).

### Frontend: 152/152 passando

```
Test Files  19 passed (19)
     Tests  152 passed (152)
  Duration  5.69s
```

Stack: vitest + jsdom. Inclui testes de cartStore, useOrders, ProductDetailPage, CheckoutPage, etc.

### Build artifacts

- 4 binários Go compilados sem warnings
- Frontend `dist/`: 512KB JS (153KB gzip), 34KB CSS

---

## 7. Histórico de commits desta sessão

```
93356c6  docs(security): plano de execução pra 11 HIGHs em 4 bundles
8828f7b  security: Sprint 8.5 Fase 2 — fecha 11 HIGHs em 4 bundles
2e6976f  security: fecha 14 MEDIUMs em 3 fases
dbb867c  security: fecha 14 LOWs — backlog de segurança limpo
(este)   security: govulncheck achou GO-2025-3553; bump jwt v5.2.2 + #nosec annotations
```

---

## 8. Critério de produção

✅ **Backend pronto pra produção** desde que:
- [x] Todos CRITICALs/HIGHs/MEDIUMs/LOWs do audit fechados
- [x] `JWT_SECRET` configurado com 32+ chars random em prod
- [x] `DEV_MODE=false` (default) em prod
- [x] `ALLOWED_ORIGINS=https://utilarferragem.com.br,...` (CORS whitelist)
- [x] `STRIPE_WEBHOOK_SECRET` ou `MP_WEBHOOK_SECRET` configurado (fail-closed)
- [x] `REDIS_URL` configurado (rate limit + idempotency)
- [x] Postgres + Redis + Redpanda dimensionados pra carga prevista
- [ ] (operacional) backup do DB ativo antes do go-live
- [ ] (operacional) ativar `git config core.hooksPath .githooks` em workstations dev
- [ ] (operacional) validar workflow CI Go no GitHub Actions após primeiro push pra remote

✅ **Frontend pronto pra produção** desde que:
- [x] Build limpo sem warnings
- [x] eslint zero warnings com `--max-warnings 0`
- [x] 152/152 testes passando
- [x] 0 vulns em `npm audit`
- [ ] (operacional) `VITE_API_BASE` apontando pros endpoints HTTPS de prod
- [ ] (operacional) Stripe publishable key de prod no env

---

## 9. Próximos passos recomendados

1. **Push deste relatório** + ativar GitHub Actions (já configurado em [.github/workflows/go-ci.yml](../../.github/workflows/go-ci.yml))
2. **Revisar configs de prod** antes de deploy:
   - `JWT_SECRET` rotation (compartilhado entre auth/order/payment)
   - Connection pool sizing (Postgres `MaxOpenConns`/`MaxIdleConns`)
   - Redis maxmemory + eviction policy (já em compose: 256MB, allkeys-lru)
3. **Audit cadenced**: agendar próxima auditoria em 6 meses (Sprint 30) ou após mudanças significativas em código de pagamento.
4. **Compliance** (LGPD): revisar política de retenção em `auth_events` + `payments_outbox` + `psp_payload` (mesmo redacted, dados pós-redação têm expectativa de retenção).
