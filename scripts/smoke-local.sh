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
applications=$(curl -fsS "http://$HOST:$PORT/v1/overview/applications")
overview=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts")
account_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012")
group_detail=$(curl -fsS "http://$HOST:$PORT/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2")
status=$(curl -fsS "http://$HOST:$PORT/v1/status/deployments/payments-api-prod")
resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
ui=$(curl -fsS "http://$HOST:$PORT/")

SUMMARY_JSON="$summary" APPLICATIONS_JSON="$applications" OVERVIEW_JSON="$overview" ACCOUNT_DETAIL_JSON="$account_detail" GROUP_DETAIL_JSON="$group_detail" STATUS_JSON="$status" RESOURCE_LOGS_JSON="$resource_logs" UI_HTML="$ui" python3 - <<'PY'
import json, os
summary = json.loads(os.environ['SUMMARY_JSON'])
applications = json.loads(os.environ['APPLICATIONS_JSON'])
overview = json.loads(os.environ['OVERVIEW_JSON'])
account_detail = json.loads(os.environ['ACCOUNT_DETAIL_JSON'])
group_detail = json.loads(os.environ['GROUP_DETAIL_JSON'])
status = json.loads(os.environ['STATUS_JSON'])
resource_logs = json.loads(os.environ['RESOURCE_LOGS_JSON'])
ui = os.environ['UI_HTML']
assert summary['accountCount'] == 2, summary
assert summary['environmentCount'] == 2, summary
assert summary['deploymentCount'] == 2, summary
assert applications[0]['executionMode'] in {'asgWeightedTargetGroups', 'asgInstanceRefresh'}, applications
assert any(application['name'] == 'payments-api-prod' for application in applications), applications
assert overview[0]['deployments'][0]['executionMode'] in {'asgWeightedTargetGroups', 'asgInstanceRefresh'}, overview
assert any(account['name'] == 'prod-core' for account in overview), overview
assert account_detail['name'] == 'prod-core', account_detail
assert account_detail['awsAccountId'] == '123456789012', account_detail
assert len(account_detail['deployments']) >= 1, account_detail
assert group_detail['accountName'] == 'prod-core', group_detail
assert group_detail['awsAccountId'] == '123456789012', group_detail
assert group_detail['appRef'] == 'payments-api', group_detail
assert group_detail['environmentRef'] == 'prod-apne2', group_detail
assert group_detail['region'] == 'ap-northeast-2', group_detail
assert len(group_detail['deployments']) == 1, group_detail
assert group_detail['deployments'][0]['name'] == 'payments-api-prod', group_detail
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
assert 'Application status overview' in ui, ui
assert '/v1/overview/applications' in ui, ui
assert 'Open account' in ui, ui
assert 'Open application' in ui, ui
assert "buildPageURL('account', { accountId: deployment.awsAccountId })" in ui, ui
assert "buildPageURL('application', { deployment: deployment.name })" in ui, ui
assert 'Drift view' in ui, ui
assert 'Plan/apply' in ui, ui
assert 'Recent logs' in ui, ui

assert "buildPageURL('drift', { deployment: deployment.name })" in ui, ui
assert "buildPageURL('plan', { deployment: deployment.name })" in ui, ui
print('smoke-local: ok')
PY
