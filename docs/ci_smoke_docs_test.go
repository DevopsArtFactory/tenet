package docs

import (
	"os"
	"strings"
	"testing"
)

func TestREADMEValidationDocumentsCISmokeFastLaneScriptChecks(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	workflowText := readTextFile(t, "../.github/workflows/ci-smoke.yml")
	for _, command := range []string{
		"bash -n scripts/smoke-local.sh",
		"bash -n scripts/smoke-docker.sh",
		"bash -n scripts/ci-smoke-runner-evidence.sh",
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
		"operator smoke helpers",
		"The Docker smoke lane remains separate",
		"In hosted PRs it is path-scoped",
		"docs-only changes normally stay on the fast lane",
		"changes under Docker/Compose, API/controller, local fixtures, runtime read models, resource evidence, webhook, plan/apply, or audit smoke surfaces run the full Docker stack before merge",
		"make smoke-docker",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README validation docs to include %q", fragment)
		}
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

func TestDocumentationMapLinksResourceInspectionGuide(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	overviewText := readTextFile(t, "architecture/tenet-v1-overview.md")
	overviewUIText := readTextFile(t, "architecture/tenet-v1-overview-ui.md")
	contractsText := readTextFile(t, "architecture/tenet-v1-operator-contracts.md")
	featureText := readTextFile(t, "features/resource-inspection.md")
	recentLogsFeatureText := readTextFile(t, "features/resource-recent-logs.md")

	for _, fragment := range []string{
		"`docs/features/resource-inspection.md` documents live resource inspection selectors, lookup context, and bounded sample metadata.",
		"`docs/features/resource-recent-logs.md` documents recent-log resource contracts.",
		"Capture the HTTP `Request-Id` response header with that lookup context when escalating a failed or unavailable Recent logs or Live inspection read",
		"Recent logs and Live inspection cards label the same value as `Tenet request ID` metadata",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README documentation map to include %q", fragment)
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

func TestApplicationDetailResourceFilterDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"`Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search",
		"with an inline `Clear resource filter` action whenever",
		"that filter is active, while rollout/history/log evidence stays on the final resource",
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
		"When `resourceFilter` hides all reported linked resources, the UI renders the filtered-empty state `No linked resources match the current filter. Clear the resource filter to show all reported linked resources.`",
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

func TestResourceDetailTroubleshootingSummaryDocsContract(t *testing.T) {
	readmeText := readTextFile(t, "../README.md")
	for _, fragment := range []string{
		"The bookmarkable resource page (`/?page=resource...`)",
		"compact `Troubleshooting summary`",
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
	} {
		if !strings.Contains(overviewUIText, fragment) {
			t.Fatalf("expected overview UI docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}

	overviewDashboardText := readTextFile(t, "features/overview-dashboard.md")
	for _, fragment := range []string{
		"compact `Selected resource context` callout",
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
		"the exact live-inspection/recent-log backing reads operators can refresh below",
		"GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}",
		"GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}",
	} {
		if !strings.Contains(operatorContractsText, fragment) {
			t.Fatalf("expected operator contracts docs to include resource-detail troubleshooting summary contract %q", fragment)
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
