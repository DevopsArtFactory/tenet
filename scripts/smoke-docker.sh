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
  local audit_logs_request_id=$5
  local ui_request_id=$6
  local audit_ui_request_id=$7

  API_LOGS_TEXT="$logs_text" \
  HEALTHZ_REQUEST_ID="$healthz_request_id" \
  SUMMARY_REQUEST_ID="$summary_request_id" \
  WORKER_RESOURCE_LOGS_REQUEST_ID="$worker_resource_logs_request_id" \
  AUDIT_LOGS_REQUEST_ID="$audit_logs_request_id" \
  UI_REQUEST_ID="$ui_request_id" \
  AUDIT_UI_REQUEST_ID="$audit_ui_request_id" \
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
    (os.environ["AUDIT_LOGS_REQUEST_ID"], "/v1/audit-logs"),
    (os.environ["UI_REQUEST_ID"], "/"),
    (os.environ["AUDIT_UI_REQUEST_ID"], "/"),
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
deployment_source_fingerprint=$(DEPLOYMENT_DETAIL_JSON="$deployment_detail" python3 - <<'PY'
import json
import os

print((json.loads(os.environ["DEPLOYMENT_DETAIL_JSON"]).get("sourceFingerprint") or "").strip())
PY
)
plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'
import json
import os

fingerprint = (os.environ.get("DEPLOYMENT_SOURCE_FINGERPRINT") or "").strip()
print(json.dumps({"ifMatch": fingerprint} if fingerprint else {}))
PY
)
plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/payments-api-prod")
plan_id=$(PLAN_PREVIEW_JSON="$plan_preview" python3 - <<'PY'
import json
import os

print(json.loads(os.environ["PLAN_PREVIEW_JSON"])["id"])
PY
)
plan_preview_get=$(curl -fsS "$API_BASE_URL/v1/plans/$plan_id")
plan_status=$(PLAN_PREVIEW_JSON="$plan_preview" PLAN_PREVIEW_GET_JSON="$plan_preview_get" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'
import json
import os

plan_preview = json.loads(os.environ["PLAN_PREVIEW_JSON"])
plan_preview_get = json.loads(os.environ["PLAN_PREVIEW_GET_JSON"])
deployment_source_fingerprint = os.environ["DEPLOYMENT_SOURCE_FINGERPRINT"]

assert plan_preview["deploymentName"] == "payments-api-prod", plan_preview
assert plan_preview["status"] in {"ready", "failed"}, plan_preview
assert isinstance(plan_preview.get("id"), str) and plan_preview["id"].startswith("plan_"), plan_preview
assert isinstance(plan_preview.get("createdAt"), str) and plan_preview["createdAt"], plan_preview
assert plan_preview["source"]["route"] == "/v1/overview/deployments/payments-api-prod", plan_preview
assert isinstance(plan_preview["source"].get("fingerprint"), str) and plan_preview["source"]["fingerprint"], plan_preview
if deployment_source_fingerprint:
    assert plan_preview["source"]["fingerprint"] == deployment_source_fingerprint, (plan_preview, deployment_source_fingerprint)
assert isinstance(plan_preview.get("operations", []), list), plan_preview
assert isinstance((plan_preview.get("risk") or {}).get("reasons", []), list), plan_preview
assert isinstance((plan_preview.get("approval") or {}).get("required"), bool), plan_preview
if plan_preview["status"] == "ready":
    assert plan_preview["operations"], plan_preview
    assert not plan_preview.get("errors"), plan_preview
else:
    assert isinstance(plan_preview.get("errors"), list) and plan_preview["errors"], plan_preview
    assert plan_preview["approval"]["required"] is True, plan_preview
assert plan_preview_get == plan_preview, (plan_preview_get, plan_preview)
print(plan_preview["status"])
PY
)
if [[ "$plan_status" != "ready" ]]; then
  echo "smoke-docker: expected compose stack to return a ready immutable plan preview for payments-api-prod, got $plan_status" >&2
  exit 1
fi
apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$plan_id")
apply_id=$(APPLY_RECORD_JSON="$apply_record" python3 - <<'PY'
import json
import os

print(json.loads(os.environ["APPLY_RECORD_JSON"])["id"])
PY
)
apply_record_get=$(curl -fsS "$API_BASE_URL/v1/applies/$apply_id")
deployment_events=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/events")
worker_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
audit_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/audit-logs?limit=2")
audit_logs=$(curl -fsS "$API_BASE_URL/v1/audit-logs?limit=2")
ui=$(curl -fsS "$API_BASE_URL/")
audit_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=audit&action=apply.requested&limit=2")
audit_ui=$(curl -fsS "$API_BASE_URL/?page=audit&action=apply.requested&limit=2")
plan_apply_ui=$(curl -fsS "$API_BASE_URL/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id")

SUMMARY_JSON="$summary" APPLICATIONS_JSON="$applications" DEPLOYMENT_DETAIL_JSON="$deployment_detail" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" PLAN_PREVIEW_JSON="$plan_preview" PLAN_PREVIEW_GET_JSON="$plan_preview_get" APPLY_RECORD_JSON="$apply_record" APPLY_RECORD_GET_JSON="$apply_record_get" DEPLOYMENT_EVENTS_JSON="$deployment_events" WORKER_RESOURCE_LOGS_JSON="$worker_resource_logs" AUDIT_LOGS_JSON="$audit_logs" UI_HTML="$ui" AUDIT_UI_HTML="$audit_ui" PLAN_APPLY_UI_HTML="$plan_apply_ui" python3 - <<'PY'
import json, os

summary = json.loads(os.environ["SUMMARY_JSON"])
applications = json.loads(os.environ["APPLICATIONS_JSON"])
deployment_detail = json.loads(os.environ["DEPLOYMENT_DETAIL_JSON"])
deployment_source_fingerprint = os.environ["DEPLOYMENT_SOURCE_FINGERPRINT"]
plan_preview = json.loads(os.environ["PLAN_PREVIEW_JSON"])
plan_preview_get = json.loads(os.environ["PLAN_PREVIEW_GET_JSON"])
apply_record = json.loads(os.environ["APPLY_RECORD_JSON"])
apply_record_get = json.loads(os.environ["APPLY_RECORD_GET_JSON"])
deployment_events = json.loads(os.environ["DEPLOYMENT_EVENTS_JSON"])
worker_resource_logs = json.loads(os.environ["WORKER_RESOURCE_LOGS_JSON"])
audit_logs = json.loads(os.environ["AUDIT_LOGS_JSON"])
ui = os.environ["UI_HTML"]
audit_ui = os.environ["AUDIT_UI_HTML"]
plan_apply_ui = os.environ["PLAN_APPLY_UI_HTML"]

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
if deployment_source_fingerprint:
    assert deployment_detail.get("sourceFingerprint") == deployment_source_fingerprint, deployment_detail

assert plan_preview["deploymentName"] == "payments-api-prod", plan_preview
assert plan_preview["status"] == "ready", plan_preview
assert isinstance(plan_preview.get("id"), str) and plan_preview["id"].startswith("plan_"), plan_preview
assert isinstance(plan_preview.get("createdAt"), str) and plan_preview["createdAt"], plan_preview
assert plan_preview["source"]["route"] == "/v1/overview/deployments/payments-api-prod", plan_preview
assert isinstance(plan_preview["source"].get("fingerprint"), str) and plan_preview["source"]["fingerprint"], plan_preview
if deployment_source_fingerprint:
    assert plan_preview["source"]["fingerprint"] == deployment_source_fingerprint, (plan_preview, deployment_detail)
assert isinstance(plan_preview.get("operations", []), list) and plan_preview["operations"], plan_preview
assert isinstance((plan_preview.get("risk") or {}).get("reasons", []), list), plan_preview
assert isinstance((plan_preview.get("approval") or {}).get("required"), bool), plan_preview
assert plan_preview_get == plan_preview, (plan_preview_get, plan_preview)

assert apply_record["planId"] == plan_preview["id"], (apply_record, plan_preview)
assert apply_record["deploymentName"] == "payments-api-prod", apply_record
assert apply_record["status"] == "failed", apply_record
assert apply_record["error"] == "apply execution is disabled", apply_record
assert isinstance(apply_record.get("id"), str) and apply_record["id"].startswith("apply_"), apply_record
assert isinstance(apply_record.get("createdAt"), str) and apply_record["createdAt"], apply_record
assert apply_record["details"] == [
    "Tenet persisted this immutable apply request in read-only mode and did not contact AWS.",
    "Live execution is intentionally disabled by default for this backend slice.",
], apply_record
assert apply_record["source"]["route"] == f"/v1/plans/{plan_preview['id']}", apply_record
assert apply_record["source"]["planId"] == plan_preview["id"], apply_record
assert apply_record["source"]["planStatus"] == "ready", apply_record
assert apply_record["source"]["fingerprint"] == plan_preview["source"]["fingerprint"], (apply_record, plan_preview)
assert apply_record_get == apply_record, (apply_record_get, apply_record)

assert isinstance(audit_logs.get("entries"), list), audit_logs
assert len(audit_logs["entries"]) == 2, audit_logs
assert {entry["action"] for entry in audit_logs["entries"]} == {"apply.requested", "plan.created"}, audit_logs
for entry in audit_logs["entries"]:
    assert entry["deploymentName"] == "payments-api-prod", entry
    assert entry["appRef"] == "payments-api", entry
    assert entry["environmentRef"] == "prod-apne2", entry
    assert entry["actor"]["available"] is False, entry
    assert "do not persist operator identity" in entry["actor"]["reason"], entry
    assert isinstance(entry.get("metadata", {}), dict), entry
    assert isinstance(entry.get("source", {}), dict), entry

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
    "Plan preview actions",
    "Apply request actions",
    "Plan preview approval guidance",
    "Apply Plan",
    "Open bookmarked workspace",
    "Plan JSON",
    "Open bookmarked apply",
    "Apply JSON",
    "Apply Plan will POST /v1/applies/plans/${planState.record.id}, persist an immutable apply request, and update this workspace URL with a shareable applyId.",
    "Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
    "fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/resource-logs?${params.toString()}`)",
    "params.get('planId') || ''",
    "params.get('applyId') || ''",
):
    assert fragment in ui, fragment

for fragment in (
    "Audit timeline",
    "Operator audit log",
    "Loading audit log…",
    "Audit log is currently unavailable.",
    "Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.",
    "const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
    "if (action) params.set('action', action);",
    "createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
):
    assert fragment in audit_ui, fragment

for fragment in (
    "Plan preview actions",
    "Apply request actions",
    "Plan preview approval guidance",
    "Apply Plan",
    "Open bookmarked workspace",
    "Plan JSON",
    "Open bookmarked apply",
    "Apply JSON",
    "Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.",
    "Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.",
    "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')",
    "createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')",
    "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')",
    "createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')",
    "params.get('planId') || ''",
    "params.get('applyId') || ''",
):
    assert fragment in plan_apply_ui, fragment
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
  if assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$audit_logs_request_id" "$ui_request_id" "$audit_ui_request_id" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$audit_logs_request_id" "$ui_request_id" "$audit_ui_request_id"; then
  echo "smoke-docker: api logs missing correlated structured request entries" >&2
  exit 1
fi

echo "smoke-docker: compose stack path ok"
