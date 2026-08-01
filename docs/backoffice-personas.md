# Personas de backoffice — contador · vendas · almoxarife

**O que é:** o painel `/admin` deixou de ser tudo-ou-nada (só `admin`). Agora
cada **persona de operação** entra e vê só o seu. A fronteira de verdade é o
**403 de cada serviço** (RequireRole no backend); o menu filtrado do painel é
só conforto (esconder o que daria 403 de qualquer jeito).

> ⚠️ `vendas` é o vendedor **interno** da loja (painel: catálogo + pedidos).
> NÃO é `seller` (lojista anunciante do marketplace) nem `store_operator` (o
> papel do balcão/PDV). Ver `pkg/roles`.

## Matriz (quem vê / faz o quê)

| Seção do painel | admin | contador | vendas | almoxarife |
|---|:--:|:--:|:--:|:--:|
| Visão geral (faturamento/margem) | ✅ | — | — | — |
| Auditoria contábil (livro) | ✅ | ✅ | — | — |
| Trilha de auditoria (contábil) | ✅ | ✅ | — | — |
| Observabilidade (saúde) | ✅ | ✅ | — | — |
| Pedidos (lista) | ✅ | 👁️ leitura | ✅ agir | ✅ separar/despachar |
| Devolução — receber físico | ✅ | — | ✅ | ✅ |
| Devolução — **reembolso (dinheiro)** | ✅ | — | — | — |
| Produtos / Categorias / Importar (**tem custo**) | ✅ | — | ✅ | — |
| Atividade (trilha do catálogo, tem custo) | ✅ | — | ✅ | — |
| Operadores / Vendedores (staff) | ✅ | — | — | — |

**Custo/margem:** só **admin** e **vendas**. É por isso que contador e
almoxarife simplesmente não têm as rotas de catálogo — a proteção é *negar a
rota*, não filtrar o campo (zero risco de vazamento). `contador` é **read-only
fora do contábil**; dentro do contábil ele age (conciliar, fechar período). O
**reembolso** (dinheiro saindo) é **só admin**.

## Onde a fronteira é aplicada (403 fail-closed)

| Serviço | Rota | Papéis |
|---|---|---|
| order | GET `/admin/orders`, `/admin/returns` | admin, operator, contador, vendas, almoxarife |
| order | PATCH fulfillment + devolução (approve/reject/receive) | admin, operator, vendas, almoxarife |
| order | PATCH `/admin/returns/:id/refund` | **admin** |
| order | `/admin/overview`, `/admin/sellers/performance` | **admin** |
| catalog | `/admin/*` (produto, categoria, import, trilha, review, custo) | admin, vendas |
| catalog | `/admin/observability` | admin, contador |
| payment | `/api/v1/ledger/*` | admin, contador |
| auth | `/admin/users`, `/stores`, `/operators` | **admin** |

Conjuntos de papéis por serviço: `handler.Ops*Roles` (order),
`handler.Catalog*Roles` (catalog), `handler.LedgerRoles` (payment). No front, a
mesma matriz vive em `app/src/lib/adminAccess.ts` (menu + guard de rota).

Testes que travam isso: `TestPersonas_OpsAuthzFailClosed` (order),
`TestPersonas_CatalogAuthzFailClosed` (catalog),
`TestLedger_AdminEContadorEntram_RestoTomam403` (payment),
`adminAccess.test.ts` + `adminRoute.test.tsx` + `admin.spec.ts` (front).

## Como atribuir uma persona a alguém

Só o admin. `PATCH /api/v1/admin/users/:id/role` `{ "role": "contador" }` —
audita de→para (promover a `vendas` passa a expor custo; a `admin`, a loja
inteira). Papel desconhecido → 400.

## Follow-up conhecido — `vendas` no balcão/PDV

A decisão foi que `vendas` também opera o PDV. Ainda **não** está ligado: o
balcão é autorizado por **vínculo com a loja** (`store_operators`) + claims de
loja no JWT, emitidas hoje só para `store_operator`, e `internal/balcao/authz.go`
exige `role=store_operator` no caminho do dinheiro. Ligar `vendas` no balcão é
mudança no caminho do dinheiro (vínculo + claims + authz) e fica para uma
entrega própria. Por ora, quem vende no balcão é `store_operator`; `vendas`
opera o painel.
