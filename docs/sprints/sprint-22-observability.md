# Sprint 22 — Observabilidade em produção (logs, métricas, alertas)

**Fase**: 3 — Commerce (gate pré-lançamento). **Estimativa**: 5–7 dias.

## Escopo

Implementar o plano de observabilidade descrito em [09-observability.md](../09-observability.md). Três pilares, todos ativos antes do lançamento: **logs JSON estruturados** com redação de PII em todos os serviços Rails + gateway Go; **Sentry** no frontend e backend com DSNs por ambiente e contexto de usuário; **métricas pelo método RED** (taxa / erros / duração) expostas via endpoints Prometheus e coletadas no CloudWatch ou Grafana.

Os alertas são mínimos, mas reais: Sentry → SMS via Twilio (celular do fundador) em condições críticas (taxa de sucesso de pagamento < 95% por 5 min, taxa de 5xx > 2% por 5 min, pico na profundidade da fila). A cultura de plantão é explicitamente evitada até que o volume justifique; o runbook [infrastructure/README.md](../../infrastructure/README.md) documenta o processo de escalonamento.

## Tarefas

### Serviços Rails — logging estruturado
1. Adicionar a gem `lograge` em todos os 4 serviços Rails + payment-service; configurar `Lograge.formatter = Lograge::Formatters::Json.new`
2. Payload customizado: `request_id`, `user_id`, `seller_id`, `duration_ms`, `db_runtime`, `view_runtime`, `controller#action`, `status`
3. Middleware de redação de PII (`app/middleware/pii_redactor.rb`): antes de logar, ocultar `password`, `cpf`, `cnpj` (parcial: manter os 2 últimos), `card_number`, `cvv`, `email` (parcial: manter o domínio). Com testes unitários para uma fixture de 20 padrões de PII conhecidos.
4. `Rails.logger.formatter` escreve para stdout em JSON (convenção de logs de container); o agente CloudWatch acompanha os logs do container e envia para os grupos `/utilar/{service-name}`

### Gateway Go — logging + request_id
5. Substituir o logger stdlib por `zerolog` (ou `slog`) configurado para saída JSON
6. Middleware: gerar `X-Request-Id` se ausente, propagá-lo para todos os serviços backend via header, logar no início e no fim de cada requisição com status + duração
7. Os serviços Rails capturam `X-Request-Id` e incluem em cada linha de log — as traces se cosuram de forma limpa entre os serviços

### Sentry — frontend + backend
8. SDK do Sentry em `utilar-web/src/main.tsx`: DSN por ambiente (`VITE_SENTRY_DSN_STAGING`, `VITE_SENTRY_DSN_PROD`); `beforeSend` oculta chaves de PII conhecidas; tag `release` lida de `__BUILD_ID__`
9. `Sentry.setUser({ id, email })` no login; `Sentry.setUser(null)` no logout — o e-mail é redatado no lado do servidor durante a ingestão conforme as configurações do projeto no Sentry
10. Sentry em cada serviço Rails + gateway (sentry-ruby, sentry-rails, sentry-go); `before_send` oculta PII; `traces_sample_rate: 0.1` em produção, 1.0 em staging
11. Source maps enviados no deploy (`@sentry/vite-plugin`); verificar se um erro frontend exibe uma stack trace decodificada

### Métricas — Prometheus
12. Gem `prometheus_exporter` em cada serviço Rails; endpoint `/metrics` exposto em uma porta lateral (não pública)
13. Instrumentar: histogramas de contagem/duração de requisições HTTP (automático); gauges customizados para profundidade da fila (lag do consumer Redpanda) e uso do connection pool do ActiveRecord
14. Contadores de negócio customizados: `payment_success_total`, `payment_failure_total`, `order_stuck_total`, `push_sent_total`, `push_failed_total`
15. Gateway expõe `/metrics` com histograma de latência por rota
16. Configuração de coleta: executar um Prometheus pequeno no compose de produção + enviar para CloudWatch via `cloudwatch-exporter`, ou usar o tier gratuito do Grafana Cloud (escolher o que iniciar mais rápido — padrão para CloudWatch por ser AWS-nativo)

### Alertas
17. Regras de alerta no Sentry: novo tipo de erro em produção > 10 eventos em 5 min, 500s no payment-service > 5 em 5 min
18. Webhook Sentry → Twilio (`services/alert-bridge/`, serviço Go pequeno ou Lambda): recebe o webhook do Sentry, envia SMS para a variável de ambiente `ONCALL_PHONE`
19. Plano de teste: `curl` forçando um 500 no sandbox do payment-service → evento Sentry → SMS chega em até 60s
20. Entradas no runbook em `infrastructure/README.md`: "Sucesso de pagamento < 95%", "Pico de 5xx", "Lag Kafka > 10k" — cada uma com os 3 primeiros passos a executar

### Dashboards
21. Dashboard CloudWatch `utilar-red`: para cada serviço (gateway, user-service, product-service, order-service, payment-service) exibir taxa de requisições, taxa de erros, latência p50/p95/p99
22. Dashboard separado `utilar-business`: taxa de sucesso de pagamento (rolling 5 min), pedidos/hora, vendedores ativos, tamanho do catálogo
23. Vincular ambos os dashboards no runbook

## Critérios de aceite

- [ ] Toda requisição rastreável nos logs de gateway → serviço pelo `request_id` (pesquisável com grep)
- [ ] Sentry captura um erro frontend com contexto de usuário (e-mail redatado) e stack trace decodificada
- [ ] Middleware de redação de PII tem testes unitários cobrindo password, CPF, CNPJ, cartão, e-mail — todos passando
- [ ] Endpoint `/metrics` em cada serviço retorna formato válido de Prometheus
- [ ] Forçar um 500 no payment-service em staging → alerta SMS chega no celular do fundador em até 60s
- [ ] Dashboard CloudWatch `utilar-red` renderiza dados ao vivo de todos os 5 serviços
- [ ] `grep request_id=abc123 /var/log/utilar/*.log` retorna entradas de cada serviço que tratou a requisição
- [ ] Nenhum dado de PII (CPF, cartão, senha) aparece em nenhuma linha de log em uma revisão red-team de 100 requisições de amostra

## Dependências

- Sprints 06–09 concluídos (stack de commerce ativa — nada útil a observar antes disso)
- Contas no Sentry + Twilio; agente CloudWatch configurado nos hosts
- Sprint 23 CI/CD preferido para que DSNs fluam via secrets de GitHub Environments — aceitável usar Parameter Store por ambiente até então

## Riscos

- Fadiga de alertas — começar com apenas 3 alertas. Adicionar mais somente após uma semana sem falsos positivos.
- Vazamento de PII nos logs — a redação deve ter testes unitários; fazer uma revisão red-team antes do lançamento onde o fundador tenta ativamente vazar dados e faz grep para localizá-los.
- Estouro da cota do Sentry — manter `traces_sample_rate` baixo em produção; definir um limite mensal nas configurações do projeto no Sentry para evitar surpresas na fatura.
