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
	for _, fragment := range []string{
		`TENET_PLAN_STORE_PATH: /tmp/tenet/plan-previews.json`,
		`TENET_APPLY_STORE_PATH: /tmp/tenet/apply-requests.json`,
		`- tenet-api-tmp:/tmp`,
		`tenet-api-tmp:`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("expected compose local api runtime to include %q", fragment)
		}
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
		`restarted_plan_preview=$(curl -fsS \`,
		`"$API_BASE_URL/v1/plans/$plan_id")`,
		`restarted_apply_record=$(curl -fsS \`,
		`"$API_BASE_URL/v1/applies/$apply_id")`,
		`restarted_plan_preview_path=$(write_payload restarted-plan-preview.json "$restarted_plan_preview")`,
		`restarted_apply_record_path=$(write_payload restarted-apply-record.json "$restarted_apply_record")`,
		`RESTARTED_PLAN_PREVIEW_PATH="$restarted_plan_preview_path" \`,
		`RESTARTED_APPLY_RECORD_PATH="$restarted_apply_record_path" \`,
		`assert restarted_plan_preview["id"] == os.environ["PLAN_ID"], restarted_plan_preview`,
		`assert restarted_apply_record["id"] == os.environ["APPLY_ID"], restarted_apply_record`,
		`assert restarted_apply_record["planId"] == os.environ["PLAN_ID"], restarted_apply_record`,
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
		`queued_deployment_detail=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/worker-prod")`,
		`queued_deployment_detail_extract_path=$(write_payload queued-deployment-detail.extract.json "$queued_deployment_detail")`,
		`queued_deployment_source_fingerprint=$(json_string_field_from_path "$queued_deployment_detail_extract_path" sourceFingerprint)`,
		`queued_plan_preview_request_body=$(DEPLOYMENT_SOURCE_FINGERPRINT="$queued_deployment_source_fingerprint" python3 - <<'PY'`,
		`queued_plan_preview=$(curl -fsS -X POST -H 'Content-Type: application/json' -d "$queued_plan_preview_request_body" "$API_BASE_URL/v1/plans/deployments/worker-prod")`,
		`queued_apply_record_get=$(wait_for_apply_status "$queued_apply_id" failed)`,
		`queued_apply_record=$(curl -fsS -X POST "$API_BASE_URL/v1/applies/plans/$queued_plan_id")`,
		`assert queued_plan_preview["deploymentName"] == "worker-prod", queued_plan_preview`,
		`assert queued_apply_record["deploymentName"] == "worker-prod", queued_apply_record`,
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
		"`TENET_PLAN_STORE_PATH`",
		"`invalid apply transition request body`",
		"`apply request not found`",
		"`TENET_APPLY_STORE_PATH`",
		"`/tmp/tenet/plan-previews.json`",
		"`/tmp/tenet/apply-requests.json`",
		"`/tmp`",
		"survive the Compose `api` service restart",
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
		"`TENET_PLAN_STORE_PATH=/tmp/tenet/plan-previews.json`",
		"`TENET_APPLY_STORE_PATH=/tmp/tenet/apply-requests.json`",
		"survive the Compose `api` service restart",
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
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"Focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows.",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README managed live drift docs to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`runtimeContext.capacity.{desired,min,max}`",
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"`runtimeContext.trafficRouting.weightedTargetGroups`",
		"`runtimeContext.trafficRouting.codeDeployTargetGroups`",
		"summary-only traffic-routing resources, unavailable AWS access, and local fixture-only runs intentionally omit those rows rather than inventing drift from static rollout summaries",
		"focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard managed live drift doc to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`runtimeContext.launchTemplate.bootstrapSource`",
		"`runtimeContext.launchTemplate.imageId`",
		"`runtimeContext.launchTemplate.instanceType`",
		"`runtimeContext.deploymentConfig.minimumHealthyPercent`",
		"`runtimeContext.deploymentConfig.maximumPercent`",
		"`runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`",
		"`runtimeContext.taskDefinition.image`",
		"`runtimeContext.trafficRouting.*`",
		"Local runs may legitimately lack AWS access, so focused API/controller unit coverage locks those additive JSON/UI and live-evidence derivation details instead.",
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
		"full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present",
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
		"`Clear audit filters` recovery action",
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
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION: ${TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION:-false}`,
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
		`AUTH_ROLE_ENFORCED=${TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION:-false}`,
		`AUTH_READ_ENFORCED=${TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION:-false}`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS="$trust_authenticated_headers"`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY="$enforce_authenticated_identity"`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION="$enforce_operator_role_authorization"`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION="$enforce_deployment_read_authorization"`,
		`start_api true true false false`,
		`start_api true false true true`,
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
		`"http://$HOST:$PORT/v1/operator/context")`,
		`operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")`,
		`operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")`,
		`operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")`,
		`operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")`,
		`"X-Tenet-Authorized-Deployments: worker-prod"`,
		`"X-Tenet-Authorized-Deployments: payments-api-staging, worker-prod"`,
		`"X-User-ID: alias-456"`,
		`"X-User-Name: Alias Operator"`,
		`"X-User-Email: alias@example.com"`,
		`"X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod"`,
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
		`assert operator_context_trusted['identity']['id'] == 'front-door-123', operator_context_trusted`,
		`assert operator_context_trusted['roles']['names'] == ['viewer', 'operator'], operator_context_trusted`,
		`assert enforcement.get('mode') == 'trusted_front_door_read_write', payload`,
		`assert enforcement.get('collectionReadsFiltered') is True, payload`,
		`assert operator_context_trusted['deploymentAccess']['names'] == ['payments-api-prod', 'worker-prod'], operator_context_trusted`,
		`assert trusted_summary['matchedCount'] == 2, operator_context_trusted`,
		`assert trusted_summary.get('unmatchedNames', []) == [], operator_context_trusted`,
		`assert trusted_summary['projectEnvironmentExamples'], operator_context_trusted`,
		`assert_operator_context_project_environment_examples(trusted_summary['projectEnvironmentExamples'], operator_context_trusted)`,
		`trusted_project_envs = {(row['appRef'], row['environmentRef']) for row in trusted_summary['projectEnvironmentExamples']}`,
		`assert operator_context_unmatched['deploymentAccess']['names'] == ['payments-api-prod', 'stale-api', 'worker-prod'], operator_context_unmatched`,
		`assert unmatched_summary['matchedCount'] == 2, operator_context_unmatched`,
		`assert unmatched_summary['unmatchedNames'] == ['stale-api'], operator_context_unmatched`,
		`assert operator_context_alias['identity']['id'] == 'alias-456', operator_context_alias`,
		`assert operator_context_alias['roles']['canWrite'] is False, operator_context_alias`,
		`assert 'names' not in operator_context_no_allowlist['deploymentAccess'], operator_context_no_allowlist`,
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
		`AUTH_UNAUTHENTICATED_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_AUDIT_COLLECTION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_PLAN_READ_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_APPLY_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_COLLECTION_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_STATUS_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_AUDIT_COLLECTION_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_PLAN_READ_BODY=$(mktemp)`,
		`AUTH_ROLE_REQUIRED_APPLY_READ_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_TRANSITION_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY=$(mktemp)`,
		`AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY=$(mktemp)`,
		`AUTH_FORBIDDEN_STATUS_READ_BODY=$(mktemp)`,
		`restart_api_with_frontdoor_read_auth() {`,
		`TENET_API_TRUST_AUTHENTICATED_HEADERS=true \`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true \`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true \`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true \`,
		`auth_unauthenticated_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/overview/applications")`,
		`auth_unauthenticated_status_collection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_STATUS_COLLECTION_BODY" -w "%{http_code}" \`,
		`"$API_BASE_URL/v1/status/deployments")`,
		`"$API_BASE_URL/v1/operator/context")`,
		`operator_context_trusted_path=$(write_payload operator-context-trusted.json "$operator_context_trusted")`,
		`operator_context_unmatched_path=$(write_payload operator-context-unmatched.json "$operator_context_unmatched")`,
		`operator_context_alias_path=$(write_payload operator-context-alias.json "$operator_context_alias")`,
		`operator_context_no_allowlist_path=$(write_payload operator-context-no-allowlist.json "$operator_context_no_allowlist")`,
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
		`"X-Tenet-Authenticated-Roles: viewer"`,
		`"X-Tenet-Authenticated-Roles: operator"`,
		`"X-Tenet-Authorized-Deployments: payments-api-prod, worker-prod"`,
		`project_environment_scope_headers = ["X-Tenet-Authorized-Projects", "X-Tenet-Authorized-Environments"]`,
		`assert access_verdicts.get("deploymentScopedRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload`,
		`assert access_verdicts.get("collectionRead", {}).get("requiredTrustedHeaders") == identity_headers + role_headers + allowlist_headers + project_environment_scope_headers, payload`,
		`assert trusted_verdicts["deploymentScopedRead"]["reason"] == "deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments", operator_context_trusted`,
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
		`assert operator_context_trusted["identity"]["id"] == "front-door-123", operator_context_trusted`,
		`assert operator_context_trusted["roles"]["names"] == ["viewer", "operator"], operator_context_trusted`,
		`assert enforcement.get("mode") == "trusted_front_door_read_write", payload`,
		`assert enforcement.get("collectionReadsFiltered") is True, payload`,
		`assert operator_context_trusted["deploymentAccess"]["names"] == ["payments-api-prod", "worker-prod"], operator_context_trusted`,
		`assert trusted_summary["matchedCount"] == 2, operator_context_trusted`,
		`assert trusted_summary.get("unmatchedNames", []) == [], operator_context_trusted`,
		`assert trusted_summary["projectEnvironmentExamples"], operator_context_trusted`,
		`assert_operator_context_project_environment_examples(trusted_summary["projectEnvironmentExamples"], operator_context_trusted)`,
		`trusted_project_envs = {(row["appRef"], row["environmentRef"]) for row in trusted_summary["projectEnvironmentExamples"]}`,
		`assert operator_context_unmatched["deploymentAccess"]["names"] == ["payments-api-prod", "stale-api", "worker-prod"], operator_context_unmatched`,
		`assert unmatched_summary["matchedCount"] == 2, operator_context_unmatched`,
		`assert unmatched_summary["unmatchedNames"] == ["stale-api"], operator_context_unmatched`,
		`assert operator_context_alias["identity"]["id"] == "alias-456", operator_context_alias`,
		`assert operator_context_alias["roles"]["canWrite"] is False, operator_context_alias`,
		`assert "names" not in operator_context_no_allowlist["deploymentAccess"], operator_context_no_allowlist`,
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
	for _, forbidden := range []string{
		`TENET_API_TRUST_AUTHENTICATED_HEADERS=*** \`,
		`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=*** \`,
		`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=*** \`,
		`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=*** \`,
	} {
		if strings.Contains(dockerSmoke, forbidden) {
			t.Fatalf("expected smoke-docker front-door auth contract to reject %q", forbidden)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-Tenet-Authenticated-Roles`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized`",
		"`unauthenticated: authenticated identity is required for this operator write route`",
		"`unauthenticated: authenticated identity is required for this operator read route`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`forbidden: authenticated viewer-capable role is required for this operator read route`",
		"`operator` / `admin`",
		"`GET /v1/operator/context` is a read-only diagnostic endpoint",
		"reports the normalized trusted identity, forwarded roles with read/write capability booleans, forwarded deployment allowlist, project/environment scopes, and which auth/RBAC enforcement toggles are active",
		"When deployment-read authorization is enforced and a catalog placement provider is available, `deploymentAccess.summary` also reports forwarded, matched/visible, and total unmatched deployment-name counts",
		"deterministic `projectEnvironmentExamples`",
		"`emptyReason`; if catalog placement data cannot be loaded, the summary stays present with `available=false` and `unavailableReason`",
		"plus `requiredTrustedHeaders` such as `X-Tenet-Authenticated-Id`, `X-User-ID`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments`",
		"both smoke lanes assert `GET /v1/operator/context` `deploymentAccess.summary` for all-matched and stale/unmatched forwarded allowlists, including region/AWS account/deployment-count fields on `projectEnvironmentExamples`",
		"That summary is diagnostic-only: it does not mint sessions, replace OAuth/RBAC, or authorize routes by itself",
		"filter to the union of exact deployment grants and project/environment catalog matches",
		"Missing, stale, or non-matching grants return `200 OK` with `[]` or `{\"entries\":[]}` on collection routes",
		"scoped reads return the existing operator-read `403` unless exact deployment membership or project/environment placement proves access",
		"retry through the trusted front door",
		"Use these settings only when every request reaches Tenet through a front door that authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values.",
		"Do not expose a header-trusting `tenet-api` instance directly to untrusted clients",
		"Tenet currently has two intentionally separate identity header families",
		"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
		"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
		"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
		"the trusted authenticated identity path projects that identity into persisted plan/apply/audit actor fields",
		"restarting `tenet-api`",
		"The same trusted front-door read contract covers `GET /v1/overview/accounts/{awsAccountId}`",
		"one trusted `X-Tenet-Authenticated-Roles: viewer` raw status detail read succeeds",
		"Operator troubleshooting checklist for guarded reads and writes",
		"Start in the embedded `Operator context` banner, or call `GET /v1/operator/context` directly",
		"Inspect the `accessVerdicts` chips and their bounded route examples",
		"Debug the specific guarded route class by checking the exact required trusted headers",
		"If an operator write returns `403 Forbidden`, confirm both `roles.canWrite=true` (`operator` or `admin`) and exact deployment membership",
		"curl -fsS http://localhost:8080/v1/operator/context",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README front-door auth contract to include %q", fragment)
		}
	}

	auditDoc := readRepoFile(t, "docs/architecture/tenet-v1-audit-log-api.md")
	for _, fragment := range []string{
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Id`",
		"`X-Tenet-Authenticated-Name`",
		"`X-Tenet-Authenticated-Email`",
		"`X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases",
		"`X-Tenet-Authenticated-*` values still win over `X-User-*` aliases",
		"`X-Tenet-Authorized-Deployments`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}`",
		"front-door identity headers",
		"strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, and trusted-alias `X-User-*` headers before forwarding trusted values",
		"not a safe deployment shape",
		"`X-Tenet-Actor-*` values still win over `X-User-*` aliases for local actor context.",
		"persisted actor fields for immutable plan/apply records",
	} {
		if !strings.Contains(auditDoc, fragment) {
			t.Fatalf("expected audit doc front-door auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true`",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"TENET_API_TRUST_AUTHENTICATED_HEADERS",
		"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
		"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
		"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
		"`X-Tenet-Authenticated-Roles`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`operator` / `admin`",
		"`X-Tenet-Authorized-Deployments`",
		"persisted plan/apply/audit actor fields",
		"Tenet currently has two intentionally separate identity header families",
		"`X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization",
		"`X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata",
		"Tenet does not treat raw browser-supplied `X-User-*` headers as a login session",
		"deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments`",
		"including the persisted `POST /v1/applies/{applyId}/transitions` write route",
		"the bookmarkable `/?page=plan` workspace",
		"strips inbound trust headers, including `X-User-*` aliases, before forwarding its own values",
		"Trusted front-door auth by restarting the Compose `api` service",
		"trusted `X-Tenet-Authorized-Deployments` filters returned application rows, raw status snapshots, account aggregates, and unscoped audit entries",
		"`[]` for guarded collections",
		"one trusted `X-Tenet-Authenticated-Roles: viewer` raw status detail read succeeds",
		"Guarded-route troubleshooting workflow",
		"Start in the embedded `Operator context` banner",
		"plus its bounded route examples and `requiredTrustedHeaders` to identify the affected route family",
		"`identity.authenticated`: `false` explains `401 Unauthorized` on guarded reads or writes",
		"`deploymentAccess.summary`: `matchedCount=0`, `emptyReason`, or non-empty `unmatchedNames` explains `200 OK` empty collections",
		"`projectEnvironmentExamples` shows which visible project/environment groups",
		"additive `requiredTrustedHeaders` (`X-Tenet-Authenticated-Id` or `X-User-ID`, `X-Tenet-Authenticated-Roles`, and/or `X-Tenet-Authorized-Deployments` depending on the route class)",
		"`accessVerdicts`: use the blocked or likely-empty verdict reason plus its bounded route examples and `requiredTrustedHeaders`",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=***",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=***",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***",
	} {
		if strings.Contains(localRuntimeDoc, forbidden) {
			t.Fatalf("expected local runtime doc front-door auth contract to reject %q", forbidden)
		}
	}

	deploymentStatusDoc := readRepoFile(t, "docs/architecture/tenet-v1-deployment-status-api.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` now covers this raw status family too.",
		"`X-Tenet-Authenticated-*`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/status/deployments` requires trusted `X-Tenet-Authenticated-*` identity and filters returned snapshots through the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"`GET /v1/status/deployments/{deploymentName}` requires exact deployment membership in that same allowlist",
		"`401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`",
		"`403 Forbidden` plus `forbidden: operator does not have access to this deployment`",
		"`200 OK` with `[]`",
		"`make smoke-docker` restarts the Compose `api` service with trusted-header auth enabled and proves this exact route contract",
		"when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows",
		"Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, and trusted-alias `X-User-*` headers before forwarding trusted values.",
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
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`X-Tenet-Authorized-Deployments`",
		"the raw status collection requires trusted authenticated identity and filters snapshots to the authorized deployment subset",
		"returning `200 OK` with `[]` when that allowlist is omitted or has no matches",
		"The raw status detail route follows the same deployment-scoped `401 Unauthorized` / `403 Forbidden` contract as other guarded operator reads",
		"missing trusted identity returns `unauthenticated: authenticated identity is required for this operator read route`",
		"a recognized operator without deployment membership gets `forbidden: operator does not have access to this deployment`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is the concrete toggle example for that guarded raw-status collection/detail contract",
	} {
		if !strings.Contains(runtimeRolloutSummaryDoc, fragment) {
			t.Fatalf("expected runtime rollout summary doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=***",
		"the raw status collection now requires",
		"Stated directly: the raw status detail route now follows",
	} {
		if strings.Contains(runtimeRolloutSummaryDoc, forbidden) {
			t.Fatalf("expected runtime rollout summary doc front-door auth contract to reject %q", forbidden)
		}
	}

	overviewFeatureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`",
		"`X-Tenet-Authenticated-Roles`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`forbidden: authenticated operator role is required for this operator write route`",
		"`operator` / `admin`",
		"`POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` also require the trusted `X-Tenet-Authorized-Deployments` allowlist",
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity and filters returned rows through the union of trusted `X-Tenet-Authorized-Deployments` exact grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"returns `200 OK` with `[]` when no grant matches anything",
		"`GET /v1/audit-logs`",
		"`{\"entries\":[]}`",
		"Guarded-read troubleshooting from the embedded dashboard follows the same diagnostic order as the API docs",
		"start with the `Operator context` banner",
		"inspect `identity.authenticated` for `401`, inspect `roles.canRead` before allowlist debugging for viewer-role `403`",
		"then inspect `accessVerdicts`, their bounded route examples, and their required trusted headers to choose the exact guarded route class",
		"it does not authorize routes client-side",
	} {
		if !strings.Contains(overviewFeatureDoc, fragment) {
			t.Fatalf("expected overview feature doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=***`",
	} {
		if strings.Contains(overviewFeatureDoc, forbidden) {
			t.Fatalf("expected overview feature doc front-door auth contract to reject %q", forbidden)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`",
		"`X-Tenet-Authorized-Deployments`",
		"`GET /v1/plans/{planId}`",
		"`GET /v1/applies/{applyId}`",
		"`POST /v1/applies/{applyId}/transitions`",
		"the trusted `X-Tenet-Authorized-Deployments` allowlist to include the requested deployment",
		"the persisted apply deployment for transitions",
		"deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}`",
		"`GET /v1/overview/applications`, which now requires trusted identity and filters returned rows through the union of trusted `X-Tenet-Authorized-Deployments` exact grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement",
		"returning `200 OK` with `[]` when no grant matches anything",
		"`GET /v1/audit-logs` now follows that same collection contract under read enforcement",
		"`{\"entries\":[]}`",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc front-door auth contract to include %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"compatibility fragments for current string-contract tests",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
	} {
		if strings.Contains(overviewUIDoc, forbidden) {
			t.Fatalf("expected overview UI doc to keep operator-facing prose instead of compatibility-only fragment %q", forbidden)
		}
	}
}

func TestResourceDrillDownFrontDoorAuthOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=`,
		`auth_unauthenticated_resource_logs_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" -w "%{http_code}" \`,
		`auth_unauthenticated_resource_inspection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \`,
		`auth_forbidden_resource_logs_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" -w "%{http_code}" \`,
		`auth_forbidden_resource_inspection_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \`,
		`allowed_resource_logs=$(curl -fsS \`,
		`allowed_resource_inspection=$(curl -fsS \`,
		`"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")`,
		`"http://$HOST:$PORT/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")`,
		`assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS']`,
		`assert os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS']`,
		`assert os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS']`,
		`assert allowed_resource_logs['deploymentName'] == 'worker-prod', allowed_resource_logs`,
		`assert allowed_resource_logs['resourceType'] == 'ecsService', allowed_resource_logs`,
		`assert allowed_resource_logs['resourceName'] == 'worker-prod-service', allowed_resource_logs`,
		`assert isinstance(allowed_resource_logs['available'], bool), allowed_resource_logs`,
		`assert allowed_resource_inspection['deploymentName'] == 'worker-prod', allowed_resource_inspection`,
		`assert allowed_resource_inspection['resourceType'] == 'ecsService', allowed_resource_inspection`,
		`assert allowed_resource_inspection['resourceName'] == 'worker-prod-service', allowed_resource_inspection`,
		`assert isinstance(allowed_resource_inspection['available'], bool), allowed_resource_inspection`,
		`assert isinstance(allowed_resource_logs.get('unavailableReason'), str) and allowed_resource_logs['unavailableReason'], allowed_resource_logs`,
		`assert isinstance(allowed_resource_inspection.get('unavailableReason'), str) and allowed_resource_inspection['unavailableReason'], allowed_resource_inspection`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local resource drill-down auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY=`,
		`auth_unauthenticated_resource_logs_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_LOGS_BODY" -w "%{http_code}" \`,
		`auth_unauthenticated_resource_inspection_status=$(curl -sS -o "$AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \`,
		`auth_forbidden_resource_logs_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_LOGS_BODY" -w "%{http_code}" \`,
		`auth_forbidden_resource_inspection_status=$(curl -sS -o "$AUTH_FORBIDDEN_RESOURCE_INSPECTION_BODY" -w "%{http_code}" \`,
		`allowed_resource_logs=$(curl -fsS \`,
		`allowed_resource_inspection=$(curl -fsS \`,
		`"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service")`,
		`"$API_BASE_URL/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service")`,
		`assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_LOGS_STATUS"]`,
		`assert os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_RESOURCE_INSPECTION_STATUS"]`,
		`assert os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_LOGS_STATUS"]`,
		`assert os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_RESOURCE_INSPECTION_STATUS"]`,
		`assert allowed_resource_logs["deploymentName"] == "worker-prod", allowed_resource_logs`,
		`assert allowed_resource_logs["resourceType"] == "ecsService", allowed_resource_logs`,
		`assert allowed_resource_logs["resourceName"] == "worker-prod-service", allowed_resource_logs`,
		`assert isinstance(allowed_resource_logs["available"], bool), allowed_resource_logs`,
		`assert allowed_resource_inspection["deploymentName"] == "worker-prod", allowed_resource_inspection`,
		`assert allowed_resource_inspection["resourceType"] == "ecsService", allowed_resource_inspection`,
		`assert allowed_resource_inspection["resourceName"] == "worker-prod-service", allowed_resource_inspection`,
		`assert isinstance(allowed_resource_inspection["available"], bool), allowed_resource_inspection`,
		`assert isinstance(allowed_resource_logs.get("unavailableReason"), str) and allowed_resource_logs["unavailableReason"], allowed_resource_logs`,
		`assert isinstance(allowed_resource_inspection.get("unavailableReason"), str) and allowed_resource_inspection["unavailableReason"], allowed_resource_inspection`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker resource drill-down auth contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`401 Unauthorized` unauthenticated reads",
		"`403 Forbidden` authenticated-but-unauthorized reads",
		"`200 OK` authorized reads",
		"`available=false` plus explicit `unavailableReason` payload contract",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource drill-down auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"The same trusted front-door read contract also covers",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc resource drill-down auth contract to include %q", fragment)
		}
	}

	overviewUIDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs`",
		"`GET /v1/overview/deployments/{deploymentName}/resource-inspection`",
		"`401`",
		"`403`",
		"`available=false` plus `unavailableReason`",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI doc resource drill-down auth contract to include %q", fragment)
		}
	}
}

func TestDeploymentEventsFrontDoorAuthOperatorContract(t *testing.T) {
	localSmoke := readRepoFile(t, "scripts/smoke-local.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`"http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/events")`,
		`auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \`,
		`allowed_events_read=$(curl -fsS \`,
		`allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")`,
		`AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status"`,
		`AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status"`,
		`ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path"`,
		`assert os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS'] == '401', os.environ['AUTH_UNAUTHENTICATED_EVENTS_STATUS']`,
		`assert unauthenticated_events_body.strip() == 'unauthenticated: authenticated identity is required for this operator read route', unauthenticated_events_body`,
		`assert os.environ['AUTH_FORBIDDEN_EVENTS_STATUS'] == '403', os.environ['AUTH_FORBIDDEN_EVENTS_STATUS']`,
		`assert forbidden_events_body.strip() == 'forbidden: operator does not have access to this deployment', forbidden_events_body`,
		`assert allowed_events_read['deploymentName'] == 'payments-api-staging', allowed_events_read`,
		`assert isinstance(allowed_events_read.get('events'), list), allowed_events_read`,
	} {
		if !strings.Contains(localSmoke, fragment) {
			t.Fatalf("expected smoke-local deployment events auth contract to include %q", fragment)
		}
	}

	dockerSmoke := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`AUTH_UNAUTHENTICATED_EVENTS_BODY=`,
		`AUTH_FORBIDDEN_EVENTS_BODY=`,

		`"$API_BASE_URL/v1/overview/deployments/payments-api-staging/events")`,
		`auth_forbidden_events_status=$(curl -sS -o "$AUTH_FORBIDDEN_EVENTS_BODY" -w "%{http_code}" \`,
		`allowed_events_read=$(curl -fsS \`,
		`allowed_events_read_path=$(write_payload allowed-events-read.json "$allowed_events_read")`,
		`AUTH_UNAUTHENTICATED_EVENTS_STATUS="$auth_unauthenticated_events_status"`,
		`AUTH_FORBIDDEN_EVENTS_STATUS="$auth_forbidden_events_status"`,
		`ALLOWED_EVENTS_READ_PATH="$allowed_events_read_path"`,
		`assert os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"] == "401", os.environ["AUTH_UNAUTHENTICATED_EVENTS_STATUS"]`,
		`assert unauthenticated_events_body.strip() == "unauthenticated: authenticated identity is required for this operator read route", unauthenticated_events_body`,
		`assert os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"] == "403", os.environ["AUTH_FORBIDDEN_EVENTS_STATUS"]`,
		`assert forbidden_events_body.strip() == "forbidden: operator does not have access to this deployment", forbidden_events_body`,
		`assert allowed_events_read["deploymentName"] == "payments-api-staging", allowed_events_read`,
		`assert isinstance(allowed_events_read.get("events"), list), allowed_events_read`,
	} {
		if !strings.Contains(dockerSmoke, fragment) {
			t.Fatalf("expected smoke-docker deployment events auth contract to include %q", fragment)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`401 Unauthorized` unauthenticated reads",
		"`403 Forbidden` authenticated-but-unauthorized reads",
		"`200 OK` authorized reads",
		"`deploymentName` plus `events` list payload contract",
		"additive `managedDriftSummary` severity counts",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README deployment events auth contract to include %q", fragment)
		}
	}

	localRuntimeDoc := readRepoFile(t, "docs/architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events`",
		"`401` / `403` / `200 OK` contract",
		"`deploymentName` plus `events` list shape",
		"additive `managedDriftSummary` object carrying explicit zero/non-zero severity counts",
	} {
		if !strings.Contains(localRuntimeDoc, fragment) {
			t.Fatalf("expected local runtime doc deployment events auth contract to include %q", fragment)
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
		"strip any inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, and trusted-alias `X-User-*` headers from untrusted clients",
		"callers can spoof operator identity or deployment access by sending those headers directly",
		"does not currently run a built-in GitHub OAuth browser login, issue Tenet-managed browser sessions, or manage an OAuth/OIDC provider lifecycle inside `tenet-api`",
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
		`traffic_resource_logs=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")`,
		`assert traffic_resource_logs.get('unavailableReason') == 'no_log_source', traffic_resource_logs`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("expected smoke-local resource logs contract to include %q", fragment)
		}
	}

	dockerScript := readRepoFile(t, "scripts/smoke-docker.sh")
	for _, fragment := range []string{
		`traffic_resource_logs=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing")`,
		`assert traffic_resource_logs.get("unavailableReason") == "no_log_source", traffic_resource_logs`,
	} {
		if !strings.Contains(dockerScript, fragment) {
			t.Fatalf("expected smoke-docker resource logs contract to include %q", fragment)
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
		"`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` also preserve the same route and lookup-context envelope",
		"`unavailableReason=no_log_source`",
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
		"newest running and recently stopped service task/container streams",
		"failed rollouts and crash loops can still surface useful logs",
		"Current route-stable rollout resources with explicit no-log-source responses",
		"weighted-target-group traffic-routing resources (`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup`)",
		"This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.",
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
		`production_listener_rule_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")`,
		`active_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")`,
		`candidate_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")`,
		`blue_task_set_target_group_resource_inspection=$(curl -fsS "http://$HOST:$PORT/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")`,
		`minimum_healthy_percent = worker_resource_inspection['ecsService'].get('minimumHealthyPercent')`,
		`maximum_percent = worker_resource_inspection['ecsService'].get('maximumPercent')`,
		`health_check_grace_period_seconds = worker_resource_inspection['ecsService'].get('healthCheckGracePeriodSeconds')`,
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
		`production_listener_rule_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener")`,
		`active_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active")`,
		`candidate_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateTargetGroup&resourceName=payments-api-prod-candidate")`,
		`blue_task_set_target_group_resource_inspection=$(curl -fsS "$API_BASE_URL/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue")`,
		`minimum_healthy_percent = worker_resource_inspection["ecsService"].get("minimumHealthyPercent")`,
		`maximum_percent = worker_resource_inspection["ecsService"].get("maximumPercent")`,
		`health_check_grace_period_seconds = worker_resource_inspection["ecsService"].get("healthCheckGracePeriodSeconds")`,
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
		"Auto Scaling payloads add `autoScalingGroup`, including optional `launchTemplate` and `mixedInstancesPolicy` metadata",
		"`productionListenerRule`",
		"`activeTargetGroup`",
		"`candidateTargetGroup`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId` and `launchTemplate.instanceType`",
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule`",
		"`ruleArn`, `priority`, `isDefault`, `conditions`, `actions`",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("expected README resource inspection contract to include %q", fragment)
		}
	}

	featureDoc := readRepoFile(t, "docs/features/overview-dashboard.md")
	for _, fragment := range []string{
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`DescribeAutoScalingGroups`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule` metadata",
	} {
		if !strings.Contains(featureDoc, fragment) {
			t.Fatalf("expected overview dashboard doc to include %q", fragment)
		}
	}

	uiDoc := readRepoFile(t, "docs/architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"`minimumHealthyPercent`",
		"`maximumPercent`",
		"`healthCheckGracePeriodSeconds`",
		"`launchTemplate`",
		"`mixedInstancesPolicy`",
		"`blueTaskSetTargetGroup`",
		"`greenTaskSetTargetGroup`",
		"`launchTemplate.imageId`",
		"`launchTemplate.instanceType`",
		"`listenerRule` details",
	} {
		if !strings.Contains(uiDoc, fragment) {
			t.Fatalf("expected overview UI doc to include %q", fragment)
		}
	}
}
