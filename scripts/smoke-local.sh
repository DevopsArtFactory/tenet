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
SOURCE_REPO_ROOT=${TENET_REPO_ROOT:-$ROOT/examples/local-repo}
REPO_ROOT=$(mktemp -d)
FIXTURE_STATUS_PATH=${TENET_STATUS_PATH:-$ROOT/examples/local-status/deployments.json}
APPLY_ENABLED=${TENET_AWS_ENABLE_APPLY:-false}
AUTH_TRUSTED_HEADERS=${TENET_API_TRUST_AUTHENTICATED_HEADERS:-false}
AUTH_ENFORCED=${TENET_API_ENFORCE_AUTHENTICATED_IDENTITY:-false}
WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}
STATUS_PATH=$(mktemp)
PLAN_STORE_PATH=$(mktemp)
APPLY_STORE_PATH=$(mktemp)
PAYLOAD_DIR=$(mktemp -d)
cp "$FIXTURE_STATUS_PATH" "$STATUS_PATH"
rm -f "$PLAN_STORE_PATH"
rm -f "$APPLY_STORE_PATH"
LOG_FILE=$(mktemp)
CONTROLLER_LOG_FILE=$(mktemp)
INVALID_AUDIT_ACTION_BODY=$(mktemp)
STALE_APPLY_CONFLICT_BODY=$(mktemp)
INVALID_APPLY_TRANSITION_BODY=$(mktemp)
UNKNOWN_APPLY_TRANSITION_BODY=$(mktemp)
TERMINAL_APPLY_TRANSITION_BODY=$(mktemp)
INVALID_WEBHOOK_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_PLAN_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_APPLY_BODY=$(mktemp)
cp -R "$SOURCE_REPO_ROOT/." "$REPO_ROOT/"
cleanup() {
  if [[ -n "${CONTROLLER_PID:-}" ]] && kill -0 "$CONTROLLER_PID" 2>/dev/null; then
    kill "$CONTROLLER_PID" 2>/dev/null || true
    wait "$CONTROLLER_PID" 2>/dev/null || true
  fi
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE" "$CONTROLLER_LOG_FILE" "$STATUS_PATH" "$PLAN_STORE_PATH" "$APPLY_STORE_PATH" "$INVALID_AUDIT_ACTION_BODY" "$STALE_APPLY_CONFLICT_BODY" "$INVALID_APPLY_TRANSITION_BODY" "$UNKNOWN_APPLY_TRANSITION_BODY" "$TERMINAL_APPLY_TRANSITION_BODY" "$INVALID_WEBHOOK_BODY" "$AUTH_UNAUTHENTICATED_PLAN_BODY" "$AUTH_UNAUTHENTICATED_APPLY_BODY"
  rm -rf "$PAYLOAD_DIR" "$REPO_ROOT"
}
trap cleanup EXIT

write_payload() {
  local name=$1
  local content=$2
  local path="$PAYLOAD_DIR/$name"
  printf '%s' "$content" >"$path"
  printf '%s' "$path"
}

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

assert headers.get("request-id"), headers
PY
}

wait_for_apply_status() {
  local apply_id=$1
  local expected_status=$2
  local response='{}'

  for _ in $(seq 1 50); do
    response=$(curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id")
    current_status=$(APPLY_RECORD_JSON="$response" python3 - <<'PY'
import json, os
print(json.loads(os.environ['APPLY_RECORD_JSON'])['status'])
PY
)
    if [[ "$current_status" == "$expected_status" ]]; then
      printf '%s' "$response"
      return 0
    fi
    sleep 0.1
  done

  printf '%s' "$response"
  return 1
}

stop_api() {
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  lingering_api_pids=$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)
  if [[ -n "$lingering_api_pids" ]]; then
    kill $lingering_api_pids 2>/dev/null || true
    sleep 1
  fi
  API_PID=""
}

start_api() {
  local trust_authenticated_headers=$1
  local enforce_authenticated_identity=$2

  TENET_API_ADDR=":$PORT" TENET_CATALOG_SOURCE=filesystem TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" TENET_PLAN_STORE_PATH="$PLAN_STORE_PATH" TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH" TENET_AWS_ENABLE_APPLY="$APPLY_ENABLED" TENET_API_TRUST_AUTHENTICATED_HEADERS="$trust_authenticated_headers" TENET_API_ENFORCE_AUTHENTICATED_IDENTITY="$enforce_authenticated_identity" TENET_GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET" \
    go run ./cmd/tenet-api >"$LOG_FILE" 2>&1 &
  API_PID=$!

  for _ in $(seq 1 30); do
    if curl -fsS "http://$HOST:$PORT/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "tenet-api did not become healthy" >&2
  cat "$LOG_FILE" >&2 || true
  return 1
}

start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED"

curl -fsS "http://$HOST:$PORT/healthz" >/dev/null
assert_request_id_header "http://$HOST:$PORT/healthz"
assert_request_id_header "http://$HOST:$PORT/v1/catalog/summary"
assert_request_id_header "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing"
assert_request_id_header "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service"
assert_request_id_header "http://$HOST:$PORT/"

summary=$(curl -fsS "http://$HOST:$PORT/v1/catalog/summary")
applications=$(curl -fsS "http://$HOST:$PORT/v1/overview/applications")
application_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/applications/payments-api-prod")
overview=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts")
account_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012")
group_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2")
deployment_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod")
deployment_events_fixture=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events")
deployment_source_fingerprint=$(DEPLOYMENT_DETAIL_JSON="$deployment_detail" python3 - <<'PY'
import json, os
print(json.loads(os.environ['DEPLOYMENT_DETAIL_JSON'])['sourceFingerprint'])
PY
)
plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "{\"ifMatch\":\"$deployment_source_fingerprint\"}" "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
plan_id=$(PLAN_PREVIEW_JSON="$plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['PLAN_PREVIEW_JSON'])['id'])
PY
)
plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$plan_id")
apply_record=$(curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id")
apply_id=$(APPLY_RECORD_JSON="$apply_record" python3 - <<'PY'
import json, os
print(json.loads(os.environ['APPLY_RECORD_JSON'])['id'])
PY
)
if [[ "$APPLY_ENABLED" == "true" ]]; then
  apply_record_get=$(wait_for_apply_status "$apply_id" failed)
else
  apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id")
fi
invalid_apply_transition_status=$(curl -sS -o "$INVALID_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1' \
  "http://$HOST:$PORT/v1/applies/$apply_id/transitions")
unknown_apply_transition_status=$(curl -sS -o "$UNKNOWN_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/apply_smoke_missing/transitions")
terminal_apply_transition_status=$(curl -sS -o "$TERMINAL_APPLY_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/$apply_id/transitions")
actor_plan_preview=$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Actor-Id: explicit-456" \
  -H "X-Tenet-Actor-Name: Alex Explicit" \
  -H "X-Tenet-Actor-Email: alex.explicit@example.com" \
  -H "X-User-ID: operator-123" \
  -H "X-User-Name: Jane Operator" \
  -H "X-User-Email: jane@example.com" \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-staging")
actor_plan_id=$(ACTOR_PLAN_PREVIEW_JSON="$actor_plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['ACTOR_PLAN_PREVIEW_JSON'])['id'])
PY
)
actor_plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$actor_plan_id")
actor_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Actor-Id: explicit-456" \
  -H "X-Tenet-Actor-Name: Alex Explicit" \
  -H "X-Tenet-Actor-Email: alex.explicit@example.com" \
  -H "X-User-ID: operator-123" \
  -H "X-User-Name: Jane Operator" \
  -H "X-User-Email: jane@example.com" \
  "http://$HOST:$PORT/v1/applies/plans/$actor_plan_id")
actor_apply_id=$(ACTOR_APPLY_RECORD_JSON="$actor_apply_record" python3 - <<'PY'
import json, os
print(json.loads(os.environ['ACTOR_APPLY_RECORD_JSON'])['id'])
PY
)
if [[ "$APPLY_ENABLED" == "true" ]]; then
  actor_apply_record_get=$(wait_for_apply_status "$actor_apply_id" failed)
else
  actor_apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$actor_apply_id")
fi
stale_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d '{"ifMatch":"plan_deadbeef"}' "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
cat >"$REPO_ROOT/deployments/payments-api-stale.yaml" <<'EOF'
name: payments-api-stale
appRef: payments-api
binary: s3://releases.example.com/payments-api/stale/payments-api-linux-amd64
userdata:
  type: git
  path: userdata/payments-api-prod.sh
stacks:
  - stack: payments-api-stale
    account: prod-core
    env: prod-apne2
    replacement_type: canary
    termination_delay_rate: 25
    capacity:
      desired: 3
      min: 2
      max: 6
    regions:
      - region: ap-northeast-2
        instance_type: t3.medium
        ami_id: ami-1234567890abcdef0
EOF
stale_apply_deployment_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-stale")
stale_apply_source_fingerprint=$(DEPLOYMENT_DETAIL_JSON="$stale_apply_deployment_detail" python3 - <<'PY'
import json, os
print(json.loads(os.environ['DEPLOYMENT_DETAIL_JSON'])['sourceFingerprint'])
PY
)
stale_apply_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "{\"ifMatch\":\"$stale_apply_source_fingerprint\"}" "http://$HOST:$PORT/v1/plans/deployments/payments-api-stale")
stale_apply_plan_id=$(STALE_APPLY_PLAN_PREVIEW_JSON="$stale_apply_plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['STALE_APPLY_PLAN_PREVIEW_JSON'])['id'])
PY
)
python3 - <<'PY' "$REPO_ROOT/deployments/payments-api-stale.yaml"
from pathlib import Path
import sys

path = Path(sys.argv[1])
content = path.read_text(encoding='utf-8')
old = "binary: s3://releases.example.com/payments-api/stale/payments-api-linux-amd64"
new = "binary: s3://releases.example.com/payments-api/stale/payments-api-linux-arm64"
if old not in content:
    raise SystemExit(f"expected stale deployment fixture to contain {old!r}")
path.write_text(content.replace(old, new, 1), encoding='utf-8')
PY
stale_apply_conflict_status=$(curl -sS -o "$STALE_APPLY_CONFLICT_BODY" -w "%{http_code}" -X POST "http://$HOST:$PORT/v1/applies/plans/$stale_apply_plan_id")
stale_apply_conflict=$(cat "$STALE_APPLY_CONFLICT_BODY")
stale_apply_audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-stale&action=apply.requested")
audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs")
audit_logs_action_filtered=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?action=apply.requested")
audit_logs_filtered=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1")
audit_logs_empty=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod")
actor_audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
invalid_audit_action_status=$(curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/audit-logs?action=invalid.action")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
webhook_push_body=$(cat <<'EOF'
{"ref":"refs/heads/main","before":"1111111111111111111111111111111111111111","after":"2222222222222222222222222222222222222222","repository":{"full_name":"acme/platform-config"},"head_commit":{"id":"2222222222222222222222222222222222222222","message":"update deployment","timestamp":"2026-04-18T15:00:00Z"},"commits":[{"id":"a","added":["deployments/api.yaml"],"modified":["environments/prod.yaml"],"removed":[]},{"id":"b","added":["accounts/prod.yaml"],"modified":["deployments/worker.yaml"],"removed":["deployments/old.yaml"]}]}
EOF
)
webhook_push_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_push_body" python3 - <<'PY'
import hashlib, hmac, os
print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_push=$(curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-push-1' \
  -H 'X-GitHub-Event: push' \
  -H "X-Hub-Signature-256: $webhook_push_signature" \
  --data "$webhook_push_body" \
  "http://$HOST:$PORT/api/v1/github/webhook")
webhook_ping_body='{"zen":"keep it logically awesome"}'
webhook_ping_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_ping_body" python3 - <<'PY'
import hashlib, hmac, os
print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_ping=$(curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-ping-1' \
  -H 'X-GitHub-Event: ping' \
  -H "X-Hub-Signature-256: $webhook_ping_signature" \
  --data "$webhook_ping_body" \
  "http://$HOST:$PORT/api/v1/github/webhook")
invalid_webhook_status=$(curl -sS -o "$INVALID_WEBHOOK_BODY" -w "%{http_code}" -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-invalid-1' \
  -H 'X-GitHub-Event: push' \
  -H 'X-Hub-Signature-256: sha256=deadbeef' \
  --data "$webhook_push_body" \
  "http://$HOST:$PORT/api/v1/github/webhook")
invalid_webhook=$(cat "$INVALID_WEBHOOK_BODY")
resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
worker_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
codedeploy_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
production_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")
test_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")
traffic_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
ui=$(curl -fsS "http://$HOST:$PORT/")
audit_ui=$(curl -fsS "http://$HOST:$PORT/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=1")
plan_ui=$(curl -fsS "http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id")
apply_ui=$(curl -fsS "http://$HOST:$PORT/?page=plan&deployment=payments-api-prod&planId=$plan_id&applyId=$apply_id")

summary_path=$(write_payload summary.json "$summary")
applications_path=$(write_payload applications.json "$applications")
application_detail_path=$(write_payload application-detail.json "$application_detail")
overview_path=$(write_payload overview.json "$overview")
account_detail_path=$(write_payload account-detail.json "$account_detail")
group_detail_path=$(write_payload group-detail.json "$group_detail")
deployment_detail_path=$(write_payload deployment-detail.json "$deployment_detail")
deployment_events_fixture_path=$(write_payload deployment-events-fixture.json "$deployment_events_fixture")
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
stale_plan_preview_path=$(write_payload stale-plan-preview.json "$stale_plan_preview")
stale_apply_plan_preview_path=$(write_payload stale-apply-plan-preview.json "$stale_apply_plan_preview")
stale_apply_conflict_path=$(write_payload stale-apply-conflict.txt "$stale_apply_conflict")
stale_apply_audit_logs_path=$(write_payload stale-apply-audit-logs.json "$stale_apply_audit_logs")
audit_logs_path=$(write_payload audit-logs.json "$audit_logs")
audit_logs_action_filtered_path=$(write_payload audit-logs-action-filtered.json "$audit_logs_action_filtered")
audit_logs_filtered_path=$(write_payload audit-logs-filtered.json "$audit_logs_filtered")
audit_logs_empty_path=$(write_payload audit-logs-empty.json "$audit_logs_empty")
actor_audit_logs_path=$(write_payload actor-audit-logs.json "$actor_audit_logs")
status_json_path=$(write_payload status.json "$status")
webhook_push_path=$(write_payload webhook-push.json "$webhook_push")
webhook_ping_path=$(write_payload webhook-ping.json "$webhook_ping")
invalid_webhook_path=$(write_payload invalid-webhook.txt "$invalid_webhook")
resource_logs_path=$(write_payload resource-logs.json "$resource_logs")
worker_resource_logs_path=$(write_payload worker-resource-logs.json "$worker_resource_logs")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
candidate_resource_inspection_path=$(write_payload candidate-resource-inspection.json "$candidate_resource_inspection")
codedeploy_group_resource_inspection_path=$(write_payload codedeploy-group-resource-inspection.json "$codedeploy_group_resource_inspection")
production_listener_resource_inspection_path=$(write_payload production-listener-resource-inspection.json "$production_listener_resource_inspection")
test_listener_resource_inspection_path=$(write_payload test-listener-resource-inspection.json "$test_listener_resource_inspection")
traffic_resource_inspection_path=$(write_payload traffic-resource-inspection.json "$traffic_resource_inspection")
ui_path=$(write_payload ui.html "$ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_ui_path=$(write_payload plan-ui.html "$plan_ui")
apply_ui_path=$(write_payload apply-ui.html "$apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" APPLICATION_DETAIL_PATH="$application_detail_path" OVERVIEW_PATH="$overview_path" ACCOUNT_DETAIL_PATH="$account_detail_path" GROUP_DETAIL_PATH="$group_detail_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_EVENTS_FIXTURE_PATH="$deployment_events_fixture_path" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" INVALID_APPLY_TRANSITION_STATUS="$invalid_apply_transition_status" INVALID_APPLY_TRANSITION_PATH="$invalid_apply_transition_path" UNKNOWN_APPLY_TRANSITION_STATUS="$unknown_apply_transition_status" UNKNOWN_APPLY_TRANSITION_PATH="$unknown_apply_transition_path" TERMINAL_APPLY_TRANSITION_STATUS="$terminal_apply_transition_status" TERMINAL_APPLY_TRANSITION_PATH="$terminal_apply_transition_path" ACTOR_PLAN_PREVIEW_PATH="$actor_plan_preview_path" ACTOR_PLAN_PREVIEW_GET_PATH="$actor_plan_preview_get_path" ACTOR_APPLY_RECORD_PATH="$actor_apply_record_path" ACTOR_APPLY_RECORD_GET_PATH="$actor_apply_record_get_path" STALE_PLAN_PREVIEW_PATH="$stale_plan_preview_path" STALE_APPLY_PLAN_PREVIEW_PATH="$stale_apply_plan_preview_path" STALE_APPLY_CONFLICT_PATH="$stale_apply_conflict_path" STALE_APPLY_CONFLICT_STATUS="$stale_apply_conflict_status" STALE_APPLY_AUDIT_LOGS_PATH="$stale_apply_audit_logs_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" AUDIT_LOGS_FILTERED_PATH="$audit_logs_filtered_path" AUDIT_LOGS_EMPTY_PATH="$audit_logs_empty_path" ACTOR_AUDIT_LOGS_PATH="$actor_audit_logs_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" STATUS_JSON_PATH="$status_json_path" WEBHOOK_PUSH_PATH="$webhook_push_path" WEBHOOK_PING_PATH="$webhook_ping_path" INVALID_WEBHOOK_PATH="$invalid_webhook_path" INVALID_WEBHOOK_STATUS="$invalid_webhook_status" RESOURCE_LOGS_PATH="$resource_logs_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH="$codedeploy_group_resource_inspection_path" PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH="$production_listener_resource_inspection_path" TEST_LISTENER_RESOURCE_INSPECTION_PATH="$test_listener_resource_inspection_path" TRAFFIC_RESOURCE_INSPECTION_PATH="$traffic_resource_inspection_path" UI_PATH="$ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_UI_PATH="$plan_ui_path" APPLY_UI_PATH="$apply_ui_path" APPLY_ENABLED="$APPLY_ENABLED" python3 - <<'PY'
import json, os

def read_json(path_env):
    with open(os.environ[path_env], encoding='utf-8') as fh:
        return json.load(fh)


def read_text(path_env):
    with open(os.environ[path_env], encoding='utf-8') as fh:
        return fh.read()

summary = read_json('SUMMARY_PATH')
applications = read_json('APPLICATIONS_PATH')
application_detail = read_json('APPLICATION_DETAIL_PATH')
overview = read_json('OVERVIEW_PATH')
account_detail = read_json('ACCOUNT_DETAIL_PATH')
group_detail = read_json('GROUP_DETAIL_PATH')
deployment_detail = read_json('DEPLOYMENT_DETAIL_PATH')
deployment_events_fixture = read_json('DEPLOYMENT_EVENTS_FIXTURE_PATH')
plan_preview = read_json('PLAN_PREVIEW_PATH')
plan_preview_get = read_json('PLAN_PREVIEW_GET_PATH')
apply_record = read_json('APPLY_RECORD_PATH')
apply_record_get = read_json('APPLY_RECORD_GET_PATH')
invalid_apply_transition = read_text('INVALID_APPLY_TRANSITION_PATH')
unknown_apply_transition = read_text('UNKNOWN_APPLY_TRANSITION_PATH')
terminal_apply_transition = read_text('TERMINAL_APPLY_TRANSITION_PATH')
actor_plan_preview = read_json('ACTOR_PLAN_PREVIEW_PATH')
actor_plan_preview_get = read_json('ACTOR_PLAN_PREVIEW_GET_PATH')
actor_apply_record = read_json('ACTOR_APPLY_RECORD_PATH')
actor_apply_record_get = read_json('ACTOR_APPLY_RECORD_GET_PATH')
stale_plan_preview = read_json('STALE_PLAN_PREVIEW_PATH')
stale_apply_plan_preview = read_json('STALE_APPLY_PLAN_PREVIEW_PATH')
stale_apply_conflict = read_text('STALE_APPLY_CONFLICT_PATH')
stale_apply_audit_logs = read_json('STALE_APPLY_AUDIT_LOGS_PATH')
audit_logs = read_json('AUDIT_LOGS_PATH')
audit_logs_action_filtered = read_json('AUDIT_LOGS_ACTION_FILTERED_PATH')
audit_logs_filtered = read_json('AUDIT_LOGS_FILTERED_PATH')
audit_logs_empty = read_json('AUDIT_LOGS_EMPTY_PATH')
actor_audit_logs = read_json('ACTOR_AUDIT_LOGS_PATH')
status = read_json('STATUS_JSON_PATH')
webhook_push = read_json('WEBHOOK_PUSH_PATH')
webhook_ping = read_json('WEBHOOK_PING_PATH')
invalid_webhook = read_text('INVALID_WEBHOOK_PATH')
resource_logs = read_json('RESOURCE_LOGS_PATH')
worker_resource_logs = read_json('WORKER_RESOURCE_LOGS_PATH')
worker_resource_inspection = read_json('WORKER_RESOURCE_INSPECTION_PATH')
candidate_resource_inspection = read_json('CANDIDATE_RESOURCE_INSPECTION_PATH')
codedeploy_group_resource_inspection = read_json('CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH')
production_listener_resource_inspection = read_json('PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH')
test_listener_resource_inspection = read_json('TEST_LISTENER_RESOURCE_INSPECTION_PATH')
traffic_resource_inspection = read_json('TRAFFIC_RESOURCE_INSPECTION_PATH')
ui = read_text('UI_PATH')
audit_ui = read_text('AUDIT_UI_PATH')
plan_ui = read_text('PLAN_UI_PATH')
apply_ui = read_text('APPLY_UI_PATH')
apply_enabled = os.environ['APPLY_ENABLED'].lower() == 'true'
assert summary['accountCount'] == 2, summary
assert summary['environmentCount'] == 2, summary
assert summary['deploymentCount'] == 3, summary
assert [application['name'] for application in applications] == [
    'payments-api-prod',
    'payments-api-staging',
    'worker-prod',
], applications
assert applications[0]['executionMode'] in {'asgWeightedTargetGroups', 'asgInstanceRefresh'}, applications
assert applications[2]['executionMode'] == 'ecsRolling', applications
assert applications[2]['appRef'] == 'worker', applications
assert applications[2]['environmentRef'] == 'prod-apne2', applications
assert application_detail['name'] == 'payments-api-prod', application_detail
assert application_detail['accountName'] == 'prod-core', application_detail
assert application_detail['awsAccountId'] == '123456789012', application_detail
assert application_detail['region'] == 'ap-northeast-2', application_detail
assert application_detail['sourceFingerprint'] == deployment_detail['sourceFingerprint'], (application_detail, deployment_detail)
assert isinstance((application_detail.get('rollout') or {}).get('execution', []), list), application_detail
assert [account['name'] for account in overview] == ['prod-core', 'staging-core'], overview
assert [deployment['name'] for deployment in overview[0]['deployments']] == ['payments-api-prod', 'worker-prod'], overview
runtime_summary = overview[0]['deployments'][0].get('runtimeSummary') or {}
assert overview[0]['deployments'][0]['executionMode'] in {'asgWeightedTargetGroups', 'asgInstanceRefresh'}, overview
assert overview[0]['deployments'][1]['executionMode'] == 'ecsRolling', overview
assert isinstance(runtime_summary.get('changeDetails', []), list), overview
assert account_detail['name'] == 'prod-core', account_detail
assert account_detail['awsAccountId'] == '123456789012', account_detail
assert [deployment['name'] for deployment in account_detail['deployments']] == ['payments-api-prod', 'worker-prod'], account_detail
account_deployment = account_detail['deployments'][0]
if 'runtimeSummary' in account_deployment and account_deployment['runtimeSummary'] is not None:
    assert isinstance(account_deployment['runtimeSummary'].get('changeDetails', []), list), account_detail
assert group_detail['accountName'] == 'prod-core', group_detail
assert group_detail['awsAccountId'] == '123456789012', group_detail
assert group_detail['appRef'] == 'payments-api', group_detail
assert group_detail['environmentRef'] == 'prod-apne2', group_detail
assert group_detail['region'] == 'ap-northeast-2', group_detail
assert len(group_detail['deployments']) == 1, group_detail
assert group_detail['deployments'][0]['name'] == 'payments-api-prod', group_detail
deployment_runtime_summary = deployment_detail.get('runtimeSummary') or {}
deployment_history = deployment_detail.get('history') or []
managed_drift = deployment_detail.get('managedDrift')
assert deployment_detail['name'] == 'payments-api-prod', deployment_detail
assert isinstance(deployment_detail['sourceFingerprint'], str) and deployment_detail['sourceFingerprint'].startswith('plan_'), deployment_detail
assert isinstance(deployment_runtime_summary.get('changeDetails', []), list), deployment_detail
if managed_drift is not None:
    assert isinstance(managed_drift, list), deployment_detail
if deployment_history:
    assert isinstance(deployment_history[0].get('changeDetails', []), list), deployment_detail
assert deployment_events_fixture['deploymentName'] == 'payments-api-prod', deployment_events_fixture
assert isinstance(deployment_events_fixture['events'], list), deployment_events_fixture
assert len(deployment_events_fixture['events']) == 0, deployment_events_fixture
assert plan_preview['deploymentName'] == 'payments-api-prod', plan_preview
assert plan_preview['status'] == 'ready', plan_preview
assert plan_preview['source']['route'] == '/v1/overview/deployments/payments-api-prod', plan_preview
assert plan_preview['source']['fingerprint'] == deployment_detail['sourceFingerprint'], (plan_preview, deployment_detail)
assert isinstance(plan_preview.get('operations', []), list), plan_preview
assert isinstance((plan_preview.get('risk') or {}).get('reasons', []), list), plan_preview
assert isinstance((plan_preview.get('approval') or {}).get('required'), bool), plan_preview
assert plan_preview_get == plan_preview, (plan_preview_get, plan_preview)
assert apply_record['planId'] == plan_preview['id'], (apply_record, plan_preview)
assert apply_record['deploymentName'] == 'payments-api-prod', apply_record
assert apply_record['status'] == ('queued' if apply_enabled else 'failed'), apply_record
if apply_enabled:
    assert not apply_record.get('error'), apply_record
    assert apply_record['details'] == [
        'Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.',
        'Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.',
    ], apply_record
else:
    assert apply_record['error'] == 'apply execution is disabled', apply_record
assert isinstance(apply_record.get('details', []), list) and apply_record['details'], apply_record
assert isinstance(apply_record.get('lifecycle', {}), dict) and apply_record['lifecycle'], apply_record
assert apply_record['lifecycle']['updatedAt'] == apply_record['createdAt'], apply_record
assert apply_record['lifecycle']['totalSteps'] == len(apply_record['operations']), apply_record
if apply_enabled:
    assert 'finishedAt' not in apply_record['lifecycle'], apply_record
else:
    assert apply_record['lifecycle']['finishedAt'] == apply_record['createdAt'], apply_record
assert isinstance(apply_record.get('operations', []), list) and apply_record['operations'], apply_record
assert len(apply_record['operations']) == len(plan_preview['operations']), (apply_record, plan_preview)
assert [operation['status'] for operation in apply_record['operations']] == [('queued' if apply_enabled else 'pending')] * len(apply_record['operations']), apply_record
assert [operation['phase'] for operation in apply_record['operations']] == [operation['phase'] for operation in plan_preview['operations']], (apply_record, plan_preview)
assert [operation['target'] for operation in apply_record['operations']] == [operation['target'] for operation in plan_preview['operations']], (apply_record, plan_preview)
assert [operation['summary'] for operation in apply_record['operations']] == [operation['summary'] for operation in plan_preview['operations']], (apply_record, plan_preview)
assert apply_record['source']['route'] == f"/v1/plans/{plan_preview['id']}", apply_record
assert apply_record['source']['planStatus'] == 'ready', apply_record
assert apply_record['source']['fingerprint'] == plan_preview['source']['fingerprint'], (apply_record, plan_preview)
if apply_enabled:
    assert apply_record_get['id'] == apply_record['id'], (apply_record_get, apply_record)
    assert apply_record_get['status'] == 'failed', apply_record_get
    assert apply_record_get['error'] == 'apply executor handoff stopped before live AWS mutation because this backend slice is still local-safe', apply_record_get
    assert apply_record_get['lifecycle']['currentStep'] == 1, apply_record_get
    assert apply_record_get['lifecycle'].get('completedSteps', 0) == 0, apply_record_get
    assert apply_record_get['lifecycle']['totalSteps'] == len(apply_record_get['operations']), apply_record_get
    assert isinstance(apply_record_get['lifecycle'].get('startedAt'), str) and apply_record_get['lifecycle']['startedAt'], apply_record_get
    assert isinstance(apply_record_get['lifecycle'].get('finishedAt'), str) and apply_record_get['lifecycle']['finishedAt'], apply_record_get
    assert apply_record_get['details'] == [
        'Tenet executor advanced this apply request on the same applyId without calling the manual transition API.',
        'Tenet then marked the request failed because live AWS mutation is not implemented for this backend slice yet.',
        'The local-safe executor stopped at step 1 after recording the handoff state.',
    ], apply_record_get
    assert [operation['status'] for operation in apply_record_get['operations']] == ['failed'] + ['queued'] * (len(apply_record_get['operations']) - 1), apply_record_get
else:
    assert apply_record_get == apply_record, (apply_record_get, apply_record)
assert os.environ['INVALID_APPLY_TRANSITION_STATUS'] == '400', os.environ['INVALID_APPLY_TRANSITION_STATUS']
assert invalid_apply_transition.strip() == 'invalid apply transition request body', invalid_apply_transition
assert os.environ['UNKNOWN_APPLY_TRANSITION_STATUS'] == '404', os.environ['UNKNOWN_APPLY_TRANSITION_STATUS']
assert 'apply request not found' in unknown_apply_transition, unknown_apply_transition
assert os.environ['TERMINAL_APPLY_TRANSITION_STATUS'] == '409', os.environ['TERMINAL_APPLY_TRANSITION_STATUS']
assert 'apply request lifecycle transition is invalid:' in terminal_apply_transition, terminal_apply_transition
assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition
assert actor_plan_preview['deploymentName'] == 'payments-api-staging', actor_plan_preview
assert actor_plan_preview['actor']['available'] is True, actor_plan_preview
assert actor_plan_preview['actor']['id'] == 'explicit-456', actor_plan_preview
assert actor_plan_preview['actor']['name'] == 'Alex Explicit', actor_plan_preview
assert actor_plan_preview['actor']['email'] == 'alex.explicit@example.com', actor_plan_preview
assert actor_plan_preview_get == actor_plan_preview, (actor_plan_preview_get, actor_plan_preview)
assert actor_apply_record['deploymentName'] == 'payments-api-staging', actor_apply_record
assert actor_apply_record['actor']['available'] is True, actor_apply_record
assert actor_apply_record['actor']['id'] == 'explicit-456', actor_apply_record
assert actor_apply_record['actor']['name'] == 'Alex Explicit', actor_apply_record
assert actor_apply_record['actor']['email'] == 'alex.explicit@example.com', actor_apply_record
assert isinstance(actor_apply_record.get('operations', []), list) and actor_apply_record['operations'], actor_apply_record
assert all(operation['status'] == ('queued' if apply_enabled else 'pending') for operation in actor_apply_record['operations']), actor_apply_record
if apply_enabled:
    assert actor_apply_record_get['id'] == actor_apply_record['id'], (actor_apply_record_get, actor_apply_record)
    assert actor_apply_record_get['status'] == 'failed', actor_apply_record_get
    assert actor_apply_record_get['actor']['available'] is True, actor_apply_record_get
    assert actor_apply_record_get['actor']['id'] == 'explicit-456', actor_apply_record_get
    assert actor_apply_record_get['actor']['name'] == 'Alex Explicit', actor_apply_record_get
    assert actor_apply_record_get['actor']['email'] == 'alex.explicit@example.com', actor_apply_record_get
else:
    assert actor_apply_record_get == actor_apply_record, (actor_apply_record_get, actor_apply_record)
assert stale_plan_preview['status'] == 'failed', stale_plan_preview
assert stale_plan_preview['source']['fingerprint'] == deployment_detail['sourceFingerprint'], (stale_plan_preview, deployment_detail)
assert isinstance(stale_plan_preview.get('errors', []), list) and stale_plan_preview['errors'], stale_plan_preview
assert stale_apply_plan_preview['status'] == 'ready', stale_apply_plan_preview
assert os.environ['STALE_APPLY_CONFLICT_STATUS'] == '409', os.environ['STALE_APPLY_CONFLICT_STATUS']
assert 'create a new plan preview before applying' in stale_apply_conflict, stale_apply_conflict
assert stale_apply_plan_preview['source']['fingerprint'] in stale_apply_conflict, stale_apply_conflict
assert stale_apply_audit_logs['entries'] == [], stale_apply_audit_logs
assert isinstance(audit_logs.get('entries', []), list), audit_logs
assert len(audit_logs['entries']) >= 4, audit_logs
assert any(entry['action'] == 'apply.requested' for entry in audit_logs['entries']), audit_logs
assert any(entry['action'] == 'plan.created' for entry in audit_logs['entries']), audit_logs
assert isinstance(audit_logs_action_filtered.get('entries', []), list), audit_logs_action_filtered
assert len(audit_logs_action_filtered['entries']) >= 2, audit_logs_action_filtered
assert all(entry['action'] == 'apply.requested' for entry in audit_logs_action_filtered['entries']), audit_logs_action_filtered
for entry in audit_logs['entries']:
    assert entry['appRef'] == 'payments-api', entry
    assert entry['environmentRef'] in {'prod-apne2', 'staging-apne2'}, entry
    assert isinstance(entry.get('metadata', {}), dict), entry
    assert isinstance(entry.get('source', {}), dict), entry
fallback_entries = [entry for entry in audit_logs['entries'] if entry['deploymentName'] == 'payments-api-prod']
assert fallback_entries, audit_logs
for entry in fallback_entries:
    assert entry['actor']['available'] is False, entry
    assert 'no authenticated request context' in entry['actor']['reason'], entry
assert isinstance(actor_audit_logs.get('entries', []), list), actor_audit_logs
assert len(actor_audit_logs['entries']) == 1, actor_audit_logs
assert actor_audit_logs['entries'][0]['deploymentName'] == 'payments-api-staging', actor_audit_logs
assert actor_audit_logs['entries'][0]['actor']['available'] is True, actor_audit_logs
assert actor_audit_logs['entries'][0]['actor']['id'] == 'explicit-456', actor_audit_logs
assert actor_audit_logs['entries'][0]['actor']['name'] == 'Alex Explicit', actor_audit_logs
assert actor_audit_logs['entries'][0]['actor']['email'] == 'alex.explicit@example.com', actor_audit_logs
assert isinstance(audit_logs_filtered.get('entries', []), list), audit_logs_filtered
assert len(audit_logs_filtered['entries']) == 1, audit_logs_filtered
assert audit_logs_filtered['entries'][0]['deploymentName'] == 'payments-api-prod', audit_logs_filtered
assert isinstance(audit_logs_empty.get('entries', []), list), audit_logs_empty
assert audit_logs_empty['entries'] == [], audit_logs_empty
assert os.environ['INVALID_AUDIT_ACTION_STATUS'] == '400', os.environ['INVALID_AUDIT_ACTION_STATUS']
invalid_audit_action = read_text('INVALID_AUDIT_ACTION_BODY_PATH')
assert 'invalid action query' in invalid_audit_action, invalid_audit_action
assert status['deploymentName'] == 'payments-api-prod', status
assert webhook_push['deliveryId'] == 'delivery-push-1', webhook_push
assert webhook_push['event'] == 'push', webhook_push
assert webhook_push['status'] == 'accepted', webhook_push
assert webhook_push['processed'] is False, webhook_push
assert webhook_push['repository'] == 'acme/platform-config', webhook_push
assert webhook_push['ref'] == 'refs/heads/main', webhook_push
assert webhook_push['before'] == '1111111111111111111111111111111111111111', webhook_push
assert webhook_push['after'] == '2222222222222222222222222222222222222222', webhook_push
assert webhook_push['headCommit']['id'] == '2222222222222222222222222222222222222222', webhook_push
assert webhook_push['changedFiles']['all'] == ['deployments/api.yaml', 'accounts/prod.yaml', 'environments/prod.yaml', 'deployments/worker.yaml', 'deployments/old.yaml'], webhook_push
assert webhook_push['desiredStateSourceMatch']['configured'] is True, webhook_push
assert webhook_push['desiredStateSourceMatch']['kind'] == 'filesystem', webhook_push
assert webhook_push['desiredStateSourceMatch']['matches'] is False, webhook_push
assert webhook_push['desiredStateSourceMatch']['matchStatus'] == 'filesystem_source', webhook_push
assert webhook_push['desiredStateSourceMatch']['reason'] == 'desired-state source uses the filesystem catalog, so GitHub push relevance is unavailable in this mode', webhook_push
assert webhook_ping['deliveryId'] == 'delivery-ping-1', webhook_ping
assert webhook_ping['event'] == 'ping', webhook_ping
assert webhook_ping['status'] == 'ignored', webhook_ping
assert webhook_ping['processed'] is False, webhook_ping
assert 'desiredStateSourceMatch' not in webhook_ping, webhook_ping
assert os.environ['INVALID_WEBHOOK_STATUS'] == '400', os.environ['INVALID_WEBHOOK_STATUS']
assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook
assert status['status']['sync']['state'] == 'Synced', status
assert resource_logs['deploymentName'] == 'payments-api-prod', resource_logs
assert resource_logs['resourceType'] == 'trafficShiftPlan', resource_logs
assert resource_logs['resourceName'] == 'payments-api-prod-weighted-routing', resource_logs
assert isinstance(resource_logs['available'], bool), resource_logs
assert isinstance(resource_logs.get('lookupContext'), dict), resource_logs
assert isinstance(resource_logs['lookupContext'].get('region'), str) and resource_logs['lookupContext']['region'], resource_logs
assert isinstance(resource_logs['lookupContext'].get('controlAccountId'), str) and resource_logs['lookupContext']['controlAccountId'], resource_logs
assert isinstance(resource_logs['lookupContext'].get('targetAccountId'), str) and resource_logs['lookupContext']['targetAccountId'], resource_logs
assert isinstance(resource_logs['lookupContext'].get('crossAccount'), bool), resource_logs
assert isinstance(resource_logs['lookupContext'].get('observerRoleConfigured'), bool), resource_logs
assert resource_logs.get('entries') is None or isinstance(resource_logs.get('entries'), list), resource_logs
if resource_logs['available']:
    assert isinstance(resource_logs.get('source'), str), resource_logs
else:
    assert isinstance(resource_logs.get('unavailableReason'), str) and resource_logs['unavailableReason'], resource_logs
assert worker_resource_logs['deploymentName'] == 'worker-prod', worker_resource_logs
assert worker_resource_logs['resourceType'] == 'ecsService', worker_resource_logs
assert worker_resource_logs['resourceName'] == 'worker-prod-service', worker_resource_logs
assert isinstance(worker_resource_logs['available'], bool), worker_resource_logs
assert isinstance(worker_resource_logs.get('lookupContext'), dict), worker_resource_logs
assert isinstance(worker_resource_logs['lookupContext'].get('region'), str) and worker_resource_logs['lookupContext']['region'], worker_resource_logs
assert isinstance(worker_resource_logs['lookupContext'].get('controlAccountId'), str) and worker_resource_logs['lookupContext']['controlAccountId'], worker_resource_logs
assert isinstance(worker_resource_logs['lookupContext'].get('targetAccountId'), str) and worker_resource_logs['lookupContext']['targetAccountId'], worker_resource_logs
assert isinstance(worker_resource_logs['lookupContext'].get('crossAccount'), bool), worker_resource_logs
assert isinstance(worker_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), worker_resource_logs
assert worker_resource_logs.get('entries') is None or isinstance(worker_resource_logs.get('entries'), list), worker_resource_logs
if worker_resource_logs['available']:
    assert isinstance(worker_resource_logs.get('source'), str), worker_resource_logs
else:
    assert isinstance(worker_resource_logs.get('unavailableReason'), str) and worker_resource_logs['unavailableReason'], worker_resource_logs
assert worker_resource_inspection['deploymentName'] == 'worker-prod', worker_resource_inspection
assert worker_resource_inspection['resourceType'] == 'ecsService', worker_resource_inspection
assert worker_resource_inspection['resourceName'] == 'worker-prod-service', worker_resource_inspection
assert isinstance(worker_resource_inspection['available'], bool), worker_resource_inspection
assert isinstance(worker_resource_inspection.get('lookupContext'), dict), worker_resource_inspection
assert isinstance(worker_resource_inspection['lookupContext'].get('region'), str) and worker_resource_inspection['lookupContext']['region'], worker_resource_inspection
assert isinstance(worker_resource_inspection['lookupContext'].get('controlAccountId'), str) and worker_resource_inspection['lookupContext']['controlAccountId'], worker_resource_inspection
assert isinstance(worker_resource_inspection['lookupContext'].get('targetAccountId'), str) and worker_resource_inspection['lookupContext']['targetAccountId'], worker_resource_inspection
assert isinstance(worker_resource_inspection['lookupContext'].get('crossAccount'), bool), worker_resource_inspection
assert isinstance(worker_resource_inspection['lookupContext'].get('observerRoleConfigured'), bool), worker_resource_inspection
if worker_resource_inspection['available']:
    assert worker_resource_inspection.get('source') == 'ecs_service', worker_resource_inspection
    assert isinstance(worker_resource_inspection.get('ecsService'), dict), worker_resource_inspection
    assert isinstance(worker_resource_inspection['ecsService'].get('serviceName'), str) and worker_resource_inspection['ecsService']['serviceName'], worker_resource_inspection
    assert isinstance(worker_resource_inspection['ecsService'].get('deployments', []), list), worker_resource_inspection
    assert isinstance(worker_resource_inspection['ecsService'].get('events', []), list), worker_resource_inspection
else:
    assert isinstance(worker_resource_inspection.get('unavailableReason'), str) and worker_resource_inspection['unavailableReason'], worker_resource_inspection
assert candidate_resource_inspection['deploymentName'] == 'payments-api-prod', candidate_resource_inspection
assert candidate_resource_inspection['resourceType'] == 'candidateAutoScalingGroup', candidate_resource_inspection
assert candidate_resource_inspection['resourceName'] == 'payments-api-prod-candidate', candidate_resource_inspection
assert isinstance(candidate_resource_inspection['available'], bool), candidate_resource_inspection
assert isinstance(candidate_resource_inspection.get('lookupContext'), dict), candidate_resource_inspection
if candidate_resource_inspection['available']:
    assert candidate_resource_inspection.get('source') == 'auto_scaling_group', candidate_resource_inspection
    assert isinstance(candidate_resource_inspection.get('autoScalingGroup'), dict), candidate_resource_inspection
    assert isinstance(candidate_resource_inspection['autoScalingGroup'].get('autoScalingGroupName'), str) and candidate_resource_inspection['autoScalingGroup']['autoScalingGroupName'], candidate_resource_inspection
    launch_template = candidate_resource_inspection['autoScalingGroup'].get('launchTemplate')
    assert launch_template is None or isinstance(launch_template, dict), candidate_resource_inspection
    if isinstance(launch_template, dict):
        assert any(isinstance(launch_template.get(key), str) and launch_template.get(key) for key in ('id', 'name', 'version')), candidate_resource_inspection
    mixed_instances_policy = candidate_resource_inspection['autoScalingGroup'].get('mixedInstancesPolicy')
    assert mixed_instances_policy is None or isinstance(mixed_instances_policy, dict), candidate_resource_inspection
    if isinstance(mixed_instances_policy, dict):
        base_launch_template = mixed_instances_policy.get('launchTemplate')
        assert base_launch_template is None or isinstance(base_launch_template, dict), candidate_resource_inspection
        overrides = mixed_instances_policy.get('overrides')
        assert overrides is None or isinstance(overrides, list), candidate_resource_inspection
        for override in overrides or []:
            assert isinstance(override, dict), candidate_resource_inspection
            override_launch_template = override.get('launchTemplate')
            assert override_launch_template is None or isinstance(override_launch_template, dict), candidate_resource_inspection
    assert isinstance(candidate_resource_inspection['autoScalingGroup'].get('instances', []), list), candidate_resource_inspection
else:
    assert isinstance(candidate_resource_inspection.get('unavailableReason'), str) and candidate_resource_inspection['unavailableReason'], candidate_resource_inspection

def assert_summary_traffic_routing(payload, deployment_name, resource_type, resource_name):
    assert payload['deploymentName'] == deployment_name, payload
    assert payload['resourceType'] == resource_type, payload
    assert payload['resourceName'] == resource_name, payload
    assert payload['available'] is True, payload
    assert payload.get('source') == 'rollout_execution_summary', payload
    assert isinstance(payload.get('lookupContext'), dict), payload
    assert isinstance(payload.get('trafficRoutingResource'), dict), payload
    assert payload['trafficRoutingResource'].get('kind') == resource_type, payload
    assert payload['trafficRoutingResource'].get('inspectionMode') == 'summary_only', payload
    assert isinstance(payload['trafficRoutingResource'].get('summary'), str) and payload['trafficRoutingResource']['summary'], payload

assert_summary_traffic_routing(codedeploy_group_resource_inspection, 'payments-api-staging', 'codeDeployDeploymentGroup', 'payments-api-staging-codedeploy-group')
assert_summary_traffic_routing(production_listener_resource_inspection, 'payments-api-staging', 'productionListener', 'payments-api-staging-prod-listener')
assert_summary_traffic_routing(test_listener_resource_inspection, 'payments-api-staging', 'testListener', 'payments-api-staging-test-listener')
assert traffic_resource_inspection['deploymentName'] == 'payments-api-prod', traffic_resource_inspection
assert traffic_resource_inspection['resourceType'] == 'trafficShiftPlan', traffic_resource_inspection
assert traffic_resource_inspection['resourceName'] == 'payments-api-prod-weighted-routing', traffic_resource_inspection
assert traffic_resource_inspection['available'] is True, traffic_resource_inspection
assert isinstance(traffic_resource_inspection.get('lookupContext'), dict), traffic_resource_inspection
assert traffic_resource_inspection.get('source') == 'rollout_execution_summary', traffic_resource_inspection
assert isinstance(traffic_resource_inspection.get('trafficRoutingResource'), dict), traffic_resource_inspection
assert traffic_resource_inspection['trafficRoutingResource'].get('kind') == 'trafficShiftPlan', traffic_resource_inspection
assert traffic_resource_inspection['trafficRoutingResource'].get('inspectionMode') == 'summary_only', traffic_resource_inspection
assert isinstance(traffic_resource_inspection['trafficRoutingResource'].get('summary'), str) and traffic_resource_inspection['trafficRoutingResource']['summary'], traffic_resource_inspection
assert 'Application status overview' in ui, ui
assert '/v1/overview/applications' in ui, ui
assert '/v1/overview/applications/${encodeURIComponent(deploymentName)}' in ui, ui
assert 'Open account' in ui, ui
assert 'Open application' in ui, ui
assert 'Open audit log' in ui, ui
assert 'Runtime change details' in ui, ui
assert 'History change details' in ui, ui
assert "buildPageURL('account', { accountId: deployment.awsAccountId })" in ui, ui
assert "buildPageURL('application', { deployment: deployment.name })" in ui, ui
assert 'Drift view' in ui, ui
assert 'Plan/apply' in ui, ui
assert 'Current source: controller-authored managedDrift rows.' in ui, ui
assert 'Current source: derived fallback read-model heuristics.' in ui, ui
assert 'Drift summary' in ui, ui
assert 'Refresh drift workspace' in ui, ui
assert 'Refreshing drift workspace from GET /v1/overview/deployments/{deploymentName}…' in ui, ui
assert 'Failed to refresh drift workspace:' in ui, ui
assert 'Total rows' in ui, ui
assert 'Visible rows' in ui, ui
assert 'No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.' in ui, ui
assert 'Controller runtime events' in ui, ui
assert 'Refresh runtime events' in ui, ui
assert 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.' in ui, ui
assert 'Loading controller runtime events…' in ui, ui
assert 'No controller runtime events have been persisted yet.' in ui, ui
assert 'Controller runtime events are currently unavailable.' in ui, ui
assert 'Live inspection' in ui, ui
assert 'Loading live inspection…' in ui, ui
assert 'Live inspection is unavailable for this resource.' in ui, ui
assert 'Refresh live inspection' in ui, ui
assert 'No live inspection details are available for this resource yet.' in ui, ui
assert 'Service deployments' in ui, ui
assert 'Auto Scaling Group name' in ui, ui
assert 'Launch template' in ui, ui
assert 'Target group ARNs' in ui, ui
assert 'Mixed instances overrides' in ui, ui
assert "createNavLink(buildPageURL('drift', { deployment: detail.name }), 'Drift view', '_self')" in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name }), 'Plan/apply', '_self')" in ui, ui
assert "createNavLink(buildPageURL('audit', { deployment: detail.name }), 'Audit log', '_self')" in ui, ui
assert "createNavLink(buildPageURL('audit', { deployment: detail.name || currentPageState.deployment || '' }), 'Audit log', '_self')" in ui, ui
assert "const createResourceQuickNav = (detail, resource) => {" in ui, ui
assert "createElement('div', 'quick-nav-title', 'Quick navigation')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Sibling resources')" in ui, ui
assert "createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')" in ui, ui
assert "createNavLink(buildPageURL('resource', {" in ui, ui
assert "No sibling linked resources are available for this application." in ui, ui
assert 'Audit timeline' in audit_ui, audit_ui
assert 'Deployment audit timeline' in audit_ui, audit_ui
assert 'Active deployment context' in audit_ui, audit_ui
assert 'Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.' in audit_ui, audit_ui
assert 'No audit activity has been persisted yet for deployment ${currentPageState.deployment}.' in audit_ui, audit_ui
assert 'Loading audit log…' in audit_ui, audit_ui
assert 'Audit log is currently unavailable.' in audit_ui, audit_ui
assert 'Refresh audit log' in audit_ui, audit_ui
assert 'Refreshing audit log from GET /v1/audit-logs…' in audit_ui, audit_ui
assert 'Failed to refresh audit log:' in audit_ui, audit_ui
assert "const createAuditLogsActions = (isLoading = false) => [" in audit_ui, audit_ui
assert "const auditLogsRefreshStateText = (auditState) => {" in audit_ui, audit_ui
assert "const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {" in audit_ui, audit_ui
assert "if (action) params.set('action', action);" in audit_ui, audit_ui
assert "const normalizeAuditLimit = (value) => {" in audit_ui, audit_ui
assert "const auditLimitSelectOptions = (limit) => {" in audit_ui, audit_ui
assert "const syncAuditQueryParams = (params, pageState) => {" in audit_ui, audit_ui
assert "createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {" in audit_ui, audit_ui
assert "createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {" in audit_ui, audit_ui
assert 'Open application drill-down' in audit_ui, audit_ui
assert 'Open plan/apply workspace' in audit_ui, audit_ui
assert "fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`)" in ui, ui
assert "const ifMatch = detail.sourceFingerprint || await derivePlanSourceFingerprint(detail).catch(() => '') || fallbackIfMatch;" in ui, ui
assert 'Recent logs' in ui, ui
assert 'Refresh recent logs' in ui, ui
assert 'Lookup region' in ui, ui
assert 'Observer role configured' in ui, ui
assert 'Plan preview actions' in ui, ui
assert 'Apply request actions' in ui, ui
assert 'Plan preview approval guidance' in ui, ui
assert 'Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.' in ui, ui
assert 'Refresh plan preview' in ui, ui
assert 'Refresh apply request' in ui, ui
assert 'const loadPlanPreviewRecord = (planId) => {' in ui, ui
assert 'const loadApplyRequestRecord = (applyId) => {' in ui, ui
assert 'const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;' in ui, ui
assert 'const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;' in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')" in ui, ui
assert "createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')" in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')" in ui, ui
assert "createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')" in ui, ui
assert 'Server-authored plan preview loaded from GET /v1/plans/${activePlanRecord.id}.' in ui, ui
assert 'Future immutable lifecycle states render here as queued, running, succeeded, cancelled, or failed.' in ui, ui
assert 'Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.' in ui, ui
assert 'Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.' in ui, ui
assert 'Apply lifecycle summary' in ui, ui
assert "params.get('planId') || ''" in ui, ui
assert "params.get('applyId') || ''" in ui, ui

assert 'planId' in plan_ui, plan_ui
assert 'applyId' in apply_ui, apply_ui
assert 'Apply request failed' in apply_ui, apply_ui
assert 'Apply lifecycle summary' in apply_ui, apply_ui
assert 'Apply lifecycle metadata' in apply_ui, apply_ui
assert 'Lifecycle updated' in apply_ui, apply_ui
assert 'Current lifecycle step' in apply_ui, apply_ui
assert 'Completed / total steps' in apply_ui, apply_ui
assert 'Apply operations' in apply_ui, apply_ui
assert 'Queued apply operation' in apply_ui, apply_ui
assert '/v1/applies/' in apply_ui, apply_ui
assert 'The ordered execution sequence below comes from the persisted apply record' in apply_ui, apply_ui
print('smoke-local: fixture api path ok')
PY

stop_api
rm -f "$PLAN_STORE_PATH" "$APPLY_STORE_PATH"
start_api true true

auth_unauthenticated_plan_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_PLAN_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
auth_unauthenticated_apply_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_APPLY_BODY" -w "%{http_code}" \
  -X POST \
  "http://$HOST:$PORT/v1/applies/plans/$plan_id")
frontdoor_plan_preview=$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-staging")
frontdoor_plan_id=$(FRONTDOOR_PLAN_PREVIEW_JSON="$frontdoor_plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['FRONTDOOR_PLAN_PREVIEW_JSON'])['id'])
PY
)
frontdoor_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  "http://$HOST:$PORT/v1/applies/plans/$frontdoor_plan_id")
frontdoor_apply_id=$(FRONTDOOR_APPLY_RECORD_JSON="$frontdoor_apply_record" python3 - <<'PY'
import json, os
print(json.loads(os.environ['FRONTDOOR_APPLY_RECORD_JSON'])['id'])
PY
)
if [[ "$APPLY_ENABLED" == "true" ]]; then
  frontdoor_apply_record_get=$(wait_for_apply_status "$frontdoor_apply_id" failed)
else
  frontdoor_apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
fi
frontdoor_audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")

AUTH_UNAUTHENTICATED_PLAN_STATUS="$auth_unauthenticated_plan_status" AUTH_UNAUTHENTICATED_PLAN_BODY_PATH="$AUTH_UNAUTHENTICATED_PLAN_BODY" AUTH_UNAUTHENTICATED_APPLY_STATUS="$auth_unauthenticated_apply_status" AUTH_UNAUTHENTICATED_APPLY_BODY_PATH="$AUTH_UNAUTHENTICATED_APPLY_BODY" FRONTDOOR_PLAN_PREVIEW_JSON="$frontdoor_plan_preview" FRONTDOOR_APPLY_RECORD_JSON="$frontdoor_apply_record" FRONTDOOR_APPLY_RECORD_GET_JSON="$frontdoor_apply_record_get" FRONTDOOR_AUDIT_LOGS_JSON="$frontdoor_audit_logs" python3 - <<'PY'
import json, os

with open(os.environ['AUTH_UNAUTHENTICATED_PLAN_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_plan_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_APPLY_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_apply_body = fh.read()

frontdoor_plan_preview = json.loads(os.environ['FRONTDOOR_PLAN_PREVIEW_JSON'])
frontdoor_apply_record = json.loads(os.environ['FRONTDOOR_APPLY_RECORD_JSON'])
frontdoor_apply_record_get = json.loads(os.environ['FRONTDOOR_APPLY_RECORD_GET_JSON'])
frontdoor_audit_logs = json.loads(os.environ['FRONTDOOR_AUDIT_LOGS_JSON'])

assert os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS']
assert unauthenticated_plan_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_plan_body
assert os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS']
assert unauthenticated_apply_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_apply_body

for payload in (frontdoor_plan_preview, frontdoor_apply_record, frontdoor_apply_record_get):
    actor = payload['actor']
    assert actor['available'] is True, payload
    assert actor['id'] == 'front-door-123', payload
    assert actor['name'] == 'Front Door Operator', payload
    assert actor['email'] == 'frontdoor@example.com', payload

entries = frontdoor_audit_logs['entries']
assert len(entries) == 1, frontdoor_audit_logs
entry = entries[0]
assert entry['action'] == 'apply.requested', entry
assert entry['actor']['available'] is True, entry
assert entry['actor']['id'] == 'front-door-123', entry
assert entry['actor']['name'] == 'Front Door Operator', entry
assert entry['actor']['email'] == 'frontdoor@example.com', entry
print('smoke-local: front-door auth path ok')
PY

TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" TENET_CONTROLLER_REQUEUE_SECONDS=1 TENET_CONTROLLER_WORKERS=1 \
  go run ./cmd/tenet-controller >"$CONTROLLER_LOG_FILE" 2>&1 &
CONTROLLER_PID=$!

controller_ready=0
for _ in $(seq 1 30); do
  controller_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod" 2>/dev/null || true)
  if [[ -z "$controller_detail" ]]; then
    sleep 1
    continue
  fi
  if CONTROLLER_DETAIL_JSON="$controller_detail" python3 - <<'PY'
import json, os, sys
payload = json.loads(os.environ['CONTROLLER_DETAIL_JSON'])
runtime_change_details = (payload.get('runtimeSummary') or {}).get('changeDetails') or []
history = payload.get('history') or []
history_change_details = (history[0].get('changeDetails') if history else []) or []
if runtime_change_details and history_change_details:
    sys.exit(0)
sys.exit(1)
PY
  then
    controller_ready=1
    break
  fi
  sleep 1
done

if [[ "$controller_ready" -ne 1 ]]; then
  echo "controller did not populate runtime/history changeDetails" >&2
  cat "$CONTROLLER_LOG_FILE" >&2 || true
  exit 1
fi

controller_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod")
controller_events=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/events")
CONTROLLER_DETAIL_JSON="$controller_detail" CONTROLLER_EVENTS_JSON="$controller_events" python3 - <<'PY'
import json, os
payload = json.loads(os.environ['CONTROLLER_DETAIL_JSON'])
events_payload = json.loads(os.environ['CONTROLLER_EVENTS_JSON'])
runtime_change_details = (payload.get('runtimeSummary') or {}).get('changeDetails') or []
history = payload.get('history') or []
history_change_details = (history[0].get('changeDetails') if history else []) or []
managed_drift = payload.get('managedDrift') or []
runtime_managed_drift = (payload.get('runtimeSummary') or {}).get('managedDrift') or []
history_managed_drift = (history[0].get('managedDrift') if history else []) or []
assert runtime_change_details, payload
assert history_change_details, payload
assert managed_drift, payload
assert runtime_managed_drift == managed_drift, payload
assert history_managed_drift == managed_drift, payload
assert all(isinstance(item.get('field'), str) and item['field'] for item in managed_drift), managed_drift
assert all(item.get('severity') in {'high', 'medium', 'low'} for item in managed_drift), managed_drift
assert events_payload['deploymentName'] == 'payments-api-prod', events_payload
assert isinstance(events_payload['events'], list) and events_payload['events'], events_payload
for event in events_payload['events']:
    assert event.get('source') == 'controller', events_payload
print('smoke-local: controller changeDetails path ok')
PY

LOG_FILE_PATH="$LOG_FILE" python3 - <<'PY'
import json, os

entries = []
with open(os.environ["LOG_FILE_PATH"], "r", encoding="utf-8") as handle:
    for raw in handle:
        line = raw.strip()
        if not line.startswith("{"):
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if entry.get("msg") == "http_request":
            entries.append(entry)

assert entries, "expected structured http_request log lines"
healthz = next((entry for entry in entries if entry.get("path") == "/healthz"), None)
assert healthz, entries
for field in ("request_id", "method", "path", "status", "duration_ms", "result"):
    assert field in healthz, healthz
assert healthz["method"] == "GET", healthz
assert healthz["status"] == 200, healthz
assert healthz["result"] == "success", healthz
PY
