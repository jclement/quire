#!/usr/bin/env bash
# Dev runner: Go API server + Vite dev server (HMR), torn down together.
# Vite proxies /api and /mcp to the Go server, so the browser talks to :5173.
#
# Two portability notes worth keeping:
#   - macOS ships bash 3.2, so no `wait -n` — poll both PIDs instead.
#   - We build and exec the binary rather than `go run`, because killing
#     `go run` leaves its compiled child running and holding the port.
set -euo pipefail
cd "$(dirname "$0")/.."

export QUIRE_DEV=1

mkdir -p tmp
echo "building quire…"
go build -o tmp/quire-dev .

./tmp/quire-dev serve &
GO_PID=$!
bun run --cwd web dev &
VITE_PID=$!

cleanup() {
  trap - EXIT INT TERM
  kill "$GO_PID" "$VITE_PID" 2>/dev/null || true
  wait "$GO_PID" 2>/dev/null || true
  wait "$VITE_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Exit as soon as either side dies, so a crashed server doesn't leave a
# half-running dev environment behind.
while kill -0 "$GO_PID" 2>/dev/null && kill -0 "$VITE_PID" 2>/dev/null; do
  sleep 1
done
