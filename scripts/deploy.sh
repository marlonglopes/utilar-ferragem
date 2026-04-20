#!/usr/bin/env bash
# Deploy SPA to S3 + invalidate CloudFront
# Usage: ./scripts/deploy.sh [staging|production]
# Requires: AWS CLI configured, env vars below set

set -euo pipefail

ENV="${1:-staging}"

if [[ "$ENV" == "production" ]]; then
  S3_BUCKET="${S3_BUCKET_PROD:?S3_BUCKET_PROD not set}"
  CF_DISTRIBUTION="${CF_DISTRIBUTION_PROD:?CF_DISTRIBUTION_PROD not set}"
elif [[ "$ENV" == "staging" ]]; then
  S3_BUCKET="${S3_BUCKET_STAGING:?S3_BUCKET_STAGING not set}"
  CF_DISTRIBUTION="${CF_DISTRIBUTION_STAGING:?CF_DISTRIBUTION_STAGING not set}"
else
  echo "Usage: $0 [staging|production]" >&2
  exit 1
fi

echo "▶ Building for $ENV..."
cd "$(dirname "$0")/../app"
npm run build

echo "▶ Uploading to s3://$S3_BUCKET..."
# Long-lived cache for hashed assets
aws s3 sync dist/ "s3://$S3_BUCKET" \
  --exclude "index.html" \
  --cache-control "public, max-age=31536000, immutable" \
  --delete

# Short cache for index.html (5 min)
aws s3 cp dist/index.html "s3://$S3_BUCKET/index.html" \
  --cache-control "public, max-age=300"

echo "▶ Invalidating CloudFront distribution $CF_DISTRIBUTION..."
aws cloudfront create-invalidation \
  --distribution-id "$CF_DISTRIBUTION" \
  --paths "/*" \
  --query 'Invalidation.Id' \
  --output text

echo "✓ Deploy to $ENV complete."
