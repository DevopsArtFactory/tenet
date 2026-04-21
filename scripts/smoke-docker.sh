#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE_FILE=${TENET_COMPOSE_FILE:-$ROOT/compose.yaml}
HOST=${TENET_DOCKER_SMOKE_HOST:-127.0.0.1}
PORT=${TENET_DOCKER_SMOKE_PORT:-8080}
API_BASE_URL=${TENET_DOCKER_SMOKE_API_BASE_URL:-http://$HOST:$PORT}
WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}
INVALID_AUDIT_ACTION_BODY=$(mktemp)
INVALID_APPLY_TRANSITION_BODY=$(mktemp)
UNKNOWN_APPLY_TRANSITION_BODY=$(mktemp)
TERMINAL_APPLY_TRANSITION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_TRANSITION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_EVENTS_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY=$(mktemp)
AUTH_FORBIDDEN_STATUS_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_TRANSITION_BODY=$(mktemp)
AUTH_FORBIDDEN_EVENTS_BODY=$(mktemp)
AUTH_FORBIDDEN_RESOURCE_LOGS_BODY=$(mktemp)
AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=$(mktemp)
WEBHOOK_PUSH_BODY_FILE=$(mktemp)
WEBHOOK_PING_BODY_FILE=$(mktemp)
INVALID_WEBHOOK_BODY=$(mktemp)
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
  local application_ui_request_id=$6
  local account_ui_request_id=$7
  local drift_ui_request_id=$8
  local audit_ui_request_id=$9
  local plan_apply_ui_request_id=${10}

  API_LOGS_TEXT="$logs_text" \
  HEALTHZ_REQUEST_ID="$healthz_request_id" \
  SUMMARY_REQUEST_ID="$summary_request_id" \
  WORKER_RESOURCE_LOGS_REQUEST_ID="$worker_resource_logs_request_id" \
  AUDIT_LOGS_REQUEST_ID="$audit_logs_request_id" \
  APPLICATION_UI_REQUEST_ID="$application_ui_request_id" \
  ACCOUNT_UI_REQUEST_ID="$account_ui_request_id" \
  DRIFT_UI_REQUEST_ID="$drift_ui_request_id" \
  AUDIT_UI_REQUEST_ID="$audit_ui_request_id" \
  PLAN_APPLY_UI_REQUEST_ID="$plan_apply_ui_request_id" \
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
    (os.environ["APPLICATION_UI_REQUEST_ID"], "/"),
    (os.environ["ACCOUNT_UI_REQUEST_ID"], "/"),
    (os.environ["DRIFT_UI_REQUEST_ID"], "/"),
    (os.environ["AUDIT_UI_REQUEST_ID"], "/"),
    (os.environ["PLAN_APPLY_UI_REQUEST_ID"], "/"),
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
    "${COMPOSE[@]}" logs controller --tail=200 >&2 || true
  fi

  "${COMPOSE[@]}" down --remove-orphans >/dev/null 2>&1 || true
  rm -f "$INVALID_AUDIT_ACTION_BODY" "$INVALID_APPLY_TRANSITION_BODY" "$UNKNOWN_APPLY_TRANSITION_BODY" "$TERMINAL_APPLY_TRANSITION_BODY" "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" "$AUTH_FORBIDDEN_STATUS_READ_BODY" "$AUTH_FORBIDDEN_TRANSITION_BODY" "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" "$WEBHOOK_PUSH_BODY_FILE" "$WEBHOOK_PING_BODY_FILE" "$INVALID_WEBHOOK_BODY"
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

json_string_field_from_path() {
  local path=$1
  local field=$2

  python3 - "$path" "$field" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
value = payload.get(sys.argv[2])
if value is None:
    print("")
elif not isinstance(value, str):
    raise SystemExit(f"expected string field {sys.argv[2]!r} in {sys.argv[1]}")
else:
    print(value.strip())
PY
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

wait_for_apply_status() {
  local apply_id=$1
  local expected_status=$2
  local response='{}'
  local response_path

  for _ in $(seq 1 50); do
    response=$(curl -fsS "$API_BASE_URL/v1/applies/$apply_id")
    response_path=$(write_payload wait-for-apply-status.json "$response")
    current_status=$(json_string_field_from_path "$response_path" status)
    if [[ "$current_status" == "$expected_status" ]]; then
      printf '%s' "$response"
      return 0
    fi
    sleep 0.1
  done

  printf '%s' "$response"
  return 1
}

restart_api_with_apply_mode() {
  local apply_enabled=$1

  TENET_AWS_ENABLE_APPLY="$apply_enabled" "${COMPOSE[@]}" up -d api >/dev/null
  wait_for_api_health
}

restart_api_with_frontdoor_read_auth() {
  TENET_AWS_ENABLE_APPLY=false \
  TENET_API_TRUST_AUTHENTICATED_HEADERS=true \
  TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=false \
  TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=false \
  TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \
  "${COMPOSE[@]}" up -d api >/dev/null
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
audit_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2")

summary=$(curl -fsS "$API_BASE_URL/v1/catalog/summary")
applications=$(curl -fsS "$API_BASE_URL/v1/overview/applications")
deployment_detail=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod")
deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")
deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)
plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'
import json
import os

fingerprint = (os.environ.get("DEPLOYMENT_SOURCE_FINGERPRINT") or "").strip()
print(json.dumps({"ifMatch": fingerprint} if fingerprint else {}))
PY
)
plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/payments-api-prod")
plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")
plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)
plan_preview_get=$(curl -fsS "$API_BASE_URL/v1/plans/$plan_id")
plan_preview_get_extract_path=$(write_payload plan-preview-get.extract.json "$plan_preview_get")
plan_status=$(PLAN_PREVIEW_PATH="$plan_preview_extract_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_extract_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'
import json
import os

with open(os.environ["PLAN_PREVIEW_PATH"], encoding="utf-8") as fh:
    plan_preview = json.load(fh)
with open(os.environ["PLAN_PREVIEW_GET_PATH"], encoding="utf-8") as fh:
    plan_preview_get = json.load(fh)
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
apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")
apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)
apply_record_get=$(curl -fsS "$API_BASE_URL/v1/applies/$apply_id")
invalid_apply_transition_status=$(curl -sS -o "$INVALID_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1' \
  "$API_BASE_URL/v1/applies/$apply_id/transitions")
unknown_apply_transition_status=$(curl -sS -o "$UNKNOWN_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "$API_BASE_URL/v1/applies/apply_smoke_missing/transitions")
terminal_apply_transition_status=$(curl -sS -o "$TERMINAL_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "$API_BASE_URL/v1/applies/$apply_id/transitions")
actor_plan_preview=$(curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-Tenet-Actor-Id: explicit-456' \
  -H 'X-Tenet-Actor-Name: Alex Explicit' \
  -H 'X-Tenet-Actor-Email: alex.explicit@example.com' \
  -H 'X-User-ID: operator-123' \
  -H 'X-User-Name: Jane Operator' \
  -H 'X-User-Email: jane@example.com' \
  -d '{}' \
  "$API_BASE_URL/v1/plans/deployments/payments-api-staging")
actor_plan_preview_extract_path=$(write_payload actor-plan-preview.extract.json "$actor_plan_preview")
actor_plan_id=$(json_string_field_from_path "$actor_plan_preview_extract_path" id)
actor_plan_preview_get=$(curl -fsS "$API_BASE_URL/v1/plans/$actor_plan_id")
actor_apply_record=$(curl -fsS -X POST \
  -H 'X-Tenet-Actor-Id: explicit-456' \
  -H 'X-Tenet-Actor-Name: Alex Explicit' \
  -H 'X-Tenet-Actor-Email: alex.explicit@example.com' \
  -H 'X-User-ID: operator-123' \
  -H 'X-User-Name: Jane Operator' \
  -H 'X-User-Email: jane@example.com' \
  "$API_BASE_URL/v1/applies/plans/$actor_plan_id")
actor_apply_record_extract_path=$(write_payload actor-apply-record.extract.json "$actor_apply_record")
actor_apply_id=$(json_string_field_from_path "$actor_apply_record_extract_path" id)
actor_apply_record_get=$(curl -fsS "$API_BASE_URL/v1/applies/$actor_apply_id")
deployment_events=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/events")
worker_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
worker_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
bootstrap_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
bootstrap_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
codedeploy_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
production_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")
test_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")
traffic_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
audit_logs=$(curl -fsS "$API_BASE_URL/v1/audit-logs")
audit_logs_action_filtered=$(curl -fsS "$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2")
actor_audit_logs=$(curl -fsS "$API_BASE_URL/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
invalid_audit_action_status=$(curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "$API_BASE_URL/v1/audit-logs?action=invalid.action")
webhook_push_body=$(cat <<'EOF'
{"ref":"refs/heads/main","before":"1111111111111111111111111111111111111111","after":"2222222222222222222222222222222222222222","repository":{"full_name":"acme/platform-config"},"head_commit":{"id":"2222222222222222222222222222222222222222","message":"update deployment","timestamp":"2026-04-18T15:00:00Z"},"commits":[{"id":"a","added":["deployments/api.yaml"],"modified":["environments/prod.yaml"],"removed":[]},{"id":"b","added":["accounts/prod.yaml"],"modified":["deployments/worker.yaml"],"removed":["deployments/old.yaml"]}]}
EOF
)
webhook_push_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_push_body" python3 - <<'PY'
import hashlib
import hmac
import os

print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_push_status=$(curl -sS -o "$WEBHOOK_PUSH_BODY_FILE" -w "%{http_code}" -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-push-1' \
  -H 'X-GitHub-Event: push' \
  -H "X-Hub-Signature-256: $webhook_push_signature" \
  --data "$webhook_push_body" \
  "$API_BASE_URL/api/v1/github/webhook")
webhook_push=$(cat "$WEBHOOK_PUSH_BODY_FILE")
webhook_ping_body='{"zen":"keep it logically awesome"}'
webhook_ping_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_ping_body" python3 - <<'PY'
import hashlib
import hmac
import os

print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_ping_status=$(curl -sS -o "$WEBHOOK_PING_BODY_FILE" -w "%{http_code}" -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-ping-1' \
  -H 'X-GitHub-Event: ping' \
  -H "X-Hub-Signature-256: $webhook_ping_signature" \
  --data "$webhook_ping_body" \
  "$API_BASE_URL/api/v1/github/webhook")
webhook_ping=$(cat "$WEBHOOK_PING_BODY_FILE")
invalid_webhook_status=$(curl -sS -o "$INVALID_WEBHOOK_BODY" -w "%{http_code}" -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-invalid-1' \
  -H 'X-GitHub-Event: push' \
  -H 'X-Hub-Signature-256: sha256=deadbeef' \
  --data "$webhook_push_body" \
  "$API_BASE_URL/api/v1/github/webhook")
invalid_webhook=$(cat "$INVALID_WEBHOOK_BODY")
ui=$(curl -fsS "$API_BASE_URL/")
application_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=application&deployment=payments-api-prod")
application_ui=$(curl -fsS "$API_BASE_URL/?page=application&deployment=payments-api-prod")
account_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=account&accountId=123456789012")
account_ui=$(curl -fsS "$API_BASE_URL/?page=account&accountId=123456789012")
resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan%3A%3Apayments-api-prod-weighted-routing%3A%3AWeighted+routing+plan")
ecs_resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService%3A%3Aworker-prod-service%3A%3AECS+service+Tenet+updates+with+native+rolling+deployment")
bootstrap_resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource%3A%3Afile%3Auserdata%2Fpayments-api-prod.sh%3A%3ANormalized+userdata+bootstrap+source+for+candidate+EC2+capacity")
drift_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=drift&deployment=payments-api-prod")
drift_ui=$(curl -fsS "$API_BASE_URL/?page=drift&deployment=payments-api-prod")
audit_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2")
audit_ui=$(curl -fsS "$API_BASE_URL/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2")
plan_apply_ui_request_id=$(assert_request_id_header "$API_BASE_URL/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id")
plan_apply_ui=$(curl -fsS "$API_BASE_URL/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id")

summary_path=$(write_payload summary.json "$summary")
applications_path=$(write_payload applications.json "$applications")
deployment_detail_path=$(write_payload deployment-detail.json "$deployment_detail")
plan_preview_path=$(write_payload plan-preview.json "$plan_preview")
plan_preview_get_path=$(write_payload plan-preview-get.json "$plan_preview_get")
apply_record_path=$(write_payload apply-record.json "$apply_record")
apply_record_get_path=$(write_payload apply-record-get.json "$apply_record_get")
invalid_apply_transition_path=$(write_payload invalid-apply-transition.txt "$(cat "$INVALID_APPLY_TRANSITION_BODY")")
unknown_apply_transition_path=$(write_payload unknown-apply-transition.txt "$(cat "$UNKNOWN_APPLY_TRANSITION_BODY")")
terminal_apply_transition_path=$(write_payload terminal-apply-transition.txt "$(cat "$TERMINAL_APPLY_TRANSITION_BODY")")
actor_plan_preview_path=$(write_payload actor-plan-preview.json "$actor_plan_preview")
actor_plan_preview_get_path=$(write_payload actor-plan-preview-get.json "$actor_plan_preview_get")
actor_apply_record_path=$(write_payload actor-apply-record.json "$actor_apply_record")
actor_apply_record_get_path=$(write_payload actor-apply-record-get.json "$actor_apply_record_get")
deployment_events_path=$(write_payload deployment-events.json "$deployment_events")
worker_resource_logs_path=$(write_payload worker-resource-logs.json "$worker_resource_logs")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
bootstrap_resource_logs_path=$(write_payload bootstrap-resource-logs.json "$bootstrap_resource_logs")
bootstrap_resource_inspection_path=$(write_payload bootstrap-resource-inspection.json "$bootstrap_resource_inspection")
candidate_resource_inspection_path=$(write_payload candidate-resource-inspection.json "$candidate_resource_inspection")
codedeploy_group_resource_inspection_path=$(write_payload codedeploy-group-resource-inspection.json "$codedeploy_group_resource_inspection")
production_listener_resource_inspection_path=$(write_payload production-listener-resource-inspection.json "$production_listener_resource_inspection")
test_listener_resource_inspection_path=$(write_payload test-listener-resource-inspection.json "$test_listener_resource_inspection")
traffic_resource_inspection_path=$(write_payload traffic-resource-inspection.json "$traffic_resource_inspection")
audit_logs_path=$(write_payload audit-logs.json "$audit_logs")
audit_logs_action_filtered_path=$(write_payload audit-logs-action-filtered.json "$audit_logs_action_filtered")
actor_audit_logs_path=$(write_payload actor-audit-logs.json "$actor_audit_logs")
webhook_push_path=$(write_payload webhook-push.json "$webhook_push")
webhook_ping_path=$(write_payload webhook-ping.json "$webhook_ping")
invalid_webhook_path=$(write_payload invalid-webhook.txt "$invalid_webhook")
ui_path=$(write_payload ui.html "$ui")
application_ui_path=$(write_payload application-ui.html "$application_ui")
account_ui_path=$(write_payload account-ui.html "$account_ui")
resource_ui_path=$(write_payload resource-ui.html "$resource_ui")
ecs_resource_ui_path=$(write_payload ecs-resource-ui.html "$ecs_resource_ui")
bootstrap_resource_ui_path=$(write_payload bootstrap-resource-ui.html "$bootstrap_resource_ui")
drift_ui_path=$(write_payload drift-ui.html "$drift_ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_apply_ui_path=$(write_payload plan-apply-ui.html "$plan_apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" INVALID_APPLY_TRANSITION_STATUS="$invalid_apply_transition_status" INVALID_APPLY_TRANSITION_PATH="$invalid_apply_transition_path" UNKNOWN_APPLY_TRANSITION_STATUS="$unknown_apply_transition_status" UNKNOWN_APPLY_TRANSITION_PATH="$unknown_apply_transition_path" TERMINAL_APPLY_TRANSITION_STATUS="$terminal_apply_transition_status" TERMINAL_APPLY_TRANSITION_PATH="$terminal_apply_transition_path" ACTOR_PLAN_PREVIEW_PATH="$actor_plan_preview_path" ACTOR_PLAN_PREVIEW_GET_PATH="$actor_plan_preview_get_path" ACTOR_APPLY_RECORD_PATH="$actor_apply_record_path" ACTOR_APPLY_RECORD_GET_PATH="$actor_apply_record_get_path" DEPLOYMENT_EVENTS_PATH="$deployment_events_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" BOOTSTRAP_RESOURCE_LOGS_PATH="$bootstrap_resource_logs_path" BOOTSTRAP_RESOURCE_INSPECTION_PATH="$bootstrap_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH="$codedeploy_group_resource_inspection_path" PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH="$production_listener_resource_inspection_path" TEST_LISTENER_RESOURCE_INSPECTION_PATH="$test_listener_resource_inspection_path" TRAFFIC_RESOURCE_INSPECTION_PATH="$traffic_resource_inspection_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" ACTOR_AUDIT_LOGS_PATH="$actor_audit_logs_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" WEBHOOK_PUSH_PATH="$webhook_push_path" WEBHOOK_PUSH_STATUS="$webhook_push_status" WEBHOOK_PING_PATH="$webhook_ping_path" WEBHOOK_PING_STATUS="$webhook_ping_status" INVALID_WEBHOOK_PATH="$invalid_webhook_path" INVALID_WEBHOOK_STATUS="$invalid_webhook_status" UI_PATH="$ui_path" APPLICATION_UI_PATH="$application_ui_path" ACCOUNT_UI_PATH="$account_ui_path" RESOURCE_UI_PATH="$resource_ui_path" ECS_RESOURCE_UI_PATH="$ecs_resource_ui_path" BOOTSTRAP_RESOURCE_UI_PATH="$bootstrap_resource_ui_path" DRIFT_UI_PATH="$drift_ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_APPLY_UI_PATH="$plan_apply_ui_path" python3 - <<'PY'
import json, os

def read_json(path_env):
    with open(os.environ[path_env], encoding="utf-8") as fh:
        return json.load(fh)


def read_text(path_env):
    with open(os.environ[path_env], encoding="utf-8") as fh:
        return fh.read()


def assert_fragments(label, body, fragments):
    for fragment in fragments:
        assert fragment in body, {"label": label, "missing": fragment}


summary = read_json("SUMMARY_PATH")
applications = read_json("APPLICATIONS_PATH")
deployment_detail = read_json("DEPLOYMENT_DETAIL_PATH")
deployment_source_fingerprint = os.environ["DEPLOYMENT_SOURCE_FINGERPRINT"]
plan_preview = read_json("PLAN_PREVIEW_PATH")
plan_preview_get = read_json("PLAN_PREVIEW_GET_PATH")
apply_record = read_json("APPLY_RECORD_PATH")
apply_record_get = read_json("APPLY_RECORD_GET_PATH")
invalid_apply_transition = read_text("INVALID_APPLY_TRANSITION_PATH")
unknown_apply_transition = read_text("UNKNOWN_APPLY_TRANSITION_PATH")
terminal_apply_transition = read_text("TERMINAL_APPLY_TRANSITION_PATH")
actor_plan_preview = read_json("ACTOR_PLAN_PREVIEW_PATH")
actor_plan_preview_get = read_json("ACTOR_PLAN_PREVIEW_GET_PATH")
actor_apply_record = read_json("ACTOR_APPLY_RECORD_PATH")
actor_apply_record_get = read_json("ACTOR_APPLY_RECORD_GET_PATH")
deployment_events = read_json("DEPLOYMENT_EVENTS_PATH")
worker_resource_logs = read_json("WORKER_RESOURCE_LOGS_PATH")
worker_resource_inspection = read_json("WORKER_RESOURCE_INSPECTION_PATH")
bootstrap_resource_logs = read_json("BOOTSTRAP_RESOURCE_LOGS_PATH")
bootstrap_resource_inspection = read_json("BOOTSTRAP_RESOURCE_INSPECTION_PATH")
candidate_resource_inspection = read_json("CANDIDATE_RESOURCE_INSPECTION_PATH")
codedeploy_group_resource_inspection = read_json("CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH")
production_listener_resource_inspection = read_json("PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH")
test_listener_resource_inspection = read_json("TEST_LISTENER_RESOURCE_INSPECTION_PATH")
traffic_resource_inspection = read_json("TRAFFIC_RESOURCE_INSPECTION_PATH")
audit_logs = read_json("AUDIT_LOGS_PATH")
audit_logs_action_filtered = read_json("AUDIT_LOGS_ACTION_FILTERED_PATH")
actor_audit_logs = read_json("ACTOR_AUDIT_LOGS_PATH")
invalid_audit_action_body = read_text("INVALID_AUDIT_ACTION_BODY_PATH")
webhook_push = read_json("WEBHOOK_PUSH_PATH")
webhook_ping = read_json("WEBHOOK_PING_PATH")
invalid_webhook = read_text("INVALID_WEBHOOK_PATH")
ui = read_text("UI_PATH")
application_ui = read_text("APPLICATION_UI_PATH")
account_ui = read_text("ACCOUNT_UI_PATH")
resource_ui = read_text("RESOURCE_UI_PATH")
ecs_resource_ui = read_text("ECS_RESOURCE_UI_PATH")
bootstrap_resource_ui = read_text("BOOTSTRAP_RESOURCE_UI_PATH")
drift_ui = read_text("DRIFT_UI_PATH")
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
managed_drift = deployment_detail.get("managedDrift")
runtime_managed_drift = (deployment_detail.get("runtimeSummary") or {}).get("managedDrift")
assert isinstance(managed_drift, list) and managed_drift, deployment_detail
assert isinstance(runtime_managed_drift, list) and runtime_managed_drift, deployment_detail
assert runtime_managed_drift == managed_drift, deployment_detail
assert all(isinstance(row, dict) for row in managed_drift), managed_drift
assert all(isinstance(row.get("field"), str) and row["field"] for row in managed_drift), managed_drift
assert all(isinstance(row.get("desired"), str) and row["desired"] for row in managed_drift), managed_drift
assert all(isinstance(row.get("actual"), str) and row["actual"] for row in managed_drift), managed_drift
assert all(row.get("severity") in {"high", "medium", "low"} for row in managed_drift), managed_drift

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
assert os.environ["INVALID_APPLY_TRANSITION_STATUS"] == "400", os.environ["INVALID_APPLY_TRANSITION_STATUS"]
assert invalid_apply_transition.strip() == "invalid apply transition request body", invalid_apply_transition
assert os.environ["UNKNOWN_APPLY_TRANSITION_STATUS"] == "404", os.environ["UNKNOWN_APPLY_TRANSITION_STATUS"]
assert "apply request not found" in unknown_apply_transition, unknown_apply_transition
assert os.environ["TERMINAL_APPLY_TRANSITION_STATUS"] == "409", os.environ["TERMINAL_APPLY_TRANSITION_STATUS"]
assert "apply request lifecycle transition is invalid:" in terminal_apply_transition, terminal_apply_transition
assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition
assert actor_plan_preview["deploymentName"] == "payments-api-staging", actor_plan_preview
assert actor_plan_preview["actor"]["available"] is True, actor_plan_preview
assert actor_plan_preview["actor"]["id"] == "explicit-456", actor_plan_preview
assert actor_plan_preview["actor"]["name"] == "Alex Explicit", actor_plan_preview
assert actor_plan_preview["actor"]["email"] == "alex.explicit@example.com", actor_plan_preview
assert actor_plan_preview_get == actor_plan_preview, (actor_plan_preview_get, actor_plan_preview)
assert actor_apply_record["deploymentName"] == "payments-api-staging", actor_apply_record
assert actor_apply_record["actor"]["available"] is True, actor_apply_record
assert actor_apply_record["actor"]["id"] == "explicit-456", actor_apply_record
assert actor_apply_record["actor"]["name"] == "Alex Explicit", actor_apply_record
assert actor_apply_record["actor"]["email"] == "alex.explicit@example.com", actor_apply_record
assert isinstance(actor_apply_record.get("operations", []), list) and actor_apply_record["operations"], actor_apply_record
assert all(operation["status"] == "pending" for operation in actor_apply_record["operations"]), actor_apply_record
assert actor_apply_record_get == actor_apply_record, (actor_apply_record_get, actor_apply_record)

assert isinstance(audit_logs.get("entries"), list), audit_logs
assert any(entry["action"] == "apply.requested" for entry in audit_logs["entries"]), audit_logs
assert any(entry["action"] == "plan.created" for entry in audit_logs["entries"]), audit_logs
assert isinstance(audit_logs_action_filtered.get("entries"), list), audit_logs_action_filtered
assert 1 <= len(audit_logs_action_filtered["entries"]) <= 2, audit_logs_action_filtered
assert all(entry["action"] == "apply.requested" for entry in audit_logs_action_filtered["entries"]), audit_logs_action_filtered
assert os.environ["INVALID_AUDIT_ACTION_STATUS"] == "400", os.environ["INVALID_AUDIT_ACTION_STATUS"]
assert "invalid action query" in invalid_audit_action_body, invalid_audit_action_body
assert os.environ["WEBHOOK_PUSH_STATUS"] == "202", os.environ["WEBHOOK_PUSH_STATUS"]
assert webhook_push["deliveryId"] == "delivery-push-1", webhook_push
assert webhook_push["event"] == "push", webhook_push
assert webhook_push["status"] == "accepted", webhook_push
assert webhook_push["processed"] is False, webhook_push
assert webhook_push["repository"] == "acme/platform-config", webhook_push
assert webhook_push["ref"] == "refs/heads/main", webhook_push
assert webhook_push["before"] == "1111111111111111111111111111111111111111", webhook_push
assert webhook_push["after"] == "2222222222222222222222222222222222222222", webhook_push
assert webhook_push["headCommit"]["id"] == "2222222222222222222222222222222222222222", webhook_push
assert webhook_push["changedFiles"]["all"] == ["deployments/api.yaml", "accounts/prod.yaml", "environments/prod.yaml", "deployments/worker.yaml", "deployments/old.yaml"], webhook_push
assert webhook_push["desiredStateSourceMatch"]["configured"] is True, webhook_push
assert webhook_push["desiredStateSourceMatch"]["kind"] == "filesystem", webhook_push
assert webhook_push["desiredStateSourceMatch"]["matches"] is False, webhook_push
assert webhook_push["desiredStateSourceMatch"]["matchStatus"] == "filesystem_source", webhook_push
assert webhook_push["desiredStateSourceMatch"]["reason"] == "desired-state source uses the filesystem catalog, so GitHub push relevance is unavailable in this mode", webhook_push
assert os.environ["WEBHOOK_PING_STATUS"] == "200", os.environ["WEBHOOK_PING_STATUS"]
assert webhook_ping["deliveryId"] == "delivery-ping-1", webhook_ping
assert webhook_ping["event"] == "ping", webhook_ping
assert webhook_ping["status"] == "ignored", webhook_ping
assert webhook_ping["processed"] is False, webhook_ping
assert "desiredStateSourceMatch" not in webhook_ping, webhook_ping
assert os.environ["INVALID_WEBHOOK_STATUS"] == "400", os.environ["INVALID_WEBHOOK_STATUS"]
assert "github webhook signature mismatch" in invalid_webhook, invalid_webhook
for entry in audit_logs["entries"]:
    assert entry["appRef"] == "payments-api", entry
    assert entry["environmentRef"] in {"prod-apne2", "staging-apne2"}, entry
    assert isinstance(entry.get("metadata", {}), dict), entry
    assert isinstance(entry.get("source", {}), dict), entry
fallback_entries = [entry for entry in audit_logs["entries"] if entry["deploymentName"] == "payments-api-prod"]
assert fallback_entries, audit_logs
for entry in fallback_entries:
    assert entry["actor"]["available"] is False, entry
    assert "no authenticated request context" in entry["actor"]["reason"], entry
assert isinstance(actor_audit_logs.get("entries"), list), actor_audit_logs
assert len(actor_audit_logs["entries"]) == 1, actor_audit_logs
assert actor_audit_logs["entries"][0]["deploymentName"] == "payments-api-staging", actor_audit_logs
assert actor_audit_logs["entries"][0]["actor"]["available"] is True, actor_audit_logs
assert actor_audit_logs["entries"][0]["actor"]["id"] == "explicit-456", actor_audit_logs
assert actor_audit_logs["entries"][0]["actor"]["name"] == "Alex Explicit", actor_audit_logs
assert actor_audit_logs["entries"][0]["actor"]["email"] == "alex.explicit@example.com", actor_audit_logs

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
    assert isinstance(worker_resource_inspection["ecsService"].get("serviceName"), str) and worker_resource_inspection["ecsService"]["serviceName"], worker_resource_inspection
    minimum_healthy_percent = worker_resource_inspection["ecsService"].get("minimumHealthyPercent")
    maximum_percent = worker_resource_inspection["ecsService"].get("maximumPercent")
    health_check_grace_period_seconds = worker_resource_inspection["ecsService"].get("healthCheckGracePeriodSeconds")
    assert minimum_healthy_percent is None or isinstance(minimum_healthy_percent, int), worker_resource_inspection
    assert maximum_percent is None or isinstance(maximum_percent, int), worker_resource_inspection
    assert health_check_grace_period_seconds is None or isinstance(health_check_grace_period_seconds, int), worker_resource_inspection
    assert isinstance(worker_resource_inspection["ecsService"].get("deployments", []), list), worker_resource_inspection
    assert isinstance(worker_resource_inspection["ecsService"].get("events", []), list), worker_resource_inspection
else:
    assert isinstance(worker_resource_inspection.get("unavailableReason"), str) and worker_resource_inspection["unavailableReason"], worker_resource_inspection

assert bootstrap_resource_logs["deploymentName"] == "payments-api-prod", bootstrap_resource_logs
assert bootstrap_resource_logs["resourceType"] == "userDataBootstrapSource", bootstrap_resource_logs
assert bootstrap_resource_logs["resourceName"] == "git:userdata/payments-api-prod.sh", bootstrap_resource_logs
assert isinstance(bootstrap_resource_logs.get("lookupContext"), dict), bootstrap_resource_logs
assert isinstance(bootstrap_resource_logs["available"], bool), bootstrap_resource_logs
assert bootstrap_resource_logs.get("entries") is None or isinstance(bootstrap_resource_logs.get("entries"), list), bootstrap_resource_logs
if bootstrap_resource_logs["available"]:
    assert bootstrap_resource_logs.get("source") == "ec2_console", bootstrap_resource_logs
    for entry in bootstrap_resource_logs.get("entries") or []:
        assert isinstance(entry.get("timestamp"), str) and entry["timestamp"], bootstrap_resource_logs
        assert isinstance(entry.get("message"), str) and entry["message"], bootstrap_resource_logs
else:
    assert bootstrap_resource_logs.get("unavailableReason") in {"aws_unavailable", "no_log_source"}, bootstrap_resource_logs

assert bootstrap_resource_inspection["deploymentName"] == "payments-api-prod", bootstrap_resource_inspection
assert bootstrap_resource_inspection["resourceType"] == "userDataBootstrapSource", bootstrap_resource_inspection
assert bootstrap_resource_inspection["resourceName"] == "git:userdata/payments-api-prod.sh", bootstrap_resource_inspection
assert isinstance(bootstrap_resource_inspection.get("lookupContext"), dict), bootstrap_resource_inspection
assert isinstance(bootstrap_resource_inspection["available"], bool), bootstrap_resource_inspection
if bootstrap_resource_inspection["available"]:
    assert bootstrap_resource_inspection.get("source") == "auto_scaling_group", bootstrap_resource_inspection
    assert isinstance(bootstrap_resource_inspection.get("userDataBootstrapSource"), dict), bootstrap_resource_inspection
    payload = bootstrap_resource_inspection["userDataBootstrapSource"]
    assert payload.get("desiredBootstrapSource") == "git:userdata/payments-api-prod.sh", bootstrap_resource_inspection
    assert payload.get("backingResourceType") == "candidateAutoScalingGroup", bootstrap_resource_inspection
    assert payload.get("backingResourceName") == "payments-api-prod-candidate", bootstrap_resource_inspection
    launch_template = payload.get("launchTemplate")
    assert launch_template is None or isinstance(launch_template, dict), bootstrap_resource_inspection
else:
    assert bootstrap_resource_inspection.get("unavailableReason") == "aws_unavailable", bootstrap_resource_inspection

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

operator_read_auth_fragments = (
    "const operatorReadAuthRequiredState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 401;",
    "const operatorReadForbiddenState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 403;",
    "const operatorReadAuthRequiredMessage = (route) => `Tenet rejected ${route} because authenticated identity is required on this operator read route. Retry this bookmarkable page through the trusted front door so it can authenticate you and forward trusted identity headers.`;",
    "const operatorReadForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized for that route or deployment.`;",
)
assert_fragments("application_ui", application_ui, operator_read_auth_fragments + (
    "return 'Application detail';",
    "return `GET /v1/overview/applications/${deploymentName}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
))
assert_fragments("account_ui", account_ui, operator_read_auth_fragments + (
    "return 'Account detail';",
    "return `GET /v1/overview/accounts/${accountID}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
))
assert "fetchApplicationsOverview()" in ui, ui
assert "const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {" in account_ui, account_ui
assert "fetchAccountDetail(currentPageState.accountId)" in account_ui, account_ui
for label, body in (("resource_ui", resource_ui), ("ecs_resource_ui", ecs_resource_ui), ("bootstrap_resource_ui", bootstrap_resource_ui)):
    assert_fragments(label, body, operator_read_auth_fragments + (
        "return 'Resource detail';",
        "return `GET /v1/overview/deployments/${deploymentName}`;",
        "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
        "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
    ))
assert_fragments("drift_ui", drift_ui, operator_read_auth_fragments + (
    "return 'Drift viewer';",
    "return `GET /v1/overview/deployments/${deploymentName}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
))
assert_fragments("audit_ui", audit_ui, operator_read_auth_fragments + (
    "return `GET ${buildAuditLogsRoute({",
    "renderAuditLogsState(operatorReadFailureHeading(error), 'error', operatorReadFailureMessage(buildEmbeddedPageReadRoute(currentPageState), error));",
))

for fragment in (
    "Open account",
    "Open application",
    "Drift view",
    "Plan/apply",
    "On this page",
    "Linked resources",
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
    "const buildResourceLogsRoute = (deploymentName, resource) => {",
    "const resourceLogsReadRoute = (deploymentName, resource) => `GET ${buildResourceLogsRoute(deploymentName, resource)}`;",
    "const buildResourceInspectionRoute = (deploymentName, resource) => {",
    "const resourceInspectionReadRoute = (deploymentName, resource) => `GET ${buildResourceInspectionRoute(deploymentName, resource)}`;",
    "params.get('planId') || ''",
    "params.get('applyId') || ''",
):
    assert fragment in ui, fragment

for fragment in (
    "Drift viewer",
    "Field-level drift",
    "Current section",
    "Active drift source",
    "On this page",
    "Linked resources",
    "const createDeploymentQuickNav = (deploymentName, currentWorkspace, linkedResources = null, sectionLinks = [], currentSectionEmptyLabel = '') => {",
    "const trackCurrentSection = Boolean(currentSectionEmptyLabel) && sectionLinks.some((sectionLink) => sectionLink?.sectionID);",
    "(mode === 'drift' || mode === 'plan') ? 'No section is currently targeted.' : '',",
    "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
    "No section is currently targeted.",
    "No linked resources are available in this drift workspace yet.",
    "Current source: controller-authored managedDrift rows.",
    "managedDrift is present in the deployment detail payload, so fallback read-model heuristics stay suppressed for this deployment.",
    "Drift summary",
    "Total rows",
    "Visible rows",
    "Field path",
    "Desired",
    "Actual",
    "Severity",
    "No controller-authored field-level drift was reported for this deployment.",
    "No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.",
):
    assert fragment in drift_ui, fragment

for fragment in (
    "Audit timeline",
    "Deployment audit timeline",
    "Operator audit log",
    "Audit filters: ${activeFilters.join(' · ')}",
    "Clear audit filters",
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
    "const clearAuditPageFilters = () => {",
):
    assert fragment in audit_ui, fragment

for fragment in (
    "const fetchApplicationDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`, {",
    "const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);",
    "const renderApplicationDetailPage = (detail) => {",
    "createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')",
    "Application resources",
):
    assert fragment in application_ui, fragment

for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui):
    for fragment in (
        "const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);",
        "Opened via bookmarked resource alias.",
        "const renderResourceDetailPage = (detail) => {",
        "const createResourceQuickNav = (detail, resource, sectionLinks = []) => {",
        "createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
        "createNavLink(buildPageURL('resource', {",
        "Quick navigation",
        "Current section",
        "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
        "No section is currently targeted.",
        "No additional section links are available for this resource.",
    ):
        assert fragment in body, fragment

for fragment in (
    "Plan preview actions",
    "Apply request actions",
    "Plan preview approval guidance",
    "Apply Plan",
    "On this page",
    "Plan operations",
    "buildPageSectionURL('plan', sectionID, {",
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
  controller_logs=$("${COMPOSE[@]}" logs controller --tail=200 2>&1 || true)
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
  if assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$audit_logs_request_id" "$application_ui_request_id" "$account_ui_request_id" "$drift_ui_request_id" "$audit_ui_request_id" "$plan_apply_ui_request_id" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$audit_logs_request_id" "$application_ui_request_id" "$account_ui_request_id" "$drift_ui_request_id" "$audit_ui_request_id" "$plan_apply_ui_request_id"; then
  echo "smoke-docker: api logs missing correlated structured request entries" >&2
  exit 1
fi

restart_api_with_frontdoor_read_auth

restarted_plan_preview=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/plans/$plan_id")
restarted_apply_record=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/applies/$apply_id")

auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/overview/applications")
auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/status/deployments")
auth_unauthenticated_audit_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/audit-logs?action=apply.requested")
auth_unauthenticated_transition_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  --data '{"status":"running","step":1}' \
  "$API_BASE_URL/v1/applies/$apply_id/transitions")
auth_unauthenticated_events_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_EVENTS_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")
auth_unauthenticated_resource_logs_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
auth_unauthenticated_resource_inspection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_status_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_STATUS_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "$API_BASE_URL/v1/status/deployments/payments-api-staging")
auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")
auth_forbidden_resource_logs_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_resource_inspection_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_transition_status=$(curl -sS -o "$AUTH_FORBIDDEN_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  --data '{"status":"running","step":1}' \
  "$API_BASE_URL/v1/applies/$apply_id/transitions")
allowed_status_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "$API_BASE_URL/v1/status/deployments/payments-api-staging")
allowed_events_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")
allowed_resource_logs=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
allowed_resource_inspection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
filtered_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/overview/applications")
filtered_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/status/deployments")
filtered_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, payments-api-staging" \
  "$API_BASE_URL/v1/audit-logs")
empty_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  "$API_BASE_URL/v1/overview/applications")
empty_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  "$API_BASE_URL/v1/status/deployments")
empty_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  "$API_BASE_URL/v1/audit-logs?action=apply.requested")
restarted_plan_preview_path=$(write_payload restarted-plan-preview.json "$restarted_plan_preview")
restarted_apply_record_path=$(write_payload restarted-apply-record.json "$restarted_apply_record")
allowed_status_read_path=$(write_payload allowed-status-read.json "$allowed_status_read")
allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")
allowed_resource_logs_path=$(write_payload allowed-resource-logs.json "$allowed_resource_logs")
allowed_resource_inspection_path=$(write_payload allowed-resource-inspection.json "$allowed_resource_inspection")
filtered_applications_collection_path=$(write_payload filtered-applications-collection.json "$filtered_applications_collection")
filtered_status_collection_path=$(write_payload filtered-status-collection.json "$filtered_status_collection")
filtered_audit_collection_path=$(write_payload filtered-audit-collection.json "$filtered_audit_collection")
empty_applications_collection_path=$(write_payload empty-applications-collection.json "$empty_applications_collection")
empty_status_collection_path=$(write_payload empty-status-collection.json "$empty_status_collection")
empty_audit_collection_path=$(write_payload empty-audit-collection.json "$empty_audit_collection")

PLAN_ID="$plan_id" \
APPLY_ID="$apply_id" \
RESTARTED_PLAN_PREVIEW_PATH="$restarted_plan_preview_path" \
RESTARTED_APPLY_RECORD_PATH="$restarted_apply_record_path" \
AUTH_UNAUTHENTICATED_COLLECTION_STATUS="$auth_unauthenticated_collection_status" \
AUTH_UNAUTHENTICATED_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS="$auth_unauthenticated_status_collection_status" \
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS="$auth_unauthenticated_audit_collection_status" \
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_TRANSITION_STATUS="$auth_unauthenticated_transition_status" \
AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH="$AUTH_UNAUTHENTICATED_TRANSITION_BODY" \
AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status" \
AUTH_UNAUTHENTICATED_EVENTS_BODY_PATH="$AUTH_UNAUTHENTICATED_EVENTS_BODY" \
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS="$auth_unauthenticated_resource_logs_status" \
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY_PATH="$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" \
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS="$auth_unauthenticated_resource_inspection_status" \
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" \
AUTH_FORBIDDEN_STATUS_READ_STATUS="$auth_forbidden_status_read_status" \
AUTH_FORBIDDEN_STATUS_READ_BODY_PATH="$AUTH_FORBIDDEN_STATUS_READ_BODY" \
AUTH_FORBIDDEN_TRANSITION_STATUS="$auth_forbidden_transition_status" \
AUTH_FORBIDDEN_TRANSITION_BODY_PATH="$AUTH_FORBIDDEN_TRANSITION_BODY" \
AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status" \
AUTH_FORBIDDEN_EVENTS_BODY_PATH="$AUTH_FORBIDDEN_EVENTS_BODY" \
AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS="$auth_forbidden_resource_logs_status" \
AUTH_FORBIDDEN_RESOURCE_LOGS_BODY_PATH="$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" \
AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS="$auth_forbidden_resource_inspection_status" \
AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY_PATH="$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" \
ALLOWED_STATUS_READ_PATH="$allowed_status_read_path" \
ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path" \
ALLOWED_RESOURCE_LOGS_PATH="$allowed_resource_logs_path" \
ALLOWED_RESOURCE_INSPECTION_PATH="$allowed_resource_inspection_path" \
FILTERED_APPLICATIONS_COLLECTION_PATH="$filtered_applications_collection_path" \
FILTERED_STATUS_COLLECTION_PATH="$filtered_status_collection_path" \
FILTERED_AUDIT_COLLECTION_PATH="$filtered_audit_collection_path" \
EMPTY_APPLICATIONS_COLLECTION_PATH="$empty_applications_collection_path" \
EMPTY_STATUS_COLLECTION_PATH="$empty_status_collection_path" \
EMPTY_AUDIT_COLLECTION_PATH="$empty_audit_collection_path" \
python3 - <<'PY'
import json
import os

with open(os.environ["AUTH_UNAUTHENTICATED_COLLECTION_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_collection_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_status_collection_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_audit_collection_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_transition_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_EVENTS_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_events_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_resource_logs_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_resource_inspection_body = fh.read()
with open(os.environ["RESTARTED_PLAN_PREVIEW_PATH"], encoding="utf-8") as fh:
    restarted_plan_preview = json.load(fh)
with open(os.environ["RESTARTED_APPLY_RECORD_PATH"], encoding="utf-8") as fh:
    restarted_apply_record = json.load(fh)
with open(os.environ["AUTH_FORBIDDEN_STATUS_READ_BODY_PATH"], encoding="utf-8") as fh:
    forbidden_status_read_body = fh.read()
with open(os.environ["AUTH_FORBIDDEN_TRANSITION_BODY_PATH"], encoding="utf-8") as fh:
    forbidden_transition_body = fh.read()
with open(os.environ["AUTH_FORBIDDEN_EVENTS_BODY_PATH"], encoding="utf-8") as fh:
    forbidden_events_body = fh.read()
with open(os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_BODY_PATH"], encoding="utf-8") as fh:
    forbidden_resource_logs_body = fh.read()
with open(os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY_PATH"], encoding="utf-8") as fh:
    forbidden_resource_inspection_body = fh.read()
with open(os.environ["ALLOWED_STATUS_READ_PATH"], encoding="utf-8") as fh:
    allowed_status_read = json.load(fh)
with open(os.environ["ALLOWED_EVENTS_READ_PATH"], encoding="utf-8") as fh:
    allowed_events_read = json.load(fh)
with open(os.environ["ALLOWED_RESOURCE_LOGS_PATH"], encoding="utf-8") as fh:
    allowed_resource_logs = json.load(fh)
with open(os.environ["ALLOWED_RESOURCE_INSPECTION_PATH"], encoding="utf-8") as fh:
    allowed_resource_inspection = json.load(fh)
with open(os.environ["FILTERED_APPLICATIONS_COLLECTION_PATH"], encoding="utf-8") as fh:
    filtered_applications_collection = json.load(fh)
with open(os.environ["FILTERED_STATUS_COLLECTION_PATH"], encoding="utf-8") as fh:
    filtered_status_collection = json.load(fh)
with open(os.environ["FILTERED_AUDIT_COLLECTION_PATH"], encoding="utf-8") as fh:
    filtered_audit_collection = json.load(fh)
with open(os.environ["EMPTY_APPLICATIONS_COLLECTION_PATH"], encoding="utf-8") as fh:
    empty_applications_collection = json.load(fh)
with open(os.environ["EMPTY_STATUS_COLLECTION_PATH"], encoding="utf-8") as fh:
    empty_status_collection = json.load(fh)
with open(os.environ["EMPTY_AUDIT_COLLECTION_PATH"], encoding="utf-8") as fh:
    empty_audit_collection = json.load(fh)

assert restarted_plan_preview["id"] == os.environ["PLAN_ID"], restarted_plan_preview
assert restarted_plan_preview["deploymentName"] == "payments-api-prod", restarted_plan_preview
assert restarted_apply_record["id"] == os.environ["APPLY_ID"], restarted_apply_record
assert restarted_apply_record["planId"] == os.environ["PLAN_ID"], restarted_apply_record
assert restarted_apply_record["deploymentName"] == "payments-api-prod", restarted_apply_record
assert os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"]
assert unauthenticated_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_collection_body
assert os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"]
assert unauthenticated_status_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_status_collection_body
assert os.environ["AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS"]
assert unauthenticated_audit_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_audit_collection_body
assert os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"]
assert unauthenticated_transition_body.strip() == "unauthenticated: authenticated identity is required for this operator write route", unauthenticated_transition_body
assert os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"]
assert unauthenticated_events_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_events_body
assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"]
assert unauthenticated_resource_logs_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_resource_logs_body
assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"]
assert unauthenticated_resource_inspection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_resource_inspection_body
assert os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"]
assert forbidden_status_read_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_status_read_body
assert os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"]
assert forbidden_transition_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_transition_body
assert os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"]
assert forbidden_events_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_events_body
assert os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"]
assert forbidden_resource_logs_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_resource_logs_body
assert os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"]
assert forbidden_resource_inspection_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_resource_inspection_body
assert allowed_status_read["deploymentName"] == "payments-api-staging", allowed_status_read
assert allowed_events_read["deploymentName"] == "payments-api-staging", allowed_events_read
assert isinstance(allowed_events_read.get("events"), list), allowed_events_read
assert allowed_resource_logs["deploymentName"] == "worker-prod", allowed_resource_logs
assert allowed_resource_logs["resourceType"] == "ecsService", allowed_resource_logs
assert allowed_resource_logs["resourceName"] == "worker-prod-service", allowed_resource_logs
assert isinstance(allowed_resource_logs["available"], bool), allowed_resource_logs
if allowed_resource_logs["available"]:
    assert isinstance(allowed_resource_logs.get("source"), str) and allowed_resource_logs["source"], allowed_resource_logs
else:
    assert isinstance(allowed_resource_logs.get("unavailableReason"), str) and allowed_resource_logs["unavailableReason"], allowed_resource_logs
assert allowed_resource_inspection["deploymentName"] == "worker-prod", allowed_resource_inspection
assert allowed_resource_inspection["resourceType"] == "ecsService", allowed_resource_inspection
assert allowed_resource_inspection["resourceName"] == "worker-prod-service", allowed_resource_inspection
assert isinstance(allowed_resource_inspection["available"], bool), allowed_resource_inspection
if allowed_resource_inspection["available"]:
    assert isinstance(allowed_resource_inspection.get("source"), str) and allowed_resource_inspection["source"], allowed_resource_inspection
else:
    assert isinstance(allowed_resource_inspection.get("unavailableReason"), str) and allowed_resource_inspection["unavailableReason"], allowed_resource_inspection
filtered_application_names = {row["name"] for row in filtered_applications_collection}
assert "worker-prod" in filtered_application_names, filtered_applications_collection
assert filtered_application_names == {"payments-api-prod", "worker-prod"}, filtered_applications_collection
filtered_status_deployment_names = {row["deploymentName"] for row in filtered_status_collection}
assert filtered_status_deployment_names == {"payments-api-prod", "worker-prod"}, filtered_status_collection
filtered_audit_deployment_names = {row["deploymentName"] for row in filtered_audit_collection["entries"]}
assert filtered_audit_deployment_names, filtered_audit_collection
assert filtered_audit_deployment_names <= {"payments-api-prod", "payments-api-staging"}, filtered_audit_collection
assert "payments-api-staging" in filtered_audit_deployment_names, filtered_audit_collection
assert empty_applications_collection == [], empty_applications_collection
assert empty_status_collection == [], empty_status_collection
assert empty_audit_collection == {"entries": []}, empty_audit_collection
PY

restart_api_with_apply_mode true

queued_deployment_detail=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod")
queued_deployment_detail_extract_path=$(write_payload queued-deployment-detail.extract.json "$queued_deployment_detail")
queued_deployment_source_fingerprint=$(json_string_field_from_path "$queued_deployment_detail_extract_path" sourceFingerprint)
queued_plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT="$queued_deployment_source_fingerprint" python3 - <<'PY'
import json
import os

fingerprint = (os.environ.get("DEPLOYMENT_SOURCE_FINGERPRINT") or "").strip()
print(json.dumps({"ifMatch": fingerprint} if fingerprint else {}))
PY
)
queued_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$queued_plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/worker-prod")
queued_plan_preview_path=$(write_payload queued-plan-preview.json "$queued_plan_preview")
queued_plan_id=$(json_string_field_from_path "$queued_plan_preview_path" id)
queued_plan_preview_get=$(curl -fsS "$API_BASE_URL/v1/plans/$queued_plan_id")
queued_apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$queued_plan_id")
queued_apply_record_path=$(write_payload queued-apply-record.json "$queued_apply_record")
queued_apply_id=$(json_string_field_from_path "$queued_apply_record_path" id)
queued_apply_record_get=$(wait_for_apply_status "$queued_apply_id" failed)
queued_plan_preview_get_path=$(write_payload queued-plan-preview-get.json "$queued_plan_preview_get")
queued_apply_record_get_path=$(write_payload queued-apply-record-get.json "$queued_apply_record_get")

QUEUED_PLAN_PREVIEW_PATH="$queued_plan_preview_path" \
QUEUED_PLAN_PREVIEW_GET_PATH="$queued_plan_preview_get_path" \
QUEUED_APPLY_RECORD_PATH="$queued_apply_record_path" \
QUEUED_APPLY_RECORD_GET_PATH="$queued_apply_record_get_path" \
python3 - <<'PY'
import json
import os

with open(os.environ["QUEUED_PLAN_PREVIEW_PATH"], encoding="utf-8") as fh:
    queued_plan_preview = json.load(fh)
with open(os.environ["QUEUED_PLAN_PREVIEW_GET_PATH"], encoding="utf-8") as fh:
    queued_plan_preview_get = json.load(fh)
with open(os.environ["QUEUED_APPLY_RECORD_PATH"], encoding="utf-8") as fh:
    queued_apply_record = json.load(fh)
with open(os.environ["QUEUED_APPLY_RECORD_GET_PATH"], encoding="utf-8") as fh:
    queued_apply_record_get = json.load(fh)

assert queued_plan_preview["deploymentName"] == "worker-prod", queued_plan_preview
assert queued_plan_preview["status"] == "ready", queued_plan_preview
assert isinstance(queued_plan_preview.get("operations", []), list) and queued_plan_preview["operations"], queued_plan_preview
assert queued_plan_preview_get == queued_plan_preview, (queued_plan_preview_get, queued_plan_preview)

assert queued_apply_record["planId"] == queued_plan_preview["id"], (queued_apply_record, queued_plan_preview)
assert queued_apply_record["deploymentName"] == "worker-prod", queued_apply_record
assert queued_apply_record["status"] == "queued", queued_apply_record
assert not queued_apply_record.get("error"), queued_apply_record
assert isinstance(queued_apply_record.get("id"), str) and queued_apply_record["id"].startswith("apply_"), queued_apply_record
assert isinstance(queued_apply_record.get("createdAt"), str) and queued_apply_record["createdAt"], queued_apply_record
assert queued_apply_record["details"] == [
    "Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",
    "Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",
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
assert queued_apply_record_get["id"] == queued_apply_record["id"], (queued_apply_record_get, queued_apply_record)
assert queued_apply_record_get["status"] == "failed", queued_apply_record_get
assert queued_apply_record_get["error"] == "apply executor handoff stopped before live AWS mutation because this backend slice is still local-safe", queued_apply_record_get
assert queued_apply_record_get["details"] == [
    "Tenet executor advanced this apply request on the same applyId without calling the manual transition API.",
    "Tenet then marked the request failed because live AWS mutation is not implemented for this backend slice yet.",
    "The local-safe executor stopped at step 1 after recording the handoff state.",
], queued_apply_record_get
assert queued_apply_record_get["lifecycle"]["currentStep"] == 1, queued_apply_record_get
assert isinstance(queued_apply_record_get["lifecycle"].get("startedAt"), str) and queued_apply_record_get["lifecycle"]["startedAt"], queued_apply_record_get
assert isinstance(queued_apply_record_get["lifecycle"].get("finishedAt"), str) and queued_apply_record_get["lifecycle"]["finishedAt"], queued_apply_record_get
assert [operation["status"] for operation in queued_apply_record_get["operations"]] == ["failed"] + ["queued"] * (len(queued_apply_record_get["operations"]) - 1), queued_apply_record_get
PY

echo "smoke-docker: compose stack path ok"
