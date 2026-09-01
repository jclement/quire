#!/usr/bin/env bash
# Dev runner: Go API server + Vite dev server (HMR), torn down together.
# Vite proxies /api and /mcp to the Go server, so the browser talks to :5173.
set -euo pipefail
cd "$(dirname "$0")/.."

export QUIRE_DEV=1

cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT

go run . serve &
bun run --cwd web dev &

wait -n
