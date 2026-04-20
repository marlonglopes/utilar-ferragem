# Fase 4 — Vertical do Vendedor

**Objetivo**: transformar a Utilar Ferragem de "catálogo de vendedores pré-cadastrados" em um marketplace self-service onde qualquer empresa legítima de ferragens pode entrar e vender.

## Sprints

- [Sprint 10 — Fluxo de onboarding de vendedores](../sprints/sprint-10-seller-onboarding.md)
- [Sprint 11 — Importação em massa de SKUs (CSV)](../sprints/sprint-11-bulk-import.md)
- [Sprint 14 — Fretes + rastreamento (Melhor Envio)](../sprints/sprint-14-shipping-correios.md)
- [Sprint 15 — Disputas de pagamento, reembolsos, chargebacks](../sprints/sprint-15-payment-disputes.md)
- [Sprint 20 — Console admin da Utilar (no gifthy-hub)](../sprints/sprint-20-utilar-admin.md)

## Definição de pronto

- Página de destino `/vender`: proposta de valor, preços, depoimentos, CTA claro
- Wizard de onboarding:
  1. Entrada do CNPJ + consulta à Receita Federal (ou preenchimento manual como fallback)
  2. Perfil da empresa (nome fantasia, telefone, endereço via CEP)
  3. Seleção de categoria (deve corresponder a ≥ 1 folha de ferragens)
  4. Concordância com a comissão + aceite dos termos
  5. Conta bancária para repasse
  6. Redirecionamento para o gifthy-hub com registro de vendedor pré-preenchido
- Vendedor pode fazer upload de produtos via gifthy-hub
- Produtos aparecem no catálogo da Utilar Ferragem em até 5 minutos após a aprovação
- Fluxo de aprovação pelo admin: novos vendedores começam com `status = pending`, o admin aprova pelo painel admin do gifthy-hub antes dos produtos irem ao ar
- Primeiro produto cadastrado → e-mail de boas-vindas com próximos passos
- Dashboard do vendedor no gifthy-hub exibe métricas específicas da Utilar Ferragem: visitas aos produtos do vendedor, conversão, top categorias

## Checkout multi-vendedor (se deferido da Fase 3)

Se um carrinho contiver itens de mais de 1 vendedor, dividir em N pedidos separados no checkout, cada um com seu próprio frete. Uma única transação de pagamento para o cliente; fundos divididos no server-side. O tradeoff de complexidade é decidido durante o Sprint 08; se deferido, fica aqui.

## Trabalho de backend

- **user-service**: formalizar transições de `seller.status` (`pending` → `approved` → `active` / `suspended`)
- **product-service**: filtrar produtos de vendedores não aprovados nos endpoints públicos
- **order-service**: suporte à divisão do carrinho por vendedor na criação do pedido
- **Interface admin** (gifthy-hub): aprovar / rejeitar vendedores pendentes com motivo

## Fora do escopo

- KYC totalmente automatizado (Sprint 10 faz consulta de CNPJ + fila de aprovação manual; regras de fraude no [ADR 009](../adr/009-seller-kyc-fraud.md))
- App do vendedor para mobile — o gifthy-hub responsivo é suficiente
- Repasses automatizados (Sprint 20 exporta CSV para repasse manual; ACH automatizado na Fase 6)
- Suporte ao vendedor via WhatsApp Business ([Sprint 16](../sprints/sprint-16-support-tooling.md) difere WhatsApp explicitamente)

## Saída para a Fase 5 quando

- ≥ 10 vendedores externos (não do seed) ativos
- ≥ 1.000 SKUs de vendedores externos no ar
- Tempo médio de onboarding de vendedores < 10 minutos
