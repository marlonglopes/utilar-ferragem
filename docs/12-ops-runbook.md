# 12 — Runbook de operações

**Status**: Rascunho. **Data**: 2026-04-20.

O que fazer quando as coisas quebram. Todo alerta em [09-observability.md](09-observability.md) §6 remete a uma seção aqui.

Leituras complementares:
- [09-observability.md](09-observability.md) — dashboards + alertas
- [11-infra.md](11-infra.md) — topologia da infraestrutura
- [08-security.md](08-security.md) §2.7 — comunicação de incidentes à ANPD

---

## 1. Plantão

### 1.1 Escala atual (Fase 3)

Fundador solo de plantão. Cobertura 24/7 via:

- **Pager principal**: SMS via Twilio (roteado de CloudWatch → SNS → Lambda → Twilio).
- **Secundário**: notificação push de plantão do Sentry no app móvel.
- **Fallback**: canal Slack `#alerts-prod` (não é pager — é um dashboard).
- **Fallback de contato** durante ausência: `suporte@utilarferragem.com.br` responde automaticamente com a URL da página de status.

### 1.2 Meta da Fase 4

Quando o time tiver 2+ pessoas, migrar para Sentry Teams (plantão com escala, roteamento por horário) ou PagerDuty starter ($21/usuário/mês). O custo é aceitável quando dois engenheiros compartilham o plantão.

### 1.3 Reconhecer + escalar

Ao receber um alerta:

1. Reconhecer no Sentry / Slack em até 5 min (evita repetição de alertas).
2. Abrir a seção do runbook correspondente abaixo.
3. Se travado por >30 min, postar em `#alerts-prod` com o que foi tentado + onde está travado.
4. Se o incidente durar >2h ou afetar o checkout, atualizar a página de status (§10).

---

## 2. Severidades de incidente

| Sev | Definição | Exemplos | SLA de resposta | Comunicação |
|-----|-----------|----------|--------------|-------|
| **P0** | Bloqueia receita; checkout ou autenticação quebrado para >5% dos usuários | Webhook de pagamento fora do ar; gateway 5xx; DB indisponível | Reconhecimento em 5 min; mitigação em 30 min | Página de status + e-mail para lista de incidentes |
| **P1** | Feature importante quebrada ou degradada, existe workaround | Busca quebrada; imagens 404; produtos de um vendedor falhando | Reconhecimento em 15 min; mitigação em 2h | Página de status |
| **P2** | Menor/cosmético, ou afeta <5% dos usuários | Link de rodapé quebrado; cache stale; typo no e-mail | Próximo dia útil | Nenhuma |
| **P3** | Informativo / rastreamento | Avisos de depreciação; ruído de query lenta | Backlog | Nenhuma |

A severidade é definida pelo primeiro respondente. Eleve livremente; rebaixe somente após confirmar que o impacto ao usuário é menor do que a leitura inicial.

---

## 3. Entradas do runbook

Numeradas para referência nas descrições de alertas.

### 3.1 Webhook de pagamento travado / outage do PSP

**Sintomas**: `payment.webhook_received_total` cai a zero por >10min; `payment.success_rate` despenca; clientes relatam ter pago mas vendo "pagamento pendente".

**Diagnóstico**:

1. Verificar a [página de status do PSP](https://status.mercadopago.com/) (para MP). Se o PSP estiver fora → aguardar, comunicar.
2. Verificar logs de `/webhooks/psp/mercadopago`:
   ```
   CloudWatch Insights → /utilar/prod/payment-service
   fields @timestamp, status, msg, request_id
   | filter path like /webhooks/psp/
   | sort @timestamp desc | limit 100
   ```
3. Se 401s: falha na validação de assinatura — verificar se `PSP_WEBHOOK_SECRET` bate com o dashboard do PSP.
4. Se 500s: verificar stack trace no Sentry.

**Mitigação**:

- **PSP fora do ar**: sem ação do nosso lado; postar na página de status com o link do incidente do PSP; enviar e-mail proativo para clientes com pedidos pendentes via `rake payments:notify_pending_customers`.
- **Assinatura falhando**: rotacionar `PSP_WEBHOOK_SECRET` via Secrets Manager (§08-security §6); redeploiar o payment-service; disparar "reenvio de webhook" do PSP para os eventos afetados.
- **Nosso código 500**: reverter o payment-service para a tag anterior (§3.10); registrar pós-mortem.
- **Reprocessamento de backlog**: executar `rake payments:poll_pending` — consulta o PSP para cada pagamento com `status='pending'` mais antigo que 10min e reconcilia.

### 3.2 Lag do consumer Kafka

**Sintomas**: `kafka.consumer_lag` > 1000 em qualquer tópico por >10min.

**Diagnóstico**:

1. Qual tópico? `order.created` → inventário; `payment.confirmed` → transição de pedido.
2. O processo consumer está em execução? `docker ps | grep inventory-service` ou `... order-service`.
3. Logs do consumer:
   ```
   fields @timestamp, msg, error
   | filter service = "inventory-service" and msg like /consume/
   | sort @timestamp desc | limit 50
   ```

**Mitigação**:

- Consumer travou: reiniciar container, verificar se o lag drena.
- Consumer lento: verificar tempo de processamento p95 por mensagem; se um padrão de mensagem específico está lento, registrar bug + considerar aumentar temporariamente `fetch.max.bytes`.
- Mensagem venenosa: inspecionar o último offset processado, mover o offset para frente manualmente (`rpk group seek`), rotear a mensagem para a DLQ para inspeção.
- DLQ não vazia: triar via console do Redpanda (`http://localhost:8081` em dev; produção é fechada — usar `rpk` via SSH).

### 3.3 Pico na taxa de 5xx

**Sintomas**: `http.5xx_rate` > 1% em qualquer serviço por >2min.

**Diagnóstico**:

1. Sentry: procurar novos erros correlacionados ao release.
2. CloudWatch: qual rota?
   ```
   fields @timestamp, service, path, status
   | filter status >= 500
   | stats count() by service, path
   ```
3. Deploy recente? `git log --since="1 hour ago" --oneline`.
4. Pool de conexão DB esgotado? Verificar dashboard `db.connections_used`.

**Mitigação**:

- Deploy correlacionado: reverter (§3.10) primeiro, investigar depois.
- Pool DB esgotado: aumentar a variável de ambiente `DB_POOL`; reiniciar; registrar bug para a query que está segurando as conexões.
- Timeout de serviço downstream: verificar o serviço específico; provavelmente outra entrada do runbook se aplica.

### 3.4 Erro no CloudFront (5xx do CDN)

**Sintomas**: Clientes relatam que o site está fora do ar, mas nosso ALB/serviços estão saudáveis.

**Diagnóstico**:

1. Verificar o [AWS Health Dashboard](https://health.aws.amazon.com/) para problema regional no CloudFront.
2. Testar diretamente: `curl -I https://utilarferragem.com.br/` — qual status?
3. Logs do CloudFront no S3 (`utilar-cf-logs-prod/`): verificar distribuição de erros.
4. Bloqueio por WAF? Verificar métricas do WAF para pico em requisições bloqueadas.

**Mitigação**:

- Problema regional da AWS: postar na página de status, aguardar, comunicar.
- Problema de acesso à origem: confirmar se a política do bucket S3 não sofreu drift; reaplicar via Terraform.
- Falso positivo no WAF: revisar a regra, adicionar exceção se o tráfego for legítimo.

### 3.5 CNPJ-lookup / ViaCEP instável

**Sintomas**: Usuários no cadastro + checkout relatam falha no autofill de endereço; taxa de 5xx do `viacep.com.br` elevada.

**Diagnóstico**:

- ViaCEP e as APIs da Receita Federal são externas, gratuitas e ocasionalmente instáveis.

**Mitigação**:

- O frontend já trata a falha de forma elegante (o usuário preenche o endereço manualmente) — sem outage do nosso lado.
- Se >30% dos usuários afetados por >30min: adicionar banner "Nosso auto-preenchimento de endereço está temporariamente indisponível — você pode preencher manualmente".
- Longo prazo: cachear CEPs comuns no Redis (Fase 4).

### 3.6 Fila de aprovação de vendedor > 48h

**Sintomas**: Alerta P2 diário indica profundidade da fila > 5 com o mais antigo > 48h.

**Ação**:

- Abrir o painel admin → `Sellers → Pending` — processar cada um.
- SLA: aprovar ou solicitar mais informações em 48h. Fase 4: auto-aprovar CNPJ bem formado + endereço + 5 listagens, manter revisão manual para casos extremos.

### 3.7 Cliente relata pedido não encontrado

**Diagnóstico**:

1. Solicitar e-mail ou CPF. Consultar no user-service:
   ```sql
   SELECT id, email, created_at FROM users WHERE email = ? OR cpf = ?;
   ```
2. Verificar `orders`:
   ```sql
   SELECT id, status, total_cents, created_at FROM orders WHERE user_id = ? ORDER BY created_at DESC;
   ```
3. Verificar `payments`:
   ```sql
   SELECT id, status, method, psp_payment_id, created_at FROM payments WHERE user_id = ? ORDER BY created_at DESC;
   ```

**Casos prováveis**:

- Pagamento em `pending` mas nunca confirmado → webhook do PSP pode ter sido perdido. Executar `rake payments:reconcile[payment_id]`.
- Pedido não criado → pagamento foi criado mas a criação do pedido falhou. Verificar Sentry por erros em `POST /api/v1/orders` naquele horário.
- Usuário logado na conta errada (e-mail diferente do esperado). Confirmar com os últimos 4 do CPF.

**Resolução**: se encontrarmos dinheiro recebido mas sem pedido, creditar o cliente manualmente + registrar pós-mortem P1.

### 3.8 Chargeback recebido

Fluxo:

1. PSP notifica via webhook (evento `payment.chargeback`).
2. `payments.status = 'chargeback'`, pedidos sinalizados, vendedor notificado.
3. Verificar pedido + pagamento + prova de envio.
4. Em até 7 dias, contestar via dashboard do PSP com evidências (rastreio, entrega assinada, prova por e-mail).
5. Se chargeback válido → reembolso confirmado; marcar pedido como `refunded`; nota no dashboard do vendedor.
6. 3+ chargebacks de um vendedor em 30 dias → suspensão automática do vendedor, revisão manual.

### 3.9 Backups do banco de dados

**Estado atual (Fase 3)**: Postgres rodando em Docker no EC2 compartilhado. Backups via `pg_dump` nightly para S3 via cron:

```
0 2 * * * pg_dump $DATABASE_URL | gzip | aws s3 cp - s3://utilar-backups-prod/$(date +\%Y\%m\%d)/$DB_NAME.sql.gz
```

Retenção: 30 dias no S3 (regra de ciclo de vida move para Glacier após 7 dias, exclui após 30).

**Migração da Fase 4 para RDS**:

- Backups automáticos do RDS: habilitados, retenção de 7 dias
- Point-in-time recovery (PITR): granularidade de 5 minutos dentro da janela de retenção
- Snapshot final antes de qualquer operação destrutiva
- Multi-AZ: ligado em produção (~$60/mês extra)

### 3.10 Procedimento de rollback

#### 3.10.1 Frontend

Os últimos 10 deploys retidos em `s3://utilar-web-prod-sa-east-1-archive/<timestamp>/`.

```bash
# Encontrar o prefixo desejado
aws s3 ls s3://utilar-web-prod-sa-east-1-archive/ | tail -11

# Restaurar
aws s3 sync "s3://utilar-web-prod-sa-east-1-archive/20260420-1530-prev/" \
            "s3://utilar-web-prod-sa-east-1/"

# Invalidar
aws cloudfront create-invalidation --distribution-id $CF_DIST_ID \
  --paths "/*"
```

Leva ~2 min de ponta a ponta.

#### 3.10.2 Backend

O ECR mantém as últimas 20 imagens tagueadas por serviço.

```bash
# SSH para o servidor prod
ssh ec2-user@<prod-host>

# Rollback do payment-service para um SHA específico
cd /opt/utilar/prod
export PAYMENT_TAG=<sha-anterior>
docker-compose pull payment-service
docker-compose up -d --no-deps payment-service

# Verificar
make prod-health
```

#### 3.10.3 Rollbacks de migração

- Se o código revertido for **compatível com o novo schema** → rollback simples de container é suficiente.
- Se o novo schema quebrar o código antigo → avançar e corrigir, ou executar `bin/rails db:rollback STEP=N` manualmente (arriscado — somente se todos os writes do novo código na coluna puderem ser descartados com segurança).
- Regra de ouro: novas migrações DEVEM ser backward-compatible (adicionar coluna nullable, depois backfill, depois tornar obrigatório) para que o rollback nunca exija alterações de schema. Imposto na revisão de PR.

Blue/green é aspiracional (Fase 4+). A estratégia atual é restart rolling em um target group ALB com 2 instâncias; durante o restart, uma instância lida com o tráfego enquanto a outra faz deploy.

---

## 4. Recuperação de desastre

### 4.1 Objetivos

| Métrica | Meta | Notas |
|--------|--------|-------|
| RTO (Recovery Time Objective) | 4h | Do incidente detectado até o serviço restaurado |
| RPO (Recovery Point Objective) | 1h | Perda máxima de dados tolerada |

### 4.2 Cenários

| Cenário | Impacto | Recuperação |
|----------|--------|----------|
| Crash de serviço único | Aquela feature indisponível | Auto-restart do container; ~5min |
| Falha no host EC2 | Outage total | Subir substituto a partir do AMI; restaurar dados dos volumes Docker (sincronizados com EBS nightly); ~2h |
| Outage regional (`sa-east-1`) | Outage total | Sem failover cross-região hoje — aguardar AWS; ~conforme SLA AWS |
| Drop acidental do DB | Perda de dados | Restaurar do último `pg_dump` (≤24h); RPO de 24h aceitável para a Fase 3, mas abaixo da meta declarada de 1h — lacuna fechada quando o PITR do RDS entrar na Fase 4 |
| Ransomware / comprometimento da conta AWS | Catastrófico | Restaurar do backup offsite (clone semanal Backblaze B2 dos backups S3); recuperação ≥24h |

### 4.3 Simulacro anual de DR

Primeiro simulacro antes do lançamento; anualmente depois.

Lista de verificação:

1. Restaurar o `pg_dump` da noite anterior em um Postgres novo → verificar se as contagens de linhas batem com a produção (dentro das tolerâncias).
2. Restaurar o archive S3 anterior → verificar manualmente se o SPA carrega a partir do bucket restaurado.
3. Simular outage do PSP: bloquear o endpoint de webhook no ALB → verificar se a rake task `poll_pending` esgota o backlog.
4. Documentar os achados; corrigir o que quebrou.

---

## 5. Fluxo de aprovação de vendedores

### 5.1 Fila

Painel admin → `/admin/sellers/pending`. Exibe os mais antigos primeiro.

### 5.2 Verificações obrigatórias

| Item | Como |
|------|-----|
| CNPJ válido (Módulo 11) | Auto — validado no cadastro |
| CNPJ ativo na Receita Federal | **Manual** Fase 3; API Fase 4 |
| Razão social bate com o CNPJ | Manual — comparar nome enviado com o registro da RF |
| CEP + endereço plausível | Verificação pontual com ViaCEP |
| Ao menos 5 produtos cadastrados | Auto — bloqueado até o upload |
| Listagens não são falsificações (revisão visual) | Manual — varredura de 30s |
| Conta bancária para repasses | Manual — Fase 4 |

### 5.3 SLA

- Primeira resposta: 24h (aprovar, rejeitar ou "precisa de mais informações").
- Decisão final: 48h após o envio.
- Escalação: se bloqueado >48h, registrar ticket P2 + contatar vendedor com pedido de desculpas.

### 5.4 Motivos de rejeição (copiar e colar)

```
Olá {{seller.name}}, sua inscrição na Utilar Ferragem não pôde ser aprovada
no momento pelo motivo: {{reason}}. Você pode corrigir e reenviar a qualquer
momento em {{url}}.
```

Motivos: CNPJ inativo, suspeita de contrafação, documentos inconsistentes, categoria não suportada.

---

## 6. Resposta a fraudes

### 6.1 Disputa de cartão de crédito (chargeback)

Ver §3.8 acima.

### 6.2 Listagem falsificada

Fluxo:

1. Reclamação chega em `legal@utilarferragem.com.br` com URL do produto + prova de autoridade de marca.
2. Validar em 24h: é uma marca conhecida? O preço está absurdamente baixo?
3. Se plausível: **ocultar a listagem imediatamente** (`status='hidden'`), notificar vendedor com a reclamação + 72h para responder.
4. Se o vendedor confirmar ou não responder: excluir listagem, documentar na tabela `admin_actions`.
5. Se o vendedor contestar: escalar para revisão jurídica.
6. Reincidente (2+ listagens falsificadas): suspender vendedor automaticamente.

### 6.3 Rede de fraude em pagamentos

Sinais agregados diariamente via consulta SQL (ou Fase 4: alerta automatizado):

- Mesmo cartão → 5+ contas diferentes em 7 dias
- Mesmo IP → 10+ novas contas em 24h
- Mesmo endereço de entrega → 5+ CPFs de compradores diferentes em 30 dias
- Limites de velocidade disparados repetidamente na mesma conta

Ação: congelar as contas afetadas enquanto aguarda revisão. Contatar o PSP se o padrão sugerir uso de cartão roubado.

---

## 7. Solicitações de titulares de dados (LGPD)

Detalhado em [08-security.md](08-security.md) §2.4 / §2.5. Lista de verificação operacional:

| Tipo de solicitação | SLA | Responsável | Canal |
|--------------|-----|-------|---------|
| Acesso a dados (artigo 18 VII) | 15 dias | Fundador | `dpo@utilarferragem.com.br` |
| Exportação de dados (portabilidade) | 15 dias | Automatizado via endpoint | Botão no app → e-mail com URL assinada |
| Exclusão | 15 dias | Automatizado via endpoint | Botão no app → carência de 15 dias |
| Correção | 15 dias | Fundador | Ticket de suporte |
| Opt-out de marketing | imediato | Automatizado | Link de cancelamento + toggle no app |

Consulta da ANPD: responder dentro do prazo estipulado (geralmente 15 dias). Registrar em `legal/anpd-inquiries.md`.

Vazamento de dados → ANPD + usuários afetados em até 72h após a detecção.

---

## 8. Página de status

### 8.1 Escolha

**Fase 3**: página HTML estática em `status.utilarferragem.com.br` (S3 + CloudFront, separado do site principal para permanecer no ar quando o principal estiver fora). Atualizado editando um arquivo JSON + redeploy.

**Fase 4**: plano Starter do statuspage.io ($29/mês) quando quisermos postagem automática de incidentes + e-mails para assinantes.

### 8.2 Componentes a monitorar

| Componente | Monitor |
|-----------|---------|
| Site (SPA) | Check HTTP UptimeRobot `https://utilarferragem.com.br/` |
| API | Check HTTP UptimeRobot `https://api.utilarferragem.com.br/health` |
| Checkout | Sintético — Playwright smoke `@smoke checkout` a cada 15min |
| Busca | HTTP UptimeRobot `GET /api/v1/marketplace/products?q=furadeira` |
| Provedor de pagamento | Embutir RSS ou iframe de status do MP |

### 8.3 Template de incidente

```markdown
**{severidade} — {título curto}** · {horário de início BRT}

Impacto: {quem é afetado e como}

Status:
- {horário}: Investigando
- {horário}: Identificado — {causa raiz}
- {horário}: Monitorando — {mitigação aplicada}
- {horário}: Resolvido

Pedimos desculpas pelo inconveniente.
```

Publicar todo incidente P0 + P1. Não publicar P2 / P3.

---

## 9. Checklist de onboarding de engenheiro (para futuras contratações)

Não é prioridade da Fase 3, mas manter um registro — primeiro dia:

- [ ] Usuário IAM AWS com a role `utilar-engineer`
- [ ] Convite para a org GitHub + 2FA obrigatório
- [ ] Acesso ao console Sentry, CloudWatch, Route53
- [ ] Workspace Slack + notificações de `#alerts-prod` desligadas por padrão (somente plantão)
- [ ] Leitura obrigatória: CLAUDE.md, este runbook, [08-security.md](08-security.md)
- [ ] Participar de um incidente menor nas primeiras 2 semanas
- [ ] Primeiro PR deve tocar somente testes (rampa de entrada)

Offboarding: revogar todos os acessos acima em 24h; rotacionar todo segredo que o engenheiro tinha acesso em até 72h; transferir a propriedade de qualquer incidente em andamento.

---

## 10. Dívida técnica conhecida que impacta as operações

| Dívida | Impacto | Correção planejada |
|------|--------|-------------|
| Postgres em Docker (não RDS) | Backup é `pg_dump` nightly, não PITR; RPO de 24h > meta declarada de 1h | Migração para RDS na Fase 4 |
| Host EC2 único | Sem HA para a camada de aplicação; janela de restart de 5min | Auto Scaling Group na Fase 4 |
| Deploys manuais em produção com aprovação solo | Risco de deploys apressados | Fase 4 quando time = 2+, exigir segundo aprovador |
| Sem verificação automatizada de CNPJ | Fila de aprovação é manual | Fase 4 API Receita Federal |
| Página de status estática | Sem e-mails para assinantes em incidente | Fase 4 statuspage.io |
| Sem failover cross-região | Outage regional = outage total | Fase 5+ |

---

## 11. Caminho de escalação para situação desconhecida

Quando nada aqui se encaixa:

1. **Estancar o sangramento**: é possível reverter ou desabilitar a feature? Faça isso primeiro.
2. **Buscar um segundo par de olhos**: acionar qualquer pessoa disponível via DMs no Slack.
3. **Documentar em tempo real**: abrir um Google Doc com o título `Incidente {AAAA-MM-DD} — {título curto}` e colar linhas de log, comandos executados, hipóteses — mesmo que ninguém esteja lendo. O eu-do-futuro vai agradecer.
4. **Pós-mortem em até 5 dias**: sem culpa, template em `docs/incidents/TEMPLATE.md`. Compartilhar com o DPO se dados pessoais estiverem envolvidos.
