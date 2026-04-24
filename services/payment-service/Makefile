export PATH := $(PATH):$(HOME)/go/bin

# Carrega .env.local da raiz do repo (gitignored) se existir.
# Formato esperado: KEY=value (um por linha, # pra comentário).
ENV_FILE := ../../.env.local
ifneq (,$(wildcard $(ENV_FILE)))
    include $(ENV_FILE)
    export
endif

.PHONY: run build test test-integration infra-up infra-down tidy

run:
	go run ./cmd/server

build:
	go build -o bin/payment-service ./cmd/server

test:
	go test ./... -short

test-integration:
	go test ./... -run Integration -v

tidy:
	go mod tidy

infra-up:
	cd ../.. && docker compose up -d postgres-payment redpanda
	@echo "Waiting for Postgres..."
	@until docker exec utilar_payment_db pg_isready -U utilar -d payment_service 2>/dev/null; do sleep 1; done
	@echo "DB ready."

infra-down:
	cd ../.. && docker compose down
