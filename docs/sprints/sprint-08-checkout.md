# Sprint 08 — Checkout (Pix, boleto, cartão)

**Fase**: 3 — Comércio. **Estimativa**: 10–14 dias (sprint mais longo; considerar dividir em 8a/8b).

## Escopo

O sprint de maior risco. Introduz o novo **payment-service** e os três métodos de pagamento brasileiros.

## Tarefas

### payment-service (novo)
1. Criar o scaffold de um novo serviço Rails em `services/payment-service/` (ou Go — questão em aberto, ver ADR 002)
2. Migration: tabela `payments` com `id, order_id, method (pix|boleto|card), status, psp_payment_id, psp_metadata, amount, currency, created_at, confirmed_at`
3. Cliente PSP: wrapper fino em torno do SDK do PSP escolhido. Começar com **um** PSP (recomendação: Mercado Pago, por ter Pix + boleto + cartão em uma única integração). Abstrair a interface desde o início para que a troca seja barata.
4. Endpoints:
   - `POST /api/v1/payments` (auth obrigatória) — cria um pagamento para um pedido; retorna o payload específico do método (QR Pix, URL do boleto, URL de desafio do cartão)
   - `GET /api/v1/payments/:id` — consulta status
   - `POST /webhooks/psp/:psp_name` — recebe confirmação assíncrona; valida assinatura; publica evento
5. Event bus: publicar `payment.confirmed` + `payment.failed` no Redpanda; order-service assina e faz a transição de estado do pedido

### Gateway
6. Rotear `/api/v1/payments/*` para o payment-service

### SPA
7. Construir a `CheckoutPage` (`/checkout`) com 3 etapas, hashadas na URL (`#endereco`, `#entrega`, `#pagamento`):
   - **Etapa 1 — Endereço**: escolher salvo ou adicionar novo; autofill de CEP via ViaCEP (copiar `src/lib/cep.ts` do gifthy-hub)
   - **Etapa 2 — Entrega**: opções de frete (stub com frete fixo + placeholder Correios; integração real na Fase 5)
   - **Etapa 3 — Pagamento**: seletor de método + UI específica do método
8. Construir o componente `PixPayment`: imagem do QR code, código copia-e-cola com botão de cópia, polling de status a cada 3s por 15 min, expira de forma elegante
9. Construir o componente `BoletoPayment`: código de barras, download em PDF, aviso de que o boleto leva até 3 dias úteis para compensar
10. Construir o componente `CardPayment`: iframe/drop-in hospedado do PSP (NÃO coletar dados de cartão diretamente); dropdown de parcelas (1x–12x)
11. Fluxo de criação de pedido:
    - Cliente cria pedido via `POST /api/v1/orders` (status `pending_payment`)
    - Cliente cria pagamento via `POST /api/v1/payments` com o order_id
    - Cliente exibe a UI específica do método
    - Webhook chega → evento → order-service transiciona o pedido para `paid`
    - Polling SPA / server-push notifica o cliente
12. Construir a `OrderConfirmationPage` (`/pedido/:id`) — exibe status e próximos passos (Pix: "Aguardando confirmação", Boleto: "Pague até DD/MM/YYYY", Cartão: "Pagamento aprovado")
13. Checkout multi-vendedor: se o carrinho tiver > 1 vendedor, criar N pedidos, um por vendedor, vinculados por um `group_id` para a visualização do cliente

## Critérios de aceite

- [ ] Caminho feliz: carrinho → checkout → Pix → leitura do QR (sandbox) → página de confirmação exibe "pago" em até 10s
- [ ] Boleto gera um PDF com código de barras válido
- [ ] Pagamento com cartão usando cartão sandbox é aprovado, incluindo desafio 3DS
- [ ] Replay de webhook não processa duplamente (idempotência no `psp_payment_id`)
- [ ] Pedido aparece em `/conta/pedidos` do cliente com os itens e o total corretos
- [ ] E-mail de confirmação enviado (SES ou stub mailer)
- [ ] Zero dados de cartão tocam nossos servidores (verificar aplicabilidade do PCI SAQ-A)

## Dependências

- Sprint 07 concluído
- Conta PSP aberta e credenciais de sandbox disponíveis
- Sprint 18 do projeto pai integrado (campos de endereço BR nos pedidos)

## Riscos

- Peculiaridades da integração com o PSP (formato de assinatura de webhook do Mercado Pago, expiração de tokens) — reservar buffer de 3 dias
- Atrito do 3DS — manter o fluxo de cartão funcionando com e sem desafios
- QR codes Pix têm TTL — tratar a expiração de forma elegante com botão de regeneração
