# Observabilidade e alertas — payment-service

**Última atualização**: 2026-07-18

Antes deste trabalho, `grep -rn 'prometheus|otel|/metrics'` em todo o Go do repo retornava **um comentário**. Num sistema que move dinheiro, isso significa que "o outbox parou" ou "a taxa de recusa triplicou" só viravam notícia quando o cliente reclamava.

---

## 1. Endpoint `/metrics`

`GET /metrics` no payment-service (`:8090`), formato Prometheus.

### Não é público. Fail-closed.

```bash
METRICS_TOKEN=<token longo e aleatório>   # obrigatório em produção
```

| Situação | Resposta |
|---|---|
| `METRICS_TOKEN` não configurado | **404** (endpoint desabilitado) |
| `Authorization` ausente ou errado | **404** |
| `Authorization: Bearer <token>` correto | 200 + métricas |

**Por que 404 e não 401**: não confirmamos nem que o endpoint existe. E por que token, se "só a rede interna acessa"? Porque no ECS/K8s o "interno" inclui qualquer pod comprometido no mesmo cluster, e `/metrics` entrega volume financeiro, taxa de recusa por método e a topologia do sistema — é reconhecimento de graça para quem já entrou.

A comparação do token é em **tempo constante** (`subtle.ConstantTimeCompare`). Comparar com `==` vaza o prefixo por timing e permite recuperá-lo byte a byte.

**Esquecer de configurar não abre o endpoint** — desabilita. O modo inseguro exige ação deliberada, nunca omissão.

### Scrape

```yaml
scrape_configs:
  - job_name: utilar-payment
    metrics_path: /metrics
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/utilar-metrics-token
    static_configs:
      - targets: ['payment-service:8090']
```

### O que /metrics NÃO expõe

Nenhum label carrega `user_id`, `order_id`, `payment_id`, e-mail ou CPF. As séries usam **o padrão da rota** (`/api/v1/payments/:id`), nunca o path com o UUID.

Duas razões, ambas suficientes: (1) `/metrics` acaba em dashboards com acesso mais amplo que o banco — PII ali é vazamento; (2) uma série por pedido é uma *cardinality bomb* que derruba o Prometheus, e um scanner batendo em URLs aleatórias a dispara de graça. Rotas não encontradas colapsam em `route="unmatched"`.

Valores agregados (faturamento total) **estão** expostos — é o propósito. Por isso o token.

---

## 2. Métricas disponíveis

### HTTP (`pkg/metrics`, todos os serviços)

| Métrica | Tipo | Labels |
|---|---|---|
| `utilar_http_requests_total` | counter | `service`, `route`, `method`, `status` (`2xx`…`5xx`) |
| `utilar_http_request_duration_seconds` | histogram | `service`, `route`, `method` |
| `utilar_http_in_flight_requests` | gauge | `service` |

### Negócio (`internal/obs`, payment-service)

| Métrica | Tipo | Labels |
|---|---|---|
| `utilar_payments_created_total` | counter | `provider`, `method` |
| `utilar_payments_confirmed_total` | counter | `provider`, `method` |
| `utilar_payments_failed_total` | counter | `provider`, `method`, `reason` |
| `utilar_psp_request_duration_seconds` | histogram | `provider`, `operation` |
| `utilar_psp_errors_total` | counter | `provider`, `operation`, `kind` |
| `utilar_webhooks_received_total` | counter | `provider`, `outcome` |
| `utilar_outbox_pending_events` | gauge | — |
| `utilar_outbox_oldest_unpublished_age_seconds` | gauge | — |
| `utilar_outbox_published_total` | counter | `event_type`, `outcome` |
| `utilar_reconciliation_checked_total` | counter | `provider` |
| `utilar_reconciliation_discrepancies_total` | counter | `provider`, `kind` |
| `utilar_reconciliation_open_discrepancies` | gauge | — |
| `utilar_reconciliation_runs_total` | counter | `provider`, `status` |
| `utilar_ledger_transactions_posted_total` | counter | `kind` |
| `utilar_ledger_transactions_rejected_total` | counter | `kind`, `reason` |

`outcome` do webhook: `accepted`, `ignored`, `duplicate`, `rejected_signature`, `rejected_amount`, `psp_error`, `parse_error`, `process_error`, `provider_mismatch`, `read_error`.

---

## 3. Alertas que importam

Nenhum destes está configurado no Alertmanager — este é o documento de referência para quando for.

Ordem de importância: **dinheiro parado ou divergente > sistema lento**.

### 🔴 P1 — acorda alguém

#### 3.1 Outbox parado

```promql
utilar_outbox_oldest_unpublished_age_seconds > 300
```
`for: 2m`

**Por que ESTA métrica e não o tamanho da fila**: a fila pode estar pequena *e travada*. Um evento de 5 minutos parado significa que o pedido foi pago e o resto do sistema não sabe — cliente pagou e não recebe confirmação.

Limiar: o drainer roda a cada 2s. Qualquer coisa acima de 5 min é falha real, não backlog.

**Cuidado**: a métrica é alimentada por um poller *independente* do drainer. Se fosse incrementada dentro do drainer, ficaria congelada exatamente quando ele morresse — e o alerta nunca dispararia. Foi por isso que o poller é separado.

#### 3.2 Divergência de dinheiro na reconciliação

```promql
increase(utilar_reconciliation_discrepancies_total{kind=~"amount_mismatch|missing_at_psp"}[1h]) > 0
```

**Limiar zero, de propósito.** Uma única divergência de valor é bug nosso ou fraude; não existe "nível aceitável". O sistema não corrige sozinho — precisa de humano.

#### 3.3 Divergências acumulando sem tratamento

```promql
utilar_reconciliation_open_discrepancies > 5
```
`for: 24h`

Divergência aberta há mais de um dia significa processo quebrado, não incidente pontual.

#### 3.4 Trilha de auditoria adulterada

Sem métrica: rode `GET /api/v1/ledger/audit/verify` como job diário e alerte em `valid: false`. **Severidade máxima** — é a única evidência de que algo foi mexido no banco por fora da aplicação.

### 🟠 P2 — abre chamado no horário comercial

#### 3.5 Taxa de recusa anormal

```promql
sum(rate(utilar_payments_failed_total{method="card"}[15m]))
  /
sum(rate(utilar_payments_created_total{method="card"}[15m]))
  > 0.30
```
`for: 15m`

Base: cartão no e-commerce brasileiro recusa ~10–15% em operação normal. 30% sustentado por 15 min é antifraude do adquirente apertado, BIN bloqueado ou credencial errada.

Pix e boleto têm perfil diferente (quase não "recusam") — alerte separado com limiar mais baixo, ~5%.

**Ajuste com dados reais depois de 30 dias.** Alerta calibrado no chute vira alerta ignorado, que é pior que nenhum alerta.

#### 3.6 Webhook falhando

```promql
sum(rate(utilar_webhooks_received_total{outcome=~"psp_error|process_error"}[10m]))
  /
sum(rate(utilar_webhooks_received_total[10m]))
  > 0.10
```
`for: 10m`

A Appmax reentrega em 0, +30min, +2h, +4h e **depois descarta**. Uma falha sustentada por horas = pagamentos confirmados que nunca chegam até nós.

#### 3.7 Webhook rejeitado por valor

```promql
increase(utilar_webhooks_received_total{outcome="rejected_amount"}[1h]) > 0
```

Limiar zero: é o detector de fraude de C3 disparando. Pode ser bug de arredondamento — mas é para alguém olhar hoje.

#### 3.8 PSP lento

```promql
histogram_quantile(0.95,
  sum by (le, provider) (rate(utilar_psp_request_duration_seconds_bucket[10m]))
) > 5
```
`for: 10m`

O timeout do client é 30s. p95 acima de 5s significa checkout visivelmente travando.

#### 3.9 Venda confirmada sem lançamento contábil

```promql
increase(utilar_reconciliation_discrepancies_total{kind="ledger_missing"}[6h]) > 0
```

O faturamento do dashboard vai deixar de bater com o extrato.

### 🟡 P3 — investigue esta semana

```promql
# Erro 5xx sustentado
sum(rate(utilar_http_requests_total{status="5xx"}[10m]))
  / sum(rate(utilar_http_requests_total[10m])) > 0.02

# Lançamentos recusados pelo livro (bug de integração)
increase(utilar_ledger_transactions_rejected_total[1h]) > 0

# Fila do outbox crescendo (ainda drenando, mas atrás)
utilar_outbox_pending_events > 1000
```

### Alerta de ausência de sinal

```promql
absent_over_time(utilar_reconciliation_runs_total[26h])
```

A reconciliação não ter rodado é tão grave quanto ela achar problema: ninguém sabe se está tudo bem. Vale o mesmo para `up{job="utilar-payment"} == 0`.

---

## 4. Correlação: seguir um pedido ponta a ponta

### `request_id` agora atravessa serviços

Era o buraco central: `pkg/requestid` existia, mas o id morria no middleware HTTP. Os clients service-to-service abriam requests novas sem header nenhum, e um checkout que passa por 3 serviços virava 3 traços desconexos.

Como funciona agora:

1. `handler.RequestID()` gera/propaga o `X-Request-Id` e o coloca **no `context.Context` da request** (`requestid.NewContext`), não só no `gin.Context`.
2. O transport de `pkg/httpclient` lê o id do context e injeta o header em **toda** chamada feita com `NewRequestWithContext` — que é o padrão de todos os clients do repo.
3. Zero mudança nos call sites.

Regras: não sobrescreve header explícito do chamador; não inventa id quando o context não tem (um id gerado ali não estaria em nenhum log a montante); clona a request antes de mexer no header (contrato do `net/http` — mutar quebra retry e é data race).

**Para os outros serviços**: adicionar a mesma linha no middleware `RequestID()` de cada um. `pkg/httpclient` já faz o resto. (auth/order/catalog são de outro agente — não toquei.)

### Logs estruturados

Toda linha de request carrega `request_id`, `user_id`, `role`, `method`, `path` (padrão da rota), `status`, `duration_ms`, `remote_ip`.

`user_id` é UUID opaco — não é PII por si só. Nome, e-mail e CPF continuam fora (audit M5), e `redactLogValue()` mascara e-mail/CPF/PAN que vazem em mensagem de erro.

Busca típica numa investigação:

```
request_id="01JABC..." | ordenar por timestamp
```

Aparecem, em ordem: o POST no payment-service, a chamada ao order-service, a ao auth-service, a ao PSP, o webhook e o lançamento contábil.

---

## 5. Pendências

- Alertmanager não configurado (só documentado aqui).
- Sem tracing distribuído (OpenTelemetry). O `request_id` cobre a correlação em log; span/timing por serviço, não.
- `/metrics` só no payment-service. auth/order/catalog precisam de 3 linhas cada (`metrics.New` + `Middleware` + `Handler`) — outro agente está nesses serviços.
- Limiares de recusa (3.5) são estimativa de mercado. **Recalibrar com 30 dias de dado real.**
