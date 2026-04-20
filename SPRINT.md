# Utilar Ferragem — Rastreador de Sprints

Documento vivo que acompanha os sprints ativos e concluídos. Segue o padrão usado no [`../SPRINT.md`](../SPRINT.md) do projeto pai.

## Estado atual

**Fase 0 — Planejamento.** Nenhum sprint ativo. Plano de implementação completo redigido (25 sprints em 5 fases), aguardando go/no-go do usuário.

## Índice de sprints

| # | Sprint | Fase | Status |
|---|--------|-------|--------|
| 01 | [Scaffold + tooling](docs/sprints/sprint-01-scaffold.md) | 1 — Fundação | ⬜ Não iniciado |
| 02 | [Design system + i18n](docs/sprints/sprint-02-design-system.md) | 1 — Fundação | ⬜ Não iniciado |
| 03 | [Catálogo + taxonomia](docs/sprints/sprint-03-catalog.md) | 2 — Catálogo | ⬜ Não iniciado |
| 04 | [Detalhe do produto (specs JSONB)](docs/sprints/sprint-04-product-detail.md) | 2 — Catálogo | ⬜ Não iniciado |
| 05 | [Busca + filtros (ILIKE)](docs/sprints/sprint-05-search-filters.md) | 2 — Catálogo | ⬜ Não iniciado |
| 06 | [Carrinho (local + persistente)](docs/sprints/sprint-06-cart.md) | 3 — Comércio | ⬜ Não iniciado |
| 07 | [Auth do cliente + conta](docs/sprints/sprint-07-auth.md) | 3 — Comércio | ⬜ Não iniciado |
| 08 | [Checkout (Pix / boleto / cartão)](docs/sprints/sprint-08-checkout.md) | 3 — Comércio | ⬜ Não iniciado |
| 09 | [Histórico de pedidos + rastreio + e-mails](docs/sprints/sprint-09-orders.md) | 3 — Comércio | ⬜ Não iniciado |
| 10 | [Wizard de onboarding de vendedor](docs/sprints/sprint-10-seller-onboarding.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 11 | [Importação em massa de SKUs (CSV)](docs/sprints/sprint-11-bulk-import.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 12 | [Avaliações & notas](docs/sprints/sprint-12-reviews-ratings.md) | 5 — Crescimento | ⬜ Não iniciado |
| 13 | [Contas Pro / B2B (CNPJ)](docs/sprints/sprint-13-pro-accounts.md) | 5 — Crescimento | ⬜ Não iniciado |
| 14 | [Tarifas de frete + rastreio (Melhor Envio)](docs/sprints/sprint-14-shipping-correios.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 15 | [Disputas de pagamento + reembolsos](docs/sprints/sprint-15-payment-disputes.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 16 | [Ferramenta de suporte ao cliente (Freshdesk)](docs/sprints/sprint-16-support-tooling.md) | 5 — Crescimento | ⬜ Não iniciado |
| 17 | [Upgrade de busca (Meilisearch)](docs/sprints/sprint-17-search-upgrade.md) | 5 — Crescimento | ⬜ Não iniciado |
| 18 | [PWA + web push](docs/sprints/sprint-18-pwa-push.md) | 5 — Crescimento | ⬜ Não iniciado |
| 19 | [Recomendações (baseadas em regras)](docs/sprints/sprint-19-recommendations.md) | 5 — Crescimento | ⬜ Não iniciado |
| 20 | [Console administrativo Utilar](docs/sprints/sprint-20-utilar-admin.md) | 4 — Ops de vendedor | ⬜ Não iniciado |
| 21 | [Otimização de performance](docs/sprints/sprint-21-performance.md) | 5 — Crescimento | ⬜ Não iniciado |
| 22 | [Observabilidade em produção](docs/sprints/sprint-22-observability.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 23 | [CI/CD + Terraform IaC](docs/sprints/sprint-23-ci-cd-iac.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 24 | [Conformidade LGPD](docs/sprints/sprint-24-lgpd.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 25 | [Prontidão para lançamento (SEO, jurídico, e-mail, vendedores)](docs/sprints/sprint-25-launch-readiness.md) | 3 — Comércio (gate de lançamento) | ⬜ Não iniciado |
| 26 | [Programa de cashback](docs/sprints/sprint-26-cashback.md) | 5 — Crescimento | ⬜ Não iniciado |

**Legenda**: ⬜ não iniciado • 🚧 ativo • ✅ concluído • ⏸ pausado • 🅒 condicional (aguarda dados ou decisão)

## Ordem recomendada de sprints

As dependências (ver [05-roadmap.md](docs/05-roadmap.md)) sugerem esta ordem dentro de cada fase:

- **Fase 1**: 01 → 02
- **Fase 2**: 03 → 04 → 05
- **Fase 3 comércio**: 06 → 07 → 08 → 09
- **Fase 3 gate de lançamento**: 22 → 23 → 24 → 25 _(pode se sobrepor aos sprints finais de comércio)_
- **Fase 4**: 10 → 11 → 20 → 14 → 15 _(20 pode começar após 10 + 15)_
- **Fase 5**: ordem orientada por dados (ver sinais de priorização no roadmap). Típica: 21 → 16 → 12 → 17 → 18 → 19 → 13

## Protocolo de handoff

Cada sprint termina com:
1. Todos os critérios de aceite no doc do sprint marcados como ✅
2. Uma nota curta neste arquivo sob "Histórico recente"
3. Qualquer follow-up capturado como issue ou novo doc de sprint
4. Tabela de status de integração em [docs/06-integration.md §7](docs/06-integration.md) atualizada
5. Se nova capacidade da plataforma for adicionada, atualizar [../docs/integration-guide.md](../docs/integration-guide.md)

## Histórico recente

_Nada ainda._
