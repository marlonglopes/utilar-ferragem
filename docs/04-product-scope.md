# 04 — Escopo do Produto

## Mapa de funcionalidades por fase

| Fase | Funcionalidade | Sprint |
|------|---------------|--------|
| **1 — Fundação** | Scaffold Vite+React, linting, CI, configuração i18n | 01 |
| 1 | Design system, layout shells, roteamento, footer/nav | 02 |
| **2 — Catálogo** | Página inicial + trilho de categorias + produtos em destaque | 03 |
| 2 | Navegação por categoria + taxonomia + breadcrumb | 03 |
| 2 | Detalhe do produto: galeria, tabela de especificações, estoque, adicionar ao carrinho | 04 |
| 2 | Barra de busca + filtros facetados + ordenação | 05 |
| **3 — Comércio** | Carrinho (persistente, localStorage) | 06 |
| 3 | Auth do cliente + cadastro com CPF | 07 |
| 3 | Checkout: endereço (ViaCEP), frete, seletor de pagamento | 08 |
| 3 | Pagamento via Pix + boleto + cartão (via payment-service) | 08 |
| 3 | Histórico de pedidos + detalhe + status de rastreamento | 09 |
| **4 — Vertical de vendedores** | Onboarding de vendedor específico para ferragens (CNPJ, compatibilidade de categorias) | 10 |
| 4 | Importação em lote de SKUs (CSV) | 11 (futuro) |
| 4 | Construtor de kit — agrupamento de produtos como SKU "kit" | 11 (futuro) |
| **5 — Crescimento** | Avaliações e notas | 12 |
| 5 | Conta pro: preço por volume, cotações, faturamento com CNPJ | 13 |
| 5 | Programa de cashback (acúmulo por produto + resgate no checkout) | 26 |
| 5 | Polimento do PWA mobile | 18 |

## Fluxos principais

### Fluxo A — Navegar e comprar (cliente DIY)

1. Chegar na home (`/`) — hero, trilho de categorias, produtos em destaque.
2. Clicar na categoria → `/categoria/:slug` — grade filtrada, facetas na coluna esquerda.
3. Clicar no produto → `/produto/:id` — galeria + especificações + avaliações + adicionar ao carrinho.
4. Clicar no ícone do carrinho → drawer do carrinho ou `/carrinho`.
5. Checkout `/checkout` — endereço (autofill por CEP), frete, pagamento.
6. Confirmação `/pedido/:id` — linha do tempo de status.

### Fluxo B — Recompra (profissional)

1. Login → `/conta/pedidos`.
2. Pedido anterior → botão "Comprar novamente" adiciona todos os itens em estoque de volta ao carrinho.
3. Checkout usa endereço salvo + método de pagamento salvo.
4. Fluxo total: < 60 segundos do acesso à confirmação.

### Fluxo C — Compra pro (pequena empresa, Fase 5)

1. Login como conta pro (CNPJ verificado).
2. Adicionar produtos → carrinho exibe precificação por faixas.
3. "Solicitar cotação" para pedidos acima de R$ 5.000 → vendedor recebe solicitação de cotação.
4. Checkout com campo de referência de OC + alternância de faturamento com CNPJ.

### Fluxo D — Onboarding de vendedor (Fase 4)

1. Landing `/vender` → "Cadastrar minha ferragem".
2. Formulário de CNPJ (preenchimento automático da Receita Federal se viável; manual caso contrário).
3. Seleção de categorias (precisa escolher ≥ 1 folha de ferragens).
4. Condições de precificação + aceite da comissão.
5. Redirecionamento para o `gifthy-hub` para upload do catálogo de produtos (reutiliza o console do vendedor existente).

## Inventário de páginas (mínimo Fases 1 a 3)

### Públicas (sem auth)
| Rota | Componente | Notas |
|------|-----------|-------|
| `/` | `HomePage` | Hero, categorias, destaques, conteúdo editorial |
| `/categoria/:slug` | `CategoryPage` | Grade + filtros facetados + ordenação |
| `/produto/:id` | `ProductDetailPage` | Galeria, especificações, avaliações, relacionados |
| `/busca?q=` | `SearchPage` | Mesmo layout da categoria, orientado por query param |
| `/carrinho` | `CartPage` | Carrinho em página inteira (o drawer é a superfície principal) |
| `/vender` | `SellLandingPage` | Página de marketing para vendedores |

### Autenticação obrigatória
| Rota | Componente | Notas |
|------|-----------|-------|
| `/entrar` | `LoginPage` | Somente clientes; vendedores são redirecionados para o gifthy-hub |
| `/cadastro` | `RegisterPage` | CPF + e-mail + senha |
| `/checkout` | `CheckoutPage` | Multi-etapas (endereço → frete → pagamento) |
| `/pedido/:id` | `OrderConfirmationPage` | Sucesso pós-checkout |
| `/conta` | `AccountPage` | Perfil, endereços, métodos de pagamento |
| `/conta/pedidos` | `OrdersPage` | Histórico de pedidos |
| `/conta/pedidos/:id` | `OrderDetailPage` | Linha do tempo de status + itens |
| `/conta/cashback` | `AccountCashbackPage` | Saldo em destaque, pendente, próximo vencimento, histórico com filtros |

## Taxonomia de categorias (detalhada)

```
ferramentas/
├── manuais/          (chaves, alicates, martelos, serrotes, trenas)
├── eletricas/        (furadeiras, parafusadeiras, lixadeiras, esmerilhadeiras)
├── pneumaticas/      (chaves de impacto, pistolas, compressores)
└── medicao/          (trenas laser, níveis, paquímetros, multímetros)

construcao/
├── cimentos-argamassa/
├── telhas-coberturas/
├── blocos-tijolos/
└── ferragens-estruturais/

eletrica/
├── cabos-fios/
├── disjuntores-protecao/
├── tomadas-interruptores/
└── iluminacao/

hidraulica/
├── tubos/
├── conexoes/
├── registros-valvulas/
└── bombas-motobombas/

pintura/
├── tintas/
├── pinceis-rolos/
├── lixas-preparacao/
└── solventes/

jardim/
├── cortadores-roçadeiras/
├── irrigacao/
└── utensilios/

seguranca/ (EPI)
├── capacetes-oculos/
├── luvas/
├── calçados/
└── protecao-auditiva/

fixacao/
├── parafusos/
├── buchas/
├── pregos-arames/
└── colas-adesivos/
```

Cada folha mapeia para as strings `category` do product-service via dicionário client-side em `src/lib/taxonomy.ts`.

## Schema de filtros por categoria (exemplos)

| Caminho da categoria | Filtros |
|---------------------|---------|
| `ferramentas/eletricas` | tensão (127V / 220V / bivolt), potência (W ou HP), marca, preço |
| `eletrica/cabos-fios` | bitola (mm²), comprimento (m), condutor (Cu/Al), marca |
| `hidraulica/tubos` | diâmetro (mm), material (PVC/PPR/CPVC), classe de pressão, comprimento |
| `fixacao/parafusos` | padrão de rosca, comprimento, material, tipo de cabeça |
| `pintura/tintas` | acabamento (fosco/acetinado/brilho), interno/externo, família de cor, volume |

Todas as categorias também suportam: **faixa de preço**, **vendedor**, **apenas em estoque**, **avaliação** (≥ 4 estrelas).

## Requisitos não-funcionais

| RNF | Meta |
|-----|------|
| First Contentful Paint (Android intermediário, 4G) | < 2,0 s |
| Largest Contentful Paint | < 2,5 s |
| Cumulative Layout Shift | < 0,1 |
| Tamanho do bundle (inicial) | < 220 kB gzip |
| Offline (PWA básico) | Fase 5 |
| Acessibilidade | WCAG 2.1 AA |

## Fora do escopo (Fases 1 a 3)

- Chat ao vivo / mensagens com vendedor
- Tours em vídeo de produto
- Visualizações 3D / AR de produto
- Alertas de preço
- Compartilhamento de lista de desejos
- Logins sociais (pode adicionar na Fase 4 se houver demanda)
- Envio internacional
