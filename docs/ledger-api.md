# API Contábil — contrato para o dashboard

**Serviço**: payment-service (`:8090`)
**Prefixo**: `/api/v1/ledger`
**Autenticação**: `Authorization: Bearer <JWT>` **com `role=admin`**. Qualquer outra role → `403 {"code":"forbidden"}`.
**Última atualização**: 2026-07-18

> Este documento é o contrato. Se algo aqui divergir do código, o código está errado — abra issue em vez de adaptar o front.

---

## Convenções que valem para TODOS os endpoints

### 1. Dinheiro é `int` em CENTAVOS. Sempre.

Todo campo monetário termina em `Cents` e é um **inteiro**. Nunca há `float` no payload.

```json
{ "grossCents": 128450 }   // R$ 1.284,50
```

O front divide por 100 **só na hora de renderizar**. Não some, não acumule e não faça média em `float` — `0.1 + 0.2 !== 0.3` e o erro acumula ao longo de mil linhas. Se precisar somar no cliente, some os inteiros.

### 2. Valores são sempre POSITIVOS; o significado está no nome do campo

`refundsCents: 5000` quer dizer "cinco mil centavos foram estornados", **não** "-5000 de receita". Quem decide o sinal na tela é o front.

### 3. Janela é `[from, to)` — `to` é EXCLUSIVO

| Query | Significado |
|---|---|
| `?period=2026-07` | mês de julho inteiro (equivale a `from=2026-07-01&to=2026-08-01`) |
| `?from=2026-07-01&to=2026-07-08` | 1º a 7 de julho (o dia 8 **não** entra) |
| `?from=<RFC3339>&to=<RFC3339>` | precisão de instante |

Datas aceitas em `YYYY-MM-DD` ou RFC3339. Tudo é **UTC**.
Janela máxima: **400 dias**. Acima disso → `400`; use o export.

### 4. Envelope de erro (igual ao resto da plataforma)

```json
{ "error": "mensagem", "code": "bad_request", "requestId": "01J..." }
```

Códigos: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `db_error`, `unavailable`.

---

## Relatórios

### `GET /api/v1/ledger/summary`

O cartão principal do dashboard.

```json
{
  "from": "2026-07-01T00:00:00Z",
  "to": "2026-08-01T00:00:00Z",
  "currency": "BRL",
  "grossCents": 12845000,
  "pspFeesCents": 513800,
  "anticipationFeesCents": 0,
  "refundsCents": 264000,
  "chargebacksCents": 47500,
  "sellerSplitCents": 7000000,
  "netCents": 5019700,
  "transactionCount": 342
}
```

`netCents = grossCents − pspFeesCents − anticipationFeesCents − refundsCents − chargebacksCents − sellerSplitCents`

**Receita é BRUTA**: `grossCents` é o valor cheio pago pelos compradores. A taxa do gateway e o repasse ao vendedor são linhas próprias, não descontos embutidos. Isso é deliberado — é o que permite responder "quanto pagamos de MDR neste trimestre".

### `GET /api/v1/ledger/by-method`

```json
{
  "from": "...", "to": "...",
  "methods": [
    { "method": "pix",    "grossCents": 8000000, "pspFeesCents": 80000,  "refundsCents": 0,      "netCents": 7920000, "saleCount": 210 },
    { "method": "card",   "grossCents": 4500000, "pspFeesCents": 420000, "refundsCents": 264000, "netCents": 3816000, "saleCount": 118 },
    { "method": "boleto", "grossCents": 345000,  "pspFeesCents": 13800,  "refundsCents": 0,      "netCents": 331200,  "saleCount": 14 }
  ]
}
```

`method` pode vir `""` em lançamentos sem forma de pagamento (saque, ajuste). Trate como "Outros" — **não** filtre fora, senão a soma por método deixa de bater com o `summary`.

Array sempre presente; pode ser vazio.

### `GET /api/v1/ledger/daily`

Série temporal para o gráfico. Máximo 400 pontos.

```json
{
  "from": "...", "to": "...",
  "points": [
    { "day": "2026-07-01", "grossCents": 412000, "pspFeesCents": 16480, "refundsCents": 0, "netCents": 395520 }
  ]
}
```

**Dias sem movimento não aparecem.** O front preenche os buracos com zero — não assuma um ponto por dia.

### `GET /api/v1/ledger/trial-balance`

Balancete. `balanced: false` é **incidente**, não erro de tela: mostre alerta vermelho.

```json
{
  "from": "...", "to": "...",
  "lines": [
    { "account": "1.1.1", "name": "Caixa em trânsito no PSP", "type": "asset",
      "debitsCents": 12845000, "creditsCents": 7825300, "balanceCents": 5019700 }
  ],
  "totalDebitsCents": 25690000,
  "totalCreditsCents": 25690000,
  "balanced": true
}
```

`balanceCents` já vem no **sentido natural** da conta (positivo = saldo normal).
`type`: `asset` | `liability` | `equity` | `revenue` | `expense`.

### `GET /api/v1/ledger/entries?limit=500`

Razão (lista de partidas), ordenado cronologicamente. `limit` default 500, máximo 50000.

```json
{
  "from": "...", "to": "...",
  "entries": [
    {
      "transactionId": "9f8c...", "occurredAt": "2026-07-10T14:02:11Z",
      "kind": "sale", "sourceType": "payment", "sourceId": "3b1e...",
      "description": "Venda pix (pedido 7a2f...)",
      "account": "1.1.1", "accountName": "Caixa em trânsito no PSP",
      "side": "debit", "amountCents": 26400,
      "paymentMethod": "pix", "sellerId": "", "memo": "captura do pedido 7a2f...",
      "requestId": "01J..."
    }
  ]
}
```

### `GET /api/v1/ledger/transactions/:id`

Um lançamento com suas partidas. `404` se não existe.

```json
{
  "id": "9f8c...", "occurredAt": "...", "period": "2026-07",
  "kind": "sale", "sourceType": "payment", "sourceId": "3b1e...",
  "description": "...", "currency": "BRL", "requestId": "01J...",
  "reversesId": "", "createdBy": "system", "createdAt": "...",
  "totalCents": 27456,
  "postings": [
    { "Account": "1.1.1", "Side": "debit",  "Amount": 26400, "PaymentMethod": "pix", "SellerID": "", "Memo": "..." }
  ]
}
```

---

## Plano de contas

| Código | Nome | Tipo | Natureza |
|---|---|---|---|
| `1.1.1` | Caixa em trânsito no PSP | asset | devedora |
| `1.1.2` | Banco - conta movimento | asset | devedora |
| `2.1.1` | Repasses a vendedores | liability | credora |
| `3.1.1` | Receita bruta de vendas | revenue | credora |
| `3.1.8` | Estornos e devoluções | revenue | **devedora** (redutora) |
| `3.1.9` | Chargebacks | revenue | **devedora** (redutora) |
| `4.1.1` | Taxas do gateway (PSP) | expense | devedora |
| `4.1.2` | Taxa de antecipação | expense | devedora |
| `4.2.1` | Custo de repasse a vendedor | expense | devedora |

`kind` possíveis: `sale`, `psp_fee`, `refund`, `chargeback`, `seller_split`, `seller_withdrawal`, `payout`, `anticipation_fee`, `reversal`, `adjustment`.

---

## Correção de lançamento

### `POST /api/v1/ledger/transactions/:id/reverse`

**O livro é imutável.** Não existe endpoint de edição nem de exclusão — o banco recusa `UPDATE`/`DELETE` por trigger. Corrigir = lançar o estorno.

```json
// request
{ "reason": "valor lançado em duplicidade na conciliação de 07/2026" }
```

`reason` obrigatório, **mínimo 10 caracteres**. Retorna `201` com o lançamento de estorno (mesmas contas, lados invertidos, `reversesId` apontando pro original).

- `404` — lançamento não existe
- `409` — já estornado

O estorno é datado de **agora**, não da data do original: estornar algo de um mês fechado cai no mês aberto. O front deve deixar isso claro na confirmação.

---

## Fechamento de período

### `GET /api/v1/ledger/periods?limit=24`

```json
{ "periods": [
  { "period": "2026-06", "status": "closed", "closedAt": "2026-07-02T09:14:00Z", "closedBy": "user-uuid", "entriesCount": 1284 }
] }
```

### `GET /api/v1/ledger/periods/:period` (`:period` = `YYYY-MM`)

Inclui `closingBalances` (mapa `conta → saldo em centavos`) e `totals` (mesmo shape do `summary`) quando fechado.

### `POST /api/v1/ledger/periods/:period/close`

Trava o mês. Depois disso, **nenhum lançamento** com data naquele mês é aceito — a trava é no banco, vale inclusive para jobs e para `psql`.

- `400` — período ainda não terminou, ou balancete não fecha
- `409` — já fechado

**Reabrir não existe.** Nem por API, nem pela aplicação — o banco recusa. É intencional: reabrir invalidaria todo balanço já entregue ao contador. O front não deve oferecer o botão.

---

## Reconciliação

### `POST /api/v1/ledger/reconcile?period=2026-07`

Compara cada pagamento local com o que o PSP diz. Síncrono; máximo 500 pagamentos por execução.

```json
{
  "id": "run-uuid", "provider": "appmax-v1",
  "startedAt": "...", "finishedAt": "...", "from": "...", "to": "...",
  "checkedCount": 342, "errorCount": 0,
  "status": "discrepancies",
  "discrepancies": [
    {
      "id": "disc-uuid", "runId": "run-uuid",
      "paymentId": "3b1e...", "pspPaymentId": "998877",
      "kind": "amount_mismatch", "severity": "critical",
      "localValue": "R$ 264,00", "pspValue": "R$ 1,00",
      "amountDeltaCents": -26300,
      "detail": "divergência de VALOR — exige apuração humana...",
      "detectedAt": "..."
    }
  ]
}
```

**`status: "discrepancies"` retorna HTTP 200.** A rotina *rodou* — divergência é o produto dela, não uma falha da chamada. Não trate como erro de rede.

`status`: `ok` | `discrepancies` | `failed`.

| `kind` | severidade | significado |
|---|---|---|
| `amount_mismatch` | critical | valor local ≠ valor no PSP |
| `missing_at_psp` | critical | temos um `psp_payment_id` que o PSP não conhece |
| `status_mismatch` | high | status local diverge do PSP |
| `ledger_missing` | high | pagamento confirmado sem lançamento no livro |
| `psp_error` | medium | não deu para consultar (não é divergência provada) |

**A reconciliação NUNCA corrige nada.** Não altera pagamento, não altera livro. Ela só reporta. Divergência de dinheiro é bug nosso ou fraude — nos dois casos, auto-corrigir apagaria a evidência.

`amountDeltaCents` é `psp − local`: negativo significa que o PSP cobrou **menos** do que registramos.

### `GET /api/v1/ledger/discrepancies?limit=200`

Fila de trabalho do financeiro: divergências ainda não resolvidas.

### `POST /api/v1/ledger/discrepancies/:id/resolve`

```json
{ "note": "conferido com o extrato: cobrança parcial autorizada pelo cliente" }
```

`note` obrigatória, mínimo 10 caracteres. Retorna `204`. **Não** altera o pagamento nem o livro — apenas registra que um humano olhou e o que concluiu (com registro na trilha de auditoria).

---

## Exportação para o contador

### `GET /api/v1/ledger/export?period=2026-07&format=csv`

| `format` | Arquivo | Conteúdo |
|---|---|---|
| `csv` (ou `razao`) | `utilar-razao-<janela>.csv` | livro razão |
| `balancete` | `utilar-balancete-<janela>.csv` | balancete |
| `ofx` | `utilar-extrato-<janela>.ofx` | extrato OFX 1.0.2 da conta de caixa |

Resposta é `attachment` com `Cache-Control: no-store`. **Cada exportação é registrada na trilha de auditoria** (quem, quando, qual janela) — é o faturamento inteiro saindo do sistema.

**Formatos brasileiros (não mexer sem falar com o contador):**
- CSV: separador `;`, decimal com **vírgula**, BOM UTF-8, data `dd/mm/aaaa`, débito e crédito em colunas separadas.
- OFX: decimal com **ponto** (padrão americano), `FITID` estável para reimportação não duplicar.

O front deve baixar via link direto com o token — não tente renderizar o corpo.

---

## Trilha de auditoria

### `GET /api/v1/ledger/audit?entityType=&entityId=&actorId=&fromSeq=0&limit=200`

Registros append-only encadeados por hash, em ordem crescente de `seq`.

```json
{ "records": [
  { "Seq": 1284, "OccurredAt": "...", "Service": "payment-service",
    "ActorID": "user-uuid", "ActorRole": "admin", "ActorIP": "203.0.113.9",
    "EntityType": "ledger_transaction", "EntityID": "9f8c...",
    "Action": "create", "OldValue": null, "NewValue": {...},
    "RequestID": "01J...", "PrevHash": "abc...", "Hash": "def..." }
] }
```

Nenhum registro contém senha, token, PAN ou CVV — o pacote mascara antes de gravar.

### `GET /api/v1/ledger/audit/verify`

Recalcula a cadeia inteira.

```json
{ "valid": true, "headSeq": 1284, "headHash": "def0123..." }
```

Adulterada:

```json
{ "valid": false, "headSeq": 1284, "headHash": "...", "brokenAtSeq": 412, "kind": "hash_mismatch", "error": "..." }
```

**Sempre HTTP 200** — inclusive quando `valid: false`. A *verificação* funcionou; um 5xx faria o dashboard mostrar "erro de rede" no exato momento em que precisa gritar "a trilha foi adulterada". Renderize `valid: false` como alerta crítico.

`headHash` é o que se publica externamente como âncora (ver "limite honesto" no doc do `pkg/audit`).

---

## Correlação e depuração

Todo response carrega `X-Request-Id`. O mesmo id atravessa payment → order → auth → PSP e aparece em `requestId` nos lançamentos e na trilha. Ao reportar um problema, mande o `X-Request-Id` — ele resolve a investigação inteira.
