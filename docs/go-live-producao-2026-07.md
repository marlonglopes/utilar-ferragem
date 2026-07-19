# Go-live da Utilar — relatório de produção

**Data:** 2026-07-19 · **Câmbio de referência:** R$ 5,40 / US$ (recalcular no fechamento).
Consolida e substitui, como visão de topo, `orcamento-utilar-aws-2026-07.md` e
`aws-build-utilar.md` (que continuam como detalhe). Leia antes: `ESTADO-DO-PROJETO.md`.

Cobre **tudo que precisa existir fora do código** para a Utilar vender de verdade:
domínio, e-mail, AWS (arquitetura EC2 + ALB + RDS), observabilidade (Sentry),
gestão de segredos (1Password / Secrets Manager), Appmax, e a **prospecção de custo
inicial** — separando o que é único (setup) do que é recorrente (mês a mês).

> **A regra de ouro deste projeto vale aqui também:** conta, credenciais, infra,
> bancos e dados da Utilar **nunca** se misturam com os da gifthy. Tudo abaixo
> pressupõe conta AWS, domínio, e-mail e contas SaaS **próprios da Utilar**.

---

## 0. Resumo executivo

| Bloco | O que é | Recorrente (mês) | Setup (único) |
|---|---|---:|---:|
| **AWS — largada** (EC2 t3.medium + ALB + RDS single-AZ) | Hospedagem de tudo | ~R$ 1.020 | — |
| **AWS — ano 1 com free tier** (conta standalone) | Mesma infra, créditos cobrindo | ~R$ 200–320 | — |
| **Domínio** `utilarferragem.com.br` | Registro.br (exige CNPJ) | ~R$ 3 | R$ 40 (1º ano) |
| **E-mail** `contato@…` (Google Workspace, 3 caixas) | Caixa de verdade + envio | ~R$ 102 | — |
| **E-mail transacional** (AWS SES) | Confirmação de pedido, NFC-e | ~R$ 5 | — |
| **Sentry** (erros/observabilidade) | Free no início → Team | R$ 0 → ~R$ 140 | — |
| **1Password / Secrets** | Segredos humanos + de aplicação | ~R$ 130 | — |
| **Appmax** (gateway) | % por venda, sem mensalidade | proporcional à receita | R$ 0 |
| **NFC-e** (fiscal, obrigatória p/ balcão) | Emissão de nota | a definir | 60–80 h (ou ~20 h integração) |

**Duas leituras de custo recorrente:**
- **Ano 1, conta standalone com free tier:** ~**R$ 500–650/mês** somando AWS enxuta + e-mail + Sentry + secrets.
- **Regime normal (sem free tier, largada dedicada):** ~**R$ 1.250–1.400/mês** tudo somado.

**O gargalo não é dinheiro nem código — é decisão do dono:** criar a conta Appmax
(sem ela não há venda), a conta AWS standalone, e resolver a NFC-e. Ver §10.

---

## 1. Domínio — Registro.br vs GoDaddy

**Recomendação: Registro.br.** É o registrador oficial do `.br`; para um
`.com.br` não há motivo para intermediário.

| | **Registro.br** ✅ | GoDaddy |
|---|---|---|
| Registra `.com.br` | Sim (oficial) | Sim (revenda, mais caro) |
| Preço `.com.br` | **R$ 40/ano** | ~R$ 60–120/ano + upsells |
| Exige CNPJ/CPF-BR | Sim (a Utilar tem CNPJ) | Sim |
| DNS gerenciável | Sim (ou delegar p/ Route53) | Sim |
| E-mail incluso | **Não** (ver §2) | Vende à parte |
| Camada extra / lock-in | Nenhuma | Painel + tentativas de upsell |

**Decisão prática:**
1. Registrar **`utilarferragem.com.br`** no Registro.br (livre em 2026-07). Custo R$ 40/ano.
2. Decidir se registra também o `www` como hábito (é subdomínio, não custa nada — resolve-se no DNS).
3. **Delegar os NS para o Route53** (AWS) — deixa DNS, ALB (alias) e certificado (ACM)
   no mesmo lugar e simplifica o TLS. Alternativa: manter DNS no Registro.br e só
   apontar registros A/CNAME; funciona, mas perde o alias nativo do ALB.
4. `.com` (`utilarferragem.com`) é **opcional** — registrar só se quiser proteger a marca;
   não é necessário para operar no Brasil.

---

## 2. E-mail — duas coisas diferentes que costumam se confundir

Há **dois** e-mails, e eles não competem:

### 2a. Caixa de e-mail (pessoas leem/escrevem) — `contato@utilarferragem.com.br`
Registro.br **não** entrega caixa. Opções:

| Provedor | Preço | Observação |
|---|---|---|
| **Google Workspace** Business Starter ✅ | **R$ 34/usuário/mês** | Gmail com domínio próprio, Drive, Meet. Padrão de mercado. |
| Zoho Mail (Forever Free) | **R$ 0** (até 5 usuários, 5 GB, 1 domínio) | Ótimo para começar sem custo; migra depois. |
| Microsoft 365 Business Basic | ~R$ 37/usuário/mês | Se a loja já vive no Outlook/Office. |

**Recomendação:** começar no **Zoho Free** (R$ 0) se orçamento é crítico, ou já ir de
**Google Workspace** com 2–3 caixas (`contato@`, `financeiro@`, `thomaz@`) ≈ R$ 68–102/mês.

### 2b. E-mail transacional (o sistema envia sozinho) — confirmação de pedido, NFC-e, recuperação de senha
Isso **não** sai da caixa humana — sai de um serviço de envio:

- **AWS SES** ✅ — ~US$ 0,10 por 1.000 e-mails. No começo é ~**R$ 5/mês**. Já está na AWS.
- Precisa: verificar o domínio no SES, configurar **SPF + DKIM + DMARC** no DNS
  (senão cai em spam), e sair do *sandbox* do SES (pedido simples à AWS).
- A NFC-e por e-mail (modelo "sem caixa, nota no e-mail" que o dono quer) passa por aqui.

---

## 3. AWS — arquitetura de produção (EC2 + ALB + RDS)

O desenho que o dono propôs — **1 EC2 + 1 ALB + 1 RDS PostgreSQL** — é exatamente
o padrão certo para esta escala. Não usar ECS/EKS/Fargate/Lambda: escala **vertical**
primeiro (mesma disciplina do blueprint gifthy). Região **`sa-east-1` (São Paulo)**.

```
                          Internet
                             │
                   ┌─────────┴─────────┐
                   │                   │
          CloudFront (CDN)         Route53 (DNS)
          utilarferragem.com.br   utilarferragem.com.br → CloudFront
                   │                api.utilarferragem.com.br → ALB
             S3 (SPA React)
                                        │
                              (WAF opcional) → ALB (HTTPS 443, redirect 80→443)
                                        │              ACM: *.utilarferragem.com.br
                                        ▼
                          ┌──────────────────────────────┐
                          │  EC2  t3.medium/large         │
                          │  docker-compose:              │
                          │   nginx (reverse-proxy) :443  │
                          │   ├ catalog   :8091           │
                          │   ├ order     :8092           │
                          │   ├ auth       :8093          │
                          │   ├ payment    :8090          │
                          │   ├ assistant  :8094 (Alice)  │
                          │   ├ redis      :6379          │
                          │   └ redpanda   :19092         │
                          └───────────────┬──────────────┘
                                          │ (SG: só a EC2 acessa o RDS)
                                          ▼
                          RDS PostgreSQL (single-AZ, privado)
                          4 bancos lógicos:
                            catalog_service · order_service
                            auth_service · payment_service
                                          
   S3: utilar-spa-prod · utilar-dumps (backups) · utilar-alb-access-logs · cloudtrail-logs
   ECR: catalog · order · auth · payment · assistant  (imagens Docker)
```

**Decisões de arquitetura que importam:**
- **1 RDS com 4 bancos lógicos**, não 4 instâncias (o dev local usa 4; produção
  consolida — economia grande, mesmo isolamento lógico). A Alice não tem banco próprio.
- **RDS privado** (só o Security Group da EC2 acessa) — mais seguro que expor.
- **nginx na EC2 faz o path-routing** para os 5 serviços; o ALB manda tudo para um
  target group (porta do nginx). Menos peças que host-routing com 5 target groups.
- **SPA no S3 + CloudFront** (não servido pela EC2) — `scripts/deploy.sh` já faz isso.
- **Elastic IP** na EC2 (a AWS hoje cobra por todo IPv4 público, ~US$ 3,6/mês).
- **SES + SPF/DKIM/DMARC** no DNS para o transacional não cair em spam.

### 3a. Setup da conta AWS (ordem importa)
- [ ] Criar conta **standalone** (e-mail próprio + cartão próprio, **fora de qualquer Organization** — senão perde o free tier; ver `utilar-dedicated-aws-account`).
- [ ] Root com **MFA**, criar usuário IAM admin, **billing alerts** (Budgets).
- [ ] VPC default serve. Security Groups: EC2 (22 do seu IP, 80/443 do ALB), RDS (5432 só da EC2).
- [ ] ACM: cert em `sa-east-1` (ALB) **e** em `us-east-1` (CloudFront exige lá).
- [ ] RDS PostgreSQL 17/18, single-AZ, privado, 4 bancos, backups automáticos ligados.
- [ ] EC2 + EIP + Docker + docker-compose. ECR (5 repos).
- [ ] CloudTrail → S3. Cron de custo (Cost Explorer → e-mail).

### 3b. Pendências de CÓDIGO antes do deploy (não são AWS)
1. **Dockerfiles** dos 5 serviços Go (multi-stage → distroless ~15 MB) — **não existem**.
2. **`docker-compose.prod.yml`**: 5 serviços + Redis + Redpanda + nginx, apontando
   para o RDS externo, com env de prod (`DEV_MODE=false`, `JWT_SECRET` forte,
   `SERVICE_JWT_SECRET`, `ALLOWED_ORIGINS`, `APPMAX_*`, `PSP_PROVIDER=appmax`).
3. **`deploy.sh` estendido**: hoje só o SPA (S3+CloudFront). Falta o caminho backend
   (build+push ECR → ssh EC2 → `docker compose pull && up -d`).
4. **Boot fail-closed** já existe (recusa subir sem segredo forte) — garantir env completo.

---

## 4. Observabilidade — Sentry + CloudWatch

O sistema já expõe **métricas Prometheus** (`pkg/metrics`) e tem **auditoria
append-only**. Falta o "olho externo" que avisa quando algo quebra em produção.

| Camada | Ferramenta | Custo |
|---|---|---|
| **Erros de aplicação** (Go + React) | **Sentry** | Free (5k erros/mês, 1 usuário) → **Team US$ 26/mês** quando crescer |
| **Logs e métricas de infra** | CloudWatch (AWS) | ~US$ 3–10/mês (já incluso no custo AWS) |
| **Uptime externo** (site caiu?) | UptimeRobot / BetterStack | Free → ~US$ 7/mês |
| **Trilha de auditoria** | já no sistema (hash encadeado) | R$ 0 |

**Recomendação:** ligar o **Sentry no plano free** desde o dia 1 (front + os 5
serviços Go). Custa R$ 0 e é a diferença entre "o cliente reclamou" e "o Sentry me
avisou antes". Subir para Team quando o volume de erros passar do free.

---

## 5. Segredos — 1Password e/ou AWS Secrets Manager

São **dois problemas diferentes**:

| Tipo de segredo | Onde guardar | Custo |
|---|---|---|
| **Segredos de aplicação** (`JWT_SECRET`, `SERVICE_JWT_SECRET`, senha do RDS, `APPMAX_*`) | **AWS Secrets Manager** (a EC2 lê por IAM role, nada em arquivo) | ~US$ 0,40/segredo/mês → ~US$ 3–4/mês |
| **Credenciais humanas** (login AWS root, Registro.br, Appmax, Google Workspace, banco do dono) | **1Password** (ou Bitwarden) | 1Password Teams ~US$ 19,95/mês · Bitwarden ~US$ 3/usuário · Bitwarden free p/ 1 pessoa |

**Recomendação:** **1Password** para as pessoas (o dono e você compartilham um cofre
"Utilar" — separado de qualquer cofre gifthy) **+ Secrets Manager** para a aplicação.
Se orçamento aperta, **Bitwarden** cobre o lado humano bem mais barato. O importante,
pela regra permanente: **segredo nunca versionado**, e o cofre da Utilar é só da Utilar.

---

## 6. Appmax — gateway de pagamento

Já decidido e implementado no código (**tudo Appmax sempre**). Falta só o
**operacional do dono**:

- **Sem mensalidade, sem setup** — cobra só **% por transação aprovada** (MDR).
  Custo 100% proporcional à receita. Taxas (Pix, boleto, cartão à vista/parcelado)
  negociadas direto com a Appmax pelo volume.
- **Integração pronta** server-side (Pix, boleto, cartão, split). Assim que a conta
  da Utilar existir: ligar credencial → validar em sandbox → produção.
- **Webhook não assinado** → a integridade vem da **reconsulta autenticada ao PSP**
  (já implementado). Não confiar no corpo do postback.
- ⚠️ **Hoje o `/health` do payment acusa `degraded`** porque a chave de teste do
  Stripe expirou — Pix/boleto retornam 502. **Isso some quando a conta Appmax entrar.**
- ⚠️ **Estorno real no PSP ainda não existe** (só o lançamento contábil). Enquanto
  não implementado, estorno é feito pelo painel da Appmax. Ver §9.

---

## 7. Arquitetura completa do sistema (o que vai para produção)

- **SPA React** (loja `/`, balcão `/balcao`, admin `/admin`) → S3 + CloudFront.
- **5 microserviços Go**, cada um dono do seu schema, **sem acesso cruzado** ao banco
  do outro (falam por API) — contém o estrago de uma invasão:
  - **auth** :8093 — usuários, papéis, lojas, operadores, JWT (access 15 min + refresh).
  - **catalog** :8091 — produtos, estoque, reservas, importação, imagens, busca pt-BR.
  - **order** :8092 — pedidos, frete, balcão, fulfillment, devolução.
  - **payment** :8090 — PSP (Appmax), webhooks, outbox, **livro contábil** (partidas dobradas).
  - **assistant** :8094 — Alice (Claude Sonnet; sem banco próprio; **não** recebe `SERVICE_JWT_SECRET`).
- **Redpanda** (Kafka-compat) — evento `payment.confirmed` → consumer do order → baixa de estoque.
- **Redis** — rate limit, idempotência.
- **Segurança já implementada:** lock HS256, dois segredos (usuário vs serviço),
  auditoria com hash encadeado, contábil soma-zero por constraint, IP mascarado (LGPD),
  fail-closed no boot, `DEV_MODE` bloqueado em prod por sinais de produção.

**Ainda aberto (segurança):** assinatura **assimétrica** (auth assina com chave
privada, os demais só verificam com a pública) é a solução definitiva do A1 — hoje
mitigado por segredo separado. **MFA para admin** e **bloqueio após N tentativas**
foram pedidos e ainda não feitos.

---

## 8. Prospecção de custo inicial

Câmbio R$ 5,40/US$. Impostos AWS Brasil (~13,8%) já embutidos nos totais AWS.

### 8a. Custo ÚNICO (setup)
| Item | Custo |
|---|---:|
| Registro `utilarferragem.com.br` (1º ano) | R$ 40 |
| Certificado TLS (ACM) | R$ 0 |
| Provisionamento AWS + Dockerfiles + deploy backend + hardening | esforço de engenharia (interno) |
| **NFC-e** (do zero: 60–80 h · ou integração com sistema fiscal existente: ~20 h) | esforço de engenharia |

### 8b. Custo RECORRENTE — três cenários de AWS

**Cenário A — Ano 1, conta standalone com free tier** (EC2/RDS elegíveis + créditos):
| Componente | US$/mês | R$/mês |
|---|---:|---:|
| EC2 (micro/small, free tier) | ~0–10 | ~0–54 |
| RDS db.t3.micro 20 GB (free tier) | ~0 | ~0 |
| ALB (não zera no free tier) | ~22 | ~119 |
| IPv4 + EBS + S3 + transfer | ~15 | ~81 |
| **Total AWS ano 1** | **~US$ 37–47** | **~R$ 200–254** |

**Cenário B — Largada dedicada** (o desenho pedido: EC2 t3.medium + ALB + RDS db.t3.small single-AZ, on-demand):
| Componente | US$/mês | R$/mês |
|---|---:|---:|
| EC2 t3.medium (2 vCPU, 4 GB) | ~39 | ~211 |
| RDS db.t3.small single-AZ + storage | ~99 | ~535 |
| ALB | ~24 | ~130 |
| IPv4 + EBS + snapshots + S3 + transfer | ~28 | ~151 |
| Impostos AWS (~13,8%) | ~26 | ~142 |
| **Total AWS largada** | **~US$ 189** | **~R$ 1.020** |

**Cenário C — Produção completa** (t3.large + ALB + WAF + RDS 200 GB) — o orçamento antigo:
| Variante | R$/mês |
|---|---:|
| On-demand | **R$ 1.703** |
| Com reserva 1 ano (Savings Plan) ✅ | **R$ 1.253** |

### 8c. SaaS recorrente (fora da AWS)
| Item | R$/mês |
|---|---:|
| Domínio (rateio do anual) | ~3 |
| Google Workspace (3 caixas) *ou* Zoho Free | 102 *ou* 0 |
| AWS SES (transacional) | ~5 |
| Sentry | 0 (free) → ~140 (Team) |
| 1Password Teams *ou* Bitwarden | ~108 *ou* ~16 |
| AWS Secrets Manager | ~22 |
| **Subtotal SaaS (enxuto → completo)** | **~R$ 45 → ~R$ 380** |

### 8d. Total recorrente consolidado
| Fase | AWS | SaaS | **Total/mês** |
|---|---:|---:|---:|
| **Ano 1 enxuto** (free tier + Zoho + Sentry free + Bitwarden) | ~R$ 230 | ~R$ 45 | **~R$ 275–350** |
| **Regime normal** (largada dedicada + Google WS + Sentry Team + 1Password) | ~R$ 1.020 | ~R$ 380 | **~R$ 1.400** |
| **Produção completa reservada** | ~R$ 1.253 | ~R$ 380 | **~R$ 1.630** |

> **+ Appmax:** % por venda (proporcional à receita) — não entra no custo fixo.

---

## 9. Pendências de código que eu posso fazer (para o go-live)

Prioridade para vender com segurança:
1. **Dockerfiles + `docker-compose.prod.yml` + `deploy.sh` backend** — sem isso não há deploy.
2. **Estorno real no PSP** (`psp.Gateway.Refund()` + `appmaxv1` + webhook `order_refund`) — hoje só o lançamento contábil existe.
3. **`POST /internal/restock`** no catalog — a devolução precisa devolver estoque; a rota não existe (só `Release`, que é para reserva).
4. **Tabela de frete do RS** — está semeada com CEP de **SP** (`01000000–05999999`), ERRADA. Refazer para `90000000–99999999` quando souber a cidade.
5. **Assinatura assimétrica** (definitiva do A1), **MFA admin**, **bloqueio de conta**.
6. **Favoritos com backend** (hoje só localStorage; merge no login já pronto no front).
7. **Seed do order-service com `user_id` desconectado do auth** — corrigir na origem.

---

## 10. Bloqueios que dependem do dono (não código)

1. **Criar a conta Appmax** — gargalo nº 1. Sem gateway não há venda.
2. **Criar a conta AWS standalone** (fora de Organization, senão perde o free tier).
3. **Registrar o domínio** no Registro.br (exige o CNPJ da Utilar).
4. **NFC-e** — obrigatória por lei para o balcão. Pergunta que reduz o esforço:
   *a Utilar já emite nota por outro sistema hoje?* Se sim, vira integração (~20 h)
   em vez de emissão do zero (60–80 h). No RS, a nota por e-mail é válida, **mas o
   estado exige vincular o pagamento à NFC-e** (Decreto 56.670/2022) — confirmar com o contador.
5. **Marketplace ou lojista?** — muda Termos, responsabilidade e devolução (a Appmax
   proíbe estorno parcial em pedido com split). Decidir antes de vender.
6. **Backup do RDS nunca restaurado** — backup não testado é backup que não existe. Testar um restore antes do go-live.

---

## 11. Cronograma sugerido (caminho crítico)

| # | Etapa | Depende de | Esforço |
|---|---|---|---|
| 1 | Conta AWS standalone + billing alerts + MFA | dono | 1 h |
| 2 | Registrar domínio (Registro.br) + delegar Route53 | dono (CNPJ) | 1 h + propagação |
| 3 | Conta Appmax + credencial em sandbox | dono | dias (aprovação Appmax) |
| 4 | E-mail (Zoho/Google) + SES + SPF/DKIM/DMARC | domínio | 2 h |
| 5 | Dockerfiles + compose.prod + deploy.sh backend | engenharia | 1–2 dias |
| 6 | Provisionar EC2 + ALB + RDS + ACM + S3/CloudFront | conta AWS | 1–2 dias |
| 7 | Sentry (free) nos 5 serviços + front | — | meio dia |
| 8 | Secrets Manager + 1Password | conta AWS | meio dia |
| 9 | Estorno PSP + restock + frete RS | engenharia | 2–3 dias |
| 10 | NFC-e | decisão fiscal | 20–80 h |
| 11 | Testar restore do RDS + smoke E2E (catálogo→checkout→pedido) | tudo acima | 1 dia |

**Caminho crítico real:** a conta Appmax (item 3) e a NFC-e (item 10) são os que
travam a venda legal. Todo o resto a engenharia entrega em ~1 semana.

---

*Fontes de custo AWS: fatura real medida (gifthy, `sa-east-1`, jun/2026) via Cost
Explorer, extrapolada para a stack Utilar. Tarifas SaaS: tabelas públicas dos
provedores em jul/2026. Recalcular câmbio e tarifas no fechamento do contrato.*
