#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/home/user/code/agy-mobile-gateway"
cd "$REPO_DIR"

cleanup() {
  docker compose -f compose.dev.yml --profile test down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> [1/2] Building frontend for embedded Go binary..."
(cd frontend && npm run build)

echo "==> [2/2] Running Playwright dev container (iOS + Desktop E2E)..."
docker compose -f compose.dev.yml --profile test up --build --abort-on-container-exit --exit-code-from playwright
