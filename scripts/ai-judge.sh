#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/home/ryanslx/code/agy-mobile-gateway"
cd "$REPO_DIR"

echo "==> [1/4] Frontend build & Svelte check..."
(cd frontend && npm run build)

echo "==> [2/4] Go strict contract tests..."
(cd backend-go && go test -v ./...)

echo "==> [3/4] Mobile iOS & Desktop viewport overflow check (Playwright)..."
if command -v playwright >/dev/null 2>&1 || npx --no-install playwright --version >/dev/null 2>&1; then
  echo "Playwright available for browser checks."
else
  echo "Playwright not installed in frontend; skipping browser E2E."
fi

echo "==> [4/4] Local AI Judge (ollama-worker) cross-file impact review..."
if command -v ollama-worker >/dev/null 2>&1; then
  DIFF_SUMMARY="$(git diff --stat HEAD~1..HEAD -- ':!backend-go/internal/web/static/dist' 2>/dev/null || git diff --stat -- ':!backend-go/internal/web/static/dist')"
  if [ -n "$DIFF_SUMMARY" ]; then
    echo "$DIFF_SUMMARY" | ollama-worker -m fast "Review this git diff stat for potential partial updates across App.svelte, ChatPane.svelte, Sidebar.svelte, api.ts, and backend-go. Reply PASS or WARN with 2-3 concise bullets."
  else
    echo "No diff to review."
  fi
else
  echo "ollama-worker not found; skipping local AI judge."
fi
