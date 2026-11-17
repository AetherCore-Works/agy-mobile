#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/home/user/code/agy-mobile-gateway"
RPI_HOST="${RPI_HOST:-rpi}"
RPI_STACK_DIR="/opt/stacks/agy-gateway"

echo "==> [1/3] Building Svelte 5 frontend & testing Go backend..."
cd "$REPO_DIR/frontend" && npm run build
cd "$REPO_DIR/backend-go" && go test ./...

echo "==> [2/3] Syncing code to ${RPI_HOST}:${RPI_STACK_DIR} (preserving ssh/, data/, .env)..."
rsync -az --delete \
  --exclude='.git' \
  --exclude='ssh' \
  --exclude='ssh/**' \
  --exclude='data' \
  --exclude='data/**' \
  --exclude='.env' \
  --exclude='frontend/node_modules' \
  --exclude='backend-go/.tmp' \
  --exclude='app' \
  "$REPO_DIR/" "${RPI_HOST}:${RPI_STACK_DIR}/"

echo "==> [3/3] Rebuilding and restarting agy-gateway container on ${RPI_HOST}..."
ssh "$RPI_HOST" "cd ${RPI_STACK_DIR} && docker compose up -d --build agy-gateway && sleep 2 && curl -sf http://127.0.0.1:8088/api/health"
echo ""
echo "==> Deploy completed safely!"
