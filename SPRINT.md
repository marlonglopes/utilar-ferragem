# Utilar Ferragem — Rastreador de Sprints

Documento vivo que acompanha os sprints ativos e concluídos. Segue o padrão usado no [`../SPRINT.md`](../SPRINT.md) do projeto pai.

## Estado atual

**Fase 3 — Comércio.** Sprints 01–09 concluídos. Sprint 10 (onboarding de vendedor) é o próximo.

## Índice de sprints

| # | Sprint | Fase | Status |
|---|--------|-------|--------|
| 01 | [Scaffold + tooling](docs/sprints/sprint-01-scaffold.md) | 1 — Fundação | ✅ Concluído |
| 02 | [Design system + i18n](docs/sprints/sprint-02-design-system.md) | 1 — Fundação | ✅ Concluído |
| 03 | [Catálogo + taxonomia](docs/sprints/sprint-03-catalog.md) | 2 — Catálogo | ✅ Concluído |
| 04 | [Detalhe do produto (specs JSONB)](docs/sprints/sprint-04-product-detail.md) | 2 — Catálogo | ✅ Concluído |
| 05 | [Busca + filtros (ILIKE)](docs/sprints/sprint-05-search-filters.md) | 2 — Catálogo | ✅ Concluído |
| 06 | [Carrinho (local + persistente)](docs/sprints/sprint-06-cart.md) | 3 — Comércio | ✅ Concluído |
| 07 | [Auth do cliente + conta](docs/sprints/sprint-07-auth.md) | 3 — Comércio | ✅ Concluído |
| 08a | [payment-service Go + Mercado Pago](docs/sprints/sprint-08-checkout.md) | 3 — Comércio | ✅ Concluído |
| 08b | [CheckoutPage SPA (Pix / boleto / cartão)](docs/sprints/sprint-08-checkout.md) | 3 — Comércio | ✅ Concluído |
| 09 | [Histórico de pedidos + rastreio + e-mails](docs/sprints/sprint-09-orders.md) | 3 — Comércio | ✅ Concluído |
| 10 | [Wizard de onboarding de vendedor](docs/sprints/sprint-10-seller-onboarding.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 11 | [Importação em massa de SKUs (CSV)](docs/sprints/sprint-11-bulk-import.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 12 | [Avaliações & notas](docs/sprints/sprint-12-reviews-ratings.md) | 5 — Crescimento | ⬜ Não iniciado |
| 13 | [Contas Pro / B2B (CNPJ)](docs/sprints/sprint-13-pro-accounts.md) | 5 — Crescimento | ⬜ Não iniciado |
| 14 | [Tarifas de frete + rastreio (Melhor Envio)](docs/sprints/sprint-14-shipping-correios.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 15 | [Disputas de pagamento + reembolsos](docs/sprints/sprint-15-payment-disputes.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 16 | [Ferramenta de suporte ao cliente (Freshdesk)](docs/sprints/sprint-16-support-tooling.md) | 5 — Crescimento | ⬜ Não iniciado |
| 17 | [Upgrade de busca (Meilisearch)](docs/sprints/sprint-17-search-upgrade.md) | 5 — Crescimento | ⬜ Não iniciado |
| 18 | [PWA + web push](docs/sprints/sprint-18-pwa-push.md) | 5 — Crescimento | ⬜ Não iniciado |
| 19 | [Recomendações (baseadas em regras)](docs/sprints/sprint-19-recommendations.md) | 5 — Crescimento | ⬜ Não iniciado |
| 20 | [Console administrativo Utilar](docs/sprints/sprint-20-utilar-admin.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 21 | [Otimização de performance](docs/sprints/sprint-21-performance.md) | 5 — Crescimento | ⬜ Não iniciado |
| 22 | [Observabilidade em produção](docs/sprints/sprint-22-observability.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 23 | [CI/CD + Terraform IaC](docs/sprints/sprint-23-ci-cd-iac.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 24 | [Conformidade LGPD](docs/sprints/sprint-24-lgpd.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 25 | [Prontidão para lançamento (SEO, jurídico, e-mail, vendedores)](docs/sprints/sprint-25-launch-readiness.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 26 | [Programa de cashback](docs/sprints/sprint-26-cashback.md) | 5 — Crescimento | ⬜ Não iniciado |

**Legenda**: ⬜ não iniciado • 🚧 ativo • ✅ concluído • ⏸ pausado • 🅒 condicional (aguarda dados ou decisão)

## Ordem recomendada de sprints

As dependências (ver [05-roadmap.md](docs/05-roadmap.md)) sugerem esta ordem dentro de cada fase:

- **Fase 1**: 01 → 02
- **Fase 2**: 03 → 04 → 05
- **Fase 3 comércio**: 06 → 07 → 08 → 09
- **Fase 3 gate de lançamento**: 22 → 23 → 24 → 25 _(pode se sobrepor aos sprints finais de comércio)_
- **Fase 4**: 10 → 11 → 20 → 14 → 15 _(20 pode começar após 10 + 15)_
- **Fase 5**: ordem orientada por dados (ver sinais de priorização no roadmap). Típica: 21 → 16 → 12 → 17 → 18 → 19 → 13

## Protocolo de handoff

Cada sprint termina com:
1. Todos os critérios de aceite no doc do sprint marcados como ✅
2. Uma nota curta neste arquivo sob "Histórico recente"
3. Qualquer follow-up capturado como issue ou novo doc de sprint
4. Tabela de status de integração em [docs/06-integration.md §7](docs/06-integration.md) atualizada
5. Se nova capacidade da plataforma for adicionada, atualizar [../docs/integration-guide.md](../docs/integration-guide.md)

## Histórico recente

**Sprint 02 — Design system + i18n** (2026-04-20): PublicLayout, Navbar, CategoryRail, Footer, LocaleSwitcher, 13 primitivos de UI (Button/Input/Select/Checkbox/Radio/Card/Badge/Tag/Modal/Drawer/Toast/Skeleton/Pagination/Breadcrumb), página `/_dev/ui`, i18n com 4 namespaces (common, catalog, checkout, account), Tailwind dark-mode class, Google Fonts (Archivo/Inter/JetBrains Mono).

**Sprint 03 — Catálogo + taxonomia** (2026-04-20): HomePage com hero, rail de categorias, grid de produtos em destaque, CategoryPage com grid paginado, ProductCard, Breadcrumb, StockBadge, taxonomia completa.

**Sprint 04 — Detalhe do produto** (2026-04-20): ProductDetailPage com ImageGallery, SellerCard, SpecSheet, tabs (Descrição/Specs/Avaliações), QuantitySelector, CTA fixo mobile, produtos relacionados.

**Sprint 05 — Busca + filtros facetados** (2026-04-20): SearchPage, FacetSidebar, ActiveFilterChips, SortDropdown, query params compartilháveis, bottom sheet mobile, estado vazio.

**Sprint 06 — Carrinho** (2026-04-20): cartStore (Zustand persist), CartDrawer, CartPage `/carrinho`, badge na navbar, "Adicionar ao carrinho" funcional no ProductDetailPage.

**Sprint 07 — Auth + conta** (2026-04-22): cpf.ts, authStore expandido, ProtectedRoute, LoginPage, RegisterPage, ForgotPasswordPage, AccountPage (perfil/endereços/CEP autofill), avatar na Navbar.

**Sprint 08a — payment-service Go** (2026-04-22): scaffold Go, docker-compose (Redpanda + Postgres), migrations (payments/webhook_events/payments_outbox), MP client (Pix/boleto/cartão), webhook handler (HMAC + idempotência + outbox), outbox drainer → Redpanda, testes de integração.

**Sprint 08b — CheckoutPage SPA** (2026-04-20): usePayment hook (mock mode + polling real), PixPayment (QR + copia-e-cola + countdown + auto-confirm mock), BoletoPayment (barcode + aviso + vencimento), CardPayment (hosted drop-in + simulate sandbox), CheckoutPage (wizard 3 passos: endereço/frete/pagamento + sidebar resumo), OrderConfirmationPage (Pix/boleto/cartão), rotas `/checkout` (ProtectedRoute) e `/pedido/:id`. 89 testes passando (12 arquivos).

**Sprint 09 — Histórico de pedidos + rastreio** (2026-04-23): mockOrders (4 pedidos: entregue/enviado/pago/aguardando), useOrders + useOrder hooks (mock mode), OrdersTab com filtros all/active/done, OrderDetailPage (timeline 5 passos, itens, endereço, pagamento, rastreamento, cancelar pedido, comprar novamente), i18n completo (orderStatus + orders.* em pt-BR e en). 117 testes passando (15 arquivos).
