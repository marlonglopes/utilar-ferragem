# 08 — Segurança & conformidade

**Status**: Rascunho. **Data**: 2026-04-20.

Postura de segurança e obrigações de conformidade da Utilar Ferragem. Escopo: LGPD, endurecimento de autenticação, rate limiting, integridade de webhooks, segredos, validação de entrada, postura PCI, noções básicas de fraude.

Leituras complementares:
- [07-data-model.md](07-data-model.md) — exatamente quais dados pessoais residem onde
- [06-integration.md](06-integration.md) — variáveis de ambiente e pontos de contato com o gateway
- [../../docs/integration-guide.md](../../docs/integration-guide.md) — contrato JWT da plataforma

---

## 1. Resumo do modelo de ameaças

| Ator | Ameaça principal | Controle principal |
|-------|----------------|-----------------|
| Atacante oportunista | Credential stuffing, SQLi, XSS | Rate limit + bcrypt + ORM + CSP |
| Cliente mal-intencionado | Fraude em pagamento, roubo de conta, estornos abusivos | Limites de velocidade + HMAC em webhook + fila de revisão manual |
| Vendedor mal-intencionado | Listagens falsificadas, manipulação de avaliações, fraude de CNPJ | Validação de CNPJ + aprovação admin + avaliação condicionada à compra |
| Credencial PSP comprometida | Webhook falso → pedidos gratuitos | Assinatura HMAC + allowlist de IP + idempotência |
| Insider / segredo vazado | Exfiltração massiva de dados pessoais | Secrets Manager + log de auditoria + IAM de menor privilégio |
| Requisição de autoridade (LGPD/ANPD) | Exportação / exclusão forçada de dados | Endpoint de exportação LGPD + fluxo de exclusão (§2) |

---

## 2. Postura LGPD

### 2.1 Dados pessoais que mantemos

| Dado | Tabela | Origem | Finalidade | Categoria |
|------|-------|--------|---------|----------|
| Nome | `users.name` | Cadastro | Prestação do serviço | PII |
| E-mail | `users.email` | Cadastro | Login + e-mail transacional | PII |
| CPF | `users.cpf` | Cadastro | Fiscal (NF-e), verificação de fraude | **Sensível** (NI) |
| CNPJ | `sellers.cnpj` | Onboarding de vendedor | Fiscal, verificação de vendedor | Identificador empresarial |
| Telefone | `users.phone` (futuro) | Perfil | Contato para envio/suporte | PII |
| Endereço de entrega | `orders.shipping_*` | Checkout | Cumprimento do pedido | PII |
| Metadados de pagamento | `payments.card_metadata`, `.psp_metadata` | Checkout | Processamento de pagamento, resolução de disputas | Financeiro (não dado de cartão — apenas os 4 últimos dígitos + bandeira) |
| Endereço IP | log de aplicação | Todas as requisições | Prevenção de abuso | PII (sob a LGPD) |
| Fingerprint do navegador | log de aplicação | Todas as requisições | Sinais de fraude | PII |

**Não** armazenamos: PAN completo (número do cartão), CVV, PIN, senhas de banco. Jamais. Apenas tokens PSP.

### 2.2 Base legal (Art. 7 LGPD)

| Finalidade | Base legal |
|---------|-------------|
| Criação de conta + login | Execução de contrato (VII) |
| Cumprimento de pedidos + registros fiscais | Execução de contrato + obrigação legal (II) |
| Prevenção de fraude | Legítimo interesse (IX) com teste de equilíbrio documentado |
| E-mail de marketing | Consentimento explícito (I) — opt-in, revogável |
| Cookies de analytics | Consentimento explícito (I) via banner de consentimento |
| Suporte ao cliente | Execução de contrato (VII) |

### 2.3 Prazos de retenção

| Dado | Retenção | Gatilho |
|------|-----------|---------|
| Perfil da conta (ativo) | Indefinido | Até solicitação de exclusão ou 24 meses sem atividade |
| Perfil da conta (excluído) | 30 dias (soft delete) | Solicitação do usuário |
| Pedidos + notas fiscais | 5 anos | Fiscal (art. 195 CTN) — não pode ser reduzido |
| Registros de pagamento | 5 anos | Mesma obrigação fiscal |
| Logs de acesso (aplicação) | 6 meses | Art. 15 Marco Civil + necessidade operacional |
| Logs de acesso (incidentes de segurança) | 2 anos | Retenção para pós-mortem de incidentes |
| Prova de consentimento para marketing | Até revogação + 2 anos | Art. 8 §5 LGPD |
| Analytics anonimizados | Indefinido | Per Art. 12 — não são dados pessoais após anonimização |

### 2.4 Fluxo de exclusão ("direito ao esquecimento")

Endpoint: `POST /api/v1/users/me/deletion-request` (requer JWT).

Fluxo:

1. O usuário se autentica e envia o motivo (texto livre, opcional).
2. O user-service cria uma linha em `deletion_requests` com `status='pending'` e envia ao usuário um link de confirmação válido por 24h.
3. Após confirmação: `status='confirmed'`, `confirmed_at=NOW()`. Um período de carência de 15 dias começa (o usuário pode cancelar via link no e-mail).
4. Após o período de carência: o cron noturno (`rake lgpd:process_deletions`) executa:
   - Pedidos + pagamentos com obrigação fiscal → **anonimizar** (`name='[apagado]'`, `email=NULL`, `cpf=NULL`, `shipping_*=NULL`, `buyer_cpf=NULL`), preservando `id`, `total_cents`, `created_at`.
   - Avaliações → anonimizar autor (`user_id=NULL`, `author_display='Usuário removido'`).
   - Linha de `users` → exclusão definitiva.
   - Publicar evento `user.deleted` para todos os serviços limparem seus caches.
5. Enviar e-mail de confirmação com recibo final de exclusão (entregue a partir do e-mail retido — e em seguida a coluna de e-mail é zerada).

Responsabilidade por serviço:

| Etapa | Serviço | Tabela |
|------|---------|-------|
| Criar requisição | user-service | `deletion_requests` |
| Anonimizar pedidos | order-service | `orders`, `order_items` |
| Anonimizar pagamentos | payment-service | `payments` |
| Anonimizar avaliações | product-service | `reviews` |
| Exclusão final do usuário | user-service | `users` |

### 2.5 Exportação de dados ("portabilidade")

Endpoint: `GET /api/v1/users/me/export` (requer JWT).

Retorna um ZIP via URL assinada no S3 contendo:

| Arquivo | Origem | Formato |
|------|--------|--------|
| `profile.json` | user-service | JSON |
| `orders.json` | order-service | Array JSON |
| `payments.json` | payment-service | Array JSON (sem dados de cartão) |
| `reviews.json` | product-service | Array JSON |
| `consents.json` | user-service | Log de consentimento |
| `README.txt` | estático | Explica o schema |

Geração: job assíncrono (Sidekiq ou delayed do Rails). O usuário recebe e-mail com URL assinada, válida por 24h. SLA: 15 dias (LGPD Art. 19 §1).

### 2.6 Banner de consentimento

Exigido pela orientação da ANPD para uso de cookies e analytics.

- **Estritamente necessários** (sessão, carrinho, CSRF): sem necessidade de consentimento.
- **Analytics** (GA4, replay de sessão do Sentry se ativado): opt-in, padrão DESLIGADO.
- **Marketing** (pixels de retargeting, Meta/Google Ads): opt-in, padrão DESLIGADO.

Implementação: banner próprio leve (sem SDK de terceiros). Estado armazenado em `localStorage` sob a chave `utilar.consent.v1` + replicado em `users.consent_state` JSONB no login. Link de revogação no rodapé.

### 2.7 DPO + contato ANPD

- E-mail do DPO: `dpo@utilarferragem.com.br` — publicado na Política de Privacidade.
- Notificação de incidente à ANPD em até 72h após a detecção (Art. 48 LGPD).
- Runbook interno em [12-ops-runbook.md](12-ops-runbook.md) §7.

---

## 3. Endurecimento de autenticação

### 3.1 Política de senhas

- Fator de custo bcrypt: **12** (padrão do Rails em produção; `has_secure_password` usa a configuração `BCrypt::Engine.cost`).
- Comprimento mínimo: 10 caracteres. Validação server-side em `user_service/app/models/user.rb`.
- Senhas rejeitadas: lista estática das 1000 senhas mais comuns (verificada no cadastro/alteração).
- Sem rotação forçada (diretriz NIST 800-63B).
- Redefinição de senha: token assinado de uso único, validade de 30 minutos, invalida o token anterior.

### 3.2 JWT

Modelo atual (do guia da plataforma §3): HMAC-SHA256, segredo compartilhado `JWT_SECRET`, sem refresh tokens, validade típica de 24h.

Plano de endurecimento:

| Mudança | Sprint | Justificativa |
|--------|--------|-----------|
| Reduzir access token para 1h | Utilar 08 | Reduzir impacto do roubo de token |
| Adicionar refresh token (opaco, baseado em DB) | Utilar 08 | Necessário após redução do access token |
| Adicionar claim `sid` (ID de sessão) | Utilar 08 | Permite revogação server-side |
| Adicionar `/auth/logout` que revoga o `sid` | Utilar 08 | UX + segurança |
| Rotacionar `JWT_SECRET` em caso de incidente | N/A | Runbook: incrementar versão, dupla verificação durante a transição |

### 3.3 Limites de sessão

- Máximo de 5 refresh tokens ativos por usuário — o mais antigo é descartado.
- Refresh token válido por 30 dias (deslizante); máximo absoluto de 90 dias.
- Endpoint de logout global: `POST /auth/logout-all` revoga todos os `sid` do usuário.

### 3.4 Proteção contra força bruta

- 5 tentativas de login fracassadas / 15min / IP → bloquear o IP por 15min (429).
- 10 tentativas de login fracassadas / 1h / e-mail → bloquear o e-mail por 1h (notificar o usuário por e-mail).
- Implementação: middleware no gateway (Go) usando contadores Redis (ver §4).

---

## 4. Rate limiting

Implementado no gateway (Go) — ponto único de controle, sem duplicação nos serviços Rails.

### 4.1 Middleware

`services/gateway/internal/middleware/ratelimit.go` — arquivo novo (prep Sprint 08).

Biblioteca: [`github.com/ulule/limiter/v3`](https://github.com/ulule/limiter) com store Redis. Algoritmo de token bucket.

### 4.2 Limites

| Escopo | Caminho | Limite | Janela | Justificativa |
|-------|------|-------|--------|-----------|
| Por IP | `POST /auth/login` | 10 | 1 min | Força bruta |
| Por IP | `POST /auth/register` | 5 | 1 hora | Flood de contas falsas |
| Por IP | `POST /api/v1/payments` | 20 | 1 min | Teste de cartão |
| Por IP | `GET /api/v1/marketplace/*` | 300 | 1 min | Proteção contra scraping (generoso — usuários reais navegam) |
| Por IP | `POST /webhooks/psp/*` | 600 | 1 min | Picos legítimos do PSP permitidos; IP-allowlistado separadamente |
| Por token | `POST /api/v1/orders` | 10 | 1 min | Spam de pedidos |
| Por token | qualquer | 600 | 1 min | Teto absoluto |
| Global por caminho | `*` | 5000 | 1 min | Fallback contra DoS |

Resposta: `429 Too Many Requests` + header `Retry-After` + body JSON `{error: "rate_limited", retry_after_seconds: 60}`.

### 4.3 Allowlist

- IPs de webhook do PSP (Mercado Pago publica uma lista estática): bypass no limite de `/webhooks/psp/*`.
- Health checks internos (ALB, Uptime Robot): bypass via header de segredo compartilhado `X-Health-Token`.

---

## 5. Validação de assinatura de webhook

Todo webhook do PSP para `/webhooks/psp/:psp_name` deve:

1. **Assinatura**: HMAC-SHA256 do body bruto usando `PSP_WEBHOOK_SECRET`. Comparado com igualdade em tempo constante. O header de origem varia por PSP:
   - Mercado Pago: `x-signature` (manifesto: `id:<id>;request-id:<req_id>;ts:<ts>;`)
   - Stripe BR: `Stripe-Signature`
   - PagSeguro: `x-authenticity-token` (esquema diferente — requer adaptador)
2. **Janela de timestamp**: rejeitar se `|agora - ts| > 300s` (proteção contra replay).
3. **Idempotência**: upsert por `psp_payment_id`; rejeitar eventos duplicados verificando `psp_metadata->'event_id'` contra um `SET NX` Redis com TTL de 24h.
4. **Allowlist de IP** (lista documentada do Mercado Pago): rejeitar IPs fora da lista com 403, logar em WARN.
5. **Versionamento fixo**: fixar uma versão específica da API do PSP via env var `PSP_API_VERSION`; rejeitar webhooks com header `api_version` diferente.

Regras de rejeição — todas retornam `401 Unauthorized` sem body. Logar em WARN com assinatura truncada.

Caminho da implementação de referência: `services/payment-service/app/controllers/webhooks/psp_controller.rb`.

Fixtures de teste: `services/payment-service/spec/fixtures/webhooks/*.json` com payloads assinados de formato real (ver [10-testing-strategy.md](10-testing-strategy.md) §3).

---

## 6. Gerenciamento de segredos

### 6.1 Armazenamento

**Produção + staging**: AWS Secrets Manager.

Caminhos (hierárquico, prefixo por ambiente):

| Caminho | Conteúdo | Rotacionado a cada |
|------|----------|---------------|
| `/utilar/prod/jwt_secret` | `JWT_SECRET` | 90 dias |
| `/utilar/prod/psp/mercadopago/api_key` | Token de acesso do PSP | 180 dias (ou em caso de comprometimento) |
| `/utilar/prod/psp/mercadopago/webhook_secret` | Chave HMAC | 180 dias |
| `/utilar/prod/db/user_service` | URL + senha | 90 dias |
| `/utilar/prod/db/product_service` | URL + senha | 90 dias |
| `/utilar/prod/db/order_service` | URL + senha | 90 dias |
| `/utilar/prod/db/payment_service` | URL + senha | 90 dias |
| `/utilar/prod/ses/smtp_credentials` | Usuário + senha SMTP do SES | 90 dias |
| `/utilar/prod/sentry/dsn` | DSN do Sentry | na rotação |
| `/utilar/staging/*` | Mesmo formato, valores de sandbox | 180 dias |

**Dev**: arquivos `.env` simples, nunca commitados. `.env.example` commitado com placeholders.

### 6.2 Injeção

As instâncias EC2 buscam os segredos na inicialização do container via um sidecar pequeno (ou `aws secretsmanager get-secret-value` + script de entrypoint). Segredos montados como variáveis de ambiente — nunca gravados em disco.

### 6.3 SLAs de rotação

| Evento | SLA |
|-------|-----|
| Suspeita de comprometimento | **4h** — rotacionar + revogar o antigo |
| Rotação programada | Conforme tabela acima |
| Desligamento de engenheiro | 24h para todos os segredos que poderiam ter acessado |
| Rotação de credencial do PSP | Testar em staging por 48h antes da produção |

### 6.4 Segredos nunca em:

- Histórico do Git (hook pre-commit executa `gitleaks`)
- Logs do CloudWatch (regras de redação em §9)
- Rastreador de erros (Sentry `beforeSend` remove os campos)
- Artifacts de CI (`.gitignore` explícito em `*.env` no Actions)

---

## 7. Validação de entrada

Obrigatória server-side — nunca confie no cliente.

### 7.1 CPF

- `app/validators/cpf_validator.rb` no user-service (espelha `cnpj_validator.rb`).
- Algoritmo Módulo 11 com pesos `[10,9,8,7,6,5,4,3,2]` (primeiro dígito) e `[11,10,9,8,7,6,5,4,3,2]` (segundo dígito).
- Rejeitar sequências inválidas conhecidas: `00000000000`, `11111111111`, ... `99999999999`.
- `before_validation` remove a máscara; armazenamento em 11 dígitos.

### 7.2 CNPJ

Validador existente (ver CLAUDE.md do projeto pai — Sprint 17). Sem alterações.

### 7.3 CEP

- 8 dígitos, validado com regex `/\A\d{8}\z/` após remoção da máscara.
- O autofill de CEP via ViaCEP é informativo — o servidor re-valida os campos de endereço de forma independente (comprimento, UF dentro da lista `BR_STATES`).

### 7.4 Valores monetários

- Armazenados como centavos inteiros. Nunca float.
- `price_cents > 0` imposto via check constraint no DB + validação no modelo.
- Totais calculados somente server-side — o endpoint de pedido recalcula com base nos itens do carrinho; um total enviado pelo cliente é descartado.

### 7.5 Upload de arquivos

Fora do escopo da Fase 3 (imagens de produtos passam pelo seller dashboard do gifthy-hub). A lista de verificação pré-lançamento ainda inclui um limite de tamanho (5MB) + detecção de MIME.

### 7.6 SQL

Somente queries parametrizadas do ActiveRecord. Sem `find_by_sql` com interpolação. Verificação no CI via grep: `rg "find_by_sql.*#\{"` e `rg "where\(['\"].*#\{"` devem retornar zero resultados.

---

## 8. Postura XSS / CSRF / injeção

### 8.1 XSS

- React escapa por padrão; nunca usamos `dangerouslySetInnerHTML` em texto fornecido pelo usuário.
- A descrição do produto suporta Markdown → renderizado via `marked` + `DOMPurify` com allowlist restrita (sem `<script>`, `<iframe>`, `<object>`, atributos `on*`).
- Corpo de avaliações: somente texto simples, sem Markdown.

### 8.2 Content Security Policy

Servida pela política de headers de resposta do CloudFront do SPA.

```
default-src 'self';
script-src  'self' https://www.googletagmanager.com https://sdk.mercadopago.com;
style-src   'self' 'unsafe-inline';
img-src     'self' data: https://*.utilarferragem.com.br https://mpago.li;
connect-src 'self' https://api.utilarferragem.com.br https://viacep.com.br https://sentry.io https://*.sentry.io https://www.google-analytics.com;
font-src    'self' data:;
frame-src   https://*.mercadopago.com https://*.mercadopago.com.br;
object-src  'none';
base-uri    'self';
form-action 'self';
frame-ancestors 'none';
upgrade-insecure-requests;
```

Headers de resposta adicionais:

| Header | Valor |
|--------|-------|
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains; preload` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` |
| `X-Frame-Options` | `DENY` (também via CSP `frame-ancestors`) |

### 8.3 CSRF

- A API é stateless com JWT no header `Authorization` — não vulnerável ao CSRF clássico baseado em cookie.
- Webhooks são protegidos por HMAC (§5).
- O SPA nunca aceita envios de formulário de origem cruzada (a allowlist CORS impõe a origem).

### 8.4 SQLi

Coberto em §7.6.

---

## 9. Logging & redação

Plano detalhado de logging em [09-observability.md](09-observability.md). Regras relevantes à segurança:

Nunca logar:

- CPF, CNPJ (completo)
- Número completo do cartão (que jamais recebemos)
- CVV (que jamais recebemos)
- Senhas (brutas ou com hash)
- Tokens JWT (completos) — logar apenas os últimos 8 caracteres para correlação
- Header `Authorization` (remover na camada de log)
- Body completo do webhook PSP — somente hash + tamanho

Pode-se logar:

- CPF/CNPJ mascarado (`123.***.***-09`)
- Últimos 4 dígitos do cartão + bandeira
- `sub` (ID do usuário), `sid`, `exp` do JWT
- Caminho da requisição, método, status, duração
- Endereço IP (base de legítimo interesse)

Camada de redação: `lograge` no Rails + lista personalizada em `Rails.application.config.filter_parameters`. O gateway Go usa um middleware de encapsulamento que passa para `zap.NewProduction()` com um hook de redação.

---

## 10. Postura PCI

**SAQ-A** (Self-Assessment Questionnaire A) é o objetivo.

Justificativa: dados de cartão **jamais** são transmitidos por, processados em ou armazenados nos nossos servidores. A tokenização hospedada do PSP (drop-in UI / iframe) coleta o cartão e retorna apenas um token. Armazenamos `last4` + `brand` + token PSP em `payments.card_metadata`.

Requisitos que ainda devemos cumprir para SAQ-A:

| # | Requisito | Como atendemos |
|---|-------------|-------------|
| 2 | Senhas padrão de fornecedor | Sem credenciais padrão em produção; runbook [12-ops-runbook.md](12-ops-runbook.md) §3 |
| 8 | IDs únicos por pessoa | AWS IAM por engenheiro, sem contas compartilhadas |
| 9 | Acesso físico | AWS gerencia; documentamos na política |
| 12 | Política de segurança | Mantida em `docs/security-policy.md` (pré-lançamento) |

Atestado anual enviado ao PSP via portal do adquirente.

---

## 11. Prevenção de fraude e abuso

### 11.1 Limites de velocidade (além do rate limiting)

Rastreados por conta, não por IP, usando contadores Redis.

| Ação | Limite | Ação ao atingir |
|--------|-------|----------------|
| Cadastros do mesmo IP / 24h | 10 | Bloquear IP por 24h, sinalizar no admin |
| Pedidos por conta / 24h | 20 | Bloqueio suave do checkout, exigir ticket de suporte |
| Pagamentos falhados por conta / 1h | 5 | 1h de cooldown + alerta por e-mail ao usuário |
| Taxa de recusa de CC por conta / 7d | 40% | Bloquear métodos de cartão, permitir apenas Pix/boleto |
| Endereços de entrega diferentes / conta / 30d | 5 | Fila de revisão admin |

### 11.2 Sinais de roubo de conta

Alerta por e-mail ao titular da conta quando:

- Login de um novo país (GeoIP via MaxMind gratuito ou ranges de IP da AWS).
- Alteração de senha.
- Endereço de entrega adicionado.
- Revogação de refresh-token (logout-all).
- Mais de 3 tentativas de login fracassadas em 1h (informativo).

### 11.3 Fraude de vendedor

- CNPJ verificado na Receita Federal na aprovação do vendedor (passo manual na Fase 3; integração via API na Fase 4).
- As primeiras 5 listagens de cada novo vendedor requerem aprovação admin antes de serem publicadas.
- Formulário de reclamação de marca: `docs/legal/trademark-complaint.md` + inbox `legal@utilarferragem.com.br`.

### 11.4 Chargebacks

- O handler de webhook faz a transição de `payments.status='chargeback'` e sinaliza `orders` para revisão.
- SLA de resposta ao adquirente: 7 dias.
- Três chargebacks em 30 dias por vendedor → suspensão automática, revisão manual.

Runbook completo em [12-ops-runbook.md](12-ops-runbook.md) §8.

---

## 12. Testes de segurança

Detalhamento em [10-testing-strategy.md](10-testing-strategy.md). Destaques:

- `gitleaks` pre-commit + CI — bloqueia segredos vazados
- `bundler-audit` no CI — bloqueia gems com CVEs conhecidos
- `npm audit --audit-level=high` — bloqueia pacotes npm com CVEs conhecidos
- Varredura baseline do ZAP contra staging semanalmente
- Pentest anual por terceiros (linha orçamentária da Fase 4)
- Revisão de segurança em todo PR que toque autenticação, pagamentos ou rotas do gateway

---

## 13. Status

| Controle | Status | Responsável / sprint |
|---------|--------|---------------|
| Endpoint de exclusão LGPD | ⬜ não iniciado | Utilar 08 / pré-lançamento |
| Endpoint de exportação LGPD | ⬜ não iniciado | Utilar 08 / pré-lançamento |
| Banner de consentimento | ⬜ não iniciado | Pré-lançamento |
| Política de Privacidade + ToS publicados | ⬜ não iniciado | Revisão jurídica → pré-lançamento |
| Middleware de rate limit | ⬜ não iniciado | Utilar 08 |
| HMAC de webhook | ⬜ não iniciado | Utilar 08 |
| Caminhos do Secrets Manager | ⬜ não iniciado | Sprint de Infra (ver [11-infra.md](11-infra.md)) |
| Header CSP no ar | ⬜ não iniciado | Utilar 01 (setup S3/CF) |
| Validador de CPF | ⬜ não iniciado | Utilar 07 |
| Custo bcrypt → 12 verificado | ⬜ não verificado | Utilar 07 |
| Limites de velocidade (Redis) | ⬜ não iniciado | Utilar 08 |
| Baseline do ZAP no CI | ⬜ não iniciado | Pré-lançamento |
