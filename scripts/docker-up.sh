#!/usr/bin/env bash
set -euo pipefail

TENET_DOCKER_HELPER_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TENET_DOCKER_COMPOSE_CMD=()

tenet_docker_compose() {
  if [[ ${#TENET_DOCKER_COMPOSE_CMD[@]} -eq 0 ]]; then
    read -r -a TENET_DOCKER_COMPOSE_CMD <<<"${DOCKER_COMPOSE:-docker compose}"
  fi
  (
    cd "$TENET_DOCKER_HELPER_ROOT"
    "${TENET_DOCKER_COMPOSE_CMD[@]}" "$@"
  )
}

tenet_port_8080_is_listening() {
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:8080 -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi

  python3 - <<'PY'
import socket
import sys

targets = [("127.0.0.1", 8080)]
if socket.has_ipv6:
    targets.append(("::1", 8080))

for host, port in targets:
    try:
        with socket.create_connection((host, port), timeout=0.25):
            sys.exit(0)
    except OSError:
        continue

sys.exit(1)
PY
}

tenet_docker_up() {
  if tenet_port_8080_is_listening; then
    {
      echo "Tenet docker-up refused to start because port 8080 is already listening."
      echo "Stop the existing listener first, or run 'make docker-down' if the compose stack is already running."
      if command -v lsof >/dev/null 2>&1; then
        echo
        echo "Current listeners on port 8080:"
        lsof -nP -iTCP:8080 -sTCP:LISTEN || true
      fi
    } >&2
    exit 1
  fi

  tenet_docker_compose up --build -d
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  tenet_docker_up "$@"
fi
