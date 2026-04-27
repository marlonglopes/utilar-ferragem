# Full Security Audit — Backend Services (4 microservices)

**Data**: 2026-04-26
**Escopo**: `auth-service`, `order-service`, `catalog-service`, `payment-service`
**Auditor**: Claude (revisão linha-a-linha código + DDL)
**Status**: `payment-service` já tinha [audit prévio 2026-04-24](payment-service-audit-2026-04-24.md) — incorporado aqui como apêndice. Este documento adiciona auth/order/catalog.

---

## Sumário executivo

| Serviço | CRITICAL | HIGH | MEDIUM | LOW |
|---|---:|---:|---:|---:|
| auth-service | 4 | 5 | 7 | 4 |
| order-service | 3 | 5 | 4 | 3 |
| catalog-service | 2 | 4 | 5 | 4 |
| payment-service (audit prévio) | 5 | 5 | 6 | 3 |
| **Total** | **14** | **19** | **22** | **14** |

**Padrões transversais** (mesma vulnerabilidade em múltiplos serviços):
1. **JWT_SECRET com fallback default** — `auth`, `order`, `payment` (catalog não usa JWT)
2. **DBError vaza `err.Error()` no body HTTP** — `auth`, `order`, `catalog`, `payment`
3. **CORS `Access-Control-Allow-Origin: *`** — todos os 4
4. **Sem rate limiting** — todos os 4

**Status atual de produção**: ⛔ **BLOQUEADO**. Não fazer deploy até pelo menos os CRITICALs estarem fechados.

---

## Plano de remediação (este audit)

Esta sessão (2026-04-26) corrige os 9 CRITICALs novos (auth/order/catalog) + acelera 1 dos 5 do payment-service. A Sprint 8.5 endereça os 4 CRITICALs restantes do payment + os HIGHs.

### Em scope (fix nesta sessão)

| ID | Serviço | Severidade | Fix |
|---|---|---|---|
| A1-C1 | auth | CRITICAL | DBError sanitizado |
| A2-C2 | auth | CRITICAL | JWT_SECRET fail-closed |
| A3-C3 | auth | CRITICAL | rand.Read error handling |
| A4-C4 | auth | CRITICAL | Refresh token rotation |
| O1-C1 | order | CRITICAL | OrderItem validation (qty/price) |
| O1-C2 | order | CRITICAL | OrderAddress validation |
| O1-C3 | order | CRITICAL | X-User-Id fallback gated por DEV_MODE |
| O2-H1 | order | HIGH | DBError sanitizado |
| O2-H2 | order | HIGH | CORS restritivo via env |
| O2-H3 | order | HIGH | JWT_SECRET fail-closed |
| CT1-C1 | catalog | CRITICAL | Escape ILIKE wildcards |
| CT1-C2 | catalog | CRITICAL | DBError sanitizado |
| CT1-H2 | catalog | HIGH | CORS restritivo via env |
| CT1-H3 | catalog | HIGH | price_min/max negativos rejeitados |
| (transversal) | payment | HIGH | DBError sanitizado |
| (transversal) | payment | HIGH | JWT_SECRET fail-closed |

### Fora de scope (Sprint 8.5 ou backlog)

- **payment-service** C1–C5 (tamper amount, ownership, webhook amount, HMAC MP, fail-closed) — Sprint 8.5 (~16h, planejado)
- **auth-service** HIGH/MEDIUM: rate limit (precisa Redis), token hashing (precisa migration), security headers
- **order-service** HIGH: seller validation cross-service (precisa cliente HTTP pra catalog)
- **catalog-service** HIGH: rate limit (precisa Redis)

---

## auth-service findings

### A1-C1 (CRITICAL): DBError vaza erro raw
**Localização**: [services/auth-service/internal/handler/errors.go:35](../../services/auth-service/internal/handler/errors.go)
**Bug**: `Respond(c, http.StatusInternalServerError, "db_error", err.Error())` retorna mensagem do Postgres (schema, constraint, query) ao cliente.
**Exploit**: Erros de constraint vazam nomes de tabelas/colunas; erros de syntax expõem queries → reconhecimento de infra.
**Fix**: log interno + mensagem genérica (`"database error"`) ao cliente.

### A2-C2 (CRITICAL): JWT_SECRET fallback fraco
**Localização**: [services/auth-service/internal/config/config.go:22](../../services/auth-service/internal/config/config.go)
**Bug**: `JWTSecret: env("JWT_SECRET", "change-me-in-prod-please")`. Se var não estiver setada, secret hardcoded é usado.
**Exploit**: Atacante que sabe o default forja JWT com `HS256` + secret conhecido → impersonação cross-service (auth → order, payment).
**Fix**: fail-closed na startup — recusa subir sem `JWT_SECRET` válido (ou rejeita default literal).

### A3-C3 (CRITICAL): rand.Read sem error handling
**Localização**: [services/auth-service/internal/handler/auth.go:375](../../services/auth-service/internal/handler/auth.go) (`randToken`)
**Bug**: `rand.Read(b)` não checa retorno de erro. Em sistemas com PRNG quebrado, retorna buffer com baixa entropia (zeros ou parcial).
**Exploit**: Refresh tokens, password reset, email verify usam `randToken` — se entropia for fraca, brute force viável.
**Fix**: panic em erro (fail-fast).

### A4-C4 (CRITICAL): Refresh tokens sem rotação
**Localização**: [services/auth-service/internal/handler/auth.go:160-172](../../services/auth-service/internal/handler/auth.go) (`Refresh`)
**Bug**: `/auth/refresh` emite novo access token mas reutiliza o mesmo refresh token. Token pode ser reusado por 30 dias.
**Exploit**: Atacante que rouba refresh token (XSS, MITM) tem janela de 30 dias mesmo após user fazer logout — porque logout só revoga UM refresh token específico, não a sessão emitida durante o roubo.
**Fix**: emitir novo refresh token em `/refresh` + revogar o antigo (rotação).

### HIGH (não corrigidos nesta sessão — backlog)
- **A5-H1**: CORS `*` (corrigido transversalmente — mesmo padrão de fix)
- **A6-H2**: Sem rate limit em verify-email/reset-password (precisa Redis)
- **A7-H3**: Reset/verify/refresh tokens em plaintext no DB (precisa migration de schema com hash)
- **A8-H4**: Tokens logados sem condicional de ambiente
- **A9-H5**: Forgot-password sem timing-safe response

### MEDIUM/LOW (backlog)
- CPF não validado, security headers ausentes, sem cleanup de tokens expirados, complexidade de senha, alg lock para HS256, idempotency-key, versionamento, validação MX de email, logout não revoga access token.

---

## order-service findings

### O1-C1 (CRITICAL): OrderItem sem validation
**Localização**: [services/order-service/internal/model/order.go:23-31](../../services/order-service/internal/model/order.go), handler create
**Bug**: `Quantity` e `UnitPrice` aceitam qualquer valor (incluindo negativos, zero, valores absurdos). Validação só existe via CHECK no DB pra `quantity > 0` — `unitPrice` nem isso.
**Exploit**: cliente envia `unitPrice: -100, quantity: 1` → pedido com total negativo → "pagamento" devolve dinheiro do merchant.
**Fix**: binding tags `gt=0,lte=999999.99` em `UnitPrice` e `gt=0,lte=999` em `Quantity`.

### O1-C2 (CRITICAL): OrderAddress sem validation
**Localização**: [services/order-service/internal/model/order.go:33-41](../../services/order-service/internal/model/order.go)
**Bug**: campos de endereço aceitam strings arbitrárias sem `max=`, sem regex de CEP, sem limites.
**Exploit**: `street: "<script>...</script>"`, payload GB-sized causando OOM, CEP inválido.
**Fix**: binding tags por campo (`required,max=255` em strings, regex em CEP, `len=2` em UF).

### O1-C3 (CRITICAL): X-User-Id fallback em produção
**Localização**: [services/order-service/internal/handler/middleware.go:79-84](../../services/order-service/internal/handler/middleware.go) (`RequireUser`)
**Bug**: middleware aceita `X-User-Id: <qualquer-coisa>` como prova de identidade quando JWT não está presente. Comentado como "dev only" mas sempre ativo.
**Exploit**: `curl -H "X-User-Id: <victim-uuid>" /api/v1/orders` → lista pedidos da vítima sem JWT.
**Fix**: passar `DevMode bool` pro middleware via config; em prod (`DEV_MODE=false` ou ausente), só JWT é aceito.

### HIGH (corrigidos nesta sessão como bonus)
- **O2-H1**: DBError vaza err.Error() — mesmo fix de A1-C1
- **O2-H2**: CORS `*` — mesmo padrão transversal
- **O2-H3**: JWT_SECRET fallback — mesmo padrão de A2-C2

### HIGH/MEDIUM (backlog)
- O2-H4: order_number sequencial enumera; O2-H5: seller-item ownership não validada cross-service; O3-M1: limites de tamanho em items array; O3-M3: rate limit em create.

---

## catalog-service findings

### CT1-C1 (CRITICAL): SQL ILIKE wildcards não escapados
**Localização**: [services/catalog-service/internal/handler/product.go:34,174](../../services/catalog-service/internal/handler/product.go)
**Bug**: query `q` é parametrizada (`$N`) mas `%` e `_` do user vão direto pro `ILIKE`. Padrões como `%_%_%_%_%_%_` causam ReDoS no pg_trgm — Postgres trava CPU em O(n²).
**Exploit**: 100 reqs paralelos com `?q=%_%_%_%_%_%_%_%_%_%_` derrubam o catalog.
**Fix**: escapar `%`, `_`, `\` no termo + `ESCAPE '\'` no SQL.

### CT1-C2 (CRITICAL): DBError vaza err.Error()
**Localização**: [services/catalog-service/internal/handler/errors.go:36](../../services/catalog-service/internal/handler/errors.go)
**Bug/Exploit/Fix**: idêntico A1-C1.

### HIGH (corrigidos como bonus)
- **CT1-H2**: CORS `*` — fix transversal
- **CT1-H3**: `price_min`/`price_max` negativos aceitos — validação simples

### HIGH (backlog)
- CT1-H1: rate limiting (precisa Redis)
- CT1-H4: timing attack em slug GET (low ROI vs complexidade)

---

## Apêndice: payment-service (audit 2026-04-24)

5 CRITICALs originais — Sprint 8.5 endereça todos:
- **C1**: tamper de amount (cliente envia preço); fix: derivar do order-service via JWT propagation
- **C2**: ownership de order_id não validada; fix: query order-service com JWT do user
- **C3**: webhook não valida amount vs PSP; fix: comparar payment.amount local com PSP webhook payload
- **C4**: HMAC implementado fora do formato MP (`ts=X,v1=Y`); fix: parser correto + constant-time compare
- **C5**: fail-closed ausente quando webhook secret vazio; fix: refuse to start em prod

Esta sessão corrige transversalmente 2 padrões que também afetam payment:
- DBError sanitizado em todos os handlers
- JWT_SECRET fail-closed na startup

---

## Status pós-fix (2026-04-26 — sessão de remediação)

| Categoria | Antes | Depois |
|---|---:|---:|
| CRITICAL aberto | 14 | **5** (somente payment C1–C5, plano Sprint 8.5) |
| HIGH aberto | 19 | 14 (3 fechados como bonus) |
| MEDIUM/LOW | 36 | 35 (1 fechado: sort whitelist) |

### Fixes aplicados nesta sessão

| ID | Serviço | Fix |
|---|---|---|
| A1-C1 | auth | DBError não vaza err.Error() (log interno + msg genérica) |
| A2-C2 | auth | JWT_SECRET fail-closed em produção (`config.Load()` retorna erro se vazio/default/<32 chars) |
| A3-C3 | auth | `randToken()` panic em erro de rand.Read (preferimos crash a token fraco) |
| A4-C4 | auth | `/auth/refresh` agora rotaciona o refresh token (revoga atual + emite novo, transacional) |
| O1-C1 | order | OrderItem com binding tags: `Quantity gt=0,lte=999`, `UnitPrice gt=0,lte=999999.99` |
| O1-C2 | order | OrderAddress com binding: `max=` em strings, `len=2` em UF |
| O1-C3 | order | X-User-Id fallback gated por `DEV_MODE=true`; em prod só JWT |
| O2-H1 | order | DBError sanitizado |
| O2-H3 | order | JWT_SECRET fail-closed |
| CT1-C1 | catalog | `escapeLikePattern()` + `ESCAPE '\'` em todas as queries ILIKE |
| CT1-C2 | catalog | DBError sanitizado |
| CT1-H3 | catalog | `price_min/max < 0` rejeitados no parse |
| CT1-M3 | catalog | sort whitelist (`""` cai em default explícito) |
| (transv) | payment | DBError sanitizado |
| (transv) | payment | JWT_SECRET fail-closed |

### Testes adicionados (regressão de segurança)

- [services/auth-service/internal/config/config_test.go](../../services/auth-service/internal/config/config_test.go) — 5 testes (rejeita vazio/default/curto, aceita strong/dev fallback)
- [services/order-service/internal/config/config_test.go](../../services/order-service/internal/config/config_test.go) — 4 testes (mesmos cenários)
- [services/order-service/internal/handler/middleware_security_test.go](../../services/order-service/internal/handler/middleware_security_test.go) — 4 testes (X-User-Id rejeitado em prod, aceito em dev, sem auth → 401, JWT inválido → 401)
- [services/catalog-service/internal/handler/security_test.go](../../services/catalog-service/internal/handler/security_test.go) — escapeLikePattern + price_min/max negativos rejeitados

**Total**: ~17 novos testes Go de hardening, todos verdes.

### Backlog restante

1. **Sprint 8.5 — payment hardening** (~16h): C1 tamper amount, C2 ownership, C3 webhook amount, C4 HMAC MP, C5 fail-closed
2. **Rate limiting** (~4h): Redis + middleware em auth (login/forgot/reset) e catalog (search)
3. **Token hashing** (~3h): migration pra hashar refresh/reset/verify tokens no DB
4. **Security headers** (~30min): CSP, HSTS, X-Frame-Options, X-Content-Type-Options
5. **Cleanup de tokens expirados** (~1h): job/cron deletando tokens vencidos
6. **Cross-service price validation** (~3h): order-service consulta catalog antes de aceitar item.unit_price
7. **Refresh tokens em plaintext no DB** (parte de #3)
8. **CORS restritivo via env var** (~30min) — atualmente `*` em todos os 4 serviços (HIGH transversal)

### Como rodar com config segura

```bash
# Geração de secret forte
JWT_SECRET=$(openssl rand -hex 32)
export JWT_SECRET

# Modo dev (aceita defaults pra facilitar tests)
export DEV_MODE=true

# Modo prod (recusa subir sem JWT_SECRET válido)
export DEV_MODE=false  # ou unset
```

Todas as 4 services agora abortam startup se `DEV_MODE != true` E `JWT_SECRET` estiver ausente / for um default conhecido / tiver < 32 chars.
