# Sprint 03 — Home do catálogo + navegação por categoria

**Fase**: 2 — Catálogo. **Estimativa**: 5–7 dias.

## Escopo

As primeiras páginas voltadas ao cliente. Somente leitura, sem carrinho.

## Tarefas

1. Conectar `GET /api/v1/products` em um hook `useProducts(params)` (TanStack Query)
2. Construir a `HomePage` (`/`): seção hero, rail de 8 categorias, 8 produtos em destaque (primeiros 8 da API), bloco editorial (estático no Sprint 03, dinâmico depois)
3. Construir o `ProductCard`: imagem, nome, preço (BRL com `formatCurrency`), badge de estoque, affordance "Ver produto"
4. Construir a `CategoryPage` (`/categoria/:slug`): breadcrumb, título, grid de produtos, paginação (server-side via `?page=&per_page=24`)
5. Construir `src/lib/taxonomy.ts` — árvore de taxonomia tipada com mapeamento para as strings legadas de `category`
6. Adicionar página 404 para slugs de categoria desconhecidos
7. Reclassificar os 65 produtos do seed na taxonomia de ferragem (via mapeamento — sem migration)
8. Ocultar produtos que não são de ferragem na camada de cliente (até definirmos o escopo de filtragem no backend)

## Critérios de aceite

- [ ] A home renderiza com 8 categorias + 8 produtos em ≤ 2s em 4G
- [ ] Clicar em uma categoria navega para `/categoria/:slug` com os produtos relevantes
- [ ] A paginação funciona e é acessível pelo teclado
- [ ] Cada card de produto exibe o preço em BRL (não USD) para usuários em pt-BR
- [ ] Skeleton loaders aparecem em redes lentas
- [ ] Estado vazio para categorias sem produtos

## Dependências

- Sprint 02 concluído (precisa dos primitivos `ProductCard`, `Breadcrumb`, `Pagination`)
- Sprint 17 do projeto pai integrado (para `products.currency`) OU fallback para exibição em USD com aviso

## Riscos

- O mapeamento de taxonomia pode não cobrir todos os produtos — aceitar 5% de classificação errada no Sprint 03 e refinar no 04
- Assets de imagem no seed podem não ser adequados para ferragem — problema cosmético, não bloqueante
