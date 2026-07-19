# Appmax AppStore API v1 (OAuth2) — provider `appmax-v1`

Integração do payment-service com a **Appmax AppStore API v1**
(`appstore-docs.appmax.com.br`). Convive com o provider `appmax` (API v3 admin,
documentado em [`appmax-integration.md`](./appmax-integration.md)) — os dois são
selecionáveis por `PSP_PROVIDER` e nenhum depende do outro.

- Código: `services/payment-service/internal/psp/appmaxv1/`
  - `client.go` — OAuth2 + transporte + endpoints + split/recebedores + parsers
  - `gateway.go` — implementação de `psp.Gateway` + webhooks + status
- Ativação: `PSP_PROVIDER=appmax-v1`
- Rota de webhook: `POST /webhooks/appmax-v1` (o segmento bate com `Gateway.Name()`)

---

## 1. v1-AppStore x v3-admin

|                | `appmax` (v3 admin)              | `appmax-v1` (AppStore)                        |
| -------------- | -------------------------------- | --------------------------------------------- |
| Autenticação   | `access-token` no header E body  | OAuth2 `client_credentials` → `Bearer`        |
| Base           | `admin.appmax.com.br/api/v3`     | `api.appmax.com.br` + prefixo `/v1`           |
| **Valores**    | **REAIS** (decimal 10,2)         | **CENTAVOS** (inteiros) em toda a API         |
| Envelope       | `{success,text,data,status}`     | `{data}` em sucesso, `{error}`/`{errors}` em falha |
| Customer       | `POST /customer` (campos flat)   | `POST /v1/customers` (`address` aninhado)     |
| Order          | `POST /order`                    | `POST /v1/orders` (`products_value` etc.)     |
| Pagamento      | `POST /payment/{pix,boleto,credit-card}` | `POST /v1/payments/{pix,boleto,credit-card}` |
| Tokenização    | Appmax JS                        | Appmax JS **ou** `POST /v1/payments/tokenize` |
| Parcelamento   | n/d                              | `POST /v1/payments/installments`              |
| Split          | n/d                              | `/v1/orders/{id}/split-order` + `/v1/recipient` |
| Rastreio       | n/d                              | `POST /v1/orders/shipping-tracking-code`      |
| Status pedido  | mesmos                           | mesmos (`pendente`, `aprovado`, …)            |
| Webhook        | sem assinatura                   | sem assinatura (idem)                         |

A conversão reais → centavos usa **arredondamento** (`ToCents`, `math.Round`),
nunca truncamento: `19.99 * 100 = 1998.9999…` viraria 1998 (1 centavo a menos por
pedido) com `int64()` puro.

---

## 2. Configuração

| Env                       | Obrigatório | Notas                                                       |
| ------------------------- | ----------- | ----------------------------------------------------------- |
| `PSP_PROVIDER=appmax-v1`  | sim         | seleciona este provider                                     |
| `APPMAX_V1_CLIENT_ID`     | sim         | fail-closed em `config.Load()`                              |
| `APPMAX_V1_CLIENT_SECRET` | sim         | fail-closed em `config.Load()`                              |
| `APPMAX_V1_AUTH_URL`      | em prod     | sandbox: `https://auth.sandboxappmax.com.br`                |
| `APPMAX_V1_API_URL`       | em prod     | sandbox: `https://api.sandboxappmax.com.br`                 |
| `APPMAX_V1_EXTERNAL_ID`   | não         | enviado como `external_id` no pedido (ver §7, ambiguidade)  |
| `APPMAX_WEBHOOK_SECRET`   | não         | se setado, exige header `X-Appmax-Token` igual              |

Produção (default do código): `https://auth.appmax.com.br` / `https://api.appmax.com.br`.
Em `DEV_MODE=false` as duas URLs são **obrigatórias** — um deploy que esquece de
apontar para o sandbox cobraria de verdade.

---

## 3. Autenticação

```
POST {auth}/oauth2/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials&client_id=…&client_secret=…
→ {"access_token":"…","token_type":"Bearer","expires_in":3600}
```

**Não existe refresh token** — renovar é pedir outro `client_credentials`.

O client mantém o token em memória (`sync.RWMutex`) e o expira `tokenSkew = 5min`
antes do vencimento real: com `expires_in=3600`, renova aos **55 minutos**. Um
`sync.Mutex` separado serializa o fetch para evitar thundering herd quando várias
goroutines encontram o cache vencido. Em **401** o cache é invalidado e a
requisição é repetida uma vez com token novo.

---

## 4. Fluxo de pagamento

`CreatePayment` orquestra: `POST /v1/customers` → `POST /v1/orders` →
`POST /v1/payments/{método}`. O `PSPID` persistido é o **id do pedido Appmax** —
é ele que `GetPayment` e o webhook usam para reconciliar.

`ClientData` devolvido ao SPA (contrato estável):

```json
{
  "provider": "appmax-v1",
  "pix_qrcode": "iVBORw0KGgo…",
  "pix_emv": "00020126…",
  "pix_expires_at": "2026-07-19 10:00:00",
  "boleto_url": "https://…/b.pdf",
  "boleto_line": "34191…",
  "installments": 1
}
```

### ⚠️ `pix_qrcode` tem dois tipos diferentes

| Origem                          | Tipo                                     |
| ------------------------------- | ---------------------------------------- |
| `POST /v1/payments/pix` (API)   | **PNG em base64**, sem o prefixo `data:` |
| Webhook `payment_info.pix`      | **URL** da imagem                        |

Mesmo nome de campo, tipos distintos. O código trata separado
(`PaymentResult.PixQRCodeB64` x `Event.PixQRCodeURL`) e há teste travando isso
(`TestPixQRCodeBase64VsURL`). O front deve montar
`data:image/png;base64,<pix_qrcode>` para o valor vindo do `ClientData`.

### Parcelamento

`POST /v1/payments/installments` devolve o **total** da compra por número de
parcelas (`{"1":{"total":20330},"3":{"total":21147}}`), em centavos. Quem divide
para achar o valor da parcela é a integração — `InstallmentAmount()` faz isso
colocando o resto de centavos na primeira parcela (decisão nossa; a Appmax não
documenta o arredondamento). Modos de juros: **PP** (parcela paga pelo comprador)
e **AM** (absorvida pelo lojista), passados em `settings`.

O sistema **não calcula juros** no `POST /v1/orders`: mandamos
`products_value`/`discount_value`/`shipping_value` já finais.

---

## 5. Payment Split

- `POST /v1/orders/{orderId}/split-order` com `{"split":[{"amount":1000,"recipient_hash":"…"}]}`.
- **Valor fixo em centavos por recebedor — não existe split percentual.**
- Só pode ser criado **depois** de criar o pedido e **antes** da aprovação; é
  proibido em pedido `aprovado` (o client checa via `GET /v1/orders/{id}`).
- Incide sobre o valor **líquido** (`partner_total` = pedido − taxas Appmax), e
  **não há endpoint público** para ler o `partner_total` antes.
- **Se a soma exceder o `partner_total`, a Appmax redistribui proporcionalmente e
  não retorna erro.** O recebedor receberia um valor diferente do combinado, em
  silêncio. Por isso `SplitOrder` aplica uma trava local: soma ≤
  `ReferenceCents × SafetyRatio` (default `0.80`), com `slog.Error` + erro
  `psp.ErrInvalidRequest` quando estoura. Ajuste o ratio conforme a taxa real da
  conta.
- Pedido com split só aceita **estorno TOTAL**.
- `POST /v1/orders/shipping-tracking-code` é **obrigatório** para liberar o saque
  do split — sem ele o saldo do recebedor não é liberado.

### Recebedores (`/v1/recipient`)

- **Este endpoint é o único da v1 em camelCase** (`storeUrl`, `dateOfBirth`,
  `companyDocumentNumber`, …) enquanto todo o resto é snake_case. Os structs
  `Recipient*` refletem isso.
- `POST /v1/recipient/{hash}/facematch-link` — o SMS **não dispara em sandbox**.
- `GET /v1/recipient/{hash}/status` tem só **3 valores**:
  `Awaiting face match completion`, `Onboarding on verification`,
  `Onboarding completed`. Só o último pode receber e sacar. **Não existe status de
  rejeição** — um onboarding reprovado simplesmente nunca avança.
- O recebedor é **imutável**: não há endpoint de update/delete. Um cadastro com
  dado errado é definitivo; crie outro.
- `GET /…/balances` (tipos `available` e `to_release`) não tem JSON documentado —
  o parser aceita tanto `{"available":…,"to_release":…}` quanto
  `[{"type":"available","value":…}]`.
- Saques: `…/withdraw-request/anticipation/simulate?value=10000` (centavos na
  query), `…/withdraw-request/anticipation` e `…/withdraw-request/available`
  (`{value}` em centavos). Status `2 = pending`.

---

## 6. Webhooks

**A Appmax não envia assinatura HMAC nem token.** `VerifyWebhook` é fail-closed
*opcional*: com `APPMAX_WEBHOOK_SECRET` setado exigimos `X-Appmax-Token` igual
(comparação em tempo constante); sem ele, aceitamos.

**A integridade real vem da re-consulta `GET /v1/orders/{id}`** feita pelo handler
(mesmo padrão do provider v3, audit C3): o status e o valor autoritativos vêm da
API, nunca do corpo do webhook. Nunca aprove um pagamento só com base no payload
recebido.

Entrega: timeout de **5 segundos**; retries em 0, +30min, +2h, +4h e depois o
evento é **descartado**. Responder 200 rápido é obrigatório.

Envelope:

```json
{
  "event": "order_paid_by_pix",
  "event_type": "order",          // order | customer | payment | subscription
  "site_id": 1, "app_id": "…", "client_key": "…", "external_key": "…",
  "data": { … },
  "partner_merchant": { … }
}
```

`data` (pedido): `order_id, status, total, freight_value, merchant_total,
discount, interest, paid_at, products[].{sku,name,price,quantity}, payment_info,
cashback_used, cashback_reserved, cashback_status`. Valores em centavos.

`payment_info` é um objeto de **uma** chave:
`credit_card.{installments,card_brand,nsu,authorization_code,captured_at}` |
`pix.{end_to_end_id,pix_expiration_date,pix_emv,pix_qrcode,pix_payment_link}` |
`boleto.{boleto_overdue_date,boleto_url,boleto_digitable_line}`.

Mapeamento evento → status normalizado:

| Evento                                                                    | Status       |
| ------------------------------------------------------------------------- | ------------ |
| `order_approved`, `order_paid`, `order_paid_by_pix`, `order_integrated`, `order_charge_back_gain` | `approved`   |
| `order_authorized`, `payment_authorized_with_delay`                       | `authorized` |
| `order_refund`, `order_partial_refund`                                    | `cancelled`  |
| `order_pix_expired`, `order_billet_overdue`                               | `expired`    |
| `order_refused_by_risk`, `payment_not_authorized`                         | `rejected`   |
| `order_pix_created`, `order_billet_created`, `order_chargeback_in_treatment`, `split_orders` | cai no status do pedido |

Eventos sem `order_id` (ex.: `customer_*`) retornam `(nil, nil)` — o handler
responde 200 e ignora.

### Cashback

Cashback **só existe como campos read-only no webhook** (`cashback_used`,
`cashback_reserved`, `cashback_status`). Não há endpoint de cashback na API v1 —
nada foi inventado. Os campos são expostos em `Event` (via `ParseEvent`) e
preservados no `RawBody` do `psp.WebhookEvent` para reconciliação.

---

## 7. Riscos e lacunas conhecidas

1. **Sem idempotência.** A API v1 não aceita chave de idempotência. Um retry cego
   de `POST /v1/payments/*` pode gerar cobrança duplicada. Mitigação no client:
   rotas financeiras **não** são re-tentadas em 5xx nem em erro de rede — só em
   401 e 429, que comprovadamente não processaram a requisição
   (`isFinancialRoute`). Erro de rede em rota financeira sobe imediatamente e
   deve ser resolvido por consulta (`GET /v1/orders/{id}`), nunca por retry.
2. **Webhook sem assinatura.** Qualquer um que descubra a URL pode postar. Só a
   re-consulta autoritativa protege. Configure `APPMAX_WEBHOOK_SECRET` como
   camada extra.
3. **Split silencioso.** Excesso é redistribuído sem erro — a trava é 100% nossa.
   Não existe endpoint para ler o `partner_total` antes de decidir os valores.
4. **Recebedor imutável** e sem status de rejeição: um onboarding travado é
   indistinguível de um em análise.
5. **JSON não publicado verbatim** em vários pontos (boleto, balances, tokenize),
   daí os parsers tolerantes com aliases. Validar contra o sandbox real antes de
   produção.
6. **Ambiguidades resolvidas por decisão nossa** (revisar quando a doc evoluir):
   - `products[].type` é obrigatório mas o enum não é publicado → usamos
     `"physical"` (constantes `ProductTypePhysical`/`ProductTypeDigital`).
   - `APPMAX_V1_EXTERNAL_ID` não tem lugar documentado no request → enviado como
     `external_id` no `POST /v1/orders` (omitido quando vazio). O campo
     `external_key` do webhook parece ser a contraparte.
   - `POST /v1/customers` exige `ip`; o handler ainda não propaga o IP do
     comprador, então enviamos `0.0.0.0`. **TODO**: propagar o IP real (melhora o
     antifraude).
   - Endereço do customer e parcelamento no cartão ainda não são preenchidos —
     `psp.CreateRequest` não carrega esses dados. Cartão vai sempre `1x`.
   - `installments` no `ClientData` é `1` quando a resposta não informa.

---

## 8. Rate limits

Por `client_id`: burst 50, sustentado 5 req/s, 100.000 req/mês. Por rota: 60/min
(5/min em rotas sensíveis). O 429 traz `X-RateLimit-Limit`,
`X-RateLimit-Remaining`, `Retry-After` e `retryAfter` no corpo.

O client faz backoff exponencial (`BackoffBase × 2^tentativa`, teto 30s)
**respeitando `Retry-After`** quando presente (header em segundos ou data HTTP;
fallback no `retryAfter` do corpo), e loga cada 429 com os headers de quota.

---

## 9. Testes

`services/payment-service/internal/psp/appmaxv1/{client,gateway}_test.go`, todos
com `httptest.Server` (nenhuma chamada de rede real):

- fluxo Pix completo (customer → order → payment), com asserção de que os valores
  saem em centavos e o `ClientData` traz base64 cru;
- boleto nos **dois** formatos de resposta (`boleto_url`/`pdf`, etc.);
- cartão tokenizado (payload e status aprovado) + rejeição sem token;
- cache de token, renovação proativa (janela de ~55min) e fetch único sob
  concorrência;
- 401 → re-auth e repetição com token novo; 401 persistente → `ErrInvalidRequest`;
- 429 com `Retry-After` (header e corpo) e backoff exponencial sem ele;
- rota financeira não re-tentada em 5xx;
- split bloqueado quando a soma excede o teto, aceito dentro dele, recusado em
  pedido aprovado, e validações de entrada;
- recebedores (payload camelCase), status, balances nos dois shapes, saques;
- os dois formatos de `pix_qrcode` (base64 na API x URL no webhook);
- cada evento de webhook mapeado + `NormalizeStatus` para todos os status
  documentados.

```bash
cd services/payment-service && go test ./internal/psp/appmaxv1/...
```
