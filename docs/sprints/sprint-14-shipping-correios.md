# Sprint 14 — Fretes + rastreamento (Melhor Envio)

**Fase**: 4 — Operações do vendedor. **Estimativa**: 6–8 dias.

## Escopo

O checkout do Sprint 08 utiliza um valor de frete fixo como placeholder. Ferragens têm cálculos não triviais de peso e dimensões (uma caixa de parafusos vs. uma serra circular vs. uma escada) e os clientes esperam a mesma transparência de frete que veem no Mercado Livre: preços reais de PAC/SEDEX dos Correios além de transportadoras privadas (Jadlog, Loggi, Total Express). Este sprint integra o **Melhor Envio** — um agregador de fretes brasileiro com API REST unificada — tanto para cotação de frete no checkout quanto para geração de etiquetas após o pagamento.

Referência: [ADR 006](../adr/006-shipping-provider.md).

Questão em aberto: criar um `shipping-service` dedicado ou adicionar um módulo de frete ao `order-service`. Decisão padrão para este sprint: **módulo dentro do order-service** (cotação de frete é sensível a latência + o order-service já é dono do ciclo de vida do pedido). Reavaliar extração se a lógica de frete superar 2 controllers.

## Tarefas

### order-service
1. Migration `services/order-service/db/migrate/YYYYMMDD_create_shipping_quotes.rb`: tabela de cache `shipping_quotes` (`id, cache_key, origin_cep, dest_cep, weight_g, quotes_json, expires_at, created_at`); índice TTL em `expires_at` (1 hora)
2. Migration `services/order-service/db/migrate/YYYYMMDD_add_shipping_to_orders.rb`: `orders.shipping_carrier`, `orders.shipping_service`, `orders.tracking_code`, `orders.shipping_label_url`
3. Migration `services/order-service/db/migrate/YYYYMMDD_create_shipping_events.rb`: tabela `shipping_events` (`id, order_id, code, description, occurred_at, raw_payload_json, created_at`)
4. Cliente Melhor Envio `services/order-service/app/clients/melhor_envio_client.rb` — auth OAuth2; encapsula `/me/shipment/calculate` (cotação), `/me/cart` (adicionar ao carrinho), `/me/shipment/checkout` (comprar etiqueta), `/me/shipment/print` (PDF); retry com backoff exponencial; timeout de 5s no calculate
5. Endpoint `POST /api/v1/shipping/quote` — corpo: `{ seller_id, destination_cep, items: [{product_id, quantity}] }`; retorna array de `{ carrier, service, price, delivery_days }`; grava no cache
6. Endpoint `POST /api/v1/orders/:id/shipping-label` (JWT vendedor) — compra + imprime a etiqueta via Melhor Envio; armazena `shipping_label_url`; define status do pedido como `ready_to_ship`
7. Endpoint `POST /webhooks/melhor-envio` (público, com verificação de assinatura) — recebe atualizações de rastreamento; faz upsert em `shipping_events`; publica `order.shipping_update` no Kafka

### product-service
8. Adicionar `products.weight_g` (inteiro, obrigatório) + `products.dimensions_cm` (JSONB: `{length, width, height}`, obrigatório) — migration + validação; backfill dos dados de seed com valores razoáveis por categoria
9. Atualizar `ProductForm` no fluxo do vendedor (AddProduct + EditProduct + template de importação em lote) para exigir peso + dimensões

### Gateway
10. Rotear `/api/v1/shipping/*` (JWT) + `/webhooks/melhor-envio` (público) para o order-service em `services/gateway/cmd/server/main.go`

### SPA (utilar-ferragem)
11. Atualizar a Etapa 2 do `CheckoutPage` (Entrega): ao chegar na etapa, fazer POST em `/shipping/quote` com o `seller_id` do carrinho + CEP de destino; exibir opções de transportadora (nome, serviço, preço em BRL, prazo); seleção por rádio; fallback para frete fixo se a API demorar mais de 5s ou retornar erro
12. Estado de carregamento com skeleton durante a cotação; estado de erro com "Tentar novamente" + fallback de frete fixo visível
13. Na `OrderDetailPage` (`/conta/pedidos/:id`), exibir componente de linha do tempo de entrega `ShippingTimeline.tsx` lendo de `shipping_events`; mostrar o status mais recente em destaque
14. Quando `tracking_code` estiver presente, exibir link externo "Rastrear na transportadora" usando templates de URL específicos por transportadora

### gifthy-hub
15. Detalhe do pedido do vendedor: botão "Gerar etiqueta" (visível quando o pedido está `paid`) — faz POST em `/orders/:id/shipping-label`; ao concluir, exibe link "Imprimir etiqueta" abrindo o PDF
16. Impressão de etiquetas em lote: "Imprimir todas" seleciona N pedidos pagos e chama o endpoint em sequência

## Critérios de aceite
- [ ] A Etapa 2 do checkout exibe tarifas reais dos Correios + transportadoras privadas em menos de 2 segundos (p50) para o caminho com 95% de cache hits
- [ ] Se a resposta do Melhor Envio ultrapassar 5s ou retornar erro, o fallback de frete fixo ativa silenciosamente e o checkout continua
- [ ] O vendedor clica em "Gerar etiqueta" e recebe um PDF imprimível em até 10 segundos
- [ ] O cliente visualiza um novo evento de rastreamento na página de detalhe do pedido em até 10 minutos após o escaneamento pela transportadora
- [ ] Pedidos sem `weight_g` / `dimensions_cm` não chegam ao checkout — aplicado na validação do product-service
- [ ] Taxa de cache hit de cotação de frete > 70% após uma semana de tráfego (gancho de observabilidade)
- [ ] Assinaturas de webhook verificadas; replay do mesmo evento é idempotente em `shipping_events`

## Dependências
- Sprint 08 (checkout) concluído
- [ADR 006](../adr/006-shipping-provider.md) aprovado
- Conta no Melhor Envio + credenciais sandbox (solicitar cedo — a configuração OAuth2 tem prazo de 2–3 dias)
- Estratégia de backfill para produtos existentes sem `weight_g` / `dimensions_cm`

## Riscos
- Indisponibilidade do Melhor Envio afeta a conversão do checkout — o fallback de frete fixo é obrigatório, não opcional
- O fluxo de impressão de etiquetas pode exigir renderização de PDF no servidor estilo `puppeteer` — testar a geração de PDF sob carga
- Qualidade dos dados de peso/dimensão fornecidos por vendedores (importação em lote) é historicamente ruim — adicionar valores padrão por categoria como rede de segurança (ex.: fixador padrão = 500g)
- Frete por vendedor (carrinho multi-vendedor) exige N chamadas de cotação separadas — paralelizar no controller e limitar concorrência a 4
