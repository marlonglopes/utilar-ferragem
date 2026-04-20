# Fase 3 — Comércio

**Objetivo**: um cliente real pode pagar com dinheiro real por produtos reais e recebê-los em casa. Esta é a fase de lançamento.

## Sprints

### Stack de comércio
- [Sprint 06 — Carrinho (local + persistido)](../sprints/sprint-06-cart.md)
- [Sprint 07 — Auth do cliente + conta](../sprints/sprint-07-auth.md)
- [Sprint 08 — Checkout (Pix, boleto, cartão)](../sprints/sprint-08-checkout.md)
- [Sprint 09 — Histórico de pedidos + rastreamento](../sprints/sprint-09-orders.md)

### Hardening pré-lançamento (gate de lançamento)
- [Sprint 22 — Observabilidade em produção](../sprints/sprint-22-observability.md)
- [Sprint 23 — CI/CD + Terraform IaC](../sprints/sprint-23-ci-cd-iac.md)
- [Sprint 24 — Conformidade com a LGPD](../sprints/sprint-24-lgpd.md)
- [Sprint 25 — Prontidão para lançamento (SEO, jurídico, e-mail, vendedores)](../sprints/sprint-25-launch-readiness.md)

## Definição de pronto

- Carrinho persiste no localStorage e sobrevive a refresh/login
- Cliente pode se cadastrar com e-mail + senha + CPF (validado)
- Autofill do ViaCEP funciona no formulário de endereço do checkout
- Checkout divide-se em 3 etapas: Endereço → Entrega → Pagamento
- Os três métodos de pagamento funcionando em PSP de produção:
  - **Pix**: QR code + copia e cola + polling para confirmação
  - **Boleto**: gerar PDF, exibir código de barras, aguardar até 3 dias úteis
  - **Cartão de crédito**: tokenizado via SDK do PSP, parcelamento opcional (até 12x)
- Página de confirmação de pedido exibe timeline de status (pago → separando → enviado → entregue)
- Histórico de pedidos em `/conta/pedidos` lista todos os pedidos anteriores com filtros
- Detalhe do pedido mostra itemização completa, endereços, método de pagamento, timeline de status
- Confirmação por e-mail enviada no pedido pago (SES ou reutilizar mailer existente)
- Cliente pode cancelar um pedido não pago
- Cliente não pode cancelar um pedido pago/enviado pela UI (deve contatar o vendedor)

## Novo serviço: payment-service

Um novo serviço de backend. Veja [ADR 002](../adr/002-integration-strategy.md).

Escopo:
- Geração de Pix + confirmação via webhook
- Emissão de boleto + confirmação via webhook
- Pass-through de tokenização de cartão + iniciação de 3DS
- Webhook → publica eventos `payment.confirmed` / `payment.failed`
- order-service consome esses eventos para fazer transição de status do pedido

PSP: **A definir durante o Sprint 08**. Candidatos: Mercado Pago, Gerencianet, Stripe BR, PagSeguro. Critérios de decisão no ADR 002.

## Explicitamente fora do escopo

- Assinaturas / pagamentos recorrentes
- Pagamentos divididos para múltiplos vendedores em um checkout (enviar apenas pedidos de vendedor único na Fase 3; divisão multi-vendedor é Fase 4)
- Reembolsos via UI (manual, pelo vendedor, na Fase 3)
- Faturamento / Nota Fiscal Eletrônica (Fase 4)

## Saída para a Fase 4 quando

- Pelo menos 5 pedidos pagos reais concluídos (podem ser de amigos e família)
- Zero bugs P0 no fluxo de pagamento por 7 dias consecutivos
- Taxa de conversão de detalhe do produto → pedido ≥ 1% (ou evidência de que é um problema de UX, não de plataforma)

## Checklist de lançamento (fim da Fase 3)

- [ ] Domínio real (`utilarferragem.com.br`) apontando para produção
- [ ] SSL válido
- [ ] Termos de uso + política de privacidade publicados (conformes com a LGPD)
- [ ] Banner de consentimento de cookies LGPD ativo
- [ ] Rastreamento de erros Sentry ativo
- [ ] Google Analytics ou Plausible ativo
- [ ] Primeiros 3 vendedores com pelo menos 20 produtos cada
- [ ] Endereço de e-mail de suporte ao cliente + SLA de resposta definidos
- [ ] Página de status para incidentes (mesmo que seja uma página estática)
