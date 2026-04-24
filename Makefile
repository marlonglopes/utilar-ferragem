APP_DIR      := app
SVC_DIR      := services/payment-service
CATALOG_DIR  := services/catalog-service
ORDER_DIR    := services/order-service
API_URL      ?= http://localhost:8090
CATALOG_URL  ?= http://localhost:8091
ORDER_URL    ?= http://localhost:8092

# ── Postgres (payment-service) ────────────────────────────────────────────────
PG_CONTAINER := utilar_payment_db
PG_USER      := utilar
PG_DB        := payment_service
PG_EXEC      := docker exec -i $(PG_CONTAINER) psql -U $(PG_USER) -d $(PG_DB) -v ON_ERROR_STOP=1
MIGRATIONS   := $(SVC_DIR)/migrations

# ── Postgres (catalog-service) ────────────────────────────────────────────────
CAT_CONTAINER := utilar_catalog_db
CAT_USER      := utilar
CAT_DB        := catalog_service
CAT_EXEC      := docker exec -i $(CAT_CONTAINER) psql -U $(CAT_USER) -d $(CAT_DB) -v ON_ERROR_STOP=1
CAT_MIGRATIONS := $(CATALOG_DIR)/migrations

# ── Postgres (order-service) ──────────────────────────────────────────────────
ORD_CONTAINER := utilar_order_db
ORD_USER      := utilar
ORD_DB        := order_service
ORD_EXEC      := docker exec -i $(ORD_CONTAINER) psql -U $(ORD_USER) -d $(ORD_DB) -v ON_ERROR_STOP=1
ORD_MIGRATIONS := $(ORDER_DIR)/migrations

.PHONY: help dev dev-live dev-full test test-watch infra-up infra-down infra-status \
        svc-run svc-build svc-test install clean \
        db-migrate db-migrate-down db-seed db-clean db-reset db-status db-psql db-dump db-restore \
        catalog-run catalog-build catalog-test \
        catalog-db-migrate catalog-db-migrate-down catalog-db-seed catalog-db-clean catalog-db-reset \
        catalog-db-status catalog-db-psql catalog-db-dump catalog-db-restore \
        order-run order-build order-test \
        order-db-migrate order-db-migrate-down order-db-seed order-db-clean order-db-reset \
        order-db-status order-db-psql order-db-dump order-db-restore

# ── default ──────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  Utilar Ferragem — comandos disponíveis"
	@echo ""
	@echo "  Desenvolvimento — mock mode (sem backend)"
	@echo "    make dev          SPA isolada (pagamento/pedidos mockados)"
	@echo "    make install      npm install na app"
	@echo ""
	@echo "  Desenvolvimento — live mode (backend real)"
	@echo "    make dev-full     tudo junto: infra + payment-service + catalog-service + SPA"
	@echo "    make dev-catalog  infra + catalog-service + SPA (catálogo live, pagamento mock)"
	@echo "    make dev-live     só a SPA apontando para \$$API_URL ($(API_URL)) e \$$CATALOG_URL ($(CATALOG_URL))"
	@echo ""
	@echo "  Testes"
	@echo "    make test         roda todos os testes (uma vez)"
	@echo "    make test-watch   modo watch (re-roda ao salvar)"
	@echo ""
	@echo "  Infraestrutura (Docker)"
	@echo "    make infra-up     Postgres + Redpanda + Console"
	@echo "    make infra-down   para e remove containers"
	@echo "    make infra-status estado dos containers"
	@echo ""
	@echo "  Backend Go (payment-service)"
	@echo "    make svc-run      roda o servidor Go (requer infra-up)"
	@echo "    make svc-build    compila binário"
	@echo "    make svc-test     testes unitários Go"
	@echo ""
	@echo "  Backend Go (catalog-service)"
	@echo "    make catalog-run       roda catalog-service em :8091"
	@echo "    make catalog-build     compila binário"
	@echo "    make catalog-test      testes unitários Go"
	@echo ""
	@echo "  Backend Go (order-service)"
	@echo "    make order-run         roda order-service em :8092"
	@echo "    make order-build       compila binário"
	@echo "    make order-test        testes unitários + integração"
	@echo ""
	@echo "  Banco de dados (Postgres order_service)"
	@echo "    make order-db-migrate        aplica migrations"
	@echo "    make order-db-migrate-down   reverte migrations"
	@echo "    make order-db-seed           popula 60 pedidos de teste"
	@echo "    make order-db-clean          TRUNCATE em todas tabelas"
	@echo "    make order-db-reset          drop + migrate + seed"
	@echo "    make order-db-status         lista tabelas + contagem"
	@echo "    make order-db-psql           shell psql interativo"
	@echo "    make order-db-dump           backup para backups/order_<date>.sql"
	@echo "    make order-db-restore FILE=<path>"
	@echo ""
	@echo "  Banco de dados (Postgres catalog_service)"
	@echo "    make catalog-db-migrate       aplica migrations"
	@echo "    make catalog-db-migrate-down  reverte migrations"
	@echo "    make catalog-db-seed          popula tabelas"
	@echo "    make catalog-db-clean         TRUNCATE em todas tabelas"
	@echo "    make catalog-db-reset         drop + migrate + seed"
	@echo "    make catalog-db-status        lista tabelas + contagem"
	@echo "    make catalog-db-psql          shell psql interativo"
	@echo "    make catalog-db-dump          backup para backups/catalog_<date>.sql"
	@echo "    make catalog-db-restore FILE=<path>"
	@echo ""
	@echo "  Banco de dados (Postgres payment_service)"
	@echo "    make db-migrate       aplica migrations (001, 002, 003)"
	@echo "    make db-migrate-down  reverte todas as migrations (drop schema)"
	@echo "    make db-seed          popula tabelas com dados de teste"
	@echo "    make db-clean         TRUNCATE em todas as tabelas"
	@echo "    make db-reset         drop + migrate + seed (limpa geral)"
	@echo "    make db-status        lista tabelas + contagem de linhas"
	@echo "    make db-psql          abre shell psql interativo"
	@echo "    make db-dump          pg_dump para backups/payment_service_<date>.sql"
	@echo "    make db-restore FILE=<path>  restaura a partir de dump"
	@echo ""
	@echo "  make clean          remove dist/ e coverage/"
	@echo ""

# ── frontend ─────────────────────────────────────────────────────────────────
install:
	cd $(APP_DIR) && npm install

dev:
	cd $(APP_DIR) && npm run dev

dev-live:
	@echo "SPA em live mode → VITE_API_URL=$(API_URL), VITE_CATALOG_URL=$(CATALOG_URL), VITE_ORDER_URL=$(ORDER_URL)"
	@echo "(lembre: make infra-up + make svc-run + make catalog-run + make order-run devem estar rodando)"
	cd $(APP_DIR) && VITE_API_URL=$(API_URL) VITE_CATALOG_URL=$(CATALOG_URL) VITE_ORDER_URL=$(ORDER_URL) npm run dev

dev-catalog:
	@$(MAKE) infra-up
	@echo ""
	@echo "→ subindo catalog-service + SPA (apenas catálogo live; pagamentos mockados)"
	@trap 'echo; echo "Encerrando..."; kill $$CAT_PID 2>/dev/null; wait 2>/dev/null; exit 0' INT TERM; \
	$(MAKE) -C $(CATALOG_DIR) run & CAT_PID=$$!; \
	sleep 2; \
	cd $(APP_DIR) && VITE_CATALOG_URL=$(CATALOG_URL) npm run dev; \
	kill $$CAT_PID 2>/dev/null; wait 2>/dev/null

dev-full:
	@$(MAKE) infra-up
	@echo ""
	@echo "→ subindo payment + catalog + order + SPA (Ctrl-C encerra todos)"
	@trap 'echo; echo "Encerrando..."; kill $$SVC_PID $$CAT_PID $$ORD_PID 2>/dev/null; wait 2>/dev/null; exit 0' INT TERM; \
	$(MAKE) -C $(SVC_DIR) run & SVC_PID=$$!; \
	$(MAKE) -C $(CATALOG_DIR) run & CAT_PID=$$!; \
	$(MAKE) -C $(ORDER_DIR) run & ORD_PID=$$!; \
	sleep 2; \
	cd $(APP_DIR) && VITE_API_URL=$(API_URL) VITE_CATALOG_URL=$(CATALOG_URL) VITE_ORDER_URL=$(ORDER_URL) npm run dev; \
	kill $$SVC_PID $$CAT_PID $$ORD_PID 2>/dev/null; wait 2>/dev/null

test:
	cd $(APP_DIR) && npm run test:run

test-watch:
	cd $(APP_DIR) && npm run test

# ── infra ─────────────────────────────────────────────────────────────────────
infra-up:
	docker compose up -d
	@echo "Aguardando Postgres (payment)..."
	@until docker exec utilar_payment_db pg_isready -U utilar -d payment_service 2>/dev/null; do sleep 1; done
	@echo "Aguardando Postgres (catalog)..."
	@until docker exec utilar_catalog_db pg_isready -U utilar -d catalog_service 2>/dev/null; do sleep 1; done
	@echo "Aguardando Postgres (order)..."
	@until docker exec utilar_order_db pg_isready -U utilar -d order_service 2>/dev/null; do sleep 1; done
	@echo "Postgres payment → localhost:5435"
	@echo "Postgres catalog → localhost:5436"
	@echo "Postgres order   → localhost:5437"
	@echo "Redpanda pronto  → localhost:19092"
	@echo "Console pronto   → http://localhost:8085"

infra-down:
	docker compose down

infra-status:
	docker compose ps

# ── backend ───────────────────────────────────────────────────────────────────
svc-run:
	$(MAKE) -C $(SVC_DIR) run

svc-build:
	$(MAKE) -C $(SVC_DIR) build

svc-test:
	$(MAKE) -C $(SVC_DIR) test

# ── banco de dados ───────────────────────────────────────────────────────────
# Todas as tarefas usam docker exec → psql. Requer `make infra-up` ativo.

define _require_pg
	@docker ps --filter "name=$(PG_CONTAINER)" --filter "status=running" --format '{{.Names}}' \
		| grep -q $(PG_CONTAINER) \
		|| (echo "→ Postgres não está rodando. Rode: make infra-up"; exit 1)
endef

db-migrate:
	$(call _require_pg)
	@echo "→ aplicando migrations em $(MIGRATIONS)"
	@$(PG_EXEC) -c "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, dirty BOOLEAN NOT NULL);" > /dev/null
	@for f in $$(ls $(MIGRATIONS)/*.up.sql | sort); do \
		v=$$(basename $$f | cut -d_ -f1 | sed 's/^0*//'); \
		echo "  · $$f (version $$v)"; \
		$(PG_EXEC) < $$f > /dev/null || exit 1; \
		$(PG_EXEC) -c "INSERT INTO schema_migrations (version, dirty) VALUES ($$v, false) ON CONFLICT (version) DO UPDATE SET dirty=false;" > /dev/null; \
	done
	@echo "✓ migrations aplicadas"

db-migrate-down:
	$(call _require_pg)
	@echo "→ revertendo migrations (ordem reversa)"
	@for f in $$(ls $(MIGRATIONS)/*.down.sql | sort -r); do \
		echo "  · $$f"; \
		$(PG_EXEC) < $$f > /dev/null 2>&1 || true; \
	done
	@$(PG_EXEC) -c "DROP TABLE IF EXISTS schema_migrations;" > /dev/null 2>&1 || true
	@echo "✓ schema limpo"

db-seed:
	$(call _require_pg)
	@echo "→ populando tabelas via seed.sql"
	@$(PG_EXEC) < $(MIGRATIONS)/seed.sql
	@echo "✓ seed aplicado"

db-clean:
	$(call _require_pg)
	@echo "→ TRUNCATE em todas as tabelas"
	@$(PG_EXEC) -c "TRUNCATE TABLE payments_outbox, webhook_events, payments RESTART IDENTITY CASCADE;" > /dev/null
	@echo "✓ tabelas vazias"

db-reset: db-migrate-down db-migrate db-seed
	@echo "✓ banco resetado e populado"

db-status:
	$(call _require_pg)
	@$(PG_EXEC) -c "\dt"
	@$(PG_EXEC) -c " \
		SELECT 'payments'        AS table_name, count(*) AS rows FROM payments \
		UNION ALL \
		SELECT 'webhook_events',  count(*) FROM webhook_events \
		UNION ALL \
		SELECT 'payments_outbox', count(*) FROM payments_outbox \
		ORDER BY table_name;"

db-psql:
	$(call _require_pg)
	@docker exec -it $(PG_CONTAINER) psql -U $(PG_USER) -d $(PG_DB)

db-dump:
	$(call _require_pg)
	@mkdir -p backups
	@FILE=backups/payment_service_$$(date +%Y%m%d_%H%M%S).sql; \
	docker exec $(PG_CONTAINER) pg_dump -U $(PG_USER) -d $(PG_DB) --clean --if-exists > $$FILE; \
	echo "✓ dump salvo em $$FILE"

db-restore:
	$(call _require_pg)
	@test -n "$(FILE)" || (echo "uso: make db-restore FILE=backups/payment_service_XXXX.sql"; exit 1)
	@test -f "$(FILE)" || (echo "arquivo não encontrado: $(FILE)"; exit 1)
	@echo "→ restaurando de $(FILE)"
	@$(PG_EXEC) < $(FILE) > /dev/null
	@echo "✓ restaurado"

# ── catalog-service ──────────────────────────────────────────────────────────
catalog-run:
	$(MAKE) -C $(CATALOG_DIR) run

catalog-build:
	$(MAKE) -C $(CATALOG_DIR) build

catalog-test:
	$(MAKE) -C $(CATALOG_DIR) test

define _require_catalog_pg
	@docker ps --filter "name=$(CAT_CONTAINER)" --filter "status=running" --format '{{.Names}}' \
		| grep -q $(CAT_CONTAINER) \
		|| (echo "→ Postgres (catalog) não está rodando. Rode: make infra-up"; exit 1)
endef

catalog-db-migrate:
	$(call _require_catalog_pg)
	@echo "→ aplicando migrations em $(CAT_MIGRATIONS)"
	@$(CAT_EXEC) -c "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, dirty BOOLEAN NOT NULL);" > /dev/null
	@for f in $$(ls $(CAT_MIGRATIONS)/*.up.sql | sort); do \
		v=$$(basename $$f | cut -d_ -f1 | sed 's/^0*//'); \
		echo "  · $$f (version $$v)"; \
		$(CAT_EXEC) < $$f > /dev/null || exit 1; \
		$(CAT_EXEC) -c "INSERT INTO schema_migrations (version, dirty) VALUES ($$v, false) ON CONFLICT (version) DO UPDATE SET dirty=false;" > /dev/null; \
	done
	@echo "✓ migrations aplicadas"

catalog-db-migrate-down:
	$(call _require_catalog_pg)
	@echo "→ revertendo migrations (ordem reversa)"
	@for f in $$(ls $(CAT_MIGRATIONS)/*.down.sql | sort -r); do \
		echo "  · $$f"; \
		$(CAT_EXEC) < $$f > /dev/null 2>&1 || true; \
	done
	@$(CAT_EXEC) -c "DROP TABLE IF EXISTS schema_migrations;" > /dev/null 2>&1 || true
	@echo "✓ schema limpo"

catalog-db-seed:
	$(call _require_catalog_pg)
	@echo "→ populando tabelas via seed.sql"
	@$(CAT_EXEC) < $(CAT_MIGRATIONS)/seed.sql
	@echo "✓ seed aplicado"

catalog-db-clean:
	$(call _require_catalog_pg)
	@echo "→ TRUNCATE em todas as tabelas"
	@$(CAT_EXEC) -c "TRUNCATE TABLE product_images, products, sellers, categories RESTART IDENTITY CASCADE;" > /dev/null
	@echo "✓ tabelas vazias"

catalog-db-reset: catalog-db-migrate-down catalog-db-migrate catalog-db-seed
	@echo "✓ catalog resetado e populado"

catalog-db-status:
	$(call _require_catalog_pg)
	@$(CAT_EXEC) -c "\dt"
	@$(CAT_EXEC) -c " \
		SELECT 'categories'     AS table_name, count(*) AS rows FROM categories \
		UNION ALL \
		SELECT 'sellers',        count(*) FROM sellers \
		UNION ALL \
		SELECT 'products',       count(*) FROM products \
		UNION ALL \
		SELECT 'product_images', count(*) FROM product_images \
		ORDER BY table_name;"

catalog-db-psql:
	$(call _require_catalog_pg)
	@docker exec -it $(CAT_CONTAINER) psql -U $(CAT_USER) -d $(CAT_DB)

catalog-db-dump:
	$(call _require_catalog_pg)
	@mkdir -p backups
	@FILE=backups/catalog_$$(date +%Y%m%d_%H%M%S).sql; \
	docker exec $(CAT_CONTAINER) pg_dump -U $(CAT_USER) -d $(CAT_DB) --clean --if-exists > $$FILE; \
	echo "✓ dump salvo em $$FILE"

catalog-db-restore:
	$(call _require_catalog_pg)
	@test -n "$(FILE)" || (echo "uso: make catalog-db-restore FILE=backups/catalog_XXXX.sql"; exit 1)
	@test -f "$(FILE)" || (echo "arquivo não encontrado: $(FILE)"; exit 1)
	@echo "→ restaurando de $(FILE)"
	@$(CAT_EXEC) < $(FILE) > /dev/null
	@echo "✓ restaurado"

# ── order-service ────────────────────────────────────────────────────────────
order-run:
	$(MAKE) -C $(ORDER_DIR) run

order-build:
	$(MAKE) -C $(ORDER_DIR) build

order-test:
	$(MAKE) -C $(ORDER_DIR) test

define _require_order_pg
	@docker ps --filter "name=$(ORD_CONTAINER)" --filter "status=running" --format '{{.Names}}' \
		| grep -q $(ORD_CONTAINER) \
		|| (echo "→ Postgres (order) não está rodando. Rode: make infra-up"; exit 1)
endef

order-db-migrate:
	$(call _require_order_pg)
	@echo "→ aplicando migrations em $(ORD_MIGRATIONS)"
	@$(ORD_EXEC) -c "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, dirty BOOLEAN NOT NULL);" > /dev/null
	@for f in $$(ls $(ORD_MIGRATIONS)/*.up.sql | sort); do \
		v=$$(basename $$f | cut -d_ -f1 | sed 's/^0*//'); \
		echo "  · $$f (version $$v)"; \
		$(ORD_EXEC) < $$f > /dev/null || exit 1; \
		$(ORD_EXEC) -c "INSERT INTO schema_migrations (version, dirty) VALUES ($$v, false) ON CONFLICT (version) DO UPDATE SET dirty=false;" > /dev/null; \
	done
	@echo "✓ migrations aplicadas"

order-db-migrate-down:
	$(call _require_order_pg)
	@echo "→ revertendo migrations (ordem reversa)"
	@for f in $$(ls $(ORD_MIGRATIONS)/*.down.sql | sort -r); do \
		echo "  · $$f"; \
		$(ORD_EXEC) < $$f > /dev/null 2>&1 || true; \
	done
	@$(ORD_EXEC) -c "DROP TABLE IF EXISTS schema_migrations;" > /dev/null 2>&1 || true
	@echo "✓ schema limpo"

order-db-seed:
	$(call _require_order_pg)
	@echo "→ populando tabelas via seed.sql"
	@$(ORD_EXEC) < $(ORD_MIGRATIONS)/seed.sql
	@echo "✓ seed aplicado"

order-db-clean:
	$(call _require_order_pg)
	@echo "→ TRUNCATE em todas as tabelas"
	@$(ORD_EXEC) -c "TRUNCATE TABLE tracking_events, shipping_addresses, order_items, orders RESTART IDENTITY CASCADE;" > /dev/null
	@echo "✓ tabelas vazias"

order-db-reset: order-db-migrate-down order-db-migrate order-db-seed
	@echo "✓ order resetado e populado"

order-db-status:
	$(call _require_order_pg)
	@$(ORD_EXEC) -c "\dt"
	@$(ORD_EXEC) -c " \
		SELECT 'orders'             AS table_name, count(*) AS rows FROM orders \
		UNION ALL \
		SELECT 'order_items',         count(*) FROM order_items \
		UNION ALL \
		SELECT 'shipping_addresses',  count(*) FROM shipping_addresses \
		UNION ALL \
		SELECT 'tracking_events',     count(*) FROM tracking_events \
		ORDER BY table_name;"

order-db-psql:
	$(call _require_order_pg)
	@docker exec -it $(ORD_CONTAINER) psql -U $(ORD_USER) -d $(ORD_DB)

order-db-dump:
	$(call _require_order_pg)
	@mkdir -p backups
	@FILE=backups/order_$$(date +%Y%m%d_%H%M%S).sql; \
	docker exec $(ORD_CONTAINER) pg_dump -U $(ORD_USER) -d $(ORD_DB) --clean --if-exists > $$FILE; \
	echo "✓ dump salvo em $$FILE"

order-db-restore:
	$(call _require_order_pg)
	@test -n "$(FILE)" || (echo "uso: make order-db-restore FILE=backups/order_XXXX.sql"; exit 1)
	@test -f "$(FILE)" || (echo "arquivo não encontrado: $(FILE)"; exit 1)
	@echo "→ restaurando de $(FILE)"
	@$(ORD_EXEC) < $(FILE) > /dev/null
	@echo "✓ restaurado"

# ── util ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf $(APP_DIR)/dist $(APP_DIR)/coverage
