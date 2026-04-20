# Sprint 13 — Contas Pro / B2B (clientes com CNPJ)

**Fase**: 5 — Crescimento. **Estimativa**: 8–10 dias.

## Escopo

Construtoras, obras e autônomos da construção civil representam uma parcela desproporcional da receita de ferragens e esperam conveniências B2B: comprar com CNPJ, receber nota fiscal eletrônica (NF-e) e negociar preços por volume via fluxo de orçamento. Este sprint adiciona suporte completo a clientes PJ ponta a ponta: toggle PF/PJ no cadastro, captura de CNPJ, um fluxo de "Solicitar orçamento" que circula entre cliente e vendedor, e emissão de NF-e em pedidos pagos via provedor brasileiro.

Questão em aberto: provedor de NF-e — **eNotas** vs. **Focus NFe**. Ambos têm SDKs Ruby e planos sandbox. Decidir no kickoff do sprint após comparar os valores com o volume projetado da Fase 5.

## Tarefas

### user-service
1. Migration `services/user-service/db/migrate/YYYYMMDD_add_pj_fields_to_users.rb`: adicionar `customer_type ∈ pf|pj` (padrão `pf`), `cnpj` (14 dígitos, único quando preenchido, Módulo 11 via `CnpjValidator` existente), `razao_social`, `inscricao_estadual` (nullable)
2. Atualizar `POST /api/v1/auth/register` em `services/user-service/app/controllers/api/v1/auth_controller.rb` para aceitar `customer_type`, `cnpj`, `razao_social`, `inscricao_estadual` quando `role='customer'` + `customer_type='pj'`
3. Atualizar o serializer de usuário para incluir os campos PJ para que a SPA possa renderizar o estado correto de interface

### fluxo de orçamento (order-service + novas tabelas)
4. Migration `services/order-service/db/migrate/YYYYMMDD_create_quote_requests.rb`: tabela `quote_requests` (`id, customer_id, seller_id, status ∈ pending|quoted|accepted|rejected|expired, notes, total, valid_until, created_at, quoted_at, accepted_at`)
5. Migration para `quote_items`: `id, quote_request_id, product_id, quantity, unit_price (nullable até o orçamento), line_total (nullable até o orçamento)`
6. Endpoint `POST /api/v1/quote-requests` (JWT cliente) — corpo: `{ seller_id, items: [{product_id, quantity}], notes }`
7. Endpoint `GET /api/v1/quote-requests` (JWT) — com escopo por papel (cliente vê os próprios, vendedor vê os que lhe são endereçados)
8. Endpoint `POST /api/v1/quote-requests/:id/quote` (JWT vendedor) — preenche `unit_price` por linha + `valid_until` (padrão +7 dias) + transita para `quoted`
9. Endpoint `POST /api/v1/quote-requests/:id/accept` (JWT cliente) — transita para `accepted`, cria um pedido real via o caminho de criação de pedidos existente, retorna `{ order_id }`
10. Worker `services/order-service/app/workers/quote_expiry_worker.rb` rodando no Sidekiq-cron toda noite — transita orçamentos `quoted` que passaram do `valid_until` para `expired`

### payment-service — integração NF-e
11. Adicionar gem do SDK do provedor (eNotas ou Focus NFe conforme decisão do kickoff) ao `services/payment-service/Gemfile`
12. Objeto de serviço `services/payment-service/app/services/nfe_issuer.rb` — abstrai a chamada ao provedor; entradas: pedido, cliente (PF ou PJ), itens, frete; saídas: número da NF-e, chave de acesso, URL do PDF, URL do XML
13. Assinar o tópico Kafka `payment.confirmed` no payment-service (ou order-service — definir o responsável no kickoff; padrão: order-service chama o endpoint `/api/v1/nfe` do payment-service) e disparar a emissão da NF-e
14. Migration `services/payment-service/db/migrate/YYYYMMDD_create_nfes.rb`: tabela `nfes` (`id, order_id, provider, provider_nfe_id, numero, chave, xml_url, pdf_url, status ∈ pending|issued|rejected, issued_at, rejection_reason`)
15. Endpoint `GET /api/v1/orders/:id/nfe` — retorna os metadados da NF-e + URL assinada para o PDF

### Gateway
16. Rotear `/api/v1/quote-requests*` para o order-service; rotear `/api/v1/orders/:id/nfe` para o payment-service

### SPA (utilar-ferragem)
17. Atualizar `RegisterPage`: toggle de rádio PF/PJ no topo; quando PJ selecionado, substituir o campo CPF por CNPJ + razão social + inscrição estadual (opcional); reutilizar o validador `cnpj.ts`
18. Página de carrinho: botão secundário "Solicitar orçamento" ao lado de "Finalizar compra"; abre um modal com notas opcionais e depois faz POST em `/quote-requests`
19. Página `/conta/orcamentos` → `QuoteListPage.tsx` — tabela de orçamentos com pills de status
20. Página `/conta/orcamentos/:id` → `QuoteDetailPage.tsx` — tabela de itens com preços (após o orçamento), CTA "Aceitar e comprar" que aceita o orçamento e redireciona para o checkout com o novo pedido já criado
21. Página de detalhe do pedido: exibir botão "Baixar nota fiscal" (com link para o PDF em `/orders/:id/nfe`) quando o pedido estiver pago e a NF-e tiver sido emitida

### gifthy-hub (lado do vendedor)
22. Nova página `/seller/orcamentos` — caixa de entrada de solicitações de orçamento pendentes com ação "Responder" que abre um formulário para preencher preços unitários + valid_until

## Critérios de aceite
- [ ] Cliente PJ pode se cadastrar, informar CNPJ + razão social, e seus pedidos geram NF-e na confirmação do pagamento
- [ ] O fluxo de cliente PF permanece inalterado (cadastro, checkout, pedido — sem novos campos obrigatórios)
- [ ] O fluxo de orçamento funciona de ponta a ponta: cliente solicita → vendedor orça → cliente aceita → pedido criado → fluxo de pagamento
- [ ] Orçamentos após `valid_until` transitam para `expired` em até 24 horas
- [ ] PDF da NF-e disponível para download na página de detalhe do pedido; a chave de acesso bate com a NF-e emitida
- [ ] Falhas na emissão de NF-e são registradas e retentadas; o pedido permanece `paid` mesmo enquanto a fila de retry está processando

## Dependências
- Sprint 08 (checkout / payment-service) concluído
- Sprint 09 (pedidos) concluído
- Conta no provedor de NF-e + credenciais sandbox (prazo longo — iniciar em paralelo com o Sprint 11)
- Revisão jurídica dos termos de serviço PJ (o operador deve ao comprador a conformidade com a nota fiscal)

## Riscos
- Conformidade com NF-e é complexa (CFOPs, CSTs, particularidades estaduais) — restringir a um conjunto reduzido de CFOPs no lançamento e expandir após aprovação jurídica
- Precificação no orçamento expõe vendedores a manipulação de preços — limitar solicitações de orçamento por cliente a ≤ 20/dia
- Cadastro PJ aumenta o atrito no formulário de registro — fazer A/B do posicionamento do toggle PF/PJ antes de solidificar
- Indisponibilidade do provedor trava a emissão de NF-e — fila de retry + alerta ao operador após 10 falhas consecutivas
