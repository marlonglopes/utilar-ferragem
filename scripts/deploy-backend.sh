#!/usr/bin/env bash
# Deploy dos 4 serviços Go: build → push ECR → EC2 pull+up.
# Espelha o padrão da Gifthy (rsync/build + docker compose up na EC2).
#
# Uso: ./scripts/deploy-backend.sh [tag]
# Requer no ambiente:
#   ECR_REGISTRY   <acct>.dkr.ecr.sa-east-1.amazonaws.com
#   EC2_HOST       ubuntu@<ip-ou-dns da EC2>   (ou um host do ~/.ssh/config)
#   AWS_REGION     sa-east-1 (default)
# A EC2 deve ter /opt/utilar com docker-compose.prod.yml, deploy/ e .env.prod.

set -euo pipefail

TAG="${1:-$(git rev-parse --short HEAD)}"
REGION="${AWS_REGION:-sa-east-1}"
: "${ECR_REGISTRY:?defina ECR_REGISTRY}"
: "${EC2_HOST:?defina EC2_HOST (ex: ubuntu@1.2.3.4)}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SERVICES=(catalog order auth payment assistant)

echo "▶ Login no ECR ($REGION)…"
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$ECR_REGISTRY"

for svc in "${SERVICES[@]}"; do
  img="$ECR_REGISTRY/$svc:$TAG"
  echo "▶ Build+push $svc → $img"
  docker build --build-arg SERVICE="$svc-service" -t "$img" .
  # garante o repositório (idempotente)
  aws ecr describe-repositories --region "$REGION" --repository-names "$svc" >/dev/null 2>&1 \
    || aws ecr create-repository --region "$REGION" --repository-name "$svc" >/dev/null
  docker push "$img"
done

echo "▶ Sincroniza compose + nginx pra EC2…"
rsync -az docker-compose.prod.yml deploy/ "$EC2_HOST":/opt/utilar/

echo "▶ Pull + up na EC2…"
# shellcheck disable=SC2029
ssh "$EC2_HOST" "cd /opt/utilar && export ECR_REGISTRY=$ECR_REGISTRY TAG=$TAG && \
  aws ecr get-login-password --region $REGION | docker login --username AWS --password-stdin $ECR_REGISTRY && \
  docker compose -f docker-compose.prod.yml --env-file .env.prod pull && \
  docker compose -f docker-compose.prod.yml --env-file .env.prod up -d && \
  docker compose -f docker-compose.prod.yml ps"

echo "✓ Backend deployado (tag=$TAG). Health: curl http://\$EC2/health"
