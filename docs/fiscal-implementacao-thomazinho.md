# Emissão de nota fiscal — spec de implementação

> **Para:** Thomazinho · **Objetivo:** documentar tudo que precisa para emitir
> NF-e (compra web) e NFC-e (balcão/tablet), como integrar, e a ordem de
> implementação. Este é o **plano de código**; o *porquê* e as decisões de
> negócio/contábil estão em [`fiscal-nota-e-integracao.md`](fiscal-nota-e-integracao.md).

---

## 0. TL;DR — o que você vai construir

1. **Contratar um emissor de NF-e como serviço** (Focus NF-e recomendado) — ele
   fala com a SEFAZ, assina o XML e devolve a nota autorizada + DANFE. **Nós NÃO
   falamos direto com a SEFAZ.**
2. **Campos fiscais no produto** (migration no catalog-service) — NCM, CEST,
   CSOSN/CST, origem, unidade tributável.
3. **Módulo `fiscal` no order-service** com um cliente de emissor atrás de uma
   interface, mais uma tabela `fiscal_notes` e um **outbox** para nunca perder
   nem duplicar uma emissão.
4. **Dois gatilhos:**
   - **Web** → assíncrono, quando o pedido vira `paid` (evento `payment.confirmed`).
   - **Balcão** → síncrono, no `settle-external`, com **DANFE impresso na hora** e
     **contingência offline** se a SEFAZ cair.
5. **Testes** em cada camada, plugados na skill `test-utilar` (novo alvo `fiscal`).

---

## 1. Decisão de arquitetura: emissor como serviço (não SEFAZ direto)

Emitir NF-e/NFC-e é montar um XML no layout 4.00 (CFOP, NCM, CST/CSOSN, origem,
ICMS/PIS/COFINS, CEST…), **assinar com certificado A1 e-CNPJ**, mandar pro
webservice da **SEFAZ do estado**, tratar contingência (SVC-AN/SVC-RS), eventos
(cancelamento, carta de correção, inutilização) e as mudanças de layout — para
cada estado, para sempre.

**Não vamos fazer isso direto.** Contratamos um **emissor (API REST)**. Nós
mandamos os dados da venda; ele monta+assina+autoriza e devolve XML + DANFE.

| Provedor | Nota |
|---|---|
| **Focus NF-e** | ⭐ recomendado — API simples, NF-e+NFC-e, sandbox bom |
| **PlugNotas** | forte em NFC-e/Simples |
| **NFe.io / eNotas** | alternativas OK |

> **Regra de ouro do projeto:** o cliente **nunca dita valor** — e agora **nunca
> dita imposto**. CFOP, base e alíquota são resolvidos **no servidor** a partir
> da classificação do produto e do destino. Nada de campo fiscal vindo do corpo
> da requisição.

---

## 2. Os dois fluxos — e por que são diferentes

### 2.1 Compra web → NF-e (modelo 55) · **assíncrona**

Fluxo atual (não mexer nele, só pendurar a emissão):

```
POST /orders (pending_payment, estoque reservado)
   → pagamento (Appmax)
   → webhook → reconsulta ao PSP → payment.confirmed no Redpanda
   → consumer do order (internal/consumer/payment.go) → status = 'paid'   ← GATILHO NF-e
   → MarkShipped (exige trackingCode) — a mercadoria SAI aqui
```

- **Gatilho:** transição para `paid` no consumer (`payment.go`, `case
  "payment.confirmed": return model.StatusPaid`).
- **Regra fiscal:** a NF-e tem que **acompanhar a mercadoria** — então precisa
  estar **autorizada antes do `MarkShipped`**. Emitimos logo no `paid`, em
  background; o DANFE (PDF) vai no pacote.
- Pode ser assíncrona: o cliente não está esperando na tela.

### 2.2 Venda no balcão → NFC-e (modelo 65) · **síncrona**

```
POST /orders (channel=counter, operador, desconto no teto do cargo)
   → [aprovação do gerente se acima do teto]
   → POST /balcao/orders/:id/settle-external (NSU/bandeira/auth do POS + ledger)  ← GATILHO NFC-e
   → imprime DANFE NFC-e (com QR) no tablet, na hora
```

- **Gatilho:** `SettleExternal` (`external_settlement.go`) — é o "pagou" do balcão.
- **Síncrono:** o cliente está no balcão esperando o cupom. Precisa de resposta
  rápida do emissor **e** de um **modo de contingência offline**: se a SEFAZ (ou
  o emissor) cair, a NFC-e é emitida em contingência e transmitida depois — o
  vendedor **não pode parar de vender**.
- Sem endereço (é retirada); o cliente pode ser anônimo (NFC-e "sem CPF") ou com
  CPF na nota (já temos `CustomerDocument` no pedido de balcão).

> **Diferença que dita o design:** web = pode falhar e reprocessar via outbox;
> balcão = caminho síncrono com fallback de contingência. Trate como dois modos
> do mesmo módulo, não dois módulos.

---

## 3. Modelo de dados

### 3.1 Campos fiscais no produto — `catalog-service`

Migration **`017_product_fiscal_fields.{up,down}.sql`** (segue a numeração; a
última é `016_recommendations`). O produto **não** tem nada fiscal hoje.

```sql
-- 017_product_fiscal_fields.up.sql
ALTER TABLE products
  ADD COLUMN ncm            char(8),              -- Nomenclatura Comum do Mercosul (obrigatório p/ emitir)
  ADD COLUMN cest           char(7),              -- só p/ itens em ICMS-ST (muito material de construção é)
  ADD COLUMN origem         smallint DEFAULT 0,   -- 0=nacional … 8; tabela da SEFAZ
  ADD COLUMN csosn          char(4),              -- Simples Nacional (ex.: 102, 500). Mutuamente exclusivo com CST
  ADD COLUMN cst_icms       char(3),              -- Lucro Presumido/Real (se sair do Simples)
  ADD COLUMN unidade_trib   varchar(6) DEFAULT 'UN', -- UN, PC, KG, M, M2, L…
  ADD COLUMN fiscal_ready   boolean  DEFAULT false;  -- true só quando o contador validou a classificação

-- Não emitir nota de produto sem classificação: trava no app, mas deixa o
-- rastro no banco também.
COMMENT ON COLUMN products.fiscal_ready IS
  'Contador validou NCM/CEST/CSOSN. Publicar p/ venda com fiscal_ready=false é permitido, emitir NF-e não.';
```

> ⚠️ **Preencher NCM/CEST/CSOSN é trabalho do contador**, não seu. É curadoria,
> igual foi foto/preço. A migration só cria os campos; a classificação por
> produto entra depois (importação/curadoria). **ICMS-ST é o ponto mais
> traiçoeiro** — errar custa caro.

**Snapshot no pedido:** assim como o preço é "congelado" no item do pedido
(`UnitPrice`), os campos fiscais têm que ser **copiados para o item do pedido no
momento da venda** — a nota reflete o produto **como era na venda**, não como
está hoje. Ver 3.2.

### 3.2 Nota fiscal e snapshot fiscal do item — `order-service`

Migration **`008_fiscal_notes.{up,down}.sql`** (última é `007_returns`).

```sql
-- 008_fiscal_notes.up.sql

-- Snapshot fiscal por item: a nota usa ISTO, não uma consulta ao catálogo hoje.
ALTER TABLE order_items
  ADD COLUMN ncm          char(8),
  ADD COLUMN cest         char(7),
  ADD COLUMN origem       smallint,
  ADD COLUMN csosn        char(4),
  ADD COLUMN cst_icms     char(3),
  ADD COLUMN unidade_trib varchar(6);

CREATE TYPE fiscal_note_kind   AS ENUM ('nfe','nfce');           -- 55 / 65
CREATE TYPE fiscal_note_status AS ENUM (
  'pending','authorized','rejected','contingency','cancelled'
);

CREATE TABLE fiscal_notes (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id       uuid NOT NULL REFERENCES orders(id),
  kind           fiscal_note_kind   NOT NULL,
  status         fiscal_note_status NOT NULL DEFAULT 'pending',
  -- idempotência: uma referência estável por (pedido,modelo) que mandamos ao
  -- emissor para NUNCA emitir duas notas do mesmo pedido (duplo clique, replay).
  ref            text NOT NULL,
  provider       text NOT NULL,               -- 'focus', 'plugnotas'…
  provider_id    text,                        -- id da nota no emissor
  numero         bigint,                      -- número da NF-e/NFC-e autorizada
  serie          int,
  chave          char(44),                    -- chave de acesso
  protocolo      text,                        -- protocolo de autorização SEFAZ
  xml_url        text,                        -- XML autorizado (guardar 5 anos)
  danfe_url      text,                        -- PDF do DANFE
  qrcode         text,                        -- NFC-e
  reject_reason  text,                        -- rejeição da SEFAZ (cStat/xMotivo)
  total_cents    bigint NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  authorized_at  timestamptz,
  cancelled_at   timestamptz
);

-- Uma nota "viva" por (pedido, modelo). Índice parcial: cancelada não bloqueia
-- reemissão. (Cuidado com o padrão do projeto: índice parcial exige o WHERE no
-- ON CONFLICT.)
CREATE UNIQUE INDEX uq_fiscal_note_live
  ON fiscal_notes (order_id, kind)
  WHERE status <> 'cancelled';

-- Outbox transacional: grava a intenção de emitir NA MESMA transação da venda,
-- um drainer processa. Mesmo padrão do payment-service (internal/outbox).
CREATE TABLE fiscal_outbox (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     uuid NOT NULL,
  kind         fiscal_note_kind NOT NULL,
  attempts     int NOT NULL DEFAULT 0,
  next_try_at  timestamptz NOT NULL DEFAULT now(),
  created_at   timestamptz NOT NULL DEFAULT now(),
  done_at      timestamptz
);
```

---

## 4. Módulo `fiscal` no order-service

### 4.1 Interface do emissor (não acopla no provedor)

`services/order-service/internal/fiscal/emitter.go`

```go
package fiscal

import "context"

type NoteKind string
const (
    NFe  NoteKind = "nfe"   // modelo 55 — web
    NFCe NoteKind = "nfce"  // modelo 65 — balcão
)

// EmitInput é montado NO SERVIDOR a partir do pedido + snapshot fiscal dos itens.
// Nada aqui vem do corpo da requisição do cliente.
type EmitInput struct {
    Ref        string      // idempotência: estável por (pedido, modelo)
    Kind       NoteKind
    Order      OrderSnapshot   // itens c/ NCM/CEST/CSOSN, valores, destino, cliente
    Contingency bool           // NFC-e offline quando a SEFAZ cai
}

type EmitResult struct {
    ProviderID string
    Status     string   // authorized | rejected | contingency
    Numero     int64
    Serie      int
    Chave      string
    Protocolo  string
    XMLURL     string
    DANFEURL   string
    QRCode     string
    Reject     string   // cStat/xMotivo quando rejeitada
}

// Emitter é o contrato. Uma impl por provedor (focus, plugnotas) + um fake p/ testes.
type Emitter interface {
    Emit(ctx context.Context, in EmitInput) (EmitResult, error)
    Cancel(ctx context.Context, providerID, reason string) error
    // Consulta status (a autorização pode ser assíncrona no lado do emissor).
    Status(ctx context.Context, providerID string) (EmitResult, error)
}
```

Implementações:
- `fiscal/focus/` — cliente HTTP do Focus NF-e (usa `pkg/httpclient`,
  `pkg/retry` com **`Safety: Idempotent`** na consulta e **`NonIdempotent`** na
  emissão — reemitir cobra nota duplicada, mesmo risco do pagamento).
- `fiscal/fake/` — emissor determinístico para testes (sem rede).

### 4.2 Gatilho web (assíncrono)

No consumer, quando transiciona para `paid`, **grave uma linha no
`fiscal_outbox` na mesma transação** que marca o pedido pago. Um drainer
(`fiscal/drainer.go`, espelho do `payment-service/internal/outbox/drainer.go`)
pega a linha, chama `Emitter.Emit`, grava em `fiscal_notes`, respeita
`next_try_at`/`attempts` com backoff.

> **Por que outbox e não emitir inline no consumer:** se o emissor estiver fora,
> o pedido pago **não pode ficar sem nota nem travar o consumer**. O outbox
> separa "receber o pagamento" de "emitir a nota" com garantia de entrega.

### 4.3 Gatilho balcão (síncrono + contingência)

No `SettleExternal`, dentro da transação que marca pago + posta no ledger:
1. Copie o snapshot fiscal dos itens (já deve estar no `order_items`).
2. Chame `Emitter.Emit(kind=NFCe)` **síncrono** (timeout curto, ex. 8s).
3. **Autorizou** → grava `fiscal_notes`, devolve `danfe_url`/`qrcode` na resposta
   → o tablet imprime.
4. **SEFAZ/emissor fora** → emite em **contingência** (`status='contingency'`),
   imprime o DANFE de contingência, e o drainer retransmite depois. **A venda não
   trava.**

### 4.4 Cancelamento

- Casar com **devolução/estorno** (CDC — ver `docs/devolucao-e-troca.md`): ao
  autorizar um estorno, chame `Emitter.Cancel` dentro da **janela legal**
  (NF-e ~24h; varia). Fora da janela → **nota de devolução/entrada**, não
  cancelamento. Marque `status='cancelled'`, `cancelled_at`.

---

## 5. Config e segredos

Novas variáveis (order-service). **Segredo nunca versionado** (já coberto pelo
`.gitignore`):

```
FISCAL_PROVIDER=focus            # focus | plugnotas | fake
FISCAL_API_BASE=...              # base do emissor (sandbox vs produção)
FISCAL_API_TOKEN=...             # token do emissor  (Secrets Manager em prod)
FISCAL_EMPRESA_CNPJ=...          # CNPJ emitente (Utilar)
FISCAL_UF=...                    # UF da loja
FISCAL_AMBIENTE=homologacao      # homologacao | producao
```

- **Certificado A1 e-CNPJ**: quem hospeda é o **emissor** (Focus/PlugNotas
  guardam o certificado na conta) — nós **não** manuseamos o `.pfx`. Se um dia
  for integração direta, aí o certificado vira segredo nosso (não é o plano).
- Boot **fail-closed** fora de `DEV_MODE`: sem `FISCAL_API_TOKEN` em produção, o
  serviço sobe mas **marca emissão como indisponível** e alerta — não emite nota
  "silenciosamente sem nota".

---

## 6. Plano de testes (regra do dono: nada sem teste)

Cada item é teste de regressão nomeado pelo que previne. Novo alvo `fiscal` na
skill `test-utilar` (`run-tests.sh` + `SKILL.md`).

| Teste | Previne |
|---|---|
| `TestFiscal_ClienteNuncaDitaImposto` | CFOP/base/alíquota do corpo da request serem usados |
| `TestFiscal_ItemUsaSnapshotNaoCatalogoAtual` | nota refletir preço/NCM de hoje, não o da venda |
| `TestFiscal_NaoEmiteNotaDuplicadaMesmoPedido` | duplo clique/replay gerar duas notas (índice parcial + `ref`) |
| `TestFiscal_SemFiscalReadyNaoEmite` | emitir com classificação não validada pelo contador |
| `TestFiscal_BalcaoContingenciaNaoTravaVenda` | SEFAZ fora derrubar o balcão |
| `TestFiscal_Web_EmiteAntesDoShipped` | mercadoria sair sem DANFE |
| `TestFiscal_CancelaCasadoComEstorno` | estorno sem cancelar/estornar a nota |
| `TestFiscal_OutboxReprocessaAposFalha` | pedido pago ficar sem nota se o emissor cair |

Contrato contra o **sandbox** do emissor: um teste `Live` guardado por env
(`FISCAL_LIVE=1`), igual ao `appmax` live — pulado sem creds, não falha.

---

## 7. Ordem de implementação (fases)

1. **[Bloqueado no dono/contador]** Definir regime tributário + NCM/CEST/CSOSN
   por produto, e contratar o emissor + certificado A1. *Sem isso o resto não
   emite de verdade — mas dá pra codar tudo contra o `fake` e o sandbox.*
2. Migration `017_product_fiscal_fields` (catalog) + expor os campos no
   `AdminProduct` e na importação/curadoria.
3. Migration `008_fiscal_notes` (order) + **snapshot fiscal** ao criar o pedido
   (`order.go Create`, copiar do catálogo como já faz com preço).
4. Módulo `fiscal`: interface `Emitter`, impl `fake`, tabela + repositório.
5. **Web**: outbox no `paid` + drainer. Testes.
6. **Balcão**: emissão síncrona no `SettleExternal` + contingência. Testes.
7. Impl `focus` (ou `plugnotas`) contra o **sandbox**. Testes de contrato.
8. Cancelamento casado com devolução.
9. Expor no admin: status da nota no detalhe do pedido, link do DANFE/XML,
   reemitir/cancelar. Front (loja/balcão): link do DANFE no pós-venda.
10. Alarme: pedido `paid` há > X sem nota autorizada (reusar o painel de
    travados do admin-dashboard).

---

## 8. Checklist de "pronto"

- [ ] Emissor contratado, certificado A1 na conta do emissor, sandbox funcionando.
- [ ] Produtos com `fiscal_ready=true` (contador validou NCM/CEST/CSOSN).
- [ ] NF-e web emitida no `paid`, **antes** do `MarkShipped`, DANFE no pacote.
- [ ] NFC-e balcão síncrona no `settle-external`, DANFE+QR no tablet, contingência OK.
- [ ] Duplo clique/replay não gera nota dupla (índice parcial + `ref`).
- [ ] Cancelamento casado com estorno dentro da janela legal.
- [ ] XML autorizado guardado (5 anos) — `xml_url` acessível.
- [ ] Todos os testes verdes no alvo `fiscal` da skill `test-utilar`.
- [ ] Boot fail-closed em produção (sem token → não emite silenciosamente).

---

## 9. Fora de escopo (deixar explícito)

- **Integração direta com a SEFAZ** (assinatura de XML, webservices por estado):
  não faremos; é o emissor.
- **Apuração de impostos / SPED / obrigações acessórias** (DAS, EFD): é do
  **contador/ERP contábil**, não deste módulo. Aqui só **emitimos o documento**.
- **Venda fracionada** (2,5 m × R$ 1,89 gerando meio centavo): quando entrar,
  casar com a virada de `float64` → decimal (ver `psp/appmaxv1/money_test.go`) —
  a base de cálculo da nota precisa bater centavo a centavo.
```
