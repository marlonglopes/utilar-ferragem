# Observabilidade & Auditoria — roadmap do Admin

Objetivo: dar ao Admin do Utilar visão completa de **conversão, bugs e auditoria —
principalmente do caminho do pagamento** — no nível do que o gifthy já tem, mas
construído para a arquitetura do Utilar (SPA + 5 serviços Go, cada um com seu banco).

> ⛔ **Contas dedicadas, sempre.** Sentry, Google/GA4, AWS, 1Password, registro.br,
> GoDaddy: **tudo novo e próprio do Utilar** — nunca reusar as contas/DSN/IDs do
> gifthy. Ver `docs/SEPARATION-utilar-vs-gifthy.md`. Todo segredo entra por variável
> de ambiente, **sem fallback hardcoded** (o gifthy já vazou DSN no git — não repetir).
> Cada integração é **fail-open/no-op quando a env não está setada**, então dá para
> mergear e testar tudo antes de os provedores existirem.

## Como ler este doc

- **Reference** = como o gifthy faz (só padrão; não copiamos credencial nem dado).
- **Utilar hoje** = o que já existe no repo.
- **Fazer** = o incremento, sempre com teste (regra do dono: nada entra sem teste).

---

## Estado atual do Utilar (resumo)

**Forte e real (não refazer):**
- `pkg/audit` — trilha append-only, hash-encadeada, imutável por trigger, IP mascarado,
  segredos/PAN removidos. **Só está ligada no payment-service.**
- Livro contábil de partidas dobradas (`payment-service/internal/ledger`) — soma-zero por
  constraint, idempotente, correção só por estorno, fechamento de período, export
  CSV/balancete/OFX. Admin: `AccountingPage`.
- Reconciliação vs PSP (`reconcile.go`) — nunca autocorrige; grava divergências
  (`amount_mismatch`, `status_mismatch`, `missing_at_psp`, `ledger_missing`).
- Métricas Prometheus (`pkg/metrics`) + métricas de negócio do pagamento
  (`payment-service/internal/obs`): pagamentos criados/confirmados/falhos, webhooks por
  desfecho, idade do outbox, reconciliação, ledger. `/metrics` fail-closed por token.
- Admin: `admin_dashboard.go` (overview, funil, pedidos travados, performance de vendedor
  com margem) + agregador de observabilidade (`catalog/observability.go`) que raspa
  `/metrics` e `/health` dos serviços (p50/p95/p99, 5xx, RPM, painel de outbox).
- `pkg/requestid` (ULID `X-Request-Id`) nos 4 serviços core; `slog` + AccessLog nos 5.

**Buracos (é o que este roadmap fecha):**
1. SPA sem **error reporting** e sem **analytics** (Sentry é um TODO; nenhum GA/telemetria).
2. `pkg/audit` **não** está em catalog/order/auth/assistant → a UI de auditoria promete
   ações (`price_change`, `order_status_change`, `user_role_change`, `admin_access`,
   `discount_approval`, `refund_issued`) que **nenhum backend emite** (hoje é mock).
3. Sem endpoint admin de **pagamentos falhos / estornos** e sem **timeline por pagamento**.
4. **auth e assistant sem métricas**; assistant invisível no painel. Sem **tracing**.
5. `auth_events` (login/registro/reset) existe mas **sem API admin** para ler.
6. Conversão é só uma **razão agregada** no servidor — sem funil de evento no front.

---

## Pilar 1 — BUGS (Sentry, FE + BE)

**Reference (gifthy):** `@sentry/react` no front (`Instrumentation.tsx`: init env-gated,
`browserTracingIntegration`, release/env tags, `tracePropagationTargets` só do domínio
próprio, `beforeSend` que remove PII e derruba ruído 4xx/401/cancelamento; `ErrorBoundary`
raiz + por-rota). `getsentry/sentry-go` + `sentry-go/gin` no back (`config/sentry.go`:
DSN só por env, `BeforeSend` redator+antirruído, spans de DB por statement, helpers
`CaptureErr/CaptureMessage`, `RecoverWorker` para os workers).

**Utilar hoje:** nada no front (só `ErrorBoundary.tsx` com `// TODO(observabilidade)`).
Nada de Sentry no back.

**Fazer:**
- **`pkg/observability` (Go, novo):** init Sentry env-gated (`SENTRY_DSN` vazio = off),
  `BeforeSend` que reusa o scrubber que já existe em `pkg/audit/scrub.go` + filtro de ruído
  (4xx de negócio, client-disconnect, PSP dormente), middleware Gin, `CaptureErr/Message`,
  `RecoverWorker` (para o drainer do outbox, o sweeper de reservas, o poller de métricas).
  Tags `service`, `env`, `release` (git SHA no build). Distribui em **todos os 5 serviços**.
- **SPA:** `@sentry/react`, `src/lib/observability.ts` (init env-gated por
  `VITE_SENTRY_DSN`, `tracePropagationTargets` só do domínio do Utilar, `beforeSend`
  redator espelhando o scrub do back, filtro de ruído). Ligar o `ErrorBoundary` raiz + o
  TODO existente. `VITE_SENTRY_*` no `.env`.
- **Testes:** redação de PII (cartão/CPF/e-mail/token), filtro de ruído (4xx não sobe, 5xx
  sobe), no-op quando DSN vazio.
- **Conta:** criar org/projetos Sentry do Utilar (1 front + 5 back, ou 1 back multi-serviço
  por tag). DSNs no 1Password do Utilar; nunca no git.

## Pilar 2 — AUDITORIA (o foco: caminho do pagamento + ações de Admin)

**Reference (gifthy):** `audit_events` (CloudTrail: actor, ação, target, before/after, ip,
correlation) + `payment_events` (transições de status append-only) + ledger com hash-chain
+ dashboard forense `Audit.tsx` (Overview, Admin Actions "quem fez o quê", Reconciliation,
Fraud com risco, Ledger balances, Webhooks com payload cru + reprocessar, Timeline por
pagamento, Timeline por correlation_id).

**Utilar hoje:** `pkg/audit` (melhor que o do gifthy: hash-chain + `seq` + imutável por
trigger) **mas só no payment**; ledger + reconciliação prontos; `AuditTrailPage` liga só no
payment e o vocabulário de ações não-financeiras é mock.

**Correção do mapa (importante):** o order **já audita** as ações sensíveis de dinheiro,
só que em tabelas próprias, não no `pkg/audit`: `balcao_audit_events` (aprovação de
desconto, liquidação externa) e `return_audit_events` (devolução/estorno), ambas
**fail-closed e no MESMO tx** do fato (`auditTx` do balcão, `auditReturnTx`). O auth
também tem `auth_events` (login/registro/reset). O buraco real é: (a) essas trilhas estão
espalhadas em tabelas por-domínio, e (b) **nenhuma tem API admin de leitura** — o
`AuditTrailPage` só lê a trilha `pkg/audit` do payment. Então a 1ª entrega é mais barata do
que parecia.

**Fazer:**
- **API admin de leitura unificada** que agrega as trilhas que JÁ existem
  (`pkg/audit` do payment + `balcao_audit_events` + `return_audit_events` + `auth_events`)
  num formato comum ("quem fez o quê, quando, antes→depois"), filtrável por ação/entidade/
  ator. É o que enche o `AuditTrailPage` de dado real (hoje o vocabulário não-financeiro é
  mock). Cada serviço expõe a leitura da sua própria trilha; o admin do front consolida.
- **Ligar `pkg/audit` (hash-chain) onde ainda não há trilha nenhuma**, nas ações que a UI
  anuncia e que hoje não são gravadas:
  - catalog: `price_change`, `product_publish/archive`, `import_commit`, `cost_change`.
  - auth: `user_role_change`, `operator_create`, `admin_access` (login de admin).
  - order: manter as tabelas de balcão/devolução (já são fail-closed) e, se valer a
    consistência, migrar depois para o `pkg/audit` — não reescrever o que já funciona agora.
  - Cada `Record` no MESMO tx da escrita de negócio (o `RecordTx` já suporta).
- **Timeline por pagamento / por request-id:** endpoint admin que junta, para um pagamento,
  `payments` + transições + webhooks (payload cru arquivado) + lançamentos do ledger +
  eventos do outbox, ordenados no tempo. (Precisa arquivar o **corpo cru do webhook** numa
  tabela `webhook_events`, hoje inexistente — o webhook valida e descarta.)
- **Lista de pagamentos falhos / estornos:** `GET /admin/payments?status=failed|refunded…`
  com motivo do decline (do-not-honor, saldo, risco…) — hoje só existe contador agregado.
- **Verificação de cadeia** já existe (`/ledger/audit/verify`); expor no dashboard forense.
- **Testes de regressão nomeados:** "admin não aprova o próprio desconto vira auditoria",
  "mudança de preço registra old→new", "webhook forjado aparece na timeline como rejeitado".

## Pilar 3 — CONVERSÃO (funil de verdade, FE + server-side)

**Reference (gifthy):** GA4 (gtag + **Consent Mode v2 / LGPD**) com eventos de e-commerce
(`view_item`, `add_to_cart`, `begin_checkout`, `purchase`), `_ga` client_id levado ao
checkout, e **purchase server-side** via Measurement Protocol no webhook de pago (sobrevive
a adblock/fechar aba). Telemetria first-party (`product_events`) em paralelo. Funil no
admin (view→checkout→purchase com % acumulado).

**Utilar hoje:** só `conversionRate = confirmed/created` derivado do order.

**Fazer:**
- **SPA:** `src/lib/analytics.ts` env-gated (`VITE_GA_MEASUREMENT_ID` vazio = no-op),
  Consent Mode v2, wrapper de e-commerce; instrumentar `view_item`, `add_to_cart`,
  `begin_checkout` no fluxo real do carrinho/checkout; capturar `_ga` client_id no checkout.
- **Server-side purchase:** no payment-service, ao confirmar pagamento, disparar `purchase`
  via Measurement Protocol (`GA_MEASUREMENT_ID`+`GA_API_SECRET`, no-op sem env) com
  `transaction_id` = id do pedido (dedup no GA). Async, best-effort.
- **Funil first-party (opcional, sem GA):** tabela `product_events` + `POST /track` para não
  depender só do GA; alimenta um painel de funil no admin.
- **Testes:** eventos disparam com os campos certos; no-op sem env; consent negado não envia.

## Pilar 4 — "TUDO" (fechar o resto)

- **Métricas em auth e assistant** (`metrics.New` + `/metrics` por token) e o agregador de
  observabilidade passar a **probar o assistant**.
- **Worker de forense agendado** (reference: `pkg/forensics` do gifthy, de hora em hora):
  Utilar já tem `reconcile.go`; agendar as invariantes (txn desbalanceada, soma≠0, pago sem
  ledger, ledger sem pago, drift de status, webhook preso, estorno sem ledger, verificar
  hash-chain) + **alerta** (Sentry sempre para crítico; e-mail via provedor do Utilar).
- **Scan de fraude** simples (velocidade, teste de cartão) com score — reference
  `pkg/forensics/fraud.go`.
- **Tracing distribuído** (OpenTelemetry) — hoje inexiste; correlação só por request-id no
  log. Item maior, fase própria.

## Pilar 5 — UI do Admin (páginas novas)

Reusar `AdminShell`, `StatTile`, `Sparkline`, `Meter`, `AlertList` já existentes.
- **Observability / Bugs:** board de issues do Sentry (via API REST do Sentry do Utilar),
  KPIs (fatais, abertas, novas 24h, usuários afetados), split back/front, deep-link.
- **Payments:** receita paga, taxa de aprovação, ticket médio, estornos; por status
  (pago/pendente/falhou/em_disputa/estorno_parcial); por método; transações recentes com
  ação de estorno (real, auditada, idempotente); **timeline por pagamento**.
- **Audit forense:** ações (quem fez o quê), webhooks com payload cru + reprocessar, ledger
  balances, reconciliação, verificação de cadeia, fraude.
- **Conversão:** funil view→checkout→purchase (GA4 + first-party), fontes, top produtos.

---

## Sequência sugerida (cada fase fecha sozinha, com teste)

| Fase | Entrega | Depende de conta externa? |
|---|---|---|
| **1** | `pkg/observability` + Sentry nos 5 serviços + Sentry na SPA (env-gated) | Não (DSN entra depois) |
| **2** | Wire `pkg/audit` em catalog/order/auth + API de leitura de `auth_events` | Não |
| **3** | `webhook_events` (payload cru) + timeline por pagamento + lista de falhos/estornos | Não |
| **4** | `src/lib/analytics.ts` + funil FE + purchase server-side (GA4) | GA4 (no-op até existir) |
| **5** | Métricas auth/assistant + worker de forense + alerta | E-mail/Sentry do Utilar |
| **6** | Páginas admin: Observability, Payments, Audit forense, Conversão | Sentry (board) |
| **7** | Tracing distribuído (OpenTelemetry) | Não (coletor próprio) |

Fases 1–3 são as de maior valor para o foco declarado (bugs + auditoria do pagamento) e
**não dependem de nenhuma conta** — dá para entregar já, com o Sentry/GA "escuro" até o
Thomazinho criar as contas do Utilar.

## Checklist de contas a provisionar (Utilar, dedicadas)

- [ ] **Appmax** — conta do Utilar (client_id/secret v1 e/ou access-token v3 próprios).
      Substituir o token de sandbox do gifthy que está hoje no `.env.local`.
- [ ] **Sentry** — org do Utilar; projeto SPA + projeto(s) dos serviços Go; DSNs no 1Password.
- [ ] **Google** — conta Google do Utilar → propriedade GA4 (measurement ID + API secret) e,
      se for usar GA Data API no admin, service-account.
- [ ] **AWS** — conta dedicada (ver `docs/orcamento-utilar-aws-2026-07.md`).
- [ ] **1Password** — cofre do Utilar para todos esses segredos.
- [ ] **registro.br / GoDaddy** — domínio próprio (também é o `tracePropagationTargets` do
      Sentry e o domínio de vendas da Appmax).
