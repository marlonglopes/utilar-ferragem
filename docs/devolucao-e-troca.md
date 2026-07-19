# Devolução e troca

> **Obrigação legal, não feature.** Até 2026-07-19 não existia fluxo nenhum: o
> cliente não tinha como pedir devolução e a loja não tinha como registrar. Isso
> é descumprimento direto do Código de Defesa do Consumidor.

Serviço: `order-service`. Regras puras em `internal/returns/`, endpoints em
`internal/handler/returns.go`, schema na migration `007_returns`.

---

## 1. As duas bases legais

Elas **não são a mesma coisa** e não podem virar um campo só.

| | **Arrependimento** (`regret`) | **Vício do produto** (`defect`) |
|---|---|---|
| Base | CDC **art. 49** | CDC **art. 26** |
| Prazo | **7 dias corridos** | 90 dias (bem durável) |
| Conta a partir de | **RECEBIMENTO** | quando o vício ficou evidente |
| Precisa de motivo? | **NÃO** | Sim |
| Tem análise? | **NÃO — direito incondicional** | Sim |
| Pode ser recusado? | **NUNCA** | Sim, com justificativa |
| Produto precisa ter defeito? | Não | Sim |
| Frete de volta | Da loja | Da loja |

Toda venda online é "fora do estabelecimento comercial", então o art. 49 se
aplica a 100% do canal web.

**A base legal é DERIVADA da data, nunca escolhida.** Não existe campo `kind` no
payload de entrada. Se o cliente escolhesse, todo mundo marcaria
"arrependimento" (que não tem análise) inclusive fora dos 7 dias; se a loja
escolhesse, todo arrependimento viraria "vício" para poder ser analisado.

**Consequência prática:** dentro dos 7 dias a devolução nasce **já aprovada**
(`status = approved`, `decided_by = system:cdc-art-49`). Mandar para uma fila de
análise humana um direito que a lei diz ser incondicional só cria o lugar onde
ele vai ser negado por engano.

### Contagem do prazo — a parte que mais dá briga

`ResolveDeadline` escolhe a data base nesta ordem, e a ordem **é a regra**:

| Situação | `basis_source` | Data base | Janela |
|---|---|---|---|
| Venda de balcão | `balcao_pickup` | `paid_at` (retirada no ato) | normal |
| `delivered_at` preenchido | `delivered_at` | a data de entrega | normal |
| Ainda não entregue | `not_delivered` | — | **ABERTA** |
| Entregue **sem** `delivered_at` | `unknown` | — | **ABERTA** |

⚠️ **Pedido sem data de entrega**: acontece com pedido migrado, com status
corrigido à mão, e com balcão. A política é: **sem data de recebimento, a loja
não pode alegar prazo vencido.** O ônus de provar quando entregou é do
fornecedor — quem tem o comprovante da transportadora é ele.

A alternativa (contar do `created_at`) parece conservadora e é o contrário:
encurta o prazo do consumidor usando uma data que a lei não manda usar, e a loja
perde no primeiro processo. Errar a favor do consumidor custa uma devolução;
errar contra custa multa.

O registro fica marcado com `basis_source = 'unknown'` e um `slog.Warn` para que
o buraco de dado seja consertado **na origem**, não na régua do prazo.

O prazo aplicado é **congelado** no registro (`deadline_basis`, `deadline_at`,
`basis_source`). Sem congelar, um `delivered_at` corrigido meses depois mudaria
retroativamente se a devolução foi tempestiva.

---

## 2. Máquina de estados

```
requested ──┬──> approved ──> in_transit ──> received ──> refunded
            │        │                          ▲            ▲
            │        └──> cancelled          ESTOQUE      DINHEIRO
            ├──> rejected  (só vício)          volta         sai
            └──> cancelled (cliente desiste)
```

As duas invariantes que valem dinheiro:

1. **Estoque só volta em `received`** — quando a mercadoria foi *conferida* na
   loja. Repor em `requested`/`approved` coloca à venda um produto que ainda
   está na casa do cliente, ou que nunca vai ser postado. O sistema venderia o
   que não tem, e quem descobre é a **segunda** venda.

2. **Dinheiro só sai em `refunded`, e só a partir de `received`.** Não existe
   aresta `approved → refunded`: estornar antes de a mercadoria voltar é
   entregar produto **e** dinheiro para a mesma pessoa.

`rejected` é inalcançável para `regret` — barrado em `returns.CanReject` **e**
pela constraint `returns_regret_cannot_be_rejected` no banco.

---

## 3. Endpoints

### Cliente (`RequireUser`)

| Método | Rota | O quê |
|---|---|---|
| `POST` | `/api/v1/orders/:id/returns` | abre a devolução |
| `GET` | `/api/v1/orders/:id/returns` | devoluções do pedido |
| `GET` | `/api/v1/returns/:rid` | detalhe |

```jsonc
// POST /api/v1/orders/{id}/returns
{
  "items": [ { "orderItemId": "uuid", "quantity": 3 } ],
  "reason": "opcional — só exigido fora dos 7 dias"
}
```

Não existe campo de **valor**: o valor a estornar é calculado no servidor a
partir do preço *snapshot* gravado em `order_items`. Deixar o cliente informar
quanto quer de volta é deixá-lo ditar quanto dinheiro sai.

O cliente manda `orderItemId`, não `productId`: o mesmo produto pode aparecer em
duas linhas do pedido com preços distintos, e "devolver o produto X" seria
ambíguo sobre qual preço estornar.

### Loja (`RequireRole admin|operator`)

| Método | Rota | O quê |
|---|---|---|
| `GET` | `/api/v1/admin/returns` | fila (`?status=`) |
| `PATCH` | `/api/v1/admin/returns/:rid/approve` | defere |
| `PATCH` | `/api/v1/admin/returns/:rid/reject` | indefere (só vício, exige `note`) |
| `PATCH` | `/api/v1/admin/returns/:rid/receive` | **← estoque volta** |
| `PATCH` | `/api/v1/admin/returns/:rid/refund` | **← dinheiro sai** |

`/receive` aceita `restockableItems`: o conferente marca o que **não** volta ao
estoque (chegou quebrado). Vazio = tudo volta. O estorno ao consumidor continua
devido; o que muda é o que vai para a prateleira.

### Erros específicos

| Código | HTTP | Quando |
|---|---|---|
| `return_window_expired` | 422 | fora dos dois prazos |
| `split_requires_full_refund` | 422 | split + devolução parcial |
| `regret_is_unconditional` | 422 | tentativa de recusar arrependimento |
| `not_found` | 404 | pedido de outro usuário (anti-enumeração) |

---

## 4. Devolução parcial

O cliente compra 10 e devolve 1. `order_return_items` diz **qual** item e
**quantos**.

- O saldo devolvível desconta o que já foi devolvido antes, ignorando
  devoluções `rejected`/`cancelled` — uma devolução que a própria loja negou não
  pode continuar consumindo o saldo do cliente.
- Itens repetidos no mesmo payload são **somados antes** de validar (mandar o
  mesmo item 3× com quantidade 1 num item comprado 2× tem que falhar).
- **Frete só volta na devolução TOTAL**, e só no arrependimento. Devolver o
  frete inteiro numa parcial daria à loja um prejuízo que a lei não impõe: a
  entrega dos demais itens de fato aconteceu.
- "Total" considera o histórico: devolver os 7 restantes de um item já devolvido
  em 3 **é** devolução total do saldo.

---

## 5. Split de pagamento — trava do PSP

⚠️ A Appmax só aceita **estorno total** em pedido com Payment Split
(`docs/appmax-v1-appstore.md` § 5).

`orders.payment_split` + `returns.ErrSplitPartialRefund` recusam a devolução
parcial **na hora da solicitação**, com 422 e mensagem acionável ("selecione
todos os itens"). Sem essa trava, a parcial seria aceita, o cliente avisado de
que o dinheiro está voltando, o produto devolvido, o estoque reposto — e a
chamada só falharia lá no PSP, com produto devolvido e dinheiro preso.

**Estado atual:** `payment_split` tem `DEFAULT false` e **nenhum produtor**.
Isso é *correto no modelo lojista* — split só existe quando há um terceiro
recebendo parte do dinheiro, o que só acontece em marketplace. A trava está
implementada e testada, mas **dormente** até a decisão da seção 8.

---

## 6. Como o estorno chega ao livro contábil

Mesmo desenho da liquidação externa, e pelo mesmo motivo: **o livro é único e
vive no payment-service**. Dois serviços escrevendo o próprio livro é como se
perde a garantia de que tudo soma zero.

```
order-service                                    payment-service
─────────────                                    ───────────────
PATCH /admin/returns/:rid/refund
  ├─ 1. QUEM     returns.CanDecide (403)
  ├─ 2. estado   received → refunded (FOR UPDATE)
  ├─ 3. RASTRO   return_audit_events  ← FAIL-CLOSED, mesma transação
  ├─ COMMIT
  └─ paymentclient.PostReturnRefund ──────────>  POST /internal/v1/ledger/return-refund
        Bearer <token role=service>                 RequireService(SERVICE_JWT_SECRET)
        SERVICE_JWT_SECRET, TTL 2min                 │
                                                     └─ ledger.Poster.Post
                                                          D 3.1.8 Estornos
                                                          C 1.1.1 Caixa PSP
```

**Idempotência: a chave é o `returnID`**, não o `paymentID`.

Isto é importante: o construtor existente `ledger.Refund` chaveia por
`paymentID + ":" + "total"/"parcial"`. Um pedido com **duas devoluções
parciais** colidiria — a segunda seria descartada como duplicata da primeira e o
lançamento sumiria (despesa subestimada, sem o contador ter como saber). Por
isso o handler `return_refund.go` monta o `TxInput` diretamente com
`source_type = order`, `source_id = returnID`.

**Ordem e assimetria:** estorna primeiro, lança depois. Se o lançamento falhar,
temos estorno sem lançamento — despesa **subestimada**, detectável e replicável
(a chamada é idempotente). A ordem inversa produziria estorno no livro para
dinheiro que não saiu: despesa inventada. A falha grava
`return.ledger_post_failed` na trilha e `ledger_posted` fica `false`.

**Frete ressarcido** vai em posting próprio (`memo: "frete ressarcido"`) para o
contador ver a composição sem abrir o pedido.

**Sem retry automático** nessa chamada, apesar de ser idempotente do outro lado:
a idempotência depende de uma garantia que vive noutro serviço e noutro banco, e
o custo de estar errado é lançar o mesmo estorno duas vezes. O retry é
explícito — o operador chama de novo, com decisão humana no meio. Mesmo
princípio de `appmaxv1.isFinancialRoute`.

---

## 7. Pendências

### 7.1 ⚠️ O estorno real no PSP não existe

**Nada neste fluxo devolve dinheiro ao cartão/Pix do cliente.** O que existe é
o **lançamento contábil**. Devolver de fato exige:

- `psp.Gateway` ganhar `Refund(ctx, paymentID, amountCents)`;
- implementação em `appmaxv1` (rota `/v1/payments/*` → **rota financeira, sem
  retry**);
- tratamento de estorno recusado pelo PSP (saldo insuficiente na conta Appmax é
  o caso comum) — hoje não há estado para "estorno solicitado, aguardando PSP";
- webhook `order_refund` / `order_partial_refund` já mapeado em
  `docs/appmax-v1-appstore.md` § 204, ainda não ligado à devolução.

Hoje, na prática, o operador estorna **pelo painel da Appmax** e usa
`/refund` para registrar o fato no sistema e no livro.

### 7.2 ⚠️ Reposição de estoque depende do catalog-service

`POST /api/v1/internal/restock` **não existe** no catalog-service — ele expõe
apenas `/reservations`, `/commit` e `/release`.

`Release` **não serve**: ele cancela uma *reserva* de pedido não pago. Numa
devolução a baixa já aconteceu (produto pago, separado, entregue, devolvido), e
o que se precisa é **incremento** de saldo. Chamar `Release` não faria nada e o
erro seria silencioso.

Contrato esperado:

```jsonc
POST /api/v1/internal/restock     // role=service
{
  "returnId": "uuid",             // chave de deduplicação (NÃO o orderId:
                                  //   um pedido tem N devoluções parciais)
  "reason": "customer_return",
  "items": [ { "productId": "uuid", "quantity": 3 } ]
}
```

**Enquanto não existir:** o recebimento é registrado normalmente,
`stock_returned` fica `false`, e `return.stock_restore_failed` entra na trilha.
O saldo fica **subestimado** (venda perdida, detectável pelo índice
`idx_returns_pendencias`) e nunca superestimado (venda do que não existe).

### 7.3 Menores

- Frontend: nenhuma tela. As rotas existem, o `app/` não as consome.
- Etiqueta de postagem reversa (a loja paga o frete de volta no art. 49) — hoje
  é processo manual fora do sistema.
- Taxonomia fechada de `reason_code` para vício: hoje é `defect_reported`
  genérico.
- `refund_shipping` não é monetariamente atualizado (o art. 49 fala em valores
  "monetariamente atualizados"). Para os prazos envolvidos (dias), o impacto é
  desprezível; vale registrar.

---

## 8. ⚠️ Lojista ou marketplace? — precisa de decisão

**Esta implementação assume LOJISTA:** a Utilar vende o próprio produto, e a
Utilar responde pela devolução — recebe, confere, repõe o estoque e estorna.

O código tem indícios das duas coisas: existe papel `seller`, `order_items` tem
`seller_id`/`seller_name`, o dashboard tem `/sellers/performance`, e a Appmax v1
tem Payment Split configurado. Isso é a cara de um marketplace. Mas o produto
descrito no `CLAUDE.md` ("loja online de ferragem **com PDV de balcão para a
loja física**") é a cara de um lojista.

**O que muda se for marketplace:**

| Aspecto | Lojista (implementado) | Marketplace |
|---|---|---|
| Quem responde | Utilar | Solidariamente Utilar + vendedor (CDC art. 18/20 — o intermediário **não** se exime) |
| Para onde volta o produto | CD da Utilar | Endereço do vendedor |
| Quem confere | Operador da Utilar | Vendedor, com prazo e escalonamento se ele sumir |
| Quem estorna | Utilar, direto | Utilar estorna ao cliente e **debita o vendedor** |
| Ledger | 1 lançamento (`D 3.1.8 / C 1.1.1`) | + reversão de `seller_split` (`D 2.x obrigação com vendedor / C 1.1.1`) |
| `payment_split` | sempre `false` (correto) | **real** — a trava de estorno parcial fica ativa |
| Estorno parcial na Appmax | funciona | **proibido** — todo pedido com split vira devolução total ou nada |
| Autorização | `admin`/`operator` decidem | `seller` precisa ver e decidir **só as próprias**, e não pode virar atendente geral |
| Prazo | só o legal | + SLA do vendedor, com default a favor do consumidor |

**Trabalho adicional se for marketplace:** ~2–3× o que foi feito. O ponto mais
caro não é o estorno — é o **split reverso** no ledger e o fato de que a Appmax
proíbe estorno parcial em pedido com split, o que na prática **impede devolução
parcial em qualquer pedido multi-vendedor**. Isso é uma limitação de produto, não
de código, e precisa ser decidida antes de vender.

**O que preservei para não fechar a porta:** `payment_split` já existe no schema
e a trava já está implementada e testada; `returns.canActOnBehalf` deixa
`seller` de fora **de propósito** (lojista do marketplace ≠ atendente — a mesma
confusão que já custou caro no PDV); e `order_return_items` guarda `product_id`,
que é por onde se chega ao vendedor.

---

## 9. Testes

Puros (`internal/returns`, rodam sempre, sem banco):

- `TestArrependimentoNoDia7AindaVale` / `...NoDia8ViraVicio` / `...NoInstanteExatoDoLimite`
- `TestRegression_PedidoSemDataDeEntregaNaoPerdeOPrazo`
- `TestRegression_ClienteNaoDevolvePedidoDeOutro`, `TestRegression_SellerNaoEAtendente`
- `TestRegression_NaoDevolveMaisDoQueComprou`, `TestRegression_ItemRepetidoNoPedidoESomadoAntesDeValidar`
- `TestRegression_SplitRecusaDevolucaoParcialAntesDeChegarNoPSP`
- `TestRegression_EstoqueSoVoltaNoRecebimento`, `TestRegression_DinheiroSoSaiDepoisDaMercadoriaChegar`
- `TestRegression_ArrependimentoNaoPodeSerRecusado`

Integração (`internal/handler`, skipam sem banco):

- `TestRegression_EstoqueVoltaSoNoRecebimentoNaoNaSolicitacao`
- `TestRegression_DinheiroSoSaiDepoisDoRecebimento`
- `TestRegression_EstornoNaoSaiDuasVezes`
- `TestRegression_ClienteNaoAbreDevolucaoDePedidoAlheio`
- `TestClienteNaoEstornaAPropriaDevolucao`
- `TestDevolucaoParcialEstornaSoOItemDevolvido`
- `TestTrilhaDeAuditoriaGravaPessoaEValor`
- `TestArrependimentoNaoPodeSerRecusadoPeloEndpoint`
