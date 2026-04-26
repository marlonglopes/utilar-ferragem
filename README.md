# Utilar Ferragem

Um marketplace especializado em ferramentas e materiais de construção, construído sobre a plataforma **product-gateway** da Gifthy — pense em um Home Depot para o Brasil, com modelo de marketplace multi-vendedor.

## O que é esta pasta

Este diretório contém o **planejamento, design e documentação** do app Utilar Ferragem. A implementação ainda não começou — este é um artefato de planejamento com o objetivo de alinhar as partes antes de qualquer scaffold de código.

Inspiração / referência de branding: [@utilar_ferragens no Instagram](https://www.instagram.com/utilar_ferragens/).

## Navegação

| Documento | Finalidade |
|----------|---------|
| **[mockups/index.html](mockups/index.html)** | **Deck de design visual — abra no navegador para revisão com o cliente** |
| [docs/01-vision.md](docs/01-vision.md) | Por que este app existe, usuários-alvo, métricas de sucesso |
| [docs/02-branding.md](docs/02-branding.md) | Identidade visual, paleta, tipografia, voz da marca |
| [docs/03-architecture.md](docs/03-architecture.md) | Stack tecnológica, integração com product-gateway, deploy |
| [docs/04-product-scope.md](docs/04-product-scope.md) | Features, fluxos de usuário, taxonomia de categorias |
| [docs/05-roadmap.md](docs/05-roadmap.md) | Fases, 25 sprints, dependências, critérios de gate |
| [docs/06-integration.md](docs/06-integration.md) | Checklist de integração específica da Utilar (referencia [../docs/integration-guide.md](../docs/integration-guide.md)) |
| [docs/07-data-model.md](docs/07-data-model.md) | Schema, migrações, índices, formato JSONB de specs |
| [docs/08-security.md](docs/08-security.md) | LGPD, rate limits, segredos, postura PCI, endurecimento de autenticação |
| [docs/09-observability.md](docs/09-observability.md) | Logs, métricas, traces, alertas, SLOs, dashboards |
| [docs/10-testing-strategy.md](docs/10-testing-strategy.md) | Pirâmide unitário / integração / E2E / carga / a11y |
| [docs/11-infra.md](docs/11-infra.md) | DNS, SSL, CDN, Terraform, CI/CD, staging, rollback |
| [docs/12-ops-runbook.md](docs/12-ops-runbook.md) | Incidentes, plantão, backups, recuperação de desastre |
| [docs/13-launch-checklist.md](docs/13-launch-checklist.md) | Linha do tempo T-minus, jurídico, SEO, e-mail, marketing |
| [docs/14-infra-custos.md](docs/14-infra-custos.md) | Infraestrutura mínima, custos por fase, domínio, AWS, SES, Mercado Pago, observabilidade |
| [docs/maintenance/database.md](docs/maintenance/database.md) | Postgres local (payment + catalog + order + auth): migrations, seed, reset, dumps, comandos `make db-*`, `make catalog-db-*`, `make order-db-*`, `make auth-db-*` |
| [docs/security/payment-service-audit-2026-04-24.md](docs/security/payment-service-audit-2026-04-24.md) | ⛔ **Audit de segurança do payment-service** — 5 vulnerabilidades críticas, plano de remediação Sprint 8.5 (BLOQUEIA prod) |
| [docs/phases/](docs/phases/) | Detalhamento por fase (5 fases) |
| [docs/sprints/](docs/sprints/) | Escopo por sprint, tarefas, critérios de aceite (25 sprints) |
| [docs/adr/](docs/adr/) | Architecture Decision Records (10 ADRs) |
| [SPRINT.md](SPRINT.md) | Rastreador de sprints ao vivo (atualizado conforme o trabalho avança) |

## Deck de mockups visuais

Sete telas HTML clicáveis mostrando a UI proposta em alta fidelidade. Abra qualquer uma no navegador:

- [01 Home](mockups/01-home.html) — hero, categorias, produtos em destaque, barra de confiança
- [02 Categoria](mockups/02-category.html) — grade, filtros facetados, atributos de comércio
- [03 Detalhe do produto](mockups/03-product-detail.html) — galeria, specs, estoque, vendedor, buy box
- [04 Carrinho](mockups/04-cart.html) — itens agrupados por vendedor, resumo, desconto Pix
- [05 Checkout](mockups/05-checkout.html) — endereço, frete, Pix/boleto/cartão
- [06 Pedidos](mockups/06-account-orders.html) — linha do tempo de status, rastreio, repetir pedido
- [07 Onboarding de vendedor](mockups/07-seller-onboarding.html) — wizard de 6 etapas com validação de CNPJ

Para apresentar ao cliente: `open utilar-ferragem/mockups/index.html` (ou sirva a pasta com qualquer servidor web estático).

## Status

**Sprints 01–09 ✅ concluídos.** Frontend SPA completo + **4 serviços Go** em operação local, **zero mocks no backend**:

- **[payment-service](services/payment-service/README.md)** — abstração `psp.Gateway` plugável (Stripe ✅ / Mercado Pago ✅) + webhooks + outbox (Sprint 08, porta :8090)
- **[catalog-service](services/catalog-service/README.md)** — produtos, categorias, vendedores, imagens (Fase B1, porta :8091)
- **[order-service](services/order-service/README.md)** — pedidos, items, endereços, tracking (Fase B2, porta :8092)
- **[auth-service](services/auth-service/README.md)** — users, addresses, argon2id, JWT HS256 (Fase B3, porta :8093)

Frontend plugado em todos os serviços via `VITE_AUTH_URL`, `VITE_CATALOG_URL`, `VITE_ORDER_URL`, `VITE_API_URL` + `VITE_STRIPE_PUBLISHABLE_KEY` (Stripe Elements no checkout). **Operação fim-a-fim real:** login → catálogo → carrinho → pedido → pagamento (cartão confirmado in-app via `<PaymentElement>`, sem redirect).

### Desenvolvimento — atalhos

```bash
make dev              # SPA em mock mode (sem backend)
make dev-catalog      # infra + catalog-service + SPA (catálogo live)
make dev-full         # infra + payment + catalog + order + auth + SPA (tudo live)
make test             # test suite do frontend (152 testes — inclui Stripe Elements)
make auth-test        # testes do auth-service (22 testes)
make catalog-test     # testes do catalog-service (16 testes)
make order-test       # testes do order-service (8 testes)
make svc-test         # testes do payment-service (PSP gateway + Stripe + MP)
```

Login em dev: `test1@utilar.com.br` / `utilar123` (ver [auth seed](services/auth-service/README.md#seed) para a lista completa de 20 users).

Gestão de banco (migrations, seed, reset, dumps): ver [docs/maintenance/database.md](docs/maintenance/database.md).

### Plano em resumo

- **Fase 1 — Fundação** (Sprints 01–02 ✅): scaffold, design system, i18n
- **Fase 2 — Catálogo** (Sprints 03–05 ✅): home, PDP com specs JSONB, busca + filtros
- **Fase 3 — Comércio + lançamento** (Sprints 06–09 ✅, 22–25): carrinho, autenticação, checkout (Pix/boleto/cartão), pedidos, observabilidade, CI/CD, LGPD, lançamento
- **Fase 4 — Vertical de vendedores** (Sprints 10, 11, 14, 15, 20): onboarding, importação em massa, frete, disputas, console administrativo
- **Fase 5 — Crescimento** (Sprints 12, 13, 16, 17, 18, 19, 21): avaliações, contas pro, suporte, upgrade de busca, PWA, recomendações, performance

Tabela completa de sprints + dependências: [docs/05-roadmap.md](docs/05-roadmap.md). Rastreador ao vivo: [SPRINT.md](SPRINT.md).

## Plataforma pai

- Raiz do repositório: [`/`](../)
- Serviços de backend: [`../services/`](../services/)
- Frontend irmão (hub de vendedores): [`../gifthy-hub/`](../gifthy-hub/)
- Visão geral da plataforma: [`../CLAUDE.md`](../CLAUDE.md), [`../ARCHITECTURE.md`](../ARCHITECTURE.md)
