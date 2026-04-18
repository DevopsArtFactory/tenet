#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE_FILE=${TENET_COMPOSE_FILE:-$ROOT/compose.yaml}
HOST=${TENET_DOCKER_SMOKE_HOST:-127.0.0.1}
PORT=${TENET_DOCKER_SMOKE_PORT:-8080}
API_BASE_URL=${TENET_DOCKER_SMOKE_API_BASE_URL:-http://$HOST:$PORT}
INVALID_AUDIT_ACTION_BODY=$(mktemp)
PAYLOAD_DIR=$(mktemp -d)

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
  rm -f "$INVALID_AUDIT_ACTION_BODY"
  rm -rf "$PAYLOAD_DIR"
}

trap 'cleanup "$?"' EXIT

write_payload() {
  local name=$1
  local content=$2
  local path="$PAYLOAD_DIR/$name"
  printf '%s' "$content" >"$path"
  printf '%s' "$path"
}

wait_for_api_health() {
  for _ in $(seq 1 60); do
    if curl -fsS "$API_BASE_URL/healthz" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  curl -fsS "$API_BASE_URL/healthz" >/dev/null
}

restart_api_with_apply_mode() {
  local apply_enabled=$1

  TENET_AWS_ENABLE_APPLY="$apply_enabled" "${COMPOSE[@]}" up -d api >/dev/null
  wait_for_api_health
}

"${COMPOSE[@]}" config >/dev/null
services=$("${COMPOSE[@]}" config --services)
for service in api controller redis; do
  if ! grep -qx "$service" <<<"$services"; then
    echo "smoke-docker: expected compose service $service" >&2
    exit 1
  fi
done

"$ROOT/scripts/docker-up.sh" "${COMPOSE[@]}" >/dev/null

wait_for_api_health

healthz_request_id=$(assert_request_id_header "$API_BASE_URL/healthz")
summary_request_id=$(assert_request_id_header "$API_BASE_URL/v1/catalog/summary")
worker_resource_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
ui_request_id=$(assert_request_id_header "$API_BASE_URL/")
audit_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2")

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
worker_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
codedeploy_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
production_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")
test_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")
traffic_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
audit_logs=$(curl -fsS "$API_BASE_URL/v1/audit-logs?limit=2")
audit_logs_action_filtered=$(curl -fsS "$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2")
invalid_audit_action_status=$(curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "$API_BASE_URL/v1/audit-logs?action=invalid.action")
ui=$(curl -fsS "$API_BASE_URL/")
audit_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=audit&action=apply.requested&limit=2")
audit_ui_deployment_request_id=$(assert_request_id_header "$API_BASE_URL/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2")
audit_ui=$(curl -fsS "$API_BASE_URL/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2")
plan_apply_ui=$(curl -fsS "$API_BASE_URL/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id")

summary_path=$(write_payload summary.json "$summary")
applications_path=$(write_payload applications.json "$applications")
deployment_detail_path=$(write_payload deployment-detail.json "$deployment_detail")
plan_preview_path=$(write_payload plan-preview.json "$plan_preview")
plan_preview_get_path=$(write_payload plan-preview-get.json "$plan_preview_get")
apply_record_path=$(write_payload apply-record.json "$apply_record")
apply_record_get_path=$(write_payload apply-record-get.json "$apply_record_get")
deployment_events_path=$(write_payload deployment-events.json "$deployment_events")
worker_resource_logs_path=$(write_payload worker-resource-logs.json "$worker_resource_logs")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
candidate_resource_inspection_path=$(write_payload candidate-resource-inspection.json "$candidate_resource_inspection")
codedeploy_group_resource_inspection_path=$(write_payload codedeploy-group-resource-inspection.json "$codedeploy_group_resource_inspection")
production_listener_resource_inspection_path=$(write_payload production-listener-resource-inspection.json "$production_listener_resource_inspection")
test_listener_resource_inspection_path=$(write_payload test-listener-resource-inspection.json "$test_listener_resource_inspection")
traffic_resource_inspection_path=$(write_payload traffic-resource-inspection.json "$traffic_resource_inspection")
audit_logs_path=$(write_payload audit-logs.json "$audit_logs")
audit_logs_action_filtered_path=$(write_payload audit-logs-action-filtered.json "$audit_logs_action_filtered")
ui_path=$(write_payload ui.html "$ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_apply_ui_path=$(write_payload plan-apply-ui.html "$plan_apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" DEPLOYMENT_EVENTS_PATH="$deployment_events_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH="$codedeploy_group_resource_inspection_path" PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH="$production_listener_resource_inspection_path" TEST_LISTENER_RESOURCE_INSPECTION_PATH="$test_listener_resource_inspection_path" TRAFFIC_RESOURCE_INSPECTION_PATH="$traffic_resource_inspection_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" UI_PATH="$ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_APPLY_UI_PATH="$plan_apply_ui_path" python3 - <<'PY'
import json, os

def read_json(path_env):
    with open(os.environ[path_env], encoding="utf-8") as fh:
        return json.load(fh)


def read_text(path_env):
    with open(os.environ[path_env], encoding="utf-8") as fh:
        return fh.read()

summary = read_json("SUMMARY_PATH")
applications = read_json("APPLICATIONS_PATH")
deployment_detail = read_json("DEPLOYMENT_DETAIL_PATH")
deployment_source_fingerprint = os.environ["DEPLOYMENT_SOURCE_FINGERPRINT"]
plan_preview = read_json("PLAN_PREVIEW_PATH")
plan_preview_get = read_json("PLAN_PREVIEW_GET_PATH")
apply_record = read_json("APPLY_RECORD_PATH")
apply_record_get = read_json("APPLY_RECORD_GET_PATH")
deployment_events = read_json("DEPLOYMENT_EVENTS_PATH")
worker_resource_logs = read_json("WORKER_RESOURCE_LOGS_PATH")
worker_resource_inspection = read_json("WORKER_RESOURCE_INSPECTION_PATH")
candidate_resource_inspection = read_json("CANDIDATE_RESOURCE_INSPECTION_PATH")
codedeploy_group_resource_inspection = read_json("CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH")
production_listener_resource_inspection = read_json("PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH")
test_listener_resource_inspection = read_json("TEST_LISTENER_RESOURCE_INSPECTION_PATH")
traffic_resource_inspection = read_json("TRAFFIC_RESOURCE_INSPECTION_PATH")
audit_logs = read_json("AUDIT_LOGS_PATH")
audit_logs_action_filtered = read_json("AUDIT_LOGS_ACTION_FILTERED_PATH")
invalid_audit_action_body = read_text("INVALID_AUDIT_ACTION_BODY_PATH")
ui = read_text("UI_PATH")
audit_ui = read_text("AUDIT_UI_PATH")
plan_apply_ui = read_text("PLAN_APPLY_UI_PATH")

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
assert isinstance(apply_record.get("operations", []), list) and apply_record["operations"], apply_record
assert len(apply_record["operations"]) == len(plan_preview["operations"]), (apply_record, plan_preview)
assert [operation["status"] for operation in apply_record["operations"]] == ["pending"] * len(apply_record["operations"]), apply_record
assert [operation["phase"] for operation in apply_record["operations"]] == [operation["phase"] for operation in plan_preview["operations"]], (apply_record, plan_preview)
assert [operation["target"] for operation in apply_record["operations"]] == [operation["target"] for operation in plan_preview["operations"]], (apply_record, plan_preview)
assert [operation["summary"] for operation in apply_record["operations"]] == [operation["summary"] for operation in plan_preview["operations"]], (apply_record, plan_preview)
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
assert isinstance(audit_logs_action_filtered.get("entries"), list), audit_logs_action_filtered
assert len(audit_logs_action_filtered["entries"]) == 1, audit_logs_action_filtered
assert audit_logs_action_filtered["entries"][0]["action"] == "apply.requested", audit_logs_action_filtered
assert os.environ["INVALID_AUDIT_ACTION_STATUS"] == "400", os.environ["INVALID_AUDIT_ACTION_STATUS"]
assert "invalid action query" in invalid_audit_action_body, invalid_audit_action_body
for entry in audit_logs["entries"]:
    assert entry["deploymentName"] == "payments-api-prod", entry
    assert entry["appRef"] == "payments-api", entry
    assert entry["environmentRef"] == "prod-apne2", entry
    assert entry["actor"]["available"] is False, entry
    assert "no authenticated request context" in entry["actor"]["reason"], entry
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

assert worker_resource_inspection["deploymentName"] == "worker-prod", worker_resource_inspection
assert worker_resource_inspection["resourceType"] == "ecsService", worker_resource_inspection
assert worker_resource_inspection["resourceName"] == "worker-prod-service", worker_resource_inspection
assert isinstance(worker_resource_inspection["available"], bool), worker_resource_inspection
assert isinstance(worker_resource_inspection.get("lookupContext"), dict), worker_resource_inspection
if worker_resource_inspection["available"]:
    assert worker_resource_inspection.get("source") == "ecs_service", worker_resource_inspection
    assert isinstance(worker_resource_inspection.get("ecsService"), dict), worker_resource_inspection
else:
    assert isinstance(worker_resource_inspection.get("unavailableReason"), str) and worker_resource_inspection["unavailableReason"], worker_resource_inspection

assert candidate_resource_inspection["deploymentName"] == "payments-api-prod", candidate_resource_inspection
assert candidate_resource_inspection["resourceType"] == "candidateAutoScalingGroup", candidate_resource_inspection
assert candidate_resource_inspection["resourceName"] == "payments-api-prod-candidate", candidate_resource_inspection
assert isinstance(candidate_resource_inspection["available"], bool), candidate_resource_inspection
assert isinstance(candidate_resource_inspection.get("lookupContext"), dict), candidate_resource_inspection
if candidate_resource_inspection["available"]:
    assert candidate_resource_inspection.get("source") == "auto_scaling_group", candidate_resource_inspection
    assert isinstance(candidate_resource_inspection.get("autoScalingGroup"), dict), candidate_resource_inspection
    launch_template = candidate_resource_inspection["autoScalingGroup"].get("launchTemplate")
    assert launch_template is None or isinstance(launch_template, dict), candidate_resource_inspection
    if isinstance(launch_template, dict):
        assert any(isinstance(launch_template.get(key), str) and launch_template.get(key) for key in ("id", "name", "version")), candidate_resource_inspection
    mixed_instances_policy = candidate_resource_inspection["autoScalingGroup"].get("mixedInstancesPolicy")
    assert mixed_instances_policy is None or isinstance(mixed_instances_policy, dict), candidate_resource_inspection
    if isinstance(mixed_instances_policy, dict):
        base_launch_template = mixed_instances_policy.get("launchTemplate")
        assert base_launch_template is None or isinstance(base_launch_template, dict), candidate_resource_inspection
        overrides = mixed_instances_policy.get("overrides")
        assert overrides is None or isinstance(overrides, list), candidate_resource_inspection
        for override in overrides or []:
            assert isinstance(override, dict), candidate_resource_inspection
            override_launch_template = override.get("launchTemplate")
            assert override_launch_template is None or isinstance(override_launch_template, dict), candidate_resource_inspection
else:
    assert isinstance(candidate_resource_inspection.get("unavailableReason"), str) and candidate_resource_inspection["unavailableReason"], candidate_resource_inspection

def assert_summary_traffic_routing(payload, deployment_name, resource_type, resource_name):
    assert payload["deploymentName"] == deployment_name, payload
    assert payload["resourceType"] == resource_type, payload
    assert payload["resourceName"] == resource_name, payload
    assert payload["available"] is True, payload
    assert isinstance(payload.get("lookupContext"), dict), payload
    assert payload.get("source") == "rollout_execution_summary", payload
    assert isinstance(payload.get("trafficRoutingResource"), dict), payload
    assert payload["trafficRoutingResource"].get("kind") == resource_type, payload
    assert payload["trafficRoutingResource"].get("inspectionMode") == "summary_only", payload
    assert isinstance(payload["trafficRoutingResource"].get("summary"), str) and payload["trafficRoutingResource"]["summary"], payload

assert_summary_traffic_routing(codedeploy_group_resource_inspection, "payments-api-staging", "codeDeployDeploymentGroup", "payments-api-staging-codedeploy-group")
assert_summary_traffic_routing(production_listener_resource_inspection, "payments-api-staging", "productionListener", "payments-api-staging-prod-listener")
assert_summary_traffic_routing(test_listener_resource_inspection, "payments-api-staging", "testListener", "payments-api-staging-test-listener")
assert traffic_resource_inspection["deploymentName"] == "payments-api-prod", traffic_resource_inspection
assert traffic_resource_inspection["resourceType"] == "trafficShiftPlan", traffic_resource_inspection
assert traffic_resource_inspection["resourceName"] == "payments-api-prod-weighted-routing", traffic_resource_inspection
assert traffic_resource_inspection["available"] is True, traffic_resource_inspection
assert isinstance(traffic_resource_inspection.get("lookupContext"), dict), traffic_resource_inspection
assert traffic_resource_inspection.get("source") == "rollout_execution_summary", traffic_resource_inspection
assert isinstance(traffic_resource_inspection.get("trafficRoutingResource"), dict), traffic_resource_inspection
assert traffic_resource_inspection["trafficRoutingResource"].get("kind") == "trafficShiftPlan", traffic_resource_inspection
assert traffic_resource_inspection["trafficRoutingResource"].get("inspectionMode") == "summary_only", traffic_resource_inspection
assert isinstance(traffic_resource_inspection["trafficRoutingResource"].get("summary"), str) and traffic_resource_inspection["trafficRoutingResource"]["summary"], traffic_resource_inspection

for fragment in (
    "Open account",
    "Open application",
    "Drift view",
    "Plan/apply",
    "Controller runtime events",
    "Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.",
    "Recent logs",
    "Lookup region",
    "Observer role configured",
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
    "Deployment audit timeline",
    "Operator audit log",
    "Audit filters: ${activeFilters.join(' · ')}",
    "Active deployment context",
    "Loading audit log…",
    "Audit log is currently unavailable.",
    "Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.",
    "Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.",
    "No audit activity has been persisted yet for deployment ${currentPageState.deployment}.",
    "action ${currentPageState.auditAction}",
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

restart_api_with_apply_mode true

queued_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/payments-api-prod")
queued_plan_id=$(QUEUED_PLAN_PREVIEW_JSON="$queued_plan_preview" python3 - <<'PY'
import json
import os

print(json.loads(os.environ["QUEUED_PLAN_PREVIEW_JSON"])["id"])
PY
)
queued_plan_preview_get=$(curl -fsS "$API_BASE_URL/v1/plans/$queued_plan_id")
queued_apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$queued_plan_id")
queued_apply_id=$(QUEUED_APPLY_RECORD_JSON="$queued_apply_record" python3 - <<'PY'
import json
import os

print(json.loads(os.environ["QUEUED_APPLY_RECORD_JSON"])["id"])
PY
)
queued_apply_record_get=$(curl -fsS "$API_BASE_URL/v1/applies/$queued_apply_id")

QUEUED_PLAN_PREVIEW_JSON="$queued_plan_preview" \
QUEUED_PLAN_PREVIEW_GET_JSON="$queued_plan_preview_get" \
QUEUED_APPLY_RECORD_JSON="$queued_apply_record" \
QUEUED_APPLY_RECORD_GET_JSON="$queued_apply_record_get" \
python3 - <<'PY'
import json
import os

queued_plan_preview = json.loads(os.environ["QUEUED_PLAN_PREVIEW_JSON"])
queued_plan_preview_get = json.loads(os.environ["QUEUED_PLAN_PREVIEW_GET_JSON"])
queued_apply_record = json.loads(os.environ["QUEUED_APPLY_RECORD_JSON"])
queued_apply_record_get = json.loads(os.environ["QUEUED_APPLY_RECORD_GET_JSON"])

assert queued_plan_preview["deploymentName"] == "payments-api-prod", queued_plan_preview
assert queued_plan_preview["status"] == "ready", queued_plan_preview
assert isinstance(queued_plan_preview.get("operations", []), list) and queued_plan_preview["operations"], queued_plan_preview
assert queued_plan_preview_get == queued_plan_preview, (queued_plan_preview_get, queued_plan_preview)

assert queued_apply_record["planId"] == queued_plan_preview["id"], (queued_apply_record, queued_plan_preview)
assert queued_apply_record["deploymentName"] == "payments-api-prod", queued_apply_record
assert queued_apply_record["status"] == "queued", queued_apply_record
assert not queued_apply_record.get("error"), queued_apply_record
assert isinstance(queued_apply_record.get("id"), str) and queued_apply_record["id"].startswith("apply_"), queued_apply_record
assert isinstance(queued_apply_record.get("createdAt"), str) and queued_apply_record["createdAt"], queued_apply_record
assert queued_apply_record["details"] == [
    "Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true.",
    "No AWS mutation has been implemented for this backend slice yet.",
], queued_apply_record
assert isinstance(queued_apply_record.get("operations", []), list) and queued_apply_record["operations"], queued_apply_record
assert len(queued_apply_record["operations"]) == len(queued_plan_preview["operations"]), (queued_apply_record, queued_plan_preview)
assert [operation["status"] for operation in queued_apply_record["operations"]] == ["queued"] * len(queued_apply_record["operations"]), queued_apply_record
assert [operation["phase"] for operation in queued_apply_record["operations"]] == [operation["phase"] for operation in queued_plan_preview["operations"]], (queued_apply_record, queued_plan_preview)
assert [operation["target"] for operation in queued_apply_record["operations"]] == [operation["target"] for operation in queued_plan_preview["operations"]], (queued_apply_record, queued_plan_preview)
assert [operation["summary"] for operation in queued_apply_record["operations"]] == [operation["summary"] for operation in queued_plan_preview["operations"]], (queued_apply_record, queued_plan_preview)
assert queued_apply_record["source"]["route"] == f"/v1/plans/{queued_plan_preview['id']}", queued_apply_record
assert queued_apply_record["source"]["planId"] == queued_plan_preview["id"], queued_apply_record
assert queued_apply_record["source"]["planStatus"] == "ready", queued_apply_record
assert queued_apply_record["source"]["fingerprint"] == queued_plan_preview["source"]["fingerprint"], (queued_apply_record, queued_plan_preview)
assert queued_apply_record_get == queued_apply_record, (queued_apply_record_get, queued_apply_record)
PY

echo "smoke-docker: compose stack path ok"
