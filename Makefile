APP_DIR   := app
SVC_DIR   := services/payment-service
API_URL   ?= http://localhost:8090

.PHONY: help dev dev-live dev-full test test-watch infra-up infra-down infra-status \
        svc-run svc-build svc-test install clean

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
	@echo "    make dev-full     tudo junto: infra + payment-service + SPA"
	@echo "    make dev-live     só a SPA apontando para \$$API_URL ($(API_URL))"
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

dev-live:
	@echo "SPA em live mode → VITE_API_URL=$(API_URL)"
	@echo "(lembre: make infra-up + make svc-run devem estar rodando em outros terminais)"
	cd $(APP_DIR) && VITE_API_URL=$(API_URL) npm run dev

dev-full:
	@$(MAKE) infra-up
	@echo ""
	@echo "→ subindo payment-service + SPA (Ctrl-C encerra ambos)"
	@trap 'echo; echo "Encerrando..."; kill $$SVC_PID 2>/dev/null; wait 2>/dev/null; exit 0' INT TERM; \
	$(MAKE) -C $(SVC_DIR) run & SVC_PID=$$!; \
	sleep 2; \
	cd $(APP_DIR) && VITE_API_URL=$(API_URL) npm run dev; \
	kill $$SVC_PID 2>/dev/null; wait 2>/dev/null

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
