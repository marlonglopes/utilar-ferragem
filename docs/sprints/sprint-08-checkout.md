# Sprint 08 — Checkout (Pix, boleto, cartão)

**Fase**: 3 — Comércio. **Status**: ✅ Concluído (08a + 08b em 2026-04-23).

## Escopo

O sprint de maior risco. Introduz o novo **payment-service** (Go 1.26 + Gin 1.12) e os três métodos de pagamento brasileiros.

## Tarefas

### Sprint 08a — payment-service (Go)
1. ✅ Scaffold Go 1.26 em `services/payment-service/` — Gin 1.12, lib/pq, franz-go, golang-migrate
2. ✅ Migration: tabela `payments` com `id, order_id, method (pix|boleto|card), status, psp_payment_id, psp_metadata, amount, currency, created_at, confirmed_at`
3. ✅ Cliente PSP Mercado Pago — `internal/mercadopago/client.go` com interface injetável (testável via `NewWithBaseURL`)
4. ✅ Endpoints:
   - `POST /api/v1/payments` (JWT obrigatório) — cria pagamento; retorna payload por método
   - `GET /api/v1/payments/:id` — consulta status
   - `POST /webhooks/mp` — webhook Mercado Pago; valida HMAC-SHA256; idempotência via `ON CONFLICT DO NOTHING`
5. ✅ Event bus: outbox drainer publica `payment.confirmed` / `payment.failed` no Redpanda v26
6. ✅ Infra Docker: `postgres:17-alpine` (:5435), `redpandadata/redpanda:v26.1.6` (:19092), `redpandadata/console:v3.7.1` (:8085)
7. ✅ Testes Go: 9 testes (5 HMAC + 4 resolveStatus + 4 MP client unit)

### Sprint 08b — SPA

8. ✅ `usePayment` hook — createPayment (pix/boleto/card), polling 3s/300 max, mock auto-confirm 6s
9. ✅ `CheckoutPage` (`/checkout`, ProtectedRoute) — wizard 3 passos: endereço (ViaCEP autofill) → frete (PAC/SEDEX/grátis stub) → pagamento
10. ✅ `PixPayment` — QR base64, copia-e-cola, countdown, expirado/regenerar, confirmado
11. ✅ `BoletoPayment` — linha digitável, download PDF, aviso 3 dias, vencimento
12. ✅ `CardPayment` — hosted drop-in MP, botão sandbox
13. ✅ `OrderConfirmationPage` (`/pedido/:id`) — mensagens por método, polling Pix, e-mail, links
14. ✅ Testes frontend: 89 passando (12 arquivos)

### Pendente (próximos sprints)
- [ ] Gateway rotear `/api/v1/payments/*` → payment-service (Sprint 09+)
- [ ] Checkout multi-vendedor (múltiplos pedidos por `group_id`)
- [ ] E-mail de confirmação via SES (Sprint 09)
- [ ] URL do webhook MP configurada (configurar após deploy produção)

## Critérios de aceite

- [x] Caminho feliz mock: carrinho → checkout → Pix → QR exibido → confirmação auto em 6s (sandbox)
- [x] Boleto exibe linha digitável e aviso de 3 dias úteis
- [x] Cartão redireciona para hosted checkout MP (sandbox: `initPoint`)
- [x] Replay de webhook não processa duplamente (idempotência no `psp_payment_id`)
- [ ] Pedido aparece em `/conta/pedidos` do cliente (Sprint 09)
- [ ] E-mail de confirmação enviado via SES (Sprint 09)
- [x] Zero dados de cartão tocam nossos servidores (hosted drop-in MP)

## Dependências

- Sprint 07 concluído
- Conta PSP aberta e credenciais de sandbox disponíveis
- Sprint 18 do projeto pai integrado (campos de endereço BR nos pedidos)

## Riscos

- Peculiaridades da integração com o PSP (formato de assinatura de webhook do Mercado Pago, expiração de tokens) — reservar buffer de 3 dias
- Atrito do 3DS — manter o fluxo de cartão funcionando com e sem desafios
- QR codes Pix têm TTL — tratar a expiração de forma elegante com botão de regeneração
