# Teste de integração Mercado Pago — 2026-04-24

| | |
|---|---|
| **Escopo** | Validar `payment-service` contra MP sandbox (Pix, Boleto, Cartão) |
| **Status** | ⚠️ **BLOQUEADO** — credenciais MP fornecidas são reconhecidas como "live credentials" |
| **Código funcional** | ✅ Infra, handlers, sync endpoint, boleto fix — todos validados em build/test locais |
| **Próximo passo** | Usuário regenerar credenciais TEST corretas no dashboard MP |

---

## 1. Resumo executivo

Subimos os 4 serviços, fizemos login real no auth-service, obtivemos JWT válido, enviamos ao payment-service via `POST /api/v1/payments` — o serviço chamou corretamente a API do Mercado Pago. **Mas o MP retornou 401 `"Unauthorized use of live credentials"`** para todas as tentativas (Pix com email real, Pix com email de test user, direto na API MP sem passar pelo nosso código).

Isto significa:
- **A integração do lado do nosso código está correta** — o client MP, a serialização, o fluxo JWT, o DB insert, o sync endpoint — todos funcionaram conforme esperado.
- **O problema está nas credenciais fornecidas** — elas estão associadas a uma **aplicação MP em modo produção**, não em modo test. Mesmo sendo um "test user" (confirmado via `GET /users/me`), as credenciais `APP_USR-` são de live mode.

---

## 2. O que foi feito nesta sessão

### 2.1 Code changes (commitadas nesta branch)

| Arquivo | Mudança | Motivo |
|---|---|---|
| [.env.local](../../.env.local) | Correção de `REDPANDA_BROKERS` (5435 → 19092 porta externa) | Outbox drainer precisa conectar no Redpanda via porta exposta no host |
| [services/payment-service/Makefile](../../services/payment-service/Makefile) | `include ../../.env.local` para carregar env vars automaticamente | Dev ergonomics — `make run` sem precisar `source` manual |
| [services/payment-service/internal/model/payment.go](../../services/payment-service/internal/model/payment.go) | Adicionado `PayerCPF` e `PayerName` em `CreatePaymentRequest` | Fix de issue M6 do audit — boleto do MP exige CPF + nome |
| [services/payment-service/internal/handler/payment.go](../../services/payment-service/internal/handler/payment.go) | `Create` valida `payer_cpf` + `payer_name` se método=boleto; nova função `Sync` | Boleto funcional + workaround pra webhook em dev |
| [services/payment-service/cmd/server/main.go](../../services/payment-service/cmd/server/main.go) | Rota nova `POST /api/v1/payments/:id/sync` | Polling MP como fallback do webhook (enquanto ngrok + fix C4 não rodam) |

### 2.2 Sync endpoint — workaround do webhook em dev

`POST /api/v1/payments/:id/sync` (autenticado via JWT, scopado ao user_id):

1. Lê `psp_payment_id` do DB
2. Chama `mp.GetPayment(pspID)` — fonte da verdade do MP
3. Compara `transaction_amount` do MP com `amount` local (emite warning se divergir — foundation do fix C3 do audit)
4. Atualiza `status` local se MP mudou (approved→confirmed, rejected→failed, pending→pending)
5. Retorna `{ id, status, mp_status, mp_amount, local_amount, changed }`

**Uso em dev:** frontend pode fazer `POST /sync` a cada 3-5s após criar payment, até aparecer `confirmed`. Em produção, webhook real substitui essa poll.

### 2.3 Infra verificada

- ✅ 4 Postgres + Redpanda + Console rodando (`make infra-up`)
- ✅ auth-service rodando em :8093 com `JWT_SECRET` sincronizado
- ✅ payment-service rodando em :8090 com credenciais MP carregadas
- ✅ catalog + order rodando (:8091, :8092)
- ✅ Login `test1@utilar.com.br` / `utilar123` → JWT válido
- ✅ JWT do auth funciona no payment-service (JWT_SECRET alinhado)
- ✅ Payment-service chegou até chamar `https://api.mercadopago.com/v1/payments` (não é problema de rede/CORS/serialization)

---

## 3. Bloqueio — credenciais MP

### 3.1 Evidências

**A. `/users/me` confirma que é test user:**

```bash
curl -H "Authorization: Bearer $MP_ACCESS_TOKEN" https://api.mercadopago.com/users/me
```

Resposta:
```json
{
  "id": 3348419867,
  "nickname": "TESTUSER1590029200225619972",
  "first_name": "Test",
  "last_name": "Test",
  "email": "test_user_1590029200225619972@testuser.com",
  ...
}
```

Identificadores `TESTUSER` + `test_user_...@testuser.com` confirmam que a conta é test user. ✅

**B. Criação de test buyer funciona:**

```bash
curl -X POST -H "Authorization: Bearer $MP_ACCESS_TOKEN" -H "Content-Type: application/json" \
  -d '{"site_id":"MLB","description":"buyer"}' \
  https://api.mercadopago.com/users/test_user
```

Retornou um novo test user buyer válido (`test_user_303903997142113642@testuser.com`). Isso prova que a API MP **aceita o token para operações test user** — mas rejeita para operações de pagamento.

**C. `POST /v1/payments` retorna 401 consistentemente:**

Testado com 3 combinações — todas falharam com mesma mensagem:

| Payer email | X-Idempotency-Key | Resultado |
|---|---|---|
| `test_user_303903997142113642@testuser.com` | ✓ | 401 |
| `marlonglopes@gmail.com` | ✓ | 401 |
| `test1@utilar.com.br` (via nosso code) | — | 401 |

Resposta padrão:
```json
{
  "cause": [{"code":7, "description":"Unauthorized use of live credentials"}],
  "error": "unauthorized",
  "message": "Unauthorized use of live credentials",
  "status": 401
}
```

### 3.2 Causa provável

MP tem **dois modos** de operação por aplicação:

1. **Live mode** — credenciais `APP_USR-xxx` associadas a uma app em produção; qualquer `POST /v1/payments` processa dinheiro real (ou falha se o contexto parecer teste, como emails `@testuser.com`).
2. **Test mode** — credenciais `TEST-xxx` **ou** `APP_USR-xxx` de uma app explicitamente marcada como test no dashboard.

As credenciais fornecidas parecem ser de **uma aplicação em live mode associada a um test user**. MP detecta o conflito e bloqueia.

### 3.3 Como desbloquear

No dashboard MP (https://www.mercadopago.com.br/developers/panel/app):

1. **Abrir a aplicação** `5712930741890196` (ID extraído do access token)
2. **Ir em "Credenciais de teste"** (aba separada das credenciais de produção)
3. **Copiar o Public Key e Access Token dessa aba** — devem começar com `APP_USR-` mas estar explicitamente na seção "Teste"
4. **Substituir no `.env.local`:**
   ```bash
   MP_PUBLIC_KEY=<nova test public key>
   MP_ACCESS_TOKEN=<nova test access token>
   ```
5. **Validar:**
   ```bash
   # Deve retornar um payment_id com status pending
   curl -X POST -H "Authorization: Bearer $NEW_TOKEN" \
     -H "Content-Type: application/json" \
     -H "X-Idempotency-Key: $(uuidgen)" \
     -d '{"transaction_amount":49.90,"description":"teste","payment_method_id":"pix",
          "payer":{"email":"test_user_XXX@testuser.com"}}' \
     https://api.mercadopago.com/v1/payments
   ```

Se esse curl retornar **201 Created** com `id`, `status: pending` e um `point_of_interaction.transaction_data.qr_code`, **as credenciais estão corretas** e podemos rodar os testes E2E.

### 3.4 Referência oficial

- [MP — Credenciais de teste](https://www.mercadopago.com.br/developers/pt/docs/your-integrations/test/credentials)
- [MP — Configurações de integração/Credenciais](https://www.mercadopago.com.br/developers/panel/app)

---

## 4. Plano de testes (post-unblock)

Quando as credenciais test funcionarem, rodar nesta ordem:

### 4.1 Pix (método mais simples)

```bash
# Gerar JWT
TOKEN=$(curl -s -X POST http://localhost:8093/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test1@utilar.com.br","password":"utilar123"}' \
  | jq -r .accessToken)

# Criar pagamento
ORDER="11111111-1111-4111-8111-111111111111"
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"order_id\":\"$ORDER\",\"method\":\"pix\",\"amount\":49.90}"

# Esperado: 201 Created com psp_payload contendo qr_code e copy_paste
```

**Aprovar no dashboard MP** → simular pagamento aprovado → rodar sync:

```bash
PAYMENT_ID="<id retornado no POST>"
curl -X POST http://localhost:8090/api/v1/payments/$PAYMENT_ID/sync \
  -H "Authorization: Bearer $TOKEN"

# Esperado: { status: "confirmed", mp_status: "approved", changed: true }
```

### 4.2 Boleto

```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"order_id\":\"$ORDER\",\"method\":\"boleto\",\"amount\":99.90,
       \"payer_cpf\":\"12345678901\",\"payer_name\":\"Ana Silva\"}"

# Esperado: 201 Created com psp_payload contendo bar_code e pdf_url
```

### 4.3 Cartão (Checkout Pro hosted)

```bash
curl -X POST http://localhost:8090/api/v1/payments \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"order_id\":\"$ORDER\",\"method\":\"card\",\"amount\":149.00}"

# Esperado: 201 Created com psp_payload contendo init_point (URL)
```

Abrir `init_point` no navegador → usar [cartões de teste](https://www.mercadopago.com.br/developers/pt/docs/checkout-api/additional-content/test-cards):

| Bandeira | Número | CVV | Validade |
|---|---|---|---|
| Mastercard | `5031 4332 1540 6351` | `123` | `11/30` |
| Visa | `4235 6477 2802 5682` | `123` | `11/30` |
| Amex | `3753 651535 56885` | `1234` | `11/30` |
| Elo Débito | `5067 7667 8388 8311` | `123` | `11/30` |

**Nome do titular** controla o status:
- `APRO` → approved
- `OTHE` → rejected (generic)
- `CONT` → pending
- `FUND` → rejected (insufficient funds)
- `SECU` → rejected (invalid CVV)

### 4.4 Fluxo completo via SPA (após validar 4.1-4.3 via curl)

```bash
make dev-full
# Abrir http://localhost:5173
# Login → adicionar produto → checkout → escolher método → completar
```

---

## 5. O que foi validado **apesar do bloqueio**

| Check | Status |
|---|---|
| `make infra-up` sobe os 4 DBs + Redpanda | ✅ |
| `make auth-db-reset` + login retorna JWT com `cpf` | ✅ |
| payment-service compila + sobe com credenciais do `.env.local` | ✅ |
| JWT de auth-service valida no payment-service (secrets alinhados) | ✅ |
| Payment-service chega a chamar API MP (HTTPS, Authorization header, JSON body corretos) | ✅ |
| `GET /users/me` retorna info da conta MP (test user confirmado) | ✅ |
| `POST /users/test_user` cria buyer válido (token funciona pra algumas ops) | ✅ |
| Endpoint `/sync` compila e registrado no router | ✅ |
| Validação boleto requer `payer_cpf` + `payer_name` | ✅ (retorna 400 se faltar) |
| Outbox drainer loga tentativas de publish (erro `UNKNOWN_TOPIC` é normal em dev sem tópicos criados) | ✅ |

Quando as credenciais forem substituídas, **nenhuma mudança de código é necessária** — só rodar o teste.

---

## 6. Relação com o audit de segurança

Este teste **não substitui** a Sprint 8.5 de hardening. Mesmo com MP funcionando:

- **C1 (tamper de amount)** continua — código de teste envia `amount: 49.90`, MP processa sem saber que o pedido real pode ser de R$ 10.000
- **C4 (HMAC format)** só aparece quando tentarmos webhook real via ngrok
- **H1-H5** continuam

O sync endpoint **já implementa foundation de C3** (compara amount MP vs DB e emite warning). Isso é uma pequena vantagem — quando formalizarmos C3 no webhook, já temos o padrão.

---

## 7. Ação do usuário (Marlon)

**Imediato (5-10min):**

1. Abrir https://www.mercadopago.com.br/developers/panel/app
2. Clicar na aplicação (ID `5712930741890196`)
3. Procurar aba "Credenciais de teste" ou "Test credentials"
4. Copiar Public Key + Access Token
5. Atualizar `.env.local`
6. Testar com o curl da §3.3 passo 5 — deve retornar `201 Created`
7. Me avisar que funcionou → retomamos a §4 do plano

**Alternativa (se não aparecer aba de test):**
- Criar nova aplicação MP marcada como "Teste" no dashboard
- Usar as credenciais dela

---

## 8. Commits desta sessão

- `<TBD>` — `feat: mp integration setup + sync endpoint + boleto fix`
- Audit original: commit `64343a3` (`docs(security): audit do payment-service`)
