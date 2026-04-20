# Sprint 16 — Ferramentas de suporte ao cliente (Freshdesk)

**Fase**: 5 — Crescimento. **Estimativa**: 4–6 dias.

## Escopo

A caixa de entrada "mande um e-mail para contato@" do pré-lançamento não escala além de alguns poucos tickets diários e não oferece nenhuma visibilidade de SLA. Este sprint a substitui pelo **Freshdesk free tier** (3 agentes, tickets ilimitados) e integra a abertura de tickets ao produto: clientes abrem um drawer "Ajuda" pré-preenchido com o contexto do pedido; vendedores têm um fluxo de tickets separado no gifthy-hub. Erros de servidor capturados pelo Sentry são encaminhados ao Freshdesk como tickets internos para que a equipe de operações não perca incidentes em produção.

Referência: [ADR 007](../adr/007-support-tooling.md). A integração com o WhatsApp Business está explicitamente fora do escopo deste sprint.

## Tarefas

### Freshdesk — configuração (operações)
1. Criar conta Freshdesk no subdomínio `utilarferragem`; provisionar chaves de API; criar um segundo produto/tenant para vendedores (ou usar tags — decidir no kickoff com base nos limites de vagas do plano gratuito)
2. Configurar categorias de ticket: `Pedido`, `Pagamento`, `Entrega`, `Produto`, `Conta`, `Outro`; configurar respostas automáticas em pt-BR
3. Verificar DKIM + SPF para o domínio de envio para que as respostas automáticas não caiam em spam

### novo `support-service` ou módulo no user-service
4. Padrão: **módulo no user-service** (mantém o sprint pequeno, sem novo container). Endpoint `POST /api/v1/support/ticket` — autenticação opcional; corpo: `{ subject, body, category, order_id (opcional), email (se não autenticado) }`; rate limit de 5/hora por IP
5. Cliente `services/user-service/app/clients/freshdesk_client.rb` — encapsula a API REST do Freshdesk (`POST /api/v2/tickets`) com autenticação básica via chave de API; adiciona tags automaticamente com tipo de cliente (PF/PJ) + categoria
6. Se `order_id` for fornecido, busca o resumo do pedido no order-service e injeta na descrição do ticket para contexto do agente
7. Migration `services/user-service/db/migrate/YYYYMMDD_create_support_tickets.rb`: tabela `support_tickets` (`id, user_id (nullable), freshdesk_ticket_id, subject, category, order_id, created_at`) — registro de auditoria leve, não é fonte da verdade

### Gateway
8. Rotear `/api/v1/support/ticket` para o user-service (autenticação opcional; o gateway encaminha com ou sem JWT)

### Ponte Sentry → Freshdesk
9. Regra de alerta no Sentry: em eventos de erro com tag `environment: production` e `level: error` ou `fatal`, disparar um webhook para `POST /webhooks/sentry/freshdesk` no user-service (novo endpoint público, com verificação de assinatura)
10. O handler normaliza o payload do Sentry em um ticket interno no Freshdesk com as tags `internal` + `sentry` para não alertar clientes

### SPA (utilar-ferragem)
11. Componente `SupportDrawer.tsx` montado em todas as páginas (gatilho: FAB flutuante "Ajuda" no canto inferior direito); conteúdo: dropdown de assunto (categorias), textarea, e-mail + nome preenchidos automaticamente se logado, seletor de pedido opcional lendo de `/api/v1/orders`
12. Envio → `POST /support/ticket` → toast de sucesso "Recebemos seu pedido de ajuda. Em breve entraremos em contato." + exibição do número do ticket
13. Assuntos pré-preenchidos por página: na OrderDetailPage, preencher "Dúvida sobre pedido #XYZ" + categoria `Pedido`; na página de confirmação de pagamento, preencher a categoria de pagamento

### gifthy-hub
14. Item "Ajuda" no menu do vendedor na barra lateral abrindo um drawer semelhante; assuntos pré-definidos voltados ao vendedor (`Repasse`, `Política`, `Onboarding`, `Técnico`)
15. Página de FAQ pública `/seller/ajuda/faq` com respostas para as 10 principais dúvidas de vendedores (JSON estático + renderização em markdown); CTA "Ainda precisa de ajuda?" abre o drawer

### Templates de e-mail
16. Confirmação de recebimento do ticket (pt-BR + en) no Freshdesk com o número do ticket, SLA ("primeiro contato em até 4 horas úteis") e link para `/conta/ajuda` para acompanhamento

## Critérios de aceite
- [ ] O cliente abre o drawer em qualquer página, envia um ticket e recebe um e-mail de confirmação em até 1 minuto
- [ ] O ticket chega no Freshdesk com o contexto do pedido (id, itens, total, status) pré-injetado quando aberto a partir de uma página de pedido
- [ ] O rate limit é acionado na 6ª submissão em uma hora por IP; retorna 429 com mensagem de erro em pt-BR
- [ ] O drawer do vendedor no gifthy-hub abre tickets no tenant Freshdesk com tag de vendedor
- [ ] Um erro do Sentry em produção cria um ticket interno no Freshdesk em até 2 minutos
- [ ] As tags de categorização automática de tickets funcionam para filtro pelos agentes
- [ ] Tempo mediano de primeira resposta ≤ 4 horas durante a semana de lançamento (medido no Freshdesk)

## Dependências
- Sprint 09 (pedidos) concluído — a busca de contexto de pedido requer o order-service
- Conta Freshdesk provisionada + chaves de API + DKIM/SPF configurados
- Conta Sentry para o frontend + backend em produção (deve já existir da Fase 3)

## Riscos
- O plano gratuito do Freshdesk limita a 3 agentes — monitorar necessidades de headcount durante a Fase 5 e planejar upgrade pago antes do 4º agente
- Entregabilidade de e-mail — DKIM/SPF devem estar verificados antes do lançamento ou os e-mails de confirmação caem em spam
- Ruído de webhook do Sentry (erros intermitentes) — deduplicar por `issue_id` dentro do handler para não criar 50 tickets duplicados para o mesmo bug
- Rate limit muito agressivo bloqueia clientes legítimos em NAT compartilhado — registrar os 429s e revisar o limite após uma semana
