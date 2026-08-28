#!/usr/bin/env bash
# Sobe TODA a stack local do Utilar: infra (Docker) + 5 serviços Go + SPA (Vite).
#
# Em segundo plano, com log por serviço em /tmp/utilar-dev/. Idempotente: o que
# já estiver no ar é pulado. Espera cada /health responder antes de seguir.
# Encerrar tudo: a skill utilar-down.
#
# ⚠️ DEV/LOCAL SÓ. Usa DEV_MODE=true e segredos de DESENVOLVIMENTO — o devguard
# se recusa a subir se farejar banco de produção. Nunca use este env fora daqui.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RUN_DIR="/tmp/utilar-dev"
mkdir -p "$RUN_DIR"

# Cores + helpers de saída ANTES de qualquer `say` (o bloco de .env.dev.local
# abaixo já loga com say/c_dim; com `set -u`, referenciá-los antes de definir
# aborta o boot com "c_dim: unbound variable" — só quando .env.dev.local existe,
# por isso passou batido até as chaves da Stripe entrarem).
c_green="\033[32m"; c_red="\033[31m"; c_dim="\033[2m"; c_rst="\033[0m"; c_bold="\033[1m"
say()  { printf "%b\n" "$*"; }
head() { printf "\n%b════════════════════════════════════════════════════════%b\n%b▶ %s%b\n%b════════════════════════════════════════════════════════%b\n" "$c_dim" "$c_rst" "$c_bold" "$1" "$c_rst" "$c_dim" "$c_rst"; }

# Go no PATH (o binário real vive em /usr/local/go/bin; ferramentas em ~/go/bin).
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
export GOTOOLCHAIN=auto

# ── env de DESENVOLVIMENTO compartilhado ──────────────────────────────────────
export DEV_MODE=true
# Declaração POSITIVA de dev: o devguard agora exige APP_ENV=development pra
# aceitar DEV_MODE=true (senão recusa o boot). Fecha o cenário "produção que
# aparenta local" (STRIDE E#5/G2).
export APP_ENV=development
export JWT_SECRET="dev-utilar-jwt-secret-0123456789abcdef"
export SERVICE_JWT_SECRET="dev-utilar-service-secret-0123456789abcdef"
export REDIS_URL="redis://localhost:6379"
export ALLOWED_ORIGINS="http://localhost:5175"
# URLs entre serviços (têm default, mas explicitar evita surpresa).
export AUTH_SERVICE_URL="http://localhost:8093"
export CATALOG_SERVICE_URL="http://localhost:8091"
export ORDER_SERVICE_URL="http://localhost:8092"
export PAYMENT_SERVICE_URL="http://localhost:8090"

# ── segredos de dev (gitignored) ──────────────────────────────────────────────
# .env.dev.local NÃO é versionado (.gitignore cobre .env*). É onde entram chaves
# REAIS de teste (Stripe, Appmax, Anthropic…) sem sujar o git. Ausente → cai nos
# placeholders dummy abaixo (a demo roda em "simular confirmação").
if [ -f "$REPO/.env.dev.local" ]; then
  set -a; . "$REPO/.env.dev.local"; set +a
  say "  ${c_dim}• segredos carregados de .env.dev.local${c_rst}"
fi
# PSP escolhido (troca fácil: PSP_PROVIDER no .env.dev.local ou no ambiente).
# stripe | appmax-v1 | mercadopago. Default: stripe.
: "${PSP_PROVIDER:=stripe}"
: "${STRIPE_SECRET_KEY:=sk_test_dummy_demo_key_do_not_use}"
: "${STRIPE_WEBHOOK_SECRET:=whsec_dummy_demo}"
: "${STRIPE_PUBLISHABLE_KEY:=pk_test_dummy_demo}"

port_up() { curl -sf --max-time 2 "http://localhost:$1/health" >/dev/null 2>&1; }

# Sobe UM serviço se a porta ainda não responde. Argumentos: nome porta dir "env extra"
start_svc() {
  local name="$1" port="$2" dir="$3" extra="${4:-}"
  if port_up "$port"; then
    say "  ${c_dim}• $name já no ar (:$port) — pulando${c_rst}"
    return 0
  fi
  local log="$RUN_DIR/$name.log"
  # setsid = grupo de processo próprio → o utilar-down mata go run + binário filho.
  # shellcheck disable=SC2086
  setsid bash -c "cd '$REPO/$dir' && $extra exec go run ./cmd/server" >"$log" 2>&1 &
  say "  ${c_dim}• $name subindo (:$port) → $log${c_rst}"
}

# Espera /health de uma porta (timeout em segundos). Mostra a cauda do log se falhar.
wait_health() {
  local name="$1" port="$2" timeout="${3:-60}" i=0
  while (( i < timeout )); do
    port_up "$port" && { say "  ${c_green}✓ $name pronto (:$port)${c_rst}"; return 0; }
    sleep 1; ((i++))
  done
  say "  ${c_red}✗ $name NÃO respondeu em ${timeout}s (:$port)${c_rst}"
  say "${c_dim}$(tail -n 15 "$RUN_DIR/$name.log" 2>/dev/null)${c_rst}"
  return 1
}

# ── 1) infra (Postgres x4 + Redis + Redpanda) ────────────────────────────────
head "infra (Docker: Postgres x4, Redis, Redpanda)"
if ! docker info >/dev/null 2>&1; then
  say "  ${c_red}✗ Docker não está rodando. Inicie o Docker e rode de novo.${c_rst}"; exit 1
fi
if ! make -C "$REPO" infra-up >"$RUN_DIR/infra.log" 2>&1; then
  say "  ${c_red}✗ infra-up falhou — ver $RUN_DIR/infra.log${c_rst}"; tail -n 20 "$RUN_DIR/infra.log"; exit 1
fi
say "  ${c_green}✓ infra no ar${c_rst} ${c_dim}(PG 5435-5438, Redis 6379, Redpanda 19092)${c_rst}"

# ── 2) serviços Go ────────────────────────────────────────────────────────────
head "serviços Go (5)"
start_svc auth     8093 services/auth-service
start_svc catalog  8091 services/catalog-service
start_svc payment  8090 services/payment-service \
  "PSP_PROVIDER=$PSP_PROVIDER STRIPE_SECRET_KEY=$STRIPE_SECRET_KEY STRIPE_WEBHOOK_SECRET=$STRIPE_WEBHOOK_SECRET STRIPE_PUBLISHABLE_KEY=$STRIPE_PUBLISHABLE_KEY ${APPMAX_ENV:-}"
start_svc order    8092 services/order-service 'KAFKA_BROKERS=localhost:19092'
# Alice roda em mock sem ANTHROPIC_API_KEY (busca real no catálogo em :8091).
start_svc assistant 8094 services/assistant-service

head "esperando /health"
rc=0
wait_health auth      8093 60 || rc=1
wait_health catalog   8091 90 || rc=1   # catalog roda migrations no boot → mais folga
wait_health payment   8090 60 || rc=1
wait_health order     8092 60 || rc=1
wait_health assistant 8094 60 || rc=1

# ── 3) SPA (Vite) ─────────────────────────────────────────────────────────────
head "SPA (Vite :5175)"
if curl -sf --max-time 2 "http://localhost:5175/" >/dev/null 2>&1; then
  say "  ${c_dim}• SPA já no ar (:5175) — pulando${c_rst}"
else
  # --host 0.0.0.0 permite o tablet do PDV alcançar; --strictPort falha alto se ocupada.
  setsid bash -c "cd '$REPO/app' && exec npx vite --host 0.0.0.0 --port 5175 --strictPort" \
    >"$RUN_DIR/spa.log" 2>&1 &
  say "  ${c_dim}• SPA subindo → $RUN_DIR/spa.log${c_rst}"
  i=0; while (( i < 40 )); do
    curl -sf --max-time 2 "http://localhost:5175/" >/dev/null 2>&1 && break
    sleep 1; ((i++))
  done
  if curl -sf --max-time 2 "http://localhost:5175/" >/dev/null 2>&1; then
    say "  ${c_green}✓ SPA pronta${c_rst}"
  else
    say "  ${c_red}✗ SPA não respondeu — ver $RUN_DIR/spa.log${c_rst}"; tail -n 15 "$RUN_DIR/spa.log"; rc=1
  fi
fi

# ── resumo ────────────────────────────────────────────────────────────────────
head "RESUMO"
say "  Loja       ${c_bold}http://localhost:5175/${c_rst}"
say "  Balcão PDV ${c_bold}http://localhost:5175/balcao${c_rst}"
say "  Admin      ${c_bold}http://localhost:5175/admin${c_rst}   ${c_dim}(admin@utilar.com.br)${c_rst}"
say "  APIs       payment:8090 catalog:8091 order:8092 auth:8093 alice:8094"
say "  Logs       ${c_dim}$RUN_DIR/*.log${c_rst}"
if (( rc == 0 )); then
  say "\n${c_green}✅ Tudo no ar.${c_rst} Encerrar: skill ${c_bold}utilar-down${c_rst}."
else
  say "\n${c_red}⚠️ Algo não subiu — veja os logs acima.${c_rst} Encerrar: skill ${c_bold}utilar-down${c_rst}."
fi
exit $rc
