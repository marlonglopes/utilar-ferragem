# Sprint 15 — Disputas de pagamento, reembolsos e chargebacks

**Fase**: 4 — Operações do vendedor. **Estimativa**: 5–7 dias.

## Escopo

Existem três vetores de reembolso/disputa no mix de pagamentos BR que suportamos: reversões Pix via janela do **MED (Mecanismo Especial de Devolução)**, chargebacks de cartão iniciados pelo emissor, e reembolsos manuais iniciados pelo nosso time de admin (ex.: reclamações de clientes, produto sem estoque após o pagamento). Este sprint fecha o ciclo: o admin consegue reembolsar de ponta a ponta a partir do gifthy-hub; o payment-service se comunica com a API de reembolso de cada PSP via adapter; e disputas iniciadas pelo PSP disparam revisão pelo admin e notificação ao vendedor.

O escopo MVP é de **reembolsos totais apenas**; reembolsos parciais serão entregues em um sprint futuro. Referência: [ADR 005](../adr/005-payment-strategy.md).

## Tarefas

### payment-service
1. Migration `services/payment-service/db/migrate/YYYYMMDD_add_refund_fields_to_payments.rb`: `payments.refunded_amount` (decimal, padrão 0), `payments.refunded_at`, `payments.dispute_status ∈ none|opened|won|lost` (padrão `none`), `payments.dispute_opened_at`, `payments.dispute_resolved_at`
2. Padrão de adapter: `services/payment-service/app/clients/psp/base_refund_client.rb` (abstrato) + `mercado_pago_refund_client.rb` + stubs para PSPs futuros — cada um implementa `#refund(payment, amount:)` e normaliza a resposta
3. Endpoint `POST /api/v1/payments/:id/refund` (JWT admin) — corpo `{ reason }`; chama o adapter; em caso de sucesso, atualiza `refunded_amount` + `refunded_at` + publica `payment.refunded` no Kafka
4. Extensão do handler de webhook em `services/payment-service/app/controllers/webhooks/psp_controller.rb` — tratar tipos de evento `payment.reversed` (Pix MED) e `payment.chargeback` (cartão); atualizar `dispute_status`; publicar `payment.disputed` no Kafka
5. Idempotência: deduplicar por `psp_event_id` via tabela `webhook_events` para que webhooks reenviados não façam dupla transição de estado
6. Mailer `services/payment-service/app/mailers/refund_mailer.rb` com templates `refund_processed` (cliente) + `dispute_opened` (vendedor); bilíngue pt-BR / en

### order-service
7. Assinar `payment.refunded` no consumer existente; transitar pedido → `refunded`; inventory-service libera estoque novamente via o caminho existente estilo `order.cancelled` (publicar novo evento `order.refunded` para o inventory-service consumir)
8. Assinar `payment.disputed`; adicionar `orders.dispute_flag` (boolean); expor no dashboard do vendedor

### inventory-service
9. Assinar `order.refunded` → incrementar estoque novamente; registrar uma linha em `inventory_events` (idempotente por `order_id`)

### Gateway
10. Rotear `/api/v1/payments/:id/refund` (JWT admin) para o payment-service; a rota existente `/webhooks/psp/*` já cobre os novos tipos de evento de webhook

### gifthy-hub (admin)
11. No detalhe do pedido admin, adicionar botão "Emitir reembolso" (visível quando o pedido está `paid` e `refunded_amount = 0`); modal exige um motivo; faz POST em `/payments/:id/refund`
12. Nova página `/admin/disputas` — tabela de pedidos com `dispute_flag = true`; colunas: id do pedido, cliente, vendedor, valor, tipo de disputa, aberta em, coluna de ação ("Ver detalhe")
13. Prévia de e-mail no admin: exibir o e-mail de cliente formatado antes de emitir o reembolso

### gifthy-hub (vendedor)
14. Adicionar aba "Disputas" no dashboard do vendedor listando pedidos com `dispute_flag = true`; o vendedor visualiza o status da disputa mas não pode agir diretamente (política: admin trata)

### SPA (utilar-ferragem)
15. No detalhe do pedido, se `status = refunded`, exibir um badge + data do reembolso; se a disputa estiver aberta, exibir aviso discreto "Em análise — entraremos em contato"

## Critérios de aceite
- [ ] O admin consegue reembolsar totalmente um pedido pago; em até 60 segundos: status do pedido muda para `refunded`, estoque restaurado, e-mail do cliente enviado
- [ ] O mesmo webhook entregue duas vezes não é processado em duplicata (idempotente por `psp_event_id`)
- [ ] Disputa iniciada pelo PSP (sandbox) sinaliza o pedido, notifica o vendedor por e-mail e aparece em `/admin/disputas`
- [ ] Os tópicos Kafka `payment.refunded` e `payment.disputed` existem e carregam os payloads documentados
- [ ] Reembolso para um PSP sem adapter retorna um 501 claro com mensagem voltada ao operador, não um 500
- [ ] O e-mail do cliente é renderizado corretamente em pt-BR e en de acordo com a preferência de idioma do usuário

## Dependências
- Sprint 08 (checkout / payment-service) concluído
- [ADR 005](../adr/005-payment-strategy.md) aprovado
- Contas sandbox de PSP com suporte a simulação de reembolso + chargeback

## Riscos
- O formato da API de reembolso de cada PSP é diferente (Mercado Pago assíncrono, alguns PSPs síncronos) — padrão de adapter + máquina de estados segura para async
- Reembolsos parciais são explicitamente fora do escopo do MVP — documentar a limitação para que o admin não tente; construir suporte parcial em sprint futuro
- A janela do Pix MED é de 80 dias — garantir que o handler de disputas funcione para pedidos antigos que podem ter saído das tabelas ativas (verificar caminhos de query antes da virada)
- Taxas de chargeback dos PSPs ainda não estão refletidas nos repasses aos vendedores — abrir ticket de TODO para reconciliação de repasses
