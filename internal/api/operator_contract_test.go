package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootForContractTest(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repoRootForContractTest(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}

	return string(body)
}

func TestDockerUpOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	if !strings.Contains(compose, `TENET_AWS_ENABLE_APPLY: ${TENET_AWS_ENABLE_APPLY:-false}`) {
		t.Fatal("expected compose api service to expose TENET_AWS_ENABLE_APPLY with a default false local contract")
	}

	makefile := readRepoFile(t, "Makefile")
	if !strings.Contains(makefile, "./scripts/docker-up.sh $(DOCKER_COMPOSE)") {
		t.Fatal("expected Makefile docker-up target to route through scripts/docker-up.sh")
	}

	script := readRepoFile(t, "scripts/docker-up.sh")
	for _, fragment := range []string{
		`port api "$PORT"`,
		`lsof -nP -iTCP:"$PORT" -sTCP:LISTEN`,
		"localhost:8080 is already occupied by something other than this Tenet compose stack",
		"lsof -nP -iTCP:8080 -sTCP:LISTEN",
		"make docker-down",
		"docker compose up --build -d",
		`exec "${compose_cmd[@]}" up --build -d`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected docker-up preflight script to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
		"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README docker troubleshooting to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"The `make docker-up` preflight intentionally fails fast when `localhost:8080` is already occupied by something other than this Compose stack.",
		"`lsof -nP -iTCP:8080 -sTCP:LISTEN`",
		"`make docker-down`",
		"`docker compose up --build -d`",
		"`TENET_AWS_ENABLE_APPLY=true make docker-up`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc troubleshooting to include %q", fragment)
		}
	}
}

func TestReadOnlyApplyOperatorContract(t *testing.T) {
	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`restart_api_with_apply_mode() {`,
		`TENET_AWS_ENABLE_APPLY="$apply_enabled" "${COMPOSE[@]}" up -d api >/dev/null`,
		`queued_apply_record_get=$(wait_for_apply_status "$queued_apply_id" failed)`,
		`queued_apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$queued_plan_id")`,
		`assert queued_apply_record["status"] == "queued", queued_apply_record`,
		`assert not queued_apply_record.get("error"), queued_apply_record`,
		`"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.",`,
		`"Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.",`,
		`assert queued_apply_record_get["status"] == "failed", queued_apply_record_get`,
		`"Tenet executor advanced this apply request on the same applyId without calling the manual transition API.",`,
		`assert [operation["status"] for operation in queued_apply_record["operations"]] == ["queued"] * len(queued_apply_record["operations"]), queued_apply_record`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker queued apply contract to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH"`,
		`TENET_AWS_ENABLE_APPLY="$APPLY_ENABLED"`,
		`curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`curl -sS -o "$STALE_APPLY_CONFLICT_BODY" -w "%{http_code}" -X POST "http://$HOST:$PORT/v1/applies/plans/$stale_apply_plan_id"`,
		`apply_record_get=$(wait_for_apply_status "$apply_id" failed)`,
		`curl -fsS "http://$HOST:$PORT/v1/applies/$apply_id"`,
		`apply_enabled = os.environ['APPLY_ENABLED'].lower() == 'true'`,
		`assert apply_record['status'] == ('queued' if apply_enabled else 'failed'), apply_record`,
		`assert apply_record['error'] == 'apply execution is disabled', apply_record`,
		`assert isinstance(apply_record.get('lifecycle', {}), dict) and apply_record['lifecycle'], apply_record`,
		`assert apply_record['lifecycle']['updatedAt'] == apply_record['createdAt'], apply_record`,
		`assert apply_record['lifecycle']['totalSteps'] == len(apply_record['operations']), apply_record`,
		`assert apply_record['lifecycle']['finishedAt'] == apply_record['createdAt'], apply_record`,
		`'Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true enabled the local-safe executor handoff path.'`,
		`'Tenet will advance this same applyId locally, but live AWS mutation is still not implemented for this backend slice.'`,
		`assert apply_record_get['status'] == 'failed', apply_record_get`,
		`assert apply_record_get['error'] == 'apply executor handoff stopped before live AWS mutation because this backend slice is still local-safe', apply_record_get`,
		`assert isinstance(apply_record.get('operations', []), list) and apply_record['operations'], apply_record`,
		`assert [operation['status'] for operation in apply_record['operations']] == [('queued' if apply_enabled else 'pending')] * len(apply_record['operations']), apply_record`,
		`assert os.environ['STALE_APPLY_CONFLICT_STATUS'] == '409', os.environ['STALE_APPLY_CONFLICT_STATUS']`,
		`assert 'create a new plan preview before applying' in stale_apply_conflict, stale_apply_conflict`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-stale&action=apply.requested"`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local apply contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`applyrequest.TransitionRequest` state model internally",
		"`TENET_APPLY_STORE_PATH`",
		"`TENET_AWS_ENABLE_APPLY=true`",
		"disabled by default for live execution",
		"queued-on-create local-safe executor handoff contract",
		"live AWS mutation is still not implemented",
		"ordered `operations[]` rows",
		"additive `lifecycle` block",
		"`updatedAt` / `startedAt` / `finishedAt`",
		"`409 Conflict`",
		"create a fresh preview",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README apply contract to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"preserving the immutable stable `applyId`",
		"failed immutable contract with operator-facing error/details",
		"`TENET_AWS_ENABLE_APPLY=true`",
		"queued immutable apply request without contacting AWS",
		"`status=pending`",
		"`status=queued`",
		"live AWS mutation is not implemented yet",
		"does not require manual transition API calls",
		"additive `lifecycle` block",
		"`currentStep` / `completedSteps` / `totalSteps`",
		"`400 Bad Request`",
		"`409 Conflict`",
		"does not persist a new apply record",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc apply contract to include %q", fragment)
		}
	}
}

func TestAuditLogOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?action=apply.requested"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=payments-api-prod&limit=1"`,
		`curl -fsS "http://$HOST:$PORT/v1/audit-logs?deployment=worker-prod"`,
		`curl -sS -o "$INVALID_AUDIT_ACTION_BODY" -w "%{http_code}" "http://$HOST:$PORT/v1/audit-logs?action=invalid.action"`,
		`assert any(entry['action'] == 'apply.requested' for entry in audit_logs['entries']), audit_logs`,
		`assert len(audit_logs_action_filtered['entries']) >= 2, audit_logs_action_filtered`,
		`assert all(entry['action'] == 'apply.requested' for entry in audit_logs_action_filtered['entries']), audit_logs_action_filtered`,
		`assert len(audit_logs_filtered['entries']) == 1, audit_logs_filtered`,
		`assert audit_logs_empty['entries'] == [], audit_logs_empty`,
		`assert os.environ['INVALID_AUDIT_ACTION_STATUS'] == '400', os.environ['INVALID_AUDIT_ACTION_STATUS']`,
		`assert entry['actor']['available'] is False, entry`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local audit contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs?action=apply.requested`",
		"`GET /v1/audit-logs?action=plan.created`",
		"`GET /v1/audit-logs?limit={n}`",
		"`GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/audit-logs?deployment={deploymentName}&action={action}`",
		"`action` is optional and must be exactly `plan.created` or `apply.requested` when present",
		"`GET /v1/audit-logs?deployment={deploymentName}&limit={n}`",
		"`deployment` is optional and must name exactly one deployment when present",
		"`X-Tenet-Actor-Id`",
		"`X-Tenet-Actor-Name`",
		"`X-Tenet-Actor-Email`",
		"actor information is unavailable because no authenticated request context was available for this local request",
		"`Audit filters: action apply.requested · limit 2`",
		"`/v1/audit-logs` is the preferred v1 route shape for this local timeline",
		"the full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present",
		"fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README audit contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"`GET /v1/audit-logs`",
		"`GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/audit-logs?action={action}`",
		"`limit` is optional and must be a positive integer when present",
		"`action` is optional and must be exactly `plan.created` or `apply.requested` when present",
		"`deployment` is optional and must name exactly one deployment when present",
		"`GET /v1/audit-logs?action=apply.requested&limit=2`",
		"fetching the full `GET /v1/audit-logs` collection",
		"asserting the expected `plan.created` and `apply.requested` actions are present without depending on a two-entry window",
		"`action=invalid.action` with `400 Bad Request`",
		"`entries: []` with `200 OK` when the selected deployment exists but has no persisted audit records yet",
		"`X-Tenet-Actor-Id`",
		"`X-Tenet-Actor-Name`",
		"`X-Tenet-Actor-Email`",
		"`actor.available=false` plus an unavailable reason",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit architecture doc to include %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`/?page=audit` route now renders that audit timeline directly",
		"`Audit filters: action apply.requested · limit 2`",
		"accepts optional `&action={action}` / `&limit={n}`",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc to include %q", fragment)
		}
	}
}

func TestGitHubWebhookOperatorContract(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`X-GitHub-Delivery`",
		"`X-GitHub-Event`",
		"`X-Hub-Signature-256`",
		"`202 Accepted`",
		"`200 OK`",
		"`400 Bad Request`",
		"`status=accepted`",
		"`status=ignored`",
		"`processed=false`",
		"no reconcile queue work is enqueued yet",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README github webhook contract to include %q", fragment)
		}
	}

	overviewDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"accepted` or `ignored` responses",
		"without enqueueing reconcile work yet",
		"webhook-driven reconcile triggering",
	} {
		if !strings.Contains(overviewDoc, fragment) {
			t.Fatalf("expected overview architecture doc to include %q", fragment)
		}
	}

	controlPlaneDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`POST /api/v1/github/webhook`",
		"HMAC-SHA256 signatures",
		"without enqueueing reconcile work yet",
		"no reconcile queue side effect happens yet",
	} {
		if !strings.Contains(controlPlaneDoc, fragment) {
			t.Fatalf("expected aws central control plane doc to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`TENET_GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET"`,
		`webhook_push=$(curl -fsS -X POST \`,
		`http://$HOST:$PORT/api/v1/github/webhook`,
		`assert webhook_push['status'] == 'accepted', webhook_push`,
		`assert webhook_ping['status'] == 'ignored', webhook_ping`,
		`assert os.environ['INVALID_WEBHOOK_STATUS'] == '400', os.environ['INVALID_WEBHOOK_STATUS']`,
		`assert 'github webhook signature mismatch' in invalid_webhook, invalid_webhook`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local github webhook contract to include %q", fragment)
		}
	}
}

func TestResourceLogsOperatorContract(t *testing.T) {
	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`assert isinstance(resource_logs.get('lookupContext'), dict), resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('region'), str) and resource_logs['lookupContext']['region'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('controlAccountId'), str) and resource_logs['lookupContext']['controlAccountId'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('targetAccountId'), str) and resource_logs['lookupContext']['targetAccountId'], resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('crossAccount'), bool), resource_logs`,
		`assert isinstance(resource_logs['lookupContext'].get('observerRoleConfigured'), bool), resource_logs`,
		`assert isinstance(worker_resource_logs.get('lookupContext'), dict), worker_resource_logs`,
		`assert isinstance(worker_resource_logs['lookupContext'].get('observerRoleConfigured'), bool), worker_resource_logs`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local resource logs contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`lookupContext` object",
		"`lookupContext.region`",
		"`lookupContext.controlAccountId`",
		"`lookupContext.targetAccountId`",
		"`lookupContext.crossAccount`",
		"`lookupContext.observerRoleConfigured`",
		"present for both `available=true` and unavailable responses",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource log contract to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/resource-recent-logs.md")
	for _, fragment := range []string{
		"- `lookupContext`",
		"- `controlAccountId`",
		"- `targetAccountId`",
		"- `crossAccount`",
		"- `observerRoleConfigured`",
		"`lookupContext` object is always present, including unavailable responses",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected resource recent logs doc to include %q", fragment)
		}
	}
}

func TestResourceInspectionOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`candidate_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")`,
		`launch_template = candidate_resource_inspection['autoScalingGroup'].get('launchTemplate')`,
		`mixed_instances_policy = candidate_resource_inspection['autoScalingGroup'].get('mixedInstancesPolicy')`,
		`override_launch_template = override.get('launchTemplate')`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local resource inspection contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`candidate_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate")`,
		`launch_template = candidate_resource_inspection["autoScalingGroup"].get("launchTemplate")`,
		`mixed_instances_policy = candidate_resource_inspection["autoScalingGroup"].get("mixedInstancesPolicy")`,
		`override_launch_template = override.get("launchTemplate")`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker resource inspection contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`autoScalingGroup`: group sizing/health settings, additive launch-template metadata, attached target groups, and current instances",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource inspection contract to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`DescribeAutoScalingGroups`",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard doc to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"additive launch-template metadata",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc to include %q", fragment)
		}
	}
}
