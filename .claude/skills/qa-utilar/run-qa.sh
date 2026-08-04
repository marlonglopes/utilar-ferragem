#!/usr/bin/env bash
# qa-utilar — QA PROFUNDA e CONFIÁVEL do Utilar.
#
# Envolve a `test-utilar` com o preparo/limpeza de ambiente que a QA manual
# exige, pra o veredito NÃO depender do estado do banco de dev (que diverge
# conforme a loja é limpa pra produção):
#
#   1. Para os serviços Go que estejam rodando (o sweeper de reservas do catalog
#      e o pool de conexões brigam com os testes de integração/concorrência —
#      ver CLAUDE.md). Lista o que parou pra você religar.
#   2. PUBLICA a fixture do catalog (produtos CUR-/UTL-) — os testes de
#      busca/list/capa precisam dela PUBLICADA, e a limpeza pra produção os deixa
#      `archived`. Restaura EXATAMENTE o estado anterior no fim (trap EXIT).
#   3. Roda a pirâmide inteira delegando pra test-utilar/run-tests.sh (backend
#      -race, frontend, e2e, a11y, security, pentest, quality, ingest, appmax,
#      integrations). O order já é isolado lá dentro (banco efêmero).
#   4. Restaura a fixture e imprime como interpretar o resultado.
#
# Uso:  .claude/skills/qa-utilar/run-qa.sh [camada]     (default: all)
#
# ⚠️ Caveat conhecido (não é bug): TestList_DevolveCapaDoProduto verifica que a
# PÁGINA 1 da lista admin tem capa. Com o catálogo real (milhares de produtos
# ainda sem foto) dominando a página 1, ele falha mesmo com a fixture publicada.
# É acoplamento do teste à composição do catálogo, não regressão. O fix de raiz é
# um banco EFÊMERO só-fixture pro catalog (como o order) OU tornar o teste
# fixture-scoped. Se a ÚNICA falha for essa, o software está verde.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 1

CAT_C=utilar_catalog_db
psqlc() { docker exec -i "$CAT_C" psql -U utilar -d catalog_service -tAc "$1" 2>/dev/null; }

SNAP="$(mktemp)"
restore_fixture() {
  if [ -s "$SNAP" ]; then
    local ids; ids="$(cat "$SNAP")"
    psqlc "UPDATE products SET status='archived' WHERE id::text IN ($ids);" >/dev/null
    echo "→ fixture do catalog restaurada (re-arquivada)."
  fi
  rm -f "$SNAP"
}
trap restore_fixture EXIT

echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: preparando ambiente"
echo "════════════════════════════════════════════════════════"

# --- 1. parar serviços Go rodando (mantém as DBs de pé) ---
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

# --- 2. publicar a fixture do catalog (snapshot p/ restaurar) ---
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "$CAT_C"; then
  # ids atualmente ARQUIVADOS da fixture → só esses voltam a archived no fim.
  psqlc "SELECT COALESCE(string_agg(quote_literal(id::text), ','), '') \
         FROM products WHERE (sku LIKE 'CUR-%' OR sku LIKE 'UTL-%') AND status='archived';" > "$SNAP"
  if [ -s "$SNAP" ] && [ "$(cat "$SNAP")" != "" ]; then
    psqlc "UPDATE products SET status='published' \
           WHERE (sku LIKE 'CUR-%' OR sku LIKE 'UTL-%') AND status='archived';" >/dev/null
    echo "→ fixture do catalog publicada temporariamente (restaura no fim)."
  else
    : > "$SNAP"  # nada a restaurar
    echo "→ fixture do catalog já estava publicada (nada a fazer)."
  fi
else
  echo "⚠️  container $CAT_C não encontrado — pulando preparo do catalog."
  : > "$SNAP"
fi

# --- 3. delega a pirâmide pra test-utilar ---
echo
echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: rodando a pirâmide (test-utilar)"
echo "════════════════════════════════════════════════════════"
.claude/skills/test-utilar/run-tests.sh "${1:-}"
rc=$?

echo
echo "════════════════════════════════════════════════════════"
echo "▶ qa-utilar: como ler o resultado"
echo "════════════════════════════════════════════════════════"
echo "  • Verde em tudo → software são."
echo "  • Se a ÚNICA falha do catalog for TestList_DevolveCapaDoProduto:"
echo "    é o caveat conhecido (página 1 dominada por produtos reais sem foto),"
echo "    NÃO é regressão. Ver o cabeçalho deste script."
echo "  • Qualquer outra falha do catalog com a fixture publicada É pra investigar."

exit $rc
