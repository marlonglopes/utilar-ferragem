# Backoffice completo do Utilar — gaps + roadmap

Objetivo do dono: **Utilar operável de ponta a ponta** — gerir produto, estoque, preço,
imagem, meios de pagamento, contabilidade e despacho; com **acesso por papel** (dono,
contador, vendas, almoxarifado — cada um sua visão); e **tudo auditável** (CloudTrail:
quem fez o quê e quando).

Este doc cruza **três camadas**:
1. **Funcionalidade** — o que dá pra gerenciar.
2. **Papel/visão** — quem acessa o quê.
3. **Auditoria** — CloudTrail em cima de tudo.

> Legenda: ✅ pronto e real · 🟡 backend existe, **sem tela** · 🔴 não existe (nem tela nem API)

---

## Camada 1 — Funcionalidade

### O que JÁ está pronto (real, ligado)
| Área | Estado |
|---|---|
| **Produto** — criar/editar/arquivar, todos os campos (nome, SKU, EAN, marca, categoria, status, preço, custo, estoque, unidade, dimensões, **NCM/CFOP/CEST/origem**, specs) | ✅ |
| **Imagens** — upload, reordenar, definir capa, excluir | ✅ |
| **Importação** — CSV + pipeline com staging/dry-run/commit + SINAPI | ✅ |
| **Contábil** — livro (partidas dobradas), reconciliação vs PSP, fechamento, export CSV/balancete/OFX | ✅ |
| **Trilha de auditoria** (contábil) + **Observabilidade** (métricas dos serviços) | ✅ |
| **Custo/margem** — edição de custo, leitura de margem, custo no balcão (PDV) | ✅ |

Essa é a parte **mais madura**: produto+imagem+import e contábil/auditoria/observabilidade.

### 🟡 Backend existe, mas NÃO tem tela (endpoints órfãos)
Isto é o mais frustrante: o motor está pronto, falta o painel.
- **Fulfillment de pedido** — `PATCH /admin/orders/:id/{picking,shipped,delivered,cancel}` (order-service). **Sem página.**
- **Devoluções/estorno** — fila `/admin/returns` + approve/reject/receive/refund. **Sem página.**
- **Operadores/lojas** — `/admin/operators`, `/admin/stores` (criar staff, teto de desconto, papel). **Sem página nem client.**
- **Moderação de avaliações** — `/admin/reviews` approve/reject. **Sem página.**
- **Preço por quantidade (price tiers)** e **histórico de preço** — endpoints existem, **nenhuma tela chama**.
- **Atributos de produto** — `/admin/products/.../attributes`. **Sem tela.**

### 🔴 Não existe (nem tela nem endpoint) — buraco de verdade
- **Lista de pedidos no admin** — os PATCH de fulfillment existem, mas **não há `GET /admin/orders`** que liste os pedidos pra agir neles. Sem isto, a loja **não processa pedido online pelo painel**.
- **Gestão de categorias** (CRUD) — hoje categoria só vem por seed/migration; só há `List` público.
- **Gestão de frete** (`shipping_rates`) — só há `Quote` público; nada edita a tabela.
- **Cadastro de fornecedor** (registry CRUD) — só `List` + relatório de performance.
- **Relatório de estoque baixo / movimento de estoque** — só dá pra sobrescrever o número absoluto; sem histórico, sem alerta.
- **Configurações da loja** (settings) — inexistente.
- **Promoções/cupons** — inexistente (o único desconto é o do balcão, negociado, com teto).
- **Bulk atômico** — publicar/editar em lote hoje é N PATCH sequenciais (não-atômico: "publicou 8 de 10").

### Meios de pagamento
- **PSP** (Appmax v1/v3, Stripe, MP) é escolhido por **env** (`PSP_PROVIDER`), fail-closed. Pix/cartão/boleto suportados no código.
- 🔴 **Sem tela de configuração de pagamento** no admin (trocar PSP, ver credenciais, habilitar método) — é tudo variável de ambiente hoje. Pra "gerenciar meios de pagamento" pelo painel, falta essa camada.

---

## Camada 2 — Acesso por papel (o que falta pra cada persona)

Hoje existem os papéis `admin · store_operator · seller · customer`. O que você quer são
**personas de operação** com visão própria. Mapa persona → o que precisa:

| Persona | Vê / faz | Estado hoje |
|---|---|---|
| **Dono / Admin** | tudo | ✅ (mas faltam as telas da Camada 1) |
| **Contador** | contábil, reconciliação, **fiscal/NF-e**, faturamento (só leitura do resto) | contábil ✅; **falta papel próprio + NF-e + esconder custo/margem estratégica** |
| **Vendas** | produtos, **pedidos (lista+detalhe)**, balcão/PDV, catálogo | balcão ✅; **falta a tela de pedidos e um papel "vendas" separado de admin** |
| **Almoxarifado** | **estoque (ajuste/entrada)**, **separação→despacho**, devolução física | **quase tudo falta tela**; papel próprio inexistente |

**O que falta na camada de papel:**
- **Papéis novos**: `contador`, `vendas` (≠ marketplace `seller`), `almoxarife` — além dos atuais.
- **Permissão por recurso** (não por rota): já é o padrão do projeto no balcão; estender pro admin.
- **Menu/rotas filtrados por papel**: cada persona vê só o seu (o `AdminShell` hoje mostra os 7 itens pra todo admin).
- **Fail-closed**: papel sem permissão → 403 no servidor, não só menu escondido.

---

## Camada 3 — Auditoria total (CloudTrail)

**A fundação já é CloudTrail** — `pkg/audit`: append-only, **hash-encadeado** (imutável),
com quem (ator+papel+IP mascarado), o quê (entidade+ação+antes→depois), quando, request-id.
É mais robusto que a média de mercado.

**O buraco é cobertura:** hoje só o **payment** grava no `pkg/audit`. Falta ligar em:
- **catalog** — mudança de produto/preço/estoque/categoria, publicação, import.
- **order** — status de pedido, separação/despacho, devolução (já tem trilha própria, unificar).
- **auth** — criação/edição de operador, mudança de papel, teto de desconto, login de admin.

E falta a **tela de auditoria unificada**: "quem fez o quê, quando", filtrável por pessoa/
ação/entidade/período — pro contador e pro dono investigarem qualquer coisa. Cada ação de
cada persona (vendas/almoxarifado/contador) vira uma linha imutável.

---

## Roadmap priorizado (cruzando as 3 camadas)

Ordem por impacto pra "loja operável + auditável + por papel":

| # | Entrega | Camadas | Por quê primeiro |
|---|---|---|---|
| **1** | **Pedidos no admin**: `GET /admin/orders` (lista/filtro) + página + ações de fulfillment (separar/despachar/entregar/cancelar) | Func + Papel(vendas/almox) + Audit | Sem isso a loja **não processa pedido online**. Desbloqueia vendas E almoxarifado. |
| **2** | **Auditoria unificada**: ligar `pkg/audit` em catalog/order/auth + **tela CloudTrail** filtrável | Audit (todas) | Seu requisito central; cada ação das telas novas já nasce auditada. |
| **3** | **Papéis + visões**: criar `contador/vendas/almoxarife`, permissão por recurso, menu filtrado, 403 fail-closed | Papel (todas) | Amarra quem acessa o quê antes de abrir mais telas. |
| **4** | **Devoluções/estorno** (tela) + **operadores/staff** (tela) — backends prontos | Func + Papel + Audit | Endpoints órfãos virando tela; almoxarifado e onboarding de staff. |
| **5** | **Estoque**: ajuste com motivo + **movimento (histórico)** + **relatório de estoque baixo** | Func + Papel(almox) + Audit | Núcleo do almoxarifado; hoje só sobrescreve número. |
| **6** | **Categorias (CRUD)** + **frete (CRUD)** + **configurações da loja** | Func + Papel(admin) + Audit | Configuração da loja que hoje é só seed. |
| **7** | **Preço por quantidade + histórico de preço** (tela) — backend pronto | Func + Papel(vendas) + Audit | Preço por volume é core de ferragem. |
| **8** | **Meios de pagamento** (tela de config PSP) + **moderação de avaliações** (tela) | Func | Tirar do env; endpoint de review pronto. |
| **9** | **NF-e / fiscal** (contador) — ver `docs/fiscal-para-o-dono.md` | Func + Papel(contador) | Depende de decisões do dono (emissor, contador, A1). |
| **10** | **Bulk atômico** + **promoções/cupons** (se o negócio quiser) | Func | Melhoria de escala / greenfield. |

**Resumo de uma linha:** o **motor** (produto, imagem, import, contábil, auditoria, PSP)
está forte; o que falta é **as telas de operação** (principalmente **pedidos/fulfillment**),
a **camada de papéis com visão por persona**, e **estender a auditoria** do payment pra todo
o resto — mais os cadastros de configuração da loja (categoria/frete/settings) que hoje só
existem por seed.

---

## Camada 4 — Venda fluida (o que um bom e-commerce tem pra vender rápido)

Além de "operável", a loja tem que **vender fácil**. O que já existe e o que agregar:

### Loja (cliente) — conversão
| Feature | Estado | Nota |
|---|---|---|
| Busca com acento/radical (tsvector) | ✅ | rápida; falta **autocomplete/sugestão ao digitar** 🟡 |
| Filtros/facets (categoria, marca, preço, atributo) | ✅ | já tem facets no catalog |
| Carrinho persistente + mini-cart | ✅ | |
| **Frete no carrinho** (não só no checkout) | ✅ | reduz abandono (ShippingEstimate) |
| **Checkout curto / convidado** (sem obrigar cadastro) | 🟡 | avaliar guest checkout de 1 passo |
| **Pix com QR instantâneo** + cartão parcelado + boleto | ✅ (PSP) | falta expor parcelamento com clareza no card |
| Recomendação (relacionados/co-compra) + capa | ✅ | recém-corrigido |
| Favoritos / wishlist | ✅ | |
| Avaliações (reviews) | ✅ | falta **tela de moderação** 🟡 |
| **Cupom/promoção no checkout** | 🔴 | não existe — lever de conversão |
| **Recompra rápida** (do histórico) / listas de compra | 🟡 | poderoso pra ferragem (compra recorrente de obra) |
| **Alice** (assistente que calcula material e acha produto) | ✅ | diferencial forte |
| Selo de estoque / "últimas unidades" / prazo de entrega | 🟡 | urgência + confiança |
| **PWA / mobile fluido** | 🟡 | app-like no celular, offline básico |

### Balcão / PDV — venda física rápida
| Feature | Estado |
|---|---|
| Busca por código/EAN/nome, adicionar item rápido | ✅ |
| Desconto com teto do cargo + fila do gerente | ✅ |
| **Leitor de código de barras** (câmera/USB) | 🟡 vale muito no balcão |
| Fechar venda rápido (Pix/maquininha/liquidação externa) | ✅ |
| **Impressão de cupom/DANFE NFC-e** | 🔴 depende do fiscal |

### Operação — despacho/estoque rápido
- **Separação com leitura de código** (bipar item ao separar) 🟡 — acelera almoxarifado.
- **Estoque em tempo real + alerta de baixo** 🟡 — evita vender o que não tem.
- **Entrada de mercadoria rápida** (dar entrada por nota/EAN) 🔴.

**Prioridade de conversão** (encaixa nas fases): **cupom/promoção** (Fase 10), **autocomplete
de busca** e **recompra rápida** entram como incrementos de storefront; **leitor de código de
barras** no balcão e na separação entra junto com a Fase 5 (estoque/almoxarifado).
