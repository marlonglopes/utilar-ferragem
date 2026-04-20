# 05 — Roadmap

**Status**: Rascunho. **Data**: 2026-04-20.

## Formato do cronograma

As fases correm de forma aproximadamente sequencial; os sprints dentro de uma fase podem se sobrepor quando as dependências permitem. As estimativas de duração assumem um engenheiro em tempo integral; escala de forma aproximadamente linear com mais pessoas.

| Fase | Sprints | Duração estimada (1 eng) | Critério para próxima fase |
|------|---------|-------------------------|---------------------------|
| **1 — Fundação** | 01–02 | 2 semanas | Scaffold faz build, lint e testes; design system renderiza; i18n carrega |
| **2 — Catálogo** | 03–05 | 4 semanas | Cliente consegue navegar + encontrar um produto no catálogo seedado |
| **3 — Comércio + lançamento** | 06–09, 22, 23, 24, 25 | 8 semanas | Cliente consegue concluir uma compra real; pronto para produção (observabilidade, CI/CD, LGPD, SEO/legal) |
| **4 — Vertical de vendedores** | 10, 11, 14, 15, 20 | 6 semanas | Vendedores externos se cadastram, importam produtos em lote e vendem com frete real + tratamento de disputas |
| **5 — Crescimento** | 12, 13, 16, 17, 18, 19, 21, 26 | em aberto | Avaliações, contas pro/B2B, cashback, ferramentas de suporte, upgrade de busca, PWA, recomendações, performance |

**MVP lançável** (fim da Fase 3): **~14 semanas** com um engenheiro, **~7 semanas** com dois. Os Sprints 22–25 são de endurecimento pré-lançamento e ficam dentro da Fase 3, embora se apoiem mais em ops/infra.

## Mapa completo de sprints

| # | Sprint | Fase | Tema |
|---|--------|------|------|
| 01 | [Scaffold](sprints/sprint-01-scaffold.md) | 1 | Projeto React/Vite/TS, wiring do gateway, i18n |
| 02 | [Design system](sprints/sprint-02-design-system.md) | 1 | Tema com CSS vars, primitivos, layout shell |
| 03 | [Catálogo](sprints/sprint-03-catalog.md) | 2 | Home, categorias, grade de produtos, paginação |
| 04 | [Detalhe do produto](sprints/sprint-04-product-detail.md) | 2 | PDP, JSONB `products.specs`, ficha técnica |
| 05 | [Busca + filtros](sprints/sprint-05-search-filters.md) | 2 | ILIKE + pg_trgm, filtros facetados, chips |
| 06 | [Carrinho](sprints/sprint-06-cart.md) | 3 | Carrinho em localStorage, drawer, agrupamento multi-vendedor |
| 07 | [Auth + conta](sprints/sprint-07-auth.md) | 3 | Cadastro/login do cliente, CPF, páginas de conta |
| 08 | [Checkout](sprints/sprint-08-checkout.md) | 3 | payment-service, Pix/boleto/cartão, criação do pedido |
| 09 | [Pedidos](sprints/sprint-09-orders.md) | 3 | Histórico de pedidos + rastreamento + notificações por e-mail |
| 10 | [Onboarding do vendedor](sprints/sprint-10-seller-onboarding.md) | 4 | Wizard `/vender`, validação de CNPJ, fila de aprovação |
| 11 | [Importação em lote](sprints/sprint-11-bulk-import.md) | 4 | Upload CSV, workers Sidekiq, erros por linha |
| 12 | [Avaliações e notas](sprints/sprint-12-reviews-ratings.md) | 5 | Avaliações de compradores verificados, respostas do vendedor, moderação |
| 13 | [Contas pro / B2B](sprints/sprint-13-pro-accounts.md) | 5 | Clientes PJ, fluxo de cotação, nota fiscal |
| 14 | [Frete (Melhor Envio)](sprints/sprint-14-shipping-correios.md) | 4 | Tarifas reais, impressão de etiqueta, webhooks de rastreamento |
| 15 | [Disputas de pagamento](sprints/sprint-15-payment-disputes.md) | 4 | Reembolsos, Pix MED, chargebacks, fluxo admin |
| 16 | [Ferramentas de suporte](sprints/sprint-16-support-tooling.md) | 5 | Freshdesk, drawer de Ajuda in-app, encaminhamento para Sentry |
| 17 | [Upgrade de busca](sprints/sprint-17-search-upgrade.md) | 5 | Meilisearch, sinônimos BR-PT, tolerância a erros de digitação |
| 18 | [PWA + push](sprints/sprint-18-pwa-push.md) | 5 | PWA instalável, service worker, web push |
| 19 | [Recomendações](sprints/sprint-19-recommendations.md) | 5 | Co-compra baseada em regras, tendências, visualizados recentemente |
| 20 | [Console admin Utilar](sprints/sprint-20-utilar-admin.md) | 4 | Moderação de vendedor/produto/avaliação/disputa no gifthy-hub |
| 21 | [Performance](sprints/sprint-21-performance.md) | 5 | Lighthouse ≥85, code splitting, pipeline de imagens |
| 26 | [Programa de cashback](sprints/sprint-26-cashback.md) | 5 | Cashback % por produto, ledger, acúmulo/crédito/resgate/expiração, página de conta |
| 22 | [Observabilidade](sprints/sprint-22-observability.md) | 3 | Logs estruturados, métricas, Sentry, alertas |
| 23 | [CI/CD + IaC](sprints/sprint-23-ci-cd-iac.md) | 3 | Terraform, GH Actions, staging, rollback |
| 24 | [Conformidade LGPD](sprints/sprint-24-lgpd.md) | 3 | Banner de consentimento, exportação de dados, exclusão de conta |
| 25 | [Prontidão para lançamento](sprints/sprint-25-launch-readiness.md) | 3 | SEO, legal, SES, 3 vendedores, soft launch |

## Dependências

### Internas à Utilar

| Este trabalho... | ...depende de |
|------------------|---------------|
| Checkout Sprint 08 | payment-service em produção, conta PSP + credenciais sandbox (iniciar em paralelo com Sprint 03) |
| Checkout Sprint 08 | Sprint 17 do pai (`cep.ts`, `cnpj.ts`) já entregue ✅ |
| Checkout Sprint 08 | Sprint 18 do pai (campos BR de cliente em pedidos) mesclado |
| Frete Sprint 14 | Sprint 08 em produção; conta Melhor Envio; peso/dimensões do vendedor capturados |
| Disputas Sprint 15 | Sprint 08 em produção; acesso à API de reembolso do PSP; schema de log de auditoria admin |
| Busca Sprint 17 | Catálogo > 10k SKUs OU p95 de busca > 500ms; container Meilisearch no compose de produção |
| Console admin Sprint 20 | Sprints 10 e 15 em produção; tabela de auditoria de ações admin |
| Observabilidade Sprint 22 | Sprints 06–09 em produção; projeto Sentry; canal de alertas |
| CI/CD Sprint 23 | Acesso à conta AWS; Sprint 15 do pai com ALB implantado |
| LGPD Sprint 24 | Sprint 07 em produção; slot de revisão jurídica reservado |
| Lançamento Sprint 25 | Sprints 22, 23, 24 aprovados; 3 vendedores reais assinados; DNS apontando para o CloudFront |

### Alinhamento com a plataforma pai

| Sprint do pai | O que a Utilar reutiliza | Quando |
|---------------|--------------------------|--------|
| Sprint 16 (i18n) ✅ | Configuração i18next, `format.ts`, `LocaleSwitcher` | Sprint 01 |
| Sprint 17 (campos BR de vendedor) ✅ | `cep.ts`, `cnpj.ts`, `CnpjValidator`, autofill ViaCEP | Sprints 08, 10 |
| Sprint 18 (campos BR de cliente em pedidos) 🅒 | Validador de CPF, schema de endereço BR em pedidos | Sprint 08 |
| Sprint 15 (deploy no ALB) ⏸ | Caminho de rede em produção | Sprint 23 (CI/CD) |

**Decisão**: os Sprints 01 a 07 da Utilar podem prosseguir em paralelo com o trabalho do projeto pai. O Sprint 08 fica bloqueado até o Sprint 18 do pai ser mesclado. O Sprint 23 fica bloqueado até o ALB do Sprint 15 do pai estar em produção.

## Critérios de aprovação por fase

### Aprovação da Fase 1
- [ ] `npm run build` produz um bundle deployável < 250 kB gzip
- [ ] `npm test` com ≥ 1 teste de amostra passando
- [ ] `npm run lint` sem erros
- [ ] CI rodando a cada push em qualquer branch
- [ ] Switcher de i18n funciona; ambos os locales renderizam todas as chaves
- [ ] Página de referência do design system renderiza todos os primitivos

### Aprovação da Fase 2
- [ ] Cliente consegue acessar `/`, navegar por qualquer categoria, filtrar por ≥ 3 facetas, abrir um produto e ver galeria + especificações + estoque
- [ ] Todos os produtos de ferragens seedados aparecem em pelo menos uma categoria correta
- [ ] A busca retorna resultados relevantes para ≥ 20 queries de smoke test
- [ ] Lighthouse performance ≥ 85 nas páginas de categoria e produto

### Aprovação da Fase 3 (LANÇAMENTO)
Stack de comércio (Sprints 06–09):
- [ ] Cliente que nunca usou o site conclui um pagamento real via Pix em < 5 min
- [ ] Fluxos de boleto + cartão verificados no sandbox do PSP em produção
- [ ] Pedido chega ao order-service com itens, total e dados do cliente corretos
- [ ] Atualizações de status do vendedor (enviado / entregue) propagam para o detalhe do pedido do cliente
- [ ] Confirmação por e-mail enviada ao criar o pedido

Endurecimento pré-lançamento (Sprints 22–25):
- [ ] Cada requisição rastreável por request_id; Sentry capturando erros com contexto do usuário
- [ ] Alerta SMS disparado quando o sucesso de pagamento < 95% por 5 min (testado com falha forçada)
- [ ] Terraform provisiona staging + produção; `git push main` → staging → aprovação manual → produção
- [ ] Rollback para versão anterior é um gatilho de 1 clique no GH Actions
- [ ] Banner de consentimento LGPD, exportação de dados e exclusão de conta funcionando
- [ ] CPF mascarado em todos os logs (verificado)
- [ ] sitemap.xml + marcação schema.org em produção; Search Console verificado
- [ ] 4 e-mails transacionais (boas-vindas/confirmação/enviado/entregue) fora do sandbox do SES, entregando na caixa de entrada
- [ ] 3 vendedores reais × 20+ produtos cada, smoke test de Pix de R$ 1 aprovado
- [ ] Revisão jurídica (ToS + Privacidade + LGPD) aprovada por advogado
- [ ] ≥ 1 compra em produção bem-sucedida por um cliente real (amigo/conhecido) antes do lançamento público

### Aprovação da Fase 4
- [ ] Vendedor externo (que não seja conta seedada) conclui o onboarding em `/vender`
- [ ] Esse vendedor importa ≥ 50 produtos via CSV sem intervenção manual
- [ ] Vendedor imprime uma etiqueta Melhor Envio e despacha um pedido real
- [ ] Reembolso via console admin vai e volta ao PSP em até 5 min
- [ ] SLA da fila admin: aprovação de vendedor < 2 dias úteis, detecção de pedido travado em até 24h

### Aprovação da Fase 5 (contínuo)
- [ ] Cada sprint da Fase 5 atende seus próprios critérios de aceitação antes do próximo começar
- [ ] Lighthouse permanece ≥ 85 em todas as páginas adicionadas
- [ ] Zero incidentes P0 em 7 dias consecutivos após cada entrega

## Riscos e mitigações

| Risco | Probabilidade | Impacto | Mitigação |
|-------|--------------|---------|-----------|
| Integração com PSP > 1 sprint | Média | Alto | Iniciar conta PSP em paralelo com Sprint 03; usar sandbox cedo; buffer de 3 dias no Sprint 08 |
| Taxonomia de categorias não bate com o estoque real dos vendedores | Alta | Baixo | Fase 1 usa mapeamento client-side, revisável sem migrations |
| Filtros técnicos errados para profissionais | Alta | Médio | Lançar Fase 2 com as 3 principais categorias de filtro; iterar com dados de uso |
| Busca degrada acima de 10k produtos | Média | Alto | Meilisearch do Sprint 17 pronto para ativar por env flag |
| Texto em pt-BR não soa nativo | Baixa | Médio | Revisor BR em cada gate de fase |
| API do Melhor Envio instável | Média | Alto | Fallback de tarifa fixa em cache; Sprint 14 exige fallback funcionando |
| Revisão jurídica LGPD atrasada | Média | Alto | Contratar advogado no kick-off da Fase 3, não na semana do lançamento |
| Fadiga de alertas encobre páginas reais | Média | Médio | Sprint 22 começa conservador; tunar semanalmente por 4 semanas pós-lançamento |
| Complexidade do CI/CD cresce demais | Baixa | Médio | Sprint 23 com escopo mínimo: lint/test/build/deploy/rollback. Sem canary/blue-green no lançamento |
| Listagens falsificadas de vendedores | Média | Alto | Aprovação manual no Sprint 10 + fila de moderação admin no Sprint 20 |
| Suporte via WhatsApp sem resposta | Alta | Baixo | Fase 5+ (Sprint 16 adia o WhatsApp explicitamente) |

## Log de decisões (resumo)

Ver [`adr/`](adr/) para as decisões completas.

| # | Decisão | Status |
|---|---------|--------|
| [001](adr/001-placement-and-stack.md) | Localização como SPA irmã, não embutida no gifthy-hub | Aceita |
| [002](adr/002-integration-strategy.md) | Sem novos serviços de backend para o catálogo; payment-service introduzido na Fase 3 | Aceita |
| [003](adr/003-branding-and-ui.md) | Tema Tailwind via variáveis CSS, primitivos compartilhados seedados do gifthy-hub | Aceita |
| [004](adr/004-search-strategy.md) | Postgres ILIKE → Meilisearch quando catálogo > 10k ou p95 > 500ms | Proposta |
| [005](adr/005-payment-webhook-resilience.md) | Webhook idempotente + fallback outbox + retry Kafka com backoff | Proposta |
| [006](adr/006-shipping-provider.md) | Tarifa fixa na Fase 3 → Melhor Envio (agregador) na Fase 4 | Proposta |
| [007](adr/007-customer-support-tool.md) | Inbox compartilhada na Fase 3 → plano gratuito Freshdesk na Fase 4 | Proposta |
| [008](adr/008-schema-evolution.md) | Migrations apenas aditivas; gem strong_migrations; JSONB com `_schema_version` | Proposta |
| [009](adr/009-seller-kyc-fraud.md) | Modulus 11 + enriquecimento ReceitaWS/BrasilAPI; fila de aprovação manual | Proposta |
| [010](adr/010-notification-architecture.md) | SES transacional + web push VAPID; SMS/WhatsApp adiados | Proposta |
| [011](adr/011-cashback-mechanism.md) | Cashback: % por produto financiado pelo vendedor, creditado na entrega, validade de 12 meses, resgate mínimo de R$ 5 | Proposta |

## Referências cruzadas

Tudo em que os sprints se baseiam:

- [07 — Modelo de dados](07-data-model.md) — schema, migrations, índices
- [08 — Segurança e conformidade](08-security.md) — LGPD, rate limits, segredos, postura PCI
- [09 — Observabilidade](09-observability.md) — logs, métricas, alertas, SLOs
- [10 — Estratégia de testes](10-testing-strategy.md) — pirâmide unitário/integração/E2E/carga
- [11 — Infraestrutura](11-infra.md) — DNS, SSL, CDN, Terraform, CI/CD
- [12 — Runbook de operações](12-ops-runbook.md) — incidentes, rollback, backups, DR
- [13 — Checklist de lançamento](13-launch-checklist.md) — cronograma T-menos, legal, SEO, marketing
