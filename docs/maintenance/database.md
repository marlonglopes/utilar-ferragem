# Banco de dados — serviços backend (dev)

Guia de manutenção dos bancos Postgres locais dos microserviços. Cobre migrations, seed, reset, backup/restore e comandos do Makefile para os dois serviços em operação.

Para incidentes de produção, ver [../12-ops-runbook.md](../12-ops-runbook.md).

Serviços com DB próprio:

- **[payment-service](#payment-service)** — pagamentos, webhooks, outbox (Sprint 08)
- **[catalog-service](#catalog-service)** — produtos, categorias, vendedores, imagens (Fase B1)

Cada serviço tem seu próprio container Postgres, seguindo o princípio de **database-per-service**: acoplamento fraco, evolução independente de schema.

## Fluxo de migrations (aplicável aos dois serviços)

Cada serviço usa [golang-migrate](https://github.com/golang-migrate/migrate) e segue a mesma convenção:

1. **Numeração sequencial** — `NNN_descricao.up.sql` + `NNN_descricao.down.sql` (ex: `001_create_payments.up.sql`). Próxima migration = próximo número disponível.
2. **Dois sentidos** — todo `up.sql` precisa de um `down.sql` testável (reversibilidade).
3. **Auto-apply no startup** — quando o servidor Go sobe, ele chama `db.Migrate()` que lê a pasta `migrations/` e aplica o que ainda não foi aplicado (verificado via tabela `schema_migrations`).
4. **Apply manual via Makefile** — útil quando o servidor não está rodando. Os targets do Makefile aplicam via `psql` e registram em `schema_migrations` para manter sincronia com o `golang-migrate`.

### Criar uma nova migration (passo a passo)

```bash
# 1. Criar os dois arquivos com o próximo número sequencial
SVC=catalog-service   # ou payment-service
N=002
touch services/$SVC/migrations/${N}_add_pickup_locations.up.sql
touch services/$SVC/migrations/${N}_add_pickup_locations.down.sql

# 2. Escrever DDL no .up.sql e o inverso no .down.sql

# 3. Aplicar localmente
make catalog-db-migrate        # (ou db-migrate para payment)

# 4. TESTAR REVERSIBILIDADE — crítico!
make catalog-db-migrate-down
make catalog-db-migrate

# 5. Ajustar seed.sql se a nova tabela precisa de dados de teste
# 6. Rodar testes Go
make catalog-test

# 7. Commitar os 3 arquivos (up, down, seed) juntos
```

### Boas práticas de DDL

- **Idempotência**: use `CREATE TABLE IF NOT EXISTS`, `DROP ... IF EXISTS`, `ON CONFLICT DO NOTHING`.
- **Zero-downtime em produção**:
  - `ADD COLUMN` sem `DEFAULT` é seguro.
  - `ADD COLUMN NOT NULL DEFAULT` trava tabelas grandes (< Postgres 11). Quebre em 3 migrations: `ADD COLUMN` (null), `UPDATE em batches`, `SET NOT NULL`.
  - `CREATE INDEX CONCURRENTLY` para tabelas grandes (fora de transação).
  - Renomear coluna: expandir (add new) → backfill → dual-write → switch readers → drop old.
- **Nomes descritivos**: `add_products_brand_index`, `alter_sellers_add_cnpj` (não `update_table`, `fix_bug`).
- **Comentários em SQL**: explique o *porquê* dos campos não óbvios, não o *o quê*.

### Rodar migrations em produção

Em produção (AWS RDS), as migrations **não** são aplicadas via Makefile. Fluxo:

1. PR merged → CI builda container com a nova migration.
2. Deploy roda o container → `db.Migrate()` no startup aplica o que falta.
3. Se a migration falhar, o servidor não passa no healthcheck e o deploy é revertido.

**Nunca rode `db-migrate-down` em produção** sem dump/snapshot prévio. Para reverter, faça forward-fix (nova migration que desfaz) — é auditável e seguro.

---

# payment-service

## 1. Topologia

| Item | Valor |
|---|---|
| Container Docker | `utilar_payment_db` |
| Imagem | `postgres:17-alpine` |
| Host / porta | `localhost:5435` |
| Database | `payment_service` |
| User / password | `utilar` / `utilar` |
| Volume persistente | `payment_pg_data` (compose) |
| DSN local | `postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable` |

Sobe junto com Redpanda via `make infra-up` (ver [docker-compose.yml](../../docker-compose.yml)).

---

## 2. Schema

Três tabelas, criadas pelas migrations em [services/payment-service/migrations/](../../services/payment-service/migrations/):

| # | Tabela | Propósito | Índices |
|---|---|---|---|
| 001 | `payments` | pagamentos (pix/boleto/card) × status × valor × metadata PSP | `order_id`, `psp_payment_id` (parcial), `user_id` |
| 002 | `webhook_events` | eventos recebidos do Mercado Pago (idempotência) | unique `(psp_id, psp_payment_id, event_type)` |
| 003 | `payments_outbox` | fila de eventos a publicar no Redpanda | `next_attempt_at` WHERE `published_at IS NULL` |

ENUMs: `payment_method` (`pix`, `boleto`, `card`) e `payment_status` (`pending`, `confirmed`, `failed`, `expired`, `cancelled`).

> **Nota:** `users` e `orders` **não existem** neste DB. Ambos vivem nos mocks do frontend ([app/src/lib/mockOrders.ts](../../app/src/lib/mockOrders.ts), auth em `app/src/contexts/`). O `payment-service` guarda apenas `user_id` e `order_id` como UUIDs opacos.

---

## 3. Migrations

As migrations rodam **automaticamente na inicialização do servidor Go** (ver [internal/db/db.go:26](../../services/payment-service/internal/db/db.go#L26) — `db.Migrate()` chamado em `cmd/server/main.go`). Para o dia a dia de desenvolvimento, use o Makefile — ele dispensa o servidor e também o CLI `migrate`.

### Aplicar / reverter manualmente

```bash
make db-migrate       # aplica todos os *.up.sql em ordem
make db-migrate-down  # reverte todos os *.down.sql em ordem reversa
```

Convenção de nomes: `NNN_descricao.up.sql` + `NNN_descricao.down.sql`. Numeração sequencial de 3 dígitos a partir de `001`.

### Criar uma nova migration

1. Criar dois arquivos com o próximo número:
   ```
   services/payment-service/migrations/004_nova_coisa.up.sql
   services/payment-service/migrations/004_nova_coisa.down.sql
   ```
2. Escrever DDL no `up.sql` e o reverso no `down.sql`.
3. Rodar `make db-migrate` para aplicar localmente.
4. Ajustar o seed, se a nova tabela precisa de dados de teste.
5. Commitar os dois arquivos juntos.

> A biblioteca usada é [golang-migrate](https://github.com/golang-migrate/migrate). Não usamos a tabela `schema_migrations` diretamente — ela é gerenciada pela lib.

---

## 4. Seed

Arquivo único: [services/payment-service/migrations/seed.sql](../../services/payment-service/migrations/seed.sql).

```bash
make db-seed
```

**O que o seed gera:**

| Tabela | Linhas | Distribuição |
|---|---|---|
| `payments` | 150 | 100 UUIDs de user únicos × 150 orders; 50 pix + 50 boleto + 50 card; status ≈ 60% confirmed, 20% pending, 10% expired, 5% failed, 5% cancelled |
| `webhook_events` | 270 | 150 `payment.created` + 120 eventos de estado final (confirmed/failed/expired/cancelled) |
| `payments_outbox` | 110 | 90 publicados (dos confirmed) + 20 pendentes simulando retry backoff com `attempts` de 0 a 4 |

**Propriedades:**

- **Idempotente** — começa com `TRUNCATE ... RESTART IDENTITY CASCADE`, então pode rodar N vezes.
- **Transacional** — todo o seed roda em `BEGIN;...COMMIT;`. Se falhar no meio, nada é persistido.
- **Determinístico** — UUIDs são construídos a partir de `generate_series`, sem randomização. Os mesmos IDs aparecem em toda execução (útil para testes que referenciam `payment_id` específico).
- **Padrão dos UUIDs sintéticos:**
  - users: `00000000-0000-4000-8000-NNNNNNNNNNNN` (N = índice hex)
  - orders: `00000000-0000-4000-9000-NNNNNNNNNNNN`
  - payments: `00000000-0000-4000-a000-NNNNNNNNNNNN`

**Como identificar um registro de seed:** todo row tem `seed: true` em algum campo JSONB (`psp_metadata`, `raw_payload`, `payload_json`). Útil para filtrar em testes ou limpar só os dados de seed sem perder dados criados por handlers.

```sql
-- encontrar só dados de seed:
SELECT count(*) FROM payments WHERE psp_metadata->>'seed' = 'true';
```

---

## 5. Reset completo

```bash
make db-reset
```

Executa, em ordem: `db-migrate-down` → `db-migrate` → `db-seed`. É a forma mais rápida de voltar para um estado limpo e conhecido. Use quando:

- Mudou uma migration e quer reaplicar do zero.
- Dados locais divergiram e você quer começar fresco.
- Antes de rodar testes de integração que assumem estado de seed.

**Diferença vs `db-clean`:** `db-clean` apenas faz `TRUNCATE` e mantém o schema; `db-reset` recria tudo (útil se mudou DDL sem criar uma nova migration durante desenvolvimento).

---

## 6. Backup / restore

```bash
make db-dump                                    # gera backups/payment_service_YYYYMMDD_HHMMSS.sql
make db-restore FILE=backups/payment_service_20260424_120000.sql
```

O `db-dump` usa `pg_dump --clean --if-exists`, então o restore substitui o estado atual.

Gitignore recomendado:
```
backups/
```

Para produção, usar o snapshot do RDS (ver [11-infra.md](../11-infra.md)).

---

## 7. Comandos do Makefile — referência

Todos os alvos verificam se o container está rodando antes de executar; se não estiver, orientam a rodar `make infra-up`.

| Alvo | O que faz | Quando usar |
|---|---|---|
| `make db-migrate` | Aplica todos `*.up.sql` via `psql` | Após criar nova migration |
| `make db-migrate-down` | Reverte `*.down.sql` em ordem reversa | Antes de reset; testar reversibilidade |
| `make db-seed` | Roda `seed.sql` | Popular para dev / testes manuais |
| `make db-clean` | `TRUNCATE` em todas as tabelas | Esvaziar sem perder schema |
| `make db-reset` | down → up → seed | Volta ao estado limpo + populado |
| `make db-status` | Lista tabelas + contagem de linhas | Sanity check rápido |
| `make db-psql` | Abre shell `psql` interativo | Debugging, queries ad-hoc |
| `make db-dump` | `pg_dump` para `backups/<timestamp>.sql` | Antes de operações destrutivas |
| `make db-restore FILE=...` | Restaura a partir de dump | Recuperar estado |

Todos os alvos estão no [Makefile](../../Makefile) raiz.

---

## 8. Troubleshooting

### `ERROR: type "payment_method" already exists`
Migrations já aplicadas antes. Rode `make db-migrate-down` primeiro, ou `make db-reset` para limpar e repopular.

### `Postgres não está rodando`
O target detectou que o container `utilar_payment_db` não está ativo. Rode `make infra-up`.

### Contagem de linhas inesperada após seed
Confirme que estamos no DB certo: `make db-psql` → `\conninfo`. O seed usa `TRUNCATE ... CASCADE`, então se novas tabelas tiverem FK para `payments`, elas serão limpas também.

### Seed travou no meio
Seed é transacional — se deu erro, o banco voltou ao estado anterior. Verifique o output do `psql` acima do "ERROR" para a causa.

### Conectar de ferramenta externa (DBeaver / TablePlus / Postico)
```
Host:     localhost
Port:     5435
Database: payment_service
User:     utilar
Password: utilar
```

---

## 9. Produção

Em produção (AWS RDS), **não** rode `db-reset` nem `db-seed` (oh deus). As migrations sobem no deploy pelo próprio servidor (`db.Migrate()` no startup). Dumps/backups são gerenciados pelo snapshot automático do RDS — ver [11-infra.md](../11-infra.md) e [14-infra-custos.md](../14-infra-custos.md).

---

# catalog-service

## 1. Topologia

| Item | Valor |
|---|---|
| Container Docker | `utilar_catalog_db` |
| Imagem | `postgres:17-alpine` |
| Host / porta | `localhost:5436` |
| Database | `catalog_service` |
| User / password | `utilar` / `utilar` |
| Volume persistente | `catalog_pg_data` (compose) |
| DSN local | `postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable` |
| Servidor HTTP | `http://localhost:8091` |

Sobe via `make infra-up` (junto com payment-service + Redpanda).

## 2. Schema

Quatro tabelas, criadas pela migration em [services/catalog-service/migrations/](../../services/catalog-service/migrations/):

| # | Tabela | Propósito | Índices |
|---|---|---|---|
| 001 | `categories` | taxonomia top-level (8 fixas) + espaço para subcategorias via `parent_id` | PK em `id` (slug), índice em `parent_id` |
| 001 | `sellers` | vendedores (lojas) do marketplace | PK em `id` (slug) |
| 001 | `products` | produtos com atributos comerciais + specs JSONB | `category_id`, `seller_id`, `brand`, `price`, `name` (trigram para busca) |
| 001 | `product_images` | imagens com ordenação via `sort_order` (primeira = hero) | `(product_id, sort_order)` |

ENUM: `product_badge` (`discount`, `free_shipping`, `last_units`).

Extensões: `pg_trgm` (busca por nome).

Triggers: `set_updated_at()` em `products` e `sellers` (mantém `updated_at` automaticamente em UPDATE).

## 3. API HTTP

Read-only e **pública** (sem auth — é catálogo de loja). CORS aberto para desenvolvimento. Endpoints:

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/health` | liveness probe |
| `GET` | `/api/v1/categories` | lista as 8 top-level |
| `GET` | `/api/v1/sellers` | lista vendedores |
| `GET` | `/api/v1/products` | lista com filtros: `category`, `q`, `brand`, `price_min`, `price_max`, `in_stock`, `sort` (`price_asc` / `price_desc` / `newest` / `top_rated`), `page`, `per_page` |
| `GET` | `/api/v1/products/facets` | contagem por marca + faixa de preço (respeita filtros) |
| `GET` | `/api/v1/products/:slug` | detalhe + imagens |

## 4. Seed

Arquivo: [services/catalog-service/migrations/seed.sql](../../services/catalog-service/migrations/seed.sql).

```bash
make catalog-db-seed
```

**Dados gerados:**

| Tabela | Linhas | Origem |
|---|---|---|
| `categories` | 8 | taxonomia fixa (`app/src/lib/taxonomy.ts`) |
| `sellers` | 11 | vendedores únicos extraídos de `app/src/lib/mockProducts.ts` |
| `products` | 111 | 31 reais portados dos mocks + 80 sintéticos (`slug: seed-item-NNN`) com variação de categoria/seller/brand |
| `product_images` | 222 | 2 imagens por produto (placeholders `picsum.photos`) |

**Propriedades:**
- Idempotente (começa com `TRUNCATE ... RESTART IDENTITY CASCADE`).
- Transacional (`BEGIN...COMMIT`).
- Produtos sintéticos têm `specs->>'seed' = 'true'` para filtrar.
- Os 31 produtos reais mantêm paridade visual com a UI (mesmos nomes, preços, specs).

## 5. Comandos do Makefile — catalog

| Alvo | O que faz |
|---|---|
| `make catalog-run` | Roda `catalog-service` em `:8091` (requer DB criado) |
| `make catalog-build` | Compila binário `services/catalog-service/bin/catalog-service` |
| `make catalog-test` | Go tests |
| `make catalog-db-migrate` | Aplica migrations |
| `make catalog-db-migrate-down` | Reverte schema |
| `make catalog-db-seed` | Popula `seed.sql` |
| `make catalog-db-clean` | `TRUNCATE` em todas tabelas |
| `make catalog-db-reset` | down → up → seed |
| `make catalog-db-status` | Lista tabelas + contagem |
| `make catalog-db-psql` | Shell interativo |
| `make catalog-db-dump` | `pg_dump` → `backups/catalog_<timestamp>.sql` |
| `make catalog-db-restore FILE=...` | Restaura dump |

## 6. Quickstart

```bash
# 1. subir infra (Postgres payment + catalog + Redpanda)
make infra-up

# 2. criar schema + popular
make catalog-db-reset

# 3. rodar servidor
make catalog-run

# em outro terminal, smoke test:
curl http://localhost:8091/health
curl 'http://localhost:8091/api/v1/products?category=ferramentas&per_page=5'
curl http://localhost:8091/api/v1/products/furadeira-bosch-gsb-13-re
```

## 7. Adicionar uma nova migration

1. Criar `services/catalog-service/migrations/002_nome.up.sql` + `.down.sql` (numeração sequencial).
2. Escrever DDL no `up.sql` + reverso no `down.sql`.
3. `make catalog-db-migrate` (aplica via `psql` + registra em `schema_migrations`).
4. Atualizar `seed.sql` se necessário.

> **Nota técnica:** o Makefile aplica migrations via `psql` diretamente e registra a versão na tabela `schema_migrations` (esquema do `golang-migrate`). Isso permite que o servidor Go (que também roda `db.Migrate()` no startup) perceba que as migrations já foram aplicadas e não tente reaplicar. Se você editar uma migration existente, rode `make catalog-db-reset` para sincronizar.

## 8. Integração com o frontend ✅ plugada

O SPA já consome o `catalog-service` real quando `VITE_CATALOG_URL` está definido. Sem a var = modo mock (funciona offline, sem backend).

**Como rodar:**
```bash
make dev-catalog    # atalho: infra-up + catalog-run + SPA em live mode
# ou manualmente, em 3 terminais:
make infra-up
make catalog-run
make dev-live       # passa VITE_CATALOG_URL=http://localhost:8091 para o Vite
```

**Arquitetura frontend** (`app/src/`):
- [lib/api.ts](../../app/src/lib/api.ts) — cliente HTTP compartilhado; `catalogGet()` usa `VITE_CATALOG_URL`, `apiGet()` usa `VITE_API_URL` (payment-service)
- [hooks/useProducts.ts](../../app/src/hooks/useProducts.ts), [hooks/useProduct.ts](../../app/src/hooks/useProduct.ts), [hooks/useFacets.ts](../../app/src/hooks/useFacets.ts) — cada hook escolhe live ou mock via `isCatalogEnabled`
- Shape: o Go emite camelCase no JSON (ex: `sellerId`, `reviewCount`) matching direto ao tipo `Product` em `app/src/types/product.ts`; sem camada de adapter.
- Paridade de slugs: os 31 produtos reais do seed usam os mesmos slugs dos mocks (ex: `/produto/furadeira-bosch-gsb-13-re`), então nenhuma URL quebra na transição.

**O que ainda é mock no SPA:** autenticação (qualquer credencial funciona), pedidos (`MOCK_ORDERS` em `app/src/lib/mockOrders.ts`), checkout Pix auto-confirma em 6s. Phase B2 (order-service) resolve pedidos; Phase B3 (auth-service) resolve login.
