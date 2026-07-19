# CLAUDE.md

Guia para o Claude Code (claude.ai/code) trabalhar neste repositório.

## O que é

**Utilar Ferragem** — loja online de ferragem e material de construção **com PDV de balcão** para a loja física. SPA React + **5 microserviços Go**, cada um com seu próprio Postgres.

Três produtos no mesmo código:

| | Rota | Quem usa |
|---|---|---|
| **Loja** | `/` | Cliente final |
| **Balcão (PDV)** | `/balcao` | Vendedor da loja física, no tablet |
| **Admin** | `/admin` | Dono e gestão |

E a **Alice**, assistente embutida que consulta o catálogo real e calcula material de obra.

> ⛔ **Utilar ≠ gifthy.** Diretriz permanente do dono: nunca misturar contas, credenciais, infraestrutura, bancos ou dados. Ler o gifthy como referência é OK; usar as contas/tokens dele, não. Ver [`docs/SEPARATION-utilar-vs-gifthy.md`](docs/SEPARATION-utilar-vs-gifthy.md).

## Regras permanentes do dono

1. **Tudo coberto por testes, sempre. Não pode falhar.** Nenhuma feature é "pronta" sem teste. Prefira teste de regressão nomeado pelo bug que previne, com comentário explicando o modo de falha.
2. **Commit + push assim que algo for corrigido.** Não acumule. Nunca commite com build ou teste quebrado.
3. **Segredo nunca é versionado.** `.gitignore` cobre `.env*`, chaves, `.creds`, `.aws/`, `.claude/settings.local.json`.

## Armadilhas que já custaram tempo — leia antes de confiar num comando

| Armadilha | Realidade |
|---|---|
| `npx tsc --noEmit` | **Não checa nada.** `tsconfig.json` tem `files: []` + references. Sai 0 sempre. **Use `npx tsc -b`.** |
| `go build ./services/...` | **Falha.** Layout de workspace. Aponte por módulo: `go build ./services/catalog-service/...` |
| Migration aplicada à mão | Deixa `schema_migrations.dirty = true` e o serviço se recusa a subir. Verifique antes de acusar bug. |
| `require github.com/utilar/pkg` em go.mod | **Quebra o build** (força fetch de rede). O `go.work` já provê. Nunca adicione. |
| `ON CONFLICT (sku)` | O índice é **parcial**. Precisa de `ON CONFLICT (sku) WHERE sku IS NOT NULL`. |
| `NOT NULL DEFAULT` no Postgres | O DEFAULT só vale quando a coluna é **omitida** do INSERT. NULL explícito viola a constraint. |
| Relatório de subagente | **Verifique você mesmo.** Build, rode a suíte, teste a lógica de risco. Vários bugs sérios saíram daí. |
| Teste de concorrência falhando do nada | O **catalog-service rodando** tem um sweeper de reservas a cada 60s no mesmo banco. Pare o serviço antes de rodar a suíte, ou trate como flake. |

## Comandos

```bash
make dev            # SPA em mock mode (sem backend), :5175
make dev-full       # Todos os serviços + infra + SPA
make infra-up       # Postgres x4, Redis, Redpanda
make test           # Frontend (vitest)
```

**Build e teste Go — por módulo:**
```bash
go build ./services/<svc>/...
go test  ./services/<svc>/... -race
```
Módulos: `pkg`, `services/{auth,catalog,order,payment,assistant}-service`.

**Frontend:**
```bash
cd app && npx tsc -b && npm run lint && npx vitest run
```

**Rodar na rede local** (testar no celular/tablet — o PDV é feito pra tablet):
```bash
cd app && npx vite --host 0.0.0.0 --port 5175
```
Aponte `app/.env.local` para o IP da máquina, não `localhost` — no celular `localhost` é o próprio celular.

**Banco** (padrão por serviço): `make <prefixo>db-{migrate,seed,reset,psql,status}`

| Serviço | Prefixo | Porta | Banco |
|---|---|---|---|
| payment | `db-` | 5435 | `payment_service` |
| catalog | `catalog-db-` | 5436 | `catalog_service` |
| order | `order-db-` | 5437 | `order_service` |
| auth | `auth-db-` | 5438 | `auth_service` |

Portas presas em `127.0.0.1` de propósito — só o SPA e as APIs ficam na rede.

## Arquitetura

```
React SPA (:5175)
  │ HTTP + JWT
  ├── Auth      :8093 ── PG :5438   usuários, papéis, lojas, operadores
  ├── Catalog   :8091 ── PG :5436   produtos, estoque, reservas, importação
  ├── Order     :8092 ── PG :5437   pedidos, frete, balcão, fulfillment
  ├── Payment   :8090 ── PG :5435   PSP, webhooks, outbox, LIVRO CONTÁBIL
  └── Assistant :8094               Alice (sem banco próprio)
                          Redpanda ── payment.confirmed → order
                          Redis    ── rate limit, idempotência
```

**5 bancos separados, sem acesso cruzado.** O order não faz `SELECT` no catálogo — chama a API. Custa mais trabalho, mas contém estrago, limita o alcance de uma invasão e permite migrar cada schema sozinho.

### Fluxos que importam

**Venda web:** carrinho → `POST /orders` (preço e frete resolvidos **no servidor**, estoque **reservado**) → pagamento → webhook → reconsulta ao PSP → `payment.confirmed` no Redpanda → consumer do order → `status='paid'` → reserva vira baixa.

**Venda no balcão:** vendedor autenticado monta o pedido, negocia desconto **dentro do teto do cargo** (resolvido no servidor, fail-closed), acima do teto vai pra fila do gerente. Sem endereço (é retirada). Tudo auditado com quem/quando/quanto.

## Segurança — o modelo mental

O caminho do dinheiro é o mais bem feito do sistema. Preserve estas invariantes:

- **O cliente nunca dita valor.** Preço vem do catálogo, frete da tabela, desconto do teto do cargo, valor do pagamento do order-service. O corpo da requisição não é fonte de verdade.
- **Webhook não é fonte de verdade.** A Appmax não assina postback. O corpo é só um *gatilho*; status e valor vêm da reconsulta autenticada ao PSP. (Já houve falha grave aqui: o status vinha do corpo e dava pra confirmar pagamento sem pagar.)
- **Zero IDOR.** Toda leitura é escopada pelo JWT. No balcão o escopo é a loja — e o escopo do cliente comum nunca foi afrouxado.
- **`cost` não existe no modelo público.** Mora só em `model.AdminProduct` (rota `/admin`) e `model.ProductCost` (rota `/store`, do balcão — `store_operator`/`admin`/`service`). Teste trava isso (`TestPublicAPI_NuncaVazaCusto`, `TestBalcao_CustoNuncaRespondeParaClienteOuAnonimo`).
- **Lock de HS256** em todo ponto de verificação de JWT.
- **Auditoria append-only** com hash encadeado, garantida por trigger no banco. Contábil com partidas dobradas e soma-zero por constraint.

### 🔴 Aberto — leia antes de fazer deploy

Ver [`docs/security/auditoria-arquitetural-2026-07-18.md`](docs/security/auditoria-arquitetural-2026-07-18.md):

- **A1 — 🟡 MITIGADO (não fechado).** Existem agora **dois segredos**: `JWT_SECRET` (identidade de usuário, todos verificam) e **`SERVICE_JWT_SECRET`** (identidade de serviço, `role=service`). `pkg/servicetoken` emite e verifica; `role=service` **só** vale assinado com o segredo de serviço, e um token de usuário com essa claim é rejeitado em todos os serviços. **A Alice não recebe `SERVICE_JWT_SECRET`** — é esse o ponto. Distribuição: **catalog, order, auth sim; assistant e payment não**. Boot é fail-closed fora de `DEV_MODE` (ausente ou igual ao `JWT_SECRET` → o serviço não sobe). **Continua aberto o definitivo**: assinatura assimétrica (auth-service assina com chave privada, os demais só verificam com a pública) — quem comprometer o order-service ainda emite token de serviço.
- **A2 — `DEV_MODE=true` em produção** entrega tudo via header `X-User-Role: admin`. Nada impede a variável de ser ligada por engano (~2h).
- **Backup nunca restaurado.** Backup não testado é backup que não existe.

## Frontend

React 18, TS strict, Vite 5, Tailwind 3, Router 6, Zustand 4, TanStack Query 5, i18next (pt-BR), Vitest + RTL, Playwright. Alias `@/` → `app/src/`.

- **Mock mode**: `VITE_*_URL` vazio → dados de `src/lib/mock*.ts`. Testes sempre em mock.
- **API** (`src/lib/api.ts`): 4 base URLs por serviço, JWT com refresh em 401, e **`Idempotency-Key` derivada de conteúdo** — UUID por chamada não protegeria contra duplo clique, que é o caso real.
- **Rotas protegidas**: `ProtectedRoute` (cliente), `BalcaoRoute` (operador), `AdminRoute` (admin). ⚠️ `ProtectedRoute` **expulsa quem não é `customer`** — não use pra área interna.
- **Papéis**: `customer | seller | admin | store_operator`. ⚠️ **`seller` é lojista do marketplace, NÃO vendedor de balcão.** Confundir dá acesso ao PDV pra todo anunciante.
- Rotas em pt-BR: `/categoria/:slug`, `/produto/:slug`, `/carrinho`, `/entrar`, `/checkout`, `/conta`, `/balcao`, `/admin`.
- Marca: `brand.orange` #F47920, `brand.blue` #1B3E8A, `brand.gold` #F5A623. Inter / Archivo / JetBrains Mono.

## Backend

Go 1.26, Gin, lib/pq, golang-migrate. `go.work` unifica `pkg/` + 5 serviços.

`pkg/`: `audit` (append-only encadeado), `metrics` (Prometheus), `httpclient`, `idempotency`, `ratelimit`, `requestid`, `circuitbreaker`, `retry`.

⚠️ **`pkg/retry`: o zero value de `Safety` é `NonIdempotent`** — quem esquecer de declarar ganha 1 tentativa só. Retry em rota de pagamento cobra o cliente duas vezes; ver `appmaxv1.isFinancialRoute` e `docs/resiliencia-entre-servicos.md`.

**Erro:** `{error, code, requestId, details?}` — códigos `not_found`, `bad_request`, `unauthorized`, `conflict`, `validation_error`, `db_error`, `insufficient_stock`.

**PSP:** `PSP_PROVIDER` = `stripe` | `mercadopago` | `appmax` (v3 admin) | `appmax-v1` (AppStore, OAuth2). São **duas APIs diferentes da Appmax** — a v1 tem Payment Split e é a recomendada. Ver [`docs/appmax-v1-appstore.md`](docs/appmax-v1-appstore.md).

⚠️ **Dinheiro em `float64`** ainda. Seguro hoje porque os valores vêm de `NUMERIC(12,2)`. **Quando entrar venda fracionada** (2,5 m × R$ 1,89), passa a gerar meio centavo e precisa virar decimal de verdade. Ver `psp/appmaxv1/money_test.go`.

## Alice

`services/assistant-service`. Orquestrador Claude onde **tool use é a única fonte de fatos** — ela consulta, não inventa.

- **Modelo: `claude-sonnet-5`** (`ALICE_MODEL`). Sem `ANTHROPIC_API_KEY` roda em mock com busca real no catálogo.
- **Dois modos**: cliente (público, didático, **nunca vê custo**) e vendedor (autenticado, vê custo e margem, sugere a troca que preserva margem em vez de dar desconto).
- Conhecimento de obra é **dado versionado com fonte citada**, não memória do modelo. Cálculos são funções Go testadas.
- **Ela não dimensiona estrutura.** Viga, pilar, laje, fundação, elétrica e gás → encaminha a profissional habilitado. É regra com teste, não pedido no prompt.
- Ver [`docs/alice-conhecimento.md`](docs/alice-conhecimento.md).

## Estrutura

```
app/src/{pages,components,hooks,store,lib,types,i18n,router,test}/
  components/{ui,layout,catalog,checkout,auth,cart,balcao,admin,assistant,common}/
services/{auth,catalog,order,payment,assistant}-service/
  cmd/server/ + internal/ + migrations/
pkg/{audit,metrics,httpclient,idempotency,ratelimit,requestid}/
scripts/ingestao/    # importador curado + imagens Wikimedia
docs/                # arquitetura, ADRs, APIs, segurança, orçamento
```

## Dados

- **Base pública**: SINAPI (Caixa/IBGE) é a fonte oficial de material de construção — insumos *e* coeficientes de consumo por serviço (que alimentam a Alice). ⚠️ O preço do SINAPI é **custo de obra pública, não preço de varejo**.
- **Não existe base pública brasileira com foto de produto.** As 288 imagens atuais são CC0 do Wikimedia — dado de teste, não catálogo final. Foto real vem do fornecedor ou da própria loja.
- **Importação nunca publica sozinha.** Entra como `draft`; publicar é decisão humana. Nunca apaga por ausência (vira `archived`). Queda de preço acima do limite segura pra revisão — erro de vírgula é o modo de falha mais caro.
- Ver [`docs/ingestao-de-produtos.md`](docs/ingestao-de-produtos.md).

## Onde olhar

| Assunto | Doc |
|---|---|
| Segurança (aberto) | `docs/security/auditoria-arquitetural-2026-07-18.md` |
| Segurança (histórico) | `docs/security/security-roadmap.md` |
| Appmax | `docs/appmax-v1-appstore.md` |
| Contábil | `docs/ledger-api.md` |
| Devolução/troca (CDC) | `docs/devolucao-e-troca.md` |
| Resiliência (disjuntor/retry) | `docs/resiliencia-entre-servicos.md` |
| Frete | `docs/shipping-api.md` |
| Dashboard | `docs/admin-dashboard-api.md` |
| Custo no balcão (PDV) | `docs/store-cost-api.md` |
| Alice | `docs/alice-conhecimento.md` |
| Ingestão | `docs/ingestao-de-produtos.md` |
| Custos / infra | `docs/orcamento-utilar-aws-2026-07.md`, `docs/aws-build-utilar.md` |
| Separação gifthy | `docs/SEPARATION-utilar-vs-gifthy.md` |

Sprints e fases: `SPRINT.md`, `docs/phases/`, `docs/sprints/`.

## Convenções

- **Comentário explica o PORQUÊ**, em pt-BR, especialmente onde a escolha não é óbvia ou onde já houve bug.
- Teste de regressão nomeado pelo que previne (`TestRegression_OperatorCannotApproveOwnDiscount`).
- Transação onde houver mais de uma escrita. `rows.Err()` sempre checado.
- Migrations reversíveis (`.up.sql` + `.down.sql`).
- Pre-commit: Husky + lint-staged (ESLint + Prettier no que está staged).
- Node em `.nvmrc` (v20), Go em `go.work` (1.26.2).
