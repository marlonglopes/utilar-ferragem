# Utilar Ferragem — Planos de Monetização

> Documento estratégico para estruturar como cobrar pela plataforma. Não é um documento de operação — é referência para decisões comerciais futuras.
>
> Última atualização: 2026-04-23 · Câmbio de referência: R$ 5,20/USD · Mercado: Brasil

## Sumário

1. [Resumo executivo](#1-resumo-executivo)
2. [Modelo A — SaaS / white-label](#2-modelo-a--saas--white-label)
3. [Modelo B — Custom / SOW](#3-modelo-b--custom--sow)
4. [Comparativo direto](#4-comparativo-direto)
5. [Estratégia híbrida recomendada](#5-estratégia-híbrida-recomendada)
6. [Custo operacional de referência](#6-custo-operacional-de-referência)
7. [Premissas e ressalvas](#7-premissas-e-ressalvas)

---

## 1. Resumo executivo

A plataforma pode ser monetizada por dois caminhos principais:

| Caminho | Quando faz sentido | Receita típica/cliente | Tempo até receita |
|---|---|---|---|
| **SaaS white-label** | Vender para muitas ferragens | R$ 590–3.900/mês + take rate | 6–12 meses (preparação) |
| **Custom / SOW** | Atender 1–2 clientes grandes | R$ 248–358k por projeto | 30 dias (sinal) |

Recomenda-se a **estratégia híbrida**: começar com SOWs custom para gerar caixa e validar features, depois usar o capital para construir multi-tenancy e lançar SaaS com clientes-âncora.

---

## 2. Modelo A — SaaS / white-label

### Tese

Plataforma multi-tenant: cada ferragem é um tenant com sua marca, catálogo, vendedores e domínio. Você opera infra + roadmap; eles pagam mensalidade + take rate sobre GMV.

### Tiers

| Plano | Setup único | Mensalidade | Take rate (GMV) | Limites |
|---|---|---|---|---|
| **Starter** | R$ 3.000 | R$ 590 | 1,5% | 1 vendedor, até 500 SKUs, subdomínio `lojinha.utilarhub.com.br` |
| **Pro** | R$ 9.000 | R$ 1.490 | 1,0% | Até 10 vendedores, ~5k SKUs, domínio próprio, suporte prioritário |
| **Enterprise** | R$ 25.000 | R$ 3.900 | 0,5% | 50+ vendedores, integrações custom, SLA 99,5%, gerente de conta |

### Economia unitária — exemplo Pro

```
GMV mensal médio do tenant:           R$ 80.000
  Mensalidade:                         R$ 1.490
  Take rate 1% sobre R$ 80k:           R$ 800
                                       ─────────
  Receita bruta/cliente:               R$ 2.290/mês

Custo marginal por tenant (Fase 2 compartilhada):
  Infra incremental (DB row-level):    R$ 80
  Suporte humano (1h/mês × R$ 60):     R$ 60
                                       ─────────
  Custo marginal:                      R$ 140

Margem bruta por cliente:              R$ 2.150/mês  (94%)
```

### Break-even

```
Custos fixos mensais (você + infra base + roadmap):  R$ 25.000–35.000
Margem bruta por Pro:                                 R$ 2.150
Break-even:                                           ~14 clientes Pro
Tempo estimado até break-even:                        12–18 meses

ARR estimado a 30 clientes Pro:                       R$ 825.000
```

### Investimento extra antes do 1º cliente

Funcionalidades **fora do roadmap atual** necessárias para SaaS multi-tenant:

| Capacidade | Esforço | Valor |
|---|---|---|
| Multi-tenancy (RLS Postgres + tenant_id em todas queries) | 15 dev-days | R$ 18–22k |
| Onboarding self-service (signup, escolha de plano, MP recorrente) | 10 dev-days | R$ 12–15k |
| Painel admin do tenant (branding, vendedores, financeiro) | 15 dev-days | R$ 18–22k |
| Subdomínio dinâmico + custom domain com ACM cert per-tenant | 5 dev-days | R$ 6–8k |
| Cobrança recorrente + dunning (inadimplência) | 10 dev-days | R$ 12–15k |
| **Total** | **55 dev-days** | **R$ 65–90k** |

### Riscos

- **CAC alto** — venda B2B para ferragem tradicional exige campo, feira, indicação. Não funciona com Google Ads. Ciclo de venda de 3–6 meses.
- **Churn precoce** — se o lojista não vender no primeiro mês, cancela. Necessário onboarding com mão na massa nos primeiros 30 dias.
- **Dependência de 3 pilotos** — vender frio sem cases é quase impossível. Precisa de mínimo 3 lojas rodando + faturando antes do go-to-market.
- **Suporte 24×7** — qualquer downtime afeta múltiplos lojistas simultaneamente. Necessário plantão ou pelo menos PagerDuty/UptimeRobot agressivo.

---

## 3. Modelo B — Custom / SOW

### Tese

Uma ferragem grande (ou rede regional, ou cooperativa) contrata você para entregar a plataforma branded, com handoff de código e deploy no ambiente do próprio cliente.

### Pacotes fixed-price

| Fase | Entregáveis | Prazo | Valor |
|---|---|---|---|
| **0 — Discovery** | Levantamento, alinhamento de escopo, backlog priorizado, mockups de identidade visual | 2 sem | **R$ 18.000** *(não-reembolsável)* |
| **1 — MVP comércio** | Sprints 01–09 (já existem), customização de branding/catálogo/vendedor inicial | 4 sem | **R$ 65.000** |
| **2 — Operação** | Sprints 10, 11, 14, 15, 20 (onboarding vendedor, import CSV, frete real Melhor Envio, disputas, admin console) | 8 sem | **R$ 95.000** |
| **3 — Lançamento** | Sprints 22, 23, 24, 25 (observabilidade, CI/CD + IaC, LGPD, prontidão de lançamento) | 6 sem | **R$ 70.000** |
| **4 — Crescimento** *(opcional)* | Sprints 12, 17, 18, 19, 26 (reviews, busca Meilisearch, PWA + push, recomendações, cashback) | 12 sem | **R$ 110.000** |

**Pacote mínimo viável (Fases 0–3):** R$ 248.000 / 5 meses
**Pacote completo (Fases 0–4):** R$ 358.000 / 8 meses

### O que está incluso

- Todo o código-fonte (repositório entregue, sem lock-in tecnológico)
- Deploy em AWS do cliente (ou em ambiente seu, se for modelo SaaS hospedado — ver suporte)
- Documentação técnica completa + 4h de treinamento da equipe técnica do cliente
- 90 dias de garantia (correção de bugs sem custo adicional)
- 1 mês de suporte 8×5 incluso (transição)

### O que **não** está incluso

| Item | Como cobrar |
|---|---|
| Cadastro de catálogo inicial | R$ 8/SKU ou contratar terceiro |
| Fotos de produto profissionais | Repassar fotógrafo (~R$ 25/foto) |
| Identidade visual / logo | R$ 6–15k (designer separado) |
| Conteúdo (copy, blog, FAQ) | Hora consultor copywriter (~R$ 120/h) |
| Integração contábil / ERP customizada | T&M R$ 250/h |
| Gateway alternativo (Pagar.me, Stone, Cielo) | T&M, ~R$ 8–15k |
| Treinamento adicional da equipe operacional | R$ 1.500/dia |

### Suporte pós-entrega (recorrente)

| Plano | Mensalidade | SLA resposta | Inclui |
|---|---|---|---|
| **Manutenção básica** | R$ 2.500 | 24h business | Correção de bugs, atualização de dependências, monitoramento, backup |
| **Manutenção + evolução** | R$ 6.500 | 8h business | + 8h/mês de novas features ou ajustes |
| **Operação dedicada** | R$ 15.000 | 1h business | + sustentação 24×7 + 30h/mês de evolução |

### Cláusulas críticas no contrato

- **Mudança de escopo (CR):** qualquer requisito fora do backlog inicial vira Change Request cobrado a **R$ 250/h**, com aprovação por e-mail antes do início
- **Pagamento:** 30% sinal + 30% no fim de cada fase + 10% retenção liberada após 30 dias de produção sem bugs P1
- **Propriedade intelectual:** código vai para o cliente, mas você mantém **direito de reuso** de componentes genéricos (UI primitives, payment-service abstrato, design system base) em outros projetos
- **Cap em T&M de suporte:** 60h/mês ou estoura → renegociar contrato
- **Não-concorrência:** cliente não pode revender a plataforma como SaaS concorrente por 24 meses
- **Exclusividade regional:** opcional — cliente paga +50% sobre o pacote para garantir que você não venda a mesma stack a concorrente direto na mesma cidade/região por 12 meses

### Riscos

- **Scope creep** — cliente vai pedir "só mais essa coisinha" toda semana. CR rigoroso é a única defesa
- **Calote nas parcelas finais** — risco real. Retenção pequena + entrega progressiva mitiga, mas nunca elimina
- **Esquecer de fechar o suporte recorrente** — é onde está o dinheiro de longo prazo. Negociar no mesmo contrato do projeto

---

## 4. Comparativo direto

|  | SaaS (white-label) | Custom (SOW) |
|---|---|---|
| **Capital de giro inicial** | Alto — investe ~R$ 80k antes do 1º cliente | Baixo — sinal cobre primeiros custos |
| **Tempo até receita** | 6–12 meses (build + GTM) | 30 dias (assinatura do discovery) |
| **Receita previsível** | Alta (MRR recorrente) | Baixa (projeto-a-projeto) |
| **Escala** | Marginalmente lucrativa após 5+ clientes | Linear ao seu tempo disponível |
| **Dependência operacional** | Suporte 24×7 obrigatório | Pontual no projeto + opcional após |
| **Valuation/exit** | Múltiplo de ARR (5–10× para SaaS B2B vertical) | Vale o que você produz no ano corrente |
| **Risco de concentração** | Diluído (N clientes) | Alto (1–2 clientes podem ser >80% da receita) |
| **Complexidade jurídica** | Termos de uso + LGPD + processador de dados | SOW + IP + cláusulas de suporte |
| **GTM (Go-to-Market)** | Conteúdo + indicação + feiras setoriais | Networking + indicação + RFP |

---

## 5. Estratégia híbrida recomendada

**Fluxo em 3 atos:**

1. **Ato 1 — Caixa (mês 0–8)**: vender 1–2 SOWs custom. Gera R$ 250–500k de caixa, valida features no mundo real, cria 2 cases reais.
2. **Ato 2 — Construção (mês 6–14, sobreposto)**: usar parte do caixa (R$ 65–90k) para construir multi-tenancy. Os clientes custom continuam pagando suporte recorrente (R$ 5–20k/mês cada).
3. **Ato 3 — SaaS (mês 12+)**: lançar SaaS com 3 lojas-piloto vindas dos contratos custom (descontadas para servirem como referência). Eles viram âncoras + cases + indicadores.

**Por quê funciona:**
- Cliente custom paga pelo desenvolvimento do produto
- Cliente custom vira primeiro caso de sucesso do SaaS
- Capital próprio em vez de captação externa
- Risco diluído em duas fontes de receita simultâneas

**Por quê pode falhar:**
- Cliente custom vira concorrente (mitigado por cláusula de não-concorrência)
- SOW consome 100% do tempo e o SaaS nunca sai do papel
- Difícil dizer "não" a CRs do cliente custom enquanto se tenta construir multi-tenancy

---

## 6. Custo operacional de referência

Independente do modelo, a infra tem o mesmo perfil de custo (já documentado em [docs/14-infra-custos.md](docs/14-infra-custos.md)):

| Fase de infra | Mensalidade | Quando se aplica |
|---|---|---|
| MVP | R$ 125–180 | Custom: cliente paga · SaaS: 1–3 tenants |
| Estabilização | R$ 280–500 | Custom: cliente paga · SaaS: 4–15 tenants compartilhados |
| Escala | R$ 900–1.400 | SaaS: 15+ tenants, ou custom Enterprise com SLA |

**Mercado Pago (em qualquer modelo):**
- Pix: 0,99% por transação
- Cartão de crédito 1×: 4,98%
- Boleto: R$ 3,49 por boleto
- Sem mensalidade

No modelo SaaS, a take rate cobre a margem sobre o que o MP cobra. No modelo custom, o cliente é quem tem a conta MP — você não toca no fluxo financeiro.

---

## 7. Premissas e ressalvas

- **Mercado:** Brasil, ferragens / construção civil / lojas independentes. Não validado para outros setores ou países.
- **Câmbio:** R$ 5,20/USD. Reavaliar números em USD se câmbio variar mais de ±10%.
- **Hora dev (referência 2026):** sênior solo R$ 150/h · agência média R$ 250/h · agência top R$ 400/h.
- **Custos fixos mensais estimados:** assumem você operando solo no Ato 1, com freelancer pontual no Ato 2. Custos sobem ~R$ 20k/mês a cada engenheiro contratado em CLT.
- **Não inclui:** tributação (Simples ~6–15%, Lucro Presumido ~13–16% para serviços), contabilidade (~R$ 800/mês), jurídico (consulta inicial ~R$ 5–8k), captação de clientes (eventos, mídia paga).
- **Multipliers de valuation:** SaaS B2B vertical no Brasil tem múltiplo médio de 4–8× ARR em rounds seed/series-A (2026). Não é compromisso — depende de churn, NRR e crescimento.

> **Reavaliação:** este documento deve ser revisitado antes de fechar qualquer contrato comercial. Os números são pontos de partida para negociação, não tabela final.
