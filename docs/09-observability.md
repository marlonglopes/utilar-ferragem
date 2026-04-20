# 09 — Observabilidade

**Status**: Rascunho. **Data**: 2026-04-20.

O que instrumentamos, como alertamos e o que significa "saudável". Escopo: logs, métricas, traces, dashboards, alertas, SLOs, escolha de ferramentas.

Leituras complementares:
- [08-security.md](08-security.md) §9 — regras de redação que limitam o que logamos
- [12-ops-runbook.md](12-ops-runbook.md) — o que fazer quando os alertas disparam
- [11-infra.md](11-infra.md) — hospedagem para o backend de telemetria

---

## 1. Escolha de ferramentas

**Recomendação: CloudWatch Logs + CloudWatch Metrics + Sentry (erros de frontend + backend), com coletores OpenTelemetry alimentando o AWS X-Ray para traces. Pular Grafana/Loki/Tempo no lançamento.**

Justificativa:

| Opção | Prós | Contras | Veredicto |
|--------|------|------|---------|
| CloudWatch + X-Ray + Sentry | Zero infra nova; IAM nativo; pay-per-use; free tier do Sentry nos atende; X-Ray integra com ALB | Busca em logs mais lenta que Loki; dashboards menos customizáveis que Grafana | **Escolhido** |
| Grafana Cloud + Loki + Tempo | Melhor DX; dashboards flexíveis; amigável ao open-source | Novo serviço para gerenciar; outra conta (≈$50–100/mês); curva de aprendizado da equipe | Reavaliar na Fase 4 |
| ELK / Prometheus self-hosted | Controle máximo | Overhead operacional para um fundador solo — não é opção | Não |
| Datadog | UX de primeira classe | ≈$15/host/mês × 4+ serviços = $720+/mês | Não |

A migração para Grafana Cloud é uma decisão da Fase 4, acionada por: (a) volume de logs >50GB/mês, (b) mais de 2 engenheiros, ou (c) limites de amostragem de traces do X-Ray tornando-se problemáticos.

Resumo da stack:

| Camada | Ferramenta | Variáveis de ambiente |
|-------|------|----------|
| Logs de backend | CloudWatch Logs (via driver Docker `awslogs`) | `AWS_LOGS_GROUP` |
| Erros de frontend | Sentry | `VITE_SENTRY_DSN` |
| Erros de backend | Sentry (SDKs Rails + Go) | `SENTRY_DSN` |
| Métricas | CloudWatch custom metrics (via coletor OTel) | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| Traces | AWS X-Ray (via coletor OTel) | Mesmo |
| Dashboards | CloudWatch Dashboards | — |
| Alertas | CloudWatch Alarms → SNS → Sentry → Telefone (via SMS Twilio) | `ALERT_PAGER_PHONE` |
| Checks de uptime | UptimeRobot free tier (50 monitores) | — |

---

## 2. Logging

### 2.1 Formato

JSON estruturado, um evento por linha. Sem tracebacks multilinha — stack trace é um campo string único em JSON.

**Serviços Rails** usam `lograge` + `lograge-sql`:

```ruby
# config/environments/production.rb
config.lograge.enabled = true
config.lograge.formatter = Lograge::Formatters::Json.new
config.lograge.custom_options = ->(event) {
  {
    time: event.time.iso8601(3),
    request_id: event.payload[:request_id],
    user_id: event.payload[:user_id],
    seller_id: event.payload[:seller_id],
    trace_id: event.payload[:trace_id]
  }
}
```

**Gateway Go** usa `go.uber.org/zap` em modo de produção (encoder JSON).

### 2.2 Campos obrigatórios (toda linha de log)

| Campo | Origem | Notas |
|-------|--------|-------|
| `time` | logger | ISO8601 com ms |
| `level` | logger | `debug` / `info` / `warn` / `error` / `fatal` |
| `service` | env (`SERVICE_NAME`) | `gateway` / `user-service` / etc. |
| `env` | env (`APP_ENV`) | `prod` / `staging` |
| `request_id` | gateway injeta via `X-Request-ID`, propagado downstream | UUID v4 |
| `trace_id` | OTel | Hex-16 |
| `span_id` | OTel | Hex-8 |
| `msg` | código | Mensagem humana curta |

Opcionais (quando aplicável):

| Campo | Quando |
|-------|------|
| `user_id` | Requisições autenticadas |
| `seller_id` | Requisições com escopo de vendedor |
| `order_id` | Eventos relacionados a pedidos |
| `payment_id` | Eventos de pagamento |
| `psp_payment_id` | Eventos de pagamento/webhook |
| `status` | Respostas HTTP |
| `duration_ms` | Respostas HTTP |
| `path`, `method` | Requisições HTTP |

### 2.3 Regras de redação

Regras rígidas (ver [08-security.md](08-security.md) §9):

| Nunca logar | Por quê |
|-----------|-----|
| CPF, CNPJ (completo, mesmo mascarado de forma inconsistente) | Sensível pela LGPD |
| PAN do cartão, CVV | PCI |
| Senha bruta, hash da senha | Credenciais |
| JWT completo (header `Authorization`) | Roubo de sessão |
| Body do webhook (completo) | Contém segredos do PSP / metadados de cartão |
| `params[:password]`, `params[:card]`, `params[:cpf]` | Filtro no nível de parâmetro |

Implementação:

- Rails: `config.filter_parameters += %i[password cpf cnpj card card_number cvv cvc authorization token psp_metadata]`
- Go: logger encapsulado `zap` com hook `redactHeaders(["authorization", "cookie"])`
- Sentry: `beforeSend` no frontend + `before_send` no Rails remove os mesmos campos

### 2.4 Retenção

| Grupo de logs | Retenção | Justificativa |
|-----------|-----------|---------------|
| `/utilar/prod/gateway` | 30 dias | Operações padrão |
| `/utilar/prod/user-service` | 30 dias | Operações padrão |
| `/utilar/prod/product-service` | 30 dias | Operações padrão |
| `/utilar/prod/order-service` | 180 dias | Janela de disputas de pedidos |
| `/utilar/prod/payment-service` | 180 dias | Janela de disputa de pagamento / chargeback |
| `/utilar/prod/audit` | 2 anos | Incidentes de segurança (stream dedicado) |
| `/utilar/staging/*` | 7 dias | Ruído de não-produção |

A retenção do CloudWatch Logs é configurada por stream; definida no Terraform.

---

## 3. Métricas

Todas as métricas emitidas como CloudWatch custom metrics sob o namespace `Utilar`.

### 3.1 Métricas HTTP (por serviço)

| Métrica | Unidade | Tags | Origem |
|--------|------|------|--------|
| `http.requests_total` | Contagem | `service`, `method`, `status`, `route` | Gateway + Rails |
| `http.request_duration_ms` | ms (distribuição por percentil) | Mesmo | Gateway + Rails |
| `http.4xx_rate` | % | `service`, `route` | Derivada |
| `http.5xx_rate` | % | `service`, `route` | Derivada |
| `http.active_connections` | Gauge | `service` | Gateway |

### 3.2 Métricas de pagamento

| Métrica | Unidade | Tags | Origem |
|--------|------|------|--------|
| `payment.created_total` | Contagem | `method`, `psp` | payment-service |
| `payment.confirmed_total` | Contagem | `method`, `psp` | payment-service |
| `payment.failed_total` | Contagem | `method`, `psp`, `reason_code` | payment-service |
| `payment.success_rate` | % | `method` | Derivada (rolante de 5min) |
| `payment.time_to_confirm_seconds` | dist | `method` | payment-service (de created → confirmed) |
| `payment.webhook_received_total` | Contagem | `psp`, `event_type` | payment-service |
| `payment.webhook_failed_total` | Contagem | `psp`, `reason` | payment-service |
| `payment.pix_expiry_total` | Contagem | — | cron na expiração |

### 3.3 Métricas de pedidos

| Métrica | Unidade | Tags | Origem |
|--------|------|------|--------|
| `order.created_total` | Contagem | `seller_id_bucket` | order-service |
| `order.status_duration_seconds` | dist | `from_status`, `to_status` | order-service |
| `order.checkout_completion_rate` | % | — | Derivada (pedidos / sessões-com-carrinho) |
| `order.cart_abandon_rate` | % | — | Derivada (carrinhos sem checkout / cart_started) |
| `order.average_ticket_cents` | gauge | `seller_id_bucket` | Derivada (rolante de 1h) |

### 3.4 Métricas de catálogo

| Métrica | Unidade | Tags | Origem |
|--------|------|------|--------|
| `catalog.search_requests_total` | Contagem | `has_results` | product-service |
| `catalog.search_zero_results_rate` | % | — | Derivada (sinal de lacunas de conteúdo) |
| `catalog.pdp_views_total` | Contagem | `category` | product-service |

### 3.5 Métricas de infra

| Métrica | Unidade | Origem |
|--------|------|--------|
| `kafka.consumer_lag` | Contagem (mensagens) | Redpanda exporter |
| `kafka.consume_errors_total` | Contagem | Consumers |
| `db.connections_used` | Gauge | Rails (rack-mini-profiler) |
| `db.slow_queries_total` | Contagem (>500ms) | Scrape do log do Postgres |
| `redis.evictions_total` | Contagem | Redis INFO |
| `s3.upload_errors_total` | Contagem | Uploads de imagens de produtos |

### 3.6 Métricas de frontend (Sentry + GA4)

Sentry Performance Monitoring (free tier, 10k transações/mês):

- `web.lcp` (Largest Contentful Paint) p75
- `web.fid` / `web.inp` (interação) p75
- `web.cls` (Cumulative Layout Shift) p75
- `web.ttfb` (Time to First Byte) p75
- `route.change_duration_ms` por rota

Stream de eventos do GA4 (para funil):

- `page_view`, `view_item`, `add_to_cart`, `begin_checkout`, `purchase`.

---

## 4. Traces

Auto-instrumentação OpenTelemetry onde disponível; spans manuais para transições de domínio.

### 4.1 Spans principais para instrumentar

| Operação | Spans | Serviço |
|-----------|-------|---------|
| **Fluxo de checkout** | `http.POST /api/v1/orders` → `order.validate_cart` → `order.persist` → `kafka.publish order.created` | order-service |
| **Criação de pagamento** | `http.POST /api/v1/payments` → `payment.validate_order` → `psp.create_preference` → `payment.persist` | payment-service |
| **Handler de webhook** | `http.POST /webhooks/psp/:name` → `webhook.verify_signature` → `payment.upsert` → `kafka.publish payment.confirmed` | payment-service |
| **Transição de pedido pago** | `kafka.consume payment.confirmed` → `order.transition paid` → `email.send receipt` | order-service |
| **Reserva de estoque** | `kafka.consume order.created` → `inventory.reserve` → `inventory.persist` | inventory-service |
| **Busca no catálogo** | `http.GET /api/v1/marketplace/products` → `db.query` → `serialize` | product-service |

### 4.2 Propagação

- O gateway injeta o header `traceparent` (W3C Trace Context) nas requisições de entrada.
- Os serviços downstream continuam o trace.
- Mensagens Kafka carregam `traceparent` nos headers; os consumers vinculam o span downstream.

### 4.3 Amostragem

- 100% para `POST /api/v1/payments`, `POST /api/v1/orders`, `POST /webhooks/psp/*` (caminhos críticos).
- 10% para `GET /api/v1/marketplace/*` (amostrado — alto volume).
- 100% para qualquer requisição que retorne 5xx (amostragem baseada em cauda via coletor OTel).

Free tier do X-Ray: 100k traces/mês, 1M queries/mês. A amostragem acima nos mantém confortavelmente abaixo desses limites.

---

## 5. Dashboards

CloudWatch Dashboards, um por público.

### 5.1 Dashboard "Saúde da API"

Widgets:

- Requisições/seg por serviço (gateway, user, product, order, payment) — linha empilhada
- Taxa de 5xx por serviço — linha, threshold em 1%
- Latência p50/p95/p99 por serviço — linha
- Conexões DB ativas por serviço — linha
- Memória Redis utilizada — valor único
- Saúde do target ALB — valor único (targets saudáveis / total)

### 5.2 Dashboard "Funil de pagamento"

Widgets:

- Pedidos criados → pagamentos criados → pagamentos confirmados (barras de funil, 24h)
- Taxa de sucesso de pagamento por método (pix / boleto / cartão) — valor único + sparkline
- Tempo para confirmação p50/p95 (pix vs boleto vs cartão) — linha
- Webhooks com sucesso vs falha — área empilhada
- Chargebacks nos últimos 30 dias — número

### 5.3 Dashboard "Operações de vendedor"

Widgets:

- Pedidos por status (pendente / pago / enviado / entregue / cancelado) — pizza
- Produtos publicados por vendedor (top 10) — barras
- Eventos de estoque baixo nas últimas 24h — linha
- Avaliações pendentes de moderação — número
- Profundidade da fila de aprovação de vendedores — número + alerta se >5

### 5.4 Dashboard "Kafka & infra"

Widgets:

- Lag do consumer por tópico (`order.created`, `payment.confirmed`, `payment.failed`) — linha, alerta em 1000
- Mensagens produzidas/seg por tópico — linha
- Profundidade da DLQ por tópico — número, alerta em >0
- Memória/CPU do container por serviço — linha
- Uso de disco por serviço — linha, alerta em 80%

### 5.5 Dashboard "Vitais do frontend" (Sentry)

- LCP p75 (orçamento 2,5s)
- INP p75 (orçamento 200ms)
- CLS p75 (orçamento 0,1)
- Taxa de erros por página — linha
- Top 10 tipos de erro — tabela

---

## 6. Alertas

Tópico SNS `utilar-alerts-prod` → assinaturas (SMS via Twilio ou integração de plantão do Sentry, e-mail, webhook Slack).

Três severidades:

- **P0 (urgente)**: SMS + Sentry + Slack — 24/7
- **P1 (notificar)**: Slack + e-mail — reconhecido em horário comercial, adiado fora do horário
- **P2 (ticket)**: Somente Slack — triado no próximo dia útil

### 6.1 Alertas P0 (urgentes)

| Alerta | Condição | Janela | Notificar |
|-------|-----------|--------|--------|
| Taxa de sucesso de pagamento | `payment.success_rate` < 95% | 5 min | SMS + Sentry |
| Taxa de 5xx por serviço | qualquer serviço > 1% | 2 min | SMS + Sentry |
| Gateway fora do ar | Health check falha | 1 min | SMS |
| Target ALB não saudável | Targets saudáveis = 0 | 1 min | SMS |
| Falhas de assinatura de webhook | `payment.webhook_failed_total` > 10 | 5 min | SMS (possível ataque) |
| Conexões DB esgotadas | `db.connections_used` / max > 90% | 2 min | SMS |

### 6.2 Alertas P1 (notificar)

| Alerta | Condição | Janela |
|-------|-----------|--------|
| Lag do consumer Kafka | `kafka.consumer_lag` > 1000 em qualquer tópico | 10 min |
| Pico na taxa de 4xx | `http.4xx_rate` > 10% por rota | 10 min |
| Zero resultados na busca | `catalog.search_zero_results_rate` > 30% | 30 min |
| Uso de disco | > 80% em qualquer serviço | 15 min |
| Novo issue no Sentry | Visto pela primeira vez em qualquer erro com >10 eventos | imediato |
| DLQ não vazia | Qualquer tópico DLQ tem > 0 mensagens | 5 min |

### 6.3 Alertas P2 (ticket)

| Alerta | Condição | Janela |
|-------|-----------|--------|
| Queries lentas | `db.slow_queries_total` aumenta >2× da baseline | 1h |
| Taxa de expiração de Pix | > 40% dos pagamentos Pix criados expiram | 1 dia |
| Fila de aprovação de vendedor | > 5 pendentes por > 48h | diário |
| Bounces de e-mail | Taxa de bounce do SES > 2% | 1 dia |

### 6.4 Higiene de alertas

- Todo alerta tem um link de runbook na descrição ([12-ops-runbook.md](12-ops-runbook.md)).
- Revisão trimestral: silenciar alertas barulhentos; adicionar alertas para incidentes do trimestre anterior que não tinham um.
- Sem alerta sem plano de resposta — se não há nada a fazer, é uma métrica, não um alerta.

---

## 7. SLOs

Quatro caminhos críticos. Cada um tem um SLO, um orçamento de erros e uma cadência de revisão (mensal durante o lançamento, trimestral após a Fase 4).

| Caminho crítico | SLI | Meta do SLO | Janela | Orçamento de erros |
|---------------|-----|-----------|--------|--------------|
| **Disponibilidade do checkout** | Taxa de sucesso de `POST /api/v1/orders` (2xx) | 99,5% | 30 dias rolantes | 3,6h/mês |
| **Confirmação de pagamento** | Latência p95 de webhook-para-pedido-pago | < 5s | 30 dias rolantes | N/A (SLO de latência) |
| **Busca no catálogo** | Latência p95 de `GET /api/v1/marketplace/products` | < 500ms | 30 dias rolantes | — |
| **Disponibilidade do PDP** | Taxa de sucesso de `GET /api/v1/products/:id` (2xx) | 99,9% | 30 dias rolantes | 43min/mês |
| **Disponibilidade de login** | Taxa de sucesso de `POST /auth/login` (não-4xx) | 99,5% | 30 dias rolantes | 3,6h/mês |

Quando o orçamento de erros consome >25% em 24h → congelar deploys não essenciais, focar em confiabilidade.

Quando o orçamento se esgota → congelamento total de deploys + retrospectiva obrigatória antes de retomar trabalho em features.

---

## 8. RUM de frontend (Real User Monitoring)

Sentry Performance + Session Replay (free tier 50 replays/mês):

- Rastreamento automático de mudanças de rota, LCP/INP/CLS/TTFB.
- Session replay somente em erros (privacidade + limite do plano).
- Redagir todos os campos `input[type=password]`, `input[data-private]` (campos de CPF têm esse atributo).
- Taxa de amostragem: 10% das sessões; 100% das sessões com erros.

GA4 configurado via Google Tag Manager, disparado somente após consentimento (§08-security.md §2.6).

---

## 9. Cheatsheet de busca em logs

Queries do CloudWatch Logs Insights mantidas em `docs/runbooks/log-queries.md`:

```
# 5xx na última 1h por serviço
fields @timestamp, service, status, path, request_id
| filter status >= 500
| stats count() by service
| sort count desc

# Webhooks de pagamento lentos
fields @timestamp, path, duration_ms, psp
| filter service = "payment-service" and path like /webhooks/psp/
| stats avg(duration_ms), max(duration_ms), count() by psp

# Rastrear uma requisição de ponta a ponta por request_id
fields @timestamp, service, msg, status
| filter request_id = "abc-123"
| sort @timestamp asc
```

---

## 10. Status

| Capacidade | Status | Sprint |
|-----------|--------|--------|
| Configuração Rails lograge + JSON | ⬜ | Pré-lançamento |
| Logging zap no gateway Go | ⬜ | Pré-lançamento |
| Grupos de logs CloudWatch + retenção | ⬜ | Sprint de Infra |
| Métricas customizadas via OTel | ⬜ | Pré-lançamento |
| Traces X-Ray habilitados | ⬜ | Pré-lançamento |
| DSN do Sentry no frontend | ⬜ | Utilar 01 |
| DSN do Sentry no backend | ⬜ | Utilar 08 |
| CloudWatch Dashboards | ⬜ | Pré-lançamento |
| Alertas P0 disparando → telefone | ⬜ | Gate pré-lançamento |
| SLOs publicados + revisados | ⬜ | Gate pré-lançamento |
| Monitores UptimeRobot | ⬜ | Pré-lançamento |
