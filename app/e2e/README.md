# E2E — Playwright

Testes end-to-end da SPA Utilar Ferragem cobrindo os fluxos de usuário.

## Como rodar

```bash
make e2e            # da raiz do repo — sobe o server e roda tudo
# ou dentro de app/:
npm run test:e2e            # headless, chromium + mobile
npm run test:e2e -- --project=chromium   # só desktop
npm run test:e2e:ui        # modo interativo (debug visual)
npm run test:e2e:report    # abre o último relatório HTML
```

Não precisa de backend: o Playwright sobe a SPA em **modo mock** (porta 5180, sem
`VITE_*_URL`), servindo os dados de `src/lib/mock*.ts`. Determinístico e pronto pra CI.

## Cobertura

| Spec | Feature |
|------|---------|
| `smoke.spec.ts` | App carrega, rotas públicas, 404, design system |
| `catalog.spec.ts` | Vitrine, categoria, abrir produto |
| `search.spec.ts` | Busca por URL, busca pela navbar, estado vazio |
| `product.spec.ts` | Detalhe: nome/preço/CTA, abas, produto inexistente |
| `cart.spec.ts` | Carrinho vazio, adicionar, badge, remover |
| `auth.spec.ts` | Login, rota protegida + `next`, cadastro, esqueci senha |
| `checkout.spec.ts` | Rota protegida, fluxo login→carrinho→checkout, métodos de pagamento |
| `account.spec.ts` | Conta após login, abas, logout |

`helpers.ts` centraliza rotas, credenciais e ações reutilizáveis (`login`,
`openFirstProduct`, `addToCartFromDetail`, `cartCount`).

## Rodar contra a stack real (live)

Suba os serviços (`make dev-full`) e aponte o Playwright pra SPA live, pulando o
webServer embutido:

```bash
E2E_BASE_URL=http://localhost:5175 npm run test:e2e -- --project=chromium
```

Em modo live as credenciais do seed valem (`test1@utilar.com.br` / `utilar123`).

## Ainda não coberto

- **Ingestão de produtos (admin)**: hoje é só API backend (sem UI). Coberto pelos
  testes Go (`services/catalog-service`) + smoke via curl. O E2E entra quando a
  página admin (upload CSV + edição de preço/estoque) for construída (Fase B).
