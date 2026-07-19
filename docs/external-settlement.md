# Liquidação externa — venda de balcão paga na maquininha da loja

> Contrato de API + desenho. Consumido pelo PDV (SPA) e pelo dashboard contábil.

## O problema que isto resolve

A maquininha da loja é de um **adquirente próprio, fora da Appmax**. O dinheiro
entra por fora do nosso PSP: não existe cobrança criada por nós, não existe
webhook, não existe transação de gateway nenhuma.

Até aqui o PDV gravava essas vendas como `paymentMethod: "card"`. Valor e
desconto certos, **meio de pagamento errado** — e o erro custava caro:

| Consequência | Detalhe |
|---|---|
| Livro contábil mentia | Registrava uma transação de PSP que nunca existiu, na conta `1.1.1` (caixa em trânsito no PSP). |
| Conciliação quebrada para sempre | `reconcile.go` compara nosso livro com o extrato da Appmax. Todo centavo de maquininha viraria divergência permanente — e alerta que sempre toca é alerta que ninguém olha. |
| Relatório por método errado | Maquininha somava com cartão online; impossível responder "quanto vendemos por qual meio". |

## O desenho escolhido

**`POST /api/v1/balcao/orders/:id/settle-external` no order-service**, com o
lançamento contábil delegado ao payment-service por uma rota `/internal`.

### Por que não `method=external` no fluxo de pagamento

1. **Não existe pagamento.** Todo o fluxo do payment-service (criar cobrança no
   PSP → guardar `psp_payment_id` → esperar webhook → conciliar contra o extrato)
   pressupõe uma transação de gateway. Um `method=external` ali criaria uma linha
   em `payments` sem `psp_payment_id` que a conciliação teria que aprender a
   ignorar — mais um caso especial no lugar mais sensível do sistema.
2. **`POST /payments` é rota de cliente.** Escopada por `user_id`. Aceitar
   `external` ali significaria que o comprador da loja online consegue chamar o
   endpoint que declara o próprio pedido pago. Não há validação de payload que
   conserte isso: a rota inteira está do lado errado da fronteira de confiança.
3. **Tudo que a operação precisa já vive no order-service**: o pedido, o vínculo
   do operador com a loja (`internal/balcao`), a máquina de estados
   (`fulfillment.Advance`), a trilha de auditoria do balcão e a baixa de estoque.

O livro contábil, por outro lado, é **único** e vive no payment-service. Dois
serviços escrevendo o próprio livro é como se perde a garantia de que tudo soma
zero — daí a rota interna.

```
PDV ──POST /balcao/orders/:id/settle-external──> order-service
                                                   │ (transação)
                                                   ├─ autoriza (internal/balcao)
                                                   ├─ pending_payment → paid
                                                   ├─ payment_method = external + NSU
                                                   └─ auditoria FAIL-CLOSED
                                                   │ (pós-commit)
                                                   ├─ POST /internal/v1/ledger/external-settlement ──> payment-service
                                                   └─ catalog: reserva → baixa definitiva
```

---

## Contrato — order-service

### `POST /api/v1/balcao/orders/:id/settle-external`

Autenticação: `Authorization: Bearer <JWT de usuário>`.

**Request**

```json
{
  "nsu": "0044-17",
  "brand": "visa",
  "authorizationCode": "A1B2C3",
  "note": "cliente pediu 2 vias do comprovante"
}
```

| Campo | Obrig. | Regra |
|---|---|---|
| `nsu` | **sim** | 4–32 caracteres alfanuméricos após normalização. Separadores (`-`, espaço, `.`) são removidos e letras viram maiúsculas. É o campo que amarra a venda à linha do extrato do adquirente. |
| `brand` | não | Lista fechada: `visa`, `mastercard`, `elo`, `amex`, `hipercard`, `cabal`, `diners`, `outros`. Desconhecida → 400. |
| `authorizationCode` | não | ≤ 32 caracteres. |
| `note` | não | ≤ 500 caracteres. Vai só para a trilha de auditoria. |

**Não existe campo de valor**, e a ausência é deliberada: o valor liquidado é o
total do pedido calculado pelo servidor. Deixar o operador informar quanto entrou
permitiria liquidar um pedido de R$ 2.000 declarando R$ 20 — e a diferença sairia
pela porta como mercadoria.

**Não existe campo de data.** A liquidação é datada pelo servidor; datar para trás
é como se maquia o fechamento de um dia que já foi conferido.

**Response `200 OK`** — o pedido completo (mesmo shape de `GET /orders/:id`), com:

```json
{
  "status": "paid",
  "paymentMethod": "external",
  "paymentInfo": "Maquininha da loja · NSU 004417",
  "externalNsu": "004417",
  "externalBrand": "visa",
  "externalAuthorizationCode": "A1B2C3",
  "externalSettledBy": "op-a",
  "externalSettledAt": "2026-07-19T14:32:10Z"
}
```

Os campos `external*` são `omitempty`: pedido web e pedido de balcão pago pelo PSP
mantêm exatamente a forma de JSON de antes.

**Erros**

| Status | `code` | Quando |
|---|---|---|
| 400 | `bad_request` | NSU ausente/curto/com caractere inválido; bandeira desconhecida. |
| 401 | `unauthorized` | Sem token. |
| 403 | `forbidden` | Papel sem poder de liquidar (`customer`, `seller`, qualquer outro). Mensagem genérica: não confirma sequer que o pedido existe. |
| 403 | `forbidden` | Operador sem loja vinculada (vínculo revogado). |
| 404 | `not_found` | Pedido inexistente **ou de outra loja** — indistinguíveis de propósito (anti-enumeração). |
| 409 | `conflict` | Pedido web (`somente venda de balcão…`); desconto pendente/recusado; pedido já liquidado com outro NSU; NSU já usado em outro pedido da mesma loja; transição de estado inválida (já pago, cancelado, entregue). |

---

## Autorização

Regras puras em `services/order-service/internal/balcao/external.go`
(`CanSettleExternal`), com testes de regressão em `external_test.go` que rodam
sem infra nenhuma.

| Papel | Pode liquidar? |
|---|---|
| `store_operator` | Sim, **apenas na loja do vínculo** (loja resolvida no auth-service, não no token). |
| `admin` | Sim, em qualquer loja — auditado igual a todo mundo. |
| `customer` | **Não.** Nem no próprio pedido. |
| `seller` | **Não.** Lojista do marketplace não é vendedor de balcão. |
| `service`, papel vazio, papel desconhecido | **Não.** O `default` do switch recusa. |

Duas travas extras:

- **Pedido web nunca é liquidável por fora**, nem por admin. A checagem de canal
  vem *antes* da de papel: se um dia a regra de papel for afrouxada, o pedido web
  continua exigindo dinheiro de verdade passando pelo PSP.
- **Desconto `pending` ou `rejected` bloqueia a liquidação.** Sem isso, a fila de
  aprovação vira decoração — bastaria dar 40% e cobrar antes de o gerente ver.

As mesmas invariantes existem no banco (migration 004), para sobreviver a um bug
no handler ou a um script de manutenção:

```sql
CHECK (payment_method <> 'external' OR channel = 'balcao')
CHECK (external_nsu / external_settled_by / external_settled_at existem juntos ou nenhum)
UNIQUE (store_id, external_nsu) WHERE external_nsu IS NOT NULL
```

---

## Auditoria

`balcao_audit_events`, na **mesma transação** do pedido e **fail-closed**: se a
trilha não grava, a liquidação não acontece. Liquidação externa sem rastro até a
pessoa é o caminho natural de fraude interna — alguém "liquida" um pedido, a
mercadoria sai e não há a quem perguntar.

| Ação | Quando |
|---|---|
| `payment.settled_external` | Liquidação concluída. |
| `payment.ledger_post_failed` | O lançamento contábil não saiu (ver riscos). Fora da transação, falha aberta. |

Cada linha carrega: `actor_id` (quem liquidou), `actor_role`, `store_id`, `amount`,
`ip`, `request_id`, `created_at`, e em `new_value` o NSU, a bandeira, o código de
autorização, quem *vendeu* (`soldBy`, que pode não ser quem liquidou) e a nota.

O `request_id` é propagado ao payment-service (`X-Request-Id`) e gravado no
lançamento: investigar uma liquidação suspeita não exige casar as duas pontas por
horário.

---

## Livro contábil

### Conta nova: `1.1.3` — Caixa em trânsito no adquirente externo

Ativo, natureza devedora (migration `006` do payment-service).

**Não** é a `1.1.1`. A `1.1.1` é conciliada contra o extrato da Appmax; todo
centavo que caísse lá sem existir no extrato viraria divergência permanente.

### O lançamento

`kind = external_sale`, `source_type = external_settlement`, `source_id = orderId`:

```
D 1.1.3 Caixa em trânsito no adquirente externo   bruto
    C 3.1.1 Receita bruta de vendas                   bruto
```

- **Soma zero**, como todo lançamento. Coberto por teste puro e por teste de banco.
- **Sem partida de taxa.** O MDR da maquininha não passa pelo nosso sistema e é
  desconhecido no ato. Entra depois, na conciliação do extrato do adquirente, como
  despesa própria. Inventar aqui uma taxa estimada seria pôr no livro um número
  que ninguém pode conferir.
- **Receita na mesma `3.1.1`.** Faturamento é faturamento, venha de onde vier. O
  que separa as origens é a conta de ativo, o `kind` e o `payment_method` das
  partidas.
- `payment_method = "external"` nas duas partidas → `GET /api/v1/ledger/by-method`
  passa a distinguir maquininha de cartão online. No CSV do contador sai como
  *"Maquininha da loja (externo)"*; o `kind`, como *"Venda externa (maquininha)"*.
- `created_by` = quem liquidou. Lançamento sem pessoa não serve de rastro.

### Conciliação com a Appmax

A liquidação externa **não entra**, por construção: `reconcile.go` só percorre
`payments` com `psp_payment_id` preenchido, e a liquidação externa não cria linha
em `payments` nenhuma. Há teste de banco travando essa propriedade.

---

## Contrato interno — payment-service

### `POST /internal/v1/ledger/external-settlement`

Autenticação: **token de serviço** (`SERVICE_JWT_SECRET`, `pkg/servicetoken`).
Nenhum token de usuário abre esta porta — nem o de um admin. A rota lança receita
sem que dinheiro tenha passado pelo nosso PSP; se aceitasse identidade de pessoa,
um admin comprometido inflaria o faturamento direto no livro.

Fail-closed: sem `SERVICE_JWT_SECRET`, o grupo `/internal` não é registrado.

```json
{
  "orderId": "uuid",
  "amount": 189.90,
  "nsu": "004417",
  "storeId": "loja-centro",
  "operatorId": "op-a",
  "settledBy": "op-a",
  "brand": "visa",
  "authorizationCode": "A1B2C3",
  "occurredAt": "2026-07-19T14:32:10Z"
}
```

| Status | Significado |
|---|---|
| 201 | Lançado agora. Devolve `transactionId`, `period`, `totalCents`. |
| 200 | Já estava lançado (`duplicate: true`). **Sucesso**, não erro. |
| 409 | Período contábil fechado — vira caso de ajuste manual com justificativa. |
| 400 | Payload inválido / lançamento não fecha. |

---

## Idempotência

Três camadas, e as três precisam existir:

1. **Handler** — `CheckSettlementIdempotency`. Mesmo NSU no mesmo pedido → 200
   sem segunda linha de auditoria e sem segundo tracking event. NSU **diferente**
   no mesmo pedido → 409: são dois comprovantes para uma venda só (possível
   cobrança em duplicidade no cartão do cliente), e o NSU original nunca é
   sobrescrito.
2. **Banco do order-service** — `UNIQUE (store_id, external_nsu)`. O mesmo
   comprovante liquidando dois pedidos seria uma venda cobrada uma vez e baixada
   duas.
3. **Livro** — `UNIQUE (kind, source_type, source_id)`. Mesmo que o handler falhe
   em detectar o retry, o segundo lançamento vira `ErrDuplicate` e é no-op.

O retry idempotente **reexecuta o lançamento contábil** de propósito: é o que dá
ao operador um jeito de recuperar uma liquidação cujo lançamento falhou na
primeira tentativa — basta reenviar com o mesmo NSU.

---

## Estoque

A liquidação chama `catalog.Commit(orderID)`: a reserva vira **baixa definitiva**,
igual ao que o consumer de pagamento faz no fluxo pago normal.

Isto é obrigatório porque a liquidação externa não passa por Kafka nenhum — não há
evento de PSP. Sem a baixa, a mercadoria sai da loja e o sweeper de expiração
devolve a reserva ao estoque em 30 minutos: o sistema passaria a acreditar que tem
um item que já foi embora na sacola do cliente.

---

## Configuração

| Serviço | Variável | Efeito se ausente |
|---|---|---|
| order-service | `PAYMENT_SERVICE_URL` | default `http://localhost:8090`. |
| order-service | `SERVICE_JWT_SECRET` | Já obrigatório fora de `DEV_MODE`. |
| payment-service | `SERVICE_JWT_SECRET` | **`/internal` não é registrado**; liquidações não são lançadas no livro (log `WARN` no boot, e cada liquidação grava `payment.ledger_post_failed`). |

Migrations: `order-service/004_external_settlement`, `payment-service/006_external_settlement_account`. Ambas reversíveis — leia o cabeçalho dos `.down.sql` antes de rodar.

---

## Riscos que permanecem

1. **O endpoint continua sendo uma declaração sem prova.** Nenhuma verificação
   criptográfica é possível: o dinheiro entra num sistema que não é nosso. Um
   operador mal-intencionado pode liquidar um pedido sem ter passado o cartão. O
   que a feature garante é que isso **fica registrado com nome, hora, loja, IP e
   NSU** — a detecção é por conciliação com o extrato do adquirente, não por
   prevenção. **A conciliação com o extrato do adquirente externo ainda não é
   automatizada**; hoje é trabalho manual do financeiro sobre a conta `1.1.3`.

2. **Janela entre liquidar e lançar.** O lançamento contábil é HTTP pós-commit. Se
   falhar, temos pedido pago sem lançamento — receita **subestimada**, com rastro
   em `payment.ledger_post_failed` e reprocessável reenviando o mesmo NSU. A ordem
   inversa produziria receita no livro para um pedido não liquidado (dinheiro
   inventado), que é o erro que não se aceita. Não há job automático de
   reprocessamento: hoje é ação manual guiada pela trilha.

3. **NSU não é chave perfeita.** É sequencial *por terminal*. Uma loja com mais de
   um terminal pode ter dois comprovantes legítimos com o mesmo NSU, e o índice
   único por loja recusaria o segundo (409). Não coletamos número de terminal;
   quando coletarmos, a chave certa é `(loja, terminal, NSU, data)`. Falso positivo
   raro e barulhento é o lado certo para errar aqui.

4. **MDR da maquininha fora do DRE.** A taxa do adquirente externo não é lançada
   (é desconhecida no ato). Enquanto não houver importação do extrato do
   adquirente, o custo de aquisição dessas vendas não aparece no resultado.

5. **`store_operator` de uma loja pode liquidar qualquer pedido daquela loja**,
   inclusive vendas de um colega. É deliberado (o caixa é compartilhado, e travar
   por vendedor emperraria a troca de turno), mas significa que o rastro aponta
   quem *liquidou*, não necessariamente quem *vendeu* — os dois campos existem
   separados na trilha (`settledBy` e `soldBy`) justamente por isso.
