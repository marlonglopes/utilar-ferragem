# Sprint 25 — Prontidão para lançamento (SEO, jurídico, e-mail, marketing)

**Fase**: 3 — Commerce (gate de lançamento). **Estimativa**: 5–7 dias.

## Escopo

Última milha antes do lançamento público. Consolida todos os itens em aberto de [13-launch-checklist.md](../13-launch-checklist.md) em tarefas executáveis. Cinco frentes em paralelo: **base de SEO** (sitemap + schema.org + meta), **jurídico** (aprovação do advogado entregue no Sprint 24), **e-mail transacional** (SES fora do sandbox + 4 templates em React Email), **analytics** (GA4 + Search Console ativos, com gate de consentimento), **onboarding de vendedores** (3 vendedores reais com 60+ produtos no total, todos ativos).

Termina com um **soft launch**: post no Instagram, 50 convites para amigos e familiares, janela fechada de 2 semanas para feedback de usuários reais antes de marketing amplo. Dashboard de KPIs diários compartilhado com os stakeholders durante a janela de soft launch.

## Tarefas

### Base de SEO
1. Gerador de sitemap `rake seo:generate_sitemap` no product-service — emite `sitemap.xml` + `sitemap-products.xml` (paginado, máx. 50k URLs por arquivo) para o bucket S3 do SPA; atualização via cron noturno
2. `utilar-web/public/robots.txt`: permitir tudo, referenciar a URL do sitemap, bloquear `/admin/*` + `/conta/*`
3. JSON-LD schema.org injetado via `react-helmet-async`:
   - `Product` + `Offer` + `AggregateRating` em todo PDP
   - `BreadcrumbList` na categoria + PDP
   - `Organization` na homepage
4. Meta tags por página via componente `SeoHead` (`utilar-web/src/components/seo/SeoHead.tsx`) — title, description, canonical, OG image, Twitter card
5. Meta descriptions por categoria escritas em PT-BR (não geradas automaticamente)
6. URLs canônicas para evitar conteúdo duplicado com query strings de filtro
7. Template de imagem OG: gerar OG por produto via Cloudflare Worker OU pré-renderizar no upload com sharp (escolher o mais simples — padrão para sharp no upload)

### Analytics — GA4 + Search Console
8. Criar propriedade GA4; measurement id em `VITE_GA4_ID`
9. `utilar-web/src/lib/analytics-ga.ts` — carrega o gtag **apenas se** `useConsent().analytics === true` (gate do Sprint 24)
10. Rastrear: `page_view`, `view_item` (PDP), `add_to_cart`, `begin_checkout`, `purchase` (com transaction_id + items + valor em BRL)
11. Google Search Console: verificação TXT via DNS no domínio, enviar ambos os sitemaps, solicitar indexação das 20 principais páginas de categoria
12. Exportação GA4 → BigQuery habilitada (tier gratuito, para análise pós-lançamento)

### E-mail transacional
13. Verificação de domínio no AWS SES para `utilarferragem.com.br`: DKIM (3 CNAMEs no Route53), SPF (`v=spf1 include:amazonses.com ~all`), DMARC (`v=DMARC1; p=quarantine; rua=mailto:dmarc@utilarferragem.com.br`)
14. Solicitação de saída do sandbox do SES — enviar **T-10 dias** (aprovação pode levar até 24h; planejar margem)
15. 4 templates React Email (`services/mailer/templates/`), tom em BR-PT:
    - `welcome.tsx` — enviado no cadastro do usuário
    - `order-confirmed.tsx` — enviado na confirmação de pagamento com itens, total e previsão de entrega
    - `order-shipped.tsx` — enviado no envio, com código de rastreamento
    - `order-delivered.tsx` — enviado na entrega, com CTA para avaliação
16. Todos os templates usam a paleta da marca Utilar (design system do Sprint 02); mobile-first; alt text nas imagens
17. Servidor de preview para edição de templates (`npm run email:dev`)
18. Mailer integrado às transições de status do order-service (reusa o padrão fan-out do notification-sender do Sprint 18 para o canal de e-mail)
19. Smoke com Litmus/Email-on-Acid — verificar renderização no Gmail, Outlook, iOS Mail, Android Mail. Cair na caixa de entrada, não no spam, para os 4 templates (testado com score mail-tester.com ≥ 9/10).

### Publicação do material jurídico
20. Privacidade + Termos revisados pelo advogado (do Sprint 24) publicados em `/privacidade` + `/termos`
21. Política de cookies em `/cookies` — alinhada com as categorias do banner de consentimento
22. Página de resumo LGPD em `/lgpd` publicada com links de autoatendimento para exportação + exclusão

### Onboarding de vendedores — 3 vendedores reais ativos
23. Identificar + assinar 3 vendedores reais de ferragens (rede do fundador); assinar Termos de Uso
24. Ligação de onboarding + modelo de planilha compartilhada para importação em lote (fluxo do Sprint 11)
25. Cada vendedor produz 20+ produtos (meta: 60+ no catálogo total, 5+ categorias: ferramentas elétricas, manuais, jardinagem, elétrica, hidráulica)
26. Aprovação do admin de cada vendedor + produtos (fluxo do Sprint 20)
27. QA de fotos: cada produto tem ao menos 1 imagem com 1280px+; imagens ausentes são bloqueante antes do lançamento

### Suíte final de smoke em produção
28. Pix sandbox: criar carrinho → checkout → pagar com QR sandbox → verificar `order.paid` + e-mail + push
29. Pix real de R$1: usar CPF real, pagar R$1 para um vendedor real, verificar de ponta a ponta e estornar depois
30. Estorno real de teste: processar reembolso pela UI do admin (Sprint 20) no pedido de R$1, verificar se o extrato de repasse do vendedor é atualizado
31. Teste de carga: script k6 com 50 usuários simultâneos navegando no catálogo por 10 min — verificar ausência de 5xx, p95 < 500ms
32. Segurança: executar scan de baseline OWASP ZAP em staging, corrigir ou aceitar todas as descobertas de alta severidade

### Plano de soft launch
33. Post de anúncio no Instagram rascunhado + agendado
34. Lista de convites com 50 amigos e familiares; enviar link + código de desconto `FRIENDSFAMILY50` (15% de desconto, com teto, válido por 2 semanas)
35. Dashboard de KPIs diários (Metabase ou página simples no Notion puxando do CloudWatch): pedidos/dia, GMV, ticket médio, produtos mais vendidos, funil de cadastro com drop-off. Compartilhado com os stakeholders.
36. Canal de feedback: formulário `/feedback` enviando para um webhook Slack; convidados da lista de invites solicitados diretamente a dar feedback
37. Reunião de go/no-go em T-2 dias; critérios de cancelamento do lançamento documentados (sucesso de pagamento < 95%, 5xx crítico, SES ainda no sandbox)

## Critérios de aceite

- [ ] Google Search Console mostra sitemaps enviados + indexados (ao menos homepage + 10 PDPs indexados em até 48h do envio)
- [ ] Os 4 e-mails transacionais caem na caixa de entrada no Gmail, Outlook, iOS Mail (não no spam) — score mail-tester ≥ 9/10
- [ ] 60+ produtos ativos em 5+ categorias de 3 vendedores aprovados
- [ ] Pix real de R$1 conclui de ponta a ponta e é estornado sem problemas
- [ ] Páginas legais (Privacidade, Termos, Cookies, LGPD) no ar com aprovação do advogado arquivada
- [ ] GA4 recebe eventos na visualização em tempo real quando o consentimento é concedido; recebe zero eventos quando apenas o consentimento essencial é dado
- [ ] Teste de carga k6 aprovado (50 simultâneos, 10 min, zero 5xx, p95 < 500ms)
- [ ] OWASP ZAP baseline: nenhuma descoberta de alta severidade sem resolução
- [ ] Post no Instagram do soft launch agendado; 50 convites para amigos e familiares enviados com código de desconto
- [ ] Dashboard de KPIs diários acessível aos stakeholders
- [ ] Reunião de go/no-go realizada; decisão registrada em `docs/launch-log.md`

## Dependências

- Sprint 22 (observabilidade) — lançar sem alertas é imprudente
- Sprint 23 (CI/CD) — hotfixes no dia do lançamento precisam de um caminho de rollback
- Sprint 24 (LGPD) — lançar sem Privacidade/Termos é ilegal no Brasil
- 3 vendedores reais assinados — confirmar em T-14 dias; manter 5 no pipeline em caso de desistências
- Revisão do advogado agendada — reserva confirmada no início do sprint
- DNS do domínio `utilarferragem.com.br` apontando para CloudFront
- SES fora do sandbox — enviar solicitação em T-10 dias

## Riscos

- Graduação do sandbox SES pode levar até 24h (às vezes mais se a AWS solicitar informações adicionais) — enviar em T-10 dias com descrição clara do caso de uso
- Vendedores desistindo em cima da hora — manter 5 assinados no pipeline; lançamento mínimo viável é 2 vendedores + 40 produtos, mas a meta é 3/60
- Atraso do advogado — iniciar a revisão jurídica no Sprint 24; se a aprovação atrasar, o lançamento atrasa (não lançar sem ela)
- Teste Pix real de R$1 encontra bug — reservar 2 dias para qualquer correção de última hora no pagamento; testar em T-5 dias
- Lista de convites do soft launch muito pequena para encontrar bugs — complementar com "ship" no Product Hunt ou teaser no Reddit `r/brasil` se os 50 convites não produzirem > 20 transações na primeira semana
