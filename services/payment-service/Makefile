export PATH := $(PATH):$(HOME)/go/bin

.PHONY: run build test test-integration infra-up infra-down tidy

run:
	MP_ACCESS_TOKEN=$$(grep MP_ACCESS_TOKEN ../../.env.local | cut -d= -f2) \
	MP_PUBLIC_KEY=$$(grep MP_PUBLIC_KEY ../../.env.local | cut -d= -f2) \
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
