# 10 — Estratégia de testes

**Status**: Rascunho. **Data**: 2026-04-20.

Pirâmide de testes concreta, escolha de ferramentas, caminhos dourados, metas de carga e gates de CI para a Utilar Ferragem.

Leituras complementares:
- [07-data-model.md](07-data-model.md) — quais schemas os testes de contrato fixam
- [08-security.md](08-security.md) §12 — varreduras de segurança incluídas no CI
- [09-observability.md](09-observability.md) — quais sinais de produção substituem/complementam os testes pós-deploy

---

## 1. Pirâmide de testes

| Camada | Ferramenta | Onde | Meta de cobertura | Executa em |
|-------|------|-------|-----------------|---------|
| Unitário | Vitest 2 / happy-dom / Testing Library | `app/src/test/` | ≥70% de linhas nas libs principais (`format`, `cpf`, `cep`, `taxonomy`, `filters`) | PR + main |
| Unitário | Go stdlib `testing` + `net/http/httptest` | `services/payment-service/internal/**/*_test.go` | ≥80% em handlers + cliente MP | PR + main |
| Integração | Go `testing` com DB real (Docker) | `services/payment-service/internal/**/*_test.go` (tag `integration`) | Todos os endpoints públicos, casos 2xx + 4xx | PR + main |
| Contrato | Snapshot + validação de schema Go | `services/payment-service/internal/` | Payloads Kafka + contratos da API MP | PR + main |
| E2E | Playwright | `utilar-ferragem/e2e/` | Caminho dourado do checkout + cadastro + navegação | Nightly + pré-deploy |
| Carga | k6 | `infrastructure/k6/` | 100 RPS de navegação, 20 checkouts simultâneos | Semanal + pré-lançamento |
| Acessibilidade | axe-core + Playwright | `utilar-ferragem/e2e/a11y/` | Todas as páginas de nível superior | PR + nightly |
| Lighthouse | Lighthouse CI | `utilar-ferragem/` | Perf ≥85, A11y ≥90, Boas Práticas ≥90, SEO ≥90 | PR |
| Segurança | gitleaks, bundler-audit, npm audit, ZAP | CI | Zero findings de alta severidade | PR + semanal |

Proporção alvo (aproximada): 70% unitário, 20% integração, 7% contrato, 3% E2E.

---

## 2. Testes de frontend (Vitest)

### 2.1 Testes de componentes

Localização: co-localizados sob `__tests__/` junto ao componente.

Padrões (do `gifthy-hub/` pai):

- Recharts deve ser mockado (conforme "Padrões conhecidos de testes" no CLAUDE.md pai).
- `useNavigate` mockado via `vi.mock('react-router-dom', async (importOriginal) => ...)`.
- Inputs `type="number"` precisam do workaround com `Object.defineProperty`.
- Formulários multi-etapas usam `MemoryRouter` com `initialEntries`.

Arquivos de teste obrigatórios na Fase 3:

| Arquivo | O quê |
|------|------|
| `src/lib/__tests__/cpf.test.ts` | Casos válidos/inválidos de CPF Módulo 11, bordas (inválidos conhecidos como `11111111111`), máscara/desmáscara |
| `src/lib/__tests__/cep.test.ts` | `lookupCep` mockado com MSW; CEP malformado; 404 |
| `src/lib/__tests__/format.test.ts` | BRL/USD, datas em pt-BR/en, mascaramento de CNPJ/CPF/CEP/telefone |
| `src/lib/__tests__/taxonomy.test.ts` | `pathFor(category)`, busca de folha, fallback para categoria inexistente |
| `src/lib/__tests__/filters.test.ts` | Aplicação de filtro de comércio contra JSON de `specs` de amostra |
| `src/store/__tests__/cartStore.test.ts` | Adicionar/remover/atualizar qtd, persistência, totalização em BRL |
| `src/components/product/__tests__/SpecSheet.test.tsx` | Renderiza schemas completos de ferramentas/elétrica/hidráulica/pintura |
| `src/components/checkout/__tests__/PaymentMethodPicker.test.tsx` | Seleção de Pix / boleto / cartão, estados desabilitados |
| `src/pages/checkout/__tests__/CheckoutPage.test.tsx` | Caminho feliz com API mockada, erros de validação |

### 2.2 Testes de hooks

Use `renderHook` do `@testing-library/react`. Todos os hooks TanStack Query encapsulados em um `QueryClientProvider` de teste com `retry: false` e `gcTime: 0`.

### 2.3 Mocking de API — MSW

`utilar-ferragem/src/test/msw/handlers.ts` exporta handlers para:

- `POST /auth/login` → retorna JWT stub + usuário
- `POST /auth/register` → 201 ou 422 (e-mail já cadastrado)
- `GET /api/v1/marketplace/products` → página paginada de produtos falsos com `specs`
- `POST /api/v1/payments` → retorna QR do Pix ou URL do boleto
- `POST /api/v1/orders` → 201 com `status='pending_payment'`

Configuração: `src/test/setup.ts` inicia o servidor MSW. Testes individuais podem sobrescrever handlers via `server.use(...)`.

### 2.4 Comandos

```bash
# a partir de utilar-ferragem/
npm test                # modo watch
npm run test:run        # execução única (CI)
npm run test:coverage   # imprime resumo de cobertura + grava lcov
npm run test:ui         # Vitest UI
```

---

## 3. Testes de backend (RSpec)

### 3.1 Specs de modelo

Obrigatórios para todo novo model + validator:

| Spec | Cobre |
|------|--------|
| `spec/models/user_spec.rb` | Unicidade de CPF, delegação de validação, escopo de role |
| `spec/validators/cpf_validator_spec.rb` | Válidos conhecidos, inválidos conhecidos, malformados, entrada mascarada |
| `spec/models/payment_spec.rb` | Transições da máquina de estados, `amount_cents > 0`, unicidade parcial de psp_payment_id |
| `spec/models/order_spec.rb` | Novos campos de endereço BR, padrão `shipping_country='BR'` |
| `spec/models/shipping_rate_spec.rb` | Correspondência de prefixo de CEP, cálculo de peso |

### 3.2 Specs de request

Uma spec de request por endpoint, cobrindo 2xx + cada ramo 4xx.

Exemplos (payment-service, Sprint 08):

```ruby
# spec/requests/api/v1/payments_spec.rb
describe "POST /api/v1/payments" do
  it "creates a Pix payment for a valid pending_payment order" do
    post "/api/v1/payments", params: { order_id: order.id, method: "pix" }, headers: auth(user)
    expect(response).to have_http_status(:created)
    expect(json["pix_qr_code"]).to be_present
    expect(json["pix_copy_paste"]).to be_present
  end

  it "rejects when order is already paid" do
    order.update!(status: "paid")
    post "/api/v1/payments", params: { order_id: order.id, method: "pix" }, headers: auth(user)
    expect(response).to have_http_status(:unprocessable_entity)
  end

  it "rejects when order belongs to another user" do
    other_user_order = create(:order, user: create(:user))
    post "/api/v1/payments", params: { order_id: other_user_order.id, method: "pix" }, headers: auth(user)
    expect(response).to have_http_status(:forbidden)
  end
end
```

### 3.3 Specs de webhook

Fixtures em `services/payment-service/spec/fixtures/webhooks/mercadopago/*.json` — payloads assinados de formato real gravados do sandbox do MP.

```ruby
describe "POST /webhooks/psp/mercadopago" do
  let(:body) { File.read("spec/fixtures/webhooks/mercadopago/payment_confirmed.json") }
  let(:ts)   { Time.now.to_i }
  let(:sig)  { compute_mp_signature(body, ts, ENV.fetch("PSP_WEBHOOK_SECRET")) }

  it "accepts a valid signed payload" do
    post "/webhooks/psp/mercadopago", params: body,
         headers: { "x-signature" => "ts=#{ts},v1=#{sig}", "content-type" => "application/json" }
    expect(response).to have_http_status(:ok)
  end

  it "rejects a payload with a tampered amount" do
    tampered = body.gsub('"amount":"199.90"', '"amount":"0.01"')
    post "/webhooks/psp/mercadopago", params: tampered,
         headers: { "x-signature" => "ts=#{ts},v1=#{sig}", "content-type" => "application/json" }
    expect(response).to have_http_status(:unauthorized)
  end

  it "rejects replays older than 5 minutes" do
    old_ts = (Time.now - 400).to_i
    old_sig = compute_mp_signature(body, old_ts, ENV.fetch("PSP_WEBHOOK_SECRET"))
    post "/webhooks/psp/mercadopago", params: body,
         headers: { "x-signature" => "ts=#{old_ts},v1=#{old_sig}" }
    expect(response).to have_http_status(:unauthorized)
  end

  it "is idempotent on duplicate event_id" do
    2.times do
      post "/webhooks/psp/mercadopago", params: body,
           headers: { "x-signature" => "ts=#{ts},v1=#{sig}" }
    end
    expect(Payment.where(psp_payment_id: "MP-123").count).to eq(1)
  end
end
```

### 3.4 Specs de Kafka

Use [`karafka-testing`](https://karafka.io/docs/Testing/) ou faça stub do produtor e do consumer com um broker em memória local.

- Teste de produtor: verificar o formato do payload `payment.confirmed` contra JSON Schema a cada transição.
- Teste de consumer: enviar uma mensagem `payment.confirmed` ao consumer do order-service → verificar que o status do pedido transiciona para `paid`.

### 3.5 Idempotência do seed

`services/*/spec/tasks/seed_spec.rb` — para cada serviço:

```ruby
describe "db:seed" do
  it "is idempotent when run twice" do
    Rake::Task["db:seed"].invoke
    count_before = User.count
    Rake::Task["db:seed"].reenable
    Rake::Task["db:seed"].invoke
    expect(User.count).to eq(count_before)
  end
end
```

---

## 4. Testes de contrato

### 4.1 Snapshot do mapa de rotas do gateway

`services/gateway/spec/contracts/route_map_spec.rb`:

- A cada execução no CI, gera um snapshot JSON das rotas registradas no gateway.
- Compara com `spec/contracts/fixtures/route_map.expected.json`.
- Qualquer divergência quebra o build — força um revisor a reconhecer a mudança de superfície.

Caminhos rastreados:

- `POST /auth/register`
- `POST /auth/login`
- `GET /api/v1/users/me`
- `GET /api/v1/marketplace/products`
- `GET /api/v1/products/:id`
- `POST /api/v1/orders`
- `GET /api/v1/orders`
- `POST /api/v1/payments` (Sprint 08+)
- `GET /api/v1/payments/:id` (Sprint 08+)
- `POST /webhooks/psp/:psp_name` (Sprint 08+)

### 4.2 Schemas de payload Kafka

`infrastructure/kafka-schemas/` mantém JSON Schemas por tópico. Os produtores validam antes de publicar; os consumers validam ao receber. Mudanças no schema exigem um PR tocando o arquivo, tornando a revisão explícita.

| Tópico | Schema |
|-------|--------|
| `order.created` | `order-created-v1.json` |
| `payment.confirmed` | `payment-confirmed-v1.json` |
| `payment.failed` | `payment-failed-v1.json` |
| `inventory.reserved` | `inventory-reserved-v1.json` |

### 4.3 OpenAPI (leve)

`utilar-ferragem/docs/openapi.yaml` — mantido manualmente durante a Fase 3 (não gerado automaticamente). `utilar-ferragem/src/lib/apiTypes.ts` regerado a partir dele via `openapi-typescript`. Divergência entre OpenAPI e tipos TypeScript é capturada pelo `npm run typecheck`, pois o frontend importa helpers tipados de `apiTypes.ts`.

---

## 5. Caminhos dourados E2E (Playwright)

Localização: `utilar-ferragem/e2e/`.

### 5.1 Cenários de teste

1. **Cadastro + login**: Home → Cadastrar → preencher (nome + e-mail + senha + CPF) → login automático → redirecionar para home.
2. **Navegação + PDP**: Home → clicar em "Ferramentas Elétricas" → grade carrega → clicar no produto → PDP renderiza ficha técnica + preço + adicionar ao carrinho.
3. **Adicionar ao carrinho + persistência**: No PDP → adicionar → badge do carrinho atualiza → recarregar a página → carrinho ainda populado.
4. **Checkout Pix (caminho feliz)**: Carrinho → checkout → preencher CEP + endereço → escolher Pix → "Pagar" → QR do Pix renderiza → simular webhook via endpoint de teste → status do pedido "pago" em até 10s.
5. **Checkout boleto**: Mesmo, escolher boleto → URL do boleto renderiza → status do pedido permanece "pendente" (boleto confirma de forma assíncrona).
6. **Checkout cartão (aprovado no sandbox)**: Usar cartão de teste MP que aprova → pedido transiciona para pago em até 5s.
7. **Checkout cartão (recusado no sandbox)**: Usar cartão de teste MP que recusa → usuário vê erro, pode tentar outro método.
8. **Histórico de pedidos + detalhe**: Meus Pedidos → lista exibe pedido recente → clicar → página de detalhe com linha do tempo de status.
9. **Logout + novo login**: Logout → carrinho limpo no servidor, mas carrinho localStorage persiste para anônimo → novo login → carrinho mescla.
10. **Solicitação de exportação LGPD**: Configurações → "Baixar meus dados" → sink de e-mail mock recebe e-mail com URL assinada.

### 5.2 Ambiente

- Playwright executa contra um ambiente de staging dedicado (`staging.utilarferragem.com.br`).
- Dados de seed são atualizados nightly via tarefa programada no CloudWatch.
- Credenciais PSP = somente sandbox. Dados de teste em §8.
- Endpoint exclusivo para testes `POST /test/webhook-simulate` (habilitado via `STAGING_MODE=true`) permite ao runner E2E disparar um webhook assinado sem rotear pelo MP.

### 5.3 Comandos

```bash
npm run e2e               # headless, paralelo
npm run e2e:headed        # com interface para debug
npm run e2e:ui            # Playwright UI
npm run e2e:smoke         # subconjunto — executa contra produção pós-deploy
```

### 5.4 Subconjunto smoke

3 testes com a tag `@smoke` são executados contra produção após cada deploy:

- Home carrega, tem produtos
- PDP de um produto conhecido carrega
- Login com uma conta sentinel pré-cadastrada tem sucesso (sem colocar pedidos em produção)

Falhas no smoke fazem o rollback do deploy (ver [12-ops-runbook.md](12-ops-runbook.md) §3).

---

## 6. Testes de carga (k6)

Localização: `infrastructure/k6/scripts/`.

### 6.1 Cenários

| Script | Alvo | Duração |
|--------|--------|----------|
| `browse.js` | 100 usuários simultâneos navegando no catálogo + PDP | 10 min |
| `search.js` | 50 usuários simultâneos pesquisando queries variadas | 5 min |
| `checkout.js` | 20 usuários simultâneos fazendo checkout com Pix (sandbox) | 5 min |
| `webhook_flood.js` | 100 POSTs de webhook simultâneos (assinados) | 2 min |
| `mixed.js` | 80% navegação + 15% PDP + 5% checkout | 15 min |

### 6.2 Thresholds (bloco `thresholds` do k6)

```js
export const options = {
  thresholds: {
    'http_req_duration{group:browse}': ['p(95)<500'],
    'http_req_duration{group:pdp}':    ['p(95)<800'],
    'http_req_duration{group:checkout}': ['p(95)<2000'],
    'http_req_failed': ['rate<0.01'],
    'checks': ['rate>0.99'],
  },
};
```

### 6.3 Cadência

- **Pré-lançamento**: executar a suíte completa. Deve passar nos thresholds.
- **Mensal**: executar `mixed.js` contra staging; comparar com a baseline; alertar se regressão > 20%.
- **Pré-feature importante**: executar `mixed.js` antes + depois, comparar.

### 6.4 Ambiente

- Executar contra staging — nunca em produção.
- Usa sandbox do PSP.
- Usa contas de usuário dedicadas para teste de carga (`loadtest+0001@utilarferragem.com.br` … `loadtest+0500@`), pré-cadastradas.

---

## 7. Acessibilidade

### 7.1 Automatizada

`utilar-ferragem/e2e/a11y/` — Playwright + `@axe-core/playwright`:

- Verifica todas as páginas de nível superior (home, categoria, PDP, carrinho, checkout, conta, detalhe do pedido).
- Falha em qualquer violação WCAG 2.1 AA com severidade "serious" ou "critical".
- Moderada/menor → somente aviso.

### 7.2 Lighthouse CI

`.lighthouserc.js`:

```js
module.exports = {
  ci: {
    collect: { url: ['http://localhost:4173/', 'http://localhost:4173/ferramentas/eletricas'] },
    assert: {
      assertions: {
        'categories:performance':   ['error', { minScore: 0.85 }],
        'categories:accessibility': ['error', { minScore: 0.90 }],
        'categories:best-practices':['error', { minScore: 0.90 }],
        'categories:seo':           ['error', { minScore: 0.90 }],
      },
    },
  },
};
```

Executa em todo PR. Regressão abaixo do threshold bloqueia o merge.

### 7.3 Auditoria manual

Pré-lançamento: passe manual com leitor de tela usando NVDA (Windows) ou VoiceOver (macOS) em:

- Fluxo de cadastro
- Fluxo de checkout
- PDP
- Detalhe do pedido

Findings registrados como tickets; críticos bloqueiam o lançamento, os demais viram backlog da Fase 4.

---

## 8. Dados de sandbox de pagamento

Referência de ambiente de teste do Mercado Pago (credenciais sandbox via `PSP_ENVIRONMENT=sandbox`):

### 8.1 CPFs de teste (pagador aprovado)

- `12345678909` — CPF de teste genérico, aprovado
- `11144477735` — válido conhecido pelo Módulo 11, aprovado
- Adicionais: gerar via `app/validators/cpf_validator.rb#check_digit`

### 8.2 Cartões de teste (publicados pelo MP)

| Bandeira | Número | CVV | Validade | Nome | Resultado |
|-------|--------|-----|--------|------|---------|
| Mastercard | `5031 4332 1540 6351` | `123` | `11/30` | `APRO` | aprovado |
| Visa | `4235 6477 2802 5682` | `123` | `11/30` | `APRO` | aprovado |
| Mastercard | `5031 4332 1540 6351` | `123` | `11/30` | `CONT` | pendente |
| Visa | `4235 6477 2802 5682` | `123` | `11/30` | `OTHE` | recusado - genérico |
| Visa | `4235 6477 2802 5682` | `123` | `11/30` | `FUND` | recusado - saldo insuficiente |

(Manter esta tabela sincronizada com a documentação atual do MP — eles publicam dados de teste sandbox no portal do desenvolvedor.)

### 8.3 CNPJs de teste (para testes de onboarding de vendedores)

CNPJs válidos sintéticos gerados pelo script de seed do user-service (helper `compute_cnpj(base12)`). Pré-cadastrados em staging:

- `11222333000181` — Ferragens Teste Ltda
- `11444777000161` — Elétrica Demo S/A

### 8.4 Pix de teste

O sandbox do MP confirma automaticamente o Pix após ~10s. Sobrescreva com o endpoint de teste `POST /test/pix-confirm?payment_id=X` em staging.

### 8.5 Boleto de teste

O sandbox do MP oferece um botão "simular pagamento" no dashboard de sandbox — boletos pagos fazem webhook da mesma forma que os reais.

---

## 9. Gates de CI

Workflow do GitHub Actions em `.github/workflows/ci.yml`. Precedência dos gates: lint → typecheck → unit → integration → contract → e2e (somente nightly) → deploy.

### 9.1 Gates bloqueantes (falha → não pode fazer merge)

| Gate | Ferramenta | Modo de falha |
|------|------|--------------|
| Lint frontend | ESLint (zero warnings) | qualquer aviso |
| Typecheck frontend | `tsc --noEmit` | qualquer erro |
| Unitário frontend | `npm run test:run` | qualquer falha |
| Cobertura frontend | Vitest | cobertura de lib < 70% |
| Lint backend | rubocop | qualquer infração |
| Specs de backend | RSpec | qualquer falha |
| Snapshot de contrato | customizado | divergência no mapa de rotas sem commit `--update` |
| Validação de schema Kafka | jsonschema | qualquer teste de produtor falha na validação |
| Lighthouse | LHCI | abaixo dos thresholds §7.2 |
| axe | Suíte a11y do Playwright | serious/critical |
| Gitleaks | pre-commit + CI | qualquer segredo vazado |
| Bundler audit | `bundle-audit check` | CVE alto/crítico |
| npm audit | `npm audit --audit-level=high` | CVE alto/crítico |
| Drift de tipagem OpenAPI | diff de saída `openapi-typescript` | drift não commitado |

### 9.2 Gates de aviso (merge permitido, comentário postado)

| Gate | Ferramenta |
|------|------|
| Tamanho do bundle | `size-limit` — avisa se > +5% vs main |
| Cobertura de backend | simplecov — avisa se < 80% |
| SEO Lighthouse | avisa se caiu >5 pontos vs main |

### 9.3 Somente nightly

| Verificação | Ferramenta |
|-------|------|
| Suíte E2E completa | Playwright contra staging |
| Varredura baseline ZAP | OWASP ZAP |
| Regressão k6 `mixed.js` | k6 Cloud ou self-hosted |
| PRs do Dependabot abertos automaticamente | Dependabot |

### 9.4 Gates pré-deploy

- Todos os §9.1 verdes no commit de merge.
- Suíte nightly verde nas últimas 24h.
- Aprovação manual para produção (GitHub Environment `production` com revisor obrigatório).

---

## 10. Política de regressão de bugs

Todo bug de produção que chegue a um cliente deve resultar em:

1. Um teste que falhe reproduzindo o bug (adicionado **antes** da correção).
2. Uma nota de 1 linha em `docs/bug-regressions.md` com data, link do incidente, arquivo de teste.
3. O teste deve permanecer na suíte principal — não marcado como `skip` ou `only`.

Meta: menos de 1 semana desde o fechamento do incidente até o teste de regressão commitado.

---

## 11. Status

| Área | Status |
|------|--------|
| Scaffold Vitest + MSW | ⬜ Utilar 01 |
| RSpec no payment-service | ⬜ Utilar 08 |
| Fixtures de webhook assinadas | ⬜ Utilar 08 |
| Teste de snapshot de contrato | ⬜ Utilar 02 |
| JSON Schemas do Kafka | ⬜ Utilar 08 |
| Suíte Playwright E2E | ⬜ Utilar 09 |
| Scripts k6 | ⬜ Pré-lançamento |
| Lighthouse CI | ⬜ Utilar 02 |
| axe CI | ⬜ Utilar 02 |
| Gitleaks pre-commit | ⬜ Utilar 01 |
