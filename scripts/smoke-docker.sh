#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE_FILE=${TENET_COMPOSE_FILE:-$ROOT/compose.yaml}
HOST=${TENET_DOCKER_SMOKE_HOST:-127.0.0.1}
PORT=${TENET_DOCKER_SMOKE_PORT:-8080}
API_BASE_URL=${TENET_DOCKER_SMOKE_API_BASE_URL:-http://$HOST:$PORT}

if ! command -v docker >/dev/null 2>&1; then
  echo "smoke-docker: docker is required" >&2
  exit 1
fi

if [[ ! -f "$COMPOSE_FILE" ]]; then
  echo "smoke-docker: compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

COMPOSE=(docker compose -f "$COMPOSE_FILE")

assert_request_id_header() {
  local url=$1
  local headers

  headers=$(curl -fsS -D - -o /dev/null "$url")
  HEADERS_TEXT="$headers" python3 - <<'PY'
import os

headers = {}
for raw in os.environ["HEADERS_TEXT"].splitlines():
    line = raw.rstrip("\r\n")
    if not line or ":" not in line:
        continue
    name, value = line.split(":", 1)
    headers[name.strip().lower()] = value.strip()

request_id = headers.get("request-id")
assert request_id, headers
print(request_id)
PY
}

assert_structured_api_request_logs() {
  local logs_text=$1
  local healthz_request_id=$2
  local summary_request_id=$3
  local worker_resource_logs_request_id=$4
  local ui_request_id=$5

  API_LOGS_TEXT="$logs_text" \
  HEALTHZ_REQUEST_ID="$healthz_request_id" \
  SUMMARY_REQUEST_ID="$summary_request_id" \
  WORKER_RESOURCE_LOGS_REQUEST_ID="$worker_resource_logs_request_id" \
  UI_REQUEST_ID="$ui_request_id" \
  python3 - <<'PY'
import json
import os

entries = []
for raw in os.environ["API_LOGS_TEXT"].splitlines():
    line = raw.strip()
    if not line:
        continue
    start = line.find("{")
    if start == -1:
        continue
    try:
        entry = json.loads(line[start:])
    except json.JSONDecodeError:
        continue
    if entry.get("msg") != "http_request":
        continue
    entries.append(entry)

assert entries, "expected structured api request log lines"

expected = [
    (os.environ["HEALTHZ_REQUEST_ID"], "/healthz"),
    (os.environ["SUMMARY_REQUEST_ID"], "/v1/catalog/summary"),
    (os.environ["WORKER_RESOURCE_LOGS_REQUEST_ID"], "/v1/overview/deployments/worker-prod/resource-logs"),
    (os.environ["UI_REQUEST_ID"], "/"),
]

for request_id, path in expected:
    matches = [entry for entry in entries if entry.get("request_id") == request_id and entry.get("path") == path]
    assert matches, {"request_id": request_id, "path": path, "entries": entries}
    entry = matches[-1]
    assert entry.get("method") == "GET", entry
    assert entry.get("status") == 200, entry
    assert entry.get("result") == "success", entry
    assert entry.get("duration_ms") is not None, entry
PY
}

cleanup() {
  local exit_code=${1:-0}

  if [[ "$exit_code" -ne 0 ]]; then
    echo "smoke-docker: failure detected, dumping recent compose logs" >&2
    "${COMPOSE[@]}" logs api --tail=50 >&2 || true
    "${COMPOSE[@]}" logs controller --tail=50 >&2 || true
  fi

  "${COMPOSE[@]}" down --remove-orphans >/dev/null 2>&1 || true
}

trap 'cleanup "$?"' EXIT

"${COMPOSE[@]}" config >/dev/null
services=$("${COMPOSE[@]}" config --services)
for service in api controller redis; do
  if ! grep -qx "$service" <<<"$services"; then
    echo "smoke-docker: expected compose service $service" >&2
    exit 1
  fi
done

"$ROOT/scripts/docker-up.sh" "${COMPOSE[@]}" >/dev/null

for _ in $(seq 1 60); do
  if curl -fsS "$API_BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl -fsS "$API_BASE_URL/healthz" >/dev/null

healthz_request_id=$(assert_request_id_header "$API_BASE_URL/healthz")
summary_request_id=$(assert_request_id_header "$API_BASE_URL/v1/catalog/summary")
worker_resource_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
ui_request_id=$(assert_request_id_header "$API_BASE_URL/")

summary=$(curl -fsS "$API_BASE_URL/v1/catalog/summary")
applications=$(curl -fsS "$API_BASE_URL/v1/overview/applications")
deployment_detail=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod")
deployment_events=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/events")
worker_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
ui=$(curl -fsS "$API_BASE_URL/")

SUMMARY_JSON="$summary" APPLICATIONS_JSON="$applications" DEPLOYMENT_DETAIL_JSON="$deployment_detail" DEPLOYMENT_EVENTS_JSON="$deployment_events" WORKER_RESOURCE_LOGS_JSON="$worker_resource_logs" UI_HTML="$ui" python3 - <<'PY'
import json, os

summary = json.loads(os.environ["SUMMARY_JSON"])
applications = json.loads(os.environ["APPLICATIONS_JSON"])
deployment_detail = json.loads(os.environ["DEPLOYMENT_DETAIL_JSON"])
deployment_events = json.loads(os.environ["DEPLOYMENT_EVENTS_JSON"])
worker_resource_logs = json.loads(os.environ["WORKER_RESOURCE_LOGS_JSON"])
ui = os.environ["UI_HTML"]

assert summary["accountCount"] == 2, summary
assert summary["environmentCount"] == 2, summary
assert summary["deploymentCount"] == 3, summary

application_names = [application["name"] for application in applications]
assert "payments-api-prod" in application_names, applications
assert "worker-prod" in application_names, applications

assert deployment_detail["name"] == "payments-api-prod", deployment_detail
assert deployment_detail["awsAccountId"] == "123456789012", deployment_detail
assert deployment_detail["accountName"] == "prod-core", deployment_detail
assert isinstance((deployment_detail.get("rollout") or {}).get("execution", []), list), deployment_detail

assert deployment_events["deploymentName"] == "payments-api-prod", deployment_events
assert isinstance(deployment_events.get("events"), list), deployment_events
for event in deployment_events["events"]:
    assert isinstance(event.get("timestamp"), str) and event["timestamp"], deployment_events
    assert isinstance(event.get("source"), str) and event["source"], deployment_events
    assert isinstance(event.get("summary"), str) and event["summary"], deployment_events
    assert isinstance((event.get("sync") or {}).get("state"), str) and (event.get("sync") or {}).get("state"), deployment_events
    assert isinstance((event.get("health") or {}).get("overall"), str) and (event.get("health") or {}).get("overall"), deployment_events
    assert event.get("changes") is None or isinstance(event.get("changes"), list), deployment_events
    assert event.get("changeDetails") is None or isinstance(event.get("changeDetails"), list), deployment_events

assert worker_resource_logs["deploymentName"] == "worker-prod", worker_resource_logs
assert worker_resource_logs["resourceType"] == "ecsService", worker_resource_logs
assert worker_resource_logs["resourceName"] == "worker-prod-service", worker_resource_logs
assert isinstance(worker_resource_logs["available"], bool), worker_resource_logs
assert worker_resource_logs.get("entries") is None or isinstance(worker_resource_logs.get("entries"), list), worker_resource_logs
if worker_resource_logs["available"]:
    assert isinstance(worker_resource_logs.get("source"), str) and worker_resource_logs["source"], worker_resource_logs
    for entry in worker_resource_logs.get("entries") or []:
        assert isinstance(entry.get("timestamp"), str) and entry["timestamp"], worker_resource_logs
        assert isinstance(entry.get("message"), str) and entry["message"], worker_resource_logs
        if "stream" in entry:
            assert isinstance(entry.get("stream"), str) and entry["stream"], worker_resource_logs
else:
    assert isinstance(worker_resource_logs.get("unavailableReason"), str) and worker_resource_logs["unavailableReason"], worker_resource_logs

for fragment in (
    "Open account",
    "Open application",
    "Drift view",
    "Plan/apply",
    "Controller runtime events",
    "Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.",
    "Recent logs",
    "fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`)",
):
    assert fragment in ui, fragment
PY

controller_logs=""
for _ in $(seq 1 60); do
  controller_logs=$("${COMPOSE[@]}" logs controller --tail=50 2>&1 || true)
  if [[ "$controller_logs" == *"starting tenet-controller"* && "$controller_logs" == *"controller reconcile deployment=payments-api-prod"* ]]; then
    break
  fi
  sleep 1
done

if [[ "$controller_logs" != *"starting tenet-controller"* ]]; then
  echo "smoke-docker: controller logs missing startup line" >&2
  exit 1
fi

if [[ "$controller_logs" != *"controller reconcile deployment=payments-api-prod"* ]]; then
  echo "smoke-docker: controller logs missing reconcile line for payments-api-prod" >&2
  exit 1
fi

api_logs=""
for _ in $(seq 1 60); do
  api_logs=$("${COMPOSE[@]}" logs api --tail=200 2>&1 || true)
  if assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$ui_request_id" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$ui_request_id"; then
  echo "smoke-docker: api logs missing correlated structured request entries" >&2
  exit 1
fi

echo "smoke-docker: compose stack path ok"
