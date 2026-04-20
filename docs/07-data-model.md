# 07 — Modelo de dados

**Status**: Rascunho. **Data**: 2026-04-20.

Plano concreto de tabelas/colunas para tudo que a Utilar Ferragem toca, adiciona ou depende. Este documento é a fonte da verdade para migrations — cada migration listada aqui deve ser entregue no sprint indicado na seção "Responsabilidade de migrations" abaixo.

Leituras complementares:
- [03-architecture.md](03-architecture.md) — por que essas tabelas ficam onde estão
- [06-integration.md](06-integration.md) — quais mudanças aditivas são nossas vs. da plataforma
- [adr/002-integration-strategy.md](adr/002-integration-strategy.md) — contexto de design do payment-service

---

## 1. Tabelas existentes da plataforma que a Utilar lê

A Utilar **não é dona** dessas tabelas, mas as consome. Documentadas aqui para que mudanças de schema sejam coordenadas.

| Tabela | Serviço | O que a Utilar lê |
|--------|---------|-------------------|
| `users` | user-service | `id`, `email`, `role`, `cpf` (novo), `name` |
| `sellers` | user-service | `id`, `user_id`, `business_name`, `cnpj`, cidade/estado (para UI de "vendido por") |
| `products` | product-service | `id`, `title`, `description`, `price`, `currency`, `images`, `category`, `specs` (novo), `seller_id`, `status` |
| `inventory_items` | inventory-service | `product_id`, `available_qty` (derivado de estoque - reservado) — lido indiretamente via flag `in_stock` do produto |
| `orders` | order-service | `id`, `user_id`, `seller_id`, `status`, `total_cents`, `currency`, `created_at`, mais novas colunas de endereço BR |
| `order_items` | order-service | `order_id`, `product_id`, `title_snapshot`, `unit_price_cents`, `qty` |

---

## 2. Colunas aditivas que a Utilar adiciona às tabelas existentes

### 2.1 `products.specs` — JSONB para especificações técnicas de ferragens

Dono: product-service. Sprint: **04** (Utilar Sprint 04 / Fase 2).

```ruby
# services/product-service/db/migrate/YYYYMMDD_add_specs_to_products.rb
class AddSpecsToProducts < ActiveRecord::Migration[7.1]
  def change
    add_column :products, :specs, :jsonb, default: {}, null: false
    add_index  :products, :specs, using: :gin
  end
end
```

Formato concreto por categoria (ver §3 abaixo para os JSON Schemas completos).

### 2.2 `products.category_path` — taxonomia desnormalizada

Dono: product-service. Sprint: **04** (mesma migration do §2.1).

```ruby
add_column :products, :category_path, :string, limit: 128
add_index  :products, :category_path
```

Formato: `"ferramentas/eletricas/parafusadeiras"`. Escrito pelo product-service no create/update, derivado da string `category` + mapeamento de taxonomia do cliente inicialmente. Padrão `NULL` para produtos legados do Gifthy.

### 2.3 `users.cpf` — CPF do cliente

Dono: user-service. Sprint: **07** (sprint de auth da Utilar).

```ruby
# services/user-service/db/migrate/YYYYMMDD_add_cpf_to_users.rb
class AddCpfToUsers < ActiveRecord::Migration[7.1]
  def change
    add_column :users, :cpf, :string, limit: 11
    add_index  :users, :cpf, unique: true, where: 'cpf IS NOT NULL'
  end
end
```

Armazenamento somente com dígitos (`12345678909`). Renderizado como `123.456.789-09` na UI. Validado via Modulus 11 em `app/validators/cpf_validator.rb` (espelha o `cnpj_validator.rb` existente). Mirror no frontend em `src/lib/cpf.ts`.

### 2.4 Campos de endereço BR + `buyer_cpf` em `orders`

Dono: order-service. Sprint: **09** (sprint de pedidos da Utilar / Fase 3 Sprint 09).

```ruby
# services/order-service/db/migrate/YYYYMMDD_add_br_fields_to_orders.rb
class AddBrFieldsToOrders < ActiveRecord::Migration[7.1]
  def change
    change_table :orders, bulk: true do |t|
      t.string :buyer_cpf,      limit: 11
      t.string :shipping_cep,   limit: 8
      t.string :shipping_street, limit: 200
      t.string :shipping_number, limit: 20
      t.string :shipping_complement, limit: 100
      t.string :shipping_neighborhood, limit: 120
      t.string :shipping_city,  limit: 120
      t.string :shipping_state, limit: 2
      t.string :shipping_country, limit: 2, default: 'BR', null: false
    end

    add_index :orders, :buyer_cpf,       where: 'buyer_cpf IS NOT NULL'
    add_index :orders, [:status, :created_at]
    add_index :orders, :shipping_cep,    where: 'shipping_cep IS NOT NULL'
  end
end
```

Mantido desnormalizado no pedido (sem FK para uma tabela de endereços) porque:
1. O snapshot de endereço de faturamento/entrega precisa sobreviver a edições no perfil do usuário.
2. Segue o mesmo padrão já adotado por `order_items.title_snapshot`.
3. Evita um join cross-service com o user-service ao renderizar a página do pedido.

---

## 3. `products.specs` — formato JSON por categoria

A coluna é JSONB livre, mas o cliente aplica um schema por categoria folha via `src/lib/filters.ts`. O seeder e a UI admin validam contra o mesmo formato.

### 3.1 Ferramentas elétricas

```json
{
  "voltage": "220V",
  "power_w": 750,
  "max_rpm": 3000,
  "battery_v": null,
  "battery_ah": null,
  "chuck_mm": 13,
  "weight_kg": 1.8,
  "includes_case": true,
  "warranty_months": 12,
  "certification": ["INMETRO"]
}
```

Lista de campos:

| Chave | Tipo | Obrigatório | Notas |
|-------|------|------------|-------|
| `voltage` | enum | sim (exceto bateria) | `110V` / `220V` / `bivolt` |
| `power_w` | int | não | Ferramentas com fio |
| `max_rpm` | int | não | Furadeiras, esmerilhadeiras, serras |
| `battery_v` | enum | condicional | `12V` / `18V` / `20V` / `36V` — mutuamente exclusivo com `voltage` |
| `battery_ah` | decimal | condicional | Obrigatório se `battery_v` estiver preenchido |
| `chuck_mm` | int | não | Somente furadeiras |
| `weight_kg` | decimal | sim | Para cálculo de frete |
| `includes_case` | bool | sim | Fator de decisão de compra |
| `warranty_months` | int | sim | Padrão 12 se desconhecido |
| `certification` | string[] | não | `INMETRO`, `ANATEL`, etc. |

### 3.2 Elétrica (cabos, disjuntores, tomadas)

```json
{
  "voltage_rating": "750V",
  "current_a": 20,
  "gauge_awg": 12,
  "cable_mm2": 2.5,
  "poles": 1,
  "curve": "C",
  "protection_ip": "IP20",
  "standard": "NBR 60898",
  "length_m": 100
}
```

| Chave | Tipo | Obrigatório | Notas |
|-------|------|------------|-------|
| `voltage_rating` | enum | sim | `450V` / `750V` / `1000V` |
| `current_a` | int | condicional | Disjuntores, tomadas |
| `gauge_awg` | int | condicional | Cabos (padrão EUA) |
| `cable_mm2` | enum | condicional | `1.5` / `2.5` / `4` / `6` / `10` — cabos BR |
| `poles` | int | condicional | `1` / `2` / `3` — disjuntores |
| `curve` | enum | condicional | `B` / `C` / `D` — disjuntores |
| `protection_ip` | string | não | `IP20`, `IP44`, `IP65` |
| `standard` | string | sim | Referência NBR |
| `length_m` | int | condicional | Rolos de cabo |

### 3.3 Hidráulica (tubos, conexões, registros)

```json
{
  "diameter_mm": 25,
  "diameter_inches": "3/4",
  "thread": "BSP",
  "thread_size": "1/2",
  "material": "PVC",
  "pressure_class": "PN10",
  "connection_type": "soldavel",
  "length_m": 6,
  "cold_hot": "cold"
}
```

| Chave | Tipo | Obrigatório | Notas |
|-------|------|------------|-------|
| `diameter_mm` | enum | sim | `20` / `25` / `32` / `40` / `50` / `75` / `100` |
| `diameter_inches` | string | não | Alias para exibição |
| `thread` | enum | condicional | `BSP` / `NPT` / `soldavel` |
| `thread_size` | string | condicional | `1/2`, `3/4`, `1`, etc. |
| `material` | enum | sim | `PVC` / `CPVC` / `PPR` / `cobre` / `ferro` |
| `pressure_class` | enum | sim | `PN6` / `PN10` / `PN16` / `PN20` |
| `connection_type` | enum | sim | `soldavel` / `rosca` / `compressao` / `flange` |
| `length_m` | decimal | condicional | Tubos |
| `cold_hot` | enum | não | `cold` / `hot` / `both` |

### 3.4 Pintura (tintas, pincéis, lixas)

```json
{
  "finish": "acetinado",
  "base": "acrilica",
  "volume_l": 18,
  "coverage_m2_per_l": 12,
  "color": "branco neve",
  "color_code": "#F5F5F0",
  "surface": ["alvenaria", "reboco"],
  "drying_h": 4,
  "recoat_h": 4,
  "grit": null,
  "bristle_type": null
}
```

| Chave | Tipo | Obrigatório | Notas |
|-------|------|------------|-------|
| `finish` | enum | sim (tintas) | `fosco` / `acetinado` / `semi-brilho` / `brilhante` |
| `base` | enum | sim (tintas) | `acrilica` / `esmalte` / `latex` / `epoxi` |
| `volume_l` | decimal | sim (tintas) | `0.9` / `3.6` / `18` |
| `coverage_m2_per_l` | decimal | sim (tintas) | Para calculadora de rendimento |
| `color` | string | sim (tintas) | Nome legível |
| `color_code` | string | não | Hex |
| `surface` | string[] | sim (tintas) | `alvenaria` / `madeira` / `metal` / `gesso` / `piscina` |
| `drying_h` | decimal | sim (tintas) | Tempo de secagem superficial |
| `recoat_h` | decimal | sim (tintas) | Tempo mínimo entre demãos |
| `grit` | int | sim (lixas) | `40`–`2000` |
| `bristle_type` | enum | sim (pincéis) | `natural` / `sintetico` / `misto` |

### 3.5 Categoria padrão / desconhecida

Produtos sem schema correspondente armazenam `{}` e não renderizam a seção de ficha técnica.

---

## 4. Novas tabelas introduzidas pela Utilar

### 4.1 `payments` — payment-service

Dono: novo payment-service (Sprint 08).

```ruby
# services/payment-service/db/migrate/YYYYMMDD_create_payments.rb
create_table :payments do |t|
  t.bigint   :order_id,        null: false, index: true
  t.bigint   :user_id,         null: false, index: true
  t.string   :psp,             null: false, limit: 32  # 'mercadopago' etc.
  t.string   :psp_payment_id,  limit: 128,  index: { unique: true, where: 'psp_payment_id IS NOT NULL' }
  t.string   :method,          null: false, limit: 16  # 'pix' / 'boleto' / 'card'
  t.string   :status,          null: false, limit: 24  # ver máquina de estados abaixo
  t.integer  :amount_cents,    null: false
  t.string   :currency,        null: false, limit: 3, default: 'BRL'
  t.jsonb    :psp_metadata,    null: false, default: {}
  t.jsonb    :card_metadata,   null: false, default: {}  # last4, brand, hash do titular — nunca PAN completo
  t.string   :pix_qr_code,     limit: 4096
  t.string   :pix_copy_paste,  limit: 4096
  t.string   :boleto_url,      limit: 512
  t.datetime :confirmed_at
  t.datetime :expires_at
  t.datetime :failed_at
  t.string   :failure_reason,  limit: 200
  t.integer  :webhook_attempts, default: 0, null: false
  t.timestamps
end

add_index :payments, [:status, :created_at]
add_index :payments, :order_id
```

Máquina de estados (coluna `status`):

```
                ┌──────────┐
                │ pending  │ ─── expires_at elapsed ───▶ expired
                └────┬─────┘
       webhook OK ───┤         ┌──────────┐
                     ├────────▶│confirmed │
                     │         └──────────┘
       webhook FAIL ─┤         ┌──────────┐
                     ├────────▶│ failed   │
                     │         └──────────┘
       user cancel ──┤         ┌──────────┐
                     └────────▶│cancelled │
                               └──────────┘
```

### 4.2 `reviews` — product-service (Fase 4)

Dono: product-service. Sprint: **phase-4-growth.md** (adiado; não entra no lançamento da Fase 3).

```ruby
create_table :reviews do |t|
  t.bigint  :product_id,  null: false, index: true
  t.bigint  :user_id,     null: false, index: true
  t.bigint  :order_id,    null: false  # obrigatório ter comprado
  t.integer :rating,      null: false  # 1..5
  t.string  :title,       limit: 120
  t.text    :body
  t.string  :status,      null: false, limit: 16, default: 'pending'  # pending / approved / rejected
  t.integer :helpful_count, default: 0, null: false
  t.jsonb   :photos,      default: []
  t.timestamps
end

add_index :reviews, [:product_id, :status, :rating]
add_index :reviews, [:user_id, :product_id], unique: true  # uma avaliação por compra
```

### 4.3 `promotions` + `coupons` — order-service (Fase 4)

Dono: order-service. Sprint: adiado (Fase 4).

```ruby
create_table :promotions do |t|
  t.string  :code,         null: false, limit: 40
  t.string  :name,         null: false, limit: 120
  t.string  :kind,         null: false, limit: 16  # 'percent' / 'fixed' / 'free_shipping'
  t.integer :value_cents                             # para fixed
  t.integer :percent_bps                             # 500 = 5,00%
  t.integer :min_order_cents, default: 0
  t.integer :max_discount_cents
  t.bigint  :seller_id     # nullable — plataforma toda se null
  t.jsonb   :applies_to,   default: {}  # { category_paths: [...], product_ids: [...] }
  t.datetime :starts_at
  t.datetime :ends_at
  t.integer :usage_limit
  t.integer :usage_count,  default: 0, null: false
  t.boolean :active,       default: true, null: false
  t.timestamps
end

add_index :promotions, :code, unique: true
add_index :promotions, [:active, :starts_at, :ends_at]

create_table :coupon_redemptions do |t|
  t.bigint :promotion_id, null: false, index: true
  t.bigint :user_id,      null: false, index: true
  t.bigint :order_id,     null: false, index: { unique: true }
  t.integer :discount_cents, null: false
  t.timestamps
end
```

### 4.4 `shipping_rates` — order-service (lançamento da Fase 3)

Dono: order-service. Sprint: **09** (sprint de pedidos).

```ruby
create_table :shipping_rates do |t|
  t.bigint  :seller_id,     null: false, index: true
  t.string  :carrier,       null: false, limit: 32   # 'correios_pac' / 'correios_sedex' / 'jadlog' / 'local'
  t.string  :cep_prefix,    null: false, limit: 3    # '010' cobre 01000-000..01999-999
  t.integer :base_cents,    null: false
  t.integer :per_kg_cents,  null: false, default: 0
  t.integer :estimated_days, null: false
  t.integer :max_weight_g
  t.boolean :active,        null: false, default: true
  t.timestamps
end

add_index :shipping_rates, [:seller_id, :cep_prefix, :active]
add_index :shipping_rates, [:seller_id, :carrier, :cep_prefix], unique: true
```

Fase 3 entrega com busca simples por prefixo de CEP. Fase 4 integra a API real dos Correios (Calcular Preço e Prazo).

---

## 5. Relações e fluxo de eventos

```
                                                    Kafka (Redpanda)
                                                    ─ tópicos ─
                                                    order.created
                                                    payment.confirmed  ◀─── NOVO Sprint 08
                                                    payment.failed     ◀─── NOVO Sprint 08
                                                    inventory.reserved
                                                    inventory.low_stock

  ┌───────────┐    POST /api/v1/orders    ┌────────────────┐
  │ Utilar SPA│──────────────────────────▶│  order-service │──┐
  └───────────┘                            └────────────────┘  │ publica order.created
        │                                          ▲           ▼
        │ POST /api/v1/payments                    │     ┌──────────────────┐
        ▼                                          │     │ inventory-service│
  ┌─────────────────┐   publica payment.confirmed  │     └──────────────────┘
  │ payment-service │──────────────────────────────┤
  │  (NOVO Sprint 08│                              │ consome payment.confirmed
  └─────────────────┘                              │ → order.status = 'paid'
        │                                          │
        │ 3. Webhook do PSP (HTTPS)                │
        ▼                                          │
  ┌──────────┐    POST assinado com HMAC  ┌──────────────────┐
  │   PSP    │────────────────────────────▶│ /webhooks/psp/:n │
  │ (Mer.Pago│                             │   (gateway)      │
  └──────────┘                             └──────────────────┘
```

Sequência (caminho feliz do Pix):

1. SPA chama `POST /api/v1/orders` → order-service cria pedido `status='pending_payment'` + evento `order.created`.
2. SPA chama `POST /api/v1/payments` com `method='pix'` + `order_id` → payment-service chama o PSP → retorna registro `payments` com `pix_qr_code` + `pix_copy_paste`.
3. Cliente paga no app do banco → PSP chama `/webhooks/psp/mercadopago`.
4. payment-service valida HMAC, atualiza `payments` para `confirmed`, publica `payment.confirmed`.
5. Consumidor do order-service transiciona pedido para `status='paid'`, envia recibo por e-mail via SES.

---

## 6. Plano de índices

Lista consolidada, agrupada por serviço. Cada índice tem uma query que o justifica.

### product-service

| Índice | Tabela | Colunas | Justificativa |
|--------|--------|---------|---------------|
| `idx_products_status` | `products` | `status` | Existente — filtro do catálogo público `status='active'` |
| `idx_products_seller_status` | `products` | `seller_id, status` | Existente — dashboard do vendedor |
| `idx_products_category_path` | `products` | `category_path` | **NOVO** — `/marketplace/products?category=ferramentas/eletricas/parafusadeiras` |
| `idx_products_specs_gin` | `products` | `specs` (gin) | **NOVO** — `specs->>'voltage' = '220V'` filtros técnicos server-side |
| `idx_reviews_product_status` | `reviews` | `product_id, status, rating` | Fase 4 — paginação de avaliações no PDP |
| `idx_reviews_user_product` | `reviews` | `user_id, product_id` (unique) | Fase 4 — restrição de uma avaliação por compra |

### user-service

| Índice | Tabela | Colunas | Justificativa |
|--------|--------|---------|---------------|
| `idx_users_cpf` | `users` | `cpf` (unique parcial) | **NOVO** — lookup para verificação de idempotência no cadastro |
| `idx_users_email` | `users` | `email` (unique) | Existente |
| `idx_sellers_cnpj` | `sellers` | `cnpj` (unique) | Existente (Sprint 17) |

### order-service

| Índice | Tabela | Colunas | Justificativa |
|--------|--------|---------|---------------|
| `idx_orders_user` | `orders` | `user_id` | Existente — histórico de pedidos |
| `idx_orders_status_created` | `orders` | `status, created_at` | **NOVO** — fila admin + cron de limpeza de pendentes expirados |
| `idx_orders_buyer_cpf` | `orders` | `buyer_cpf` (parcial) | **NOVO** — lookup de suporte ao cliente |
| `idx_orders_shipping_cep` | `orders` | `shipping_cep` (parcial) | **NOVO** — roteamento de fulfillment |
| `idx_coupon_redemptions_order` | `coupon_redemptions` | `order_id` (unique) | Fase 4 — deduplicação de resgates |
| `idx_shipping_rates_lookup` | `shipping_rates` | `seller_id, cep_prefix, active` | **NOVO** — consulta de frete no checkout |

### payment-service

| Índice | Tabela | Colunas | Justificativa |
|--------|--------|---------|---------------|
| `idx_payments_order` | `payments` | `order_id` | **NOVO** — join na página de detalhe do pedido |
| `idx_payments_status_created` | `payments` | `status, created_at` | **NOVO** — cron de expiração + dashboard |
| `idx_payments_psp_payment_id` | `payments` | `psp_payment_id` (unique parcial) | **NOVO** — idempotência do webhook |

---

## 7. Responsabilidade de migrations e atribuição de sprint

| # | Migration | Serviço | Sprint | Reversível? | Downtime? |
|---|-----------|---------|--------|-------------|-----------|
| 1 | `add_specs_to_products` + `category_path` + índice GIN | product-service | Utilar 04 | sim | nenhum |
| 2 | `add_cpf_to_users` + índice unique parcial | user-service | Utilar 07 | sim | nenhum |
| 3 | `create_payments` | payment-service | Utilar 08 | sim | nenhum (novo serviço) |
| 4 | `add_br_fields_to_orders` + índice `status+created_at` | order-service | Utilar 09 | sim | nenhum |
| 5 | `create_shipping_rates` | order-service | Utilar 09 | sim | nenhum |
| 6 | `create_reviews` | product-service | Fase 4 | sim | nenhum |
| 7 | `create_promotions` + `coupon_redemptions` | order-service | Fase 4 | sim | nenhum |

**Regras**:
- Toda migration deve ser reversível (`def change` com blocos reversíveis ou `up`/`down` explícitos).
- Nenhuma migration pode alterar o tipo de uma coluna em tabela com > 10k linhas sem um plano de backfill (nenhuma das acima exige).
- Todos os índices GIN/BTREE em tabelas com > 100k linhas devem ser adicionados com `algorithm: :concurrently` (não necessário no volume atual, mas a regra vale para a Fase 4+).
- Rollout sem downtime: adicionar coluna (nullable) → fazer deploy do código que escreve → backfill → fazer deploy do código que lê → adicionar `NOT NULL` em um seguimento se necessário.

---

## 8. Retenção de dados

Não exaustivo; política completa em [08-security.md](08-security.md) §2.

| Tabela | Retenção | Notas |
|--------|----------|-------|
| `users` (ativos) | indefinido | Até solicitação de exclusão (LGPD) |
| `users` (excluídos) | 30 dias soft-delete, depois hard delete | Manter holds legais sobre pedidos |
| `orders` | 5 anos | Obrigação fiscal (BR) |
| `payments` | 5 anos | Obrigação fiscal |
| `reviews` | indefinido | Vinculado ao produto; anonimizar na exclusão do usuário |
| `webhooks_log` (payment-service) | 90 dias | Rotacionar via cron |
| `products` (excluídos) | soft delete, nunca purgado | Necessário para consulta histórica de pedidos |

---

---

## 9. Cashback ledger (Sprint 26)

Referência: [ADR 011](adr/011-cashback-mechanism.md), [Sprint 26](sprints/sprint-26-cashback.md).

### Colunas aditivas em tabelas existentes

| Tabela | Coluna | Tipo | Notas |
|--------|--------|------|-------|
| `products` (product-service) | `cashback_percent` | `DECIMAL(4,2) NOT NULL DEFAULT 0.00` | CHECK `0 <= cashback_percent <= 10`; vendedor define por produto |
| `order_items` (order-service) | `cashback_percent_snapshot` | `DECIMAL(4,2) NOT NULL DEFAULT 0.00` | Snapshotado na criação do pedido; vendedor não pode alterar retroativamente |
| `orders` (order-service) | `cashback_earned_cents` | `BIGINT NOT NULL DEFAULT 0` | Calculado na criação do pedido: `SUM(qty * unit_price_cents * cashback_percent_snapshot / 100)` |
| `payments` (payment-service) | `cashback_redemption_id` | `BIGINT nullable` | FK para `cashback_ledger.id` |
| `payments` (payment-service) | `cashback_discount_cents` | `BIGINT NOT NULL DEFAULT 0` | Valor deduzido da cobrança no PSP |

### Nova tabela: `cashback_ledger` (user-service)

```sql
CREATE TABLE cashback_ledger (
  id                BIGSERIAL PRIMARY KEY,
  user_id           BIGINT NOT NULL REFERENCES users(id),
  order_id          BIGINT,                                  -- NULL para ajustes manuais
  amount_cents      BIGINT NOT NULL CHECK (amount_cents > 0),
  currency          CHAR(3) NOT NULL DEFAULT 'BRL',
  kind              VARCHAR(20) NOT NULL                     -- earned|redeemed|expired|reversed|adjusted
                      CHECK (kind IN ('earned','redeemed','expired','reversed','adjusted')),
  status            VARCHAR(20) NOT NULL                     -- pending|available|used|expired|reversed
                      CHECK (status IN ('pending','available','used','expired','reversed')),
  earned_at         TIMESTAMPTZ,
  available_at      TIMESTAMPTZ,
  expires_at        TIMESTAMPTZ,
  metadata_json     JSONB,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### Índices

```sql
-- Idempotência: uma linha "earned" por pedido
CREATE UNIQUE INDEX idx_cashback_ledger_order_earned
  ON cashback_ledger (order_id, kind)
  WHERE kind = 'earned' AND order_id IS NOT NULL;

-- Consultas de saldo
CREATE INDEX idx_cashback_ledger_user_status
  ON cashback_ledger (user_id, status);

-- Job de expiração
CREATE INDEX idx_cashback_ledger_expiry
  ON cashback_ledger (expires_at)
  WHERE status = 'available';

CREATE INDEX idx_cashback_ledger_order
  ON cashback_ledger (order_id);
```

### Máquina de estados

```
  pending ──(pedido entregue)──▶ available ──(resgatado)──▶ used
     │                                │
     │                                └──(12 meses sem uso)──▶ expired
     │
     └──(pedido cancelado / estornado antes da entrega)──▶ reversed
```

### Responsabilidade de migrations

| Arquivo de migration | Serviço | Sprint |
|----------------------|---------|--------|
| `YYYYMMDD_add_cashback_percent_to_products.rb` | product-service | 26 |
| `YYYYMMDD_add_cashback_snapshot_to_order_items.rb` | order-service | 26 |
| `YYYYMMDD_add_cashback_to_orders.rb` | order-service | 26 |
| `YYYYMMDD_add_cashback_columns_to_payments.rb` | payment-service | 26 |
| `YYYYMMDD_create_cashback_ledger.rb` | user-service | 26 |

---

## 10. Fora do escopo deste documento

- Schema de warehouse analítico / OLAP — Fase 5
- Ledger multi-moeda — Fase 5 (somente BRL no lançamento)
- Schema de repasse para vendedores / liquidação do marketplace — Fase 4
- Tabelas de chat / mensagens — Fase 5
- Campanhas de boost / multiplicador de cashback — Fase 6 (adiciona `cashback_multiplier` à tabela de promoções)
