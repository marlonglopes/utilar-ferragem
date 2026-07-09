# Integração Appmax (payment-service) — API v3

Status: **backend implementado na API v3, espelhando a integração de produção do
gifthy** (validada contra o sandbox real). Aguardando `APPMAX_ACCESS_TOKEN` da
conta da Utilar para validação E2E.

> **Por que v3?** Existem duas APIs Appmax. A **v1** (appmax.readme.io) usa OAuth2
> e valores em centavos; a **v3 oficial** (docs.appmax.com.br) usa `access-token`
> e valores em reais. O gifthy re-mirou o adapter da v1 → v3 em 2026-06-09 e
> validou contra o sandbox em 2026-06-10. A Utilar segue a **v3**, igual ao gifthy
> (`gifthy/api/pkg/payments/appmax`).

## Como ativar

```bash
PSP_PROVIDER=appmax
APPMAX_ACCESS_TOKEN=<token do gerente de conta Appmax>
# Sandbox/homologação:
APPMAX_BASE_URL=https://homolog.sandboxappmax.com.br/api/v3
# Produção (default se APPMAX_BASE_URL vazio): https://admin.appmax.com.br/api/v3
APPMAX_WEBHOOK_SECRET=   # opcional (ver "Modelo de confiança do webhook")
```

O token sai do **gerente de conta** Appmax (não há signup self-service). O
`access-token` vai **no corpo JSON E no header** de cada request.

## Fluxo v3 (order-centric — valores em REAIS)

`CreatePayment` orquestra três chamadas:

1. `POST /customer` → `{success, data:{id}}` → **customer_id = `data.id`**
   (CPF **não** vai aqui — vai no pagamento)
2. `POST /order` → `{success, data:{id, status, total}}` → **order_id = `data.id`**
3. `POST /payment/{credit-card|pix|boleto}` — envelope:
   ```json
   { "access-token":"...", "cart":{"order_id":N}, "customer":{"customer_id":N},
     "payment":{ "pix":{"document_number":"CPF"} } }
   ```
   Note a assimetria de chaves: **`pix`** minúsculo, **`Boleto`** e **`CreditCard`**
   PascalCase (igual gifthy/doc oficial).

O **id do pedido Appmax** vira o `psp_payment_id`. `GET /order/:id` reconcilia
status/valor. Estorno: `POST /refund {order_id, refund_type:"total"}`.

### Respostas (validadas no sandbox pelo gifthy, 2026-06-10)

- **Pix** → `data.pix_emv` (copia-e-cola), `data.pix_qrcode` (PNG base64),
  `data.pix_expiration_date`. ⚠️ `success` vem **`"ATIVA"`** (string) no Pix — o
  client decide por **HTTP status** (≥400 = erro), ignora `success`.
- **Boleto** → `data.pdf` (URL), `data.digitable_line`, `data.due_date`.
- **Cartão** → auto-capturado; `{success:true}` sem status final — a confirmação
  fina (aprovado/cancelado) vem por **webhook**.

O parser é **tolerante** (`digID`/`parseDisplay`) porque a doc não publica o JSON
verbatim. O `ClientData` devolvido ao SPA normaliza: `{pix_qrcode, pix_emv,
pix_expires_at, boleto_url, boleto_line}`.

## Status de pedido v3 → normalizado

| Appmax | Normalizado |
|---|---|
| `aprovado`, `integrado`, `pendente_integracao` | approved (**pago**) |
| `autorizado` | authorized |
| `cancelado`, `estornado`, `chargeback_perdido` | cancelled |
| `recusado_por_risco`, `recusado` | rejected |
| `pendente`, `análise antifraude` | pending |

## Webhook (v3)

Payload: `{environment, event, data}`. Eventos em **PascalCase** (`OrderApproved`,
`OrderPaid`, `OrderPaidByPix`, `OrderIntegrated`, `OrderRefund`, `OrderPixExpired`,
`OrderBilletOverdue`, `PaymentNotAuthorized`, …). Dois formatos tolerados:

- **DefaultResponse**: `data.id` + `data.status`
- **TwoLevel**: `data.order_id` + `data.order_status`
- (também aceita `data.order.*` aninhado)

`normEvent` normaliza case/separador (`OrderPaid`/`order_paid` → `orderpaid`).
Evento inconclusivo cai pro status do pedido.

### Modelo de confiança do webhook (audit C3)

A Appmax **não assina postbacks com HMAC**. A integridade vem da **re-consulta**:
o handler chama `GetPayment` (`GET /order/:id`) e compara o amount autoritativo do
PSP com o nosso DB antes de confirmar — mismatch vira `payment.fraud_suspect` e o
pagamento fica pendente. `APPMAX_WEBHOOK_SECRET` é **opcional** (header
`X-Appmax-Token`). Por isso o `config.Load` não exige webhook secret pra `appmax`.

## Lacunas conhecidas (a resolver antes de produção)

1. **Endereço + telefone do cliente**: a Appmax exige endereço pra **boleto** e
   recomenda pra antifraude; hoje o `CreatePayment` não recebe esses dados.
   → Coletar telefone + endereço no `CheckoutPage` e propagar via `psp.CreateRequest`.
2. **Cartão de crédito**: exige o cartão **tokenizado no browser** (Appmax JS) —
   nunca trafegamos PAN pelo backend (PCI SAQ-A). O gateway aceita `CardToken`,
   mas o SPA ainda não gera esse token (falta a URL do SDK + credencial pública).
   Pix e boleto funcionam 100% server-side.
3. **Frontend Pix/boleto**: `PixPayment.tsx`/`BoletoPayment.tsx` precisam mapear o
   `ClientData` da Appmax (`pix_emv`, `pix_qrcode`, `boleto_url`, `boleto_line`).
4. **`GET /order/:id`**: usado pela reconciliação — confirmar o endpoint exato no
   sandbox (o gifthy reconcilia pelo payload do webhook; aqui preferimos re-consultar).

## Checklist de validação (quando a conta estiver ativa)

- [ ] `APPMAX_ACCESS_TOKEN` + `APPMAX_BASE_URL` (sandbox) no `.env.local`, `PSP_PROVIDER=appmax`
- [ ] Rodar customer→order→payment/pix real → QR + copia-e-cola renderizam
- [ ] Boleto: PDF + linha digitável
- [ ] Configurar URL de postback no painel Appmax → confirmar recebimento + formato
- [ ] Testar mismatch de amount → `payment.fraud_suspect` no outbox
- [ ] Confirmar `GET /order/:id`; coletar telefone/endereço; integrar Appmax JS (cartão)

## Arquivos

- `services/payment-service/internal/psp/appmax/client.go` — HTTP client v3
- `services/payment-service/internal/psp/appmax/gateway.go` — `psp.Gateway`
- `services/payment-service/internal/psp/appmax/gateway_test.go` — testes (mock v3)
- Referência: `gifthy/api/pkg/payments/appmax/{client,helpers}.go` + `payment-webhook/appmax.go`
