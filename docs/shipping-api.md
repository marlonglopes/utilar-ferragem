# Frete — contrato para o frontend

Serviço: **order-service** (`:8092`)
Status: implementado (migration `002_fulfillment_and_shipping`)

## Por que isto existe

Até a migration 002, o total do pedido era `subtotal + shippingCost`, com
`shippingCost` vindo do corpo do request. Mandar `shippingCost: 0` funcionava e
o pedido era gravado com frete grátis.

Agora o servidor calcula o frete a partir de uma tabela (`shipping_rates`) e do
CEP do endereço de entrega. **O `shippingCost` enviado pelo cliente é ignorado.**

---

## `POST /api/v1/shipping/quote`

Cotação para o carrinho. Chame quando o usuário digitar/alterar o CEP.

Requer o mesmo `Authorization: Bearer <jwt>` das demais rotas.

### Request

```json
{
  "cep": "01310-100",
  "subtotal": 150.00,
  "itemCount": 2
}
```

| campo | tipo | obrigatório | observação |
|---|---|---|---|
| `cep` | string | sim | com ou sem hífen; 8 dígitos |
| `subtotal` | number | não (default 0) | soma dos itens **sem** frete — decide o frete grátis |
| `itemCount` | number | não (default 0) | soma das quantidades, não número de linhas do carrinho |

### Response `200`

```json
{
  "cep": "01310-100",
  "options": [
    {
      "serviceCode": "standard",
      "serviceName": "Entrega padrão",
      "zoneName": "São Paulo - Capital",
      "cost": 24.90,
      "deliveryDays": 2,
      "free": false
    },
    {
      "serviceCode": "express",
      "serviceName": "Entrega expressa",
      "zoneName": "São Paulo - Capital",
      "cost": 47.90,
      "deliveryDays": 1,
      "free": false
    }
  ]
}
```

- `options` vem **ordenado da mais barata para a mais cara** (empate: prazo menor primeiro). Pode ser 1 ou 2 opções conforme a região.
- `deliveryDays` é em **dias úteis**.
- `free: true` significa que a compra passou do limiar de frete grátis daquela faixa — mostre "Frete grátis" em vez de "R$ 0,00".

### Erros

| status | `code` | quando | o que mostrar |
|---|---|---|---|
| `400` | `bad_request` | CEP fora do formato de 8 dígitos | "CEP inválido" |
| `422` | `no_shipping_coverage` | nenhuma faixa cobre o CEP | "Não entregamos nesta região" |
| `500` | `db_error` / `internal` | falha ao ler a tabela | erro genérico + retry |

Envelope de erro padrão da plataforma:

```json
{ "error": "...", "code": "...", "requestId": "01K..." }
```

---

## `POST /api/v1/orders` — o que mudou

```json
{
  "paymentMethod": "pix",
  "shippingService": "standard",
  "shippingCost": 24.90,
  "items": [ ... ],
  "address": { "cep": "01310-100", ... }
}
```

- **`shippingService`** (novo, opcional): `"standard"` ou `"express"` — qual opção
  da cotação o usuário escolheu. **Omitido = a mais barata disponível.**
- **`shippingCost`**: continua aceito, mas **não entra no total**. O servidor
  recalcula. Se divergir do valor do servidor, um `WARN` é logado (indica
  cotação velha em cache no front) e o valor do servidor prevalece.

A resposta traz os valores autoritativos:

```json
{
  "subtotal": 150.00,
  "shippingCost": 24.90,
  "shippingService": "standard",
  "total": 174.90
}
```

### Novos erros em `POST /orders`

| status | `code` | quando |
|---|---|---|
| `409` | `insufficient_stock` | algum item não tem saldo |
| `422` | `no_shipping_coverage` | CEP do endereço fora da área de entrega |
| `400` | `bad_request` | CEP malformado |

O `insufficient_stock` traz `details` para o carrinho se autocorrigir:

```json
{
  "error": "estoque insuficiente para o produto <uuid>: pedido 5, disponível 2",
  "code": "insufficient_stock",
  "requestId": "01K...",
  "details": { "productId": "<uuid>", "requested": 5, "available": 2 }
}
```

Sugestão de UX: ajustar a quantidade da linha para `available` (ou remover, se
`available: 0`) e reapresentar o carrinho, em vez de mandar o usuário de volta
ao início do checkout.

---

## Tabela de frete (referência)

Faixas do seed — o operador ajusta em `shipping_rates` **sem deploy** (o cache
do serviço expira em 60s).

| Zona | Faixa de CEP | Serviço | Base | Por item | Prazo | Frete grátis acima de |
|---|---|---|---|---|---|---|
| São Paulo - Capital | 01000-000 – 05999-999 | standard | 19,90 | 2,50 | 2 | 299,00 |
| São Paulo - Capital | 01000-000 – 05999-999 | express | 39,90 | 4,00 | 1 | — |
| Grande São Paulo | 06000-000 – 09999-999 | standard | 24,90 | 3,00 | 3 | 299,00 |
| Grande São Paulo | 06000-000 – 09999-999 | express | 49,90 | 5,00 | 2 | — |
| Interior de SP | 10000-000 – 19999-999 | standard | 34,90 | 3,50 | 5 | 499,00 |
| Interior de SP | 10000-000 – 19999-999 | express | 69,90 | 6,00 | 3 | — |
| Sudeste | 20000-000 – 39999-999 | standard | 49,90 | 4,50 | 7 | 799,00 |
| Nordeste | 40000-000 – 65999-999 | standard | 79,90 | 6,50 | 12 | — |
| Norte | 66000-000 – 69999-999 | standard | 99,90 | 8,00 | 15 | — |
| Sul e Centro-Oeste | 70000-000 – 89999-999 | standard | 64,90 | 5,50 | 10 | — |
| Sul | 90000-000 – 99999-999 | standard | 64,90 | 5,50 | 10 | — |

Fórmula: `custo = base + (por_item × itemCount)`, zerado quando
`subtotal >= free_above` (e `free_above > 0`).

### Limitação conhecida

O cálculo **não considera peso** porque `products` não tem coluna de peso. O
`cost_per_item` aproxima o efeito do volume. Quando o catálogo ganhar peso, a
tabela ganha faixas de peso e este contrato HTTP não muda — só os valores de
`cost`.
