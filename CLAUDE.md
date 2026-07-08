# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is this?

Utilar Ferragem is a white-label marketplace for tools and construction materials. React SPA frontend + 4 Go microservices, each with its own Postgres database. Part of the gifthy monorepo but operates independently (no shared API or NATS bus with gifthy core).

## Commands

All commands run from the repo root via `Makefile`. Run `make help` for the full list.

### Development

```bash
make dev            # SPA only, mock mode (no backend needed), port 5175
make dev-full       # All 4 Go services + infra + SPA
make dev-catalog    # Infra + catalog-service + SPA (catalog live, payment mock)
make dev-live       # SPA pointing at running backends (set API_URL, CATALOG_URL, etc.)
```

### Testing

```bash
make test           # Frontend tests (vitest, single run)
make test-watch     # Frontend tests (watch mode)
make svc-test       # Payment-service Go tests
make catalog-test   # Catalog-service Go tests
make order-test     # Order-service Go tests
make auth-test      # Auth-service Go tests
```

Run a single frontend test: `cd app && npx vitest run src/path/to/file.test.ts`

### Lint & Format

```bash
cd app && npm run lint        # ESLint (max-warnings: 0)
cd app && npm run lint:fix    # ESLint auto-fix
cd app && npm run format      # Prettier
cd app && npm run build       # TypeScript check + Vite bundle
```

### Infrastructure

```bash
make infra-up       # Docker Compose: 4x Postgres, Redis, Redpanda, Console
make infra-down     # Stop all containers
make infra-status   # Container status
```

### Database (per service)

Pattern: `make <prefix>db-{migrate,seed,reset,psql,status,dump,restore}`

| Service | Prefix | Port | DB name |
|---------|--------|------|---------|
| payment | `db-` | 5435 | payment_service |
| catalog | `catalog-db-` | 5436 | catalog_service |
| order | `order-db-` | 5437 | order_service |
| auth | `auth-db-` | 5438 | auth_service |

Example: `make auth-db-reset` (drop + migrate + seed with 20 test users, password: `utilar123`)

## Architecture

```
React SPA (Vite, :5175)
  │ HTTP + JWT Bearer
  ├── Auth Service    (Go/Gin, :8093) ── Postgres :5438
  ├── Catalog Service (Go/Gin, :8091) ── Postgres :5436
  ├── Order Service   (Go/Gin, :8092) ── Postgres :5437
  └── Payment Service (Go/Gin, :8090) ── Postgres :5435 ── Redpanda (outbox)
```

- **4 separate Postgres databases** — one per service, no cross-service DB access
- **Redpanda** (Kafka-compatible) used by payment-service transactional outbox pattern
- **Redis** available at :6379 for cache/sessions
- Services share `JWT_SECRET` for token validation; auth-service issues tokens

### Frontend stack

React 18, TypeScript (strict), Vite 5, Tailwind 3, React Router 6, Zustand 4, TanStack Query 5, i18next (pt-BR primary), Vitest + React Testing Library, @stripe/react-stripe-js.

Import alias: `@/` maps to `app/src/`.

### Backend stack

Go 1.26, Gin 1.12, lib/pq, golang-migrate. Go workspace (`go.work`) unifies 4 services + `pkg/` shared utilities. Each service follows `cmd/server/main.go` + `internal/{config,db,handler,model}/` + `migrations/` layout.

## Key conventions

**Mock mode**: When `VITE_*_URL` env vars are empty, the frontend returns mock data from `src/lib/mock*.ts`. Tests always run in mock mode (forced in `vite.config.ts`).

**API layer** (`src/lib/api.ts`): Custom fetch wrapper with JWT injection and auto-refresh on 401. Separate functions per service: `apiGet/apiPost` (payment), `catalogGet` (catalog), `orderGet/orderPost` (order), `authPost` (auth).

**State**: Zustand stores (auth, cart, address, locale) with localStorage persistence. Server state via TanStack Query.

**Error envelope** (backend): `{error, code, requestId, details?}` — codes: `not_found`, `bad_request`, `unauthorized`, `conflict`, `validation_error`, `db_error`.

**Routes use pt-BR slugs**: `/categoria/:slug`, `/produto/:slug`, `/carrinho`, `/entrar`, `/cadastro`, `/esqueci-senha`, `/conta`, `/checkout`, `/pedido/:id`.

**Brand colors** (Tailwind): `brand.orange` (#F47920), `brand.blue` (#1B3E8A), `brand.gold` (#F5A623). Fonts: Inter (body), Archivo (headings), JetBrains Mono (code).

**Pre-commit hook**: Husky + lint-staged runs ESLint fix + Prettier on staged `.ts/.tsx` files.

**PSP provider**: Configured via `PSP_PROVIDER` env var (`stripe` or `mercadopago`). Payment-service abstracts both behind the same API surface.

## Project structure

```
app/                          # React SPA
  src/
    pages/                    # Route components (home, category, product, checkout, etc.)
    components/
      ui/                     # 13 design-system primitives (Button, Input, Modal, etc.)
      layout/                 # PublicLayout, Navbar, Footer, CategoryRail
      catalog/                # ProductCard, SpecSheet, StockBadge, ImageGallery
      checkout/               # PaymentMethodPicker, PixPayment, BoletoPayment, CardPayment
      auth/                   # ProtectedRoute, LoginForm, RegisterForm
      cart/                   # CartDrawer
    hooks/                    # usePayment, useOrders, useProducts, useFacets
    store/                    # Zustand stores (authStore, cartStore, addressStore, localeStore)
    lib/                      # api.ts, format.ts, taxonomy.ts, mock data
    types/                    # API response interfaces
    i18n/                     # Translations (pt-BR, en)
    router/                   # React Router 6 config
    test/                     # Vitest setup + test files

services/
  auth-service/               # Users, JWT, addresses, password reset
  catalog-service/            # Products, categories, sellers (111 seed products)
  order-service/              # Orders, items, tracking events
  payment-service/            # Pix/boleto/card, PSP abstraction, outbox

pkg/                          # Shared Go: requestid, idempotency, ratelimit, httpclient
docs/                         # 13 architecture docs + ADRs + sprint/phase breakdowns
```

## Things to know

- **Payment-service production readiness** — originally blocked by 5 critical vulnerabilities from the security audit. `docs/security/` holds the full trail: `payment-service-audit-2026-04-24.md` (original findings), `full-audit-2026-04-26.md`, dated sweeps/verifications, and `security-roadmap.md` (the live remediation tracker — check it for current status before assuming prod-blocked).
- **Secrets & PSP config**: `.env.local` (and `.creds`) hold PSP keys (`STRIPE_*`, `MP_*`), `JWT_SECRET`, and DB URLs. When `VITE_*_URL` vars are unset, the frontend runs in mock mode — no secrets needed.
- Design system showcase lives at `/_dev/ui` route in the running app.
- Node version pinned in `.nvmrc` (v20). Go version in `go.work` (1.26.2).
- Seed data: 111 products, 65 sellers, 8 categories, 60 orders, 20 users.
- Service READMEs in `services/*/README.md` have detailed API docs.
- **Live trackers**: `SPRINT.md` (sprint progress, 25 sprints across 5 phases), `PRICING.md` (pricing model), `docs/phases/` and `docs/sprints/` for scoped breakdowns.
- Git hooks: pre-commit runs via both `.husky/` and `.githooks/` (lint-staged: ESLint fix + Prettier on staged files).
