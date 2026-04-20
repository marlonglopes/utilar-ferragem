# Fase 2 — Catálogo

**Objetivo**: clientes podem descobrir e entender produtos. Ainda não podem comprá-los.

## Sprints

- [Sprint 03 — Home do catálogo + navegação por categoria](../sprints/sprint-03-catalog.md)
- [Sprint 04 — Página de detalhe do produto](../sprints/sprint-04-product-detail.md)
- [Sprint 05 — Busca + filtros facetados + ordenação](../sprints/sprint-05-search-filters.md)

## Definição de pronto

- Página inicial com hero, trilho de 8 categorias, 8 produtos em destaque, bloco editorial
- Página de categoria (`/categoria/:slug`) renderiza grid de até 24 produtos por página com paginação server-side
- Página de detalhe do produto: galeria de imagens (lightbox), tabela de especificações, badge de estoque, card do vendedor, "produtos relacionados"
- Busca (`/busca?q=`) acessa um endpoint básico de busca (inicialmente filtro client-side da lista paginada; promover para server-side na Fase 2.5 se necessário)
- Filtros facetados funcionam por categoria (mínimo 3 facetas: preço, marca, em estoque, mais 1 filtro profissional)
- Ordenação: relevância (padrão), preço ↑, preço ↓, mais recente, mais bem avaliado
- Breadcrumb reflete a posição atual na taxonomia
- Todos os 65 produtos do seed estão reclassificados na taxonomia de ferragens (ou ocultos se não forem de ferragens)
- Performance no Lighthouse ≥ 85 nas páginas de categoria e produto
- Todas as strings renderizam corretamente em pt-BR e em en

## Adições de backend (mínimas)

- **product-service**: migração da coluna JSONB `products.specs` (aditiva, nullable)
- **product-service**: `GET /api/v1/products` aceita `?category=<taxonomy-path>&spec[key]=value&sort=&page=&per_page=` (parsear e aplicar quando possível; ignorar parâmetros desconhecidos para compatibilidade futura)
- **product-service**: coluna `products.currency` (Sprint 17 do projeto pai — confirmar merge antes de usar)

## Explicitamente fora do escopo

- Carrinho (Fase 3)
- Qualquer tela de checkout
- Exibição de avaliações/reviews (renderizar placeholder `4,2 ★` de um campo stub; avaliações reais na Fase 5)

## Saída para a Fase 3 quando

Uma gravação de sessão de um cliente DIY encontrando um produto específico em menos de 90 segundos for capturada e revisada.
