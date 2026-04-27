# Security Roadmap — pós Sprint 8.5 Fase 1

**Última atualização**: 2026-04-27

Documento vivo do trabalho de segurança restante. CRITICALs estão todos fechados; este doc rastreia HIGHs e MEDIUMs por ordem de impacto + esforço, e organiza os HIGHs em **4 bundles de trabalho** prontos pra execução em sequência.

---

## Status atual (snapshot)

| Categoria | Aberto | Fechado |
|---|---:|---:|
| CRITICAL | **0** ✅ | 14 (audit completo + Sprint 8.5 Fase 1) |
| HIGH | 11 | 8 |
| MEDIUM | 18 | 4 |
| LOW | 14 | 0 |

Ver detalhes em:
- [full-audit-2026-04-26.md](full-audit-2026-04-26.md) — auth/order/catalog
- [payment-service-audit-2026-04-24.md](payment-service-audit-2026-04-24.md) — payment

---

## Plano de execução — Sprint 8.5 Fase 2 (~19h, 2.5 dias)

Os 11 HIGHs em aberto agrupados em 4 bundles pelo eixo dependência/coesão. Cada bundle pode ser um PR independente, na ordem proposta abaixo (de menor pra maior risco de regressão).

### Bundle 1 — Quick wins isolados (~3h, sem dependências)

Pequenos fixes pontuais, baixo risco, alto sinal-pra-ruído. Fazer primeiro pra reduzir surface de ataque enquanto bundles maiores são planejados.

| ID | Serviço | Issue | Esforço | Fix |
|---|---|---|---:|---|
| **O2-H4** | order | Order number sequencial enumerável | 1h | `crypto/rand` 8 chars base32 em vez de `UnixNano()%100000` (mantém prefix de ano) |
| **CT1-H4** | catalog | Slug GET timing attack | 1h | Pad fixo de ~50ms via `time.Sleep(50ms - elapsed)` na resposta |
| **A9-H5** | auth | Forgot-password timing-safe | 30min | Jitter artificial fixo quando email não encontrado |
| **H2** | payment | JWT claims tipadas | 1h | Extrair `auth.Claims` struct pra pacote shared ou redefinir local; trocar `jwt.MapClaims` cru por struct tipada |
| **H5** | payment | MaxBytesReader em `POST /payments` | 30min | `c.Request.Body = http.MaxBytesReader(...)` antes do bind (já feito no webhook) |

**Validação**: testes unitários novos pra cada (timing-safe verifiable; entropy de order number; etc).

### Bundle 2 — Rate limiting + Idempotency-Key via Redis (~9h)

Maior PR. Adiciona dependência **Redis** ao projeto. Resolve 4 HIGHs num só pacote (todos compartilham infra).

**Setup (~2h)**:
- Adicionar `redis` ao `docker-compose.yml`
- `go get github.com/redis/go-redis/v9`
- Helper `internal/ratelimit/limiter.go` (token bucket no Redis, key=`rl:{endpoint}:{key}`)
- Helper `internal/idempotency/store.go` (hash → response com TTL 24h)
- Config: `REDIS_URL` env var em `auth/order/catalog/payment`

**Aplicação dos middlewares (~7h)**:

| ID | Serviço | Endpoint | Limite | Esforço |
|---|---|---|---|---:|
| **A6-H2** | auth | `/auth/login` | 5/min/IP | 30min |
| | auth | `/auth/forgot-password` | 5/min/IP | 30min |
| | auth | `/auth/reset-password` | 5/min/IP | 30min |
| | auth | `/auth/verify-email` | 10/min/IP | 30min |
| **CT1-H1** | catalog | `/products` (search) | 100/min/IP | 1h |
| **H4** | payment | `/payments` | 10/min/user | 1h |
| **H1** | payment | `/payments` | `Idempotency-Key` middleware (separado do rate limit) | 3h |

**Validação**:
- Tests integration: 11ª req em janela retorna 429
- Tests integration: segundo POST `/payments` com mesma `Idempotency-Key` retorna response cached do primeiro
- Smoke test fim-a-fim com Redis rodando

### Bundle 3 — Cross-service price validation (~3h)

Resolve **O2-H5**. Aproveita o fato de que agora o order-service precisa chamar catalog (segundo caller cross-service depois do payment→order) — bom momento pra **extrair** `internal/orderclient/` do payment pra `pkg/serviceclient/` shared.

**Etapas**:
1. (~1h) Extrair `payment-service/internal/orderclient/` → `pkg/serviceclient/order/` (shared) e `payment-service/internal/orderclient/` vira thin wrapper
2. (~1h) Criar `pkg/serviceclient/catalog/` espelhando o pattern (`Get(ctx, productID, jwt)`) — sem JWT propagation aqui (catalog é público)
3. (~1h) `OrderHandler.Create` consulta catalog antes de aceitar `unitPrice` do body; usa `product.price` autoritativo. Logs warning se diverge. Mesma pattern do payment vs order

**Validação**: testes integration que tentam tampering com `unitPrice` e verificam que o pedido fica com preço autoritativo do catalog.

### Bundle 4 — Token hashing migration (~3h)

Resolve **A7-H3** (e MEDIUMs A12, A13 por consequência). Maior risco operacional dos 4 bundles porque envolve migration + backfill + zero-downtime deploy.

**Etapas**:

1. (~30min) Migration `add_token_hash`:
   ```sql
   ALTER TABLE refresh_tokens ADD COLUMN token_hash TEXT;
   ALTER TABLE password_reset_tokens ADD COLUMN token_hash TEXT;
   ALTER TABLE email_verification_tokens ADD COLUMN token_hash TEXT;
   CREATE UNIQUE INDEX idx_refresh_token_hash ON refresh_tokens(token_hash) WHERE token_hash IS NOT NULL;
   -- idem outras
   ```

2. (~30min) Backfill SQL:
   ```sql
   UPDATE refresh_tokens SET token_hash = encode(digest(token, 'sha256'), 'base64') WHERE token_hash IS NULL;
   ```
   (Requer extension `pgcrypto`. Se não tiver, fazer backfill em Go.)

3. (~1h30min) Handlers:
   - `Refresh`/`Logout`: hash o token recebido e compara com `token_hash` (substitui SELECT atual por `WHERE token_hash = $1`)
   - `VerifyEmail`/`ResetPassword`: idem
   - Constant-time compare via `subtle.ConstantTimeCompare`

4. (~30min) Migration de drop da coluna `token` (em PR separado, após confirmar que nada lê `token` direto):
   ```sql
   ALTER TABLE refresh_tokens DROP COLUMN token;
   ```

**Validação**:
- Testes unitários do hashing (constant-time)
- Testes integration: registrar → login → /refresh com token hashado funciona
- Backup do DB antes do deploy de prod (rollback path conhecido)

---

## Ordem de execução recomendada

1. **Bundle 1** (~3h) — Quick wins. Sem risco de regressão, fecha 5 HIGHs imediato.
2. **Bundle 2** (~9h) — Redis. Maior PR mas autocontido. Exige `docker-compose up redis` em dev.
3. **Bundle 3** (~3h) — Cross-service price. Refactor do orderclient + cliente novo pro catalog.
4. **Bundle 4** (~3h) — Token hashing. Última coisa porque é a mais arriscada (DB migration).

**Total**: ~19h. Distribuição sugerida: 2.5 dias dedicados, ou 4–5 sessões de ~4h.

Após Bundle 4, **payment-service e auth-service estão em estado de produção saudável**: 0 CRITICAL, 0 HIGH abertos. Restam apenas MEDIUM/LOW (hardening orgânico, sem urgência).

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
