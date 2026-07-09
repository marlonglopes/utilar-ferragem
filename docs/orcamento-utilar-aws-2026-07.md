# Orçamento de Infraestrutura — Utilar Ferragem

**Para:** Thomaz Sanchotene — proprietário, Utilar Ferragem
**De:** Equipe de Engenharia
**Data:** 08 de julho de 2026
**Assunto:** Custo de infraestrutura em nuvem (AWS) para colocar a Utilar Ferragem no ar, replicando exatamente o mesmo ambiente de produção que já opera o Gifthy.

---

## 1. Resumo executivo

Este orçamento reproduz, para a Utilar, a **mesma arquitetura de produção** que hoje sustenta o Gifthy na AWS (região São Paulo — `sa-east-1`). Os valores abaixo **não são estimativas de tabela**: foram extraídos da fatura real da AWS do Gifthy, componente por componente.

| | Valor mensal | Valor anual |
|---|---:|---:|
| **Infraestrutura AWS (réplica dedicada)** | **US$ 315,44 ≈ R$ 1.703** | **US$ 3.785 ≈ R$ 20.441** |
| Domínio + e-mail transacional | ≈ R$ 10 | ≈ R$ 120 |
| **Total recorrente** | **≈ R$ 1.713/mês** | **≈ R$ 20.560/ano** |
| Gateway de pagamento (Appmax) | Sem mensalidade — só % por venda | Proporcional à receita |

> **Câmbio de referência:** R$ 5,40 / US$ (ajustável na data do contrato). Todos os valores AWS já refletem os **impostos brasileiros** cobrados na fatura.

**Duas observações importantes:**

1. A Appmax (gateway de pagamento escolhido) **não cobra mensalidade** — apenas um percentual por transação. Portanto não entra no custo fixo de infraestrutura; é um custo variável proporcional às vendas (detalhes na seção 5).
2. Existe espaço para **reduzir de 25% a 40%** este valor com planos de reserva da AWS ou compartilhando o servidor com o Gifthy. As opções estão na seção 6.

---

## 2. O que é este ambiente

É uma cópia fiel do ambiente que já roda o Gifthy em produção: um servidor de aplicação, um banco de dados gerenciado, balanceador de carga, firewall de aplicação (WAF), backups e armazenamento de arquivos. Tudo na região da AWS em **São Paulo**, garantindo baixa latência para clientes no Brasil.

```
Internet
   │
   ▼
WAF (firewall)  →  Load Balancer (HTTPS)  →  EC2 t3.large  (todos os serviços em Docker)
                                                   │
                                                   ├── SPA React (loja)
                                                   ├── catalog / order / auth / payment (Go)
                                                   └── Redis + fila de eventos
                                                   │
                                           RDS PostgreSQL (banco gerenciado, backups automáticos)
                                                   │
                              S3 (arquivos/imagens)  •  ECR (imagens Docker)
```

---

## 3. Detalhamento de custos AWS (valores reais medidos)

Custos observados na fatura do Gifthy em **junho/2026**, região São Paulo. É exatamente o que a réplica da Utilar consumiria.

| Componente | Especificação | US$/mês | R$/mês |
|---|---|---:|---:|
| **Servidor de aplicação** (EC2) | `t3.large` — 2 vCPU, 8 GB RAM | 96,52 | 521,21 |
| **Banco de dados** (RDS PostgreSQL) | `db.t3.small`, 200 GB SSD, backups | 98,52 | 532,01 |
| **Load Balancer** (ALB) | Balanceador HTTPS | 24,62 | 132,95 |
| **Firewall de aplicação** (WAF) | Proteção contra ataques web | 8,85 | 47,79 |
| **Disco** (EBS gp2) | Volume SSD (~190 GB) | 19,00 | 102,60 |
| **Backups de disco** (snapshots) | Cópias automáticas do disco | 6,80 | 36,72 |
| **IP público** (IPv4) | Endereços fixos em uso | 18,06 | 97,52 |
| **Armazenamento de arquivos** (S3) | Assets e imagens | 1,77 | 9,56 |
| **Transferência de dados** | Tráfego entre zonas | 0,88 | 4,75 |
| **Registro de imagens** (ECR) | Imagens Docker | 0,10 | 0,54 |
| **Monitoramento de custo** | Cost Explorer + Budgets | 1,99 | 10,75 |
| **Subtotal (infraestrutura)** | | **277,11** | **1.496,39** |
| Impostos (AWS Brasil, ~13,8%) | | 38,33 | 206,98 |
| **TOTAL AWS / mês** | | **315,44** | **1.703,38** |

### Domínio e e-mail (fora da AWS)

| Item | Provedor | Custo |
|---|---|---|
| `utilarferragem.com.br` | Registro.br | R$ 40,00/ano (≈ R$ 3,33/mês) |
| E-mail transacional (SES) | AWS SES | ~US$ 0,10 por 1.000 e-mails — desprezível no início (~R$ 5/mês) |

---

## 4. Histórico de custo do Gifthy (referência de estabilidade)

Custo total real do ambiente Gifthy nos últimos 6 meses, para dar previsibilidade:

| Mês | Custo total (US$) | Custo total (R$) |
|---|---:|---:|
| Jan/2026 | 537,68 | 2.903 |
| Fev/2026 | 499,47 | 2.697 |
| Mar/2026 | 537,84 | 2.904 |
| Abr/2026 | 524,83 | 2.834 |
| Mai/2026 | 536,80 | 2.899 |
| **Jun/2026** | **315,44** | **1.703** |

> Os meses de janeiro a maio ficaram em torno de US$ 500–537 porque incluíam **volumes e snapshots extras** (ambiente de testes/dados históricos) que foram desativados. **Junho (US$ 315) representa o custo real do ambiente enxuto** — e é a base deste orçamento. Ou seja, o número que usamos já é o cenário otimizado.

---

## 5. Gateway de pagamento — Appmax

A Utilar usará a **Appmax** como meio de pagamento (Pix, boleto e cartão de crédito, parcelamento em até 21x, recebimento em D+1).

- **Sem mensalidade e sem custo de setup** — a Appmax cobra apenas um **percentual por transação aprovada** (MDR).
- O custo é 100% **proporcional à receita**: só se paga quando há venda.
- As taxas exatas (Pix, boleto, cartão à vista e parcelado) são **negociadas diretamente com a Appmax** conforme o volume da Utilar. Valores típicos de mercado ficam em torno de ~1% no Pix, taxa fixa por boleto e ~3–4% no cartão, mas isso é definido no contrato com a Appmax.
- Do lado técnico, a integração **já está implementada** no sistema (Pix e boleto prontos server-side; cartão aguardando só a configuração da conta). Assim que a conta da Utilar for criada, é ligar a credencial e validar.

---

## 6. Opções para reduzir custo

O valor da seção 3 é o de "lista" (on-demand). Há três caminhos para baixar a conta, do mais conservador ao mais econômico:

### Opção A — Planos de reserva AWS (recomendado) 💡
Comprometendo-se com 1 ano de uso (Savings Plan / Reserved Instances) para o servidor e o banco:

| Componente | On-demand | Com reserva 1 ano | Economia |
|---|---:|---:|---:|
| EC2 `t3.large` | US$ 96,52 | ~US$ 58 | ~40% |
| RDS `db.t3.small` | US$ 98,52 | ~US$ 64 | ~35% |

**Novo total:** ~US$ 232/mês ≈ **R$ 1.253/mês** → economia de **~R$ 5.400/ano**, sem mudar nada na arquitetura.

### Opção B — MVP enxuto (primeiros meses)
Para começar com tráfego baixo e crescer depois: servidor `t3.small`, banco menor (ou PostgreSQL no próprio servidor), sem Load Balancer nem WAF no início.
**Custo:** ~US$ 50–70/mês ≈ **R$ 270–380/mês**. Migração para o ambiente completo quando o movimento justificar.

### Opção C — Compartilhar o servidor do Gifthy
O servidor atual do Gifthy tem folga de capacidade. A Utilar poderia rodar como containers adicionais no mesmo servidor, com **custo incremental de infraestrutura próximo de zero** no início.
**Contrapartida:** os dois projetos passam a compartilhar o mesmo servidor (uma eventual instabilidade afeta ambos). Recomendado apenas na largada, com plano de separar quando a Utilar ganhar tração.

---

## 7. Recomendação

| Cenário | Custo mensal | Quando faz sentido |
|---|---:|---|
| **Réplica completa (on-demand)** | R$ 1.713 | Ambiente dedicado e independente desde o Dia 1 |
| **Réplica completa + reserva 1 ano** ✅ | **R$ 1.253** | **Melhor custo-benefício para operação séria** |
| MVP enxuto | R$ 270–380 | Validar o negócio com investimento mínimo |
| Compartilhado com Gifthy | ~R$ 0 incremental | Largada rápida, aceitando risco compartilhado |

**Nossa recomendação:** subir a **réplica completa com plano de reserva de 1 ano** (~R$ 1.253/mês). Entrega o mesmo nível de confiabilidade do Gifthy — banco gerenciado com backup, balanceador, firewall e ambiente dedicado — pelo menor custo possível sem abrir mão de robustez.

---

## 8. Próximos passos

1. **Aprovação do orçamento** e definição do cenário (recomendamos o de reserva 1 ano).
2. **Criação da conta Appmax** em nome da Utilar (em andamento pelo Thomaz) → validação do checkout em ambiente de testes.
3. **Registro do domínio** `utilarferragem.com.br` no Registro.br (exige CNPJ).
4. **Provisionamento da infraestrutura** na AWS (réplica do ambiente Gifthy) — 1 a 2 dias úteis.
5. **Deploy e validação** ponta a ponta (catálogo, pedidos, pagamento via Appmax).

---

*Valores em dólar extraídos diretamente da fatura AWS (região sa-east-1) via AWS Cost Explorer em 08/07/2026. Conversão a R$ 5,40/US$ — recalcular na data do contrato. Impostos AWS Brasil já incluídos nos totais.*
