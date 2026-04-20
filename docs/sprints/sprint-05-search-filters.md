# Sprint 05 — Busca + filtros facetados + ordenação

**Fase**: 2 — Catálogo. **Estimativa**: 5–7 dias.

## Escopo

Descoberta de produtos. Os clientes conseguem afunilar um catálogo grande até o SKU certo com rapidez.

## Tarefas

1. Barra de busca global na navbar: query → `/busca?q=`
2. `SearchPage` (`/busca`): mesmo grid da página de categoria, mas com escopo nos resultados da busca
3. Suporte a `?q=` server-side em `GET /api/v1/products` (backend: `ILIKE` básico contra nome + descrição + SKU; evoluir para full-text se necessário)
4. Construir `FacetSidebar`: grupos recolhíveis, multi-seleção por checkbox, slider de intervalo de preço
5. Construir `src/lib/filters.ts` com o schema de filtros por categoria de [product-scope](../04-product-scope.md)
6. Conectar os filtros aos query params: `?category=&brand=&price_min=&price_max=&spec[voltage]=220V&in_stock=true`
7. Construir `SortDropdown`: relevância / preço ↑ / preço ↓ / mais recente / melhor avaliado
8. Linha de chips de filtros ativos acima do grid com ✕ individual e "Limpar tudo"
9. Preservar a posição de scroll ao voltar de uma página de detalhe
10. Estado vazio para zero resultados: sugerir categoria mais ampla, "Limpar filtros", rail de produtos em destaque

## Critérios de aceite

- [ ] Buscar "furadeira" retorna todos os produtos que correspondem ao nome/descrição/SKU
- [ ] Selecionar "220V" + "Bosch" filtra os resultados corretamente e atualiza a URL
- [ ] A URL é compartilhável — colar em nova aba reproduz a visualização exata
- [ ] Mobile: filtros abrem como bottom sheet, não como sidebar
- [ ] Resultados atualizam em até 300ms após mudança de filtro (estado de loading se demorar mais)
- [ ] Lighthouse performance ≥ 85 com 5 filtros aplicados

## Dependências

- Sprint 04 (para navegar entre páginas)
- Backend: parâmetro `?q=` no endpoint de produtos
- Idealmente: filtragem `spec[key]=value` server-side. Se não estiver pronto, client-side com banner indicando "Filtros em beta".

## Riscos

- Qualidade da busca full-text — migrar para uma ferramenta de busca real (Meilisearch / Algolia) na Fase 5 se ILIKE for insuficiente
- Proliferação de filtros — limitar a 6 filtros visíveis por categoria; mover o restante para "Mais filtros"
