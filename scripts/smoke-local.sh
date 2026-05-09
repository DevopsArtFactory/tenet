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
AUTH_ROLE_ENFORCED=${TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION:-false}
AUTH_READ_ENFORCED=${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}
WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}
STATUS_PATH=$(mktemp)
PLAN_STORE_PATH=$(mktemp)
APPLY_STORE_PATH=$(mktemp)
ENTERPRISE_SSO_STATE_STORE_PATH=$(mktemp)
ENTERPRISE_SSO_IDENTITY_STORE_PATH=$(mktemp)
PAYLOAD_DIR=$(mktemp -d)
cp "$FIXTURE_STATUS_PATH" "$STATUS_PATH"
rm -f "$PLAN_STORE_PATH"
rm -f "$APPLY_STORE_PATH"
LOG_FILE=$(mktemp)
CONTROLLER_LOG_FILE=$(mktemp)
INVALID_AUDIT_ACTION_BODY=$(mktemp)
INVALID_RESOURCE_LOG_LIMIT_BODY=$(mktemp)
INVALID_RESOURCE_LOG_INSTANCE_BODY=$(mktemp)
INVALID_RESOURCE_INSPECTION_LIMIT_BODY=$(mktemp)
INVALID_RESOURCE_INSPECTION_INSTANCE_BODY=$(mktemp)
STALE_APPLY_CONFLICT_BODY=$(mktemp)
INVALID_APPLY_TRANSITION_BODY=$(mktemp)
UNKNOWN_APPLY_TRANSITION_BODY=$(mktemp)
TERMINAL_APPLY_TRANSITION_BODY=$(mktemp)
INVALID_WEBHOOK_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_PLAN_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_APPLY_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_TRANSITION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_COLLECTION_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_STATUS_READ_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_READ_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_STATUS_READ_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_EVENTS_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY=$(mktemp)
AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY=$(mktemp)
AUTH_FORBIDDEN_GROUP_DETAIL_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_PLAN_READ_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_APPLY_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_STATUS_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_EVENTS_BODY=$(mktemp)
AUTH_FORBIDDEN_RESOURCE_LOGS_BODY=$(mktemp)
AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=$(mktemp)
AUTH_FORBIDDEN_PLAN_WRITE_BODY=$(mktemp)
AUTH_FORBIDDEN_APPLY_WRITE_BODY=$(mktemp)
AUTH_FORBIDDEN_TRANSITION_BODY=$(mktemp)
AUTH_FORBIDDEN_PLAN_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_APPLY_READ_BODY=$(mktemp)
AUTH_FORBIDDEN_AUDIT_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_PLAN_READ_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_APPLY_READ_BODY=$(mktemp)
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
  rm -f "$LOG_FILE" "$CONTROLLER_LOG_FILE" "$STATUS_PATH" "$PLAN_STORE_PATH" "$APPLY_STORE_PATH" "$ENTERPRISE_SSO_STATE_STORE_PATH" "$ENTERPRISE_SSO_IDENTITY_STORE_PATH" "$INVALID_AUDIT_ACTION_BODY" "$INVALID_RESOURCE_LOG_LIMIT_BODY" "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY" "$STALE_APPLY_CONFLICT_BODY" "$INVALID_APPLY_TRANSITION_BODY" "$UNKNOWN_APPLY_TRANSITION_BODY" "$TERMINAL_APPLY_TRANSITION_BODY" "$INVALID_WEBHOOK_BODY" "$AUTH_UNAUTHENTICATED_PLAN_BODY" "$AUTH_UNAUTHENTICATED_APPLY_BODY" "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" "$AUTH_ROLE_REQUIRED_COLLECTION_BODY" "$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" "$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_READ_BODY" "$AUTH_UNAUTHENTICATED_STATUS_READ_BODY" "$AUTH_UNAUTHENTICATED_EVENTS_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" "$AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY" "$AUTH_FORBIDDEN_GROUP_DETAIL_BODY" "$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" "$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" "$AUTH_FORBIDDEN_READ_BODY" "$AUTH_FORBIDDEN_STATUS_READ_BODY" "$AUTH_FORBIDDEN_EVENTS_BODY" "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" "$AUTH_FORBIDDEN_PLAN_WRITE_BODY" "$AUTH_FORBIDDEN_APPLY_WRITE_BODY" "$AUTH_FORBIDDEN_TRANSITION_BODY" "$AUTH_FORBIDDEN_PLAN_READ_BODY" "$AUTH_FORBIDDEN_APPLY_READ_BODY" "$AUTH_FORBIDDEN_AUDIT_BODY" "$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" "$AUTH_ROLE_REQUIRED_APPLY_READ_BODY"
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
  local response_path

  for _ in $(seq 1 50); do
    response=$(curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id")
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

assert_enterprise_sso_diagnostic() {
  local payload_path=$1
  local provider=$2
  local expected_login_route=$3
  local expected_callback_route=$4
  local expected_active=$5
  shift 5

  python3 - "$payload_path" "$provider" "$expected_login_route" "$expected_callback_route" "$expected_active" "$@" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
provider = sys.argv[2]
expected_login_route = sys.argv[3]
expected_callback_route = sys.argv[4]
expected_active = sys.argv[5] == 'true'
expected_required = list(sys.argv[6:])
enterprise_sso = (payload.get('authRuntime') or {}).get('enterpriseSso') or {}
assert enterprise_sso.get('configured') is True, payload
assert enterprise_sso.get('active') is expected_active, payload
assert enterprise_sso.get('provider') == provider, payload
assert enterprise_sso.get('loginRoute') == expected_login_route, payload
assert enterprise_sso.get('callbackRoute') == expected_callback_route, payload
assert enterprise_sso.get('logoutRoute') == '/auth/logout', payload
assert enterprise_sso.get('requiredSettings') == expected_required, payload
assert not enterprise_sso.get('missingSettings'), payload
assert enterprise_sso.get('stateStoreConfigured') is True, payload
assert enterprise_sso.get('identityStoreConfigured') is True, payload
assert enterprise_sso.get('sessionSecretConfigured') is True, payload
managed_login = (payload.get('authRuntime') or {}).get('tenetManagedLogin') or {}
assert managed_login.get('logoutRevokesProviderToken') is False, payload
assert managed_login.get('providerTokenRevocationSupported') is False, payload
if expected_active:
    assert managed_login.get('logoutRoute') == '/auth/logout', payload
    assert managed_login.get('logoutClearsSessionCookie') is True, payload
else:
    assert managed_login.get('logoutClearsSessionCookie') is False, payload

if expected_active:
    assert enterprise_sso.get('unavailableReason', '') == '', payload
else:
    assert enterprise_sso.get('unavailableReason') == 'enterprise SSO provider diagnostics are configured, but provider login/callback runtime is not active for this API instance', payload
serialized = json.dumps(enterprise_sso, sort_keys=True).lower()
for forbidden in ('secret-value', 'google-client-secret', 'okta-private-key', 'saml assertion', 'authorization_code'):
    assert forbidden not in serialized, enterprise_sso
PY
}

assert_enterprise_sso_logout_boundary() {
  local method=$1
  local return_url=$2
  local encoded_return_url headers_path body_path status

  encoded_return_url=$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$return_url")
  headers_path=$(mktemp "${TMPDIR:-/tmp}/enterprise-sso-logout-headers.XXXXXX")
  body_path=$(mktemp "${TMPDIR:-/tmp}/enterprise-sso-logout-body.XXXXXX")
  status=$(curl -sS -D "$headers_path" -o "$body_path" -w "%{http_code}" \
    -X "$method" \
    "http://$HOST:$PORT/auth/logout?returnUrl=$encoded_return_url")

  python3 - "$headers_path" "$body_path" "$status" "$return_url" <<'PY'
import sys
from pathlib import Path

headers = Path(sys.argv[1]).read_text(encoding='utf-8', errors='replace')
body = Path(sys.argv[2]).read_text(encoding='utf-8', errors='replace')
status = sys.argv[3]
return_url = sys.argv[4]

assert status == '302', (status, headers, body)
assert f'Location: {return_url}' in headers, headers
set_cookie_lines = [line for line in headers.splitlines() if line.lower().startswith('set-cookie:')]
assert len(set_cookie_lines) == 1, headers
cookie = set_cookie_lines[0].lower()
assert 'tenet_session=' in cookie, set_cookie_lines[0]
assert 'max-age=0' in cookie or 'max-age=-1' in cookie, set_cookie_lines[0]
assert 'path=/' in cookie, set_cookie_lines[0]
assert 'httponly' in cookie, set_cookie_lines[0]
assert 'samesite=lax' in cookie, set_cookie_lines[0]
for forbidden in ('google', 'okta', 'github', 'provider', 'token', 'revoke'):
    assert forbidden not in body.lower(), body
PY
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
  local enforce_operator_role_authorization=$3
  local enforce_deployment_read_authorization=$4

  TENET_API_ADDR=":$PORT" TENET_CATALOG_SOURCE=filesystem TENET_REPO_ROOT="$REPO_ROOT" TENET_STATUS_PATH="$STATUS_PATH" TENET_PLAN_STORE_PATH="$PLAN_STORE_PATH" TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH" TENET_AWS_ENABLE_APPLY="$APPLY_ENABLED" TENET_API_TRUST_AUTHENTICATED_HEADERS="$trust_authenticated_headers" TENET_API_ENFORCE_AUTHENTICATED_IDENTITY="$enforce_authenticated_identity" TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION="$enforce_operator_role_authorization" TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION="$enforce_deployment_read_authorization" TENET_GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET" \
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

start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED" "$AUTH_ROLE_ENFORCED" "$AUTH_READ_ENFORCED"

curl -fsS "http://$HOST:$PORT/healthz" >/dev/null
assert_request_id_header "http://$HOST:$PORT/healthz"
assert_request_id_header "http://$HOST:$PORT/v1/catalog/summary"
assert_request_id_header "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group"
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
deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")
deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)
plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "{\"ifMatch\":\"$deployment_source_fingerprint\"}" "http://$HOST:$PORT/v1/plans/deployments/payments-api-prod")
plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")
plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)
plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$plan_id")
apply_record=$(curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id")
apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")
apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)
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
actor_plan_preview_extract_path=$(write_payload actor-plan-preview.extract.json "$actor_plan_preview")
actor_plan_id=$(json_string_field_from_path "$actor_plan_preview_extract_path" id)
actor_plan_preview_get=$(curl -fsS "http://$HOST:$PORT/v1/plans/$actor_plan_id")
actor_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Actor-Id: explicit-456" \
  -H "X-Tenet-Actor-Name: Alex Explicit" \
  -H "X-Tenet-Actor-Email: alex.explicit@example.com" \
  -H "X-User-ID: operator-123" \
  -H "X-User-Name: Jane Operator" \
  -H "X-User-Email: jane@example.com" \
  "http://$HOST:$PORT/v1/applies/plans/$actor_plan_id")
actor_apply_record_extract_path=$(write_payload actor-apply-record.extract.json "$actor_apply_record")
actor_apply_id=$(json_string_field_from_path "$actor_apply_record_extract_path" id)
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
stale_apply_deployment_detail_extract_path=$(write_payload stale-apply-deployment-detail.extract.json "$stale_apply_deployment_detail")
stale_apply_source_fingerprint=$(json_string_field_from_path "$stale_apply_deployment_detail_extract_path" sourceFingerprint)
stale_apply_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "{\"ifMatch\":\"$stale_apply_source_fingerprint\"}" "http://$HOST:$PORT/v1/plans/deployments/payments-api-stale")
stale_apply_plan_preview_extract_path=$(write_payload stale-apply-plan-preview.extract.json "$stale_apply_plan_preview")
stale_apply_plan_id=$(json_string_field_from_path "$stale_apply_plan_preview_extract_path" id)
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
webhook_deleted_body=$(cat <<'EOF'
{"ref":"refs/heads/obsolete-release","before":"3333333333333333333333333333333333333333","after":"0000000000000000000000000000000000000000","repository":{"full_name":"acme/platform-config"},"head_commit":null,"commits":[{"id":"c","added":[],"modified":[],"removed":["deployments/obsolete.yaml"]}]}
EOF
)
webhook_deleted_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_deleted_body" python3 - <<'PY'
import hashlib, hmac, os
print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_deleted=$(curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-deleted-1' \
  -H 'X-GitHub-Event: push' \
  -H "X-Hub-Signature-256: $webhook_deleted_signature" \
  --data "$webhook_deleted_body" \
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
resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
codedeploy_application_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app")
test_listener_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=testListener&resourceName=payments-api-staging-test-listener")
worker_cluster_arn="arn:aws:ecs:ap-northeast-2:123456789012:cluster/worker-prod"
worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
selected_worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")
limited_worker_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=1")
ecs_task_resource_name="arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123"
ecs_task_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")
ecs_task_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod")
invalid_resource_log_limit_status=$(curl -sS -o "$INVALID_RESOURCE_LOG_LIMIT_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=0")
invalid_resource_inspection_limit_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0")
bootstrap_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
selected_bootstrap_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-0abc123def4567890")
invalid_resource_log_instance_status=$(curl -sS -o "$INVALID_RESOURCE_LOG_INSTANCE_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-")
traffic_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
production_listener_rule_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")
active_target_group_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")
candidate_target_group_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")
bootstrap_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
worker_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
selected_worker_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=1")
candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
bounded_candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=2")
selected_candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890")
invalid_resource_inspection_instance_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_INSTANCE_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-")
codedeploy_application_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app")
codedeploy_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
production_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")
test_listener_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")
blue_task_set_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")
traffic_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
production_listener_rule_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")
active_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")
candidate_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")
ui=$(curl -fsS "http://$HOST:$PORT/")
application_ui=$(curl -fsS "http://$HOST:$PORT/?page=application&deployment=payments-api-prod")
account_ui=$(curl -fsS "http://$HOST:$PORT/?page=account&accountId=123456789012")
resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan%3A%3Apayments-api-prod-weighted-routing%3A%3AWeighted+routing+plan")
ecs_resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService%3A%3Aworker-prod-service%3A%3AECS+service+Tenet+updates+with+native+rolling+deployment")
bootstrap_resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource%3A%3Afile%3Auserdata%2Fpayments-api-prod.sh%3A%3ANormalized+userdata+bootstrap+source+for+candidate+EC2+capacity")
codedeploy_application_resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication%3A%3Apayments-api-staging-codedeploy-app%3A%3ACodeDeploy+application+coordinating+ECS+traffic+shifting")
blue_task_set_target_group_resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup%3A%3Apayments-api-staging-blue%3A%3ABlue+task+set+target+group+CodeDeploy+treats+as+the+live+ECS+traffic+destination")
production_listener_rule_resource_ui=$(curl -fsS "http://$HOST:$PORT/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule%3A%3Apayments-api-prod-prod-listener%3A%3AProduction+listener+rule+operators+inspect+while+Tenet+updates+weighted+forwarding+during+rollout")
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
webhook_deleted_path=$(write_payload webhook-deleted.json "$webhook_deleted")
invalid_webhook_path=$(write_payload invalid-webhook.txt "$invalid_webhook")
resource_logs_path=$(write_payload resource-logs.json "$resource_logs")
codedeploy_application_resource_logs_path=$(write_payload codedeploy-application-resource-logs.json "$codedeploy_application_resource_logs")
test_listener_resource_logs_path=$(write_payload test-listener-resource-logs.json "$test_listener_resource_logs")
worker_resource_logs_path=$(write_payload worker-resource-logs.json "$worker_resource_logs")
selected_worker_resource_logs_path=$(write_payload selected-worker-resource-logs.json "$selected_worker_resource_logs")
limited_worker_resource_logs_path=$(write_payload limited-worker-resource-logs.json "$limited_worker_resource_logs")
ecs_task_resource_logs_path=$(write_payload ecs-task-resource-logs.json "$ecs_task_resource_logs")
ecs_task_resource_inspection_path=$(write_payload ecs-task-resource-inspection.json "$ecs_task_resource_inspection")
invalid_resource_log_limit_path=$(write_payload invalid-resource-log-limit.txt "$(cat "$INVALID_RESOURCE_LOG_LIMIT_BODY")")
invalid_resource_log_instance_path=$(write_payload invalid-resource-log-instance.txt "$(cat "$INVALID_RESOURCE_LOG_INSTANCE_BODY")")
invalid_resource_inspection_limit_path=$(write_payload invalid-resource-inspection-limit.txt "$(cat "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY")")
bootstrap_resource_logs_path=$(write_payload bootstrap-resource-logs.json "$bootstrap_resource_logs")
selected_bootstrap_resource_logs_path=$(write_payload selected-bootstrap-resource-logs.json "$selected_bootstrap_resource_logs")
traffic_resource_logs_path=$(write_payload traffic-resource-logs.json "$traffic_resource_logs")
production_listener_rule_resource_logs_path=$(write_payload production-listener-rule-resource-logs.json "$production_listener_rule_resource_logs")
active_target_group_resource_logs_path=$(write_payload active-target-group-resource-logs.json "$active_target_group_resource_logs")
candidate_target_group_resource_logs_path=$(write_payload candidate-target-group-resource-logs.json "$candidate_target_group_resource_logs")
bootstrap_resource_inspection_path=$(write_payload bootstrap-resource-inspection.json "$bootstrap_resource_inspection")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
selected_worker_resource_inspection_path=$(write_payload selected-worker-resource-inspection.json "$selected_worker_resource_inspection")
candidate_resource_inspection_path=$(write_payload candidate-resource-inspection.json "$candidate_resource_inspection")
bounded_candidate_resource_inspection_path=$(write_payload bounded-candidate-resource-inspection.json "$bounded_candidate_resource_inspection")
selected_candidate_resource_inspection_path=$(write_payload selected-candidate-resource-inspection.json "$selected_candidate_resource_inspection")
invalid_resource_inspection_instance_path=$(write_payload invalid-resource-inspection-instance.txt "$(cat "$INVALID_RESOURCE_INSPECTION_INSTANCE_BODY")")
codedeploy_application_resource_inspection_path=$(write_payload codedeploy-application-resource-inspection.json "$codedeploy_application_resource_inspection")
codedeploy_group_resource_inspection_path=$(write_payload codedeploy-group-resource-inspection.json "$codedeploy_group_resource_inspection")
production_listener_resource_inspection_path=$(write_payload production-listener-resource-inspection.json "$production_listener_resource_inspection")
test_listener_resource_inspection_path=$(write_payload test-listener-resource-inspection.json "$test_listener_resource_inspection")
blue_task_set_target_group_resource_inspection_path=$(write_payload blue-task-set-target-group-resource-inspection.json "$blue_task_set_target_group_resource_inspection")
traffic_resource_inspection_path=$(write_payload traffic-resource-inspection.json "$traffic_resource_inspection")
production_listener_rule_resource_inspection_path=$(write_payload production-listener-rule-resource-inspection.json "$production_listener_rule_resource_inspection")
active_target_group_resource_inspection_path=$(write_payload active-target-group-resource-inspection.json "$active_target_group_resource_inspection")
candidate_target_group_resource_inspection_path=$(write_payload candidate-target-group-resource-inspection.json "$candidate_target_group_resource_inspection")
ui_path=$(write_payload ui.html "$ui")
application_ui_path=$(write_payload application-ui.html "$application_ui")
account_ui_path=$(write_payload account-ui.html "$account_ui")
resource_ui_path=$(write_payload resource-ui.html "$resource_ui")
ecs_resource_ui_path=$(write_payload ecs-resource-ui.html "$ecs_resource_ui")
bootstrap_resource_ui_path=$(write_payload bootstrap-resource-ui.html "$bootstrap_resource_ui")
codedeploy_application_resource_ui_path=$(write_payload codedeploy-application-resource-ui.html "$codedeploy_application_resource_ui")
blue_task_set_target_group_resource_ui_path=$(write_payload blue-task-set-target-group-resource-ui.html "$blue_task_set_target_group_resource_ui")
production_listener_rule_resource_ui_path=$(write_payload production-listener-rule-resource-ui.html "$production_listener_rule_resource_ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_ui_path=$(write_payload plan-ui.html "$plan_ui")
apply_ui_path=$(write_payload apply-ui.html "$apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" APPLICATION_DETAIL_PATH="$application_detail_path" OVERVIEW_PATH="$overview_path" ACCOUNT_DETAIL_PATH="$account_detail_path" GROUP_DETAIL_PATH="$group_detail_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_EVENTS_FIXTURE_PATH="$deployment_events_fixture_path" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" INVALID_APPLY_TRANSITION_STATUS="$invalid_apply_transition_status" INVALID_APPLY_TRANSITION_PATH="$invalid_apply_transition_path" UNKNOWN_APPLY_TRANSITION_STATUS="$unknown_apply_transition_status" UNKNOWN_APPLY_TRANSITION_PATH="$unknown_apply_transition_path" TERMINAL_APPLY_TRANSITION_STATUS="$terminal_apply_transition_status" TERMINAL_APPLY_TRANSITION_PATH="$terminal_apply_transition_path" ACTOR_PLAN_PREVIEW_PATH="$actor_plan_preview_path" ACTOR_PLAN_PREVIEW_GET_PATH="$actor_plan_preview_get_path" ACTOR_APPLY_RECORD_PATH="$actor_apply_record_path" ACTOR_APPLY_RECORD_GET_PATH="$actor_apply_record_get_path" STALE_PLAN_PREVIEW_PATH="$stale_plan_preview_path" STALE_APPLY_PLAN_PREVIEW_PATH="$stale_apply_plan_preview_path" STALE_APPLY_CONFLICT_PATH="$stale_apply_conflict_path" STALE_APPLY_CONFLICT_STATUS="$stale_apply_conflict_status" STALE_APPLY_AUDIT_LOGS_PATH="$stale_apply_audit_logs_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" AUDIT_LOGS_FILTERED_PATH="$audit_logs_filtered_path" AUDIT_LOGS_EMPTY_PATH="$audit_logs_empty_path" ACTOR_AUDIT_LOGS_PATH="$actor_audit_logs_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" STATUS_JSON_PATH="$status_json_path" WEBHOOK_PUSH_PATH="$webhook_push_path" WEBHOOK_PING_PATH="$webhook_ping_path" WEBHOOK_DELETED_PATH="$webhook_deleted_path" INVALID_WEBHOOK_PATH="$invalid_webhook_path" INVALID_WEBHOOK_STATUS="$invalid_webhook_status" RESOURCE_LOGS_PATH="$resource_logs_path" CODEDEPLOY_APPLICATION_RESOURCE_LOGS_PATH="$codedeploy_application_resource_logs_path" TEST_LISTENER_RESOURCE_LOGS_PATH="$test_listener_resource_logs_path" WORKER_CLUSTER_ARN="$worker_cluster_arn" SELECTED_WORKER_RESOURCE_LOGS_PATH="$selected_worker_resource_logs_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" LIMITED_WORKER_RESOURCE_LOGS_PATH="$limited_worker_resource_logs_path" ECS_TASK_RESOURCE_LOGS_PATH="$ecs_task_resource_logs_path" ECS_TASK_RESOURCE_INSPECTION_PATH="$ecs_task_resource_inspection_path" INVALID_RESOURCE_LOG_LIMIT_STATUS="$invalid_resource_log_limit_status" INVALID_RESOURCE_LOG_LIMIT_PATH="$invalid_resource_log_limit_path" INVALID_RESOURCE_LOG_INSTANCE_STATUS="$invalid_resource_log_instance_status" INVALID_RESOURCE_LOG_INSTANCE_PATH="$invalid_resource_log_instance_path" INVALID_RESOURCE_INSPECTION_LIMIT_STATUS="$invalid_resource_inspection_limit_status" INVALID_RESOURCE_INSPECTION_LIMIT_PATH="$invalid_resource_inspection_limit_path" INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS="$invalid_resource_inspection_instance_status" INVALID_RESOURCE_INSPECTION_INSTANCE_PATH="$invalid_resource_inspection_instance_path" BOOTSTRAP_RESOURCE_LOGS_PATH="$bootstrap_resource_logs_path" SELECTED_BOOTSTRAP_RESOURCE_LOGS_PATH="$selected_bootstrap_resource_logs_path" TRAFFIC_RESOURCE_LOGS_PATH="$traffic_resource_logs_path" PRODUCTION_LISTENER_RULE_RESOURCE_LOGS_PATH="$production_listener_rule_resource_logs_path" ACTIVE_TARGET_GROUP_RESOURCE_LOGS_PATH="$active_target_group_resource_logs_path" CANDIDATE_TARGET_GROUP_RESOURCE_LOGS_PATH="$candidate_target_group_resource_logs_path" BOOTSTRAP_RESOURCE_INSPECTION_PATH="$bootstrap_resource_inspection_path" SELECTED_WORKER_RESOURCE_INSPECTION_PATH="$selected_worker_resource_inspection_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" BOUNDED_CANDIDATE_RESOURCE_INSPECTION_PATH="$bounded_candidate_resource_inspection_path" SELECTED_CANDIDATE_RESOURCE_INSPECTION_PATH="$selected_candidate_resource_inspection_path" CODEDEPLOY_APPLICATION_RESOURCE_INSPECTION_PATH="$codedeploy_application_resource_inspection_path" CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH="$codedeploy_group_resource_inspection_path" PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH="$production_listener_resource_inspection_path" TEST_LISTENER_RESOURCE_INSPECTION_PATH="$test_listener_resource_inspection_path" BLUE_TASK_SET_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$blue_task_set_target_group_resource_inspection_path" TRAFFIC_RESOURCE_INSPECTION_PATH="$traffic_resource_inspection_path" PRODUCTION_LISTENER_RULE_RESOURCE_INSPECTION_PATH="$production_listener_rule_resource_inspection_path" ACTIVE_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$active_target_group_resource_inspection_path" CANDIDATE_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$candidate_target_group_resource_inspection_path" UI_PATH="$ui_path" APPLICATION_UI_PATH="$application_ui_path" ACCOUNT_UI_PATH="$account_ui_path" RESOURCE_UI_PATH="$resource_ui_path" ECS_RESOURCE_UI_PATH="$ecs_resource_ui_path" BOOTSTRAP_RESOURCE_UI_PATH="$bootstrap_resource_ui_path" CODEDEPLOY_APPLICATION_RESOURCE_UI_PATH="$codedeploy_application_resource_ui_path" BLUE_TASK_SET_TARGET_GROUP_RESOURCE_UI_PATH="$blue_task_set_target_group_resource_ui_path" PRODUCTION_LISTENER_RULE_RESOURCE_UI_PATH="$production_listener_rule_resource_ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_UI_PATH="$plan_ui_path" APPLY_UI_PATH="$apply_ui_path" APPLY_ENABLED="$APPLY_ENABLED" python3 - <<'PY'
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
webhook_deleted = read_json('WEBHOOK_DELETED_PATH')
invalid_webhook = read_text('INVALID_WEBHOOK_PATH')
resource_logs = read_json('RESOURCE_LOGS_PATH')
codedeploy_application_resource_logs = read_json('CODEDEPLOY_APPLICATION_RESOURCE_LOGS_PATH')
test_listener_resource_logs = read_json('TEST_LISTENER_RESOURCE_LOGS_PATH')
worker_cluster_arn = os.environ['WORKER_CLUSTER_ARN']
selected_worker_resource_logs = read_json('SELECTED_WORKER_RESOURCE_LOGS_PATH')
worker_resource_logs = read_json('WORKER_RESOURCE_LOGS_PATH')
limited_worker_resource_logs = read_json('LIMITED_WORKER_RESOURCE_LOGS_PATH')
ecs_task_resource_logs = read_json('ECS_TASK_RESOURCE_LOGS_PATH')
ecs_task_resource_inspection = read_json('ECS_TASK_RESOURCE_INSPECTION_PATH')
invalid_resource_log_limit = read_text('INVALID_RESOURCE_LOG_LIMIT_PATH')
invalid_resource_log_instance = read_text('INVALID_RESOURCE_LOG_INSTANCE_PATH')
invalid_resource_inspection_limit = read_text('INVALID_RESOURCE_INSPECTION_LIMIT_PATH')
invalid_resource_inspection_instance = read_text('INVALID_RESOURCE_INSPECTION_INSTANCE_PATH')
bootstrap_resource_logs = read_json('BOOTSTRAP_RESOURCE_LOGS_PATH')
selected_bootstrap_resource_logs = read_json('SELECTED_BOOTSTRAP_RESOURCE_LOGS_PATH')
traffic_resource_logs = read_json('TRAFFIC_RESOURCE_LOGS_PATH')
production_listener_rule_resource_logs = read_json('PRODUCTION_LISTENER_RULE_RESOURCE_LOGS_PATH')
active_target_group_resource_logs = read_json('ACTIVE_TARGET_GROUP_RESOURCE_LOGS_PATH')
candidate_target_group_resource_logs = read_json('CANDIDATE_TARGET_GROUP_RESOURCE_LOGS_PATH')
bootstrap_resource_inspection = read_json('BOOTSTRAP_RESOURCE_INSPECTION_PATH')
selected_worker_resource_inspection = read_json('SELECTED_WORKER_RESOURCE_INSPECTION_PATH')
worker_resource_inspection = read_json('WORKER_RESOURCE_INSPECTION_PATH')
candidate_resource_inspection = read_json('CANDIDATE_RESOURCE_INSPECTION_PATH')
bounded_candidate_resource_inspection = read_json('BOUNDED_CANDIDATE_RESOURCE_INSPECTION_PATH')
selected_candidate_resource_inspection = read_json('SELECTED_CANDIDATE_RESOURCE_INSPECTION_PATH')
codedeploy_application_resource_inspection = read_json('CODEDEPLOY_APPLICATION_RESOURCE_INSPECTION_PATH')
codedeploy_group_resource_inspection = read_json('CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH')
production_listener_resource_inspection = read_json('PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH')
test_listener_resource_inspection = read_json('TEST_LISTENER_RESOURCE_INSPECTION_PATH')
blue_task_set_target_group_resource_inspection = read_json('BLUE_TASK_SET_TARGET_GROUP_RESOURCE_INSPECTION_PATH')
traffic_resource_inspection = read_json('TRAFFIC_RESOURCE_INSPECTION_PATH')
production_listener_rule_resource_inspection = read_json('PRODUCTION_LISTENER_RULE_RESOURCE_INSPECTION_PATH')
active_target_group_resource_inspection = read_json('ACTIVE_TARGET_GROUP_RESOURCE_INSPECTION_PATH')
candidate_target_group_resource_inspection = read_json('CANDIDATE_TARGET_GROUP_RESOURCE_INSPECTION_PATH')
ui = read_text('UI_PATH')
application_ui = read_text('APPLICATION_UI_PATH')
account_ui = read_text('ACCOUNT_UI_PATH')
resource_ui = read_text('RESOURCE_UI_PATH')
ecs_resource_ui = read_text('ECS_RESOURCE_UI_PATH')
bootstrap_resource_ui = read_text('BOOTSTRAP_RESOURCE_UI_PATH')
codedeploy_application_resource_ui = read_text('CODEDEPLOY_APPLICATION_RESOURCE_UI_PATH')
blue_task_set_target_group_resource_ui = read_text('BLUE_TASK_SET_TARGET_GROUP_RESOURCE_UI_PATH')
production_listener_rule_resource_ui = read_text('PRODUCTION_LISTENER_RULE_RESOURCE_UI_PATH')
audit_ui = read_text('AUDIT_UI_PATH')
plan_ui = read_text('PLAN_UI_PATH')
apply_ui = read_text('APPLY_UI_PATH')
apply_enabled = os.environ['APPLY_ENABLED'].lower() == 'true'

def assert_recent_log_empty_reason_contract(label, payload):
    entries = payload.get('entries') or []
    assert isinstance(entries, list), {label: payload}
    if payload.get('available') is True:
        if entries:
            assert 'emptyReason' not in payload, {label: payload}
        else:
            assert payload.get('emptyReason') in {'no_recent_events', 'no_formatted_entries'}, {label: payload}
    else:
        assert isinstance(payload.get('unavailableReason'), str) and payload['unavailableReason'], {label: payload}


def assert_recent_log_entry_ids(label, payload):
    entries = payload.get('entries') or []
    seen = set()
    for entry in entries:
        entry_id = entry.get('entryId')
        assert isinstance(entry_id, str) and entry_id.startswith('rlog_'), {label: payload}
        assert entry_id not in seen, {label: payload}
        seen.add(entry_id)

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
assert all('controllerRefresh' not in event for event in deployment_events_fixture['events']), deployment_events_fixture
assert deployment_events_fixture.get('managedDriftSummary') == {'total': 0, 'high': 0, 'medium': 0, 'low': 0}, deployment_events_fixture
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
    assert isinstance(entry.get('entryId'), str) and entry['entryId'].startswith('audit_'), entry
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
assert webhook_deleted['deliveryId'] == 'delivery-deleted-1', webhook_deleted
assert webhook_deleted['event'] == 'push', webhook_deleted
assert webhook_deleted['status'] == 'ignored', webhook_deleted
assert webhook_deleted['processed'] is False, webhook_deleted
assert webhook_deleted['deleted'] is True, webhook_deleted
assert webhook_deleted['repository'] == 'acme/platform-config', webhook_deleted
assert webhook_deleted['ref'] == 'refs/heads/obsolete-release', webhook_deleted
assert webhook_deleted['before'] == '3333333333333333333333333333333333333333', webhook_deleted
assert webhook_deleted['after'] == '0000000000000000000000000000000000000000', webhook_deleted
assert webhook_deleted['changedFiles']['all'] == ['deployments/obsolete.yaml'], webhook_deleted
assert 'controller refresh handoff skipped' in webhook_deleted['reason'], webhook_deleted
assert 'controllerRefresh' not in webhook_deleted, webhook_deleted
assert os.environ['INVALID_WEBHOOK_STATUS'] == '400', os.environ['INVALID_WEBHOOK_STATUS']
assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook
assert status['status']['sync']['state'] == 'Synced', status
assert resource_logs['deploymentName'] == 'payments-api-staging', resource_logs
assert resource_logs['resourceType'] == 'codeDeployDeploymentGroup', resource_logs
assert resource_logs['resourceName'] == 'payments-api-staging-codedeploy-group', resource_logs
assert isinstance(resource_logs['available'], bool), resource_logs
assert isinstance(resource_logs.get('lookupContext'), dict), resource_logs
if resource_logs['available']:
    assert resource_logs.get('source') == 'codedeploy', resource_logs
    assert isinstance(resource_logs.get('entries'), list), resource_logs
else:
    assert resource_logs.get('unavailableReason') in {'aws_unavailable', 'resource_not_found'}, resource_logs

assert test_listener_resource_logs['deploymentName'] == 'payments-api-staging', test_listener_resource_logs
assert test_listener_resource_logs['resourceType'] == 'testListener', test_listener_resource_logs
assert test_listener_resource_logs['resourceName'] == 'payments-api-staging-test-listener', test_listener_resource_logs
assert isinstance(test_listener_resource_logs['available'], bool), test_listener_resource_logs
assert isinstance(test_listener_resource_logs.get('lookupContext'), dict), test_listener_resource_logs
assert test_listener_resource_logs.get('backingResourceType') == 'codeDeployDeploymentGroup', test_listener_resource_logs
assert test_listener_resource_logs.get('backingResourceName') == 'payments-api-staging-codedeploy-group', test_listener_resource_logs
if test_listener_resource_logs['available']:
    assert test_listener_resource_logs.get('source') == 'codedeploy', test_listener_resource_logs
    assert isinstance(test_listener_resource_logs.get('entries'), list), test_listener_resource_logs
else:
    assert test_listener_resource_logs.get('unavailableReason') in {'aws_unavailable', 'resource_not_found', 'no_log_source'}, test_listener_resource_logs

assert bootstrap_resource_logs['deploymentName'] == 'payments-api-prod', bootstrap_resource_logs
assert bootstrap_resource_logs['resourceType'] == 'userDataBootstrapSource', bootstrap_resource_logs
assert bootstrap_resource_logs['resourceName'] == 'git:userdata/payments-api-prod.sh', bootstrap_resource_logs
assert isinstance(bootstrap_resource_logs.get('lookupContext'), dict), bootstrap_resource_logs
assert isinstance(bootstrap_resource_logs['available'], bool), bootstrap_resource_logs
assert bootstrap_resource_logs.get('backingResourceType') == 'candidateAutoScalingGroup', bootstrap_resource_logs
assert bootstrap_resource_logs.get('backingResourceName') == 'payments-api-prod-candidate', bootstrap_resource_logs
if bootstrap_resource_logs['available']:
    assert bootstrap_resource_logs.get('source') == 'ec2_console', bootstrap_resource_logs
    for entry in bootstrap_resource_logs.get('entries', []):
        assert entry.get('instanceId'), bootstrap_resource_logs
else:
    assert bootstrap_resource_logs.get('unavailableReason') in {'aws_unavailable', 'no_log_source'}, bootstrap_resource_logs

assert selected_bootstrap_resource_logs['deploymentName'] == 'payments-api-prod', selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs['resourceType'] == 'userDataBootstrapSource', selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs['resourceName'] == 'git:userdata/payments-api-prod.sh', selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get('instanceId') == 'i-0abc123def4567890', selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get('backingResourceType') == 'candidateAutoScalingGroup', selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get('backingResourceName') == 'payments-api-prod-candidate', selected_bootstrap_resource_logs
assert isinstance(selected_bootstrap_resource_logs.get('lookupContext'), dict), selected_bootstrap_resource_logs
if selected_bootstrap_resource_logs['available']:
    assert selected_bootstrap_resource_logs.get('source') == 'ec2_console', selected_bootstrap_resource_logs
    for entry in selected_bootstrap_resource_logs.get('entries', []):
        assert entry.get('instanceId') == 'i-0abc123def4567890', selected_bootstrap_resource_logs
else:
    assert selected_bootstrap_resource_logs.get('unavailableReason') in {'aws_unavailable', 'resource_not_found', 'no_log_source'}, selected_bootstrap_resource_logs
assert os.environ['INVALID_RESOURCE_LOG_INSTANCE_STATUS'] == '400', os.environ['INVALID_RESOURCE_LOG_INSTANCE_STATUS']
assert invalid_resource_log_instance.strip() == 'invalid instanceId query', invalid_resource_log_instance

assert traffic_resource_logs['deploymentName'] == 'payments-api-prod', traffic_resource_logs
assert traffic_resource_logs['resourceType'] == 'trafficShiftPlan', traffic_resource_logs
assert traffic_resource_logs['resourceName'] == 'payments-api-prod-weighted-routing', traffic_resource_logs
assert traffic_resource_logs['available'] is False, traffic_resource_logs
assert traffic_resource_logs.get('unavailableReason') == 'no_log_source', traffic_resource_logs
assert isinstance(traffic_resource_logs.get('lookupContext'), dict), traffic_resource_logs
assert traffic_resource_logs['lookupContext'].get('region') == 'ap-northeast-2', traffic_resource_logs
assert isinstance(traffic_resource_logs['lookupContext'].get('controlAccountId'), str) and traffic_resource_logs['lookupContext']['controlAccountId'], traffic_resource_logs
assert isinstance(traffic_resource_logs['lookupContext'].get('targetAccountId'), str) and traffic_resource_logs['lookupContext']['targetAccountId'], traffic_resource_logs
assert isinstance(traffic_resource_logs['lookupContext'].get('crossAccount'), bool), traffic_resource_logs
assert isinstance(traffic_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), traffic_resource_logs

assert production_listener_rule_resource_logs['deploymentName'] == 'payments-api-prod', production_listener_rule_resource_logs
assert production_listener_rule_resource_logs['resourceType'] == 'productionListenerRule', production_listener_rule_resource_logs
assert production_listener_rule_resource_logs['resourceName'] == 'payments-api-prod-prod-listener', production_listener_rule_resource_logs
assert production_listener_rule_resource_logs['available'] is False, production_listener_rule_resource_logs
assert production_listener_rule_resource_logs.get('unavailableReason') == 'no_log_source', production_listener_rule_resource_logs
no_log_source = production_listener_rule_resource_logs.get('resultScope')
assert isinstance(no_log_source, dict), production_listener_rule_resource_logs
assert no_log_source.get('returnedEntryCount') == 0, production_listener_rule_resource_logs
assert no_log_source.get('truncationStatus') == 'not_applicable', production_listener_rule_resource_logs
assert isinstance(production_listener_rule_resource_logs.get('lookupContext'), dict), production_listener_rule_resource_logs
assert production_listener_rule_resource_logs['lookupContext'].get('region') == 'ap-northeast-2', production_listener_rule_resource_logs
assert isinstance(production_listener_rule_resource_logs['lookupContext'].get('controlAccountId'), str) and production_listener_rule_resource_logs['lookupContext']['controlAccountId'], production_listener_rule_resource_logs
assert isinstance(production_listener_rule_resource_logs['lookupContext'].get('targetAccountId'), str) and production_listener_rule_resource_logs['lookupContext']['targetAccountId'], production_listener_rule_resource_logs
assert isinstance(production_listener_rule_resource_logs['lookupContext'].get('crossAccount'), bool), production_listener_rule_resource_logs
assert isinstance(production_listener_rule_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), production_listener_rule_resource_logs

for payload, resource_type, resource_name in (
    (active_target_group_resource_logs, 'activeTargetGroup', 'payments-api-prod-active'),
    (candidate_target_group_resource_logs, 'candidateTargetGroup', 'payments-api-prod-candidate'),
):
    assert payload['deploymentName'] == 'payments-api-prod', payload
    assert payload['resourceType'] == resource_type, payload
    assert payload['resourceName'] == resource_name, payload
    assert payload['available'] is False, payload
    assert payload.get('unavailableReason') == 'no_log_source', payload
    assert isinstance(payload.get('lookupContext'), dict), payload
    assert payload['lookupContext'].get('region') == 'ap-northeast-2', payload
    assert isinstance(payload['lookupContext'].get('controlAccountId'), str) and payload['lookupContext']['controlAccountId'], payload
    assert isinstance(payload['lookupContext'].get('targetAccountId'), str) and payload['lookupContext']['targetAccountId'], payload
    assert isinstance(payload['lookupContext'].get('crossAccount'), bool), payload
    assert isinstance(payload['lookupContext'].get('observerRoleConfigured'), bool), payload

assert bootstrap_resource_inspection['deploymentName'] == 'payments-api-prod', bootstrap_resource_inspection
assert bootstrap_resource_inspection['resourceType'] == 'userDataBootstrapSource', bootstrap_resource_inspection
assert bootstrap_resource_inspection['resourceName'] == 'git:userdata/payments-api-prod.sh', bootstrap_resource_inspection
assert isinstance(bootstrap_resource_inspection.get('lookupContext'), dict), bootstrap_resource_inspection
assert isinstance(bootstrap_resource_inspection['available'], bool), bootstrap_resource_inspection
if bootstrap_resource_inspection['available']:
    assert bootstrap_resource_inspection.get('source') == 'auto_scaling_group', bootstrap_resource_inspection
    assert isinstance(bootstrap_resource_inspection.get('userDataBootstrapSource'), dict), bootstrap_resource_inspection
    payload = bootstrap_resource_inspection['userDataBootstrapSource']
    assert payload.get('desiredBootstrapSource') == 'git:userdata/payments-api-prod.sh', bootstrap_resource_inspection
    assert payload.get('backingResourceType') == 'candidateAutoScalingGroup', bootstrap_resource_inspection
    assert payload.get('backingResourceName') == 'payments-api-prod-candidate', bootstrap_resource_inspection
else:
    assert bootstrap_resource_inspection.get('unavailableReason') == 'aws_unavailable', bootstrap_resource_inspection

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
assert codedeploy_application_resource_logs['deploymentName'] == 'payments-api-staging', codedeploy_application_resource_logs
assert codedeploy_application_resource_logs['resourceType'] == 'codeDeployApplication', codedeploy_application_resource_logs
assert codedeploy_application_resource_logs['resourceName'] == 'payments-api-staging-codedeploy-app', codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['available'], bool), codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs.get('lookupContext'), dict), codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['lookupContext'].get('region'), str) and codedeploy_application_resource_logs['lookupContext']['region'], codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['lookupContext'].get('controlAccountId'), str) and codedeploy_application_resource_logs['lookupContext']['controlAccountId'], codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['lookupContext'].get('targetAccountId'), str) and codedeploy_application_resource_logs['lookupContext']['targetAccountId'], codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['lookupContext'].get('crossAccount'), bool), codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), codedeploy_application_resource_logs
assert codedeploy_application_resource_logs.get('entries') is None or isinstance(codedeploy_application_resource_logs.get('entries'), list), codedeploy_application_resource_logs
if codedeploy_application_resource_logs['available']:
    assert codedeploy_application_resource_logs.get('source') == 'codedeploy', codedeploy_application_resource_logs
else:
    assert isinstance(codedeploy_application_resource_logs.get('unavailableReason'), str) and codedeploy_application_resource_logs['unavailableReason'], codedeploy_application_resource_logs
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
worker_scope = worker_resource_logs.get('resultScope')
assert isinstance(worker_scope, dict), worker_resource_logs
assert 'requestedLimit' not in worker_scope, worker_resource_logs
assert worker_scope.get('effectiveLimit') == 50, worker_resource_logs
assert worker_scope.get('returnedEntryCount') == len(worker_resource_logs.get('entries') or []), worker_resource_logs
assert worker_scope.get('truncationStatus') in ('not_truncated', 'unknown', 'not_applicable'), worker_resource_logs
if worker_resource_logs['available']:
    assert isinstance(worker_resource_logs.get('source'), str), worker_resource_logs
else:
    assert isinstance(worker_resource_logs.get('unavailableReason'), str) and worker_resource_logs['unavailableReason'], worker_resource_logs
assert limited_worker_resource_logs['deploymentName'] == 'worker-prod', limited_worker_resource_logs
assert limited_worker_resource_logs['resourceType'] == 'ecsService', limited_worker_resource_logs
assert limited_worker_resource_logs['resourceName'] == 'worker-prod-service', limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs.get('resourceDescription'), str) and limited_worker_resource_logs['resourceDescription'], limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs['available'], bool), limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs.get('lookupContext'), dict), limited_worker_resource_logs
assert limited_worker_resource_logs.get('entries') is None or isinstance(limited_worker_resource_logs.get('entries'), list), limited_worker_resource_logs
assert len(limited_worker_resource_logs.get('entries') or []) <= 1, limited_worker_resource_logs
limited_scope = limited_worker_resource_logs.get('resultScope')
assert isinstance(limited_scope, dict), limited_worker_resource_logs
assert limited_scope.get('requestedLimit') == 1, limited_worker_resource_logs
assert limited_scope.get('effectiveLimit') == 1, limited_worker_resource_logs
assert limited_scope.get('returnedEntryCount') == len(limited_worker_resource_logs.get('entries') or []), limited_worker_resource_logs
assert limited_scope.get('truncationStatus') in ('truncated', 'not_truncated', 'unknown', 'not_applicable'), limited_worker_resource_logs
if limited_worker_resource_logs['available']:
    assert isinstance(limited_worker_resource_logs.get('source'), str), limited_worker_resource_logs
else:
    assert isinstance(limited_worker_resource_logs.get('unavailableReason'), str) and limited_worker_resource_logs['unavailableReason'], limited_worker_resource_logs

assert selected_worker_resource_logs['deploymentName'] == 'worker-prod', selected_worker_resource_logs
assert selected_worker_resource_logs['resourceType'] == 'ecsService', selected_worker_resource_logs
assert selected_worker_resource_logs['resourceName'] == 'worker-prod-service', selected_worker_resource_logs
assert selected_worker_resource_logs.get('clusterArn') == worker_cluster_arn, selected_worker_resource_logs
assert isinstance(selected_worker_resource_logs.get('lookupContext'), dict), selected_worker_resource_logs
assert len(selected_worker_resource_logs.get('entries') or []) <= 1, selected_worker_resource_logs
selected_worker_scope = selected_worker_resource_logs.get('resultScope')
assert isinstance(selected_worker_scope, dict), selected_worker_resource_logs
assert selected_worker_scope.get('requestedLimit') == 1, selected_worker_resource_logs
assert selected_worker_scope.get('effectiveLimit') == 1, selected_worker_resource_logs
if selected_worker_resource_logs['available']:
    assert isinstance(selected_worker_resource_logs.get('source'), str), selected_worker_resource_logs
else:
    assert isinstance(selected_worker_resource_logs.get('unavailableReason'), str) and selected_worker_resource_logs['unavailableReason'], selected_worker_resource_logs
assert ecs_task_resource_logs['deploymentName'] == 'worker-prod', ecs_task_resource_logs
assert ecs_task_resource_logs['resourceType'] == 'ecsTask', ecs_task_resource_logs
assert ecs_task_resource_logs['resourceName'] == 'arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123', ecs_task_resource_logs
assert ecs_task_resource_logs.get('resourceDescription') == 'ECS task selected from live service inspection', ecs_task_resource_logs
assert ecs_task_resource_logs.get('containerName') == 'worker', ecs_task_resource_logs
assert ecs_task_resource_logs.get('clusterArn') == worker_cluster_arn, ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['available'], bool), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs.get('lookupContext'), dict), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['lookupContext'].get('region'), str) and ecs_task_resource_logs['lookupContext']['region'], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['lookupContext'].get('controlAccountId'), str) and ecs_task_resource_logs['lookupContext']['controlAccountId'], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['lookupContext'].get('targetAccountId'), str) and ecs_task_resource_logs['lookupContext']['targetAccountId'], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['lookupContext'].get('crossAccount'), bool), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), ecs_task_resource_logs
assert ecs_task_resource_logs.get('entries') is None or isinstance(ecs_task_resource_logs.get('entries'), list), ecs_task_resource_logs
assert len(ecs_task_resource_logs.get('entries') or []) <= 1, ecs_task_resource_logs
ecs_task_scope = ecs_task_resource_logs.get('resultScope')
assert isinstance(ecs_task_scope, dict), ecs_task_resource_logs
assert ecs_task_scope.get('requestedLimit') == 1, ecs_task_resource_logs
assert ecs_task_scope.get('effectiveLimit') == 1, ecs_task_resource_logs
assert ecs_task_scope.get('returnedEntryCount') == len(ecs_task_resource_logs.get('entries') or []), ecs_task_resource_logs
assert ecs_task_scope.get('truncationStatus') in ('truncated', 'not_truncated', 'unknown', 'not_applicable'), ecs_task_resource_logs
if ecs_task_resource_logs['available']:
    assert ecs_task_resource_logs.get('source') == 'cloudwatch', ecs_task_resource_logs
else:
    assert ecs_task_resource_logs.get('unavailableReason') in {'aws_unavailable', 'resource_not_found', 'cross_account_not_configured', 'no_log_source'}, ecs_task_resource_logs
for label, payload in (
    ('resource_logs', resource_logs),
    ('codedeploy_application_resource_logs', codedeploy_application_resource_logs),
    ('test_listener_resource_logs', test_listener_resource_logs),
    ('worker_resource_logs', worker_resource_logs),
    ('limited_worker_resource_logs', limited_worker_resource_logs),
    ('selected_worker_resource_logs', selected_worker_resource_logs),
    ('ecs_task_resource_logs', ecs_task_resource_logs),
    ('bootstrap_resource_logs', bootstrap_resource_logs),
    ('selected_bootstrap_resource_logs', selected_bootstrap_resource_logs),
    ('traffic_resource_logs', traffic_resource_logs),
    ('production_listener_rule_resource_logs', production_listener_rule_resource_logs),
    ('active_target_group_resource_logs', active_target_group_resource_logs),
    ('candidate_target_group_resource_logs', candidate_target_group_resource_logs),
):
    assert_recent_log_empty_reason_contract(label, payload)
    assert_recent_log_entry_ids(label, payload)

assert ecs_task_resource_inspection['deploymentName'] == 'worker-prod', ecs_task_resource_inspection
assert ecs_task_resource_inspection['resourceType'] == 'ecsTask', ecs_task_resource_inspection
assert ecs_task_resource_inspection['resourceName'] == 'arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123', ecs_task_resource_inspection
assert ecs_task_resource_inspection.get('clusterArn') == worker_cluster_arn, ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['available'], bool), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection.get('lookupContext'), dict), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['lookupContext'].get('region'), str) and ecs_task_resource_inspection['lookupContext']['region'], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['lookupContext'].get('controlAccountId'), str) and ecs_task_resource_inspection['lookupContext']['controlAccountId'], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['lookupContext'].get('targetAccountId'), str) and ecs_task_resource_inspection['lookupContext']['targetAccountId'], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['lookupContext'].get('crossAccount'), bool), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection['lookupContext'].get('observerRoleConfigured'), bool), ecs_task_resource_inspection
if ecs_task_resource_inspection['available']:
    assert ecs_task_resource_inspection.get('source') == 'ecs_task', ecs_task_resource_inspection
    assert isinstance(ecs_task_resource_inspection.get('ecsTask'), dict), ecs_task_resource_inspection
else:
    assert ecs_task_resource_inspection.get('unavailableReason') in {'aws_unavailable', 'resource_not_found', 'cross_account_not_configured'}, ecs_task_resource_inspection
assert os.environ['INVALID_RESOURCE_LOG_LIMIT_STATUS'] == '400', os.environ['INVALID_RESOURCE_LOG_LIMIT_STATUS']
assert invalid_resource_log_limit.strip() == 'invalid limit query', invalid_resource_log_limit
assert os.environ['INVALID_RESOURCE_INSPECTION_LIMIT_STATUS'] == '400', os.environ['INVALID_RESOURCE_INSPECTION_LIMIT_STATUS']
assert invalid_resource_inspection_limit.strip() == 'invalid limit query', invalid_resource_inspection_limit
assert os.environ['INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS'] == '400', os.environ['INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS']
assert invalid_resource_inspection_instance.strip() == 'invalid instanceId query', invalid_resource_inspection_instance
assert selected_candidate_resource_inspection.get('instanceId') == 'i-0abc123def4567890', selected_candidate_resource_inspection
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
    minimum_healthy_percent = worker_resource_inspection['ecsService'].get('minimumHealthyPercent')
    maximum_percent = worker_resource_inspection['ecsService'].get('maximumPercent')
    health_check_grace_period_seconds = worker_resource_inspection['ecsService'].get('healthCheckGracePeriodSeconds')
    assert minimum_healthy_percent is None or isinstance(minimum_healthy_percent, int), worker_resource_inspection
    assert maximum_percent is None or isinstance(maximum_percent, int), worker_resource_inspection
    assert health_check_grace_period_seconds is None or isinstance(health_check_grace_period_seconds, int), worker_resource_inspection
    assert isinstance(worker_resource_inspection['ecsService'].get('deployments', []), list), worker_resource_inspection
    assert isinstance(worker_resource_inspection['ecsService'].get('events', []), list), worker_resource_inspection
else:
    assert isinstance(worker_resource_inspection.get('unavailableReason'), str) and worker_resource_inspection['unavailableReason'], worker_resource_inspection

assert selected_worker_resource_inspection['deploymentName'] == 'worker-prod', selected_worker_resource_inspection
assert selected_worker_resource_inspection['resourceType'] == 'ecsService', selected_worker_resource_inspection
assert selected_worker_resource_inspection['resourceName'] == 'worker-prod-service', selected_worker_resource_inspection
assert selected_worker_resource_inspection.get('clusterArn') == worker_cluster_arn, selected_worker_resource_inspection
assert isinstance(selected_worker_resource_inspection['available'], bool), selected_worker_resource_inspection
assert isinstance(selected_worker_resource_inspection.get('lookupContext'), dict), selected_worker_resource_inspection
if selected_worker_resource_inspection['available']:
    assert selected_worker_resource_inspection.get('source') == 'ecs_service', selected_worker_resource_inspection
    assert isinstance(selected_worker_resource_inspection.get('ecsService'), dict), selected_worker_resource_inspection
else:
    assert isinstance(selected_worker_resource_inspection.get('unavailableReason'), str) and selected_worker_resource_inspection['unavailableReason'], selected_worker_resource_inspection
assert candidate_resource_inspection['deploymentName'] == 'payments-api-prod', candidate_resource_inspection
assert candidate_resource_inspection['resourceType'] == 'candidateAutoScalingGroup', candidate_resource_inspection
assert candidate_resource_inspection['resourceName'] == 'payments-api-prod-candidate', candidate_resource_inspection
assert isinstance(candidate_resource_inspection['available'], bool), candidate_resource_inspection
assert isinstance(candidate_resource_inspection.get('lookupContext'), dict), candidate_resource_inspection
def assert_asg_resource_inspection(payload, *, expect_limit=None, expect_instance_id=None):
    assert payload['deploymentName'] == 'payments-api-prod', payload
    assert payload['resourceType'] == 'candidateAutoScalingGroup', payload
    assert payload['resourceName'] == 'payments-api-prod-candidate', payload
    assert isinstance(payload['available'], bool), payload
    assert isinstance(payload.get('lookupContext'), dict), payload
    if expect_instance_id is not None:
        assert payload.get('instanceId') == expect_instance_id, payload
    if payload['available']:
        assert payload.get('source') == 'auto_scaling_group', payload
        asg = payload.get('autoScalingGroup')
        assert isinstance(asg, dict), payload
        assert isinstance(asg.get('autoScalingGroupName'), str) and asg['autoScalingGroupName'], payload
        launch_template = asg.get('launchTemplate')
        assert launch_template is None or isinstance(launch_template, dict), payload
        if isinstance(launch_template, dict):
            assert any(isinstance(launch_template.get(key), str) and launch_template.get(key) for key in ('id', 'name', 'version')), payload
        mixed_instances_policy = asg.get('mixedInstancesPolicy')
        assert mixed_instances_policy is None or isinstance(mixed_instances_policy, dict), payload
        if isinstance(mixed_instances_policy, dict):
            base_launch_template = mixed_instances_policy.get('launchTemplate')
            assert base_launch_template is None or isinstance(base_launch_template, dict), payload
            overrides = mixed_instances_policy.get('overrides')
            assert overrides is None or isinstance(overrides, list), payload
            for override in overrides or []:
                assert isinstance(override, dict), payload
                override_launch_template = override.get('launchTemplate')
                assert override_launch_template is None or isinstance(override_launch_template, dict), payload
        assert isinstance(asg.get('instances', []), list), payload
        if expect_instance_id is not None:
            for instance in asg.get('instances', []):
                assert instance.get('instanceId') == expect_instance_id, payload
        scope = asg.get('instancesScope')
        if expect_limit is not None:
            assert isinstance(scope, dict), payload
            assert isinstance(scope.get('requestedLimit'), int), payload
            assert isinstance(scope.get('effectiveLimit'), int), payload
            assert isinstance(scope.get('returnedInstanceCount'), int), payload
            assert scope.get('truncationStatus') in {'complete', 'truncated', 'unknown'}, payload
            assert scope.get('requestedLimit') == expect_limit, payload
            assert scope.get('effectiveLimit') <= expect_limit, payload
            assert scope.get('returnedInstanceCount') <= expect_limit, payload
        else:
            assert scope is None or isinstance(scope, dict), payload
    else:
        assert isinstance(payload.get('unavailableReason'), str) and payload['unavailableReason'], payload

assert_asg_resource_inspection(candidate_resource_inspection)
assert_asg_resource_inspection(bounded_candidate_resource_inspection, expect_limit=2)
assert_asg_resource_inspection(selected_candidate_resource_inspection, expect_instance_id='i-0abc123def4567890')

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

assert_summary_traffic_routing(codedeploy_application_resource_inspection, 'payments-api-staging', 'codeDeployApplication', 'payments-api-staging-codedeploy-app')
assert_summary_traffic_routing(codedeploy_group_resource_inspection, 'payments-api-staging', 'codeDeployDeploymentGroup', 'payments-api-staging-codedeploy-group')
assert_summary_traffic_routing(production_listener_resource_inspection, 'payments-api-staging', 'productionListener', 'payments-api-staging-prod-listener')
assert_summary_traffic_routing(test_listener_resource_inspection, 'payments-api-staging', 'testListener', 'payments-api-staging-test-listener')
assert_summary_traffic_routing(blue_task_set_target_group_resource_inspection, 'payments-api-staging', 'blueTaskSetTargetGroup', 'payments-api-staging-blue')
assert_summary_traffic_routing(production_listener_rule_resource_inspection, 'payments-api-prod', 'productionListenerRule', 'payments-api-prod-prod-listener')
assert_summary_traffic_routing(active_target_group_resource_inspection, 'payments-api-prod', 'activeTargetGroup', 'payments-api-prod-active')
assert_summary_traffic_routing(candidate_target_group_resource_inspection, 'payments-api-prod', 'candidateTargetGroup', 'payments-api-prod-candidate')
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
assert 'Visible applications after filters:' in ui, ui
assert 'Visible subset last update' in ui, ui
assert 'Latest runtime signal' in ui, ui
assert "const latestRuntimeSignalParts = (deployment) => {" in ui, ui
assert "buildPageSectionURL('application', 'application-runtime-events', { deployment: deployment.name })" in ui, ui
assert 'Operator context' in ui, ui
assert "const operatorContextRoute = () => '/v1/operator/context';" in ui, ui
assert "const operatorContextReadRoute = () => 'GET /v1/operator/context';" in ui, ui
assert "const operatorContextDeploymentAccessSummaryLabel = (context) => {" in ui, ui
assert "const operatorContextStaleScopeDiagnosticsLabel = (context) => {" in ui, ui
assert "Project/environment scope warning:" in ui, ui
assert "unmatchedProjects" in ui, ui
assert "unmatchedEnvironments" in ui, ui
assert "X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments" in ui, ui
assert "const operatorContextProjectEnvironmentExamplesLabel = (context) => {" in ui, ui
assert "Project/environment visibility:" in ui, ui
assert "projectEnvironmentExamples" in ui, ui
assert "const operatorContextVisibleDeploymentSourceLabel = (deploymentName, example, access) => {" in ui, ui
assert "const operatorContextVisibleDeploymentExamplesLabel = (context) => {" in ui, ui
assert 'Visible deployment examples:' in ui, ui
assert "via exact deployment grant + project/environment scope" in ui, ui
assert "via project/environment scope" in ui, ui
assert "const operatorContextDesiredStateSourceEntries = (context) => {" in ui, ui
assert "Desired-state source:" in ui, ui
assert "Controller refresh:" in ui, ui
assert "Webhook match statuses" in ui, ui
assert "Webhook ignored reasons" in ui, ui
assert "Diagnostic only; no webhook is sent from this page." in ui, ui
assert "operatorContextDesiredStateSourceEntries(context)" in ui, ui
assert "const operatorContextEnforcementModeLabel = (context) => {" in ui, ui
assert "Enforcement mode:" in ui, ui
assert "read/write guarded" in ui, ui
assert "local unrestricted" in ui, ui
assert "const operatorContextAccessVerdictExampleRoutesLabel = (verdict) => {" in ui, ui
assert "const visibleRoutes = routes.slice(0, 2).join('; ');" in ui, ui
assert "const hiddenCount = routes.length - 2;" in ui, ui
assert "const operatorContextAccessVerdictRequiredHeadersLabel = (verdict) => {" in ui, ui
assert "const headers = Array.isArray(verdict?.requiredTrustedHeaders) ? verdict.requiredTrustedHeaders.filter(Boolean) : [];" in ui, ui
assert "return ` Required trusted headers: ${headers.join('; ')}`;" in ui, ui
assert 'requiredTrustedHeaders' in ui, ui
assert 'Required trusted headers:' in ui, ui
assert "const operatorContextAccessVerdictScopeGrantLabel = (verdict) => verdict?.acceptsProjectEnvironmentScopes" in ui, ui
assert 'Scope grants: exact deployments or matching project/environment scopes' in ui, ui
assert 'acceptsProjectEnvironmentScopes' in ui, ui
assert "const operatorContextAccessVerdictCollectionFilterLabel = (verdict) => verdict?.filtersCollectionsByDeploymentAccess" in ui, ui
assert 'filtersCollectionsByDeploymentAccess' in ui, ui
assert "const operatorContextAccessVerdictEmptyCollectionLabel = (verdict) => verdict?.emptyCollectionLikely" in ui, ui
assert "const headers = operatorContextAccessVerdictRequiredHeadersLabel(verdict);" in ui, ui
assert "const collectionFilter = operatorContextAccessVerdictCollectionFilterLabel(verdict);" in ui, ui
assert "const emptyCollection = operatorContextAccessVerdictEmptyCollectionLabel(verdict);" in ui, ui
assert "const scopeGrants = operatorContextAccessVerdictScopeGrantLabel(verdict);" in ui, ui
assert 'return `${label} ${state}${reason}${examples}${headers}${collectionFilter}${emptyCollection}${scopeGrants}`;' in ui, ui
assert "const operatorContextAccessVerdictEntries = (context) => {" in ui, ui
assert 'Operator reads' in ui, ui
assert 'Collection reads' in ui, ui
assert 'Examples:' in ui, ui
assert 'operator-context-denied' in ui, ui
assert 'Collection scope: filtered to visible deployments' in ui, ui
assert 'Collection result: likely empty' in ui, ui
assert 'emptyCollectionLikely' in ui, ui
assert 'Deployment access summary unavailable for this response' in ui, ui
assert 'forwarded match catalog' in ui, ui
assert 'unmatched' in ui, ui
assert "loadOperatorContextBanner().catch(() => {});" in ui, ui
assert "loadOperatorContextBanner({ refresh: true });" in ui, ui
assert 'Refresh context' in ui, ui
assert 'Last checked ${timestamp.toLocaleString()}' in ui, ui
assert 'lastCheckedAt: new Date()' in ui, ui
assert 'id="operator-context-status" role="status" aria-live="polite" aria-atomic="true"' in ui, ui
assert '.operator-context-live { position: absolute;' in ui, ui
assert "liveStatus.setAttribute('aria-live', 'polite');" in ui, ui
assert "statusAnnouncement = `Operator context ready: ${summary.textContent}`;" in ui, ui
assert "statusAnnouncement = 'Refreshing trusted front-door context…';" in ui, ui
assert "statusAnnouncement = `Operator context refresh failed: ${failureMessage}`;" in ui, ui
assert '<section class="operator-context" id="operator-context" aria-live="polite"' not in ui, ui
assert 'Show access details' in ui, ui
assert 'Hide access details' in ui, ui
assert 'Role header source: ${labels[source] || labels.absent}' in ui, ui
assert 'Supported Tenet roles: ${supported.join(\', \')}' in ui, ui
assert 'operatorContextSupportedRolesLabel(context)' in ui, ui
assert "const operatorContextSupportedIdentityHeadersLabel = (context) => {" in ui, ui
assert "Supported identity headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Canonical X-Tenet-Authenticated-* identity headers are preferred when both canonical and alias values are present." in ui, ui
assert "const operatorContextManagedLogin = (context) => context?.authRuntime?.tenetManagedLogin || {};" in ui, ui
assert "const operatorContextManagedLoginActive = (context = operatorContextState.context) => {" in ui, ui
assert "const operatorContextManagedLoginProviderLabel = (provider) => {" in ui, ui
assert "google_oidc: 'Google Workspace OIDC'" in ui, ui
assert "okta_saml: 'Okta SAML'" in ui, ui
assert "Managed login provider boundary:" in ui, ui
assert "Gmail mailbox access is not requested or authorized by this runtime." in ui, ui
assert "Tenet authorization still comes from trusted roles and deployment grants." in ui, ui
assert 'Managed login lifecycle: OAuth callback state TTL' in ui, ui
assert 'Managed login provider token boundary: access token stored' in ui, ui
assert 'Managed login logout boundary: clears Tenet session cookie' in ui, ui
assert 'Authentication boundary: Tenet-managed browser login is active for this API instance; guarded read recovery may offer a same-shell sign-in handoff while trusted front-door headers still take precedence when present.' in ui, ui
assert 'Authentication boundary: this embedded shell consumes trusted front-door headers only; it does not start an in-page GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page unless the optional login/callback/session routes report active.' in ui, ui
assert 'operatorContextAuthenticationBoundaryLabel(context)' in ui, ui
assert "const operatorContextManagedLoginStateLabel = (context) => {" in ui, ui
assert "Managed login runtime:" in ui, ui
assert "Managed login missing settings:" in ui, ui
assert "Managed login storage/session:" in ui, ui
assert "const operatorContextManagedLoginCookieAttributesLabel = (context) => {" in ui, ui
assert "Managed login cookie attributes:" in ui, ui
assert "local HTTP without TLS or forwarded HTTPS evidence is expected for development" in ui, ui
assert "configured secure-cookie override" in ui, ui
assert "X-Forwarded-Proto=https" in ui, ui
assert "operatorContextManagedLoginClass(context)" in ui, ui
assert 'operatorContextSupportedIdentityHeadersLabel(context)' in ui, ui
assert "const operatorContextSupportedActorHeadersLabel = (context) => {" in ui, ui
assert "Supported actor attribution headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Explicit X-Tenet-Actor-* actor headers win over X-User-* actor aliases; trusted authenticated identity is projected only when explicit actor context is absent; actor attribution records persisted plan/apply/audit metadata and does not authorize routes." in ui, ui
assert 'operatorContextSupportedActorHeadersLabel(context)' in ui, ui
assert "Actor header sources: ${entries.join('; ')}" in ui, ui
assert 'Actor attribution: ${status}. ${summary}' in ui, ui
assert 'operatorContextActorAttributionLabel(context)' in ui, ui
assert "const operatorContextSupportedRoleHeadersLabel = (context) => {" in ui, ui
assert "Supported role headers: ${supportedHeaders.join(', ')} (role forwarding has no X-User-* alias)" in ui, ui
assert 'operatorContextSupportedRoleHeadersLabel(context)' in ui, ui
assert "const operatorContextSupportedDeploymentAccessHeadersLabel = (context) => {" in ui, ui
assert "Supported deployment access headers: ${supportedHeaders.join(', ')}" in ui, ui
assert 'operatorContextSupportedDeploymentAccessHeadersLabel(context)' in ui, ui
assert 'canonical X-Tenet-Authenticated-Roles' in ui, ui
assert 'operatorContextRoleProvenanceClass' in ui, ui
assert 'Recognized capability roles:' in ui, ui
assert 'Unsupported forwarded roles ignored for capability checks:' in ui, ui
assert 'for read capability; forward operator or admin for write capability.' in ui, ui
assert 'operatorContextUnsupportedRolesClass' in ui, ui
assert 'operator-context-details' in ui, ui
assert 'Last refresh failed:' in ui, ui
assert "const operatorContextFailureHeading = (error) => operatorReadFailureHeading(error);" in ui, ui
assert "const operatorContextFailureMessage = (error) => {" in ui, ui
assert "const message = operatorReadFailureMessage(operatorContextReadRoute(), error);" in ui, ui
assert "return message && message !== 'unknown error' ? message : 'Tenet could not load trusted front-door context.';" in ui, ui
assert "summary.textContent = operatorContextFailureHeading(operatorContextState.error) ||" in ui, ui
assert '/v1/overview/applications' in ui, ui
assert '/v1/overview/applications/${encodeURIComponent(deploymentName)}' in ui, ui
assert "const fetchApplicationsOverview = () => fetchEmbeddedReadRouteJSON('/v1/overview/applications', {" in ui, ui
assert "failure: 'applications overview request failed'," in ui, ui
assert 'No visible applications' in ui, ui
assert 'Open Operator context details' in ui, ui
assert 'currentBookmarkableURL(operatorContextAccessDetailsPanelID)' in ui, ui
assert "return 'Applications overview';" in ui, ui
assert "const setEmbeddedBrowserTitle = (...parts) => {" in ui, ui
assert "document.title = [...titleParts, 'Tenet'].join(' · ');" in ui, ui
assert "setBrowserTitleFromPageState(currentPageState);" in ui, ui
assert "setEmbeddedBrowserTitle(resource.name || pageState.resourceType, pageState.deployment, pageLabel);" in ui, ui
assert "return 'Account detail';" in ui, ui
assert "return 'GET /v1/overview/applications';" in ui, ui
assert "return `GET /v1/overview/accounts/${accountID}`;" in ui, ui
assert 'Open account' in ui, ui
assert 'Open application' in ui, ui
assert 'More actions' in ui, ui
assert 'row-primary-action' in ui, ui
assert 'row-secondary-actions' in ui, ui
assert 'Open audit log' in ui, ui
assert "buildPageURL('drift', { deployment: deployment.name })" in ui, ui
assert "buildPageURL('plan', { deployment: deployment.name })" in ui, ui
assert "buildPageURL('audit', { deployment: deployment.name })" in ui, ui
assert 'Refresh applications overview' in ui, ui
assert 'Health' in ui, ui
assert 'All health states' in ui, ui
assert "params.set('health', currentViewState.healthState)" in ui, ui
assert 'matchesHealth' in ui, ui
assert 'Refreshing applications overview from GET /v1/overview/applications…' in ui, ui
assert 'Failed to refresh applications overview:' in ui, ui
assert 'row-match-context' in ui, ui
assert 'Matched ${label}: ${value}' in ui, ui
assert 'Runtime change details' in ui, ui
assert 'History change details' in ui, ui
assert "buildPageURL('account', { accountId: deployment.awsAccountId })" in ui, ui
assert "buildPageURL('application', { deployment: deployment.name })" in ui, ui
assert 'Drift view' in ui, ui
assert 'Plan/apply' in ui, ui
assert 'Linked resources' in ui, ui
assert 'On this page' in ui, ui
assert 'Breadcrumbs' in ui, ui
assert 'No linked resources are available in this drift workspace yet.' in ui, ui
assert 'Current source: controller-authored managedDrift rows.' in ui, ui
assert 'Current source: derived fallback read-model heuristics.' in ui, ui
assert 'Severity' in ui, ui
assert 'Evidence' in ui, ui
assert 'Open resource evidence' in ui, ui
assert 'Share drift row' in ui, ui
assert "const managedDriftAnchorBase = (row) => {" in ui, ui
assert "const backendID = auditAnchorSlug(row?.driftId);" in ui, ui
assert "managedDriftAnchorURL(detail.name, rowID)" in ui, ui
assert "tableRow.setAttribute('id', rowID);" in ui, ui
assert "resolveManagedDriftResourceRef(detail, row.resourceRef)" in ui, ui
assert 'Drift summary' in ui, ui
assert 'Refresh drift workspace' in ui, ui
assert 'Clear drift filters' in ui, ui
assert "const workspaceLabel = mode === 'plan' ? 'plan workspace' : 'drift workspace';" in ui, ui
assert 'Refreshing ${workspaceLabel} from GET /v1/overview/deployments/{deploymentName}…' in ui, ui
assert 'Failed to refresh ${workspaceLabel}:' in ui, ui
assert 'Total rows' in ui, ui
assert 'Visible rows' in ui, ui
assert 'Visible high' in ui, ui
assert 'No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.' in ui, ui
assert 'Controller runtime events' in ui, ui
assert 'Refresh runtime events' in ui, ui
assert 'Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events?limit=${normalizeDeploymentEventsLimit(limit)}. Tenet shows the selected bounded window, newest first; use Event window to change how much controller history this panel reads without leaving the application drill-down. This is not audit/auth history.' in ui, ui
assert 'Loading controller runtime events…' in ui, ui
assert 'No controller runtime events were returned in the latest ${selectedLimit}-event window. Increase Event window or use Refresh runtime events before treating controller history as absent.' in ui, ui
assert 'Controller runtime events are currently unavailable.' in ui, ui
assert 'Managed drift summary · ${counts.total} total · ${counts.high} high · ${counts.medium} medium · ${counts.low} low' in ui, ui
assert "const deploymentEventsResultScopeFields = (scope) => {" in ui, ui
assert "Requested window" in ui, ui
assert "Window status" in ui, ui
assert "Capped response; more controller history may exist" in ui, ui
assert "At cap; older history may exist" in ui, ui
assert "const createControllerRefreshCue = (event) => {" in ui, ui
assert 'GitHub refresh evidence · ${fields.join(\' · \')}' in ui, ui
assert "const runtimeEventAnchorBase = (event) => {" in ui, ui
assert "const backendID = auditAnchorSlug(event?.eventId);" in ui, ui
assert "if (backendID && backendID !== 'unknown') return `runtime-event-${backendID}`;" in ui, ui
assert "const uniqueRuntimeEventAnchorID = (event, seenAnchors) => {" in ui, ui
assert "createNavLink(runtimeEventAnchorURL(deploymentName, eventID, selectedLimit), 'Share event', '_self')" in ui, ui
assert "actions.push(createNavLink(buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName }), 'Open field-level drift', '_self'));" in ui, ui
assert "const deploymentEventsReadRoute = (deploymentName, limit = currentPageState.eventLimit) => `GET ${buildDeploymentEventsReadRoute(deploymentName, { limit })}`;" in ui, ui
assert "createDeploymentEventsActions(section, deploymentName)," in ui, ui
assert "createDeploymentEventsActions(container, deploymentName)," in ui, ui
assert "deploymentEventsReadRoute(deploymentName, limit)," in ui, ui
assert 'Live inspection' in ui, ui
assert 'Loading live inspection…' in ui, ui
assert 'Live inspection is unavailable for this resource.' in ui, ui
assert 'Refresh live inspection' in ui, ui
assert 'Inspection path ·' in ui, ui
assert 'createResourceInspectionLookupPathSummary(result?.lookupContext)' in ui, ui
assert 'No live inspection details are available for this resource yet.' in ui, ui
assert 'Service deployments' in ui, ui
assert 'Container instance' in ui, ui
assert 'EC2 instance' in ui, ui
assert 'Capacity provider' in ui, ui
assert 'Availability Zone' in ui, ui
assert 'EC2 host details' in ui, ui
assert 'Optional host evidence resolved from the inspected ECS task.' in ui, ui
assert 'EC2 state' in ui, ui
assert 'EC2 instance type' in ui, ui
assert 'AMI image ID' in ui, ui
assert 'Private IP' in ui, ui
assert 'Public IP' in ui, ui
assert 'Launch time' in ui, ui
assert "const ecsTaskScopeUnavailableReasonText = (reason) => {" in ui, ui
assert "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}" in ui, ui
assert "Task sampling is unavailable for this service; service inspection remains available." in ui, ui
assert "const createECSTaskScopeSummary = (scope) => {" in ui, ui
assert "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}" in ui, ui
assert "[createECSTaskScopeSummary(service?.tasksScope)]" in ui, ui
assert 'Auto Scaling Group name' in ui, ui
assert 'Open EC2 console logs' in ui, ui
assert "const autoScalingGroupSupportsInstanceHandoff = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup'].includes(String(resource?.type || '').trim());" in ui, ui
assert "const autoScalingGroupRecentLogsHandoff = (deploymentName, resource) => {" in ui, ui
assert "const createAutoScalingInstanceHandoffLinks = (deploymentName, resource, instance) => {" in ui, ui
assert 'Inspect this instance' in ui, ui
assert 'Open instance logs' in ui, ui
assert "buildPageSectionURL('resource', 'resource-recent-logs'" in ui, ui
assert 'Launch template' in ui, ui
assert 'Target group ARNs' in ui, ui
assert 'Mixed instances overrides' in ui, ui
assert "const createBreadcrumbChrome = (items) => {" in ui, ui
assert "const createDeploymentBreadcrumb = ({ deploymentName = '', awsAccountId = '', accountName = '', currentLabel = '' } = {}) => {" in ui, ui
assert "head.append(createElement('div', 'breadcrumb-title', 'Breadcrumbs'));" in ui, ui
assert "awsAccountId ? { label: accountBreadcrumbLabel(accountName, awsAccountId), href: buildPageURL('account', { accountId: awsAccountId }) } : null," in ui, ui
assert "trail.append(createBreadcrumbCurrentChip(item.label, item.ariaLabel || `Current destination: ${item.label}`));" in ui, ui
assert "const createQuickNavChip = (text, ariaLabel) => {" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Hierarchy')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Current destination')" in ui, ui
assert "createNavLink(buildPageURL('applications'), 'Applications overview', '_self')" in ui, ui
assert "createQuickNavChip(currentWorkspace, `Current destination: ${currentWorkspace}`)" in ui, ui
assert "createNavLink(buildPageURL('audit', { deployment: detail.name || currentPageState.deployment || '' }), 'Audit log', '_self')" in ui, ui
assert "const createWorkspaceQuickNav = (detail, mode, sectionLinks = null) => {" in ui, ui
assert "const currentWorkspace = mode === 'drift' ? 'Drift view' : 'Plan/apply';" in ui, ui
assert "const linkedResources = mode === 'drift' ? groupResourceSections(detail).flatMap((group) => group.resources || []) : null;" in ui, ui
assert "const resolvedSectionLinks = mode === 'drift'" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Current section')" in ui, ui
assert "(mode === 'drift' || mode === 'plan') ? 'No section is currently targeted.' : ''" in ui, ui
assert "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)" in ui, ui
assert "const buildPageSectionURL = (page, sectionID, extras = {}) => {" in ui, ui
assert "const scrollToCurrentHashTarget = () => {" in ui, ui
assert "target.scrollIntoView({ block: 'start', behavior: 'auto' });" in ui, ui
assert "if (currentPageState.page !== 'application' && currentPageState.page !== 'account' && currentPageState.page !== 'resource' && currentPageState.page !== 'drift' && currentPageState.page !== 'plan' && currentPageState.page !== 'audit') return;" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Deployment workspaces')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'On this page')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Linked resources')" in ui, ui
assert "buildPageSectionURL('plan', sectionID, {" in ui, ui
assert "buildPageSectionURL('drift', 'drift-runtime-signals', { deployment: deploymentName })" in ui, ui
assert "buildPageSectionURL('drift', 'drift-rollout-execution', { deployment: deploymentName })" in ui, ui
assert "buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName })" in ui, ui
assert "buildPageSectionURL('drift', 'drift-status-conditions', { deployment: deploymentName })" in ui, ui
assert "runtimeSignalsSection.id = 'drift-runtime-signals';" in ui, ui
assert "executionSection.id = 'drift-rollout-execution';" in ui, ui
assert "driftSection.id = 'drift-field-level';" in ui, ui
assert "conditionsSection.id = 'drift-status-conditions';" in ui, ui
assert "createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · ${currentWorkspace}`)" in ui, ui
assert "const pushPlanSection = (section, sectionID, label) => {" in ui, ui
assert "sectionID," in ui, ui
assert "href: buildPageSectionURL('plan', sectionID, {" in ui, ui
assert "planId: currentPageState.planId || planState?.planId || planState?.record?.id || ''," in ui, ui
assert "applyId: currentPageState.applyId || applyState?.applyId || applyState?.record?.id || ''," in ui, ui
assert "pushPlanSection(runtimeSignalsSection, 'plan-runtime-signals', 'Runtime signals');" in ui, ui
assert "pushPlanSection(executionSection, 'plan-rollout-execution', 'Rollout execution');" in ui, ui
assert "pushPlanSection(actionsSection, 'plan-preview-actions', 'Plan preview actions');" in ui, ui
assert "pushPlanSection(applySection, 'plan-apply-actions', 'Apply request actions');" in ui, ui
assert "pushPlanSection(planOperationsSection, 'plan-operations', 'Plan operations');" in ui, ui
assert "const quickNav = createWorkspaceQuickNav(detail, mode, planSectionLinks);" in ui, ui
assert "activeResourceQuickNavSync = typeof quickNav?.syncCurrentSection === 'function' ? quickNav.syncCurrentSection : null;" in ui, ui
assert "header.append(head, quickNav, ...sections);" in ui, ui
assert "const createResourceQuickNav = (detail, resource, sectionLinks = []) => {" in ui, ui
assert "createElement('div', 'quick-nav-title', 'Quick navigation')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Current resource')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Current section')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'On this page')" in ui, ui
assert "createElement('div', 'quick-nav-label', 'Sibling resources')" in ui, ui
assert "createQuickNavChip(resource.name, `Current resource: ${resource.name}`)" in ui, ui
assert "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)" in ui, ui
assert "createElement('div', 'quick-nav-empty', 'No section is currently targeted.')" in ui, ui
assert "sectionLink?.sectionID !== currentSectionLink?.sectionID" in ui, ui
assert "const pushResourceSection = (section, sectionID, label) => {" in ui, ui
assert "sectionID," in ui, ui
assert "href: buildPageSectionURL('resource', sectionID, {" in ui, ui
assert "containerName: resourceRouteState.routeContainerName," in ui, ui
assert "routeContainerName: String(currentPageState.containerName || resource.containerName || '').trim()" in ui, ui
assert "containerName: params.get('containerName') || ''" in ui, ui
assert "buildPageSectionURL('resource', 'resource-recent-logs'" in ui, ui
assert "pushResourceSection(historySection, 'resource-runtime-history', 'Runtime history');" in ui, ui
assert "pushResourceSection(executionSection, 'resource-rollout-execution', 'Rollout execution');" in ui, ui
assert "pushResourceSection(conditionsSection, 'resource-status-conditions', 'Conditions');" in ui, ui
assert "pushResourceSection(inspectionSection, 'resource-live-inspection', 'Live inspection');" in ui, ui
assert "pushResourceSection(logsSection, 'resource-recent-logs', 'Recent logs');" in ui, ui
assert "const resourceLogsLimitOptions = [" in ui, ui
assert "Log entry cap · ${resourceLogsLimitText(limit)}" in ui, ui
assert "'Log limit'" in ui, ui
assert "params.set('limit', String(normalizedLimit));" in ui, ui
assert "resourceLogLimit: normalizeResourceLogsLimit(params.get('resourceLogLimit') || '')" in ui, ui
assert "const resourceLogAnchorBase = (entry) => {" in ui, ui
assert "const backendID = auditAnchorSlug(entry?.entryId);" in ui, ui
assert "return `resource-log-${backendID}`;" in ui, ui
assert "const resourceLogAnchorURL = (deploymentName, resource, logID, limit = 0) => buildPageSectionURL('resource', logID, {" in ui, ui
assert "containerName: String(currentPageState.containerName || resource?.containerName || '').trim()," in ui, ui
assert "instanceId: String(currentPageState.instanceId || resource?.instanceId || '').trim()," in ui, ui
assert "clusterArn: supportsECSClusterScope(resource) ? String(currentPageState.clusterArn || resource?.clusterArn || '').trim() : ''," in ui, ui
assert "createNavLink(resourceLogAnchorURL(deploymentName, resource, logID, selectedLimit), 'Share log entry', '_self')" in ui, ui
assert "clusterArn: params.get('clusterArn') || ''" in ui, ui
assert "routeClusterArn: supportsECSClusterScope(resource) ? String(currentPageState.clusterArn || resource.clusterArn || '').trim() : ''" in ui, ui
assert "Clear selected cluster" in ui, ui
assert "const clearSelectedClusterRoute = (detail, resource) => {" in ui, ui
assert "clusterArn: ''," in ui, ui
assert "const supportsASGInstanceScope = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup', 'userDataBootstrapSource'].includes(String(resource?.type || '').trim());" in ui, ui
assert "Clear selected instance" in ui, ui
assert "const appendAutoScalingEC2InstanceDetail = (item, instance) => {" in ui, ui
assert "const ec2Instance = instance?.ec2Instance;" in ui, ui
assert "EC2 instance type" in ui, ui
assert "AMI image ID" in ui, ui
assert "Public IP" in ui, ui
assert "Launch time" in ui, ui
assert "const clearSelectedInstanceRoute = (detail, resource) => {" in ui, ui
assert "instanceId: ''," in ui, ui
assert "createClearSelectedInstanceButton(detail, resource, resourceState)" in ui, ui
assert "renderResourceDetailPage(detail, resourceState);" in ui, ui
assert "params.set('resourceLogLimit', String(limit));" in ui, ui
assert "syncResourcePanelLimitQueryParams(deploymentName, resource);" in ui, ui
assert "Refresh recent logs" in ui, ui
assert "const responseRequestID = (response) => {" in ui, ui
assert "Tenet request ID" in ui, ui
assert "Last refreshed" in ui, ui
assert "tenetRequestId: requestId" in ui, ui
assert "createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')" in ui, ui
assert "createNavLink(buildPageURL('resource', {" in ui, ui
assert "No sibling linked resources are available for this application." in ui, ui
def assert_fragments(label, body, fragments):
    for fragment in fragments:
        assert fragment in body, f"{label} missing {fragment!r}"

operator_read_auth_fragments = (
    "const operatorReadAuthRequiredState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 401;",
    "const operatorReadForbiddenState = (error) => error?.kind === 'fetch-error' && Number(error?.status) === 403;",
    "const operatorReadRoleRequiredState = (error) => operatorReadForbiddenState(error) && String(error?.message || '').includes('authenticated viewer-capable role is required for this operator read route');",
    "const operatorReadAuthRequiredMessage = (route, context = operatorContextState.context) => {",
    "${signInRuntime.message}; use the sign-in handoff below to authenticate through ${signInRuntime.loginRoute} and return to this same bookmarkable URL.",
    "Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; reopen this same bookmarkable URL through the trusted front door",
    "const operatorReadRoleRequiredMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity because this operator read route requires a trusted forwarded viewer/operator/admin role. Open Operator context details (#operator-context-access-details-panel) and check roles.provenance.source, roles.canRead, and the forwarded X-Tenet-Authenticated-Roles value; ask the front-door/RBAC admin to forward viewer, operator, or admin, then retry this same bookmarkable page.`;",
    "const operatorReadForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized for that route or deployment. Open Operator context details (#operator-context-access-details-panel) and compare deploymentAccess.summary, access verdicts, stale/unmatched deployment/project/environment grants, and the required trusted headers for this route; ask an admin to grant the matching deployment, project, or environment scope, then retry this same bookmarkable page.`;",
    "const currentBookmarkableURL = (hashOverride) => {",
    "const operatorContextManagedLoginSignInURL = (context = operatorContextState.context) => {",
    "const operatorContextEnterpriseSSOLogoutActive = (context = operatorContextState.context) => {",
    "const operatorContextSignOutRuntime = (context = operatorContextState.context) => {",
    "operatorContextEnterpriseSSOLogoutActive(context)",
    "logoutRoute: safeManagedLoginRoute(enterpriseSso.logoutRoute)",
    "const operatorContextSignOutURL = (context = operatorContextState.context) => {",
    "const operatorContextSignOutLabel = (context = operatorContextState.context) => {",
    "Sign out of ${runtime.label} session",
    "const operatorContextSignOutHandoffLabel = (context = operatorContextState.context) => {",
    "does not sign out of upstream ${runtime.label}, trusted front-door sessions, or revoke provider tokens.",
    "Tenet-managed sign-out handoff: clears only the Tenet session cookie and returns to this bookmarkable page; it does not sign out of upstream provider or trusted front-door sessions, and it does not revoke provider tokens.",
    "Sign out of Tenet-managed browser session",
    "params.set('returnUrl', currentBookmarkableURL());",
    "return runtime?.label ? `Sign in with ${runtime.label}` : 'Sign in';",
    "const createOperatorReadFailureActions = (error) => {",
    "if (operatorReadAuthRequiredState(error)) {",
    "Retry current page after front-door sign-in",
    "if (operatorReadForbiddenState(error)) {",
    "Open Operator context details",
    "Retry current page after access update",
    "currentBookmarkableURL(operatorContextAccessDetailsPanelID)",
)
assert_fragments("application_ui", application_ui, operator_read_auth_fragments + (
    "return 'Application detail';",
    "return `GET /v1/overview/applications/${deploymentName}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
    "const deploymentEventsReadRoute = (deploymentName, limit = currentPageState.eventLimit) => `GET ${buildDeploymentEventsReadRoute(deploymentName, { limit })}`;",
    "const applicationManagedDriftCounts = (detail) => {",
    "const applicationManagedDriftSummaryText = (detail) => {",
    "No managed drift reported",
    "['Managed drift', applicationManagedDriftSummaryText(detail)],",
    "createDeploymentEventsActions(section, deploymentName),",
    "createDeploymentEventsActions(container, deploymentName),",
    "deploymentEventsReadRoute(deploymentName, limit),",
))
assert_fragments("account_ui", account_ui, operator_read_auth_fragments + (
    "return 'Account detail';",
    "return `GET /v1/overview/accounts/${accountID}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
))
for label, body in (("resource_ui", resource_ui), ("ecs_resource_ui", ecs_resource_ui), ("bootstrap_resource_ui", bootstrap_resource_ui), ("codedeploy_application_resource_ui", codedeploy_application_resource_ui), ("blue_task_set_target_group_resource_ui", blue_task_set_target_group_resource_ui), ("production_listener_rule_resource_ui", production_listener_rule_resource_ui)):
    assert_fragments(label, body, operator_read_auth_fragments + (
        "return 'Resource detail';",
        "return `GET /v1/overview/deployments/${deploymentName}`;",
        "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
        "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
    ))
assert_fragments("audit_ui", audit_ui, operator_read_auth_fragments + (
    "return `GET ${buildAuditLogsRoute({",
    "renderAuditLogsState(operatorReadFailureHeading(error), 'error', operatorReadFailureMessage(buildEmbeddedPageReadRoute(currentPageState), error), [createOperatorReadFailureActions(error)]);",
))
assert "fetchApplicationsOverview()" in ui, ui
assert "const fetchApplicationDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`, {" in application_ui, application_ui
assert "const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {" in ui, ui
assert "fetchAccountDetail(currentPageState.accountId)" in account_ui, account_ui
assert 'Quick navigation' in account_ui, account_ui
assert 'Applications overview' in account_ui, account_ui
assert 'Current destination' in account_ui, account_ui
assert 'Current section' in account_ui, account_ui
assert 'No section is currently targeted.' in account_ui, account_ui
assert 'On this page' in account_ui, account_ui
assert "const createAccountQuickNav = (account, sectionLinks = []) => {" in account_ui, account_ui
assert "createNavLink(buildPageURL('applications'), 'Applications overview', '_self')" in account_ui, account_ui
assert "createQuickNavChip('Account detail', 'Current destination: Account detail')" in account_ui, account_ui
assert 'Refresh account detail' in account_ui, account_ui
assert 'Refreshing account detail from GET /v1/overview/accounts/{awsAccountId}…' in account_ui, account_ui
assert 'Failed to refresh account detail:' in account_ui, account_ui
assert "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)" in account_ui, account_ui
assert "detailSummary.id = 'account-summary';" in account_ui, account_ui
assert "const accountGroupSectionLabel = (group) => {" in account_ui, account_ui
assert "return region === '—' ? `${app} · ${environment}` : `${app} · ${environment} · ${region}`;" in account_ui, account_ui
assert "groupSection.id = accountGroupSectionID(group);" in account_ui, account_ui
assert "href: buildPageSectionURL('account', sectionID, { accountId: accountID })," in account_ui, account_ui
assert "failure: 'account detail request failed'," in ui, ui
assert "const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);" in application_ui, application_ui
assert "const applicationDetailReadRoute = (deploymentName) => `GET /v1/overview/applications/${deploymentName || '{deploymentName}'}`;" in application_ui, application_ui
assert "const applicationStatusSignal = (detail) => {" in application_ui, application_ui
assert "const applicationRuntimeEventSummary = (detail) => {" in application_ui, application_ui
assert "const createApplicationStatusSummary = (detail, resourceGroups = []) => {" in application_ui, application_ui
assert "const createApplicationDetailActions = (applicationState = null) => [" in application_ui, application_ui
assert 'Refresh application detail' in application_ui, application_ui
assert 'Refreshing application detail from GET /v1/overview/applications/{deploymentName}…' in application_ui, application_ui
assert 'Failed to refresh application detail:' in application_ui, application_ui
assert "const renderApplicationDetailPage = (detail, applicationState = null) => {" in application_ui, application_ui
assert "const breadcrumb = createDeploymentBreadcrumb({" in application_ui, application_ui
assert "const nonEmptyResourceSections = (detail) => groupResourceSections(detail).filter((group) => Array.isArray(group?.resources) && group.resources.length > 0);" in application_ui, application_ui
assert "const resourceGroups = nonEmptyResourceSections(detail);" in application_ui, application_ui
assert 'Current application status' in application_ui, application_ui
assert 'Application rollout context:' in application_ui, application_ui
assert "['Runtime events', applicationRuntimeEventSummary(detail)]," in application_ui, application_ui
assert "page.append(createApplicationStatusSummary(detail, resourceGroups));" in application_ui, application_ui
assert "createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')" in application_ui, application_ui
assert "const resourceSupportsLiveInspection = (resource) => {" in application_ui, application_ui
assert "const resourceSupportsRecentLogs = (resource) => {" in application_ui, application_ui
assert "'codeDeployApplication',\n          'codeDeployDeploymentGroup'" in application_ui, application_ui
assert "'trafficShiftPlan',\n          'productionListenerRule',\n          'activeTargetGroup',\n          'candidateTargetGroup'" in application_ui, application_ui
assert "createNavLink(buildPageSectionURL('resource', 'resource-live-inspection', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Open live inspection', '_self')" in application_ui, application_ui
assert "createNavLink(buildPageSectionURL('resource', 'resource-recent-logs', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Open recent logs', '_self')" in application_ui, application_ui
assert 'Open live inspection' in application_ui, application_ui
assert 'Open recent logs' in application_ui, application_ui
assert 'Application resources' in application_ui, application_ui
assert 'No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.' in application_ui, application_ui
assert 'Jump to runtime events' in application_ui, application_ui
assert "emptyStateActions.append(createNavLink(buildPageSectionURL('application', 'application-runtime-events', { deployment: deploymentName }), 'Jump to runtime events', '_self'));" in application_ui, application_ui
assert 'Quick navigation' in application_ui, application_ui
assert 'Applications overview' in application_ui, application_ui
assert 'Current destination' in application_ui, application_ui
assert 'Current section' in application_ui, application_ui
assert 'No section is currently targeted.' in application_ui, application_ui
assert 'On this page' in application_ui, application_ui
assert "const createApplicationQuickNav = (detail, sectionLinks = []) => {" in application_ui, application_ui
assert "createNavLink(buildPageURL('applications'), 'Applications overview', '_self')" in application_ui, application_ui
assert "createQuickNavChip('Application detail', 'Current destination: Application detail')" in application_ui, application_ui
assert "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)" in application_ui, application_ui
assert "createElement('div', 'quick-nav-empty', 'No additional section links are available for this application.')" in application_ui, application_ui
assert "eventsSection.id = 'application-runtime-events';" in application_ui, application_ui
for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui, codedeploy_application_resource_ui, blue_task_set_target_group_resource_ui, production_listener_rule_resource_ui):
    assert "const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);" in body, body
    assert 'Opened via bookmarked resource alias.' in body, body
    assert "const renderResourceDetailPage = (detail, resourceState = null) => {" in body, body
    assert "const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;" in body, body
    assert "const createResourceDetailActions = (resourceState = null, detail = null, resource = null) => [" in body, body
    assert 'Refresh resource detail' in body, body
    assert 'Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…' in body, body
    assert 'Current resource detail remains visible below.' in body, body
    assert 'Failed to refresh resource detail:' in body, body
    assert "renderResourceDetailPage(resourceState.detail, resourceState);" in body, body
    assert "return fetchDeploymentDetail(deploymentName)" in body, body
    assert "currentLabel: resource.name," in body, body
    assert "const createResourceQuickNav = (detail, resource, sectionLinks = []) => {" in body, body
    assert "const pushResourceSection = (section, sectionID, label) => {" in body, body
    assert "pushResourceSection(historySection, 'resource-runtime-history', 'Runtime history');" in body, body
    assert "pushResourceSection(executionSection, 'resource-rollout-execution', 'Rollout execution');" in body, body
    assert "pushResourceSection(conditionsSection, 'resource-status-conditions', 'Conditions');" in body, body
    assert "pushResourceSection(inspectionSection, 'resource-live-inspection', 'Live inspection');" in body, body
    assert "pushResourceSection(logsSection, 'resource-recent-logs', 'Recent logs');" in body, body
    assert "const renderOperatorReadFailurePanel = (container, title, actions, route, error) => {" in body, body
    assert "const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;" in body, body
    assert "const resourceLogsLimitOptions = [" in body, body
    assert "Log entry cap · ${resourceLogsLimitText(limit)}" in body, body
    assert "'Log limit'" in body, body
    assert "resourceLogLimit: normalizeResourceLogsLimit(params.get('resourceLogLimit') || '')" in body, body
    assert "clusterArn: params.get('clusterArn') || ''" in body, body
    assert "routeClusterArn: supportsECSClusterScope(resource) ? String(currentPageState.clusterArn || resource.clusterArn || '').trim() : ''" in body, body
    assert "Clear selected cluster" in body, body
    assert "const clearSelectedClusterRoute = (detail, resource) => {" in body, body
    assert "clusterArn: ''," in body, body
    assert "renderResourceDetailPage(detail, resourceState);" in body, body
    assert "params.set('resourceLogLimit', String(limit));" in body, body
    assert "syncResourcePanelLimitQueryParams(deploymentName, resource);" in body, body
    assert "...resourcePanelLimitURLParams(deploymentName, resource)" in body, body
    assert "history.replaceState({}, '', `${nextURL}${window.location.hash || ''}`);" in body, body
    assert "const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;" in body, body
    assert "const resourceInspectionTaskDefaultLimit = 10;" in body, body
    assert "const resourceInspectionLimitOptions = [" in body, body
    assert "resourceInspectionLimitLabel(resource)" in body, body
    assert "'Task sample limit'" in body, body
    assert "'ASG instance sample limit'" in body, body
    assert "resourceInspectionLimit: normalizeResourceInspectionTaskLimit(params.get('resourceInspectionLimit') || '')" in body, body
    assert "params.set('resourceInspectionLimit', String(limit));" in body, body
    assert "supportsBoundedResourceInspection(resource)" in body, body
    assert "const ecsTaskScopeUnavailableReasonText = (reason) => {" in body, body
    assert "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}" in body, body
    assert "Task sampling is unavailable for this service; service inspection remains available." in body, body
    assert "const createECSTaskScopeSummary = (scope) => {" in body, body
    assert "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}" in body, body
    assert "[createECSTaskScopeSummary(service?.tasksScope)]" in body, body
    assert "const createAutoScalingInstanceScopeSummary = (scope) => {" in body, body
    assert "ASG instance sample scope · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)} · returned ${countText(returnedInstanceCount)} instance(s) · ${truncationStatus}" in body, body
    assert "ASG instance sampling is unavailable for this group; group inspection remains available." in body, body
    assert "createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')" in body, body
    assert "createNavLink(buildPageURL('resource', {" in body, body
    assert "const resolveBackingResourceDrillDown = (detail, inspectionDetail) => {" in body, body
    assert "const createBackingResourceHandoffSection = (detail, inspectionDetail) => {" in body, body
    assert 'Resolved backing resource' in body, body
    assert 'Quick navigation' in body, body
    assert 'Current section' in body, body
    assert 'On this page' in body, body
assert 'Linked resources' in ui, ui
assert 'No linked resources are available in this drift workspace yet.' in ui, ui
assert 'Audit timeline' in audit_ui, audit_ui
assert 'Deployment audit timeline' in audit_ui, audit_ui
assert 'Visible audit entries' in audit_ui, audit_ui
assert 'Plan previews' in audit_ui, audit_ui
assert 'Apply requests' in audit_ui, audit_ui
assert 'Actor recorded' in audit_ui, audit_ui
assert 'Actor unavailable/unknown' in audit_ui, audit_ui
assert 'Active deployment context' in audit_ui, audit_ui
assert 'Breadcrumbs' in audit_ui, audit_ui
assert 'Quick navigation' in audit_ui, audit_ui
assert 'Deployment workspaces' in audit_ui, audit_ui
assert 'Current section' in audit_ui, audit_ui
assert 'No section is currently targeted.' in audit_ui, audit_ui
assert 'On this page' in audit_ui, audit_ui
assert "const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(" in audit_ui, audit_ui
assert "const deploymentContextSectionID = 'audit-deployment-context';" in audit_ui, audit_ui
assert "deploymentCallout.setAttribute('id', deploymentContextSectionID);" in audit_ui, audit_ui
assert "const timelineSectionID = 'audit-timeline';" in audit_ui, audit_ui
assert "timelineSection.setAttribute('id', timelineSectionID);" in audit_ui, audit_ui
assert "buildPageSectionURL('audit', sectionID, {" in audit_ui, audit_ui
assert "createElement('div', 'quick-nav-current', `${deploymentName || 'Deployment'} · ${currentWorkspace}`)" in audit_ui, audit_ui
assert 'Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.' in audit_ui, audit_ui
assert 'Current audit read: ${buildEmbeddedPageReadRoute(currentPageState)}' in audit_ui, audit_ui
assert 'Source record · ${entry.source.recordRoute}' in audit_ui, audit_ui
assert 'Source detail · ${entry.source.sourceRoute}' in audit_ui, audit_ui
assert "createNavLink(entry.source.recordRoute, 'Open source record')" in audit_ui, audit_ui
assert "createNavLink(entry.source.sourceRoute, 'Open source detail')" in audit_ui, audit_ui
assert 'Inspect audit sources' in audit_ui, audit_ui
assert "const auditEntryAnchorID = (entry) => {" in audit_ui, audit_ui
assert "row.setAttribute('id', auditEntryAnchorID(entry));" in audit_ui, audit_ui
assert "createNavLink(auditEntryAnchorURL(entry), 'Open audit entry', '_self')" in audit_ui, audit_ui
assert 'Share or continue in app' in audit_ui, audit_ui
assert 'No audit activity has been persisted yet.' in audit_ui, audit_ui
assert 'No audit activity has been persisted yet for deployment ${currentPageState.deployment}. Create a plan preview or apply request from the plan/apply workspace to add immutable audit rows for this deployment.' in audit_ui, audit_ui
assert "const auditFilterLabels = () => {" in audit_ui, audit_ui
assert "labels.push(`action ${auditActionLabel(currentPageState.auditAction)}`);" in audit_ui, audit_ui
assert "const auditEmptyStateText = () => {" in audit_ui, audit_ui
assert "No audit entries match the current filters (${filters.join(' · ')}${hasLimit ? `; ${auditLimitLabel()}` : ''}). Clear audit controls to return to the full audit timeline." in audit_ui, audit_ui
assert 'Loading audit log…' in audit_ui, audit_ui
assert 'Audit log is currently unavailable.' in audit_ui, audit_ui
assert 'Refresh audit log' in audit_ui, audit_ui
assert 'Refreshing audit log from GET /v1/audit-logs…' in audit_ui, audit_ui
assert 'Failed to refresh audit log:' in audit_ui, audit_ui
assert 'Clear audit filters' in audit_ui, audit_ui
assert "const createAuditLogsActions = (isLoading = false) => [" in audit_ui, audit_ui
assert "const auditLogsRefreshStateText = (auditState) => {" in audit_ui, audit_ui
assert "const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {" in audit_ui, audit_ui
assert "if (action) params.set('action', action);" in audit_ui, audit_ui
assert "const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(" in audit_ui, audit_ui
assert "const normalizeAuditLimit = (value) => {" in audit_ui, audit_ui
assert "{ value: 'plan.created', label: 'Plan preview created (plan.created)' }," in audit_ui, audit_ui
assert "{ value: 'apply.requested', label: 'Apply requested (apply.requested)' }," in audit_ui, audit_ui
assert "const auditActionLabel = (value) => {" in audit_ui, audit_ui
assert "labels.push(`action ${auditActionLabel(currentPageState.auditAction)}`);" in audit_ui, audit_ui
assert "const auditLimitSelectOptions = (limit) => {" in audit_ui, audit_ui
assert "const syncAuditQueryParams = (params, pageState) => {" in audit_ui, audit_ui
assert "const clearAuditPageFilters = () => {" in audit_ui, audit_ui
assert "createElement('button', 'toolbar-button', activeFilters.length ? 'Clear audit filters' : 'Clear audit controls')" in audit_ui, audit_ui
assert "createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {" in audit_ui, audit_ui
assert "createSelect('Limit', String(currentPageState.auditLimit || ''), auditLimitSelectOptions(currentPageState.auditLimit), (event) => {" in audit_ui, audit_ui
assert "const deploymentName = auditDeploymentName();" in audit_ui, audit_ui
assert "? createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')" in audit_ui, audit_ui
assert ": createNavLink(buildPageURL('applications'), 'Back to applications', '_self')" in audit_ui, audit_ui
assert 'Open application drill-down' in audit_ui, audit_ui
assert 'Open drift view' in audit_ui, audit_ui
assert 'Open plan/apply workspace' in audit_ui, audit_ui
assert "const defaultDeploymentEventsLimit = 10;" in ui, ui
assert "const buildDeploymentEventsReadRoute = (deploymentName, { limit = defaultDeploymentEventsLimit } = {}) => {" in ui, ui
assert "return `${route}?limit=${normalizedLimit}`;" in ui, ui
assert "const fetchDeploymentEvents = (deploymentName, limit = currentPageState.eventLimit) => fetchEmbeddedReadRouteJSON(buildDeploymentEventsReadRoute(deploymentName, { limit }), {" in ui, ui
assert "const ifMatch = workspaceDetail.sourceFingerprint || await derivePlanSourceFingerprint(workspaceDetail).catch(() => '') || fallbackIfMatch;" in ui, ui
assert 'Recent logs' in ui, ui
assert 'Refresh recent logs' in ui, ui
assert 'Requested cap' in ui, ui
assert 'Effective cap' in ui, ui
assert 'Returned entries' in ui, ui
assert 'Scope status' in ui, ui
assert 'Capped response; more rows may exist' in ui, ui
assert 'At cap; provider totals unknown' in ui, ui
assert 'Backing resource type' in ui, ui
assert 'Backing resource name' in ui, ui
assert 'Resource description' in ui, ui
assert 'Selected container' in ui, ui
assert 'Selected cluster' in ui, ui
assert 'Log container' in ui, ui
assert 'Open container recent logs' in ui, ui
assert 'Container instance' in ui, ui
assert 'EC2 instance' in ui, ui
assert 'Capacity provider' in ui, ui
assert 'Availability Zone' in ui, ui
assert "params.set('containerName', containerName);" in ui, ui
assert 'Lookup path ·' in ui, ui
assert 'Lookup region' in ui, ui
assert 'Observer role configured' in ui, ui
assert 'This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.' in ui, ui
assert "const resourceLogsEmptyReasonText = (reason) => {" in ui, ui
assert 'Tenet reached the recent-log provider, but no events were found in the selected bounded window.' in ui, ui
assert 'Tenet found provider records, but none could be formatted into operator-visible recent-log rows.' in ui, ui
assert 'Plan preview actions' in ui, ui
assert 'Apply request actions' in ui, ui
assert 'Plan preview approval guidance' in ui, ui
assert 'Create Plan will POST /v1/plans/deployments/${deploymentName}, store an immutable server-authored preview, and update this workspace URL with a shareable planId.' in ui, ui
assert 'Authenticated operator required' in ui, ui
assert 'const operatorWriteAuthRequiredMessage = (route, action, context = operatorContextState.context) => {' in ui, ui
assert '${signInRuntime.message}; sign in through ${signInRuntime.loginRoute}, then return to this same bookmarkable workspace and retry ${action}.' in ui, ui
assert 'Trusted front-door X-Tenet-Authenticated-* identity headers still take precedence when present, and Tenet will not send the write until authenticated identity is available.' in ui, ui
assert 'Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; retry ${action} for this same bookmarkable workspace through the trusted front door' in ui, ui
assert 'const operatorWriteForbiddenMessage = (route, action) => `Tenet rejected ${route} after recognizing the caller\x27s identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route. Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry ${action}. Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.`;' in ui, ui
assert 'Operator access denied' in ui, ui
assert "const operatorPlanPreviewReadRoute = (planId) => `GET /v1/plans/${planId || 'planId'}`;" in ui, ui
assert "const operatorApplyRequestReadRoute = (applyId) => `GET /v1/applies/${applyId || 'applyId'}`;" in ui, ui
assert "return `${operatorReadFailureMessage(operatorPlanPreviewReadRoute(activePlanId), planState.error)} Deployment-derived context is still shown below and Apply Plan stays disabled until a persisted ready preview is available.`;" in ui, ui
assert "return `${operatorReadFailureMessage(operatorApplyRequestReadRoute(activeApplyID), applyState.error)} Deployment and plan-derived context are still shown below.`;" in ui, ui
assert "if (operatorReadStateForbidden(state) || operatorWriteForbiddenState(state)) return 'Access denied';" in ui, ui
assert 'Refresh plan preview' in ui, ui
assert 'Refresh apply request' in ui, ui
assert 'Refresh plan workspace' in ui, ui
assert "const workspaceLabel = mode === 'plan' ? 'plan workspace' : 'drift workspace';" in ui, ui
assert 'Refreshing ${workspaceLabel} from GET /v1/overview/deployments/{deploymentName}…' in ui, ui
assert 'Failed to refresh ${workspaceLabel}:' in ui, ui
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
assert 'GET /v1/plans/${activePlanId} is currently unavailable. Deployment-derived context is still shown below and Apply Plan stays disabled until the persisted ready preview can be loaded again.' in ui, ui
assert 'GET /v1/applies/${activeApplyID} is currently unavailable. Deployment and plan-derived context are still shown below.' in ui, ui
assert 'Apply lifecycle summary' in ui, ui
assert "params.get('planId') || ''" in ui, ui
assert "params.get('applyId') || ''" in ui, ui
assert 'Current section' in plan_ui, plan_ui
assert 'No section is currently targeted.' in plan_ui, plan_ui
assert 'On this page' in plan_ui, plan_ui
assert "buildPageSectionURL('plan', sectionID, {" in plan_ui, plan_ui
assert 'planId' in plan_ui, plan_ui
assert 'applyId' in apply_ui, apply_ui
assert 'Current section' in apply_ui, apply_ui
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
start_api true true false false

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
frontdoor_plan_preview_path=$(write_payload frontdoor-plan-preview.json "$frontdoor_plan_preview")
frontdoor_plan_id=$(json_string_field_from_path "$frontdoor_plan_preview_path" id)
frontdoor_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  "http://$HOST:$PORT/v1/applies/plans/$frontdoor_plan_id")
frontdoor_apply_record_path=$(write_payload frontdoor-apply-record.json "$frontdoor_apply_record")
frontdoor_apply_id=$(json_string_field_from_path "$frontdoor_apply_record_path" id)
auth_unauthenticated_transition_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id/transitions")
if [[ "$APPLY_ENABLED" == "true" ]]; then
  frontdoor_apply_record_get=$(wait_for_apply_status "$frontdoor_apply_id" failed)
else
  frontdoor_apply_record_get=$(curl -fsS "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
fi
frontdoor_audit_logs=$(curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
frontdoor_apply_record_get_path=$(write_payload frontdoor-apply-record-get.json "$frontdoor_apply_record_get")
frontdoor_audit_logs_path=$(write_payload frontdoor-audit-logs.json "$frontdoor_audit_logs")

AUTH_UNAUTHENTICATED_PLAN_STATUS="$auth_unauthenticated_plan_status" AUTH_UNAUTHENTICATED_PLAN_BODY_PATH="$AUTH_UNAUTHENTICATED_PLAN_BODY" AUTH_UNAUTHENTICATED_APPLY_STATUS="$auth_unauthenticated_apply_status" AUTH_UNAUTHENTICATED_APPLY_BODY_PATH="$AUTH_UNAUTHENTICATED_APPLY_BODY" AUTH_UNAUTHENTICATED_TRANSITION_STATUS="$auth_unauthenticated_transition_status" AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH="$AUTH_UNAUTHENTICATED_TRANSITION_BODY" FRONTDOOR_PLAN_PREVIEW_PATH="$frontdoor_plan_preview_path" FRONTDOOR_APPLY_RECORD_PATH="$frontdoor_apply_record_path" FRONTDOOR_APPLY_RECORD_GET_PATH="$frontdoor_apply_record_get_path" FRONTDOOR_AUDIT_LOGS_PATH="$frontdoor_audit_logs_path" python3 - <<'PY'
import json, os

with open(os.environ['AUTH_UNAUTHENTICATED_PLAN_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_plan_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_APPLY_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_apply_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_transition_body = fh.read()
with open(os.environ['FRONTDOOR_PLAN_PREVIEW_PATH'], encoding='utf-8') as fh:
    frontdoor_plan_preview = json.load(fh)
with open(os.environ['FRONTDOOR_APPLY_RECORD_PATH'], encoding='utf-8') as fh:
    frontdoor_apply_record = json.load(fh)
with open(os.environ['FRONTDOOR_APPLY_RECORD_GET_PATH'], encoding='utf-8') as fh:
    frontdoor_apply_record_get = json.load(fh)
with open(os.environ['FRONTDOOR_AUDIT_LOGS_PATH'], encoding='utf-8') as fh:
    frontdoor_audit_logs = json.load(fh)

assert os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS']
assert unauthenticated_plan_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_plan_body
assert os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS']
assert unauthenticated_apply_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_apply_body
assert os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS']
assert unauthenticated_transition_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_transition_body

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

stop_api
start_api true false true true

auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/applications")
auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/status/deployments")
auth_unauthenticated_account_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/accounts")
auth_unauthenticated_catalog_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/catalog/deployments")
auth_unauthenticated_audit_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/audit-logs?action=apply.requested")
operator_context_trusted=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_mixed_roles=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: mixed-role-123" \
  -H "X-Tenet-Authenticated-Name: Mixed Role Operator" \
  -H "X-Tenet-Authenticated-Roles: viewer, auditor" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_combined_access=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: prod-apne2" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_unmatched=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, stale-api, worker-prod" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_stale_scopes=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Projects: payments-api, archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2, stale-env" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_alias=$(curl -fsS \
  -H "X-User-ID: alias-456" \
  -H "X-User-Name: Alias Operator" \
  -H "X-User-Email: alias@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_actor_precedence=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-actor-789" \
  -H "X-Tenet-Authenticated-Name: Front Door Actor" \
  -H "X-Tenet-Authenticated-Email: frontdoor.actor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Actor-Id: explicit-789" \
  -H "X-Tenet-Actor-Name: Explicit Actor" \
  -H "X-Tenet-Actor-Email: explicit.actor@example.com" \
  -H "X-User-ID: alias-789" \
  -H "X-User-Name: Alias Should Not Win" \
  -H "X-User-Email: alias-should-not-win@example.com" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_no_allowlist=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: no-allowlist-789" \
  -H "X-Tenet-Authenticated-Name: No Allowlist Operator" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_no_roles=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: no-roles-321" \
  -H "X-Tenet-Authenticated-Name: Missing Role Operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "http://$HOST:$PORT/v1/operator/context")
operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")
operator_context_mixed_roles_path=$(write_payload operator-context-mixed-roles.json "$operator_context_mixed_roles")
operator_context_combined_access_path=$(write_payload operator-context-combined-access.json "$operator_context_combined_access")
operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")
operator_context_stale_scopes_path=$(write_payload operator-context-stale-scopes.json "$operator_context_stale_scopes")
operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")
operator_context_actor_precedence_path=$(write_payload operator-context-actor-precedence.json "$operator_context_actor_precedence")
operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")
operator_context_no_roles_path=$(write_payload operator-context-no-roles.json "$operator_context_no_roles")
auth_role_required_collection_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_COLLECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "http://$HOST:$PORT/v1/overview/applications")
auth_role_required_status_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "http://$HOST:$PORT/v1/status/deployments/payments-api-staging")
auth_role_required_audit_collection_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "http://$HOST:$PORT/v1/audit-logs?action=apply.requested")
auth_unauthenticated_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_READ_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging")
auth_unauthenticated_status_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_READ_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/status/deployments/payments-api-staging")
auth_unauthenticated_events_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_EVENTS_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/events")
auth_unauthenticated_resource_logs_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
auth_unauthenticated_resource_inspection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
auth_unauthenticated_plan_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/plans/$frontdoor_plan_id")
auth_unauthenticated_apply_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" -w "%{http_code}" \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
auth_role_required_plan_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "http://$HOST:$PORT/v1/plans/$frontdoor_plan_id")
auth_role_required_apply_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_APPLY_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
auth_unauthenticated_transition_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H 'Content-Type: application/json' \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id/transitions")
auth_forbidden_plan_write_status=$(curl -sS -o "$AUTH_FORBIDDEN_PLAN_WRITE_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-staging")
auth_forbidden_apply_write_status=$(curl -sS -o "$AUTH_FORBIDDEN_APPLY_WRITE_BODY" -w "%{http_code}" \
  -X POST \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/applies/plans/$frontdoor_plan_id")
auth_forbidden_transition_status=$(curl -sS -o "$AUTH_FORBIDDEN_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id/transitions")
auth_forbidden_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging")
auth_forbidden_status_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_STATUS_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/status/deployments/payments-api-staging")
auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/events")
auth_forbidden_resource_logs_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_resource_inspection_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_account_detail_status=$(curl -sS -o "$AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "http://$HOST:$PORT/v1/overview/accounts/123456789012")
auth_forbidden_group_detail_status=$(curl -sS -o "$AUTH_FORBIDDEN_GROUP_DETAIL_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2")
auth_forbidden_plan_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_PLAN_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/plans/$frontdoor_plan_id")
auth_forbidden_apply_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_APPLY_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
allowed_guarded_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging")
allowed_status_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/status/deployments/payments-api-staging")
allowed_events_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/events?limit=1")
allowed_resource_logs=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
allowed_resource_inspection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
allowed_plan_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/plans/$frontdoor_plan_id")
allowed_apply_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/applies/$frontdoor_apply_id")
scoped_write_plan_preview=$(curl -fsS -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: scoped-writer-123" \
  -H "X-Tenet-Authenticated-Name: Scoped Write Operator" \
  -H "X-Tenet-Authenticated-Email: scoped-writer@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  -d '{}' \
  "http://$HOST:$PORT/v1/plans/deployments/payments-api-staging")
scoped_write_plan_preview_path=$(write_payload scoped-write-plan-preview.json "$scoped_write_plan_preview")
scoped_write_plan_id=$(json_string_field_from_path "$scoped_write_plan_preview_path" id)
scoped_write_apply_record=$(curl -fsS -X POST \
  -H "X-Tenet-Authenticated-Id: scoped-writer-123" \
  -H "X-Tenet-Authenticated-Name: Scoped Write Operator" \
  -H "X-Tenet-Authenticated-Email: scoped-writer@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/applies/plans/$scoped_write_plan_id")
scoped_write_apply_record_path=$(write_payload scoped-write-apply-record.json "$scoped_write_apply_record")
scoped_write_apply_id=$(json_string_field_from_path "$scoped_write_apply_record_path" id)
SCOPED_WRITE_TRANSITION_BODY=$(write_payload scoped-write-transition-body.txt "")
scoped_write_transition_status=$(curl -sS -o "$SCOPED_WRITE_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: scoped-writer-123" \
  -H "X-Tenet-Authenticated-Name: Scoped Write Operator" \
  -H "X-Tenet-Authenticated-Email: scoped-writer@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  --data '{"status":"running","step":1}' \
  "http://$HOST:$PORT/v1/applies/$scoped_write_apply_id/transitions")
filtered_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "http://$HOST:$PORT/v1/overview/applications")
filtered_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "http://$HOST:$PORT/v1/status/deployments")
filtered_catalog_deployments=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "http://$HOST:$PORT/v1/catalog/deployments")
filtered_accounts_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "http://$HOST:$PORT/v1/overview/accounts")
filtered_account_detail=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "http://$HOST:$PORT/v1/overview/accounts/123456789012")
filtered_group_detail=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "http://$HOST:$PORT/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2")
empty_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/overview/applications")
empty_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/status/deployments")
empty_catalog_deployments=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/catalog/deployments")
empty_accounts_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  "http://$HOST:$PORT/v1/overview/accounts")
filtered_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, payments-api-staging" \
  "http://$HOST:$PORT/v1/audit-logs")
empty_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/audit-logs?action=apply.requested")
auth_forbidden_audit_status=$(curl -sS -o "$AUTH_FORBIDDEN_AUDIT_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
allowed_scoped_audit=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1")
allowed_guarded_read_path=$(write_payload allowed-guarded-read.json "$allowed_guarded_read")
allowed_plan_read_path=$(write_payload allowed-plan-read.json "$allowed_plan_read")
allowed_apply_read_path=$(write_payload allowed-apply-read.json "$allowed_apply_read")
allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")
allowed_resource_logs_path=$(write_payload allowed-resource-logs.json "$allowed_resource_logs")
allowed_resource_inspection_path=$(write_payload allowed-resource-inspection.json "$allowed_resource_inspection")
filtered_applications_collection_path=$(write_payload filtered-applications-collection.json "$filtered_applications_collection")
filtered_accounts_collection_path=$(write_payload filtered-accounts-collection.json "$filtered_accounts_collection")
filtered_account_detail_path=$(write_payload filtered-account-detail.json "$filtered_account_detail")
filtered_group_detail_path=$(write_payload filtered-group-detail.json "$filtered_group_detail")
filtered_audit_collection_path=$(write_payload filtered-audit-collection.json "$filtered_audit_collection")
empty_applications_collection_path=$(write_payload empty-applications-collection.json "$empty_applications_collection")
empty_accounts_collection_path=$(write_payload empty-accounts-collection.json "$empty_accounts_collection")
empty_audit_collection_path=$(write_payload empty-audit-collection.json "$empty_audit_collection")
allowed_scoped_audit_path=$(write_payload allowed-scoped-audit.json "$allowed_scoped_audit")
allowed_status_read_path=$(write_payload allowed-status-read.json "$allowed_status_read")
filtered_status_collection_path=$(write_payload filtered-status-collection.json "$filtered_status_collection")
filtered_catalog_deployments_path=$(write_payload filtered-catalog-deployments.json "$filtered_catalog_deployments")
empty_catalog_deployments_path=$(write_payload empty-catalog-deployments.json "$empty_catalog_deployments")
empty_status_collection_path=$(write_payload empty-status-collection.json "$empty_status_collection")

AUTH_UNAUTHENTICATED_COLLECTION_STATUS="$auth_unauthenticated_collection_status" \
AUTH_UNAUTHENTICATED_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS="$auth_unauthenticated_status_collection_status" \
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_STATUS="$auth_unauthenticated_account_collection_status" \
AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_STATUS="$auth_unauthenticated_catalog_collection_status" \
AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS="$auth_unauthenticated_audit_collection_status" \
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" \
AUTH_ROLE_REQUIRED_COLLECTION_STATUS="$auth_role_required_collection_status" \
AUTH_ROLE_REQUIRED_COLLECTION_BODY_PATH="$AUTH_ROLE_REQUIRED_COLLECTION_BODY" \
AUTH_ROLE_REQUIRED_STATUS_READ_STATUS="$auth_role_required_status_read_status" \
AUTH_ROLE_REQUIRED_STATUS_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" \
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS="$auth_role_required_audit_collection_status" \
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY_PATH="$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" \
AUTH_UNAUTHENTICATED_READ_STATUS="$auth_unauthenticated_read_status" \
AUTH_UNAUTHENTICATED_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_READ_BODY" \
AUTH_UNAUTHENTICATED_STATUS_READ_STATUS="$auth_unauthenticated_status_read_status" \
AUTH_UNAUTHENTICATED_STATUS_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_STATUS_READ_BODY" \
AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status" \
AUTH_UNAUTHENTICATED_EVENTS_BODY_PATH="$AUTH_UNAUTHENTICATED_EVENTS_BODY" \
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS="$auth_unauthenticated_resource_logs_status" \
AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY_PATH="$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" \
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS="$auth_unauthenticated_resource_inspection_status" \
AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY_PATH="$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" \
AUTH_FORBIDDEN_ACCOUNT_DETAIL_STATUS="$auth_forbidden_account_detail_status" \
AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY_PATH="$AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY" \
AUTH_FORBIDDEN_GROUP_DETAIL_STATUS="$auth_forbidden_group_detail_status" \
AUTH_FORBIDDEN_GROUP_DETAIL_BODY_PATH="$AUTH_FORBIDDEN_GROUP_DETAIL_BODY" \
AUTH_UNAUTHENTICATED_PLAN_READ_STATUS="$auth_unauthenticated_plan_read_status" \
AUTH_UNAUTHENTICATED_PLAN_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" \
AUTH_UNAUTHENTICATED_APPLY_READ_STATUS="$auth_unauthenticated_apply_read_status" \
AUTH_UNAUTHENTICATED_APPLY_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" \
AUTH_ROLE_REQUIRED_PLAN_READ_STATUS="$auth_role_required_plan_read_status" \
AUTH_ROLE_REQUIRED_PLAN_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" \
AUTH_ROLE_REQUIRED_APPLY_READ_STATUS="$auth_role_required_apply_read_status" \
AUTH_ROLE_REQUIRED_APPLY_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_APPLY_READ_BODY" \
AUTH_UNAUTHENTICATED_TRANSITION_STATUS="$auth_unauthenticated_transition_status" \
AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH="$AUTH_UNAUTHENTICATED_TRANSITION_BODY" \
AUTH_FORBIDDEN_PLAN_WRITE_STATUS="$auth_forbidden_plan_write_status" \
AUTH_FORBIDDEN_PLAN_WRITE_BODY_PATH="$AUTH_FORBIDDEN_PLAN_WRITE_BODY" \
AUTH_FORBIDDEN_APPLY_WRITE_STATUS="$auth_forbidden_apply_write_status" \
AUTH_FORBIDDEN_APPLY_WRITE_BODY_PATH="$AUTH_FORBIDDEN_APPLY_WRITE_BODY" \
AUTH_FORBIDDEN_TRANSITION_STATUS="$auth_forbidden_transition_status" \
AUTH_FORBIDDEN_TRANSITION_BODY_PATH="$AUTH_FORBIDDEN_TRANSITION_BODY" \
AUTH_FORBIDDEN_READ_STATUS="$auth_forbidden_read_status" \
AUTH_FORBIDDEN_READ_BODY_PATH="$AUTH_FORBIDDEN_READ_BODY" \
AUTH_FORBIDDEN_STATUS_READ_STATUS="$auth_forbidden_status_read_status" \
AUTH_FORBIDDEN_STATUS_READ_BODY_PATH="$AUTH_FORBIDDEN_STATUS_READ_BODY" \
AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status" \
AUTH_FORBIDDEN_EVENTS_BODY_PATH="$AUTH_FORBIDDEN_EVENTS_BODY" \
AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS="$auth_forbidden_resource_logs_status" \
AUTH_FORBIDDEN_RESOURCE_LOGS_BODY_PATH="$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" \
AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS="$auth_forbidden_resource_inspection_status" \
AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY_PATH="$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" \
AUTH_FORBIDDEN_PLAN_READ_STATUS="$auth_forbidden_plan_read_status" \
AUTH_FORBIDDEN_PLAN_READ_BODY_PATH="$AUTH_FORBIDDEN_PLAN_READ_BODY" \
AUTH_FORBIDDEN_APPLY_READ_STATUS="$auth_forbidden_apply_read_status" \
AUTH_FORBIDDEN_APPLY_READ_BODY_PATH="$AUTH_FORBIDDEN_APPLY_READ_BODY" \
AUTH_FORBIDDEN_AUDIT_STATUS="$auth_forbidden_audit_status" \
AUTH_FORBIDDEN_AUDIT_BODY_PATH="$AUTH_FORBIDDEN_AUDIT_BODY" \
OPERATOR_CONTEXT_TRUSTED_PATH="$operator_context_trusted_path" \
OPERATOR_CONTEXT_MIXED_ROLES_PATH="$operator_context_mixed_roles_path" \
OPERATOR_CONTEXT_COMBINED_ACCESS_PATH="$operator_context_combined_access_path" \
OPERATOR_CONTEXT_UNMATCHED_PATH="$operator_context_unmatched_path" \
OPERATOR_CONTEXT_STALE_SCOPES_PATH="$operator_context_stale_scopes_path" \
OPERATOR_CONTEXT_ALIAS_PATH="$operator_context_alias_path" \
OPERATOR_CONTEXT_ACTOR_PRECEDENCE_PATH="$operator_context_actor_precedence_path" \
OPERATOR_CONTEXT_NO_ALLOWLIST_PATH="$operator_context_no_allowlist_path" \
OPERATOR_CONTEXT_NO_ROLES_PATH="$operator_context_no_roles_path" \
ALLOWED_GUARDED_READ_PATH="$allowed_guarded_read_path" \
ALLOWED_STATUS_READ_PATH="$allowed_status_read_path" \
ALLOWED_PLAN_READ_PATH="$allowed_plan_read_path" \
ALLOWED_APPLY_READ_PATH="$allowed_apply_read_path" \
SCOPED_WRITE_PLAN_PREVIEW_PATH="$scoped_write_plan_preview_path" \
SCOPED_WRITE_APPLY_RECORD_PATH="$scoped_write_apply_record_path" \
SCOPED_WRITE_TRANSITION_STATUS="$scoped_write_transition_status" \
SCOPED_WRITE_TRANSITION_BODY_PATH="$SCOPED_WRITE_TRANSITION_BODY" \
ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path" \
ALLOWED_RESOURCE_LOGS_PATH="$allowed_resource_logs_path" \
ALLOWED_RESOURCE_INSPECTION_PATH="$allowed_resource_inspection_path" \
FILTERED_APPLICATIONS_COLLECTION_PATH="$filtered_applications_collection_path" \
FILTERED_STATUS_COLLECTION_PATH="$filtered_status_collection_path" \
FILTERED_CATALOG_DEPLOYMENTS_PATH="$filtered_catalog_deployments_path" \
FILTERED_ACCOUNTS_COLLECTION_PATH="$filtered_accounts_collection_path" \
FILTERED_ACCOUNT_DETAIL_PATH="$filtered_account_detail_path" \
FILTERED_GROUP_DETAIL_PATH="$filtered_group_detail_path" \
FILTERED_AUDIT_COLLECTION_PATH="$filtered_audit_collection_path" \
EMPTY_APPLICATIONS_COLLECTION_PATH="$empty_applications_collection_path" \
EMPTY_STATUS_COLLECTION_PATH="$empty_status_collection_path" \
EMPTY_CATALOG_DEPLOYMENTS_PATH="$empty_catalog_deployments_path" \
EMPTY_ACCOUNTS_COLLECTION_PATH="$empty_accounts_collection_path" \
EMPTY_AUDIT_COLLECTION_PATH="$empty_audit_collection_path" \
ALLOWED_SCOPED_AUDIT_PATH="$allowed_scoped_audit_path" \
FRONTDOOR_PLAN_ID="$frontdoor_plan_id" \
FRONTDOOR_APPLY_ID="$frontdoor_apply_id" \
python3 - <<'PY'
import json, os

def read_json(path_env):
    with open(os.environ[path_env], encoding='utf-8') as fh:
        return json.load(fh)

with open(os.environ['AUTH_UNAUTHENTICATED_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_collection_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_status_collection_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_account_collection_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_catalog_collection_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_audit_collection_body = fh.read()
with open(os.environ['AUTH_ROLE_REQUIRED_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    role_required_collection_body = fh.read()
with open(os.environ['AUTH_ROLE_REQUIRED_STATUS_READ_BODY_PATH'], encoding='utf-8') as fh:
    role_required_status_read_body = fh.read()
with open(os.environ['AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY_PATH'], encoding='utf-8') as fh:
    role_required_audit_collection_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_READ_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_read_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_status_read_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_EVENTS_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_events_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_resource_logs_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_resource_inspection_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_ACCOUNT_DETAIL_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_account_detail_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_GROUP_DETAIL_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_group_detail_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_plan_read_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_apply_read_body = fh.read()
with open(os.environ['AUTH_ROLE_REQUIRED_PLAN_READ_BODY_PATH'], encoding='utf-8') as fh:
    role_required_plan_read_body = fh.read()
with open(os.environ['AUTH_ROLE_REQUIRED_APPLY_READ_BODY_PATH'], encoding='utf-8') as fh:
    role_required_apply_read_body = fh.read()
with open(os.environ['AUTH_UNAUTHENTICATED_TRANSITION_BODY_PATH'], encoding='utf-8') as fh:
    unauthenticated_transition_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_PLAN_WRITE_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_plan_write_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_APPLY_WRITE_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_apply_write_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_TRANSITION_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_transition_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_READ_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_read_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_STATUS_READ_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_status_read_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_EVENTS_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_events_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_resource_logs_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_resource_inspection_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_PLAN_READ_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_plan_read_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_APPLY_READ_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_apply_read_body = fh.read()
with open(os.environ['AUTH_FORBIDDEN_AUDIT_BODY_PATH'], encoding='utf-8') as fh:
    forbidden_audit_body = fh.read()
with open(os.environ['OPERATOR_CONTEXT_TRUSTED_PATH'], encoding='utf-8') as fh:
    operator_context_trusted = json.load(fh)
operator_context_mixed_roles = read_json('OPERATOR_CONTEXT_MIXED_ROLES_PATH')
with open(os.environ['OPERATOR_CONTEXT_COMBINED_ACCESS_PATH'], encoding='utf-8') as fh:
    operator_context_combined_access = json.load(fh)
with open(os.environ['OPERATOR_CONTEXT_UNMATCHED_PATH'], encoding='utf-8') as fh:
    operator_context_unmatched = json.load(fh)
operator_context_stale_scopes = read_json('OPERATOR_CONTEXT_STALE_SCOPES_PATH')
with open(os.environ['OPERATOR_CONTEXT_ALIAS_PATH'], encoding='utf-8') as fh:
    operator_context_alias = json.load(fh)
with open(os.environ['OPERATOR_CONTEXT_ACTOR_PRECEDENCE_PATH'], encoding='utf-8') as fh:
    operator_context_actor_precedence = json.load(fh)
with open(os.environ['OPERATOR_CONTEXT_NO_ALLOWLIST_PATH'], encoding='utf-8') as fh:
    operator_context_no_allowlist = json.load(fh)
with open(os.environ['OPERATOR_CONTEXT_NO_ROLES_PATH'], encoding='utf-8') as fh:
    operator_context_no_roles = json.load(fh)
with open(os.environ['ALLOWED_GUARDED_READ_PATH'], encoding='utf-8') as fh:
    allowed_guarded_read = json.load(fh)
with open(os.environ['ALLOWED_STATUS_READ_PATH'], encoding='utf-8') as fh:
    allowed_status_read = json.load(fh)
with open(os.environ['ALLOWED_PLAN_READ_PATH'], encoding='utf-8') as fh:
    allowed_plan_read = json.load(fh)
with open(os.environ['ALLOWED_APPLY_READ_PATH'], encoding='utf-8') as fh:
    allowed_apply_read = json.load(fh)
with open(os.environ['SCOPED_WRITE_PLAN_PREVIEW_PATH'], encoding='utf-8') as fh:
    scoped_write_plan_preview = json.load(fh)
with open(os.environ['SCOPED_WRITE_APPLY_RECORD_PATH'], encoding='utf-8') as fh:
    scoped_write_apply_record = json.load(fh)
with open(os.environ['SCOPED_WRITE_TRANSITION_BODY_PATH'], encoding='utf-8') as fh:
    scoped_write_transition_body = fh.read()
with open(os.environ['ALLOWED_EVENTS_READ_PATH'], encoding='utf-8') as fh:
    allowed_events_read = json.load(fh)
with open(os.environ['ALLOWED_RESOURCE_LOGS_PATH'], encoding='utf-8') as fh:
    allowed_resource_logs = json.load(fh)
with open(os.environ['ALLOWED_RESOURCE_INSPECTION_PATH'], encoding='utf-8') as fh:
    allowed_resource_inspection = json.load(fh)
with open(os.environ['FILTERED_APPLICATIONS_COLLECTION_PATH'], encoding='utf-8') as fh:
    filtered_applications_collection = json.load(fh)
with open(os.environ['FILTERED_STATUS_COLLECTION_PATH'], encoding='utf-8') as fh:
    filtered_status_collection = json.load(fh)
with open(os.environ['FILTERED_CATALOG_DEPLOYMENTS_PATH'], encoding='utf-8') as fh:
    filtered_catalog_deployments = json.load(fh)
with open(os.environ['FILTERED_ACCOUNTS_COLLECTION_PATH'], encoding='utf-8') as fh:
    filtered_accounts_collection = json.load(fh)
with open(os.environ['FILTERED_ACCOUNT_DETAIL_PATH'], encoding='utf-8') as fh:
    filtered_account_detail = json.load(fh)
with open(os.environ['FILTERED_GROUP_DETAIL_PATH'], encoding='utf-8') as fh:
    filtered_group_detail = json.load(fh)
with open(os.environ['FILTERED_AUDIT_COLLECTION_PATH'], encoding='utf-8') as fh:
    filtered_audit_collection = json.load(fh)
with open(os.environ['EMPTY_APPLICATIONS_COLLECTION_PATH'], encoding='utf-8') as fh:
    empty_applications_collection = json.load(fh)
with open(os.environ['EMPTY_STATUS_COLLECTION_PATH'], encoding='utf-8') as fh:
    empty_status_collection = json.load(fh)
with open(os.environ['EMPTY_CATALOG_DEPLOYMENTS_PATH'], encoding='utf-8') as fh:
    empty_catalog_deployments = json.load(fh)
with open(os.environ['EMPTY_ACCOUNTS_COLLECTION_PATH'], encoding='utf-8') as fh:
    empty_accounts_collection = json.load(fh)
with open(os.environ['EMPTY_AUDIT_COLLECTION_PATH'], encoding='utf-8') as fh:
    empty_audit_collection = json.load(fh)
with open(os.environ['ALLOWED_SCOPED_AUDIT_PATH'], encoding='utf-8') as fh:
    allowed_scoped_audit = json.load(fh)

assert os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS']
assert unauthenticated_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_collection_body
assert os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS']
assert unauthenticated_status_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_collection_body
assert os.environ['AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_ACCOUNT_COLLECTION_STATUS']
assert unauthenticated_account_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_account_collection_body
assert os.environ['AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_CATALOG_COLLECTION_STATUS']
assert unauthenticated_catalog_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_catalog_collection_body
assert os.environ['AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_STATUS']
assert unauthenticated_audit_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_audit_collection_body
assert os.environ['AUTH_ROLE_REQUIRED_COLLECTION_STATUS'] == '403', os.environ['AUTH_ROLE_REQUIRED_COLLECTION_STATUS']
assert role_required_collection_body.strip() == 'forbidden: authenticated viewer-capable role is required for this operator read route', role_required_collection_body
assert os.environ['AUTH_ROLE_REQUIRED_STATUS_READ_STATUS'] == '403', os.environ['AUTH_ROLE_REQUIRED_STATUS_READ_STATUS']
assert role_required_status_read_body.strip() == 'forbidden: authenticated viewer-capable role is required for this operator read route', role_required_status_read_body
assert os.environ['AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS'] == '403', os.environ['AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS']
assert role_required_audit_collection_body.strip() == 'forbidden: authenticated viewer-capable role is required for this operator read route', role_required_audit_collection_body
assert os.environ['AUTH_UNAUTHENTICATED_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_READ_STATUS']
assert unauthenticated_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_read_body
assert os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS']
assert unauthenticated_status_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_read_body
assert os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS']
assert unauthenticated_events_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_events_body
assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS']
assert unauthenticated_resource_logs_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_resource_logs_body
assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS']
assert unauthenticated_resource_inspection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_resource_inspection_body
assert os.environ['AUTH_FORBIDDEN_ACCOUNT_DETAIL_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_ACCOUNT_DETAIL_STATUS']
assert forbidden_account_detail_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_account_detail_body
assert os.environ['AUTH_FORBIDDEN_GROUP_DETAIL_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_GROUP_DETAIL_STATUS']
assert forbidden_group_detail_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_group_detail_body
assert os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS']
assert unauthenticated_plan_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_plan_read_body
assert os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS']
assert unauthenticated_apply_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_apply_read_body
assert os.environ['AUTH_ROLE_REQUIRED_PLAN_READ_STATUS'] == '403', os.environ['AUTH_ROLE_REQUIRED_PLAN_READ_STATUS']
assert role_required_plan_read_body.strip() == 'forbidden: authenticated viewer-capable role is required for this operator read route', role_required_plan_read_body
assert os.environ['AUTH_ROLE_REQUIRED_APPLY_READ_STATUS'] == '403', os.environ['AUTH_ROLE_REQUIRED_APPLY_READ_STATUS']
assert role_required_apply_read_body.strip() == 'forbidden: authenticated viewer-capable role is required for this operator read route', role_required_apply_read_body
assert os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS']
assert unauthenticated_transition_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_transition_body
assert os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS']
assert forbidden_plan_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_plan_write_body
assert os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS']
assert forbidden_apply_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_apply_write_body
assert os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS']
assert forbidden_transition_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_transition_body
assert os.environ['AUTH_FORBIDDEN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_READ_STATUS']
assert forbidden_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_read_body
assert os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS']
assert forbidden_status_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_status_read_body
assert os.environ['AUTH_FORBIDDEN_EVENTS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_EVENTS_STATUS']
assert forbidden_events_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_events_body
assert os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS']
assert forbidden_resource_logs_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_resource_logs_body
assert os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS']
assert forbidden_resource_inspection_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_resource_inspection_body
assert os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS']
assert forbidden_plan_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_plan_read_body
assert os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS']
assert forbidden_apply_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_apply_read_body

def assert_operator_context_project_environment_examples(examples, payload):
    assert 1 <= len(examples) <= 5, payload
    for row in examples:
        assert isinstance(row.get('appRef'), str) and row['appRef'], payload
        assert isinstance(row.get('environmentRef'), str) and row['environmentRef'], payload
        assert isinstance(row.get('region'), str) and row['region'], payload
        assert isinstance(row.get('awsAccountId'), str) and row['awsAccountId'], payload
        assert isinstance(row.get('deploymentCount'), int) and row['deploymentCount'] > 0, payload
        example_deployments = row.get('exampleDeployments') or []
        assert len(example_deployments) <= row['deploymentCount'], payload
        assert all(isinstance(name, str) and name for name in example_deployments), payload


def assert_operator_context_common(payload):
    trusted = payload.get('trustedHeaders') or {}
    assert trusted.get('enabled') is True, payload
    assert trusted.get('authenticatedIdentityRequired') is True, payload
    assert trusted.get('deploymentReadAuthorizationEnforced') is True, payload
    assert trusted.get('operatorReadRoleEnforced') is True, payload
    assert trusted.get('operatorWriteRoleEnforced') is True, payload
    enforcement = payload.get('enforcement') or {}
    assert enforcement.get('mode') == 'trusted_front_door_read_write', payload
    assert enforcement.get('trustedHeadersEnabled') is True, payload
    assert enforcement.get('readRoutesGuarded') is True, payload
    assert enforcement.get('writeRoutesGuarded') is True, payload
    assert enforcement.get('deploymentScopedRoutesRequireAllowlist') is True, payload
    assert enforcement.get('collectionReadsFiltered') is True, payload
    access_verdicts = payload.get('accessVerdicts') or {}
    identity_headers = ['X-Tenet-Authenticated-Id', 'X-User-ID']
    canonical_identity_headers = ['X-Tenet-Authenticated-Id', 'X-Tenet-Authenticated-Name', 'X-Tenet-Authenticated-Email']
    alias_identity_headers = ['X-User-ID', 'X-User-Name', 'X-User-Email']
    canonical_actor_headers = ['X-Tenet-Actor-Id', 'X-Tenet-Actor-Name', 'X-Tenet-Actor-Email']
    alias_actor_headers = ['X-User-ID', 'X-User-Name', 'X-User-Email']
    role_headers = ['X-Tenet-Authenticated-Roles']
    read_roles = ['viewer', 'operator', 'admin']
    write_roles = ['operator', 'admin']
    access_headers = ['X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments']
    allowlist_headers = ['X-Tenet-Authorized-Deployments']
    managed_login_required_settings = [
        'TENET_AUTH_PROVIDER',
        'TENET_GITHUB_OAUTH_CLIENT_ID',
        'TENET_GITHUB_OAUTH_CLIENT_SECRET',
        'TENET_GITHUB_OAUTH_REDIRECT_URL',
        'TENET_AUTH_STATE_STORE_PATH',
        'TENET_AUTH_IDENTITY_STORE_PATH',
        'TENET_AUTH_SESSION_SECRET',
    ]
    auth_runtime = payload.get('authRuntime') or {}
    tenet_managed_login = auth_runtime.get('tenetManagedLogin') or {}
    assert tenet_managed_login.get('configured') is False, payload
    assert tenet_managed_login.get('active') is False, payload
    assert tenet_managed_login.get('provider') == '', payload
    assert tenet_managed_login.get('unavailableReason') == 'tenet-managed login is disabled because OAuth runtime configuration is incomplete or absent', payload
    assert tenet_managed_login.get('oauthStateTtlSeconds') == 600, payload
    assert tenet_managed_login.get('sessionTtlSeconds') == 43200, payload
    assert tenet_managed_login.get('sessionExpiryEnforced') is True, payload
    assert tenet_managed_login.get('sessionCookiePath') == '/', payload
    assert tenet_managed_login.get('sessionCookieHttpOnly') is True, payload
    assert tenet_managed_login.get('sessionCookieSameSite') == 'Lax', payload
    assert tenet_managed_login.get('sessionCookieSecureOverride') is False, payload
    assert tenet_managed_login.get('sessionCookieSecureForRequest') is False, payload
    assert tenet_managed_login.get('providerAccessTokenStored') is False, payload
    assert tenet_managed_login.get('providerAccessTokenReturned') is False, payload
    assert tenet_managed_login.get('providerTokenRefreshSupported') is False, payload
    assert tenet_managed_login.get('providerTokenRevocationSupported') is False, payload
    assert tenet_managed_login.get('logoutClearsSessionCookie') is False, payload
    assert tenet_managed_login.get('logoutRevokesProviderToken') is False, payload
    assert tenet_managed_login.get('requiredSettings') == managed_login_required_settings, payload
    assert tenet_managed_login.get('missingSettings') == managed_login_required_settings, payload
    assert 'loginRoute' not in tenet_managed_login, payload
    assert 'callbackRoute' not in tenet_managed_login, payload
    identity_header_support = payload.get('identityHeaderSupport') or {}
    actor_header_support = payload.get('actorHeaderSupport') or {}
    assert identity_header_support.get('canonical') == canonical_identity_headers, payload
    assert identity_header_support.get('aliases') == alias_identity_headers, payload
    assert actor_header_support.get('canonical') == canonical_actor_headers, payload
    assert actor_header_support.get('aliases') == alias_actor_headers, payload
    assert (payload.get('deploymentAccess') or {}).get('supportedHeaderNames') == access_headers, payload
    assert access_verdicts.get('operatorRead', {}).get('requiresAuthenticatedIdentity') is True, payload
    assert access_verdicts.get('operatorRead', {}).get('requiresViewerCapableRole') is True, payload
    assert access_verdicts.get('operatorRead', {}).get('exampleRoutes') == ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/audit-logs'], payload
    assert access_verdicts.get('operatorRead', {}).get('requiredTrustedHeaders') == identity_headers + role_headers, payload
    assert access_verdicts.get('operatorRead', {}).get('acceptedRoleNames') == read_roles, payload
    assert access_verdicts.get('operatorWrite', {}).get('requiresOperatorRole') is True, payload
    assert access_verdicts.get('operatorWrite', {}).get('exampleRoutes') == ['POST /v1/plans/deployments/{deploymentName}', 'POST /v1/applies/plans/{planId}', 'POST /v1/applies/{applyId}/transitions'], payload
    assert access_verdicts.get('operatorWrite', {}).get('requiredTrustedHeaders') == identity_headers + role_headers, payload
    assert access_verdicts.get('operatorWrite', {}).get('acceptedRoleNames') == write_roles, payload
    assert access_verdicts.get('deploymentScopedRead', {}).get('requiresExactDeploymentAllowlistMatch') is True, payload
    assert access_verdicts.get('deploymentScopedRead', {}).get('acceptsProjectEnvironmentScopes') is True, payload
    assert access_verdicts.get('deploymentScopedRead', {}).get('exampleRoutes') == ['GET /v1/overview/deployments/{deploymentName}', 'GET /v1/status/deployments/{deploymentName}', 'GET /v1/audit-logs?deployment={deploymentName}'], payload
    assert access_verdicts.get('deploymentScopedRead', {}).get('requiredTrustedHeaders') == identity_headers + role_headers + access_headers, payload
    assert access_verdicts.get('deploymentScopedRead', {}).get('acceptedRoleNames') == read_roles, payload
    assert access_verdicts.get('deploymentScopedWrite', {}).get('requiresExactDeploymentAllowlistMatch') is True, payload
    assert access_verdicts.get('deploymentScopedWrite', {}).get('acceptsProjectEnvironmentScopes') is True, payload
    assert access_verdicts.get('deploymentScopedWrite', {}).get('requiredTrustedHeaders') == identity_headers + role_headers + access_headers, payload
    assert access_verdicts.get('deploymentScopedWrite', {}).get('acceptedRoleNames') == write_roles, payload
    assert access_verdicts.get('collectionRead', {}).get('filtersCollectionsByDeploymentAccess') is True, payload
    assert access_verdicts.get('collectionRead', {}).get('exampleRoutes') == ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/overview/accounts', 'GET /v1/audit-logs'], payload
    assert access_verdicts.get('collectionRead', {}).get('requiredTrustedHeaders') == identity_headers + role_headers + access_headers, payload
    assert access_verdicts.get('collectionRead', {}).get('acceptedRoleNames') == read_roles, payload
    assert access_verdicts.get('catalogRead', {}).get('filtersCollectionsByDeploymentAccess') is True, payload
    assert access_verdicts.get('catalogRead', {}).get('exampleRoutes') == ['GET /v1/catalog/deployments', 'GET /v1/catalog/accounts', 'GET /v1/catalog/placements', 'GET /v1/catalog/topology', 'GET /v1/catalog/summary'], payload
    assert access_verdicts.get('catalogRead', {}).get('requiredTrustedHeaders') == identity_headers + role_headers + access_headers, payload
    assert access_verdicts.get('catalogRead', {}).get('acceptedRoleNames') == read_roles, payload
    desired_state_source = payload.get('desiredStateSource') or {}
    assert desired_state_source.get('configured') is True, payload
    assert desired_state_source.get('kind') == 'filesystem', payload
    assert (desired_state_source.get('filesystem') or {}).get('mode') == 'local_repository', payload
    assert 'github' not in desired_state_source, payload
    source_refresh = desired_state_source.get('controllerRefresh') or {}
    assert source_refresh.get('configured') is False, payload
    assert source_refresh.get('available') is False, payload
    assert source_refresh.get('status') == 'filesystem_noop', payload
    assert source_refresh.get('diagnosticOnly') is True, payload
    assert 'filesystem catalog' in source_refresh.get('reason', ''), payload
    assert 'matched' in (desired_state_source.get('webhookMatchStatuses') or []), payload
    assert 'filesystem_source' in (desired_state_source.get('webhookProcessedFalseReasons') or []), payload

assert_operator_context_common(operator_context_trusted)
assert operator_context_trusted['identity']['authenticated'] is True, operator_context_trusted
assert operator_context_trusted['identity']['id'] == 'front-door-123', operator_context_trusted
assert operator_context_trusted['identity']['name'] == 'Front Door Operator', operator_context_trusted
assert operator_context_trusted['identity']['email'] == 'frontdoor@example.com', operator_context_trusted
assert operator_context_trusted['identityProvenance'] == {'idSource': 'canonical', 'nameSource': 'canonical', 'emailSource': 'canonical'}, operator_context_trusted
assert operator_context_trusted['actor']['available'] is True, operator_context_trusted
assert operator_context_trusted['actor']['id'] == 'front-door-123', operator_context_trusted
assert operator_context_trusted['actor']['name'] == 'Front Door Operator', operator_context_trusted
assert operator_context_trusted['actor']['email'] == 'frontdoor@example.com', operator_context_trusted
assert operator_context_trusted['actorProvenance'] == {'idSource': 'absent', 'nameSource': 'absent', 'emailSource': 'absent'}, operator_context_trusted
assert operator_context_trusted['actorAttribution']['available'] is True, operator_context_trusted
assert operator_context_trusted['actorAttribution']['source'] == 'trusted_authenticated_identity', operator_context_trusted
actor_precedence = ['explicit_actor_headers', 'actor_alias_headers', 'trusted_authenticated_identity', 'unavailable']
assert operator_context_trusted['actorAttribution']['precedence'] == actor_precedence, operator_context_trusted
assert 'trusted authenticated identity' in operator_context_trusted['actorAttribution']['summary'], operator_context_trusted
assert operator_context_trusted['roles']['names'] == ['viewer', 'operator'], operator_context_trusted
assert operator_context_trusted['roles']['canRead'] is True, operator_context_trusted
assert operator_context_trusted['roles']['canWrite'] is True, operator_context_trusted
assert operator_context_trusted['roles']['provenance']['source'] == 'canonical', operator_context_trusted
assert operator_context_trusted['roles']['supportedNames'] == ['viewer', 'operator', 'admin'], operator_context_trusted
assert operator_context_trusted['roles']['supportedHeaderNames'] == ['X-Tenet-Authenticated-Roles'], operator_context_trusted
assert operator_context_trusted['roles']['recognizedNames'] == ['viewer', 'operator'], operator_context_trusted
assert operator_context_trusted['roles']['unrecognizedNames'] == [], operator_context_trusted
assert operator_context_trusted['deploymentAccess']['enforced'] is True, operator_context_trusted
assert operator_context_trusted['deploymentAccess']['forwarded'] is True, operator_context_trusted
assert operator_context_trusted['deploymentAccess']['names'] == ['payments-api-prod', 'worker-prod'], operator_context_trusted
trusted_summary = operator_context_trusted['deploymentAccess']['summary']
assert trusted_summary['available'] is True, operator_context_trusted
assert trusted_summary['forwardedCount'] == 2, operator_context_trusted
assert trusted_summary['matchedCount'] == 2, operator_context_trusted
assert trusted_summary['exactDeploymentMatchedCount'] == 2, operator_context_trusted
assert trusted_summary.get('projectEnvironmentMatchedCount', 0) == 0, operator_context_trusted
assert trusted_summary.get('unmatchedNames', []) == [], operator_context_trusted
assert trusted_summary['accountCount'] >= 1, operator_context_trusted
assert trusted_summary['projectCount'] >= 1, operator_context_trusted
assert trusted_summary['environmentCount'] >= 1, operator_context_trusted
assert trusted_summary.get('emptyReason') is None, operator_context_trusted
assert trusted_summary['visibleDeploymentExamples'], operator_context_trusted
trusted_visible = {row['deploymentName']: row for row in trusted_summary['visibleDeploymentExamples']}
assert trusted_visible['payments-api-prod']['matchSources'] == ['exactDeployment'], operator_context_trusted
assert trusted_visible['payments-api-prod']['appRef'] == 'payments-api', operator_context_trusted
assert trusted_visible['payments-api-prod']['environmentRef'] == 'prod-apne2', operator_context_trusted
assert trusted_visible['payments-api-prod']['awsAccountId'], operator_context_trusted
assert trusted_summary['projectEnvironmentExamples'], operator_context_trusted
assert_operator_context_project_environment_examples(trusted_summary['projectEnvironmentExamples'], operator_context_trusted)
trusted_project_envs = {(row['appRef'], row['environmentRef']) for row in trusted_summary['projectEnvironmentExamples']}
assert ('payments-api', 'prod-apne2') in trusted_project_envs or ('worker', 'prod-apne2') in trusted_project_envs, operator_context_trusted
trusted_verdicts = operator_context_trusted['accessVerdicts']
assert trusted_verdicts['operatorRead']['allowed'] is True, operator_context_trusted
assert trusted_verdicts['operatorWrite']['allowed'] is True, operator_context_trusted
assert trusted_verdicts['deploymentScopedRead']['reason'] == 'deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments', operator_context_trusted
assert trusted_verdicts['collectionRead']['allowed'] is True, operator_context_trusted
assert_operator_context_common(operator_context_mixed_roles)
assert operator_context_mixed_roles['identity']['authenticated'] is True, operator_context_mixed_roles
assert operator_context_mixed_roles['identity']['id'] == 'mixed-role-123', operator_context_mixed_roles
assert operator_context_mixed_roles['identity']['name'] == 'Mixed Role Operator', operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['names'] == ['viewer', 'auditor'], operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['canRead'] is True, operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['canWrite'] is False, operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['provenance']['source'] == 'canonical', operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['recognizedNames'] == ['viewer'], operator_context_mixed_roles
assert operator_context_mixed_roles['roles']['unrecognizedNames'] == ['auditor'], operator_context_mixed_roles
assert operator_context_mixed_roles['accessVerdicts']['operatorRead']['allowed'] is True, operator_context_mixed_roles
assert operator_context_mixed_roles['accessVerdicts']['operatorWrite']['allowed'] is False, operator_context_mixed_roles
assert operator_context_mixed_roles['accessVerdicts']['operatorWrite']['reason'] == 'authenticated operator role is required for guarded operator write routes', operator_context_mixed_roles
assert operator_context_mixed_roles['deploymentAccess']['names'] == ['payments-api-prod'], operator_context_mixed_roles
assert operator_context_mixed_roles['deploymentAccess']['summary']['matchedCount'] == 1, operator_context_mixed_roles
assert operator_context_mixed_roles['deploymentAccess']['summary']['exactDeploymentMatchedCount'] == 1, operator_context_mixed_roles
assert_operator_context_common(operator_context_combined_access)
assert operator_context_combined_access['deploymentAccess']['enforced'] is True, operator_context_combined_access
assert operator_context_combined_access['deploymentAccess']['forwarded'] is True, operator_context_combined_access
assert operator_context_combined_access['deploymentAccess']['names'] == ['payments-api-prod', 'worker-prod'], operator_context_combined_access
combined_scopes = operator_context_combined_access['deploymentAccess']['scopes']
assert combined_scopes['forwarded'] is True, operator_context_combined_access
assert combined_scopes['projects'] == ['payments-api'], operator_context_combined_access
assert combined_scopes['environments'] == ['prod-apne2'], operator_context_combined_access
combined_summary = operator_context_combined_access['deploymentAccess']['summary']
assert combined_summary['available'] is True, operator_context_combined_access
assert combined_summary['forwardedCount'] == 2, operator_context_combined_access
assert combined_summary['forwardedProjectCount'] == 1, operator_context_combined_access
assert combined_summary['forwardedEnvironmentCount'] == 1, operator_context_combined_access
assert combined_summary['matchedCount'] >= 2, operator_context_combined_access
assert combined_summary['exactDeploymentMatchedCount'] >= 2, operator_context_combined_access
assert combined_summary['projectEnvironmentMatchedCount'] >= 1, operator_context_combined_access
assert combined_summary.get('unmatchedNames', []) == [], operator_context_combined_access
assert combined_summary.get('unmatchedProjects', []) == [], operator_context_combined_access
assert combined_summary.get('unmatchedEnvironments', []) == [], operator_context_combined_access
combined_visible = {row['deploymentName']: row for row in combined_summary['visibleDeploymentExamples']}
combined_payment = combined_visible['payments-api-prod']
assert combined_payment['deploymentName'] == 'payments-api-prod', operator_context_combined_access
assert combined_payment['appRef'] == 'payments-api', operator_context_combined_access
assert combined_payment['environmentRef'] == 'prod-apne2', operator_context_combined_access
assert combined_payment['region'], operator_context_combined_access
assert combined_payment['awsAccountId'], operator_context_combined_access
assert combined_payment['matchSources'] == ['exactDeployment', 'projectEnvironment'], operator_context_combined_access
assert_operator_context_project_environment_examples(combined_summary['projectEnvironmentExamples'], operator_context_combined_access)
assert_operator_context_common(operator_context_unmatched)
assert operator_context_unmatched['deploymentAccess']['enforced'] is True, operator_context_unmatched
assert operator_context_unmatched['deploymentAccess']['forwarded'] is True, operator_context_unmatched
assert operator_context_unmatched['deploymentAccess']['names'] == ['payments-api-prod', 'stale-api', 'worker-prod'], operator_context_unmatched
unmatched_summary = operator_context_unmatched['deploymentAccess']['summary']
assert unmatched_summary['available'] is True, operator_context_unmatched
assert unmatched_summary['forwardedCount'] == 3, operator_context_unmatched
assert unmatched_summary['matchedCount'] == 2, operator_context_unmatched
assert unmatched_summary['exactDeploymentMatchedCount'] == 2, operator_context_unmatched
assert unmatched_summary['unmatchedNames'] == ['stale-api'], operator_context_unmatched
assert unmatched_summary['accountCount'] >= 1, operator_context_unmatched
assert unmatched_summary['projectCount'] >= 1, operator_context_unmatched
assert unmatched_summary['environmentCount'] >= 1, operator_context_unmatched
assert unmatched_summary.get('emptyReason') is None, operator_context_unmatched
assert unmatched_summary['visibleDeploymentExamples'], operator_context_unmatched
unmatched_visible = {row['deploymentName']: row for row in unmatched_summary['visibleDeploymentExamples']}
assert 'stale-api' not in unmatched_visible, operator_context_unmatched
assert unmatched_visible['payments-api-prod']['matchSources'] == ['exactDeployment'], operator_context_unmatched
assert unmatched_summary['projectEnvironmentExamples'], operator_context_unmatched
assert_operator_context_common(operator_context_stale_scopes)
assert operator_context_stale_scopes['deploymentAccess']['enforced'] is True, operator_context_stale_scopes
assert operator_context_stale_scopes['deploymentAccess']['forwarded'] is True, operator_context_stale_scopes
assert 'names' not in operator_context_stale_scopes['deploymentAccess'], operator_context_stale_scopes
stale_scopes = operator_context_stale_scopes['deploymentAccess']['scopes']
assert stale_scopes['forwarded'] is True, operator_context_stale_scopes
assert stale_scopes['projects'] == ['payments-api', 'archived-payments'], operator_context_stale_scopes
assert stale_scopes['environments'] == ['staging-apne2', 'stale-env'], operator_context_stale_scopes
stale_scopes_summary = operator_context_stale_scopes['deploymentAccess']['summary']
assert stale_scopes_summary['available'] is True, operator_context_stale_scopes
assert stale_scopes_summary['forwardedCount'] == 0, operator_context_stale_scopes
assert stale_scopes_summary['forwardedProjectCount'] == 2, operator_context_stale_scopes
assert stale_scopes_summary['forwardedEnvironmentCount'] == 2, operator_context_stale_scopes
assert stale_scopes_summary['matchedCount'] == 1, operator_context_stale_scopes
assert stale_scopes_summary['projectEnvironmentMatchedCount'] == 1, operator_context_stale_scopes
assert stale_scopes_summary.get('exactDeploymentMatchedCount', 0) == 0, operator_context_stale_scopes
assert stale_scopes_summary.get('unmatchedNames', []) == [], operator_context_stale_scopes
assert stale_scopes_summary['unmatchedProjects'] == ['archived-payments'], operator_context_stale_scopes
assert stale_scopes_summary['unmatchedProjectCount'] == 1, operator_context_stale_scopes
assert stale_scopes_summary['unmatchedEnvironments'] == ['stale-env'], operator_context_stale_scopes
assert stale_scopes_summary['unmatchedEnvironmentCount'] == 1, operator_context_stale_scopes
assert stale_scopes_summary['visibleDeploymentExamples'], operator_context_stale_scopes
assert stale_scopes_summary['visibleDeploymentExamples'][0]['deploymentName'] == 'payments-api-staging', operator_context_stale_scopes
assert stale_scopes_summary['visibleDeploymentExamples'][0]['matchSources'] == ['projectEnvironment'], operator_context_stale_scopes
assert stale_scopes_summary['projectEnvironmentExamples'], operator_context_stale_scopes
stale_scope_project_envs = {(row['appRef'], row['environmentRef']) for row in stale_scopes_summary['projectEnvironmentExamples']}
assert stale_scope_project_envs == {('payments-api', 'staging-apne2')}, operator_context_stale_scopes
assert operator_context_stale_scopes['accessVerdicts']['deploymentScopedRead']['allowed'] is True, operator_context_stale_scopes
assert operator_context_stale_scopes['accessVerdicts']['collectionRead']['allowed'] is True, operator_context_stale_scopes
assert_operator_context_common(operator_context_alias)
assert operator_context_alias['identity']['authenticated'] is True, operator_context_alias
assert operator_context_alias['identity']['id'] == 'alias-456', operator_context_alias
assert operator_context_alias['identity']['name'] == 'Alias Operator', operator_context_alias
assert operator_context_alias['identity']['email'] == 'alias@example.com', operator_context_alias
assert operator_context_alias['identityProvenance'] == {'idSource': 'alias', 'nameSource': 'alias', 'emailSource': 'alias'}, operator_context_alias
assert operator_context_alias['actor']['available'] is True, operator_context_alias
assert operator_context_alias['actor']['id'] == 'alias-456', operator_context_alias
assert operator_context_alias['actor']['name'] == 'Alias Operator', operator_context_alias
assert operator_context_alias['actor']['email'] == 'alias@example.com', operator_context_alias
assert operator_context_alias['actorProvenance'] == {'idSource': 'alias', 'nameSource': 'alias', 'emailSource': 'alias'}, operator_context_alias
assert operator_context_alias['actorAttribution']['available'] is True, operator_context_alias
assert operator_context_alias['actorAttribution']['source'] == 'explicit_actor_headers', operator_context_alias
assert operator_context_alias['actorAttribution']['precedence'] == actor_precedence, operator_context_alias
assert 'explicit X-Tenet-Actor-* headers' in operator_context_alias['actorAttribution']['summary'], operator_context_alias
assert_operator_context_common(operator_context_actor_precedence)
assert operator_context_actor_precedence['identity']['id'] == 'front-door-actor-789', operator_context_actor_precedence
assert operator_context_actor_precedence['identity']['name'] == 'Front Door Actor', operator_context_actor_precedence
assert operator_context_actor_precedence['identity']['email'] == 'frontdoor.actor@example.com', operator_context_actor_precedence
assert operator_context_actor_precedence['identityProvenance'] == {'idSource': 'canonical', 'nameSource': 'canonical', 'emailSource': 'canonical'}, operator_context_actor_precedence
assert operator_context_actor_precedence['actor']['available'] is True, operator_context_actor_precedence
assert operator_context_actor_precedence['actor']['id'] == 'explicit-789', operator_context_actor_precedence
assert operator_context_actor_precedence['actor']['name'] == 'Explicit Actor', operator_context_actor_precedence
assert operator_context_actor_precedence['actor']['email'] == 'explicit.actor@example.com', operator_context_actor_precedence
assert operator_context_actor_precedence['actorProvenance'] == {'idSource': 'canonical', 'nameSource': 'canonical', 'emailSource': 'canonical'}, operator_context_actor_precedence
assert operator_context_actor_precedence['actorAttribution']['available'] is True, operator_context_actor_precedence
assert operator_context_actor_precedence['actorAttribution']['source'] == 'explicit_actor_headers', operator_context_actor_precedence
assert operator_context_actor_precedence['actorAttribution']['precedence'] == actor_precedence, operator_context_actor_precedence
assert 'explicit X-Tenet-Actor-* headers' in operator_context_actor_precedence['actorAttribution']['summary'], operator_context_actor_precedence
assert operator_context_alias['roles']['names'] == ['viewer'], operator_context_alias
assert operator_context_alias['roles']['canRead'] is True, operator_context_alias
assert operator_context_alias['roles']['canWrite'] is False, operator_context_alias
assert operator_context_alias['roles']['provenance']['source'] == 'canonical', operator_context_alias
assert operator_context_alias['roles']['supportedNames'] == ['viewer', 'operator', 'admin'], operator_context_alias
assert operator_context_alias['roles']['recognizedNames'] == ['viewer'], operator_context_alias
assert operator_context_alias['roles']['unrecognizedNames'] == [], operator_context_alias
assert operator_context_alias['deploymentAccess']['enforced'] is True, operator_context_alias
assert operator_context_alias['deploymentAccess']['forwarded'] is False, operator_context_alias
assert 'names' not in operator_context_alias['deploymentAccess'], operator_context_alias
assert_operator_context_common(operator_context_no_allowlist)
assert operator_context_no_allowlist['identity']['authenticated'] is True, operator_context_no_allowlist
assert operator_context_no_allowlist['identity']['id'] == 'no-allowlist-789', operator_context_no_allowlist
assert operator_context_no_allowlist['identity']['name'] == 'No Allowlist Operator', operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['names'] == ['viewer'], operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['canRead'] is True, operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['canWrite'] is False, operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['provenance']['source'] == 'canonical', operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['supportedNames'] == ['viewer', 'operator', 'admin'], operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['recognizedNames'] == ['viewer'], operator_context_no_allowlist
assert operator_context_no_allowlist['roles']['unrecognizedNames'] == [], operator_context_no_allowlist
assert operator_context_no_allowlist['deploymentAccess']['enforced'] is True, operator_context_no_allowlist
assert operator_context_no_allowlist['deploymentAccess']['forwarded'] is False, operator_context_no_allowlist
assert 'names' not in operator_context_no_allowlist['deploymentAccess'], operator_context_no_allowlist
no_allowlist_verdicts = operator_context_no_allowlist['accessVerdicts']
assert no_allowlist_verdicts['deploymentScopedRead']['allowed'] is False, operator_context_no_allowlist
assert no_allowlist_verdicts['collectionRead']['allowed'] is True, operator_context_no_allowlist
assert no_allowlist_verdicts['collectionRead']['emptyCollectionLikely'] is True, operator_context_no_allowlist

assert_operator_context_common(operator_context_no_roles)
assert operator_context_no_roles['identity']['authenticated'] is True, operator_context_no_roles
assert operator_context_no_roles['identity']['id'] == 'no-roles-321', operator_context_no_roles
assert operator_context_no_roles['identity']['name'] == 'Missing Role Operator', operator_context_no_roles
assert operator_context_no_roles['identityProvenance'] == {'idSource': 'canonical', 'nameSource': 'canonical', 'emailSource': 'absent'}, operator_context_no_roles
assert operator_context_no_roles['roles'].get('names', []) == [], operator_context_no_roles
assert operator_context_no_roles['roles']['canRead'] is False, operator_context_no_roles
assert operator_context_no_roles['roles']['canWrite'] is False, operator_context_no_roles
assert operator_context_no_roles['roles']['provenance']['source'] == 'absent', operator_context_no_roles
assert operator_context_no_roles['roles']['supportedNames'] == ['viewer', 'operator', 'admin'], operator_context_no_roles
assert operator_context_no_roles['roles']['recognizedNames'] == [], operator_context_no_roles
assert operator_context_no_roles['roles']['unrecognizedNames'] == [], operator_context_no_roles
no_roles_verdicts = operator_context_no_roles['accessVerdicts']
assert no_roles_verdicts['operatorRead']['allowed'] is False, operator_context_no_roles
assert no_roles_verdicts['operatorRead']['reason'] == 'authenticated viewer-capable role is required for guarded operator read routes', operator_context_no_roles
assert no_roles_verdicts['operatorWrite']['allowed'] is False, operator_context_no_roles
assert no_roles_verdicts['operatorWrite']['reason'] == 'authenticated operator role is required for guarded operator write routes', operator_context_no_roles
assert allowed_guarded_read['name'] == 'payments-api-staging', allowed_guarded_read
assert allowed_status_read['deploymentName'] == 'payments-api-staging', allowed_status_read
assert allowed_plan_read['id'] == os.environ['FRONTDOOR_PLAN_ID'], allowed_plan_read
assert allowed_plan_read['deploymentName'] == 'payments-api-staging', allowed_plan_read
assert allowed_apply_read['id'] == os.environ['FRONTDOOR_APPLY_ID'], allowed_apply_read
assert allowed_apply_read['deploymentName'] == 'payments-api-staging', allowed_apply_read
assert allowed_events_read['deploymentName'] == 'payments-api-staging', allowed_events_read
assert isinstance(allowed_events_read.get('events'), list), allowed_events_read
allowed_events_scope = allowed_events_read.get('resultScope') or {}
assert allowed_events_scope.get('requestedLimit') == 1, allowed_events_read
assert allowed_events_scope.get('effectiveLimit') == 1, allowed_events_read
assert allowed_events_scope.get('returnedEventCount') == len(allowed_events_read['events']), allowed_events_read
assert allowed_events_scope.get('truncationStatus') in {'truncated', 'not_truncated', 'unknown'}, allowed_events_read
assert allowed_resource_logs['deploymentName'] == 'worker-prod', allowed_resource_logs
assert allowed_resource_logs['resourceType'] == 'ecsService', allowed_resource_logs
assert allowed_resource_logs['resourceName'] == 'worker-prod-service', allowed_resource_logs
assert isinstance(allowed_resource_logs['available'], bool), allowed_resource_logs
if allowed_resource_logs['available']:
    assert isinstance(allowed_resource_logs.get('source'), str) and allowed_resource_logs['source'], allowed_resource_logs
else:
    assert isinstance(allowed_resource_logs.get('unavailableReason'), str) and allowed_resource_logs['unavailableReason'], allowed_resource_logs
assert allowed_resource_inspection['deploymentName'] == 'worker-prod', allowed_resource_inspection
assert allowed_resource_inspection['resourceType'] == 'ecsService', allowed_resource_inspection
assert allowed_resource_inspection['resourceName'] == 'worker-prod-service', allowed_resource_inspection
assert isinstance(allowed_resource_inspection['available'], bool), allowed_resource_inspection
if allowed_resource_inspection['available']:
    assert isinstance(allowed_resource_inspection.get('source'), str) and allowed_resource_inspection['source'], allowed_resource_inspection
else:
    assert isinstance(allowed_resource_inspection.get('unavailableReason'), str) and allowed_resource_inspection['unavailableReason'], allowed_resource_inspection
filtered_application_names = {row['name'] for row in filtered_applications_collection}
assert 'worker-prod' in filtered_application_names, filtered_applications_collection
assert filtered_application_names == {'payments-api-prod', 'worker-prod'}, filtered_applications_collection
filtered_status_deployment_names = {row['deploymentName'] for row in filtered_status_collection}
assert filtered_status_deployment_names == {'payments-api-prod'}, filtered_status_collection
filtered_catalog_deployment_names = {row['name'] for row in filtered_catalog_deployments}
assert filtered_catalog_deployment_names == {'payments-api-prod', 'worker-prod'}, filtered_catalog_deployments
assert all(row['name'] != 'payments-api-staging' for row in filtered_catalog_deployments), filtered_catalog_deployments
filtered_audit_deployment_names = {row['deploymentName'] for row in filtered_audit_collection['entries']}
assert filtered_audit_deployment_names, filtered_audit_collection
assert filtered_audit_deployment_names <= {'payments-api-prod', 'payments-api-staging'}, filtered_audit_collection
assert 'payments-api-staging' in filtered_audit_deployment_names, filtered_audit_collection
assert len(filtered_accounts_collection) == 1, filtered_accounts_collection
filtered_account = filtered_accounts_collection[0]
assert filtered_account['awsAccountId'] == '123456789012', filtered_accounts_collection
assert filtered_account['deploymentCount'] == 2, filtered_account
assert filtered_account['projectCount'] == 2, filtered_account
assert filtered_account['environmentCount'] == 1, filtered_account
assert filtered_account['syncedCount'] == 1, filtered_account
assert filtered_account['progressingCount'] == 0, filtered_account
assert filtered_account['pausedCount'] == 0, filtered_account
assert {row['name'] for row in filtered_account['deployments']} == {'payments-api-prod', 'worker-prod'}, filtered_account
assert filtered_account_detail['awsAccountId'] == '123456789012', filtered_account_detail
assert filtered_account_detail['deploymentCount'] == 1, filtered_account_detail
assert filtered_account_detail['projectCount'] == 1, filtered_account_detail
assert filtered_account_detail['environmentCount'] == 1, filtered_account_detail
assert filtered_account_detail['syncedCount'] == 1, filtered_account_detail
assert filtered_account_detail['progressingCount'] == 0, filtered_account_detail
assert filtered_account_detail['pausedCount'] == 0, filtered_account_detail
assert [row['name'] for row in filtered_account_detail['deployments']] == ['payments-api-prod'], filtered_account_detail
assert filtered_group_detail['awsAccountId'] == '123456789012', filtered_group_detail
assert filtered_group_detail['appRef'] == 'payments-api', filtered_group_detail
assert filtered_group_detail['environmentRef'] == 'prod-apne2', filtered_group_detail
assert filtered_group_detail['deploymentCount'] == 1, filtered_group_detail
assert filtered_group_detail['syncedCount'] == 1, filtered_group_detail
assert filtered_group_detail['progressingCount'] == 0, filtered_group_detail
assert filtered_group_detail['pausedCount'] == 0, filtered_group_detail
assert [row['name'] for row in filtered_group_detail['deployments']] == ['payments-api-prod'], filtered_group_detail
assert empty_applications_collection == [], empty_applications_collection
assert empty_status_collection == [], empty_status_collection
assert empty_catalog_deployments == [], empty_catalog_deployments
assert empty_accounts_collection == [], empty_accounts_collection
assert empty_audit_collection == {'entries': []}, empty_audit_collection
assert os.environ['AUTH_FORBIDDEN_AUDIT_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_AUDIT_STATUS']
assert forbidden_audit_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_audit_body
assert len(allowed_scoped_audit['entries']) == 1, allowed_scoped_audit
assert allowed_scoped_audit['entries'][0]['deploymentName'] == 'payments-api-staging', allowed_scoped_audit
assert allowed_scoped_audit['entries'][0]['action'] == 'apply.requested', allowed_scoped_audit
print('smoke-local: deployment read auth path ok')
PY

stop_api
TENET_AUTH_PROVIDER=google_oidc \
TENET_GOOGLE_OIDC_CLIENT_ID=google-smoke-client \
TENET_GOOGLE_OIDC_CLIENT_SECRET=placeholder-google-client-value \
TENET_GOOGLE_OIDC_ISSUER_URL=https://accounts.google.com \
TENET_GOOGLE_OIDC_REDIRECT_URL=http://127.0.0.1/auth/callback/google \
TENET_AUTH_STATE_STORE_PATH="$ENTERPRISE_SSO_STATE_STORE_PATH" \
TENET_AUTH_IDENTITY_STORE_PATH="$ENTERPRISE_SSO_IDENTITY_STORE_PATH" \
TENET_AUTH_SESSION_SECRET=placeholder-enterprise-session-value \
  start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED" false false
enterprise_sso_google_context=$(curl -fsS "http://$HOST:$PORT/v1/operator/context")
enterprise_sso_google_context_path=$(write_payload enterprise-sso-google-context.json "$enterprise_sso_google_context")
assert_enterprise_sso_diagnostic "$enterprise_sso_google_context_path" google /auth/login/google /auth/callback/google true \
  TENET_AUTH_PROVIDER \
  TENET_GOOGLE_OIDC_CLIENT_ID \
  TENET_GOOGLE_OIDC_CLIENT_SECRET \
  TENET_GOOGLE_OIDC_ISSUER_URL \
  TENET_GOOGLE_OIDC_REDIRECT_URL \
  TENET_AUTH_STATE_STORE_PATH \
  TENET_AUTH_IDENTITY_STORE_PATH \
  TENET_AUTH_SESSION_SECRET
assert_enterprise_sso_logout_boundary GET "/?page=operator-context"
assert_enterprise_sso_logout_boundary POST "/?page=operator-context&action=logout"

stop_api
TENET_AUTH_PROVIDER=okta_oidc \
TENET_OKTA_OIDC_ISSUER_URL=https://okta.example.com/oauth2/default \
TENET_OKTA_OIDC_CLIENT_ID=okta-smoke-client \
TENET_OKTA_OIDC_CLIENT_SECRET=placeholder-secret-value \
TENET_OKTA_OIDC_REDIRECT_URL=http://127.0.0.1/auth/callback/okta/oidc \
TENET_AUTH_STATE_STORE_PATH="$ENTERPRISE_SSO_STATE_STORE_PATH" \
TENET_AUTH_IDENTITY_STORE_PATH="$ENTERPRISE_SSO_IDENTITY_STORE_PATH" \
TENET_AUTH_SESSION_SECRET=placeholder-session-secret-value \
  start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED" false false
enterprise_sso_okta_oidc_context=$(curl -fsS "http://$HOST:$PORT/v1/operator/context")
enterprise_sso_okta_oidc_context_path=$(write_payload enterprise-sso-okta-oidc-context.json "$enterprise_sso_okta_oidc_context")
assert_enterprise_sso_diagnostic "$enterprise_sso_okta_oidc_context_path" okta_oidc /auth/login/okta /auth/callback/okta/oidc true \
  TENET_AUTH_PROVIDER \
  TENET_OKTA_OIDC_ISSUER_URL \
  TENET_OKTA_OIDC_CLIENT_ID \
  TENET_OKTA_OIDC_CLIENT_SECRET \
  TENET_OKTA_OIDC_REDIRECT_URL \
  TENET_AUTH_STATE_STORE_PATH \
  TENET_AUTH_IDENTITY_STORE_PATH \
  TENET_AUTH_SESSION_SECRET

stop_api
TENET_AUTH_PROVIDER=okta_saml \
TENET_OKTA_SAML_METADATA_URL=https://okta.example.com/app/smoke/sso/saml/metadata \
TENET_OKTA_SAML_ACS_URL=http://127.0.0.1/auth/callback/okta/saml \
TENET_AUTH_STATE_STORE_PATH="$ENTERPRISE_SSO_STATE_STORE_PATH" \
TENET_AUTH_IDENTITY_STORE_PATH="$ENTERPRISE_SSO_IDENTITY_STORE_PATH" \
TENET_AUTH_SESSION_SECRET=placeholder-enterprise-session-value \
  start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED" false false
enterprise_sso_okta_saml_context=$(curl -fsS "http://$HOST:$PORT/v1/operator/context")
enterprise_sso_okta_saml_context_path=$(write_payload enterprise-sso-okta-saml-context.json "$enterprise_sso_okta_saml_context")
assert_enterprise_sso_diagnostic "$enterprise_sso_okta_saml_context_path" okta_saml /auth/login/okta /auth/callback/okta/saml false \
  TENET_AUTH_PROVIDER \
  TENET_OKTA_SAML_METADATA_URL \
  TENET_OKTA_SAML_ACS_URL \
  TENET_AUTH_STATE_STORE_PATH \
  TENET_AUTH_IDENTITY_STORE_PATH \
  TENET_AUTH_SESSION_SECRET

echo "smoke-local: configured enterprise SSO diagnostics ok"
stop_api
start_api "$AUTH_TRUSTED_HEADERS" "$AUTH_ENFORCED" false false

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
  controller_detail_probe_path=$(write_payload controller-detail-probe.json "$controller_detail")
  if CONTROLLER_DETAIL_PATH="$controller_detail_probe_path" python3 - <<'PY'
import json, os, sys

with open(os.environ['CONTROLLER_DETAIL_PATH'], encoding='utf-8') as fh:
    payload = json.load(fh)

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
controller_detail_path=$(write_payload controller-detail.json "$controller_detail")
controller_events_path=$(write_payload controller-events.json "$controller_events")
CONTROLLER_DETAIL_PATH="$controller_detail_path" CONTROLLER_EVENTS_PATH="$controller_events_path" python3 - <<'PY'
import json, os

with open(os.environ['CONTROLLER_DETAIL_PATH'], encoding='utf-8') as fh:
    payload = json.load(fh)
with open(os.environ['CONTROLLER_EVENTS_PATH'], encoding='utf-8') as fh:
    events_payload = json.load(fh)

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
assert all(isinstance(item.get('driftId'), str) and item['driftId'].startswith('drift_') for item in managed_drift), managed_drift
assert all(item.get('severity') in {'high', 'medium', 'low'} for item in managed_drift), managed_drift
assert events_payload['deploymentName'] == 'payments-api-prod', events_payload
assert isinstance(events_payload['events'], list) and events_payload['events'], events_payload
summary = events_payload.get('managedDriftSummary') or {}
event_managed_drift = [item for event in events_payload['events'] for item in event.get('managedDrift', [])]
assert summary.get('total') == len(event_managed_drift), (summary, event_managed_drift)
assert summary.get('high', 0) == sum(1 for item in event_managed_drift if item.get('severity') == 'high'), (summary, event_managed_drift)
assert summary.get('medium', 0) == sum(1 for item in event_managed_drift if item.get('severity') == 'medium'), (summary, event_managed_drift)
assert summary.get('low', 0) == sum(1 for item in event_managed_drift if item.get('severity') == 'low'), (summary, event_managed_drift)
for event in events_payload['events']:
    assert event.get('source') == 'controller', events_payload
    assert 'controllerRefresh' not in event, events_payload
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
