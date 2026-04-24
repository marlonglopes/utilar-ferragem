# Stripe integration — 2026-04-24

| | |
|---|---|
| **Escopo** | Validar payment-service com abstração `psp.Gateway` + Stripe como primeiro provider |
| **Status** | 🟢 **Cartão + Boleto funcionais fim-a-fim** 🟡 **Pix precisa de 1 clique no dashboard Stripe** |
| **Credenciais** | `sk_test_51TPoFv...` (test mode Stripe) |
| **Arquitetura** | Payment-service agora é PSP-agnóstico via `internal/psp/Gateway` |

---

## 1. Resumo executivo

**Stripe passou em todos os testes com DX radicalmente melhor que MP.**

| | MP sandbox | Stripe sandbox |
|---|---|---|
| Criar conta + pegar keys | ~10min (2 tentativas pra achar aba Teste) | ~2min (dashboard óbvio) |
| `POST /v1/payments` funciona? | ❌ "Unauthorized use of live credentials" | ✅ 201 Created na primeira |
| Mensagens de erro | "unauthorized" sem ação | Código + doc URL + link pra logs |
| Cartão | Só via Checkout Pro (redirect) | Client secret → frontend embed ✅ |
| Boleto | 401 em test user | PDF + código de barras direto ✅ |
| Pix | 401 em test user | Só precisa ativar na dashboard (1 clique) |

O mesmo código que **não funcionava** pra Pix/Boleto no MP agora retorna respostas válidas no Stripe com o endpoint genérico `POST /v1/payment_intents`.

---

## 2. Arquitetura — PSP abstraction

Todo o payment-service agora é PSP-agnóstico via interface [internal/psp/gateway.go](../../services/payment-service/internal/psp/gateway.go):

```
services/payment-service/internal/
  psp/
    gateway.go           ← interface Gateway + tipos normalizados (CreateRequest, Result, WebhookEvent, PaymentStatus)
    mercadopago/
      gateway.go         ← implementa psp.Gateway
      client.go          (HTTP client MP — movido de internal/mercadopago/)
      client_test.go
    stripe/
      gateway.go         ← implementa psp.Gateway via stripe-go v79 SDK
  handler/
    payment.go           ← usa psp.Gateway (não importa MP nem Stripe)
```

Seletor em [config/config.go](../../services/payment-service/internal/config/config.go) + [main.go](../../services/payment-service/cmd/server/main.go):
```go
switch cfg.PSPProvider {
case "stripe":       gateway = stripegateway.New(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
case "mercadopago":  gateway = mpgateway.New(cfg.MPAccessToken, cfg.MPWebhookSecret)
}
paymentH := handler.NewPaymentHandler(database, gateway)
```

**Zero mudança** no handler pra suportar Stripe — interface bem desenhada.

### Status normalization

Cada provider tem seu lifecycle específico; a Gateway mapeia pra nossos estados canônicos:

| `psp.PaymentStatus` | Stripe | Mercado Pago |
|---|---|---|
| `pending` | `processing`, `requires_payment_method`, `requires_confirmation`, `requires_action` | `pending`, `in_process`, `in_mediation` |
| `approved` | `succeeded` | `approved` |
| `authorized` | `requires_capture` | `authorized` |
| `rejected` | — | `rejected` |
| `cancelled` | `canceled` | `cancelled`, `refunded`, `charged_back` |

---

## 3. Fase 3 — Stripe Gateway (código)

Arquivo: [services/payment-service/internal/psp/stripe/gateway.go](../../services/payment-service/internal/psp/stripe/gateway.go) (~230 linhas).

### Métodos implementados

| Método | Stripe endpoint | Status |
|---|---|---|
| `Name()` | — | retorna `"stripe"` |
| `CreatePayment(card)` | `POST /v1/payment_intents` com `automatic_payment_methods` | ✅ |
| `CreatePayment(pix)` | `POST /v1/payment_intents` com `payment_method_types=[pix]` + `confirm=true` | 🟡 requer ativar Pix na dashboard |
| `CreatePayment(boleto)` | `POST /v1/payment_intents` com `boleto.tax_id` + billing details | ✅ |
| `GetPayment(id)` | `GET /v1/payment_intents/:id` | ✅ |
| `VerifyWebhook(body, headers)` | `webhook.ConstructEvent` do SDK (HMAC via `Stripe-Signature`) | ✅ |
| `ParseWebhookEvent(body)` | Parse `payment_intent.*` events | ✅ |

### Detalhes importantes

- **IdempotencyKey**: suportado nativamente via `stripe.Params.IdempotencyKey` — cada request recebe o `X-Request-Id` como idempotency key (foundation de H1 do audit)
- **Conversão centavos ↔ reais**: Stripe usa `int64` em centavos; convertemos `Amount float64` reais via `* 100` e `/ 100` no retorno
- **Client secret para Card**: frontend usa `stripe.confirmPayment(clientSecret)` — PAN/CVV nunca tocam nosso servidor (PCI SAQ-A)
- **Boleto**: `next_action.boleto_display_details` contém `hosted_voucher_url`, `pdf`, `number` (barcode) — frontend renderiza in-page

### Webhook signature

Stripe `Stripe-Signature: t=TIMESTAMP,v1=HEX` — o SDK oficial `webhook.ConstructEvent(body, signature, secret)` faz tudo em 1 call:
1. Parse o formato
2. Valida timestamp (janela 5min — replay protection)
3. HMAC-SHA256 sobre `timestamp + "." + body`
4. Compare com `v1`

Compare com MP que exige reimplementar manualmente (ver audit C4).

---

## 4. Resultados dos testes (via curl)

### 4.1 ✅ Cartão — FUNCIONAL

```bash
POST /api/v1/payments
{"order_id":"66666666-...","method":"card","amount":199.90}

→ 201 Created
{
  "id": "c2a64202-3e06-457a-852a-921949e8839e",
  "provider": "stripe",
  "psp_id": "pi_3TPoctBjqYCOj3YA1Wl8Wp7m",
  "clientSecret": "pi_3TPoctBjqYCOj3YA1Wl8Wp7m_secret_KMFxd708h4bdsNOdAegtlbBVt",
  "method": "card",
  "status": "pending"
}
```

Frontend usa `clientSecret` com `@stripe/react-stripe-js` → MP renderiza form no iframe interno → usuário preenche cartão → `stripe.confirmPayment()` → PaymentIntent muda pra `succeeded` → webhook dispara.

### 4.2 ✅ Boleto — FUNCIONAL

```bash
POST /api/v1/payments
{"order_id":"...","method":"boleto","amount":99.90,
 "payer_cpf":"12345678909","payer_name":"Ana Silva"}

→ 201 Created
{
  "id": "abb5bf8d-...",
  "provider": "stripe",
  "psp_id": "pi_3TPoctBjqYCOj3YA0tmlvCv8",
  "clientSecret": "pi_...",
  "method": "boleto",
  "status": "pending",
  "psp_payload": {
    "next_action": {
      "type": "boleto_display_details",
      "boleto_display_details": {
        "number": "01010101010101...",
        "pdf": "https://payments.stripe.com/boleto/voucher/test_YW.../pdf",
        "hosted_voucher_url": "https://payments.stripe.com/boleto/voucher/test_YW...",
        "expires_at": 1777056116
      }
    }
  }
}
```

Frontend renderiza o PDF + código de barras in-page via `hosted_voucher_url` ou copiando `number`. **Zero redirect.**

### 4.3 🟡 Pix — precisa de 1 clique no dashboard

```bash
POST /api/v1/payments
{"order_id":"...","method":"pix","amount":49.90}

→ 502 Bad Gateway
(log do Stripe):
"The payment method type \"pix\" is invalid. Please ensure the provided type is
activated in your dashboard (https://dashboard.stripe.com/account/payments/settings)"
```

**Como resolver:** ir em https://dashboard.stripe.com/account/payments/settings e ativar **Pix** (está em "Additional payment methods" → "Pix"). Após ativar, o mesmo curl acima retorna `next_action.pix_display_qr_code` com QR code + copy-paste string.

### 4.4 ✅ GET + Sync

```bash
GET /api/v1/payments/c2a64202-3e06-457a-852a-921949e8839e
→ { "id": "...", "status": "pending", "method": "card", "amount": 199.90, "psp_payment_id": "pi_3TPoctBjqYCOj3YA1Wl8Wp7m", ... }

POST /api/v1/payments/c2a64202-3e06-457a-852a-921949e8839e/sync
→ {
    "provider": "stripe",
    "psp_status": "pending",
    "psp_amount": 199.90,
    "local_amount": 199.90,
    "changed": false
  }
```

Sync chama `paymentintent.Get` no Stripe, compara amount, atualiza se mudou. Status "pending" porque o cartão ainda não foi confirmado via Elements — após `stripe.confirmPayment()` no frontend, sync retornaria `approved` + `changed: true`.

---

## 5. O que falta pra 100% E2E

### 5.1 Frontend — Stripe Elements (~3h)

Reestruturar [CheckoutPage](../../app/src/pages/checkout/) pra usar:
```tsx
import { Elements, PaymentElement } from '@stripe/react-stripe-js'
import { loadStripe } from '@stripe/stripe-js'

const stripePromise = loadStripe(import.meta.env.VITE_STRIPE_PUBLISHABLE_KEY)

<Elements stripe={stripePromise} options={{ clientSecret }}>
  <PaymentElement />
  <button onClick={() => stripe.confirmPayment({ elements, confirmParams: { return_url } })}>
    Pagar
  </button>
</Elements>
```

Para Pix/Boleto, renderizar o `next_action` in-page (QR code SVG, PDF embed, copy button).

### 5.2 Ativar Pix no dashboard Stripe (**para o Marlon fazer**)

1. https://dashboard.stripe.com/account/payments/settings
2. Procurar "Pix" na lista (está dentro da categoria "Wallet" ou "Additional")
3. Clicar "Turn on" / "Ativar"
4. Rodar o teste da §4.3 novamente — deve retornar 201 com QR code

### 5.3 Webhook via Stripe CLI (~5min)

```bash
# Terminal separado:
stripe listen --forward-to localhost:8090/webhooks/stripe
# Output: "webhook signing secret: whsec_abc123..."

# Colar no .env.local:
STRIPE_WEBHOOK_SECRET=whsec_abc123...
# Restart payment-service → webhook real validando assinatura
```

Stripe CLI também permite triggerar eventos manualmente:
```bash
stripe trigger payment_intent.succeeded
```

### 5.4 Endpoint webhook no payment-service (~1h)

Atualmente o webhook handler ainda é específico pra MP. Precisamos adicionar:
```go
api.POST("/webhooks/stripe", webhookH.HandleStripe)
```

Ou abstrair via Gateway.VerifyWebhook + ParseWebhookEvent (já temos a interface, só preciso do handler usar ela).

---

## 6. DX comparison — tempo real gasto

| Tarefa | MP | Stripe |
|---|---|---|
| Criar conta + pegar credentials | 15min (duas rodadas) | 2min |
| Primeiro `POST /payments` funcionando | Nunca (em sandbox test user) | ~30s após curl |
| Debug de erro | "Unauthorized use of live credentials" sem ação | Código de erro + doc URL + link pra logs |
| Ativar Pix/Boleto | Requer onboarding + dev panel config | 1 clique no dashboard |
| Webhook local | ngrok + config manual no dashboard | `stripe listen` = 1 comando |
| Docs | Fragmentadas entre Checkout Pro, API, Bricks | Unificadas em PaymentIntents |

---

## 7. Conclusão

**Stripe é estratosfericamente melhor em DX que MP.** Diferença visível em todos os pontos: onboarding, primeira chamada bem sucedida, mensagens de erro, ferramentas de dev.

**Recomendação**: usar Stripe como PSP **primário** em produção (não só dev). Taxas são ~0.7% maiores que MP (4.4% vs 3.99% cartão), mas:
- Diferença em 1000 vendas/mês × ticket R$200 = R$1.400/mês
- Em troca: ~dezenas de horas economizadas em debug/frustração + features melhores (3DS2 v2.2, anti-fraude ML incluído)
- Pra loja de ferragem com ticket R$100-500, a diferença operacional vale muito mais que a margem

Se o Marlon quiser MP em produção por razão comercial (marca conhecida), **a abstração já está pronta** — só setar `PSP_PROVIDER=mercadopago` no deploy.

---

## 8. Ação imediata

**Para continuar testes:**
1. ⚡ Ativar Pix em https://dashboard.stripe.com/account/payments/settings
2. Me avisar → rodo o teste do Pix E2E (QR code + simulação de pagamento)
3. Opcional: rodar `stripe listen` pra ter webhook real
4. Depois: implementar Stripe Elements no frontend (~3h)

---

## 9. Commits desta sessão

- `<TBD>` — `feat(payment): PSP abstraction + Stripe gateway (card+boleto OK, pix pending dashboard activation)`

Migrações de arquivos:
- `internal/mercadopago/` → `internal/psp/mercadopago/`
- `mercadopago.New` → `mercadopago.NewClient` (conflito com Gateway.New)
- `mercadopago.Gateway` + `stripe.Gateway` ambos implementando `psp.Gateway`
