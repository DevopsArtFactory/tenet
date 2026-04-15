package api

import (
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedUILazyLoadsDeploymentDetailsOnExpandOnly(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const detailStateByDeployment = new Map();",
		"const loadDeploymentDetail = (deploymentName, container) => {",
		"detailPanel.hidden = true;",
		"loadDeploymentDetail(deployment.name, detailPanel).catch(() => {});",
		"fetch(`/v1/overview/deployments/${encodeURIComponent(deploymentName)}`)",
		"if (!response.ok) throw new Error(`detail request failed (${response.status})`);",
		"renderDetailState(container, 'Loading deployment details…');",
		"detailStateByDeployment.set(deploymentName, { status: 'ready', detail });",
		"renderDetailState(container, `Failed to load deployment details: ${error.message}`, 'error');",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	clickContract := regexp.MustCompile(`toggle\.addEventListener\('click', \(\) => \{[\s\S]*?const expanded = toggle\.getAttribute\('aria-expanded'\) === 'true';[\s\S]*?const nextExpanded = !expanded;[\s\S]*?toggle\.setAttribute\('aria-expanded', String\(nextExpanded\)\);[\s\S]*?row\.classList\.toggle\('expanded', nextExpanded\);[\s\S]*?detailPanel\.hidden = !nextExpanded;[\s\S]*?if \(nextExpanded\) \{[\s\S]*?loadDeploymentDetail\(deployment\.name, detailPanel\)\.catch\(\(\) => \{\}\);[\s\S]*?\}[\s\S]*?\}\);`)
	if !clickContract.MatchString(body) {
		t.Fatalf("expected embedded UI to keep the expand-toggle lazy-load contract intact")
	}
}

func TestEmbeddedUIUsesSafeDetailRenderingAndPreservesFalsyMetrics(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const metricValue = (value) => value ?? '—';",
		"replaceChildren",
		"textContent",
		"Sync reason",
		"Account placement",
		"Rollout generations",
		"Rollout steps",
		"Conditions",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	for _, unsafe := range []string{
		"details.innerHTML",
		"detailPanel.innerHTML",
		"conditionList.innerHTML",
		"stepsList.innerHTML",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected embedded UI to avoid unsafe detail rendering %q", unsafe)
		}
	}
}

func TestEmbeddedUIIncludesOverviewStatusInFetchErrors(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"fetch('/v1/overview/accounts')",
		"if (!response.ok) throw new Error(`overview request failed (${response.status})`);",
		"renderEmptyState(`Failed to load overview: ${error.message}`);",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}
