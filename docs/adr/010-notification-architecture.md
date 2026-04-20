# ADR 010 — Arquitetura de notificações (e-mail, SMS, push, in-app)

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

Confirmações de pedido, atualizações de pagamento, rastreamento de entrega, redefinições de senha, disparos de marketing opt-in, avisos de repasse ao vendedor, alertas de admin — todo evento de marketplace eventualmente vira uma notificação. Fazer isso de forma ad-hoc (cada serviço publica seus próprios e-mails via seu próprio SMTP) resulta em templates inconsistentes, lógica duplicada de limite de taxa e nenhuma forma de respeitar preferências do usuário ou horários de silêncio globalmente. Construir um "serviço de notificações" completo no primeiro dia também é exagero para o volume de lançamento.

Forças em jogo:
- E-mail é o único canal universalmente confiável no lançamento (transacional + requisitos de descadastro da LGPD)
- Web push é gratuito, sem custo por mensagem, alto engajamento — mas exige chaves VAPID e uma tabela de assinaturas
- SMS é caro no BR (~R$ 0,10–0,30 por mensagem via Zenvia/Twilio); a taxa de queima fica real em escala
- WhatsApp Business é culturalmente dominante mas tem overhead de aprovação WABA (1–3 semanas) e aprovação de template por tipo de mensagem
- Usuários em `/marketplace/*` podem navegar em `en` mas finalizar a compra em `pt-BR`; as notificações precisam resolver o idioma correto

Depende desta decisão: confirmações de pedido na Fase 3, atualizações de rastreamento de entrega na Fase 4, canais de marketing na Fase 5.

## Decisão

### Três canais no lançamento

| Canal | Provedor | Templates | Modelo de assinatura |
|------|----------|-----------|--------------------|
| **E-mail transacional** | AWS SES (região BR ou us-east-1 com tópico SNS de bounce/reclamação) | React Email → MJML → HTML, renderizado server-side | Sempre ativo para críticos; opt-in para marketing |
| **Web push** | VAPID + service worker + tabela `push_subscriptions` própria | Payload JSON simples, curto | Opt-in por navegador |
| **In-app** | Tabela `notifications` no user-service; SSE ou polling no lançamento | JSON simples | Sempre ativo, dispensável |

### SMS — reservado, não entregue

Construir uma interface `SmsAdapter` mas **não integrar um provedor** no lançamento. Toggle no código, sem provedor por trás. Entregar SMS somente se:
- Análise de churn mostrar que usuários estão perdendo atualizações críticas (pagamento, entrega), E
- E-mail + web push não fecharem a lacuna

Nesse momento, **Zenvia** (nativo do BR) é a escolha padrão; Twilio é o fallback.

### WhatsApp — somente Fase 5+

Aprovação WABA, aprovação de template, contrato com BSP, registro de opt-in — nada disso é trivial. Coberto primeiro pelo ADR 007 (add-on WhatsApp do Freshdesk para suporte); WhatsApp transacional para notificações de pedido é deferido.

### Mini-lib compartilhada, não um serviço

`notification-core` — uma gem compartilhada pequena (Rails) + pacote npm (frontend) que expõe:
- `Notifier.send(user:, event:, channels:, data:)` — distribui para cada adaptador de canal
- Lógica de limite de taxa (por usuário, por canal, por 24h) — `EMAIL_MAX_PER_DAY = 10`, `PUSH_MAX_PER_DAY = 5`, etc.
- Resolução de preferências (lê `user.notification_preferences`)
- Resolução de locale (`user.locale` → fallback para `pt-BR`)
- Aplicação de horário de silêncio (abaixo)

**Não** é um serviço standalone. Um serviço adiciona peso operacional (pipeline de deploy, monitoramento, banco de dados) sem retorno até estarmos de fato roteando por mais de 4 canais com orquestração complexa. Revisitar se:
- SMS + WhatsApp forem entregues
- A lógica de limite de taxa crescer além de "contagem por usuário por dia"
- Agendamento assíncrono / campanhas drip se tornarem realidade

### Horário de silêncio

Janela de silêncio padrão: **22:00–08:00 BRT**. Notificações não críticas ficam em fila para entrega às 08:00. Notificações críticas (confirmação de pagamento, alerta de fraude) ignoram o horário de silêncio. O usuário pode desativar o horário de silêncio nas preferências.

### Alternativas comparadas — provedores de e-mail

| Provedor | Entregabilidade no BR | Preço @ 100k/mês | Templating | Setup | DKIM/DMARC |
|----------|-------------------|-----------------|------------|-------|------------|
| **AWS SES** | Bom (com aquecimento) | ~$10 | Próprio (React Email) | Médio (DKIM, SPF, aquecimento) | Nativo |
| Sendgrid | Bom | ~$15 | Nativo | Fácil | Nativo |
| Postmark | Excelente (só transacional) | ~$50 | Nativo | Fácil | Nativo |
| Resend | Não verificado no BR | ~$20 | Nativo | Fácil | Nativo |

### Alternativas comparadas — web push

| Opção | Custo | Lock-in | Funcionalidades |
|--------|------|-------------|----------|
| **VAPID self-host** | $0 | Nenhum | Controle total, sem dashboards |
| OneSignal | Gratuito → creep pago | Sim | Dashboards ricos, A/B, segmentação |

### Alternativas comparadas — SMS

| Opção | Presença no BR | Preço/SMS (BR) | Bundle WhatsApp |
|--------|-------------|----------------|-----------------|
| **Zenvia** | Nativo do BR | ~R$ 0,08–0,12 | Sim (parceiro WABA) |
| Twilio | Global, BR mais fraco | ~R$ 0,15–0,30 | Sim |
| AWS SNS SMS | Global | Variável, frequentemente rotas ruins no BR | Não |

## Consequências

### Positivo
- **Três canais cobrem >95% dos casos de uso** a custo praticamente zero por mensagem (SES é ~R$ 0,0005/e-mail, push é gratuito)
- Mini-lib compartilhada centraliza limite de taxa, preferências e horário de silêncio — um único lugar para corrigir, não cinco
- Sem espalhamento prematuro de serviço de notificações; adicionamos essa caixa somente quando a complexidade de coordenação exigir
- Trocar provedores de e-mail é uma mudança de um adaptador (`SesAdapter` → `SendgridAdapter`) graças à interface da mini-lib
- Conformidade de descadastro LGPD centralizada: um endpoint, uma UI de preferências, todos os canais a respeitam

### Negativo
- Templates React Email são DIY; sem editor drag-drop, o time de marketing depende de engenheiros para mudanças de layout. Aceitável antes de ter um time de marketing; revisitar quando contratar.
- VAPID self-host significa nenhum dashboard nativo — escrevemos nosso próprio relatório de "entregue / clicado / dispensado". Pequeno esforço, custo real.
- Lógica de horário de silêncio em uma lib compartilhada significa que bugs de timezone vão acontecer; testar com uma transição de horário de verão real (o Brasil aboliu o horário de verão em 2019, mas timezones fornecidos pelo usuário de usuários `en` ainda importam).
- O aquecimento do SES leva 2–4 semanas de rampa cuidadosa de volume. Começar cedo.

### Alternativas rejeitadas
- **Servidor SMTP próprio**: zero vantagem, enorme sobrecarga de ops (reputação, entregabilidade, tratamento de abuso, aquecimento de IP). Não inequívoco.
- **OneSignal para web push**: tentador pelo dashboard, mas lock-in de fornecedor + creep de preço além do plano gratuito, e não precisamos das funcionalidades de marketing. VAPID é o mesmo padrão; apenas somos donos dos nossos dados.
- **Resend**: ótima DX, entregabilidade no BR incerta (infraestrutura de envio sediada nos EUA; a colocação na caixa de entrada em provedores brasileiros como UOL, Terra, Locaweb depende de reputação). Revisitar em 12 meses quando tivermos mais dados.
- **Serviço de notificações standalone agora**: sem retorno operacional no volume de lançamento. A mini-lib nos dá o mesmo contrato sem um novo alvo de deploy.
- **SMS no lançamento**: custo por mensagem vs retorno desfavorável quando push + e-mail já cobrem os caminhos críticos.

## Questões em aberto

1. **Horário de silêncio** — padrão 22:00–08:00 BRT. Toggle de substituição pelo usuário nas preferências? Substituição por canal (e-mail sempre OK, push suprimido)? Responsável: produto no Sprint 09.
2. **Detecção de idioma** — usuário tem `locale = 'en'` mas o pedido é entregue no Brasil; qual idioma prevalece para a notificação de entrega? Padrão: `locale` do usuário para o corpo, sempre bilíngue `pt-BR` para o rodapé legal (CDC / LGPD). Responsável: líder de i18n.
3. **Conformidade LGPD + CAN-SPAM** — link de descadastro deve aparecer em todos os e-mails de marketing; e-mails transacionais são isentos mas devem ser honestamente transacionais. Definir claramente a fronteira transacional/marketing no registro `Notifier.send(event:)`. Responsável: revisão jurídica antes da Fase 5.
4. **Ciclo de vida das assinaturas push** — quando descartamos linhas obsoletas de `push_subscriptions` (assinatura expirada, 410 Gone do serviço push)? Job de limpeza diária, descartar linhas com 3 retornos 410 consecutivos.
5. **Ajuste de limites de taxa** — `EMAIL_MAX_PER_DAY = 10` é um chute. Medir o volume real por usuário nos primeiros 30 dias e reajustar.
6. **Fluxo de localização de templates** — componentes React Email leem de i18next; quem é responsável pelo conteúdo `pt-BR` vs `en` por template? Alinhar com o ADR 003 (cada app é dono do seu próprio i18n). Centralizar as chaves de template em `notification-core/locales/`.
7. **Tratamento de falhas** — tópico SNS de bounce/reclamação do SES → suprimir automaticamente o e-mail do nosso lado. Construir no Sprint 09.
