# Pendências de backend abertas pela experiência do cliente

Escrito ao construir favoritos, recompra, linha do tempo e relacionados no SPA
(`app/src/`). Cada item abaixo é uma coisa que o **frontend já sabe fazer** e
está esperando o servidor. Onde não havia endpoint, o front escolheu um caminho
honesto e deixou o encaixe pronto — a intenção é que o trabalho de backend seja
"ligar o fio", não redesenhar a feature.

Ordem sugerida: **1 (favoritos) → 2 (co-compra) → 3 (etapas do pedido) → 4 (logout)**.

---

## 1. Favoritos — não existe backend nenhum

**Hoje:** a lista vive só no `localStorage` do dispositivo
(`app/src/store/favoritesStore.ts`, chave `utilar-favorites`).

**Consequência para o cliente:** quem monta a lista no celular da obra não a
encontra no computador de casa. Trocar de aparelho ou limpar o navegador apaga
tudo. A página `/favoritos` diz isso em texto ("Sua lista fica salva neste
dispositivo") em vez de fingir que sincroniza.

### Endpoints necessários (auth-service ou serviço próprio)

Escopados pelo JWT, como todo o resto — zero IDOR.

```
GET    /api/v1/favorites                 → { data: FavoriteItem[] }
POST   /api/v1/favorites                 { productId }        → 201 | 409 se já existe
DELETE /api/v1/favorites/:productId      → 204
POST   /api/v1/favorites/merge           { items: FavoriteItem[] } → { data: FavoriteItem[] }
```

`FavoriteItem` (mesmo formato que o store já usa):

```ts
{ productId, slug, name, icon, priceSnapshot, imageUrl?, seller, addedAt }
```

### O ponto que precisa de cuidado: o merge no login

O caminho ingênuo — `GET /favorites` e sobrescrever o local — **apaga a lista que
o visitante montou antes de criar a conta**, que é exatamente quando ele mais
tem itens salvos (veio pesquisando material, favoritou 6 coisas, aí decidiu
comprar e se cadastrou).

A regra correta já está implementada e testada no front, em
`favoritesStore.mergeFromServer()`:

- união por `productId`;
- em caso de empate, vence o **`addedAt` mais antigo** (favoritou cimento no
  celular semana passada e no desktop hoje ⇒ um favorito só, datado de semana
  passada);
- resultado ordenado por mais recente primeiro.

Testes: `app/src/test/favoritesStore.test.ts`, bloco
*"merge com o servidor (caminho pronto pro backend)"*.

**Sugestão:** `POST /favorites/merge` recebe a lista local, faz a união no
servidor com essa mesma regra e devolve a lista canônica. Assim a regra vive num
lugar só. Se preferir manter no cliente, o front já chama `mergeFromServer` com
o que vier do `GET`.

### Onde ligar

`useLogout` e a tela de login são os dois pontos: ao autenticar, chamar o merge;
ao sair, decidir se a lista local persiste (hoje persiste de propósito — ver
comentário em `useLogout`).

### Guardar o snapshot ou só o id?

O front guarda um snapshot do produto para a página `/favoritos` renderizar
instantaneamente e offline (no 4G da obra, N requisições antes de mostrar
qualquer coisa é inaceitável). O servidor pode guardar só `productId` e
reidratar o resto no `GET` via join com o catálogo — o formato de resposta é que
precisa vir completo.

⚠️ O `priceSnapshot` é **preço histórico**, exibido rotulado como "preço quando
você salvou". Não use esse campo para nada além de exibição.

---

## 2. "Compre junto" — backend chegou; falta ligar em produção

**Atualização:** enquanto este trabalho acontecia, outro agente implementou
`RecommendationHandler` no catalog-service (`internal/handler/recommendation.go`,
`internal/reco/`, migração `016_recommendations`). O contrato dele é **melhor**
do que o que eu tinha proposto, e o frontend já foi adaptado para ele.

### Contrato real (já consumido pelo front)

```jsonc
GET /api/v1/products/:slug/related?limit=4
{
  "data": [
    { /* ...Product... */,
      "reason": { "kind": "copurchase", "label": "12 clientes levaram junto", "orders": 12 } }
  ],
  "meta": {
    "strategy": "copurchase" | "complement" | "mixed" | "category_fallback",
    "fallback": true,                  // algum item veio do preenchimento por categoria
    "minCopurchaseOrders": 5,
    "counts": { "copurchase": 2, "complement": 1, "fallback": 1 }
  }
}
```

Como o front usa cada campo:

| Campo | Efeito na tela |
|---|---|
| `strategy: copurchase` / `mixed` | título "Quem comprou este levou também" |
| `strategy: complement` | título "Costuma ir junto" |
| `strategy: category_fallback` | "Outros produtos de {categoria}" + ressalva + link pra categoria |
| `meta.fallback: true` | **derruba o título otimista** mesmo em `copurchase` |
| `reason.label` | exibido embaixo de cada card ("12 clientes levaram junto") |

`meta.fallback` derrubar o título é deliberado: meia lista completada por
categoria, anunciada como recomendação, é promessa quebrada para metade dos
itens. Teste: `app/src/test/RelatedProducts.test.tsx`.

### O que ainda falta

1. **Subir o binário novo.** O catalog-service em execução hoje ainda é o
   antigo: `GET /products/cabo-flexivel-2-5mm-100m/related` responde
   **`meta: null`** e `reason: null` em todos os itens. O front trata isso — sem
   `meta`, cai em `category_fallback` (o rótulo modesto) e não quebra —, mas
   ninguém vê a recomendação até o deploy.

2. **Popular `product_copurchase`.** Existe o endpoint administrativo
   `POST /api/v1/admin/recommendations/copurchase/rebuild`. Falta agendá-lo (job
   periódico) e confirmar de onde saem os pares, já que `order_items` mora no
   order-service e o catálogo não faz `SELECT` lá.

3. **Conferir o limiar `MinCopurchaseOrders`** com dado real. Com pouco volume,
   quase tudo vai cair em `category_fallback` — que é o comportamento correto e
   honesto, mas significa que a feature só "aparece" depois de um certo volume
   de pedidos.

4. **Regra de estratégia desconhecida:** o front normaliza qualquer valor não
   reconhecido para `category_fallback` (errar para o lado humilde). Se
   aparecerem estratégias novas, avise para o front passar a rotulá-las.

---

## 3. Linha do tempo — carimbos de etapa faltando

**Hoje:** a linha do tempo (`app/src/components/orders/OrderTimeline.tsx`) usa
`createdAt`, `paidAt`, `pickedAt`, `shippedAt`, `deliveredAt`, `cancelledAt`.

**Verificado contra o order-service rodando:** todos os 6 pedidos do usuário
semeado `test1@utilar.com.br` voltam com `paidAt`, `pickedAt`, `shippedAt` e
`deliveredAt` **nulos**. Ou seja: para pedidos que avançaram de status, a linha
do tempo mostra a etapa como concluída mas com **"Data não informada"**.

Isso é proposital do lado do front — sumir com a etapa faria um pedido já
enviado parecer que pulou a separação. Mas a informação que o cliente quer
("saiu pra entrega quando?") não existe no banco.

### O que falta

- **Popular os carimbos** em cada transição de status no order-service. Se as
  colunas já existem e só não estão sendo escritas, é o menor trabalho desta
  lista.
- **Alternativa melhor, se houver apetite:** expor o histórico real em vez de
  cinco colunas:

  ```jsonc
  GET /api/v1/orders/:id
  {
    "timeline": [
      { "status": "paid",    "at": "2026-04-10T14:30:00Z", "note": null },
      { "status": "picking", "at": "2026-04-11T12:00:00Z", "note": null },
      { "status": "shipped", "at": "2026-04-12T09:00:00Z", "note": "Correios PAC" }
    ]
  }
  ```

  Isso permite etapas fora da sequência canônica (tentativa de entrega frustrada,
  devolução, reenvio) que hoje não têm como ser exibidas. Se for por aqui, avise
  — o componente muda pouco, mas muda.

- **Rastreio:** `trackingCode` já é lido e ancorado no passo "enviado", com link
  para os Correios. Nenhum pedido semeado tem código; vale confirmar que o campo
  é preenchido quando a etiqueta é emitida.

- **Previsão de entrega** não existe no modelo. É a informação nº 1 que o cliente
  procura depois do status. Se o cálculo de frete já produz um prazo no
  checkout, persistir esse prazo no pedido (`estimatedDeliveryAt`) seria barato e
  de alto retorno — a linha do tempo tem lugar reservado para ele.

---

## 4. Logout não revoga o refresh token

**Hoje:** sair da conta é **puramente local** — `useLogout` zera o `authStore`,
limpa o cache do TanStack Query e navega. Nenhuma chamada ao servidor.

**Consequência:** o `refreshToken` (30 dias, revogável) é **descartado no
cliente sem ser invalidado no servidor**. Quem tiver capturado esse token
continua renovando sessão depois que o cliente "saiu". Num aparelho
compartilhado — o caso que motivou colocar o "Sair" no cabeçalho — isso é
justamente o cenário que se quer cobrir.

O comentário em `app/src/lib/api.ts` já menciona um endpoint `logout` protegido,
mas **nada no frontend o chama** e não achei implementação no auth-service.

### O que falta

```
POST /api/v1/auth/logout      Authorization: Bearer <access>
     { refreshToken }         → 204
```

Deve revogar o refresh token (marcar como revogado na tabela de tokens). O front
liga isso em `useLogout` — uma linha — mas com duas ressalvas de comportamento:

- **Falha do endpoint não pode bloquear a saída.** Se a rede cair, a sessão local
  tem que ser encerrada do mesmo jeito. Logout que falha e deixa o usuário logado
  é pior que logout sem revogação.
- Chamar **antes** de limpar o `authStore`, senão não há mais token para enviar.

Há um teste que documenta o estado atual e vai precisar mudar junto:
`app/src/test/logout.test.tsx` → *"não chama endpoint de logout no servidor"*.

---

## 5. Menores, mas reais

- **`GET /api/v1/products/by-id/:id`** — já existe e a recompra depende dele
  (o item do pedido guarda `productId`; buscar por slug faria produto renomeado
  aparecer como "fora de linha"). Só registrando que agora é caminho quente:
  uma recompra dispara N chamadas em paralelo. **Um endpoint em lote
  (`GET /products/by-ids?ids=a,b,c`) reduziria um pedido de 15 linhas de 15
  requisições para 1** — vale se a recompra pegar.

- **Preço e estoque na recompra** vêm do catálogo no momento do clique, nunca do
  pedido antigo. O `priceSnapshot` do carrinho é preenchido com o preço atual —
  o order-service continua sendo a autoridade no checkout, como deve ser.

- **Aviso de preço nos favoritos:** hoje a página mostra o preço de quando o item
  foi salvo, rotulado como tal. Com o backend de favoritos, dá para trazer o
  preço atual e destacar a variação ("caiu R$ 4,10 desde que você salvou") — é o
  tipo de coisa que traz o cliente de volta para fechar a compra.
