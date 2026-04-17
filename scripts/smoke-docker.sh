#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# shellcheck source=./docker-up.sh
source "$ROOT/scripts/docker-up.sh"

HOST=${TENET_API_HOST:-127.0.0.1}
PORT=${TENET_API_PORT:-8080}
STACK_STARTED=0

cleanup() {
  if [[ "$STACK_STARTED" -eq 1 ]]; then
    tenet_docker_compose down --remove-orphans || true
  fi
}
trap cleanup EXIT

wait_for_running_services() {
  local services
  for _ in $(seq 1 30); do
    services=$(tenet_docker_compose ps --services --status running 2>/dev/null || true)
    if RUNNING_SERVICES="$services" python3 - <<'PY'
import os
import sys

services = {line.strip() for line in os.environ["RUNNING_SERVICES"].splitlines() if line.strip()}
required = {"api", "controller", "redis"}
sys.exit(0 if required.issubset(services) else 1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "compose stack did not report api/controller/redis as running" >&2
  tenet_docker_compose ps >&2 || true
  return 1
}

wait_for_http() {
  local url=$1
  for _ in $(seq 1 30); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "endpoint did not become ready: $url" >&2
  return 1
}

wait_for_logs() {
  local service=$1
  local pattern=$2
  local logs

  for _ in $(seq 1 30); do
    logs=$(tenet_docker_compose logs "$service" --tail=50 2>&1 || true)
    if [[ -n "$logs" ]] && grep -Fq "$pattern" <<<"$logs"; then
      printf '%s' "$logs"
      return 0
    fi
    sleep 1
  done

  echo "logs for service '$service' were missing or did not include expected evidence: $pattern" >&2
  tenet_docker_compose logs "$service" --tail=100 >&2 || true
  return 1
}

tenet_docker_compose config >/dev/null
tenet_docker_up
STACK_STARTED=1

wait_for_running_services
wait_for_http "http://$HOST:$PORT/healthz"

service_ps=$(tenet_docker_compose ps)
summary=$(curl -fsS "http://$HOST:$PORT/v1/catalog/summary")
overview=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts")
deployment_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
ui=$(curl -fsS "http://$HOST:$PORT/")
api_logs=$(wait_for_logs api "starting tenet-api")
controller_logs=$(wait_for_logs controller "controller reconcile deployment=payments-api-prod")

RUNNING_SERVICES=$(tenet_docker_compose ps --services --status running) \
SERVICE_PS="$service_ps" \
SUMMARY_JSON="$summary" \
OVERVIEW_JSON="$overview" \
DEPLOYMENT_DETAIL_JSON="$deployment_detail" \
STATUS_JSON="$status" \
UI_HTML="$ui" \
API_LOGS="$api_logs" \
CONTROLLER_LOGS="$controller_logs" \
python3 - <<'PY'
import json
import os

services = {line.strip() for line in os.environ["RUNNING_SERVICES"].splitlines() if line.strip()}
summary = json.loads(os.environ["SUMMARY_JSON"])
overview = json.loads(os.environ["OVERVIEW_JSON"])
deployment_detail = json.loads(os.environ["DEPLOYMENT_DETAIL_JSON"])
status = json.loads(os.environ["STATUS_JSON"])
ui = os.environ["UI_HTML"]
api_logs = os.environ["API_LOGS"]
controller_logs = os.environ["CONTROLLER_LOGS"]

assert {"api", "controller", "redis"}.issubset(services), services
assert "api" in os.environ["SERVICE_PS"], os.environ["SERVICE_PS"]
assert "controller" in os.environ["SERVICE_PS"], os.environ["SERVICE_PS"]
assert "redis" in os.environ["SERVICE_PS"], os.environ["SERVICE_PS"]
assert summary["accountCount"] == 2, summary
assert summary["environmentCount"] == 2, summary
assert summary["deploymentCount"] == 3, summary
assert [account["name"] for account in overview] == ["prod-core", "staging-core"], overview
assert [deployment["name"] for deployment in overview[0]["deployments"]] == ["payments-api-prod", "worker-prod"], overview
assert deployment_detail["name"] == "payments-api-prod", deployment_detail
assert deployment_detail["accountName"] == "prod-core", deployment_detail
assert deployment_detail["awsAccountId"] == "123456789012", deployment_detail
assert deployment_detail["region"] == "ap-northeast-2", deployment_detail
assert status["deploymentName"] == "payments-api-prod", status
assert status["status"]["sync"]["state"] == "Synced", status
assert "Application status overview" in ui, ui
assert "starting tenet-api" in api_logs, api_logs
assert "starting tenet-controller" in controller_logs, controller_logs
assert "controller reconcile deployment=payments-api-prod" in controller_logs, controller_logs
print("smoke-docker: compose stack ok")
PY

echo "$service_ps"
echo "smoke-docker: verified /healthz, summary, overview, deployment detail, status, and UI"
