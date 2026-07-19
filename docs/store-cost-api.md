# API de custo do balcão — `/api/v1/store`

Serviço: **catalog-service** (`:8091`).
Consumidores: **PDV de balcão** (`/balcao` no SPA) e **order-service** (CMV do pedido de balcão).

---

## Por que esta rota existe

O custo de aquisição só saía por `GET /api/v1/admin/products/by-id/:id`, atrás de
`RequireAdmin`. O papel `store_operator` não alcançava, então o PDV rodava a barra de
margem em **custo estimado** (`preço × 0,72`).

Num caso medido, o custo real dava **60% de margem** e a estimativa dava **28%** —
**32 pontos de diferença**, e é esse número que o vendedor usa para decidir até onde pode
dar desconto.

A rota entrega custo e margem a quem opera a loja **sem afrouxar a regra pública**:
`cost` continua estruturalmente ausente de `model.Product` e da projeção
`productColumns`. Nada mudou nas rotas abertas — o teste `TestPublicAPI_NuncaVazaCusto`
segue valendo sem nenhum enfraquecimento.

---

## Autorização

Middleware: `handler.RequireStore(JWT_SECRET, SERVICE_JWT_SECRET, DEV_MODE)`.

| Identidade | Segredo que assina | Resultado |
|---|---|---|
| `role=store_operator` | `JWT_SECRET` | **200** |
| `role=admin` | `JWT_SECRET` | **200** |
| `role=service` | `SERVICE_JWT_SECRET` | **200** |
| `role=service` forjado com `JWT_SECRET` | — | **401** |
| `role=customer`, `role=seller`, papel desconhecido | `JWT_SECRET` | **403** |
| sem `Authorization` | — | **401** |
| token inválido/expirado | — | **401** |

> ⚠️ `seller` é **lojista do marketplace**, não vendedor de balcão. Ele **não** entra.

Em `DEV_MODE=true` vale o fallback de header `X-User-Role: store_operator|admin|service`
(o `pkg/devguard` recusa `DEV_MODE` em ambiente com sinal de produção).

Coberto por `TestBalcao_CustoNuncaRespondeParaClienteOuAnonimo` e
`TestBalcao_RecusaRoleServiceForjadoComSegredoDeUsuario`.

---

## Endpoints

Os dois fazem a mesma coisa. Use o **POST** quando o carrinho for grande: 200 UUIDs em
query string dão ~7,4 KB, e um proxy que trunca a URL devolveria o custo de *parte* do
carrinho — pior que não devolver nenhum.

### `GET /api/v1/store/products/costs?ids=<uuid>,<uuid>,...`

### `POST /api/v1/store/products/costs`

```json
{ "ids": ["8f14e45f-…", "c9f0f895-…"] }
```

**É em lote de propósito.** O PDV monta um carrinho com vários itens; uma chamada por
item viraria N+1 no caminho mais quente do balcão.

### Regras de entrada

| Regra | Comportamento |
|---|---|
| id fora do formato UUID | `400 bad_request` (`invalid product id: …`) — nunca 500 |
| `ids` ausente / vazio / só vírgulas | `400 bad_request` |
| mais de **200** ids distintos | `400 bad_request` (`too many ids (max 200)`) |
| ids repetidos | deduplicados; uma linha por produto na resposta |

---

## Resposta — `200 OK`

```json
{
  "data": [
    {
      "id": "8f14e45f-…",
      "sku": "UTL-CON-0002",
      "name": "Cimento CP II-E-32 Votoran 50kg",
      "price": 42.90,
      "currency": "BRL",
      "cost": 35.10,
      "marginPct": 18.18,
      "unitOfMeasure": "sc",
      "status": "published"
    }
  ],
  "missing": ["00000000-0000-0000-0000-0000000000ff"]
}
```

Header: `Cache-Control: no-store`. Custo é dado sensível e volátil — um proxy
intermediário guardando a resposta serviria custo a quem não pediu.

### Campos

| Campo | Tipo | Observação |
|---|---|---|
| `id` | `string` (uuid) | |
| `sku` | `string \| ausente` | omitido quando o produto não tem SKU |
| `name` | `string` | |
| `price` | `number` | preço de tabela **do servidor**. Vem junto para que a margem não seja calculada contra um preço velho em memória do PDV |
| `currency` | `string` | `BRL` |
| `cost` | `number \| null` | **sempre presente**, pode ser `null` |
| `marginPct` | `number \| null` | `(price − cost) / price × 100`. `null` quando não há custo ou `price ≤ 0` |
| `unitOfMeasure` | `string` | `un`, `sc`, `m3`, … |
| `status` | `string` | `draft` \| `published` \| `archived` |
| `missing` | `string[]` | ids pedidos que não existem — **sempre presente**, pode ser `[]` |

### Três contratos que o frontend precisa respeitar

1. **`cost: null` não é "sem resposta", é "a loja não sabe o custo".** Nesse caso o PDV
   deve exibir *margem indisponível* — **nunca** cair de volta na estimativa
   `preço × 0,72`. Esse fallback é exatamente o defeito que esta rota conserta.
2. **`missing` não pode ser ignorado.** Item que não voltou não pode sumir em silêncio,
   senão a barra de margem cobre parte do carrinho sem ninguém notar.
3. **A rota não filtra por `status='published'`.** O item está na prateleira mesmo quando
   está em rascunho na vitrine. Por isso `status` vem no payload: quem exibe decide se
   avisa.

### O que **não** vem (e por quê)

`supplierId`, `supplierSku`, `ncm`, `cfop`, `cest`, `origem`. É inteligência de compra —
quem opera o caixa não precisa saber de quem a loja compra. Menos campo exposto, menor o
estrago se um token de operador vazar. Continua tudo em
`GET /api/v1/admin/products/by-id/:id` para quem é admin.
(`TestBalcao_NaoExpoeFornecedorNemDadosFiscais`)

### Coerência com a rota de admin

`marginPct` sai da **mesma função** (`handler.marginPct`) usada por
`GET /api/v1/admin/products/by-id/:id`. Gerente e vendedor veem o mesmo número para o
mesmo produto — travado por `TestBalcao_MargemBateComARotaAdmin`.

---

## Erros

Envelope padrão da casa: `{error, code, requestId}`.
Códigos: `bad_request`, `unauthorized`, `forbidden`, `db_error`.
Nenhuma resposta de erro contém a chave `cost`.

---

## SKU e código de barras

Ambos saem na **listagem** (`GET /api/v1/products`) e no **detalhe**
(`/products/:slug`, `/products/by-id/:id`) — são públicos de propósito: o vendedor
confere na tela o item que bipou.

Busca exata do leitor: `GET /api/v1/products?sku=…` e `?barcode=…`. Usa `=` (nunca
`ILIKE`) contra os índices únicos B-tree `idx_products_sku` e `idx_products_barcode`.
O índice trigram existe só para a busca por **prefixo** do `?q=`.
(`TestIndices_LookupExatoDeSKUeBarcodeTemBtree`)

### Decisão sobre código de barras

Códigos gerados pela casa vivem na **faixa de circulação restrita do GS1** — GTIN-13
começando com `2`, no formato `200` + 9 dígitos sequenciais + dígito verificador.

**Não inventamos EAN de fabricante.** Um código `789…` (prefixo brasileiro) é identidade
física falsa: aquele número pertence, ou vai pertencer, a um produto real. No dia em que
a loja bipar o item de verdade, o código casaria com o produto errado e a venda sairia
com preço, custo e estoque de outra coisa. O seed antigo fazia isso
(`'789' || hash(slug)`); foi removido. A faixa `2` é reservada pela norma para uso
interno de loja e nunca é alocada a fabricante — é o mesmo mecanismo da etiqueta de
balança do supermercado.

Quando a planilha real do fornecedor chegar com os EANs verdadeiros, eles substituem os
internos.

Atribuição: `services/catalog-service/migrations/balcao_ids.sql` (idempotente — só
preenche `NULL`). Rode-o depois de `scripts/ingestao/importar_curado.py`; o mesmo bloco
já está embutido no `seed.sql`, e `TestSeed_BlocoDeIdentificacaoDeBalcaoNaoDivergiu`
impede os dois de divergirem.
