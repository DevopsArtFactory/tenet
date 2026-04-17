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
cp "$FIXTURE_STATUS_PATH" "$STATUS_PATH"
rm -f "$PLAN_STORE_PATH"
LOG_FILE=$(mktemp)
CONTROLLER_LOG_FILE=$(mktemp)
cleanup() {
  if [[ -n "${CONTROLLER_PID:-}" ]] && kill -0 "$CONTROLLER_PID" 2>/dev/null; then
    kill "$CONTROLLER_PID" 2>/dev/null || true
    wait "$CONTROLLER_PID" 2>/dev/null || true
  fi
  if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  rm -f "$LOG_FILE" "$CONTROLLER_LOG_FILE" "$STATUS_PATH" "$PLAN_STORE_PATH"
}
trap cleanup EXIT

TENET_API_ADDR=":$PORT" TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" TENET_PLAN_STORE_PATH="$PLAN_STORE_PATH" \
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
applications=$(curl -fsS "http://$HOST:$PORT/v1/overview/applications")
application_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/applications/payments-api-prod")
overview=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts")
account_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012")
group_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2")
deployment_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod")
plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d '{}' "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
plan_id=$(PLAN_PREVIEW_JSON="$plan_preview" python3 - <<'PY'
import json, os
print(json.loads(os.environ['PLAN_PREVIEW_JSON'])['id'])
PY
)
plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$plan_id")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
ui=$(curl -fsS "http://$HOST:$PORT/")

SUMMARY_JSON="$summary" APPLICATIONS_JSON="$applications" APPLICATION_DETAIL_JSON="$application_detail" OVERVIEW_JSON="$overview" ACCOUNT_DETAIL_JSON="$account_detail" GROUP_DETAIL_JSON="$group_detail" DEPLOYMENT_DETAIL_JSON="$deployment_detail" PLAN_PREVIEW_JSON="$plan_preview" PLAN_PREVIEW_GET_JSON="$plan_preview_get" STATUS_JSON="$status" RESOURCE_LOGS_JSON="$resource_logs" WORKER_RESOURCE_LOGS_JSON="$worker_resource_logs" UI_HTML="$ui" python3 - <<'PY'
import json, os
summary = json.loads(os.environ['SUMMARY_JSON'])
applications = json.loads(os.environ['APPLICATIONS_JSON'])
application_detail = json.loads(os.environ['APPLICATION_DETAIL_JSON'])
overview = json.loads(os.environ['OVERVIEW_JSON'])
account_detail = json.loads(os.environ['ACCOUNT_DETAIL_JSON'])
group_detail = json.loads(os.environ['GROUP_DETAIL_JSON'])
deployment_detail = json.loads(os.environ['DEPLOYMENT_DETAIL_JSON'])
plan_preview = json.loads(os.environ['PLAN_PREVIEW_JSON'])
plan_preview_get = json.loads(os.environ['PLAN_PREVIEW_GET_JSON'])
status = json.loads(os.environ['STATUS_JSON'])
resource_logs = json.loads(os.environ['RESOURCE_LOGS_JSON'])
worker_resource_logs = json.loads(os.environ['WORKER_RESOURCE_LOGS_JSON'])
ui = os.environ['UI_HTML']
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
assert deployment_detail['name'] == 'payments-api-prod', deployment_detail
assert isinstance(deployment_runtime_summary.get('changeDetails', []), list), deployment_detail
if deployment_history:
    assert isinstance(deployment_history[0].get('changeDetails', []), list), deployment_detail
assert plan_preview['deploymentName'] == 'payments-api-prod', plan_preview
assert plan_preview['status'] in {'ready', 'failed'}, plan_preview
assert plan_preview['source']['route'] == '/v1/overview/deployments/payments-api-prod', plan_preview
assert isinstance(plan_preview.get('operations', []), list), plan_preview
assert isinstance((plan_preview.get('risk') or {}).get('reasons', []), list), plan_preview
assert isinstance((plan_preview.get('approval') or {}).get('required'), bool), plan_preview
assert plan_preview_get == plan_preview, (plan_preview_get, plan_preview)
assert status['deploymentName'] == 'payments-api-prod', status
assert status['status']['sync']['state'] == 'Synced', status
assert resource_logs['deploymentName'] == 'payments-api-prod', resource_logs
assert resource_logs['resourceType'] == 'trafficShiftPlan', resource_logs
assert resource_logs['resourceName'] == 'payments-api-prod-weighted-routing', resource_logs
assert isinstance(resource_logs['available'], bool), resource_logs
assert resource_logs.get('entries') is None or isinstance(resource_logs.get('entries'), list), resource_logs
if resource_logs['available']:
    assert isinstance(resource_logs.get('source'), str), resource_logs
else:
    assert isinstance(resource_logs.get('unavailableReason'), str) and resource_logs['unavailableReason'], resource_logs
assert worker_resource_logs['deploymentName'] == 'worker-prod', worker_resource_logs
assert worker_resource_logs['resourceType'] == 'ecsService', worker_resource_logs
assert worker_resource_logs['resourceName'] == 'worker-prod-service', worker_resource_logs
assert isinstance(worker_resource_logs['available'], bool), worker_resource_logs
assert worker_resource_logs.get('entries') is None or isinstance(worker_resource_logs.get('entries'), list), worker_resource_logs
if worker_resource_logs['available']:
    assert isinstance(worker_resource_logs.get('source'), str), worker_resource_logs
else:
    assert isinstance(worker_resource_logs.get('unavailableReason'), str) and worker_resource_logs['unavailableReason'], worker_resource_logs
assert 'Application status overview' in ui, ui
assert '/v1/overview/applications' in ui, ui
assert '/v1/overview/applications/${encodeURIComponent(deploymentName)}' in ui, ui
assert 'Open account' in ui, ui
assert 'Open application' in ui, ui
assert 'Runtime change details' in ui, ui
assert 'History change details' in ui, ui
assert "buildPageURL('account', { accountId: deployment.awsAccountId })" in ui, ui
assert "buildPageURL('application', { deployment: deployment.name })" in ui, ui
assert 'Drift view' in ui, ui
assert 'Plan/apply' in ui, ui
assert 'Recent logs' in ui, ui
assert 'Derived approval guidance' in ui, ui
assert 'Derived risk level' in ui, ui
assert 'Plan preview is derived from rollout steps, rollout execution resources, and the latest runtime summary in the deployment detail read model.' in ui, ui

assert "buildPageURL('drift', { deployment: deployment.name })" in ui, ui
assert "buildPageURL('plan', { deployment: deployment.name })" in ui, ui
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
CONTROLLER_DETAIL_JSON="$controller_detail" python3 - <<'PY'
import json, os
payload = json.loads(os.environ['CONTROLLER_DETAIL_JSON'])
runtime_change_details = (payload.get('runtimeSummary') or {}).get('changeDetails') or []
history = payload.get('history') or []
history_change_details = (history[0].get('changeDetails') if history else []) or []
assert runtime_change_details, payload
assert history_change_details, payload
print('smoke-local: controller changeDetails path ok')
PY
