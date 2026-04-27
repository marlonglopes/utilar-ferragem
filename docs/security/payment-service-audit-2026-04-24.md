# Auditoria de segurança — payment-service

| | |
|---|---|
| **Data** | 2026-04-24 |
| **Escopo** | `services/payment-service/` (commit `a3a860a`) — handlers, middlewares, webhook, outbox, cliente Mercado Pago, migrations |
| **Revisor** | Claude Opus 4.7 (1M context) + validação conjunta com fundador |
| **Motivação** | Payment-service é o core financeiro do ecossistema; antes de habilitar MP em prod ou alto volume sandbox, blindar contra fraude |
| **Status** | 🟡 **CRITICAL fechados em 2026-04-27** — payment-service tecnicamente desbloqueado pra prod; HIGH/MEDIUM/LOW pendentes (ver §8) |
| **Próximo passo** | Sprint 8.5 — Hardening operacional (Idempotency-Key, rate limit, CORS whitelist, etc) |
| **Status fix CRITICALs** | C1 ✅ \| C2 ✅ \| C3 ✅ \| C4 ✅ \| C5 ✅ — ver [§8 status atualizado](#status-da-remediação-2026-04-27) |

---

## 1. Sumário executivo

**Não habilitar Mercado Pago em produção com este código.** A revisão identificou **5 vulnerabilidades críticas** que permitem fraude direta (manipulação de preço, confirmação forjada, confusão de ownership entre usuários). As principais causas:

1. O serviço **confia no cliente** para informar o `amount` e o `order_id`, sem verificar contra o order-service que armazena a verdade.
2. O webhook handler **não valida o amount** retornado pelo Mercado Pago contra o armazenado no banco local.
3. O formato do HMAC do webhook **não segue o protocolo real do MP** — em produção, webhooks válidos serão rejeitados e tentativas de bypass podem passar sem replay protection.
4. Ausência de fail-closed quando secrets críticos (`MP_WEBHOOK_SECRET`, `JWT_SECRET`) não estão configurados — um deploy esquecido derruba toda a segurança.

**Boas notícias:** a arquitetura está correta (transactional outbox implementado, JWT HS256 com middleware centralizado, SQL parametrizado, idempotência do webhook via unique constraint). As vulnerabilidades são **consertáveis sem redesenho** — cerca de 2 dias de trabalho (Sprint 8.5).

**Riscos residuais após Sprint 8.5:** ainda será necessário rate limiting, redação de PII em logs (Sprint 22), e rotação automática de secrets (infra/IaC).

---

## 2. Metodologia

Revisão manual linha a linha dos arquivos:

- [cmd/server/main.go](../../services/payment-service/cmd/server/main.go)
- [internal/config/config.go](../../services/payment-service/internal/config/config.go)
- [internal/handler/payment.go](../../services/payment-service/internal/handler/payment.go)
- [internal/handler/webhook.go](../../services/payment-service/internal/handler/webhook.go)
- [internal/handler/auth_middleware.go](../../services/payment-service/internal/handler/auth_middleware.go)
- [internal/handler/middleware.go](../../services/payment-service/internal/handler/middleware.go)
- [internal/handler/errors.go](../../services/payment-service/internal/handler/errors.go)
- [internal/mercadopago/client.go](../../services/payment-service/internal/mercadopago/client.go)
- [internal/outbox/drainer.go](../../services/payment-service/internal/outbox/drainer.go)
- [internal/model/payment.go](../../services/payment-service/internal/model/payment.go)
- [migrations/*.sql](../../services/payment-service/migrations/)

**Categorias avaliadas:**

- Autenticação e autorização (JWT, user scoping)
- Validação de entrada (tamper, type safety)
- Verificação de assinatura (HMAC do webhook)
- Injection (SQL, log forging)
- Race conditions (concorrência payment ↔ webhook)
- Idempotência e replay protection
- Transactional guarantees (outbox)
- Tratamento de secrets (defaults, fail-closed)
- Information disclosure (error messages, logs, PII)
- DoS (rate limiting, body size)
- CORS
- Dependências

**Não avaliado (fora do escopo deste audit):**

- Segurança de rede/TLS (responsabilidade do CloudFront/ALB em prod)
- Segurança do cliente MP SDK (é código externo auditado pela MP)
- Segurança do outbox drainer → Redpanda (sem PII; eventos idempotentes)

---

## 3. Classificação de severidade

| Sev | Definição | Ação requerida |
|---|---|---|
| 🔴 **CRITICAL** | Permite fraude financeira direta ou comprometimento total. Exploit trivial. | **Bloqueia produção.** Corrigir antes de qualquer deploy prod. |
| 🟠 **HIGH** | Ataque viável com alguma complexidade; impacto significativo (perda financeira, DoS, disclosure). | Corrigir antes de habilitar tráfego real (mesmo sandbox com dados reais). |
| 🟡 **MEDIUM** | Impacto limitado ou requer pré-condições específicas. | Corrigir antes do launch oficial. |
| 🟢 **LOW** | Code smell, observabilidade, boas práticas. | Backlog; abordar quando tocar no código relacionado. |

---

## 4. 🔴 Vulnerabilidades críticas

### C1. Tamper de `amount` — cliente controla o preço

| | |
|---|---|
| **Severidade** | 🔴 CRITICAL |
| **Tipo** | Broken Access Control (OWASP A01) / Business Logic Flaw |
| **Local** | [payment.go:44](../../services/payment-service/internal/handler/payment.go#L44) |
| **Esforço** | Médio (3-4h) |

**Descrição:**
O handler `Create` aceita `amount` diretamente do payload do cliente e o passa (a) para o `INSERT` da tabela `payments` e (b) para a chamada ao Mercado Pago.

```go
// req.Amount é 100% controlado pelo cliente
INSERT INTO payments (order_id, user_id, method, amount) VALUES ($1, $2, $3, $4)
...
h.mp.CreatePixPayment(req.OrderID, req.Amount, userEmail)
```

**Exploit:**
```bash
JWT=<token válido>
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"order_id":"<uuid-de-pedido-de-R$-2000>","method":"pix","amount":0.01}'
```

Resultado: o pedido de R$ 2.000,00 é pago por R$ 0,01. MP processa o Pix válido, webhook confirma (sem validar valor — ver C3), sistema marca pedido como pago. **Fraude direta.**

**Remediação:**
1. Remover `Amount` de `CreatePaymentRequest` (ou ignorá-lo).
2. Antes de chamar MP, fazer `GET order-service/api/v1/orders/:order_id` propagando o JWT do cliente.
3. Usar `order.Total` (que o order-service calculou no momento da criação do pedido) como o `amount` real.
4. O order-service **já valida** `user_id` no `WHERE` — se o pedido não é do user, retorna 404.

**Impacto secundário:** essa mudança também resolve C2 (ownership) gratuitamente.

**Validação:**
- Teste: criar pedido no order-service (total R$ 500), tentar POST payment com `amount: 1` → deve ser ignorado, cobrar R$ 500
- Teste: POST payment com `order_id` de outro usuário → 404 (order-service nega)
- Teste: POST payment sem `order_id` → 400

---

### C2. Ownership do `order_id` não validado

| | |
|---|---|
| **Severidade** | 🔴 CRITICAL |
| **Tipo** | Insecure Direct Object Reference (OWASP A01) |
| **Local** | [payment.go:44](../../services/payment-service/internal/handler/payment.go#L44) |
| **Esforço** | Incluído em C1 |

**Descrição:**
Usuário autenticado A pode criar pagamento para `order_id` do usuário B. O insert em `payments` usa o `user_id` de A (do JWT), mas o `order_id` é aceito sem verificação.

**Exploit:**
- Usuário A descobre (via vazamento de logs, teste, força bruta de UUID) o `order_id` do usuário B.
- A paga o pedido de B com próprio Pix.
- Consequências: confusão contábil, B recebe produto pago por A (lavagem), disputa se B contestar.

**Remediação:**
Resolvido pela mesma mudança de C1: se `GET /orders/:id` com o JWT de A devolve 404 para pedido de B, a chamada falha antes de criar payment.

**Defense-in-depth:** incluir `user_id` no GET `/api/v1/orders/:id` já está feito — order-service filtra `WHERE user_id = $1` ([order-service/internal/handler/order.go:226-228](../../services/order-service/internal/handler/order.go#L226-L228)).

---

### C3. Webhook confirma pagamento sem validar `amount`

| | |
|---|---|
| **Severidade** | 🔴 CRITICAL |
| **Tipo** | Broken Access Control + Insufficient Verification |
| **Local** | [webhook.go:106-115](../../services/payment-service/internal/handler/webhook.go#L106-L115) |
| **Esforço** | Médio (2h) |

**Descrição:**
Ao receber webhook `payment.updated`, o handler faz:

```go
UPDATE payments SET status='confirmed', confirmed_at=... WHERE psp_payment_id = $2
```

Não há verificação de que o **valor pago** no MP bate com o `amount` armazenado. Se o HMAC vazar (C4/C5) ou MP for comprometido, atacante pode forjar webhook confirmando R$ 10.000 para um payment de R$ 10.

**Exploit (requer HMAC comprometido ou ausente):**
```bash
# atacante conhece payment_id de uma compra alheia
curl -X POST https://prod/webhooks/mp \
  -H "Content-Type: application/json" \
  -d '{"type":"payment","action":"payment.updated","data":{"id":"12345"}}'
# Se HMAC passa (C5) → payment confirmed sem pagamento real
```

**Remediação:**
Após idempotency check, antes do UPDATE:

```go
// 1. Fetch real state from MP
mpState, err := h.mp.GetPayment(pspPaymentID)
if err != nil { ... }

// 2. Parse mpState.status e mpState.transaction_amount
var mpResp struct {
    Status            string  `json:"status"`
    TransactionAmount float64 `json:"transaction_amount"`
}
json.Unmarshal(mpState, &mpResp)

// 3. Confirma apenas se MP diz approved AND amount bate
var localAmount float64
err = tx.QueryRow(`SELECT amount FROM payments WHERE psp_payment_id=$1`, pspPaymentID).Scan(&localAmount)
if err == sql.ErrNoRows { return tx.Commit() }  // já tratado
if mpResp.Status != "approved" {
    slog.Warn("webhook: mp status not approved", "status", mpResp.Status)
    return tx.Commit()
}
if math.Abs(mpResp.TransactionAmount - localAmount) > 0.01 {
    slog.Error("webhook: amount mismatch — POSSIBLE FRAUD",
        "local", localAmount, "mp", mpResp.TransactionAmount, "psp_id", pspPaymentID)
    return tx.Commit()  // NÃO confirma
}
// agora sim, UPDATE status='confirmed'
```

**Validação:**
- Teste: criar payment R$ 100 → webhook forjado dizendo R$ 1 → payment NÃO confirmado, log de fraude emitido
- Teste: amount bate → payment confirmed normalmente
- Teste: MP retorna `status: rejected` → payment NÃO confirmed

---

### C4. HMAC do webhook implementado fora do protocolo Mercado Pago

| | |
|---|---|
| **Severidade** | 🔴 CRITICAL |
| **Tipo** | Protocol Violation + Missing Replay Protection |
| **Local** | [webhook.go:48-55](../../services/payment-service/internal/handler/webhook.go#L48-L55), [webhook.go:150-155](../../services/payment-service/internal/handler/webhook.go#L150-L155) |
| **Esforço** | Médio (3h, inclui testes) |

**Descrição:**
A implementação atual:

```go
sig := c.GetHeader("x-signature")
// compara hex cru do HMAC-SHA256(body, secret) com o header inteiro
hmac.Equal([]byte(expectedHex), []byte(signature))
```

**Problema:** o MP envia `x-signature` no formato:
```
x-signature: ts=1704472800,v1=abc123def456...
```

E o HMAC é calculado sobre um **template específico**, não sobre o body cru:
```
template = "id:<data.id>;request-id:<x-request-id>;ts:<ts>;"
v1 = HMAC-SHA256(template, secret)
```

**Consequências em produção:**

1. **Webhooks legítimos do MP serão rejeitados** (falha funcional total).
2. Se um atacante enviar só o hex direto no header (ignorando `ts=`), o código atual `hmac.Equal(expected, signature)` falha → 401. OK.
3. **Mas:** sem `ts` validado, não há replay protection. Se HMAC correto vazar uma vez, é replayable indefinidamente.

**Remediação (implementação correta):**

```go
func verifyMPSignature(c *gin.Context, body []byte, secret string) bool {
    sigHeader := c.GetHeader("x-signature")
    requestID := c.GetHeader("x-request-id")
    dataID := c.Query("data.id")  // MP também envia no query string

    // Parse "ts=<ts>,v1=<hex>"
    var ts, v1 string
    for _, part := range strings.Split(sigHeader, ",") {
        kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
        if len(kv) != 2 { continue }
        switch kv[0] {
        case "ts": ts = kv[1]
        case "v1": v1 = kv[1]
        }
    }
    if ts == "" || v1 == "" || requestID == "" || dataID == "" { return false }

    // Replay protection: rejeitar se > 5 min
    tsInt, err := strconv.ParseInt(ts, 10, 64)
    if err != nil { return false }
    if time.Since(time.Unix(tsInt/1000, 0)) > 5*time.Minute { return false }

    // Template oficial do MP
    template := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, ts)
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(template))
    expected := hex.EncodeToString(mac.Sum(nil))

    return hmac.Equal([]byte(expected), []byte(v1))
}
```

**Referência oficial:** [MP Webhooks — Validar origem da notificação](https://www.mercadopago.com.br/developers/pt/docs/your-integrations/notifications/webhooks#bookmark_valida%C3%A7%C3%A3o_da_origem_da_notifica%C3%A7%C3%A3o)

**Validação:**
- Teste: webhook real do MP (via ngrok em sandbox) → passa
- Teste: webhook com `ts` de 1h atrás → rejeitado (replay protection)
- Teste: webhook com `v1` adulterado → rejeitado
- Teste: webhook sem `x-request-id` → rejeitado

---

### C5. Fail-closed ausente quando `MP_WEBHOOK_SECRET` não configurado

| | |
|---|---|
| **Severidade** | 🔴 CRITICAL (produção) / LOW (dev) |
| **Tipo** | Security Misconfiguration (OWASP A05) |
| **Local** | [webhook.go:48](../../services/payment-service/internal/handler/webhook.go#L48) |
| **Esforço** | Trivial (30min) |

**Descrição:**
```go
if h.webhookSecret != "" {
    // validate HMAC
}
// se secret é "", validação é skipada → qualquer um pode forjar webhook
```

**Cenário:** deploy em produção esquece de setar `MP_WEBHOOK_SECRET`. Serviço sobe sem erros. Atacante descobre e envia webhooks forjados confirmando qualquer pagamento. **Desastre silencioso.**

**Remediação:**
1. Em `config.Load()`, detectar ambiente prod:
   ```go
   if env("APP_ENV", "development") == "production" && cfg.MPWebhookSecret == "" {
       return nil, errors.New("MP_WEBHOOK_SECRET is required in production")
   }
   ```
2. O server não sobe → healthcheck falha → deploy é revertido automaticamente.
3. Adicionar mesma proteção para `JWT_SECRET == "change-me"` em prod.

**Validação:**
- Teste: `APP_ENV=production` sem `MP_WEBHOOK_SECRET` → `os.Exit(1)` no startup
- Teste: `APP_ENV=development` sem secret → warning log, server sobe (dev ok)
- Teste: secret configurado → server sobe normalmente

---

## 5. 🟠 Vulnerabilidades altas

### H1. `Create` não é idempotente — double charge em retry

| | |
|---|---|
| **Severidade** | 🟠 HIGH |
| **Local** | [payment.go:24-91](../../services/payment-service/internal/handler/payment.go#L24-L91) |
| **Esforço** | Médio (3h) |

**Cenário:** cliente envia `POST /payments`, resposta perde por timeout (rede, browser, tab fechada). Cliente re-envia. Resultado: **2 payments criados, 2 charges no MP, 2 QR codes**. Usuário paga um, o outro expira como `failed`. Reconciliação chato, disputa possível.

**Remediação:**
1. Requerer header `Idempotency-Key: <uuid>` no request (ou gerar do lado do cliente).
2. Tabela nova `payment_idempotency_keys (key TEXT PRIMARY KEY, user_id TEXT, payment_id UUID, response JSONB, expires_at TIMESTAMPTZ)`.
3. Na primeira chamada: insert com `ON CONFLICT DO NOTHING RETURNING key`; se conflito, retornar response guardado.
4. TTL de 24h na chave.

---

### H2. JWT claims não validados por tipo

| | |
|---|---|
| **Severidade** | 🟠 HIGH |
| **Local** | [auth_middleware.go:37-38](../../services/payment-service/internal/handler/auth_middleware.go#L37-L38) |
| **Esforço** | Pequeno (1h) |

**Descrição:**
```go
c.Set("user_id", claims["sub"])     // interface{}, pode ser nil, number, etc
c.Set("user_email", claims["email"]) // idem
```

Se um JWT mal-formado (sem `sub` ou com `sub: 123` número) passa pela validação de assinatura, o handler `Create` chama `c.GetString("user_id")` → retorna `""` → cai no check `if userID == ""`. Funciona **por acidente**.

**Remediação:**
Reusar a struct `Claims` do auth-service ([auth-service/internal/auth/jwt.go:10-15](../../services/auth-service/internal/auth/jwt.go#L10-L15)):

```go
import "github.com/utilar/auth-service/internal/auth"  // ou copiar

claims := &auth.Claims{}
token, err := jwt.ParseWithClaims(tokenStr, claims, ...)
if err != nil || !token.Valid { ... 401 }
if _, err := uuid.Parse(claims.UserID); err != nil { ... 401 }
c.Set("user_id", claims.UserID)
```

---

### H3. CORS `Origin: *` com credenciais

| | |
|---|---|
| **Severidade** | 🟠 HIGH (produção) |
| **Local** | [middleware.go:46](../../services/payment-service/internal/handler/middleware.go#L46) |
| **Esforço** | Pequeno (1h) |

**Descrição:**
```go
c.Header("Access-Control-Allow-Origin", "*")
```

Em combinação com `Authorization: Bearer`, qualquer site (mobile app, script, iframe malicioso) pode fazer requisições contra o API se obtiver o JWT (via XSS no frontend, extensão maliciosa, phishing).

**Remediação (config-driven):**
```go
// em config.Load()
AllowedOrigins: strings.Split(env("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5176"), ",")

// em CORS middleware
origin := c.GetHeader("Origin")
for _, allowed := range cfg.AllowedOrigins {
    if origin == allowed {
        c.Header("Access-Control-Allow-Origin", origin)
        break
    }
}
```

**Produção:** `ALLOWED_ORIGINS=https://utilarferragem.com.br,https://www.utilarferragem.com.br`.

---

### H4. Sem rate limiting em `POST /api/v1/payments`

| | |
|---|---|
| **Severidade** | 🟠 HIGH |
| **Local** | [main.go:63-67](../../services/payment-service/cmd/server/main.go#L63-L67) |
| **Esforço** | Médio (2-3h) |

**Descrição:**
Usuário com JWT válido (próprio ou roubado) pode criar milhares de payments em loop → esgota quota MP, gera custos, polui DB.

**Remediação:**
Middleware de rate limit por user_id:
- `POST /payments`: 10/min/user, 100/hora/user
- Webhook: 1000/min por IP (dev) ou skip em prod (MP tem IPs conhecidos)

Bibliotecas: `github.com/ulule/limiter/v3` (token bucket, Redis-backed) ou `github.com/didip/tollbooth/v7`. Em prod, usar Redis (ElastiCache) para compartilhar estado entre instâncias.

---

### H5. Webhook sem size limit no body

| | |
|---|---|
| **Severidade** | 🟠 HIGH |
| **Local** | [webhook.go:41](../../services/payment-service/internal/handler/webhook.go#L41) |
| **Esforço** | Trivial (15min) |

**Descrição:**
```go
rawBody, err := io.ReadAll(c.Request.Body)
```

Sem limite. Atacante envia body de 1 GB → OOM no processo.

**Remediação:**
```go
// antes de ReadAll
c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024) // 64KB é mais que suficiente
```

---

## 6. 🟡 Vulnerabilidades médias

| ID | Título | Local | Esforço |
|---|---|---|---|
| **M1** | Subquery do fallback no webhook é código morto com bug lógico | [webhook.go:108-115](../../services/payment-service/internal/handler/webhook.go#L108-L115) | 1h |
| **M2** | `psp_payload` persistido sem redação de dados do cartão (last4, bandeira) | [payment.go:82-83](../../services/payment-service/internal/handler/payment.go#L82-L83) | 2h |
| **M3** | `err.Error()` retornado ao cliente vaza internals do DB | [errors.go:31](../../services/payment-service/internal/handler/errors.go#L31) | 30min |
| **M4** | `JWT_SECRET` default fraco (`"change-me"`) sem fail-closed em prod | [config.go:35](../../services/payment-service/internal/config/config.go#L35) | 30min (mesmo patch de C5) |
| **M5** | Logs de acesso emitem paths com IDs — necessita redactor de PII em prod | [middleware.go:32-39](../../services/payment-service/internal/handler/middleware.go#L32-L39) | Sprint 22 |
| **M6** | Boleto chamado com `cpf=""` e `name=""` — MP rejeita em prod | [payment.go:59](../../services/payment-service/internal/handler/payment.go#L59) | 2h (buscar do auth-service) |

### Detalhe M1 (bug, não security, mas deve ser corrigido)

```sql
WHERE psp_payment_id = $2 OR (psp_payment_id IS NULL AND order_id IN (
    SELECT order_id FROM payments WHERE psp_payment_id = $2 LIMIT 1
))
```

Se a primeira condição falha (`psp_payment_id != $2` ou NULL), a subquery tenta achar o mesmo `psp_payment_id` — que por definição não existe. Subquery sempre retorna vazio. **Código morto.** Remover ou substituir por busca via `external_reference` (que o MP devolve nos webhooks de preference/boleto).

### Detalhe M2 (PCI/compliance)

MP retorna em `/v1/payments/:id` campos como `card.last_four_digits`, `card.first_six_digits`, `card.cardholder.name`. Salvar isso em `psp_payload` JSONB é permitido pelo PCI-DSS (não é PAN completo, não é CVV), mas por cuidado deve-se:
- Redactor que aplica `redact_card_data(mpRaw)` antes do INSERT
- Manter apenas `payment_method`, `last_four`, `bin6`, `issuer` num campo estruturado separado
- Descartar `cardholder.name` e dados de billing se presentes

---

## 7. 🟢 Vulnerabilidades baixas

| ID | Título | Notas |
|---|---|---|
| **L1** | `newRequestID()` usa só `time.UnixNano()` — colide em bursts (mesmo nano) | Observability only, trocar por ULID ou UUID v4 na Sprint 22 |
| **L2** | Sem circuit breaker / timeout config no client MP | Se MP fica lento, requests empilham até saturar connection pool. Backlog. |
| **L3** | Dependências sem scan automatizado de CVE | Adicionar `govulncheck` no CI na Sprint 23 |

---

## Status da remediação (2026-04-27)

Todos os 5 CRITICALs originais foram fechados. Detalhes:

| Issue | Status | Commit/PR | Localização do fix |
|---|---|---|---|
| **C1** Tamper de amount | ✅ Fechado | 2026-04-27 | [payment.go](../../services/payment-service/internal/handler/payment.go) — `authoritativeAmount` vem do order-service; body amount é apenas hint (logado se diverge). [orderclient/](../../services/payment-service/internal/orderclient/) implementa o fetch com JWT propagation. |
| **C2** Ownership de order_id | ✅ Fechado | 2026-04-27 | order-service já filtra por user_id — payment chama `orderClient.Get` e recebe 404 se não-dono. Defesa em profundidade: `order.UserID == jwt.sub` revalidado no payment. |
| **C3** Webhook não valida amount | ✅ Fechado | 2026-04-27 | [webhook.go](../../services/payment-service/internal/handler/webhook.go) — `processEvent` chama `gateway.GetPayment(pspID)` antes de promover status. Amount mismatch → flag `psp_metadata.amount_mismatch=true` + outbox event `payment.fraud_suspect` + status fica em pending pra revisão. |
| **C4** HMAC MP fora do protocolo | ✅ Fechado | 2026-04-27 | [psp/mercadopago/gateway.go](../../services/payment-service/internal/psp/mercadopago/gateway.go) — `VerifyWebhook` parsa `ts=X,v1=Y`, monta manifest `id:<data.id>;request-id:<x-request-id>;ts:<ts>;`, HMAC-SHA256 + constant-time compare. Replay window de 5min. Stripe já estava OK via SDK oficial. |
| **C5** Fail-closed ausente | ✅ Fechado | 2026-04-27 | [config/config.go](../../services/payment-service/internal/config/config.go) — `Load()` recusa subir em prod se `STRIPE_WEBHOOK_SECRET` (PSP=stripe) ou `MP_WEBHOOK_SECRET` (PSP=mp) estiver vazio. Mesma proteção pro `JWT_SECRET` (audit transversal — fechado em 2026-04-26). |

**Testes adicionados** (28 novos):
- [internal/config/config_test.go](../../services/payment-service/internal/config/config_test.go) — 8 testes de fail-closed do JWT + webhook secrets
- [internal/handler/webhook_test.go](../../services/payment-service/internal/handler/webhook_test.go) — 4 integration tests: idempotency, invalid signature, provider mismatch, **amount mismatch rejecting confirmation** (C3)
- [internal/handler/payment_security_test.go](../../services/payment-service/internal/handler/payment_security_test.go) — 7 testes: amount tamper bloqueado, order not found = 404, user mismatch = 404, already-paid = 400, missing bearer = 401, dev/prod sem orderClient
- [internal/orderclient/client_test.go](../../services/payment-service/internal/orderclient/client_test.go) — 7 testes: success, 404, 401, 5xx, empty inputs, JWT propagation
- [internal/psp/mercadopago/webhook_test.go](../../services/payment-service/internal/psp/mercadopago/webhook_test.go) — 23 subtests: V2 format, legacy resource format, missing/bad/tampered signature, replay (old/future ts), malformed header, dataID extraction

Total payment-service: ~50 testes Go, **todos verdes**.

**Nota**: o `payment-service` está agora tecnicamente desbloqueado para deploy em prod no que diz respeito aos CRITICALs originais. As HIGH (Idempotency-Key, rate limit, CORS whitelist, etc.) ainda recomendam-se antes de tráfego significativo — ver §8 abaixo.

---

## 8. Plano de remediação — Sprint 8.5 (Payment Hardening)

**Objetivo:** desbloquear produção do payment-service. Esforço estimado: **~2 dias** (16h).

### Fase 1 — Bloqueadores de produção (dia 1, ~8h) — ✅ CONCLUÍDA 2026-04-27

| Task | Issue | Esforço | Status |
|---|---|---|---|
| 1.1 Propagar JWT do cliente no HTTP call ao order-service para validar order + pegar total | C1, C2 | 3h | ✅ |
| 1.2 Validar amount do webhook contra PSP via `GetPayment` | C3 | 2h | ✅ |
| 1.3 Implementar HMAC do MP com formato `ts=X,v1=Y` + replay window | C4 | 2h | ✅ |
| 1.4 Fail-closed em config para webhook secrets + `JWT_SECRET` em prod | C5, M4 | 30min | ✅ |
| 1.5 Testes de integração para C1-C5 | — | 30min | ✅ (50+ testes adicionados) |

### Fase 2 — Hardening operacional (dia 2, ~8h)

| Task | Issue | Esforço |
|---|---|---|
| 2.1 `Idempotency-Key` em `POST /payments` | H1 | 3h |
| 2.2 JWT claims tipadas (reusar struct do auth-service) | H2 | 1h |
| 2.3 CORS whitelist config-driven | H3 | 1h |
| 2.4 Rate limit em `/payments` | H4 | 2h |
| 2.5 `MaxBytesReader` no webhook | H5 | 15min |
| 2.6 Error responses sem leak de DB internals | M3 | 30min |
| 2.7 Buscar CPF do auth-service para boleto | M6 | 30min (pode usar mesma chamada de 1.1) |

### Pós-sprint 8.5 (backlog Sprint 22)

- M2 — redactor PII em psp_payload
- M5 — redactor PII em logs de acesso
- L1 — ULID em request ID
- L2 — circuit breaker
- L3 — govulncheck no CI

---

## 9. Critérios de aceite para reabrir produção

Para o payment-service ser aprovado para deploy em produção, **todos** os itens abaixo devem estar ✅:

- [ ] C1 — Amount vem exclusivamente do order-service; teste de tamper com amount diferente falha (retorna erro, não cobra)
- [ ] C2 — Ownership de order_id validado via order-service com JWT propagado; teste cross-user retorna 404
- [ ] C3 — Webhook faz `GetPayment(pspID)` e compara `transaction_amount` com DB; teste de amount forjado não confirma + emite log de fraude
- [ ] C4 — HMAC implementado no formato oficial MP (`ts=X,v1=Y`); teste com webhook real em sandbox via ngrok passa; teste com `ts` antigo é rejeitado
- [ ] C5 — `APP_ENV=production` sem `MP_WEBHOOK_SECRET` ou com `JWT_SECRET=change-me` falha o startup
- [ ] H1 — `Idempotency-Key` aceito; segundo POST com mesma chave retorna resultado original
- [ ] H2 — JWT sem `sub` string UUID é rejeitado com 401
- [ ] H3 — `ALLOWED_ORIGINS` configurado; origem não-whitelisted é rejeitada no preflight
- [ ] H4 — Rate limit 10/min/user funcionando; 11ª req retorna 429
- [ ] H5 — Body > 64KB retorna 413
- [ ] Todos os testes Go passando (incluindo novos para cada fix)
- [ ] `govulncheck ./...` sem findings críticos
- [ ] `.env.local` nunca commitado (confirmado em `.gitignore`)
- [ ] Review final do código com owner (Marlon)

---

## 10. Referências

- [OWASP API Security Top 10 (2023)](https://owasp.org/www-project-api-security/)
- [OWASP Cheat Sheet — Authorization](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
- [MP Webhooks — formato oficial](https://www.mercadopago.com.br/developers/pt/docs/your-integrations/notifications/webhooks)
- [MP Pix API](https://www.mercadopago.com.br/developers/pt/docs/checkout-api/integration-configuration/integrate-with-pix)
- [Transactional Outbox Pattern (microservices.io)](https://microservices.io/patterns/data/transactional-outbox.html)
- [RFC 7519 — JSON Web Tokens](https://datatracker.ietf.org/doc/html/rfc7519)
- [docs/08-security.md](../08-security.md) — política geral de segurança do projeto
- [docs/12-ops-runbook.md](../12-ops-runbook.md) — runbook de incidentes

---

## 11. Histórico de revisões

| Data | Autor | Mudança |
|---|---|---|
| 2026-04-24 | Claude + Marlon | Auditoria inicial; 5 críticas, 5 altas, 6 médias, 3 baixas identificadas |
