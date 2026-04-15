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
		"renderDeploymentDetail(container, cached.detail);",
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
		"Rollout execution",
		"detail.rollout?.execution || []",
		"Execution resource",
		"Execution description",
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

func TestEmbeddedUIIncludesQuickLinksToDeploymentReadModels(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const createNavLink = (href, text) => {",
		"link.target = '_blank';",
		"link.rel = 'noreferrer noopener';",
		"link.href = href;",
		"const detailLinks = createElement('div', 'row-links row-links-shell');",
		"createNavLink(`/v1/overview/deployments/${encodeURIComponent(deployment.name)}`, 'Overview JSON')",
		"createNavLink(`/v1/status/deployments/${encodeURIComponent(deployment.name)}`, 'Status JSON')",
		"detailLinks.append(",
		"row.append(toggle, detailLinks, detailPanel);",
		"Overview JSON",
		"Status JSON",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
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

func TestEmbeddedUIRendersAccountStatusCountPills(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"const deriveAccountCounts = (account) => {",
		"const deployments = account.deployments || [];",
		"const counts = {",
		"deploymentCount: deployments.length,",
		"syncedCount: 0,",
		"progressingCount: 0,",
		"pausedCount: 0,",
		"errorCount: 0,",
		"counts.syncedCount += 1;",
		"counts.progressingCount += 1;",
		"counts.pausedCount += 1;",
		"counts.errorCount += 1;",
		"const fallbackCounts = deriveAccountCounts(account);",
		"[account.deploymentCount ?? fallbackCounts.deploymentCount, 'deployments', 'pill target']",
		"[account.syncedCount ?? fallbackCounts.syncedCount, 'synced', 'pill synced']",
		"[account.progressingCount ?? fallbackCounts.progressingCount, 'progressing', 'pill progress']",
		"[account.pausedCount ?? fallbackCounts.pausedCount, 'paused', 'pill paused']",
		"[account.errorCount ?? fallbackCounts.errorCount, 'error', 'pill error']",
		"accountCountPills.forEach(([value, label, className]) => {",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	fallbackContract := regexp.MustCompile(`const deriveAccountCounts = \(account\) => \{[\s\S]*?const deployments = account\.deployments \|\| \[\];[\s\S]*?deployments\.forEach\(\(deployment\) => \{[\s\S]*?switch \(deployment\.sync\?\.state\) \{[\s\S]*?case 'Synced':[\s\S]*?counts\.syncedCount \+= 1;[\s\S]*?case 'Reconciling':[\s\S]*?case 'OutOfSync':[\s\S]*?case 'Drifted':[\s\S]*?counts\.progressingCount \+= 1;[\s\S]*?case 'Paused':[\s\S]*?counts\.pausedCount \+= 1;[\s\S]*?case 'Error':[\s\S]*?counts\.errorCount \+= 1;[\s\S]*?default:[\s\S]*?\}[\s\S]*?\};`)
	if !fallbackContract.MatchString(body) {
		t.Fatalf("expected embedded UI to derive missing account status counts from deployment sync state")
	}
}

func TestEmbeddedUIProvidesFilterControlsAndURLSyncedViewState(t *testing.T) {
	body := string(indexHTML)

	for _, fragment := range []string{
		"<section class=\"toolbar\" id=\"toolbar\"></section>",
		"const createSelect = (labelText, value, options, onChange) => {",
		"const createInput = (labelText, value, placeholder, onInput) => {",
		"const parseViewState = () => {",
		"const normalizeViewState = (accounts, state) => {",
		"const syncViewState = (state) => {",
		"const applyViewState = (accounts, state) => {",
		"const searchNeedle = state.query.trim().toLowerCase();",
		"account.name,",
		"account.awsAccountId,",
		"state.account === 'all' || account.awsAccountId === state.account",
		"deployment.rollout?.summary,",
		"plannedSteps(deployment.rollout),",
		"`${healthLabel(deployment.health)}${deployment.health?.summary ? ` ${deployment.health.summary}` : ''}`,",
		"const matchesState = state.syncState === 'all' || deployment.sync?.state === state.syncState;",
		"const query = params.toString();",
		"const hash = window.location.hash || '';",
		"const nextURL = query ? `${window.location.pathname}?${query}${hash}` : `${window.location.pathname}${hash}`;",
		"history.replaceState({}, '', nextURL);",
		"value: account.awsAccountId,",
		"renderToolbar(allAccounts, currentViewState);",
		"currentViewState = normalizeViewState(accounts, parseViewState());",
		"renderOverview();",
		"renderEmptyState('No deployments match the current filters.');",
		"Clear filters",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}

	urlStateContract := regexp.MustCompile(`const parseViewState = \(\) => \{[\s\S]*?new URLSearchParams\(window\.location\.search\)[\s\S]*?query: params\.get\('q'\) \|\| ''[\s\S]*?syncState: params\.get\('sync'\) \|\| 'all'[\s\S]*?account: params\.get\('account'\) \|\| 'all'[\s\S]*?\};[\s\S]*?const normalizeViewState = \(accounts, state\) => \{[\s\S]*?const validSyncStates = new Set\([\s\S]*?'Synced'[\s\S]*?'Error'[\s\S]*?\);[\s\S]*?const validAccounts = new Set\(\['all', \.\.\.accounts\.map\(\(account\) => account\.awsAccountId\)\.filter\(Boolean\)\]\);[\s\S]*?query: \(state\.query \|\| ''\)\.trim\(\),[\s\S]*?account: validAccounts\.has\(state\.account\) \? state\.account : 'all'[\s\S]*?\};[\s\S]*?const syncViewState = \(state\) => \{[\s\S]*?params\.set\('q', state\.query\);[\s\S]*?params\.set\('sync', state\.syncState\);[\s\S]*?params\.set\('account', state\.account\);[\s\S]*?history\.replaceState\(\{\}, '', nextURL\);`)
	if !urlStateContract.MatchString(body) {
		t.Fatalf("expected embedded UI to round-trip validated filter state through URL query params")
	}
}

func TestEmbeddedUIPreservesToolbarFocusAndRestoresCaretAcrossFilterRerenders(t *testing.T) {
	body := string(indexHTML)

	if strings.Count(body, "const captureToolbarFocus = () => {") != 1 {
		t.Fatalf("expected embedded UI to declare captureToolbarFocus exactly once")
	}
	if strings.Count(body, "const restoreToolbarFocus = (focusState) => {") != 1 {
		t.Fatalf("expected embedded UI to declare restoreToolbarFocus exactly once")
	}

	for _, fragment := range []string{
		"select.dataset.toolbarField = labelText;",
		"input.dataset.toolbarField = labelText;",
		"const captureToolbarFocus = () => {",
		"const restoreToolbarFocus = (focusState) => {",
		"selectionStart: typeof active.selectionStart === 'number' ? active.selectionStart : null,",
		"selectionEnd: typeof active.selectionEnd === 'number' ? active.selectionEnd : null,",
		"const selector = `[data-toolbar-field=\"${focusState.field}\"]`;",
		"const node = document.querySelector(selector);",
		"node.focus();",
		"if (node instanceof HTMLInputElement && focusState.selectionStart !== null && focusState.selectionEnd !== null) {",
		"node.setSelectionRange(focusState.selectionStart, focusState.selectionEnd);",
		"const focusState = captureToolbarFocus();",
		"renderOverview(focusState);",
		"restoreToolbarFocus(focusState);",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected embedded UI to include %q", fragment)
		}
	}
}
