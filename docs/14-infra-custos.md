# 14 — Infraestrutura Mínima e Custos

**Documento de referência para colocar a Utilar Ferragem em produção com o menor custo possível, mantendo confiabilidade suficiente para o primeiro ano de operação.**

> Câmbio de referência utilizado neste documento: **R$ 5,20 / USD** (ajuste conforme o dia do contrato).  
> Todos os valores AWS referem-se à região **sa-east-1 (São Paulo)** salvo indicação contrária.

---

## Índice

1. [Resumo executivo](#1-resumo-executivo)
2. [Visão geral das fases](#2-visão-geral-das-fases)
3. [Domínio e DNS](#3-domínio-e-dns)
4. [Infraestrutura AWS](#4-infraestrutura-aws)
5. [Serviço de e-mail (SES)](#5-serviço-de-e-mail-ses)
6. [Gateway de pagamento — Mercado Pago](#6-gateway-de-pagamento--mercado-pago)
7. [Observabilidade (gratuita)](#7-observabilidade-gratuita)
8. [Segurança e segredos](#8-segurança-e-segredos)
9. [Custo total por fase](#9-custo-total-por-fase)
10. [Checklist de ativação](#10-checklist-de-ativação)
11. [Decisões adiadas](#11-decisões-adiadas)

---

## 1. Resumo executivo

| Fase | Período | Foco | Custo estimado/mês |
|------|---------|------|--------------------|
| **Fase 1 — MVP** | Meses 1–3 | Um servidor, free tier AWS, Pix funcionando | **R$ 180–280** |
| **Fase 2 — Estabilização** | Meses 4–8 | Separar banco, Redis, backups automáticos | **R$ 450–650** |
| **Fase 3 — Escala inicial** | Meses 9–18 | Auto-scaling, CDN de imagens, observabilidade avançada | **R$ 900–1.400** |

A estratégia é simples: começar com quase zero usando o **free tier da AWS durante 12 meses**, avançar de forma incremental e só adicionar custo quando o tráfego justificar. O único custo inevitável desde o Dia 1 é o domínio .com.br e o e-mail transacional.

O gateway de pagamento (Mercado Pago) **não tem mensalidade** — cobra apenas percentual por transação, então o custo é proporcional à receita.

---

## 2. Visão geral das fases

### Fase 1 — MVP (Meses 1–3)

**Objetivo:** Marketplace no ar, catálogo real, clientes podem comprar via Pix.

```
Internet
   │
   ▼
Elastic IP  ──►  EC2 t3.small  (Nginx + todos os serviços Docker)
                      │
                      ├── gateway (Go, porta 8080)
                      ├── user-service (Rails, 3001)
                      ├── product-service (Rails, 3002)
                      ├── order-service (Rails, 3004)
                      ├── payment-service (Rails, 3005)
                      ├── PostgreSQL 15 (local, porta 5432)
                      └── Redis 7 (local, porta 6379)

S3 + CloudFront  ──►  SPA React (Utilar Ferragem)
SES              ──►  E-mails transacionais
Mercado Pago     ──►  Pix + boleto + cartão
```

**Compromisso:** sem alta disponibilidade, sem auto-scaling. Downtime planejado para deploy (~2 min). Aceitável para MVP.

---

### Fase 2 — Estabilização (Meses 4–8)

**Objetivo:** Banco de dados gerenciado, backups automáticos, Redis separado, deploy sem downtime.

```
Internet
   │
   ▼
ALB (porta 443)  ──►  EC2 t3.small  (Docker Compose — apenas serviços Rails/Go)
                           │
                      RDS db.t3.micro  (PostgreSQL gerenciado, Multi-AZ na Fase 3)
                      ElastiCache t3.micro  (Redis gerenciado)
```

**Por que adiar o RDS?** O free tier do RDS db.t3.micro dura 12 meses. Ao entrar na Fase 2, os 12 meses de free tier provavelmente ainda estão ativos — avalie antes de migrar.

---

### Fase 3 — Escala inicial (Meses 9–18)

**Objetivo:** Suportar crescimento de tráfego, separar serviços críticos, observabilidade profissional.

```
CloudFront  ──►  ALB  ──►  ECS Fargate (gateway + serviços críticos)
                            RDS db.t3.medium  (Multi-AZ)
                            ElastiCache t3.small
                            S3  (imagens de produto com CloudFront dedicado)
                            MSK (Kafka gerenciado) ou Redpanda Cloud free tier
```

---

## 3. Domínio e DNS

### 3.1 Domínio principal — `.com.br` via Registro.br

O domínio **`.com.br`** é gerenciado pelo **Registro.br** (NIC.br), entidade oficial brasileira. **Não registre via GoDaddy** para `.com.br` — o preço é 3× mais caro e o suporte é mais lento.

| Item | Provedor | Custo |
|------|----------|-------|
| `utilarferragem.com.br` | [Registro.br](https://registro.br) | **R$ 40,00/ano** |
| `utilarferragem.com` (opcional, redirect) | Registro.br ou GoDaddy | R$ 60–80/ano |
| Total domínios | — | **R$ 40–120/ano ≈ R$ 3–10/mês** |

**Como registrar no Registro.br:**
1. Criar conta pessoa jurídica em registro.br (exige CNPJ)
2. Buscar `utilarferragem.com.br`
3. Pagar via Pix ou boleto — liberado em minutos

> **Dica:** registre também `utilar.com.br` se disponível — é mais curto e vale o R$ 40.

---

### 3.2 DNS — Amazon Route 53

Usar o Route 53 junto com os outros serviços AWS simplifica muito a gestão (certificados ACM automáticos, health checks, etc.).

| Item | Custo |
|------|-------|
| Zona hospedada (`utilarferragem.com.br`) | **US$ 0,50/mês ≈ R$ 2,60** |
| Consultas DNS (primeiros 1 bilhão/mês) | US$ 0,40 por milhão ≈ **< R$ 1/mês** no início |
| **Total Route 53** | **≈ R$ 3–5/mês** |

**Registros DNS necessários no Dia 1:**

```
# SPA (CloudFront)
utilarferragem.com.br        A    → CloudFront distribution (Alias)
www.utilarferragem.com.br    CNAME → utilarferragem.com.br

# API Gateway
api.utilarferragem.com.br    A    → EC2 Elastic IP (Fase 1) ou ALB (Fase 2+)

# E-mail (SES — ver §5)
mail.utilarferragem.com.br   MX   → SES SMTP
_dmarc.utilarferragem.com.br TXT  → "v=DMARC1; p=quarantine; rua=mailto:dmarc@utilarferragem.com.br"
                             TXT  → SPF: "v=spf1 include:amazonses.com ~all"
                             CNAME → DKIM (3 registros gerados pelo SES)

# Webhook do Mercado Pago
(aponta para api.utilarferragem.com.br — mesmo endpoint)
```

---

### 3.3 SSL/TLS — AWS Certificate Manager (ACM)

**Custo: R$ 0,00.** O ACM emite e renova certificados SSL automaticamente para domínios usados com CloudFront, ALB ou API Gateway.

Certificados a criar:
- `utilarferragem.com.br` + `*.utilarferragem.com.br` (wildcard cobre todos os subdomínios)

> **Atenção:** certificados para uso com CloudFront **devem ser criados na região `us-east-1`** (não `sa-east-1`). Para ALB, criar em `sa-east-1`.

---

## 4. Infraestrutura AWS

### 4.1 Free Tier — O que está disponível nos primeiros 12 meses

| Serviço | Free Tier | Observação |
|---------|-----------|------------|
| EC2 | 750h/mês de t2.micro ou t3.micro | Use `t3.micro` — melhor performance |
| RDS | 750h/mês de db.t3.micro + 20 GB storage | PostgreSQL 15 |
| S3 | 5 GB storage + 20.000 GET + 2.000 PUT/mês | Suficiente para SPA |
| CloudFront | 1 TB de transferência + 10M requisições/mês | Suficiente para MVP |
| SES | 62.000 e-mails/mês (enviados de EC2) | Mais que suficiente |
| SNS | 1M publicações/mês | — |
| CloudWatch | 10 métricas + 5 GB logs + 3 dashboards | Suficiente para MVP |
| Secrets Manager | **Sem free tier** | US$ 0,40/segredo/mês |
| ALB | **Sem free tier** | Evitar na Fase 1 |
| ElastiCache | **Sem free tier** | Redis no EC2 na Fase 1 |

---

### 4.2 EC2 — Servidor de aplicação

#### Fase 1: t3.small (um único servidor)

O `t3.micro` do free tier tem apenas 1 GB de RAM — insuficiente para rodar 5 serviços Rails + Go + PostgreSQL + Redis. Use **t3.small** desde o início.

| Instância | vCPU | RAM | Custo (sa-east-1) |
|-----------|------|-----|-------------------|
| t3.micro *(free tier)* | 2 | 1 GB | **R$ 0/mês** (12 meses) → R$ 42/mês |
| **t3.small** *(recomendado Fase 1)* | 2 | 2 GB | **R$ 84/mês** |
| t3.medium *(Fase 2 se necessário)* | 2 | 4 GB | R$ 168/mês |

> **Estratégia de free tier:** nos primeiros meses, use o t3.micro **gratuitamente** para o banco de dados (RDS) e pague apenas o t3.small para a aplicação. Economiza ~R$ 86/mês durante 12 meses.

**Configuração do EC2 (Fase 1):**
```
OS: Ubuntu 24.04 LTS
Disco: 30 GB gp3 (incluso nos ~R$7/mês; 8 GB é free tier)
Elastic IP: R$ 0 (gratuito enquanto associado a instância ligada)
Security Group:
  - 22 (SSH): apenas seu IP
  - 80 (HTTP): 0.0.0.0/0 → redirect para 443
  - 443 (HTTPS): 0.0.0.0/0
  - 8080 (gateway): 0.0.0.0/0 (ou apenas do CloudFront IP ranges)
```

**Custo de storage EBS:**

| Volume | Tipo | Tamanho | Custo |
|--------|------|---------|-------|
| Root do servidor | gp3 | 30 GB | **R$ 8/mês** |
| *Free tier inclui 30 GB gp2* | — | — | *R$ 0 nos 12 meses* |

---

### 4.3 RDS — Banco de dados PostgreSQL

#### Fase 1: PostgreSQL no próprio EC2 (R$ 0)

Para MVP, rodar o Postgres dentro do Docker no mesmo EC2. Simples, sem custo extra.

#### Fase 2: RDS db.t3.micro (free tier ainda ativo ou ~R$ 86/mês)

| Instância | RAM | Storage | Custo (sa-east-1) |
|-----------|-----|---------|-------------------|
| db.t3.micro *(free tier 12 meses)* | 1 GB | 20 GB | **R$ 0** → R$ 86/mês |
| db.t3.small | 2 GB | — | R$ 172/mês |
| db.t3.medium *(Fase 3)* | 4 GB | — | R$ 344/mês |

**Extras RDS:**
- Backup automático (7 dias): incluso
- Snapshot manual: R$ 0,115/GB/mês (planejar ~R$ 3/mês)
- Multi-AZ (alta disponibilidade): dobra o custo — adiar para Fase 3

---

### 4.4 S3 + CloudFront — SPA e imagens

#### SPA React (Utilar Ferragem)

| Item | Free Tier | Após free tier |
|------|-----------|---------------|
| S3 storage (SPA ~5 MB) | 5 GB grátis | US$ 0,023/GB = **R$ 0,06/mês** |
| CloudFront (transferência) | 1 TB/mês grátis | US$ 0,085/GB |
| CloudFront (requisições) | 10M/mês grátis | US$ 0,0090/10k |
| **Custo real para MVP** | — | **≈ R$ 0–5/mês** |

#### Imagens de produto

Na Fase 1, imagens ficam no mesmo bucket S3. Estimar:
- 500 produtos × 4 fotos × 500 KB média = ~1 GB
- Custo: **R$ 0** (dentro do free tier)

**Configuração do bucket S3 para SPA:**
```
Bucket: utilarferragem-spa
Region: sa-east-1
Static website hosting: sim
Block public access: NÃO (arquivos públicos)
CloudFront Origin: S3 com OAC (Origin Access Control)
Cache policy: CachingOptimized (1 ano para assets com hash, 5 min para index.html)
```

---

### 4.5 ALB — Load Balancer (Fase 2+)

Na Fase 1, o Nginx no EC2 faz o papel do load balancer. Adicionar ALB na Fase 2 para:
- SSL termination gerenciada
- Health checks automáticos
- Zero-downtime deploy (conexões drenadas suavemente)

| Item | Custo (sa-east-1) |
|------|-------------------|
| ALB fixo | US$ 0,016/h = **R$ 4,16/h = ~R$ 60/mês** |
| LCU (Loadbalancer Capacity Units) | US$ 0,008/LCU/h — tráfego baixo ≈ R$ 20/mês |
| **Total ALB** | **≈ R$ 80/mês** |

> Não há free tier para ALB. Por isso adiamos para Fase 2.

---

### 4.6 ElastiCache — Redis (Fase 2+)

Na Fase 1, Redis roda no Docker no mesmo EC2. Na Fase 2:

| Instância | RAM | Custo (sa-east-1) |
|-----------|-----|-------------------|
| cache.t3.micro | 0,5 GB | **R$ 42/mês** |
| cache.t3.small | 1,4 GB | R$ 84/mês |

---

### 4.7 Outros serviços AWS

| Serviço | Uso | Custo estimado |
|---------|-----|----------------|
| **Elastic IP** | IP fixo para o EC2 | R$ 0 (grátis enquanto associado) |
| **Secrets Manager** | JWT_SECRET, PSP_API_KEY, DB_PASSWORD (4 segredos) | R$ 8/mês |
| **CloudWatch Logs** | Logs dos serviços (retém 7 dias) | R$ 0 (free tier 5 GB) |
| **CloudWatch Alarms** | 10 alarmes básicos | R$ 0 (free tier 10 alarmes) |
| **SNS** | Notificações de alarme por e-mail | R$ 0 (free tier) |
| **ACM SSL** | Certificados para CloudFront + ALB | **R$ 0** |
| **IAM** | Usuários, roles, políticas | **R$ 0** |
| **VPC** | Rede privada padrão | **R$ 0** |
| **Data transfer** | Saída para internet (~10 GB/mês) | R$ 5/mês |

---

## 5. Serviço de E-mail (SES)

### Por que AWS SES?

- **Custo mais baixo do mercado** para e-mail transacional
- Integração nativa com a infraestrutura AWS já existente
- Reputação de entrega excelente para domínios verificados
- Suporta DKIM, SPF, DMARC nativamente

### Preço

| Origem do envio | Volume | Custo |
|-----------------|--------|-------|
| Enviado **de uma instância EC2** | Primeiros 62.000 e-mails/mês | **R$ 0,00** |
| Enviado de EC2 | Acima de 62.000/mês | US$ 0,10 / 1.000 = **R$ 0,52/1.000** |
| Enviado de fora da AWS | Qualquer volume | US$ 0,10 / 1.000 |
| Anexos | Por GB | US$ 0,12/GB ≈ R$ 0,62/GB |

**Para MVP:** praticamente **R$ 0/mês** — até 62k e-mails mensais são gratuitos.

### Templates de e-mail necessários (Fase 1)

| Template | Gatilho | Idioma |
|----------|---------|--------|
| Boas-vindas | Cadastro de cliente | pt-BR + en |
| Confirmação de pedido | `order.created` | pt-BR |
| Pagamento confirmado | `payment.confirmed` | pt-BR |
| Pedido enviado | Status → `shipped` | pt-BR |
| Redefinição de senha | Solicitação pelo usuário | pt-BR + en |
| Novo pedido (vendedor) | `order.created` | pt-BR |

### Setup do SES (passo a passo)

```
1. Criar identidade de domínio: utilarferragem.com.br
2. Aguardar verificação DNS (DKIM — 3 registros CNAME no Route 53)
3. Configurar SPF e DMARC no Route 53
4. Testar envio (sandbox: apenas e-mails verificados recebem)
5. Solicitar saída do sandbox via AWS Support (gratuito, 1–2 dias úteis)
   → Justificar: "marketplace transacional, estimativa inicial de 500 e-mails/mês"
6. Configurar supressão de hard bounces (SES faz automaticamente)
7. Monitorar taxa de rejeição < 0,1% e reclamações < 0,1%
```

> **Atenção LGPD:** e-mails transacionais (confirmação de pedido, rastreamento) não precisam de opt-in. E-mails de marketing (promoções, cashback) precisam de consentimento explícito — implemente o banner de consentimento antes de enviar qualquer marketing.

---

## 6. Gateway de Pagamento — Mercado Pago

### Por que Mercado Pago?

- **Sem mensalidade** — custo zero se não houver vendas
- Maior cobertura de Pix no Brasil
- SDK para Rails e React bem documentado
- Suporte a parcelamento e antecipação de recebíveis
- Ambiente sandbox gratuito para desenvolvimento

### Estrutura de taxas (atualizada 2026)

| Método | Taxa | Observações |
|--------|------|-------------|
| **Pix** | 0,99% por transação | Recebe em 1 dia útil |
| **Boleto bancário** | R$ 3,49 por boleto | Vence em 3 dias; sem estorno |
| **Cartão de crédito 1×** | 4,98% | Recebe em 14 dias úteis |
| **Cartão 2–6×** | 5,98% | Recebe em 14 dias (1ª parcela) |
| **Cartão 7–12×** | 6,98% | — |
| **Antecipação de recebíveis** | 1,99% a.m. sobre o valor antecipado | Opcional |
| **Estorno (chargeback)** | Sem custo adicional (valor devolvido) | Contestação via painel |
| **Mensalidade** | **R$ 0,00** | — |

### Simulação de custo para vendedor (exemplo)

```
Pedido de R$ 300,00 via Pix:
  Taxa Mercado Pago: R$ 300 × 0,99% = R$ 2,97
  Repasse ao vendedor: R$ 297,03  (no dia seguinte)

Pedido de R$ 300,00 via cartão 1×:
  Taxa Mercado Pago: R$ 300 × 4,98% = R$ 14,94
  Repasse ao vendedor: R$ 285,06  (14 dias úteis)
  → Com antecipação: R$ 285,06 × (1 - 1,99%) = R$ 279,39 (no dia seguinte)
```

### Conta necessária

| Tipo | Requisito | Custo |
|------|-----------|-------|
| Conta Mercado Pago Empresas | CNPJ + contrato social + dados bancários PJ | **R$ 0** |
| Credenciais de sandbox | Automático ao criar conta de dev | **R$ 0** |
| Certificação PCI | Mercado Pago é responsável (SAQ-A) | **R$ 0** |

### Variáveis de ambiente (já mapeadas no `06-integration.md`)

```env
PSP_ENVIRONMENT=sandbox        # → production no go-live
PSP_API_KEY=APP_USR-...        # Access token (segredo — nunca comitar)
PSP_WEBHOOK_SECRET=...         # HMAC para verificação de webhook
PAYMENT_SERVICE_URL=http://payment-service:3005
```

### Webhooks do Mercado Pago

O Mercado Pago envia notificações para `POST /webhooks/psp/mercadopago`. Eventos relevantes:

| Evento | Ação no sistema |
|--------|----------------|
| `payment.created` | Registrar pagamento pendente |
| `payment.approved` | Publicar `payment.confirmed` → order.paid |
| `payment.rejected` | Publicar `payment.failed` → order.cancelled |
| `payment.refunded` | Publicar `payment.refunded` → reembolso |
| `chargebacks.created` | Alertar admin — abrir disputa |

---

## 7. Observabilidade (Gratuita)

Toda a stack de observabilidade do MVP é **R$ 0/mês** usando free tiers.

### 7.1 Sentry — Rastreamento de erros

| Plano | Preço | Inclui |
|-------|-------|--------|
| **Developer (gratuito)** | **R$ 0/mês** | 5.000 erros/mês · 10.000 transações de performance · 1 projeto · 1 usuário |
| Team | US$ 26/mês ≈ R$ 135 | 50k erros · projetos ilimitados · alertas avançados |

**Para MVP:** plano gratuito é suficiente. Upgrade quando erros ultrapassarem 5k/mês (sinal de crescimento real).

```
DSNs a configurar:
- utilar-frontend  (React SPA)
- utilar-gateway   (Go)
- utilar-services  (compartilhado entre Rails services)
```

### 7.2 Grafana Cloud — Métricas e logs

| Plano | Preço | Inclui |
|-------|-------|--------|
| **Free** | **R$ 0/mês** | 10k métricas ativas · 50 GB logs/mês · 50 GB traces/mês · 14 dias retenção |
| Pro | US$ 299/mês | Escala sob demanda |

**Para MVP:** free tier cobre 100% das necessidades do primeiro ano.

```
Integração:
- Instalar Grafana Agent no EC2 (coleta métricas, logs, traces)
- Rails services: gem prometheus_exporter → porta 9394
- Go gateway: exposição de métricas Prometheus nativa
- Logs: envio via Loki (incluído no Grafana Agent)
```

### 7.3 UptimeRobot — Monitoramento de disponibilidade

| Plano | Preço | Inclui |
|-------|-------|--------|
| **Free** | **R$ 0/mês** | 50 monitores · verificação a cada 5 min · alertas por e-mail |
| Pro | US$ 9/mês ≈ R$ 47 | Verificação a cada 1 min · alertas SMS |

**Monitores a configurar (Fase 1):**

| Monitor | URL | Alerta |
|---------|-----|--------|
| SPA home | `https://utilarferragem.com.br` | E-mail imediato |
| API health | `https://api.utilarferragem.com.br/health` | E-mail imediato |
| Checkout | `https://utilarferragem.com.br/checkout` | E-mail imediato |
| Webhook PSP | `https://api.utilarferragem.com.br/webhooks/psp/mercadopago` (POST) | E-mail |

### 7.4 CloudWatch — Alarmes básicos (AWS free tier)

10 alarmes são gratuitos. Configurar:

| Alarme | Métrica | Threshold |
|--------|---------|-----------|
| CPU alta | EC2 CPUUtilization | > 80% por 5 min |
| Disco cheio | EC2 disk_used_percent | > 85% |
| Memória alta | EC2 mem_used_percent (via CloudWatch Agent) | > 90% |
| Erros 5xx | (via log group) | > 10 em 5 min |
| Latência alta | (via log group) | p95 > 3s |

---

## 8. Segurança e Segredos

### 8.1 AWS Secrets Manager

| Segredo | Valor estimado | Custo/mês |
|---------|---------------|-----------|
| `JWT_SECRET` | String 256-bit random | R$ 2,08 |
| `PSP_API_KEY` | Mercado Pago access token | R$ 2,08 |
| `PSP_WEBHOOK_SECRET` | HMAC key 256-bit | R$ 2,08 |
| `DB_PASSWORD` | Senha RDS (Fase 2) | R$ 2,08 |
| **Total (4 segredos)** | — | **R$ 8,32/mês** |

> **Alternativa gratuita para MVP:** usar `.env` encriptado no EC2 com permissão de arquivo `600` + IAM role no EC2 sem acesso a Secrets Manager. Funciona, mas menos seguro. Migrar para Secrets Manager antes do go-live público.

### 8.2 IAM — Princípio do menor privilégio

```
Perfis IAM a criar:
1. utilar-ec2-role          → S3 get/put, SES send, CloudWatch logs, Secrets Manager get
2. utilar-deploy-user       → S3 sync (apenas bucket SPA), sem acesso a produção
3. utilar-admin-user        → MFA obrigatório, acesso completo (apenas humanos)

Política de senhas IAM:
- Mínimo 16 caracteres
- MFA obrigatório para todas as contas humanas
- Rotação a cada 90 dias
```

### 8.3 Backup

| Item | Estratégia | Frequência | Retenção | Custo |
|------|-----------|------------|----------|-------|
| PostgreSQL (Fase 1) | `pg_dump` → S3 | Diário às 3h | 7 dias | R$ 0,10/mês |
| RDS (Fase 2+) | Backup automático | Diário | 7 dias | Incluso |
| Código | GitHub (já existe) | A cada push | Infinito | R$ 0 |
| Variáveis de env | Secrets Manager ou arquivo offline | Manual | — | R$ 8/mês |

---

## 9. Custo Total por Fase

### Fase 1 — MVP (Meses 1–3): **R$ 180–250/mês**

| Serviço | Custo/mês | Observação |
|---------|-----------|------------|
| EC2 t3.small | R$ 84 | Sem free tier para t3.small |
| EC2 EBS 30 GB gp3 | R$ 8 | Free tier cobre 8 GB |
| S3 + CloudFront (SPA) | R$ 0 | Free tier |
| Route 53 | R$ 4 | 1 zona + queries |
| SES (e-mails) | R$ 0 | Free tier 62k/mês |
| Secrets Manager (4) | R$ 8 | Sem free tier |
| CloudWatch | R$ 0 | Free tier |
| Data transfer (~10 GB saída) | R$ 5 | US$ 0,09/GB |
| Domínio .com.br (rateado) | R$ 4 | R$ 40/ano ÷ 12 |
| Sentry | R$ 0 | Free tier |
| Grafana Cloud | R$ 0 | Free tier |
| UptimeRobot | R$ 0 | Free tier |
| Mercado Pago | R$ 0 | Sem mensalidade |
| **TOTAL** | **R$ 113** | |
| + Buffer imprevistos (10%) | R$ 12 | |
| **TOTAL CONSERVADOR** | **≈ R$ 125–180/mês** | |

> **Nota:** os primeiros R$ 300 em créditos AWS (AWS Activate para startups) podem cobrir os primeiros 2–3 meses inteiros se você se qualificar. Ver [aws.amazon.com/activate](https://aws.amazon.com/activate).

---

### Fase 2 — Estabilização (Meses 4–8): **R$ 450–600/mês**

| Serviço | Custo/mês | Observação |
|---------|-----------|------------|
| EC2 t3.small (aplicação) | R$ 84 | — |
| RDS db.t3.micro (free tier pode acabar) | R$ 0–86 | Depende dos 12 meses |
| ElastiCache cache.t3.micro | R$ 42 | Redis gerenciado |
| ALB | R$ 80 | Necessário para zero-downtime |
| S3 + CloudFront | R$ 5 | Tráfego crescendo |
| Route 53 | R$ 4 | — |
| SES | R$ 0–5 | Pode ultrapassar 62k/mês |
| Secrets Manager | R$ 10 | 5 segredos |
| Backups S3 (pg_dump + imagens) | R$ 5 | ~2 GB |
| Data transfer (~30 GB saída) | R$ 14 | — |
| Domínio | R$ 4 | — |
| **TOTAL** | **≈ R$ 248–448/mês** | |
| + Buffer (10%) | R$ 25–45 | |
| **TOTAL CONSERVADOR** | **≈ R$ 280–500/mês** | |

---

### Fase 3 — Escala (Meses 9–18): **R$ 900–1.400/mês**

| Serviço | Custo/mês | Observação |
|---------|-----------|------------|
| EC2 t3.medium × 2 (ou ECS Fargate) | R$ 336 | Redundância mínima |
| RDS db.t3.medium Multi-AZ | R$ 688 | Alta disponibilidade |
| ElastiCache cache.t3.small | R$ 84 | — |
| ALB | R$ 80 | — |
| S3 + CloudFront (imagens) | R$ 20 | CDN dedicado para produto |
| MSK / Redpanda Cloud free | R$ 0–200 | Kafka gerenciado |
| Route 53 | R$ 5 | — |
| SES | R$ 10 | Volume crescendo |
| Sentry Team | R$ 135 | > 5k erros/mês |
| Secrets Manager | R$ 12 | — |
| Data transfer (~100 GB) | R$ 47 | — |
| Domínio | R$ 4 | — |
| **TOTAL** | **≈ R$ 1.421–1.621/mês** | |

---

### Resumo visual de custos

```
Fase 1 (MVP)
════════════════════════╗ R$ 125–180/mês
Fase 2 (Estabilização)
════════════════════════════════════════╗ R$ 280–500/mês
Fase 3 (Escala)
══════════════════════════════════════════════════════════════╗ R$ 900–1.400/mês
```

---

## 10. Checklist de Ativação

### T-30 dias antes do lançamento

- [ ] Criar conta AWS (conta raiz) com MFA ativado
- [ ] Criar usuário IAM de trabalho (nunca usar conta raiz no dia a dia)
- [ ] Registrar `utilarferragem.com.br` no Registro.br
- [ ] Criar zona hospedada no Route 53 e apontar NS do Registro.br
- [ ] Criar conta no Mercado Pago (PJ) e solicitar credenciais de sandbox
- [ ] Criar conta no Sentry e configurar DSNs
- [ ] Criar conta no Grafana Cloud e instalar agent no EC2
- [ ] Criar conta no UptimeRobot

### T-14 dias

- [ ] Lançar EC2 t3.small em sa-east-1 com Ubuntu 24.04
- [ ] Instalar Docker + Docker Compose + Nginx
- [ ] Configurar `docker-compose.prod.yml` (já existe em `infrastructure/prod/`)
- [ ] Subir serviços e rodar `make prod-migrate && make prod-seed`
- [ ] Emitir certificado ACM (wildcard `*.utilarferragem.com.br`) — **em us-east-1**
- [ ] Criar distribuição CloudFront para SPA
- [ ] Criar bucket S3 e fazer deploy da SPA: `npm run build && aws s3 sync dist/ s3://utilarferragem-spa`
- [ ] Configurar DNS: `api.` → EC2, `utilarferragem.com.br` → CloudFront

### T-7 dias

- [ ] Verificar domínio no SES e configurar DKIM/SPF/DMARC no Route 53
- [ ] Solicitar saída do sandbox SES via AWS Support
- [ ] Testar fluxo completo: cadastro → produto → checkout → Pix sandbox → e-mail de confirmação
- [ ] Configurar alarmes CloudWatch (CPU, disco, memória, erros 5xx)
- [ ] Configurar monitores UptimeRobot
- [ ] Armazenar segredos no Secrets Manager (ou `.env` seguro)
- [ ] Fazer backup manual do banco + testar restore

### T-1 dia

- [ ] Trocar credenciais Mercado Pago de sandbox para produção (`PSP_ENVIRONMENT=production`)
- [ ] Testar compra real de R$ 1,00 com Pix
- [ ] Verificar recebimento do webhook e transição de status do pedido
- [ ] Verificar e-mail de confirmação chegando na caixa de entrada (não spam)
- [ ] Smoke test completo: todos os endpoints do [checklist de lançamento](13-launch-checklist.md)

### T+0 — Dia do lançamento

- [ ] Remover modo manutenção / página "em breve"
- [ ] Monitorar Grafana + Sentry em tempo real por 2 horas
- [ ] Verificar UptimeRobot — todos os monitores verdes

---

## 11. Decisões Adiadas

Itens deliberadamente excluídos do MVP por custo ou complexidade. Reavaliar com dados reais.

| Item | Custo estimado | Quando reavaliar |
|------|---------------|-----------------|
| **MSK (Kafka gerenciado)** | R$ 300–500/mês | Redpanda local funciona para MVP |
| **RDS Multi-AZ** | +100% custo do RDS | Quando SLA > 99,5% for exigência de cliente |
| **WAF (Web Application Firewall)** | R$ 25/mês + R$ 5/regra | Após primeiro incidente de abuso |
| **Shield Advanced (DDoS)** | US$ 3.000/mês | Somente se volume justificar |
| **CloudTrail (auditoria AWS)** | R$ 15/mês | Exigência de auditoria ou compliance |
| **Config (drift detection)** | R$ 10/mês | Após equipe crescer > 3 engenheiros |
| **SNS SMS (alertas)** | R$ 0,40/SMS | UptimeRobot free com e-mail é suficiente |
| **Elasticsearch / OpenSearch** | R$ 200+/mês | Só se busca full-text no Postgres não escalar |
| **Cloudflare (proxy + DDoS gratuito)** | R$ 0 (plano free) | Alternativa ao WAF — adicionar qualquer hora |
| **App nativo iOS/Android** | — | Só após PWA ter install rate < 5% |

---

*Última atualização: 2026-04-20*  
*Responsável: owner do Sprint 23 (CI/CD + IaC) deve revisar este doc antes do início da Fase 2.*
