# catalog-service

Serviço de catálogo do marketplace Utilar Ferragem — lê produtos, categorias, vendedores e imagens. Read-only, público, sem autenticação.

| | |
|---|---|
| **Stack** | Go 1.26 + Gin 1.12 + Postgres 17 |
| **Porta** | `:8091` |
| **DB** | `utilar_catalog_db` (Postgres em `localhost:5436`) |
| **Status** | Phase B1 ✅ em produção no dev, plugado no frontend (Vite) |

Para visão geral do projeto, ver [README raiz](../../README.md). Para gestão do banco em dev, ver [docs/maintenance/database.md](../../docs/maintenance/database.md).

---

## Estrutura

```
catalog-service/
  cmd/server/main.go          ← bootstrap (config, DB, migrate, router, CORS, graceful shutdown)
  internal/
    config/                   ← env vars (PORT, CATALOG_DB_URL)
    db/                       ← sql.Open + golang-migrate auto-migrate no startup
    handler/                  ← product, category, seller — handlers Gin + testes
    model/                    ← structs Go com JSON tags em camelCase
  migrations/
    001_create_catalog.up.sql ← DDL (4 tabelas, ENUM, pg_trgm, triggers)
    001_create_catalog.down.sql
    seed.sql                  ← 111 produtos de teste (31 reais + 80 sintéticos)
  Makefile                    ← run / build / test / tidy
  bin/                        ← binário compilado (gitignored)
```

---

## API

Base URL em dev: `http://localhost:8091`. CORS liberado (`Access-Control-Allow-Origin: *`) para desenvolvimento.

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/health` | liveness probe (verifica DB) |
| `GET` | `/api/v1/categories` | 8 categorias top-level |
| `GET` | `/api/v1/sellers` | lista de vendedores |
| `GET` | `/api/v1/products` | listagem com filtros + paginação |
| `GET` | `/api/v1/products/facets` | marcas + faixa de preço para sidebar |
| `GET` | `/api/v1/products/:slug` | detalhe + imagens |

### Query params de `/api/v1/products`

| Param | Tipo | Default | Descrição |
|---|---|---|---|
| `category` | string (slug) | — | filtra por categoria (`ferramentas`, `construcao`, ...) |
| `q` | string | — | busca ILIKE em `name`, `description`, `seller.name` |
| `brand` | string | — | filtra por marca exata (ex: `Bosch`) |
| `price_min` | float | — | preço mínimo (inclusivo) |
| `price_max` | float | — | preço máximo (inclusivo) |
| `in_stock` | bool | `false` | `true` → apenas produtos com `stock > 0` |
| `sort` | enum | `newest` | `price_asc`, `price_desc`, `top_rated`, `newest` |
| `page` | int | `1` | página (≥ 1) |
| `per_page` | int | `24` | itens por página (1–100) |

### Exemplo

```bash
curl 'http://localhost:8091/api/v1/products?category=ferramentas&q=bosch&sort=price_asc&per_page=5'
```

```json
{
  "data": [
    {
      "id": "uuid",
      "slug": "furadeira-bosch-gsb-13-re",
      "name": "Furadeira de Impacto Bosch GSB 13 RE 650W 127V",
      "category": "ferramentas",
      "price": 329,
      "originalPrice": 389,
      "currency": "BRL",
      "icon": "⚒",
      "brand": "Bosch",
      "seller": "Ferragem Silva",
      "sellerId": "ferragem-silva",
      "sellerRating": 4.8,
      "sellerReviewCount": 1240,
      "stock": 42,
      "rating": 5,
      "reviewCount": 142,
      "cashbackAmount": 24.9,
      "badge": "discount",
      "badgeLabel": "-15%",
      "installments": 12,
      "description": "...",
      "specs": {"Potência":"650 W", "Mandril":"13 mm", ...},
      "createdAt": "2026-04-24T12:21:47Z",
      "updatedAt": "2026-04-24T12:21:47Z"
    }
  ],
  "meta": { "page": 1, "per_page": 5, "total": 7, "total_pages": 2 }
}
```

> **Shape:** JSON em **camelCase** dentro de `data[]` (matching direto com `app/src/types/product.ts`). `meta` fica em snake_case por compatibilidade com o tipo `ProductsResponse` já existente.

---

## Configuração

Variáveis de ambiente (todas opcionais — têm default de dev):

| Var | Default | Descrição |
|---|---|---|
| `PORT` | `8091` | porta HTTP |
| `CATALOG_DB_URL` | `postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable` | DSN Postgres |

Em produção, `CATALOG_DB_URL` apontará para RDS (ver [docs/11-infra.md](../../docs/11-infra.md)).

---

## Rodar localmente

### Via Makefile raiz (recomendado)

```bash
# 1) subir infra (Postgres + Redpanda)
make infra-up

# 2) criar schema + popular com 111 produtos
make catalog-db-reset

# 3) rodar o servidor
make catalog-run
```

### Atalho dev-mode completo (infra + server + SPA)

```bash
make dev-catalog
# sobe infra + catalog-service + Vite com VITE_CATALOG_URL=http://localhost:8091
# SPA em http://localhost:5173 → catálogo live, pagamento/pedidos mockados
```

### Manual (para debug)

```bash
cd services/catalog-service
go run ./cmd/server
```

---

## Migrations

**Ferramenta**: [golang-migrate](https://github.com/golang-migrate/migrate). Migrations rodam automaticamente no startup do servidor (`db.Migrate()` em [cmd/server/main.go](cmd/server/main.go)) — mas também podem ser aplicadas via Makefile quando o servidor não está rodando.

### Comandos

```bash
make catalog-db-migrate       # aplica *.up.sql em ordem
make catalog-db-migrate-down  # reverte *.down.sql em ordem reversa
make catalog-db-seed          # roda seed.sql (idempotente)
make catalog-db-reset         # down + up + seed
make catalog-db-clean         # TRUNCATE sem perder schema
make catalog-db-status        # \dt + contagem de linhas
make catalog-db-psql          # shell interativo
make catalog-db-dump          # backup em backups/catalog_<ts>.sql
make catalog-db-restore FILE=<path>
```

> **Detalhe técnico:** os targets do Makefile aplicam SQL via `docker exec psql` e também registram a versão em `schema_migrations` (esquema do `golang-migrate`), evitando que o servidor Go tente reaplicar quando subir depois.

### Criar uma nova migration

1. Criar dois arquivos seguindo a numeração sequencial (próxima disponível):

   ```
   migrations/002_add_seller_pickup_locations.up.sql
   migrations/002_add_seller_pickup_locations.down.sql
   ```

2. Escrever DDL no `up.sql` e o inverso no `down.sql`. Boas práticas:

   - **Idempotência**: prefira `CREATE TABLE IF NOT EXISTS`, `DROP ... IF EXISTS`.
   - **Reversibilidade**: toda migration deve ter `down.sql` testável — crie FK `ON DELETE CASCADE` para não ficar órfão.
   - **Zero-downtime**: em produção, `ADD COLUMN` sem `DEFAULT` é seguro; `ADD COLUMN NOT NULL DEFAULT` bloqueia tabelas grandes no Postgres < 11. Use `ADD COLUMN` + `UPDATE em batches` + `SET NOT NULL` em 3 migrations.
   - **Nomes**: `NNN_verbo_objeto.up.sql` (`add_products_brand_index`, `alter_sellers_add_cnpj`).

3. Ajustar `seed.sql` se a nova tabela precisa de dados de teste.

4. Rodar localmente:

   ```bash
   make catalog-db-migrate       # aplica a nova
   make catalog-db-migrate-down  # testa reversibilidade
   make catalog-db-migrate       # reaplica
   ```

5. Commitar `002_*.up.sql`, `002_*.down.sql` e (se aplicável) alterações ao `seed.sql` juntos.

### Em produção

As migrations rodam automaticamente no deploy via `db.Migrate()` durante o startup do servidor. Se a migration falhar, o servidor não sobe (detectável via healthcheck). **Nunca** rode `catalog-db-migrate-down` em produção sem backup.

---

## Testes

Dois tipos:

| Tipo | Arquivo | Requer DB? | Comando |
|---|---|---|---|
| Unitário | `product_unit_test.go` | ❌ não | `make catalog-test` |
| Integração | `product_test.go` | ✅ sim (com seed) | `make catalog-test` |

Os testes de integração fazem `setupTestDB()` que:
1. Conecta em `CATALOG_DB_URL` (ou default `localhost:5436`).
2. Se o DB não responder → `t.Skip()` (CI sem Postgres passa).
3. Se `products` tiver 0 linhas → `t.Skip()` (pede para rodar `make catalog-db-seed`).

### Rodar

```bash
# pré-requisito: DB rodando e populado
make infra-up
make catalog-db-reset

# rodar tudo
make catalog-test

# ou, direto com go test (mais verbose):
cd services/catalog-service
go test ./... -v
```

### Cobertura atual (16 testes)

**Unitários** (`TestParseProductsQuery_*`):
- Defaults (page=1, per_page=24, filtros vazios)
- Parse completo de todos os params
- Sanitização de bounds (page ≥ 1, per_page ∈ [1, 100])
- `in_stock=true` (literal, não `1`)
- Trim de whitespace em `q`

**Integração** (`Test*` nos handlers):
- `/api/v1/categories` retorna 8 categorias
- `/api/v1/sellers` retorna ≥ 1 vendedor
- `/api/v1/products` respeita paginação (`per_page`, `page`)
- `/api/v1/products?category=X` filtra corretamente
- `/api/v1/products?price_min=X&price_max=Y` respeita range
- `/api/v1/products?q=bosch` retorna ≥ 1 resultado
- `/api/v1/products?sort=price_asc` ordenação monotônica
- `/api/v1/products/:slug` retorna produto com `sellerId`, `sellerRating`, imagens
- `/api/v1/products/:slug` → 404 para slug inexistente
- `/api/v1/products/facets?category=X` retorna marcas + faixa de preço

Testes criam um router Gin isolado via `gin.CreateTestContext` / `gin.New()` + `httptest.NewRecorder`, sem subir o server completo.

---

## Decisões arquiteturais

- **Database-per-service**: `catalog_service` é um DB separado de `payment_service`. Vantagem: cada serviço evolui o schema sem coordenação. Desvantagem: nada de JOINs cross-service — o frontend (ou um BFF futuro) compõe os dados.
- **JSON em camelCase**: os tags Go do `Product` emitem `sellerId`, `reviewCount`, etc, matching 1:1 com `app/src/types/product.ts`. Zero adapter no frontend.
- **Sem auth**: catálogo é público, como qualquer e-commerce. A autenticação entra quando o usuário for operar carrinho/pedido.
- **pg_trgm para busca**: simples e funciona para MVP. Upgrade para `tsvector` + ranking virá na Sprint 17 (`sprint-17-search-upgrade.md`).
- **CORS aberto em dev**: origem `*`. Em produção será restrito ao domínio oficial — responsabilidade do CloudFront + resposta do serviço.

---

## Próximos passos

- **Phase B2** — `order-service`: orders + order_items referenciando `products.id` do catalog.
- **Phase B3** — `auth-service`: users + addresses + JWT.
- **Sprint 10** — onboarding de vendedor: POST/PATCH em `sellers`, approval flow.
- **Sprint 17** — upgrade de busca: `tsvector` + ranking + stemming pt-BR.
