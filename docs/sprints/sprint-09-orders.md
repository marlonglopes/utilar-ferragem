# Sprint 09 — Histórico de pedidos + rastreamento

**Fase**: 3 — Comércio. **Estimativa**: 4–5 dias.

## Escopo

Fechar o loop do comércio: os clientes conseguem ver, entender e agir sobre seus pedidos anteriores.

## Tarefas

1. Construir a `OrdersPage` (`/conta/pedidos`): lista com filtros (status, intervalo de datas), busca por número de pedido, paginação
2. Construir a `OrderDetailPage` (`/conta/pedidos/:id`):
   - Timeline de status: `pending_payment → paid → picking → shipped → delivered` (reaproveitar o padrão do gifthy-hub)
   - Tabela de itens (imagem, nome, vendedor, qty, preço unitário, total)
   - Card de endereço de entrega
   - Card de forma de pagamento (método + informação parcial: "Pix ∙ pago em DD/MM HH:MM")
   - Card de contato do vendedor (telefone / e-mail / link para a loja do vendedor)
   - Código de rastreamento (quando o vendedor adicionar) → link para o site da transportadora
3. CTA "Comprar novamente": adiciona todos os itens ainda disponíveis de volta ao carrinho
4. CTA "Cancelar pedido": visível apenas quando status = `pending_payment`; confirma via modal; faz POST para o order-service
5. Link de ticket de suporte: "Problema com este pedido" → e-mail para o suporte com contexto do pedido
6. Filtro `?status=` no lado do cliente em `GET /api/v1/orders?mine=true`

## Critérios de aceite

- [ ] O cliente vê apenas seus próprios pedidos (não todos os pedidos)
- [ ] A timeline de status avança quando o vendedor marca o pedido como enviado no gifthy-hub (via evento Kafka → order-service)
- [ ] "Comprar novamente" trata corretamente itens sem estoque (adiciona o que está disponível, notifica sobre os que faltam)
- [ ] A paginação preserva os filtros na URL

## Dependências

- Sprint 08 concluído
- order-service: garantir que `GET /api/v1/orders` suporte o escopo `?mine=true` pelo subject do JWT

## Riscos

- As transições de status são acionadas por ações do vendedor no gifthy-hub; qualquer latência aparece aqui como status desatualizado — adicionar um timestamp "última atualização"
