#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/home/ryanslx/code/agy-mobile-gateway"
OUT_DIR="${REPO_DIR}/dist-releases"
mkdir -p "$OUT_DIR"

echo "==> [1/2] Building frontend..."
(cd "${REPO_DIR}/frontend" && npm run build)

echo "==> [2/2] Building multi-OS Go binaries (with Windows PE icon)..."
cd "${REPO_DIR}/backend-go"
for target in \
  "linux amd64 agy-gateway-linux-amd64" \
  "linux arm64 agy-gateway-linux-arm64" \
  "darwin amd64 agy-gateway-darwin-amd64" \
  "darwin arm64 agy-gateway-darwin-arm64" \
  "windows amd64 agy-gateway-windows-amd64.exe" \
  "windows arm64 agy-gateway-windows-arm64.exe"
do
  read -r goos goarch outname <<<"$target"
  echo "  -> ${goos}/${goarch} (${outname})"
  GOOS="$goos" GOARCH="$goarch" go build -ldflags="-s -w" -o "${OUT_DIR}/${outname}" ./cmd/agy-gateway
done

ls -lh "$OUT_DIR"
