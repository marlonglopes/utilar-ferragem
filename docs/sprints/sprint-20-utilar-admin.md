# Sprint 20 — Console admin da Utilar (no gifthy-hub)

**Fase**: 4 — Ops de vendedores. **Estimativa**: 8–10 dias.

## Escopo

Ferramentas de administração para operações específicas da Utilar, integradas ao painel admin existente do `gifthy-hub` sob uma nova árvore de rotas `/admin/utilar/*`. Nenhuma nova aplicação frontend — shell compartilhado, páginas e endpoints com escopo Utilar.

Cobre o ciclo completo de ops de vendedores: **aprovação de vendedor** (verificação de CNPJ + revisão de documentos + aprovar/rejeitar com motivo), **moderação de produtos** (fila de admin vs. publicação automática com base no nível do vendedor), **supervisão de pedidos** (pedidos travados, disputas), **processamento de reembolsos** (via payment-service), **moderação de avaliações** (ocultar/reexibir) e **repasses aos vendedores** (MVP: visualizar extrato + marcar como pago com exportação CSV; repasses automáticos via ACH/Pix são Fase 6).

Toda mutação do admin escreve em uma nova tabela de auditoria `admin_actions` para que seja possível reconstruir quem fez o quê.

## Tarefas

### gifthy-hub — roteamento + shell
1. Expandir `src/router/index.tsx` com a subárvore `/admin/utilar/*` sob o guard `ProtectedRoute + RoleRoute(admin)` existente
2. Adicionar `AdminUtilarSidebar` (ou estender `Sidebar` para exibir uma seção "Utilar" quando `user.role === 'admin'`) com links: Vendedores, Produtos, Pedidos, Disputas, Avaliações, Repasses
3. i18n: adicionar chaves de admin em `src/i18n/{en,pt-BR}/admin.json` (motivos de aprovação, status de repasse etc.)

### Páginas de admin (todas em `src/pages/admin/utilar/`)
4. `SellerApprovalQueue.tsx` — tabela de vendedores com `status=pending`, colunas: nome, CNPJ, cidade/UF, registrado em, ações (Ver, Aprovar, Rejeitar)
5. `SellerDetailPage.tsx` — perfil completo do vendedor: CNPJ + status de validação do CNPJ (verificação Módulo 11 em tempo real), endereço, documentos anexados, histórico de produtos. Modais de Aprovar/Rejeitar com dropdown de motivo + texto livre
6. `ProductModerationQueue.tsx` — produtos com `status=pending`; aprovação em lote; rejeitar com motivo (dispara e-mail para o vendedor)
7. `OrdersOversight.tsx` — filtros: travados (pago + não enviado há > 3 dias), cancelados pelo vendedor > 5%, reembolso solicitado. Clique na linha → detalhes do pedido
8. `DisputesQueue.tsx` — conectado às linhas de disputa do Sprint 15; admin pode dar razão ao comprador/vendedor e acionar reembolso
9. `ReviewModerationQueue.tsx` — lista avaliações sinalizadas (por usuários ou filtro de palavrões); ocultar/reexibir, banir avaliador
10. `PayoutsList.tsx` — extrato por vendedor: total vendido (período), taxa da plataforma, líquido a pagar, status (pending/paid). Botão marcar como pago escreve na tabela `payouts` + log de auditoria
11. `PayoutDetailPage.tsx` — detalhamento dos pedidos incluídos em um repasse; botão "Exportar CSV"

### user-service — endpoints de admin
12. `GET /api/v1/admin/sellers?status=pending&page=N` (JWT + papel admin)
13. `PATCH /api/v1/admin/sellers/:id/approve` — define `status=active`, escreve linha de auditoria, enfileira e-mail de boas-vindas
14. `PATCH /api/v1/admin/sellers/:id/reject` — body `{ reason: string }`, define `status=rejected`, escreve linha de auditoria, enfileira e-mail de rejeição
15. Tabela `admin_actions` + model `AdminAction`: `admin_user_id, action (string), target_type, target_id, metadata (jsonb), created_at`. Concern `Auditable` nos controllers.

### product-service — endpoints de admin
16. `GET /api/v1/admin/products?status=pending&seller_id=&page=` (JWT + admin)
17. `PATCH /api/v1/admin/products/:id/approve` + `/reject` com motivo
18. Regra de publicação automática: se `seller.tier >= 2` (onde o nível reflete bom histórico ao longo do tempo), novos produtos pulam a fila; caso contrário, vão para a fila. Campo `tier` com padrão 1; promoção de nível é uma ação manual do admin no MVP.

### order-service — endpoints de admin
19. `GET /api/v1/admin/orders?stuck=true` — `paid_at < NOW() - INTERVAL '3 days' AND status IN ('paid','processing')`
20. `GET /api/v1/admin/orders?refund_requested=true`
21. `POST /api/v1/admin/orders/:id/refund` — aciona reembolso no payment-service; define `status=refunded`; linha de auditoria

### Pipeline de repasses
22. Migration `create_payouts`: `seller_id, period_start, period_end, gross_amount, fee_amount, net_amount, status (pending|paid|failed), paid_at, external_ref, created_at`
23. `rake payouts:generate` noturno — agrega pedidos pagos ainda não incluídos em um repasse, cria linhas de repasse pendentes por vendedor
24. `POST /api/v1/admin/payouts/:id/mark-paid` — admin confirma que fez o Pix; escreve linha de auditoria
25. Exportação CSV `GET /api/v1/admin/payouts/:id/export.csv` — colunas prontas para bancário (CNPJ do vendedor, chave Pix, valor líquido, referência)

## Critérios de aceite

- [ ] Admin consegue aprovar um vendedor pendente em menos de 60 segundos (medido do clique até o banco)
- [ ] Rejeitar um vendedor envia um e-mail em PT-BR com o motivo preenchido em texto livre
- [ ] O filtro de pedidos travados em `OrdersOversight` exibe qualquer pedido pago há > 3 dias sem envio
- [ ] Admin consegue emitir reembolso pela UI; o payment-service processa; o pedido transita para `refunded`
- [ ] O `payouts:generate` noturno produz uma linha por vendedor com líquido não nulo
- [ ] O CSV de repasse é baixado com todas as colunas preenchidas; abre corretamente no Excel (UTF-8 BOM)
- [ ] Toda mutação do admin escreve uma linha em `admin_actions` com o id do admin que realizou a ação
- [ ] Publicação automática: o produto de um vendedor nível 2 vai direto para `status=active`, pulando a fila

## Dependências

- Sprint 10 (onboarding de vendedores) — origem dos vendedores pendentes
- Sprint 15 (disputas) — origem das linhas de disputa
- Sprint 12 (avaliações) — origem das avaliações sinalizadas
- Endpoint de reembolso do payment-service disponível

## Riscos

- Dívida de esquema no log de auditoria — adicionar `admin_actions` no primeiro PR deste sprint; retroagir depois é trabalhoso
- Precisão dos repasses — o MVP usa Pix manual com exportação CSV; um único bug de dados pode causar erros financeiros reais. Exija dupla assinatura em cada CSV de repasse (admin A gera, admin B aprova) até ganhar confiança.
- Expansão de permissões no painel admin — manter `RoleRoute(admin)` como único guard; não introduzir sub-papéis neste sprint (reavaliar na Fase 6 se necessário).
