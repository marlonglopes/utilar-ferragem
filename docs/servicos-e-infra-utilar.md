# Serviços & Infra do Utilar — o que precisamos provisionar

Lista única dos serviços de terceiro e infra que o Utilar Ferragem precisa para ir ao ar e
para ter observabilidade/auditoria de nível de produção. Serve de checklist de
provisionamento e de mapa de custo.

> ⛔ **Tudo dedicado ao Utilar.** Cada conta aqui é **nova e própria do Utilar** — nunca
> reaproveitar as do gifthy (nem DSN, nem token, nem measurement ID, nem chave). Ver
> `docs/SEPARATION-utilar-vs-gifthy.md`. Todo segredo vive no **1Password do Utilar** e entra
> na aplicação por **variável de ambiente**, sem fallback hardcoded (o gifthy já vazou DSN no
> git — não repetir).

Legenda de status: 🟢 já em uso · 🟡 decidido, falta criar a conta · ⚪ opcional/avaliar.
Custos são **estimativas** (podem mudar por volume/câmbio), não compromisso.

---

## 1. Cofre de segredos

| Serviço | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **1Password** | Cofre único do Utilar: guarda TODO segredo (DSN Sentry, chaves GA, tokens Appmax, senhas de banco, chaves AWS/IAM, credenciais de domínio). Acesso por pessoa, com log de quem viu o quê. É a **fonte** de onde as envs saem pro deploy. | 🟡 criar | ~US$ 8/usuário (plano Teams) |

**Por que 1Password e não `.env` no git:** segredo em `.env`/git vaza (aconteceu no gifthy).
O cofre dá rotação, acesso por pessoa, e um único lugar auditável. As envs de produção são
injetadas a partir dele (ou de um secrets manager que espelha ele), nunca commitadas.

## 2. Pagamento (o caminho do dinheiro)

| Serviço | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **Appmax** | PSP (gateway): Pix, cartão, boleto + Payment Split (v1 AppStore). Conta própria do Utilar (client_id/secret v1 e/ou access-token v3). Hoje o `.env.local` usa o **token de sandbox do gifthy** — trocar. | 🟡 criar | % por transação (sem mensalidade fixa) |

Ver `docs/appmax-v1-appstore.md`. A Appmax **não emite nota fiscal** — ver
`docs/fiscal-para-o-dono.md` (emissor de NF-e é outro serviço, decisão do contador).

## 3. Observabilidade & analytics

| Serviço | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **Sentry** | Report de erro/bug (SPA + os 5 serviços Go), com release/ambiente, tracing e alerta. Org própria do Utilar; 1 projeto front + projeto(s) back. | 🟡 criar | Free até ~5k erros/mês; Team ~US$ 26 |
| **Google / GA4** | Conta Google própria → propriedade GA4 (measurement ID + API secret). Funil de conversão (view→checkout→purchase) e `purchase` server-side via Measurement Protocol. | 🟡 criar | Grátis (GA4 padrão) |

Ver `docs/observabilidade-e-auditoria.md` — o roadmap que consome esses dois. Ambos entram
**env-gated** (no-op enquanto o DSN/ID não existir), então dá pra codar antes de criar as contas.

## 4. E-mail (transacional + marketing)

| Serviço | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **AWS SES** | E-mail **transacional** (confirmação de pedido, recibo de pagamento, aviso de envio, reset de senha). Barato e já dentro da conta AWS dedicada. **Recomendado** para transacional. | 🟡 criar | ~US$ 0,10 / mil e-mails |
| **Mailchimp** (+ Mandrill) | **Marketing/CRM**: automação de **carrinho abandonado** (ataca direto a queda no checkout), pós-compra, win-back, newsletter. Mandrill é o transacional do Mailchimp — só se NÃO usar SES. | ⚪ avaliar | Free até ~500 contatos; pago a partir de ~US$ 13 |

**Recomendação:** SES para transacional (mais barato, já na AWS); Mailchimp **só** se quiser as
automações de marketing/recuperação de conversão. Não usar os dois pro mesmo transacional.
O gifthy usa Mandrill — o Utilar precisa da **própria** conta de qualquer forma.

## 5. Nuvem / infra (AWS, conta dedicada)

| Recurso | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **Conta AWS dedicada** | Standalone, **não** dentro de uma Org (entrar numa Org mata os créditos de free-tier). | 🟡 criar | — |
| Compute (ECS/EC2/Fargate) | Rodar a SPA + 5 serviços Go. | 🟡 | varia; ver orçamento |
| **RDS Postgres** (×4–5) | 1 banco por serviço (payment/catalog/order/auth). Pode ser instâncias separadas ou 1 com bancos lógicos. | 🟡 | maior item do custo |
| **S3** | Mídia de produto (fotos), export contábil, backup. | 🟡 | centavos–poucos US$ |
| **ElastiCache Redis** | Rate limit, idempotência (hoje Redis local). | 🟡 | pequeno |
| **MSK / Redpanda** | Barramento de eventos `payment.confirmed → order` (hoje Redpanda local). | 🟡 | pequeno–médio |
| CloudWatch / Secrets Manager | Logs/métricas de infra e injeção de segredo (espelha o 1Password). | 🟡 | pequeno |

Ver `docs/orcamento-utilar-aws-2026-07.md` e `docs/aws-build-utilar.md` para o dimensionamento
e o número fechado. ⚠️ Os números de custo daqueles docs vieram da conta do **gifthy** — refazer
na conta do Utilar.

## 6. Domínio

| Serviço | Para quê | Status | Custo/ano (est.) |
|---|---|---|---|
| **registro.br** | Domínio `.com.br` do Utilar (loja). É também o `tracePropagationTargets` do Sentry e a **página de vendas** que a Appmax pede no cadastro. | 🟡 criar | ~R$ 40/ano |
| **GoDaddy** | Domínio alternativo/genérico (`.com`) se quiser, e/ou DNS. | ⚪ avaliar | ~US$ 12–20/ano |

## 7. IA (a Alice)

| Serviço | Para quê | Status | Custo/mês (est.) |
|---|---|---|---|
| **Anthropic API** | Backend da Alice (`claude-sonnet-5`). Chave própria do Utilar. Sem chave, a Alice roda em mock com busca real no catálogo. | 🟡 criar | por uso (tokens) |

## 8. Fiscal (nota fiscal) — decisão do dono/contador

| Serviço | Para quê | Status |
|---|---|---|
| **Emissor de NF-e/NFC-e** (Focus NF-e, NFe.io, PlugNotas, eNotas…) | Emitir a nota e falar com a SEFAZ — a Appmax **não** faz isso. Precisa de **certificado A1 e-CNPJ** e dos campos fiscais (NCM/CEST/CSOSN/origem). | ⚪ decisão em aberto |

Ver `docs/fiscal-para-o-dono.md` e `docs/fiscal-implementacao.md`.

---

## Checklist de provisionamento (ordem sugerida)

1. [ ] **1Password** (Utilar) — criar o cofre primeiro; todo o resto guarda segredo nele.
2. [ ] **AWS** (conta dedicada, standalone) — base da infra + SES + Secrets Manager.
3. [ ] **Appmax** (Utilar) — trocar o token de sandbox do gifthy no `.env.local`.
4. [ ] **registro.br** — domínio (a Appmax pede a página de vendas no cadastro).
5. [ ] **Sentry** (Utilar) — org + DSNs → 1Password.
6. [ ] **Google/GA4** (Utilar) — measurement ID + API secret → 1Password.
7. [ ] **Anthropic** — chave da Alice → 1Password.
8. [ ] SES verificado (domínio + DKIM/SPF/DMARC) para e-mail transacional.
9. [ ] ⚪ **Mailchimp** — só se for usar automação de marketing/carrinho abandonado.
10. [ ] ⚪ **GoDaddy** — se quiser o `.com` além do `.com.br`.
11. [ ] ⚪ **Emissor de NF-e** — quando a decisão fiscal fechar.

**Estimativa de piso mensal** (fora AWS de produção, que domina o custo): 1Password (~US$8/usuário)
+ Sentry (Free→US$26) + GA4 (grátis) + SES (centavos) + Anthropic (por uso) + domínios (anuais).
O grosso do custo é **RDS/compute na AWS** — dimensionado em `docs/orcamento-utilar-aws-2026-07.md`.
