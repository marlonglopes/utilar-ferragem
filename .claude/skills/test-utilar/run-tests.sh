#!/usr/bin/env bash
# Runner unificado de testes do Utilar Ferragem.
#
# Uso: run-tests.sh [alvo]
#   all        (default) backend + frontend unit + e2e
#   backend    os 4 serviços Go (catalog/order/auth/payment)
#   frontend   vitest (unit/component)
#   e2e        Playwright (sobe a SPA em modo mock sozinho)
#   catalog|order|auth|payment   um serviço Go específico
#
# Testes de integração backend precisam do Postgres (make infra-up). Eles
# SKIPam sozinhos se o banco não estiver acessível — o runner avisa.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"
export PATH="$PATH:/usr/local/go/bin"

TARGET="${1:-all}"
declare -a RESULTS
FAIL=0

run() { # nome, comando...
  local name="$1"; shift
  echo ""
  echo "════════════════════════════════════════════════════════"
  echo "▶ $name"
  echo "════════════════════════════════════════════════════════"
  if "$@"; then
    RESULTS+=("✅ $name")
  else
    RESULTS+=("❌ $name")
    FAIL=1
  fi
}

go_svc() { # serviço, dir
  run "backend: $1" bash -c "cd '$2' && go test ./... 2>&1"
}

backend() {
  command -v go >/dev/null 2>&1 || { echo "go não encontrado no PATH"; RESULTS+=("⚠️  backend (go ausente)"); return; }
  if ! (echo > /dev/tcp/localhost/5436) 2>/dev/null; then
    echo "⚠️  Postgres :5436 indisponível — testes de integração vão SKIPar."
    echo "    Rode 'make infra-up' (+ *-db-reset) para cobertura completa."
  fi
  go_svc catalog services/catalog-service
  go_svc order   services/order-service
  go_svc auth    services/auth-service
  go_svc payment services/payment-service
}

frontend() { run "frontend: vitest" bash -c "cd app && npm run test:run 2>&1"; }
e2e()      { run "frontend: e2e (playwright)" bash -c "cd app && npm run test:e2e 2>&1"; }

case "$TARGET" in
  all)      backend; frontend; e2e ;;
  backend)  backend ;;
  frontend|unit) frontend ;;
  e2e)      e2e ;;
  catalog)  go_svc catalog services/catalog-service ;;
  order)    go_svc order services/order-service ;;
  auth)     go_svc auth services/auth-service ;;
  payment)  go_svc payment services/payment-service ;;
  *) echo "alvo desconhecido: $TARGET"; exit 2 ;;
esac

echo ""
echo "════════════════════════════════════════════════════════"
echo "RESUMO"
echo "════════════════════════════════════════════════════════"
for r in "${RESULTS[@]}"; do echo "  $r"; done
echo ""
[ "$FAIL" -eq 0 ] && echo "✅ Tudo verde." || echo "❌ Há falhas — veja acima."
exit "$FAIL"
