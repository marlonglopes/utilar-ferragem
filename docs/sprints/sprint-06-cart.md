# Sprint 06 — Carrinho (local + persistente)

**Fase**: 3 — Comércio. **Estimativa**: 3–4 dias.

## Escopo

A superfície de comércio mais simples. Totalmente client-side neste sprint.

## Tarefas

1. Construir `cartStore.ts` (Zustand + middleware `persist` → chave localStorage `utilar-cart`)
2. Shape do estado: `{ items: Array<{ productId, sellerId, quantity, priceSnapshot, addedAt }> }`
3. Actions: `addItem`, `removeItem`, `updateQuantity`, `clearCart`, `mergeCarts` (chamada após o login)
4. Conectar "Adicionar ao carrinho" no detalhe do produto + card de produto
5. Construir `CartDrawer`: drawer lateral direito, lista de itens (imagem, nome, vendedor, seletor de qty, subtotal, remover), rodapé com total + "Ir para o checkout"
6. Construir `CartPage` (`/carrinho`): versão em página inteira; mesmo conteúdo, mais espaço para cupom de desconto + estimativa de frete stub
7. Exibir badge com contagem de itens no ícone do carrinho na navbar
8. Revalidar estoque ao abrir o carrinho: itens desatualizados exibem aviso "Preço ou estoque mudou" e atualizam o snapshot
9. Carrinho multi-vendedor: agrupar itens por vendedor com um subtítulo (preparar para divisão no checkout no Sprint 08)

## Critérios de aceite

- [ ] Adicionar → fechar o navegador → reabrir → carrinho ainda lá
- [ ] O seletor de quantidade respeita o limite de estoque
- [ ] Remover o último item exibe estado vazio com CTA "Explorar catálogo"
- [ ] Após o login, itens adicionados antes do login persistem (sem sobrescrever)
- [ ] O Drawer é acessível pelo teclado (Esc fecha, trap de Tab)

## Dependências

- Sprints 04 + 05 concluídos

## Riscos

- Snapshot de preço vs. preço atual — se houver divergência no checkout, exibir a diferença e exigir confirmação do usuário antes de prosseguir
