#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
COMPOSE_FILE=${TENET_COMPOSE_FILE:-$ROOT/compose.yaml}
HOST=${TENET_DOCKER_SMOKE_HOST:-127.0.0.1}
PORT=${TENET_DOCKER_SMOKE_PORT:-8080}
API_BASE_URL=${TENET_DOCKER_SMOKE_API_BASE_URL:-http://$HOST:$PORT}
WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}
INVALID_AUDIT_ACTION_BODY=$(mktemp)
INVALID_RESOURCE_LOG_LIMIT_BODY=$(mktemp)
INVALID_RESOURCE_LOG_INSTANCE_BODY=$(mktemp)
INVALID_RESOURCE_INSPECTION_LIMIT_BODY=$(mktemp)
INVALID_RESOURCE_INSPECTION_INSTANCE_BODY=$(mktemp)
INVALID_APPLY_TRANSITION_BODY=$(mktemp)
UNKNOWN_APPLY_TRANSITION_BODY=$(mktemp)
TERMINAL_APPLY_TRANSITION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_PLAN_READ_BODY=$(mktemp)
AUTH_UNAUTHENTICATED_APPLY_READ_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_COLLECTION_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_STATUS_READ_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_PLAN_READ_BODY=$(mktemp)
AUTH_ROLE_REQUIRED_APPLY_READ_BODY=$(mktemp)
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
WEBHOOK_DELETED_BODY_FILE=$(mktemp)
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

  for _ in $(seq 1 20); do
    if headers=$(curl -fsS -D - -o /dev/null "$url" 2>/dev/null); then
      break
    fi
    sleep 1
  done
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
  local codedeploy_application_resource_logs_request_id=$5
  local audit_logs_request_id=$6
  local application_ui_request_id=$7
  local account_ui_request_id=$8
  local drift_ui_request_id=$9
  local audit_ui_request_id=${10}
  local plan_apply_ui_request_id=${11}

  API_LOGS_TEXT="$logs_text" \
  HEALTHZ_REQUEST_ID="$healthz_request_id" \
  SUMMARY_REQUEST_ID="$summary_request_id" \
  WORKER_RESOURCE_LOGS_REQUEST_ID="$worker_resource_logs_request_id" \
  CODEDEPLOY_APPLICATION_RESOURCE_LOGS_REQUEST_ID="$codedeploy_application_resource_logs_request_id" \
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
    (os.environ["CODEDEPLOY_APPLICATION_RESOURCE_LOGS_REQUEST_ID"], "/v1/overview/deployments/payments-api-staging/resource-logs"),
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
  rm -f "$INVALID_AUDIT_ACTION_BODY" "$INVALID_RESOURCE_LOG_LIMIT_BODY" "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY" "$INVALID_APPLY_TRANSITION_BODY" "$UNKNOWN_APPLY_TRANSITION_BODY" "$TERMINAL_APPLY_TRANSITION_BODY" "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" "$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" "$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" "$AUTH_ROLE_REQUIRED_COLLECTION_BODY" "$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" "$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" "$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" "$AUTH_ROLE_REQUIRED_APPLY_READ_BODY" "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" "$AUTH_UNAUTHENTICATED_EVENTS_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" "$AUTH_FORBIDDEN_STATUS_READ_BODY" "$AUTH_FORBIDDEN_TRANSITION_BODY" "$AUTH_FORBIDDEN_EVENTS_BODY" "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" "$WEBHOOK_PUSH_BODY_FILE" "$WEBHOOK_PING_BODY_FILE" "$INVALID_WEBHOOK_BODY"
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
  headers_path=$(mktemp "$PAYLOAD_DIR/enterprise-sso-logout-headers.XXXXXX")
  body_path=$(mktemp "$PAYLOAD_DIR/enterprise-sso-logout-body.XXXXXX")
  status=$(curl -sS -D "$headers_path" -o "$body_path" -w "%{http_code}" \
    -X "$method" \
    "$API_BASE_URL/auth/logout?returnUrl=$encoded_return_url")

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

wait_for_deployment_managed_drift() {
  local deployment_name=$1
  local response='{}'
  local response_path

  for _ in $(seq 1 90); do
    if ! response=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/$deployment_name" 2>/dev/null); then
      sleep 1
      continue
    fi
    response_path=$(write_payload wait-for-deployment-detail.json "$response")
    if python3 - "$response_path" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
managed = payload.get('managedDrift')
runtime_managed = (payload.get('runtimeSummary') or {}).get('managedDrift')
if isinstance(managed, list) and managed and isinstance(runtime_managed, list) and runtime_managed:
    raise SystemExit(0)
raise SystemExit(1)
PY
    then
      printf '%s' "$response"
      return 0
    fi
    sleep 1
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
  TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true \
  TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true \
  TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \
  "${COMPOSE[@]}" up -d api >/dev/null
  wait_for_api_health
}
restart_api_with_enterprise_sso() {
  local provider=$1

  case "$provider" in
    google_oidc)
      TENET_AWS_ENABLE_APPLY=false \
      TENET_API_TRUST_AUTHENTICATED_HEADERS=true \
      TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=false \
      TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=false \
      TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=false \
      TENET_AUTH_PROVIDER=google_oidc \
      TENET_GOOGLE_OIDC_CLIENT_ID=google-smoke-client \
      TENET_GOOGLE_OIDC_CLIENT_SECRET=placeholder-google-client-value \
      TENET_GOOGLE_OIDC_ISSUER_URL=https://accounts.google.com \
      TENET_GOOGLE_OIDC_REDIRECT_URL=http://127.0.0.1/auth/callback/google \
      TENET_AUTH_STATE_STORE_PATH=/tmp/tenet-enterprise-sso-state.json \
      TENET_AUTH_IDENTITY_STORE_PATH=/tmp/tenet-enterprise-sso-identities.json \
      TENET_AUTH_SESSION_SECRET=placeholder-enterprise-session-value \
      "${COMPOSE[@]}" up -d api >/dev/null
      ;;
    okta_oidc)
      TENET_AWS_ENABLE_APPLY=false \
      TENET_API_TRUST_AUTHENTICATED_HEADERS=true \
      TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true \
      TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true \
      TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \
      TENET_AUTH_PROVIDER=okta_oidc \
      TENET_OKTA_OIDC_ISSUER_URL=https://okta.example.com/oauth2/default \
      TENET_OKTA_OIDC_CLIENT_ID=okta-smoke-client \
      TENET_OKTA_OIDC_CLIENT_SECRET=placeholder-secret-value \
      TENET_OKTA_OIDC_REDIRECT_URL=http://127.0.0.1/auth/callback/okta/oidc \
      TENET_AUTH_STATE_STORE_PATH=/tmp/tenet/enterprise-sso-state.json \
      TENET_AUTH_IDENTITY_STORE_PATH=/tmp/tenet/enterprise-sso-identities.json \
      TENET_AUTH_SESSION_SECRET=placeholder-session-secret-value \
      "${COMPOSE[@]}" up -d api >/dev/null
      ;;
    okta_saml)
      TENET_AWS_ENABLE_APPLY=false \
      TENET_API_TRUST_AUTHENTICATED_HEADERS=true \
      TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=false \
      TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=false \
      TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=false \
      TENET_AUTH_PROVIDER=okta_saml \
      TENET_OKTA_SAML_METADATA_URL=https://okta.example.com/app/smoke/sso/saml/metadata \
      TENET_OKTA_SAML_ACS_URL=http://127.0.0.1/auth/callback/okta/saml \
      TENET_AUTH_STATE_STORE_PATH=/tmp/tenet-enterprise-sso-state.json \
      TENET_AUTH_IDENTITY_STORE_PATH=/tmp/tenet-enterprise-sso-identities.json \
      TENET_AUTH_SESSION_SECRET=placeholder-enterprise-session-value \
      "${COMPOSE[@]}" up -d api >/dev/null
      ;;
    *)
      echo "smoke-docker: unsupported enterprise SSO provider $provider" >&2
      exit 1
      ;;
  esac
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
codedeploy_application_resource_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app")
audit_logs_request_id=$(assert_request_id_header "$API_BASE_URL/v1/audit-logs?action=apply.requested&limit=2")

summary=$(curl -fsS "$API_BASE_URL/v1/catalog/summary")
applications=$(curl -fsS "$API_BASE_URL/v1/overview/applications")
deployment_detail=$(wait_for_deployment_managed_drift payments-api-prod)
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
limited_worker_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=1")
ecs_task_resource_name="arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123"
ecs_task_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=$ecs_task_resource_name&containerName=worker&limit=1")
ecs_task_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=$ecs_task_resource_name")
invalid_resource_log_limit_status=$(curl -sS -o "$INVALID_RESOURCE_LOG_LIMIT_BODY" -w "%{http_code}" "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=0")
invalid_resource_inspection_limit_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY" -w "%{http_code}" "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=0")
codedeploy_application_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app")
test_listener_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=testListener&resourceName=payments-api-staging-test-listener")
worker_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
bootstrap_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
selected_bootstrap_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-0abc123def4567890")
invalid_resource_log_instance_status=$(curl -sS -o "$INVALID_RESOURCE_LOG_INSTANCE_BODY" -w "%{http_code}" "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh&instanceId=i-")
traffic_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
production_listener_rule_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")
active_target_group_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")
candidate_target_group_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")
bootstrap_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh")
candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")
bounded_candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&limit=2")
selected_candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890")
invalid_resource_inspection_instance_status=$(curl -sS -o "$INVALID_RESOURCE_INSPECTION_INSTANCE_BODY" -w "%{http_code}" "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-")
codedeploy_application_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployApplication&resourceName=payments-api-staging-codedeploy-app")
codedeploy_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging-codedeploy-group")
production_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListener&resourceName=payments-api-staging-prod-listener")
test_listener_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=testListener&resourceName=payments-api-staging-test-listener")
blue_task_set_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")
traffic_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")
production_listener_rule_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")
active_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")
candidate_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")
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
webhook_deleted_body=$(cat <<'EOF'
{"ref":"refs/heads/obsolete-release","before":"3333333333333333333333333333333333333333","after":"0000000000000000000000000000000000000000","repository":{"full_name":"acme/platform-config"},"head_commit":null,"commits":[{"id":"c","added":[],"modified":[],"removed":["deployments/obsolete.yaml"]}]}
EOF
)
webhook_deleted_signature=$(WEBHOOK_SECRET="$WEBHOOK_SECRET" WEBHOOK_BODY="$webhook_deleted_body" python3 - <<'PY'
import hashlib
import hmac
import os

print('sha256=' + hmac.new(os.environ['WEBHOOK_SECRET'].encode('utf-8'), os.environ['WEBHOOK_BODY'].encode('utf-8'), hashlib.sha256).hexdigest())
PY
)
webhook_deleted_status=$(curl -sS -o "$WEBHOOK_DELETED_BODY_FILE" -w "%{http_code}" -X POST \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Delivery: delivery-deleted-1' \
  -H 'X-GitHub-Event: push' \
  -H "X-Hub-Signature-256: $webhook_deleted_signature" \
  --data "$webhook_deleted_body" \
  "$API_BASE_URL/api/v1/github/webhook")
webhook_deleted=$(cat "$WEBHOOK_DELETED_BODY_FILE")
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
codedeploy_application_resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication%3A%3Apayments-api-staging-codedeploy-app%3A%3ACodeDeploy+application+coordinating+ECS+traffic+shifting")
blue_task_set_target_group_resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup%3A%3Apayments-api-staging-blue%3A%3ABlue+task+set+target+group+CodeDeploy+treats+as+the+live+ECS+traffic+destination")
production_listener_rule_resource_ui=$(curl -fsS "$API_BASE_URL/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule%3A%3Apayments-api-prod-prod-listener%3A%3AProduction+listener+rule+operators+inspect+while+Tenet+updates+weighted+forwarding+during+rollout")
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
limited_worker_resource_logs_path=$(write_payload limited-worker-resource-logs.json "$limited_worker_resource_logs")
ecs_task_resource_logs_path=$(write_payload ecs-task-resource-logs.json "$ecs_task_resource_logs")
ecs_task_resource_inspection_path=$(write_payload ecs-task-resource-inspection.json "$ecs_task_resource_inspection")
invalid_resource_log_limit_path=$(write_payload invalid-resource-log-limit.txt "$(cat "$INVALID_RESOURCE_LOG_LIMIT_BODY")")
invalid_resource_log_instance_path=$(write_payload invalid-resource-log-instance.txt "$(cat "$INVALID_RESOURCE_LOG_INSTANCE_BODY")")
invalid_resource_inspection_limit_path=$(write_payload invalid-resource-inspection-limit.txt "$(cat "$INVALID_RESOURCE_INSPECTION_LIMIT_BODY")")
codedeploy_application_resource_logs_path=$(write_payload codedeploy-application-resource-logs.json "$codedeploy_application_resource_logs")
test_listener_resource_logs_path=$(write_payload test-listener-resource-logs.json "$test_listener_resource_logs")
worker_resource_inspection_path=$(write_payload worker-resource-inspection.json "$worker_resource_inspection")
bootstrap_resource_logs_path=$(write_payload bootstrap-resource-logs.json "$bootstrap_resource_logs")
selected_bootstrap_resource_logs_path=$(write_payload selected-bootstrap-resource-logs.json "$selected_bootstrap_resource_logs")
traffic_resource_logs_path=$(write_payload traffic-resource-logs.json "$traffic_resource_logs")
production_listener_rule_resource_logs_path=$(write_payload production-listener-rule-resource-logs.json "$production_listener_rule_resource_logs")
active_target_group_resource_logs_path=$(write_payload active-target-group-resource-logs.json "$active_target_group_resource_logs")
candidate_target_group_resource_logs_path=$(write_payload candidate-target-group-resource-logs.json "$candidate_target_group_resource_logs")
bootstrap_resource_inspection_path=$(write_payload bootstrap-resource-inspection.json "$bootstrap_resource_inspection")
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
audit_logs_path=$(write_payload audit-logs.json "$audit_logs")
audit_logs_action_filtered_path=$(write_payload audit-logs-action-filtered.json "$audit_logs_action_filtered")
actor_audit_logs_path=$(write_payload actor-audit-logs.json "$actor_audit_logs")
webhook_push_path=$(write_payload webhook-push.json "$webhook_push")
webhook_deleted_path=$(write_payload webhook-deleted.json "$webhook_deleted")
webhook_ping_path=$(write_payload webhook-ping.json "$webhook_ping")
invalid_webhook_path=$(write_payload invalid-webhook.txt "$invalid_webhook")
ui_path=$(write_payload ui.html "$ui")
application_ui_path=$(write_payload application-ui.html "$application_ui")
account_ui_path=$(write_payload account-ui.html "$account_ui")
resource_ui_path=$(write_payload resource-ui.html "$resource_ui")
ecs_resource_ui_path=$(write_payload ecs-resource-ui.html "$ecs_resource_ui")
bootstrap_resource_ui_path=$(write_payload bootstrap-resource-ui.html "$bootstrap_resource_ui")
codedeploy_application_resource_ui_path=$(write_payload codedeploy-application-resource-ui.html "$codedeploy_application_resource_ui")
blue_task_set_target_group_resource_ui_path=$(write_payload blue-task-set-target-group-resource-ui.html "$blue_task_set_target_group_resource_ui")
production_listener_rule_resource_ui_path=$(write_payload production-listener-rule-resource-ui.html "$production_listener_rule_resource_ui")
drift_ui_path=$(write_payload drift-ui.html "$drift_ui")
audit_ui_path=$(write_payload audit-ui.html "$audit_ui")
plan_apply_ui_path=$(write_payload plan-apply-ui.html "$plan_apply_ui")

SUMMARY_PATH="$summary_path" APPLICATIONS_PATH="$applications_path" DEPLOYMENT_DETAIL_PATH="$deployment_detail_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" PLAN_PREVIEW_PATH="$plan_preview_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_path" APPLY_RECORD_PATH="$apply_record_path" APPLY_RECORD_GET_PATH="$apply_record_get_path" INVALID_APPLY_TRANSITION_STATUS="$invalid_apply_transition_status" INVALID_APPLY_TRANSITION_PATH="$invalid_apply_transition_path" UNKNOWN_APPLY_TRANSITION_STATUS="$unknown_apply_transition_status" UNKNOWN_APPLY_TRANSITION_PATH="$unknown_apply_transition_path" TERMINAL_APPLY_TRANSITION_STATUS="$terminal_apply_transition_status" TERMINAL_APPLY_TRANSITION_PATH="$terminal_apply_transition_path" ACTOR_PLAN_PREVIEW_PATH="$actor_plan_preview_path" ACTOR_PLAN_PREVIEW_GET_PATH="$actor_plan_preview_get_path" ACTOR_APPLY_RECORD_PATH="$actor_apply_record_path" ACTOR_APPLY_RECORD_GET_PATH="$actor_apply_record_get_path" DEPLOYMENT_EVENTS_PATH="$deployment_events_path" WORKER_RESOURCE_LOGS_PATH="$worker_resource_logs_path" LIMITED_WORKER_RESOURCE_LOGS_PATH="$limited_worker_resource_logs_path" ECS_TASK_RESOURCE_LOGS_PATH="$ecs_task_resource_logs_path" ECS_TASK_RESOURCE_INSPECTION_PATH="$ecs_task_resource_inspection_path" INVALID_RESOURCE_LOG_LIMIT_STATUS="$invalid_resource_log_limit_status" INVALID_RESOURCE_LOG_LIMIT_PATH="$invalid_resource_log_limit_path" INVALID_RESOURCE_LOG_INSTANCE_STATUS="$invalid_resource_log_instance_status" INVALID_RESOURCE_LOG_INSTANCE_PATH="$invalid_resource_log_instance_path" INVALID_RESOURCE_INSPECTION_LIMIT_STATUS="$invalid_resource_inspection_limit_status" INVALID_RESOURCE_INSPECTION_LIMIT_PATH="$invalid_resource_inspection_limit_path" INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS="$invalid_resource_inspection_instance_status" INVALID_RESOURCE_INSPECTION_INSTANCE_PATH="$invalid_resource_inspection_instance_path" CODEDEPLOY_APPLICATION_RESOURCE_LOGS_PATH="$codedeploy_application_resource_logs_path" TEST_LISTENER_RESOURCE_LOGS_PATH="$test_listener_resource_logs_path" WORKER_RESOURCE_INSPECTION_PATH="$worker_resource_inspection_path" BOOTSTRAP_RESOURCE_LOGS_PATH="$bootstrap_resource_logs_path" SELECTED_BOOTSTRAP_RESOURCE_LOGS_PATH="$selected_bootstrap_resource_logs_path" TRAFFIC_RESOURCE_LOGS_PATH="$traffic_resource_logs_path" PRODUCTION_LISTENER_RULE_RESOURCE_LOGS_PATH="$production_listener_rule_resource_logs_path" ACTIVE_TARGET_GROUP_RESOURCE_LOGS_PATH="$active_target_group_resource_logs_path" CANDIDATE_TARGET_GROUP_RESOURCE_LOGS_PATH="$candidate_target_group_resource_logs_path" BOOTSTRAP_RESOURCE_INSPECTION_PATH="$bootstrap_resource_inspection_path" CANDIDATE_RESOURCE_INSPECTION_PATH="$candidate_resource_inspection_path" BOUNDED_CANDIDATE_RESOURCE_INSPECTION_PATH="$bounded_candidate_resource_inspection_path" SELECTED_CANDIDATE_RESOURCE_INSPECTION_PATH="$selected_candidate_resource_inspection_path" CODEDEPLOY_APPLICATION_RESOURCE_INSPECTION_PATH="$codedeploy_application_resource_inspection_path" CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH="$codedeploy_group_resource_inspection_path" PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH="$production_listener_resource_inspection_path" TEST_LISTENER_RESOURCE_INSPECTION_PATH="$test_listener_resource_inspection_path" BLUE_TASK_SET_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$blue_task_set_target_group_resource_inspection_path" TRAFFIC_RESOURCE_INSPECTION_PATH="$traffic_resource_inspection_path" PRODUCTION_LISTENER_RULE_RESOURCE_INSPECTION_PATH="$production_listener_rule_resource_inspection_path" ACTIVE_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$active_target_group_resource_inspection_path" CANDIDATE_TARGET_GROUP_RESOURCE_INSPECTION_PATH="$candidate_target_group_resource_inspection_path" AUDIT_LOGS_PATH="$audit_logs_path" AUDIT_LOGS_ACTION_FILTERED_PATH="$audit_logs_action_filtered_path" ACTOR_AUDIT_LOGS_PATH="$actor_audit_logs_path" INVALID_AUDIT_ACTION_STATUS="$invalid_audit_action_status" INVALID_AUDIT_ACTION_BODY_PATH="$INVALID_AUDIT_ACTION_BODY" WEBHOOK_PUSH_PATH="$webhook_push_path" WEBHOOK_PUSH_STATUS="$webhook_push_status" WEBHOOK_DELETED_PATH="$webhook_deleted_path" WEBHOOK_DELETED_STATUS="$webhook_deleted_status" WEBHOOK_PING_PATH="$webhook_ping_path" WEBHOOK_PING_STATUS="$webhook_ping_status" INVALID_WEBHOOK_PATH="$invalid_webhook_path" INVALID_WEBHOOK_STATUS="$invalid_webhook_status" UI_PATH="$ui_path" APPLICATION_UI_PATH="$application_ui_path" ACCOUNT_UI_PATH="$account_ui_path" RESOURCE_UI_PATH="$resource_ui_path" ECS_RESOURCE_UI_PATH="$ecs_resource_ui_path" BOOTSTRAP_RESOURCE_UI_PATH="$bootstrap_resource_ui_path" CODEDEPLOY_APPLICATION_RESOURCE_UI_PATH="$codedeploy_application_resource_ui_path" BLUE_TASK_SET_TARGET_GROUP_RESOURCE_UI_PATH="$blue_task_set_target_group_resource_ui_path" PRODUCTION_LISTENER_RULE_RESOURCE_UI_PATH="$production_listener_rule_resource_ui_path" DRIFT_UI_PATH="$drift_ui_path" AUDIT_UI_PATH="$audit_ui_path" PLAN_APPLY_UI_PATH="$plan_apply_ui_path" python3 - <<'PY'
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
limited_worker_resource_logs = read_json("LIMITED_WORKER_RESOURCE_LOGS_PATH")
ecs_task_resource_logs = read_json("ECS_TASK_RESOURCE_LOGS_PATH")
ecs_task_resource_inspection = read_json("ECS_TASK_RESOURCE_INSPECTION_PATH")
invalid_resource_log_limit = read_text("INVALID_RESOURCE_LOG_LIMIT_PATH")
invalid_resource_log_instance = read_text("INVALID_RESOURCE_LOG_INSTANCE_PATH")
invalid_resource_inspection_limit = read_text("INVALID_RESOURCE_INSPECTION_LIMIT_PATH")
invalid_resource_inspection_instance = read_text("INVALID_RESOURCE_INSPECTION_INSTANCE_PATH")
codedeploy_application_resource_logs = read_json("CODEDEPLOY_APPLICATION_RESOURCE_LOGS_PATH")
test_listener_resource_logs = read_json("TEST_LISTENER_RESOURCE_LOGS_PATH")
worker_resource_inspection = read_json("WORKER_RESOURCE_INSPECTION_PATH")
bootstrap_resource_logs = read_json("BOOTSTRAP_RESOURCE_LOGS_PATH")
selected_bootstrap_resource_logs = read_json("SELECTED_BOOTSTRAP_RESOURCE_LOGS_PATH")
traffic_resource_logs = read_json("TRAFFIC_RESOURCE_LOGS_PATH")
production_listener_rule_resource_logs = read_json("PRODUCTION_LISTENER_RULE_RESOURCE_LOGS_PATH")
active_target_group_resource_logs = read_json("ACTIVE_TARGET_GROUP_RESOURCE_LOGS_PATH")
candidate_target_group_resource_logs = read_json("CANDIDATE_TARGET_GROUP_RESOURCE_LOGS_PATH")
bootstrap_resource_inspection = read_json("BOOTSTRAP_RESOURCE_INSPECTION_PATH")
candidate_resource_inspection = read_json("CANDIDATE_RESOURCE_INSPECTION_PATH")
bounded_candidate_resource_inspection = read_json("BOUNDED_CANDIDATE_RESOURCE_INSPECTION_PATH")
selected_candidate_resource_inspection = read_json("SELECTED_CANDIDATE_RESOURCE_INSPECTION_PATH")
codedeploy_application_resource_inspection = read_json("CODEDEPLOY_APPLICATION_RESOURCE_INSPECTION_PATH")
codedeploy_group_resource_inspection = read_json("CODEDEPLOY_GROUP_RESOURCE_INSPECTION_PATH")
production_listener_resource_inspection = read_json("PRODUCTION_LISTENER_RESOURCE_INSPECTION_PATH")
test_listener_resource_inspection = read_json("TEST_LISTENER_RESOURCE_INSPECTION_PATH")
blue_task_set_target_group_resource_inspection = read_json("BLUE_TASK_SET_TARGET_GROUP_RESOURCE_INSPECTION_PATH")
traffic_resource_inspection = read_json("TRAFFIC_RESOURCE_INSPECTION_PATH")
production_listener_rule_resource_inspection = read_json("PRODUCTION_LISTENER_RULE_RESOURCE_INSPECTION_PATH")
active_target_group_resource_inspection = read_json("ACTIVE_TARGET_GROUP_RESOURCE_INSPECTION_PATH")
candidate_target_group_resource_inspection = read_json("CANDIDATE_TARGET_GROUP_RESOURCE_INSPECTION_PATH")
audit_logs = read_json("AUDIT_LOGS_PATH")
audit_logs_action_filtered = read_json("AUDIT_LOGS_ACTION_FILTERED_PATH")
actor_audit_logs = read_json("ACTOR_AUDIT_LOGS_PATH")
invalid_audit_action_body = read_text("INVALID_AUDIT_ACTION_BODY_PATH")
webhook_push = read_json("WEBHOOK_PUSH_PATH")
webhook_deleted = read_json("WEBHOOK_DELETED_PATH")
webhook_ping = read_json("WEBHOOK_PING_PATH")
invalid_webhook = read_text("INVALID_WEBHOOK_PATH")
ui = read_text("UI_PATH")
application_ui = read_text("APPLICATION_UI_PATH")
account_ui = read_text("ACCOUNT_UI_PATH")
resource_ui = read_text("RESOURCE_UI_PATH")
ecs_resource_ui = read_text("ECS_RESOURCE_UI_PATH")
bootstrap_resource_ui = read_text("BOOTSTRAP_RESOURCE_UI_PATH")
codedeploy_application_resource_ui = read_text("CODEDEPLOY_APPLICATION_RESOURCE_UI_PATH")
blue_task_set_target_group_resource_ui = read_text("BLUE_TASK_SET_TARGET_GROUP_RESOURCE_UI_PATH")
production_listener_rule_resource_ui = read_text("PRODUCTION_LISTENER_RULE_RESOURCE_UI_PATH")
drift_ui = read_text("DRIFT_UI_PATH")
audit_ui = read_text("AUDIT_UI_PATH")
plan_apply_ui = read_text("PLAN_APPLY_UI_PATH")

def assert_recent_log_empty_reason_contract(label, payload):
    entries = payload.get("entries") or []
    assert isinstance(entries, list), {label: payload}
    if payload.get("available") is True:
        if entries:
            assert "emptyReason" not in payload, {label: payload}
        else:
            assert payload.get("emptyReason") in {"no_recent_events", "no_formatted_entries"}, {label: payload}
    else:
        assert isinstance(payload.get("unavailableReason"), str) and payload["unavailableReason"], {label: payload}

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
resource_refs = [row.get("resourceRef") for row in managed_drift if isinstance(row.get("resourceRef"), dict)]
for ref in resource_refs:
    assert isinstance(ref.get("resourceType"), str) and ref["resourceType"], (ref, managed_drift)
    assert isinstance(ref.get("resourceName"), str) and ref["resourceName"], (ref, managed_drift)

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
assert os.environ["WEBHOOK_DELETED_STATUS"] == "200", os.environ["WEBHOOK_DELETED_STATUS"]
assert webhook_deleted["deliveryId"] == "delivery-deleted-1", webhook_deleted
assert webhook_deleted["event"] == "push", webhook_deleted
assert webhook_deleted["status"] == "ignored", webhook_deleted
assert webhook_deleted["processed"] is False, webhook_deleted
assert webhook_deleted["deleted"] is True, webhook_deleted
assert webhook_deleted["repository"] == "acme/platform-config", webhook_deleted
assert webhook_deleted["ref"] == "refs/heads/obsolete-release", webhook_deleted
assert webhook_deleted["before"] == "3333333333333333333333333333333333333333", webhook_deleted
assert webhook_deleted["after"] == "0000000000000000000000000000000000000000", webhook_deleted
assert webhook_deleted["changedFiles"]["all"] == ["deployments/obsolete.yaml"], webhook_deleted
assert "controller refresh handoff skipped" in webhook_deleted["reason"], webhook_deleted
assert "controllerRefresh" not in webhook_deleted, webhook_deleted
assert os.environ["WEBHOOK_PING_STATUS"] == "200", os.environ["WEBHOOK_PING_STATUS"]
assert webhook_ping["deliveryId"] == "delivery-ping-1", webhook_ping
assert webhook_ping["event"] == "ping", webhook_ping
assert webhook_ping["status"] == "ignored", webhook_ping
assert webhook_ping["processed"] is False, webhook_ping
assert "desiredStateSourceMatch" not in webhook_ping, webhook_ping
assert os.environ["INVALID_WEBHOOK_STATUS"] == "400", os.environ["INVALID_WEBHOOK_STATUS"]
assert "github webhook signature mismatch" in invalid_webhook, invalid_webhook
expected_audit_refs = {
    "payments-api-prod": ("payments-api", "prod-apne2"),
    "payments-api-staging": ("payments-api", "staging-apne2"),
    "worker-prod": ("worker", "prod-apne2"),
}
for entry in audit_logs["entries"]:
    app_ref, environment_ref = expected_audit_refs[entry["deploymentName"]]
    assert entry["appRef"] == app_ref, entry
    assert entry["environmentRef"] == environment_ref, entry
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
event_managed_drift = [item for event in deployment_events["events"] for item in event.get("managedDrift", [])]
events_drift_summary = deployment_events.get("managedDriftSummary") or {}
assert events_drift_summary.get("total") == len(event_managed_drift), (events_drift_summary, event_managed_drift)
assert events_drift_summary.get("high", 0) == sum(1 for item in event_managed_drift if item.get("severity") == "high"), (events_drift_summary, event_managed_drift)
assert events_drift_summary.get("medium", 0) == sum(1 for item in event_managed_drift if item.get("severity") == "medium"), (events_drift_summary, event_managed_drift)
assert events_drift_summary.get("low", 0) == sum(1 for item in event_managed_drift if item.get("severity") == "low"), (events_drift_summary, event_managed_drift)
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
worker_scope = worker_resource_logs.get("resultScope")
assert isinstance(worker_scope, dict), worker_resource_logs
assert "requestedLimit" not in worker_scope, worker_resource_logs
assert worker_scope.get("effectiveLimit") == 50, worker_resource_logs
assert worker_scope.get("returnedEntryCount") == len(worker_resource_logs.get("entries") or []), worker_resource_logs
assert worker_scope.get("truncationStatus") in ("not_truncated", "unknown", "not_applicable"), worker_resource_logs
if worker_resource_logs["available"]:
    assert isinstance(worker_resource_logs.get("source"), str) and worker_resource_logs["source"], worker_resource_logs
    for entry in worker_resource_logs.get("entries") or []:
        assert isinstance(entry.get("timestamp"), str) and entry["timestamp"], worker_resource_logs
        assert isinstance(entry.get("message"), str) and entry["message"], worker_resource_logs
        if "stream" in entry:
            assert isinstance(entry.get("stream"), str) and entry["stream"], worker_resource_logs
else:
    assert isinstance(worker_resource_logs.get("unavailableReason"), str) and worker_resource_logs["unavailableReason"], worker_resource_logs
assert limited_worker_resource_logs["deploymentName"] == "worker-prod", limited_worker_resource_logs
assert limited_worker_resource_logs["resourceType"] == "ecsService", limited_worker_resource_logs
assert limited_worker_resource_logs["resourceName"] == "worker-prod-service", limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs.get("resourceDescription"), str) and limited_worker_resource_logs["resourceDescription"], limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs["available"], bool), limited_worker_resource_logs
assert isinstance(limited_worker_resource_logs.get("lookupContext"), dict), limited_worker_resource_logs
assert limited_worker_resource_logs.get("entries") is None or isinstance(limited_worker_resource_logs.get("entries"), list), limited_worker_resource_logs
assert len(limited_worker_resource_logs.get("entries") or []) <= 1, limited_worker_resource_logs
limited_scope = limited_worker_resource_logs.get("resultScope")
assert isinstance(limited_scope, dict), limited_worker_resource_logs
assert limited_scope.get("requestedLimit") == 1, limited_worker_resource_logs
assert limited_scope.get("effectiveLimit") == 1, limited_worker_resource_logs
assert limited_scope.get("returnedEntryCount") == len(limited_worker_resource_logs.get("entries") or []), limited_worker_resource_logs
assert limited_scope.get("truncationStatus") in ("truncated", "not_truncated", "unknown", "not_applicable"), limited_worker_resource_logs
if limited_worker_resource_logs["available"]:
    assert isinstance(limited_worker_resource_logs.get("source"), str) and limited_worker_resource_logs["source"], limited_worker_resource_logs
else:
    assert isinstance(limited_worker_resource_logs.get("unavailableReason"), str) and limited_worker_resource_logs["unavailableReason"], limited_worker_resource_logs
assert ecs_task_resource_logs["deploymentName"] == "worker-prod", ecs_task_resource_logs
assert ecs_task_resource_logs["resourceType"] == "ecsTask", ecs_task_resource_logs
assert ecs_task_resource_logs["resourceName"] == "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123", ecs_task_resource_logs
assert ecs_task_resource_logs.get("resourceDescription") == "ECS task selected from live service inspection", ecs_task_resource_logs
assert ecs_task_resource_logs.get("containerName") == "worker", ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["available"], bool), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs.get("lookupContext"), dict), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["lookupContext"].get("region"), str) and ecs_task_resource_logs["lookupContext"]["region"], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["lookupContext"].get("controlAccountId"), str) and ecs_task_resource_logs["lookupContext"]["controlAccountId"], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["lookupContext"].get("targetAccountId"), str) and ecs_task_resource_logs["lookupContext"]["targetAccountId"], ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["lookupContext"].get("crossAccount"), bool), ecs_task_resource_logs
assert isinstance(ecs_task_resource_logs["lookupContext"].get("observerRoleConfigured"), bool), ecs_task_resource_logs
assert ecs_task_resource_logs.get("entries") is None or isinstance(ecs_task_resource_logs.get("entries"), list), ecs_task_resource_logs
assert len(ecs_task_resource_logs.get("entries") or []) <= 1, ecs_task_resource_logs
ecs_task_scope = ecs_task_resource_logs.get("resultScope")
assert isinstance(ecs_task_scope, dict), ecs_task_resource_logs
assert ecs_task_scope.get("requestedLimit") == 1, ecs_task_resource_logs
assert ecs_task_scope.get("effectiveLimit") == 1, ecs_task_resource_logs
assert ecs_task_scope.get("returnedEntryCount") == len(ecs_task_resource_logs.get("entries") or []), ecs_task_resource_logs
assert ecs_task_scope.get("truncationStatus") in ("truncated", "not_truncated", "unknown", "not_applicable"), ecs_task_resource_logs
if ecs_task_resource_logs["available"]:
    assert ecs_task_resource_logs.get("source") == "cloudwatch", ecs_task_resource_logs
else:
    assert ecs_task_resource_logs.get("unavailableReason") in {"aws_unavailable", "resource_not_found", "cross_account_not_configured", "no_log_source"}, ecs_task_resource_logs
for label, payload in (
    ("worker_resource_logs", worker_resource_logs),
    ("limited_worker_resource_logs", limited_worker_resource_logs),
    ("ecs_task_resource_logs", ecs_task_resource_logs),
    ("codedeploy_application_resource_logs", codedeploy_application_resource_logs),
    ("test_listener_resource_logs", test_listener_resource_logs),
    ("bootstrap_resource_logs", bootstrap_resource_logs),
    ("selected_bootstrap_resource_logs", selected_bootstrap_resource_logs),
    ("traffic_resource_logs", traffic_resource_logs),
    ("production_listener_rule_resource_logs", production_listener_rule_resource_logs),
    ("active_target_group_resource_logs", active_target_group_resource_logs),
    ("candidate_target_group_resource_logs", candidate_target_group_resource_logs),
):
    assert_recent_log_empty_reason_contract(label, payload)

assert ecs_task_resource_inspection["deploymentName"] == "worker-prod", ecs_task_resource_inspection
assert ecs_task_resource_inspection["resourceType"] == "ecsTask", ecs_task_resource_inspection
assert ecs_task_resource_inspection["resourceName"] == "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123", ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["available"], bool), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection.get("lookupContext"), dict), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["lookupContext"].get("region"), str) and ecs_task_resource_inspection["lookupContext"]["region"], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["lookupContext"].get("controlAccountId"), str) and ecs_task_resource_inspection["lookupContext"]["controlAccountId"], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["lookupContext"].get("targetAccountId"), str) and ecs_task_resource_inspection["lookupContext"]["targetAccountId"], ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["lookupContext"].get("crossAccount"), bool), ecs_task_resource_inspection
assert isinstance(ecs_task_resource_inspection["lookupContext"].get("observerRoleConfigured"), bool), ecs_task_resource_inspection
if ecs_task_resource_inspection["available"]:
    assert ecs_task_resource_inspection.get("source") == "ecs_task", ecs_task_resource_inspection
    assert isinstance(ecs_task_resource_inspection.get("ecsTask"), dict), ecs_task_resource_inspection
else:
    assert ecs_task_resource_inspection.get("unavailableReason") in {"aws_unavailable", "resource_not_found", "cross_account_not_configured"}, ecs_task_resource_inspection
assert os.environ["INVALID_RESOURCE_LOG_LIMIT_STATUS"] == "400", os.environ["INVALID_RESOURCE_LOG_LIMIT_STATUS"]
assert invalid_resource_log_limit.strip() == "invalid limit query", invalid_resource_log_limit
assert os.environ["INVALID_RESOURCE_INSPECTION_LIMIT_STATUS"] == "400", os.environ["INVALID_RESOURCE_INSPECTION_LIMIT_STATUS"]
assert invalid_resource_inspection_limit.strip() == "invalid limit query", invalid_resource_inspection_limit
assert os.environ["INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS"] == "400", os.environ["INVALID_RESOURCE_INSPECTION_INSTANCE_STATUS"]
assert invalid_resource_inspection_instance.strip() == "invalid instanceId query", invalid_resource_inspection_instance
assert selected_candidate_resource_inspection.get("instanceId") == "i-0abc123def4567890", selected_candidate_resource_inspection

assert codedeploy_application_resource_logs["deploymentName"] == "payments-api-staging", codedeploy_application_resource_logs
assert codedeploy_application_resource_logs["resourceType"] == "codeDeployApplication", codedeploy_application_resource_logs
assert codedeploy_application_resource_logs["resourceName"] == "payments-api-staging-codedeploy-app", codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs.get("lookupContext"), dict), codedeploy_application_resource_logs
assert isinstance(codedeploy_application_resource_logs["available"], bool), codedeploy_application_resource_logs
assert codedeploy_application_resource_logs.get("entries") is None or isinstance(codedeploy_application_resource_logs.get("entries"), list), codedeploy_application_resource_logs
if codedeploy_application_resource_logs["available"]:
    assert isinstance(codedeploy_application_resource_logs.get("source"), str) and codedeploy_application_resource_logs["source"], codedeploy_application_resource_logs
    for entry in codedeploy_application_resource_logs.get("entries") or []:
        assert isinstance(entry.get("timestamp"), str) and entry["timestamp"], codedeploy_application_resource_logs
        assert isinstance(entry.get("message"), str) and entry["message"], codedeploy_application_resource_logs
        if "stream" in entry:
            assert isinstance(entry.get("stream"), str) and entry["stream"], codedeploy_application_resource_logs
else:
    assert isinstance(codedeploy_application_resource_logs.get("unavailableReason"), str) and codedeploy_application_resource_logs["unavailableReason"], codedeploy_application_resource_logs

assert test_listener_resource_logs["deploymentName"] == "payments-api-staging", test_listener_resource_logs
assert test_listener_resource_logs["resourceType"] == "testListener", test_listener_resource_logs
assert test_listener_resource_logs["resourceName"] == "payments-api-staging-test-listener", test_listener_resource_logs
assert isinstance(test_listener_resource_logs.get("lookupContext"), dict), test_listener_resource_logs
assert isinstance(test_listener_resource_logs["available"], bool), test_listener_resource_logs
assert test_listener_resource_logs.get("backingResourceType") == "codeDeployDeploymentGroup", test_listener_resource_logs
assert test_listener_resource_logs.get("backingResourceName") == "payments-api-staging-codedeploy-group", test_listener_resource_logs
assert test_listener_resource_logs.get("entries") is None or isinstance(test_listener_resource_logs.get("entries"), list), test_listener_resource_logs
if test_listener_resource_logs["available"]:
    assert isinstance(test_listener_resource_logs.get("source"), str) and test_listener_resource_logs["source"], test_listener_resource_logs
    for entry in test_listener_resource_logs.get("entries") or []:
        assert isinstance(entry.get("timestamp"), str) and entry["timestamp"], test_listener_resource_logs
        assert isinstance(entry.get("message"), str) and entry["message"], test_listener_resource_logs
        if "stream" in entry:
            assert isinstance(entry.get("stream"), str) and entry["stream"], test_listener_resource_logs
else:
    assert isinstance(test_listener_resource_logs.get("unavailableReason"), str) and test_listener_resource_logs["unavailableReason"], test_listener_resource_logs

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
        assert entry.get("instanceId"), bootstrap_resource_logs
else:
    assert bootstrap_resource_logs.get("unavailableReason") in {"aws_unavailable", "no_log_source"}, bootstrap_resource_logs

assert selected_bootstrap_resource_logs["deploymentName"] == "payments-api-prod", selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs["resourceType"] == "userDataBootstrapSource", selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs["resourceName"] == "git:userdata/payments-api-prod.sh", selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get("instanceId") == "i-0abc123def4567890", selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get("backingResourceType") == "candidateAutoScalingGroup", selected_bootstrap_resource_logs
assert selected_bootstrap_resource_logs.get("backingResourceName") == "payments-api-prod-candidate", selected_bootstrap_resource_logs
assert isinstance(selected_bootstrap_resource_logs.get("lookupContext"), dict), selected_bootstrap_resource_logs
if selected_bootstrap_resource_logs["available"]:
    assert selected_bootstrap_resource_logs.get("source") == "ec2_console", selected_bootstrap_resource_logs
    for entry in selected_bootstrap_resource_logs.get("entries") or []:
        assert entry.get("instanceId") == "i-0abc123def4567890", selected_bootstrap_resource_logs
else:
    assert selected_bootstrap_resource_logs.get("unavailableReason") in {"aws_unavailable", "resource_not_found", "no_log_source"}, selected_bootstrap_resource_logs
assert os.environ["INVALID_RESOURCE_LOG_INSTANCE_STATUS"] == "400", os.environ["INVALID_RESOURCE_LOG_INSTANCE_STATUS"]
assert invalid_resource_log_instance.strip() == "invalid instanceId query", invalid_resource_log_instance

assert traffic_resource_logs["deploymentName"] == "payments-api-prod", traffic_resource_logs
assert traffic_resource_logs["resourceType"] == "trafficShiftPlan", traffic_resource_logs
assert traffic_resource_logs["resourceName"] == "payments-api-prod-weighted-routing", traffic_resource_logs
assert traffic_resource_logs["available"] is False, traffic_resource_logs
assert traffic_resource_logs.get("unavailableReason") == "no_log_source", traffic_resource_logs
assert isinstance(traffic_resource_logs.get("lookupContext"), dict), traffic_resource_logs
assert isinstance(traffic_resource_logs["lookupContext"].get("region"), str) and traffic_resource_logs["lookupContext"]["region"], traffic_resource_logs
assert isinstance(traffic_resource_logs["lookupContext"].get("controlAccountId"), str) and traffic_resource_logs["lookupContext"]["controlAccountId"], traffic_resource_logs
assert isinstance(traffic_resource_logs["lookupContext"].get("targetAccountId"), str) and traffic_resource_logs["lookupContext"]["targetAccountId"], traffic_resource_logs
assert isinstance(traffic_resource_logs["lookupContext"].get("crossAccount"), bool), traffic_resource_logs
assert isinstance(traffic_resource_logs["lookupContext"].get("observerRoleConfigured"), bool), traffic_resource_logs

for payload, resource_type, resource_name in (
    (production_listener_rule_resource_logs, "productionListenerRule", "payments-api-prod-prod-listener"),
    (active_target_group_resource_logs, "activeTargetGroup", "payments-api-prod-active"),
    (candidate_target_group_resource_logs, "candidateTargetGroup", "payments-api-prod-candidate"),
):
    assert payload["deploymentName"] == "payments-api-prod", payload
    assert payload["resourceType"] == resource_type, payload
    assert payload["resourceName"] == resource_name, payload
    assert payload["available"] is False, payload
    assert payload.get("unavailableReason") == "no_log_source", payload
    no_log_source = payload.get("resultScope")
    assert isinstance(no_log_source, dict), payload
    assert no_log_source.get("returnedEntryCount") == 0, payload
    assert no_log_source.get("truncationStatus") == "not_applicable", payload
    assert isinstance(payload.get("lookupContext"), dict), payload
    assert isinstance(payload["lookupContext"].get("region"), str) and payload["lookupContext"]["region"], payload
    assert isinstance(payload["lookupContext"].get("controlAccountId"), str) and payload["lookupContext"]["controlAccountId"], payload
    assert isinstance(payload["lookupContext"].get("targetAccountId"), str) and payload["lookupContext"]["targetAccountId"], payload
    assert isinstance(payload["lookupContext"].get("crossAccount"), bool), payload
    assert isinstance(payload["lookupContext"].get("observerRoleConfigured"), bool), payload

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

def assert_asg_resource_inspection(payload, *, expect_limit=None, expect_instance_id=None):
    assert payload["deploymentName"] == "payments-api-prod", payload
    assert payload["resourceType"] == "candidateAutoScalingGroup", payload
    assert payload["resourceName"] == "payments-api-prod-candidate", payload
    assert isinstance(payload["available"], bool), payload
    assert isinstance(payload.get("lookupContext"), dict), payload
    if expect_instance_id is not None:
        assert payload.get("instanceId") == expect_instance_id, payload
    if payload["available"]:
        assert payload.get("source") == "auto_scaling_group", payload
        asg = payload.get("autoScalingGroup")
        assert isinstance(asg, dict), payload
        assert isinstance(asg.get("autoScalingGroupName"), str) and asg["autoScalingGroupName"], payload
        launch_template = asg.get("launchTemplate")
        assert launch_template is None or isinstance(launch_template, dict), payload
        if isinstance(launch_template, dict):
            assert any(isinstance(launch_template.get(key), str) and launch_template.get(key) for key in ("id", "name", "version")), payload
        mixed_instances_policy = asg.get("mixedInstancesPolicy")
        assert mixed_instances_policy is None or isinstance(mixed_instances_policy, dict), payload
        if isinstance(mixed_instances_policy, dict):
            base_launch_template = mixed_instances_policy.get("launchTemplate")
            assert base_launch_template is None or isinstance(base_launch_template, dict), payload
            overrides = mixed_instances_policy.get("overrides")
            assert overrides is None or isinstance(overrides, list), payload
            for override in overrides or []:
                assert isinstance(override, dict), payload
                override_launch_template = override.get("launchTemplate")
                assert override_launch_template is None or isinstance(override_launch_template, dict), payload
        assert isinstance(asg.get("instances", []), list), payload
        if expect_instance_id is not None:
            for instance in asg.get("instances", []):
                assert instance.get("instanceId") == expect_instance_id, payload
        scope = asg.get("instancesScope")
        if expect_limit is not None:
            assert isinstance(scope, dict), payload
            assert isinstance(scope.get("requestedLimit"), int), payload
            assert isinstance(scope.get("effectiveLimit"), int), payload
            assert isinstance(scope.get("returnedInstanceCount"), int), payload
            assert scope.get("truncationStatus") in {"complete", "truncated", "unknown"}, payload
            assert scope.get("requestedLimit") == expect_limit, payload
            assert scope.get("effectiveLimit") <= expect_limit, payload
            assert scope.get("returnedInstanceCount") <= expect_limit, payload
        else:
            assert scope is None or isinstance(scope, dict), payload
    else:
        assert isinstance(payload.get("unavailableReason"), str) and payload["unavailableReason"], payload

assert_asg_resource_inspection(candidate_resource_inspection)
assert_asg_resource_inspection(bounded_candidate_resource_inspection, expect_limit=2)
assert_asg_resource_inspection(selected_candidate_resource_inspection, expect_instance_id="i-0abc123def4567890")

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

assert_summary_traffic_routing(codedeploy_application_resource_inspection, "payments-api-staging", "codeDeployApplication", "payments-api-staging-codedeploy-app")
assert_summary_traffic_routing(codedeploy_group_resource_inspection, "payments-api-staging", "codeDeployDeploymentGroup", "payments-api-staging-codedeploy-group")
assert_summary_traffic_routing(production_listener_resource_inspection, "payments-api-staging", "productionListener", "payments-api-staging-prod-listener")
assert_summary_traffic_routing(test_listener_resource_inspection, "payments-api-staging", "testListener", "payments-api-staging-test-listener")
assert_summary_traffic_routing(blue_task_set_target_group_resource_inspection, "payments-api-staging", "blueTaskSetTargetGroup", "payments-api-staging-blue")
assert_summary_traffic_routing(production_listener_rule_resource_inspection, "payments-api-prod", "productionListenerRule", "payments-api-prod-prod-listener")
assert_summary_traffic_routing(active_target_group_resource_inspection, "payments-api-prod", "activeTargetGroup", "payments-api-prod-active")
assert_summary_traffic_routing(candidate_target_group_resource_inspection, "payments-api-prod", "candidateTargetGroup", "payments-api-prod-candidate")
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
    "const operatorReadRoleRequiredState = (error) => operatorReadForbiddenState(error) && String(error?.message || '').includes('authenticated viewer-capable role is required for this operator read route');",
    "const operatorReadAuthRequiredMessage = (route, context = operatorContextState.context) => {",
    "${signInRuntime.message}; use the sign-in handoff below to authenticate through ${signInRuntime.loginRoute} and return to this same bookmarkable URL.",
    "Tenet does not start a built-in GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page; reopen this same bookmarkable URL through the trusted front door",
    "const operatorReadRoleRequiredMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity because this operator read route requires a trusted forwarded viewer/operator/admin role. Open Operator context details (#operator-context-access-details-panel) and check roles.provenance.source, roles.canRead, and the forwarded X-Tenet-Authenticated-Roles value; ask the front-door/RBAC admin to forward viewer, operator, or admin, then retry this same bookmarkable page.`;",
    "const operatorReadForbiddenMessage = (route) => `Tenet rejected ${route} after recognizing the caller's identity, but this operator is not authorized for that route or deployment. Open Operator context details (#operator-context-access-details-panel) and compare deploymentAccess.summary, access verdicts, stale/unmatched deployment/project/environment grants, and the required trusted headers for this route; ask an admin to grant the matching deployment, project, or environment scope, then retry this same bookmarkable page.`;",
    "const currentBookmarkableURL = (hashOverride) => {",
    "const operatorContextManagedLoginSignInURL = (context = operatorContextState.context) => {",
    "const operatorContextManagedLoginProviderLabel = (provider) => {",
    "google_oidc: 'Google Workspace OIDC'",
    "okta_saml: 'Okta SAML'",
    "Managed login provider boundary:",
    "Gmail mailbox access is not requested or authorized by this runtime.",
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
assert_fragments("ui", ui, (
    "const setEmbeddedBrowserTitle = (...parts) => {",
    "document.title = [...titleParts, 'Tenet'].join(' · ');",
    "setBrowserTitleFromPageState(currentPageState);",
    "setEmbeddedBrowserTitle(resource.name || pageState.resourceType, pageState.deployment, pageLabel);",
))
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
assert_fragments("ui", ui, (
    "Operator context",
    "const operatorContextRoute = () => '/v1/operator/context';",
    "const operatorContextReadRoute = () => 'GET /v1/operator/context';",
    "const operatorContextDeploymentAccessSummaryLabel = (context) => {",
    "const operatorContextProjectEnvironmentExamplesLabel = (context) => {",
    "Project/environment visibility:",
    "projectEnvironmentExamples",
    "const operatorContextVisibleDeploymentSourceLabel = (deploymentName, example, access) => {",
    "const operatorContextVisibleDeploymentExamplesLabel = (context) => {",
    "Visible deployment examples:",
    "via exact deployment grant + project/environment scope",
    "via project/environment scope",
    "const operatorContextDesiredStateSourceEntries = (context) => {",
    "Desired-state source:",
    "Controller refresh:",
    "Webhook match statuses",
    "Webhook ignored reasons",
    "Diagnostic only; no webhook is sent from this page.",
    "operatorContextDesiredStateSourceEntries(context)",
    "const operatorContextEnforcementModeLabel = (context) => {",
    "Enforcement mode:",
    "read/write guarded",
    "local unrestricted",
    "const operatorContextAccessVerdictExampleRoutesLabel = (verdict) => {",
    "const visibleRoutes = routes.slice(0, 2).join('; ');",
    "const hiddenCount = routes.length - 2;",
    "const operatorContextAccessVerdictRequiredHeadersLabel = (verdict) => {",
    "const headers = Array.isArray(verdict?.requiredTrustedHeaders) ? verdict.requiredTrustedHeaders.filter(Boolean) : [];",
    "return ` Required trusted headers: ${headers.join('; ')}`;",
    "requiredTrustedHeaders",
    "Required trusted headers:",
    "const operatorContextAccessVerdictScopeGrantLabel = (verdict) => verdict?.acceptsProjectEnvironmentScopes",
    "Scope grants: exact deployments or matching project/environment scopes",
    "acceptsProjectEnvironmentScopes",
    "const operatorContextAccessVerdictCollectionFilterLabel = (verdict) => verdict?.filtersCollectionsByDeploymentAccess",
    "filtersCollectionsByDeploymentAccess",
    "const operatorContextAccessVerdictEmptyCollectionLabel = (verdict) => verdict?.emptyCollectionLikely",
    "const headers = operatorContextAccessVerdictRequiredHeadersLabel(verdict);",
    "const collectionFilter = operatorContextAccessVerdictCollectionFilterLabel(verdict);",
    "const emptyCollection = operatorContextAccessVerdictEmptyCollectionLabel(verdict);",
    "const scopeGrants = operatorContextAccessVerdictScopeGrantLabel(verdict);",
    "return `${label} ${state}${reason}${examples}${headers}${collectionFilter}${emptyCollection}${scopeGrants}`;",
    "const operatorContextAccessVerdictEntries = (context) => {",
    "Operator reads",
    "Collection reads",
    "Examples:",
    "operator-context-denied",
    "Collection scope: filtered to visible deployments",
    "Collection result: likely empty",
    "emptyCollectionLikely",
    "Deployment access summary unavailable for this response",
    "forwarded match catalog",
    "unmatched",
    "loadOperatorContextBanner().catch(() => {});",
    "loadOperatorContextBanner({ refresh: true });",
    "Refresh context",
    "Last checked ${timestamp.toLocaleString()}",
    "lastCheckedAt: new Date()",
    "id=\"operator-context-status\" role=\"status\" aria-live=\"polite\" aria-atomic=\"true\"",
    ".operator-context-live { position: absolute;",
    "liveStatus.setAttribute('aria-live', 'polite');",
    "statusAnnouncement = `Operator context ready: ${summary.textContent}`;",
    "statusAnnouncement = 'Refreshing trusted front-door context…';",
    "statusAnnouncement = `Operator context refresh failed: ${failureMessage}`;",
    "Show access details",
    "Hide access details",
    "Role header source: ${labels[source] || labels.absent}",
    "Supported Tenet roles: ${supported.join(', ')}",
    "operatorContextSupportedRolesLabel(context)",
    "const operatorContextSupportedIdentityHeadersLabel = (context) => {",
    "Supported identity headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Canonical X-Tenet-Authenticated-* identity headers are preferred when both canonical and alias values are present.",
    "const operatorContextManagedLogin = (context) => context?.authRuntime?.tenetManagedLogin || {};",
    "const operatorContextManagedLoginActive = (context = operatorContextState.context) => {",
    "Managed login lifecycle: OAuth callback state TTL",
    "Managed login provider token boundary: access token stored",
    "Managed login logout boundary: clears Tenet session cookie",
    "Authentication boundary: Tenet-managed browser login is active for this API instance; guarded read recovery may offer a same-shell sign-in handoff while trusted front-door headers still take precedence when present.",
    "Authentication boundary: this embedded shell consumes trusted front-door headers only; it does not start an in-page GitHub OAuth login, enterprise SSO login, or Tenet-managed browser session from this page unless the optional login/callback/session routes report active.",
    "operatorContextAuthenticationBoundaryLabel(context)",
    "const operatorContextManagedLoginStateLabel = (context) => {",
    "Managed login runtime:",
    "Managed login missing settings:",
    "Managed login storage/session:",
    "const operatorContextManagedLoginCookieAttributesLabel = (context) => {",
    "Managed login cookie attributes:",
    "local HTTP without TLS or forwarded HTTPS evidence is expected for development",
    "configured secure-cookie override",
    "X-Forwarded-Proto=https",
    "operatorContextManagedLoginClass(context)",
    "operatorContextSupportedIdentityHeadersLabel(context)",
    "const operatorContextSupportedActorHeadersLabel = (context) => {",
    "Supported actor attribution headers: canonical ${canonicalText}; compatibility aliases ${aliasText}. Explicit X-Tenet-Actor-* actor headers win over X-User-* actor aliases; trusted authenticated identity is projected only when explicit actor context is absent; actor attribution records persisted plan/apply/audit metadata and does not authorize routes.",
    "operatorContextSupportedActorHeadersLabel(context)",
    "Actor header sources: ${entries.join('; ')}",
    "Actor attribution: ${status}. ${summary}",
    "operatorContextActorAttributionLabel(context)",
    "const operatorContextSupportedRoleHeadersLabel = (context) => {",
    "Supported role headers: ${supportedHeaders.join(', ')} (role forwarding has no X-User-* alias)",
    "operatorContextSupportedRoleHeadersLabel(context)",
    "const operatorContextSupportedDeploymentAccessHeadersLabel = (context) => {",
    "Supported deployment access headers: ${supportedHeaders.join(', ')}",
    "operatorContextSupportedDeploymentAccessHeadersLabel(context)",
    "canonical X-Tenet-Authenticated-Roles",
    "Recognized capability roles:",
    "Unsupported forwarded roles ignored for capability checks:",
    "for read capability; forward operator or admin for write capability.",
    "operatorContextRoleProvenanceClass",
    "operator-context-details",
    "Last refresh failed:",
    "const operatorContextFailureHeading = (error) => operatorReadFailureHeading(error);",
    "const operatorContextFailureMessage = (error) => {",
    "const message = operatorReadFailureMessage(operatorContextReadRoute(), error);",
    "return message && message !== 'unknown error' ? message : 'Tenet could not load trusted front-door context.';",
    "summary.textContent = operatorContextFailureHeading(operatorContextState.error) ||",
    "fetchApplicationsOverview()",
    "const applicationsOverviewReadRoute = () => 'GET /v1/overview/applications';",
    "const createApplicationsOverviewActions = (overviewState = null) => [",
    "'Refresh applications overview'",
    "overviewState?.status === 'loading'",
    "() => overviewState?.onRefresh?.().catch(() => {})",
    "const applicationsOverviewRefreshState = (overviewState = null) => {",
    "'Refreshing applications overview from GET /v1/overview/applications…'",
    "Current applications overview remains visible below.",
    "Failed to refresh applications overview:",
    "Visible applications after filters:",
    "Visible subset last update",
    "const latestRuntimeSignalParts = (deployment) => {",
    "Latest runtime signal",
    "buildPageSectionURL('application', 'application-runtime-events', { deployment: deployment.name })",
    "createApplicationsOverviewActions(applicationsOverviewState).forEach((action) => {",
    "const refreshState = applicationsOverviewRefreshState(applicationsOverviewState);",
    "actions.append(createNavLink(buildPageURL('audit'), 'Open audit log', '_self'));",
))
assert "const fetchAccountDetail = (accountID) => fetchEmbeddedReadRouteJSON(`/v1/overview/accounts/${encodeURIComponent(accountID)}`, {" in account_ui, account_ui
assert "fetchAccountDetail(currentPageState.accountId)" in account_ui, account_ui
assert_fragments("account_ui", account_ui, (
    "Quick navigation",
    "Applications overview",
    "Current destination",
    "Current section",
    "No section is currently targeted.",
    "On this page",
    "createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
    "createQuickNavChip('Account detail', 'Current destination: Account detail')",
    "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
    "detailSummary.id = 'account-summary';",
    "const accountGroupSectionLabel = (group) => {",
    "return region === '—' ? `${app} · ${environment}` : `${app} · ${environment} · ${region}`;",
    "groupSection.id = accountGroupSectionID(group);",
    "href: buildPageSectionURL('account', sectionID, { accountId: accountID }),",
))
for label, body in (("resource_ui", resource_ui), ("ecs_resource_ui", ecs_resource_ui), ("bootstrap_resource_ui", bootstrap_resource_ui), ("codedeploy_application_resource_ui", codedeploy_application_resource_ui), ("blue_task_set_target_group_resource_ui", blue_task_set_target_group_resource_ui), ("production_listener_rule_resource_ui", production_listener_rule_resource_ui)):
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
    "{ value: 'plan.created', label: 'Plan preview created (plan.created)' },",
    "{ value: 'apply.requested', label: 'Apply requested (apply.requested)' },",
    "const auditActionLabel = (value) => {",
    "renderAuditLogsState(operatorReadFailureHeading(error), 'error', operatorReadFailureMessage(buildEmbeddedPageReadRoute(currentPageState), error), [createOperatorReadFailureActions(error)]);",
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
    "Managed drift summary · ${counts.total} total · ${counts.high} high · ${counts.medium} medium · ${counts.low} low",
    "const createControllerRefreshCue = (event) => {",
    "GitHub refresh evidence · ${fields.join(' · ')}",
    "Recent logs",
    "Requested cap",
    "Effective cap",
    "Returned entries",
    "Scope status",
    "Capped response; more rows may exist",
    "At cap; provider totals unknown",
    "Resource description",
    "Selected container",
    "Log container",
    "Open container recent logs",
    "params.set('containerName', containerName);",
    "Lookup path ·",
    "Lookup region",
    "Observer role configured",
    "This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.",
    "const resourceLogsEmptyReasonText = (reason) => {",
    "Tenet reached the recent-log provider, but no events were found in the selected bounded window.",
    "Tenet found provider records, but none could be formatted into operator-visible recent-log rows.",
    "Resolved backing resource",
    "actions.push(createNavLink(buildPageSectionURL('drift', 'drift-field-level', { deployment: deploymentName }), 'Open field-level drift', '_self'));",
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
    "const buildResourceLogsRoute = (deploymentName, resource, limit = 0) => {",
    "const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;",
    "const buildResourceInspectionRoute = (deploymentName, resource, limit = 0) => {",
    "const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;",
    "const createResourceInspectionLimitControl = (section, detail, resource, isLoading = false) => {",
    "resourceInspectionLimitLabel(resource)",
    "'Task sample limit'",
    "'ASG instance sample limit'",
    "supportsBoundedResourceInspection(resource)",
    "const ecsTaskScopeUnavailableReasonText = (reason) => {",
    "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}",
    "Task sampling is unavailable for this service; service inspection remains available.",
    "const createECSTaskScopeSummary = (scope) => {",
    "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}",
    "[createECSTaskScopeSummary(service?.tasksScope)]",
    "const createAutoScalingInstanceScopeSummary = (scope) => {",
    "ASG instance sample scope · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)} · returned ${countText(returnedInstanceCount)} instance(s) · ${truncationStatus}",
    "ASG instance sampling is unavailable for this group; group inspection remains available.",
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
    "Visible high",
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
    "Current section",
    "No section is currently targeted.",
    "On this page",
    "Loading audit log…",
    "Audit log is currently unavailable.",
    "Immutable operator audit log from GET /v1/audit-logs. Newest entries first. Actor identity remains explicit when unavailable or unknown.",
    "Immutable plan preview and apply-request activity for deployment ${deploymentName} rendered from GET /v1/audit-logs.",
    "Current audit read: ${buildEmbeddedPageReadRoute(currentPageState)}",
    "No audit activity has been persisted yet.",
    "const auditFilterLabels = () => {",
    "const auditEmptyStateText = () => {",
    "No audit entries match the current filters (${filters.join(' · ')}${hasLimit ? `; ${auditLimitLabel()}` : ''}). Clear audit controls to return to the full audit timeline.",
    "labels.push(`action ${auditActionLabel(currentPageState.auditAction)}`);",
    "const fetchAuditLogs = ({ deploymentName = '', action = '', limit = 0 } = {}) => {",
    "if (action) params.set('action', action);",
    "createSelect('Action', currentPageState.auditAction, auditActionOptions, (event) => {",
    "const deploymentName = auditDeploymentName();",
    "? createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
    ": createNavLink(buildPageURL('applications'), 'Back to applications', '_self')",
    "const clearAuditPageFilters = () => {",
    "const createAuditQuickNav = (sectionLinks = []) => createDeploymentQuickNav(",
    "const deploymentContextSectionID = 'audit-deployment-context';",
    "deploymentCallout.setAttribute('id', deploymentContextSectionID);",
    "const timelineSectionID = 'audit-timeline';",
    "timelineSection.setAttribute('id', timelineSectionID);",
    "buildPageSectionURL('audit', sectionID, {",
):
    assert fragment in audit_ui, fragment

assert_fragments("application_ui", application_ui, operator_read_auth_fragments + (
    "return 'Application detail';",
    "return `GET /v1/overview/applications/${deploymentName}`;",
    "const renderOperatorReadFailurePage = (pageLabel, route, error) => {",
    "renderOperatorReadFailurePage(embeddedPageTitle(currentPageState.page), buildEmbeddedPageReadRoute(currentPageState), error);",
))

for fragment in (
    "const fetchApplicationDetail = (deploymentName) => fetchEmbeddedReadRouteJSON(`/v1/overview/applications/${encodeURIComponent(deploymentName)}`, {",
    "const applicationDetailReadRoute = (deploymentName) => `GET /v1/overview/applications/${deploymentName || '{deploymentName}'}`;",
    "const applicationStatusSignal = (detail) => {",
    "const applicationRuntimeEventSummary = (detail) => {",
    "const applicationManagedDriftCounts = (detail) => {",
    "const applicationManagedDriftSummaryText = (detail) => {",
    "No managed drift reported",
    "const createApplicationStatusSummary = (detail, resourceGroups = []) => {",
    "const createApplicationDetailActions = (applicationState = null) => [",
    "'Refresh application detail'",
    "const applicationDetailRefreshState = (applicationState = null) => {",
    "'Refreshing application detail from GET /v1/overview/applications/{deploymentName}…'",
    "Current application detail remains visible below.",
    "Failed to refresh application detail:",
    "const applicationDetail = currentPageState.page === 'application' ? fetchApplicationDetail(currentPageState.deployment) : fetchDeploymentDetail(currentPageState.deployment);",
    "const renderApplicationDetailPage = (detail, applicationState = null) => {",
    "renderApplicationDetailPage(applicationState.detail, applicationState);",
    "Current application status",
    "Application rollout context:",
    "['Managed drift', applicationManagedDriftSummaryText(detail)],",
    "['Runtime events', applicationRuntimeEventSummary(detail)],",
    "page.append(createApplicationStatusSummary(detail, resourceGroups));",
    "createNavLink(buildPageURL('resource', { deployment: detail.name, resourceType: resource.type, resourceId: resource.id }), 'Open resource', '_self')",
    "const resourceSupportsLiveInspection = (resource) => {",
    "const resourceSupportsRecentLogs = (resource) => {",
    "createNavLink(buildPageSectionURL('resource', 'resource-live-inspection', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Open live inspection', '_self')",
    "createNavLink(buildPageSectionURL('resource', 'resource-recent-logs', { deployment: deploymentName, resourceType: resource.type, resourceId: resource.id }), 'Open recent logs', '_self')",
    "Application resources",
):
    assert fragment in application_ui, fragment

assert_fragments("application_ui", application_ui, (
    "Quick navigation",
    "Applications overview",
    "Current destination",
    "Current section",
    "No section is currently targeted.",
    "createNavLink(buildPageURL('applications'), 'Applications overview', '_self')",
    "createQuickNavChip('Application detail', 'Current destination: Application detail')",
))

for body in (resource_ui, ecs_resource_ui, bootstrap_resource_ui, codedeploy_application_resource_ui, blue_task_set_target_group_resource_ui, production_listener_rule_resource_ui):
    for fragment in (
        "const bookmarkedResource = parseResourceIdentity(currentPageState.resourceId);",
        "Opened via bookmarked resource alias.",
        "const renderResourceDetailPage = (detail, resourceState = null) => {",
        "const resourceDetailReadRoute = (deploymentName) => `GET /v1/overview/deployments/${deploymentName || '{deploymentName}'}`;",
        "const createResourceDetailActions = (resourceState = null) => [",
        "Refresh resource detail",
        "Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…",
        "Current resource detail remains visible below.",
        "Failed to refresh resource detail:",
        "renderResourceDetailPage(resourceState.detail, resourceState);",
        "return fetchDeploymentDetail(deploymentName)",
        "const createResourceQuickNav = (detail, resource, sectionLinks = []) => {",
        "createNavLink(buildPageURL('application', { deployment: deploymentName }), 'Back to application', '_self')",
        "createNavLink(buildPageURL('resource', {",
        "containerName: resourceRouteState.routeContainerName,",
        "routeContainerName: String(currentPageState.containerName || resource.containerName || '').trim()",
        "containerName: params.get('containerName') || ''",
        "buildPageSectionURL('resource', 'resource-recent-logs'",
        "Quick navigation",
        "Current section",
        "createQuickNavChip(currentSectionLink.label, `Current section: ${currentSectionLink.label}`)",
        "No section is currently targeted.",
        "No additional section links are available for this resource.",
    ):
        assert fragment in body, fragment
    assert_fragments("resource_ui", body, operator_read_auth_fragments + (
        "const createResourceLogsActions = (section, deploymentName, resource, isLoading = false, detail = null) => [",
        "createResourceLogsLimitControl(section, deploymentName, resource, isLoading, detail)",
        "'Log limit'",
        "'Refresh recent logs'",
        "const buildResourceLogsRoute = (deploymentName, resource, limit = 0) => {",
        "const resourceLogsReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceLogsRoute(deploymentName, resource, limit)}`;",
        "renderResourceLogsState(section, 'Loading recent logs…', '', [createResourceLogsLimitNote(selectedLimit)], [], loadingActions);",
        "Log entry cap · ${resourceLogsLimitText(limit)}",
        "params.set('limit', String(normalizedLimit));",
        "resourceLogLimit: normalizeResourceLogsLimit(params.get('resourceLogLimit') || '')",
        "params.set('resourceLogLimit', String(limit));",
        "syncResourcePanelLimitQueryParams(deploymentName, resource);",
        "Refresh recent logs",
        "renderOperatorReadFailurePanel(",
        "'Recent logs'",
        "resourceLogsReadRoute(deploymentName, resource, target?.limit ?? selectedLimit)",
        "`Failed to load recent logs: ${error.message}`",
        "const createResourceInspectionActions = (section, detail, resource, isLoading = false) => [",
        "createResourceInspectionLimitControl(section, detail, resource, isLoading)",
        "'Task sample limit'",
        "'ASG instance sample limit'",
        "resourceInspectionLimit: normalizeResourceInspectionTaskLimit(params.get('resourceInspectionLimit') || '')",
        "params.set('resourceInspectionLimit', String(limit));",
        "resourceInspectionLimitLabel(resource)",
        "supportsBoundedResourceInspection(resource)",
        "'Refresh live inspection'",
        "const buildResourceInspectionRoute = (deploymentName, resource, limit = 0) => {",
        "const resourceInspectionReadRoute = (deploymentName, resource, limit = 0) => `GET ${buildResourceInspectionRoute(deploymentName, resource, limit)}`;",
        "const ecsTaskScopeUnavailableReasonText = (reason) => {",
        "Task sample unavailable · ${reasonText} · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)}",
        "Task sampling is unavailable for this service; service inspection remains available.",
        "const createECSTaskScopeSummary = (scope) => {",
                    "Task sample scope · returned ${countText(returnedTaskCount)} task(s) from effective limit ${countText(effectiveLimit)} · ${truncationStatus}",
            "[createECSTaskScopeSummary(service?.tasksScope)]",
        "const createAutoScalingInstanceScopeSummary = (scope) => {",
        "ASG instance sample scope · requested ${countText(scope.requestedLimit)} · effective ${countText(effectiveLimit)} · returned ${countText(returnedInstanceCount)} instance(s) · ${truncationStatus}",
        "ASG instance sampling is unavailable for this group; group inspection remains available.",
        "renderResourceInspectionState(section, 'Loading live inspection…', '', [], [], loadingActions);",
        "`Failed to load live inspection: ${error.message}`",
        "renderOperatorReadFailurePanel(",
        "'Live inspection'",
        "resourceInspectionReadRoute(deploymentName, resource, targetLimit)",
        "const autoScalingGroupSupportsInstanceHandoff = (resource) => ['candidateAutoScalingGroup', 'serviceAutoScalingGroup'].includes(String(resource?.type || '').trim());",
        "const autoScalingGroupRecentLogsHandoff = (deploymentName, resource) => {",
        "Open EC2 console logs",
        "const createAutoScalingInstanceHandoffLinks = (deploymentName, resource, instance) => {",
        "Inspect this instance",
        "Open instance logs",
        "buildPageSectionURL('resource', 'resource-recent-logs'",
    ))

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
  if assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$codedeploy_application_resource_logs_request_id" "$audit_logs_request_id" "$application_ui_request_id" "$account_ui_request_id" "$drift_ui_request_id" "$audit_ui_request_id" "$plan_apply_ui_request_id" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! assert_structured_api_request_logs "$api_logs" "$healthz_request_id" "$summary_request_id" "$worker_resource_logs_request_id" "$codedeploy_application_resource_logs_request_id" "$audit_logs_request_id" "$application_ui_request_id" "$account_ui_request_id" "$drift_ui_request_id" "$audit_ui_request_id" "$plan_apply_ui_request_id"; then
  echo "smoke-docker: api logs missing correlated structured request entries" >&2
  exit 1
fi

restart_api_with_frontdoor_read_auth

restarted_plan_preview=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/plans/$plan_id")
restarted_apply_record=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/applies/$apply_id")

auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/overview/applications")
auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/status/deployments")
auth_unauthenticated_audit_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/audit-logs?action=apply.requested")
operator_context_trusted=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/operator/context")
operator_context_combined_access=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: prod-apne2" \
  "$API_BASE_URL/v1/operator/context")
operator_context_unmatched=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, stale-api, worker-prod" \
  "$API_BASE_URL/v1/operator/context")
operator_context_stale_scopes=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer, operator" \
  -H "X-Tenet-Authorized-Projects: payments-api, archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2, stale-env" \
  "$API_BASE_URL/v1/operator/context")
operator_context_alias=$(curl -fsS \
  -H "X-User-ID: alias-456" \
  -H "X-User-Name: Alias Operator" \
  -H "X-User-Email: alias@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  "$API_BASE_URL/v1/operator/context")
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
  "$API_BASE_URL/v1/operator/context")
operator_context_no_allowlist=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: no-allowlist-789" \
  -H "X-Tenet-Authenticated-Name: No Allowlist Operator" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  "$API_BASE_URL/v1/operator/context")
operator_context_no_roles=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: no-roles-321" \
  -H "X-Tenet-Authenticated-Name: Missing Role Operator" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/operator/context")
operator_context_unsupported_roles=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: mixed-roles-654" \
  -H "X-Tenet-Authenticated-Name: Mixed Role Operator" \
  -H "X-Tenet-Authenticated-Roles: viewer, auditor" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/operator/context")
auth_unauthenticated_plan_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/plans/$plan_id")
auth_unauthenticated_apply_read_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" -w "%{http_code}" \
  "$API_BASE_URL/v1/applies/$apply_id")
auth_role_required_collection_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_COLLECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/overview/applications")
auth_role_required_status_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "$API_BASE_URL/v1/status/deployments/payments-api-staging")
auth_role_required_audit_collection_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod" \
  "$API_BASE_URL/v1/audit-logs?action=apply.requested")
auth_role_required_plan_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/plans/$plan_id")
auth_role_required_apply_read_status=$(curl -sS -o "$AUTH_ROLE_REQUIRED_APPLY_READ_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod" \
  "$API_BASE_URL/v1/applies/$apply_id")
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
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "$API_BASE_URL/v1/status/deployments/payments-api-staging")
auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  "$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")
auth_forbidden_resource_logs_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_resource_inspection_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
auth_forbidden_transition_status=$(curl -sS -o "$AUTH_FORBIDDEN_TRANSITION_BODY" -w "%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: operator" \
  -H "X-Tenet-Authorized-Deployments: worker-prod" \
  --data '{"status":"running","step":1}' \
  "$API_BASE_URL/v1/applies/$apply_id/transitions")
allowed_status_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "$API_BASE_URL/v1/status/deployments/payments-api-staging")
allowed_events_read=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: payments-api" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")
allowed_resource_logs=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")
allowed_resource_inspection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: worker-prod, payments-api-staging" \
  "$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")
filtered_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/overview/applications")
filtered_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod" \
  "$API_BASE_URL/v1/status/deployments")
filtered_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Deployments: payments-api-prod, payments-api-staging" \
  "$API_BASE_URL/v1/audit-logs")
empty_applications_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "$API_BASE_URL/v1/overview/applications")
empty_status_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
  "$API_BASE_URL/v1/status/deployments")
empty_audit_collection=$(curl -fsS \
  -H "X-Tenet-Authenticated-Id: front-door-123" \
  -H "X-Tenet-Authenticated-Name: Front Door Operator" \
  -H "X-Tenet-Authenticated-Email: frontdoor@example.com" \
  -H "X-Tenet-Authenticated-Roles: viewer" \
  -H "X-Tenet-Authorized-Projects: archived-payments" \
  -H "X-Tenet-Authorized-Environments: staging-apne2" \
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
operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")
operator_context_combined_access_path=$(write_payload operator-context-combined-access.json "$operator_context_combined_access")
operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")
operator_context_stale_scopes_path=$(write_payload operator-context-stale-scopes.json "$operator_context_stale_scopes")
operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")
operator_context_actor_precedence_path=$(write_payload operator-context-actor-precedence.json "$operator_context_actor_precedence")
operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")
operator_context_no_roles_path=$(write_payload operator-context-no-roles.json "$operator_context_no_roles")
operator_context_unsupported_roles_path=$(write_payload operator-context-unsupported-roles.json "$operator_context_unsupported_roles")

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
AUTH_UNAUTHENTICATED_PLAN_READ_STATUS="$auth_unauthenticated_plan_read_status" \
AUTH_UNAUTHENTICATED_PLAN_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_PLAN_READ_BODY" \
AUTH_UNAUTHENTICATED_APPLY_READ_STATUS="$auth_unauthenticated_apply_read_status" \
AUTH_UNAUTHENTICATED_APPLY_READ_BODY_PATH="$AUTH_UNAUTHENTICATED_APPLY_READ_BODY" \
AUTH_ROLE_REQUIRED_COLLECTION_STATUS="$auth_role_required_collection_status" \
AUTH_ROLE_REQUIRED_COLLECTION_BODY_PATH="$AUTH_ROLE_REQUIRED_COLLECTION_BODY" \
AUTH_ROLE_REQUIRED_STATUS_READ_STATUS="$auth_role_required_status_read_status" \
AUTH_ROLE_REQUIRED_STATUS_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_STATUS_READ_BODY" \
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS="$auth_role_required_audit_collection_status" \
AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY_PATH="$AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY" \
AUTH_ROLE_REQUIRED_PLAN_READ_STATUS="$auth_role_required_plan_read_status" \
AUTH_ROLE_REQUIRED_PLAN_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_PLAN_READ_BODY" \
AUTH_ROLE_REQUIRED_APPLY_READ_STATUS="$auth_role_required_apply_read_status" \
AUTH_ROLE_REQUIRED_APPLY_READ_BODY_PATH="$AUTH_ROLE_REQUIRED_APPLY_READ_BODY" \
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
OPERATOR_CONTEXT_TRUSTED_PATH="$operator_context_trusted_path" \
OPERATOR_CONTEXT_COMBINED_ACCESS_PATH="$operator_context_combined_access_path" \
OPERATOR_CONTEXT_UNMATCHED_PATH="$operator_context_unmatched_path" \
OPERATOR_CONTEXT_STALE_SCOPES_PATH="$operator_context_stale_scopes_path" \
OPERATOR_CONTEXT_ALIAS_PATH="$operator_context_alias_path" \
OPERATOR_CONTEXT_ACTOR_PRECEDENCE_PATH="$operator_context_actor_precedence_path" \
OPERATOR_CONTEXT_NO_ALLOWLIST_PATH="$operator_context_no_allowlist_path" \
OPERATOR_CONTEXT_NO_ROLES_PATH="$operator_context_no_roles_path" \
OPERATOR_CONTEXT_UNSUPPORTED_ROLES_PATH="$operator_context_unsupported_roles_path" \
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
with open(os.environ["AUTH_UNAUTHENTICATED_PLAN_READ_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_plan_read_body = fh.read()
with open(os.environ["AUTH_UNAUTHENTICATED_APPLY_READ_BODY_PATH"], encoding="utf-8") as fh:
    unauthenticated_apply_read_body = fh.read()
with open(os.environ["AUTH_ROLE_REQUIRED_COLLECTION_BODY_PATH"], encoding="utf-8") as fh:
    role_required_collection_body = fh.read()
with open(os.environ["AUTH_ROLE_REQUIRED_STATUS_READ_BODY_PATH"], encoding="utf-8") as fh:
    role_required_status_read_body = fh.read()
with open(os.environ["AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY_PATH"], encoding="utf-8") as fh:
    role_required_audit_collection_body = fh.read()
with open(os.environ["AUTH_ROLE_REQUIRED_PLAN_READ_BODY_PATH"], encoding="utf-8") as fh:
    role_required_plan_read_body = fh.read()
with open(os.environ["AUTH_ROLE_REQUIRED_APPLY_READ_BODY_PATH"], encoding="utf-8") as fh:
    role_required_apply_read_body = fh.read()
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
with open(os.environ["OPERATOR_CONTEXT_TRUSTED_PATH"], encoding="utf-8") as fh:
    operator_context_trusted = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_COMBINED_ACCESS_PATH"], encoding="utf-8") as fh:
    operator_context_combined_access = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_UNMATCHED_PATH"], encoding="utf-8") as fh:
    operator_context_unmatched = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_STALE_SCOPES_PATH"], encoding="utf-8") as fh:
    operator_context_stale_scopes = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_ALIAS_PATH"], encoding="utf-8") as fh:
    operator_context_alias = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_ACTOR_PRECEDENCE_PATH"], encoding="utf-8") as fh:
    operator_context_actor_precedence = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_NO_ALLOWLIST_PATH"], encoding="utf-8") as fh:
    operator_context_no_allowlist = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_NO_ROLES_PATH"], encoding="utf-8") as fh:
    operator_context_no_roles = json.load(fh)
with open(os.environ["OPERATOR_CONTEXT_UNSUPPORTED_ROLES_PATH"], encoding="utf-8") as fh:
    operator_context_unsupported_roles = json.load(fh)
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
assert os.environ["AUTH_UNAUTHENTICATED_PLAN_READ_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_PLAN_READ_STATUS"]
assert unauthenticated_plan_read_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_plan_read_body
assert os.environ["AUTH_UNAUTHENTICATED_APPLY_READ_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_APPLY_READ_STATUS"]
assert unauthenticated_apply_read_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_apply_read_body
assert os.environ["AUTH_ROLE_REQUIRED_COLLECTION_STATUS"] == "403", os.environ["AUTH_ROLE_REQUIRED_COLLECTION_STATUS"]
assert role_required_collection_body.strip() == "forbidden: authenticated viewer-capable role is required for this operator read route", role_required_collection_body
assert os.environ["AUTH_ROLE_REQUIRED_STATUS_READ_STATUS"] == "403", os.environ["AUTH_ROLE_REQUIRED_STATUS_READ_STATUS"]
assert role_required_status_read_body.strip() == "forbidden: authenticated viewer-capable role is required for this operator read route", role_required_status_read_body
assert os.environ["AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS"] == "403", os.environ["AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_STATUS"]
assert role_required_audit_collection_body.strip() == "forbidden: authenticated viewer-capable role is required for this operator read route", role_required_audit_collection_body
assert os.environ["AUTH_ROLE_REQUIRED_PLAN_READ_STATUS"] == "403", os.environ["AUTH_ROLE_REQUIRED_PLAN_READ_STATUS"]
assert role_required_plan_read_body.strip() == "forbidden: authenticated viewer-capable role is required for this operator read route", role_required_plan_read_body
assert os.environ["AUTH_ROLE_REQUIRED_APPLY_READ_STATUS"] == "403", os.environ["AUTH_ROLE_REQUIRED_APPLY_READ_STATUS"]
assert role_required_apply_read_body.strip() == "forbidden: authenticated viewer-capable role is required for this operator read route", role_required_apply_read_body
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

def assert_operator_context_project_environment_examples(examples, payload):
    assert 1 <= len(examples) <= 5, payload
    for row in examples:
        assert isinstance(row.get("appRef"), str) and row["appRef"], payload
        assert isinstance(row.get("environmentRef"), str) and row["environmentRef"], payload
        assert isinstance(row.get("region"), str) and row["region"], payload
        assert isinstance(row.get("awsAccountId"), str) and row["awsAccountId"], payload
        assert isinstance(row.get("deploymentCount"), int) and row["deploymentCount"] > 0, payload
        example_deployments = row.get("exampleDeployments") or []
        assert len(example_deployments) <= row["deploymentCount"], payload
        assert all(isinstance(name, str) and name for name in example_deployments), payload


def assert_operator_context_common(payload):
    trusted = payload.get("trustedHeaders") or {}
    assert trusted.get("enabled") is True, payload
    assert trusted.get("authenticatedIdentityRequired") is True, payload
    assert trusted.get("deploymentReadAuthorizationEnforced") is True, payload
    assert trusted.get("operatorReadRoleEnforced") is True, payload
    assert trusted.get("operatorWriteRoleEnforced") is True, payload
    enforcement = payload.get("enforcement") or {}
    assert enforcement.get("mode") == "trusted_front_door_read_write", payload
    assert enforcement.get("trustedHeadersEnabled") is True, payload
    assert enforcement.get("readRoutesGuarded") is True, payload
    assert enforcement.get("writeRoutesGuarded") is True, payload
    assert enforcement.get("deploymentScopedRoutesRequireAllowlist") is True, payload
    assert enforcement.get("collectionReadsFiltered") is True, payload
    access_verdicts = payload.get("accessVerdicts") or {}
    identity_headers = ["X-Tenet-Authenticated-Id", "X-User-ID"]
    canonical_identity_headers = ["X-Tenet-Authenticated-Id", "X-Tenet-Authenticated-Name", "X-Tenet-Authenticated-Email"]
    alias_identity_headers = ["X-User-ID", "X-User-Name", "X-User-Email"]
    canonical_actor_headers = ["X-Tenet-Actor-Id", "X-Tenet-Actor-Name", "X-Tenet-Actor-Email"]
    alias_actor_headers = ["X-User-ID", "X-User-Name", "X-User-Email"]
    role_headers = ["X-Tenet-Authenticated-Roles"]
    read_roles = ["viewer", "operator", "admin"]
    write_roles = ["operator", "admin"]
    allowlist_headers = ["X-Tenet-Authorized-Deployments"]
    project_environment_scope_headers = ["X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"]
    managed_login_required_settings = [
        "TENET_AUTH_PROVIDER",
        "TENET_GITHUB_OAUTH_CLIENT_ID",
        "TENET_GITHUB_OAUTH_CLIENT_SECRET",
        "TENET_GITHUB_OAUTH_REDIRECT_URL",
        "TENET_AUTH_STATE_STORE_PATH",
        "TENET_AUTH_IDENTITY_STORE_PATH",
        "TENET_AUTH_SESSION_SECRET",
    ]
    auth_runtime = payload.get("authRuntime") or {}
    tenet_managed_login = auth_runtime.get("tenetManagedLogin") or {}
    assert tenet_managed_login.get("configured") is False, payload
    assert tenet_managed_login.get("active") is False, payload
    assert tenet_managed_login.get("provider") == "", payload
    assert tenet_managed_login.get("unavailableReason") == "tenet-managed login is disabled because OAuth runtime configuration is incomplete or absent", payload
    assert tenet_managed_login.get("oauthStateTtlSeconds") == 600, payload
    assert tenet_managed_login.get("sessionTtlSeconds") == 43200, payload
    assert tenet_managed_login.get("sessionExpiryEnforced") is True, payload
    assert tenet_managed_login.get("sessionCookiePath") == "/", payload
    assert tenet_managed_login.get("sessionCookieHttpOnly") is True, payload
    assert tenet_managed_login.get("sessionCookieSameSite") == "Lax", payload
    assert tenet_managed_login.get("sessionCookieSecureOverride") is False, payload
    assert tenet_managed_login.get("sessionCookieSecureForRequest") is False, payload
    assert tenet_managed_login.get("providerAccessTokenStored") is False, payload
    assert tenet_managed_login.get("providerAccessTokenReturned") is False, payload
    assert tenet_managed_login.get("providerTokenRefreshSupported") is False, payload
    assert tenet_managed_login.get("providerTokenRevocationSupported") is False, payload
    assert tenet_managed_login.get("logoutClearsSessionCookie") is False, payload
    assert tenet_managed_login.get("logoutRevokesProviderToken") is False, payload
    assert tenet_managed_login.get("requiredSettings") == managed_login_required_settings, payload
    assert tenet_managed_login.get("missingSettings") == managed_login_required_settings, payload
    assert "loginRoute" not in tenet_managed_login, payload
    assert "callbackRoute" not in tenet_managed_login, payload
    identity_header_support = payload.get("identityHeaderSupport") or {}
    actor_header_support = payload.get("actorHeaderSupport") or {}
    assert identity_header_support.get("canonical") == canonical_identity_headers, payload
    assert identity_header_support.get("aliases") == alias_identity_headers, payload
    assert actor_header_support.get("canonical") == canonical_actor_headers, payload
    assert actor_header_support.get("aliases") == alias_actor_headers, payload
    assert (payload.get("deploymentAccess") or {}).get("supportedHeaderNames") == allowlist_headers + project_environment_scope_headers, payload
    assert access_verdicts.get("operatorRead", {}).get("requiresAuthenticatedIdentity") is True, payload
    assert access_verdicts.get("operatorRead", {}).get("requiresViewerCapableRole") is True, payload
    assert access_verdicts.get("operatorRead", {}).get("exampleRoutes") == ["GET /v1/overview/applications", "GET /v1/status/deployments", "GET /v1/catalog/deployments", "GET /v1/audit-logs"], payload
    assert access_verdicts.get("operatorRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers, payload
    assert access_verdicts.get("operatorRead", {}).get("acceptedRoleNames") == read_roles, payload
    assert access_verdicts.get("operatorWrite", {}).get("requiresOperatorRole") is True, payload
    assert access_verdicts.get("operatorWrite", {}).get("exampleRoutes") == ["POST /v1/plans/deployments/{deploymentName}", "POST /v1/applies/plans/{planId}", "POST /v1/applies/{applyId}/transitions"], payload
    assert access_verdicts.get("operatorWrite", {}).get("requiredTrustedHeaders") == identity_headers + role_headers, payload
    assert access_verdicts.get("operatorWrite", {}).get("acceptedRoleNames") == write_roles, payload
    assert access_verdicts.get("deploymentScopedRead", {}).get("requiresExactDeploymentAllowlistMatch") is True, payload
    assert access_verdicts.get("deploymentScopedRead", {}).get("acceptsProjectEnvironmentScopes") is True, payload
    assert access_verdicts.get("deploymentScopedRead", {}).get("exampleRoutes") == ["GET /v1/overview/deployments/{deploymentName}", "GET /v1/status/deployments/{deploymentName}", "GET /v1/audit-logs?deployment={deploymentName}"], payload
    assert access_verdicts.get("deploymentScopedRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload
    assert access_verdicts.get("deploymentScopedRead", {}).get("acceptedRoleNames") == read_roles, payload
    assert access_verdicts.get("deploymentScopedWrite", {}).get("requiresExactDeploymentAllowlistMatch") is True, payload
    assert access_verdicts.get("deploymentScopedWrite", {}).get("acceptsProjectEnvironmentScopes") is True, payload
    assert access_verdicts.get("deploymentScopedWrite", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload
    assert access_verdicts.get("deploymentScopedWrite", {}).get("acceptedRoleNames") == write_roles, payload
    assert access_verdicts.get("collectionRead", {}).get("filtersCollectionsByDeploymentAccess") is True, payload
    assert access_verdicts.get("collectionRead", {}).get("exampleRoutes") == ["GET /v1/overview/applications", "GET /v1/status/deployments", "GET /v1/catalog/deployments", "GET /v1/overview/accounts", "GET /v1/audit-logs"], payload
    assert access_verdicts.get("collectionRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload
    assert access_verdicts.get("collectionRead", {}).get("acceptedRoleNames") == read_roles, payload
    assert access_verdicts.get("catalogRead", {}).get("filtersCollectionsByDeploymentAccess") is True, payload
    assert access_verdicts.get("catalogRead", {}).get("exampleRoutes") == ["GET /v1/catalog/deployments", "GET /v1/catalog/accounts", "GET /v1/catalog/placements", "GET /v1/catalog/topology", "GET /v1/catalog/summary"], payload
    assert access_verdicts.get("catalogRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload
    assert access_verdicts.get("catalogRead", {}).get("acceptedRoleNames") == read_roles, payload
    desired_state_source = payload.get("desiredStateSource") or {}
    assert desired_state_source.get("configured") is True, payload
    assert desired_state_source.get("kind") == "filesystem", payload
    assert (desired_state_source.get("filesystem") or {}).get("mode") == "local_repository", payload
    assert "github" not in desired_state_source, payload
    source_refresh = desired_state_source.get("controllerRefresh") or {}
    assert source_refresh.get("configured") is False, payload
    assert source_refresh.get("available") is False, payload
    assert source_refresh.get("status") == "filesystem_noop", payload
    assert source_refresh.get("diagnosticOnly") is True, payload
    assert "filesystem catalog" in source_refresh.get("reason", ""), payload
    assert "matched" in (desired_state_source.get("webhookMatchStatuses") or []), payload
    assert "filesystem_source" in (desired_state_source.get("webhookProcessedFalseReasons") or []), payload

assert_operator_context_common(operator_context_trusted)
assert operator_context_trusted["identity"]["authenticated"] is True, operator_context_trusted
assert operator_context_trusted["identity"]["id"] == "front-door-123", operator_context_trusted
assert operator_context_trusted["identity"]["name"] == "Front Door Operator", operator_context_trusted
assert operator_context_trusted["identity"]["email"] == "frontdoor@example.com", operator_context_trusted
assert operator_context_trusted["actor"]["available"] is True, operator_context_trusted
assert operator_context_trusted["actor"]["id"] == "front-door-123", operator_context_trusted
assert operator_context_trusted["actor"]["name"] == "Front Door Operator", operator_context_trusted
assert operator_context_trusted["actor"]["email"] == "frontdoor@example.com", operator_context_trusted
assert operator_context_trusted["actorProvenance"] == {"idSource": "absent", "nameSource": "absent", "emailSource": "absent"}, operator_context_trusted
assert operator_context_trusted["actorAttribution"]["available"] is True, operator_context_trusted
assert operator_context_trusted["actorAttribution"]["source"] == "trusted_authenticated_identity", operator_context_trusted
actor_precedence = ["explicit_actor_headers", "actor_alias_headers", "trusted_authenticated_identity", "unavailable"]
assert operator_context_trusted["actorAttribution"]["precedence"] == actor_precedence, operator_context_trusted
assert "trusted authenticated identity" in operator_context_trusted["actorAttribution"]["summary"], operator_context_trusted
assert operator_context_trusted["roles"]["names"] == ["viewer", "operator"], operator_context_trusted
assert operator_context_trusted["roles"]["canRead"] is True, operator_context_trusted
assert operator_context_trusted["roles"]["canWrite"] is True, operator_context_trusted
assert operator_context_trusted["roles"]["provenance"]["source"] == "canonical", operator_context_trusted
assert operator_context_trusted["roles"]["supportedNames"] == ["viewer", "operator", "admin"], operator_context_trusted
assert operator_context_trusted["roles"]["supportedHeaderNames"] == ["X-Tenet-Authenticated-Roles"], operator_context_trusted
assert operator_context_trusted["roles"]["recognizedNames"] == ["viewer", "operator"], operator_context_trusted
assert operator_context_trusted["roles"]["unrecognizedNames"] == [], operator_context_trusted
assert operator_context_trusted["deploymentAccess"]["enforced"] is True, operator_context_trusted
assert operator_context_trusted["deploymentAccess"]["forwarded"] is True, operator_context_trusted
assert operator_context_trusted["deploymentAccess"]["names"] == ["payments-api-prod", "worker-prod"], operator_context_trusted
trusted_summary = operator_context_trusted["deploymentAccess"]["summary"]
assert trusted_summary["available"] is True, operator_context_trusted
assert trusted_summary["forwardedCount"] == 2, operator_context_trusted
assert trusted_summary["matchedCount"] == 2, operator_context_trusted
assert trusted_summary["exactDeploymentMatchedCount"] == 2, operator_context_trusted
assert trusted_summary.get("projectEnvironmentMatchedCount", 0) == 0, operator_context_trusted
assert trusted_summary.get("unmatchedNames", []) == [], operator_context_trusted
assert trusted_summary["accountCount"] >= 1, operator_context_trusted
assert trusted_summary["projectCount"] >= 1, operator_context_trusted
assert trusted_summary["environmentCount"] >= 1, operator_context_trusted
assert trusted_summary.get("emptyReason") is None, operator_context_trusted
assert trusted_summary["visibleDeploymentExamples"], operator_context_trusted
trusted_visible = {row["deploymentName"]: row for row in trusted_summary["visibleDeploymentExamples"]}
assert trusted_visible["payments-api-prod"]["matchSources"] == ["exactDeployment"], operator_context_trusted
assert trusted_visible["payments-api-prod"]["appRef"] == "payments-api", operator_context_trusted
assert trusted_visible["payments-api-prod"]["environmentRef"] == "prod-apne2", operator_context_trusted
assert trusted_visible["payments-api-prod"]["awsAccountId"], operator_context_trusted
assert trusted_summary["projectEnvironmentExamples"], operator_context_trusted
assert_operator_context_project_environment_examples(trusted_summary["projectEnvironmentExamples"], operator_context_trusted)
trusted_project_envs = {(row["appRef"], row["environmentRef"]) for row in trusted_summary["projectEnvironmentExamples"]}
assert ("payments-api", "prod-apne2") in trusted_project_envs or ("worker", "prod-apne2") in trusted_project_envs, operator_context_trusted
trusted_verdicts = operator_context_trusted["accessVerdicts"]
assert trusted_verdicts["operatorRead"]["allowed"] is True, operator_context_trusted
assert trusted_verdicts["operatorWrite"]["allowed"] is True, operator_context_trusted
assert trusted_verdicts["deploymentScopedRead"]["reason"] == "deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments", operator_context_trusted
assert trusted_verdicts["collectionRead"]["allowed"] is True, operator_context_trusted
assert_operator_context_common(operator_context_combined_access)
assert operator_context_combined_access["deploymentAccess"]["enforced"] is True, operator_context_combined_access
assert operator_context_combined_access["deploymentAccess"]["forwarded"] is True, operator_context_combined_access
assert operator_context_combined_access["deploymentAccess"]["names"] == ["payments-api-prod", "worker-prod"], operator_context_combined_access
combined_scopes = operator_context_combined_access["deploymentAccess"]["scopes"]
assert combined_scopes["forwarded"] is True, operator_context_combined_access
assert combined_scopes["projects"] == ["payments-api"], operator_context_combined_access
assert combined_scopes["environments"] == ["prod-apne2"], operator_context_combined_access
combined_summary = operator_context_combined_access["deploymentAccess"]["summary"]
assert combined_summary["available"] is True, operator_context_combined_access
assert combined_summary["forwardedCount"] == 2, operator_context_combined_access
assert combined_summary["forwardedProjectCount"] == 1, operator_context_combined_access
assert combined_summary["forwardedEnvironmentCount"] == 1, operator_context_combined_access
assert combined_summary["matchedCount"] >= 2, operator_context_combined_access
assert combined_summary["exactDeploymentMatchedCount"] >= 2, operator_context_combined_access
assert combined_summary["projectEnvironmentMatchedCount"] >= 1, operator_context_combined_access
assert combined_summary.get("unmatchedNames", []) == [], operator_context_combined_access
assert combined_summary.get("unmatchedProjects", []) == [], operator_context_combined_access
assert combined_summary.get("unmatchedEnvironments", []) == [], operator_context_combined_access
combined_visible = {row["deploymentName"]: row for row in combined_summary["visibleDeploymentExamples"]}
combined_payment = combined_visible["payments-api-prod"]
assert combined_payment["deploymentName"] == "payments-api-prod", operator_context_combined_access
assert combined_payment["appRef"] == "payments-api", operator_context_combined_access
assert combined_payment["environmentRef"] == "prod-apne2", operator_context_combined_access
assert combined_payment["region"], operator_context_combined_access
assert combined_payment["awsAccountId"], operator_context_combined_access
assert combined_payment["matchSources"] == ["exactDeployment", "projectEnvironment"], operator_context_combined_access
assert_operator_context_project_environment_examples(combined_summary["projectEnvironmentExamples"], operator_context_combined_access)
assert_operator_context_common(operator_context_unmatched)
assert operator_context_unmatched["deploymentAccess"]["enforced"] is True, operator_context_unmatched
assert operator_context_unmatched["deploymentAccess"]["forwarded"] is True, operator_context_unmatched
assert operator_context_unmatched["deploymentAccess"]["names"] == ["payments-api-prod", "stale-api", "worker-prod"], operator_context_unmatched
unmatched_summary = operator_context_unmatched["deploymentAccess"]["summary"]
assert unmatched_summary["available"] is True, operator_context_unmatched
assert unmatched_summary["forwardedCount"] == 3, operator_context_unmatched
assert unmatched_summary["matchedCount"] == 2, operator_context_unmatched
assert unmatched_summary["exactDeploymentMatchedCount"] == 2, operator_context_unmatched
assert unmatched_summary["unmatchedNames"] == ["stale-api"], operator_context_unmatched
assert unmatched_summary["accountCount"] >= 1, operator_context_unmatched
assert unmatched_summary["projectCount"] >= 1, operator_context_unmatched
assert unmatched_summary["environmentCount"] >= 1, operator_context_unmatched
assert unmatched_summary.get("emptyReason") is None, operator_context_unmatched
assert unmatched_summary["visibleDeploymentExamples"], operator_context_unmatched
unmatched_visible = {row["deploymentName"]: row for row in unmatched_summary["visibleDeploymentExamples"]}
assert "stale-api" not in unmatched_visible, operator_context_unmatched
assert unmatched_visible["payments-api-prod"]["matchSources"] == ["exactDeployment"], operator_context_unmatched
assert unmatched_summary["projectEnvironmentExamples"], operator_context_unmatched
assert_operator_context_common(operator_context_stale_scopes)
assert operator_context_stale_scopes["deploymentAccess"]["enforced"] is True, operator_context_stale_scopes
assert operator_context_stale_scopes["deploymentAccess"]["forwarded"] is True, operator_context_stale_scopes
assert "names" not in operator_context_stale_scopes["deploymentAccess"], operator_context_stale_scopes
stale_scopes = operator_context_stale_scopes["deploymentAccess"]["scopes"]
assert stale_scopes["forwarded"] is True, operator_context_stale_scopes
assert stale_scopes["projects"] == ["payments-api", "archived-payments"], operator_context_stale_scopes
assert stale_scopes["environments"] == ["staging-apne2", "stale-env"], operator_context_stale_scopes
stale_scopes_summary = operator_context_stale_scopes["deploymentAccess"]["summary"]
assert stale_scopes_summary["available"] is True, operator_context_stale_scopes
assert stale_scopes_summary["forwardedCount"] == 0, operator_context_stale_scopes
assert stale_scopes_summary["forwardedProjectCount"] == 2, operator_context_stale_scopes
assert stale_scopes_summary["forwardedEnvironmentCount"] == 2, operator_context_stale_scopes
assert stale_scopes_summary["matchedCount"] == 1, operator_context_stale_scopes
assert stale_scopes_summary["projectEnvironmentMatchedCount"] == 1, operator_context_stale_scopes
assert stale_scopes_summary.get("exactDeploymentMatchedCount", 0) == 0, operator_context_stale_scopes
assert stale_scopes_summary.get("unmatchedNames", []) == [], operator_context_stale_scopes
assert stale_scopes_summary["unmatchedProjects"] == ["archived-payments"], operator_context_stale_scopes
assert stale_scopes_summary["unmatchedProjectCount"] == 1, operator_context_stale_scopes
assert stale_scopes_summary["unmatchedEnvironments"] == ["stale-env"], operator_context_stale_scopes
assert stale_scopes_summary["unmatchedEnvironmentCount"] == 1, operator_context_stale_scopes
assert stale_scopes_summary["visibleDeploymentExamples"], operator_context_stale_scopes
assert stale_scopes_summary["visibleDeploymentExamples"][0]["deploymentName"] == "payments-api-staging", operator_context_stale_scopes
assert stale_scopes_summary["visibleDeploymentExamples"][0]["matchSources"] == ["projectEnvironment"], operator_context_stale_scopes
assert stale_scopes_summary["projectEnvironmentExamples"], operator_context_stale_scopes
stale_scope_project_envs = {(row["appRef"], row["environmentRef"]) for row in stale_scopes_summary["projectEnvironmentExamples"]}
assert stale_scope_project_envs == {("payments-api", "staging-apne2")}, operator_context_stale_scopes
assert operator_context_stale_scopes["accessVerdicts"]["deploymentScopedRead"]["allowed"] is True, operator_context_stale_scopes
assert operator_context_stale_scopes["accessVerdicts"]["collectionRead"]["allowed"] is True, operator_context_stale_scopes
assert_operator_context_common(operator_context_alias)
assert operator_context_alias["identity"]["authenticated"] is True, operator_context_alias
assert operator_context_alias["identity"]["id"] == "alias-456", operator_context_alias
assert operator_context_alias["identity"]["name"] == "Alias Operator", operator_context_alias
assert operator_context_alias["identity"]["email"] == "alias@example.com", operator_context_alias
assert operator_context_alias["actor"]["available"] is True, operator_context_alias
assert operator_context_alias["actor"]["id"] == "alias-456", operator_context_alias
assert operator_context_alias["actor"]["name"] == "Alias Operator", operator_context_alias
assert operator_context_alias["actor"]["email"] == "alias@example.com", operator_context_alias
assert operator_context_alias["actorProvenance"] == {"idSource": "alias", "nameSource": "alias", "emailSource": "alias"}, operator_context_alias
assert operator_context_alias["actorAttribution"]["available"] is True, operator_context_alias
assert operator_context_alias["actorAttribution"]["source"] == "explicit_actor_headers", operator_context_alias
assert operator_context_alias["actorAttribution"]["precedence"] == actor_precedence, operator_context_alias
assert "explicit X-Tenet-Actor-* headers" in operator_context_alias["actorAttribution"]["summary"], operator_context_alias
assert_operator_context_common(operator_context_actor_precedence)
assert operator_context_actor_precedence["identity"]["id"] == "front-door-actor-789", operator_context_actor_precedence
assert operator_context_actor_precedence["identity"]["name"] == "Front Door Actor", operator_context_actor_precedence
assert operator_context_actor_precedence["identity"]["email"] == "frontdoor.actor@example.com", operator_context_actor_precedence
assert operator_context_actor_precedence["identityProvenance"] == {"idSource": "canonical", "nameSource": "canonical", "emailSource": "canonical"}, operator_context_actor_precedence
assert operator_context_actor_precedence["actor"]["available"] is True, operator_context_actor_precedence
assert operator_context_actor_precedence["actor"]["id"] == "explicit-789", operator_context_actor_precedence
assert operator_context_actor_precedence["actor"]["name"] == "Explicit Actor", operator_context_actor_precedence
assert operator_context_actor_precedence["actor"]["email"] == "explicit.actor@example.com", operator_context_actor_precedence
assert operator_context_actor_precedence["actorProvenance"] == {"idSource": "canonical", "nameSource": "canonical", "emailSource": "canonical"}, operator_context_actor_precedence
assert operator_context_actor_precedence["actorAttribution"]["available"] is True, operator_context_actor_precedence
assert operator_context_actor_precedence["actorAttribution"]["source"] == "explicit_actor_headers", operator_context_actor_precedence
assert operator_context_actor_precedence["actorAttribution"]["precedence"] == actor_precedence, operator_context_actor_precedence
assert "explicit X-Tenet-Actor-* headers" in operator_context_actor_precedence["actorAttribution"]["summary"], operator_context_actor_precedence
assert operator_context_alias["roles"]["names"] == ["viewer"], operator_context_alias
assert operator_context_alias["roles"]["canRead"] is True, operator_context_alias
assert operator_context_alias["roles"]["canWrite"] is False, operator_context_alias
assert operator_context_alias["roles"]["provenance"]["source"] == "canonical", operator_context_alias
assert operator_context_alias["roles"]["supportedNames"] == ["viewer", "operator", "admin"], operator_context_alias
assert operator_context_alias["roles"]["recognizedNames"] == ["viewer"], operator_context_alias
assert operator_context_alias["roles"]["unrecognizedNames"] == [], operator_context_alias
assert operator_context_alias["deploymentAccess"]["enforced"] is True, operator_context_alias
assert operator_context_alias["deploymentAccess"]["forwarded"] is False, operator_context_alias
assert "names" not in operator_context_alias["deploymentAccess"], operator_context_alias
assert_operator_context_common(operator_context_no_allowlist)
assert operator_context_no_allowlist["identity"]["authenticated"] is True, operator_context_no_allowlist
assert operator_context_no_allowlist["identity"]["id"] == "no-allowlist-789", operator_context_no_allowlist
assert operator_context_no_allowlist["identity"]["name"] == "No Allowlist Operator", operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["names"] == ["viewer"], operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["canRead"] is True, operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["canWrite"] is False, operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["provenance"]["source"] == "canonical", operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["supportedNames"] == ["viewer", "operator", "admin"], operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["recognizedNames"] == ["viewer"], operator_context_no_allowlist
assert operator_context_no_allowlist["roles"]["unrecognizedNames"] == [], operator_context_no_allowlist
assert operator_context_no_allowlist["deploymentAccess"]["enforced"] is True, operator_context_no_allowlist
assert operator_context_no_allowlist["deploymentAccess"]["forwarded"] is False, operator_context_no_allowlist
assert "names" not in operator_context_no_allowlist["deploymentAccess"], operator_context_no_allowlist
no_allowlist_verdicts = operator_context_no_allowlist["accessVerdicts"]
assert no_allowlist_verdicts["deploymentScopedRead"]["allowed"] is False, operator_context_no_allowlist
assert no_allowlist_verdicts["collectionRead"]["allowed"] is True, operator_context_no_allowlist
assert no_allowlist_verdicts["collectionRead"]["emptyCollectionLikely"] is True, operator_context_no_allowlist

assert_operator_context_common(operator_context_no_roles)
assert operator_context_no_roles["identity"]["authenticated"] is True, operator_context_no_roles
assert operator_context_no_roles["identity"]["id"] == "no-roles-321", operator_context_no_roles
assert operator_context_no_roles["identity"]["name"] == "Missing Role Operator", operator_context_no_roles
assert operator_context_no_roles["identityProvenance"] == {"idSource": "canonical", "nameSource": "canonical", "emailSource": "absent"}, operator_context_no_roles
assert operator_context_no_roles["roles"].get("names", []) == [], operator_context_no_roles
assert operator_context_no_roles["roles"]["canRead"] is False, operator_context_no_roles
assert operator_context_no_roles["roles"]["canWrite"] is False, operator_context_no_roles
assert operator_context_no_roles["roles"]["provenance"]["source"] == "absent", operator_context_no_roles
assert operator_context_no_roles["roles"]["supportedNames"] == ["viewer", "operator", "admin"], operator_context_no_roles
assert operator_context_no_roles["roles"]["recognizedNames"] == [], operator_context_no_roles
assert operator_context_no_roles["roles"]["unrecognizedNames"] == [], operator_context_no_roles
no_roles_verdicts = operator_context_no_roles["accessVerdicts"]
assert no_roles_verdicts["operatorRead"]["allowed"] is False, operator_context_no_roles
assert no_roles_verdicts["operatorRead"]["reason"] == "authenticated viewer-capable role is required for guarded operator read routes", operator_context_no_roles
assert no_roles_verdicts["operatorWrite"]["allowed"] is False, operator_context_no_roles
assert no_roles_verdicts["operatorWrite"]["reason"] == "authenticated operator role is required for guarded operator write routes", operator_context_no_roles

assert_operator_context_common(operator_context_unsupported_roles)
assert operator_context_unsupported_roles["identity"]["authenticated"] is True, operator_context_unsupported_roles
assert operator_context_unsupported_roles["identity"]["id"] == "mixed-roles-654", operator_context_unsupported_roles
assert operator_context_unsupported_roles["identity"]["name"] == "Mixed Role Operator", operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["names"] == ["viewer", "auditor"], operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["canRead"] is True, operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["canWrite"] is False, operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["provenance"]["source"] == "canonical", operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["supportedNames"] == ["viewer", "operator", "admin"], operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["recognizedNames"] == ["viewer"], operator_context_unsupported_roles
assert operator_context_unsupported_roles["roles"]["unrecognizedNames"] == ["auditor"], operator_context_unsupported_roles
unsupported_roles_verdicts = operator_context_unsupported_roles["accessVerdicts"]
assert unsupported_roles_verdicts["operatorRead"]["allowed"] is True, operator_context_unsupported_roles
assert unsupported_roles_verdicts["operatorWrite"]["allowed"] is False, operator_context_unsupported_roles
assert unsupported_roles_verdicts["operatorWrite"]["reason"] == "authenticated operator role is required for guarded operator write routes", operator_context_unsupported_roles
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

restart_api_with_enterprise_sso google_oidc
enterprise_sso_google_context=$(curl -fsS "$API_BASE_URL/v1/operator/context")
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

restart_api_with_enterprise_sso okta_oidc
enterprise_sso_okta_oidc_context=$(curl -fsS "$API_BASE_URL/v1/operator/context")
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

restart_api_with_enterprise_sso okta_saml
enterprise_sso_okta_saml_context=$(curl -fsS "$API_BASE_URL/v1/operator/context")
enterprise_sso_okta_saml_context_path=$(write_payload enterprise-sso-okta-saml-context.json "$enterprise_sso_okta_saml_context")
assert_enterprise_sso_diagnostic "$enterprise_sso_okta_saml_context_path" okta_saml /auth/login/okta /auth/callback/okta/saml false \
  TENET_AUTH_PROVIDER \
  TENET_OKTA_SAML_METADATA_URL \
  TENET_OKTA_SAML_ACS_URL \
  TENET_AUTH_STATE_STORE_PATH \
  TENET_AUTH_IDENTITY_STORE_PATH \
  TENET_AUTH_SESSION_SECRET

echo "smoke-docker: configured enterprise SSO diagnostics ok"
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
