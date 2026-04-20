# Fase 5 — Crescimento

**Objetivo**: retenção, recompra e alavancagem operacional. Mudar de "funciona?" para "compõe?".

Esta fase é menos prescritiva do que as Fases 1–4. A ordem dos sprints dentro da Fase 5 é guiada por dados reais das Fases 3–4, não por uma sequência fixa.

## Sprints

- [Sprint 12 — Avaliações & ratings](../sprints/sprint-12-reviews-ratings.md)
- [Sprint 13 — Contas Pro / B2B (clientes com CNPJ)](../sprints/sprint-13-pro-accounts.md)
- [Sprint 16 — Ferramenta de suporte ao cliente (Freshdesk)](../sprints/sprint-16-support-tooling.md)
- [Sprint 17 — Upgrade de busca (Meilisearch)](../sprints/sprint-17-search-upgrade.md)
- [Sprint 18 — PWA + notificações web push](../sprints/sprint-18-pwa-push.md)
- [Sprint 19 — Recomendações (baseadas em regras)](../sprints/sprint-19-recommendations.md)
- [Sprint 21 — Otimização de performance](../sprints/sprint-21-performance.md)
- [Sprint 26 — Programa de cashback](../sprints/sprint-26-cashback.md)

## Sinais de priorização

Escolha o próximo sprint da Fase 5 com base em dados medidos, não em intuição.

| Sinal | Limiar | Priorizar |
|--------|-----------|------------|
| Taxa de recompra | < 20% | [Sprint 18 PWA + push](../sprints/sprint-18-pwa-push.md), [Sprint 16 Suporte](../sprints/sprint-16-support-tooling.md) |
| Tickets de suporte | > 20/dia | [Sprint 16 Ferramenta de suporte](../sprints/sprint-16-support-tooling.md) |
| NPS de busca / bounce na busca | p95 busca > 500ms OU catálogo > 10k SKUs | [Sprint 17 Upgrade de busca](../sprints/sprint-17-search-upgrade.md) |
| Participação de tráfego mobile | > 60% | [Sprint 18 PWA + push](../sprints/sprint-18-pwa-push.md) |
| Demanda por conta Pro (pesquisa) | > 30 solicitações | [Sprint 13 Contas Pro](../sprints/sprint-13-pro-accounts.md) |
| Profundidade média de sessão | > 8 páginas/visita | [Sprint 19 Recomendações](../sprints/sprint-19-recommendations.md) |
| Cobertura de avaliações no catálogo | < 30% dos produtos entregues avaliados | [Sprint 12 Avaliações & ratings](../sprints/sprint-12-reviews-ratings.md) |
| Taxa de recompra estagnada | < 15% após 60 dias do lançamento | [Sprint 26 Programa de cashback](../sprints/sprint-26-cashback.md) |
| Lighthouse perf | < 85 em qualquer página-chave | [Sprint 21 Performance](../sprints/sprint-21-performance.md) |

## Ainda no backlog (sem sprint escrito ainda)

Estes são temas candidatos da Fase 5+ sem um documento de sprint escrito. Promover para um sprint real quando os dados mostrarem a necessidade.

- **Fulfillment próprio**: armazém operado pela Utilar para os SKUs de maior giro; selo "Enviado por Utilar" com entrega em 24h nas regiões metropolitanas. Dado de entrada para a decisão: qual % dos pedidos tem reclamações de entrega acima de 72h?
- **App mobile nativo**: React Native ou Flutter; revisitar somente se as métricas do PWA mostrarem fricção (taxa de instalação < 5% ou taxa de abertura de push < 10%)
- **Programa de fidelidade / indicação**: pontos por compras, descontos por indicações. Revisitar quando a taxa de recompra estiver > 30% (vale amplificar) ou < 10% (vale tentar mover).
- **Tier "Pro+" com assinatura**: mensalidade flat para frete grátis + suporte prioritário. Revisitar quando as contas Pro (Sprint 13) tiverem mais de 100 usuários ativos.
- **Recomendações baseadas em ML**: fazer upgrade das regras do Sprint 19 para filtragem colaborativa + baseada em conteúdo. Revisitar quando o volume de eventos for > 1M eventos/mês.
- **Suporte via WhatsApp Business**: muito relevante no Brasil. Defer até que o Freshdesk (Sprint 16) tenha 3 meses de dados sobre preferências de canal.
- **Analytics para vendedores v2**: funis de conversão, retenção por coorte, sinais de elasticidade de preço. Evoluir a partir do admin do Sprint 20.

## Critérios de saída da Fase 5

A Fase 5 não "termina" — ela roda continuamente. Mas cada sprint deve atender aos seus próprios critérios de aceite antes que o próximo comece, e os seguintes invariantes valem para todo o trabalho da Fase 5:

- [ ] Lighthouse performance + a11y ≥ 85 em toda página pública (Sprint 21 em diante)
- [ ] Zero incidentes P0 em 7 dias consecutivos após cada entrega
- [ ] Postura de LGPD inalterada: nenhum novo PII coletado sem atualizar a Privacidade + consentimento
- [ ] Cobertura de observabilidade: todo novo endpoint logado, todo novo caminho crítico com alerta
- [ ] Procedimento de rollback testado para cada entrega da Fase 5
