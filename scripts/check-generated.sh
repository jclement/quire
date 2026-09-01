#!/usr/bin/env bash
# Fails when web/src/api/generated.ts is out of date with the Go structs it
# derives from. Run by `mise run lint` (and therefore by CI), so a renamed
# field breaks the build rather than surfacing as a runtime bug.
set -euo pipefail
cd "$(dirname "$0")/.."

generated="web/src/api/generated.ts"
before=$(cat "$generated" 2>/dev/null || true)
mise run gen >/dev/null
after=$(cat "$generated")

if [ "$before" != "$after" ]; then
  echo "error: $generated is out of date with internal/service/apitypes.go" >&2
  echo "Run 'mise run gen' and commit the result. Diff:" >&2
  printf '%s\n' "$before" > /tmp/quire-generated-before.ts
  diff -u /tmp/quire-generated-before.ts "$generated" >&2 || true
  exit 1
fi
echo "generated types are current"
