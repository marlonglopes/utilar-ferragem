# ADR 002 — Estratégia de integração com product-gateway + serviço de pagamento

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

A Utilar Ferragem precisa (a) ler e escrever nos serviços existentes do product-gateway, e (b) introduzir capacidade de pagamento que ainda não existe. Precisamos decidir: quanto de backend novo, e onde ele vive.

## Decisão

### Catálogo / pedidos / autenticação
**Nenhum novo serviço de backend.** A Utilar Ferragem consome diretamente os endpoints existentes do product-gateway. Lógica específica de ferragens (taxonomia, filtros de comércio, mapeamento categoria → especificação) vive na SPA.

Mudanças de schema aditivas e retrocompatíveis nos serviços existentes conforme necessário:
- `products.specs` JSONB (Sprint 04)
- `products.category_path` opcional (adiar; usar mapeamento no cliente primeiro)
- `products.currency` (já planejado na Sprint 17 do projeto pai)
- `users.cpf` para clientes (estender o trabalho planejado na Sprint 18)

### Pagamentos
**Introduzir um novo `payment-service`** na Phase 3 / Sprint 08. Este é o único novo serviço de backend.

Responsabilidades do serviço:
- Abstração de cliente PSP (começar com UM PSP; manter a interface intercambiável)
- Geração de QR Pix + tratamento de webhook
- Emissão de boleto + tratamento de webhook
- Passagem de tokenização de cartão + fluxo 3DS
- Publicar `payment.confirmed` + `payment.failed` no Redpanda
- order-service consome eventos → transiciona o estado do pedido

### Critérios de seleção de PSP (a decidir no prep da Sprint 08)

| PSP | Pix | Boleto | Cartão | Observações |
|-----|-----|--------|--------|-------------|
| **Mercado Pago** | ✅ | ✅ | ✅ | Integração única; boa documentação BR; taxas razoáveis |
| **Gerencianet (Efí)** | ✅ | ✅ | ✅ | Especialista em Pix; bom preço para volume pesado em Pix |
| **Stripe BR** | ✅ | ✅ | ✅ | Melhor DX globalmente; taxas mais altas; nem todos os recursos no BR |
| **PagSeguro** | ✅ | ✅ | ✅ | Marca consolidada; APIs mais antigas |

Recomendação: **Mercado Pago** como escolha padrão pela abrangência + ubiquidade no BR. Decisão adiada para o kickoff da Sprint 08, quando conta real e cotações estiverem em mãos.

## Consequências

### Positivas
- Expansão mínima de backend — um novo serviço, somente quando genuinamente necessário
- Mantém os serviços existentes de produto/pedido/estoque focados
- Mudanças de schema são todas aditivas — sem migrações com downtime
- A abstração do payment-service permite trocar PSPs futuramente sem alterar a SPA

### Negativas
- `products.specs` JSONB limita a performance de filtro server-side além de ~50k produtos — mitigar com índice GIN e, se necessário, promover campos comuns a colunas reais mais tarde
- O acoplamento evento-driven de pagamento → pedido significa que uma queda de webhook atrasa as transições de estado do pedido — aceitável, mas monitorar
- Risco de lock-in de PSP mitigado pela abstração, mas nunca zero

### Alternativas rejeitadas
- **Incorporar lógica de pagamento no order-service**: mistura responsabilidades; piora a superfície de troca de PSP e conformidade
- **Fork de catalog-service para campos específicos de ferragens**: prematuro; 90% da lógica de catálogo é compartilhada com o marketplace geral
- **Integração direta de PSP a partir da SPA**: viola a postura PCI e acopla o frontend às versões de API do PSP

## Questões em aberto

1. Linguagem para o payment-service: Ruby/Rails (consistência com serviços existentes) vs. Go (menor overhead para workloads pesadas em webhook)? Padrão: **Rails** para consistência, a menos que encontremos um bloqueador.
2. Validação de assinatura de webhook — cada PSP é diferente; construir uma camada de adaptador limpa por PSP.
3. Idempotência: `psp_payment_id` como chave natural evita processamento duplo em retentativas de webhook.
