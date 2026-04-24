APP_DIR   := app
SVC_DIR   := services/payment-service

.PHONY: help dev test test-watch infra-up infra-down infra-status \
        svc-run svc-build svc-test install clean

# ── default ──────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  Utilar Ferragem — comandos disponíveis"
	@echo ""
	@echo "  Desenvolvimento"
	@echo "    make dev          SPA em modo mock (http://localhost:5175)"
	@echo "    make install      npm install na app"
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
	@echo "  make clean          remove dist/ e coverage/"
	@echo ""

# ── frontend ─────────────────────────────────────────────────────────────────
install:
	cd $(APP_DIR) && npm install

dev:
	cd $(APP_DIR) && npm run dev

test:
	cd $(APP_DIR) && npm run test:run

test-watch:
	cd $(APP_DIR) && npm run test

# ── infra ─────────────────────────────────────────────────────────────────────
infra-up:
	docker compose up -d
	@echo "Aguardando Postgres..."
	@until docker exec utilar_payment_db pg_isready -U utilar -d payment_service 2>/dev/null; do sleep 1; done
	@echo "Postgres pronto  → localhost:5435"
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

# ── util ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf $(APP_DIR)/dist $(APP_DIR)/coverage
