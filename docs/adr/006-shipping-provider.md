# ADR 006 — Seleção de provedor de frete

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

A Utilar Ferragem envia produtos físicos por todo o Brasil — ferragens frequentemente pesadas e volumosas (furadeiras, caixas de parafusos, ferramentas elétricas). O custo e o prazo de entrega são visíveis durante o checkout e influenciam materialmente a conversão. Precisamos de uma estratégia que (a) permita lançar sem depender de um contrato com transportadora, (b) escale para cotação multi-transportadora real quando o volume justificar, e (c) mantenha a UX do vendedor simples (um formulário, um fluxo de etiqueta).

Forças em jogo:
- Integração direta com Correios requer contrato (`contrato de vendedor`), certificação e uma superfície SOAP/REST hostil
- O frete para o Norte e Nordeste é sistematicamente mais caro do que para o Sudeste/Sul — qualquer tabela de frete fixo deve ser segmentada por região ou absorvemos prejuízo
- Vendedores precisam imprimir etiquetas de envio e receber códigos de rastreamento de volta no app
- Itens pesados (> 30kg) são um caso de borda comum em ferragens; algumas transportadoras recusam, outras cobram adicional

Dependem disto: checkout Phase 3 (tabela fixa), integração com transportadora Phase 4 (Melhor Envio), UI de impressão de etiqueta do vendedor.

## Decisão

### Phase 3 / Sprint 08 — Tabela de frete fixo por região

Tabela de tarifas com chave na macrorregião da UF de destino, com faixas de peso:

| Região | Estados | 0–3kg | 3–10kg | 10–30kg |
|--------|---------|-------|--------|---------|
| **SE** | SP, RJ, MG, ES | R$ 19 | R$ 29 | R$ 49 |
| **S** | PR, SC, RS | R$ 25 | R$ 39 | R$ 65 |
| **CO** | GO, MT, MS, DF | R$ 29 | R$ 45 | R$ 79 |
| **NE** | BA, PE, CE, PB, RN, AL, SE, MA, PI | R$ 39 | R$ 59 | R$ 99 |
| **N** | AM, PA, RO, RR, AP, AC, TO | R$ 49 | R$ 75 | R$ 139 |

Itens > 30kg sinalizados com "consultar frete" — vendedor cotiza manualmente (backlog Sprint 08+ para automatizar). Tarifas ajustáveis pelo admin sem deploy (tabela no DB, não em código).

### Phase 4 / Sprint 14 — Integração Melhor Envio

Substituir o calculador de tabela fixa por uma chamada de tarifa em tempo real para o **Melhor Envio** (agregador sobre Correios + Jadlog + Loggi + Azul Cargo + J&T). Uma API, múltiplas transportadoras, OAuth2, integração gratuita, taxa por envio embutida na tarifa cotada.

Tarifa ao vivo → checkout mostra opções de transportadora. Vendedor imprime etiqueta na página de detalhe do pedido. Eventos de rastreamento sincronizam via webhook.

Manter a tabela de frete fixo como **calculador de fallback** (idempotência no estilo ADR 005 não é necessária aqui — isso é somente leitura). Se a API do Melhor Envio estiver fora do ar ou não retornar cotações, o checkout cai de volta para a tabela fixa com um banner "prazo estimado — cotação em tempo real indisponível".

### Alternativas comparadas

| Opção | Custo de setup | Tempo de onboarding | SLA | API de rastreamento | Cobertura BR | Tier gratuito |
|-------|----------------|---------------------|-----|---------------------|--------------|---------------|
| **Melhor Envio** | Baixo (OAuth2, sem contrato) | ~1 dia | 99,5% | Sim, webhooks | Todas as UFs via 5+ transportadoras | Sim (taxa por envio) |
| Correios direto | Alto (contrato, certificação) | 2–6 semanas | 99% | Sim (SRO) | Todas as UFs | Não (preço contratual) |
| Frete Rápido | Médio | ~3 dias | 99,5% | Sim | Todas as UFs | Tier pago obrigatório acima de X volume |
| Jadlog direto | Médio | ~1 semana | 99% | Sim | Todas as UFs (via parceiros no N) | Não |
| Kangu | Baixo | ~2 dias | ~99% | Sim | Todas as UFs | Sim |

### Mix de transportadoras que esperamos via Melhor Envio

- **PAC / SEDEX** (Correios) — padrão para residencial, rural, pacotes pequenos
- **Jadlog .Package** — competitivo para SE/S até 30kg
- **Loggi** — entrega no mesmo dia/próximo dia em capitais
- **Azul Cargo** — mais rápido para NE/N via avião de carga, mais caro

## Consequências

### Positivas
- Lançar sem contrato com Correios — podemos enviar na Sprint 08 em vez de esperar semanas pelo onboarding
- Tabela fixa é explicável a vendedores e clientes; sem surpresas no checkout
- Melhor Envio nos dá diversidade de transportadoras gratuitamente — quando os Correios entram em greve (e entram), Jadlog/Loggi ainda funcionam
- Uma única superfície de API para cotação, etiquetagem e rastreamento reduz o trabalho de integração de serviço ao vendedor para um único adaptador

### Negativas
- Tabela fixa subavalora algumas rotas (frete pesado para o Amazonas) — absorvemos margem ou vendedores recusam pedidos. Mitigar com a válvula de escape "consultar frete" para 30kg.
- Melhor Envio é um intermediário — se eles caírem, a cotação no checkout também cai. O fallback para tabela fixa cobre isso, mas piora a experiência do cliente.
- A taxa por envio no Melhor Envio é pequena mas não é zero; contabilizada nos custos unitários.
- Eventos de rastreamento chegam via webhook; mais um pipeline de webhook para robustecer (reutilizar padrões do ADR 005).

### Alternativas rejeitadas
- **Correios direto**: API hostil, certificação demorada, preço contratual só faz sentido após ~500 envios/mês. Reavaliar na Phase 5 se o volume pelos Correios dominar nosso mix.
- **Frete Rápido**: produto similar ao Melhor Envio, mas te empurra para um tier pago mais cedo. O tier base gratuito para sempre do Melhor Envio vence na nossa escala.
- **Jadlog direto**: ótima transportadora para SE/S, mas lock-in em transportadora única; cobertura no Norte é dependente de parceiros e degrada o prazo.
- **Kangu**: considerado; rede de transportadoras menor que o Melhor Envio, menos adoção entre marketplaces BR.

## Questões em aberto

1. **Atualização da tabela de fallback** — quando o Melhor Envio está fora do ar, usamos a tabela de frete fixo atual ou tarifas ao vivo em cache das últimas 24h? Tarifas em cache são mais precisas, mas correm risco de desatualização; padrão: tabela fixa. Responsável: lead de checkout na Sprint 14.
2. **Seguro de frete** — o Melhor Envio oferece seguro de valor declarado opcional. Habilitar por vendedor, por pedido ou sempre ativo para pedidos > R$ 500? Responsável: ops.
3. **Entrada de dimensões/peso pelo vendedor** — precisamos de `weight_kg`, `length_cm`, `width_cm`, `height_cm` nos produtos para cotação ao vivo. Adicionar à tabela `products` como nullable na Sprint 08; tornar obrigatórios na Sprint 14 para vendedores BRL. Retroalimentar via ferramenta admin para listagens existentes.
4. **Envios divididos** — se um carrinho contém itens de múltiplos vendedores, cada vendedor envia separadamente. O checkout deve cotar cada trecho e somar. Direto, mas dobra as chamadas à API de cotação — observar os rate limits.
5. **Frete de devolução** — quem paga? Padrão: vendedor paga por itens com defeito, cliente paga por arrependimento (alinhado com o art. 49 do CDC para direito de desistência de 7 dias). Formalizar na Sprint 14.
6. **Pontos de retirada (agências)** — o Melhor Envio suporta PUDO para algumas transportadoras; útil em metrópoles. Phase 5+.
