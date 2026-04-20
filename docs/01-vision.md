# 01 — Visão

## Em uma frase

**Utilar Ferragem** é um marketplace de ferragens e ferramentas com foco no Brasil — uma vitrine especializada construída sobre a plataforma Gifthy product-gateway, voltada para clientes DIY, profissionais de obras e pequenas construtoras.

## Problema

O varejo de ferragens no Brasil é fragmentado: grandes redes (Leroy Merlin, Telhanorte, C&C) dominam os centros urbanos, mas com estoque online raso; as ferragens locais têm variedade profunda, mas sem e-commerce; e os marketplaces generalistas (Mercado Livre, Amazon.br) tratam ferramentas como mais uma categoria, com UX ruim para o setor (sem kits de SKU, sem preço por volume/pro, sem filtros técnicos como tensão, padrão de rosca ou HP).

## Oportunidade

O product-gateway do Gifthy já oferece:
- Infraestrutura multi-tenant para vendedores (10 vendedores seedados; suporta muitos mais)
- CRUD de produtos + paginação server-side
- Ciclo de vida de estoque e pedidos
- Streaming de eventos via Kafka para reservas
- Camada de autenticação JWT com controle de acesso por papel (role-based)

Uma vitrine vertical construída sobre esse backend permite especializar a UX sem reescrever a infraestrutura. A Utilar Ferragem se torna o **ponto de entrada voltado ao consumidor** para o vertical de ferragens; o `gifthy-hub/` continua sendo o **console de operações do vendedor/admin**.

## Usuários-alvo

| Persona | Descrição | Necessidade principal |
|---------|-----------|----------------------|
| **Cliente DIY (Maria)** | Proprietária fazendo um projeto de fim de semana | Informações claras do produto, busca rápida, avaliações confiáveis, meios de pagamento BR |
| **Profissional (João)** | Eletricista / encanador / marceneiro | Filtros técnicos (tensão, bitola, padrão), disponibilidade por volume, recompra rápida |
| **Pequena construtora (Construtora XYZ)** | Empreiteira de 5 a 20 pessoas | Conta pro com preço negociado, pedidos tipo OC, faturamento com CNPJ |
| **Vendedor de ferragens (Ferragem Silva)** | Ferragem local querendo alcance online | Cadastro sem burocracia, UX em português, sincronização simples de estoque |

## Posicionamento frente às alternativas

| Alternativa | Como nos diferenciamos |
|-------------|------------------------|
| Home Depot (sem presença no BR) | Feito para o Brasil: CNPJ/CPF, Pix, boleto, ViaCEP, BRL, pt-BR como padrão |
| Leroy Merlin / Telhanorte (grandes redes) | Modelo marketplace — estoque de cauda longa com múltiplos vendedores |
| Mercado Livre (marketplace generalista) | UX vertical: filtros específicos para ferramentas, kits, contas pro, vocabulário técnico |
| Sites de ferragens independentes | Catálogo unificado, checkout compartilhado, avaliações compartilhadas, credibilidade compartilhada |

## Métricas de sucesso (metas para o lançamento na Fase 3)

- **Catálogo**: ≥ 5.000 SKUs em ≥ 10 vendedores de ferragens
- **Conversão**: ≥ 1,5% de visitante para pedido nas páginas de categoria
- **Confiança**: média de avaliações ≥ 4,2/5, taxa de disputas < 2%
- **Retenção**: ≥ 25% dos clientes logados recompram em até 90 dias
- **Contas pro**: ≥ 50 contas com CNPJ verificado até o fim da Fase 4

## O que não é escopo (explícito)

- **Logística própria / armazenagem** — os vendedores enviam seu próprio estoque nas Fases 1 a 3. Fulfillment pela Utilar é consideração para a Fase 5 em diante.
- **Expansão global** — somente Brasil até o modelo BR estar consolidado.
- **Categorias fora de ferragens** — sem cosméticos, sem moda. Foco total no vertical.
- **Substituir o gifthy-hub** — a Utilar Ferragem é o vertical voltado ao cliente; o gifthy-hub continua sendo o console do vendedor.

## Premissas sinalizadas para confirmação

1. **Localização**: este app fica como uma nova SPA em `utilar-ferragem/`, irmã ao `gifthy-hub/`, usando o mesmo stack (React + Vite + TS + Tailwind + Zustand + TanStack Query + i18next).
2. **Backend**: sem backend novo. Consome os serviços existentes do product-gateway pelo Go gateway na porta 8080. A lógica de domínio específica de ferragens (taxonomia de categorias, filtros técnicos, precificação pro) é adicionada sobre o schema de produtos existente.
3. **Branding**: industrial/prático, inspirado no feed do Instagram referenciado. Paleta + logo a definir com os assets reais do usuário.
4. **Go-to-market**: começa com os 10 vendedores já seedados (reclassificados/filtrados para o segmento de ferragens) mais um esforço inicial de onboarding. Sem aquisição paga nas Fases 1 e 2.

As quatro premissas são ajustáveis — sinalizar qualquer divergência antes de confirmarmos o plano.
