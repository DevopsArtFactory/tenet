#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PORT=8080

compose_cmd=("$@")
if [[ ${#compose_cmd[@]} -eq 0 ]]; then
  compose_cmd=(docker compose)
fi

cd "$ROOT"

compose_port=$("${compose_cmd[@]}" port api "$PORT" 2>/dev/null || true)
lsof_output=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)
stack_owns_port=0
case "$compose_port" in
  *":$PORT") stack_owns_port=1 ;;
esac

if [[ -n "$lsof_output" && "$stack_owns_port" -ne 1 ]]; then
  cat >&2 <<'EOF'
docker-up preflight failed: localhost:8080 is already occupied by something other than this Tenet compose stack.

Inspect the current listener:
  lsof -nP -iTCP:8080 -sTCP:LISTEN

If you were expecting a prior Tenet stack to own that port, stop it first:
  make docker-down

Then rerun the local stack:
  docker compose up --build -d
EOF
  echo >&2
  echo "$lsof_output" >&2
  exit 1
fi

exec "${compose_cmd[@]}" up --build -d
