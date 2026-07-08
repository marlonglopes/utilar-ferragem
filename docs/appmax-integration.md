# Integração Appmax (payment-service)

Status: **implementado no backend, aguardando credenciais da conta Utilar para validação em sandbox.**

A Appmax foi adicionada como terceiro provedor PSP, atrás da mesma abstração
`psp.Gateway` que já suporta Stripe e Mercado Pago. Trocar de provedor é só mudar
uma env var — nenhum handler importa um PSP específico.

## Como ativar

```bash
PSP_PROVIDER=appmax
APPMAX_ACCESS_TOKEN=<token da API Appmax>
APPMAX_WEBHOOK_SECRET=      # opcional (ver "Modelo de confiança do webhook")
```

O token sai do Painel Appmax → **Aplicações → API → Instalar** (a chave é
liberada pelo chat de suporte). O mesmo host serve sandbox e produção
(`https://api.appmax.com.br/v1`) — o ambiente é definido pela credencial.

## Fluxo (order-centric)

Diferente de Stripe/MP (que são amount-centric), a Appmax exige criar o pedido
antes de cobrar. `CreatePayment` orquestra três chamadas internamente:

1. `POST /v1/customers` → cria/atualiza cliente → `data.customer.id`
2. `POST /v1/orders` → cria pedido com line items → `data.order.id`
3. `POST /v1/payments/{pix|boleto|credit-card}` → cobra o pedido

O **id do pedido Appmax** é usado como `psp_payment_id` no nosso DB. É por ele
que `GetPayment` (`GET /v1/orders/:id`) e o webhook reconciliam.

Autenticação: o `access-token` vai **no corpo JSON** de cada request (não é
header Bearer); em GET, vai como query param. O client injeta isso sozinho.

## Segurança

- **Amount autoritativo (audit C1/C2)**: preservado. O handler já deriva o valor
  do `order.total` do order-service; passamos esse valor pra Appmax como um único
  line item. O cliente não consegue adulterar o preço.
- **Modelo de confiança do webhook (audit C3)**: a Appmax **não assina postbacks
  com HMAC**. A integridade vem da **re-consulta**: ao receber um webhook, o
  handler chama `GetPayment` (GET do pedido na Appmax) e compara o amount
  autoritativo do PSP com o nosso DB antes de confirmar qualquer pagamento —
  mismatch vira `payment.fraud_suspect` e o pagamento fica pendente pra revisão.
  - `APPMAX_WEBHOOK_SECRET` é **opcional**: se setado, exigimos que o header
    `X-Appmax-Token` bata (defesa em profundidade). Por isso o `config.Load`
    **não** exige webhook secret pra `appmax` em prod (diferente de Stripe/MP).

## Lacunas conhecidas (a resolver antes de produção)

1. **Telefone do cliente**: a Appmax marca `phone` como obrigatório no customer.
   Hoje não coletamos telefone no checkout — mandamos vazio. O antifraude pode
   pedir revisão. → Coletar telefone no `CheckoutPage`.
2. **Cartão de crédito**: exige o cartão **tokenizado no browser** (Appmax.js) —
   nunca trafegamos PAN pelo backend (PCI SAQ-A). O gateway aceita `CardToken`,
   mas o SPA ainda não gera esse token. Pix e boleto funcionam 100% server-side.
   → Integrar o tokenizador Appmax.js no `CardPayment.tsx`.
3. **Nomes exatos de campos de resposta**: os campos do QR Pix (`pix_qrcode` /
   `pix_emv`) e a URL do PDF do boleto precisam ser reconfirmados contra o
   sandbox. Guardamos o payload cru inteiro em `psp_payload` e repassamos ao SPA,
   então o `PixPayment.tsx`/`BoletoPayment.tsx` vão precisar mapear os campos reais
   da Appmax (hoje esperam o shape do MP/Stripe).
4. **Status via webhook não assinado**: o handler usa `event.Status` (do corpo do
   postback) para o estado final, validando só o amount via re-consulta. Como a
   Appmax não assina, considerar derivar o status também do `GetPayment`
   autoritativo — hardening recomendado antes de tráfego real.

## Checklist de validação (quando a conta estiver ativa)

- [ ] `APPMAX_ACCESS_TOKEN` no `.env.local`, `PSP_PROVIDER=appmax`, service sobe
- [ ] Criar pedido real → Pix: QR + copia-e-cola renderizam no checkout
- [ ] Confirmar campos reais de resposta e ajustar `PixPayment`/`BoletoPayment`
- [ ] Boleto: PDF + linha digitável
- [ ] Configurar URL de postback no painel Appmax → confirmar recebimento
- [ ] Testar mismatch de amount → `payment.fraud_suspect` no outbox
- [ ] Coletar telefone no checkout; integrar Appmax.js pro cartão

## Arquivos

- `services/payment-service/internal/psp/appmax/client.go` — HTTP client
- `services/payment-service/internal/psp/appmax/gateway.go` — `psp.Gateway`
- `services/payment-service/internal/psp/appmax/gateway_test.go` — testes (mock server)
- `services/payment-service/internal/config/config.go` — env vars + validação
- `services/payment-service/cmd/server/main.go` — seleção do gateway
