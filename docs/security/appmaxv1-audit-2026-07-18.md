# Auditoria de segurança — `internal/psp/appmaxv1` + caminho do webhook

**Data**: 2026-07-18
**Escopo**: `services/payment-service/internal/psp/appmaxv1/` (OAuth2, split, recebedores, saque) e o caminho de webhook/erro que consome esse gateway.
**Contexto**: código implementado recentemente e **nunca auditado**. É o provider de produção (ver `docs/appmax-v1-appstore.md`).
**Status**: 5 achados corrigidos, 1 documentado como risco aceito, todos com teste de regressão.

| ID | Severidade | Achado | Status |
|---|---|---|---|
| AV1-H1 | 🔴 **CRITICAL** | Status do pagamento vinha do corpo do webhook **não assinado** | ✅ corrigido |
| AV1-H2 | 🟠 HIGH | Trava do split contornável por overflow de `int64` | ✅ corrigido |
| AV1-H3 | 🟠 HIGH | Verificação "pedido já aprovado" era **fail-open** (3 caminhos) | ✅ corrigido |
| AV1-H4 | 🟠 HIGH | Credenciais e token OAuth2 vazáveis em log/`fmt`/panic | ✅ corrigido |
| AV1-H5 | 🟠 HIGH | Corpo cru do PSP devolvido ao cliente em erro | ✅ corrigido |
| AV1-M1 | 🟡 MEDIUM | Endpoints de saque sem validação de valor nem autorização documentada | ✅ mitigado |
| AV1-M2 | 🟡 MEDIUM | Comparação de valor no webhook com tolerância de 1 centavo | ✅ corrigido |
| AV1-L1 | 🔵 LOW | 401 do PSP classificado como `ErrInvalidRequest` | ⚠️ risco aceito |

---

## AV1-H1 🔴 — Webhook não assinado ditava o status do pagamento

**Arquivo**: `internal/handler/webhook.go`

### O problema

A Appmax **não assina webhooks** — sem HMAC, sem token. A defesa documentada (audit C3) era re-consultar `GET /v1/orders/{id}` e comparar o **valor**. Mas o status usado para atualizar o pagamento vinha do **corpo**:

```go
newStatus := mapPSPStatus(event.Status)   // event = corpo do webhook
```

`pspResult` (a re-consulta autenticada) era usado **só** para validar o amount.

### Exploração

Um atacante que conheça o endpoint público `POST /webhooks/appmax-v1` monta:

```json
{"event":"order_paid","data":{"id":<id do pedido>,"total":<valor>,"status":"aprovado"}}
```

O valor "certo" é trivial de acertar: **é o preço do produto no catálogo público**. Sem assinatura para falhar e com o amount batendo, o pagamento era promovido a `confirmed` — pedido liberado para expedição sem nenhum dinheiro ter entrado.

O inverso também: um corpo com `order_refused_by_risk` derrubava um pagamento legítimo (negação de serviço no checkout).

### Correção

O status agora vem exclusivamente de `pspResult`, obtido pela re-consulta autenticada. O corpo do webhook virou **apenas um gatilho** ("vá olhar o pedido X"). Divergência entre o alegado e o confirmado é logada como sinal de forja.

```go
newStatus := mapPSPStatus(pspResult.Status)
if claimed := mapPSPStatus(event.Status); claimed != newStatus {
    slog.Warn("webhook: status do corpo diverge do status autoritativo do PSP — corpo ignorado", ...)
}
```

**Regressão**: `internal/handler/webhook_status_test.go`
`TestWebhookForjadoNaoConfirmaPagamentoQueOPSPDizPendente`, `TestWebhookForjadoNaoFalhaPagamentoQueOPSPAprovou`, `TestWebhookConfirmaQuandoOPSPConfirma`.

### Resposta direta a "a re-consulta cobre todos os caminhos?"

**Não cobria.** Cobria o valor e deixava o status passar pelo corpo. Agora cobre os dois. Os demais campos do corpo (produtos, cashback, dados de cartão) são persistidos como payload redigido e **não alimentam nenhuma decisão** — apenas conciliação manual.

---

## AV1-H2 🟠 — Overflow de `int64` contorna a trava do split

**Arquivo**: `internal/psp/appmaxv1/client.go`, `SplitOrder`

A trava existe porque a Appmax **redistribui o split em silêncio** se a soma exceder o `partner_total` — sem erro. O código somava sem checar overflow:

```go
var sum int64
for _, e := range entries { sum += e.Amount }   // cada Amount > 0, validado
if sum > cap { /* bloqueia */ }
```

Duas entries de ~4.6e18 centavos estouram o `int64`: `sum` fica **negativo**, `sum > cap` é falso, e o split passa. Cada `Amount` individual é positivo, então a validação por entrada não pega.

**Correção**: soma com detecção de overflow antes de acumular.

```go
if sum > math.MaxInt64-e.Amount {
    return nil, fmt.Errorf("%w: soma do split estoura int64 ...", psp.ErrInvalidRequest)
}
```

**Regressão**: `TestSplitNaoEhBurlavelPorOverflowDeInt64` + `TestSplitDeValorAltoLegitimoContinuaPassando` (a correção não pode virar falso positivo em split de valor alto legítimo).

### Sobre a margem de 80%

`DefaultSplitSafetyRatio = 0.80` **não é** a trava — é um teto de sanidade sobre um `ReferenceCents` que o *chamador* fornece. Um chamador que passe `ReferenceCents` inflado contorna o limite trivialmente. A trava real é: (a) o chamador ser código nosso, (b) `ReferenceCents` vir do valor autoritativo do order-service. Hoje não há caller em produção; **quando houver, `ReferenceCents` tem que vir do `payments.amount`, nunca de input do usuário.**

---

## AV1-H3 🟠 — Split fail-open em três caminhos

Split é proibido em pedido já aprovado. A checagem era:

```go
if ov, err := c.GetOrder(...); err == nil && ov != nil { /* checa */ }
```

Ou seja: **qualquer falha na consulta pulava a verificação** e o split seguia. Guarda que desaparece quando o sistema está sob stress é guarda que não existe — e é justamente sob stress (ou sob ataque de disponibilidade dirigido a essa chamada) que ela importa.

Três caminhos, o terceiro descoberto ao escrever o teste:

1. erro de rede / 5xx → `err != nil` → checagem pulada;
2. 404 → idem;
3. **200 com corpo ilegível** → `err == nil`, `OrderView` zerado, `NormalizeStatus("")` devolve `pending` → "não consegui ler" ficava **indistinguível de "pedido pendente"**.

**Correção**: fail-closed nos três. Erro na consulta bloqueia; status vazio bloqueia. Ausência de informação nunca é permissão.

**Regressão**: `TestSplitEhFailClosedQuandoNaoConsegueConfirmarOStatus` (subtests: 5xx, 404, resposta ilegível).

---

## AV1-H4 🟠 — Credenciais vazáveis por log, `fmt` ou panic

Resposta direta a "token OAuth em memória — vaza em log? em erro? em panic?":

- **Em log**: vazava. `Client` guarda o access token e o `client_secret`. Um `slog.Error("appmax falhou", "client", c)` — que é a coisa mais natural de escrever — serializaria a struct inteira.
- **Em erro**: vazava em um caminho. O erro do `/oauth2/token` ecoava o corpo cru da resposta. Como o parser é *tolerante* (existe justamente porque a Appmax já mudou o shape), uma mudança de formato colocaria o token dentro da mensagem de erro — que é logada e sobe pelo stack até o handler.
- **Em panic**: vazava. Stack dump com a struct nos argumentos.

### Correção

`internal/psp/appmaxv1/redact.go`: `Config`, `Client` e `Gateway` implementam `slog.LogValuer`, `fmt.Stringer` e `fmt.GoStringer`. A forma **natural** de imprimir passou a ser a mascarada — `%v`, `%+v`, `%#v` e `slog` não têm caminho fácil para o segredo. Asserções de compile-time impedem que alguém remova os métodos sem quebrar o build.

Segredo com ≤12 caracteres vira `***` inteiro (mostrar 4 de 8 é entregar metade da chave). O que continua visível é diagnóstico não-secreto: URL da API, se há token em cache e quando expira — exatamente o que se precisa para debugar um 401.

O erro do endpoint de token não ecoa mais o corpo; reporta status, tamanho e o que verificar.

**Regressão**: `TestCredenciaisNuncaAparecemEmFmtNemEmSlog` (11 formas de impressão, incluindo struct envolvente), `TestMascaraNaoEntregaMetadeDoSegredo`, `TestErroDoEndpointDeTokenNaoEcoaOCorpo`.

### Não corrigido (aceito)

O token vive em memória como `string` — não é zerado após uso e pode aparecer num core dump. Mitigar exigiria `memguard` e complicaria muito para o ganho. **Aceito**: quem lê a memória do processo já tem acesso equivalente.

---

## AV1-H5 🟠 — Corpo do PSP devolvido ao cliente

**Arquivo**: `internal/handler/payment.go`

```go
case errors.Is(pspErr, psp.ErrInvalidRequest):
    BadRequest(c, pspErr.Error())   // ← vazamento
```

`appmaxv1.httpError` embute **até 2000 bytes do corpo cru do PSP** na mensagem, e `ErrInvalidRequest` cobre 400/401/403/409/422. Consequências reais:

- **401** (credencial *nossa* errada ou expirada) devolvia o corpo de auth da Appmax ao comprador — e virava HTTP 400, mandando o cliente "corrigir" um problema de infraestrutura;
- **422** de validação devolvia o payload ecoado pelo PSP, que inclui **CPF, nome, telefone** e às vezes dados de cartão — PII de terceiro num response body que o front loga e o Sentry indexa.

### Correção

`internal/handler/psperror.go`: **allowlist**. Casamos os erros de validação que nós mesmos geramos e devolvemos texto **nosso**; qualquer coisa não reconhecida cai numa mensagem genérica. Allowlist e não denylist — "remova o que parecer sensível" falha no primeiro formato novo que o PSP inventar, e a gente só descobre pelo vazamento.

O erro completo continua no log, correlacionado por `request_id`: suporte tem tudo, cliente não recebe nada que não seja dele. A mensagem genérica também não revela **motivo de recusa** — motivo ajuda o fraudador a calibrar a próxima tentativa.

**Regressão**: `internal/handler/psperror_test.go` — 4 cenários de vazamento (401 com client_id, 422 com CPF/e-mail, 400 com PAN, erro de upstream), mais os testes que garantem que erros acionáveis continuam acionáveis.

---

## AV1-M1 🟡 — Endpoints de saque

Resposta direta a "endpoints de saque/antecipação: quem pode chamar? há autorização?":

**Hoje não há falha explorável, porque não há rota HTTP.** `RequestAnticipation`, `RequestAvailableWithdraw` e `SimulateAnticipation` são chamáveis apenas de dentro do processo; nenhum handler os expõe. Mas nada no código dizia isso, e a próxima pessoa a expor um endpoint não teria como saber o que é obrigatório.

### O que foi feito

1. **Validação de valor antes da rede**: valor `<= 0` era enviado à Appmax sem contrato documentado sobre negativos — "comportamento desconhecido" numa rota que move dinheiro para fora é inaceitável. Também há teto de sanidade de R$ 1.000.000,00 para pegar erro de unidade (reais tratados como centavos).
2. **Log obrigatório** de toda solicitação de saque (hash do recebedor + valor + `request_id`): é a linha que permite reconstruir quem tirou dinheiro e quando.
3. **Aviso normativo no código**, imediatamente acima dos três métodos, com os cinco requisitos para expor isso em HTTP:
   - `role=admin` (`handler.AdminOnly`), nunca o JWT do vendedor direto;
   - **checagem de posse do `recipient_hash`** — o hash é o *único* parâmetro; sem isso qualquer vendedor autenticado saca o saldo de outro trocando a string (IDOR clássico);
   - `Idempotency-Key` — a Appmax v1 não tem idempotência, duplo clique é duplo saque;
   - lançamento no livro (`ledger.SellerWithdrawal`) na mesma operação;
   - registro em `pkg/audit` com ator, IP e valor.

**Regressão**: `TestSaqueRecusaValorInvalidoAntesDaRede` (5 casos × 3 endpoints), `TestSaqueValidoContinuaPassando`.

---

## AV1-M2 🟡 — Tolerância de 1 centavo na validação do webhook

```go
if math.Abs(pspResult.Amount-localAmount) > 0.01 {   // antes
```

A tolerância existia para absorver ruído de `float64`. Mas ela aceita uma diferença **real** de um centavo por transação — em volume, sangria silenciosa que nenhum alerta pega, porque nunca dispara.

**Correção**: converter os dois lados para centavos inteiros (com arredondamento) e exigir **igualdade exata**. Elimina o ruído de float *e* a brecha. Mesmo raciocínio de `internal/psp/appmaxv1/money_test.go`. A mesma regra vale no `internal/ledger` e na reconciliação.

---

## AV1-L1 🔵 — 401 do PSP classificado como `ErrInvalidRequest` (risco aceito)

`httpError` mapeia 401/403 para `psp.ErrInvalidRequest`, junto com 400/422. Semanticamente errado: um 401 é problema de **credencial nossa** e deveria ser `ErrUpstream` → HTTP 502, não 400.

**Não corrigido**: o teste existente `TestPersistent401BecomesInvalidRequest` documenta o comportamento atual deliberadamente, e mudar a classificação alteraria o status HTTP em um caminho de produção sem ganho de segurança — o vazamento que isso causava (AV1-H5) já foi fechado na borda, que é onde importa.

**Impacto residual**: com credencial errada, o cliente recebe 400 em vez de 502. Ruim para diagnóstico, inofensivo para segurança. Registrado para uma futura limpeza.

---

## Verificado e considerado OK

- **Timing attack no webhook**: `VerifyWebhook` usa `subtle.ConstantTimeCompare`. Correto. Comprimentos diferentes retornam 0 sem short-circuit observável.
- **Retry em rota financeira**: `isFinancialRoute` desliga retry de 5xx e de erro de rede em `/v1/payments/*`. Correto, dado que a v1 não tem chave de idempotência. Só 401 e 429 são re-tentados — os dois comprovadamente não processaram o request.
- **PAN no backend**: `Tokenize` existe mas o fluxo de produção exige `CardToken` já tokenizado no browser (SAQ-A). `CreatePayment` recusa cartão sem token. Correto.
- **Thundering herd no OAuth**: `authMu` + double-check no cache. Correto.
- **Redação do payload persistido**: `redactPSPPayload` mascara PAN/CVV/CPF antes de gravar em `webhook_events`. Mantido.
- **URL de ambiente**: `config.Load()` exige `APPMAX_V1_AUTH_URL`/`API_URL` explícitas fora de dev — evita que um deploy aponte para produção achando que é sandbox. Boa decisão, mantida.

---

## Fora de escopo

- **Rotação de credencial**: não há mecanismo. Trocar `APPMAX_V1_CLIENT_SECRET` exige redeploy. Aceitável no volume atual.
- **Rate limit de saída** para a Appmax: só há backoff reativo em 429.
- **Assinatura de webhook**: depende da Appmax. `APPMAX_WEBHOOK_SECRET` (header `X-Appmax-Token`) é defesa em profundidade opcional e **deve ser configurado em produção**, mesmo sendo um segredo compartilhado fraco — eleva o custo de um webhook forjado de "trivial" para "precisa vazar o segredo".
