# 13 — Checklist de lançamento

**Status**: Rascunho. **Data**: 2026-04-20.

Tudo o que precisa ser verdade antes e imediatamente após a Utilar Ferragem entrar no ar. Escopo: T-30 dias até T+7 dias. Cada item tem um responsável (fundador, salvo indicação contrária) e uma classificação de bloqueio total / nice-to-have.

Leituras complementares:
- [05-roadmap.md](05-roadmap.md) — cronograma de sprints que alimenta esta linha do tempo
- [08-security.md](08-security.md) — gates de segurança
- [09-observability.md](09-observability.md) — gates de observabilidade
- [10-testing-strategy.md](10-testing-strategy.md) — gates de testes
- [11-infra.md](11-infra.md) — tarefas de infra
- [12-ops-runbook.md](12-ops-runbook.md) — o runbook deve existir antes do lançamento

---

## 1. Linha do tempo resumida

| Marco | Offset de data | O que deve estar feito |
|-----------|-------------|-------------------|
| **T-30 dias** | | Feature freeze, início do push de conteúdo |
| **T-21 dias** | | Revisão jurídica concluída |
| **T-14 dias** | | Staging = paridade com prod; suíte E2E completa verde |
| **T-10 dias** | | Soft launch (amigos & família, 30 contas) |
| **T-7 dias** | | Smoke em produção, ensaio de observabilidade |
| **T-3 dias** | | SES fora do sandbox, propagação de domínio confirmada |
| **T-1 dia** | | Smoke final; chamada de go/no-go |
| **T-0 (dia do lançamento)** | | Público; fundador de plantão dedicado |
| **T+1 a T+7** | | Revisão diária de KPIs, janela de hot-patch |
| **T+7** | | Primeira retro; avaliar critérios de saída |

---

## 2. Jurídico (T-30 a T-21)

Todos os itens são **bloqueio total** — não é possível lançar sem eles.

| Item | Responsável | Status |
|------|-------|--------|
| Termos de Uso redigidos | Fundador | ⬜ |
| Política de Privacidade redigida | Fundador + DPO | ⬜ |
| Divulgações específicas de LGPD | Fundador + DPO | ⬜ |
| Política de Cookies | Fundador | ⬜ |
| Política de reembolso e devolução (direito de arrependimento CDC de 7 dias + políticas dos vendedores) | Fundador | ⬜ |
| Contrato do vendedor / termos do marketplace | Fundador | ⬜ |
| Revisão jurídica por advogado brasileiro | Externo | ⬜ |
| Documentos publicados em `/termos`, `/privacidade`, `/cookies`, `/devolucao`, `/vender/termos` | Fundador | ⬜ |
| Banner de consentimento no ar + testado | Fundador | ⬜ |
| Contato do DPO publicado | Fundador | ⬜ |
| Estrutura jurídica (MEI → LTDA se receita projetada > R$ 81k/ano) | Contador | ⬜ |
| Certificado digital e-CNPJ (A1 ou A3) para emissão de NF-e | Contador | ⬜ |
| Plano de resposta a incidentes ANPD aprovado | Fundador + DPO | ⬜ |
| Busca e registro de marca "Utilar Ferragem" + logotipo | Advogado | ⬜ |

---

## 3. SEO & conteúdo (T-30 a T-7)

### 3.1 SEO técnico

| Item | Responsável | Status |
|------|-------|--------|
| `sitemap.xml` gerado automaticamente no build (todas as rotas de categoria + PDP) | Fundador | ⬜ |
| `robots.txt` configurado (permitir tudo público, bloquear `/account/*`, `/admin/*`) | Fundador | ⬜ |
| `schema.org/Product` JSON-LD no PDP (nome, imagem, descrição, marca, SKU, preço, disponibilidade, aggregateRating quando houver avaliações) | Fundador | ⬜ |
| `schema.org/Offer` com `priceCurrency: "BRL"` + `priceValidUntil` | Fundador | ⬜ |
| `schema.org/BreadcrumbList` em categoria + PDP | Fundador | ⬜ |
| Tags OpenGraph em todas as páginas (`og:title`, `og:description`, `og:image`, `og:url`, `og:type`) | Fundador | ⬜ |
| Twitter cards (`twitter:card="summary_large_image"`) | Fundador | ⬜ |
| URLs canônicas em todas as rotas (`<link rel="canonical">`) | Fundador | ⬜ |
| Tags `hreflang` para versões `pt-BR` + `en` | Fundador | ⬜ |
| Plano de redirecionamentos 301 (apex → www; non-www se for por esse caminho) | Fundador | ⬜ |
| Lighthouse SEO ≥ 90 em home + PDP + categoria | Fundador | ⬜ |
| Google Search Console verificado (registro TXT) | Fundador | ⬜ |
| Bing Webmaster Tools verificado | Fundador | ⬜ |
| `sitemap.xml` submetido aos dois | Fundador | ⬜ |
| Propriedade GA4 criada + Measurement ID conectado | Fundador | ⬜ |
| Container GTM configurado (opcional, facilita integrações de pixel futuras) | Fundador | ⬜ |
| Core Web Vitals passando no mobile (LCP < 2,5s, INP < 200ms, CLS < 0,1) | Fundador | ⬜ |

### 3.2 Marketing de conteúdo

| Item | Responsável | Status |
|------|-------|--------|
| 5 guias de compra iniciais publicados (`/guias/`) | Fundador | ⬜ |
| — "Como escolher a furadeira certa" | | |
| — "Checklist de EPI para obra residencial" | | |
| — "Guia de tintas: fosca, acetinada, brilhante" | | |
| — "Cabo elétrico: bitola para cada circuito" | | |
| — "Tubos e conexões: PVC, CPVC, PPR quando usar cada um" | | |
| Texto hero de categoria (título + subtítulo + meta description) para 8 categorias de nível superior | Fundador | ⬜ |
| Descrições de 200 palavras nos 8 produtos destaque (para SEO + compartilhamento social) | Fundador | ⬜ |
| Favicon + imagens OG (1200×630) por categoria | Fundador | ⬜ |
| Texto alternativo em todas as imagens de produtos (começando pelos dados de seed) | Fundador | ⬜ |

---

## 4. E-mail (T-21 a T-7)

| Item | Responsável | Status |
|------|-------|--------|
| Identidade de domínio SES verificada para `utilarferragem.com.br` | Fundador | ⬜ |
| Registros DKIM (3 CNAMEs) publicados no Route53 | Fundador | ⬜ |
| Registro SPF publicado (`v=spf1 include:amazonses.com -all`) | Fundador | ⬜ |
| Registro DMARC publicado (`v=DMARC1; p=quarantine; rua=mailto:dmarc@utilarferragem.com.br`) | Fundador | ⬜ |
| Solicitação de acesso à produção do SES (fora do sandbox) enviada e aprovada | Fundador | ⬜ |
| Reputação do remetente iniciada: plano de aquecimento (volume inicial baixo, máx. 200/dia na semana 1) | Fundador | ⬜ |
| IP dedicado (opcional — permanecer no compartilhado na Fase 3) | — | — |
| Templates construídos + revisados em pt-BR e en: | Fundador | ⬜ |
| — `welcome.html` (confirmação de cadastro) | | |
| — `order_confirmation.html` | | |
| — `payment_pending_boleto.html` (com URL do boleto) | | |
| — `payment_pending_pix.html` (com QR e expiração) | | |
| — `payment_confirmed.html` | | |
| — `order_shipped.html` (com código de rastreio) | | |
| — `order_delivered.html` | | |
| — `password_reset.html` | | |
| — `email_changed_alert.html` | | |
| — `lgpd_data_export_ready.html` | | |
| — `lgpd_deletion_scheduled.html` | | |
| Envio de teste para Gmail pessoal + Outlook + Yahoo, verificar pasta de spam | Fundador | ⬜ |
| Link de cancelamento em e-mails de marketing (revogação de consentimento CAN-SPAM + LGPD) | Fundador | ⬜ |
| E-mails transacionais marcados como não-canceláveis (CAN-SPAM permite isso para conta/pedido) | Fundador | ⬜ |
| Webhook de bounce e reclamação → ação automática de pausa do remetente | Fundador | ⬜ |

---

## 5. Suporte ao cliente (T-14 a T-7)

| Item | Responsável | Status |
|------|-------|--------|
| `suporte@utilarferragem.com.br` configurado (SES inbound ou Google Workspace) | Fundador | ⬜ |
| Formulário de suporte em `/ajuda` — envia para inbox de suporte com contexto (usuário, pedido, URL) | Fundador | ⬜ |
| Horário de atendimento publicado: 9:00–18:00 BRT seg-sex | Fundador | ⬜ |
| Página de FAQ com as 10 principais perguntas (pagamento, envio, devolução, CNPJ, LGPD) | Fundador | ⬜ |
| SLA de primeira resposta definido (4h em horário comercial; 24h fora do horário) | Fundador | ⬜ |
| Template de auto-resposta quando e-mail chega | Fundador | ⬜ |
| Ferramenta de helpdesk: adiada — usar label compartilhado no Gmail na Fase 3; Zendesk/Freshdesk na Fase 4 | Fundador | ⬜ |
| Respostas prontas para os 10 cenários principais | Fundador | ⬜ |
| Caminho de escalação: suporte → fundador → jurídico para disputas | Fundador | ⬜ |

---

## 6. Primeiros vendedores (T-30 a T-10)

A Utilar lança com **pelo menos 3 vendedores × 20 produtos cada = 60 listagens ativas**.

| Item | Responsável | Status |
|------|-------|--------|
| 3 parceiros vendedores assinados (LOI ou verbal + follow-up escrito) | Fundador | ⬜ |
| CNPJ de cada vendedor verificado manualmente na Receita Federal | Fundador | ⬜ |
| 20 produtos por vendedor cadastrados com: fotos reais (≥3 por SKU), category_path correto, specs precisas, precificação em BRL, quantidade em estoque | Fundador + vendedores | ⬜ |
| Chamada de onboarding realizada com cada vendedor (45 min) cobrindo: tour do dashboard, fulfillment de pedidos, repasses, fluxo de devolução | Fundador | ⬜ |
| Tarifas de frete configuradas por vendedor (prefixos de CEP + transportadora + preço + prazo) | Fundador + vendedores | ⬜ |
| Pedido de teste ponta a ponta com cada vendedor (Pix + boleto + cartão); dinheiro realmente movimenta (menor transação real R$ 1) | Fundador | ⬜ |
| Cada vendedor comprometido com SLA de reconhecimento de pedido em 24h | Fundador | ⬜ |
| Números de telefone de contato dos vendedores no DB do admin | Fundador | ⬜ |

---

## 7. Suíte de smoke em produção (T-7 a T-1)

Executar cada cenário ponta a ponta **contra produção**, dinheiro real, valor mínimo possível.

| Cenário | Método | Status |
|----------|--------|--------|
| Cadastrar novo cliente + CPF válido | Manual | ⬜ |
| Cadastro falha com elegância com CPF inválido | Manual | ⬜ |
| Autofill de CEP funciona + cai no manual quando offline | Manual | ⬜ |
| Navegar pelo catálogo, filtrar por spec, paginar | Manual | ⬜ |
| PDP renderiza todos os 4 schemas de spec de categoria | Manual | ⬜ |
| Adicionar ao carrinho + persistência após atualização da página | Manual | ⬜ |
| Checkout com **Pix** (R$ 1, cartão pessoal no vendedor parceiro) | Manual | ⬜ |
| Pix confirma via webhook; e-mail chega | Manual | ⬜ |
| Checkout com **boleto**; PDF abre; (simular pagamento via produção MP) | Manual | ⬜ |
| Pagamento do boleto confirma via webhook (pode levar 1-2 dias úteis — agendar antes) | Manual | ⬜ |
| Checkout com **cartão** (cartão real, menor transação real) | Manual | ⬜ |
| Fluxo de cartão recusado exibe erro útil | Manual | ⬜ |
| Transições de status do pedido: vendedor marca `shipped` no gifthy-hub → Utilar mostra enviado em até 10s | Manual | ⬜ |
| Status do pedido `delivered` → e-mail chega | Manual | ⬜ |
| Solicitação de exportação de dados LGPD → e-mail chega com URL assinada → conteúdo do ZIP correto | Manual | ⬜ |
| Fluxo de solicitação de exclusão LGPD → e-mail de carência → cancelamento funciona | Manual | ⬜ |
| Logout limpa sessão; refresh token revogado | Manual | ⬜ |

Se qualquer item P0 (checkout, cadastro, status do pedido) falhar → **sem lançamento**. Itens P2 (typo no template de e-mail) podem ser lançados e corrigidos com hot-patch no dia 1.

---

## 8. Observabilidade (T-10 a T-3)

| Item | Responsável | Status |
|------|-------|--------|
| Projetos Sentry no ar (frontend, backend — 4 serviços); releases conectados ao CI | Fundador | ⬜ |
| CloudWatch Dashboards criados (saúde da API, funil de pagamento, ops de vendedor, Kafka/infra) | Fundador | ⬜ |
| Alertas P0 configurados + testados ao disparar propositalmente um (5xx falso em staging) | Fundador | ⬜ |
| Pager SMS (Twilio) recebendo alerta de teste | Fundador | ⬜ |
| Telefone do plantão carregado, campainha ligada | Fundador | ⬜ |
| Entradas do runbook impressas ou salvas nos favoritos para consulta offline | Fundador | ⬜ |
| Monitores UptimeRobot configurados (site + API + busca + checkout sintético) | Fundador | ⬜ |
| Página de status no ar em `status.utilarferragem.com.br` | Fundador | ⬜ |
| Metas de SLO publicadas internamente | Fundador | ⬜ |
| Suíte Playwright E2E nightly rodando contra staging | Fundador | ⬜ |
| Smoke E2E (`@smoke`) bloqueando deploys em produção | Fundador | ⬜ |
| Regras de redação de logs testadas (grep por CPF no CloudWatch — zero resultados) | Fundador | ⬜ |

---

## 9. Marketing de lançamento (T-10 a T+7)

### 9.1 Soft launch (T-10 a T-0)

| Item | Responsável | Status |
|------|-------|--------|
| 30 contas de amigos & família, cada uma fazendo ≥ 1 pedido com dinheiro real | Fundador | ⬜ |
| Feedback coletado via Google Form de 5 minutos | Fundador | ⬜ |
| Bugs críticos encontrados → hot-patch durante a janela de soft launch | Fundador | ⬜ |
| 10 avaliações no Google / Instagram solicitadas dos clientes do soft launch (pós-lançamento) | Fundador | ⬜ |

### 9.2 Lançamento público (T-0 a T+7)

| Item | Responsável | Status |
|------|-------|--------|
| Contas sociais no ar: Instagram `@utilar_ferragens`, Twitter `@utilarferragem` | Fundador | ⬜ |
| Conteúdo do post de lançamento preparado (carrossel, reel, templates de stories) | Fundador | ⬜ |
| Link na bio do Instagram → landing page | Fundador | ⬜ |
| Conta Google Ads no ar com campanha inicial: "ferramentas baratas são paulo" + 20 outras palavras-chave, orçamento R$ 50/dia | Fundador | ⬜ |
| Conta Meta Ads no ar; pixel de retargeting em home + PDP (somente após consentimento) | Fundador | ⬜ |
| Outreach para imprensa redigido: Estadão Link, Valor Econômico, StartSe, Época Negócios | Fundador | ⬜ |
| Lançamento no Product Hunt / similar para audiência BR (opcional) | — | — |
| Post no Linkedin a partir da conta do fundador | Fundador | ⬜ |
| Lista de e-mails (inscrições antecipadas da landing page) recebe e-mail de lançamento | Fundador | ⬜ |
| Post de blog de lançamento no site (`/blog/estamos-no-ar`) | Fundador | ⬜ |

---

## 10. Semana pós-lançamento (T+1 a T+7)

| Item | Cadência | Responsável |
|------|---------|-------|
| Revisão de KPIs às 9:00 BRT cada manhã (pedidos, GMV, ticket médio, conversão de checkout, taxa de sucesso de pagamento, taxa de 5xx, taxa de cadastro, volume de tickets) | Diário | Fundador |
| Triagem Sentry: cada erro com > 5 ocorrências investigado no mesmo dia | Diário | Fundador |
| Inbox de suporte: zero-inbox até 18:00 BRT cada dia | Diário | Fundador |
| Cadência de deploy: até 3 hot-patches/dia nas primeiras 72h; desacelerar para 1/dia após o dia 3 | Conforme necessário | Fundador |
| Plantão: fundador, telefone ligado, sem viagens, sem álcool (a sério) nas primeiras 7 noites | Contínuo | Fundador |
| Retro semanal na sexta T+7: o que quebrou, o que vem a seguir | Uma vez | Fundador |
| Página de status atualizada para todo P0 e P1 | Conforme os eventos | Fundador |
| Atualizar §11 deste doc com o progresso dos critérios de saída | Diário | Fundador |

---

## 11. Critérios de saída

Sucesso = Utilar está "lançada e estável" quando todos os itens abaixo forem verdadeiros:

| Critério | Meta | Real |
|-----------|--------|--------|
| Pedidos pagos nos primeiros 30 dias | ≥ 50 | — |
| Taxa de conversão do checkout (carrinho → pago) | ≥ 1% | — |
| Taxa de sucesso de pagamento | ≥ 95% (Pix ≥ 98%, boleto ≥ 99% quando pago, cartão ≥ 90%) | — |
| Latência p95 do checkout | < 2s | — |
| Zero incidentes P0 com duração > 1h | 7 dias consecutivos | — |
| Taxa de erros no Sentry (sessões de usuário com erros) | < 1% | — |
| Taxa de cumprimento do SLA de primeira resposta de suporte | ≥ 90% | — |
| Chargebacks | < 0,5% dos pedidos pagos | — |
| NPS dos primeiros 30 clientes | ≥ 40 | — |

Atingir todos os 9 critérios dispara o início da Fase 4 (ver [05-roadmap.md](05-roadmap.md) Fase 4).

Não atingir 3 ou mais dispara uma **retrospectiva de lançamento + pausa** antes de qualquer trabalho em novas features.

---

## 12. Chamada de go/no-go (T-1 dia)

30 minutos com o fundador + DPO + pelo menos um assessor externo.

Pauta:

1. Percorrer cada ✅/❌ neste doc, do início ao fim.
2. Qualquer ❌ em item de "bloqueio total" → **no-go**, reagendar para a próxima janela viável.
3. Qualquer bug conhecido em checkout, cadastro ou pagamento → no-go.
4. Ensaio de observabilidade: o fundador simula um incidente; consegue seguir o runbook de forma fria? → no-go se não.
5. Prontidão pessoal do fundador: telefone carregado, sem PTO, família ciente do plantão. → no-go se não.

Documentar a decisão + assinaturas em `docs/launch-gono-go.md`.

---

## 13. Plano de rollback / "des-lançamento"

Se nas primeiras 72h ocorrer um problema catastrófico que não possa ser corrigido em menos de 4h:

1. Colocar o site em modo de manutenção — política de headers de resposta do CloudFront trocada para servir um `/maintenance.html` estático.
2. Postar na página de status.
3. Notificar clientes com pedidos pendentes (Pix/boleto ainda não confirmados) via e-mail — pedir desculpas, reembolsar Pix, cancelar boleto se não pago.
4. **Não** excluir dados — preservar pedidos, pagamentos, avaliações para o pós-mortem.
5. Retornar ao beta privado; aplicar hot-patch; re-executar o go/no-go antes de reabrir.

---

## 14. O que não faremos no lançamento (intencional)

Para evitar expansão do escopo. Cada um desses itens é explicitamente **não bloqueante**.

- Avaliações + notas (Fase 4)
- Promoções / cupons (Fase 4)
- Lista de desejos / favoritos (Fase 4)
- App móvel nativo (Fase 5+)
- Multi-moeda (Fase 5+)
- Parcelamento / BNPL (Fase 4)
- Chat ao vivo (Fase 4)
- Afiliados / indicação (Fase 4)
- UI de repasse para vendedores (Fase 4 — repasses manuais no lançamento)
- Integração com a API de CNPJ da Receita Federal (Fase 4)
- Migração para RDS (Fase 4)
- Deploys blue/green (Fase 4)

---

## 15. Checklist final T-0 (manhã do lançamento)

Versão em papel na parede:

- [ ] Todos os itens ❌ em §2–§9 são agora ✅ ou explicitamente dispensados
- [ ] Pager ligado, volume alto, no bolso
- [ ] Laptop carregado, hotspot carregado, runbook nos favoritos
- [ ] `main` atualizado há 24h, sem novos commits desde o smoke
- [ ] Dashboards abertos em abas do navegador
- [ ] URL da página de status na bio do Twitter/Instagram
- [ ] Posts de lançamento agendados (não auto-postados — botão manual após o primeiro pedido pago)
- [ ] Inbox de suporte vazia
- [ ] Cartão de crédito para Google/Meta Ads não está vencido
- [ ] Água, lanches, janela dedicada de 8 horas sem reuniões
- [ ] Respira fundo.
