# Sprint 21 — Otimização de performance

**Fase**: 5 — Crescimento. **Estimativa**: 4–6 dias.

## Escopo

Atingir **Lighthouse ≥ 85** em todas as quatro categorias nas páginas home, PDP e de categoria. Atingir **LCP < 2,5s** e **TTI < 3,5s** na emulação Slow 4G / Moto G4. Manter o **bundle JS inicial < 200KB gzipado**.

Trabalho no frontend: code splitting por rota, pipeline de imagens (WebP + srcset responsivo + blur placeholder), CSS crítico, `font-display: swap`, hints de preconnect, React.lazy para páginas pesadas (checkout, admin, bulk-import). Trabalho no backend: headers de cache HTTP em endpoints públicos, compressão no gateway/CloudFront (gzip + brotli) e auditoria de N+1 no product-service.

Fazer isso **próximo ao lançamento** — otimizar um produto instável desperdiça ciclos. O Lighthouse CI consolida os ganhos para evitar regressões futuras.

## Tarefas

### SPA — bundle + code splitting
1. Executar `vite-bundle-visualizer` (ou `rollup-plugin-visualizer`), commitar o relatório em `utilar-web/docs/bundle-baseline.html`, identificar as 5 importações mais pesadas
2. Converter rotas pesadas para `React.lazy`: `CheckoutPage`, `BulkImportPage`, `AdminUtilarLayout`, `OrderDetailPage`, `ReviewsPage`. Envolver com `<Suspense fallback={<PageSkeleton />}>`
3. Dividir o bundle de vendors: `manualChunks` no Vite para `react`, `react-router`, `tanstack/react-query`, `recharts` (apenas admin, deve carregar somente em rotas de admin)
4. Remover moment/date-fns se ambos estiverem presentes — padronizar em um (date-fns, com tree-shaking)
5. Auditar uso de lodash — substituir por importações por função ou equivalentes nativos

### SPA — imagens
6. Adicionar redimensionamento de imagem baseado em sharp ao fluxo de upload do product-service (`services/product-service/app/services/image_processor.rb`): gerar `thumb` (320w), `md` (640w), `lg` (1280w) em WebP + fallback JPEG; armazenar URLs em `product_images`
7. Elemento `<picture>` em `ProductCard` + `ProductDetailImage`: `<source type="image/webp" srcset="thumb 320w, md 640w, lg 1280w">` + fallback JPEG
8. Blur placeholder: computar um JPEG base64 de 16x16 no upload, armazenar em `product_images.placeholder_data_url`, renderizar como `background-image` até a imagem principal carregar
9. `loading="lazy"` em todas as imagens abaixo da dobra; `fetchpriority="high"` na imagem hero do LCP

### SPA — caminho de renderização crítico
10. Adicionar `vite-plugin-critical` para extração de CSS crítico nas rotas home e PDP; injetar inline no `<head>`
11. Hospedar as duas fontes de exibição localmente (woff2), `font-display: swap`, preload do peso usado acima da dobra
12. `<link rel="preconnect">` para a origem do gateway em `index.html`
13. Remover scripts de terceiros bloqueadores de renderização do `<head>` — deferir ou mover para depois do estado interativo

### Backend — cache + compressão
14. Adicionar `Cache-Control: public, max-age=60, stale-while-revalidate=300` em `GET /api/v1/marketplace/products*` no gateway Go
15. Behaviors de cache do CloudFront por padrão de path (o Terraform do Sprint 23 cobre isso — por enquanto adicionar entrada manual): `/api/v1/marketplace/*` com cache de 60s, `/api/v1/orders/*` sem cache, `/assets/*` imutável por 1 ano
16. O gateway já retorna `Content-Encoding: gzip`; habilitar brotli no CloudFront
17. Gem `bullet` no product-service (ambientes development e test); executar a suíte de testes e corrigir todos os N+1 sinalizados — provavelmente `Product.includes(:images, :seller)` no endpoint de listagem do marketplace
18. Adicionar índices: `products (status, created_at)`, `products (category, status)`, `orders (seller_id, created_at)` — verificar com `EXPLAIN ANALYZE` nas consultas mais frequentes

### CI — orçamentos Lighthouse
19. `.github/workflows/lighthouse-ci.yml` — executado após deploy em staging, reprova PR se algum orçamento cair
20. Orçamentos em `lighthouserc.json`: `performance >= 85`, `accessibility >= 90`, `best-practices >= 85`, `seo >= 90`, `first-contentful-paint <= 1800`, `largest-contentful-paint <= 2500`, `interactive <= 3500`, `total-blocking-time <= 200`, `cumulative-layout-shift <= 0.1`, `resource-summary:script:size <= 200000`
21. Executar contra 3 URLs: `/`, `/produtos`, `/produtos/:id` (usar um slug estável)

## Critérios de aceite

- [ ] Lighthouse ≥ 85 em todas as quatro categorias para home, categoria e PDP (URL de produção, cache frio, emulação mobile)
- [ ] LCP < 2,5s na emulação Slow 4G para home + PDP
- [ ] Bundle JS inicial < 200KB gzipado (rota home)
- [ ] Chunks de admin + checkout carregam apenas quando a rota é acessada (verificado na aba Network)
- [ ] Bullet não reporta N+1 no endpoint de listagem do marketplace nos testes
- [ ] Taxa de cache hit no CloudFront > 80% para `/api/v1/marketplace/*` após 24h de tráfego
- [ ] O gate do Lighthouse CI reprova um PR que faça regressão em qualquer orçamento

## Dependências

- Sprints 01–09 concluídos (funcionalidades de produto estáveis antes de otimizar)
- Sprint 23 (CI/CD) — o step do Lighthouse CI se encaixa ali
- Distribuição CloudFront ativa

## Riscos

- Otimizar um alvo em movimento — agendar este sprint próximo ao lançamento, não cedo demais
- Custo do pipeline de imagens — sharp em processo é suficiente para até ~10 mil novas imagens de produtos por mês; acima disso, migrar para um worker Lambda acionado por eventos S3 put
- Deriva do CSS crítico — o inline pode quebrar quando novos componentes alterem estilos acima da dobra; re-executar `vite-plugin-critical` como parte do build de produção, não manualmente
