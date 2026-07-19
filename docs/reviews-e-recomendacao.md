# Avaliações de produto e recomendação — contrato e decisões

**Serviço:** `catalog-service`
**Migrations:** `015_product_reviews`, `016_recommendations`
**Data:** 2026-07-19

---

## O que existia antes

| | Antes | Agora |
|---|---|---|
| Estrelas do produto | `products.rating` / `review_count` inventados pelo seed, **sem nenhuma linha de avaliação por trás** | Agregado calculado por gatilho a partir de `product_reviews` |
| Aba de avaliações | "disponível em breve" | `GET /products/:slug/reviews` |
| `sort=top_rated` | ordenava por um número fictício, e por média pura | ordena por média **bayesiana** |
| "Produtos relacionados" | `mesma categoria ORDER BY rating DESC LIMIT 4` → **os mesmos 4 itens para todo produto da categoria** | co-compra agregada → regra técnica → fallback marcado |

---

# 1. Avaliações

## 1.1 Compra verificada — o contrato com o order-service

**Só quem comprou avalia.** Como o pedido vive em outro serviço, com banco
próprio e sem acesso cruzado, a verificação usa **duas provas independentes,
exigidas juntas**.

### Prova 1 — "purchase grant" (JWT emitido pelo order-service)

O frontend, ao abrir o formulário de avaliação, pede ao **order-service** um
comprovante. O order-service verifica no banco DELE que o usuário autenticado
comprou aquele produto e devolve um JWT:

```
HS256, assinado com SERVICE_JWT_SECRET
{
  "iss": "utilar-order",              // obrigatório e verificado
  "aud": "utilar-catalog-reviews",    // obrigatório e verificado
  "sub": "<user_id do comprador>",    // obrigatório
  "pid": "<product_id (uuid)>",       // obrigatório
  "oid": "<order_id>",                // obrigatório
  "nm":  "<nome do titular do pedido>", // opcional, mas recomendado
  "iat": <unix>,                      // obrigatório
  "exp": <unix>                       // obrigatório, exp - iat <= 15 min
}
```

Regras verificadas pelo catalog (`internal/review/grant.go`):

- algoritmo travado em **HS256** (`alg: none` e troca de algoritmo recusados);
- assinatura conferida com o **SERVICE_JWT_SECRET**, nunca com o `JWT_SECRET` de
  usuário — comprovante de compra é afirmação de **serviço** (auditoria A1);
- `iss` **precisa** ser `utilar-order`. Um token de serviço genérico
  (`iss: utilar-internal`, o de `pkg/servicetoken`) **não serve** como
  comprovante: são escopos diferentes;
- `aud` precisa ser `utilar-catalog-reviews`;
- `exp` obrigatório, e `exp - iat` no máximo **15 minutos**;
- `sub` precisa bater com o usuário autenticado no `Authorization` do POST;
- `pid` precisa bater com o produto da rota.

> **Referência executável do formato:** `review.IssueGrantForTest` em
> `internal/review/grant.go` produz exatamente este token. Quem implementar o
> lado do order-service pode espelhá-la.

### Prova 2 — reserva confirmada local

O catalog exige, **além** do comprovante, que exista no banco dele uma linha em
`stock_reservations` com `status='committed'` para `(oid, pid)`. Esse dado já
está lá porque foi o próprio catálogo que baixou o estoque
(`POST /internal/reservations/:orderId/commit`).

### Por que as duas

Cada serviço conhece **metade** do fato:

- o **order-service** sabe de quem é o pedido, mas não escreve no catálogo;
- o **catalog** sabe que o pedido levou aquele produto, mas não sabe de quem
  ele é.

Sozinha, a prova 1 seria "confio inteiramente no order-service": um bug de
autorização lá vira avaliação falsa aqui, sem detecção. Sozinha, a prova 2 não
amarra o pedido a ninguém — quem descobrisse um `order_id` avaliaria em nome
dele. Exigir as duas fecha a lacuna **sem criar acesso cruzado a banco**.

**Não foi usada chamada HTTP síncrona** ao order-service: colocaria outro
serviço no caminho crítico de uma escrita do cliente, exigiria timeout/retry, e
não seria mais seguro — a resposta seria confiada do mesmo jeito que o token
assinado, que ainda por cima é verificável offline e expira.

**O que isto não resolve:** quem comprometer o order-service consegue emitir
comprovantes. É a mesma limitação que `pkg/servicetoken` documenta, e só
assinatura assimétrica elimina. A prova 2 limita o estrago a pedidos que
realmente confirmaram aquele produto.

## 1.2 Moderação — publica, com duas barreiras antes

**Decisão: a avaliação entra `published`**, exceto quando a triagem automática a
segura em `pending`.

Por que não pré-moderação total: a Utilar **não tem equipe de moderação**. O que
acontece de fato numa loja pequena que exige aprovação manual é a fila crescer,
ninguém revisar, e o produto ficar semanas com "seja o primeiro a avaliar"
enquanto há dez avaliações paradas. Moderação que não é executada equivale a
censura arbitrária, com o custo de ter sido implementada.

Por que não publicação livre: texto público sem barreira vira spam de SEO, de
contato ("chama no zap") e ataque a concorrente.

As duas barreiras que substituem a fila:

1. **Compra verificada.** É qualitativamente diferente de um captcha: para
   postar spam é preciso **comprar o produto, com dinheiro**, e cada compra
   rende **uma** avaliação (índice único por pessoa/produto). Spam em massa
   deixa de fechar a conta.
2. **Triagem automática** (`internal/review/moderation.go`) — só vai para
   `pending` o que casa com: link/domínio, e-mail, telefone (≥10 dígitos),
   convite a contato fora da plataforma, parágrafo em caixa alta (>70% das
   letras, com ≥20 letras) ou repetição de caractere (≥6 iguais).
   Avaliação **só com estrela nunca entra na fila** — não há o que moderar num
   número.

A triagem é **conservadora de propósito**: cada falso positivo é um cliente
honesto que não vê a própria avaliação, e avaliação técnica de ferragem é cheia
de número e sigla ("cabo 2,5mm² 750V", "argamassa AC-III", "GSB 13 RE 650W") —
está coberta por teste explícito para **não** cair na fila.

**Editar reavalia a triagem**: publicar limpo e editar inserindo o link seria o
contorno óbvio.

**Consequência aceita:** ofensa escrita em português normal, sem link nem
telefone, entra publicada. O canal de **denúncia** ficou de fora deste corte.

## 1.3 Agregado consistente — gatilho, não recálculo em lote

`products.rating`, `review_count` e `rating_bayes` são mantidos por gatilho
`AFTER INSERT OR UPDATE OR DELETE` em `product_reviews`.

Por quê, decidido pelo **consumidor** do agregado:

- ele **alimenta a ordenação** da vitrine (`sort=top_rated`) e a nota do card.
  Um job de 5 em 5 minutos é uma janela de até 5 minutos em que "mais bem
  avaliado" mente e o cliente não vê a própria nota. Defasagem em agregado que
  ordena não é atraso, é **resultado errado**;
- o custo do gatilho é irrelevante **aqui**: escrita de avaliação é rara (uma
  por pessoa por produto, só de quem comprou) e leitura de produto é o caminho
  mais quente da loja. Fosse um contador de visualizações, a resposta seria a
  oposta;
- gatilho não pode ser esquecido por um caminho de escrita novo — e o modo de
  falha de "esqueci de recalcular" é silencioso.

Só linhas `published` entram na conta.

## 1.4 Relevância — média bayesiana

```
score = (v·R + m·C) / (v + m)      m = 5 (PRIOR_COUNT), C = 4.0 (PRIOR_MEAN)
```

Média pura ordena, na prática, por *quem teve menos avaliações*: um 5★ único
passa na frente de um 4,8★ com 400. Com o prior, o 5★ único pontua
`(1·5 + 5·4)/6 = 4,17`, abaixo do 4,8★ com 400 (`4,79`). Item novo não fica
punido para sempre — ~10 avaliações boas já o levam ao topo.

`C` é **constante** e não a média global da loja: a média global muda a cada
avaliação e obrigaria a reescrever o score de **todos** os produtos a cada
review. Mudar `C` é uma migration — o que é honesto, porque é decisão de
produto.

`rating_bayes` é `NUMERIC(4,3)`: com uma casa decimal metade do catálogo
empataria e a "ordenação por relevância" viraria ordem física no disco.

---

# 2. Recomendação

Cascata de três fontes, nesta ordem, preenchendo até o `limit`:

### 2.1 Co-compra agregada

Fonte: `stock_reservations` com `status='committed'`, que **já vive no banco do
catálogo** — sem acesso ao banco de pedidos, sem chamada de rede.

**LGPD.** `product_copurchase` guarda **apenas** `(product_id,
related_product_id, order_count)`. Não há `order_id` nem usuário: é impossível
reconstruir a cesta de alguém a partir dela, e a lista exibida **não depende de
quem está olhando** — dois visitantes na mesma página veem o mesmo. Perfil de
compra individual seria tratamento de dado pessoal exigindo base legal, aviso e
controle de oposição que a loja não tem; o agregado entrega quase todo o valor
fora desse território.

**Mínimo de ocorrências: 5 pedidos distintos** (`handler.MinCopurchaseOrders`).
Com 1 ou 2, duas pessoas que por acaso levaram cimento e uma tomada juntos
produzem uma "recomendação" — e nas primeiras semanas da loja *toda* a lista
seria feita desse tipo de coincidência.

**Cestas com mais de 25 produtos distintos são ignoradas**
(`reco.MaxBasketSize`). Um pedido de 60 itens é reposição de construtora, não
escolha de consumo; como esses pedidos concentram volume, eles dominariam a
recomendação inteira.

**Job incremental** (`internal/reco/copurchase.go`), a cada 10 min, processando a
janela `(watermark, now() - 1min]`. O atraso de 1 min é **correção, não
paranoia**: `updated_at` é gravado quando a transação *executa*, não quando
comita, e sem a folga um pedido comitado logo após a leitura seria pulado para
sempre.

### 2.2 Complementar por regra técnica

`product_complement_rules`: `(categoria de origem + termo) → (categoria de
destino + termo)`, com `note` obrigatória e exibível.

Funciona no **dia 1**, sem histórico nenhum, e é onde mora o conhecimento do
balcão: quem leva porcelanato precisa de argamassa AC-III, rejunte e espaçador.
17 regras vêm na migration (piso, hidráulica, elétrica, fixação, ferramentas,
pintura).

Os termos são casados contra `search_vector` (migration 014) com
`websearch_to_tsquery`, que aceita `OR` e **nunca levanta erro de sintaxe** —
uma regra mal escrita deixa de casar em vez de derrubar a página com 500.

**Máximo 2 produtos por regra**: a ideia é *cobrir* as necessidades (argamassa E
rejunte E espaçador), não empilhar oito argamassas.

### 2.3 Fallback por categoria — **marcado**

Quando não há dado suficiente, devolve outros produtos da categoria, e **cada
item vem com `reason.kind = "category_fallback"`** e `meta.fallback = true`.
O rótulo exibível é "Outros produtos desta categoria" — não diz "recomendado".

A ordenação é `rating_bayes DESC, md5(id || <id do produto de origem>)`. É o que
conserta o defeito original: ordenar só por qualidade devolve os mesmos N itens
para **todos** os produtos da categoria. Misturar o id da origem no desempate dá
um recorte diferente por página, de forma **determinística** (cache e paginação
continuam coerentes) e sem `random()`, que quebraria as duas coisas.

---

# 3. Custo medido da co-compra

Método de `docs/performance-banco.md`: banco `perf_lab_reco` criado do schema
real (`pg_dump --schema-only`), populado com **20.001 produtos**, **200.000
pedidos confirmados** e **610.000 linhas de reserva** (2 anos de datas,
espalhadas por aritmética — a armadilha do `random()` do doc original foi
evitada e as datas conferidas: `2024-07-20` a `2026-07-19`).

### 3.1 O que acontece sem tabela materializada

Self-join em `stock_reservations` na hora da leitura:

| Produto | Plano | Tempo | Buffers |
|---|---|---|---|
| Mediano (30 pedidos) | Nested Loop por `idx_stock_reservations_product` | **0,67 ms** | 243 |
| **Campeão de vendas** (10.000 pedidos) | **Parallel Seq Scan** em `stock_reservations` + Hash Join | **57–102 ms** | **30.167** |

O ponto não é a média — é a **forma da curva**. O custo é proporcional a *quão
popular o produto é*, e popularidade é exatamente o que gera visualização de
página. O produto mais visto da loja é o mais caro de renderizar, e piora a cada
pedido fechado. É o mesmo modo de falha do item 1 de `performance-banco.md`
(custo proporcional ao catálogo inteiro para entregar 24 linhas).

### 3.2 Com a tabela agregada

| Consulta | Tempo | Buffers |
|---|---|---|
| Leitura no caso real (limiar 5, campeão de vendas) | **0,09 ms** | 15 |
| Leitura no pior caso do índice (16.666 pares, limiar 1) | **1,85 ms** | 336 |
| Reconstrução COMPLETA (200.000 pedidos → 153.300 pares) | **15,7 s** | — (fora do caminho da requisição) |

### 3.3 Uma correção que a medição obrigou

A primeira versão da leitura era um `JOIN` único com
`ORDER BY cp.order_count DESC, p.rating_bayes DESC`. Como `rating_bayes` é
coluna de `products`, o planejador precisa ler a linha de produto de **todos** os
pares antes de aplicar o `LIMIT`:

| | Tempo | Buffers |
|---|---|---|
| JOIN direto (16.666 pares) | 10,9 ms | 10.195 |
| Corte no índice primeiro (CTE `candidatos`) | **1,85 ms** | **336** |

**5,9× mais rápido, 30× menos buffers.** O `LIMIT` interno usa só colunas de
`product_copurchase`, então o corte cabe dentro de `idx_copurchase_lookup` e o
JOIN passa a tocar no máximo `limit*4` linhas — o custo deixa de depender de
quantos pares o produto acumulou.

---

# 4. Contrato das APIs

Envelope de erro: o de sempre (`{error, code, requestId}`).

## 4.1 Público

### `GET /api/v1/products/:slug/reviews`

Query: `page` (1), `per_page` (10, máx 50), `sort` = `recent` (default) |
`relevance` | `rating_desc` | `rating_asc`.

Devolve **só avaliações publicadas**.

```json
{
  "data": [
    {
      "id": "uuid",
      "rating": 5,
      "title": "Ótima furadeira",
      "body": "Pegou bem no concreto.",
      "authorName": "Marlon L.",
      "verifiedPurchase": true,
      "createdAt": "2026-07-19T10:00:00Z",
      "updatedAt": "2026-07-19T10:00:00Z"
    }
  ],
  "meta": { "page": 1, "perPage": 10, "total": 37, "sort": "recent" },
  "summary": {
    "average": 4.6,
    "count": 37,
    "score": 4.529,
    "distribution": { "1": 1, "2": 0, "3": 2, "4": 8, "5": 26 }
  }
}
```

- `authorName` é **sempre** "Primeiro N." — minimização de dado pessoal, não
  formatação. Nome completo associado a consumo numa página indexável seria
  exposição desnecessária.
- `verifiedPurchase` é sempre `true` (não existe caminho de escrita sem compra).
- `summary.average` é a média simples — **é essa que vai nas estrelas**.
- `summary.score` é a bayesiana — é a que **ordena** a vitrine. Serve para a UI
  explicar por que um 4,6★ com 200 aparece acima de um 5,0★ com uma.
- `authorUserId` e `orderId` **não existem** no payload público.

### `GET /api/v1/products/:slug/related?limit=8`

`limit` 1–24, default 8.

```json
{
  "data": [
    {
      "...": "todos os campos de Product, no mesmo nível de antes",
      "reason": {
        "kind": "copurchase",
        "label": "Quem levou este produto também levou",
        "orders": 12
      }
    },
    {
      "...": "...",
      "reason": {
        "kind": "complement",
        "label": "Costuma ser necessário junto",
        "note": "Porcelanato e piso cerâmico exigem argamassa colante — AC-III para porcelanato."
      }
    },
    {
      "...": "...",
      "reason": { "kind": "category_fallback", "label": "Outros produtos desta categoria" }
    }
  ],
  "meta": {
    "strategy": "mixed",
    "fallback": true,
    "minCopurchaseOrders": 5,
    "counts": { "copurchase": 2, "complement": 3, "fallback": 3 }
  }
}
```

**Compatibilidade:** `Product` é embutido, então todos os campos de antes
continuam no mesmo nível. Quem já consumia `data[]` não quebra.

**`reason.kind`** é valor estável de contrato: `copurchase` | `complement` |
`category_fallback`.

**`meta.fallback`** é `true` se **qualquer** item veio do preenchimento por
categoria. Com ele `true`, a seção **não deve** ser intitulada "recomendado para
você" nem "quem comprou também levou" — use um título neutro ou separe visualmente
os itens por `reason.kind`.

**`meta.strategy`**: `copurchase` | `complement` | `mixed` | `category_fallback`.

Novo em `Product`: **`ratingScore`** (a média bayesiana). `rating` continua
sendo a média simples.

## 4.2 Cliente autenticado (`Authorization: Bearer <JWT de usuário>`)

### `POST /api/v1/products/by-id/:id/reviews`

```json
{ "rating": 5, "title": "Ótima", "body": "...", "purchaseGrant": "<JWT>" }
```

`rating` obrigatório (1–5), `title` ≤120, `body` ≤2000, `purchaseGrant`
obrigatório. **Não existe campo de nome do autor** — autoria vem da claim `nm`
do comprovante, senão qualquer comprador assinaria com o nome que quisesse.

| Status | Quando |
|---|---|
| `201` | criada. Devolve `status` (`published` ou `pending`) e `moderationNote` |
| `400` | payload inválido / sem `purchaseGrant` / texto acima do limite |
| `403` | comprovante inválido, expirado, de outro usuário/produto, ou **sem reserva confirmada** |
| `404` | produto inexistente |
| `409` | já avaliou este produto — **edite** em vez de criar |

⚠️ O frontend precisa tratar `status: "pending"` no 201: mostrar "sua avaliação
está em análise". Sem isso o cliente não vê o texto no ar e conclui que sumiu.

### `GET|PUT|DELETE /api/v1/products/by-id/:id/reviews/mine`

`GET` devolve a própria avaliação **com `status` e `moderationNote`** — é o que
distingue "está em moderação" de "sumiu". `PUT` reavalia a triagem.
`DELETE` → `204`, e o agregado volta sozinho (gatilho).

## 4.3 Admin (`role=admin`)

| Rota | Efeito |
|---|---|
| `GET /api/v1/admin/reviews?status=pending&limit=50` | fila de moderação (default `pending`) |
| `POST /api/v1/admin/reviews/:id/approve` | publica; agregado recalcula na mesma transação |
| `POST /api/v1/admin/reviews/:id/reject` | remove do ar |
| `POST /api/v1/admin/recommendations/copurchase/rebuild` | reconstrói `product_copurchase` do zero |

Aprovar/recusar grava trilha em `catalog_audit_log`.

O rebuild é manual porque o job incremental **não consegue se corrigir sozinho**:
se uma rodada somar errado, recontar do zero é a única saída.

---

# 5. O que ficou de fora

- **Denúncia de avaliação** pelo público. É o buraco conhecido da política de
  publicação imediata (ofensa em português normal passa). Próximo item natural.
- **Votos de "foi útil"**. `sort=relevance` hoje usa tamanho do texto e recência.
- **Resposta do lojista** a uma avaliação.
- **Fotos na avaliação** — exigiria reuso do pipeline de `internal/imaging` e
  moderação de imagem, que a triagem de texto não cobre.
- **Pedidos de balcão/PDV** que baixam estoque sem passar por
  `stock_reservations` **não geram avaliação nem co-compra**. Só a venda online
  alimenta as duas coisas hoje.
- **CRUD admin das regras de complemento.** Elas são criadas por migration;
  mudar exige uma nova.
- **Notificação** ao cliente quando a avaliação é aprovada ou recusada.
- **`sort=top_rated` não expõe** um "mínimo de avaliações" configurável — o
  prior bayesiano resolve o caso, mas não é o mesmo que "só produtos com 10+".
