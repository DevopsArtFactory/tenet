package docs

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestMarkdownRelativeLinksResolve(t *testing.T) {
	linkRE := regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
	files := markdownDocs(t)
	anchorsByFile := markdownAnchorsByFile(t, files)

	for _, file := range files {
		body := readTextFile(t, file)
		for _, match := range linkRE.FindAllStringSubmatch(body, -1) {
			link := strings.TrimSpace(match[1])
			if link == "" || strings.HasPrefix(link, "/") || strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
				continue
			}
			linkTarget, fragment, _ := strings.Cut(link, "#")
			linkTarget = strings.SplitN(linkTarget, "?", 2)[0]
			if linkTarget == "" && fragment == "" {
				continue
			}

			target := file
			if linkTarget != "" {
				target = filepath.Clean(filepath.Join(filepath.Dir(file), linkTarget))
				if _, err := os.Stat(target); err != nil {
					t.Fatalf("expected local markdown link %q in %s to resolve to %s: %v", match[1], file, target, err)
				}
			}

			if fragment == "" {
				continue
			}
			decodedFragment, err := url.PathUnescape(fragment)
			if err != nil {
				t.Fatalf("expected markdown anchor %q in %s to be URL-decodable: %v", fragment, file, err)
			}
			anchors, ok := anchorsByFile[target]
			if !ok {
				continue
			}
			if !anchors[strings.ToLower(decodedFragment)] {
				t.Fatalf("expected local markdown anchor %q in %s to resolve inside %s", match[1], file, target)
			}
		}
	}
}

func markdownDocs(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range []string{".", "../README.md"} {
		info, err := os.Stat(root)
		if err != nil {
			t.Fatalf("stat %s: %v", root, err)
		}
		if !info.IsDir() {
			files = append(files, root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk markdown docs under %s: %v", root, err)
		}
	}
	return files
}

func markdownAnchorsByFile(t *testing.T, files []string) map[string]map[string]bool {
	t.Helper()
	headingRE := regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	explicitIDRE := regexp.MustCompile(`(?i)<[^>]+\bid=["']([^"']+)["']`)
	anchorsByFile := make(map[string]map[string]bool, len(files))
	for _, file := range files {
		anchors := map[string]bool{"top": true}
		seen := map[string]int{}
		for _, line := range strings.Split(readTextFile(t, file), "\n") {
			if match := headingRE.FindStringSubmatch(line); match != nil {
				anchor := markdownHeadingAnchor(match[1])
				if anchor != "" {
					count := seen[anchor]
					seen[anchor] = count + 1
					if count > 0 {
						anchor = anchor + "-" + strconv.Itoa(count)
					}
					anchors[anchor] = true
				}
			}
			for _, match := range explicitIDRE.FindAllStringSubmatch(line, -1) {
				anchors[strings.ToLower(match[1])] = true
			}
		}
		anchorsByFile[file] = anchors
	}
	return anchorsByFile
}

func markdownHeadingAnchor(heading string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(stripInlineHTML(heading)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func stripInlineHTML(value string) string {
	tagRE := regexp.MustCompile(`<[^>]*>`)
	return tagRE.ReplaceAllString(value, "")
}

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
		"local Markdown file links and same-file/cross-file heading anchors",
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

func TestAuditLogDocsDocumentControllerRefreshStatusShortcut(t *testing.T) {
	auditLogText := readTextFile(t, "architecture/tenet-v1-audit-log-api.md")

	for _, fragment := range []string{
		"GET /v1/controller/refresh/status",
		"GET /v1/audit-logs?action=controller.refresh.requested&limit=1",
		"newest-first `requestedAt`/timestamp ordering",
		"preserves the bounded `resultScope`",
	} {
		if !strings.Contains(auditLogText, fragment) {
			t.Fatalf("expected audit log docs to preserve controller-refresh status shortcut fragment %q", fragment)
		}
	}
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
		"go test ./docs -run TestOperatorTroubleshootingRunbookDocumentsDailyTriageFlow -count=1",
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
		"Keep `scripts/ci-smoke-runner-evidence.sh` directly executable",
		"Runner-start verdict",
		"runner_id=0",
		"steps=[]",
		"jobs: []",
		"startup_failure",
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
		"Actions jobs API returned jobs: []",
		"startup_failure/no runner records",
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
		"events?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate",
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
		"The selector keeps backend values in the route while showing operator-readable labels such as `Controller history`, `Deployment controller history`, and `GitHub refresh`",
		"exact bookmarked custom source values remain visible instead of being discarded",
		"`retention.limit` may still be non-zero",
		"`sourceSummary` array of `{source, count}` entries computed from the retained window before optional source filtering and before positive limit truncation (blank persisted sources are bucketed as `unknown`, and `?source=unknown` selects those legacy entries)",
		"`returnedCount` as a compact alias for `returnedEventCount`",
		"`GET /v1/overview/deployments/payments-api-prod/events?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate`",
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
		"The selector keeps backend values in the route while showing operator-readable labels such as `Controller history`, `Deployment controller history`, and `GitHub refresh`",
		"exact bookmarked custom source values remain visible instead of being discarded",
	} {
		if !strings.Contains(overviewText, fragment) {
			t.Fatalf("expected overview dashboard deployment-events docs to include %q", fragment)
		}
	}

	for _, fragment := range []string{
		"Invalid source filters return `400 Bad Request` with `invalid source query`.",
		"additive `sourceSummary` entries with `source` and `count` computed from the retained runtime window before optional source filtering and before positive limit truncation, including the `unknown` bucket for blank persisted sources",
		"calls the fixture-empty deployment-events route with `?limit=1`, `?source=controller&limit=1`, and `?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate`",
		"direct-process smoke owns retention metadata, exact `?source=controller&limit=1`, resource-filter `resultScope` metadata, and invalid-source `400` coverage",
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

func TestOperatorTroubleshootingCallsOutRelatedRuntimeEvents(t *testing.T) {
	runbookText := readTextFile(t, "runbooks/operator-troubleshooting.md")
	contractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	readmeText := readTextFile(t, "../README.md")
	overviewGuideText := readTextFile(t, "features/overview-dashboard.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"| Related runtime events | `#resource-related-runtime-events` | `GET /v1/overview/deployments/{deploymentName}/events?resourceType={type}&resourceName={name}&limit={n}[&source={source}]` |",
		"the selected `Related event source`, and the selected event window",
		"additive generic `events[].resourceRefs[]` rollout/execution references",
		"managedDrift[].resourceRef",
		"managedDrift[].source.resourceType/resourceName",
		"controller history filtered by retained resource evidence; not Recent logs, Live inspection, or immutable audit evidence",
		"no matching rollout, drift, or resource evidence",
		"If only a selected source is empty, use `Show all related event sources` before treating controller history as absent.",
		"curl -i \"$TENET_API_BASE/v1/overview/deployments/{deploymentName}/events?resourceType={type}&resourceName={name}&limit=10&source=controller\"",
	} {
		if !strings.Contains(runbookText, fragment) {
			t.Fatalf("expected operator troubleshooting runbook to include resource-events fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"GET /v1/overview/deployments/{deploymentName}/events?resourceType={type}&resourceName={name}&limit={n}[&source={source}]",
		"renders explicit loading/empty/source-filtered no-match/unavailable/denied/error states",
		"compact `Related event source` and `Related event window` controls plus `Copy related events route` and `Refresh related events` actions",
	} {
		if !strings.Contains(contractsText, fragment) {
			t.Fatalf("expected operator contracts to include resource-events fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"application detail lists the linked resource, resource detail narrows retained controller history to that resource",
		"Source-filtered empty states also include a one-click `Show all related event sources` recovery action",
		"empty related-event results mean no retained matching rollout/drift/resource evidence in the selected source/window rather than missing Recent logs, Live inspection, or immutable audit data",
	} {
		if !strings.Contains(overviewGuideText, fragment) {
			t.Fatalf("expected overview dashboard guide to include resource-events journey fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"The resource page reuses the same backend contract for the `Related runtime events` panel after the application detail has already listed the selected resource",
		"empty states mean no retained matching controller-history evidence for that resource/source/window rather than missing Recent logs, Live inspection, or audit rows",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI architecture doc to include resource-events journey fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"Resource drill-down Related runtime events, Recent logs, and Live inspection panels summarize controller history, lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract.",
		"distinguishes empty retained history from source-filtered no-match states and route failures",
		"provides a one-click `Show all related event sources` recovery action for source-filtered empty states",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include resource-events journey fragment %q", fragment)
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
		"Resource drill-down Related runtime events, Recent logs, and Live inspection panels summarize controller history, lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract.",
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
		"`source` (stable API token; the embedded UI translates known values such as `aws_ecs`, `aws_cloudwatch_logs`, and `rollout_execution_summary` into operator-friendly labels)",
		"Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating",
		"`Tenet request ID` metadata in the Live inspection card",
		"Unavailable reason operator actions",
		"Use the unavailable reason as a routing cue rather than treating every",
		"`aws_unavailable` | Capture `Request-Id` plus `lookupContext`, then verify credentials, region/account placement, IAM permissions, and AWS API reachability.",
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
		"`source` (stable API token; embedded UI evidence panels translate known values such as `aws_cloudwatch_logs`, `cloudwatch`, `ecs_service`, and `ec2_console` into operator-friendly labels)",
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
		"the resource-inspection guide maps each unavailable reason to a",
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
		"`available=false`, `unavailableReason=no_log_source`, `entries=[]`, populated `lookupContext`, `resultScope.truncationStatus=not_applicable`, and `relatedLogRoutes[]` handoffs to sibling recent-log sources when Tenet can derive them",
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
			local:     []string{"traffic_resource_logs", "production_listener_rule_resource_logs", "active_target_group_resource_logs", "candidate_target_group_resource_logs", "no_log_source", "assert_related_log_routes", "relatedLogRoutes"},
			docker:    []string{"traffic_resource_logs", "production_listener_rule_resource_logs", "active_target_group_resource_logs", "candidate_target_group_resource_logs", "no_log_source", "assert_related_log_routes", "relatedLogRoutes"},
			docs:      []string{"Traffic-routing no-log-source", "resultScope.truncationStatus=not_applicable", "`relatedLogRoutes[]` handoffs"},
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
		"when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is enabled, `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, `POST /v1/applies/{applyId}/claim`, `POST /v1/applies/{applyId}/heartbeat`, and `POST /v1/applies/{applyId}/transitions`",
		"when that same `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` setting is enabled, `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, `POST /v1/applies/{applyId}/claim`, `POST /v1/applies/{applyId}/heartbeat`, and `POST /v1/applies/{applyId}/transitions`",
		"when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is enabled, stored `GET /v1/applies/{applyId}`",
		"`GET /v1/applies?status=active` follows the same collection contract",
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

func TestPlanWorkspaceDocsMentionCopyableWorkspaceReadRoute(t *testing.T) {
	docs := map[string]string{
		"README":             readTextFile(t, "../README.md"),
		"overview dashboard": readTextFile(t, "features/overview-dashboard.md"),
		"overview UI":        readTextFile(t, "architecture/tenet-v1-overview-ui.md"),
		"operator runbook":   readTextFile(t, "runbooks/operator-troubleshooting.md"),
	}

	for name, text := range docs {
		for _, fragment := range []string{
			"Current workspace read: GET /v1/overview/deployments/{deploymentName}.",
			"Workspace JSON",
			"Copy workspace read route",
			"/v1/overview/deployments/{deploymentName}",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to include copyable workspace read route fragment %q", name, fragment)
			}
		}
	}

	planWorkspaceFragment := "the embedded `/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable"
	if !strings.Contains(docs["overview UI"], planWorkspaceFragment) {
		t.Fatalf("expected overview UI docs to keep the plan/apply workspace route contract")
	}
	if !strings.Contains(docs["overview UI"], "exposes top-level `Workspace JSON` and `Copy workspace read route` handoffs") {
		t.Fatalf("expected overview UI plan/apply docs to mention both workspace read handoffs")
	}
	for _, fragment := range []string{
		"| Drift workspace | `/?page=drift&deployment={deploymentName}` | `GET /v1/overview/deployments/{deploymentName}` |",
		"drift-source mode, managed-drift row counts, severity/query filters, and runtime context",
		"filtered-empty rows mean the current query/severity controls are hiding evidence",
		"`Workspace JSON` / `Copy workspace read route` handoffs to the current deployment-detail read",
	} {
		if !strings.Contains(docs["operator runbook"], fragment) {
			t.Fatalf("expected operator runbook to include workspace troubleshooting fragment %q", fragment)
		}
	}
}

func TestClaimedApplyTransitionGuardDocsAndSmoke(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	overviewDashboardText := readTextFile(t, "features/overview-dashboard.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	smokeLocalText := readTextFile(t, "../scripts/smoke-local.sh")

	for _, fragment := range []string{
		"claimed apply can still advance through `POST /v1/applies/{applyId}/transitions`",
		"`POST /v1/applies/{applyId}/heartbeat`",
		"the heartbeat response updates `claim.lastHeartbeatAt` / `claim.expiresAt` while preserving the stable apply ID, lifecycle state, operation state, details, and transition history",
		"the transition response keeps `claim.workerId`, `claim.claimedAt`, `claim.lastHeartbeatAt`, `claim.expiresAt`, and additive lifecycle metadata",
		"local-safe only: claim/heartbeat/transition records update Tenet's persisted apply lifecycle without live AWS mutation",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to document claimed apply transition guard %q", fragment)
		}
	}

	for name, text := range map[string]string{
		"README":             readmeText,
		"overview dashboard": overviewDashboardText,
		"overview UI":        overviewUIText,
	} {
		for _, fragment := range []string{
			"Apply transition timeline",
			"transitions[]",
			"actor availability",
			"error/details",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to describe apply transition timeline evidence via %q", name, fragment)
			}
		}
	}

	for _, fragment := range []string{
		"claim_transition_apply_id",
		"claimed_apply_transition_status",
		"/v1/applies/$claim_transition_apply_id/claim",
		"/v1/applies/$claim_transition_apply_id/heartbeat",
		"/v1/applies/$claim_transition_apply_id/transitions",
		"claimed_apply_heartbeat",
		"MANUAL_CLAIMED_APPLY_TRANSITION_STATUS",
		"manual claimed local-safe transition",
	} {
		if !strings.Contains(smokeLocalText, fragment) {
			t.Fatalf("expected smoke-local to cover claimed apply transition guard %q", fragment)
		}
	}
}

func TestExpiredApplyClaimRecoveryDocsAndSmoke(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	localRuntimeText := readTextFile(t, "architecture/tenet-v1-local-runtime.md")
	smokeLocalText := readTextFile(t, "../scripts/smoke-local.sh")

	for _, fragment := range []string{
		"\"allowExpiredClaim\":true",
		"only when the persisted `claim.expiresAt` is expired",
		"fresh competing claims still return `409 Conflict`",
		"legacy stale claims without persisted `claim.expiresAt` remain manual-recovery conflicts",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to document explicit expired claim recovery %q", fragment)
		}
		if !strings.Contains(localRuntimeText, fragment) {
			t.Fatalf("expected local runtime docs to document explicit expired claim recovery %q", fragment)
		}
	}

	for _, fragment := range []string{
		"stale_claim_recovery_apply_id",
		"allowExpiredClaim\":true",
		"stale_claim_recovery_record",
		"STALE_CLAIM_RECOVERY_APPLY_ID",
		"expired local-safe claim recovery",
	} {
		if !strings.Contains(smokeLocalText, fragment) {
			t.Fatalf("expected smoke-local to cover expired claim recovery %q", fragment)
		}
	}
}

func TestLocalSafeApplyExecutorReleaseGuardDocs(t *testing.T) {
	docs := map[string]string{
		"README":             readTextFile(t, "../README.md"),
		"local runtime":      readTextFile(t, "architecture/tenet-v1-local-runtime.md"),
		"overview dashboard": readTextFile(t, "features/overview-dashboard.md"),
		"operator contract":  readTextFile(t, "architecture/tenet-v1-operator-contracts.md"),
		"operator runbook":   readTextFile(t, "runbooks/operator-troubleshooting.md"),
	}

	for name, text := range docs {
		for _, fragment := range []string{
			"Local-safe apply executor release guard",
			"active collection (`GET /v1/applies?status=active`)",
			"stale active-claim collection",
			"(`GET /v1/applies?status=active&claim=stale`)",
			"queued exact subset",
			"(`GET /v1/applies?status=queued`)",
			"running exact subset",
			"(`GET /v1/applies?status=running`)",
			"terminal exact subsets (`GET /v1/applies?status=failed`, `GET /v1/applies?status=succeeded`, and `GET /v1/applies?status=cancelled`)",
			"stored apply read",
			"(`GET /v1/applies/{applyId}`)",
			"non-mutating preflight read",
			"(`GET /v1/applies/{applyId}/preflight`)",
			"read-only execution preview",
			"(`GET /v1/applies/{applyId}/execution-preview`)",
			"claim (`POST /v1/applies/{applyId}/claim`)",
			"heartbeat (`POST /v1/applies/{applyId}/heartbeat`)",
			"transition",
			"(`POST /v1/applies/{applyId}/transitions`)",
			"local-safe record lifecycle does not",
			"perform live AWS mutation",
			"read-only `/?page=applies` monitor defaults to",
			"queued subset read",
			"explicit queued/active/stale view controls",
			"`?appliesView=active`",
			"`?appliesView=stale`",
			"Refresh applies",
			"fresh/stale/pending",
			"plan/apply workspace for detail review",
			"JSON handoff, `Copy read route`, refresh, and Applications overview recovery actions visible on populated, empty, loading, unavailable, and generic fetch-error monitor states",
			"Empty monitor states are actionable: queued empty points operators back to plan/apply workspaces, active empty explains that work may be terminal or still unclaimed, and stale empty is called out as the healthy recovery-queue state",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to include local-safe apply release guard fragment %q", name, fragment)
			}
		}
	}

	for name, text := range map[string]string{
		"README":        docs["README"],
		"local runtime": docs["local runtime"],
	} {
		for _, fragment := range []string{
			"Local and Docker smoke cover the active-vs-queued/running apply collection split, stale-claim refinement, and terminal exact subsets",
			"`GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` return only matching terminal records",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to include Docker terminal apply filter release-guard fragment %q", name, fragment)
			}
		}
	}

	for name, text := range map[string]string{
		"smoke-local":  readTextFile(t, "../scripts/smoke-local.sh"),
		"smoke-docker": readTextFile(t, "../scripts/smoke-docker.sh"),
	} {
		for _, fragment := range []string{
			"PAYLOAD_DIR=$(mktemp -d)",
			"write_payload()",
			"Path(sys.argv[1]).read_text",
			"Verified active/queued/running apply collection split and claim freshness",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to keep apply release-guard payloads file-backed via %q", name, fragment)
			}
		}
	}

	for name, fragments := range map[string][]string{
		"smoke-local": {
			"active_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=active\")",
			"running_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=running\")",
			"stale_active_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=active&claim=stale\")",
			"failed_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=failed\")",
			"succeeded_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=succeeded\")",
			"cancelled_apply_records=$(curl -fsS \"http://$HOST:$PORT/v1/applies?status=cancelled\")",
			"STALE_ACTIVE_APPLY_RECORDS_PATH",
			"apply_execution_preview=$(curl -fsS \"http://$HOST:$PORT/v1/applies/$apply_id/execution-preview\")",
			"APPLY_EXECUTION_PREVIEW_PATH",
			"assert apply_execution_preview['readOnly'] is True",
			"SUCCEEDED_APPLY_RECORDS_PATH",
			"CANCELLED_APPLY_RECORDS_PATH",
			"assert active_apply_records[0]['claim']['expiresAt'] == claimed_apply_heartbeat['claim']['expiresAt']",
			"assert running_apply_records[0]['id'] == claimed_apply_heartbeat['id']",
			"assert stale_active_apply_records[0]['id'] == os.environ['STALE_CLAIM_RECOVERY_APPLY_ID']",
			"No active apply handoffs are visible right now. GET /v1/applies?status=active returned an empty collection.",
			"No stale active apply claims are visible right now. GET /v1/applies?status=active&claim=stale returned an empty collection.",
			"createAppliesMonitorRefreshButton = (state) => createRefreshButton('Refresh applies'",
		},
		"smoke-docker": {
			"file_backed_active_apply_records_before_claim=$(curl -fsS \"$API_BASE_URL/v1/applies?status=active\")",
			"file_backed_running_apply_records_after_heartbeat=$(curl -fsS \"$API_BASE_URL/v1/applies?status=running\")",
			"file_backed_queued_apply_records_after_heartbeat=$(curl -fsS \"$API_BASE_URL/v1/applies?status=queued\")",
			"file_backed_succeeded_apply_records=$(curl -fsS \"$API_BASE_URL/v1/applies?status=succeeded\")",
			"file_backed_cancelled_apply_records=$(curl -fsS \"$API_BASE_URL/v1/applies?status=cancelled\")",
			"apply_preflight=$(curl -fsS \"$API_BASE_URL/v1/applies/$apply_id/preflight\")",
			"APPLY_PREFLIGHT_PATH",
			"apply_execution_preview=$(curl -fsS \"$API_BASE_URL/v1/applies/$apply_id/execution-preview\")",
			"APPLY_EXECUTION_PREVIEW_PATH",
			"assert apply_preflight[\"state\"] == apply_record[\"executionReadiness\"][\"state\"]",
			"assert \"did not contact AWS\" in \" \".join(apply_preflight[\"evidence\"])",
			"assert apply_execution_preview[\"state\"] == apply_preflight[\"state\"]",
			"assert apply_execution_preview[\"readOnly\"] is True",
			"assert (apply_execution_preview.get(\"blocker\") or {}).get(\"reason\") == apply_preflight[\"operations\"][0].get(\"unavailableReason\")",
			"FILE_BACKED_SUCCEEDED_APPLY_RECORDS_PATH",
			"FILE_BACKED_CANCELLED_APPLY_RECORDS_PATH",
			"it can claim or advance seeded queued records between read requests",
			"assert file_backed_claimed_apply_heartbeat[\"claim\"][\"expiresAt\"]",
			"assert file_backed_claim_apply_id not in queued_after_heartbeat_ids",
			"assert all(record.get(\"status\") == \"succeeded\" for record in file_backed_succeeded_records)",
			"assert all(record.get(\"status\") == \"cancelled\" for record in file_backed_cancelled_records)",
			"Verified active/queued and terminal apply collection filters plus claim freshness",
		},
	} {
		text := map[string]string{
			"smoke-local":  readTextFile(t, "../scripts/smoke-local.sh"),
			"smoke-docker": readTextFile(t, "../scripts/smoke-docker.sh"),
		}[name]
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s smoke to lock active/queued applies release guard evidence %q", name, fragment)
			}
		}
	}
}

func TestLocalSafeApplyEnableApplyStaysMetadataOnlyAndCredentialFree(t *testing.T) {
	docs := map[string]string{
		"README":             readTextFile(t, "../README.md"),
		"overview dashboard": readTextFile(t, "features/overview-dashboard.md"),
		"local runtime":      readTextFile(t, "architecture/tenet-v1-local-runtime.md"),
		"operator contract":  readTextFile(t, "architecture/tenet-v1-operator-contracts.md"),
		"operator runbook":   readTextFile(t, "runbooks/operator-troubleshooting.md"),
	}

	for name, text := range docs {
		for _, fragment := range []string{
			"`TENET_AWS_ENABLE_APPLY=true` queues local-safe apply handoff metadata only",
			"requires no live AWS credentials",
			"does not perform live AWS mutation",
			"disabled/default apply records are expected local-safe evidence, not a product failure",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to lock local-safe apply boundary fragment %q", name, fragment)
			}
		}
	}

	for name, text := range map[string]string{
		"README":             docs["README"],
		"local runtime":      docs["local runtime"],
		"overview dashboard": docs["overview dashboard"],
		"operator contract":  docs["operator contract"],
		"operator runbook":   docs["operator runbook"],
	} {
		for _, fragment := range []string{
			"Queued applies",
			"Lifecycle signals",
			"Operation signals",
			"Claim signals",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to name applies monitor summary metric %q", name, fragment)
			}
		}
	}

	for name, text := range map[string]string{
		"README":        docs["README"],
		"local runtime": docs["local runtime"],
	} {
		for _, fragment := range []string{
			"local-safe record lifecycle does not",
			"perform CodeDeploy, ECS, ALB, ASG, or other live AWS mutation",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s docs to lock concrete local-safe non-mutation boundary fragment %q", name, fragment)
			}
		}
	}

	operatorContract := docs["operator contract"]
	for _, fragment := range []string{
		"`GET /v1/applies/{applyId}/preflight` is a non-mutating readiness read",
		"read-only STS identity probe",
		"it never performs CodeDeploy, ECS, ALB, or ASG mutation",
		"executionReadiness and preflight evidence are readiness context, not proof that live AWS mutation ran or succeeded",
		"read-only IAM `SimulatePrincipalPolicy` evidence",
		"local or no-AWS environments still return `200 OK` with explicit `aws_unavailable`",
		"queued/running apply records are local-safe executor handoffs until a future live executor exists",
		"disabled/default apply records are expected local-safe evidence, not a product failure",
	} {
		if !strings.Contains(operatorContract, fragment) {
			t.Fatalf("expected operator contract docs to lock preflight/readiness boundary fragment %q", fragment)
		}
	}

	runbook := docs["operator runbook"]
	for _, fragment := range []string{
		"execution-readiness summary/evidence when `executionReadiness` is present",
		"Backend `executionReadiness` evidence is readiness/preflight context, not live AWS execution success",
		"explicit no-backend-preflight fallback copy",
		"per-operation preflight `handoffs`",
		"Apply preflight next action",
		"Preflight next-action and row handoffs are read-only next-step routes",
		"`deploymentDetailRoute`, `planApplyWorkspaceRoute`, `resourceInspectionRoute`, `resourceLogsRoute`",
		"Apply handoff safety check",
		"the immutable apply record state (`GET /v1/applies/{applyId}`)",
		"the non-mutating readiness/preflight evidence (`GET /v1/applies/{applyId}/preflight`)",
		"any local-safe claim freshness (`GET /v1/applies?status=active[&claim=stale]`)",
		"Treat the execution preview `blocker.reason` as the operator-safe stoplight",
		"`execution_disabled`, `live_aws_mutation_not_implemented`, `missing_deployer_role`",
		"and `aws_unavailable` are expected read-only blockers",
		"not evidence that a live",
		"executor attempted mutation",
		"This proves Tenet preserved and can route the",
		"handoff; it does not prove that a CodeDeploy, ECS, ALB, ASG, or EC2 mutation ran",
		"or succeeded",
	} {
		if !strings.Contains(runbook, fragment) {
			t.Fatalf("expected operator runbook to lock apply readiness evidence fragment %q", fragment)
		}
	}

	overviewDashboard := docs["overview dashboard"]
	for _, fragment := range []string{
		"execution-readiness label/summary/evidence when backend `executionReadiness` is present",
		"records without backend preflight data show explicit missing-preflight fallback copy",
		"do not mistake local-safe handoff metadata for live AWS readiness",
		"Apply preflight operation handoffs",
		"Apply preflight next action",
		"local-safe handoff is ready to be claimed without implying live AWS mutation succeeded",
		"deploymentDetailRoute`, `planApplyWorkspaceRoute`, `resourceInspectionRoute`, and `resourceLogsRoute",
		"never performs CodeDeploy/ECS/ALB/ASG mutation",
		"may add only read-only deployer-role STS identity evidence when a target-account role is configured",
	} {
		if !strings.Contains(overviewDashboard, fragment) {
			t.Fatalf("expected overview dashboard docs to lock applies monitor readiness fragment %q", fragment)
		}
	}

	for name, text := range map[string]string{
		"smoke-local":  readTextFile(t, "../scripts/smoke-local.sh"),
		"smoke-docker": readTextFile(t, "../scripts/smoke-docker.sh"),
	} {
		for _, fragment := range []string{
			"TENET_AWS_ENABLE_APPLY=true queues local-safe apply handoff metadata only",
			"requires no live AWS credentials",
			"does not perform live AWS mutation",
		} {
			if !strings.Contains(text, fragment) {
				t.Fatalf("expected %s to keep local-safe apply smoke contract fragment %q", name, fragment)
			}
		}
	}
}

func TestApplicationDetailResourceFilterDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"`Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search",
		"with visible/total impact, a compact `Linked resource inventory` summary with per-group counts, and section-specific empty guidance alongside the inline",
		"`Clear resource filter`. Their linked-resource summary also includes `Linked-resource source`, distinguishing controller-authored `rollout.execution[]` rows from canonical fallback rollout inventory when that array is missing or empty, plus compact resource-row `Evidence cues`:",
		"`Live evidence available` / `Recent logs supported` / `Related runtime events supported` `_self` handoffs to the final",
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
		"It matches normalized linked-resource `resource` type, `name`, `description`, and visible source-cue text",
		"Filtering never changes the backing read route: application detail still reads `GET /v1/overview/applications/{deploymentName}`",
		"deeper resource/drift/plan links still preserve their existing route contracts",
		"The resource list starts with a compact `Linked resource inventory` line that shows total/visible resources and per-group counts",
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
		"The filter matches linked-resource type, name, description, and source-cue text after the application-detail payload is loaded.",
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

func TestRuntimeStateDocsDescribeConfigurableControllerHistoryRetention(t *testing.T) {
	runtimeStateText := readTextFile(t, "architecture/tenet-v1-runtime-state-storage.md")
	statusAPIText := readTextFile(t, "architecture/tenet-v1-deployment-status-api.md")

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "runtime state storage docs", text: runtimeStateText},
		{name: "deployment status API docs", text: statusAPIText},
	} {
		for _, fragment := range []string{
			"`TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT`",
			"defaults to `10`",
			"invalid, zero, negative, or above-1000 values fail controller startup",
			"retention metadata reports the effective controller limit",
		} {
			if !strings.Contains(doc.text, fragment) {
				t.Fatalf("expected %s to document configurable controller-history retention %q", doc.name, fragment)
			}
		}
	}
}

func TestOverviewDashboardDocumentsRuntimeRetentionAndLatestRuntimeCueGuard(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	operatorContractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")

	for _, fragment := range []string{
		"Each row keeps compact `Project`, `Linked resources`, and `Last update` fields",
		"falling back to `Latest runtime status` when no timestamp is present",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include compact project/latest-runtime row cue %q", fragment)
		}
	}

	applicationsSection := markdownSection(t, overviewText, "### Applications overview")
	for _, fragment := range []string{
		"each row includes compact `Project`, `Linked resources`, and `Last update` fields next to account/environment and health, using `appRef` for ownership",
		"`Last update`, `Visible subset last update`, and the optional row-local `Runtime` line are latest controller-runtime cues, not UI read-freshness cues",
		"`Applications read refreshed` is the only applications-overview cue for when the browser last fetched `GET /v1/overview/applications`",
	} {
		if !strings.Contains(applicationsSection, fragment) {
			t.Fatalf("expected applications overview docs to include latest-runtime cue guard %q", fragment)
		}
	}

	for _, fragment := range []string{
		"The overview intentionally stays compact: rows show the app name, account/environment, project, linked-resource availability, sync, health, last update, and one dominant `Open application` action",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts docs to include applications-overview compact row contract %q", fragment)
		}
	}

	overviewUISection := markdownSection(t, overviewUIText, "### 1. Applications overview")
	for _, fragment := range []string{
		"Individual rows now include compact row-local `Project` and `Last update` fields",
		"operators can see ownership without turning the first page into a deep runtime dashboard",
	} {
		if !strings.Contains(overviewUISection, fragment) {
			t.Fatalf("expected overview UI docs to include compact project cue contract %q", fragment)
		}
	}
	if strings.Contains(overviewUISection, "project details remain omitted from the first page") {
		t.Fatalf("expected overview UI docs not to claim project details are omitted from the applications overview")
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

func TestOverviewDashboardNearTermRoadmapTracksCurrentOperatorGaps(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	section := markdownSection(t, overviewText, "## Near-term next steps")

	for _, fragment := range []string{
		"Already-shipped dashboard surfaces should not be reopened as generic roadmap items",
		"controller runtime-event timelines, resource-scoped event navigation, account/group runtime-event cues, the drift workspace `Drift source coverage` panel, and local-safe apply/executor metadata are current operator surfaces",
		"keep future drift-source coverage wording tied to new backend-authored `managedDriftSourceCoverage[]` evidence",
		"including source-specific unavailable or unsupported states, rather than inventing UI-only drift telemetry",
		"real CodeDeploy, ECS, ALB, ASG, or EC2 mutation execution remains a separate future live-AWS executor lane",
	} {
		if !strings.Contains(section, fragment) {
			t.Fatalf("expected near-term roadmap to include current operator gap %q", fragment)
		}
	}

	for _, stale := range []string{
		"richer runtime history navigation and event timelines",
		"real AWS mutation execution from reviewed plan/apply operations",
		"account/group summary sparklines or event cues",
		"deeper drift diagnostics once the backend exposes richer live drift sources beyond the current derived read model",
		"render backend-authored `managedDriftSourceCoverage[]` in the drift workspace",
	} {
		if strings.Contains(section, stale) {
			t.Fatalf("expected near-term roadmap not to reopen shipped or superseded work %q", stale)
		}
	}
}

func TestOverviewDashboardDocumentsDriftSourceCoveragePanel(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	section := markdownSection(t, overviewText, "### Drift workspace")

	for _, fragment := range []string{
		"renders a compact `Drift source coverage` panel from backend-authored `managedDriftSourceCoverage[]` entries",
		"reported rows, checked-but-empty evidence, unsupported bindings, or unavailable coverage",
		"before scanning the field-level table",
		"prefers backend-authored additive `managedDrift[]` rows",
		"falls back to the existing client-derived artifact/generation/runtime-intent comparisons for older payloads",
	} {
		if !strings.Contains(section, fragment) {
			t.Fatalf("expected drift workspace docs to describe drift source coverage panel contract %q", fragment)
		}
	}
}

func TestOverviewDashboardDeploymentScopedAuditWorkspaceDocumentsClearableAuditChips(t *testing.T) {
	overviewText := readTextFile(t, "features/overview-dashboard.md")
	readmeText := readTextFile(t, "../README.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
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
	for _, fragment := range []string{
		"`Refresh audit log` control",
		"`q`/`sync`/`account`, and stripping stale `#audit-*` anchors",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README audit workspace docs to include unsplit refresh/control contract %q", fragment)
		}
	}
	for _, fragment := range []string{
		"safe action filter control for exactly `plan.created`, `apply.requested`, `apply.transitioned`, and `controller.refresh.requested`",
		"`GET /v1/audit-logs?action={action}` narrows the same local audit timeline to exactly `plan.created`, `apply.requested`, `apply.transitioned`, or `controller.refresh.requested`",
		"visible-entry summary for total rows, `plan.created`, `apply.requested`, `apply.transitioned`, `controller.refresh.requested`, recorded actors, and unavailable/unknown actors",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI audit workspace docs to include complete audit action contract %q", fragment)
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
		"compact retained runtime-events cue from already-loaded `account.deployments[].runtimeSummary.historyCount` and `history[]` fields",
		"`No recent runtime events` without adding another backend read",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts docs to include account detail contract %q", fragment)
		}
	}
}

func TestManagedLoginGuideDocumentsProviderRoutesAndVerificationContract(t *testing.T) {
	managedLoginText := readTextFile(t, "features/managed-login.md")
	for _, fragment := range []string{
		"`TENET_AUTH_PROVIDER=github` activates GitHub OAuth",
		"`TENET_AUTH_PROVIDER=google_oidc` activates Google Workspace OIDC",
		"`TENET_AUTH_PROVIDER=okta_oidc` activates Okta OIDC",
		"`TENET_AUTH_PROVIDER=okta_saml` is diagnostic-only",
		"Never document `TENET_AUTH_PROVIDER` with a secret-shaped placeholder",
		"Copy these mode values exactly in operator runbooks and issue evidence; redaction should stop at credential-bearing settings.",
		"`GET /auth/login/github`",
		"`GET /auth/login/google`",
		"`GET /auth/login/okta`",
		"`GET /auth/callback/okta/oidc`",
		"`GET /auth/logout` and `POST /auth/logout`",
		"The legacy `/api/v1/auth/*` family is not the managed-login route contract.",
		"`TENET_AUTH_SESSION_COOKIE_SECURE=true`",
		"sessionCookieSecureOverride",
		"sessionCookieSecureForRequest",
		"sessionCookieSecureSources",
		"`TENET_API_TRUST_AUTHENTICATED_HEADERS=true`",
		"Trusted front-door identity still wins when trusted headers are present",
		"`go test ./internal/api ./cmd/tenet-api`",
		"`go test ./internal/authprovider ./internal/authidentity ./internal/authstate`",
		"Local and Docker smoke intentionally cover the out-of-box disabled managed-login runtime",
		"active GitHub, Google Workspace, and Okta OIDC sign-in/sign-out flows without live GitHub, Google, or Okta credentials",
		"`/auth/logout?returnUrl=...` in the same shell",
		"clear only Tenet's signed `tenet_session` cookie",
		"avoid claiming provider-token revocation or upstream front-door sign-out",
	} {
		if !strings.Contains(managedLoginText, fragment) {
			t.Fatalf("expected managed-login guide to include %q", fragment)
		}
	}
	if strings.Contains(managedLoginText, "provider=***") || strings.Contains(managedLoginText, "`***") {
		t.Fatalf("expected managed-login guide to avoid secret-shaped provider-mode placeholders")
	}
}

func TestManagedLoginDocsKeepNonSecretProviderExamplesConcrete(t *testing.T) {
	redacted := strings.Repeat("*", 3)
	for _, source := range []struct {
		path     string
		examples []string
	}{
		{path: "../README.md", examples: []string{"TENET_AUTH_PROVIDER=github", "TENET_AUTH_PROVIDER=google_oidc", "TENET_AUTH_PROVIDER=okta_oidc", "TENET_AUTH_PROVIDER=okta_saml", "TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true"}},
		{path: "architecture/tenet-v1-enterprise-sso.md", examples: []string{"TENET_AUTH_PROVIDER=github", "TENET_AUTH_PROVIDER=google_oidc", "TENET_AUTH_PROVIDER=okta_oidc", "TENET_AUTH_PROVIDER=okta_saml", "TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true"}},
		{path: "architecture/tenet-v1-operator-contracts.md", examples: []string{"TENET_AUTH_PROVIDER=github", "TENET_AUTH_PROVIDER=google_oidc", "TENET_AUTH_PROVIDER=okta_oidc", "TENET_AUTH_PROVIDER=okta_saml", "TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true"}},
		{path: "features/managed-login.md", examples: []string{"TENET_AUTH_PROVIDER=github", "TENET_AUTH_PROVIDER=google_oidc", "TENET_AUTH_PROVIDER=okta_oidc", "TENET_AUTH_PROVIDER=okta_saml"}},
	} {
		body := readTextFile(t, source.path)
		for _, placeholder := range []string{
			"TENET_AUTH_PROVIDER=" + redacted,
			"TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=" + redacted,
		} {
			if strings.Contains(body, placeholder) {
				t.Fatalf("expected %s to avoid redacted non-secret provider/RBAC examples; found %q", source.path, placeholder)
			}
		}
		for _, example := range source.examples {
			if !strings.Contains(body, example) {
				t.Fatalf("expected %s to include concrete non-secret example %q", source.path, example)
			}
		}
	}
}

func TestResourceDetailTroubleshootingSummaryDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"The bookmarkable resource page (`/?page=resource...`)",
		"compact `Troubleshooting summary`",
		"before lazy Related runtime events, Recent logs, or Live inspection panels load",
		"selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"latest lazy Related runtime events, Recent logs, and Live inspection panel states",
		"empty-log and unavailable-provider next-step guidance",
		"Unsupported for this resource type",
		"points operators back to the current resource read",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	for _, fragment := range []string{
		"The resource-detail Troubleshooting summary is a derived UI contract, not a separate API read.",
		"latest completed Related runtime events, Live inspection, and Recent logs panel states",
		"selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"Current resource read: GET /v1/overview/deployments/{deploymentName}",
		"keeps `Current resource read: GET /v1/overview/deployments/{deploymentName}` visible before lazy Related runtime events, Recent logs, or Live inspection reads resolve",
		"keeps the selected resource's `Linked-resource source` cue visible beside it",
		"controller-authored-versus-fallback resource provenance remain clear",
		"unsupported focused-evidence states",
		"routes the next check back to the current resource read",
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	overviewDashboardText := readTextFile(t, "features/overview-dashboard.md")
	for _, fragment := range []string{
		"compact `Selected resource context` callout",
		"visible before the lazy Recent logs and Live inspection panels load",
		"repeats the `Linked-resource source` cue in the selected-resource context",
		"controller-authored `rollout.execution[]` rows or from canonical fallback rollout inventory",
		"the final resource page also renders a compact `Troubleshooting summary`",
		"any selected lookup scope (`containerName`, `clusterArn`, or `instanceId`)",
		"available, loading, empty, unavailable, fetch-error, and unsupported focused-evidence states",
		"unsupported-resource guidance",
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
		"Unsupported for this resource type",
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
