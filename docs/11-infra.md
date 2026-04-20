# 11 — Infraestrutura

**Status**: Rascunho. **Data**: 2026-04-20.

Layout AWS, DNS/SSL, hospedagem do SPA + API, pipeline de CI/CD, staging, estrutura do Terraform, estimativa de custos, estratégia de cache.

Leituras complementares:
- [../adr/001-placement-and-stack.md](adr/001-placement-and-stack.md) — decisão de SPA no S3 + CloudFront
- [08-security.md](08-security.md) §6 — estrutura do Secrets Manager
- [09-observability.md](09-observability.md) §1 — ferramentas de telemetria

---

## 1. Layout da conta AWS

**Escolha: conta AWS única para a Fase 3, com separação lógica por prefixo de recurso (`utilar-prod-*`, `utilar-staging-*`). Migrar para multi-conta (Control Tower) somente na Fase 4+ quando o time tiver mais de 2 pessoas ou uma auditoria de conformidade exigir.**

Justificativa: um fundador solo + uma conta é o mais simples. O Control Tower custa $0, mas adiciona overhead de configuração (organizations, SSO, roles cross-account) que não é justificado para o MVP. Todos os recursos são tagueados com `app=utilar`, `env=prod|staging`, `owner=utilar-team` para facilitar a migração futura sem renomear tudo.

| Prós | Contras |
|------|------|
| Zero custo de setup | Raio de explosão se credenciais vazarem = tudo |
| Fatura única | Políticas IAM devem ser rígidas (ver §7) |
| Zona hospedada Route53 compartilhada | Risco de colisão prod/staging — disciplina de prefixos obrigatória |

Critérios de saída para separar contas: qualquer um dos seguintes — (a) >2 engenheiros em tempo integral, (b) auditoria SOC 2 / ISO-27001 acionada, (c) qualquer cliente solicitando ambiente isolado.

---

## 2. DNS

**Zona hospedada Route53: `utilarferragem.com.br`** (domínio apex registrado no Registro.br, com registros NS delegados para o Route53).

Subdomínios:

| Registro | Tipo | Destino | Finalidade |
|--------|------|--------|---------|
| `utilarferragem.com.br` | A (alias) | Distribuição CloudFront (SPA prod) | Landing de marketing (redireciona para `www`) |
| `www.utilarferragem.com.br` | A (alias) | Distribuição CloudFront (SPA prod) | SPA |
| `api.utilarferragem.com.br` | A (alias) | ALB do product-gateway pai | API de backend |
| `staging.utilarferragem.com.br` | A (alias) | Distribuição CloudFront (SPA staging) | SPA de staging |
| `api-staging.utilarferragem.com.br` | A (alias) | Mesmo ALB, regra de listener de staging | Backend de staging |
| `status.utilarferragem.com.br` | CNAME | `statuspage.io` ou S3 estático | Página de status pública |
| `_dmarc.utilarferragem.com.br` | TXT | `v=DMARC1; p=quarantine; rua=mailto:dmarc@utilarferragem.com.br` | Autenticação de e-mail |
| Seletores DKIM (3) | CNAME | Fornecidos pelo SES | Autenticação de e-mail |
| `_domainkey` etc. | TXT | SPF `v=spf1 include:amazonses.com -all` | Autenticação de e-mail |

Registros MX: gerenciados pelo SES inicialmente; migrar para Google Workspace quando e-mail de suporte inbound for necessário.

Bloqueio de registrador: habilitado no Registro.br. Bloqueio de transferência + carência de 60 dias após qualquer alteração de contato.

---

## 3. SSL

**ACM (AWS Certificate Manager) em `us-east-1` para CloudFront** (obrigatório) + **`sa-east-1`** para o ALB (região São Paulo, alinhada com nossa meta de latência).

| Certificado | Região | Cobre | Usado por |
|------|--------|--------|---------|
| `utilar-apex-wildcard` | `us-east-1` | `utilarferragem.com.br`, `*.utilarferragem.com.br` | CloudFront prod + staging |
| `utilar-api` | `sa-east-1` | `api.utilarferragem.com.br`, `api-staging.utilarferragem.com.br` | Listener do ALB |

Validação: DNS (Route53 cria automaticamente os registros CNAME). Renovação automática pelo ACM (60 dias antes do vencimento, totalmente automática enquanto os registros DNS permanecerem).

Sem wildcard em `api.*` — somente nomes explícitos, para reduzir o raio de explosão.

---

## 4. Hospedagem do SPA (S3 + CloudFront)

### 4.1 S3

Dois buckets, um por ambiente. **Não** público — somente CloudFront OAC.

| Bucket | Env | Região | Conteúdo |
|--------|-----|--------|----------|
| `utilar-web-prod-sa-east-1` | prod | `sa-east-1` | Bundle do SPA do último deploy de `main` |
| `utilar-web-staging-sa-east-1` | staging | `sa-east-1` | Bundle do SPA do último deploy de `develop` |

Política do bucket: **privada, somente CloudFront OAC** (sem ACLs, sem desabilitar bloqueio público). Objetos:

- `index.html` — cache curto
- `assets/*-[fingerprint].{js,css,woff2,webp}` — imutável, cache longo
- `robots.txt`, `sitemap.xml` — cache curto (reconstruídos no deploy)
- `favicon.ico`, `apple-touch-icon.png` — cache longo

Versionamento: habilitado. Ciclo de vida: excluir versões não-atuais após 30 dias.

### 4.2 Distribuições CloudFront

Uma distribuição por ambiente. Cada distribuição:

| Configuração | Valor |
|---------|-------|
| Origem | Bucket S3 acima, via Origin Access Control |
| Classe de preço | PriceClass_100 (EUA, Canadá, Europa) — upgrade para All quando tráfego LATAM justificar |
| Certificado SSL | Wildcard ACM em us-east-1 |
| Política SSL | Mínimo `TLSv1.2_2021` |
| HTTP/2 | ligado |
| HTTP/3 (QUIC) | ligado |
| IPv6 | ligado |
| Raiz padrão | `/index.html` |
| WAF | AWSManagedRulesCommonRuleSet + AWSManagedRulesKnownBadInputsRuleSet |
| Compressão | gzip + brotli |
| Respostas de erro customizadas | 403/404 → 200 `/index.html` (roteamento SPA) |
| Política de headers de resposta | Ver [08-security.md](08-security.md) §8.2 (CSP + HSTS + X-CTO + RP + PP) |

### 4.3 Políticas de cache

| Padrão de caminho | Política de cache | TTL |
|--------------|--------------|-----|
| `/index.html` | `CachingDisabled` OU `min=0, max=60, default=10` | Curto; re-buscado para descobrir novos bundles |
| `/assets/*` | `CachingOptimized` (min=31536000) | 1 ano imutável (com fingerprint) |
| `/robots.txt`, `/sitemap.xml` | `CachingOptimized` c/ TTL=3600 | 1h |
| `/*` (rotas SPA) | Cai no `/index.html` via resposta de erro | — |

Política de requisição de origem: `CORS-S3Origin` (encaminha somente os headers que o S3 precisa).

### 4.4 Invalidação no deploy

Somente `/index.html` (e `/robots.txt`, `/sitemap.xml` se modificados). Os bundles têm fingerprint — fazer upload de um novo `main-a1b2c3.js` **não** invalida nada; o antigo permanece em cache para usuários no meio de uma sessão.

Custo de invalidação: gratuito para as primeiras 1000/mês; fazemos ≈10 deploys/mês.

```bash
aws cloudfront create-invalidation \
  --distribution-id $DIST_ID \
  --paths "/index.html" "/robots.txt" "/sitemap.xml"
```

---

## 5. Hospedagem da API (reutilizar o ALB pai)

A Utilar **não** adiciona um novo ALB. Reutiliza o ALB do product-gateway do Sprint 15.

### 5.1 Adicionado no ALB

| Recurso | Valor |
|----------|-------|
| Regra de listener (HTTPS 443) | Header de host = `api.utilarferragem.com.br` → target group `utilar-api-prod` (containers Go gateway no EC2) |
| Regra de listener (staging) | Header de host = `api-staging.utilarferragem.com.br` → target group `utilar-api-staging` |
| Cert adicionado ao listener | `utilar-api` do §3 |
| Regra padrão | Roteamento gifthy-hub existente — sem alterações |

### 5.2 Atualização de CORS do gateway

`services/gateway/cmd/server/main.go` allowlist CORS (controlada por variável de ambiente):

```
CORS_ALLOWED_ORIGINS=https://utilarferragem.com.br,https://www.utilarferragem.com.br,https://staging.utilarferragem.com.br,https://app.gifthy.com,https://staging.gifthy.com
```

Sem wildcards em produção. Origens localhost permitidas somente quando `APP_ENV=development`.

### 5.3 Hospedagem do payment-service

Adicionado ao `infrastructure/prod/docker-compose.yml` como novo serviço:

```yaml
payment-service:
  image: ${ECR_URI}/utilar-payment-service:${PAYMENT_TAG}
  environment:
    DATABASE_URL: ${PAYMENT_DB_URL}
    JWT_SECRET: ${JWT_SECRET}
    PSP_API_KEY: ${PSP_API_KEY}
    PSP_WEBHOOK_SECRET: ${PSP_WEBHOOK_SECRET}
    PSP_ENVIRONMENT: ${PSP_ENVIRONMENT:-sandbox}
    KAFKA_BROKERS: ${KAFKA_BROKERS}
    SENTRY_DSN: ${SENTRY_DSN_PAYMENT}
    SERVICE_NAME: payment-service
  depends_on:
    - postgres
    - redpanda
  ports:
    - "3005:3000"
  logging:
    driver: awslogs
    options:
      awslogs-group: /utilar/prod/payment-service
      awslogs-region: sa-east-1
```

O gateway adiciona rotas de proxy reverso:

- `POST /api/v1/payments` → `payment-service:3000` (JWT obrigatório)
- `GET /api/v1/payments/:id` → `payment-service:3000` (JWT obrigatório)
- `POST /webhooks/psp/*` → `payment-service:3000` (público; assinatura verificada no destino)

---

## 6. CI/CD

### 6.1 Visão geral do pipeline

GitHub Actions. Gatilhos: push em qualquer branch → lint/test; merge para `develop` → deploy staging; merge para `main` → deploy prod (com aprovação).

```
  [PR aberto]
      │
      ▼
  lint → typecheck → unit → integration → contract → lighthouse → axe
      │                                                            │
      └────────────── todos verdes ──────────────────────────────────┘
                                │
                                ▼
                   [merge para develop]
                                │
                                ▼
              build (vite + docker)
                                │
                                ▼
              push para S3 / ECR (staging)
                                │
                                ▼
              invalidar CF staging + restart rolling dos containers staging
                                │
                                ▼
              smoke test (Playwright @smoke) contra staging
                                │
                                ▼
          [sucesso em staging → auto-merge para main? não — promoção manual]
                                │
                                ▼
              [merge para main → deploy em produção exige aprovação]
                                │
                                ▼
              build, push, deploy prod, invalidar CF prod, smoke test prod
```

### 6.2 Jobs

| Job | Gatilho | Tempo de execução | Notas |
|-----|---------|---------|-------|
| `lint-test-frontend` | PR + push | ~3min | Node 20, cache de `node_modules` via actions/cache |
| `lint-test-backend` | PR + push (paths `services/**`) | ~5min | Ruby 3.2, cache do bundler, matrix paralela por serviço |
| `contract-tests` | PR | ~2min | Comparação de snapshot |
| `lighthouse` | PR | ~4min | Serve o vite preview, executa LHCI |
| `axe` | PR | ~3min | Subconjunto a11y do Playwright |
| `security-scan` | PR + semanal | ~5min | gitleaks + bundler-audit + npm audit + ZAP (somente semanal) |
| `build-spa` | push para develop/main | ~2min | `vite build`, faz upload do artifact |
| `build-payment-service` | push para develop/main (paths `services/payment-service/**`) | ~4min | Docker build, push para ECR |
| `deploy-staging` | push para develop | ~3min | S3 sync + CF invalidate + ECR pull + compose restart |
| `smoke-staging` | após deploy-staging | ~3min | `playwright test --grep @smoke` |
| `deploy-prod` | push para main | ~5min | **Requer aprovação** via GitHub Environment |
| `smoke-prod` | após deploy-prod | ~3min | Mesmo conjunto de smoke, contra prod |
| `rollback-prod` | dispatch manual | ~2min | Restaura tag ECR anterior + prefixo S3 anterior |

### 6.3 Segredos (GitHub Environments)

Dois environments: `staging`, `production`. Ambos exigem dois conjuntos de segredos distintos para evitar uso cross-env.

| Segredo | Valor de staging | Valor de prod |
|--------|---------------|-----------|
| `AWS_ACCESS_KEY_ID` | role de deploy (staging) | role de deploy (prod) |
| `AWS_SECRET_ACCESS_KEY` | " | " |
| `CF_DISTRIBUTION_ID` | ID do CF de staging | ID do CF de prod |
| `S3_BUCKET` | `utilar-web-staging-sa-east-1` | `utilar-web-prod-sa-east-1` |
| `ECR_REGISTRY` | `...sa-east-1.amazonaws.com/utilar` | mesmo |
| `PSP_API_KEY` | Sandbox MP | Produção MP |
| `SENTRY_AUTH_TOKEN` | Token de upload de release do Sentry | mesmo |

Deploys em produção exigem aprovação manual — a configuração "required reviewers" do GitHub Environment força um segundo par de olhos (ou auto-aprovação com delay de 10min na fase solo).

### 6.4 Rollback

- Frontend: `aws s3 sync s3://utilar-web-prod-sa-east-1-archive/<sha-anterior>/ s3://utilar-web-prod-sa-east-1/` + invalidação do CF.
- Backend: re-deploy da tag ECR anterior via workflow dispatch (`rollback-prod` com `target_tag=<sha>`).
- Migrações: se o código revertido for incompatível com o novo schema, intervenção manual é necessária — ver [12-ops-runbook.md](12-ops-runbook.md) §3.

Arquivo mantido para os últimos 10 deploys em um prefixo separado.

---

## 7. Ambiente de staging

Réplica completa de produção, porém:

- Distribuição CloudFront separada (`staging.utilarferragem.com.br`)
- Bucket S3 separado
- ALB compartilhado com regra de listener separada → `api-staging.utilarferragem.com.br`
- Target group EC2 separado (menor — 1 instância vs 2 em prod)
- Postgres separado (RDS único para todos os serviços de staging, ou reutilizar Postgres da máquina dev — decisão pendente; recomendamos RDS `db.t4g.micro` compartilhado)
- Credenciais PSP de sandbox
- Sentry environment = `staging`
- Seed nightly com `rake staging:reseed` — dados novos, sem PII

Acesso: protegido por Basic Auth do CloudFront (função Lambda@Edge que verifica uma senha compartilhada) até o lançamento. Removido antes do lançamento.

---

## 8. Estrutura do Terraform

`infrastructure/terraform/` — repositório único, backends de estado por ambiente.

```
infrastructure/terraform/
├── envs/
│   ├── prod/
│   │   ├── main.tf              # compõe módulos para prod
│   │   ├── variables.tf
│   │   ├── terraform.tfvars     # versionado (valores não secretos)
│   │   └── backend.tf           # s3://utilar-tfstate/prod.tfstate
│   └── staging/
│       └── ...                  # mesmo formato
└── modules/
    ├── network/                 # VPC (reutilizar a do pai), registros Route53
    ├── s3-cloudfront/           # bucket + OAC + distribuição + política de headers de resposta
    ├── acm/                     # certs (us-east-1 para CF, sa-east-1 para ALB)
    ├── route53/                 # zona hospedada + registros DNS
    ├── secrets/                 # caminhos do Secrets Manager (ver [08-security.md](08-security.md) §6)
    ├── ecr/                     # registries + políticas de ciclo de vida
    ├── alb-rules/               # regras de listener adicionadas ao ALB compartilhado (baseadas em host-header)
    ├── iam/                     # roles de deploy do CI (mínimo escopo)
    └── ses/                     # identidade de domínio + DKIM + registros SPF
```

### 8.1 Ordem de apply (bootstrap)

1. `route53` — zona hospedada (deve existir antes da validação DNS do ACM)
2. `acm` — certs (validados via Route53)
3. `ecr` — registries vazios
4. `s3-cloudfront` — bucket do SPA + distribuição
5. `alb-rules` — regras de listener no ALB compartilhado
6. `secrets` — caminhos de segredos vazios (valores preenchidos fora do Terraform)
7. `iam` — roles de deploy do CI
8. `ses` — identidade de domínio + DKIM

Applies subsequentes são idempotentes. Prod + staging compartilham módulos mas nunca o state.

### 8.2 State

Backend: S3 + lock DynamoDB.

- `s3://utilar-tfstate-sa-east-1/prod.tfstate`
- `s3://utilar-tfstate-sa-east-1/staging.tfstate`
- Tabela DynamoDB `utilar-tf-locks` (pay-per-request)

Bucket + tabela provisionados manualmente uma vez (ovo-galinha).

### 8.3 Plan / apply

```bash
cd infrastructure/terraform/envs/prod
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

Plans postados em PRs via `atlantis` ou GitHub Action (fase 4 — manual na Fase 3).

---

## 9. Estimativa de custos

Ordem de grandeza mensal, primeiros 6 meses (baixo tráfego: ~1k visitantes únicos/dia, ~10 pedidos/dia).

| Item | Est. USD/mês | Notas |
|-----------|----------------|-------|
| Zona hospedada Route53 | $0,50 | $0,50/zona + queries (negligível) |
| Certificados ACM | $0 | Gratuito |
| S3 (SPA) | $0,50 | 5GB × $0,023 + GET negligível |
| CloudFront | $5 | 100GB transferência × $0,085 + 1M requisições × $0,0075 |
| ALB (compartilhado com Gifthy) | $0 (custo compartilhado) | Amortizado com o pai |
| EC2 (compartilhado com Gifthy) | $0 (custo compartilhado) | `t3.medium` × 2 cobre todos os serviços por ora |
| Container do payment-service | ~$0 | Cabe no footprint EC2 existente |
| RDS (staging compartilhado) | $15 | `db.t4g.micro` single-AZ staging |
| RDS prod (Fase 4) | $0 agora; ~$60 depois | Atualmente em Postgres de container; migrar para RDS na Fase 4 |
| Redpanda | $0 | Self-hosted no EC2 compartilhado |
| Secrets Manager | $5 | ~10 segredos × $0,40 + chamadas de API |
| CloudWatch Logs | $10 | ~20GB ingeridos + 20GB armazenados |
| CloudWatch Metrics + X-Ray | $5 | Dentro do free tier para baixo volume |
| SES | $1 | ~10k e-mails × $0,10/1k |
| Sentry | $0 | Free tier (5k erros, 10k transações) |
| UptimeRobot | $0 | Free tier |
| GitHub Actions | $0 | Repositório público OU 2k minutos gratuitos para privado |
| **Subtotal (somente Utilar)** | **≈ $42/mês** | |
| Taxas do PSP (variável) | 0,99–3,99% + R$ 0,40/tx | Não é item AWS |
| Renovação do domínio (Registro.br) | ~R$ 40/ano | ≈$7/ano |

**Sinais de crescimento para monitorar**:

- Fatura do CloudFront ultrapassa $20 → considerar PriceClass_All
- Postgres em container no EC2 compartilhado → migrar para RDS quando >5GB
- Cota do Sentry excedida → upgrade para plano Team ($26/mês)

---

## 10. Estratégia de invalidação de cache

Regras práticas:

| Arquivo | Estratégia |
|------|----------|
| `index.html` | Sempre invalidar no deploy. Max-age curto (60s) para que os usuários peguem novos bundles rapidamente. |
| `assets/*-[hash].*` | **Nunca** invalidar. O nome de arquivo com fingerprint garante a frescura. |
| `robots.txt`, `sitemap.xml` | Invalidar no deploy se modificados (verificar via git diff no CI). |
| Páginas estáticas (privacidade, tos) | Fazem parte do bundle do SPA — o fingerprint cuida disso. |

### 10.1 Snippet do script de deploy

```bash
#!/usr/bin/env bash
set -euo pipefail

SHA=$(git rev-parse --short HEAD)

# 1. Build
cd utilar-ferragem && npm ci && npm run build

# 2. Arquivar o prefixo anterior para rollback
aws s3 sync "s3://$S3_BUCKET/" "s3://$S3_BUCKET_ARCHIVE/$(date +%Y%m%d-%H%M)-prev/" \
  --exclude "archive/*"

# 3. Upload dos assets com fingerprint com cache longo (antes do index.html)
aws s3 sync dist/assets "s3://$S3_BUCKET/assets/" \
  --cache-control "public, max-age=31536000, immutable" \
  --exclude "*.html"

# 4. Upload dos arquivos de cache curto por último (para que a corrida de clientes para o novo bundle seja segura)
aws s3 cp dist/index.html "s3://$S3_BUCKET/index.html" \
  --cache-control "public, max-age=60, s-maxage=60"
aws s3 cp dist/robots.txt "s3://$S3_BUCKET/robots.txt" \
  --cache-control "public, max-age=3600"
aws s3 cp dist/sitemap.xml "s3://$S3_BUCKET/sitemap.xml" \
  --cache-control "public, max-age=3600"

# 5. Invalidar apenas os caminhos de cache curto
aws cloudfront create-invalidation \
  --distribution-id "$CF_DIST_ID" \
  --paths "/index.html" "/robots.txt" "/sitemap.xml"

# 6. Registrar release no Sentry
sentry-cli releases new "utilar@$SHA"
sentry-cli releases set-commits "utilar@$SHA" --auto
sentry-cli releases finalize "utilar@$SHA"
```

A ordem importa: fazer upload dos assets **antes** do `index.html` garante que, no momento em que o `index.html` é servido, suas referências com fingerprint já estejam disponíveis.

---

## 11. Fora do escopo (para este doc)

- Migração para Kubernetes — Fase 5+
- Multi-região ativo-ativo — Fase 5+
- Auto Scaling Group para EC2 — Fase 4
- RDS Multi-AZ — Fase 4
- Regras WAF customizadas além das AWS Managed — Fase 4
- Edge compute / Lambda@Edge além do gate de basic auth — Fase 4+
