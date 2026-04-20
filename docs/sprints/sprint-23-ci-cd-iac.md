# Sprint 23 — CI/CD + Terraform IaC

**Fase**: 3 — Commerce (gate pré-lançamento). **Estimativa**: 6–8 dias.

## Escopo

Cada passo feito manualmente no deploy se torna reproduzível. Os módulos **Terraform** capturam a infraestrutura AWS específica da Utilar (S3 + CloudFront + ACM + Route53 + Secrets Manager + regra de listener do ALB como adição ao ALB compartilhado da plataforma pai). O estado fica em S3 com uma tabela de lock no DynamoDB. **GitHub Actions** para o SPA: lint → typecheck → test → build → deploy-staging → Lighthouse CI → promoção manual → deploy-prod com invalidação do CloudFront.

Staging faz deploy automático em cada merge para `main`; **produção exige um aprovador** via regra de proteção de GitHub Environment. Rollback é uma ação "Run workflow" de 1 clique que reimplanta a tag anterior. O CI/CD do product-gateway pai (build de imagem + deploy do compose) permanece separado — este sprint cobre apenas o SPA específico da Utilar + adições de infraestrutura.

## Tarefas

### Terraform — estado + módulos
1. Criar `infrastructure/terraform/utilar/` com subdiretórios `modules/` + `envs/staging/` + `envs/prod/`
2. Estado remoto: `terraform init -backend-config="bucket=utilar-tf-state" -backend-config="key=envs/{env}/terraform.tfstate" -backend-config="region=us-east-1" -backend-config="dynamodb_table=utilar-tf-locks"`. Bootstrap do bucket de estado + tabela de lock com uma stack `bootstrap/` avulsa que fica fora do estado remoto.
3. Módulo `s3-spa`: bucket de website, política OAC, bucket policy permitindo apenas CloudFront
4. Módulo `cloudfront-spa`: distribuição com duas origens (S3 para estáticos, ALB para `/api/*`), behaviors de cache (estáticos: imutável 1 ano, `/api/*`: sem cache, `/api/v1/marketplace/*`: 60s), respostas de erro customizadas (404 → `/index.html` para roteamento SPA), price class 100 (NA + EU apenas, mais barato)
5. Módulo `acm-cert`: certificado ACM em us-east-1 (requisito do CloudFront) para `utilarferragem.com.br` + `www.utilarferragem.com.br` + `staging.utilarferragem.com.br`, validação DNS via Route53
6. Módulo `route53-records`: aliases A/AAAA apontando para a distribuição CloudFront
7. Módulo `alb-listener-rule`: adiciona uma regra de host-header ao ALB da plataforma pai (`utilarferragem.com.br` + `staging.utilarferragem.com.br` → target group da Utilar para `/api/*`). ARN do ALB pai passado como variável.
8. Módulo `secrets`: um secret do Secrets Manager por ambiente com as chaves `JWT_SECRET`, `VAPID_PRIVATE`, `SENTRY_DSN`, `TWILIO_AUTH_TOKEN`, `PSP_API_KEY`
9. `envs/staging/main.tf` + `envs/prod/main.tf` conectam os módulos; `variables.tf` + `terraform.tfvars.example`; `README.md` com a ordem de aplicação

### GitHub Actions — CI
10. `.github/workflows/utilar-ci.yml`: disparado em PR + push para `main` quando há mudanças em `utilar-web/**` ou `services/**`. Jobs: `lint` (eslint + rubocop) + `typecheck` (tsc) + `test` (vitest + rspec em matriz por serviço) + `build` (vite build, upload de artefato)
11. Cache de dependências Node + Ruby (`actions/cache` com chave baseada em hashes dos lockfiles)
12. Fail-fast desabilitado na matriz de testes para que todos os serviços reportem

### GitHub Actions — deploy
13. `.github/workflows/utilar-deploy.yml`: acionado pelo `utilar-ci` bem-sucedido em `main`. Jobs:
    - `deploy-staging`: baixar artefato de build → `aws s3 sync` para o bucket de staging → invalidação CloudFront `/*` → smoke test (curl `/` + `/api/v1/health`)
    - `lighthouse`: executa o Lighthouse CI do Sprint 21 contra a URL de staging; reprova o workflow se os orçamentos forem violados
    - `deploy-prod`: **environment: production** (exige 1 aprovador pela regra de GitHub Environment); mesmo sync + invalidação + smoke contra a URL de produção
14. Cada job assume uma role AWS via OIDC (`aws-actions/configure-aws-credentials`) — sem chaves AWS de longa duração no GitHub
15. Políticas de trust da role IAM travadas em `repo:OWNER/REPO:environment:staging` / `:environment:production`

### GitHub Environments
16. Criar ambiente `staging` — sem aprovadores, secrets: `VITE_API_URL=https://staging.utilarferragem.com.br/api`, `VITE_SENTRY_DSN`, `AWS_ROLE_ARN_STAGING`, `CLOUDFRONT_DIST_ID_STAGING`, `S3_BUCKET_STAGING`
17. Criar ambiente `production` — 1 revisor obrigatório (fundador), secrets equivalentes de produção, regra de branches protegidos permite apenas `main`
18. Secrets em nível de repositório: `LHCI_GITHUB_APP_TOKEN`, `SENTRY_AUTH_TOKEN` (para upload de source maps)

### Rollback
19. `.github/workflows/utilar-rollback.yml`: acionamento manual (`workflow_dispatch`) com input `tag`; reimplanta aquele artefato de build no ambiente selecionado; cria um registro de rollback em um `deployments.log` no S3
20. Manter os últimos 10 artefatos de build fixos em um bucket S3 (regra de lifecycle deleta os mais antigos)

### Detecção de drift
21. Cron semanal `.github/workflows/tf-drift.yml`: executa `terraform plan` em modo apenas detecção para staging + produção, posta um comentário resumindo se houver drift
22. Política: **nenhuma alteração pelo console AWS** — o PR de drift é o mecanismo para correções

### Runbook
23. `infrastructure/terraform/utilar/README.md` com: bootstrap inicial, adicionar novo ambiente, fluxo de apply, fluxo de rollback, fluxo de resolução de drift

## Critérios de aceite

- [ ] `terraform apply` em uma conta AWS zerada (com bootstrap feito) provisiona toda a infra da Utilar de ponta a ponta
- [ ] Um push para `main` com mudança no frontend faz deploy automático para staging em até 10 minutos
- [ ] O deploy de produção fica aguardando o aprovador; ao aprovar, aciona o sync + invalidação
- [ ] A invalidação do CloudFront é concluída a cada deploy (verificado pelo hash nos nomes dos arquivos em `/assets/`)
- [ ] O workflow de rollback reimplanta uma tag anterior em menos de 3 minutos
- [ ] A detecção de drift semanal é executada; qualquer alteração no console aparece como drift no relatório
- [ ] Nenhuma chave AWS armazenada no GitHub — todo acesso via assumição de role OIDC
- [ ] O gate do Lighthouse CI bloqueia a promoção de staging → produção se a performance regredir além dos orçamentos

## Dependências

- Acesso à conta AWS com permissões para criar o usuário IAM de bootstrap
- Sprint 15 da plataforma pai (ALB) concluído — a regra de listener do ALB da Utilar se anexa ao ALB existente
- Domínio `utilarferragem.com.br` registrado e zona hospedada no Route53 criada
- Certificado do subdomínio `staging.utilarferragem.com.br` validado

## Riscos

- Drift do Terraform por alterações manuais no console — impor "sem alterações no console" no runbook + detecção de drift semanal + PR obrigatório para correção
- Configuração incorreta da política de trust IAM OIDC — testar com um repositório de teste antes de migrar definitivamente
- Colisão de regra de listener do ALB com a plataforma pai — coordenar prioridades; a regra de host-header da Utilar deve ter prioridade mais alta do que a ação default catch-all
- Instabilidade do Lighthouse CI — definir orçamentos 5% mais flexíveis do que os números de aceite do Sprint 21; re-executar uma vez em caso de falha antes de bloquear
