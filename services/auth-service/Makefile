export PATH := $(PATH):$(HOME)/go/bin

.PHONY: run build test tidy

run:
	go run ./cmd/server

build:
	go build -o bin/auth-service ./cmd/server

test:
	go test ./... -short

tidy:
	go mod tidy
