# Security Roadmap — pós Sprint 8.5 Fase 1

**Última atualização**: 2026-04-27

Documento vivo do trabalho de segurança restante. CRITICALs estão todos fechados; este doc rastreia HIGHs e MEDIUMs por ordem de impacto + esforço.

---

## Status atual (snapshot)

| Categoria | Aberto | Fechado |
|---|---:|---:|
| CRITICAL | **0** ✅ | 14 (audit completo + Sprint 8.5 Fase 1) |
| HIGH | 12 | 7 |
| MEDIUM | 18 | 4 |
| LOW | 14 | 0 |

Ver detalhes em:
- [full-audit-2026-04-26.md](full-audit-2026-04-26.md) — auth/order/catalog
- [payment-service-audit-2026-04-24.md](payment-service-audit-2026-04-24.md) — payment

---

## HIGH ainda em aberto (12)

Ordenados por impacto. Esforço total: **~22h**.

### 1. Rate limiting (~6h, precisa Redis)

Atinge: payment, auth, catalog.

- **payment H4** — `/payments`: 10/min/user (anti order-bomb)
- **auth A6-H2** — `/verify-email`, `/reset-password`, `/login`, `/forgot-password`: 5/min/IP
- **catalog CT1-H1** — `/products` (especialmente search com `q`): 100/min/IP

Implementação: middleware `RateLimit(reqs, window)` baseado em token bucket no Redis (`go-redis/redis/v9`). Chave: `rl:{endpoint}:{user_id_or_ip}`.

### 2. Idempotency-Key em `POST /payments` (~3h)

Audit **payment H1**. Cliente que retransmite (network blip) cria pagamento duplicado e cobra 2x.

Fix: middleware lê header `Idempotency-Key`, faz hash, busca em Redis. Se hit → retorna response cached. Se miss → processa + salva response com TTL 24h.

### 3. Token hashing pra refresh/reset/verify (~3h, precisa migration)

Audit **auth A7-H3, M3, M4**. Tokens em plaintext no DB → se DB vaza, atacante reseta qualquer conta / impersona qualquer sessão.

Fix:
1. Migration: adicionar coluna `token_hash` em `refresh_tokens`, `password_reset_tokens`, `email_verification_tokens`. Backfill com SHA-256 dos tokens existentes. Drop coluna `token` em migration separada (rollback-safe).
2. Handlers: hash do token antes de SELECT/UPDATE. Constant-time compare.

### 4. Cross-service price validation (~3h, order→catalog)

Audit **order O2-H5**. Hoje cliente envia `unitPrice` no body do `POST /orders` — backend confia. Atacante pode forjar pedido com `unitPrice: 0.01`.

Fix: order-service passa a chamar catalog-service via `GET /products/:id` (com cache) e usa `product.price` autoritativo. Padrão idêntico ao orderclient do payment.

### 5. Order number não-sequencial (~1h)

Audit **order O2-H4**. `2026-12345` é enumerável, vaza volume de pedidos.

Fix: trocar por `2026-` + 8 chars random base32 (`crypto/rand`). Mantém prefix de ano pra debugging.

### 6. Forgot-password timing-safe (~1h)

Audit **auth A9-H5**. Endpoint diferente em latência se email existe vs não → enumeration.

Fix: adicionar jitter artificial fixo (~50ms) com `time.Sleep` quando email não encontrado.

### 7. CORS whitelist em produção (~30min, **fechado em 2026-04-27**)

✅ **Concluído**: 4 services agora aceitam `ALLOWED_ORIGINS=https://a.com,https://b.com`. Vazio = wildcard (dev).

### 8. Security headers transversais (~30min, **fechado em 2026-04-27**)

✅ **Concluído**: `SecurityHeaders()` middleware em 4 services. Adiciona CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy.

### 9. Tokens logados condicional ao DEV_MODE (auth, ~15min, **fechado em 2026-04-27**)

✅ **Concluído**: `slog.Info("...token...")` em `Register`/`ForgotPassword` agora gated por `cfg.DevMode`.

### 10. JWT alg lock pra HS256 exato (~15min)

Audit **auth A16-M7**. Atualmente aceita qualquer HMAC. Se library default mudar, pode aceitar HS512 com chave HS256 → confusão de algorítmos.

Fix: validar `t.Method.Alg() == "HS256"` em vez de só `*jwt.SigningMethodHMAC`.

### 11. JWT claims tipadas no payment (~1h)

Audit **payment H2**. Hoje payment usa `jwt.MapClaims` cru — possível bug se claims mudarem no auth.

Fix: extrair `auth.Claims` struct do auth-service pra um pacote shared (ou redefinir no payment).

### 12. MaxBytesReader em endpoints (~30min)

Audit **payment H5**. Adicionar `io.LimitReader` em todos os `c.Request.Body` reads.

Fix: middleware genérico `BodyLimit(64*1024)` que envolve `c.Request.Body` antes do bind. Já está aplicado no webhook do payment.

---

## MEDIUM em aberto (~18)

Resumo (impacto menor, fila orgânica):

- **auth A10-M1** — validação de CPF (algoritmo de check digit) — 30min
- **auth A12-M3** — email verification tokens em plaintext — engloba HIGH #3
- **auth A14-M5** — cleanup automático de tokens expirados via job/cron — 1h
- **auth A15-M6** — complexidade mínima de senha (10 chars min, blacklist top-passwords) — 1h
- **catalog CT1-M1** — limite no número de filtros simultâneos — 30min
- **catalog CT1-M4** — RequestID via UUID v4 (precisa ULID lib ou `github.com/google/uuid`) — 1h
- **catalog CT1-M5, L1** — CHECK constraints no schema (`stock >= 0`, `price >= 0`) — 30min (migration)
- **order O3-M3** — rate limit em create order — engloba HIGH #1
- **order O3-M4** — pessimistic locking em cancel pra evitar TOCTOU — 1h
- **payment M2** — redactor PII em `psp_payload` — 2h
- **payment M5** — redactor PII em logs de acesso — 1h
- **payment M6** — buscar CPF do auth-service pro boleto — 30min (depois do auth client)
- **(transversal) M11** — request_id via ULID em todos os 4 services — 1h

---

## LOW em aberto (14)

Backlog orgânico, sem urgência:

- audit logging tables, slug enumeration, govulncheck no CI, circuit breaker, SAST (gosec), dependabot, container scanning, etc.

---

## Como abrir issues no GitHub

Sugiro 4 issues principais (HIGHs agrupados por tema):

1. **Rate limiting + Idempotency-Key** (Sprint 8.5 Fase 2 — esforço ~9h, precisa Redis)
2. **Token hashing migration** (audit A7-H3 — esforço ~3h, precisa migration deploy)
3. **Cross-service price validation** (audit O2-H5 — esforço ~3h)
4. **Forgot-password timing + JWT alg lock + claims tipadas** (HIGH miscellaneous — ~3h)

E 1 issue agrupando MEDIUMs como "Hardening operacional Sprint 22".

Comando sugerido:

```bash
gh issue create --title "Security: rate limiting + idempotency-key" \
  --body "$(cat docs/security/security-roadmap.md | head -50)" \
  --label security,sprint-8.5
```

---

## Critério pra reabrir produção

Já cumprido (CRITICAL block clear) — **payment-service tecnicamente desbloqueado** em 2026-04-27.

Para tráfego significativo (> 10 reqs/min de pagamento real), recomenda-se também HIGH #1 (rate limit) + HIGH #2 (idempotency) antes do go-live.
