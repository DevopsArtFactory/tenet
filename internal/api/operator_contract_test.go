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
		`json_string_field_from_path() {`,
		`response_path=$(write_payload wait-for-apply-status.json "$response")`,
		`current_status=$(json_string_field_from_path "$response_path" status)`,
		`restart_api_with_apply_mode() {`,
		`TENET_AWS_ENABLE_APPLY="$apply_enabled" "${COMPOSE[@]}" up -d api >/dev/null`,
		`drift_ui=$(curl -fsS "$API_BASE_URL/?page=drift&deployment=payments-api-prod")`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions"`,
		`"$API_BASE_URL/v1/applies/apply_smoke_missing/transitions"`,
		`deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")`,
		`deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)`,
		`plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")`,
		`plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)`,
		`plan_status=$(PLAN_PREVIEW_PATH="$plan_preview_extract_path" PLAN_PREVIEW_GET_PATH="$plan_preview_get_extract_path" DEPLOYMENT_SOURCE_FINGERPRINT="$deployment_source_fingerprint" python3 - <<'PY'`,
		`apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")`,
		`apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)`,
		`managed_drift = deployment_detail.get("managedDrift")`,
		`runtime_managed_drift = (deployment_detail.get("runtimeSummary") or {}).get("managedDrift")`,
		`assert isinstance(managed_drift, list) and managed_drift, deployment_detail`,
		`assert isinstance(runtime_managed_drift, list) and runtime_managed_drift, deployment_detail`,
		`assert runtime_managed_drift == managed_drift, deployment_detail`,
		`assert all(isinstance(row.get("field"), str) and row["field"] for row in managed_drift), managed_drift`,
		`assert all(isinstance(row.get("desired"), str) and row["desired"] for row in managed_drift), managed_drift`,
		`assert all(isinstance(row.get("actual"), str) and row["actual"] for row in managed_drift), managed_drift`,
		`assert all(row.get("severity") in {"high", "medium", "low"} for row in managed_drift), managed_drift`,
		`"Active drift source",`,
		`"Current source: controller-authored managedDrift rows.",`,
		`"managedDrift is present in the deployment detail payload, so fallback read-model heuristics stay suppressed for this deployment.",`,
		`"Drift summary",`,
		`"Field path",`,
		`"Desired",`,
		`"Actual",`,
		`"Severity",`,
		`"No controller-authored field-level drift was reported for this deployment.",`,
		`"No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.",`,
		`assert invalid_apply_transition.strip() == "invalid apply transition request body", invalid_apply_transition`,
		`assert "apply request not found" in unknown_apply_transition, unknown_apply_transition`,
		`assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition`,
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
	for _, fragment := range []string{
		`DEPLOYMENT_DETAIL_JSON="$deployment_detail"`,
		`PLAN_PREVIEW_JSON="$plan_preview"`,
		`APPLY_RECORD_JSON="$apply_record"`,
		`QUEUED_PLAN_PREVIEW_JSON="$queued_plan_preview"`,
		`QUEUED_APPLY_RECORD_JSON="$queued_apply_record"`,
	} {
		if strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker to avoid env-blob JSON parsing fragment %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`json_string_field_from_path() {`,
		`response_path=$(write_payload wait-for-apply-status.json "$response")`,
		`current_status=$(json_string_field_from_path "$response_path" status)`,
		`TENET_APPLY_STORE_PATH="$APPLY_STORE_PATH"`,
		`TENET_AWS_ENABLE_APPLY="$APPLY_ENABLED"`,
		`curl -fsS -X POST "http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`"http://$HOST:$PORT/v1/applies/$apply_id/transitions"`,
		`"http://$HOST:$PORT/v1/applies/apply_smoke_missing/transitions"`,
		`assert invalid_apply_transition.strip() == 'invalid apply transition request body', invalid_apply_transition`,
		`assert 'apply request not found' in unknown_apply_transition, unknown_apply_transition`,
		`assert f"{apply_record['id']} is already terminal with status failed" in terminal_apply_transition, terminal_apply_transition`,
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
		`deployment_detail_extract_path=$(write_payload deployment-detail.extract.json "$deployment_detail")`,
		`deployment_source_fingerprint=$(json_string_field_from_path "$deployment_detail_extract_path" sourceFingerprint)`,
		`plan_preview_extract_path=$(write_payload plan-preview.extract.json "$plan_preview")`,
		`plan_id=$(json_string_field_from_path "$plan_preview_extract_path" id)`,
		`apply_record_extract_path=$(write_payload apply-record.extract.json "$apply_record")`,
		`apply_id=$(json_string_field_from_path "$apply_record_extract_path" id)`,
		`frontdoor_plan_preview_path=$(write_payload frontdoor-plan-preview.json "$frontdoor_plan_preview")`,
		`FRONTDOOR_PLAN_PREVIEW_PATH="$frontdoor_plan_preview_path"`,
		`controller_detail_probe_path=$(write_payload controller-detail-probe.json "$controller_detail")`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local apply contract to include %q", fragment)
		}
	}
	for _, fragment := range []string{
		`DEPLOYMENT_DETAIL_JSON="$deployment_detail"`,
		`PLAN_PREVIEW_JSON="$plan_preview"`,
		`APPLY_RECORD_JSON="$apply_record"`,
		`FRONTDOOR_PLAN_PREVIEW_JSON="$frontdoor_plan_preview"`,
		`CONTROLLER_DETAIL_JSON="$controller_detail"`,
	} {
		if strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local to avoid env-blob JSON parsing fragment %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`POST /v1/applies/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`applyrequest.TransitionRequest` state model internally",
		"`invalid apply transition request body`",
		"`apply request not found`",
		"`TENET_APPLY_STORE_PATH`",
		"`TENET_AWS_ENABLE_APPLY=true`",
		"disabled by default for live execution",
		"queued-on-create local-safe executor handoff contract",
		"live AWS mutation is still not implemented",
		"ordered `operations[]` rows",
		"additive `lifecycle` block",
		"`updatedAt` / `startedAt` / `finishedAt`",
		"`400 Bad Request`",
		"`404 Not Found`",
		"`409 Conflict`",
		"terminal `applyId`",
		"clear operator-facing text",
		"create a fresh preview",
		"temp files via the shared `write_payload` helper",
		"environment variables",
		"Linux CI runners",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README apply contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`POST /v1/applies/{applyId}/transitions`",
		"`400` malformed JSON",
		"`404` unknown apply",
		"`409` invalid terminal move",
		"operator-facing text",
		"smoke-created terminal `applyId`",
		"temp files through the shared `write_payload` helper",
		"Linux CI runners with tighter environment-size limits",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc apply contract to include %q", fragment)
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

func TestManagedLiveDriftDocsPreferFocusedContractCoverageOverLocalSmoke(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max`",
		"`runtimeContext.taskDefinition.image`",
		"Focused `go test ./internal/api` contract coverage locks that additive JSON/UI detail because local smoke may legitimately lack AWS access and therefore does not claim to prove live-backed capacity/image rows.",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README managed live drift docs to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`runtimeContext.capacity.{desired,min,max}`",
		"`runtimeContext.taskDefinition.image`",
		"focused router/UI contract tests lock that additive live-backed capacity/image detail because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard managed live drift doc to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`runtimeContext.taskDefinition.image`",
		"Local runs may legitimately lack AWS access, so focused `go test ./internal/api` contract coverage locks that additive JSON/UI detail instead.",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime managed live drift doc to include %q", fragment)
		}
	}
}

func TestDesiredStateSourceOperatorDocsContract(t *testing.T) {
	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"Both `tenet-api` and `tenet-controller` can read that desired state from either:",
		"`TENET_REPO_ROOT` remains the filesystem-mode input when you want a local checkout; it is not a universal runtime requirement.",
		"The AWS central-control-plane docs and Terraform bootstrap README now explicitly call out that the shipped repo-sync bootstrap path works out of the box only with an anonymously readable `repo_url` (for example a public HTTPS clone URL).",
		"If you plan to use the AWS central-control-plane Terraform bootstrap, do the preflight before `terraform apply`: it builds and pushes the current local Tenet checkout from the machine running apply, requires Terraform 1.5+, Docker Buildx, and AWS CLI v2 with working credentials, and expects you to verify that `repo_url` and `repo_branch` are reachable from that machine first, for example with `git ls-remote --heads <repo_url> <repo_branch>`.",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README desired-state source contract to include %q", fragment)
		}
	}

	architectureDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"Tenet selects desired-state reads with `TENET_CATALOG_SOURCE`:",
		"`filesystem` uses `TENET_REPO_ROOT` to point `tenet-api` and `tenet-controller` at the same checked-out repo or synced repo tree",
		"`github` uses `TENET_CATALOG_GITHUB_*` so both services read the same desired-state manifests directly from GitHub in read-only mode",
		"The shipped Terraform/bootstrap path in this doc still uses repo-sync plus shared filesystem delivery today because that works for both services now.",
		"That repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL.",
		"Private desired-state repo authentication is not wired into this module yet.",
		"the shipped Terraform bootstrap below uses the shared repo tree path at `/workspace/repo`",
		"### 3. Choose desired-state delivery",
		"The shipped Terraform/bootstrap path still uses the shared checkout plus repo-sync pattern today.",
		"If you use the shipped Terraform bootstrap, run this operator preflight on the same machine before `terraform apply`:",
		"`terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship",
		"Terraform 1.5+ is installed",
		"Docker Buildx is installed and working locally",
		"AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment",
		"verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`",
		"the shipped repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL",
		"private desired-state repo authentication is not wired into this module yet",
		"#### Filesystem-mode repo delivery options",
		"TENET_CATALOG_SOURCE=filesystem",
	} {
		if !strings.Contains(architectureDoc, fragment) {
			t.Fatalf("expected AWS central control-plane doc desired-state source contract to include %q", fragment)
		}
	}

	if strings.Contains(architectureDoc, "The current runtime contract is still filesystem-root based via `TENET_REPO_ROOT`") {
		t.Fatal("expected AWS central control-plane doc to drop stale filesystem-only runtime wording")
	}

	terraformDoc := readRepoFile(t, "deploy/terraform/aws-central-control-plane/README.md")
	for _, fragment := range []string{
		"Both `tenet-api` and `tenet-controller` now support `TENET_CATALOG_SOURCE=filesystem|github` for desired-state reads.",
		"This Terraform bootstrap intentionally chooses the filesystem path and sets `TENET_REPO_ROOT` because the shared repo-sync volume works for both services today.",
		"The shipped repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL.",
		"Private desired-state repo authentication is not wired into this module yet.",
		"`repo_url` (an anonymously readable clone URL, such as a public HTTPS clone URL)",
		"## Preflight before `terraform apply`",
		"Run this checklist on the same machine that will run `terraform apply`:",
		"`terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship",
		"Terraform 1.5+ is installed",
		"Docker Buildx is installed and working locally",
		"AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment",
		"verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`:",
		"the shipped repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL",
		"private desired-state repo authentication is not wired into this module yet",
	} {
		if !strings.Contains(terraformDoc, fragment) {
			t.Fatalf("expected Terraform README desired-state source contract to include %q", fragment)
		}
	}

	if strings.Contains(terraformDoc, "The current Tenet runtime still expects `TENET_REPO_ROOT` to point at a filesystem tree.") {
		t.Fatal("expected Terraform README to drop stale filesystem-only runtime wording")
	}

	terraformVars := readRepoFile(t, "deploy/terraform/aws-central-control-plane/variables.tf")
	for _, fragment := range []string{
		`description = "Git clone URL that the repo-sync sidecars keep mounted into both Tenet tasks. The shipped repo-sync path works out of the box only when this URL is anonymously readable, for example a public HTTPS clone URL. Private desired-state repo authentication is not wired into this module yet."`,
	} {
		if !strings.Contains(terraformVars, fragment) {
			t.Fatalf("expected Terraform variables desired-state source contract to include %q", fragment)
		}
	}

	terraformTfvarsExample := readRepoFile(t, "deploy/terraform/aws-central-control-plane/terraform.tfvars.example")
	for _, fragment := range []string{
		"# The shipped repo-sync path works out of the box only when `repo_url` is",
		"# anonymously readable, for example a public HTTPS clone URL. Private",
		"# desired-state repo authentication is not wired into this module yet.",
		`repo_url             = "https://github.com/DevopsArtFactory/tenet.git"`,
	} {
		if !strings.Contains(terraformTfvarsExample, fragment) {
			t.Fatalf("expected Terraform tfvars example desired-state source contract to include %q", fragment)
		}
	}

	terraformModule := readRepoFile(t, "deploy/terraform/aws-central-control-plane/main.tf")
	for _, fragment := range []string{
		`{ name = "TENET_CATALOG_SOURCE", value = "filesystem" },`,
		`{ name = "TENET_REPO_ROOT", value = "/workspace/repo" },`,
	} {
		if !strings.Contains(terraformModule, fragment) {
			t.Fatalf("expected Terraform bootstrap task definition to include %q", fragment)
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
		"`X-User-ID`",
		"`X-User-Name`",
		"`X-User-Email`",
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
		"`X-User-ID`",
		"`X-User-Name`",
		"`X-User-Email`",
		"Tenet prefers the explicit `X-Tenet-Actor-*` values over `X-User-*` aliases",
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

func TestFrontDoorAuthOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	for _, fragment := range []string{
		`TENET_API_TRUST_AUTHENTICATED_HEADERS: ${TENET_API_TRUST_AUTHENTICATED_HEADERS:-false}`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY: ${TENET_API_ENFORCE_AUTHENTICATED_IDENTITY:-false}`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION: ${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose auth contract to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_TRUSTED_HEADERS=${TENET_API_TRUST_AUTHENTICATED_HEADERS:-false}`,
		`AUTH_ENFORCED=${TENET_API_ENFORCE_AUTHENTICATED_IDENTITY:-false}`,
		`AUTH_READ_ENFORCED=${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS="$trust_authenticated_headers"`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY="$enforce_authenticated_identity"`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION="$enforce_deployment_read_authorization"`,
		`start_api true true false`,
		`start_api true false true`,
		`"http://$HOST:$PORT/v1/plans/deployments/payments-api-prod"`,
		`"http://$HOST:$PORT/v1/applies/plans/$plan_id"`,
		`"http://$HOST:$PORT/v1/plans/$frontdoor_plan_id"`,
		`"http://$HOST:$PORT/v1/applies/$frontdoor_apply_id"`,
		`"X-Tenet-Authenticated-Id: front-door-123"`,
		`"X-Tenet-Authenticated-Name: Front Door Operator"`,
		`"X-Tenet-Authenticated-Email: frontdoor@example.com"`,
		`"http://$HOST:$PORT/v1/overview/applications"`,
		`"http://$HOST:$PORT/v1/status/deployments"`,
		`"http://$HOST:$PORT/v1/status/deployments/payments-api-staging"`,
		`"X-Tenet-Authorized-Deployments: worker-prod"`,
		`"X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod"`,
		`assert os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_COLLECTION_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS']`,
		`assert unauthenticated_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_collection_body`,
		`assert unauthenticated_status_collection_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_collection_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_STATUS']`,
		`assert unauthenticated_plan_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_plan_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_TRANSITION_STATUS']`,
		`assert unauthenticated_transition_body.strip() == 'unauthenticated: authenticated identity is required for this operator write route', unauthenticated_transition_body`,
		`assert os.environ['AUTH_UNAUTHENTICATED_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_STATUS_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_PLAN_READ_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_APPLY_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_WRITE_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_WRITE_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_TRANSITION_STATUS']`,
		`assert unauthenticated_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_read_body`,
		`assert unauthenticated_status_read_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_status_read_body`,
		`assert os.environ['AUTH_FORBIDDEN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_STATUS_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_PLAN_READ_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_APPLY_READ_STATUS']`,
		`assert forbidden_plan_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_plan_write_body`,
		`assert forbidden_apply_write_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_apply_write_body`,
		`assert forbidden_transition_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_transition_body`,
		`assert forbidden_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_read_body`,
		`assert forbidden_status_read_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_status_read_body`,
		`assert allowed_status_read['deploymentName'] == 'payments-api-staging', allowed_status_read`,
		`filtered_status_deployment_names = {row['deploymentName'] for row in filtered_status_collection}`,
		`assert filtered_status_deployment_names == {'payments-api-prod'}, filtered_status_collection`,
		`assert allowed_guarded_read['name'] == 'payments-api-staging', allowed_guarded_read`,
		`filtered_application_names = {row['name'] for row in filtered_applications_collection}`,
		`assert 'worker-prod' in filtered_application_names, filtered_applications_collection`,
		`assert filtered_application_names == {'payments-api-prod', 'worker-prod'}, filtered_applications_collection`,
		`assert empty_applications_collection == [], empty_applications_collection`,
		`assert empty_status_collection == [], empty_status_collection`,
		`assert allowed_plan_read['deploymentName'] == 'payments-api-staging', allowed_plan_read`,
		`assert allowed_apply_read['deploymentName'] == 'payments-api-staging', allowed_apply_read`,
		`assert os.environ['AUTH_FORBIDDEN_AUDIT_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_AUDIT_STATUS']`,
		`assert forbidden_audit_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_audit_body`,
		`assert allowed_scoped_audit['entries'][0]['deploymentName'] == 'payments-api-staging', allowed_scoped_audit`,
		`assert 'Authenticated operator required' in ui, ui`,
		`assert 'const operatorWriteAuthRequiredMessage = (route, action) => ` + "`Tenet rejected ${route} because authenticated identity is required on operator write routes. When auth enforcement is enabled, retry ${action} through the trusted front door so it can authenticate you and forward trusted identity headers.`;" + `' in ui, ui`,
		`assert 'const operatorWriteForbiddenMessage = (route, action) => ` + "`Tenet rejected ${route} after recognizing the caller\\x27s identity, but this operator is not authorized to ${action.toLowerCase()} for that deployment through this route.`;" + `' in ui, ui`,
		`assert 'Operator access denied' in ui, ui`,
		`assert actor['id'] == 'front-door-123', payload`,
		`assert entry['actor']['email'] == 'frontdoor@example.com', entry`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local front-door auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`restart_api_with_frontdoor_read_auth() {`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS=true \`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \`,
		`auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/overview/applications")`,
		`auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/status/deployments")`,
		`auth_unauthenticated_transition_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_TRANSITION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions")`,
		`auth_forbidden_status_read_status=$(curl -sS -o "$AUTH_FORBIDDEN_STATUS_READ_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/status/deployments/payments-api-staging")`,
		`auth_forbidden_transition_status=$(curl -sS -o "$AUTH_FORBIDDEN_TRANSITION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/applies/$apply_id/transitions")`,
		`allowed_status_read=$(curl -fsS \`,
		`filtered_status_collection=$(curl -fsS \`,
		`empty_status_collection=$(curl -fsS \`,
		`"X-Tenet-Authenticated-Id: front-door-123"`,
		`"X-Tenet-Authenticated-Name: Front Door Operator"`,
		`"X-Tenet-Authenticated-Email: frontdoor@example.com"`,
		`"X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod"`,
		`assert os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_COLLECTION_STATUS"]`,
		`assert unauthenticated_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_collection_body`,
		`assert os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_STATUS_COLLECTION_STATUS"]`,
		`assert unauthenticated_status_collection_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_status_collection_body`,
		`assert os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_TRANSITION_STATUS"]`,
		`assert unauthenticated_transition_body.strip() == "unauthenticated: authenticated identity is required for this operator write route", unauthenticated_transition_body`,
		`assert os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_STATUS_READ_STATUS"]`,
		`assert forbidden_status_read_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_status_read_body`,
		`assert os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_TRANSITION_STATUS"]`,
		`assert forbidden_transition_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_transition_body`,
		`assert allowed_status_read["deploymentName"] == "payments-api-staging", allowed_status_read`,
		`filtered_application_names = {row["name"] for row in filtered_applications_collection}`,
		`assert "worker-prod" in filtered_application_names, filtered_applications_collection`,
		`assert filtered_application_names == {"payments-api-prod", "worker-prod"}, filtered_applications_collection`,
		`filtered_status_deployment_names = {row["deploymentName"] for row in filtered_status_collection}`,
		`assert filtered_status_deployment_names == {"payments-api-prod", "worker-prod"}, filtered_status_collection`,
		`assert empty_applications_collection == [], empty_applications_collection`,
		`assert empty_status_collection == [], empty_status_collection`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker front-door auth contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS=***",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for that trusted authenticated identity path",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized`",
		"`unauthenticated: authenticated identity is required for this operator write route`",
		"`unauthenticated: authenticated identity is required for this operator read route`",
		"`forbidden: operator does not have access to this deployment`",
		"authenticated identity is required on operator write routes",
		"`POST /v1/applies/{applyId}/transitions`",
		"write creation for `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}`",
		"persisted preview deployment",
		"persisted apply deployment",
		"`GET /v1/overview/applications` now requires trusted authenticated identity and filters the returned application rows to only deployments present in that allowlist",
		"`GET /v1/status/deployments` now follows the same collection contract",
		"`GET /v1/status/deployments/{deploymentName}` now reuses the same scoped deployment-read authorization contract",
		"`GET /v1/audit-logs` remain unchanged in this slice",
		"`200 OK` with an empty JSON array",
		"trusted allowlist is missing or none of the returned rows match it",
		"retry through the trusted front door",
		"only enable any of these settings when every request reaches Tenet through a front door that authenticates the caller, strips any client-supplied `X-Tenet-Authenticated-*` headers",
		"strips any client-supplied `X-Tenet-Authorized-Deployments` header",
		"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
		"`X-Tenet-Actor-*` values still win over `X-User-*` aliases for that local actor path",
		"the trusted authenticated identity path resolves from `X-Tenet-Authenticated-*` headers or additive `X-User-*` aliases, and when explicit actor headers are absent Tenet now projects that authenticated identity into the persisted actor fields",
		"restarting `tenet-api`",
		"restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=*** plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=*** to prove unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/status/deployments`, and unauthenticated `GET /v1/overview/accounts` fail with `401`",
		"unauthenticated `GET /v1/status/deployments`",
		"trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, raw status snapshots, and account aggregates",
		"when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows",
		"omitting that allowlist returns `[]` for those guarded collections",
		"`GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`",
		"one allowed raw status detail read succeeds",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README front-door auth contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS=***",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for the trusted authenticated identity path",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}`",
		"Tenet can also consume trusted front-door identity headers when the API instance is configured for them.",
		"Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments` before forwarding trusted values.",
		"A directly exposed header-trusting API instance is not a safe deployment shape",
		"`X-Tenet-Actor-*` values still win over `X-User-*` aliases for local actor context.",
		"Tenet projects that authenticated identity into the persisted actor fields for immutable plan/apply records and therefore into their projected audit entries.",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit doc front-door auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS=***",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***",
		"`X-Tenet-Authorized-Deployments`",
		"`X-Tenet-Authenticated-*` headers project into persisted plan/apply/audit actor fields",
		"deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments`",
		"including the persisted `POST /v1/applies/{applyId}/transitions` write route",
		"unauthorized `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` fail with the same explicit `403 Forbidden` deployment-access-denied response",
		"unauthenticated transition callers still short-circuit with the operator-write `401` before any apply lookup",
		"the bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures",
		"strips inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments`",
		"restarts the Compose `api` service",
		"unauthenticated `GET /v1/overview/applications`",
		"unauthenticated `GET /v1/status/deployments`",
		"unauthenticated `GET /v1/overview/accounts` fail with `401`",
		"unauthenticated `POST /v1/applies/{applyId}/transitions` fail with `401`",

		"unauthenticated `GET /v1/status/deployments`",
		"trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, raw status snapshots, and account aggregates",
		"when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows",
		"omitting that allowlist returns `[]` for those guarded collections",
		"`GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`",
		"one allowed raw status detail read succeeds",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc front-door auth contract to include %q", fragment)
		}
	}

	deploymentStatusDoc := readRepoFile(t, "docs/architecture/tenet-v1-deployment-status-api.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***`",
		"`X-Tenet-Authenticated-*`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/status/deployments` requires trusted `X-Tenet-Authenticated-*` identity and filters returned snapshots through the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"`GET /v1/status/deployments/{deploymentName}` requires exact deployment membership in that same allowlist",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`200 OK` with `[]`",
		"`make smoke-docker` restarts the Compose `api` service with trusted-header auth enabled and proves this exact route contract",
		"when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows",
		"Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments` before forwarding trusted values.",
	} {
		if !strings.Contains(deploymentStatusDoc, fragment) {
			t.Fatalf("expected deployment status doc front-door auth contract to include %q", fragment)
		}
	}

	runtimeRolloutSummaryDoc := readRepoFile(t, "docs/architecture/tenet-v1-runtime-rollout-summary.md")
	for _, fragment := range []string{
		"`GET /v1/status/deployments`",
		"`GET /v1/status/deployments/{deploymentName}`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"the raw status collection now requires trusted authenticated identity and filters snapshots to the authorized deployment subset",
		"the raw status detail route now follows the same deployment-scoped `401` / `403` contract as other guarded operator reads",
	} {
		if !strings.Contains(runtimeRolloutSummaryDoc, fragment) {
			t.Fatalf("expected runtime rollout summary doc front-door auth contract to include %q", fragment)
		}
	}

	overviewFeatureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` also require the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity and filters returned rows through the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"when the applications allowlist is omitted or matches nothing, the API returns `200 OK` with `[]`",
		"unscoped audit reads stay unchanged",
	} {
		if !strings.Contains(overviewFeatureDoc, fragment) {
			t.Fatalf("expected overview feature doc front-door auth contract to include %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"the trusted `X-Tenet-Authorized-Deployments` allowlist to include the requested deployment",
		"the persisted apply deployment for transitions",
		"deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/overview/applications`, which now requires trusted identity and filters returned rows through the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"returning `200 OK` with `[]` when that allowlist is omitted or has no matching deployments",
		"unscoped audit routes remain unchanged",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc front-door auth contract to include %q", fragment)
		}
	}
}

func TestGitHubWebhookOperatorContract(t *testing.T) {
	compose := readRepoFile(t, "compose.yaml")
	for _, fragment := range []string{
		`TENET_GITHUB_WEBHOOK_SECRET: ${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose github webhook contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`smoke-webhook-secret`",
		"`X-GitHub-Delivery`",
		"`X-GitHub-Event`",
		"`X-Hub-Signature-256`",
		"`202 Accepted`",
		"`200 OK`",
		"`400 Bad Request`",
		"`status=accepted`",
		"`status=ignored`",
		"`processed=false`",
		"`processed=true`",
		"`desiredStateSourceMatch`",
		"`matchStatus`",
		"`controllerRefresh`",
		"`TENET_STATUS_BACKEND=redis`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README github webhook contract to include %q", fragment)
		}
	}

	overviewDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview.md")
	for _, fragment := range []string{
		"`POST /api/v1/github/webhook`",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`accepted` or `ignored` responses",
		"`desiredStateSourceMatch`",
		"`controllerRefresh`",
		"`processed=false`",
		"`processed=true`",
	} {
		if !strings.Contains(overviewDoc, fragment) {
			t.Fatalf("expected overview architecture doc to include %q", fragment)
		}
	}

	controlPlaneDoc := readRepoFile(t, "docs/architecture/tenet-v1-aws-central-control-plane.md")
	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true`",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"strip any inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments` from untrusted clients",
		"callers can spoof operator identity or deployment access by sending those headers directly",
		"`TENET_GITHUB_WEBHOOK_SECRET`",
		"`POST /api/v1/github/webhook`",
		"`smoke-webhook-secret`",
		"HMAC-SHA256 signatures",
		"`202 Accepted`",
		"`200 OK`",
		"`400 Bad Request`",
		"`repository_mismatch`",
		"`ref_mismatch`",
		"`path_untouched`",
		"`matched`",
		"`controllerRefresh`",
		"`processed=true`",
		"`processed=false`",
	} {
		if !strings.Contains(controlPlaneDoc, fragment) {
			t.Fatalf("expected aws central control plane doc to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`WEBHOOK_SECRET=${TENET_GITHUB_WEBHOOK_SECRET:-smoke-webhook-secret}`,
		`webhook_push_status=$(curl -sS -o "$WEBHOOK_PUSH_BODY_FILE" -w "%{http_code}" -X POST \`,
		`"$API_BASE_URL/api/v1/github/webhook")`,
		`assert os.environ["WEBHOOK_PUSH_STATUS"] == "202", os.environ["WEBHOOK_PUSH_STATUS"]`,
		`assert webhook_push["desiredStateSourceMatch"]["kind"] == "filesystem", webhook_push`,
		`assert webhook_push["desiredStateSourceMatch"]["matchStatus"] == "filesystem_source", webhook_push`,
		`assert os.environ["WEBHOOK_PING_STATUS"] == "200", os.environ["WEBHOOK_PING_STATUS"]`,
		`assert webhook_ping["status"] == "ignored", webhook_ping`,
		`assert os.environ["INVALID_WEBHOOK_STATUS"] == "400", os.environ["INVALID_WEBHOOK_STATUS"]`,
		`assert "github webhook signature mismatch" in invalid_webhook, invalid_webhook`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker github webhook contract to include %q", fragment)
		}
	}

	script := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`TENET_GITHUB_WEBHOOK_SECRET="$WEBHOOK_SECRET"`,
		`webhook_push=$(curl -fsS -X POST \`,
		`http://$HOST:$PORT/api/v1/github/webhook`,
		`assert webhook_push['status'] == 'accepted', webhook_push`,
		`assert webhook_push['desiredStateSourceMatch']['kind'] == 'filesystem', webhook_push`,
		`assert webhook_push['desiredStateSourceMatch']['matchStatus'] == 'filesystem_source', webhook_push`,
		`assert webhook_ping['status'] == 'ignored', webhook_ping`,
		`assert 'desiredStateSourceMatch' not in webhook_ping, webhook_ping`,
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
