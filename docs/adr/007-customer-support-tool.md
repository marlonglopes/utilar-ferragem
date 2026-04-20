# ADR 007 — Ferramenta de suporte ao cliente

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

Um marketplace acumula tickets de suporte de forma linear conforme os pedidos crescem: "cadê meu pacote", "item errado", "reembolso", "nota fiscal por favor". No lançamento temos volume praticamente zero; daqui a 12 meses podemos ter entre 20 e 100 tickets/dia dependendo do crescimento. Construir nosso próprio sistema de tickets é uma armadilha clássica de distração — cada hora gasta num helpdesk é uma hora a menos no catálogo, no checkout ou nas ferramentas do vendedor.

Forças em jogo:
- Clientes brasileiros preferem fortemente o WhatsApp ao e-mail para suporte (norma cultural)
- Precisamos encadear conversas com contexto do pedido (ID do pedido, status do pagamento, rastreamento)
- Atendentes precisam de macros em pt-BR e capacidade de escalonar para vendedores
- Planos gratuitos existem de verdade, mas têm limites duros; os planos pagos sobem de preço rapidamente quando você os ultrapassa
- Suporte ao vendedor (onboarding, repasses, disputas) é um produto diferente do suporte ao cliente, mas compartilha a mesma ferramenta

Depende desta decisão: formulário "Ajuda" na página de detalhes do pedido na Fase 3, adoção do helpdesk na Fase 4, canal de WhatsApp na Fase 5.

## Decisão

### Fase 3 / Sprint 09 — Caixa de entrada compartilhada + formulário no produto

- **`support@utilarferragem.com.br`** hospedado no Google Workspace. Todo suporte entrante cai numa caixa de entrada compartilhada; 1–2 operadores fazem a triagem.
- Botão **"Ajuda"** no produto, na página de detalhes do pedido, que compõe um e-mail com `Subject: [Pedido #1234] <assunto do usuário>` e um corpo pré-preenchido com `order_id`, `payment_status`, `shipping_status`, `seller_name` e a mensagem do cliente. Enviado pelo nosso canal de e-mail transacional existente (ADR 010 / AWS SES) com `Reply-To: customer@email`.
- Sem IDs de ticket, sem rastreamento de SLA, sem macros — apenas e-mail. Aceitável abaixo de ~20 tickets/dia.

### Fase 4 / Sprint ~15 — Plano gratuito do Freshdesk

**Gatilho**: mais de 20 tickets/dia de forma sustentada por 2 semanas, OU reclamações sobre SLA de suporte se tornarem um tema recorrente no feedback dos clientes.

- Apontar o MX do `support@utilarferragem.com.br` → Freshdesk, que cria tickets automaticamente
- Plano gratuito: agentes ilimitados (até 10), canal de e-mail, automações básicas, portal de base de conhecimento
- Macros em pt-BR para as 20 resoluções mais comuns
- Vincular tickets do Freshdesk a registros de pedidos por meio de um iframe leve no admin (ou simplesmente colar a URL do pedido no ticket)

**NÃO** construir nossa própria UI de tickets. Se o plano gratuito do Freshdesk se tornar insuficiente, avaliar o próximo plano pago ou migrar — ainda é mais barato do que construir.

### Fase 5+ — WhatsApp Business

Adicionar WhatsApp como canal de suporte pela integração do Freshdesk com WhatsApp (ou Z-API / 360Dialog como BSP). Somente após termos um fluxo de trabalho estável no e-mail.

### Alternativas comparadas

| Opção | Suporte pt-BR | Preço com 20 tickets/dia | Preço com 100/dia | WhatsApp | Instagram DM | Plano gratuito | Adequação ao marketplace |
|--------|---------------|------------------------|------------------|----------|--------------|-----------|-----------------|
| **Freshdesk** | UI + KB em pt-BR nativo | Gratuito | ~$15/agente/mês | Sim (plano pago) | Sim (pago) | Agentes ilimitados, só e-mail | Bom |
| Zendesk | pt-BR forte | ~$19/agente/mês | ~$55/agente/mês | Sim | Sim | Sem plano gratuito real | Bom (mas o preço sobe) |
| Intercom | pt-BR OK | ~$39/assento/mês | ~$99/assento/mês | Add-on pago | Add-on pago | Trial de 14 dias | Fraco (orientado a SaaS, não a e-com) |
| HelpScout | pt-BR OK (UI em inglês) | ~$20/usuário/mês | ~$40/usuário/mês | Via integrações | Limitado | Trial de 15 dias | Razoável |
| Própria solução | Nativo (a gente constrói) | Custo de eng: semanas | Custo de eng: meses | Construir | Construir | N/A | Péssimo desperdício de tempo |

## Consequências

### Positivo
- **Zero gasto com ferramentas por meses** — a caixa de entrada compartilhada nos sustenta durante o período de lançamento
- O plano gratuito do Freshdesk é genuinamente generoso (agentes ilimitados é raro); a migração acontece quando tivermos dados reais, não antes
- O formulário "Ajuda" no produto já preenche o contexto — o suporte recebe o ID do pedido em vez de perguntar "qual seu número de pedido?"
- Sem código de ticketing personalizado significa nenhuma rotação de plantão por causa do sistema de suporte

### Negativo
- A caixa de entrada compartilhada não tem rastreamento de SLA, nem responsável atribuído, nem status além de "respondido ou não". Aceitável abaixo de 20/dia; doloroso acima disso.
- Migrar da caixa de entrada compartilhada para o Freshdesk cria uma costura — threads antigas no Gmail, novos tickets no Freshdesk. Documente a data de corte; não tente migrar o histórico retroativamente.
- O plano gratuito do Freshdesk nos impede de usar WhatsApp, pesquisas e automações avançadas. O plano pago ($15/agente/mês) é o estado estável realista.
- LGPD: a ferramenta de suporte armazena PII de clientes. É necessário assinar um DPA com o Freshdesk antes de entrar em produção na Fase 4.

### Alternativas rejeitadas
- **Zendesk**: produto premium, preço escala agressivamente a partir do primeiro assento. Se crescermos até lá, migraremos; não é uma escolha de lançamento.
- **Intercom**: construído para chat em SaaS, não para suporte transacional de marketplace. Precificação por "assento" mais por contato não tem a forma certa para e-commerce.
- **Ticketing próprio**: viola a regra de "não construir infraestrutura de distração". Cada minuto no nosso próprio helpdesk é um minuto a menos no catálogo/checkout/ferramentas do vendedor.
- **HelpScout**: bom produto, UI é inglês-first, nenhuma vantagem significativa de pt-BR em relação ao Freshdesk.

## Questões em aberto

1. **Integração com WhatsApp Business** — o principal canal de suporte no Brasil. A escolha do BSP (Z-API / 360Dialog / Gupshup / Freshdesk nativo) tem implicações em custo, tempo de aprovação do WABA e capacidade de automação. Responsável: líder de suporte, kickoff da Fase 5.
2. **Suporte ao vendedor vs suporte ao cliente** — mesma ferramenta, fila diferente? Mesmo time, times diferentes? Padrão: uma ferramenta (Freshdesk) com dois endereços de entrada (`support@` para clientes, `sellers@` para vendedores), marcados e roteados pelas regras do Freshdesk. Revisitar se o volume de vendedores justificar um agente dedicado.
3. **Instagram DMs / Facebook Messenger** — canais relevantes no e-commerce brasileiro. Adicionar pelo plano omnichannel do Freshdesk se/quando o tráfego social orgânico justificar.
4. **Base de conhecimento** — criamos uma no lançamento (reduz o volume entrante) ou esperamos ver as perguntas recorrentes reais? Padrão: esperar; artigo com as top 20 FAQs após 4 semanas de tickets.
5. **Chatbot / triagem por IA** — o Freshdesk Freddy AI é pago; triagem por LLM open-source é tentador, mas é uma armadilha de projeto paralelo. Revisitar quando o volume de tickets justificar a sobrecarga operacional.
6. **Cobertura fora do horário comercial** — caixa de entrada compartilhada → "respondemos em até 1 dia útil" é honesto no lançamento. Fase 5: considerar BPO contratado para fins de semana.
