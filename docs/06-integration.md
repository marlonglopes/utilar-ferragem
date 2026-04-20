# 06 — Integração (adoções específicas da Utilar)

**Este documento registra as escolhas de integração específicas da Utilar Ferragem.** Ele **não** duplica o guia da plataforma — leia-o primeiro:

> 📘 **Referência da plataforma**: [../../docs/integration-guide.md](../../docs/integration-guide.md)

Tudo abaixo é específico da Utilar: o que escolhemos, o que adicionamos à plataforma e quais mudanças entram em qual sprint. Quando o guia da plataforma descreve um padrão genérico, registramos aqui o que a Utilar adotou.

---

## 1. Escolhas específicas da Utilar

| Decisão | Utilar Ferragem | Referência |
|---------|-----------------|-----------|
| **Destino de deploy** | S3 + CloudFront, API via `api.utilarferragem.com.br` (roteamento por subdomínio para o gateway compartilhado) | [ADR 001](adr/001-placement-and-stack.md) |
| **Porta de dev** | `5175` (5173 = gifthy-hub, 5174 reservado) | §11 do guia da plataforma |
| **Locale padrão** | `pt-BR`, `en` como secundário | [03-architecture.md](03-architecture.md) |
| **Papel do usuário no cadastro** | `customer` — novo valor, requer uma pequena mudança no user-service (ver §2) | — |
| **PSP** | Mercado Pago (recomendado; decisão final no kick-off do Sprint 08) | [ADR 002](adr/002-integration-strategy.md) |
| **Tópicos de evento adicionados** | `payment.confirmed`, `payment.failed` (Sprint 08) | §4 abaixo |
| **Hostname / CORS** | `utilarferragem.com.br`, `*.utilarferragem.com.br` precisam ser adicionados à lista de permissões CORS do gateway antes de ir para produção | §2 abaixo |

---

## 2. Mudanças no backend que a Utilar precisa

Todas as mudanças são **aditivas e retrocompatíveis**. Nenhuma exige downtime.

### Sprint 04 — product-service
- Adicionar coluna JSONB `products.specs` (nullable). A Utilar vai populá-la para produtos de ferragens; outros consumidores deixam null.
- Arquivo de migration: `services/product-service/db/migrate/YYYYMMDD_add_specs_to_products.rb`.
- Seguimento opcional: índice GIN em `specs` quando a carga de queries justificar.

### Sprint 07 — user-service
- Aceitar `role='customer'` em `POST /api/v1/auth/register` ([services/user-service/app/controllers/api/v1/auth_controller.rb](../../services/user-service/app/controllers/api/v1/auth_controller.rb)) e **pular** a chamada `create_seller!`.
- Adicionar coluna `users.cpf` (nullable, validada com o mesmo algoritmo do CNPJ). Em paralelo ao trabalho planejado do Sprint 18 — coordenar com o responsável por aquele sprint.
- Sem breaking change no caminho existente de `role='seller'`.

### Sprint 08 — novo `payment-service`
- Novo serviço Rails em `services/payment-service/`.
- Endpoints (requer JWT): `POST /api/v1/payments`, `GET /api/v1/payments/:id`. Públicos: `POST /webhooks/psp/:psp_name`.
- Registro de rota no gateway em [services/gateway/cmd/server/main.go](../../services/gateway/cmd/server/main.go): montar `/api/v1/payments` (protegido por JWT) e `/webhooks/psp/*` (público).
- Novas variáveis de ambiente: `PAYMENT_SERVICE_URL`, `PSP_API_KEY`, `PSP_WEBHOOK_SECRET`, `PSP_ENVIRONMENT`.
- Publica eventos: `payment.confirmed`, `payment.failed` (novos tópicos Kafka — ver §4).
- O order-service consome esses eventos e transiciona o status do pedido (`pending_payment → paid` na confirmação, `pending_payment → cancelled` na falha após retries).

### Sprint 26 — user-service + order-service + product-service (cashback)

- **product-service**: adicionar coluna `cashback_percent` DECIMAL(4,2) (nullable, 0–10) em `products`; expor em todos os serializers de produto. Migration: `services/product-service/db/migrate/YYYYMMDD_add_cashback_percent_to_products.rb`.
- **order-service**: adicionar `cashback_percent_snapshot` DECIMAL(4,2) em `order_items`; adicionar `cashback_earned_cents` INTEGER em `orders`. Ao receber `order.paid`, publicar `cashback.earned`; ao receber `order.delivered`, publicar `cashback.credited`; em cancelamento/estorno, publicar `cashback.reversed`.
- **user-service — módulo de cashback** (`app/services/cashback/`): `EarnService`, `CreditService`, `RedeemService`, `ExpireService`, `ReverseService`, `AdjustService`. Consumidores Kafka para `cashback.earned` e `cashback.credited`. Job noturno de expiração (`CashbackExpiryJob`). Nova tabela `cashback_ledger` — ver [07-data-model.md §9](07-data-model.md#9-cashback-ledger-sprint-26).
- **payment-service**: aceitar `cashback_redemption_id` + `cashback_discount_cents` do checkout; repassar desconto ao valor do pedido no PSP; armazenar na tabela `payments`.
- **Rotas do gateway** (todas requerem JWT): `GET /api/v1/me/cashback`, `GET /api/v1/me/cashback/history`, `POST /api/v1/me/cashback/redeem`, `POST /api/v1/admin/cashback/adjust`.
- Sem breaking changes em nenhum caminho existente.

### Rollout em produção — CORS do gateway
- Substituir `Access-Control-Allow-Origin: *` por uma lista de permissões orientada por variável de ambiente, incluindo `https://utilarferragem.com.br` e a URL de staging do S3/CloudFront.
- Rastreado no checklist de lançamento ([phases/phase-3-commerce.md](phases/phase-3-commerce.md)).

---

## 3. Endpoints que a Utilar consome

Do [catálogo do guia da plataforma](../../docs/integration-guide.md#4-endpoint-catalog):

### Públicos (sem JWT)
- `POST /auth/register` — cadastro de novo cliente (`role='customer'`, CPF obrigatório)
- `POST /auth/login` — login do cliente
- `GET /api/v1/marketplace/products` — listagem do catálogo (paginado server-side)

### Requerem JWT
- `GET /api/v1/users/me` — perfil do cliente
- `POST /api/v1/orders` — checkout
- `GET /api/v1/orders` (+ `/:id`, `/:id/status`) — histórico e rastreamento de pedidos

### Introduzidos pela Utilar
- `POST /api/v1/payments` — criar pagamento para um pedido (Pix / boleto / cartão)
- `GET /api/v1/payments/:id` — consultar status do pagamento
- `POST /webhooks/psp/:psp_name` — confirmação assíncrona do PSP (público, assinatura verificada)

### Cashback (Sprint 26 — cliente + admin)
- `GET /api/v1/me/cashback` — saldo + pendente + próximo vencimento
- `GET /api/v1/me/cashback/history?page=N&per=20` — ledger paginado
- `POST /api/v1/me/cashback/redeem` — reservar resgate no checkout; retorna `redemption_id`
- `POST /api/v1/admin/cashback/adjust` — débito/crédito manual pelo admin com justificativa

### A Utilar NÃO consome
- `/api/v1/sellers/*` — gestão de vendedores é feita no `gifthy-hub`, não no app de cliente da Utilar
- `/api/v1/inventory/*` — estoque é superfície do vendedor, não exibida diretamente ao cliente
- `/api/v1/admin/*` — operações admin ficam no `gifthy-hub`

---

## 4. Eventos Kafka que a Utilar adiciona

| Tópico | Produtor | Consumidor | Payload |
|--------|----------|-----------|---------|
| `payment.confirmed` | payment-service (handler de webhook) | order-service | `{payment_id, order_id, psp_payment_id, amount, method, confirmed_at}` |
| `payment.failed` | payment-service | order-service | `{payment_id, order_id, psp_payment_id, reason, failed_at}` |
| `payment.confirmed.dlq` | (automático, na falha do consumidor) | (inspeção manual) | Mesmo + metadados de erro |

O fluxo existente de `order.created` permanece inalterado — a reserva de estoque continua disparando na criação do pedido. A confirmação de pagamento é uma transição separada.

---

## 5. Variáveis de ambiente introduzidas pela Utilar

| Variável | Onde é usada | Notas |
|----------|-------------|-------|
| `VITE_API_URL` | SPA da Utilar | `http://localhost:8080` em dev; `https://api.utilarferragem.com.br` em produção |
| `PAYMENT_SERVICE_URL` | Gateway | ex: `http://payment-service:3000` |
| `PSP_API_KEY` | payment-service | Token de acesso do Mercado Pago (ou PSP escolhido) — **segredo, nunca commitar** |
| `PSP_WEBHOOK_SECRET` | payment-service | Chave HMAC para verificação da assinatura do webhook |
| `PSP_ENVIRONMENT` | payment-service | `sandbox` / `production` |
| `SES_FROM_ADDRESS` | payment-service + order-service | Remetente dos e-mails transacionais (confirmação de pedido) |

Variáveis compartilhadas do guia da plataforma (`JWT_SECRET`, `KAFKA_BROKERS`, etc.) são reutilizadas sem alteração.

---

## 6. Checkpoints de wiring por sprint

Esta é a sequência de eventos de wiring entre backend e frontend. Cada checkpoint tem uma condição "pronto quando" antes de o sprint poder ser fechado.

| Sprint | Backend entrega primeiro | Frontend consome | Pronto quando |
|--------|--------------------------|-----------------|---------------|
| 01 — Scaffold | — | Acessa `/health` via gateway | 200 do gateway `/health` no CI |
| 03 — Catálogo | `GET /api/v1/marketplace/products` (existente) | Hook `useProducts()` | Home + categoria renderizam produtos reais |
| 04 — PDP | Migration `products.specs` implantada | PDP lê `product.specs` | Ficha técnica renderiza para produtos com specs; degrada sem elas |
| 05 — Busca | Parâmetro `?q=` no endpoint do marketplace | Página de busca conectada | Busca por "furadeira" retorna correspondências ILIKE reais |
| 07 — Auth | `role='customer'` aceito; coluna `users.cpf` | Páginas de cadastro + login | Novo cliente cadastra → login automático → sessão persiste |
| 08 — Checkout | payment-service em staging com sandbox do PSP | UIs de Pix/boleto/cartão no CheckoutPage | Teste Pix no sandbox de ponta a ponta aprovado |
| 09 — Pedidos | order-service consome `payment.confirmed` + publica atualizações | Detalhe do pedido reflete status do Kafka | Vendedor marca enviado no gifthy-hub → Utilar mostra "enviado" em até 10s |
| 10 — Onboarding do vendedor | Criação do vendedor reutiliza `/auth/register` com `role='seller'` (sem mudança no backend) | Wizard `/vender` | Novo CNPJ fica como vendedor pendente no user-service |

---

## 7. Status

Status atualizado de cada ponto de integração. Atualizar conforme os sprints entram em produção.

| Integração | Status | Notas |
|------------|--------|-------|
| Wiring `VITE_API_URL` do gateway | ⬜ não iniciado | Sprint 01 |
| JSONB `products.specs` | ⬜ não iniciado | Preparação Sprint 04 |
| Suporte a `role='customer'` em `auth#register` | ⬜ não iniciado | Preparação Sprint 07 |
| Coluna `users.cpf` | ⬜ não iniciado | Coordenar com Sprint 18 do pai |
| Scaffold do payment-service | ⬜ não iniciado | Sprint 08 |
| Conta PSP + credenciais sandbox | ⬜ não iniciado | **Iniciar em paralelo com Sprint 03** — lead time longo |
| Tópicos `payment.confirmed` + `payment.failed` | ⬜ não iniciado | Sprint 08 |
| `cashback_percent` em products + `cashback_percent_snapshot` em order_items | ⬜ não iniciado | Sprint 26 |
| Tabela `cashback_ledger` + módulo cashback no user-service | ⬜ não iniciado | Sprint 26 |
| Tópicos Kafka de cashback (5 tópicos) | ⬜ não iniciado | Sprint 26 |
| Lista de permissões CORS do gateway (produção) | ⬜ não iniciado | Pré-lançamento (gate da Fase 3) |
| Identidade do remetente no SES | ⬜ não iniciado | Pré-lançamento |

---

## 8. Protocolo de handoff

Quando um sprint da Utilar lança uma mudança no backend, o responsável pelo sprint:
1. Atualiza a tabela no §7 para ✅.
2. Se a mudança adiciona ou modifica uma capacidade da plataforma (novo endpoint, novo tópico, nova variável de ambiente), também atualiza [../../docs/integration-guide.md](../../docs/integration-guide.md). O guia da plataforma é o contrato compartilhado — não deixar os docs específicos da Utilar divergirem silenciosamente.
3. Registra qualquer desvio do plano no §1 deste arquivo.

---

## 9. Perguntas em aberto

- **Escolha do PSP**: Mercado Pago é a recomendação atual, mas Gerencianet, Stripe BR e PagSeguro continuam como candidatos. Decisão final no kick-off do Sprint 08, após cotar taxas reais. Ver [ADR 002](adr/002-integration-strategy.md).
- **Endurecimento do CORS**: o gateway atual permite `*`. Quem é o responsável pela mudança no nível de infra para mover para uma lista baseada em variável de ambiente, e quando?
- **Rate limiting**: nenhum hoje no nível do gateway. Precisamos antes de a Utilar abrir ao público? (Recomendação: sim — um limitador simples por IP+path antes do gate da Fase 3.)
- **Exportação de dados do cliente (LGPD)**: obrigatório por lei. Hoje não existe nenhum mecanismo. Provavelmente Fase 3 pré-lançamento ou Fase 4 — precisa de um responsável real.
