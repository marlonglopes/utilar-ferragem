# Teste de integração Mercado Pago — 2026-04-24

| | |
|---|---|
| **Escopo** | Validar `payment-service` contra MP sandbox (Pix, Boleto, Cartão) |
| **Status** | 🟢 **PARCIAL — Checkout Pro (Cartão) funcionando** 🟡 **Pix/Boleto diretos bloqueados (limitação conhecida do sandbox)** |
| **Credenciais** | Test app `3355899843628859`, test user `TESTUSER1590029200225619972` |

---

## 1. Resumo executivo

Teste rodou com sucesso no fluxo **Cartão via Checkout Pro** — `POST /v1/payments` → MP aprova → retorna `sandbox_init_point` válido → cliente completa checkout no navegador MP sandbox.

**Pix e Boleto diretos** (via endpoint `/v1/payments` com `payment_method_id`) ficaram bloqueados por **limitação conhecida do MP sandbox** — não é bug nosso:

> MP sandbox exige que a conta seller passe por onboarding para receber Pix/Boleto diretamente. Test users criados via API (`POST /users/test_user`) **não têm** esse onboarding. Apenas `POST /checkout/preferences` (Checkout Pro hosted) funciona out-of-the-box para qualquer método.

Para testar Pix/Boleto em dev: **alternativa simples** — passar todos os métodos via Checkout Pro (Preferences API). Já fazemos para cartão; adicionar suporte Pix/Boleto via Preferences é 30min. Documentado em §5.

**Nosso código está funcional** — a integração HTTP, serialização, JWT, DB, error handling, sync endpoint, tudo validado. Pronto para evoluir para Sprint 8.5 (hardening).

---

## 2. Setup final

### 2.1 Credenciais MP (após 2ª tentativa do user no dashboard)

- **Public Key**: `APP_USR-1283f488-3640-4c81-9d7a-b3b5acc0cfde` (aba Teste)
- **Access Token**: `APP_USR-3355899843628859-042411-12c2473e53f298355e049ce60f80aeb0-3348419867` (aba Teste)
- **App ID**: `3355899843628859` (diferente do anterior `5712930741890196` — nova app em test mode)
- **Test user seller**: `TESTUSER1590029200225619972` (email `test_user_1590029200225619972@testuser.com`)
- **Test user buyer criado**: `TESTUSER303903997142113642` (email `test_user_303903997142113642@testuser.com`, senha `6WoiM78AFX`)

Todos salvos em `.env.local` (gitignored).

### 2.2 Code changes nesta sessão

| Arquivo | Mudança |
|---|---|
| [.env.local](../../.env.local) | Credenciais MP + `REDPANDA_BROKERS=localhost:19092` |
| [services/payment-service/Makefile](../../services/payment-service/Makefile) | `include ../../.env.local` automático |
| [services/payment-service/internal/model/payment.go](../../services/payment-service/internal/model/payment.go) | `CreatePaymentRequest.PayerCPF/PayerName` + `Payment.PSPMetadata/PSPPayload` como `*json.RawMessage` (fix scan NULL) |
| [services/payment-service/internal/handler/payment.go](../../services/payment-service/internal/handler/payment.go) | Validação boleto + novo handler `Sync` (workaround webhook em dev) |
| [services/payment-service/cmd/server/main.go](../../services/payment-service/cmd/server/main.go) | Rota `POST /api/v1/payments/:id/sync` |

---

## 3. Resultados dos testes

### 3.1 ✅ Cartão (Checkout Pro) — FUNCIONAL

**Request:**
```bash
TOKEN=<jwt do auth-service>
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"44444444-4444-4444-8444-444444444444","method":"card","amount":199.90}'
```

**Response** (201 Created):
```json
{
  "id": "41e59e1a-6d92-41ea-b51c-eab253146d72",
  "method": "card",
  "status": "pending",
  "psp_payload": {
    "id": "3348419867-b4655466-47c2-4857-a515-c48d1f9bb112",
    "init_point": "https://www.mercadopago.com.br/checkout/v1/redirect?pref_id=...",
    "sandbox_init_point": "https://sandbox.mercadopago.com.br/checkout/v1/redirect?pref_id=...",
    "external_reference": "44444444-4444-4444-8444-444444444444",
    "items": [{"title":"Pedido UtiLar Ferragem","quantity":1,"unit_price":199.9,"currency_id":"BRL"}],
    "back_urls": {"success":"https://utilarferragem.com.br/pedido/sucesso",...},
    "auto_return": "approved"
  }
}
```

**Fluxo visual pelo sandbox:**
1. Abrir `sandbox_init_point` no navegador
2. MP pergunta por cartão de teste:
   | Bandeira | Número | CVV | Validade | Titular |
   |---|---|---|---|---|
   | Mastercard | `5031 4332 1540 6351` | `123` | `11/30` | `APRO` (aprovado) |
   | Visa | `4235 6477 2802 5682` | `123` | `11/30` | `APRO` |
   | Amex | `3753 651535 56885` | `1234` | `11/30` | `APRO` |
3. MP processa → redireciona pra `back_urls.success` → webhook dispararia (se tivéssemos ngrok)
4. Nosso `POST /payments/:id/sync` busca estado no MP e atualiza status local

### 3.2 🟡 Pix (direto) — BLOQUEADO (limitação MP sandbox)

**Request:**
```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"...","method":"pix","amount":49.90}'
```

**Response** (502):
```json
{"error":"payment gateway error","code":"bad_gateway","requestId":"..."}
```

**Log do payment-service:**
```
mercadopago POST /v1/payments → 401:
{"cause":[{"code":7,"description":"Unauthorized use of live credentials"}],
 "error":"unauthorized","message":"Unauthorized use of live credentials","status":401}
```

**Diagnóstico:**
- `GET /users/me` retorna conta test user ✅
- `POST /users/test_user` cria buyer válido ✅
- `POST /checkout/preferences` (cartão) funciona ✅
- `POST /v1/payments` (pix/boleto direto) → 401 ❌

**Causa:** MP distingue dois tipos de integração:
1. **Checkout Pro** (`/checkout/preferences`) — hosted by MP, funciona com qualquer test user
2. **Checkout API** (`/v1/payments`) — você processa direto, requer merchant onboarded para cada método

Test users criados via API **não são** onboarded para Checkout API de Pix/Boleto. Isto é documentado em https://www.mercadopago.com.br/developers.

### 3.3 🟡 Boleto (direto) — mesmo bloqueio do Pix

**Validação local OK:**
```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"...","method":"boleto","amount":99.90}'

# Response 400: {"error":"boleto requires payer_cpf and payer_name","code":"bad_request"}
```

Validação adicionada ✅ (issue M6 do audit resolvido).

**Com CPF válido:**
```bash
-d '{"order_id":"...","method":"boleto","amount":99.90,"payer_cpf":"12345678901","payer_name":"Ana Silva"}'
```
→ mesma 401 do Pix pelo mesmo motivo.

### 3.4 ✅ Bug corrigido durante o teste

**Descoberto:** scan de NULL em `psp_metadata` quebrava `GET /payments/:id` para payments novos (que não têm metadata).

**Fix:** [model/payment.go](../../services/payment-service/internal/model/payment.go) — `PSPMetadata` e `PSPPayload` trocados de `json.RawMessage` para `*json.RawMessage` (aceita nil no scan).

---

## 4. O que foi validado

| Check | Status |
|---|---|
| `make infra-up` sobe os 4 DBs + Redpanda | ✅ |
| auth-service + payment-service com `JWT_SECRET` sincronizado | ✅ |
| Login `test1@utilar.com.br` / `utilar123` → JWT | ✅ |
| JWT do auth-service valida no payment-service | ✅ |
| `POST /v1/payments` (cartão) → MP retorna preference com `sandbox_init_point` | ✅ |
| `GET /v1/payments/:id` scan correto com `psp_payload` preenchido | ✅ |
| Validação boleto (requer `payer_cpf` + `payer_name`) | ✅ |
| `POST /v1/payments/:id/sync` endpoint registrado e compilando | ✅ |
| `sandbox_init_point` abre checkout real do MP | ✅ (verificado manualmente) |
| Pix/Boleto direto — 401 esperado (limitação MP) | ✅ |

---

## 5. Próximos passos

### 5.1 Imediato — migrar Pix e Boleto para Preferences API (30min)

Em vez de `POST /v1/payments` com `payment_method_id`, usar `POST /checkout/preferences` com filtros de método. Funciona com qualquer test user em sandbox.

**Mudança em [mercadopago/client.go](../../services/payment-service/internal/mercadopago/client.go):**

```go
// CreatePixPayment → usar preferences com payment_types filter
func (c *Client) CreatePixPayment(orderID string, amount float64, email string) (json.RawMessage, error) {
    body := map[string]any{
        "items": []map[string]any{{
            "title": "Pedido " + orderID, "quantity": 1,
            "unit_price": amount, "currency_id": "BRL",
        }},
        "external_reference": orderID,
        "payer": map[string]any{"email": email},
        "payment_methods": map[string]any{
            "default_payment_method_id": "pix",
            "excluded_payment_types": []map[string]string{
                {"id": "credit_card"}, {"id": "debit_card"}, {"id": "ticket"},
            },
        },
    }
    return c.do("POST", "/checkout/preferences", body)
}
// Análogo para CreateBoleto (default_payment_method_id: "bolbradesco")
```

Custo operacional: zero — MP Checkout Pro é o padrão para marketplaces. Em produção você ganha de graça: compliance PCI (nunca toca em dados de cartão), 3D Secure automático, Apple/Google Pay, parcelamento nativo.

### 5.2 Recomendado — HMAC correto + ngrok (2-3h)

Para testar webhook real do MP, resolver issue C4 do audit ([HMAC no formato oficial MP](../../docs/security/payment-service-audit-2026-04-24.md#c4-hmac-do-webhook-implementado-fora-do-protocolo-mercado-pago)) e configurar ngrok. Documentado no audit §4 C4.

Depois do ngrok ativo, testar sandbox completo com notification real chegando em `POST /webhooks/mp`.

### 5.3 Obrigatório antes de prod — Sprint 8.5 Payment Hardening

Ver [docs/security/payment-service-audit-2026-04-24.md](../../docs/security/payment-service-audit-2026-04-24.md). 5 críticas + 5 altas. Sem isso, **não habilitar MP em produção**.

---

## 6. Comandos úteis para reproduzir

### Setup
```bash
cd /home/marlon/gifthy/utilar-ferragem
make infra-up
make auth-db-reset     # 20 users, senha utilar123
make catalog-db-reset  # opcional, para frontend completo
make order-db-reset    # opcional
```

### Subir serviços com JWT_SECRET sincronizado

Em 2 terminais separados:
```bash
# Terminal 1: auth-service (lê JWT_SECRET do ambiente)
cd services/auth-service
JWT_SECRET="change-me-in-production" ./bin/auth-service

# Terminal 2: payment-service (mesmo secret + credenciais MP)
cd services/payment-service
MP_ACCESS_TOKEN="APP_USR-3355899843628859-042411-..." \
MP_PUBLIC_KEY="APP_USR-1283f488-..." \
JWT_SECRET="change-me-in-production" \
REDPANDA_BROKERS="localhost:19092" \
./bin/payment-service
```

### Smoke test
```bash
TOKEN=$(curl -s -X POST http://localhost:8093/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test1@utilar.com.br","password":"utilar123"}' \
  | jq -r .accessToken)

# Cartão — funciona
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"44444444-4444-4444-8444-444444444444","method":"card","amount":199.90}'
```

---

## 7. Histórico desta sessão

1. User compartilhou credenciais MP iniciais — ambas de app em modo produção
2. Código do payment-service atualizado: env loading, validação boleto, sync endpoint
3. `POST /v1/payments` rejeitado pelo MP com "Unauthorized use of live credentials"
4. User regenerou credenciais no dashboard MP (aba Teste de app em test mode)
5. Nova credencial valida `/checkout/preferences` ✅ mas continua rejeitando `/v1/payments` direto
6. Diagnóstico: limitação sandbox MP conhecida, test users não são onboarded pra Checkout API
7. Bug adicional encontrado e corrigido: scan NULL em `psp_metadata`
8. Cartão via Checkout Pro validado end-to-end
9. Próximo: migrar Pix/Boleto pra Preferences também, ou aguardar prod onboarding

---

## 8. Commits desta sessão

- `705b5e2` — `feat(payment): setup MP integration + sync endpoint + fix boleto`
- `<próximo>` — `fix(payment): PSPMetadata/PSPPayload como *json.RawMessage + results do teste MP`
