---
name: test-utilar
description: "Roda a suíte de testes do Utilar Ferragem — backend (4 serviços Go: catalog, order, auth, payment), frontend unit (vitest) e E2E (Playwright). Use quando o usuário pedir para rodar testes, testar as features, validar o Utilar, checar se está tudo verde, rodar a suíte, testar backend/frontend/e2e, ou verificar regressões antes de commit/deploy."
---

# Testar o Utilar Ferragem

Executa toda a pirâmide de testes do Utilar e reporta um resumo consolidado.

## Como usar

Rode o script conforme o escopo pedido:

```bash
# tudo (backend + frontend unit + e2e)
.claude/skills/test-utilar/run-tests.sh

# só uma camada
.claude/skills/test-utilar/run-tests.sh backend    # os 4 serviços Go
.claude/skills/test-utilar/run-tests.sh frontend   # vitest (unit/component)
.claude/skills/test-utilar/run-tests.sh e2e        # Playwright (sobe SPA mock)

# um serviço específico
.claude/skills/test-utilar/run-tests.sh catalog    # ou order | auth | payment
```

O script sai com código ≠ 0 se qualquer camada falhar, e imprime um resumo
(✅/❌ por camada) no final.

## O que cada camada cobre

- **backend** — `go test ./...` em cada serviço:
  - `catalog` — produtos/busca/facets, ingestão admin (CRUD + import CSV), auth admin
  - `order` — pedidos, pricing, cap de itens, JWT middleware
  - `auth` — login/registro, JWT, argon2id, CPF, token hashing
  - `payment` — PSP (Appmax/Stripe/MP), webhooks, redação de PII, fail-closed
- **frontend** — vitest (`app`): stores, hooks, páginas, componentes de checkout
- **e2e** — Playwright (`app/e2e`): storefront ponta a ponta (catálogo, busca,
  carrinho, auth, checkout, conta) em chromium + mobile, modo mock

## Pré-requisitos

- **Go** no PATH (`/usr/local/go/bin`) — o script já adiciona.
- **Node** + deps instaladas (`cd app && npm install`). Para E2E, os browsers do
  Playwright (`npx playwright install chromium`).
- **Postgres** para os testes de integração backend: `make infra-up` e, se
  necessário, `make catalog-db-reset` / `order-db-reset` / `auth-db-reset` /
  `db-reset`. Sem o banco, os testes de integração **SKIPam** (não falham) e o
  runner avisa.

## Equivalentes via Makefile

`make test` (vitest) · `make e2e` (Playwright) · `make catalog-test` /
`order-test` / `auth-test` / `svc-test` (payment).

## Ao concluir

Reporte ao usuário o resumo por camada e, se houver falhas, os testes que
quebraram (nome + mensagem), não o log inteiro.
