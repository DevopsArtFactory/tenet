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
FIXTURE_STATUS_PATH=${TENET_STATUS_PATH:-$ROOT/examples/local-status/deployments.json}
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
cleanup() {
  if [[ -n "${CONTROLLER_PID:-}" ]] && kill -0 "$CONTROLLER_PID" 2>/dev/null; then
    kill "$CONTROLLER_PID" 2>/dev/null || true
    wait "$CONTROLLER_PID" 2>/dev/null || true
  fi
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE" "$CONTROLLER_LOG_FILE" "$STATUS_PATH" "$PLAN_STORE_PATH" "$APPLY_STORE_PATH" "$INVALID_AUDIT_ACTION_BODY"
  rm -rf "$PAYLOAD_DIR"
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

TENET_API_ADDR=":$PORT" TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" TENET_PLAN_STORE_PATH="$PLAN_STORE_PATH" TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH" \
  go run ./cmd/tenet-api >"$LOG_FILE" 2>&1 &
API_PID=$!

for _ in $(seq 1 30); do
  if curl -fsS "http://$HOST:$PORT/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

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
apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id")
actor_plan_preview=$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Actor-Id: operator-123" \
  -H "X-Tenet-Actor-Name: Jane Operator" \
  -H "X-Tenet-Actor-Email: jane@example.com" \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-staging")
actor_plan_id=$(ACTOR_PLAN_PREVIEW_JSON="$actor_plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['ACTOR_PLAN_PREVIEW_JSON'])['id'])
PY
)
actor_plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$actor_plan_id")
actor_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Actor-Id: operator-123" \
  -H "X-Tenet-Actor-Name: Jane Operator" \
  -H "X-Tenet-Actor-Email: jane@example.com" \
  "http://$HOST:$PORT/v1/applies/plans/$actor_plan_id")
actor_apply_id=$(ACTOR_APPLY_RECORD_JSON="$actor_apply_record" python3 - <<'PY'
import json, os
print(json.loads(os.environ['ACTOR_APPLY_RECORD_JSON'])['id'])
PY
)
actor_apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$actor_apply_id")
stale_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d '{"ifMatch":"plan_deadbeef"}' "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs")
audit_logs_action_filtered=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?action=apply.requested")
audit_logs_filtered=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1")
audit_logs_empty=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod")
actor_audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
invalid_audit_action_status=$(curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/audit-logs?action=invalid.action")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
worker_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
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
actor_plan_preview_path=$(write_payload actor-plan-preview.json "$actor_plan_preview")
actor_plan_preview_get_path=$(write_payload actor-plan-preview-get.json "$actor_plan_preview_get")
actor_apply_record_path=$(write_payload actor-apply-record.json "$actor_apply_record")
actor_apply_record_get_path=$(write_payload actor-apply-record-get.json "$actor_apply_record_get")
stale_plan_preview_path=$(write_payload stale-plan-preview.json "$stale_plan_preview")
audit_logs_path=$(write_payload audit-logs.json "$audit_logs")
audit_logs_action_filtered_path=$(write_payload audit-logs-action-filtered.json "$audit_logs_action_filtered")
audit_logs_filtered_path=$(write_payload audit-logs-filtered.json "$audit_logs_filtered")
audit_logs_empty_path=$(write_payload audit-logs-empty.json "$audit_logs_empty")
actor_audit_logs_path=$(write_payload actor-audit-logs.json "$actor_audit_logs")
status_json_path=$(write_payload status.json "$status")
resource_logs_path=$(write_payload resource-logs.json "$resource_logs")
worker_resource_logs_path=$(write_payload worker-resource-logs.json "$worker_resource_logs")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
candidate_resource_inspection_path=$(write_payload candidate-resource-inspection.json "$candidate_resource_inspection")
ui_path=$(write_payload ui.html "$ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_ui_path=$(write_payload plan-ui.html "$plan_ui")
apply_ui_path=$(write_payload apply-ui.html "$apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" APPLICATION_DETAIL_PATH="$application_detail_path" OVERVIEW_PATH="$overview_path" ACCOUNT_DETAIL_PATH="$account_detail_path" GROUP_DETAIL_PATH="$group_detail_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_EVENTS_FIXTURE_PATH="$deployment_events_fixture_path" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" ACTOR_PLAN_PREVIEW_PATH="$actor_plan_preview_path" ACTOR_PLAN_PREVIEW_GET_PATH="$actor_plan_preview_get_path" ACTOR_APPLY_RECORD_PATH="$actor_apply_record_path" ACTOR_APPLY_RECORD_GET_PATH="$actor_apply_record_get_path" STALE_PLAN_PREVIEW_PATH="$stale_plan_preview_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" AUDIT_LOGS_FILTERED_PATH="$audit_logs_filtered_path" AUDIT_LOGS_EMPTY_PATH="$audit_logs_empty_path" ACTOR_AUDIT_LOGS_PATH="$actor_audit_logs_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" STATUS_JSON_PATH="$status_json_path" RESOURCE_LOGS_PATH="$resource_logs_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" UI_PATH="$ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_UI_PATH="$plan_ui_path" APPLY_UI_PATH="$apply_ui_path" python3 - <<'PY'
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
actor_plan_preview = read_json('ACTOR_PLAN_PREVIEW_PATH')
actor_plan_preview_get = read_json('ACTOR_PLAN_PREVIEW_GET_PATH')
actor_apply_record = read_json('ACTOR_APPLY_RECORD_PATH')
actor_apply_record_get = read_json('ACTOR_APPLY_RECORD_GET_PATH')
stale_plan_preview = read_json('STALE_PLAN_PREVIEW_PATH')
audit_logs = read_json('AUDIT_LOGS_PATH')
audit_logs_action_filtered = read_json('AUDIT_LOGS_ACTION_FILTERED_PATH')
audit_logs_filtered = read_json('AUDIT_LOGS_FILTERED_PATH')
audit_logs_empty = read_json('AUDIT_LOGS_EMPTY_PATH')
actor_audit_logs = read_json('ACTOR_AUDIT_LOGS_PATH')
status = read_json('STATUS_JSON_PATH')
resource_logs = read_json('RESOURCE_LOGS_PATH')
worker_resource_logs = read_json('WORKER_RESOURCE_LOGS_PATH')
worker_resource_inspection = read_json('WORKER_RESOURCE_INSPECTION_PATH')
candidate_resource_inspection = read_json('CANDIDATE_RESOURCE_INSPECTION_PATH')
ui = read_text('UI_PATH')
audit_ui = read_text('AUDIT_UI_PATH')
plan_ui = read_text('PLAN_UI_PATH')
apply_ui = read_text('APPLY_UI_PATH')
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
assert apply_record['status'] == 'failed', apply_record
assert apply_record['error'] == 'apply execution is disabled', apply_record
assert isinstance(apply_record.get('details', []), list) and apply_record['details'], apply_record
assert apply_record['source']['route'] == f"/v1/plans/{plan_preview['id']}", apply_record
assert apply_record['source']['planStatus'] == 'ready', apply_record
assert apply_record['source']['fingerprint'] == plan_preview['source']['fingerprint'], (apply_record, plan_preview)
assert apply_record_get == apply_record, (apply_record_get, apply_record)
assert actor_plan_preview['deploymentName'] == 'payments-api-staging', actor_plan_preview
assert actor_plan_preview['actor']['available'] is True, actor_plan_preview
assert actor_plan_preview['actor']['name'] == 'Jane Operator', actor_plan_preview
assert actor_plan_preview['actor']['email'] == 'jane@example.com', actor_plan_preview
assert actor_plan_preview_get == actor_plan_preview, (actor_plan_preview_get, actor_plan_preview)
assert actor_apply_record['deploymentName'] == 'payments-api-staging', actor_apply_record
assert actor_apply_record['actor']['available'] is True, actor_apply_record
assert actor_apply_record['actor']['name'] == 'Jane Operator', actor_apply_record
assert actor_apply_record['actor']['email'] == 'jane@example.com', actor_apply_record
assert actor_apply_record_get == actor_apply_record, (actor_apply_record_get, actor_apply_record)
assert stale_plan_preview['status'] == 'failed', stale_plan_preview
assert stale_plan_preview['source']['fingerprint'] == deployment_detail['sourceFingerprint'], (stale_plan_preview, deployment_detail)
assert isinstance(stale_plan_preview.get('errors', []), list) and stale_plan_preview['errors'], stale_plan_preview
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
assert actor_audit_logs['entries'][0]['actor']['name'] == 'Jane Operator', actor_audit_logs
assert actor_audit_logs['entries'][0]['actor']['email'] == 'jane@example.com', actor_audit_logs
assert isinstance(audit_logs_filtered.get('entries', []), list), audit_logs_filtered
assert len(audit_logs_filtered['entries']) == 1, audit_logs_filtered
assert audit_logs_filtered['entries'][0]['deploymentName'] == 'payments-api-prod', audit_logs_filtered
assert isinstance(audit_logs_empty.get('entries', []), list), audit_logs_empty
assert audit_logs_empty['entries'] == [], audit_logs_empty
assert os.environ['INVALID_AUDIT_ACTION_STATUS'] == '400', os.environ['INVALID_AUDIT_ACTION_STATUS']
invalid_audit_action = read_text('INVALID_AUDIT_ACTION_BODY_PATH')
assert 'invalid action query' in invalid_audit_action, invalid_audit_action
assert status['deploymentName'] == 'payments-api-prod', status
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
else:
    assert isinstance(candidate_resource_inspection.get('unavailableReason'), str) and candidate_resource_inspection['unavailableReason'], candidate_resource_inspection
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
assert 'Controller runtime events' in ui, ui
assert 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.' in ui, ui
assert 'Loading controller runtime events…' in ui, ui
assert 'No controller runtime events have been persisted yet.' in ui, ui
assert 'Controller runtime events are currently unavailable.' in ui, ui
assert "createNavLink(buildPageURL('drift', { deployment: detail.name }), 'Drift view', '_self')" in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name }), 'Plan/apply', '_self')" in ui, ui
assert "createNavLink(buildPageURL('audit', { deployment: detail.name }), 'Audit log', '_self')" in ui, ui
assert "createNavLink(buildPageURL('audit', { deployment: detail.name || currentPageState.deployment || '' }), 'Audit log', '_self')" in ui, ui
assert 'Audit timeline' in audit_ui, audit_ui
assert 'Deployment audit timeline' in audit_ui, audit_ui
assert 'Active deployment context' in audit_ui, audit_ui
assert 'Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.' in audit_ui, audit_ui
assert 'No audit activity has been persisted yet for deployment ${currentPageState.deployment}.' in audit_ui, audit_ui
assert 'Loading audit log…' in audit_ui, audit_ui
assert 'Audit log is currently unavailable.' in audit_ui, audit_ui
assert "const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {" in audit_ui, audit_ui
assert "if (action) params.set('action', action);" in audit_ui, audit_ui
assert "createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {" in audit_ui, audit_ui
assert 'Open application drill-down' in audit_ui, audit_ui
assert 'Open plan/apply workspace' in audit_ui, audit_ui
assert "fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}/events`)" in ui, ui
assert "const ifMatch = detail.sourceFingerprint || await derivePlanSourceFingerprint(detail).catch(() => '') || fallbackIfMatch;" in ui, ui
assert 'Recent logs' in ui, ui
assert 'Plan preview actions' in ui, ui
assert 'Apply request actions' in ui, ui
assert 'Plan preview approval guidance' in ui, ui
assert 'Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.' in ui, ui
assert 'const buildPlanPreviewRoute = (planId) => `/v1/plans/${encodeURIComponent(planId)}`;' in ui, ui
assert 'const buildApplyRequestRoute = (applyId) => `/v1/applies/${encodeURIComponent(applyId)}`;' in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activePlanRecord.id, applyId: activeApplyRecord?.id || applyState?.applyId || '' }), 'Open bookmarked workspace', '_self')" in ui, ui
assert "createNavLink(buildPlanPreviewRoute(activePlanRecord.id), 'Plan JSON')" in ui, ui
assert "createNavLink(buildPageURL('plan', { deployment: detail.name || currentPageState.deployment, planId: activeApplyRecord.planId || activePlanRecord?.id || currentPageState.planId, applyId: activeApplyRecord.id }), 'Open bookmarked apply', '_self')" in ui, ui
assert "createNavLink(buildApplyRequestRoute(activeApplyRecord.id), 'Apply JSON')" in ui, ui
assert 'Server-authored plan preview loaded from GET /v1/plans/${activePlanRecord.id}.' in ui, ui
assert 'Apply Plan will POST /v1/applies/plans/${planState.record.id}, persist an immutable apply request, and update this workspace URL with a shareable applyId.' in ui, ui
assert 'Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.' in ui, ui
assert 'Live execution is intentionally disabled by default, so failed immutable apply records are expected and useful for operator review.' in ui, ui
assert "params.get('planId') || ''" in ui, ui
assert "params.get('applyId') || ''" in ui, ui

assert 'planId' in plan_ui, plan_ui
assert 'applyId' in apply_ui, apply_ui
print('smoke-local: fixture api path ok')
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
