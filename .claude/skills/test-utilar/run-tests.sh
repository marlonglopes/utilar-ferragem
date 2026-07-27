#!/usr/bin/env bash
# Runner unificado de testes do Utilar Ferragem.
#
# Uso: run-tests.sh [alvo]
#   all           (default) backend + ingestão + security + quality + frontend + a11y + e2e
#   backend       os 5 serviços Go (catalog/order/auth/payment/assistant), -race
#   frontend      tsc -b + lint + vitest (unit/component)
#   e2e           Playwright (sobe a SPA em modo mock sozinho)
#   a11y          axe-core WCAG 2.1 A/AA nas páginas públicas (Playwright)
#   security      SAST (gosec) + CVE (govulncheck) + npm audit + segredo/DEV_MODE + invariantes
#   pentest       cenários adversariais como teste (webhook forjado, vazamento de custo, papéis)
#   quality       go vet (gate) + gofmt/prettier (débito, informativo) — boas práticas
#   ingest        ingestão de produtos: pacotes Go `ingest` + smoke dos scripts Python
#   appmax        PSP Appmax (contratos v1/v3) + teste live se houver creds de sandbox
#   integrations  cross-service: assistant (Alice), clients payment↔order, service tokens
#   catalog|order|auth|payment|assistant   um serviço Go específico
#
# ISOLAÇÃO DE BANCO (por que existe): os testes de integração conectam no MESMO
# Postgres que um serviço em execução usa. Duas armadilhas já documentadas:
#   • order-service — o painel admin lê os 100 pedidos `paid` mais antigos; com o
#     serviço rodando há dias o banco acumula >100 travados reais e afoga o pedido
#     que o teste insere (TestOverview_PedidosTravados). Regressão? Não. Poluição.
#   • catalog-service — um sweeper de reservas roda a cada 60s no mesmo banco e
#     briga com os testes de concorrência.
# Solução: para o order, rodamos contra um banco EFÊMERO (clona schema+seed do
# order_service, zera a tabela `orders`, dropa no fim). Dados de dev intactos.
# Sem Docker/containers, cai no banco compartilhado e AVISA sobre a flake.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"
export PATH="$PATH:/usr/local/go/bin"
# gosec/govulncheck vivem no GOPATH/bin — o PATH do harness não os inclui.
command -v go >/dev/null 2>&1 && export PATH="$PATH:$(go env GOPATH 2>/dev/null)/bin"
# NB: staticcheck NÃO é usado — o binário deste ambiente está linkado a um loader
# linuxbrew ausente e falha com "no such file". O gate de estática é `go vet`.

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

note() { RESULTS+=("$1"); }

pg_up() { # porta -> 0 se o Postgres responde
  (echo > "/dev/tcp/localhost/$1") 2>/dev/null
}

# ───────────────────────── isolação de banco (order) ─────────────────────────
# Cria um banco efêmero clonando schema+seed do order_service e zerando `orders`.
# Ecoa a DSN em stdout se der certo; string vazia se não for possível isolar.
# O caller é responsável por dropar via ephemeral_order_drop.
EPH_ORDER_DB="order_service_ephtest"
ephemeral_order_up() {
  command -v docker >/dev/null 2>&1 || return 1
  docker ps --format '{{.Names}}' 2>/dev/null | grep -qx utilar_order_db || return 1
  docker exec utilar_order_db psql -U utilar -d postgres -q \
    -c "DROP DATABASE IF EXISTS ${EPH_ORDER_DB};" \
    -c "CREATE DATABASE ${EPH_ORDER_DB};" >/dev/null 2>&1 || return 1
  # pg_dump tira snapshot consistente mesmo com o serviço conectado.
  docker exec utilar_order_db bash -c \
    "pg_dump -U utilar order_service | psql -U utilar -d ${EPH_ORDER_DB} -q" >/dev/null 2>&1 || return 1
  # Zera os pedidos (CASCADE limpa a árvore transacional); shipping_rates/zones,
  # que não referenciam orders, sobrevivem — os testes de frete dependem deles.
  docker exec utilar_order_db psql -U utilar -d ${EPH_ORDER_DB} -q \
    -c "TRUNCATE orders CASCADE;" >/dev/null 2>&1 || return 1
  echo "postgres://utilar:utilar@localhost:5437/${EPH_ORDER_DB}?sslmode=disable"
}
ephemeral_order_drop() {
  command -v docker >/dev/null 2>&1 || return 0
  docker exec utilar_order_db psql -U utilar -d postgres -q \
    -c "DROP DATABASE IF EXISTS ${EPH_ORDER_DB};" >/dev/null 2>&1 || true
}

# ───────────────────────────────── backend ───────────────────────────────────
go_svc() { # serviço, dir
  run "backend: $1 (-race)" bash -c "cd '$2' && go test ./... -race 2>&1"
}

go_order() {
  local dsn; dsn="$(ephemeral_order_up || true)"
  if [ -n "$dsn" ]; then
    echo "→ order: banco efêmero isolado (${EPH_ORDER_DB}); dados de dev intactos."
    run "backend: order (-race, isolado)" \
      bash -c "cd services/order-service && ORDER_DB_URL='$dsn' go test ./... -race 2>&1"
    ephemeral_order_drop
  else
    echo "⚠️  Sem Docker/container utilar_order_db — rodando no banco compartilhado."
    echo "    TestOverview_PedidosTravados pode FLAKAR se houver >100 pedidos pagos"
    echo "    travados acumulados (não é regressão; ver cabeçalho deste script)."
    go_svc order services/order-service
  fi
}

backend() {
  command -v go >/dev/null 2>&1 || { echo "go ausente no PATH"; note "⚠️  backend (go ausente)"; return; }
  if ! pg_up 5436; then
    echo "⚠️  Postgres :5436 indisponível — testes de integração vão SKIPar."
    echo "    Rode 'make infra-up' (+ *-db-reset) para cobertura completa."
  fi
  go_svc catalog   services/catalog-service
  go_order
  go_svc auth      services/auth-service
  go_svc payment   services/payment-service
  go_svc assistant services/assistant-service
}

# ───────────────────────────────── frontend ──────────────────────────────────
# Receita do CLAUDE.md: tsc -b (o --noEmit NÃO checa nada aqui) + lint + vitest.
frontend() {
  run "frontend: tsc -b"  bash -c "cd app && npx tsc -b 2>&1"
  run "frontend: lint"    bash -c "cd app && npm run lint 2>&1"
  run "frontend: vitest"  bash -c "cd app && npm run test:run 2>&1"
}
e2e() { run "frontend: e2e (playwright)" bash -c "cd app && npm run test:e2e 2>&1"; }

# ──────────────────────────────── ingestão ───────────────────────────────────
# Importação de produtos: publica como `draft`, nunca apaga por ausência (vira
# `archived`), segura queda de preço acima do limite. A lógica testável está em
# Go; os scripts Python de curadoria/imagens não têm suíte — checamos só sintaxe.
ingest() {
  run "ingest: catalog (Go)"   bash -c "cd services/catalog-service   && go test ./internal/ingest/... -race 2>&1"
  run "ingest: assistant (Go)" bash -c "cd services/assistant-service && go test ./internal/ingest/... -race 2>&1"
  if command -v python3 >/dev/null 2>&1 && [ -d scripts/ingestao ]; then
    run "ingest: scripts Python (smoke de sintaxe)" \
      bash -c "python3 -m py_compile scripts/ingestao/*.py && echo 'py_compile OK — sem suíte funcional Python'"
  else
    note "⚠️  ingest: python3/scripts ausentes — smoke pulado"
  fi
}

# ───────────────────────────────── appmax ────────────────────────────────────
# Duas APIs distintas da Appmax: v3 (admin) e v1 (AppStore, com Payment Split).
# Contratos e redação de PII são testados offline. O teste LIVE contra o sandbox
# só roda se as credenciais estiverem no ambiente — senão, é pulado (não falha).
appmax() {
  run "appmax: PSP v3 (contrato)" bash -c "cd services/payment-service && go test ./internal/psp/appmax/...   -race 2>&1"
  run "appmax: PSP v1 (contrato)" bash -c "cd services/payment-service && go test ./internal/psp/appmaxv1/... -race 2>&1"
  if [ -n "${APPMAX_CLIENT_ID:-}${APPMAX_ACCESS_TOKEN:-}${APPMAX_API_KEY:-}" ]; then
    run "appmax: live sandbox" bash -c "cd services/payment-service && APPMAX_LIVE=1 go test ./internal/psp/appmaxv1/... -run Live -race 2>&1"
  else
    note "⚠️  appmax: sem creds de sandbox no ambiente — teste LIVE pulado (defina APPMAX_ACCESS_TOKEN p/ rodar)"
  fi
}

# ──────────────────────────────── integrações ────────────────────────────────
# Comunicação entre serviços: Alice consulta o catálogo real; payment fala com
# order (paymentclient/orderclient); identidade de serviço (role=service) só vale
# assinada com o SERVICE_JWT_SECRET.
integrations() {
  run "integr: assistant/Alice" bash -c "cd services/assistant-service && go test ./... -race 2>&1"
  run "integr: payment↔order"   bash -c "cd services/payment-service   && go test ./internal/orderclient/... -race 2>&1"
  run "integr: order↔payment"   bash -c "cd services/order-service     && go test ./internal/paymentclient/... -race 2>&1"
  run "integr: service tokens"  bash -c "go test ./pkg/servicetoken/... -race 2>&1"
}

# ───────────────────────────── acessibilidade ────────────────────────────────
# axe-core (WCAG 2.1 A/AA) nas páginas públicas, contra a SPA em modo mock. É o
# piso legal do varejo (Lei 13.146/2015). Roda só no chromium — a11y é do DOM,
# não do viewport; duplicar no mobile só dobra o tempo sem achar violação nova.
a11y() {
  command -v npx >/dev/null 2>&1 || { note "⚠️  a11y (npx ausente)"; return; }
  run "a11y: axe WCAG A/AA (playwright)" \
    bash -c "cd app && npx playwright test a11y --project=chromium 2>&1"
}

# ─────────────────────────── invariantes de segurança ────────────────────────
# O caminho do dinheiro e a autorização, encodados como teste — a "suíte de
# pentest" do projeto. Cada modo de ataque conhecido vira um teste nomeado.
# (Precisam do Postgres pros que tocam DB; sem banco, esses SKIPam.)
security_invariants() {
  run "sec-inv: cliente NUNCA vê custo (Alice)" \
    bash -c "cd services/assistant-service && go test ./internal/alice/... -run 'Custo|Vaza|Redig' -race 2>&1"
  run "sec-inv: papel — seller≠balcão, token sem teto de desconto" \
    bash -c "cd services/auth-service && go test ./internal/auth/... ./internal/handler/... -run 'Regression|Role|Store|Internal' -race 2>&1"
  run "sec-inv: webhook forjado não confirma pagamento" \
    bash -c "cd services/payment-service && go test ./internal/handler/... ./internal/psp/... -run 'Webhook|Forjad|Amount|Signature|Tamper|Replay' -race 2>&1"
  run "sec-inv: custo nunca vaza na API pública/balcão" \
    bash -c "cd services/catalog-service && go test ./internal/... -run 'Custo|Cost|NuncaVaza' -race 2>&1"
  run "sec-inv: token de serviço só vale assinado com SERVICE_JWT_SECRET" \
    bash -c "go test ./pkg/servicetoken/... -race 2>&1"
}

# ───────────────────────────────── security ──────────────────────────────────
# SAST + CVE conhecidas + audit de deps + higiene de segredo/DEV_MODE + as
# invariantes acima. Gates de verdade só onde o sinal é limpo:
#   • gosec  -severity high -confidence high  → GATE (ruído baixo)
#   • govulncheck                             → INFORMATIVO (CVE de stdlib/deps
#       se resolve subindo toolchain/versão, não é bug do código)
#   • npm audit --omit=dev --audit-level=high → GATE (só prod, só high+)
GO_MODS=(pkg services/catalog-service services/order-service services/auth-service services/payment-service services/assistant-service)
security() {
  command -v go >/dev/null 2>&1 || { note "⚠️  security (go ausente)"; return; }
  local GB; GB="$(go env GOPATH)/bin"

  if [ -x "$GB/gosec" ]; then
    for m in "${GO_MODS[@]}"; do
      run "security: gosec (SAST) $m" \
        bash -c "cd '$m' && '$GB/gosec' -severity high -confidence high -quiet ./... 2>&1"
    done
  else
    note "⚠️  security: gosec ausente — SAST pulado"
  fi

  if [ -x "$GB/govulncheck" ]; then
    for m in pkg services/payment-service services/order-service; do
      local v; v="$(cd "$m" && "$GB/govulncheck" ./... 2>&1 | grep -c '^Vulnerability #' || true)"
      note "ℹ️  security: govulncheck $m → ${v:-?} CVE conhecida(s) (stdlib/deps → subir versão)"
    done
  else
    note "⚠️  security: govulncheck ausente — CVE scan pulado"
  fi

  run "security: npm audit (prod, high+)" \
    bash -c "cd app && npm audit --omit=dev --audit-level=high 2>&1 || { echo '↑ high/critical em dep de produção'; exit 1; }"

  # Higiene: nenhum segredo versionado; DEV_MODE desligado em arquivo de produção.
  run "security: sem segredo versionado / DEV_MODE prod off" bash -c '
    fail=0
    if git ls-files | grep -iE "\.env$|\.env\.(local|prod|production)$|\.pem$|\.key$|_rsa$|\.p12$|\.pfx$|(^|/)credentials$"; then
      echo "❌ arquivo de segredo versionado (deveria estar no .gitignore)"; fail=1; fi
    if git grep -nIE "AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC )?PRIVATE KEY-----" -- ":!*_test.go" ":!*/testdata/*" ":!docs/**" >/dev/null 2>&1; then
      echo "❌ possível credencial hardcoded (AWS key / private key) fora de teste"; fail=1; fi
    if grep -riE "DEV_MODE[[:space:]]*[:=][[:space:]]*\"?true" docker-compose.prod.yml deploy 2>/dev/null; then
      echo "❌ DEV_MODE ligado em arquivo de produção (bypass de auth via X-User-Role)"; fail=1; fi
    [ $fail -eq 0 ] && echo "ok — sem segredo versionado; DEV_MODE de produção = false"
    exit $fail'

  security_invariants
}

# ───────────────────────────────── pentest ───────────────────────────────────
# Não há DAST ativo (ZAP/burp contra o stack no ar) neste runner — o que temos é
# cada cenário de ataque conhecido virado teste de regressão. Roda sem stack.
pentest() {
  echo "Pentest aqui = cenários adversariais como teste de regressão (não precisa do stack no ar)."
  echo "DAST ativo (varredura contra os serviços rodando) fica fora do escopo deste runner."
  security_invariants
}

# ───────────────────────────── quality / boas práticas ───────────────────────
# go vet é o gate de estática (limpo). gofmt/prettier são DÉBITO pré-existente
# (dezenas de arquivos) — informamos a contagem, não travamos. eslint já é gate
# na camada frontend (--max-warnings 0). staticcheck: ver nota no topo do script.
quality() {
  command -v go >/dev/null 2>&1 || { note "⚠️  quality (go ausente)"; return; }
  for m in "${GO_MODS[@]}"; do
    run "quality: go vet $m" bash -c "cd '$m' && go vet ./... 2>&1"
  done
  local unfmt; unfmt="$(gofmt -l pkg services 2>/dev/null | grep -c . || true)"
  note "ℹ️  quality: gofmt — ${unfmt:-?} arquivo(s) Go fora do padrão (débito; gofmt -w para sanar)"
  if command -v npx >/dev/null 2>&1; then
    local pf; pf="$(cd app && npx prettier --check . 2>&1 | grep -cE '^\[warn\]' || true)"
    note "ℹ️  quality: prettier — ${pf:-?} arquivo(s) front fora do padrão (débito; npm run format)"
  fi
}

case "$TARGET" in
  all)          backend; ingest; security; quality; frontend; a11y; e2e ;;
  backend)      backend ;;
  frontend|unit) frontend ;;
  e2e)          e2e ;;
  a11y|acessibilidade) a11y ;;
  security|seguranca) security ;;
  pentest)      pentest ;;
  quality|qualidade|lint) quality ;;
  ingest|ingestao) ingest ;;
  appmax)       appmax ;;
  integrations|integr) integrations ;;
  catalog)      go_svc catalog services/catalog-service ;;
  order)        go_order ;;
  auth)         go_svc auth services/auth-service ;;
  payment)      go_svc payment services/payment-service ;;
  assistant)    go_svc assistant services/assistant-service ;;
  *) echo "alvo desconhecido: $TARGET"; echo "veja o cabeçalho do script para os alvos válidos"; exit 2 ;;
esac

echo ""
echo "════════════════════════════════════════════════════════"
echo "RESUMO"
echo "════════════════════════════════════════════════════════"
for r in "${RESULTS[@]}"; do echo "  $r"; done
echo ""
[ "$FAIL" -eq 0 ] && echo "✅ Tudo verde." || echo "❌ Há falhas — veja acima."
exit "$FAIL"
