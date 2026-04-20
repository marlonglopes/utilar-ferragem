# ADR 011 — Mecanismo de cashback

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

Retenção e frequência de recompra são as duas alavancas que compõem o LTV de um marketplace. No mercado brasileiro, cashback é hoje uma expectativa básica: Méliuz, Ame (Americanas), Magalu Pay, PicPay e todo grande aplicativo bancário oferecem algum tipo de "dinheiro de volta". Um cliente que compra pela primeira vez e volta uma segunda vez tem ~5× mais probabilidade de voltar uma terceira. Um comprador de ferragens que gastou R$ 2.000 num kit de furadeira hoje e tem R$ 100 parados no saldo de cashback está fortemente inclinado a voltar para os consumíveis do dia a dia (broca, parafuso, disco).

Forças em jogo:
- Clientes no BR já conhecem cashback; sem necessidade de educação do consumidor
- Programas de pontos exigem explicar taxas de conversão, regras de expiração e mecânicas de tier — carga cognitiva que o comprador casual de bricolagem não vai absorver
- R$ como unidade de recompensa é legível de relance; sem confusão do tipo "800 Gifthy Points = R$ 10"
- Categorias de ferragens têm margens muito apertadas (10–25%); cashback financiado pela plataforma comeria diretamente o lucro bruto — o programa precisa ser financiado pelo vendedor para escalar
- Vendedores querem uma alavanca de crescimento, mas hoje não têm as ferramentas para rodar promoções por conta própria
- Vetores de abuso: loops de reembolso para cashback, cadeias de autocompra, acúmulo de saldo parado

Depende desta decisão: métricas de retenção da Fase 5, automação de repasse ao vendedor na Fase 6 (o passivo de cashback é descontado dos repasses).

## Decisão

### Por produto, financiado pelo vendedor, creditado na entrega

Cada produto carrega um `cashback_percent` (0,00–10,00, DECIMAL(4,2), padrão 0,00) definido pelo vendedor no cadastro/edição. Na compra, o percentual é registrado como snapshot no item do pedido para que uma alteração posterior não afete um pedido já realizado. Em `order.status = paid` emitimos `cashback.earned` → uma linha no ledger é escrita como `kind='earned', status='pending'`. Em `order.status = delivered` emitimos `cashback.credited` → a linha passa para `status='available'`.

### Modelo de financiamento

| Aspecto | Regra |
|--------|------|
| Quem financia | Vendedor (percentual por produto) |
| Quando cobrado | Descontado do repasse ao vendedor na liquidação — **não** da margem no momento da venda |
| Reforço da plataforma | Nenhum no MVP; revisitar na Fase 6 |
| Mecânica de liquidação | Deferida para o sprint de automação de repasse (Fase 6) |
| Tratamento contábil | Passivo em nossos livros a partir de `status=available` até `used`/`expired` |

Cobrar na liquidação (não na venda) significa que os vendedores não sentem impacto no fluxo de caixa por pedido e o ledger é a única fonte de verdade para o cálculo do repasse.

### Design do ledger

Uma tabela `cashback_ledger` no user-service. Todos os valores monetários armazenados em **centavos (BIGINT)**. As entradas são sempre positivas; `kind` codifica a direção.

| Coluna | Tipo | Notas |
|--------|------|-------|
| `id` | bigserial | — |
| `user_id` | bigint | FK para users, indexado |
| `order_id` | bigint nullable | NULL para ajustes manuais |
| `amount_cents` | bigint | Sempre > 0 |
| `currency` | string(3) | `BRL` no lançamento |
| `kind` | enum | `earned` / `redeemed` / `expired` / `reversed` / `adjusted` |
| `status` | enum | `pending` / `available` / `used` / `expired` |
| `earned_at` | timestamptz | Quando o pedido foi pago (para `earned`) |
| `available_at` | timestamptz nullable | Definido na transição `pending → available` |
| `expires_at` | timestamptz nullable | 12 meses após `available_at` |
| `metadata_json` | jsonb | Motivo, actor_id para `adjusted`, etc. |
| `created_at` | timestamptz | — |

Máquina de estados:

```
  pending ──(pedido entregue)──▶ available ──(resgatar)──▶ used
     │                                │
     │                                └──(12 meses)──▶ expired
     │
     └──(pedido cancelado/reembolsado antes da entrega)──▶ reversed
```

### Ganho, crédito, resgate, expiração, reversão

| Operação | Gatilho | Efeito no ledger |
|-----------|---------|---------------|
| **Ganho** | `order.status = paid` | Inserir `kind=earned, status=pending` para cada item: `qty * unit_price_cents * cashback_percent_snapshot / 100` |
| **Crédito** | `order.status = delivered` | Atualizar linhas `pending` correspondentes → `status=available, available_at=now, expires_at=now+12mo` |
| **Resgate** | `POST /me/cashback/redeem` | Inserir `kind=redeemed, status=used` para o valor resgatado; advisory-lock em `user_id` |
| **Expiração** | Job Sidekiq noturno | Linhas `available` onde `expires_at < now` → `status=expired`; inserir linha espelho `kind=expired` para auditoria |
| **Reversão** | Cancelamento/reembolso de pedido antes da entrega | Linhas `pending` para esse `order_id` → `status=reversed`; inserir linha espelho `kind=reversed` |
| **Ajuste** | `POST /admin/cashback/adjust` | Inserir `kind=adjusted, status=used` (débito) ou `status=available` (crédito) com `metadata_json.reason` |

### Regras de resgate

| Regra | Valor |
|------|-------|
| Resgate mínimo | R$ 5,00 (500 centavos) |
| Máximo por pedido | `min(saldo_disponível, 50% * subtotal_pedido_em_centavos)` |
| Granularidade | Reais inteiros (incrementos de 100 centavos) |
| Escopo | Somente subtotal dos produtos — **não** frete, **não** impostos |
| Acumulável com cupons | Não (cupons não existem no MVP; revisitar quando chegarem) |
| Contas Pro/B2B | Excluídas no MVP (têm precificação separada de qualquer forma) |

O fluxo de resgate retorna um `redemption_id` que o SPA passa para o payment-service ao criar a cobrança. O payment-service verifica com o user-service antes de aplicar o desconto. Se o pagamento falhar ou for revertido, o resgate é liberado (linha `kind=redeemed` marcada como `status=reversed`, saldo restaurado).

### Eventos

Todos publicados no Redpanda. Consumidores devem ser idempotentes em `(order_id, kind)`.

| Tópico | Produtor | Consumidor | Gatilho |
|-------|----------|----------|---------|
| `cashback.earned` | order-service | user-service (módulo cashback) | `order.status: pending_payment → paid` |
| `cashback.credited` | order-service | user-service (módulo cashback) | `order.status: shipped → delivered` |
| `cashback.redeemed` | user-service | analytics / notification-service | Resgate bem-sucedido |
| `cashback.expired` | user-service | notification-service | Job de expiração noturno |
| `cashback.reversed` | user-service | analytics / notification-service | Cancelamento/reembolso de pedido |

### Endpoints

| Método | Caminho | Auth | Finalidade |
|--------|------|------|---------|
| `GET` | `/api/v1/me/cashback` | JWT do cliente | `{ available_balance_cents, pending_balance_cents, next_expiration: {amount_cents, expires_at} \| null }` |
| `GET` | `/api/v1/me/cashback/history?page=N&per=20` | JWT do cliente | Linhas do ledger paginadas |
| `POST` | `/api/v1/me/cashback/redeem` | JWT do cliente | Body `{ order_id, amount_cents }` → `{ redemption_id }` |
| `POST` | `/api/v1/admin/cashback/adjust` | JWT de admin | Body `{ user_id, amount_cents, reason, direction: credit\|debit }` |

### Alternativas comparadas

| Opção | Complexidade | Impacto na margem | Incentivo ao vendedor | Clareza para o cliente | Veredicto |
|--------|------------|---------------|------------------|------------------|---------|
| **Cashback por produto, financiado pelo vendedor** | Média | Neutro para a plataforma | Forte (vendedor controla o %) | Alta (R$ nativo) | **ESCOLHIDA** |
| Pontos de fidelidade (estilo Méliuz "Utilar Points") | Alta | Variável | Nenhum | Baixa (confusão com a taxa de conversão) | Rejeitado — carga cognitiva |
| Cashback flat financiado pela plataforma (ex.: 1% em tudo) | Baixa | Forte pressão sobre a margem | Nenhum | Alta | Rejeitado — corrói a margem da plataforma sem skin in the game do vendedor |
| Cashback por tier (clientes Bronze/Prata/Ouro) | Muito alta | Variável | Fraco | Média | Rejeitado — complexo demais para o MVP; revisitar na Fase 6 |
| Somente cupons (sem cashback) | Baixa | Variável | Variável | Alta | Rejeitado — sem flywheel de retenção, sem saldo recorrente |

## Consequências

### Positivo
- **Flywheel de recompra**: clientes com saldo positivo têm materialmente mais probabilidade de retornar
- **Alavanca controlada pelo vendedor**: vendedores que querem volume aumentam o `cashback_percent`; os que não querem deixam em 0 — pressão competitiva natural
- **Unidade em R$ nativo** é instantaneamente legível; sem o imposto cognitivo de "quanto vale um ponto?"
- **Design baseado em ledger** torna reembolsos, auditorias e contabilidade tratáveis — toda mudança de saldo tem uma linha
- **Aditivo ao schema existente** — sem breaking changes em `users`, `orders` ou `payments`
- **Desconta na liquidação** — sem dor de fluxo de caixa por venda para os vendedores
- **Passivo limitado** — expiração em 12 meses limita o float em aberto

### Negativo
- Volume do ledger cresce ~1 linha por item de pedido no lançamento; ~2 linhas por pedido na entrega. A 1M de pedidos/ano com média de 2 itens → ~4M linhas/ano. Aceitável; revisitar particionamento na Fase 6
- O fluxo de resgate adiciona um hop entre serviços (payment-service → user-service); P95 deve ficar < 200ms — usar advisory locks + uma prepared SQL statement
- Job de expiração é mais uma superfície de cron que pode falhar silenciosamente — deve alertar quando 0 linhas foram processadas quando se esperava processamento
- Confusão do vendedor sobre "por que meu repasse diminuiu?" — clareza do item da fatura importa (UX de liquidação da Fase 6)
- Confusão do cliente sobre pendente vs disponível — o texto da UI deve ser preciso ("Creditado após entrega")

### Alternativas rejeitadas
- **Cashback acumulável com cupons**: o MVP não tem cupons; deferir a questão evita uma regra de ordenação prematura (aplicar cupom primeiro? cashback primeiro?). Revisitar quando os cupons chegarem.
- **Cashback sobre frete**: frete é um custo de repasse à transportadora (Correios/Jadlog) no lançamento; ganhar cashback sobre ele significaria a plataforma financiando um desconto de frete. Somente subtotal dos produtos.
- **Cashback para contas Pro/B2B**: contas pro têm precificação por tier e faturamento com NF-e (Fase 5); sobrepor cashback embaralha a proposta de valor. Revisitar após o Pro ser lançado.
- **Serviço separado de cashback**: um módulo no user-service é o correto para <10M eventos/ano. Extrair para um serviço próprio somente se: (a) volume do ledger >10M linhas/ano, (b) QPS de leitura em `/me/cashback` ultrapassa 500 rps, ou (c) adicionamos tipos de recompensa multi-moeda.
- **Creditar em `paid` (não em `delivered`)**: abre arbitragem de reembolso para cashback. Aguardar a entrega é o padrão nas plataformas brasileiras (Méliuz, Ame) e é o default seguro.

## Questões em aberto

1. **Campanhas de reforço financiadas pela plataforma** (ex.: 2× cashback nos fins de semana, impulsos por categoria)? Resposta: defer para a Fase 6. O ledger já suporta `kind=adjusted, metadata_json.campaign_id` para que possamos adicionar isso sem mudanças de schema.
2. **Reembolsos parciais de pedido** (1 de 3 itens reembolsados após a entrega)? Resposta: reverter pro-rata — calcular o valor revertido somente dos itens reembolsados, emitir um `cashback.reversed` com `metadata_json.partial=true`.
3. **Avisos de expiração** — devemos enviar e-mail para os clientes 30 dias antes do cashback expirar? Resposta: sim, via notification-service ([ADR 010](010-notification-architecture.md)) como e-mail transacional. Agrupa diariamente: um único e-mail lista tudo que expira nessa janela de 30 dias.
4. **Cashback em pedidos cancelados pelo vendedor** antes da confirmação do pagamento? Resposta: nunca foi ganho — `cashback.earned` dispara somente em `paid`, portanto não há nada a reverter.
5. **Exclusão de cliente (LGPD)** — o que acontece com o saldo de cashback em aberto? Resposta: zerar com uma linha terminal `kind=adjusted, reason='lgpd_deletion'`; decisão documentada no sprint de LGPD.
6. **Multi-moeda** — produtos em USD ganham cashback em USD? Resposta: o MVP é somente BRL (o catálogo da Utilar é todo em BRL). Quando um vendedor não-BRL for para o Gifthy mais amplo, revisitar; provavelmente forçar resgate na mesma moeda.
7. **Detecção de fraude** — redes de autocompra onde um vendedor compra seu próprio produto para ganhar cashback? Resposta: fora do escopo do MVP; depende do KYC do vendedor do Sprint 15 + monitoramento de anomalias da Fase 6.
