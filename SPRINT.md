# Utilar Ferragem — Rastreador de Sprints

Documento vivo que acompanha os sprints ativos e concluídos. Segue o padrão usado no [`../SPRINT.md`](../SPRINT.md) do projeto pai.

## Estado atual

**Fase 3 — Comércio.** Sprints 01–09 concluídos + **Phases B1 + B2 + B3** entregues em 2026-04-24. Stack completa sem mocks no backend: catálogo, pedidos, autenticação e pagamentos são serviços Go reais. Próximo: **Sprint 22 (observabilidade)** — gate de lançamento.

### Fases fora do roadmap original

Serviços backend reais criados fora da numeração de sprints (decisão arquitetural de 2026-04-24 — migração progressiva mock → live):

| Phase | Escopo | Status |
|---|---|---|
| **B1** | catalog-service (Go + Postgres): products, categories, sellers, images + 111 seed rows + SPA plugada + endpoint `related` | ✅ Concluído |
| **B2** | order-service (Go + Postgres): orders, order_items, shipping_addresses, tracking_events + 60 seed + SPA plugada | ✅ Concluído |
| **B3** | auth-service (Go + Postgres): users, addresses, argon2id, JWT HS256, refresh tokens + 20 seed users + SPA plugada + order-service aceitando JWT | ✅ Concluído |
| **8.5** | **Payment Hardening** — Fase 1 ✅ 2026-04-27 (5 CRITICALs); Fase 2 plano consolidado em [security-roadmap](docs/security/security-roadmap.md) (11 HIGHs em 4 bundles, ~19h) | 🟡 Fase 1 done; Fase 2 plano pronto |

### Polish transversal (2026-04-24)

Foundations aplicadas em catalog + payment + order:
- **Request ID middleware** (`X-Request-Id`) gerando/propagando IDs para correlação entre serviços.
- **Structured access log JSON** via slog em toda requisição.
- **Error envelope consistente** `{error, code, requestId}` com códigos estáveis (`not_found`, `bad_request`, `unauthorized`, `conflict`, `db_error`, ...).
- **CORS** aberto em dev (produção fica restrito via CloudFront + resposta do serviço).

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

**Phase B1 — catalog-service + SPA plugada** (2026-04-24): scaffold Go espelhando payment-service (cmd/server, internal/{config,db,handler,model}, migrations); 4 tabelas (categories, sellers, products, product_images) + ENUM + pg_trgm + triggers updated_at; seed com 111 produtos (31 reais dos mocks + 80 sintéticos) + 222 imagens + 11 sellers; endpoints `/api/v1/{categories,sellers,products,products/:slug,products/facets}` em `:8091`, CORS aberto para dev; JSON em camelCase matching direto com `app/src/types/product.ts`; hooks `useProducts`/`useProduct`/`useFacets` com switch mock/live via `VITE_CATALOG_URL`; docker-compose estendido (postgres-catalog :5436); 12 novos Makefile targets (`catalog-*`, `dev-catalog`); 16 testes Go (4 unit + 12 integration); docs: README do serviço + seção de migrations em database.md. 131 testes frontend passam em mock mode.

**Phase B2 — order-service + polish transversal** (2026-04-24): scaffold Go (cmd/server, internal/*, migrations); 4 tabelas (orders, order_items, shipping_addresses, tracking_events) + 2 ENUMs (order_status, payment_method) + triggers updated_at; seed com 60 pedidos × 20 usuários em todos os status + 120 items + 60 endereços + 170 tracking events; endpoints `/api/v1/orders` (POST/GET/GET :id/PATCH :id/cancel) em `:8092`; auth via header `X-User-Id` (temporário até Phase B3); error envelope consistente `{error, code, requestId}`; hooks `useOrders`/`useOrder` plugados via `VITE_ORDER_URL` + client helpers `orderGet/orderPost/orderPatch` em `app/src/lib/api.ts`; docker-compose estendido (postgres-order :5437); 12 novos Makefile targets (`order-*`); 8 testes Go de integração (auth, list, filters, get, create, cancel, conflict, bad_request); dev-full sobe 3 serviços + SPA em paralelo. **Polish transversal aplicado em catalog + payment + order:** Request ID middleware (gera/propaga `X-Request-Id`), structured JSON access log via slog, error envelope compartilhado, CORS padronizado. catalog ganhou endpoint `GET /products/:slug/related` usado pelo ProductDetailPage. 131 testes frontend + 16 catalog + 8 order + payment unit tests passando.

**Phase B3 — auth-service + cross-service JWT** (2026-04-24): scaffold Go (cmd/server + cmd/hash tool, internal/{auth,config,db,handler,model}, migrations); 5 tabelas (users, addresses, email_verification_tokens, password_reset_tokens, refresh_tokens) + ENUM user_role + triggers; seed com 20 users (senha universal `utilar123` via hash argon2id embutido) + 29 addresses (17 customers, 2 sellers, 1 admin); argon2id com params OWASP 2023 (m=19MiB, t=2); JWT HS256 com claims `sub`/`email`/`role`/`exp`/`iat`/`iss`, access TTL 15min, refresh token opaco TTL 30d em DB, revogável no logout/reset; 10 endpoints em `:8093`: `POST /auth/register|login|refresh|forgot-password|reset-password|verify-email` (públicos) + `GET /me`, `POST /auth/logout`, addresses CRUD (protegidos por JWTAuth middleware); order-service ganhou JWT parsing em `RequireUser(secret)` com fallback para X-User-Id; docker-compose estendido (postgres-auth :5438); 13 novos targets Makefile `auth-*` + `dev-full` sobe os 4 serviços; 22 testes Go (5 password hashing + 4 JWT + 13 integration: register/login/me/refresh/addresses); frontend: authStore com refreshToken, `authPost`/`authGet` + `orderXxxWithJWT` helpers em api.ts, LoginPage/RegisterPage migradas para `VITE_AUTH_URL`, useOrders roteia JWT quando auth-service está ligado. E2E validado: login test1@utilar.com.br → JWT → order-service aceita → cria pedido real. 131 testes frontend + 16 catalog + 8 order + 22 auth + payment unit tests passando.

**Audit de segurança payment-service** (2026-04-24): revisão manual linha-a-linha de todo payment-service identificou **5 críticas + 5 altas + 6 médias + 3 baixas**. Payment-service marcado como ⛔ BLOQUEADO PARA PRODUÇÃO. Documento completo em [docs/security/payment-service-audit-2026-04-24.md](docs/security/payment-service-audit-2026-04-24.md) com exploit scenario + remediação em código + plano Sprint 8.5 (~16h, 2 dias). Críticas (todas permitem fraude direta): C1 tamper de amount, C2 ownership de order_id, C3 webhook sem validar amount vs MP, C4 HMAC implementado fora do formato MP, C5 fail-closed ausente. Sprint 8.5 pendente.

**Teste de integração MP sandbox** (2026-04-24): validação parcial com credenciais de teste fornecidas pelo usuário. Resultados em [docs/security/mp-integration-test-2026-04-24.md](docs/security/mp-integration-test-2026-04-24.md). ✅ **Cartão via Checkout Pro funcional fim-a-fim** — `POST /v1/payments method=card` retorna `sandbox_init_point` válido, abre checkout real MP, aceita cartões de teste. 🟡 **Pix/Boleto diretos bloqueados** por limitação conhecida do sandbox (`POST /v1/payments` rejeita test users que não passaram por onboarding de Checkout API). Mitigação: migrar Pix/Boleto pra Preferences API (~30min). Durante o teste: `.env.local` configurado com credenciais da aba "Teste" do dashboard MP (test app `3355899843628859`), Makefile payment-service agora inclui `.env.local` automaticamente, validação boleto (requer `payer_cpf`+`payer_name`), bug de scan NULL em `psp_metadata`/`psp_payload` descoberto e corrigido (trocado para `*json.RawMessage`), novo endpoint `POST /api/v1/payments/:id/sync` implementado como workaround de webhook em dev (chama `mp.GetPayment`, compara amount local vs MP — foundation da issue C3 do audit). Test buyers criados via API: `test_user_303903997142113642@testuser.com` / `6WoiM78AFX` e `test_user_7016085891382039787@testuser.com` / `wyw51IhglH`.

**PSP gateway abstraction + Stripe** (2026-04-24/26): introduzida camada `psp.Gateway` (`Name`, `CreatePayment`, `GetPayment`, `VerifyWebhook`, `ParseWebhookEvent`) em [services/payment-service/internal/psp/gateway.go](services/payment-service/internal/psp/gateway.go), permitindo trocar PSP via env var `PSP_PROVIDER=stripe|mercadopago`. Implementado `internal/psp/stripe/gateway.go` com stripe-go/v79 (PaymentIntents API, currency=brl, amount em centavos, mapeamento de status normalizados, `extractClientData` empacotando `next_action.{pix_display_qr_code,boleto_display_details}` pro frontend). Handler `POST /api/v1/payments` agora consome `psp.Gateway` em vez do client direto, expondo `clientSecret` na resposta. Sync handler valida amount local vs PSP (foundation C3). Validado E2E com conta Stripe BR (`acct_1TQFpiLQCtijFcSY`) test mode: ✅ Cartão funcional, ✅ Boleto funcional, 🟡 Pix exige onboarding completo (`details_submitted=true`) — bloqueador no dashboard, não no código. Documentado em [docs/security/stripe-integration-test-2026-04-24.md](docs/security/stripe-integration-test-2026-04-24.md). Test users seedados continuam válidos (senha `utilar123`).

**Quick wins de hardening transversal** (2026-04-27): após Fase 1 da Sprint 8.5, aplicados 3 fixes baixo-risco e alto-impacto que não dependem de Redis/migrations: (1) **SecurityHeaders middleware** em todos os 4 services (auth/order/catalog/payment) com `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'none'`, `Strict-Transport-Security`, `Referrer-Policy`; (2) **CORS via `ALLOWED_ORIGINS` env var** (vírgula-separada) em vez de `*` hardcoded — vazio = wildcard pra dev, lista = whitelist pra prod com `Vary: Origin` header; (3) **tokens em logs gated por DevMode** — `slog.Info("...token...")` no auth-service só roda em `cfg.DevMode=true`. Roadmap detalhado dos HIGHs/MEDIUMs ainda em aberto: [docs/security/security-roadmap.md](docs/security/security-roadmap.md). **Restam ~22h de hardening operacional** (rate limit/Redis, idempotency-key, token hashing/migration, cross-service price validation, etc).

**Sprint 8.5 — Payment hardening Fase 1 (5 CRITICALs fechados)** (2026-04-27): todos os 5 CRITICALs do [audit do payment-service](docs/security/payment-service-audit-2026-04-24.md) endereçados. Mudanças: (1) **C1+C2 — cross-service amount/ownership**: novo package [`internal/orderclient`](services/payment-service/internal/orderclient/) faz `GET /api/v1/orders/:id` no order-service propagando o JWT do cliente; `PaymentHandler.Create` agora deriva amount autoritativo de `order.total` (body amount vira hint, logado se diverge) + 404 se `order.userId ≠ jwt.sub` (defesa em profundidade); fail-closed em prod se `orderClient` for nil; (2) **C3 — webhook valida amount via PSP**: [webhook handler reescrito](services/payment-service/internal/handler/webhook.go) provider-agnostic usando `psp.Gateway` (`/webhooks/:provider`); chama `gateway.GetPayment(pspID)` ANTES de promover status; mismatch → flag `psp_metadata.amount_mismatch=true` + outbox event `payment.fraud_suspect` + status fica pending (revisão manual); idempotência preservada via `(psp_id, psp_payment_id, event_type)`; (3) **C4 — HMAC MP no formato V2 oficial**: `parseMPSignatureHeader` parsa `ts=X,v1=Y`, manifest `id:<data.id>;request-id:<x-request-id>;ts:<ts>;`, HMAC-SHA256 + `hmac.Equal` constant-time, replay window de 5min, suporte a body V2 e legacy `resource` URL; (4) **C5 — fail-closed completo**: `config.Load` em prod (DEV_MODE=false) recusa subir sem `STRIPE_WEBHOOK_SECRET`/`MP_WEBHOOK_SECRET` (PSP-condicional). Webhook handler agora generic: `/webhooks/:provider` em vez de `/webhooks/mp` — `:provider` precisa bater com `gateway.Name()` ou 404 (anti-enumeration). 50+ novos testes Go: 8 config (fail-closed), 4 webhook integration (incl. amount mismatch), 7 payment_security (cross-service), 7 orderclient, 23 MP webhook (formato V2 + replay + malformed). Status: payment-service tecnicamente desbloqueado pra prod; HIGH (Idempotency-Key, rate limit, CORS whitelist) ainda recomendados antes de tráfego significativo (Sprint 8.5 Fase 2). **Resultado: 152/152 frontend + ~120 backend Go = 272+ testes verdes.**

**Full security audit + remediação cross-services** (2026-04-26): audit linha-a-linha dos 4 serviços Go (auth, order, catalog + revisão do payment audit prévio). Total de findings: **14 CRITICAL + 19 HIGH + 22 MEDIUM + 14 LOW = 69**. Documentado em [docs/security/full-audit-2026-04-26.md](docs/security/full-audit-2026-04-26.md). **9 CRITICALs novos fechados nesta sessão** (auth/order/catalog) + **3 HIGHs bonus** (transversais) + 1 MEDIUM. Mudanças principais: (1) `JWT_SECRET` fail-closed em auth/order/payment — recusa subir em prod sem secret de 32+ chars não-default (gated por nova flag `DEV_MODE`); (2) `DBError` sanitizado nos 4 serviços (log interno + msg genérica em vez de `err.Error()` no body); (3) `randToken()` em auth panic em erro de rand.Read (preferir crash a token fraco); (4) `/auth/refresh` com rotação obrigatória do refresh token (revoga atual + emite novo, transacional — fecha janela de 30d pra atacante após logout); (5) `OrderItem`/`OrderAddress` com binding tags (Quantity gt=0,lte=999, UnitPrice gt=0,lte=999999.99, max= em strings, len=2 em UF, max=100 itens) — bloqueia preço negativo/absurdo no body; (6) `RequireUser` middleware do order recusa X-User-Id fallback em prod (`devMode=false`); (7) `escapeLikePattern()` + `ESCAPE '\'` em todas as queries ILIKE do catalog — bloqueia ReDoS via wildcard injection; (8) `price_min/max < 0` rejeitados no parse; (9) sort whitelist explícita. Adicionados ~17 testes Go de regressão de segurança em `internal/config/config_test.go` (auth+order) + `middleware_security_test.go` (order) + `security_test.go` (catalog). Restantes pra Sprint 8.5 + backlog: 5 CRITICALs em payment (tamper amount, ownership, webhook amount, HMAC MP, fail-closed) + rate limiting (precisa Redis) + token hashing (precisa migration) + security headers + CORS restritivo via env. **152/152 frontend + ~88 backend Go = 240+ testes verdes.**

**Stripe Elements no frontend SPA** (2026-04-26): cartão confirmado in-app sem redirect via `<PaymentElement>`. Adicionado `app/src/lib/stripe.ts` (singleton `loadStripe()`); `usePayment` refatorado com `parseStripeResult`/`parseMercadoPagoResult` branchando por `provider` na resposta da API, expõe `clientSecret`, `markConfirmed`, `markFailed`; `CardPayment.tsx` usa `<Elements stripe={stripePromise} options={{ clientSecret }}>` + `stripe.confirmPayment({ redirect: 'if_required' })` no path Stripe e mantém o redirect MP no fallback; `BoletoPayment.tsx` ganhou link pro `hosted_voucher_url` da Stripe (página HTML imprimível) além do PDF; `CheckoutPage.tsx` coleta CPF+Nome quando method=boleto, gera UUID v4 (via `crypto.randomUUID()`) pro `order_id` (backend valida UUID), e só navega após `succeeded` no path cartão; i18n PT-BR + EN atualizados; novos testes adicionados — `usePayment.stripe.test.ts` (8 testes — parsers Stripe e MP, propagação de extras boleto, marker functions), `BoletoPayment.test.tsx` (6 testes — Stripe vs MP variants, mock mode), `CardPayment.test.tsx` (7 testes — Elements path com `@stripe/react-stripe-js` mockado, MP redirect fallback, branch confirmed) + Stripe gateway Go (`internal/psp/stripe/gateway_test.go` — 17 subtests: normalizeStatus, validation rules, webhook verify, ParseWebhookEvent). Bug fix: `vite.config.ts` agora zera `VITE_*_URL` e `VITE_STRIPE_PUBLISHABLE_KEY` em test mode (antes só `VITE_API_URL` era zerada — adicionar URLs ao `.env.local` vazava pros tests fazendo `isOrderEnabled/isAuthEnabled=true` e quebrando hooks mock). PCI scope = SAQ-A (campos sensíveis ficam no iframe Stripe). Documentado em [services/payment-service/README.md](services/payment-service/README.md). **152/152 testes frontend passam + 71 testes Go (4 serviços) passam = 223 testes verdes.**
