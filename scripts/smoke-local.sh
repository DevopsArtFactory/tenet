#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
HOST=${TENET_API_HOST:-127.0.0.1}
PORT=${TENET_API_PORT:-$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
)}
REPO_ROOT=${TENET_REPO_ROOT:-$ROOT/examples/local-repo}
STATUS_PATH=${TENET_STATUS_PATH:-$ROOT/examples/local-status/deployments.json}
LOG_FILE=$(mktemp)
cleanup() {
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE"
}
trap cleanup EXIT

TENET_API_ADDR=":$PORT" TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" \
  go run ./cmd/tenet-api >"$LOG_FILE" 2>&1 &
API_PID=$!

for _ in $(seq 1 30); do
  if curl -fsS "http://$HOST:$PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl -fsS "http://$HOST:$PORT/healthz" >/dev/null

summary=$(curl -fsS "http://$HOST:$PORT/v1/catalog/summary")
overview=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
ui=$(curl -fsS "http://$HOST:$PORT/")

SUMMARY_JSON="$summary" OVERVIEW_JSON="$overview" STATUS_JSON="$status" UI_HTML="$ui" python3 - <<'PY'
import json, os
summary = json.loads(os.environ['SUMMARY_JSON'])
overview = json.loads(os.environ['OVERVIEW_JSON'])
status = json.loads(os.environ['STATUS_JSON'])
ui = os.environ['UI_HTML']
assert summary['accountCount'] == 2, summary
assert summary['environmentCount'] == 2, summary
assert summary['deploymentCount'] == 2, summary
assert overview[0]['deployments'][0]['executionMode'] in {'asgWeightedTargetGroups', 'asgInstanceRefresh'}, overview
assert any(account['name'] == 'prod-core' for account in overview), overview
assert status['deploymentName'] == 'payments-api-prod', status
assert status['status']['sync']['state'] == 'Synced', status
assert status['status']['health']['overall'] == 'Healthy', status
assert status['status']['deployedArtifact']['binary'].startswith('s3://releases.example.com/'), status
assert '/v1/overview/accounts' in ui, ui
print('smoke-local: ok')
PY
