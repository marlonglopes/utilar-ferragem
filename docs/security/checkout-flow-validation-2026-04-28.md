# Checkout Flow — End-to-End Validation

**Data**: 2026-04-28
**Tipo**: Continuação do [verification-2026-04-27](verification-2026-04-27.md) cobrindo o fluxo de pagamento end-to-end (não estava no scope do anterior).
**Resultado**: ✅ Card e Boleto validados com Stripe Test mode real; Pix bloqueado por config da conta Stripe.

---

## Sumário das mudanças desde 27/04

3 commits novos no flow do checkout:

| Commit | Resumo |
|---|---|
| `f032d91` | fix(checkout): integra POST /orders ao fluxo + auto-refresh de JWT + UX dos 3 métodos |
| `97a4738` | fix(boleto): redactor não masca número público + recupera boleto na confirmação |
| `9e489bf` | test(checkout): vitest verifica renderização do boleto na confirmação |

### Bug crítico encontrado e corrigido

**Antes**: `CheckoutPage` gerava um UUID via `crypto.randomUUID()` no client e mandava direto pro `POST /payments`. payment-service consultava order-service e recebia 404 → "order not found". O fluxo nunca chamava `POST /orders`. Boleto/Pix/Card **nunca completavam end-to-end**.

**Depois**: `ensureOrderId(method)` faz `POST /api/v1/orders` real, captura `id` + `number`, e cacheia em PaymentStep. O cache é invalidado quando items/endereço/método mudam (evita order com paymentMethod errado no DB).

### Bugs UX adicionais corrigidos

| ID | Bug | Fix |
|---|---|---|
| **A** | Trocar método após order criada → order tinha `paymentMethod` errado | `committedOrderId` reseta quando `method` muda |
| **B** | Pre-fill CPF/Nome não re-sync se user mudar (login/logout) durante checkout | useEffect re-sincroniza com user store |
| **isCreating** | Trocar de método mid-flight travava botão | Filtra por método atual |
| **B1 (Card UX)** | Stripe Elements só aparecia após click "Pagar" | useEffect cria PaymentIntent automaticamente ao selecionar Cartão |
| **B2 (Pix QR)** | `<img>` prefixava `data:image/png;base64,` em URL HTTP da Stripe → broken render | Detecta URL vs base64 |
| **B3 (Status)** | OrderConfirmation sempre mostrava "pending" — callback do polling era no-op | `useState<PaymentStatus>` reativo |
| **B4 (Card status)** | `requires_action`/`requires_confirmation`/etc caíam silenciosamente | switch completo de status Stripe |
| **U1 (Boleto)** | CPF/Nome inputs vazios mesmo com user logado | Pre-fill de `useAuthStore.user.cpf/name` |
| **U5** | OrderConfirmation mostrava UUID do payment como "número do pedido" | orderNumber humano (`2026-XYZ`) via location.state |
| **JWT 401** | Token JWT expirava em 15min e SPA não fazia refresh | `fetchWithAutoRefresh` em api.ts: 401 → POST /auth/refresh → retry |
| **M2 redactor** | Linha digitável do boleto era mascarada como "***REDACTED***" | Removida key `number` genérica; mantidas keys explícitas (`cardnumber`, `cvv`, etc.) |
| **Boleto recovery** | Após fechar a aba, user perdia o boleto | OrderConfirmation faz GET /payments/:id e re-renderiza BoletoPayment inline |

---

## Validação E2E das 3 formas de pagamento

### 1. Card (Stripe Test mode) — ✅ FUNCIONAL

```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"order_id":"<uuid>","method":"card","amount":329}'
```

Response:
```json
{
  "id": "...",
  "method": "card",
  "status": "pending",
  "provider": "stripe",
  "clientSecret": "pi_3TQzEjLQCtijFcSY..._secret_...",
  "psp_id": "pi_3TQzEjLQCtijFcSY..."
}
```

SPA: Stripe Elements monta automaticamente, user preenche cartão, `stripe.confirmPayment(clientSecret)`, status confirmed via `markConfirmed`.

**Teste de cartões** (Stripe Test mode):
- `4242 4242 4242 4242` → aprovado
- `4000 0000 0000 0002` → recusado
- `4000 0025 0000 3155` → 3DS challenge

### 2. Boleto (Stripe Test mode) — ✅ FUNCIONAL

```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"order_id":"<uuid>","method":"boleto","amount":329,"payer_cpf":"52998224725","payer_name":"Ana Silva"}'
```

Response inclui:
- `next_action.boleto_display_details.number` — linha digitável (visível, não mascarada)
- `next_action.boleto_display_details.pdf` — URL do PDF
- `next_action.boleto_display_details.hosted_voucher_url` — página HTML do boleto

SPA renderiza `BoletoPayment` inline com botões "Visualizar boleto" + "Baixar PDF".

**CPF**: precisa ser válido (Stripe valida check digit em test mode). O seed da Ana foi atualizado pra `52998224725` (válido).

### 3. Pix (Stripe Test mode) — 🟡 BLOQUEADO POR CONFIG DA CONTA

```
ERROR psp: upstream error: {"code":"payment_intent_invalid_parameter",
"message":"The payment method type 'pix' is invalid. Please ensure the
provided type is activated in your dashboard..."}
```

**Causa**: a conta `acct_1TQFpiLQCtijFcSY` não tem Pix habilitado. Não é bug do código.

**Pra ativar**:
- Stripe Dashboard → Settings → Payments → Payment methods → Pix → Enable
- Em alguns países requer onboarding completo BR

**Alternativa em dev**: trocar `PSP_PROVIDER=mercadopago` com credenciais MP. Code path do Pix está implementado (testado em unit tests via `parseStripeResult`).

---

## Suite de testes — estado atual

### Go (race detector ativo): **238 PASS, 0 FAIL, 0 SKIP**

| Módulo | Tests |
|---|---:|
| pkg (httpclient + idempotency + ratelimit + requestid) | 16 |
| auth-service | 47 |
| catalog-service | 41 |
| order-service | 31 |
| payment-service | 103 (era 102, +1 por boleto preservation no redact_test) |

### Frontend (vitest): **154 PASS** em 19 test files

| Test file | Cobertura |
|---|---|
| OrderConfirmationPage.test.tsx | 10 (era 8, +2: boleto fetch + render, no-fetch em mock mode) |
| BoletoPayment / CardPayment / CheckoutPage / PixPayment / etc. | 144 |

### Comandos de validação

```bash
# Backend
for s in pkg services/auth-service services/catalog-service services/order-service services/payment-service; do
  (cd $s && go test -race -count=1 ./...)
done

# Frontend
cd app && npm run lint && npm test -- --run && npm run build
```

---

## Setup pra reproduzir local

```bash
# Infra
make infra-up
make auth-db-reset && make catalog-db-reset && make order-db-reset && make db-reset

# Pegar Stripe Test secret real (sk_test_...) do dashboard:
# https://dashboard.stripe.com/test/apikeys

# Atualizar CPF da Ana pra um válido (seed tem invalid):
docker exec utilar_auth_db psql -U utilar -d auth_service -c \
  "UPDATE users SET cpf='52998224725' WHERE email='test1@utilar.com.br'"

# Subir backend (4 services)
export DEV_MODE=true
export REDIS_URL=redis://localhost:6379
export JWT_SECRET="dev-secret-with-32-chars-minimum-zzz"
export PSP_PROVIDER=stripe
export STRIPE_SECRET_KEY="sk_test_REAL_KEY_HERE"
export STRIPE_WEBHOOK_SECRET=whsec_dummy_for_dev_testing_only
export ALLOWED_ORIGINS=http://localhost:5173,http://localhost:5176
make dev-full

# Login
# Email: test1@utilar.com.br
# Senha: utilar123
```

---

## Observações pra produção

| Item | Recomendação |
|---|---|
| Pix | Provider primário = Mercado Pago (Stripe BR Pix tem onboarding extenso) |
| Card | Stripe é mais simples e estável |
| Boleto | Stripe ou MP — ambos suportados |
| CPF do user | Validação A10-M1 garante check-digit válido em novos registros |
| JWT TTL | Access 15min + refresh 30 dias com rotação a cada `/refresh` |
| Refresh tokens revogados | Cleanup auto a cada 1h (A14-M5) — após dias offline, user precisa re-login |
| Webhooks em dev | `stripe listen --forward-to localhost:8090/webhooks/stripe` pra receber confirmação real de boleto/pix em test mode |

---

## Status de produção

✅ **Backend e frontend prontos** desde que:
- [x] Todas as auditorias de segurança fechadas (65 findings — verification-2026-04-27)
- [x] Card e Boleto end-to-end testados via Stripe Test mode
- [x] 392 tests passing (238 Go + 154 frontend)
- [x] Lint clean, build clean, gosec/govulncheck clean
- [ ] (operacional) Stripe live key + Pix habilitado OU integração MP pra Pix
- [ ] (operacional) Stripe webhook configurado em prod (HTTPS endpoint público)
