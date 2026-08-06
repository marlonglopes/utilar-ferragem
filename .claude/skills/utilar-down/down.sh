#!/usr/bin/env bash
# Encerra a stack local do Utilar: SPA (Vite :5175) + 5 serviços Go (8090-8094).
#
# Mata por PORTA (robusto: não depende de PID salvo) e derruba o GRUPO de
# processo inteiro — pega o `go run` e o binário-filho que ele compila. Os bancos
# (Docker) ficam DE PÉ por padrão; use `--infra` para derrubá-los também.
#
# Uso:
#   down.sh            → serviços + SPA (mantém Docker/DBs)
#   down.sh --infra    → também `docker compose down` (para Postgres/Redis/Redpanda)
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WITH_INFRA=0
[[ "${1:-}" == "--infra" ]] && WITH_INFRA=1

c_green="\033[32m"; c_dim="\033[2m"; c_rst="\033[0m"; c_bold="\033[1m"
say() { printf "%b\n" "$*"; }

# Mata quem estiver escutando na porta, junto do grupo de processo (go run + filho).
kill_port() {
  local label="$1" port="$2"
  local pids; pids="$(lsof -ti tcp:"$port" -s tcp:LISTEN 2>/dev/null || true)"
  if [[ -z "$pids" ]]; then
    say "  ${c_dim}• $label (:$port) já parado${c_rst}"
    return 0
  fi
  for pid in $pids; do
    # Grupo de processo do listener → derruba go run + binário compilado.
    local pgid; pgid="$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ')"
    if [[ -n "$pgid" ]]; then
      kill -TERM -- "-$pgid" 2>/dev/null || true
    else
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done
  # Dá um tempo pro TERM; se insistir, SIGKILL.
  sleep 1
  pids="$(lsof -ti tcp:"$port" -s tcp:LISTEN 2>/dev/null || true)"
  if [[ -n "$pids" ]]; then
    for pid in $pids; do
      local pgid; pgid="$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ')"
      [[ -n "$pgid" ]] && kill -KILL -- "-$pgid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
    done
    sleep 1
  fi
  if lsof -ti tcp:"$port" -s tcp:LISTEN >/dev/null 2>&1; then
    say "  ⚠️  $label (:$port) ainda de pé — verifique manualmente"
  else
    say "  ${c_green}✓ $label parado (:$port)${c_rst}"
  fi
}

say "${c_bold}Encerrando a stack local do Utilar…${c_rst}"

# SPA primeiro (evita erros de proxy enquanto os serviços caem).
kill_port "SPA (Vite)" 5175
kill_port "payment"    8090
kill_port "catalog"    8091
kill_port "order"      8092
kill_port "auth"       8093
kill_port "assistant"  8094

if (( WITH_INFRA )); then
  say "\n${c_bold}Derrubando a infra (Docker)…${c_rst}"
  if make -C "$REPO" infra-down >/tmp/utilar-dev/infra-down.log 2>&1; then
    say "  ${c_green}✓ Docker parado (Postgres/Redis/Redpanda)${c_rst}"
  else
    say "  ⚠️  infra-down falhou — ver /tmp/utilar-dev/infra-down.log"
  fi
else
  say "\n${c_dim}Infra (Docker/DBs) mantida de pé. Para parar também: down.sh --infra${c_rst}"
fi

say "\n${c_green}✅ Encerrado.${c_rst} Subir de novo: skill ${c_bold}utilar-up${c_rst}."
