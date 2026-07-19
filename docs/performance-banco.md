# Performance de banco — investigação, medições e correções

**Data:** 2026-07-19
**Escopo:** os 4 Postgres da Utilar (catalog, order, auth, payment)
**Método:** medição com `EXPLAIN (ANALYZE, BUFFERS)` em **volume realista**, não nos ~400 produtos de dev.

---

## Por que o volume importa (e por que dev não serve pra concluir nada)

Com 400 produtos o planejador escolhe **seq scan mesmo quando existe o índice certo** — ler a tabela
inteira é mais barato que descer a árvore. Qualquer conclusão tirada da base de dev é ruído.

Todas as medições abaixo foram feitas em bancos `perf_lab` criados **a partir do schema real**
(`pg_dump --schema-only` do banco de dev, com todos os índices, constraints e triggers), populados com:

| Base | Volume |
|---|---|
| catalog | **150.000 produtos** (92% `published`), 300.000 imagens, **10.000.000 de linhas de auditoria** |
| order | **208.000 pedidos**, 600.000 itens, 400.000 eventos de tracking, 150.000 endereços |
| payment | **300.000 lançamentos contábeis**, 600.000 partidas, 2 anos de datas |

Os `perf_lab` foram removidos ao fim da investigação. Para recriar, ver "Como reproduzir" no fim.

> ⚠️ Uma armadilha encontrada na própria geração de dados: `generate_series(...) , LATERAL (SELECT now() - random()*...)`
> avalia o `random()` **uma vez só** e grava a mesma data em todas as linhas. A primeira medição de
> janela temporal do contábil foi feita em cima disso e era inválida. Datas agora são espalhadas por
> aritmética sobre `g`. Se for reproduzir, confira `min/max/count` antes de confiar em qualquer número.

---

## Os 5 maiores gargalos, em ordem de impacto

### 1. 🔴 Listagem de produtos sem índice de ordenação — **33,9 ms → 0,32 ms (106x)**

`ORDER BY created_at DESC` é o default de **toda** listagem pública (`product.go`, `orderBy` cai em
`p.created_at DESC` quando `sort` é vazio ou `newest`). Não havia **nenhum** índice servindo essa ordenação.
Existiam `idx_products_status`, `idx_products_category`, `idx_products_price` — nenhum com `created_at`.

**Listagem default (home / `LIMIT 24`), 150k produtos:**

| | Plano | Tempo | Buffers |
|---|---|---|---|
| Antes | `Parallel Seq Scan` + top-N heapsort de **138.000 linhas** | 33,9 ms | 6.110 |
| Depois | `Index Scan using idx_products_published_created` | **0,32 ms** | **67** |

**Listagem por categoria:**

| | Plano | Tempo | Buffers |
|---|---|---|---|
| Antes | `Bitmap Heap Scan` de 17.250 linhas + top-N heapsort | 20,2 ms | 5.949 |
| Depois | `Index Scan using idx_products_published_cat_created` | **0,34 ms** | **65** |

O custo do plano antigo é proporcional ao **catálogo inteiro**, não ao que a página devolve: para
entregar 24 produtos ele ordenava 138 mil linhas. Dobrar o catálogo dobra o custo de cada visita à home.

✅ **Corrigido** — `services/catalog-service/migrations/013_perf_listing_indexes.{up,down}.sql`

Os dois índices são **parciais em `status='published'`** (filtro fixo da rota pública), o que os deixa
~8% menores sem penalizar o admin, que lista por outros status e continua em `idx_products_status`.
O índice de categoria **não** torna o primeiro redundante: a home lista sem filtro de categoria, e aí
`(category_id, created_at)` não pode ordenar — a primeira coluna não está no `WHERE`.

---

### 2. 🔴 N+1 na listagem de pedidos — **4 consultas por pedido → 4 fixas**

`loadOrder` (order.go) faz **4 consultas por pedido**: cabeçalho, itens, endereço, tracking.
`List` buscava só os ids e chamava `loadOrder` **num laço**:

| Endpoint | per_page | Consultas ANTES | Consultas DEPOIS |
|---|---|---|---|
| `GET /orders` (default) | 20 | 1 + **4×20** + 1 = **82** | **6** |
| `GET /orders` (teto) | 100 | 1 + **4×100** + 1 = **402** | **6** |
| Fila de aprovação do balcão | 50 | 1 + **4×50** = **201** | **5** |

A fila de aprovação é a pior das três na prática: é tela que o gerente deixa recarregando, e 200 idas
ao banco por refresh competem com a venda pelo pool de conexões — que agora é de 10 por serviço (item 4).

✅ **Corrigido** — `loadOrders(ids)` em `order.go`, com `WHERE ... = ANY($1)` nas 4 consultas e remontagem
na ordem dos ids. Rewired em `order.go` (`List`) e `balcao.go` (`ListPendingApprovals`).
Mesmo padrão do `loadThumbnails` que já existia no catalog.

**Cobertura de teste** (`order_batchload_test.go`) — os dois modos de falha do carregamento em lote
não geram erro, geram **dado errado**, então precisam de teste:

- `TestRegression_ListaEmLoteNaoTrocaItemDePedido` — confere contra o banco que cada item saiu no
  pedido certo. Um erro de agrupamento entregaria ao cliente o item de **outro cliente**.
- `TestRegression_ListaEmLotePreservaOrdemDecrescente` — `= ANY()` não devolve na ordem do array;
  sem a remontagem, "meus pedidos" para de começar pelo mais recente, silenciosamente.
- `TestRegression_ListaEmLoteNaoDevolveItemsNulo` — trava `items: []` (nunca `null`) no JSON.

Os três foram **verificados por mutação**: quebrando o agrupamento de propósito, o primeiro falha com
a mensagem certa. Teste que não falha quando o código quebra não cobre nada.

---

### 3. 🟠 Pool de conexões: 92% da capacidade da instância de produção + `SetConnMaxLifetime` ausente

Dois problemas no mesmo lugar, nos 4 `internal/db/db.go` (que eram **byte a byte idênticos**).

**a) `SetConnMaxLifetime` nunca era chamado** — conexão do pool vivia pra sempre. Contra RDS isso dá
`driver: bad connection` esporádico e sem padrão: o RDS derruba conexão em failover e em troca de
parâmetro, e NAT/firewall no meio matam sessão ociosa sem avisar o cliente. O pool não sabe, entrega o
socket morto pra próxima query, que falha **uma vez** e "some" — o modo de falha mais caro de
diagnosticar, porque não reproduz.

**b) Dimensionamento contra o alvo real de produção.** Segundo `docs/aws-build-utilar.md`, produção é
**UMA instância RDS com os 4 bancos lógicos dentro** — não 4 instâncias como em dev. É esse
compartilhamento que manda no número:

| | Conexões | % da instância |
|---|---|---|
| `db.t3.micro` (1 GB, free tier ano 1) | `max_connections` ≈ **112**, menos 3 reservadas = **109 úteis** | — |
| Config anterior (4 × 25) | **100** | **92%** |
| Config nova (4 × 10) | **40** | 37% |

Sobravam **9 conexões** para migration, `psql`, backup e monitoração. Na primeira rajada simultânea os
4 serviços saturam junto e o Postgres responde `FATAL: sorry, too many clients already` — que não
degrada, **derruba**. Localmente o problema é invisível porque cada serviço tem seu próprio container
com 100 conexões só pra ele.

✅ **Corrigido** nos 4 `internal/db/db.go`: `SetConnMaxLifetime(30m)`, `SetConnMaxIdleTime(5m)`,
`MaxOpenConns` 25 → 10, `MaxIdleConns` 10 → 5. Todos com override por env
(`DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME`) e
*fail-safe*: valor ausente, ilegível ou ≤ 0 cai no default — config errada não pode virar pool de
tamanho zero, que trava o serviço sem erro visível.

10 conexões por serviço continuam folgadas: a conexão é usada só durante a query e o Go **enfileira**
em vez de falhar quando o pool esgota. Se a fila virar gargalo medido, sobe por env, sem rebuild.

---

### 4. 🟠 "Meus pedidos" de cliente pesado — **1,28 ms → 0,12 ms, 398 → 23 buffers**

`SELECT id FROM orders WHERE user_id=$1 ORDER BY created_at DESC` tinha `idx_orders_user_id`, em
`user_id` puro: resolve o filtro, **não** a ordenação.

Com a distribuição achatada do seed (~40 pedidos por usuário) o problema **não aparece** — é preciso um
cliente pesado, que é justamente o caso que dói: construtora/lojista recomprando.

| Usuário com 8.000 pedidos | Plano | Tempo | Buffers |
|---|---|---|---|
| Antes | `Index Scan` em `idx_orders_created_at` varrendo a tabela **inteira** em ordem de data, descartando quem não é o usuário | 1,28 ms | 398 |
| Depois | `Index Scan using idx_orders_user_created` | **0,12 ms** | **23** |

Usuário comum (40 pedidos): 21 buffers, sem regressão.

O número absoluto é pequeno hoje, mas o custo do plano antigo é proporcional ao **total de pedidos da
loja**, não aos do usuário: com 2 milhões de pedidos a mesma consulta lê ~10x mais páginas. Com o
índice, continua lendo ~23.

✅ **Corrigido** — `services/order-service/migrations/005_perf_order_listing_index.{up,down}.sql`

As filas do balcão **já estavam bem servidas** e não precisaram de nada:
`idx_orders_pending_approval` (52 buffers) e `idx_orders_store` (22 buffers), ambos parciais.

---

### 5. 🟡 Busca textual não usa nenhum índice — **6.110 buffers, seq scan**

A busca por `q` faz seq scan em 150k produtos e **nunca** usa os índices trigram que existem
(`idx_products_name_trgm`, `idx_products_sku_trgm`).

A causa é a forma do `OR`: o predicado mistura colunas de **duas tabelas** —

```sql
p.name ILIKE $1 OR p.description ILIKE $1 OR s.name ILIKE $1 OR p.sku ILIKE $2 OR p.barcode = $3
```

Com `s.name` (tabela `sellers`, do JOIN) dentro do mesmo `OR`, o Postgres não consegue satisfazer o
predicado por índice em `products` — teria que provar que nenhum dos outros ramos casa, e o ramo do
seller depende do JOIN. Resultado: varre tudo e filtra.

❌ **NÃO corrigido — precisa de decisão sua.** Ver "Decisões pendentes" abaixo. É a única correção do
levantamento que exige mudar a **semântica** da busca ou o schema, e por isso não entrou.

**O caminho do balcão está OK**: `sku` e `barcode` exatos caem em `Index Scan` (8 buffers, 0,04 ms).
O leitor de código de barras não passa pelo caminho lento.

---

## O que mais foi medido e está saudável (medido, não presumido)

**Trilha de auditoria a 10 milhões de linhas — não degrada.** Foi a medição mais surpreendente:

| Consulta @ 10M linhas | Plano | Tempo | Buffers |
|---|---|---|---|
| Por entidade (`entity`+`entity_id`) | `Index Scan` em `idx_audit_entity` | 0,17 ms | 54 |
| Por ator | `Index Scan` em `idx_audit_actor` | 0,16 ms | 53 |
| Timeline global | `Index Scan` em `idx_audit_time` | ~0,2 ms | 53 |
| `COUNT(*)` por ator (paginação) | `Bitmap Heap Scan` | **5,3 ms** | **2.608** |

Os três índices de auditoria já incluem `created_at DESC` no lugar certo. **Não há nada a corrigir aqui** —
o risco de crescimento é de **disco e paginação**, não de latência de leitura (ver item 6 abaixo).

**Reserva de estoque — já é o formato ótimo.** `UPDATE products SET stock = stock - $2 WHERE id = $1
AND status='published' AND stock >= $2` é update de **linha única por PK**, sem `SELECT ... FOR UPDATE`
separado. A contenção que resta é a inerente ao produto quente (duas vendas do mesmo item **têm** que
serializar) e o `sort.Slice` por `productID` antes do laço já ordena os locks, prevenindo deadlock.
Nenhum índice ajuda aqui. Não mexi.

**Autovacuum está dando conta.** Percentuais altos de `n_dead_tup` aparecem só em tabelas minúsculas
(`import_profiles` 97%, mas são **40 linhas mortas**), que é comportamento normal do threshold
(`50 + 20%`). As tabelas do payment marcadas `last_autovacuum = NUNCA` têm 60–89 tuplas mortas —
estão **abaixo** do gatilho, não esquecidas. Sem bloat real. Defaults servem.

**Dinheiro no contábil já está certo:** `ledger_entries.amount_cents` é `BIGINT`, com trigger de
soma-zero e imutabilidade (`UPDATE` bloqueado, correção só por estorno). Verificado na prática — o
gerador de dados foi rejeitado pelas duas constraints antes de eu acertar os pares débito/crédito.
(O `float64` no Go continua sendo o risco conhecido de `CLAUDE.md`, mas é outro assunto, não de banco.)

---

## 6. Crescimento sem limite — medido, não corrigido (é decisão de negócio + LGPD)

Conforme combinado, **não implementei política de retenção**. Segue a medição.

**`catalog_audit_log` a 10 milhões de linhas: 2.905 MB** — 1.647 MB de tabela + **1.258 MB de índices**
(76% de overhead: 3 índices sobre uma tabela append-only).

Isso é o número que importa, porque `db.t3.micro` do free tier vem com **20 GB de disco**:

| Linhas de auditoria | Tamanho | % dos 20 GB |
|---|---|---|
| 1 M | ~290 MB | 1,5% |
| 10 M | **2,9 GB** | **15%** |
| 50 M | ~14,5 GB | **73%** — e isso é UMA tabela de UM dos 4 bancos |

**Quando dói:** a leitura **não** degrada (medido acima). O que dói primeiro, em ordem:

1. **`COUNT(*)` de paginação** — 5,3 ms / 2.608 buffers a 10M, e cresce **linear**. A ~50M vira ~25 ms
   por página de trilha de auditoria.
2. **Disco** — a 50M de linhas de auditoria o volume do free tier está em 73% com um banco só.
3. **Backup e restore** — o tempo de restauração cresce com o dump. `CLAUDE.md` já lista
   "backup nunca restaurado" como pendência aberta; uma auditoria de 3 GB torna esse teste mais
   demorado e mais urgente.

Tabelas na mesma categoria (só crescem, nunca expurgam): `catalog_audit_log`, `auth_events`,
`balcao_audit_events`, `store_audit_events`, `audit_log` (payment), `webhook_events`,
`import_rows`, `product_price_history`, `ledger_entries`/`ledger_transactions`.

⚠️ Atenção para quando a política for definida: `audit_log` do payment e `catalog_audit_log` são
**append-only com hash encadeado** (garantido por trigger). Apagar linha do meio **quebra a cadeia**.
Expurgo de auditoria tem que ser por corte de prefixo antigo + registro do ponto de corte, não
`DELETE WHERE created_at < x` genérico. O contábil (`ledger_*`) tem exigência legal de retenção
própria — não é decisão técnica.

---

## Decisões pendentes (não mexi, precisa da sua palavra)

**1. Busca textual (gargalo nº 5).** Duas saídas, ambas mudam comportamento:

- **Tirar `s.name` do `OR`** — a busca deixa de casar por nome do fornecedor. É a correção barata e
  destrava os índices trigram existentes. **Muda o que o usuário encontra.**
- **Coluna `tsvector` + índice GIN** (`name || description || sku`), com `websearch_to_tsquery`.
  Busca de verdade, com stemming em português e ranking. Custa uma migration com `GENERATED ALWAYS AS`,
  reescrita do handler e revisão do teste de escape do `ILIKE` (a proteção contra ReDoS do `pg_trgm`,
  audit CT1-C1, deixa de ser necessária nesse caminho — mas não pode simplesmente sumir).

Recomendo a segunda se busca é caminho de receita; a primeira se a prioridade é fechar o gargalo agora.

**2. Índices redundantes por prefixo — não removi (removal é sua decisão).**

- `catalog.idx_price_tiers_product (product_id, min_qty)` é **duplicata exata** de
  `product_price_tiers_product_id_min_qty_key UNIQUE (product_id, min_qty)`. Um dos dois é puro custo
  de escrita e espaço.
- `order.idx_orders_user_id (user_id)` virou **prefixo estrito** de `idx_orders_user_created`
  (criado agora): todo plano que usava o primeiro é atendido pelo segundo.

**Sobre `pg_stat_user_indexes` / `idx_scan = 0`:** rodei nos 4 bancos e o resultado **não serve** como
prova de índice inútil. Nos bancos de dev até **chaves primárias** aparecem com `idx_scan = 0`
(`payments_pkey`, `order_items_pkey`, `tracking_events_pkey`) — o que isso mede é que a base de dev mal
foi exercitada, não que o índice é morto. Sua cautela ("pode ser sazonal") está certa e vai além:
**essa estatística só vale colhida em produção, com o contador acumulado desde o último reset.**
As duas redundâncias acima são estruturais (prova por prefixo), não estatísticas — por isso são as
únicas que reporto.

**3. Batelar o resumo contábil** (`ledger/reports.go`, `Summary`). Roda `accountFlow` **6 vezes**, uma
por conta, cada uma varrendo a mesma janela de `ledger_entries ⋈ ledger_transactions`:

| | Tempo (300k lançamentos, janela de 30 dias) |
|---|---|
| 6 consultas separadas (hoje) | 6 × 36 ms = **216 ms** |
| 1 consulta com `account_code = ANY($1) ... GROUP BY` | **64 ms** (3,4x) |

Não mexi por ser **código do caminho do dinheiro** e endpoint de relatório (não está no caminho da
venda). A mudança é em consulta de leitura, sem alterar lançamento — é segura, mas quero seu aval antes
de tocar em `ledger/`.

**4. Outros N+1 mapeados, fora do caminho quente** (levantados, não corrigidos, por não terem medição
de impacto em produção que justifique o risco agora):

| Local | Custo |
|---|---|
| `ingest/commit.go` `upsertRow` | ~6 round trips **por linha de planilha** (transação por linha é proposital, mas o `SELECT ... WHERE sku=$1` é pré-carregável) — planilha de 4.000 linhas ≈ 24.000 idas |
| `admin_import.go:667` | 1 `UPDATE import_rows` por linha, **fora de transação e com erro ignorado** (`_, _ =`) |
| `admin_import.go` `saveCompositions` | `2×C + Σ itens` execs — dump SINAPI dá dezenas de milhares |
| `reservation.go` `reserveOne` | 3–4 round trips **por item do carrinho**, síncrono dentro do `POST /orders` |
| `reservation/sweeper.go:107` | 1 `UPDATE` por produto liberado, a cada tick de 60s |
| `order.go` `applyAuthoritativePricing` | 1 chamada HTTP ao catalog **por item** do carrinho (o catalog já tem caminho batch com `= ANY($1)`) |

O do `sweeper` responde a uma das perguntas da investigação: **ele não bloqueia a venda**. Os `UPDATE`
são por linha e por PK, e a reserva concorrente disputa a mesma linha por microssegundos — o sweeper
não pega lock de tabela nem varre `products`.

---

## Correções aplicadas (resumo)

| # | Arquivo | O quê |
|---|---|---|
| 1 | `catalog-service/migrations/013_perf_listing_indexes.{up,down}.sql` | 2 índices parciais de listagem — **106x / 59x** |
| 2 | `order-service/migrations/005_perf_order_listing_index.{up,down}.sql` | `(user_id, created_at DESC)` — **10x**, 17x menos I/O |
| 3 | `order-service/internal/handler/order.go` | `loadOrders()` em lote + `List` rewired — **402 → 6** consultas |
| 4 | `order-service/internal/handler/balcao.go` | `ListPendingApprovals` rewired — **201 → 5** consultas |
| 5 | `{catalog,order,auth,payment}-service/internal/db/db.go` | `SetConnMaxLifetime` + pool 25→10, tunável por env |
| 6 | `order-service/internal/handler/order_batchload_test.go` | 3 testes de regressão (verificados por mutação) |

**Validação:** `go build` e `go test -race` verdes **por módulo** nos 4 serviços
(`go build ./services/...` falha por layout de workspace — sempre por módulo).
`TestReserve_Concurrent*`, `TestPublicAPI_NuncaVazaCusto` e `TestList_DevolveCapaDoProduto` passam.
Migrations aplicadas e revertidas nos bancos de dev, round-trip up→down→up verificado,
`schema_migrations.dirty = false` nos dois.

### ⚠️ Nota de produção: `CREATE INDEX CONCURRENTLY`

O golang-migrate roda cada migration **dentro de uma transação**, e `CREATE INDEX CONCURRENTLY` não
pode rodar em transação. As migrations usam `CREATE INDEX` normal, que **trava escrita** na tabela
enquanto constrói (leitura continua). Medido: ~400 ms em 150k produtos, ~500 ms em 208k pedidos.

Se a tabela já estiver grande e a loja não puder parar de vender, aplique à mão **antes** de subir o
serviço — o procedimento completo está comentado no topo de
`services/catalog-service/migrations/013_perf_listing_indexes.up.sql`, incluindo o
`INSERT INTO schema_migrations`. **Sem esse INSERT o migrate tenta criar de novo e falha; e com
`dirty = true` o serviço se recusa a subir.**

### 🐛 Achado incidental: `make catalog-db-migrate` / `make order-db-migrate` só funcionam em banco zerado

O alvo do Makefile faz `for f in *.up.sql` e aplica **todas** as migrations sem consultar quais já
foram aplicadas — em banco existente ele quebra na 001 (`relation "categories" already exists`,
`type "order_status" already exists`). Não é regressão das minhas migrations: falha igual sem elas.
Quem versiona de verdade é o `db.Migrate()` no boot do serviço (golang-migrate, que consulta
`schema_migrations`). **Não corrigi** — é fora do escopo desta investigação e mexe no fluxo de
migration de todo mundo. Fica registrado.

### Schema: duplicata inofensiva

`products` tem **duas CHECK constraints idênticas**: `products_stock_nonneg` e
`products_stock_non_negative`, ambas `CHECK (stock >= 0)`. Custo é uma validação a mais por escrita —
desprezível, mas é resíduo de migration. Não removi (mudança de schema em tabela quente sem ganho medido).

---

## Como reproduzir

Os bancos `perf_lab` foram removidos. Para recriar (exemplo do catalog):

```bash
docker exec utilar_catalog_db psql -U utilar -d postgres -c "CREATE DATABASE perf_lab OWNER utilar;"
docker exec utilar_catalog_db pg_dump -U utilar -d catalog_service --schema-only --no-owner \
  | docker exec -i utilar_catalog_db psql -U utilar -d perf_lab
# popular com generate_series (ver volumes na tabela do topo), depois:
docker exec utilar_catalog_db psql -U utilar -d perf_lab -c "VACUUM ANALYZE;"
```

Sempre `VACUUM ANALYZE` antes de medir — sem estatística atualizada o planejador escolhe errado e a
medição não vale. E confira `min/max/count` das colunas de data antes de confiar em qualquer número
(ver a armadilha do `LATERAL` no topo).
