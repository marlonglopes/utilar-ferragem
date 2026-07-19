# API do dashboard administrativo

Contrato que o SPA (`app/src/pages/admin/`) consome. Os tipos em
`app/src/lib/adminTypes.ts` são o **modelo de view** (a forma que a tela usa) e
`app/src/lib/adminAdapters.ts` é a tradução do formato real das APIs para ele.
Onde a API ainda não existe, o modelo de view é também a especificação do que
se espera dela.

Status: **front pronto.** A fatia contábil e a trilha de auditoria já existem
no payment-service (`/api/v1/ledger/*`) e o front está ligado nelas através de
`app/src/lib/adminAdapters.ts`. A visão geral, o desempenho de vendedores e a
observabilidade ainda não têm backend — ver § Pendências.

---

## Convenções (valem para todas as rotas)

| Regra | Detalhe |
|---|---|
| Dinheiro | **inteiro em centavos**, sufixo `*Cents`. `1234` = R$ 12,34. Nunca float. |
| Percentual | **fração 0..1** (`0.0342` = 3,42%), nunca 0..100. |
| Datas/hora | ISO-8601 UTC com timezone explícito (`2026-07-18T13:05:00Z`). |
| Período | `?from=YYYY-MM-DD&to=YYYY-MM-DD`, **inclusivo nos dois extremos**. |
| Paginação | `?page=` (1-based) `&pageSize=`; resposta `{items, page, pageSize, total}` — `total` é de **itens**, não de páginas. |
| Erro | envelope existente do projeto: `{error, code, requestId, details?}`. |
| Severidade | `"ok" \| "warn" \| "critical"` — calculada no backend onde o limiar é de negócio. |

### Autorização — requisito não-negociável

**O guard do front (`components/admin/AdminRoute.tsx`) não é fronteira de
segurança.** Ele decide o que renderizar; qualquer pessoa edita o `localStorage`
e força `role: "admin"`. Toda rota administrativa (`/api/v1/ledger/*`,
`/api/v1/admin/*`) **precisa** validar:

1. `Authorization: Bearer <jwt>` com assinatura válida;
2. claim de papel `admin` **vinda do token assinado**, nunca do corpo/query;
3. resposta `403` com `code: "forbidden"` quando o papel não bate (o front já
   trata esse código com mensagem específica).

Além disso, para dados de auditoria/contábeis:

- responder com `Cache-Control: no-store` (o front já manda `cache: 'no-store'`,
  mas a defesa tem que ser dos dois lados);
- **registrar cada acesso na própria trilha de auditoria** como
  `action: "admin_access"` — quem abriu o livro-razão é informação auditável.

### Dados que NÃO devem trafegar

Levantado durante a implementação do front. Vale como requisito, não sugestão:

- **Custo de aquisição por item.** `SellerPerformance.avgMarginPct` exige custo
  para ser calculado — calcule no servidor e envie **só a margem agregada**.
  Custo unitário no navegador vaza a estrutura de compra da Utilar para qualquer
  um com o DevTools aberto (inclusive o próprio vendedor, se um dia ganhar
  acesso a alguma tela de admin).
- **Documento de cliente (CPF/CNPJ) e endereço** nas telas de admin. O painel
  precisa de `customerName` para identificar o pedido travado, e nada além.
- **Token/PAN do PSP.** `pspTransactionId` é identificador opaco e basta para
  reconciliar; nada de token de cartão, últimos dígitos ou bandeira.
- **IP completo** na trilha. Enviar com o último octeto mascarado
  (`189.12.34.xxx`) — o front exibe o que receber, então o mascaramento tem que
  acontecer no servidor.

---

## 1. Visão geral — order-service ⛔ A IMPLEMENTAR

```
GET {ORDER_URL}/api/v1/admin/overview?from&to
→ AdminOverview
```

```ts
{
  period: { from, to },
  kpis: {
    todayCents, weekCents, monthCents,
    todayPrevCents, weekPrevCents, monthPrevCents,  // período anterior equivalente
    avgTicketCents, avgTicketPrevCents,
    orderCount, orderCountPrev
  },
  series: [{ date: "YYYY-MM-DD", valueCents, orders }],   // um ponto por dia do período
  byStatus: [{ status, count, valueCents }],
  funnel: {
    created, confirmed, failed, expired,
    byMethod: [{ method, created, confirmed }]
  },
  stuckOrders: [{ orderId, orderNumber, status, paidAt, stuckForHours, totalCents, customerName }],
  alerts: [Alert]
}
```

Notas:

- **Não envie a taxa de conversão pronta.** O front deriva `confirmed / created`
  para poder recalcular ao cruzar filtros sem uma segunda chamada.
- `stuckForHours` é calculado no servidor — não dá para confiar no relógio do
  cliente para decidir o que está atrasado.
- "Travado" = pagamento confirmado e pedido ainda em `paid` (sem separação).
  O limiar de exibição sugerido é 4h.
- `status` ∈ `pending | paid | picking | shipped | delivered | canceled | refunded`.

---

## 2. Auditoria contábil — payment-service ✅ IMPLEMENTADO

Rotas reais, todas sob `/api/v1/ledger` com `JWTMiddleware + AdminOnly`:

| Rota | Uso na tela |
|---|---|
| `GET /summary?from&to` | KPIs de bruto / taxas / estornos / chargebacks / líquido |
| `GET /by-method?from&to` | tabela "por método de pagamento" |
| `GET /daily?from&to` | série diária (sparkline + barras) |
| `GET /entries?from&to&limit` | livro de lançamentos |
| `GET /discrepancies?limit` | divergências de reconciliação |
| `GET /export?from&to&format=csv\|balancete\|ofx` | exportação para o contador |
| `GET /audit`, `GET /audit/verify` | trilha de auditoria (§4) |

O front NÃO consome esses formatos direto: `app/src/lib/adminAdapters.ts`
traduz para o modelo de view. Os dois modelos são legitimamente diferentes — o
da API é do contador (partida dobrada, balancete, período contábil), o da tela é
de quem decide. Acoplar os componentes ao esquema contábil os quebraria a cada
evolução dele.

### Divergências que o adaptador absorve hoje

Cada item aqui é dado que a tela gostaria de ter e não tem. Estão listados
porque são invisíveis no componente e viram bug silencioso.

1. **`by-method` não recorta chargeback.** O adaptador devolve `null`, e a
   célula mostra "—". Exibir `0` afirmaria "não houve chargeback neste método",
   que é diferente de "não sabemos" — num relatório contábil essa diferença
   importa. *Pedido: acrescentar `chargebacksCents` ao `MethodBreakdown`.*
2. **`by-method` não traz antecipação nem repasse**, então a soma das linhas não
   fecha com o total do `/summary`. O rodapé da tabela usa o `/summary`, nunca a
   soma das linhas.
3. **Ambiguidade na fórmula do líquido.** O comentário do `Summary` diz
   `bruto − taxas − estornos − chargebacks − repasses`, sem citar a taxa de
   antecipação (conta 4.1.2), que também é despesa financeira. A checagem do
   front (`accountingImbalanceCents`) aceita as duas leituras e só acusa
   desequilíbrio quando nenhuma fecha — um banner "não fecha" falso destruiria a
   confiança na única tela feita para gerar confiança. *Pedido: fixar a fórmula
   na doc do `Summary`.*
4. **`entries` não pagina** — só `limit` (padrão 500, teto 50 000). O front pede
   5 000 e pagina no cliente. Acima disso o caminho é o export CSV, e a UI já
   oferece o botão. *Pedido: `page`/`pageSize` + `total`, e os totais de débito
   e crédito do filtro inteiro para o rodapé fechar o período.*
5. **`entries` não traz filtro de natureza nem de método** — o front filtra no
   cliente sobre a janela carregada, o que fica errado quando a janela satura.
6. **`entries` não expõe o id da transação no PSP**, então a coluna que ligaria
   o lançamento ao extrato do Appmax fica vazia; só a reconciliação tem
   `pspPaymentId`.
7. **`discrepancies` não devolve os dois lados em centavos** (só `localValue` /
   `pspValue` como texto e o `amountDeltaCents`). As colunas "nosso livro" e
   "extrato PSP" exibem o delta, que é o número acionável. Também não informa
   `matchedCount` nem quando o job rodou — isso só existe no resultado de um
   `POST /reconcile`.
8. **O export exige `Authorization`**, então não pode ser uma navegação de topo:
   o navegador não anexa header nenhum nesse caso e o resultado seria um 401
   renderizado como página. O front baixa via `fetch` autenticado + blob.

### Formato do CSV

Quando gerado no cliente (modo mock), o front emite exatamente:

```
data;conta_codigo;conta_nome;natureza;debito;credito;metodo;pedido;pedido_id;pagamento_id;psp_transacao;historico
```

separador `;`, decimal com vírgula, BOM UTF-8, CRLF, escape RFC 4180 — o que o
Excel pt-BR precisa. Vale conferir se `ExportRazaoCSV` casa com isso; se
divergir, o contador recebe dois formatos diferentes conforme o ambiente.

## 3. Desempenho dos vendedores — order-service ⛔ A IMPLEMENTAR

```
GET {ORDER_URL}/api/v1/admin/sellers/performance?from&to&storeId
→ SellerPerformanceReport
```

```ts
{
  period,
  stores: [{ id, name }],
  sellers: [{
    sellerId, sellerName, storeId, storeName,
    totalCents, orderCount, avgTicketCents,
    avgDiscountPct,      // fração do bruto
    avgMarginPct,        // fração, JÁ AGREGADA (ver "dados que não devem trafegar")
    managerApprovals,    // pedidos que estouraram o teto de desconto do cargo
    series: [{ date, valueCents, orders }]
  }]
}
```

**Dependência:** exige o papel de operador e a atribuição de pedido
(`orders.operator_id`) que está sendo criado em paralelo. Sem isso não há como
agrupar. Enquanto não existir, a rota pode responder `501` com
`code: "not_implemented"` — o front mostra o estado de erro com botão de retry.

`managerApprovals` conta pedidos cujo desconto excedeu o teto do cargo e passou
por aprovação — o mesmo evento que gera `discount_approval` na trilha (§4).

---

## 4. Trilha de auditoria — payment-service ✅ IMPLEMENTADO

Vive no payment-service (`pkg/audit` + `LedgerHandler`), **não** no
auth-service como este documento supunha na primeira versão.

```
GET /api/v1/ledger/audit?entityType&entityId&actorId&fromSeq&limit → {records: [...]}
GET /api/v1/ledger/audit/verify                                    → {valid, headSeq, headHash, brokenAtSeq?, kind?}
```

O `verify` responde **200 mesmo com `valid: false`** — decisão certa: um 5xx
faria o painel mostrar "erro de rede" no exato instante em que precisa gritar
"a trilha foi adulterada".

### Pendências da trilha

1. **🔴 O IP vai completo.** `pkg/audit.Record.ActorIP` é `c.ClientIP()` sem
   mascaramento. O front mascara o último octeto ao exibir, mas isso é
   mitigação, não correção: o dado já cruzou a rede e está na memória da aba.
   **Mascarar na gravação ou na serialização.**
2. **🟡 `Record` não tem tags `json`**, então a resposta sai em **PascalCase**
   (`Seq`, `ActorID`, `PrevHash`), divergindo do camelCase de todo o resto da
   API. O adaptador lê PascalCase hoje; se as tags forem adicionadas, o único
   lugar a mudar é `adminAdapters.ts`.
3. **🟡 Só o `actorId` é gravado, não o nome.** A tela exibe o id. Resolver com
   um join no auth-service ou desnormalizando o nome no momento da gravação —
   uma chamada por linha não é opção.
4. **🟡 As ações são genéricas** (`create|update|delete|access|approve|export`).
   A tela nomeia verbos de negócio ("mudança de preço", "aprovação de
   desconto"), então o adaptador infere cruzando `entityType` com `action` — o
   que é heurística, e heurística erra. Ideal: um campo de ação de negócio
   explícito.
5. **🟡 `OldValue`/`NewValue` são JSON bruto.** O front serializa em texto para
   caber na célula. Quem grava o evento sabe o que a mudança significa; a camada
   de apresentação não.
6. **🟡 `verify` não aceita janela** — verifica a cadeia inteira. Correto
   conceitualmente (verificar um recorte não prova nada sobre os elos fora
   dele), mas vai ficar caro conforme a tabela cresce. Considerar uma âncora
   periódica publicada externamente.

## 5. Observabilidade — payment-service (agregador) ⛔ A IMPLEMENTAR

```
GET {API_URL}/api/v1/admin/observability
→ ObservabilitySnapshot
```

```ts
{
  collectedAt,
  services: [{
    name,                      // auth | catalog | order | payment
    status, up,
    p50Ms, p95Ms, p99Ms,       // janela de 5 min
    errorRate,                 // fração 0..1 de 5xx
    rpm, uptimePct, version,
    latencySeries: [number]    // últimos ~30 pontos de p95, para a sparkline
  }],
  outbox: {
    pending, failed,
    oldestAgeSeconds,          // idade do pendente mais antigo; 0 se a fila está vazia
    publishedPerMinute,
    severity
  },
  alerts: [Alert]
}
```

`Alert`:

```ts
{ id, severity, title, detail, source, firedAt, href? }
```

`href` é a rota do painel que investiga o alerta (`/admin/observabilidade`,
`/admin/contabil`, …) — o front transforma o alerta em link.

**A fila do outbox é o número mais importante desta rota.** Ela é o elo entre
"pagamento confirmado" e "pedido pago": se `oldestAgeSeconds` cresce, os pedidos
travados de §1 são consequência. Os limiares usados pelo front
(`outboxSeverity`) — `warn` a partir de 120s ou 100 pendentes, `critical` a
partir de 900s ou 500 — priorizam **idade sobre tamanho**: 500 eventos escoando
em 10s é saudável, 3 eventos parados há uma hora é incidente.

Esta rota é chamada a cada 30s enquanto a tela está aberta. Deve ser barata —
ler de um cache/agregado, não varrer tabela.

---

## Costura da rota no SPA

`app/src/router/adminRoutes.tsx` exporta `adminRoutes: RouteObject[]`, no mesmo
padrão do balcão. Em `app/src/router/index.tsx`:

```tsx
import { adminRoutes } from '@/router/adminRoutes'

const router = createBrowserRouter([
  { path: '/', element: <PublicLayout />, children: [ ... ] },
  ...balcaoRoutes,
  ...adminRoutes,
  ...
])
```

Rotas: `/admin`, `/admin/contabil`, `/admin/vendedores`, `/admin/trilha`,
`/admin/observabilidade`. Todas fora do `PublicLayout` (chrome próprio) e
lazy-loaded.
