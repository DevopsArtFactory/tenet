package docs

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestREADMEValidationDocumentsCISmokeFastLaneScriptChecks(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	workflowText := readTextFile(t, "../.github/workflows/ci-smoke.yml")
	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")
	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	for _, command := range []string{
		"go test ./...",
		"bash -n scripts/smoke-local.sh",
		"bash -n scripts/smoke-docker.sh",
		"bash -n scripts/ci-smoke-runner-evidence.sh",
		"make smoke-local",
	} {
		if !strings.Contains(readmeText, command) {
			t.Fatalf("expected README validation docs to include %q", command)
		}
		if !strings.Contains(workflowText, "run: "+command) {
			t.Fatalf("expected CI Smoke workflow to include %q", command)
		}
	}

	for _, fragment := range []string{
		"CI Smoke mirrors those checks in the hosted fast lane",
		"Hosted fast-lane order is `go test ./...`, smoke helper syntax checks, then `make smoke-local`",
		"operator smoke helpers",
		"The Docker smoke lane remains separate",
		"In hosted PRs it is path-scoped",
		"docs-only changes normally stay on the fast lane",
		"changes under Docker/Compose, front-door auth, API/controller, local fixtures, runtime read models, RBAC policy diagnostics, resource evidence, webhook, plan/apply, or audit smoke surfaces run the full Docker stack before merge",
		"make smoke-docker",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README validation docs to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"internal/rbacpolicy/*|internal/rbacpolicy/**",
		"RBAC policy diagnostics",
	} {
		if !strings.Contains(workflowText, fragment) {
			t.Fatalf("expected CI Smoke Docker scope to include %q", fragment)
		}
	}

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "local runtime docs", text: localRuntimeText},
		{name: "operator contracts docs", text: operatorContractsText},
	} {
		if !strings.Contains(doc.text, "RBAC policy diagnostics, resource evidence") {
			t.Fatalf("expected %s to document RBAC policy diagnostics in the broad Docker-smoke scope", doc.name)
		}
	}
}

func TestComposeLocalRuntimeUsesExplicitFilesystemCatalogSource(t *testing.T) {
	composeText := readTextFile(t, "../compose.yaml")
	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")

	if got := strings.Count(composeText, "TENET_CATALOG_SOURCE: filesystem"); got != 2 {
		t.Fatalf("expected compose local runtime to set TENET_CATALOG_SOURCE=filesystem for api and controller, got %d occurrences", got)
	}
	if !strings.Contains(localRuntimeText, "`TENET_CATALOG_SOURCE=filesystem`") {
		t.Fatalf("expected local runtime docs to keep the filesystem catalog source contract")
	}
}

func TestReadmeDocumentsRawDeploymentStatusProbes(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"curl -fsS http://localhost:18083/v1/status/deployments",
		"`GET /v1/status/deployments` and `GET /v1/status/deployments/{deploymentName}` for raw persisted runtime snapshots",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README raw deployment-status docs to include %q", fragment)
		}
	}
}

func TestReadmeDocumentsRecentLogResultScopeFields(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`",
		"`resultScope.requestedLimit`",
		"`effectiveLimit`",
		"`returnedEntryCount`",
		"`returnedCount`",
		"`truncationStatus`",
		"`truncated`",
		"compact aliases such as `limit`/`returned` keep bounded reads terse",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README recent-log resultScope docs to include %q", fragment)
		}
	}
}

func TestOverviewDashboardDocumentsOperatorContextFreshnessPlacement(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	section := markdownSection(t, overviewText, "### Applications overview")
	for _, fragment := range []string{
		"a `Last checked` timestamp for the last successful context read inside expanded `Show access details` rather than the compact banner",
		"A status-only live region announces only compact checking/loading, ready identity, refresh, unavailable, and error status changes",
	} {
		if !strings.Contains(section, fragment) {
			t.Fatalf("expected applications overview docs to include operator-context freshness contract %q", fragment)
		}
	}
	if strings.Contains(section, "a visible `Last checked` timestamp for the last successful context read") {
		t.Fatalf("expected applications overview docs to avoid stale compact-banner Last checked wording")
	}
}

func TestOverviewUIDocumentsApplicationFilterResetTooltips(t *testing.T) {
	overviewText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	section := markdownSection(t, overviewText, "### 1. Applications overview")
	for _, fragment := range []string{
		"the toolbar and filtered-empty reset buttons mirror that same active-filter list in their native tooltip",
		"pointer users can inspect the exact `q`, `sync`, `health`, and `account` state before clearing it",
	} {
		if !strings.Contains(section, fragment) {
			t.Fatalf("expected overview UI docs to include application filter reset tooltip contract %q", fragment)
		}
	}

	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"active-filter tooltip",
		"names the exact search, sync, health, and account state before reset",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include application filter reset tooltip summary %q", fragment)
		}
	}
}

func TestSmokeHelperScriptsRemainDirectlyRunnable(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"Smoke helper scripts must keep their executable bits",
		"Makefile targets and runner-start docs invoke them directly by path",
		"remove every mktemp-backed helper file or directory during cleanup",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README validation docs to include executable helper contract %q", fragment)
		}
	}

	for _, path := range []string{
		"../scripts/smoke-local.sh",
		"../scripts/smoke-docker.sh",
		"../scripts/ci-smoke-runner-evidence.sh",
		"../scripts/docker-up.sh",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode()&0111 == 0 {
			t.Fatalf("expected %s to keep at least one executable bit because operators invoke it directly", path)
		}
		if !strings.HasPrefix(readTextFile(t, path), "#!/usr/bin/env bash\n") {
			t.Fatalf("expected %s to keep its bash shebang", path)
		}
	}
}

func TestSmokeHelpersCleanUpEveryTempPath(t *testing.T) {
	assignmentRE := regexp.MustCompile(`(?m)^\s*(?:local\s+)?([A-Za-z_][A-Za-z0-9_]*)=\$\(mktemp(?:\s+([^)]*))?\)$`)
	cleanupRE := regexp.MustCompile(`(?s)cleanup\(\) \{(.*?)\n\}`)

	for _, path := range []string{
		"../scripts/smoke-local.sh",
		"../scripts/smoke-docker.sh",
		"../scripts/ci-smoke-runner-evidence.sh",
	} {
		text := readTextFile(t, path)
		assignments := assignmentRE.FindAllStringSubmatch(text, -1)
		if len(assignments) == 0 {
			continue
		}

		cleanupMatch := cleanupRE.FindStringSubmatch(text)
		if cleanupMatch == nil {
			t.Fatalf("expected %s to define a cleanup function when it creates mktemp-backed paths", path)
		}
		cleanupBody := cleanupMatch[1]

		for _, match := range assignments {
			variable := match[1]
			mktempArgs := strings.TrimSpace(match[2])
			if mktempUnderPayloadDir(mktempArgs) {
				continue
			}
			if !strings.Contains(cleanupBody, "$"+variable) {
				t.Fatalf("expected %s mktemp assignment for %s to be under PAYLOAD_DIR or removed by cleanup", path, variable)
			}
		}
	}
}

func mktempUnderPayloadDir(args string) bool {
	args = strings.Trim(args, `"'`)
	return args == "$PAYLOAD_DIR" || strings.HasPrefix(args, "$PAYLOAD_DIR/")
}

func TestOperatorTroubleshootingRunbookDocumentsDailyTriageFlow(t *testing.T) {
	runbookText := readTextFile(t, "runbooks/operator-troubleshooting.md")

	for _, fragment := range []string{
		"GET /v1/operator/context",
		"POST /v1/controller/refresh",
		"/?page=audit&deployment={deploymentName}&action=controller.refresh.requested",
		"GET /v1/controller/refresh/status",
		"/?page=application&deployment={deploymentName}",
		"/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}",
		"Request-Id",
		"Tenet request ID",
		"lookupContext",
		"aws_unavailable",
		"no_log_source",
		"source_mismatch",
		"no retained controller history",
	} {
		if !strings.Contains(runbookText, fragment) {
			t.Fatalf("expected operator troubleshooting runbook to preserve daily triage fragment %q", fragment)
		}
	}

	orderedFragments := []string{
		"## Identity and Refresh Preflight",
		"## Surface Matrix",
		"## Unavailable-State Decoder",
		"## Verification Commands",
	}
	last := -1
	for _, fragment := range orderedFragments {
		idx := strings.Index(runbookText, fragment)
		if idx < 0 {
			t.Fatalf("expected operator troubleshooting runbook to include ordered section %q", fragment)
		}
		if idx <= last {
			t.Fatalf("expected operator troubleshooting section %q to appear after the previous triage section", fragment)
		}
		last = idx
	}
}

func TestRunnerStartRunbookDocumentsEvidenceHelperContract(t *testing.T) {
	runbookText := readTextFile(t, "runbooks/ci-smoke-runner-start.md")
	helperText := readTextFile(t, "../scripts/ci-smoke-runner-evidence.sh")
	readmeText := readTextFile(t, "../README.md")
	workflowText := readTextFile(t, "../.github/workflows/ci-smoke.yml")

	for _, fragment := range []string{
		"scripts/ci-smoke-runner-evidence.sh <run-id>",
		"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
		"`--summary` and `--markdown` are aliases",
		"Runner-start verdict",
		"runner_id=0",
		"steps=[]",
		"log not found",
		"billing/spending-limit",
		"bash -n scripts/ci-smoke-runner-evidence.sh",
	} {
		if !strings.Contains(runbookText, fragment) {
			t.Fatalf("expected runner-start runbook to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"Usage: scripts/ci-smoke-runner-evidence.sh [--summary|--markdown] <run-id>",
		"if [[ \"${1:-}\" == \"--summary\" || \"${1:-}\" == \"--markdown\" ]]",
		"section \"Runner-start verdict\"",
		"runner_id=",
		"steps=[] before repository commands ran",
		"log not found",
		"billing/spending-limit",
		"### CI Smoke runner-start evidence",
	} {
		if !strings.Contains(helperText, fragment) {
			t.Fatalf("expected CI smoke evidence helper to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"scripts/ci-smoke-runner-evidence.sh --summary <run-id>",
		"docs/runbooks/ci-smoke-runner-start.md",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to point operators at runner-start evidence helper/runbook via %q", fragment)
		}
	}

	if !strings.Contains(workflowText, "run: bash -n scripts/ci-smoke-runner-evidence.sh") {
		t.Fatalf("expected CI Smoke workflow to syntax-check runner-start evidence helper")
	}
}

func TestDeploymentRuntimeEventsDocsTrackSmokeSourceFilterContracts(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")
	smokeLocalText := readTextFile(t, "../scripts/smoke-local.sh")
	smokeDockerText := readTextFile(t, "../scripts/smoke-docker.sh")

	for _, fragment := range []string{
		"events?source=controller&limit=1",
		"events?source=bad%20source",
		"INVALID_DEPLOYMENT_EVENTS_SOURCE_STATUS",
		"invalid source query",
	} {
		if !strings.Contains(smokeLocalText, fragment) {
			t.Fatalf("expected direct-process smoke to include deployment-events source-filter contract %q", fragment)
		}
	}

	for _, fragment := range []string{
		"events?limit=1",
		"requestedLimit",
		"effectiveLimit",
		"returnedEventCount",
		"sourceSummary",
		"truncationStatus",
	} {
		if !strings.Contains(smokeDockerText, fragment) {
			t.Fatalf("expected Docker smoke to include bounded deployment-events resultScope contract %q", fragment)
		}
	}

	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events?source={source}&limit={positiveInteger}` for exact source-filtered, bounded controller-history reads",
		"controller-history timeline is not the immutable audit log",
		"Invalid source values return `400 Bad Request` with `invalid source query`",
		"direct-process smoke covers that validation while Compose smoke keeps the bounded `?limit=1` runtime-events `resultScope` and `sourceSummary` contract exercised",
		"`eventLimit` and optional `eventSource`",
		"`Copy runtime events route` handoff for the exact selected read",
		"the selector offers `All sources`, `controller`, `deployment-controller`, and `github`",
		"exact bookmarked custom source values remain visible instead of being discarded",
		"`retention.limit` may still be non-zero",
		"`sourceSummary` array of `{source, count}` entries computed from the retained window before optional source filtering and before positive limit truncation (blank persisted sources are bucketed as `unknown`, and `?source=unknown` selects those legacy entries)",
		"`returnedCount` as a compact alias for `returnedEventCount`",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README deployment-events docs to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"`GET /v1/overview/deployments/{deploymentName}/events?source={source}&limit={n}` for exact source-filtered windows, with source filtering applied before limit",
		"It is a controller-history timeline for operator drill-down and timeline rendering, not the immutable audit log",
		"Invalid source filters fail before a controller-history read with `400 Bad Request` and `invalid source query`",
		"`sourceSummary` array of `{source, count}` entries derived from the retained controller-history window before optional source filtering and before positive limit truncation, normalizing blank persisted sources into an explicit `unknown` bucket",
		"`source=unknown` selects retained legacy events whose persisted source is blank or whitespace",
		"compact alias `returnedCount`",
		"compact boolean alias `truncated`",
		"keeps selected source state in `eventSource`",
		"preserves `eventLimit` plus `eventSource` in share links",
		"includes bookmarkable `Event window` and `Event source` selectors",
		"writes `eventLimit` and optional `eventSource` into `/?page=application&deployment=...`",
		"`GET /v1/overview/deployments/{deploymentName}/events?limit={n}[&source={source}]`",
		"`Copy runtime events route` handoff for the exact selected read",
		"The selector offers `All sources`, `controller`, `deployment-controller`, and `github`",
		"exact bookmarked custom source values remain visible instead of being discarded",
	} {
		if !strings.Contains(overviewText, fragment) {
			t.Fatalf("expected overview dashboard deployment-events docs to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"Invalid source filters return `400 Bad Request` with `invalid source query`.",
		"additive `sourceSummary` entries with `source` and `count` computed from the retained runtime window before optional source filtering and before positive limit truncation, including the `unknown` bucket for blank persisted sources",
		"calls the fixture-empty deployment-events route with `?limit=1` and `?source=controller&limit=1`",
		"direct-process smoke owns retention metadata, exact `?source=controller&limit=1`, and invalid-source `400` coverage",
	} {
		if !strings.Contains(localRuntimeText, fragment) {
			t.Fatalf("expected local runtime smoke docs to include %q", fragment)
		}
	}

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "overview UI architecture docs", text: overviewUIText},
		{name: "operator contract docs", text: operatorContractsText},
	} {
		for _, fragment := range []string{
			"invalid source query",
			"invalid limit query",
			"source-filtered",
			"Runtime event navigation",
			"Newest visible event",
			"Oldest visible event",
			"Jump to newest event",
			"Jump to oldest visible event",
			"eventLimit",
			"eventSource",
			"Copy runtime events route",
		} {
			if !strings.Contains(doc.text, fragment) {
				t.Fatalf("expected %s deployment-events docs to include %q", doc.name, fragment)
			}
		}
	}

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "direct-process smoke", text: smokeLocalText},
		{name: "Docker smoke", text: smokeDockerText},
	} {
		for _, fragment := range []string{
			"Runtime event navigation",
			"Newest visible event",
			"Oldest visible event",
			"Jump to newest event",
			"Jump to oldest visible event",
			"Copy runtime events route",
			"deploymentEventsReadRoute(deploymentName, currentPageState.eventLimit, currentPageState.eventSource)",
			"const createDeploymentEventsNavigation = (deploymentName, eventRows, limit, source) => {",
		} {
			if !strings.Contains(doc.text, fragment) {
				t.Fatalf("expected %s deployment-events smoke contract to include %q", doc.name, fragment)
			}
		}
	}
}

func TestDocumentationMapLinksResourceInspectionGuide(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	overviewText := readTextFile(t, "architecture/tenet-v1-overview.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	contractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	featureText := readTextFile(t, "features/resource-inspection.md")
	recentLogsFeatureText := readTextFile(t, "features/resource-recent-logs.md")

	for _, fragment := range []string{
		"`docs/features/resource-inspection.md` documents the focused live resource inspection route, selectors, lookup context, unavailable states, and detailed field contracts.",
		"`docs/features/resource-recent-logs.md` documents recent-log resource contracts, including lookup context, selector echoes, and friendly empty/unavailable states.",
		"Resource drill-down Recent logs and Live inspection panels summarize lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract.",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README documentation map to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"`ecsService.deployments[]`",
		"`autoScalingGroup.scalingActivities[]`",
		"`deploymentControllerType`",
		"`deploymentCircuitBreaker`",
		"`deploymentAlarmConfiguration`",
		"`resultScope.returnedItemCount`",
		"`Traffic-routing status`",
	} {
		if strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to link detailed resource-inspection field contracts instead of duplicating %q", fragment)
		}
	}

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "overview", text: overviewText},
		{name: "overview UI", text: overviewUIText},
		{name: "operator contracts", text: contractsText},
	} {
		if !strings.Contains(doc.text, "docs/features/resource-inspection.md") {
			t.Fatalf("expected %s docs map to link resource inspection guide", doc.name)
		}
		if !strings.Contains(doc.text, "docs/features/resource-recent-logs.md") {
			t.Fatalf("expected %s docs map to link resource recent logs guide", doc.name)
		}
	}

	for _, fragment := range []string{
		"GET /v1/overview/deployments/{deploymentName}/resource-inspection",
		"`clusterArn`",
		"`limit`",
		"`instanceId`",
		"`lookupContext`",
		"`unavailableReason`",
		"`resultScope`",
		"Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating",
		"`Tenet request ID` metadata in the Live inspection card",
		"`cross_account_not_configured` means the target deployment needs observer-role wiring before Tenet can inspect the selected resource",
		"`resource_not_found` means the selected AWS resource, task, instance, or rollout target was not found in the resolved lookup path",
		"`aws_unavailable` means Tenet could not complete the provider lookup because credentials, permissions, region/account placement, or the AWS API were unavailable",
		"`unsupported_resource_type` means the route is stable but Tenet has not implemented live inspection for that resource family yet",
		"`no_log_source` means the resource is valid, but inspection can only use summary or sibling evidence rather than a dedicated live provider read",
		"`Deployment controller`",
		"`Deployment circuit breaker`",
		"`Deployment alarms`",
		"ECS service deployment rollout-state rows are exposed as",
		"`ecsService.deployments[]` from `DescribeServices`",
		"`status`, `rolloutState`, `rolloutStateReason`, `desiredCount`,",
		"`runningCount`, `pendingCount`, `taskDefinition`, `createdAt`, `updatedAt`,",
		"`primary`, and `active`.",
		"Smoke treats `ecsService.deployments[]` as optional, but validates row field types whenever AWS returns deployment rows.",
	} {
		if !strings.Contains(featureText, fragment) {
			t.Fatalf("expected resource inspection guide to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"optional `clusterArn` when an ECS service/task recent-log request supplied an accepted cluster selector",
		"Tenet validates and echoes that selector just like `ecsService`",
	} {
		if !strings.Contains(recentLogsFeatureText, fragment) {
			t.Fatalf("expected resource recent logs guide to include %q", fragment)
		}
	}
}

func TestResourceEvidenceDocsLockOperatorFieldFamilies(t *testing.T) {
	inspectionText := readTextFile(t, "features/resource-inspection.md")
	recentLogsText := readTextFile(t, "features/resource-recent-logs.md")
	overviewText := readTextFile(t, "features/overview-dashboard.md")

	for _, fragment := range []string{
		"recent service events",
		"bounded task sample",
		"operator-facing ECS service",
		"`Deployment controller`",
		"`Deployment circuit breaker`",
		"`Service deployments`",
		"`ecsService.deployments[]` from `DescribeServices`",
		"`primary`, and `active`.",
		"operator-facing ECS task/container labels include",
		"`Container {name} image digest`, `Container {name} runtime ID`,",
		"`Container {name} exit`, `Container {name} awslogs`, and",
		"operator-facing ASG/EC2 labels include `Launch template`,",
		"`Instances`, `Recent scaling activities`, `EC2 state`, `EC2 instance type`,",
		"`EC2 instance status check`, `EC2 system status check`,",
		"`Attached EBS status check`, and `EC2 scheduled event {n}`",
		"traffic-routing inspection",
		"`Traffic-routing status`",
		"`CodeDeploy deployment group status`",
		"`Target group live details`, `Registered target health`, `Target group health`,",
		"fixture and Compose",
	} {
		if !strings.Contains(inspectionText, fragment) {
			t.Fatalf("expected resource inspection guide to lock operator field-family fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"optional `clusterArn` when an ECS service/task recent-log request supplied an accepted cluster selector",
		"optional `containerName` when an ECS task recent-log request supplied a container selector",
		"optional `instanceId` when an Auto Scaling Group-backed EC2 console-log request supplied an instance selector",
		"Supported resource, AWS lookup unavailable: `available=false`, `unavailableReason=aws_unavailable`",
		"Route-stable resource with no independent stream: `available=false`, `unavailableReason=no_log_source`",
		"Missing provider wiring: `available=false`, `entries=[]`, `unavailableReason=\"overview deployment resource logs unavailable\"`",
		"empty: `No recent logs are available for this resource.`",
		"unavailable: `Recent logs are unavailable for this resource.`",
		"The additive `lookupContext` object is always present, including unavailable responses",
	} {
		if !strings.Contains(recentLogsText, fragment) {
			t.Fatalf("expected resource recent logs guide to lock route/field fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"Resource evidence docs intentionally lock the operator-facing labels that reviewers use to spot drift",
		"ECS service deployment configuration and recent events",
		"ECS task container status/identity/failure/log-binding/network evidence",
		"Auto Scaling launch-template, sampled-instance, EC2 host/status-check, and scaling-activity evidence",
		"traffic-routing summary-only fallback and live CodeDeploy/listener/target-group health evidence",
		"recent-log lookup context plus friendly empty/unavailable distinctions",
	} {
		if !strings.Contains(overviewText, fragment) {
			t.Fatalf("expected overview dashboard docs to summarize resource evidence docs-contract scope %q", fragment)
		}
	}
}

func TestResourceUnavailableSmokeMatrixDocsAndScripts(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	inspectionText := readTextFile(t, "features/resource-inspection.md")
	recentLogsText := readTextFile(t, "features/resource-recent-logs.md")
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	smokeLocalText := readTextFile(t, "../scripts/smoke-local.sh")
	smokeDockerText := readTextFile(t, "../scripts/smoke-docker.sh")
	uiBehaviorText := readTextFile(t, "../internal/api/ui_behavior_test.go")

	for _, fragment := range []string{
		"compact no-AWS resource-evidence matrix for ECS service/task, ASG/bootstrap, traffic-routing no-log-source, selector validation, successful-empty, and guarded-auth paths",
		"Local/no-AWS release-guard matrix",
	} {
		if !strings.Contains(readmeText+inspectionText+recentLogsText, fragment) {
			t.Fatalf("expected resource unavailable docs matrix to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"ECS service recent logs",
		"ECS task recent logs",
		"ASG console logs",
		"Traffic-routing no-log-source",
		"Successful-empty provider read",
		"`available=false`, `unavailableReason=no_log_source`, `entries=[]`, populated `lookupContext`, and `resultScope.truncationStatus=not_applicable`",
	} {
		if !strings.Contains(recentLogsText, fragment) {
			t.Fatalf("expected recent-log unavailable matrix to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"representative no-AWS matrix for ECS service/task, ASG and bootstrap-source, traffic-routing, selector-validation, and guarded-auth paths",
		"The matrix checks machine fields (`available`, `unavailableReason`, `lookupContext`, `resultScope`, `clusterArn`, `instanceId`) together with the embedded UI's friendly Live inspection copy",
	} {
		if !strings.Contains(inspectionText, fragment) {
			t.Fatalf("expected resource-inspection unavailable matrix to include %q", fragment)
		}
	}

	matrix := []struct {
		name      string
		local     []string
		docker    []string
		docs      []string
		uiHarness []string
	}{
		{
			name:   "ecs service logs selector/result scope",
			local:  []string{"worker_resource_logs", "selected_worker_resource_logs", "limited_worker_resource_logs", "assert_recent_log_result_window", "resultScope"},
			docker: []string{"worker_resource_logs", "selected_worker_resource_logs", "limited_worker_resource_logs", "assert_recent_log_result_window", "resultScope"},
			docs:   []string{"ECS service recent logs", "`ecsService` with optional `clusterArn`"},
		},
		{
			name:   "ecs task logs unavailable selectors",
			local:  []string{"ecs_task_resource_logs", "containerName", "cross_account_not_configured", "no_log_source"},
			docker: []string{"ecs_task_resource_logs", "containerName", "cross_account_not_configured", "no_log_source"},
			docs:   []string{"ECS task recent logs", "`ecsTask` with `containerName` and `clusterArn`"},
		},
		{
			name:   "asg/bootstrap selected instance",
			local:  []string{"bootstrap_resource_logs", "selected_bootstrap_resource_logs", "invalid_resource_log_instance", "instanceId"},
			docker: []string{"bootstrap_resource_logs", "selected_bootstrap_resource_logs", "invalid_resource_log_instance", "instanceId"},
			docs:   []string{"ASG console logs", "selected-instance echo", "malformed `instanceId` rejected with `400`"},
		},
		{
			name:      "traffic routing no log source",
			local:     []string{"traffic_resource_logs", "production_listener_rule_resource_logs", "active_target_group_resource_logs", "candidate_target_group_resource_logs", "no_log_source"},
			docker:    []string{"traffic_resource_logs", "production_listener_rule_resource_logs", "active_target_group_resource_logs", "candidate_target_group_resource_logs", "no_log_source"},
			docs:      []string{"Traffic-routing no-log-source", "resultScope.truncationStatus=not_applicable"},
			uiHarness: []string{"Recent logs are unavailable for this resource.", "This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet."},
		},
		{
			name:      "live inspection unavailable reasons",
			local:     []string{"worker_resource_inspection", "selected_worker_resource_inspection", "bootstrap_resource_inspection", "selected_bootstrap_resource_inspection", "aws_unavailable"},
			docker:    []string{"worker_resource_inspection", "selected_worker_resource_inspection", "bootstrap_resource_inspection", "selected_bootstrap_resource_inspection", "aws_unavailable"},
			docs:      []string{"Live inspection copy", "available`, `unavailableReason`, `lookupContext`, `resultScope`, `clusterArn`, `instanceId"},
			uiHarness: []string{"AWS access is currently unavailable for live inspection.", "No running tasks were found for this ECS service."},
		},
	}

	for _, entry := range matrix {
		for _, fragment := range entry.local {
			if !strings.Contains(smokeLocalText, fragment) {
				t.Fatalf("expected smoke-local to cover %s fragment %q", entry.name, fragment)
			}
		}
		for _, fragment := range entry.docker {
			if !strings.Contains(smokeDockerText, fragment) {
				t.Fatalf("expected smoke-docker to cover %s fragment %q", entry.name, fragment)
			}
		}
		for _, fragment := range entry.docs {
			if !strings.Contains(recentLogsText+inspectionText+overviewText, fragment) {
				t.Fatalf("expected docs to cover %s fragment %q", entry.name, fragment)
			}
		}
		for _, fragment := range entry.uiHarness {
			if !strings.Contains(uiBehaviorText, fragment) {
				t.Fatalf("expected UI behavior harness to cover %s fragment %q", entry.name, fragment)
			}
		}
	}
}

func TestAuthToggleDocsUseReadableBooleanExamples(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	for _, fragment := range []string{
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`",
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, `GET /v1/overview/accounts`",
		"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is also enabled, those same guarded account reads",
		"When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, the same narrow read gate also applies",
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, `GET /v1/overview/applications`",
		"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is also enabled, the same guarded read routes additionally require",
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, stored `GET /v1/plans/{planId}`",
		"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is enabled, `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, `POST /v1/applies/{applyId}/claim`, and `POST /v1/applies/{applyId}/transitions`",
		"when that same `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` setting is enabled, `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, `POST /v1/applies/{applyId}/claim`, and `POST /v1/applies/{applyId}/transitions`",
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, stored `GET /v1/applies/{applyId}`",
		"`GET /v1/applies?status=queued` follows the same collection contract",
	} {
		if !strings.Contains(overviewText, fragment) {
			t.Fatalf("expected overview dashboard docs to include readable boolean auth-toggle example %q", fragment)
		}
	}

	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")
	for _, fragment := range []string{
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true` trusts front-door-forwarded",
		"`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true` requires authenticated identity",
		"`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` trusts `X-Tenet-Authenticated-Roles`",
		"`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` trusts `X-Tenet-Authorized-Deployments`",
		"`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true`",
	} {
		if !strings.Contains(localRuntimeText, fragment) {
			t.Fatalf("expected local runtime docs to include readable boolean auth-toggle example %q", fragment)
		}
	}

	overviewUIDoc := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is also enabled, those same write routes require",
	} {
		if !strings.Contains(overviewUIDoc, fragment) {
			t.Fatalf("expected overview UI docs to keep balanced inline-code boolean auth-toggle example %q", fragment)
		}
	}

	redactedPlaceholder := strings.Repeat("*", 3)
	for docName, docText := range map[string]string{
		"overview dashboard docs": overviewText,
		"local runtime docs":      localRuntimeText,
		"overview UI docs":        overviewUIDoc,
	} {
		for _, toggle := range []string{
			"TENET_API_TRUST_AUTHENTICATED_HEADERS",
			"TENET_API_ENFORCE_AUTHENTICATED_IDENTITY",
			"TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION",
			"TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION",
		} {
			for _, placeholder := range []string{
				"`" + toggle + "=" + redactedPlaceholder + "`",
				toggle + "=" + redactedPlaceholder,
			} {
				if strings.Contains(docText, placeholder) {
					t.Fatalf("expected %s to reject non-secret auth-toggle placeholder %q", docName, placeholder)
				}
			}
		}
	}

	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	if !strings.Contains(operatorContractsText, "TENET_CATALOG_GITHUB_TOKEN=***") {
		t.Fatal("expected credential-bearing GitHub token example to remain redacted")
	}
}

func TestLocalRuntimeDocsSeparateShippedWriteRoutesFromDeferredAWSMutation(t *testing.T) {
	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")

	for _, fragment := range []string{
		"shipped immutable plan/apply write routes",
		"`POST /v1/plans/deployments/{deploymentName}`",
		"`POST /v1/applies/plans/{planId}`",
		"`POST /v1/applies/{applyId}/claim`",
		"`POST /v1/applies/{applyId}/transitions`",
		"`TENET_AWS_ENABLE_APPLY=true` queued local-safe handoff",
		"without AWS mutation",
		"live AWS observation/execution adapters",
		"real mutation executor loops",
	} {
		if !strings.Contains(localRuntimeText, fragment) {
			t.Fatalf("expected local runtime docs to separate shipped write routes from deferred AWS mutation via %q", fragment)
		}
	}

	if strings.Contains(localRuntimeText, "- write APIs") {
		t.Fatal("expected deferred work docs not to describe generic write APIs as deferred")
	}
}

func TestClaimedApplyTransitionGuardDocsAndSmoke(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	smokeLocalText := readTextFile(t, "../scripts/smoke-local.sh")

	for _, fragment := range []string{
		"claimed apply can still advance through `POST /v1/applies/{applyId}/transitions`",
		"the transition response keeps `claim.workerId`, `claim.claimedAt`, `claim.lastHeartbeatAt`, `claim.expiresAt`, and additive lifecycle metadata",
		"local-safe only: claim/transition records update Tenet's persisted apply lifecycle without live AWS mutation",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to document claimed apply transition guard %q", fragment)
		}
	}

	for _, fragment := range []string{
		"claim_transition_apply_id",
		"claimed_apply_transition_status",
		"/v1/applies/$claim_transition_apply_id/claim",
		"/v1/applies/$claim_transition_apply_id/transitions",
		"MANUAL_CLAIMED_APPLY_TRANSITION_STATUS",
		"manual claimed local-safe transition",
	} {
		if !strings.Contains(smokeLocalText, fragment) {
			t.Fatalf("expected smoke-local to cover claimed apply transition guard %q", fragment)
		}
	}
}

func TestApplicationDetailResourceFilterDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"`Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search",
		"visible/total impact and section-specific empty guidance alongside the inline",
		"`Clear resource filter`, plus compact resource-row `Evidence cues`:",
		"`Live evidence available` / `Recent logs supported` `_self` handoffs to the final",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include application-detail resource-filter summary %q", fragment)
		}
	}

	for _, fragment := range []string{
		"log and inspection caps stay bookmarkable as `resourceLogLimit=` and `resourceInspectionLimit=` UI state",
		"positive caps are sent only to the matching lazy `resource-logs` or `resource-inspection` read as backend `limit=` query parameters",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include resource-page cap state summary %q", fragment)
		}
	}

	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"The application-detail linked-resource filter is a client-side, bookmarkable `resourceFilter` query parameter",
		"It matches normalized linked-resource `resource` type, `name`, and `description` text",
		"Filtering never changes the backing read route: application detail still reads `GET /v1/overview/applications/{deploymentName}`",
		"deeper resource/drift/plan links still preserve their existing route contracts",
		"When `resourceFilter` hides rows in one or more reported resource groups, the UI keeps those sections visible with section-specific filtered-empty copy",
		"Active filters also switch section quick-nav labels to visible/total counts such as `Traffic routing (1 of 3)`.",
		"When the application read reports no linked resources at all, the page keeps the separate no-resource state `No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.`",
		"Resource-filter updates and clear actions preserve the surrounding bookmarkable shell state (`q`, `sync`, `account`, `deployment`, and the current hash) while only changing or removing `resourceFilter`.",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI docs to include application-detail resource-filter contract %q", fragment)
		}
	}

	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`resourceFilter` is UI query state only; it is not sent to `GET /v1/overview/applications/{deploymentName}` and does not add a read-route variant.",
		"The filter matches linked-resource type, name, and description text after the application-detail payload is loaded.",
		"The filtered-empty state is distinct from the no-resource state so operators can tell whether client-side filtering hid resources or the read model reported none.",
		"Application-detail links and filter controls must preserve `q`, `sync`, `account`, `deployment`, and hash state while changing only `resourceFilter`.",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts doc to include application-detail resource-filter contract %q", fragment)
		}
	}
}

func TestOverviewDashboardRuntimeEventCueDocumentsDeterministicTieBreak(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	for _, fragment := range []string{
		"`runtimeEventCue` when the visible deployment slice has retained controller history",
		"newest retained event's deployment, timestamp, source, summary, sync/health, rollout phase/current step",
		"lexicographically smaller deployment name, then lexicographically smaller source",
		"aggregate counts across all visible retained-history deployments",
	} {
		if !strings.Contains(overviewText, fragment) {
			t.Fatalf("expected overview dashboard docs to include runtimeEventCue deterministic contract %q", fragment)
		}
	}
}

func TestOverviewDashboardDocumentsRuntimeRetentionAndLatestRuntimeCueGuard(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")

	applicationsSection := markdownSection(t, overviewText, "### Applications overview")
	for _, fragment := range []string{
		"`Last update`, `Visible subset last update`, and the optional row-local `Runtime` line are latest controller-runtime cues, not UI read-freshness cues",
		"`Applications read refreshed` is the only applications-overview cue for when the browser last fetched `GET /v1/overview/applications`",
	} {
		if !strings.Contains(applicationsSection, fragment) {
			t.Fatalf("expected applications overview docs to include latest-runtime cue guard %q", fragment)
		}
	}

	runtimeEventsSection := markdownSection(t, overviewText, "### Deployment runtime events API")
	for _, fragment := range []string{
		"Retention metadata is independent of the selected query window: `retention` describes the controller-retained history, while `resultScope` describes only the rows returned by the current `source` / `limit` read",
		"`retentionStatus: \"at_retention_limit\"` is not the same signal as `truncationStatus: \"truncated\"`",
	} {
		if !strings.Contains(runtimeEventsSection, fragment) {
			t.Fatalf("expected runtime events docs to include retention release guard %q", fragment)
		}
	}
}

func TestOverviewDashboardDeploymentScopedAuditWorkspaceDocumentsClearableAuditChips(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	section := markdownSection(t, overviewText, "### Deployment-scoped audit workspace")
	for _, fragment := range []string{
		"clearable toolbar chips",
		"deployment/action/limit",
		"removes only its selected control",
		"preserves `q`, `sync`, `account`",
		"remaining audit query state",
		"removes stale `#audit-*` section anchors",
		"reloads the audit timeline",
		"newest/oldest visible entry navigation cues",
		"Jump to newest entry",
		"Jump to oldest visible entry",
	} {
		if !strings.Contains(section, fragment) {
			t.Fatalf("expected deployment-scoped audit workspace docs to include clearable chip contract %q", fragment)
		}
	}
}

func TestAccountDetailReadRouteCopyDocsContract(t *testing.T) {
	overviewDashboardText := readTextFile(t, "features/overview-dashboard.md")
	quickLinksSection := markdownSection(t, overviewDashboardText, "## Quick links")
	for _, fragment := range []string{
		"account detail page: `Account JSON` / `Copy account read route`, `Group JSON` / `Copy group read route`, `_self` `Open application`, and a guarded `_self` `Open runtime events` row link",
		"`Applications overview` plus `Account JSON` / `Copy account read route` recovery handoffs",
	} {
		if !strings.Contains(quickLinksSection, fragment) {
			t.Fatalf("expected overview dashboard quick links docs to include account copy-route contract %q", fragment)
		}
	}

	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"deployment rows with meaningful runtime/event evidence add a `_self` `Open runtime events` link",
		"while preserving `q` / `sync` / `account`",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README docs to include account runtime-events handoff contract %q", fragment)
		}
	}

	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"rows with meaningful runtime/event evidence add a `_self` `Open runtime events` handoff",
		"preserving `q` / `sync` / `account`",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI docs to include account runtime-events handoff contract %q", fragment)
		}
	}

	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"`Applications overview` plus `Account JSON` / `Copy account read route` recovery handoffs",
		"operators can open or copy the exact scoped read route",
		"Deployment rows with meaningful runtime/event evidence add a guarded `_self` `Open runtime events` link",
		"omitting timestamp-only rows",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts docs to include account detail contract %q", fragment)
		}
	}
}

func TestResourceDetailTroubleshootingSummaryDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"The bookmarkable resource page (`/?page=resource...`)",
		"compact `Troubleshooting summary`",
		"before lazy Recent logs or Live inspection panels load",
		"selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"latest lazy Recent logs and Live inspection panel states",
		"empty-log and unavailable-provider next-step guidance",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"The resource-detail Troubleshooting summary is a derived UI contract, not a separate API read.",
		"latest completed Live inspection state and the latest completed Recent logs panel state",
		"selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"Current resource read: GET /v1/overview/deployments/{deploymentName}",
		"visible before lazy Recent logs or Live inspection reads resolve",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	overviewDashboardText := readTextFile(t, "features/overview-dashboard.md")
	for _, fragment := range []string{
		"compact `Selected resource context` callout",
		"visible before the lazy Recent logs and Live inspection panels load",
		"the final resource page also renders a compact `Troubleshooting summary`",
		"any selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"available, loading, empty, unavailable, and fetch-error evidence states",
	} {
		if !strings.Contains(overviewDashboardText, fragment) {
			t.Fatalf("expected overview dashboard docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	for _, fragment := range []string{
		"top-level `Selected resource context` callout",
		"the supported live-inspection/recent-log backing reads operators can refresh below",
		"GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}",
		"GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}
}

func markdownSection(t *testing.T, text, heading string) string {
	t.Helper()

	start := strings.Index(text, heading)
	if start < 0 {
		t.Fatalf("expected markdown text to include heading %q", heading)
	}
	section := text[start+len(heading):]
	if next := regexp.MustCompile(`(?m)^### `).FindStringIndex(section); next != nil {
		section = section[:next[0]]
	}
	return section
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
