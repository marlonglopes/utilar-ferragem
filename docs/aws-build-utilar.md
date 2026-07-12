# O que construir na conta AWS dedicada da Utilar

Derivado do blueprint [`AWS-INFRA.md`](../AWS-INFRA.md) (infra real da Gifthy),
**adaptado à stack da Utilar** e a uma **conta AWS nova e exclusiva** (separada da
Gifthy — decisão do dono). Mesma filosofia: **1 EC2 roda tudo via docker-compose**,
RDS gerenciado, ALB na frente, ECR pras imagens, S3+CloudFront pro SPA.

> **Conta nova = free tier de 12 meses zerado.** No ano 1 dá pra rodar quase de
> graça (EC2/RDS elegíveis); o ALB e a transferência são os custos que não zeram.
> Região: **`sa-east-1` (São Paulo)**, como a Gifthy.

---

## Diferenças da Utilar vs. o blueprint Gifthy

| | Gifthy | **Utilar** |
|---|---|---|
| Backend | ~30 containers (v1+v2 Go/Ruby) | **4 serviços Go**: catalog, order, auth, payment |
| Frontend | container nginx na EC2 | **SPA React → S3 + CloudFront** (já tem `scripts/deploy.sh`) |
| Banco | 1 RDS, N DBs lógicos por ambiente | **1 RDS, 4 DBs lógicos**: `catalog_service`, `order_service`, `auth_service`, `payment_service` |
| Mensageria | NATS | **Redpanda** (Kafka-compat; outbox do payment) |
| Cache | Redis container | **Redis container** (igual) |
| Pagamento | PagarMe/Plugg | **Appmax** (sem infra — API externa) |

**Ponto-chave adaptado:** em vez de 4 instâncias RDS (uma por serviço, como no
dev local), usamos **1 RDS com 4 bancos lógicos** — economia enorme, mesmo padrão
"1 cluster, N DBs" do blueprint (§3).

---

## Inventário a provisionar (conta nova `utilar`)

### 1. Compute — 1× EC2 (roda os 4 serviços Go + Redis + Redpanda)

| Item | Valor Utilar |
|---|---|
| Tipo | **`t3.large`** (2 vCPU, 8 GB) — Redpanda + 4 serviços Go pedem RAM. *Free-tier: `t3.micro` só aguenta MVP mínimo; ver Fase 1.* |
| AMI | Ubuntu 22.04+ |
| Disco | **50 GB** gp3 (root) |
| IP | **Elastic IP** (1) |
| Security group (ingress) | 22 (SSH, seu IP), 80/443 (via ALB), 8090-8093 só interno |
| Roda dentro (docker-compose) | `catalog:8091`, `order:8092`, `auth:8093`, `payment:8090`, `redis:6379`, `redpanda:19092`, `nginx` (reverse proxy) |

> **Redpanda numa t3.large:** subir com `--smp 1 --memory 1G --overprovisioned
> --reserve-memory 0M` pra caber junto dos serviços. Redpanda é mais pesado que o
> NATS da Gifthy — se apertar, subir a instância antes.

### 2. Banco — 1× RDS PostgreSQL

| Item | Valor Utilar |
|---|---|
| Engine | PostgreSQL **17/18** |
| Classe | **`db.t3.micro`** (free tier ano 1) → `db.t3.small` depois |
| Storage | **20 GB** (free tier) → 200 GB c/ autoscaling |
| Multi-AZ | **não** (single-AZ) |
| Público | **não** (só o SG da EC2 acessa) — mais seguro que o setup Gifthy |
| **4 bancos lógicos** | `catalog_service`, `order_service`, `auth_service`, `payment_service` |

Migrations rodam no boot de cada serviço (`db.Migrate` já existe no `main.go`).

### 3. Load balancer — ALB (TLS + roteamento por host)

| Item | Valor Utilar |
|---|---|
| Tipo | Application LB, internet-facing, HTTPS **443** + redirect **80→443** |
| Cert | **ACM** (`utilarferragem.com.br`, `*.utilarferragem.com.br`) |
| Access logs | → S3 `utilar-alb-access-logs-<acct>` |

**Target groups + roteamento host-based** (adaptado do §6):

| Host | Target group → porta | Serve |
|---|---|---|
| `api.utilarferragem.com.br` + path `/catalog/*` ou header | `utilar-catalog` → 8091 | catalog |
| `api.utilarferragem.com.br` + `/orders/*` | `utilar-order` → 8092 | order |
| `api.utilarferragem.com.br` + `/auth/*` | `utilar-auth` → 8093 | auth |
| `api.utilarferragem.com.br` + `/payments/*` + `/webhooks/*` | `utilar-payment` → 8090 | payment |

> Alternativa mais simples: **1 nginx** na EC2 faz o path-routing pros 4 serviços, e
> o ALB manda tudo pra 1 target group (porta do nginx). Menos target groups, mesmo
> resultado. Recomendado pra começar.

### 4. Frontend — S3 + CloudFront (SPA)

| Item | Valor Utilar |
|---|---|
| Bucket | `utilar-spa-prod` (privado, OAC) |
| CDN | CloudFront → `utilarferragem.com.br` + `www` (ACM em `us-east-1`) |
| Deploy | **`scripts/deploy.sh production`** (já existe: build → S3 sync → invalidação) |
| Env | `VITE_*_URL` apontando pra `api.utilarferragem.com.br` |

### 5. ECR — imagens dos 4 serviços

`<acct>.dkr.ecr.sa-east-1.amazonaws.com` — repos: `catalog`, `order`, `auth`,
`payment`. (Falta criar os **Dockerfiles** — ver "Pendências".)

### 6. S3 — buckets de apoio

`utilar-spa-prod` (SPA) · `utilar-dumps` (backups de banco) ·
`utilar-alb-access-logs-<acct>` · `aws-cloudtrail-logs-<acct>`.

### 7. DNS — Route53

Zona `utilarferragem.com.br` (registrar no **Registro.br**, apontar NS pra Route53):
`utilarferragem.com.br` + `www` → CloudFront; `api` → ALB.

### 8. Auditoria & observabilidade

CloudTrail → S3 · cron de custo (Cost Explorer → email via Mandrill, igual Gifthy) ·
Sentry (SaaS) opcional.

### 9. NÃO usar (mesma disciplina do blueprint)

Sem **ECS/EKS/Fargate**, sem **ElastiCache** (Redis é container), sem **MSK**
(Redpanda é container), sem **Lambda**. Escala **vertical** primeiro.

---

## Pendências antes do deploy (não são AWS, são código)

1. **Dockerfiles** dos 4 serviços Go (multi-stage → imagem distroless ~15 MB) — **não existem ainda**.
2. **`docker-compose.prod.yml`**: 4 serviços + Redis + Redpanda + nginx, apontando pro RDS externo, com env de prod (`DEV_MODE=false`, `JWT_SECRET` forte, `ALLOWED_ORIGINS`, `APPMAX_*`, `PSP_PROVIDER=appmax`).
3. **`deploy.sh` estendido**: hoje só faz o SPA (S3+CloudFront). Adicionar o caminho backend (build+push ECR → ssh EC2 → `docker compose pull && up -d`).
4. **Fail-closed check**: os serviços recusam subir em prod sem `JWT_SECRET` 32+ e (payment) sem config PSP — garantir env completo.

---

## Faseamento (aproveitando o free tier)

**Fase 1 — MVP free-tier (ano 1, ~US$20-40/mês):**
EC2 (t3.small; t3.micro se couber) + RDS `db.t3.micro` 20 GB + **sem ALB** (nginx na
EC2 + EIP + certbot/ACM) + S3+CloudFront pro SPA. Só paga transfer + o que passar do
free tier. Redpanda tunado pra RAM baixa.

**Fase 2 — produção (ano 2 ou quando faturar, ~US$130-170/mês):**
Sobe pra `t3.large` + RDS `db.t3.small` 200 GB + **ALB** (TLS + host-routing) + WAF.
É o cenário do orçamento em [`orcamento-utilar-aws-2026-07.md`](orcamento-utilar-aws-2026-07.md).

**Fase 3 — HA (só quando doer):** RDS Multi-AZ, 2ª EC2 atrás do ALB, Redis/Redpanda gerenciados.

---

## Checklist de provisionamento (conta nova)

- [ ] Criar conta AWS Utilar, root MFA, usuário IAM admin, billing alerts
- [ ] Registrar `utilarferragem.com.br` (Registro.br) → zona Route53
- [ ] ACM: cert `sa-east-1` (ALB) + `us-east-1` (CloudFront)
- [ ] VPC (default serve) + Security Groups (EC2, RDS)
- [ ] RDS PostgreSQL single-AZ, privado, 4 bancos lógicos
- [ ] EC2 + EIP + Docker + docker-compose
- [ ] ECR (4 repos) + Dockerfiles + push
- [ ] S3 (spa/dumps/logs) + CloudFront (OAC) + `deploy.sh production`
- [ ] ALB (Fase 2) ou nginx+certbot (Fase 1) + regras host/path
- [ ] CloudTrail → S3 + cron de custo (Cost Explorer → email)
- [ ] Env de prod completo (JWT, CORS, APPMAX) + `DEV_MODE=false`
- [ ] Smoke E2E: catálogo → login → carrinho → checkout (Appmax) → pedido

---
*Derivado de `AWS-INFRA.md` (estado real da Gifthy, 2026-07-09), adaptado à stack
Utilar e à conta dedicada nova. Ver também `docs/14-infra-custos.md` e o orçamento.*
