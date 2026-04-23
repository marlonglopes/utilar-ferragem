# 03 — Arquitetura

## Visão geral

```
                                ┌──────────────────────────────────┐
                                │       utilar-ferragem (SPA)      │
                                │   React + Vite + TS + Tailwind   │
                                └──────────────┬───────────────────┘
                                               │ HTTPS (JSON, JWT)
                                               ▼
                                ┌──────────────────────────────────┐
                                │     Go API gateway :8080         │
                                │   (existente — services/gateway) │
                                └──────────────┬───────────────────┘
            ┌────────────────┬─────────────────┼───────────────────┬──────────────────┐
            ▼                ▼                 ▼                   ▼                  ▼
     user-service    product-service    inventory-service    order-service     (futuro)
       :3001             :3002               :3003              :3004         payment-service
```

A Utilar Ferragem **não adiciona novos serviços de backend** nas Fases 1 a 3. Ela é uma cliente do product-gateway existente. Todo comportamento específico de ferragens (taxonomia de categorias, filtros técnicos, faixas de preço pro) é implementado (a) inteiramente na SPA, ou (b) como campos aditivos nos schemas existentes de produto/vendedor — nunca como novos serviços.

Um novo **payment-service** (Pix + boleto + tokenização de cartão) é introduzido na Fase 3 / Sprint 08. Veja [ADR 002](adr/002-integration-strategy.md).

## Stack tecnológico

Espelha o `gifthy-hub/` para maximizar reuso, sinal de treinamento e fluxo de contribuidores.

| Camada | Escolha | Por quê |
|--------|---------|---------|
| Build | Vite 5 | Mesmo do gifthy-hub |
| Linguagem | TypeScript 5 (strict) | Mesmo |
| UI | React 18 | Mesmo |
| Estilização | Tailwind CSS 3 + variáveis CSS para tema | Mesmo; permite trocar para a paleta Utilar via CSS vars |
| Roteamento | React Router 6 | Mesmo |
| Dados | TanStack Query 5 | Mesmo |
| Estado | Zustand 4 | Mesmo, já usado para auth/locale |
| Formulários | React Hook Form + Zod | Mesmo, incluindo o padrão Zod + i18n |
| i18n | i18next + react-i18next | Mesmo — reutiliza o padrão `gifthy-hub/src/i18n/*`; **pt-BR é o padrão** |
| Ícones | lucide-react | Mesmo |
| Testes | Vitest 2 + happy-dom + Testing Library | Mesmo (Node 20+) |
| Lint | ESLint (política zero-warning) + Prettier | Mesmo |
| HTTP | `fetch` nativo via um `src/lib/api.ts` fino | Mesmo padrão |

Dependência: **Node 20** (v20.20.2 LTS — atualizado em 2026-04-23).

### payment-service

| Camada | Escolha |
|--------|---------|
| Linguagem | Go 1.26.2 |
| Framework HTTP | Gin 1.12 |
| Banco de dados | PostgreSQL 17 (Docker: `postgres:17-alpine`) |
| Event bus | Redpanda v26.1.6 |
| Console | Redpanda Console v3.7.1 |
| Migrações | golang-migrate v4 |
| PSP | Mercado Pago (Pix + boleto + cartão) |
| Auth | golang-jwt/jwt v5 (JWT compartilhado com gateway) |

## Estrutura de diretórios

```
utilar-ferragem/
├── docs/                          # (esta pasta — docs de planejamento)
├── public/                        # favicon, robots, assets estáticos
└── src/
    ├── main.tsx
    ├── App.tsx
    ├── router/                    # Árvore de rotas (catálogo público + conta autenticada)
    ├── pages/
    │   ├── catalog/               # Home, categoria, busca, PDP
    │   ├── cart/
    │   ├── checkout/
    │   ├── account/               # Pedidos, endereços, perfil CNPJ/CPF
    │   └── seller-onboarding/     # Fase 4 — cadastro específico para ferragens
    ├── components/
    │   ├── layout/                # Navbar, Footer, CategoryRail
    │   ├── product/               # ProductCard, SpecSheet, StockBadge
    │   ├── checkout/              # PaymentMethodPicker, CepAutofill
    │   └── ui/                    # Componentes primitivos do design system
    ├── lib/
    │   ├── api.ts                 # wrapper fetch + anexação do JWT
    │   ├── format.ts              # Formatadores de BRL/USD, CNPJ/CPF, CEP, telefone
    │   ├── taxonomy.ts            # Árvore de categorias (gerenciada pelo cliente, ver abaixo)
    │   └── filters.ts             # Schema de filtros técnicos (tensão, rosca, HP, etc.)
    ├── store/
    │   ├── authStore.ts
    │   ├── cartStore.ts           # Persistido no localStorage
    │   └── localeStore.ts
    ├── i18n/
    │   ├── pt-BR/{common,catalog,checkout,account}.json
    │   └── en/{common,catalog,checkout,account}.json
    ├── types/                     # Tipos de resposta da API
    └── test/                      # setup + utilitários compartilhados
```

## Integração com o product-gateway

### O que reutilizamos sem alteração

| Endpoint | Uso |
|----------|-----|
| `POST /auth/login` | Login do cliente |
| `POST /auth/register` | Cadastro do cliente (role = `customer`) |
| `GET /api/v1/users/me` | Carrega o perfil |
| `GET /api/v1/products` (endpoint público do marketplace) | Listagem do catálogo |
| `GET /api/v1/products/:id` | Detalhe do produto |
| `POST /api/v1/orders` | Checkout |
| `GET /api/v1/orders` | Histórico de pedidos |

### O que precisa de trabalho aditivo no backend

São pequenas alterações aditivas nos serviços Rails existentes — **sem novos serviços, sem breaking changes**.

1. **`products.category_path`** (product-service) — string desnormalizada como `"ferramentas/eletricas/parafusadeiras"` para a taxonomia. Alternativa: mapeamento client-side da string `category` para o path (preferido na Fase 1; promover para coluna no DB se o desempenho de query exigir).
2. **`products.specs`** (product-service) — coluna JSONB para especificações técnicas (`{ "voltage": "220V", "thread": "M6", "power_hp": 1.5 }`). Filtrado server-side via índice GIN quando necessário.
3. **`users.role = 'customer'`** (user-service) — já suportado implicitamente (qualquer usuário que não seja seller/admin); apenas formalizar o valor e escopar o endpoint de registro adequadamente.
4. **`users.cpf` + `users.cnpj`** — já planejados nos Sprints 17/18 para vendedores. Estender para clientes com o mesmo validador.

Cada item acima terá seu próprio ADR / migration quando chegarmos ao respectivo sprint.

### O que é totalmente novo (Fase 3 / Sprint 08)

**payment-service** — serviço em Go 1.26, responsável por:
- Geração de QR/copia-e-cola Pix (via PSP: Gerencianet, Mercado Pago, Stripe BR ou PagSeguro)
- Emissão de boleto
- Tokenização de cartão + 3DS
- Webhooks para confirmações assíncronas

Veja [`docs/adr/002-integration-strategy.md`](adr/002-integration-strategy.md) para os critérios de seleção de PSP.

## Lógica de domínio específica de ferragens (client-side)

### Taxonomia de categorias

Armazenada em `src/lib/taxonomy.ts` como uma árvore tipada. Nós de nível superior:

- `ferramentas` — Ferramentas (manuais, elétricas, pneumáticas, medição)
- `construcao` — Material de construção (cimento, argamassa, telhas, blocos)
- `eletrica` — Elétrica (cabos, disjuntores, tomadas, iluminação)
- `hidraulica` — Hidráulica (tubos, conexões, registros, bombas)
- `pintura` — Pintura (tintas, pincéis, lixas, solventes)
- `jardim` — Jardim e externa (furadeiras, cortadores, irrigação)
- `seguranca` — EPI e segurança (capacetes, luvas, óculos, calçados)
- `fixacao` — Fixação (parafusos, buchas, pregos, arames)

Cada folha mapeia para uma ou mais strings `category` legadas do product-service, permitindo reclassificar os 65 produtos seedados sem migration no DB na Fase 1.

### Filtros técnicos

Definidos em `src/lib/filters.ts` — um schema de filtros por categoria:

```ts
type TradeFilter = {
  key: string                 // ex: "voltage"
  label: { 'pt-BR': string; en: string }
  type: 'enum' | 'range' | 'boolean'
  options?: string[]          // para enum
  appliesTo: string[]         // paths da taxonomia
}
```

Os filtros são aplicados client-side contra `product.specs` nas Fases 1 e 2. Quando o catálogo crescer, promover para server-side via query params `?spec[voltage]=220V` com índice GIN na coluna JSONB.

## Fluxo de auth

Reutiliza exatamente o padrão JWT do gifthy-hub:

1. `authStore.ts` gerencia `{ user, token }` via Zustand + localStorage persist.
2. `src/lib/api.ts` anexa `Authorization: Bearer <token>` a toda requisição quando `token` está definido.
3. Um novo papel `customer` é adicionado. Papéis `seller` e `admin` não podem usar os fluxos de cliente da Utilar Ferragem — são redirecionados para o `gifthy-hub`.
4. O segredo JWT deve ser o mesmo no gateway + user-service + (novo) payment-service. Continuar usando a convenção da variável de ambiente `JWT_SECRET`.

## i18n

- **Locale padrão**: `pt-BR`. `en` é secundário (para clientes expatriados, turistas, profissionais de língua inglesa).
- Mesmo padrão de `LocaleSwitcher` que o gifthy-hub.
- Namespaces: `common`, `catalog`, `checkout`, `account`, `seller-onboarding`.
- Moeda: BRL por padrão; respeitar `product.currency` quando presente.
- Formatadores de CEP, CNPJ, CPF e telefone: **copiar verbatim** de `gifthy-hub/src/lib/cep.ts`, `cnpj.ts` e estender com um novo `cpf.ts`. Não reimplementar.

## Deploy

### Dev

- Servido pelo `vite dev` em `http://localhost:5174` (5173 está ocupado pelo gifthy-hub).
- Aponta para o gateway local em `http://localhost:8080` via `VITE_API_URL`.

### Prod

Duas opções (ADR pendente):

| Opção | Prós | Contras |
|-------|------|---------|
| **Mesmo ALB, novo hostname** (`utilarferragem.com.br` → ALB → novo container nginx servindo a SPA) | Compartilha a infra; SSL compartilhado; custo marginal baixo | O trabalho no ALB do Sprint 15 precisa terminar primeiro |
| **Hosting estático** (S3 + CloudFront, ou Vercel, ou Netlify) | Mais barato, CDN mais rápida, zero infra | Adiciona um segundo pipeline de deploy |

Recomendação: **S3 + CloudFront** para o bundle da SPA (é estático após o build), com a API ainda atrás do gateway/ALB existente. O mais barato + rápido + simples. Decidido em [ADR 001](adr/001-placement-and-stack.md).

## Observabilidade

- Client-side: Sentry (plano gratuito) para rastreamento de erros. Reutilizar o projeto do Gifthy ou criar um novo.
- Server-side: herda o logging existente do product-gateway. Nenhuma nova superfície de telemetria necessária até a Fase 5.

## Notas de segurança

- Todo PII (CPF, CNPJ, endereços) flui pelo gateway + user-service existentes, que já são a fonte da verdade.
- Dados de cartão **nunca tocam nosso frontend** — tokenizar via SDK hospedado do PSP (iframe ou drop-in), armazenar apenas o token.
- Payloads de Pix / boleto são gerados server-side pelo payment-service; a SPA apenas renderiza o QR code e o status.
- CORS: apenas as origens `utilarferragem.com.br` + `*.utilarferragem.com.br` + localhost na lista de permissões do gateway. Sem wildcard `*`.

## Fora do escopo deste documento de arquitetura

- App mobile (Fase 5+)
- Logística / fulfillment próprio (Fase 5+)
- Multi-moeda / multi-país — somente BR
- Funcionalidades em tempo real (chat com vendedor, estoque ao vivo) — Fase 5+
