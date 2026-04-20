# ADR 005 — Resiliência de webhooks de pagamento

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

O `payment-service` (introduzido na Phase 3 / Sprint 08 conforme ADR 002) é o ponto único onde o estado do PSP cruza para o nosso sistema. Webhooks conduzem transições de pedido, repasses a vendedores, notificações ao cliente e liberação de estoque. Um webhook perdido, duplicado ou fora de ordem corrompe o estado downstream — difícil de desfazer quando os pedidos já foram despachados.

Forças em jogo:
- PSPs fazem retentativas agressivamente em respostas não-2xx (Mercado Pago retenta por até 24h com backoff exponencial; outros variam). O handler deve ser **rápido e idempotente**.
- Nossa própria publicação no Kafka pode falhar — reinício do broker Redpanda durante deploys, falhas de rede transitórias.
- Verificação de assinatura não é opcional: um webhook sem assinatura é uma confirmação de pedido forjada.
- Reinicializações de deploy não devem perder trabalho em andamento. Qualquer coisa bufferizada apenas em memória está em risco.

Dependem disto: máquina de estados do `order-service` (Phase 3), fluxo de repasse ao vendedor (Phase 5), notificações por e-mail/push ao cliente (Phase 4).

## Decisão

### Caminho quente (handler, < 2s)

1. Ler corpo bruto + header de assinatura. **Verificar HMAC-SHA256** contra o segredo do PSP (chave por PSP em `Rails.application.credentials.psp[:mercado_pago][:webhook_secret]`). Rejeitar com 401 se inválido — não vazar qual verificação falhou.
2. Parsear payload, extrair `psp_payment_id`.
3. **Verificação de idempotência**: `INSERT INTO webhook_events (psp_id, psp_payment_id, payload, received_at) VALUES (...) ON CONFLICT (psp_id, psp_payment_id, event_type) DO NOTHING RETURNING id`. Se nenhuma linha retornada, já vimos esse evento — ACK 200 imediatamente.
4. Atualizar a linha de `payments` (status, amount, paid_at) dentro de uma única transação.
5. Dentro da mesma transação, inserir em `payments_outbox` (abaixo).
6. Commit. ACK 200.
7. Uma thread em background drena `payments_outbox` → Kafka `payment.confirmed` / `payment.failed`.

Meta: latência do handler p95 < 800ms, p99 < 2s. O timer de retentativa do PSP é tipicamente 3–10s na primeira falha.

### Outbox + retentativas

- `payments_outbox(id, event_type, payload_json, published_at, attempts, next_attempt_at)` — outbox transacional clássico.
- Um drainer (job Sidekiq, rodando a cada 2s com encadeamento `perform_in`, ou uma thread dedicada) pega as linhas não publicadas e chama o Kafka.
- Retentativas: **3 tentativas, backoff exponencial 1s → 5s → 30s**.
- Após 3 falhas, a linha permanece no outbox com `attempts = 3`; alerta dispara (CloudWatch / Slack) quando `COUNT(*) WHERE attempts >= 3` > 0 por > 5 minutos.
- Um job sweeper secundário (a cada 5 min) promove linhas travadas de volta para retentativa quando a saúde do broker é restaurada.

### Diagrama de sequência

```
PSP          payment-service                       Postgres                  Redpanda
 |  POST /webhook  |                                   |                        |
 |---------------->| verify HMAC                       |                        |
 |                 | INSERT webhook_events (idem)      |                        |
 |                 |---------------------------------->|                        |
 |                 | UPDATE payments, INSERT outbox    |                        |
 |                 |---------------------------------->|                        |
 |                 | COMMIT                            |                        |
 |    200 OK       |<----------------------------------|                        |
 |<----------------|                                                            |
 |                 | drainer thread                    |                        |
 |                 | SELECT ... FROM payments_outbox   |                        |
 |                 |---------------------------------->|                        |
 |                 | publish payment.confirmed         |                        |
 |                 |------------------------------------------------------------>|
 |                 | UPDATE outbox SET published_at    |                        |
 |                 |---------------------------------->|                        |
```

### Alternativas comparadas

| Abordagem | Durabilidade no deploy | Latência do handler | Exatamente-uma-vez para Kafka | Complexidade |
|-----------|------------------------|---------------------|-------------------------------|--------------|
| **Kafka-first + outbox como fallback (escolhida)** | Total (outbox persiste) | <2s | Efetivamente (consumidor idempotente) | Média |
| Kafka-first puro, sem outbox | Nenhuma se publicação falhar após commit | <1s | Não — lacuna entre commit no DB e publicação | Baixa |
| Somente Sidekiq (sem outbox, fila de retry retém o trabalho) | Depende da persistência Redis do Sidekiq (`appendonly yes`) | <1s | Não — AOF do Redis pode ficar para trás | Baixa |
| Outbox transacional com polling (drenar por cron, sem publicação inline) | Total | <1s | Sim | Média (adiciona latência de poll ~2s) |

## Consequências

### Positivas
- **Nenhum evento de pagamento jamais é perdido** após o commit do handler — o outbox é a fonte da verdade até o Kafka confirmar
- Handler idempotente sobrevive à janela de retentativas de 24 horas do PSP sem creditamento duplo de pedidos
- A verificação de assinatura no início significa que eventos malformados/forjados nunca tocam tabelas de negócio
- O drainer pode ser reiniciado / reimplantado de forma independente; o outbox retoma de onde parou

### Negativas
- O outbox adiciona uma escrita em cada webhook — uma linha extra por evento (aceitável no nosso volume)
- O drainer é uma peça móvel: requer monitoramento (lag, linhas com falha, saúde da thread)
- A tabela `webhook_events` cresce indefinidamente a menos que particionemos ou podemos (pós-lançamento: retenção de 90 dias, depois arquivar)
- "Publicar exatamente uma vez" vira "publicar pelo menos uma vez" — consumidores devem deduplicar por `psp_payment_id + event_type`. Esse hábito já temos do consumidor Kafka no inventory-service.

### Alternativas rejeitadas
- **Somente Sidekiq com retry**: a persistência AOF do Redis não é suficientemente robusta; um reinício de pod no meio de uma retentativa pode perder jobs. Bom para trabalho best-effort, não para dinheiro.
- **Outbox transacional como padrão com drenagem somente por poll**: adiciona ~2s de latência a cada confirmação de pagamento no caso comum. Preferimos publicação inline com outbox como fallback — o melhor dos dois mundos. (O drainer somente por poll é mantido como sweeper secundário.)
- **Store-and-forward via SQS antes do payment-service**: adiciona uma dependência AWS e um segundo hop sem ganho de durabilidade que não temos já com o outbox.

## Questões em aberto

1. **Janelas de retentativa por PSP** — Mercado Pago retenta por 24h, Gerencianet ~6h, Stripe até 3 dias. A chave de idempotência de `webhook_events` deve sobreviver a todas elas. Responsável: lead do payment-service no kickoff da Sprint 08; retenção padrão 30 dias.
2. **Convenção de nomenclatura de DLQ** — `payment.confirmed.dlq` vs `dlq.payment.confirmed`? Alinhar com as convenções de tópico Kafka existentes (Phase 2 usou `order.created`). Responsável: plataforma.
3. **Limiares de alerta** — disparar quando o lag do outbox > 60s? > 100 linhas não publicadas? Precisa de uma semana de dados de baseline pós-lançamento antes de definir limiares fixos.
4. **Ferramental de replay de webhook** — expor um endpoint admin para re-executar um `psp_payment_id` específico pelo handler (útil durante resposta a incidentes)? Propenso a sim, Sprint 09.
5. **Distorção de relógio em TTLs de assinatura** — alguns PSPs incluem um timestamp no payload assinado com TTL curto. NTP em nossos hosts deve ser confiável; documentar a dependência.
