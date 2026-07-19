# API do dashboard administrativo

Contrato que o SPA (`app/src/pages/admin/`) consome. Os tipos em
`app/src/lib/adminTypes.ts` são o **modelo de view** (a forma que a tela usa) e
`app/src/lib/adminAdapters.ts` é a tradução do formato real das APIs para ele.
Onde a API ainda não existe, o modelo de view é também a especificação do que
se espera dela.

Status: **front e backend prontos.** A fatia contábil e a trilha de auditoria
vivem no payment-service (`/api/v1/ledger/*`); a visão geral e o desempenho de
vendedores no order-service; a observabilidade no **catalog-service** (ver
§5 — o doc dizia payment-service e estava errado). O front se liga a tudo
através de `app/src/lib/adminAdapters.ts`.

> ⚠️ **Três divergências foram encontradas ao implementar** e estão corrigidas
> abaixo, cada uma marcada com **CORRIGIDO**. A que exige mudança no front é a
> §5: a URL base da observabilidade passou de `{API_URL}` (payment) para
> `{CATALOG_URL}`.

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
- `status` ∈ `pending | paid | picking | shipped | delivered | canceled`.

**CORRIGIDO — o vocabulário de status diverge do banco.** O enum do
order-service é `pending_payment | paid | picking | shipped | delivered |
cancelled` (grafia britânica). O contrato do front usa `pending` e `canceled`
(americana). O backend **traduz na serialização** (`statusToWire` em
`internal/handler/admin_dashboard.go`) — o front não muda, e uma migration de
ENUM em produção fica evitada. Há teste travando a tradução nos dois sentidos.

`refunded` **não existe** no order-service e nunca sai desta rota: estorno é
fato contábil e vive no payment-service (`/api/v1/ledger`). Mantê-lo no tipo do
front é inofensivo (nenhum bucket virá com ele), mas a tela não deve esperá-lo.

**CORRIGIDO — a semântica do funil é aproximada, e o doc não dizia.** A
intenção de pagamento vive no payment-service, em outro banco; o order-service
só enxerga o pedido. Portanto:

| Campo | O que realmente é |
|---|---|
| `created` | pedidos criados no período |
| `confirmed` | os que chegaram a ter `paid_at` |
| `failed` | cancelados que **nunca** foram pagos |
| `expired` | ainda em `pending_payment` e criados há mais de 24h |

`confirmed / created` (a conversão que o front deriva) é exata. O recorte
`failed` / `expired` é conservador — tentativa de cartão recusada e Pix expirado
não são eventos separados aqui. Fechar isso exige o payment-service publicar um
agregado de funil; até lá o número é honesto, só menos granular.

**Âncora temporal** (não estava no doc e muda o resultado): `kpis` e `series`
são ancorados em **`paid_at`** — receita é reconhecida quando o dinheiro entra,
e pedido criado e não pago não é faturamento. Já `byStatus` e `funnel` são
ancorados em **`created_at`**, porque a pergunta ali é "dos pedidos que
entraram, onde eles estão" e pedido nunca pago não tem `paid_at`. As KPIs
`today/week/month` **ignoram o `?from&to`** de propósito: "vendas de hoje" é
sempre hoje, independente do filtro aplicado ao gráfico.

**Período**: janela máxima de **366 dias**. Acima disso a rota responde `400`
com `code: "validation_error"` — período longo demais é recusado, nunca
truncado em silêncio. Sem `from`/`to`, o padrão são os últimos 30 dias.

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

**Dependência: RESOLVIDA.** `orders.operator_id` e `orders.store_id` existem
desde a migration 003 do order-service. A rota está implementada e **não**
responde 501.

Notas de implementação:

- Só o canal `balcao` entra. Venda web não tem vendedor, e incluí-la criaria
  uma linha fantasma "sem operador" carregando o faturamento inteiro do site,
  que esconderia todos os vendedores reais.
- `avgDiscountPct` é fração do **bruto** (`total + desconto`), não do líquido:
  R$ 20 de desconto em R$ 100 de bruto é 20%, não 25%.
- `avgMarginPct` é **ponderada por receita** — `(Σreceita − ΣCMV) / Σreceita`,
  não a média das margens por item. A média simples daria o mesmo peso a um
  parafuso e a uma betoneira, e o número serve para decidir desconto em
  dinheiro. O desconto do pedido é rateado proporcionalmente entre os itens.
- **Produto sem custo cadastrado fica fora da conta de margem inteira**
  (receita e CMV). Assumir custo zero daria margem inflada — o erro mais
  perigoso possível aqui, porque faria o gerente enxergar folga para desconto
  onde não existe. Há teste de regressão para isso.
- ⚠️ **`avgMarginPct: 0` é ambíguo** e o tipo do front não permite resolver
  isso hoje (`number`, não `number | null`). Zero significa tanto "margem
  realmente zero" quanto "nenhum item vendido tinha custo cadastrado". No
  banco de dev é o segundo caso — os itens de balcão do seed apontam para um
  produto que não existe no catálogo. *Pedido: `avgMarginPct: number | null`
  no front, com a célula exibindo "—" no `null`, pela mesma razão que o
  `chargebacksCents` do `by-method` já é anulável (§2.1).*
- O custo vem do catalog-service via `POST /api/v1/store/products/costs` com
  token `role=service`, em lotes de 200. **O custo unitário nunca é
  serializado na resposta** — só a margem agregada. Teste
  `TestSellersPerformance_NuncaVazaCusto` verifica o JSON cru por substring,
  porque um campo novo (`cost`, `cogs`) passaria despercebido por decodificação
  tipada.
- `sellerName` e `storeName` vêm de `GET {AUTH}/api/v1/admin/operators`,
  propagando o **token do admin que chamou** (não um token de serviço: a rota
  do auth-service já é `RequireAdmin`, e emitir identidade de serviço só para
  ler nomes ampliaria o alcance do `SERVICE_JWT_SECRET` sem necessidade). Se o
  auth-service estiver fora do ar, a tela **degrada para o id** em vez de
  falhar — o dono ainda precisa ver quanto vendeu.

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

## 5. Observabilidade — catalog-service (agregador) ✅ IMPLEMENTADO

```
GET {CATALOG_URL}/api/v1/admin/observability
→ ObservabilitySnapshot
```

> 🔴 **CORRIGIDO — mudança que o front precisa aplicar.** Este documento dizia
> `{API_URL}` (payment-service) e `app/src/lib/adminApi.ts` foi escrito assim
> (`adminGet<ObservabilitySnapshot>(PAYMENT_URL, ...)`). A rota foi
> implementada no **catalog-service**. Trocar `PAYMENT_URL` por `CATALOG_URL`
> nessa chamada é a única mudança necessária no front — o formato do payload é
> exatamente o especificado abaixo.
>
> **Por que mudou:** a rota não pertence a nenhum serviço em particular (é
> sobre todos eles), e o payment-service é o mais sensível do sistema. Dar a
> quem move dinheiro a função de cliente HTTP dos outros três amplia a
> superfície de ataque sem ganho nenhum. O agregador só **lê** métricas, então
> mora no serviço de menor privilégio que já tinha um grupo `/admin`.

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

### Como funciona (e o que ainda é aproximação)

O agregador sonda `/health` e `/metrics` dos quatro serviços **em paralelo**
(em série, quatro timeouts somariam 12s e o painel de diagnóstico seria a coisa
mais lenta da tela) e guarda o snapshot por **15s** — menor que o polling de
30s, para que um refresh manual durante um incidente traga dado novo, e
suficiente para que N abas abertas custem o mesmo que uma.

`/metrics` é lido com o `METRICS_TOKEN` no servidor. **O token nunca vai ao
navegador**: `/metrics` expõe volume financeiro, taxa de recusa de cartão e
topologia, e é fail-closed (sem token, 404). Esse é o motivo principal de haver
um agregado JSON em vez de o SPA falar Prometheus direto.

Limitações **conhecidas e deliberadas**, que o doc não pode omitir:

1. **A janela não é de 5 minutos.** Os contadores do Prometheus são cumulativos
   desde o boot do processo; uma janela real exige duas coletas e uma subtração
   (`rate()`). `errorRate` e `rpm` são médias da vida do processo. Um serviço
   de pé há dias com um pico agora mostra `rpm` baixo.
2. **p50/p95/p99 são estimados por interpolação nos buckets** — a mesma conta
   do `histogram_quantile`, com a mesma limitação: a precisão é a dos limites
   dos buckets do `pkg/metrics`. Um p95 real de 700 ms é reportado em algum
   ponto entre 500 ms e 1 s. É por isso que os limiares de severidade são
   grossos (1 s / 3 s), não finos.
3. **`uptimePct` é a fração de sondagens bem-sucedidas** desde que o
   catalog-service subiu, não um SLA. Reiniciar o catalog zera a conta.
   Uptime de verdade exige TSDB com retenção.
4. **`version` sai como `"unknown"`** — nenhum serviço publica hoje uma série
   `utilar_build_info{version=...}`. O agregador já sabe lê-la assim que
   existir.

As três primeiras somem quando houver um Prometheus de verdade fazendo
`rate()` sobre séries retidas (ver `docs/observability-alerts.md`). Até lá, um
número honesto e limitado é melhor que um campo inventado — e muito melhor que
`uptimePct: 100` fixo, que mentiria.

### Instrumentação: o que faltava

Só o **payment-service** usava `pkg/metrics`. Sem isso, o painel mostraria três
dos quatro serviços "de pé" e nada mais — cego justamente no catálogo (o
caminho mais lido do sistema) e no order-service (onde o pedido trava depois de
pago). Foram instrumentados agora:

| Serviço | `/metrics` | Observação |
|---|---|---|
| payment | ✅ (já tinha) | também publica as séries do outbox |
| catalog | ✅ **novo** | |
| order | ✅ **novo** | |
| auth | ❌ **pendente** | fora do escopo deste trabalho; aparece com latência 0 e `status` derivado só do `/health` até ser instrumentado |

Todos fail-closed por `METRICS_TOKEN`.

### Limiares (decididos no servidor)

| | `warn` | `critical` |
|---|---|---|
| Outbox — idade do mais antigo | ≥ 120 s | ≥ 900 s |
| Outbox — pendentes | ≥ 100 | ≥ 500 |
| Taxa de 5xx | ≥ 1% | ≥ 5% |
| p95 | ≥ 1 s | ≥ 3 s |
| Serviço sem resposta no `/health` | — | sempre `critical` |

A severidade é calculada no backend, não no front: são regras de negócio, e
regra duplicada no navegador diverge na primeira mudança — aqui divergir
significa o painel dizer "ok" durante um incidente. O front pode continuar com
`outboxSeverity` como fallback do modo mock; os números batem.

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
