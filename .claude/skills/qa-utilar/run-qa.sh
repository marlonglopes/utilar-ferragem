#!/usr/bin/env bash
# qa-utilar — QA PROFUNDA do Utilar (entrada de pré-deploy).
#
# A isolação de banco que antes era feita aqui virou parte da `test-utilar`: ela
# agora roda o catalog E o order contra bancos EFÊMEROS (clone normalizado), com
# o dev intocado. Então esta skill só:
#   1. para os serviços Go rodando (higiene; os testes já são isolados, mas
#      evita ruído de porta/log). Lista o que parou pra você religar;
#   2. delega a pirâmide inteira pra test-utilar/run-tests.sh;
#   3. lembra como ler o resultado.
#
# Uso:  .claude/skills/qa-utilar/run-qa.sh [camada]     (default: all)
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1

echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: preparando ambiente"
echo "════════════════════════════════════════════════════════"

# Para serviços Go rodando (mantém as DBs). Não é mais necessário pra corretude
# (o catalog/order rodam em banco efêmero), mas mantém o ambiente limpo.
stopped=""
for port in 8090 8091 8092 8093 8094; do
  pids="$(lsof -ti tcp:"$port" 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    echo "$pids" | xargs -r kill 2>/dev/null || true
    stopped="$stopped $port"
  fi
done
if [ -n "$stopped" ]; then
  echo "⚠️  serviços parados nas portas:$stopped — religue depois (make dev-full ou individual)."
  sleep 2
fi

echo
echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: rodando a pirâmide (test-utilar, com bancos efêmeros)"
echo "════════════════════════════════════════════════════════"
.claude/skills/test-utilar/run-tests.sh "${1:-}"
rc=$?

echo
echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: como ler o resultado"
echo "════════════════════════════════════════════════════════"
echo "  • Verde em tudo → software são; pode seguir."
echo "  • O catalog e o order rodam em banco EFÊMERO (dev intocado), então uma"
echo "    falha NÃO é mais poluição de estado — é regressão de verdade. Investigue."
echo "  • Débito informado (prettier/gofmt/CVE) não trava — ver o resumo."

exit $rc
