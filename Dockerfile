# syntax=docker/dockerfile:1
#
# Dockerfile único para os 4 serviços Go (parametrizado por SERVICE).
# Build do ROOT do repo (o go.work resolve o pkg/ compartilhado).
#
#   docker build --build-arg SERVICE=catalog-service -t utilar/catalog .
#   docker build --build-arg SERVICE=payment-service -t utilar/payment .
#
# Imagem final = distroless static (~sem shell, nonroot). O binário roda com
# WORKDIR=/app pra que o `file://migrations` (relativo) do db.Migrate resolva.

ARG GO_VERSION=1.26.2

FROM golang:${GO_VERSION}-alpine AS build
ARG SERVICE
RUN test -n "$SERVICE" || (echo "ERRO: --build-arg SERVICE=<serviço> é obrigatório" && exit 1)
WORKDIR /src

# Cache de dependências: copia só os arquivos de módulo primeiro.
COPY go.work go.work.sum ./
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY services/auth-service/go.mod services/auth-service/go.sum ./services/auth-service/
COPY services/catalog-service/go.mod services/catalog-service/go.sum ./services/catalog-service/
COPY services/order-service/go.mod services/order-service/go.sum ./services/order-service/
COPY services/payment-service/go.mod services/payment-service/go.sum ./services/payment-service/
RUN go mod download

# Código-fonte + build estático.
COPY pkg/ ./pkg/
COPY services/ ./services/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/server ./services/${SERVICE}/cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
ARG SERVICE
WORKDIR /app
COPY --from=build /out/server /app/server
# Migrations rodam no boot (db.Migrate lê file://migrations relativo ao WORKDIR).
COPY --from=build /src/services/${SERVICE}/migrations /app/migrations
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
