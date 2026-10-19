#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

pnpm --dir frontend dev &
VITE_PID=$!

cleanup() {
  kill "$VITE_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:9245/ >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

FRONTEND_DEVSERVER_URL=http://127.0.0.1:9245 go run -tags nodynamic .
