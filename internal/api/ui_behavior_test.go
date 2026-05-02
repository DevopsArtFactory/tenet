package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func embeddedUIScriptForTest(t *testing.T) string {
	t.Helper()

	scriptBlock := regexp.MustCompile(`(?s)<script>(.*)</script>`)
	matches := scriptBlock.FindSubmatch(indexHTML)
	if len(matches) != 2 {
		t.Fatal("expected embedded UI HTML to contain exactly one script block")
	}

	return string(matches[1])
}

func runNodeHarness(t *testing.T, nodeProgram string) {
	t.Helper()

	scriptFile, err := os.CreateTemp(t.TempDir(), "node-harness-*.js")
	if err != nil {
		t.Fatalf("create node harness: %v", err)
	}

	if _, err := scriptFile.WriteString(nodeProgram); err != nil {
		_ = scriptFile.Close()
		t.Fatalf("write node harness: %v", err)
	}

	if err := scriptFile.Close(); err != nil {
		t.Fatalf("close node harness: %v", err)
	}

	cmd := exec.Command("node", scriptFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestUIFilterBehaviorMatchesRuntimeSummaryTextWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestUIProjectEnvironmentGroupingWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestUIAccountTopologyUsesAuthoritativeCountsWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIRendersAccountJSONQuickLinkInAccountHeaderWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIRendersGroupQuickLinksInGroupHeadersWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIRendersDeploymentQuickLinksBesideExpandButtonWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIFilterBehaviorWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIBehaviorWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIDetailFetchRerenderRaceWithNodeHarness(t *testing.T) {
	t.Skip("legacy dense-overview behavior replaced by applications-overview slice; coverage moved to slice-specific tests")
}

func TestEmbeddedUIReadRouteAuthGuidanceWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const collectText = (node) => node ? [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean) : [];
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }

const scenarios = [
  {
    name: 'applications overview 401 explains trusted front door retry',
    search: '?q=payments&sync=OutOfSync&account=210987654321',
    route: '/v1/overview/applications',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expected: [
      'Applications overview',
      'Authenticated operator required',
      'Tenet rejected GET /v1/overview/applications because authenticated identity is required on this operator read route.',
      'Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'applications overview 403 explains operator authorization failure',
    search: '?q=payments',
    route: '/v1/overview/applications',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expected: [
      'Applications overview',
      'Operator access denied',
      'Tenet rejected GET /v1/overview/applications after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'applications overview 403 explains viewer role requirement',
    search: '?q=payments&sync=OutOfSync',
    route: '/v1/overview/applications',
    status: 403,
    body: 'forbidden: authenticated viewer-capable role is required for this operator read route',
    expected: [
      'Applications overview',
      'Viewer role required',
      'Tenet rejected GET /v1/overview/applications after recognizing the caller\'s identity because this operator read route requires a trusted forwarded viewer/operator/admin role.',
      'Ask the front-door/RBAC admin to forward X-Tenet-Authenticated-Roles with viewer, operator, or admin, then retry this same bookmarkable page.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'account 401 keeps bookmarked route and explains trusted front door retry',
    search: '?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321',
    route: '/v1/overview/accounts/210987654321',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expected: [
      'Account detail',
      'Authenticated operator required',
      'Tenet rejected GET /v1/overview/accounts/210987654321 because authenticated identity is required on this operator read route.',
      'Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'account 403 keeps bookmarked route and explains operator authorization failure',
    search: '?page=account&accountId=210987654321&q=payments',
    route: '/v1/overview/accounts/210987654321',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expected: [
      'Account detail',
      'Operator access denied',
      'Tenet rejected GET /v1/overview/accounts/210987654321 after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'application 401 keeps bookmarked route and explains trusted front door retry',
    search: '?page=application&deployment=api-prod',
    route: '/v1/overview/applications/api-prod',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expected: [
      'Authenticated operator required',
      'Tenet rejected GET /v1/overview/applications/api-prod because authenticated identity is required on this operator read route.',
      'Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'resource 403 keeps bookmarked route and explains operator authorization failure',
    search: '?page=resource&deployment=api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::api-prod::Weighted%20routing%20plan',
    route: '/v1/overview/deployments/api-prod',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expected: [
      'Operator access denied',
      'Tenet rejected GET /v1/overview/deployments/api-prod after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'drift 401 keeps bookmarked route and explains trusted front door retry',
    search: '?page=drift&deployment=api-prod',
    route: '/v1/overview/deployments/api-prod',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expected: [
      'Authenticated operator required',
      'Tenet rejected GET /v1/overview/deployments/api-prod because authenticated identity is required on this operator read route.',
      'trusted front door',
    ],
    unexpected: 'Failed to load overview:',
  },
  {
    name: 'audit 403 keeps bookmarked route and explains operator authorization failure',
    search: '?page=audit&deployment=api-prod&action=apply.requested&limit=2',
    route: '/v1/audit-logs?deployment=api-prod&action=apply.requested&limit=2',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expected: [
      'Operator access denied',
      'Tenet rejected GET /v1/audit-logs?deployment=api-prod&action=apply.requested&limit=2 after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    ],
    unexpected: 'Failed to load audit log:',
  },
];

(async () => {
  for (const scenario of scenarios) {
    const document = new Document();
    document.byID.set('toolbar', document.createElement('section'));
    document.byID.set('summary', document.createElement('section'));
    document.byID.set('accounts', document.createElement('section'));
    const initialSearch = scenario.search;
    const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
    const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
    let fetchCount = 0;
    const fetch = (url) => {
      fetchCount += 1;
      if (String(url) !== scenario.route) throw new Error('unexpected fetch url for ' + scenario.name + ': ' + url);
      return Promise.resolve({
        ok: false,
        status: scenario.status,
        text: () => Promise.resolve(scenario.body),
      });
    };
    new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
    await settle();
    await settle();
    await settle();
    if (fetchCount !== 1) throw new Error('expected one fetch for ' + scenario.name + ', got ' + fetchCount);
    if (window.location.search !== initialSearch) throw new Error('expected bookmarkable URL to stay unchanged for ' + scenario.name + ', got ' + window.location.search);
    const text = [
      ...collectText(document.getElementById('toolbar')),
      ...collectText(document.getElementById('summary')),
      ...collectText(document.getElementById('accounts')),
    ].join(' | ');
    scenario.expected.forEach((fragment) => {
      if (!text.includes(fragment)) throw new Error('expected fragment for ' + scenario.name + ': ' + fragment + '\nUI text: ' + text);
    });
    if (scenario.unexpected && text.includes(scenario.unexpected)) throw new Error('unexpected generic failure copy for ' + scenario.name + ': ' + text);
  }
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestEmbeddedUIOperatorContextBannerWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = [{ name: 'payments-api-prod', appRef: 'payments', environmentRef: 'prod', accountName: 'prod-core', awsAccountId: '123456789012', region: 'us-west-2', targetType: 'ecs', executionMode: 'ecsRolling', sync: { state: 'Synced' }, health: { overall: 'Healthy' }, runtimeSummary: { timestamp: '2026-04-16T06:40:00Z' } }];
class Element {
  constructor(tagName, ownerDocument) { this.tagName = String(tagName).toUpperCase(); this.ownerDocument = ownerDocument; this.children = []; this.parentNode = null; this.className = ''; this.textContent = ''; this.dataset = {}; this.listeners = {}; this.attributes = {}; this.hidden = false; this.value = ''; this.disabled = false; this.type = ''; this.placeholder = ''; this.autocomplete = ''; this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } }; }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); await Promise.resolve(); }
const findFirst = (node, predicate) => {
  if (!node) return null;
  if (predicate(node)) return node;
  for (const child of node.children || []) {
    const found = findFirst(child, predicate);
    if (found) return found;
  }
  return null;
};
async function render(operatorResponses) {
  const document = new Document();
  document.byID.set('operator-context', document.createElement('section'));
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  const fetchCalls = [];
  const responses = Array.isArray(operatorResponses) ? operatorResponses.slice() : [operatorResponses];
  const window = { location: { pathname: '/', search: '', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/operator/context') return Promise.resolve(responses.shift());
    if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  const text = () => collectText(document.getElementById('operator-context')).join(' | ');
  const refresh = () => findFirst(document.getElementById('operator-context'), (node) => node.tagName === 'BUTTON' && node.textContent.includes('Refresh'));
  return { document, window, text, fetchCalls, refresh };
}
(async () => {
  const firstContext = { trustedHeaders: { enabled: true }, enforcement: { mode: 'trusted_front_door_read_write', summary: 'trusted front-door identity, roles, and deployment allowlists are enforcing guarded operator read/write routes' }, identity: { authenticated: true, name: 'Front Door Operator', email: 'frontdoor@example.com' }, roles: { names: ['viewer', 'operator'], canRead: true, canWrite: true }, deploymentAccess: { enforced: true, forwarded: true, names: ['payments-api-prod', 'worker-prod'], scopes: { forwarded: true, projects: ['payments', 'worker'], environments: ['prod'] }, summary: { available: true, forwardedCount: 2, matchedCount: 2, accountCount: 1, projectCount: 1, environmentCount: 2, unmatchedNames: [], projectEnvironmentExamples: [{ appRef: 'payments', environmentRef: 'prod', region: 'us-west-2', awsAccountId: '123456789012', deploymentCount: 1, exampleDeployments: ['payments-api-prod'] }, { appRef: 'worker', environmentRef: 'prod', region: 'us-west-2', awsAccountId: '123456789012', deploymentCount: 1, exampleDeployments: ['worker-prod'] }] } }, accessVerdicts: { operatorRead: { allowed: true, reason: 'guarded operator read prerequisites are satisfied for this request', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, exampleRoutes: ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/audit-logs'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles'] }, operatorWrite: { allowed: true, reason: 'guarded operator write prerequisites are satisfied for this request', requiresAuthenticatedIdentity: true, requiresOperatorRole: true, exampleRoutes: ['POST /v1/plans/deployments/{deploymentName}', 'POST /v1/applies/plans/{planId}', 'POST /v1/applies/{applyId}/transitions'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles'] }, deploymentScopedRead: { allowed: true, reason: 'deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, requiresExactDeploymentAllowlistMatch: true, acceptsProjectEnvironmentScopes: true, exampleRoutes: ['GET /v1/overview/deployments/{deploymentName}', 'GET /v1/status/deployments/{deploymentName}', 'GET /v1/audit-logs?deployment={deploymentName}'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] }, deploymentScopedWrite: { allowed: true, reason: 'deployment-scoped writes can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments', requiresAuthenticatedIdentity: true, requiresOperatorRole: true, requiresExactDeploymentAllowlistMatch: true, acceptsProjectEnvironmentScopes: true, exampleRoutes: ['POST /v1/plans/deployments/{deploymentName}', 'POST /v1/applies/plans/{planId}'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] }, collectionRead: { allowed: true, reason: 'collection reads are filtered to the union of exact deployment grants and project/environment-scoped catalog matches', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, filtersCollectionsByDeploymentAccess: true, exampleRoutes: ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/overview/accounts', 'GET /v1/audit-logs'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] }, catalogRead: { allowed: true, reason: 'raw catalog reads are filtered to deployment-visible desired-state metadata before returning catalog collections or summaries', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, filtersCollectionsByDeploymentAccess: true, exampleRoutes: ['GET /v1/catalog/deployments', 'GET /v1/catalog/accounts', 'GET /v1/catalog/placements', 'GET /v1/catalog/topology', 'GET /v1/catalog/summary'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] } } };
  const unmatchedContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: ['payments-api-prod', 'stale-api', 'archived-api'], summary: { available: true, forwardedCount: 3, matchedCount: 1, unmatchedCount: 2, accountCount: 1, projectCount: 1, environmentCount: 1, unmatchedNames: ['archived-api', 'stale-api'] } } };
  const truncatedUnmatchedContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: ['payments-api-prod', 'stale-a', 'stale-b', 'stale-c', 'stale-d'], summary: { available: true, forwardedCount: 29, matchedCount: 1, unmatchedCount: 28, unmatchedNamesTruncated: true, accountCount: 1, projectCount: 1, environmentCount: 1, unmatchedNames: ['stale-a', 'stale-b'] } } };
  const staleScopeContext = { ...firstContext, deploymentAccess: { ...firstContext.deploymentAccess, summary: { ...firstContext.deploymentAccess.summary, forwardedProjectCount: 3, forwardedEnvironmentCount: 4, unmatchedProjectCount: 1, unmatchedProjects: ['legacy-payments'], unmatchedEnvironmentCount: 2, unmatchedEnvironments: ['old-prod', 'qa-apne2'], unmatchedEnvironmentsTruncated: true } } };
  const localContext = { ...firstContext, trustedHeaders: { enabled: false }, enforcement: { mode: 'local_unrestricted', summary: 'trusted front-door headers are not enabled; Tenet is not enforcing authenticated operator gates for this API instance' } };
  const emptyCollectionContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: ['stale-api'], summary: { available: true, forwardedCount: 1, matchedCount: 0, unmatchedNames: ['stale-api'], emptyReason: 'no forwarded deployment names or project/environment scopes match catalog placements' } }, accessVerdicts: { ...firstContext.accessVerdicts, collectionRead: { allowed: true, reason: 'collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, filtersCollectionsByDeploymentAccess: true, emptyCollectionLikely: true, exampleRoutes: ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/overview/accounts', 'GET /v1/audit-logs'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] }, catalogRead: { allowed: true, reason: 'collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, filtersCollectionsByDeploymentAccess: true, emptyCollectionLikely: true, exampleRoutes: ['GET /v1/catalog/deployments', 'GET /v1/catalog/accounts', 'GET /v1/catalog/placements', 'GET /v1/catalog/topology', 'GET /v1/catalog/summary'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles', 'X-Tenet-Authorized-Deployments', 'X-Tenet-Authorized-Projects', 'X-Tenet-Authorized-Environments'] } } };
  const deniedContext = { ...firstContext, identity: { authenticated: true, name: 'Readless Operator' }, roles: { names: ['auditor'], canRead: false, canWrite: false }, accessVerdicts: { ...firstContext.accessVerdicts, operatorRead: { allowed: false, reason: 'authenticated viewer-capable role is required for guarded operator read routes', requiresAuthenticatedIdentity: true, requiresViewerCapableRole: true, exampleRoutes: ['GET /v1/overview/applications', 'GET /v1/status/deployments', 'GET /v1/catalog/deployments', 'GET /v1/audit-logs'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles'] }, operatorWrite: { allowed: false, reason: 'authenticated operator role is required for guarded operator write routes', requiresAuthenticatedIdentity: true, requiresOperatorRole: true, exampleRoutes: ['POST /v1/plans/deployments/{deploymentName}', 'POST /v1/applies/plans/{planId}', 'POST /v1/applies/{applyId}/transitions'], requiredTrustedHeaders: ['X-Tenet-Authenticated-Id', 'X-User-ID', 'X-Tenet-Authenticated-Roles'] } } };
  const projectOnlyScopeContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: [], scopes: { forwarded: true, projects: ['payments'], environments: [] }, summary: { available: true, forwardedCount: 0, matchedCount: 0, emptyReason: 'no forwarded deployment names or project/environment scopes match catalog placements' } } };
  const scopedReadOnlyContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: [], scopes: { forwarded: true, projects: ['payments'], environments: ['prod'] }, summary: { available: true, forwardedCount: 0, matchedCount: 1, accountCount: 1, projectCount: 1, environmentCount: 1, projectEnvironmentExamples: [{ appRef: 'payments', environmentRef: 'prod', region: 'us-west-2', awsAccountId: '123456789012', deploymentCount: 1, exampleDeployments: ['payments-api-prod'] }] } } };
  const unavailableSummaryContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: ['payments-api-prod'], scopes: { forwarded: true, projects: ['payments'], environments: ['prod'] }, summary: { available: false, forwardedCount: 1, unavailableReason: 'catalog placement provider is unavailable' } } };
  const absentSummaryContext = { ...firstContext, deploymentAccess: { enforced: true, forwarded: true, names: ['payments-api-prod'] } };
  const ready = await render({ ok: true, json: () => Promise.resolve(firstContext) });
  if (ready.fetchCalls[0] !== '/v1/operator/context') throw new Error('expected operator context endpoint to load before route data');
  if (!ready.text().includes('Operator context')) throw new Error('expected operator context banner title');
  if (!ready.text().includes('Refresh context')) throw new Error('expected refresh action in operator context banner');
  if (ready.refresh().disabled) throw new Error('refresh action should be enabled after first context load');
  if (!ready.text().includes('Front Door Operator')) throw new Error('expected trusted identity in operator context banner');
  if (!ready.text().includes('Write-capable roles: viewer, operator')) throw new Error('expected role capability in operator context banner');
  if (!ready.text().includes('Exact deployment grants: payments-api-prod, worker-prod')) throw new Error('expected exact deployment grants summary');
  if (!ready.text().includes('Project/environment scopes: projects payments, worker; environments prod')) throw new Error('expected project/environment scope summary');
  if (!ready.text().includes('Deployment access: all 2 forwarded deployments match catalog across 1 account, 1 project, 2 environments')) throw new Error('expected matched deployment access diagnostic summary');
  if (!ready.text().includes('Project/environment visibility: payments / prod / us-west-2 / 123456789012 (1 deployment) e.g. payments-api-prod; worker / prod / us-west-2 / 123456789012 (1 deployment) e.g. worker-prod')) throw new Error('expected project/environment examples in operator context banner');
  if (!ready.text().includes('Enforcement mode: read/write guarded')) throw new Error('expected read/write enforcement mode');
  if (!ready.text().includes('Enforcement summary: trusted front-door identity, roles, and deployment allowlists are enforcing guarded operator read/write routes')) throw new Error('expected enforcement summary');
  if (!ready.text().includes('Operator reads allowed: guarded operator read prerequisites are satisfied for this request Examples: GET /v1/overview/applications; GET /v1/status/deployments; +2 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles')) throw new Error('expected operator read access verdict with route examples and required headers');
  if (!ready.text().includes('Deployment reads allowed: deployment-scoped reads can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments Examples: GET /v1/overview/deployments/{deploymentName}; GET /v1/status/deployments/{deploymentName}; +1 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Scope grants: exact deployments or matching project/environment scopes')) throw new Error('expected deployment read access verdict with route examples, required headers, and scope grant cue');
  if (!ready.text().includes('Deployment writes allowed: deployment-scoped writes can pass for deployments named in X-Tenet-Authorized-Deployments or matched by X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments Examples: POST /v1/plans/deployments/{deploymentName}; POST /v1/applies/plans/{planId} Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Scope grants: exact deployments or matching project/environment scopes')) throw new Error('expected deployment write access verdict with scope grant cue');
  if (!ready.text().includes('Collection reads allowed: collection reads are filtered to the union of exact deployment grants and project/environment-scoped catalog matches Examples: GET /v1/overview/applications; GET /v1/status/deployments; +3 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Collection scope: filtered to visible deployments')) throw new Error('expected collection read access verdict with route examples, required headers, and collection filtering cue');
  if (!ready.text().includes('Raw catalog reads allowed: raw catalog reads are filtered to deployment-visible desired-state metadata before returning catalog collections or summaries Examples: GET /v1/catalog/deployments; GET /v1/catalog/accounts; +3 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Collection scope: filtered to visible deployments')) throw new Error('expected raw catalog read access verdict with route examples, required headers, and collection filtering cue');
  if ((ready.text().match(/Scope grants: exact deployments or matching project\/environment scopes/g) || []).length !== 2) throw new Error('expected scope grant cue only on deployment read/write verdicts');
  if (!ready.text().includes('Trusted headers enabled')) throw new Error('expected trusted header status');
  if (ready.text().includes('Project/environment scope warning:')) throw new Error('normal operator context should stay quiet when no stale project/environment scopes are reported');

  const unmatched = await render({ ok: true, json: () => Promise.resolve(unmatchedContext) });
  if (!unmatched.text().includes('Exact deployment grants: payments-api-prod, stale-api, archived-api')) throw new Error('expected exact deployment grants to stay visible');
  if (!unmatched.text().includes('Deployment access warning: 1 of 3 forwarded match catalog across 1 account, 1 project, 1 environment; 2 unmatched front-door names: archived-api, stale-api. Check the front-door allowlist.')) throw new Error('expected unmatched deployment access diagnostic summary');
  const warningPill = findFirst(unmatched.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning'));
  if (!warningPill || !warningPill.textContent.includes('Check the front-door allowlist.')) throw new Error('expected unmatched deployment access warning pill');

  const truncatedUnmatched = await render({ ok: true, json: () => Promise.resolve(truncatedUnmatchedContext) });
  if (!truncatedUnmatched.text().includes('Deployment access warning: 1 of 29 forwarded match catalog across 1 account, 1 project, 1 environment; 28 unmatched front-door names: stale-a, stale-b, …. Check the front-door allowlist.')) throw new Error('expected truncated unmatched deployment sample diagnostic summary');

  const staleScopes = await render({ ok: true, json: () => Promise.resolve(staleScopeContext) });
  if (!staleScopes.text().includes('Exact deployment grants: payments-api-prod, worker-prod')) throw new Error('expected exact deployment grants to stay separate from stale scope diagnostics');
  if (!staleScopes.text().includes('Project/environment scope warning: 2 of 3 forwarded projects match catalog; 1 unmatched: legacy-payments. 2 of 4 forwarded environments match catalog; 2 unmatched: old-prod, qa-apne2, …. Check X-Tenet-Authorized-Projects and X-Tenet-Authorized-Environments.')) throw new Error('expected stale project/environment scope diagnostic summary');
  const staleScopePill = findFirst(staleScopes.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning') && node.textContent.includes('legacy-payments'));
  if (!staleScopePill) throw new Error('expected stale project/environment scope warning pill');

  const local = await render({ ok: true, json: () => Promise.resolve(localContext) });
  if (!local.text().includes('Enforcement mode: local unrestricted')) throw new Error('expected local unrestricted enforcement mode');
  if (!local.text().includes('Enforcement summary: trusted front-door headers are not enabled; Tenet is not enforcing authenticated operator gates for this API instance')) throw new Error('expected local unrestricted enforcement summary');
  const localPill = findFirst(local.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning') && node.textContent.includes('local unrestricted'));
  if (!localPill) throw new Error('expected local unrestricted enforcement warning pill');

  const emptyCollection = await render({ ok: true, json: () => Promise.resolve(emptyCollectionContext) });
  if (!emptyCollection.text().includes('Collection reads allowed: collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments Examples: GET /v1/overview/applications; GET /v1/status/deployments; +3 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Collection scope: filtered to visible deployments Collection result: likely empty')) throw new Error('expected likely-empty collection read verdict with route examples, required headers, collection filtering cue, and likely-empty cue');
  if (!emptyCollection.text().includes('Raw catalog reads allowed: collection reads will be allowed but likely empty because the forwarded deployment grants or project/environment scopes do not match catalog deployments Examples: GET /v1/catalog/deployments; GET /v1/catalog/accounts; +3 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles; X-Tenet-Authorized-Deployments; X-Tenet-Authorized-Projects; X-Tenet-Authorized-Environments Collection scope: filtered to visible deployments Collection result: likely empty')) throw new Error('expected likely-empty raw catalog read verdict with route examples, required headers, collection filtering cue, and likely-empty cue');
  if (!emptyCollection.text().includes('Project/environment visibility: no visible project/environment groups (no forwarded deployment names or project/environment scopes match catalog placements)')) throw new Error('expected empty project/environment visibility diagnostic');
  const likelyEmptyPill = findFirst(emptyCollection.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning') && node.textContent.includes('Collection reads allowed'));
  if (!likelyEmptyPill) throw new Error('expected likely-empty collection verdict warning pill');
  if (ready.text().includes('Collection result: likely empty')) throw new Error('healthy collection read verdict should omit likely-empty cue');

  const denied = await render({ ok: true, json: () => Promise.resolve(deniedContext) });
  if (!denied.text().includes('Operator reads blocked: authenticated viewer-capable role is required for guarded operator read routes Examples: GET /v1/overview/applications; GET /v1/status/deployments; +2 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles')) throw new Error('expected denied operator read verdict with route examples and required headers');
  if (!denied.text().includes('Operator writes blocked: authenticated operator role is required for guarded operator write routes Examples: POST /v1/plans/deployments/{deploymentName}; POST /v1/applies/plans/{planId}; +1 more Required trusted headers: X-Tenet-Authenticated-Id; X-User-ID; X-Tenet-Authenticated-Roles')) throw new Error('expected denied operator write verdict with route examples and required headers');
  const deniedPill = findFirst(denied.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-denied') && node.textContent.includes('Operator reads blocked'));
  if (!deniedPill) throw new Error('expected blocked verdict warning pill');

  const projectOnlyScope = await render({ ok: true, json: () => Promise.resolve(projectOnlyScopeContext) });
  if (!projectOnlyScope.text().includes('Exact deployment grants: none forwarded')) throw new Error('expected exact grants empty state when only scopes are present');
  if (!projectOnlyScope.text().includes('Project/environment scopes warning: projects payments; no environments. Scoped reads require both project and environment headers.')) throw new Error('expected partial scope warning');
  const partialScopePill = findFirst(projectOnlyScope.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning') && node.textContent.includes('Scoped reads require both project and environment headers.'));
  if (!partialScopePill) throw new Error('expected partial project/environment scope warning pill');

  const scopedReadOnly = await render({ ok: true, json: () => Promise.resolve(scopedReadOnlyContext) });
  if (!scopedReadOnly.text().includes('Exact deployment grants: none forwarded')) throw new Error('expected exact grants empty state when scoped reads are available without exact grants');
  if (!scopedReadOnly.text().includes('Project/environment scopes: projects payments; environments prod')) throw new Error('expected complete project/environment scope summary');
  if (!scopedReadOnly.text().includes('Write scope reminder: deployment-scoped writes can pass with an exact X-Tenet-Authorized-Deployments grant or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement.')) throw new Error('expected scoped write reminder');
  const writeReminderPill = findFirst(scopedReadOnly.document.getElementById('operator-context'), (node) => String(node.className || '').includes('operator-context-warning') && node.textContent.includes('matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement'));
  if (!writeReminderPill) throw new Error('expected scoped write reminder warning pill');

  const unavailableSummary = await render({ ok: true, json: () => Promise.resolve(unavailableSummaryContext) });
  if (!unavailableSummary.text().includes('Deployment access summary unavailable: catalog placement provider is unavailable (1 forwarded)')) throw new Error('expected unavailable summary diagnostic copy');
  if (!unavailableSummary.text().includes('Project/environment visibility unavailable: catalog placement provider is unavailable')) throw new Error('expected unavailable project/environment diagnostic copy');

  const absentSummary = await render({ ok: true, json: () => Promise.resolve(absentSummaryContext) });
  if (!absentSummary.text().includes('Deployment access summary unavailable for this response')) throw new Error('expected summary-absent fallback copy');

  const refreshed = await render([
    { ok: true, json: () => Promise.resolve(firstContext) },
    { ok: true, json: () => Promise.resolve({ ...firstContext, identity: { authenticated: true, name: 'Refreshed Operator' }, roles: { names: ['viewer'], canRead: true, canWrite: false } }) },
  ]);
  refreshed.refresh().dispatch('click');
  await settle();
  if (refreshed.fetchCalls.filter((url) => url === '/v1/operator/context').length !== 2) throw new Error('expected refresh to re-fetch operator context');
  if (refreshed.window.location.search !== '') throw new Error('refresh should not change bookmarkable URL state');
  if (!refreshed.text().includes('Refreshed Operator')) throw new Error('expected refreshed operator identity');
  if (!refreshed.text().includes('Read-capable roles: viewer')) throw new Error('expected refreshed role capability');

  const failedRefresh = await render([
    { ok: true, json: () => Promise.resolve(firstContext) },
    { ok: false, status: 503, text: () => Promise.resolve('operator context provider unavailable') },
  ]);
  failedRefresh.refresh().dispatch('click');
  await settle();
  if (!failedRefresh.text().includes('Front Door Operator')) throw new Error('failed refresh should keep last successful operator context visible');
  if (!failedRefresh.text().includes('Last refresh failed:')) throw new Error('expected failed refresh diagnostic');
  if (!failedRefresh.text().includes('operator context provider unavailable')) throw new Error('expected failed refresh backend detail');

  const failedAuthRefresh = await render([
    { ok: true, json: () => Promise.resolve(firstContext) },
    { ok: false, status: 401, text: () => Promise.resolve('authenticated identity is required on this operator read route') },
  ]);
  failedAuthRefresh.refresh().dispatch('click');
  await settle();
  if (failedAuthRefresh.window.location.search !== '') throw new Error('auth failed refresh should not change bookmarkable URL state');
  if (!failedAuthRefresh.text().includes('Front Door Operator')) throw new Error('auth failed refresh should keep last successful operator context visible');
  if (!failedAuthRefresh.text().includes('Last refresh failed: Tenet rejected GET /v1/operator/context because authenticated identity is required on this operator read route. Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.')) throw new Error('expected auth guidance on failed operator context refresh');

  const authRequired = await render({ ok: false, status: 401, text: () => Promise.resolve('authenticated identity is required on this operator read route') });
  if (!authRequired.text().includes('Authenticated operator required')) throw new Error('expected auth-required operator context heading');
  if (!authRequired.text().includes('Tenet rejected GET /v1/operator/context because authenticated identity is required on this operator read route. Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.')) throw new Error('expected auth-required operator context recovery guidance');

  const roleRequired = await render({ ok: false, status: 403, text: () => Promise.resolve('authenticated viewer-capable role is required for this operator read route') });
  if (!roleRequired.text().includes('Viewer role required')) throw new Error('expected role-required operator context heading');
  if (!roleRequired.text().includes('Tenet rejected GET /v1/operator/context after recognizing the caller\'s identity because this operator read route requires a trusted forwarded viewer/operator/admin role. Ask the front-door/RBAC admin to forward X-Tenet-Authenticated-Roles with viewer, operator, or admin, then retry this same bookmarkable page.')) throw new Error('expected role-required operator context recovery guidance');

  const forbidden = await render({ ok: false, status: 403, text: () => Promise.resolve('deployment access denied') });
  if (!forbidden.text().includes('Operator access denied')) throw new Error('expected forbidden operator context heading');
  if (!forbidden.text().includes('Tenet rejected GET /v1/operator/context after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.')) throw new Error('expected forbidden operator context recovery guidance');

  const unavailable = await render({ ok: false, status: 503, text: () => Promise.resolve('operator context provider unavailable') });
  if (!unavailable.text().includes('Operator context unavailable')) throw new Error('expected unavailable operator context state');
  if (!unavailable.text().includes('GET /v1/operator/context')) throw new Error('expected diagnostic route in unavailable state');
  if (!unavailable.text().includes('operator context provider unavailable')) throw new Error('expected backend error detail in unavailable state');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestApplicationsOverviewPageWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	applicationsJSON, err := json.Marshal([]map[string]any{
		{
			"name":           "payments-api-prod",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"targetType":     "ec2",
			"strategy":       "canary",
			"executionMode":  "asgWeightedTargetGroups",
			"sync":           map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
			"health":         map[string]any{"overall": "Healthy", "summary": "3/3 targets healthy"},
			"runtimeSummary": map[string]any{"timestamp": "2026-04-16T06:40:00Z"},
		},
		{
			"name":           "payments-api-staging",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"accountName":    "staging-core",
			"awsAccountId":   "210987654321",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
			"runtimeSummary": map[string]any{
				"timestamp":        "2026-04-16T07:10:00Z",
				"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v2"},
				"conditions": []map[string]any{{
					"type":    "Ready",
					"status":  true,
					"reason":  "TrafficShiftComplete",
					"message": "green task set is now serving traffic",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample applications: %v", err)
	}

	refreshedApplicationsJSON, err := json.Marshal([]map[string]any{
		{
			"name":           "payments-api-prod",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"targetType":     "ec2",
			"strategy":       "canary",
			"executionMode":  "asgWeightedTargetGroups",
			"sync":           map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
			"health":         map[string]any{"overall": "Healthy", "summary": "3/3 targets healthy"},
			"runtimeSummary": map[string]any{"timestamp": "2026-04-16T06:40:00Z"},
		},
		{
			"name":           "payments-api-canary",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"accountName":    "staging-core",
			"awsAccountId":   "210987654321",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"sync":           map[string]any{"state": "OutOfSync", "reason": "traffic shifting"},
			"health":         map[string]any{"overall": "Progressing", "summary": "2/3 tasks healthy"},
			"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:20:00Z"},
		},
	})
	if err != nil {
		t.Fatalf("marshal refreshed sample applications: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "prod-core",
			"awsAccountId": "123456789012",
			"deployments": []map[string]any{{
				"name":           "payments-api-prod",
				"appRef":         "payments",
				"environmentRef": "prod-apne2",
				"accountName":    "prod-core",
				"awsAccountId":   "123456789012",
				"region":         "ap-northeast-2",
				"targetType":     "ec2",
				"strategy":       "canary",
				"executionMode":  "asgWeightedTargetGroups",
				"sync":           map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
				"health":         map[string]any{"overall": "Healthy", "summary": "3/3 targets healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T06:40:00Z"},
			}},
		},
		{
			"name":         "staging-core",
			"awsAccountId": "210987654321",
			"deployments": []map[string]any{{
				"name":           "payments-api-staging",
				"appRef":         "payments",
				"environmentRef": "staging-apne2",
				"accountName":    "staging-core",
				"awsAccountId":   "210987654321",
				"region":         "ap-northeast-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
				"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:10:00Z"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	applicationDetailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-staging",
		"appRef":           "payments",
		"environmentRef":   "staging-apne2",
		"region":           "ap-northeast-2",
		"accountName":      "staging-core",
		"awsAccountId":     "210987654321",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"desiredArtifact":  map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
		"sync":             map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":           map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
		"runtimeSummary": map[string]any{
			"timestamp":          "2026-04-16T07:15:00Z",
			"historyCount":       2,
			"observedGeneration": 7,
			"sync":               map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":             map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
			"deployedArtifact":   map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
		},
	})
	if err != nil {
		t.Fatalf("marshal application detail: %v", err)
	}

	accountDetailJSON, err := json.Marshal(map[string]any{
		"name":             "staging-core",
		"awsAccountId":     "210987654321",
		"allowedRegions":   []string{"ap-northeast-2", "us-west-2"},
		"deploymentCount":  2,
		"projectCount":     1,
		"environmentCount": 1,
		"progressingCount": 1,
		"syncedCount":      1,
		"deployments": []map[string]any{
			{
				"name":           "payments-api-staging",
				"appRef":         "payments",
				"environmentRef": "staging-apne2",
				"accountName":    "staging-core",
				"awsAccountId":   "210987654321",
				"region":         "ap-northeast-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
				"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:10:00Z"},
			},
			{
				"name":           "payments-worker-staging",
				"appRef":         "payments",
				"environmentRef": "staging-apne2",
				"accountName":    "staging-core",
				"awsAccountId":   "210987654321",
				"region":         "us-west-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "Synced", "reason": "steady"},
				"health":         map[string]any{"overall": "Healthy", "summary": "2/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:12:00Z"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal account detail: %v", err)
	}

	t.Run("applications overview entry page", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = ` + string(applicationsJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?q=payments&sync=OutOfSync&health=Progressing&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/applications') throw new Error('expected only overview applications fetch on applications page');
  if (document.title !== 'Applications overview · Tenet') throw new Error('expected applications page browser title, got ' + document.title);
  if (!toolbarText.includes('Search applications')) throw new Error('expected applications page toolbar');
  if (!toolbarText.includes('Health')) throw new Error('expected applications page health filter label');
  const healthSelect = document.querySelector('[data-toolbar-field="Health"]');
  if (!healthSelect || healthSelect.value !== 'Progressing') throw new Error('expected applications page health filter to restore bookmarkable Progressing state');
  if (!summaryText.includes('Application status overview')) throw new Error('expected applications page overview title');
  if (!summaryText.includes('Last update')) throw new Error('expected applications page last update column label');
  if (!summaryText.includes('2026-04-16T07:10:00Z')) throw new Error('expected applications page last update value');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected applications page row text');
  if (accountsText.includes('payments-api-prod')) throw new Error('expected health/sync/account filters to hide the prod row');
  if (accountsText.includes('Overview JSON')) throw new Error('applications page should not expose deployment read-model quick links');
  if (accountsText.includes('Status JSON')) throw new Error('applications page should not expose status quick links');
  if (accountsText.includes('Rollout execution')) throw new Error('applications page should not render deep detail sections');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const accountLink = links.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&health=Progressing&account=210987654321&accountId=210987654321');
  if (!accountLink) throw new Error('expected applications page link to account detail route');
  if (accountLink.target !== '_self') throw new Error('expected applications page account link to stay in embedded UI flow');
  if (accountLink.rel) throw new Error('expected applications page account link to avoid new-tab rel attributes');
  const appLink = links.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&health=Progressing&account=210987654321&deployment=payments-api-staging');
  if (!appLink) throw new Error('expected applications page link to application detail route');
  if (appLink.target !== '_self') throw new Error('expected applications page link to stay in embedded UI flow');
  if (appLink.rel) throw new Error('expected applications page link to avoid new-tab rel attributes');
  const driftLink = links.find((node) => node.href === '/?page=drift&q=payments&sync=OutOfSync&health=Progressing&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Drift view');
  if (!driftLink) throw new Error('expected applications page link to deployment-scoped drift view');
  if (driftLink.target !== '_self') throw new Error('expected applications page drift link to stay in embedded UI flow');
  if (driftLink.rel) throw new Error('expected applications page drift link to avoid new-tab rel attributes');
  const planLink = links.find((node) => node.href === '/?page=plan&q=payments&sync=OutOfSync&health=Progressing&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Plan/apply');
  if (!planLink) throw new Error('expected applications page link to deployment-scoped plan/apply workspace');
  if (planLink.target !== '_self') throw new Error('expected applications page plan link to stay in embedded UI flow');
  if (planLink.rel) throw new Error('expected applications page plan link to avoid new-tab rel attributes');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("applications overview explains hidden runtime search matches", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = [{
  name: 'payments-api-prod',
  appRef: 'payments',
  environmentRef: 'prod-apne2',
  accountName: 'prod-core',
  awsAccountId: '123456789012',
  region: 'ap-northeast-2',
  targetType: 'ec2',
  executionMode: 'asgWeightedTargetGroups',
  sync: { state: 'Synced', reason: 'latest desired generation applied' },
  health: { overall: 'Healthy', summary: '3/3 targets healthy' },
  runtimeSummary: {
    timestamp: '2026-04-16T06:40:00Z',
    changeDetails: ['managed drift added 2 rows'],
    conditions: [{ type: 'Ready', status: false, reason: 'DeploymentProgressing' }],
    deployedArtifact: { image: 'ghcr.io/example/payments@sha256:abcdef' },
    currentStep: 'ShiftTrafficToCandidate'
  },
}];
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(search) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  return collectText(document.getElementById('accounts')).join(' | ');
}
(async () => {
  const runtimeMatchText = await render('?q=managed+drift+added&sync=all&account=all');
  if (!runtimeMatchText.includes('payments-api-prod')) throw new Error('expected runtime-only search to keep matching row');
  if (!runtimeMatchText.includes('Search match')) throw new Error('expected visible search match context');
  if (!runtimeMatchText.includes('Matched runtime: managed drift added 2 rows')) throw new Error('expected runtime change-detail match context');
  const visibleMatchText = await render('?q=payments&sync=all&account=all');
  if (visibleMatchText.includes('Search match')) throw new Error('visible application-name search should keep overview row compact');
  const noSearchText = await render('?sync=all&account=all');
  if (noSearchText.includes('Search match')) throw new Error('no-search overview should stay compact');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("applications overview refresh keeps bookmarkable state and shared auth guidance", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = ` + string(applicationsJSON) + `;
const refreshedApplications = ` + string(refreshedApplicationsJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(fetchImpl) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    return fetchImpl(String(url));
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const overviewURL = '/v1/overview/applications';
  const initialSearch = '?q=payments&sync=OutOfSync&account=210987654321';

  let refreshFetchCount = 0;
  let resolveRefresh;
  const loadingView = await render((url) => {
    if (url !== overviewURL) throw new Error('unexpected fetch url: ' + url);
    refreshFetchCount += 1;
    if (refreshFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    if (refreshFetchCount === 2) return new Promise((resolve) => { resolveRefresh = resolve; });
    throw new Error('unexpected overview fetch count: ' + refreshFetchCount);
  });
  let toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  let refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected applications overview refresh button enabled after initial render');
  let accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  let summaryText = collectText(loadingView.document.getElementById('summary')).join(' | ');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected initial applications overview content before refresh');
  if (!summaryText.includes('Application status overview')) throw new Error('expected applications overview summary before refresh');
  refreshButton.dispatch('click');
  await settle();
  toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton || !refreshButton.disabled) throw new Error('expected applications overview refresh button disabled only while refresh is in flight');
  accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  summaryText = collectText(loadingView.document.getElementById('summary')).join(' | ');
  if (!summaryText.includes('Refreshing applications overview from GET /v1/overview/applications…')) throw new Error('expected applications overview refresh loading copy');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected current applications overview content to stay visible during refresh');
  if (loadingView.fetchCalls.length !== 2 || loadingView.fetchCalls[0] !== overviewURL || loadingView.fetchCalls[1] !== overviewURL) throw new Error('expected applications overview refresh to re-fetch only the overview route');
  if (loadingView.window.location.search !== initialSearch) throw new Error('expected applications overview refresh to preserve bookmarkable query state');
  resolveRefresh({ ok: true, json: () => Promise.resolve(refreshedApplications) });
  await settle();
  await settle();
  accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!accountsText.includes('payments-api-canary')) throw new Error('expected refreshed applications overview to render in place');
  if (accountsText.includes('payments-api-staging')) throw new Error('expected refreshed applications overview to replace stale row content');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected applications overview refresh button re-enabled after refresh completes');

  let authFetchCount = 0;
  const authView = await render((url) => {
    if (url !== overviewURL) throw new Error('unexpected auth-view fetch url: ' + url);
    authFetchCount += 1;
    if (authFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    if (authFetchCount === 2) return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: authenticated identity is required for this operator read route') });
    throw new Error('unexpected auth-view overview fetch count: ' + authFetchCount);
  });
  toolbarButtons = collectNodes(authView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton) throw new Error('expected applications overview refresh button in auth error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  accountsText = collectText(authView.document.getElementById('accounts')).join(' | ');
  summaryText = collectText(authView.document.getElementById('summary')).join(' | ');
  if (!summaryText.includes('Authenticated operator required')) throw new Error('expected applications overview refresh auth heading');
  if (!summaryText.includes('Tenet rejected GET /v1/overview/applications because authenticated identity is required on this operator read route.')) throw new Error('expected applications overview refresh trusted-front-door guidance');
  if (!summaryText.includes('Current applications overview remains visible below.')) throw new Error('expected applications overview refresh auth guidance to keep stale-content copy');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected applications overview content to remain visible after auth failure');
  if (summaryText.includes('Failed to refresh applications overview:')) throw new Error('expected applications overview auth refresh to avoid generic refresh failure copy');
  toolbarButtons = collectNodes(authView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected applications overview refresh button re-enabled after auth failure');
  if (authFetchCount !== 2) throw new Error('expected applications overview auth refresh to retry exactly once');
  if (authView.window.location.search !== initialSearch) throw new Error('expected applications overview auth refresh to preserve bookmarkable query state');

  let errorFetchCount = 0;
  const errorView = await render((url) => {
    if (url !== overviewURL) throw new Error('unexpected error-view fetch url: ' + url);
    errorFetchCount += 1;
    if (errorFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    if (errorFetchCount === 2) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected error-view overview fetch count: ' + errorFetchCount);
  });
  toolbarButtons = collectNodes(errorView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton) throw new Error('expected applications overview refresh button in generic error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  accountsText = collectText(errorView.document.getElementById('accounts')).join(' | ');
  summaryText = collectText(errorView.document.getElementById('summary')).join(' | ');
  if (!summaryText.includes('Failed to refresh applications overview: applications overview request failed (500): boom')) throw new Error('expected applications overview refresh error copy');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected current applications overview content to remain visible after refresh error');
  toolbarButtons = collectNodes(errorView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh applications overview');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected applications overview refresh button re-enabled after refresh error');
  if (errorFetchCount !== 2) throw new Error('expected applications overview refresh error to retry exactly once');
  if (errorView.window.location.search !== initialSearch) throw new Error('expected applications overview refresh error state to preserve bookmarkable query state');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("applications overview search matches runtime artifact and condition context", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = ` + string(applicationsJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
async function render(search) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return collectText(document.getElementById('accounts')).join(' | ');
}
(async () => {
  const artifactText = await render('?q=ghcr.io%2Fexample%2Fpayments%3Av2&sync=all&account=all');
  if (!artifactText.includes('payments-api-staging')) throw new Error('expected runtime artifact search to match the staging row');
  if (artifactText.includes('payments-api-prod')) throw new Error('expected runtime artifact search not to match rows without that artifact context');
  const conditionText = await render('?q=TrafficShiftComplete&sync=all&account=all');
  if (!conditionText.includes('payments-api-staging')) throw new Error('expected runtime condition search to match the staging row');
  if (conditionText.includes('payments-api-prod')) throw new Error('expected runtime condition search not to match rows without runtime condition context');
  const healthyText = await render('?health=Healthy&sync=all&account=all');
  if (!healthyText.includes('payments-api-prod')) throw new Error('expected health filter to match rows by displayed Healthy label');
  if (healthyText.includes('payments-api-staging')) throw new Error('expected health filter to hide rows with non-matching displayed health labels');
  const invalidHealthText = await render('?health=NeedsAttention&sync=all&account=all');
  if (!invalidHealthText.includes('payments-api-prod') || !invalidHealthText.includes('payments-api-staging')) throw new Error('expected invalid health filter value to normalize back to all');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("embedded drill-down links preserve non-root pathname", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = ` + string(applicationsJSON) + `;
const accountDetail = ` + string(accountDetailJSON) + `;
const applicationDetail = ` + string(applicationDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();

async function render(search, fetchImpl) {
  const document = createDocument();
  const window = { location: { pathname: '/console', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  await settle();
  await settle();
  return { document, window };
}

(async () => {
  const applicationsView = await render('?q=payments&sync=OutOfSync&account=210987654321', (url) => {
    if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleApplications) });
    throw new Error('unexpected applications fetch url: ' + url);
  });
  const overviewLinks = collectNodes(applicationsView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (!overviewLinks.find((node) => node.href === '/console?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321' && node.textContent === 'Open account' && node.target === '_self')) {
    throw new Error('expected applications overview account drill-down link to preserve /console pathname');
  }
  if (!overviewLinks.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Open application' && node.target === '_self')) {
    throw new Error('expected applications overview application drill-down link to preserve /console pathname');
  }
  if (!overviewLinks.find((node) => node.href === '/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Drift view' && node.target === '_self')) {
    throw new Error('expected applications overview drift handoff link to preserve /console pathname');
  }
  if (!overviewLinks.find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Audit log' && node.target === '_self')) {
    throw new Error('expected applications overview deployment-scoped audit link to preserve /console pathname');
  }

  const accountView = await render('?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321', (url) => {
    if (url === '/v1/overview/accounts/210987654321') return Promise.resolve({ ok: true, json: () => Promise.resolve(accountDetail) });
    throw new Error('unexpected account fetch url: ' + url);
  });
  const toolbarLinks = collectNodes(accountView.document.getElementById('toolbar'), (node) => node.tagName === 'A');
  if (!toolbarLinks.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Back to applications' && node.target === '_self')) {
    throw new Error('expected account detail back link to preserve /console pathname');
  }
  const accountLinks = collectNodes(accountView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (!accountLinks.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications overview' && node.target === '_self')) {
    throw new Error('expected account detail quick-nav overview link to preserve /console pathname');
  }
  if (!accountLinks.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Open application' && node.target === '_self')) {
    throw new Error('expected account detail drill-down link to preserve /console pathname');
  }

  const applicationView = await render('?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', (url) => {
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(applicationDetail) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
    throw new Error('unexpected application fetch url: ' + url);
  });
  const applicationLinks = collectNodes(applicationView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (!applicationLinks.find((node) => node.href === '/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Drift view' && node.target === '_self')) {
    throw new Error('expected application detail drift link to preserve /console pathname');
  }
  if (!applicationLinks.find((node) => node.href === '/console?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Plan/apply' && node.target === '_self')) {
    throw new Error('expected application detail plan link to preserve /console pathname');
  }
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("missing identifiers suppress broken self-links", func(t *testing.T) {
		brokenApplicationsJSON, err := json.Marshal([]map[string]any{
			{
				"name":           "payments-api-staging",
				"appRef":         "payments",
				"environmentRef": "staging-apne2",
				"accountName":    "staging-core",
				"awsAccountId":   "210987654321",
				"region":         "ap-northeast-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
				"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:10:00Z"},
			},
			{
				"name":           "missing-account-id",
				"appRef":         "payments",
				"environmentRef": "dev-apne2",
				"accountName":    "dev-core",
				"region":         "ap-northeast-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "Synced", "reason": "steady"},
				"health":         map[string]any{"overall": "Healthy", "summary": "2/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:11:00Z"},
			},
			{
				"appRef":         "payments",
				"environmentRef": "qa-apne2",
				"accountName":    "qa-core",
				"awsAccountId":   "999999999999",
				"region":         "ap-northeast-2",
				"targetType":     "ecs",
				"strategy":       "linear",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "Synced", "reason": "steady"},
				"health":         map[string]any{"overall": "Healthy", "summary": "2/2 tasks healthy"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:12:00Z"},
			},
		})
		if err != nil {
			t.Fatalf("marshal broken applications: %v", err)
		}

		brokenAccountDetailJSON, err := json.Marshal(map[string]any{
			"name":             "staging-core",
			"awsAccountId":     "210987654321",
			"allowedRegions":   []string{"ap-northeast-2"},
			"deploymentCount":  2,
			"projectCount":     2,
			"environmentCount": 2,
			"progressingCount": 1,
			"syncedCount":      1,
			"deployments": []map[string]any{
				{
					"name":           "payments-api-staging",
					"appRef":         "payments",
					"environmentRef": "staging-apne2",
					"accountName":    "staging-core",
					"awsAccountId":   "210987654321",
					"region":         "ap-northeast-2",
					"targetType":     "ecs",
					"strategy":       "linear",
					"executionMode":  "ecsCodeDeploy",
					"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
					"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
				},
				{
					"appRef":         "payments",
					"environmentRef": "qa-apne2",
					"accountName":    "staging-core",
					"awsAccountId":   "210987654321",
					"region":         "ap-northeast-2",
					"targetType":     "ecs",
					"strategy":       "linear",
					"executionMode":  "ecsCodeDeploy",
					"sync":           map[string]any{"state": "Synced", "reason": "steady"},
					"health":         map[string]any{"overall": "Healthy", "summary": "2/2 tasks healthy"},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal broken account detail: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const brokenApplications = ` + string(brokenApplicationsJSON) + `;
const brokenAccountDetail = ` + string(brokenAccountDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();

async function render(search, fetchImpl) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  await settle();
  await settle();
  return document;
}

(async () => {
  const applicationsDocument = await render('?q=payments', (url) => {
    if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve(brokenApplications) });
    throw new Error('unexpected applications fetch url: ' + url);
  });
  const overviewLinks = collectNodes(applicationsDocument.getElementById('accounts'), (node) => node.tagName === 'A');
  if (overviewLinks.find((node) => node.textContent === 'Open account' && node.href.includes('missing-account-id'))) {
    throw new Error('expected applications overview to suppress account drill-down without account id');
  }
  if (overviewLinks.find((node) => node.textContent === 'Open application' && node.href === '/?page=application&q=payments')) {
    throw new Error('expected applications overview to suppress application drill-down without deployment name');
  }

  const accountDocument = await render('?page=account&accountId=210987654321', (url) => {
    if (url === '/v1/overview/accounts/210987654321') return Promise.resolve({ ok: true, json: () => Promise.resolve(brokenAccountDetail) });
    throw new Error('unexpected account fetch url: ' + url);
  });
  const accountLinks = collectNodes(accountDocument.getElementById('accounts'), (node) => node.tagName === 'A');
  if (accountLinks.find((node) => node.textContent === 'Open application' && node.href === '/?page=application')) {
    throw new Error('expected account detail page to suppress application drill-down without deployment name');
  }
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("account detail route renders embedded overview page", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const accountDetail = ` + string(accountDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts/210987654321') return Promise.resolve({ ok: true, json: () => Promise.resolve(accountDetail) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/accounts/210987654321') throw new Error('expected only account overview detail fetch on account page');
  if (document.title !== '210987654321 · Account detail · Tenet') throw new Error('expected account page browser title, got ' + document.title);
  if (!toolbarText.includes('Back to applications')) throw new Error('expected account page back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink) throw new Error('expected account page back link to preserve filters');
  if (!summaryText.includes('Account deployment overview')) throw new Error('expected account page summary title');
  if (!summaryText.includes('staging-core')) throw new Error('expected account page account name');
  if (!accountsText.includes('Quick navigation')) throw new Error('expected account page quick navigation');
  if (!accountsText.includes('Refresh account detail')) throw new Error('expected account page refresh control');
  if (!accountsText.includes('Hierarchy')) throw new Error('expected account page hierarchy quick-nav group');
  if (!accountsText.includes('Applications overview')) throw new Error('expected account page overview quick-nav entry');
  if (!accountsText.includes('Current destination')) throw new Error('expected account page current-destination quick-nav group');
  if (!accountsText.includes('Current section')) throw new Error('expected account page current-section quick-nav group');
  if (!accountsText.includes('No section is currently targeted.')) throw new Error('expected account page empty current-section copy without hash');
  if (!accountsText.includes('On this page')) throw new Error('expected account page section quick-nav group');
  if (!accountsText.includes('staging-core · Account detail')) throw new Error('expected account page quick-nav current label');
  if (!accountsText.includes('payments · staging-apne2')) throw new Error('expected grouped project/environment heading');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected account page deployment row');
  if (!accountsText.includes('Account JSON')) throw new Error('expected account page account json link');
  if (!accountsText.includes('Group JSON')) throw new Error('expected account page group json link');
  if (!accountsText.includes('Open application')) throw new Error('expected account page application drill-down links');
  if (accountsText.includes('Drift view')) throw new Error('expected account page to stop exposing drift workspace shortcut');
  if (accountsText.includes('Plan/apply')) throw new Error('expected account page to stop exposing plan workspace shortcut');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const refreshButton = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Refresh account detail');
  const quickNavChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected enabled account refresh button after initial render');
  if (!links.find((node) => node.href === '/v1/overview/accounts/210987654321' && node.textContent === 'Account JSON')) throw new Error('expected account json href');
  if (!links.find((node) => node.href === '/v1/overview/accounts/210987654321/projects/payments/environments/staging-apne2' && node.textContent === 'Group JSON')) throw new Error('expected group json href');
  const applicationsOverviewLink = links.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications overview' && node.target === '_self');
  if (!applicationsOverviewLink || applicationsOverviewLink.rel) throw new Error('expected account page overview quick-nav self link');
  const accountDetailChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current destination: Account detail');
  if (!accountDetailChip) throw new Error('expected account page current-destination chip');
  if (quickNavChips.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '))) throw new Error('expected account page without hash not to render a current-section chip');
  if (links.find((node) => node.textContent === 'Account detail')) throw new Error('expected account page current destination to stay non-click');
  if (!links.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Open application' && node.target === '_self')) throw new Error('expected account page open application self link');
  if (!links.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321#account-summary' && node.textContent === 'Account summary' && node.target === '_self')) throw new Error('expected account page summary section link');
  if (!links.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321#account-group-payments-staging-apne2-ap-northeast-2' && node.textContent === 'payments · staging-apne2 · ap-northeast-2' && node.target === '_self')) throw new Error('expected account page ap-northeast-2 group section link');
  if (!links.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321#account-group-payments-staging-apne2-us-west-2' && node.textContent === 'payments · staging-apne2 · us-west-2' && node.target === '_self')) throw new Error('expected account page us-west-2 group section link');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("account detail refresh reloads the current route in place without changing bookmarkable state", func(t *testing.T) {
		refreshedAccountDetailJSON, err := json.Marshal(map[string]any{
			"name":             "staging-core",
			"awsAccountId":     "210987654321",
			"allowedRegions":   []string{"ap-northeast-2", "us-west-2"},
			"deploymentCount":  1,
			"projectCount":     1,
			"environmentCount": 1,
			"progressingCount": 1,
			"syncedCount":      0,
			"deployments": []map[string]any{
				{
					"name":           "payments-api-canary",
					"appRef":         "payments",
					"environmentRef": "staging-apne2",
					"accountName":    "staging-core",
					"awsAccountId":   "210987654321",
					"region":         "ap-northeast-2",
					"targetType":     "ecs",
					"strategy":       "canary",
					"executionMode":  "ecsCodeDeploy",
					"sync":           map[string]any{"state": "OutOfSync", "reason": "canary traffic progressing"},
					"health":         map[string]any{"overall": "Progressing", "summary": "1/2 target groups promoted"},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal refreshed account detail body: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const initialAccountDetail = ` + string(accountDetailJSON) + `;
const refreshedAccountDetail = ` + string(refreshedAccountDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(fetchImpl) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    return fetchImpl(String(url));
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const accountURL = '/v1/overview/accounts/210987654321';
  const initialSearch = '?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321';
  let refreshFetchCount = 0;
  let resolveRefresh;
  const loadingView = await render((url) => {
    if (url !== accountURL) throw new Error('unexpected fetch url: ' + url);
    refreshFetchCount += 1;
    if (refreshFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialAccountDetail) });
    if (refreshFetchCount === 2) return new Promise((resolve) => { resolveRefresh = resolve; });
    throw new Error('unexpected account fetch count: ' + refreshFetchCount);
  });
  let buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  let refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected account refresh button enabled after initial render');
  let text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('payments-api-staging')) throw new Error('expected initial account deployment before refresh');
  refreshButton.dispatch('click');
  await settle();
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton || !refreshButton.disabled) throw new Error('expected account refresh button disabled only while refresh is in flight');
  text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('Refreshing account detail from GET /v1/overview/accounts/{awsAccountId}…')) throw new Error('expected account refresh loading copy');
  if (!text.includes('payments-api-staging')) throw new Error('expected current account content to stay visible during refresh');
  if (loadingView.fetchCalls.length !== 2 || loadingView.fetchCalls[0] !== accountURL || loadingView.fetchCalls[1] !== accountURL) throw new Error('expected account refresh to re-fetch only the account detail route');
  if (loadingView.window.location.search !== initialSearch) throw new Error('expected account refresh to preserve bookmarkable route');
  resolveRefresh({ ok: true, json: () => Promise.resolve(refreshedAccountDetail) });
  await settle();
  await settle();
  text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('payments-api-canary')) throw new Error('expected refreshed account detail to render in place');
  if (text.includes('payments-api-staging')) throw new Error('expected refreshed account detail to replace stale deployment content');
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected account refresh button re-enabled after refresh completes');

  let authFetchCount = 0;
  const authView = await render((url) => {
    if (url !== accountURL) throw new Error('unexpected auth-view fetch url: ' + url);
    authFetchCount += 1;
    if (authFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialAccountDetail) });
    if (authFetchCount === 2) return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: authenticated identity is required for this operator read route') });
    throw new Error('unexpected auth-view account fetch count: ' + authFetchCount);
  });
  buttons = collectNodes(authView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton) throw new Error('expected account refresh button in auth error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  text = collectText(authView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('Authenticated operator required')) throw new Error('expected account refresh auth heading');
  if (!text.includes('Tenet rejected GET /v1/overview/accounts/210987654321 because authenticated identity is required on this operator read route.')) throw new Error('expected account refresh trusted-front-door guidance');
  if (!text.includes('Current account detail remains visible below.')) throw new Error('expected account refresh auth guidance to keep stale-content copy');
  if (!text.includes('payments-api-staging')) throw new Error('expected account content to remain visible after auth failure');
  if (text.includes('Failed to refresh account detail:')) throw new Error('expected account auth refresh to avoid generic refresh failure copy');
  buttons = collectNodes(authView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected account refresh button re-enabled after auth failure');
  if (authFetchCount !== 2) throw new Error('expected account auth refresh to retry exactly once');
  if (authView.window.location.search !== initialSearch) throw new Error('expected account auth refresh to preserve bookmarkable route');

  let errorFetchCount = 0;
  const errorView = await render((url) => {
    if (url !== accountURL) throw new Error('unexpected error-view fetch url: ' + url);
    errorFetchCount += 1;
    if (errorFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialAccountDetail) });
    if (errorFetchCount === 2) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected error-view account fetch count: ' + errorFetchCount);
  });
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton) throw new Error('expected account refresh button in generic error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  text = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('Failed to refresh account detail: account detail request failed (500): boom')) throw new Error('expected account refresh error copy');
  if (!text.includes('payments-api-staging')) throw new Error('expected current account content to remain visible after refresh error');
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh account detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected account refresh button re-enabled after refresh error');
  if (errorFetchCount !== 2) throw new Error('expected account refresh error to retry exactly once');
  if (errorView.window.location.search !== initialSearch) throw new Error('expected account refresh error state to preserve bookmarkable route');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("account detail route keeps hash-aware section navigation in shell", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const accountDetail = ` + string(accountDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const createWindow = (initialSearch, hash) => {
  const listeners = {};
  return {
    location: { pathname: '/console', search: initialSearch, hash },
    addEventListener(type, handler) {
      listeners[type] = listeners[type] || [];
      listeners[type].push(handler);
    },
    dispatchEvent(event) {
      const handlers = listeners[event?.type] || [];
      handlers.forEach((handler) => handler.call(this, event));
    },
    dispatchHashChange() {
      this.dispatchEvent({ type: 'hashchange', target: this });
    },
  };
};
const collectNodes = (root, predicate) => {
  const matches = [];
  const visit = (node) => {
    if (!node) return;
    if (predicate(node)) matches.push(node);
    (node.children || []).forEach(visit);
  };
  visit(root);
  return matches;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const initialSearch = '?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321';
const window = createWindow(initialSearch, '#account-group-payments-staging-apne2-ap-northeast-2');
const history = { replaceState(_state, _title, url) {
  const [pathAndQuery, hash = ''] = String(url).split('#');
  const [pathname, query = ''] = pathAndQuery.split('?');
  window.location.pathname = pathname || '/console';
  window.location.search = query ? '?' + query : '';
  window.location.hash = hash ? '#' + hash : '';
} };
const fetchCalls = [];
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (String(url) === '/v1/overview/accounts/210987654321') return Promise.resolve({ ok: true, json: () => Promise.resolve(accountDetail) });
  throw new Error('unexpected fetch url: ' + url);
};
(async () => {
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/accounts/210987654321') throw new Error('expected account hash route to load only account detail');
  if (window.location.pathname !== '/console') throw new Error('expected account pathname to stay stable, got ' + window.location.pathname);
  if (window.location.search !== initialSearch) throw new Error('expected account query to stay stable, got ' + window.location.search);
  if (window.location.hash !== '#account-group-payments-staging-apne2-ap-northeast-2') throw new Error('expected account hash to stay stable, got ' + window.location.hash);
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'account-group-payments-staging-apne2-ap-northeast-2') throw new Error('expected account hash to scroll the matching section into view');
  const accounts = document.getElementById('accounts');
  const quickNavChips = collectNodes(accounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const currentSectionChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current section: payments · staging-apne2 · ap-northeast-2');
  if (!currentSectionChip) throw new Error('expected account hash to render the current-section chip');
  const sectionLinks = collectNodes(accounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#account-'));
  const summaryLink = sectionLinks.find((node) => node.href === '/console?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321#account-summary' && node.textContent === 'Account summary');
  if (!summaryLink) throw new Error('expected account hash route to keep account summary in On this page links');
  if (summaryLink.target !== '_self' || summaryLink.rel) throw new Error('expected account summary section link to stay in-shell');
  if (sectionLinks.find((node) => node.textContent === 'payments · staging-apne2 · ap-northeast-2')) throw new Error('expected active account section to be removed from On this page links');
  if (!sectionLinks.find((node) => node.textContent === 'payments · staging-apne2 · us-west-2')) throw new Error('expected sibling account section to remain visible in On this page links');
  window.location.hash = '#account-summary';
  window.dispatchHashChange();
  await settle();
  const updatedAccounts = document.getElementById('accounts');
  const updatedChips = collectNodes(updatedAccounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const summaryChip = updatedChips.find((node) => node.getAttribute('aria-label') === 'Current section: Account summary');
  if (!summaryChip) throw new Error('expected account hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedAccounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#account-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Account summary')) throw new Error('expected updated account current section to be removed from On this page links');
  if (!updatedSectionLinks.find((node) => node.textContent === 'payments · staging-apne2 · ap-northeast-2')) throw new Error('expected previous account current section to return to On this page links after hashchange');
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'account-summary') throw new Error('expected account hashchange to scroll the new current section into view');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("application detail route uses application detail fetch", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleApplications = ` + string(applicationsJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(applicationDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  const expectedFetches = [
    '/v1/overview/applications/payments-api-staging',
    '/v1/overview/deployments/payments-api-staging/events',
  ];
  if (fetchCalls.length !== expectedFetches.length || expectedFetches.some((url, index) => fetchCalls[index] !== url)) {
    throw new Error('expected application route to load application detail plus deployment events, got ' + fetchCalls.join(', '));
  }
  if (document.title !== 'payments-api-staging · Application detail · Tenet') throw new Error('expected application page browser title, got ' + document.title);
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected application route back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const accountLinks = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const quickNavChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const breadcrumbChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className.includes('breadcrumb-current'));
  const backLink = toolbarLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink) throw new Error('expected application route self-navigation back link');
  if (!accountsText.includes('Breadcrumbs')) throw new Error('expected application route breadcrumb chrome');
  const applicationsBreadcrumb = accountLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications' && node.target === '_self');
  if (!applicationsBreadcrumb || applicationsBreadcrumb.rel) throw new Error('expected application route applications breadcrumb link');
  const accountBreadcrumb = accountLinks.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321' && node.textContent === 'staging-core (210987654321)' && node.target === '_self');
  if (!accountBreadcrumb || accountBreadcrumb.rel) throw new Error('expected application route account breadcrumb link');
  if (!breadcrumbChips.find((node) => node.textContent === 'payments-api-staging')) throw new Error('expected application route current breadcrumb chip');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected application route summary title');
  if (!summaryText.includes('Application detail page: show only application-linked resources before the final resource-detail page.')) throw new Error('expected application route drill-down guidance');
  if (!accountsText.includes('Application resources')) throw new Error('expected application route resources body');
  if (!accountsText.includes('Quick navigation')) throw new Error('expected application route quick navigation');
  if (!accountsText.includes('Hierarchy')) throw new Error('expected application route hierarchy quick-nav group');
  if (!accountsText.includes('Applications overview')) throw new Error('expected application route overview quick-nav entry');
  if (!accountsText.includes('Current destination')) throw new Error('expected application route current-destination quick-nav group');
  if (!accountsText.includes('Deployment workspaces')) throw new Error('expected application route deployment-workspaces quick-nav group');
  if (!accountsText.includes('Current section')) throw new Error('expected application route current-section quick-nav group');
  if (!accountsText.includes('No section is currently targeted.')) throw new Error('expected application route empty current-section message without hash');
  if (!accountsText.includes('On this page')) throw new Error('expected application route section quick-nav group');
  if (!accountsText.includes('payments-api-staging · Application detail')) throw new Error('expected application route quick-nav current label');
  const applicationsOverviewLink = accountLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications overview' && node.target === '_self');
  if (!applicationsOverviewLink || applicationsOverviewLink.rel) throw new Error('expected application route overview quick-nav self link');
  const applicationDetailChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current destination: Application detail');
  if (!applicationDetailChip) throw new Error('expected application route current-destination chip');
  if (accountLinks.find((node) => node.textContent === 'Application detail')) throw new Error('expected application route current destination to stay non-click');
  const currentSectionChip = quickNavChips.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '));
  if (currentSectionChip) throw new Error('expected application route without hash not to render a current-section chip');
  if (!accountsText.includes('Drift view')) throw new Error('expected application route drift workspace entry');
  if (!accountsText.includes('Plan/apply')) throw new Error('expected application route plan workspace entry');
  if (!accountsText.includes('Workloads')) throw new Error('expected application route workloads group');
  if (!accountsText.includes('Traffic routing')) throw new Error('expected application route traffic routing group');
  if (!accountsText.includes('Controller runtime events')) throw new Error('expected application route runtime-events section');
  if (!accountsText.includes('payments-api-staging-service')) throw new Error('expected application route synthesized workload resource');
  if (!accountsText.includes('payments-api-staging-codedeploy-group')) throw new Error('expected application route synthesized deployment-group resource');
  const sectionLinks = accountLinks.filter((node) => typeof node.href === 'string' && node.href.includes('#application-'));
  const expectedSectionLinks = [
    ['/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging#application-workloads', 'Workloads'],
    ['/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging#application-traffic-routing', 'Traffic routing'],
    ['/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging#application-runtime-events', 'Controller runtime events'],
  ];
  if (sectionLinks.length !== expectedSectionLinks.length) throw new Error('expected three application section links, got ' + sectionLinks.length);
  expectedSectionLinks.forEach(([href, text]) => {
    const link = sectionLinks.find((node) => node.href === href && node.textContent === text);
    if (!link) throw new Error('missing application section link ' + text + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected application section link to stay in-shell for ' + text);
  });
  const sectionIDs = collectNodes(document.getElementById('accounts'), (node) => typeof node.id === 'string' && node.id.startsWith('application-')).map((node) => node.id).sort();
  const expectedSectionIDs = ['application-runtime-events', 'application-traffic-routing', 'application-workloads'];
  if (JSON.stringify(sectionIDs) !== JSON.stringify(expectedSectionIDs)) throw new Error('unexpected application section ids: ' + JSON.stringify(sectionIDs));
  if (accountsText.includes('Create Plan')) throw new Error('application route should not render plan workspace actions');
  if (accountsText.includes('Field-level drift')) throw new Error('application route should not render drift workspace sections');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("bookmarked application hash deep links restore section targets after render", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(applicationDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.id = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) current.add(name); else current.delete(name);
        this.className = Array.from(current).join(' ');
      },
    };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const createWindow = (initialSearch, hash) => {
  const listeners = {};
  return {
    location: { pathname: '/console', search: initialSearch, hash },
    addEventListener(type, handler) {
      listeners[type] = listeners[type] || [];
      listeners[type].push(handler);
    },
    dispatchEvent(event) {
      const handlers = listeners[event?.type] || [];
      handlers.forEach((handler) => handler.call(this, event));
    },
    dispatchHashChange() {
      this.dispatchEvent({ type: 'hashchange', target: this });
    },
  };
};
const collectNodes = (root, predicate) => {
  const matches = [];
  const visit = (node) => {
    if (!node) return;
    if (predicate(node)) matches.push(node);
    (node.children || []).forEach(visit);
  };
  visit(root);
  return matches;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const initialSearch = '?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging';
const window = createWindow(initialSearch, '#application-runtime-events');
const history = { replaceState(_state, _title, url) {
  const [pathAndQuery, hash = ''] = String(url).split('#');
  const [pathname, query = ''] = pathAndQuery.split('?');
  window.location.pathname = pathname || '/console';
  window.location.search = query ? '?' + query : '';
  window.location.hash = hash ? '#' + hash : '';
} };
const fetchCalls = [];
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (String(url) === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (String(url) === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
(async () => {
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/applications/payments-api-staging, /v1/overview/deployments/payments-api-staging/events') throw new Error('expected application hash route to load application detail plus deployment events');
  if (window.location.pathname !== '/console') throw new Error('expected application pathname to stay stable, got ' + window.location.pathname);
  if (window.location.search !== initialSearch) throw new Error('expected application query to stay stable, got ' + window.location.search);
  if (window.location.hash !== '#application-runtime-events') throw new Error('expected application hash to stay stable, got ' + window.location.hash);
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'application-runtime-events') throw new Error('expected application hash to scroll the matching section into view');
  const accounts = document.getElementById('accounts');
  const quickNavChips = collectNodes(accounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const currentDestinationChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current destination: Application detail');
  if (!currentDestinationChip) throw new Error('expected application hash route to keep current-destination chip');
  const currentSectionChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current section: Controller runtime events');
  if (!currentSectionChip) throw new Error('expected application hash to render the current-section chip');
  const sectionLinks = collectNodes(accounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#application-'));
  const expectedLinks = [
    ['/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging#application-workloads', 'Workloads'],
    ['/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging#application-traffic-routing', 'Traffic routing'],
  ];
  if (sectionLinks.length !== expectedLinks.length) throw new Error('expected two remaining application section links, got ' + sectionLinks.length);
  expectedLinks.forEach(([href, text]) => {
    const link = sectionLinks.find((node) => node.href === href && node.textContent === text);
    if (!link) throw new Error('missing application hash link ' + text + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected application hash link to stay in-shell for ' + text);
  });
  if (sectionLinks.find((node) => node.textContent === 'Controller runtime events')) throw new Error('expected active application section to be removed from On this page links');
  window.location.hash = '#application-traffic-routing';
  window.dispatchHashChange();
  await settle();
  const updatedAccounts = document.getElementById('accounts');
  const updatedChips = collectNodes(updatedAccounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const trafficChip = updatedChips.find((node) => node.getAttribute('aria-label') === 'Current section: Traffic routing');
  if (!trafficChip) throw new Error('expected application hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedAccounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#application-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Traffic routing')) throw new Error('expected updated application current section to be removed from On this page links');
  if (!updatedSectionLinks.find((node) => node.textContent === 'Controller runtime events')) throw new Error('expected previous application current section to return to On this page links after hashchange');
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'application-traffic-routing') throw new Error('expected application hashchange to scroll the new current section into view');
})();
`

		runNodeHarness(t, nodeProgram)
	})
}

func TestEmbeddedUIApplicationDetailSynthesizesECSRollingFallbackResource(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "prod-core",
		"awsAccountId": "123456789012",
		"deployments": []map[string]any{{
			"name":           "payments-api-rolling",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "rolling",
			"executionMode":  "ecsRolling",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":           "payments-api-rolling",
		"accountName":    "prod-core",
		"awsAccountId":   "123456789012",
		"region":         "ap-northeast-2",
		"appRef":         "payments",
		"environmentRef": "prod-apne2",
		"targetType":     "ecs",
		"strategy":       "rolling",
		"executionMode":  "ecsRolling",
		"rolloutConfig":  map[string]any{"strategy": map[string]any{"minHealthyPercent": 90, "warmupSeconds": 30}},
		"sync":           map[string]any{"state": "OutOfSync", "reason": "rolling deployment in progress"},
		"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks replaced"},
		"rollout":        map[string]any{"phase": "Progressing", "currentStep": "UpdateECSService"},
	})
	if err != nil {
		t.Fatalf("marshal ecs rolling detail: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-rolling&q=payments&sync=OutOfSync&account=123456789012', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/applications/payments-api-rolling') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-rolling/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-rolling', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-rolling' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-rolling/events') throw new Error('expected ecs rolling application page to load application detail plus deployment events');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('payments-api-rolling-service')) throw new Error('expected ecs rolling fallback service resource');
  if (!accountsText.includes('ECS service Tenet updates with native rolling deployment while respecting min healthy 90% and warmup 30s')) throw new Error('expected ecs rolling fallback resource description');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const resourceId = 'ecsService::payments-api-rolling-service::ECS service Tenet updates with native rolling deployment while respecting min healthy 90% and warmup 30s';
  const href = '/?' + new URLSearchParams({
    page: 'resource',
    q: 'payments',
    sync: 'OutOfSync',
    account: '123456789012',
    deployment: 'payments-api-rolling',
    resourceType: 'ecsService',
    resourceId,
  }).toString();
  if (!links.find((node) => node.href === href && node.target === '_self' && !node.rel)) throw new Error('expected ecs rolling fallback stable link for ' + resourceId);
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestApplicationsOverviewPageWithNodeHarnessSynthesizesRolloutFallbackResources(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "prod-core",
		"awsAccountId": "123456789012",
		"deployments": []map[string]any{{
			"name":           "payments-api-prod",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ec2",
			"strategy":       "canary",
			"executionMode":  "asgWeightedTargetGroups",
		}, {
			"name":           "payments-api-staging",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	t.Run("ecs codedeploy fallback resources stay bookmarkable", func(t *testing.T) {
		detailJSON, err := json.Marshal(map[string]any{
			"name":           "payments-api-staging",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"rolloutConfig":  map[string]any{"strategy": map[string]any{"minHealthyPercent": 80, "warmupSeconds": 45}},
			"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
		})
		if err != nil {
			t.Fatalf("marshal ecs fallback detail: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-staging' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging/events') throw new Error('expected ecs fallback page to load application detail plus deployment events');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-staging-codedeploy-app', 'codeDeployApplication', 'CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
    ['payments-api-staging-codedeploy-group', 'codeDeployDeploymentGroup', 'CodeDeploy deployment group Tenet targets for ECS rollout (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
    ['payments-api-staging-prod-listener', 'productionListener', 'Production listener CodeDeploy shifts during ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
    ['payments-api-staging-test-listener', 'testListener', 'Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
    ['payments-api-staging-blue', 'blueTaskSetTargetGroup', 'Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
    ['payments-api-staging-green', 'greenTaskSetTargetGroup', 'Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear; qualifiers: min healthy 80% and warmup 45s)'],
  ];
  for (const [name, type, description] of expectedResources) {
    if (!accountsText.includes(name)) throw new Error('expected ecs fallback resource ' + name);
    const resourceId = type + '::' + name + '::' + description;
    const href = '/?' + new URLSearchParams({
      page: 'resource',
      q: 'payments',
      sync: 'OutOfSync',
      account: '210987654321',
      deployment: 'payments-api-staging',
      resourceType: type,
      resourceId,
    }).toString();
    if (!links.find((node) => node.href === href && node.target === '_self' && !node.rel)) throw new Error('expected ecs fallback stable link for ' + resourceId);
  }
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("ec2 weighted-target-group fallback resources stay bookmarkable", func(t *testing.T) {
		detailJSON, err := json.Marshal(map[string]any{
			"name":           "payments-api-prod",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"targetType":     "ec2",
			"strategy":       "canary",
			"executionMode":  "asgWeightedTargetGroups",
			"rolloutConfig":  map[string]any{"capacity": map[string]any{"desired": 4, "min": 2, "max": 8}, "strategy": map[string]any{"warmupSeconds": 90}},
			"userData":       map[string]any{"type": "file", "path": "userdata/payments-api-prod.sh"},
			"sync":           map[string]any{"state": "OutOfSync", "reason": "weighted rollout in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "candidate fleet bootstrapping"},
			"rollout":        map[string]any{"phase": "Progressing", "currentStep": "ShiftTrafficCanary"},
		})
		if err != nil {
			t.Fatalf("marshal asg fallback detail: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/applications/payments-api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-prod/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-prod', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-prod' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-prod/events') throw new Error('expected asg fallback page to load application detail plus deployment events');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-prod-candidate', 'candidateAutoScalingGroup', 'Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/payments-api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout'],
    ['payments-api-prod-active', 'activeTargetGroup', 'Currently active target group receiving production traffic'],
    ['payments-api-prod-candidate', 'candidateTargetGroup', 'Candidate target group Tenet will register and warm before traffic shifting'],
    ['payments-api-prod-prod-listener', 'productionListenerRule', 'Production listener rule operators inspect while Tenet updates weighted forwarding during rollout'],
    ['payments-api-prod-weighted-routing', 'trafficShiftPlan', 'Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s'],
    ['file:userdata/payments-api-prod.sh', 'userDataBootstrapSource', 'Normalized userdata bootstrap source for candidate EC2 capacity'],
  ];
  for (const [name, type, description] of expectedResources) {
    if (!accountsText.includes(name)) throw new Error('expected asg fallback resource ' + name);
    const resourceId = type + '::' + name + '::' + description;
    const href = '/?' + new URLSearchParams({
      page: 'resource',
      deployment: 'payments-api-prod',
      resourceType: type,
      resourceId,
    }).toString();
    if (!links.find((node) => node.href === href && node.target === '_self' && !node.rel)) throw new Error('expected asg fallback stable link for ' + resourceId);
  }
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("ec2 instance-refresh fallback resources stay bookmarkable", func(t *testing.T) {
		detailJSON, err := json.Marshal(map[string]any{
			"name":           "payments-api-refresh",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"targetType":     "ec2",
			"strategy":       "rolling",
			"executionMode":  "asgInstanceRefresh",
			"rolloutConfig":  map[string]any{"stacks": []map[string]any{{"rollingUpdateInstanceCount": 2}}},
			"userData":       map[string]any{"type": "file", "path": "userdata/payments-api-refresh.sh"},
			"sync":           map[string]any{"state": "OutOfSync", "reason": "instance refresh in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "instances replacing in service auto scaling group"},
			"rollout":        map[string]any{"phase": "Progressing", "currentStep": "StartInstanceRefresh"},
		})
		if err != nil {
			t.Fatalf("marshal instance refresh fallback detail: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-refresh', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/applications/payments-api-refresh') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-refresh/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-refresh', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-refresh' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-refresh/events') throw new Error('expected instance refresh fallback page to load application detail plus deployment events');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-refresh-service', 'serviceAutoScalingGroup', 'Service Auto Scaling Group Tenet refreshes in place with rolling-update batch size 2'],
    ['file:userdata/payments-api-refresh.sh', 'userDataBootstrapSource', 'Normalized userdata bootstrap source for refreshed EC2 instances'],
  ];
  for (const [name, type, description] of expectedResources) {
    if (!accountsText.includes(name)) throw new Error('expected instance refresh fallback resource ' + name);
    const resourceId = type + '::' + name + '::' + description;
    const href = '/?' + new URLSearchParams({
      page: 'resource',
      deployment: 'payments-api-refresh',
      resourceType: type,
      resourceId,
    }).toString();
    if (!links.find((node) => node.href === href && node.target === '_self' && !node.rel)) throw new Error('expected instance refresh fallback stable link for ' + resourceId);
  }
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("resource detail renders synthesized fallback execution metadata", func(t *testing.T) {
		detailJSON, err := json.Marshal(map[string]any{
			"name":           "payments-api-staging",
			"accountName":    "prod-core",
			"awsAccountId":   "123456789012",
			"region":         "ap-northeast-2",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"rolloutConfig":  map[string]any{"strategy": map[string]any{"minHealthyPercent": 80, "warmupSeconds": 45}},
			"sync":           map[string]any{"state": "OutOfSync", "reason": "candidate tasks warming"},
			"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while CodeDeploy warms the green task set"},
			"rollout": map[string]any{
				"phase":       "Provisioning",
				"currentStep": "CreateCodeDeployDeployment",
				"execution":   []map[string]any{},
			},
		})
		if err != nil {
			t.Fatalf("marshal resource fallback detail: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const resourceId = 'testListener::payments-api-staging-test-listener::Test listener CodeDeploy uses to validate the replacement ECS task set before promotion';
(async () => {
  const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=testListener&resourceId=' + encodeURIComponent(resourceId), hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetchCalls = [];
  const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=testListener&resourceName=payments-api-staging-test-listener';
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'codedeploy', backingResourceType: 'codeDeployDeploymentGroup', backingResourceName: 'payments-api-staging-codedeploy-group', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true }, entries: [{ timestamp: '2026-04-18T09:18:00Z', stream: 'd-newest', message: 'Status: Succeeded — deployment completed for the latest linear rollout' }] }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  if (!fetchCalls.includes('/v1/overview/deployments/payments-api-staging')) throw new Error('expected fallback resource page to load deployment detail');
  if (!fetchCalls.includes(logsURL)) throw new Error('expected fallback resource page to load recent logs for the synthesized resource');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!summaryText.includes('payments-api-staging-test-listener')) throw new Error('expected fallback resource page to resolve the synthesized resource name');
  if (!accountsText.includes('Execution resource · testListener')) throw new Error('expected fallback resource page to render synthesized execution resource type');
  if (!accountsText.includes('Execution description · Test listener CodeDeploy uses to validate the replacement ECS task set before promotion')) throw new Error('expected fallback resource page to render synthesized execution description');
  if (accountsText.includes('Recent logs are unavailable for this resource.')) throw new Error('expected fallback resource page not to render unsupported recent-log state');
  if (accountsText.includes('No recent logs are available for this resource.')) throw new Error('expected fallback resource page not to render empty recent-log state for populated CodeDeploy fallback logs');
  if (!accountsText.includes('Resolved backing resource')) throw new Error('expected recent logs to render resolved backing-resource handoff');
  if (!accountsText.includes('Open payments-api-staging-codedeploy-group drill-down')) throw new Error('expected recent logs backing-resource handoff label');
  const backingLinks = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open payments-api-staging-codedeploy-group drill-down');
  const backingLink = backingLinks.find((node) => node.target === '_self');
  if (!backingLink) throw new Error('expected recent logs backing-resource handoff to use _self navigation');
  const backingQuery = new URLSearchParams(String(backingLink.href).split('?')[1] || '');
  if (backingQuery.get('page') !== 'resource') throw new Error('expected backing-resource handoff to route to resource page');
  if (backingQuery.get('deployment') !== 'payments-api-staging') throw new Error('expected backing-resource handoff to preserve deployment context');
  if (backingQuery.get('resourceType') !== 'codeDeployDeploymentGroup') throw new Error('expected backing-resource handoff to target the backing deployment-group type');
  const backingResourceId = backingQuery.get('resourceId') || '';
  if (!backingResourceId.startsWith('codeDeployDeploymentGroup::payments-api-staging-codedeploy-group::')) throw new Error('expected backing-resource handoff to use stable backing resource identity');
  if (accountsText.includes('No rollout execution resources are currently reported for this selection.')) throw new Error('expected fallback resource page not to render empty execution state');
})();
`

		runNodeHarness(t, nodeProgram)
	})
}

func TestApplicationDetailPageWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":                "payments-api-staging",
		"accountName":         "staging-core",
		"awsAccountId":        "210987654321",
		"region":              "ap-northeast-2",
		"appRef":              "payments",
		"environmentRef":      "staging-apne2",
		"targetType":          "ecs",
		"strategy":            "linear",
		"executionMode":       "ecsCodeDeploy",
		"desiredArtifact":     map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
		"deployedArtifact":    map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
		"sync":                map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":              map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
		"managedDriftSummary": map[string]any{"total": 3, "high": 1, "medium": 1, "low": 1},
		"runtimeSummary": map[string]any{
			"timestamp":    "2026-04-16T07:15:00Z",
			"historyCount": 2,
			"sync":         map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":       map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
		},
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging ECS Service", "resource": "ecsService", "description": "Primary ECS service for the staging environment"},
				{"name": "payments-api-staging CodeDeploy App", "resource": "codeDeployApplication", "description": "CodeDeploy application coordinating ECS traffic shifting"},
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				{"name": "prod listener", "resource": "productionListener", "description": "Routes production traffic"},
				{"name": "green task set target group", "resource": "greenTaskSetTargetGroup", "description": "Candidate task set traffic target"},
				{"name": "bootstrap source", "resource": "userDataBootstrapSource", "description": "Bootstrap source placeholder"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-staging' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging/events') throw new Error('expected application page to load application detail plus deployment events');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected application page back link');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected application summary title');
  if (!accountsText.includes('Application resources')) throw new Error('expected application resources heading');
  if (!accountsText.includes('Current application status')) throw new Error('expected compact application status summary');
  if (!accountsText.includes('Target ecs · Strategy linear · Execution ecsCodeDeploy')) throw new Error('expected application status summary rollout context');
  if (!accountsText.includes('ghcr.io/devopsartfactory/payments-api:staging-20260415')) throw new Error('expected application status summary artifact');
  if (!accountsText.includes('Managed drift')) throw new Error('expected application status summary managed drift metric');
  if (!accountsText.includes('3 total · 1 high · 1 medium · 1 low')) throw new Error('expected application status summary to use server-authored managed drift totals');
  if (!accountsText.includes('2 runtime events')) throw new Error('expected application status summary runtime event count');
  if (!accountsText.includes('2026-04-16T07:15:00Z')) throw new Error('expected application status summary last update');
  if (!accountsText.includes('Workloads')) throw new Error('expected workloads group');
  if (!accountsText.includes('Traffic routing')) throw new Error('expected traffic routing group');
  if (!accountsText.includes('Config and access')) throw new Error('expected config and access group');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected workload resource');
  if (!accountsText.includes('prod listener')) throw new Error('expected traffic routing resource');
  if (!accountsText.includes('bootstrap source')) throw new Error('expected config/access resource');
  if (!accountsText.includes('Drift view')) throw new Error('expected application page drift workspace link');
  if (!accountsText.includes('Plan/apply')) throw new Error('expected application page plan workspace link');
  if (accountsText.includes('Runtime history')) throw new Error('application page should not expose deep runtime history yet');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const resourceLink = links.find((node) => node.textContent === 'Open resource');
  const liveInspectionLink = links.find((node) => node.textContent === 'Open live inspection');
  const recentLogsLink = links.find((node) => node.textContent === 'Open recent logs');
  const driftLink = links.find((node) => node.textContent === 'Drift view');
  const planLink = links.find((node) => node.textContent === 'Plan/apply');
  const auditLink = links.find((node) => node.textContent === 'Audit log');
  if (!resourceLink) throw new Error('expected resource drill-down link');
  if (!liveInspectionLink) throw new Error('expected direct live inspection handoff link');
  if (!recentLogsLink) throw new Error('expected direct recent logs handoff link');
  if (!driftLink || !driftLink.href.includes('/?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected drift workspace href to include deployment and preserved filters');
  if (!planLink || !planLink.href.includes('/?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected plan workspace href to include deployment and preserved filters');
  if (!auditLink || !auditLink.href.includes('/?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected audit workspace href to include deployment and preserved filters');
  if (driftLink.target !== '_self' || planLink.target !== '_self') throw new Error('expected workspace drill-down links to stay in embedded UI flow');
  if (auditLink.target !== '_self') throw new Error('expected audit workspace link to stay in embedded UI flow');
  if (!resourceLink.href.includes('/?page=resource&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected resource detail href to include deployment and preserved filters');
  if (resourceLink.target !== '_self') throw new Error('expected resource drill-down link to stay in embedded UI flow');
  if (resourceLink.rel) throw new Error('expected resource drill-down link to avoid new-tab rel attributes');
  const resourceIDFragment = new URLSearchParams({ resourceId: 'ecsService::payments-api-staging ECS Service::Primary ECS service for the staging environment' }).toString();
  const codeDeployApplicationIDFragment = new URLSearchParams({ resourceId: 'codeDeployApplication::payments-api-staging CodeDeploy App::CodeDeploy application coordinating ECS traffic shifting' }).toString();
  const codeDeployApplicationLiveInspectionLink = links.find((node) => node.textContent === 'Open live inspection' && node.href.includes(codeDeployApplicationIDFragment));
  const codeDeployApplicationRecentLogsLink = links.find((node) => node.textContent === 'Open recent logs' && node.href.includes(codeDeployApplicationIDFragment));
  if (!resourceLink.href.includes(resourceIDFragment)) throw new Error('expected stable unique resource identifier in drill-down href');
  if (!liveInspectionLink.href.includes('/?page=resource&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected live inspection href to preserve filters and deployment context');
  if (!recentLogsLink.href.includes('/?page=resource&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected recent logs href to preserve filters and deployment context');
  if (!liveInspectionLink.href.includes(resourceIDFragment) || !liveInspectionLink.href.endsWith('#resource-live-inspection')) throw new Error('expected live inspection handoff to target the existing resource section');
  if (!recentLogsLink.href.includes(resourceIDFragment) || !recentLogsLink.href.endsWith('#resource-recent-logs')) throw new Error('expected recent logs handoff to target the existing resource section');
  if (!codeDeployApplicationLiveInspectionLink) throw new Error('expected CodeDeploy application row to link directly to live inspection');
  if (!codeDeployApplicationLiveInspectionLink.href.includes('/?page=resource&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging') || !codeDeployApplicationLiveInspectionLink.href.endsWith('#resource-live-inspection')) throw new Error('expected CodeDeploy application live inspection link to preserve route state and target the live-inspection panel');
  if (codeDeployApplicationRecentLogsLink) throw new Error('CodeDeploy application rows should not link to recent logs without backend support');
  if (liveInspectionLink.target !== '_self' || recentLogsLink.target !== '_self' || codeDeployApplicationLiveInspectionLink.target !== '_self') throw new Error('expected direct evidence handoffs to stay in embedded UI flow');
})();
`

	runNodeHarness(t, nodeProgram)

	t.Run("refreshes application detail in place", func(t *testing.T) {
		initialDetailJSON, err := json.Marshal(map[string]any{
			"name":             "payments-api-staging",
			"accountName":      "staging-core",
			"awsAccountId":     "210987654321",
			"region":           "ap-northeast-2",
			"appRef":           "payments",
			"environmentRef":   "staging-apne2",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"desiredArtifact":  map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260415"},
			"sync":             map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":           map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
			"managedDrift": []map[string]any{
				{"field": "runtimeContext.capacity.desired", "severity": "high"},
				{"field": "runtimeContext.launchTemplate.imageId", "severity": "medium"},
			},
			"runtimeSummary": map[string]any{
				"timestamp":    "2026-04-16T07:15:00Z",
				"historyCount": 1,
				"sync":         map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
				"health":       map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
			},
			"rollout": map[string]any{
				"execution": []map[string]any{
					{"name": "payments-api-staging ECS Service", "resource": "ecsService", "description": "Primary ECS service for the staging environment"},
					{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal initial application detail: %v", err)
		}

		refreshedDetailJSON, err := json.Marshal(map[string]any{
			"name":             "payments-api-staging",
			"accountName":      "staging-core",
			"awsAccountId":     "210987654321",
			"region":           "ap-northeast-2",
			"appRef":           "payments",
			"environmentRef":   "staging-apne2",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"desiredArtifact":  map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260416"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/devopsartfactory/payments-api:staging-20260416"},
			"sync":             map[string]any{"state": "Reconciling", "reason": "replacement task set is warming up"},
			"health":           map[string]any{"overall": "Progressing", "summary": "Replacement task set is warming up"},
			"runtimeSummary": map[string]any{
				"timestamp":    "2026-04-16T07:20:00Z",
				"historyCount": 3,
				"sync":         map[string]any{"state": "Reconciling", "reason": "replacement task set is warming up"},
				"health":       map[string]any{"overall": "Progressing", "summary": "Replacement task set is warming up"},
			},
			"rollout": map[string]any{
				"execution": []map[string]any{
					{"name": "payments-api-canary ECS Service", "resource": "ecsService", "description": "Primary ECS service for the refreshed staging environment"},
					{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal refreshed application detail: %v", err)
		}

		eventsJSON, err := json.Marshal(map[string]any{
			"deploymentName": "payments-api-staging",
			"events":         []any{},
		})
		if err != nil {
			t.Fatalf("marshal application events body: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const initialDetail = ` + string(initialDetailJSON) + `;
const refreshedDetail = ` + string(refreshedDetailJSON) + `;
const eventsBody = ` + string(eventsJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(fetchImpl) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    return fetchImpl(String(url));
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const applicationURL = '/v1/overview/applications/payments-api-staging';
  const eventsURL = '/v1/overview/deployments/payments-api-staging/events';
  const initialSearch = '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321';

  let refreshFetchCount = 0;
  let resolveRefresh;
  const loadingView = await render((url) => {
    if (url === eventsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(eventsBody) });
    if (url !== applicationURL) throw new Error('unexpected fetch url: ' + url);
    refreshFetchCount += 1;
    if (refreshFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetail) });
    if (refreshFetchCount === 2) return new Promise((resolve) => { resolveRefresh = resolve; });
    throw new Error('unexpected application fetch count: ' + refreshFetchCount);
  });
  let toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  let refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected application refresh button enabled after initial render');
  let accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected initial application resource before refresh');
  if (!accountsText.includes('Current application status')) throw new Error('expected initial application status summary before refresh');
  if (!accountsText.includes('1 runtime event')) throw new Error('expected initial application runtime event summary before refresh');
  if (!accountsText.includes('2 total · 1 high · 1 medium · 0 low')) throw new Error('expected initial application managed drift summary to fall back to managedDrift rows');
  refreshButton.dispatch('click');
  await settle();
  toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton || !refreshButton.disabled) throw new Error('expected application refresh button disabled only while refresh is in flight');
  accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('Refreshing application detail from GET /v1/overview/applications/{deploymentName}…')) throw new Error('expected application refresh loading copy');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected current application content to stay visible during refresh');
  if (loadingView.fetchCalls.length !== 3 || loadingView.fetchCalls[0] !== applicationURL || loadingView.fetchCalls[1] !== eventsURL || loadingView.fetchCalls[2] !== applicationURL) throw new Error('expected application refresh to re-fetch only the application detail route');
  if (loadingView.window.location.search !== initialSearch) throw new Error('expected application refresh to preserve bookmarkable route');
  resolveRefresh({ ok: true, json: () => Promise.resolve(refreshedDetail) });
  await settle();
  await settle();
  accountsText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('payments-api-canary ECS Service')) throw new Error('expected refreshed application detail to render in place');
  if (accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected refreshed application detail to replace stale resource content');
  if (!accountsText.includes('replacement task set is warming up')) throw new Error('expected refreshed application summary signal');
  if (!accountsText.includes('3 runtime events')) throw new Error('expected refreshed application runtime event summary');
  if (!accountsText.includes('No managed drift reported')) throw new Error('expected refreshed application to show managed drift empty state');
  if (!accountsText.includes('ghcr.io/devopsartfactory/payments-api:staging-20260416')) throw new Error('expected refreshed application artifact summary');
  toolbarButtons = collectNodes(loadingView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected application refresh button re-enabled after refresh completes');

  let authFetchCount = 0;
  const authView = await render((url) => {
    if (url === eventsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(eventsBody) });
    if (url !== applicationURL) throw new Error('unexpected auth-view fetch url: ' + url);
    authFetchCount += 1;
    if (authFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetail) });
    if (authFetchCount === 2) return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: authenticated identity is required for this operator read route') });
    throw new Error('unexpected auth-view application fetch count: ' + authFetchCount);
  });
  toolbarButtons = collectNodes(authView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton) throw new Error('expected application refresh button in auth error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  accountsText = collectText(authView.document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('Authenticated operator required')) throw new Error('expected application refresh auth heading');
  if (!accountsText.includes('Tenet rejected GET /v1/overview/applications/payments-api-staging because authenticated identity is required on this operator read route.')) throw new Error('expected application refresh trusted-front-door guidance');
  if (!accountsText.includes('Current application detail remains visible below.')) throw new Error('expected application refresh auth guidance to keep stale-content copy');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected application content to remain visible after auth failure');
  if (accountsText.includes('Failed to refresh application detail:')) throw new Error('expected application auth refresh to avoid generic refresh failure copy');
  toolbarButtons = collectNodes(authView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected application refresh button re-enabled after auth failure');
  if (authFetchCount !== 2) throw new Error('expected application auth refresh to retry exactly once');
  if (authView.window.location.search !== initialSearch) throw new Error('expected application auth refresh to preserve bookmarkable route');
  if (authView.fetchCalls.filter((url) => url === eventsURL).length !== 1) throw new Error('expected application auth refresh not to refetch controller runtime events');

  let errorFetchCount = 0;
  const errorView = await render((url) => {
    if (url === eventsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(eventsBody) });
    if (url !== applicationURL) throw new Error('unexpected error-view fetch url: ' + url);
    errorFetchCount += 1;
    if (errorFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetail) });
    if (errorFetchCount === 2) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected error-view application fetch count: ' + errorFetchCount);
  });
  toolbarButtons = collectNodes(errorView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton) throw new Error('expected application refresh button in generic error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  accountsText = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('Failed to refresh application detail: application detail request failed (500): boom')) throw new Error('expected application refresh error copy');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected current application content to remain visible after refresh error');
  toolbarButtons = collectNodes(errorView.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refreshButton = toolbarButtons.find((node) => node.textContent === 'Refresh application detail');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected application refresh button re-enabled after refresh error');
  if (errorFetchCount !== 2) throw new Error('expected application refresh error to retry exactly once');
  if (errorView.window.location.search !== initialSearch) throw new Error('expected application refresh error state to preserve bookmarkable route');
  if (errorView.fetchCalls.filter((url) => url === eventsURL).length !== 1) throw new Error('expected application refresh error not to refetch controller runtime events');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("suppresses broken resource links when detail name missing", func(t *testing.T) {
		detailWithoutNameJSON, err := json.Marshal(map[string]any{
			"accountName":    "staging-core",
			"awsAccountId":   "210987654321",
			"region":         "ap-northeast-2",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
			"rollout": map[string]any{
				"execution": []map[string]any{
					{"name": "payments-api-staging ECS Service", "resource": "ecsService", "description": "Primary ECS service for the staging environment"},
					{"name": "prod listener", "resource": "productionListener", "description": "Routes production traffic"},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal nameless detail body: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailWithoutNameJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-staging' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging/events') throw new Error('expected nameless application page to load application detail plus deployment events');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  if (links.length !== 0) throw new Error('expected nameless application page to suppress broken resource drill-down links');
})();
`

		runNodeHarness(t, nodeProgram)
	})
}

func TestApplicationDetailPageDeploymentEventsTimelineStatesWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":           "payments-api-staging",
		"accountName":    "staging-core",
		"awsAccountId":   "210987654321",
		"region":         "ap-northeast-2",
		"appRef":         "payments",
		"environmentRef": "staging-apne2",
		"targetType":     "ecs",
		"strategy":       "linear",
		"executionMode":  "ecsCodeDeploy",
		"sync":           map[string]any{"state": "OutOfSync", "reason": "traffic shift running"},
		"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging ECS Service", "resource": "ecsService", "description": "Primary ECS service for the staging environment"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal application detail body: %v", err)
	}

	eventsJSON, err := json.Marshal(map[string]any{
		"deploymentName":      "payments-api-staging",
		"managedDriftSummary": map[string]any{"total": 2, "high": 1, "medium": 1, "low": 0},
		"events": []map[string]any{
			{
				"timestamp":     "2026-04-16T07:05:00Z",
				"summary":       "Created deployment group revision for v2",
				"source":        "controller",
				"targetType":    "ecs",
				"strategy":      "linear",
				"executionMode": "ecsCodeDeploy",
				"sync":          map[string]any{"state": "OutOfSync"},
				"health":        map[string]any{"overall": "Progressing", "summary": "warming"},
				"rolloutPhase":  "Accepted",
				"currentStep":   "CreateCodeDeployDeployment",
				"changeDetails": []string{"Created deployment group revision for v2"},
			},
			{
				"timestamp":          "2026-04-16T07:10:00Z",
				"summary":            "Shifted 50 percent of traffic",
				"source":             "controller",
				"targetType":         "ecs",
				"strategy":           "linear",
				"executionMode":      "ecsCodeDeploy",
				"sync":               map[string]any{"state": "OutOfSync", "reason": "traffic shift running"},
				"health":             map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
				"rolloutPhase":       "Accepted",
				"currentStep":        "ShiftTrafficLinear",
				"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v2"},
				"changeDetails":      []string{"Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear"},
				"conditions":         []map[string]any{{"type": "Progressing", "status": true, "reason": "TrafficShiftLinear", "message": "controller is shifting production traffic"}},
				"desiredGeneration":  12,
				"observedGeneration": 11,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal application events body: %v", err)
	}

	refreshedEventsJSON, err := json.Marshal(map[string]any{
		"deploymentName":      "payments-api-staging",
		"managedDriftSummary": map[string]any{"total": 0, "high": 0, "medium": 0, "low": 0},
		"events": []map[string]any{
			{
				"timestamp":          "2026-04-16T07:14:00Z",
				"summary":            "Completed traffic shift for v2",
				"source":             "controller",
				"targetType":         "ecs",
				"strategy":           "linear",
				"executionMode":      "ecsCodeDeploy",
				"sync":               map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
				"health":             map[string]any{"overall": "Healthy", "summary": "2/2 tasks healthy"},
				"rolloutPhase":       "Completed",
				"currentStep":        "CompleteDeployment",
				"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v2"},
				"changeDetails":      []string{"Sync state changed from OutOfSync to Synced"},
				"conditions":         []map[string]any{{"type": "Ready", "status": true, "reason": "DeploymentComplete", "message": "controller finished the traffic shift"}},
				"desiredGeneration":  12,
				"observedGeneration": 12,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal refreshed application events body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const eventsBody = ` + string(eventsJSON) + `;
const refreshedEventsBody = ` + string(refreshedEventsJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(fetchImpl) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    return fetchImpl(String(url));
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const detailURL = '/v1/overview/applications/payments-api-staging';
  const eventsURL = '/v1/overview/deployments/payments-api-staging/events';
  const initialSearch = '?page=application&deployment=payments-api-staging';
  let eventsFetchCount = 0;
  let resolveInitialEvents;
  let resolveRefreshEvents;
  const loadingView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) {
      eventsFetchCount += 1;
      if (eventsFetchCount === 1) {
        return new Promise((resolve) => { resolveInitialEvents = resolve; });
      }
      if (eventsFetchCount === 2) {
        return new Promise((resolve) => { resolveRefreshEvents = resolve; });
      }
      throw new Error('unexpected deployment events fetch count: ' + eventsFetchCount);
    }
    throw new Error('unexpected fetch url: ' + url);
  });
  let buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  let refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents) throw new Error('expected runtime-events refresh button while initial load is pending');
  if (!refreshEvents.disabled) throw new Error('expected runtime-events refresh button disabled while initial fetch is in flight');
  const loadingText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!loadingText.includes('Loading controller runtime events…')) throw new Error('expected loading state while deployment events request is pending');
  if (loadingView.fetchCalls.length !== 2 || loadingView.fetchCalls[0] !== detailURL || loadingView.fetchCalls[1] !== eventsURL) throw new Error('expected application detail fetch followed by deployment events fetch');
  resolveInitialEvents({ ok: true, json: () => Promise.resolve(eventsBody) });
  await settle();
  await settle();
  let readyText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!readyText.includes('Controller runtime events')) throw new Error('expected controller runtime events section heading');
  if (!readyText.includes('Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.')) throw new Error('expected application page to distinguish controller runtime events from audit/auth history');
  if (!readyText.includes('2 events · newest controller event first')) throw new Error('expected event ordering guidance');
  if (!readyText.includes('Managed drift summary · 2 total · 1 high · 1 medium · 0 low')) throw new Error('expected managed drift severity summary from deployment events payload');
  if (!readyText.includes('Shifted 50 percent of traffic')) throw new Error('expected newest controller event summary');
  if (!readyText.includes('Time · 2026-04-16T07:10:00Z')) throw new Error('expected newest controller event timestamp');
  if (!readyText.includes('Sync · OutOfSync · traffic shift running')) throw new Error('expected sync context');
  if (!readyText.includes('Health · Progressing · 1/2 tasks healthy')) throw new Error('expected health context');
  if (!readyText.includes('Context · Target ecs · Strategy linear · Execution ecsCodeDeploy')) throw new Error('expected rollout context');
  if (!readyText.includes('Rollout phase · Accepted')) throw new Error('expected rollout phase');
  if (!readyText.includes('Rollout step · ShiftTrafficLinear')) throw new Error('expected rollout step');
  if (!readyText.includes('Artifact · ghcr.io/example/payments:v2')) throw new Error('expected deployed artifact context');
  if (!readyText.includes('Conditions · Progressing = True · TrafficShiftLinear')) throw new Error('expected conditions context');
  if (!readyText.includes('Event details · Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear')) throw new Error('expected event change details');
  if (!readyText.includes('Source · controller')) throw new Error('expected controller source metadata');
  if (readyText.indexOf('Shifted 50 percent of traffic') > readyText.indexOf('Created deployment group revision for v2')) throw new Error('expected newest event to render before older event');
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents) throw new Error('expected runtime-events refresh button after initial response');
  if (refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled after initial fetch settles');
  let runtimeEventLinks = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  const driftHandoff = runtimeEventLinks.find((node) => node.textContent === 'Open field-level drift' && node.href === '/?page=drift&deployment=payments-api-staging#drift-field-level');
  if (!driftHandoff || driftHandoff.target !== '_self' || driftHandoff.rel) throw new Error('expected runtime-events managed-drift handoff to open the field-level drift workspace section in shell');

  refreshEvents.dispatch('click');
  await settle();
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents) throw new Error('expected runtime-events refresh button during refresh');
  if (!refreshEvents.disabled) throw new Error('expected runtime-events refresh button disabled only while refresh is in flight');
  if (loadingView.fetchCalls.length !== 3 || loadingView.fetchCalls[2] !== eventsURL) throw new Error('expected in-place refresh to re-fetch only the deployment events route');
  if (loadingView.fetchCalls.filter((url) => url === detailURL).length !== 1) throw new Error('expected in-place refresh to avoid reloading application detail');
  if (loadingView.window.location.search !== initialSearch) throw new Error('expected in-place refresh to preserve bookmarkable application route');

  resolveRefreshEvents({ ok: true, json: () => Promise.resolve(refreshedEventsBody) });
  await settle();
  await settle();
  readyText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!readyText.includes('Completed traffic shift for v2')) throw new Error('expected refreshed deployment events payload to render in place');
  if (!readyText.includes('Managed drift summary · 0 total · 0 high · 0 medium · 0 low')) throw new Error('expected refreshed managed drift summary to replace stale counts');
  if (readyText.includes('Shifted 50 percent of traffic')) throw new Error('expected refreshed deployment events payload to replace stale ready-state content');
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || refreshEvents.disabled) throw new Error('expected runtime-events refresh button re-enabled after refresh completes');
  runtimeEventLinks = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (runtimeEventLinks.find((node) => node.textContent === 'Open field-level drift' && node.href === '/?page=drift&deployment=payments-api-staging#drift-field-level')) throw new Error('expected zero-drift runtime-events refresh to remove field-level drift workspace handoff');

  const emptyView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  });
  const emptyText = collectText(emptyView.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No controller runtime events have been persisted yet.')) throw new Error('expected empty deployment events state');
  buttons = collectNodes(emptyView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled in empty state');
  const emptyLinks = collectNodes(emptyView.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (emptyLinks.find((node) => node.textContent === 'Open field-level drift' && node.href === '/?page=drift&deployment=payments-api-staging#drift-field-level')) throw new Error('expected empty runtime-events state not to show field-level drift workspace handoff');

  const unavailableView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) return Promise.resolve({ ok: false, status: 503, text: () => Promise.resolve('overview deployment events unavailable') });
    throw new Error('unexpected fetch url: ' + url);
  });
  const unavailableText = collectText(unavailableView.document.getElementById('accounts')).join(' | ');
  if (!unavailableText.includes('Controller runtime events are currently unavailable.')) throw new Error('expected unavailable deployment events state');
  if (!unavailableText.includes('deployment events unavailable (503): overview deployment events unavailable')) throw new Error('expected unavailable deployment events detail');
  buttons = collectNodes(unavailableView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled in unavailable state');

  let authFetchCount = 0;
  let resolveAuthRefresh;
  const unauthorizedView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) {
      authFetchCount += 1;
      if (authFetchCount === 1) return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: operator identity missing') });
      if (authFetchCount === 2) {
        return new Promise((resolve) => { resolveAuthRefresh = resolve; });
      }
      throw new Error('unexpected unauthorized-view deployment events fetch count: ' + authFetchCount);
    }
    throw new Error('unexpected fetch url: ' + url);
  });
  let authText = collectText(unauthorizedView.document.getElementById('accounts')).join(' | ');
  if (!authText.includes('Authenticated operator required')) throw new Error('expected operator auth-required heading for deployment events');
  if (!authText.includes('Tenet rejected GET /v1/overview/deployments/payments-api-staging/events because authenticated identity is required on this operator read route. Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.')) throw new Error('expected trusted front door guidance for deployment events 401');
  if (authText.includes('Failed to load controller runtime events:')) throw new Error('expected deployment events 401 to replace generic fetch error copy');
  buttons = collectNodes(unauthorizedView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled in auth-required state');
  refreshEvents.dispatch('click');
  await settle();
  buttons = collectNodes(unauthorizedView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || !refreshEvents.disabled) throw new Error('expected runtime-events refresh button disabled during auth-required refresh');
  if (authFetchCount !== 2) throw new Error('expected runtime-events auth-required refresh to bypass cached error state');
  if (unauthorizedView.fetchCalls.filter((url) => url === detailURL).length !== 1) throw new Error('expected auth-required refresh to avoid reloading application detail');
  if (unauthorizedView.window.location.search !== initialSearch) throw new Error('expected auth-required refresh to preserve bookmarkable application route');
  resolveAuthRefresh({ ok: true, json: () => Promise.resolve(refreshedEventsBody) });
  await settle();
  await settle();
  authText = collectText(unauthorizedView.document.getElementById('accounts')).join(' | ');
  if (!authText.includes('Completed traffic shift for v2')) throw new Error('expected auth-required refresh to render refreshed deployment events payload');
  if (authText.includes('Authenticated operator required')) throw new Error('expected auth-required refresh to replace stale operator guidance');

  const forbiddenView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) return Promise.resolve({ ok: false, status: 403, text: () => Promise.resolve('forbidden: operator does not have access to this deployment') });
    throw new Error('unexpected fetch url: ' + url);
  });
  const forbiddenText = collectText(forbiddenView.document.getElementById('accounts')).join(' | ');
  if (!forbiddenText.includes('Operator access denied')) throw new Error('expected operator access-denied heading for deployment events');
  if (!forbiddenText.includes('Tenet rejected GET /v1/overview/deployments/payments-api-staging/events after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.')) throw new Error('expected operator access-denied guidance for deployment events 403');
  if (forbiddenText.includes('Failed to load controller runtime events:')) throw new Error('expected deployment events 403 to replace generic fetch error copy');
  buttons = collectNodes(forbiddenView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled in access-denied state');

  let errorFetchCount = 0;
  let resolveErrorRefresh;
  const errorView = await render((url) => {
    if (url === detailURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === eventsURL) {
      errorFetchCount += 1;
      if (errorFetchCount === 1) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
      if (errorFetchCount === 2) {
        return new Promise((resolve) => { resolveErrorRefresh = resolve; });
      }
      throw new Error('unexpected error-view deployment events fetch count: ' + errorFetchCount);
    }
    throw new Error('unexpected fetch url: ' + url);
  });
  let errorText = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!errorText.includes('Failed to load controller runtime events: deployment events request failed (500): boom')) throw new Error('expected generic deployment events fetch error state');
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents) throw new Error('expected runtime-events refresh button in error state');
  if (refreshEvents.disabled) throw new Error('expected runtime-events refresh button enabled after error state settles');
  refreshEvents.dispatch('click');
  await settle();
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshEvents = buttons.find((node) => node.textContent === 'Refresh runtime events');
  if (!refreshEvents || !refreshEvents.disabled) throw new Error('expected runtime-events refresh button disabled during error-state refresh');
  if (errorFetchCount !== 2) throw new Error('expected runtime-events refresh to bypass cached error state');
  if (errorView.fetchCalls.filter((url) => url === detailURL).length !== 1) throw new Error('expected error-state refresh to avoid reloading application detail');
  if (errorView.window.location.search !== initialSearch) throw new Error('expected error-state refresh to preserve bookmarkable application route');
  resolveErrorRefresh({ ok: true, json: () => Promise.resolve(refreshedEventsBody) });
  await settle();
  await settle();
  errorText = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!errorText.includes('Completed traffic shift for v2')) throw new Error('expected refreshed deployment events payload after error-state refresh');
  if (errorText.includes('Failed to load controller runtime events: deployment events request failed (500): boom')) throw new Error('expected error-state refresh to replace stale error copy');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestEmbeddedUIApplicationDetailHidesEmptyLinkedResourceGroupsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-empty",
		"accountName":      "staging-core",
		"awsAccountId":     "210987654321",
		"region":           "ap-northeast-2",
		"appRef":           "payments",
		"environmentRef":   "staging-apne2",
		"sync":             map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
		"health":           map[string]any{"overall": "Healthy", "summary": "service steady"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v9"},
	})
	if err != nil {
		t.Fatalf("marshal empty application detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-empty', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/applications/payments-api-empty') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/overview/deployments/payments-api-empty/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-empty', events: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[0] !== '/v1/overview/applications/payments-api-empty' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-empty/events') throw new Error('expected empty application page to load application detail plus deployment events');
  const accounts = document.getElementById('accounts');
  const accountsText = collectText(accounts).join(' | ');
  if (!accountsText.includes('No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.')) throw new Error('expected explicit application empty-state copy');
  if (!accountsText.includes('Controller runtime events')) throw new Error('expected runtime-events section to remain visible without linked resources');
  if (accountsText.includes('Workloads')) throw new Error('expected empty application page to hide workloads group');
  if (accountsText.includes('Traffic routing')) throw new Error('expected empty application page to hide traffic routing group');
  if (accountsText.includes('Config and access')) throw new Error('expected empty application page to hide config and access group');
  const links = collectNodes(accounts, (node) => node.tagName === 'A');
  const sectionLinks = links.filter((node) => typeof node.href === 'string' && node.href.includes('#application-'));
  if (sectionLinks.length !== 1) throw new Error('expected empty application page quick nav to keep only runtime-events section link, got ' + sectionLinks.length);
  const runtimeEventsLink = sectionLinks.find((node) => node.href === '/?page=application&deployment=payments-api-empty#application-runtime-events' && node.textContent === 'Controller runtime events');
  if (!runtimeEventsLink || runtimeEventsLink.target !== '_self' || runtimeEventsLink.rel) throw new Error('expected empty application page runtime-events quick-nav link to stay in shell');
  const sectionIDs = collectNodes(accounts, (node) => typeof node.id === 'string' && node.id.startsWith('application-')).map((node) => node.id).sort();
  if (JSON.stringify(sectionIDs) !== JSON.stringify(['application-runtime-events'])) throw new Error('expected only runtime-events section id for empty application page, got ' + JSON.stringify(sectionIDs));
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageUsesStableResourceIDsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "prod-core",
		"awsAccountId": "123456789012",
		"deployments": []map[string]any{{
			"name":           "payments-api-prod",
			"appRef":         "payments",
			"environmentRef": "prod-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ec2",
			"strategy":       "canary",
			"executionMode":  "asgWeightedTargetGroups",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	fallbackDetailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-prod",
		"accountName":      "prod-core",
		"awsAccountId":     "123456789012",
		"region":           "ap-northeast-2",
		"appRef":           "payments",
		"environmentRef":   "prod-apne2",
		"targetType":       "ec2",
		"strategy":         "canary",
		"executionMode":    "asgWeightedTargetGroups",
		"rolloutConfig":    map[string]any{"capacity": map[string]any{"desired": 4, "min": 2, "max": 8}, "strategy": map[string]any{"warmupSeconds": 90}},
		"userData":         map[string]any{"type": "file", "path": "userdata/payments-api-prod.sh"},
		"sync":             map[string]any{"state": "OutOfSync", "reason": "weighted rollout in progress"},
		"health":           map[string]any{"overall": "Progressing", "summary": "candidate fleet bootstrapping"},
		"desiredArtifact":  map[string]any{"binary": "s3://artifacts/payments/v2.zip"},
		"deployedArtifact": map[string]any{"binary": "s3://artifacts/payments/v1.zip"},
		"runtimeSummary": map[string]any{
			"source":             "runtime-agent",
			"timestamp":          "2026-04-16T07:10:00Z",
			"historyCount":       1,
			"desiredGeneration":  12,
			"observedGeneration": 11,
		},
		"rollout": map[string]any{
			"phase":              "Progressing",
			"currentStep":        "ShiftTrafficCanary",
			"desiredGeneration":  12,
			"observedGeneration": 11,
		},
	})
	if err != nil {
		t.Fatalf("marshal fallback detail body: %v", err)
	}

	canonicalDetailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-prod",
		"accountName":      "prod-core",
		"awsAccountId":     "123456789012",
		"region":           "ap-northeast-2",
		"appRef":           "payments",
		"environmentRef":   "prod-apne2",
		"targetType":       "ec2",
		"strategy":         "canary",
		"executionMode":    "asgWeightedTargetGroups",
		"rolloutConfig":    map[string]any{"capacity": map[string]any{"desired": 4, "min": 2, "max": 8}, "strategy": map[string]any{"warmupSeconds": 90}},
		"userData":         map[string]any{"type": "file", "path": "userdata/payments-api-prod.sh"},
		"sync":             map[string]any{"state": "OutOfSync", "reason": "weighted rollout in progress"},
		"health":           map[string]any{"overall": "Progressing", "summary": "candidate fleet bootstrapping"},
		"desiredArtifact":  map[string]any{"binary": "s3://artifacts/payments/v2.zip"},
		"deployedArtifact": map[string]any{"binary": "s3://artifacts/payments/v1.zip"},
		"runtimeSummary": map[string]any{
			"source":             "runtime-agent",
			"timestamp":          "2026-04-16T07:10:00Z",
			"historyCount":       1,
			"desiredGeneration":  12,
			"observedGeneration": 11,
		},
		"rollout": map[string]any{
			"phase":              "Progressing",
			"currentStep":        "ShiftTrafficCanary",
			"desiredGeneration":  12,
			"observedGeneration": 11,
			"execution": []map[string]any{
				{"name": "payments-api-prod-candidate", "resource": "candidateAutoScalingGroup", "description": "Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/payments-api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout"},
				{"name": "payments-api-prod-active", "resource": "activeTargetGroup", "description": "Currently active target group receiving production traffic"},
				{"name": "payments-api-prod-candidate", "resource": "candidateTargetGroup", "description": "Candidate target group Tenet will register and warm before traffic shifting"},
				{"name": "payments-api-prod-prod-listener", "resource": "productionListenerRule", "description": "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
				{"name": "payments-api-prod-weighted-routing", "resource": "trafficShiftPlan", "description": "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s"},
				{"name": "file:userdata/payments-api-prod.sh", "resource": "userDataBootstrapSource", "description": "Normalized userdata bootstrap source for candidate EC2 capacity"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal canonical detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const fallbackDetailBody = ` + string(fallbackDetailJSON) + `;
const canonicalDetailBody = ` + string(canonicalDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
const runUI = async (search, detailBody) => {
  const document = createDocument();
  const fetchCalls = [];
  const resourceLogsURL = '/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing';
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/applications/payments-api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-prod/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-prod', events: [] }) });
    if (url === '/v1/overview/deployments/payments-api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === resourceLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, window, fetchCalls };
};
(async () => {
  const fallbackRun = await runUI('?page=application&deployment=payments-api-prod', fallbackDetailBody);
  if (fallbackRun.fetchCalls.length !== 2 || fallbackRun.fetchCalls[0] !== '/v1/overview/applications/payments-api-prod' || fallbackRun.fetchCalls[1] !== '/v1/overview/deployments/payments-api-prod/events') throw new Error('expected fallback application page to load application detail plus deployment events');
  const fallbackLinks = collectNodes(fallbackRun.document.getElementById('accounts'), (node) => node.tagName === 'A');
  const resourceLinks = fallbackLinks.filter((node) => node.textContent === 'Open resource');
  const liveInspectionLinks = fallbackLinks.filter((node) => node.textContent === 'Open live inspection');
  const recentLogsLinks = fallbackLinks.filter((node) => node.textContent === 'Open recent logs');
  const expectedResourceID = 'trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s';
  const resourceHref = '/?' + new URLSearchParams({
    page: 'resource',
    deployment: 'payments-api-prod',
    resourceType: 'trafficShiftPlan',
    resourceId: expectedResourceID,
  }).toString();
  const fallbackLink = resourceLinks.find((node) => node.href === resourceHref);
  const inspectionHref = resourceHref + '#resource-live-inspection';
  const inspectionLink = liveInspectionLinks.find((node) => node.href === inspectionHref);
  if (!fallbackLink) throw new Error('expected fallback application page to synthesize canonical trafficShiftPlan resource link');
  if (!inspectionLink) throw new Error('expected fallback application page to expose direct live inspection handoff for trafficShiftPlan');
  if (recentLogsLinks.some((node) => node.href === resourceHref + '#resource-recent-logs')) throw new Error('expected fallback application page to suppress unsupported recent logs handoff for trafficShiftPlan');
  const canonicalRun = await runUI(fallbackLink.href.slice(1), canonicalDetailBody);
  if (!canonicalRun.fetchCalls.includes('/v1/overview/deployments/payments-api-prod')) throw new Error('expected canonical resource page to load deployment detail');
  if (!canonicalRun.fetchCalls.includes('/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing')) throw new Error('expected canonical resource page to load recent logs for the selected resource');
  const summaryText = collectText(canonicalRun.document.getElementById('summary')).join(' | ');
  const accountsText = collectText(canonicalRun.document.getElementById('accounts')).join(' | ');
  if (!summaryText.includes('payments-api-prod-weighted-routing')) throw new Error('expected canonical resource page to resolve trafficShiftPlan resource title');
  if (!summaryText.includes('trafficShiftPlan')) throw new Error('expected canonical resource page to resolve trafficShiftPlan resource type');
  if (!summaryText.includes('Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s')) throw new Error('expected canonical resource page to resolve trafficShiftPlan resource description');
  if (summaryText.includes('No linked resource metadata is available for this selection.')) throw new Error('expected canonical resource page not to fall back to placeholder metadata');
  if (!accountsText.includes('Resource id')) throw new Error('expected canonical resource page to render resource id');
  if (!accountsText.includes(expectedResourceID)) throw new Error('expected canonical resource page to preserve the fallback-generated canonical resource id');
  if (accountsText.includes('No rollout execution resources are currently reported for this selection.')) throw new Error('expected canonical resource page to find rollout execution resources after transition');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "staging-core",
		"awsAccountId": "210987654321",
		"deployments": []map[string]any{{
			"name":           "payments-api-staging",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
			"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
			"runtimeSummary": map[string]any{"timestamp": "2026-04-16T07:10:00Z", "source": "runtime-agent", "historyCount": 2, "desiredGeneration": 12, "observedGeneration": 11},
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-staging",
		"accountName":      "staging-core",
		"awsAccountId":     "210987654321",
		"region":           "ap-northeast-2",
		"appRef":           "payments",
		"environmentRef":   "staging-apne2",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"sync":             map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":           map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v2"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1"},
		"runtimeSummary": map[string]any{
			"source":             "runtime-agent",
			"timestamp":          "2026-04-16T07:10:00Z",
			"targetType":         "ecs",
			"strategy":           "linear",
			"executionMode":      "ecsCodeDeploy",
			"historyCount":       2,
			"desiredGeneration":  12,
			"observedGeneration": 11,
			"changes":            []string{"deployment group switched to linear-10-percent-every-minute"},
			"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v1-runtime"},
			"conditions":         []map[string]any{{"type": "Available", "status": true, "reason": "RuntimeHealthy", "message": "Runtime summary copied the latest healthy condition"}},
		},
		"history": []map[string]any{{
			"timestamp":          "2026-04-16T07:05:00Z",
			"summary":            "Created deployment group revision for v2",
			"source":             "controller",
			"desiredGeneration":  12,
			"observedGeneration": 10,
			"rolloutPhase":       "Progressing",
			"currentStep":        "CreateCodeDeployDeployment",
			"targetType":         "ecs",
			"strategy":           "linear",
			"executionMode":      "ecsCodeDeploy",
			"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v0"},
			"conditions":         []map[string]any{{"type": "Progressing", "status": true, "reason": "DeploymentStarted", "message": "CodeDeploy started shifting traffic"}},
		}},
		"rollout": map[string]any{
			"phase":              "Progressing",
			"currentStep":        "ShiftTrafficCanary",
			"desiredGeneration":  12,
			"observedGeneration": 11,
			"execution": []map[string]any{
				{"name": "payments-api-staging ECS Service", "resource": "ecsService", "description": "Primary ECS service for the staging environment"},
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
			},
			"steps": []map[string]any{{"kind": "CreateCodeDeployDeployment", "status": "completed", "detail": "Created CodeDeploy deployment"}, {"kind": "ShiftTrafficCanary", "status": "current", "detail": "Moving 10% traffic to the green task set"}},
		},
		"conditions": []map[string]any{{
			"type":    "Available",
			"status":  true,
			"reason":  "ServiceReady",
			"message": "Service remains available during the staged rollout",
		}},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=codeDeployDeploymentGroup%3A%3Apayments-api-staging%20Deployment%20Group%3A%3ALinear%20traffic%20shifting%20deployment%20group&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (!fetchCalls.includes('/v1/overview/deployments/payments-api-staging')) throw new Error('expected resource page to load deployment detail');
  if (!fetchCalls.includes(logsURL)) throw new Error('expected resource page to load recent logs for selected resource');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Back to application')) throw new Error('expected resource page back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const pageLinks = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const quickNavChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const breadcrumbChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className.includes('breadcrumb-current'));
  const backLink = toolbarLinks.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.target === '_self');
  if (!backLink) throw new Error('expected resource page self-navigation back link');
  if (backLink.rel) throw new Error('expected resource page back link to avoid new-tab rel attributes');
  if (!accountsText.includes('Breadcrumbs')) throw new Error('expected resource page breadcrumb chrome');
  const applicationsBreadcrumb = pageLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications' && node.target === '_self');
  if (!applicationsBreadcrumb || applicationsBreadcrumb.rel) throw new Error('expected resource page applications breadcrumb link');
  const accountBreadcrumb = pageLinks.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321' && node.textContent === 'staging-core (210987654321)' && node.target === '_self');
  if (!accountBreadcrumb || accountBreadcrumb.rel) throw new Error('expected resource page account breadcrumb link');
  const applicationBreadcrumb = pageLinks.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'payments-api-staging' && node.target === '_self');
  if (!applicationBreadcrumb || applicationBreadcrumb.rel) throw new Error('expected resource page application breadcrumb link');
  if (!breadcrumbChips.find((node) => node.textContent === 'payments-api-staging Deployment Group')) throw new Error('expected resource page current breadcrumb chip');
  if (!accountsText.includes('Quick navigation')) throw new Error('expected resource page quick navigation strip');
  if (!accountsText.includes('Current resource')) throw new Error('expected resource page current-resource label');
  if (!accountsText.includes('Current section')) throw new Error('expected resource page current-section label');
  const quickBackLink = pageLinks.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Back to application' && node.target === '_self');
  if (!quickBackLink || quickBackLink.rel) throw new Error('expected resource page quick navigation back link');
  const driftLink = pageLinks.find((node) => node.href === '/?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Drift view' && node.target === '_self');
  if (!driftLink || driftLink.rel) throw new Error('expected resource page quick navigation drift link');
  const planLink = pageLinks.find((node) => node.href === '/?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Plan/apply' && node.target === '_self');
  if (!planLink || planLink.rel) throw new Error('expected resource page quick navigation plan link');
  const auditLink = pageLinks.find((node) => node.href === '/?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Audit log' && node.target === '_self');
  if (!auditLink || auditLink.rel) throw new Error('expected resource page quick navigation audit link');
  const currentResourceChip = quickNavChips.find((node) => node.textContent === 'payments-api-staging Deployment Group');
  if (!currentResourceChip) throw new Error('expected resource page current-resource chip');
  const currentSectionChip = quickNavChips.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '));
  if (currentSectionChip) throw new Error('expected resource page without hash not to render a current-section chip');
  const siblingLink = pageLinks.find((node) => node.textContent === 'payments-api-staging ECS Service'
    && node.target === '_self'
    && node.href.includes('/?page=resource')
    && node.href.includes('deployment=payments-api-staging')
    && node.href.includes('resourceType=ecsService')
    && node.href.includes('resourceId=ecsService%3A%3Apayments-api-staging+ECS+Service%3A%3APrimary'));
  if (!siblingLink || siblingLink.rel) throw new Error('expected resource page sibling quick link');
  if (!summaryText.includes('Resource detail')) throw new Error('expected resource page summary heading');
  if (!summaryText.includes('payments-api-staging Deployment Group')) throw new Error('expected resource page resource title');
  if (!summaryText.includes('codeDeployDeploymentGroup')) throw new Error('expected resource page resource type');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected resource page deployment context');
  if (!accountsText.includes('Desired artifact')) throw new Error('expected resource page desired artifact context');
  if (!accountsText.includes('ghcr.io/example/payments:v2')) throw new Error('expected resource page desired artifact value');
  if (!accountsText.includes('Deployed artifact')) throw new Error('expected resource page deployed artifact context');
  if (!accountsText.includes('ghcr.io/example/payments:v1')) throw new Error('expected resource page deployed artifact value');
  if (!accountsText.includes('Rollout phase')) throw new Error('expected resource page rollout phase');
  if (!accountsText.includes('ShiftTrafficCanary')) throw new Error('expected resource page current rollout step');
  if (!accountsText.includes('Runtime generations')) throw new Error('expected resource page runtime generation detail');
  if (!accountsText.includes('desired 12 · observed 11')) throw new Error('expected resource page runtime generation values');
  if (!accountsText.includes('Runtime context')) throw new Error('expected resource page runtime context label');
  if (!accountsText.includes('Target ecs · Strategy linear · Execution ecsCodeDeploy')) throw new Error('expected resource page runtime context value');
  if (!accountsText.includes('Runtime artifact')) throw new Error('expected resource page runtime artifact label');
  if (!accountsText.includes('ghcr.io/example/payments:v1-runtime')) throw new Error('expected resource page runtime artifact value');
  if (!accountsText.includes('Runtime conditions')) throw new Error('expected resource page runtime conditions label');
  if (!accountsText.includes('Available = True · RuntimeHealthy')) throw new Error('expected resource page runtime condition summary');
  if (!accountsText.includes('Runtime history')) throw new Error('expected resource page runtime history section');
  if (!accountsText.includes('Created deployment group revision for v2')) throw new Error('expected resource page history detail');
  if (!accountsText.includes('History context')) throw new Error('expected resource page history context label');
  if (!accountsText.includes('History artifact')) throw new Error('expected resource page history artifact label');
  if (!accountsText.includes('ghcr.io/example/payments:v0')) throw new Error('expected resource page history artifact value');
  if (!accountsText.includes('History conditions')) throw new Error('expected resource page history conditions label');
  if (!accountsText.includes('Progressing = True · DeploymentStarted')) throw new Error('expected resource page history condition summary');
  if (!accountsText.includes('Conditions')) throw new Error('expected resource page conditions section');
  if (!accountsText.includes('ServiceReady')) throw new Error('expected resource page condition reason');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageHashNavigationWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"phase":       "Progressing",
			"currentStep": "ShiftTrafficLinear",
			"execution": []map[string]any{
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) current.add(name); else current.delete(name);
        this.className = Array.from(current).join(' ');
      },
    };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
  scrollIntoView(options) {
    const entry = { id: this.id || '', options };
    this.ownerDocument.lastScrolledTarget = entry;
    this.ownerDocument.scrollEvents.push(entry);
  }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() {
    this.activeElement = null;
    this.byID = new Map();
    this.lastScrolledTarget = null;
    this.scrollEvents = [];
  }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const createWindow = (initialSearch, hash) => {
  const listeners = {};
  return {
    location: { pathname: '/console', search: initialSearch, hash },
    addEventListener(type, handler) {
      listeners[type] = listeners[type] || [];
      listeners[type].push(handler);
    },
    dispatchEvent(event) {
      const handlers = listeners[event?.type] || [];
      handlers.forEach((handler) => handler.call(this, event));
    },
    dispatchHashChange() {
      this.dispatchEvent({ type: 'hashchange', target: this });
    },
  };
};
const collectNodes = (node, predicate) => {
  if (!node) return [];
  const matches = [];
  const visit = (current) => {
    if (!current) return;
    if (predicate(current)) matches.push(current);
    (current.children || []).forEach(visit);
  };
  visit(node);
  return matches;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const resourceID = 'ecsService::payments-api-staging-service::Primary ECS service';
const deploymentURL = '/v1/overview/deployments/payments-api-staging';
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service';
const inspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service';
async function renderScenario(hash, fetchImpl) {
  const document = createDocument();
  const initialSearch = '?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(resourceID) + '&q=payments&sync=OutOfSync&account=210987654321';
  const window = createWindow(initialSearch, hash);
  const history = { replaceState(_state, _title, url) {
    const [pathAndQuery, nextHash = ''] = String(url).split('#');
    const [pathname, query = ''] = pathAndQuery.split('?');
    window.location.pathname = pathname || '/console';
    window.location.search = query ? '?' + query : '';
    window.location.hash = nextHash ? '#' + nextHash : '';
  } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  await settle();
  await settle();
  return { document, window, initialSearch };
}

(async () => {
  let resolveLogs;
  const recentLogsRun = await renderScenario('#resource-recent-logs', (url) => {
    if (url === deploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return new Promise((resolve) => { resolveLogs = resolve; });
    if (url === inspectionURL) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'ecs_service', lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true }, ecsService: { clusterArn: 'arn:aws:ecs:ap-northeast-2:210987654321:cluster/payments-staging', serviceArn: 'arn:aws:ecs:ap-northeast-2:210987654321:service/payments-staging/payments-api-staging-service', serviceName: 'payments-api-staging-service', status: 'ACTIVE', launchType: 'FARGATE', desiredCount: 2, runningCount: 2, pendingCount: 0, minimumHealthyPercent: 100, maximumPercent: 200, healthCheckGracePeriodSeconds: 45, taskDefinition: 'payments-api:42', deployments: [], events: [] } }) });
    }
    throw new Error('unexpected recent-logs fetch url: ' + url);
  });
  if (recentLogsRun.window.location.pathname !== '/console') throw new Error('expected resource hash pathname to stay stable, got ' + recentLogsRun.window.location.pathname);
  if (recentLogsRun.window.location.search !== recentLogsRun.initialSearch) throw new Error('expected resource hash query to stay stable, got ' + recentLogsRun.window.location.search);
  if (recentLogsRun.window.location.hash !== '#resource-recent-logs') throw new Error('expected recent logs hash to stay stable, got ' + recentLogsRun.window.location.hash);
  const recentLogsRoot = recentLogsRun.document.getElementById('accounts');
  const sectionIDs = collectNodes(recentLogsRoot, (node) => typeof node.id === 'string' && node.id.startsWith('resource-')).map((node) => node.id).sort();
  const expectedSectionIDs = ['resource-live-inspection', 'resource-recent-logs', 'resource-rollout-execution', 'resource-runtime-history', 'resource-status-conditions'];
  if (JSON.stringify(sectionIDs) !== JSON.stringify(expectedSectionIDs)) throw new Error('unexpected resource section ids: ' + JSON.stringify(sectionIDs));
  const recentLogsChip = collectNodes(recentLogsRoot, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip').find((node) => node.getAttribute('aria-label') === 'Current section: Recent logs');
  if (!recentLogsChip) throw new Error('expected recent logs hash to render the current-section chip');
  const sectionLinks = collectNodes(recentLogsRoot, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#resource-'));
  const expectedLinks = [
    ['#resource-runtime-history', 'Runtime history'],
    ['#resource-rollout-execution', 'Rollout execution'],
    ['#resource-status-conditions', 'Conditions'],
    ['#resource-live-inspection', 'Live inspection'],
  ];
  if (sectionLinks.length !== expectedLinks.length) throw new Error('expected four remaining resource section links, got ' + sectionLinks.length);
  expectedLinks.forEach(([hashSuffix, text]) => {
    const link = sectionLinks.find((node) => node.textContent === text);
    if (!link) throw new Error('missing resource section link ' + text);
    if (!link.href.startsWith('/console?page=resource')) throw new Error('expected resource section link to stay on the resource page for ' + text + ', got ' + link.href);
    const decodedHref = decodeURIComponent(link.href.replace(/\+/g, '%20'));
    ['q=payments', 'sync=OutOfSync', 'account=210987654321', 'deployment=payments-api-staging', 'resourceType=ecsService', 'resourceId=' + resourceID].forEach((fragment) => {
      const source = fragment.startsWith('resourceId=') ? decodedHref : link.href;
      if (!source.includes(fragment)) throw new Error('expected resource section link to preserve ' + fragment + ' for ' + text + ', got ' + link.href);
    });
    if (!link.href.endsWith(hashSuffix)) throw new Error('expected resource section link to end with ' + hashSuffix + ' for ' + text + ', got ' + link.href);
    if (link.target !== '_self' || link.rel) throw new Error('expected resource section link to stay in-shell for ' + text);
  });
  if (sectionLinks.find((node) => node.textContent === 'Recent logs')) throw new Error('expected active current section to be removed from On this page links');
  const recentLogsScrollsBefore = recentLogsRun.document.scrollEvents.filter((event) => event.id === 'resource-recent-logs').length;
  if (recentLogsScrollsBefore < 1) throw new Error('expected initial recent logs hash to scroll the matching section into view');
  resolveLogs({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true }, entries: [] }) });
  await settle();
  await settle();
  const recentLogsScrollsAfter = recentLogsRun.document.scrollEvents.filter((event) => event.id === 'resource-recent-logs').length;
  if (recentLogsScrollsAfter <= recentLogsScrollsBefore) throw new Error('expected recent logs hash to re-scroll after lazy panel load');
  if (!recentLogsRun.document.lastScrolledTarget || recentLogsRun.document.lastScrolledTarget.id !== 'resource-recent-logs') throw new Error('expected recent logs hash to remain the active scroll target after lazy panel load');
  recentLogsRun.window.location.hash = '#resource-status-conditions';
  recentLogsRun.window.dispatchHashChange();
  await settle();
  const updatedRecentLogsRoot = recentLogsRun.document.getElementById('accounts');
  const statusConditionsChip = collectNodes(updatedRecentLogsRoot, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip').find((node) => node.getAttribute('aria-label') === 'Current section: Conditions');
  if (!statusConditionsChip) throw new Error('expected hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedRecentLogsRoot, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#resource-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Conditions')) throw new Error('expected updated current section to be removed from On this page links after hashchange');
  if (!updatedSectionLinks.find((node) => node.textContent === 'Recent logs')) throw new Error('expected previous current section to return to On this page links after hashchange');
  if (!recentLogsRun.document.lastScrolledTarget || recentLogsRun.document.lastScrolledTarget.id !== 'resource-status-conditions') throw new Error('expected hashchange to scroll the new current section into view');

  let resolveInspection;
  const inspectionRun = await renderScenario('#resource-live-inspection', (url) => {
    if (url === deploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === inspectionURL) return new Promise((resolve) => { resolveInspection = resolve; });
    throw new Error('unexpected live-inspection fetch url: ' + url);
  });
  if (inspectionRun.window.location.hash !== '#resource-live-inspection') throw new Error('expected live inspection hash to stay stable, got ' + inspectionRun.window.location.hash);
  const inspectionChip = collectNodes(inspectionRun.document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip').find((node) => node.getAttribute('aria-label') === 'Current section: Live inspection');
  if (!inspectionChip) throw new Error('expected live inspection hash to render the current-section chip');
  const inspectionScrollsBefore = inspectionRun.document.scrollEvents.filter((event) => event.id === 'resource-live-inspection').length;
  if (inspectionScrollsBefore < 1) throw new Error('expected initial live inspection hash to scroll the matching section into view');
  resolveInspection({ ok: true, json: () => Promise.resolve({ available: true, source: 'ecs_service', lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true }, ecsService: { clusterArn: 'arn:aws:ecs:ap-northeast-2:210987654321:cluster/payments-staging', serviceArn: 'arn:aws:ecs:ap-northeast-2:210987654321:service/payments-staging/payments-api-staging-service', serviceName: 'payments-api-staging-service', status: 'ACTIVE', launchType: 'FARGATE', desiredCount: 2, runningCount: 2, pendingCount: 0, minimumHealthyPercent: 100, maximumPercent: 200, healthCheckGracePeriodSeconds: 45, taskDefinition: 'payments-api:42', deployments: [], events: [] } }) });
  await settle();
  await settle();
  const inspectionScrollsAfter = inspectionRun.document.scrollEvents.filter((event) => event.id === 'resource-live-inspection').length;
  if (inspectionScrollsAfter <= inspectionScrollsBefore) throw new Error('expected live inspection hash to re-scroll after lazy panel load');
  if (!inspectionRun.document.lastScrolledTarget || inspectionRun.document.lastScrolledTarget.id !== 'resource-live-inspection') throw new Error('expected live inspection hash to remain the active scroll target after lazy panel load');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPagePreservesBookmarkedAliasRouteIdentityWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":            "payments-api-prod",
		"accountName":     "prod-core",
		"awsAccountId":    "123456789012",
		"region":          "ap-northeast-2",
		"targetType":      "ec2",
		"strategy":        "canary",
		"executionMode":   "asgWeightedTargetGroups",
		"userData":        map[string]any{"type": "git", "path": "userdata/payments-api-prod.sh"},
		"rolloutConfig":   map[string]any{"capacity": map[string]any{"desired": 4, "min": 2, "max": 8}, "strategy": map[string]any{"warmupSeconds": 90}},
		"sync":            map[string]any{"state": "OutOfSync", "reason": "candidate capacity bootstrapping"},
		"health":          map[string]any{"overall": "Progressing", "summary": "candidate warmup in progress"},
		"desiredArtifact": map[string]any{"binary": "s3://artifacts/payments/v2.zip"},
		"deployedArtifact": map[string]any{
			"binary": "s3://artifacts/payments/v1.zip",
		},
		"runtimeSummary": map[string]any{
			"source":        "deployment-controller",
			"timestamp":     "2026-04-16T07:10:00Z",
			"targetType":    "ec2",
			"strategy":      "canary",
			"executionMode": "asgWeightedTargetGroups",
			"historyCount":  1,
		},
		"history":    []map[string]any{},
		"conditions": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) current.add(name); else current.delete(name);
        this.className = Array.from(current).join(' ');
      },
    };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() {
    this.activeElement = null;
    this.byID = new Map();
    this.lastScrolledTarget = null;
  }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => {
  if (!node) return [];
  const matches = [];
  const visit = (current) => {
    if (!current) return;
    if (predicate(current)) matches.push(current);
    (current.children || []).forEach(visit);
  };
  visit(node);
  return matches;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const requestedAliasID = 'userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity';
const canonicalID = 'userDataBootstrapSource::git:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity';
const deploymentURL = '/v1/overview/deployments/payments-api-prod';
const logsURL = '/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh';
const inspectionURL = '/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git%3Auserdata%2Fpayments-api-prod.sh';

(async () => {
  const document = createDocument();
  const window = {
    location: {
      pathname: '/console',
      search: '?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=' + encodeURIComponent(requestedAliasID),
      hash: '#resource-recent-logs',
    },
    addEventListener() {},
  };
  const fetchCalls = [];
  const history = { replaceState(_state, _title, url) {
    const [pathAndQuery, nextHash = ''] = String(url).split('#');
    const [pathname, query = ''] = pathAndQuery.split('?');
    window.location.pathname = pathname || '/console';
    window.location.search = query ? '?' + query : '';
    window.location.hash = nextHash ? '#' + nextHash : '';
  } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === deploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'no_log_source', entries: [] }) });
    if (url === inspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: 'auto_scaling_group', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true }, unavailableReason: 'aws_unavailable' }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();

  if (!fetchCalls.includes(logsURL)) throw new Error('expected alias resource page to load recent logs with canonical resource name');
  if (!fetchCalls.includes(inspectionURL)) throw new Error('expected alias resource page to load live inspection with canonical resource name');

  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsRoot = document.getElementById('accounts');
  const accountsText = collectText(accountsRoot).join(' | ');
  if (!summaryText.includes('Resource detail')) throw new Error('expected alias resource page summary heading');
  if (!accountsText.includes('Opened via bookmarked resource alias.')) throw new Error('expected alias resource page cue');
  if (!accountsText.includes(requestedAliasID)) throw new Error('expected alias resource page cue to keep requested bookmarked resource id visible');
  if (!accountsText.includes(canonicalID)) throw new Error('expected alias resource page to keep resolved canonical resource id visible');
  if (!accountsText.includes('Recent logs are unavailable for this resource.')) throw new Error('expected no-log-source resource page to render recent logs unavailable state');
  if (!accountsText.includes('This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.')) throw new Error('expected no-log-source recent logs state to render friendly operator copy');
  if (accountsText.includes('no_log_source')) throw new Error('expected no-log-source recent logs state not to expose raw unavailableReason code');

  const sectionLinks = collectNodes(accountsRoot, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#resource-'));
  if (!sectionLinks.length) throw new Error('expected alias resource page section links');
  sectionLinks.forEach((link) => {
    const decodedHref = decodeURIComponent(link.href.replace(/\+/g, '%20'));
    if (!decodedHref.includes('resourceId=' + requestedAliasID)) throw new Error('expected section link to preserve requested alias resource id, got ' + link.href);
    if (decodedHref.includes('resourceId=' + canonicalID)) throw new Error('expected section link not to rewrite to canonical resource id, got ' + link.href);
    if (link.target !== '_self' || link.rel) throw new Error('expected alias section link to stay in embedded UI');
  });
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageRecentLogsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":          "payments-api-staging",
		"accountName":   "staging-core",
		"awsAccountId":  "210987654321",
		"region":        "ap-northeast-2",
		"executionMode": "ecsCodeDeploy",
		"runtimeSummary": map[string]any{
			"timestamp":     "2026-04-16T07:10:00Z",
			"source":        "runtime-agent",
			"targetType":    "ecs",
			"strategy":      "linear",
			"executionMode": "ecsCodeDeploy",
			"historyCount":  1,
		},
		"rollout": map[string]any{
			"phase":       "Progressing",
			"currentStep": "ShiftTrafficLinear",
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const resourceID = 'codeDeployDeploymentGroup::payments-api-staging Deployment Group::Linear traffic shifting deployment group';
async function renderWithFetch(fetchImpl, search = '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent(resourceID)) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  await settle();
  return { document, window };
}

(async () => {
  const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';

  let resolveLogs;
  const loadingFetchCalls = [];
  const loadingRun = await renderWithFetch((url) => {
    loadingFetchCalls.push(String(url));
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return new Promise((resolve) => { resolveLogs = resolve; });
    throw new Error('unexpected loading fetch url: ' + url);
  });
  const loadingText = collectText(loadingRun.document.getElementById('accounts')).join(' | ');
  if (!loadingText.includes('Recent logs')) throw new Error('expected resource page to render recent logs section heading while loading');
  if (!loadingText.includes('Loading recent logs…')) throw new Error('expected resource page to render loading state for recent logs');
  if (!loadingFetchCalls.includes(logsURL)) throw new Error('expected resource page to lazy-load logs for the selected resource');
  resolveLogs({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true }, entries: [] }) });
  await settle();

  const unavailableRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: false }, unavailableReason: 'cross_account_not_configured', entries: [] }) });
    throw new Error('unexpected unavailable fetch url: ' + url);
  });
  const unavailableText = collectText(unavailableRun.document.getElementById('accounts')).join(' | ');
  if (!unavailableText.includes('Recent logs are unavailable for this resource.')) throw new Error('expected resource page unavailable recent logs state');
  if (!unavailableText.includes('Cross-account access is not configured for this deployment.')) throw new Error('expected unavailable recent logs state to surface friendly provider reason');
  if (!unavailableText.includes('Lookup region')) throw new Error('expected unavailable recent logs state to label lookup region');
  if (!unavailableText.includes('ap-northeast-2')) throw new Error('expected unavailable recent logs state to show lookup region value');
  if (!unavailableText.includes('Control account ID')) throw new Error('expected unavailable recent logs state to label control account');
  if (!unavailableText.includes('123456789012')) throw new Error('expected unavailable recent logs state to show control account');
  if (!unavailableText.includes('Target account ID')) throw new Error('expected unavailable recent logs state to label target account');
  if (!unavailableText.includes('210987654321')) throw new Error('expected unavailable recent logs state to show target account');
  if (!unavailableText.includes('Cross-account lookup')) throw new Error('expected unavailable recent logs state to label cross-account lookup');
  if (!unavailableText.includes('Observer role configured')) throw new Error('expected unavailable recent logs state to label observer role configuration');
  if (!unavailableText.includes('False')) throw new Error('expected unavailable recent logs state to show observer role not configured');

  const emptyRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true }, entries: [] }) });
    throw new Error('unexpected empty fetch url: ' + url);
  });
  const emptyText = collectText(emptyRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No recent logs are available for this resource.')) throw new Error('expected resource page empty recent logs state');
  if (!emptyText.includes('Lookup region')) throw new Error('expected empty recent logs state to label lookup region');
  if (!emptyText.includes('Cross-account lookup')) throw new Error('expected empty recent logs state to label cross-account lookup');
  if (!emptyText.includes('Observer role configured')) throw new Error('expected empty recent logs state to label observer role configuration');
  if (!emptyText.includes('False')) throw new Error('expected empty recent logs state to show same-account lookup');
  if (!emptyText.includes('True')) throw new Error('expected empty recent logs state to show observer role configured');

  const failedStatusRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: false, status: 503, json: () => Promise.resolve({}) });
    throw new Error('unexpected failed-status fetch url: ' + url);
  });
  await settle();
  const failedStatusText = collectText(failedStatusRun.document.getElementById('accounts')).join(' | ');
  if (!failedStatusText.includes('Failed to load recent logs: resource logs request failed (503)')) throw new Error('expected resource page failed recent logs state for non-2xx responses');

  const rejectedRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.reject(new Error('<img src=x onerror=1> upstream timeout'));
    throw new Error('unexpected rejected fetch url: ' + url);
  });
  await settle();
  const rejectedText = collectText(rejectedRun.document.getElementById('accounts')).join(' | ');
  if (!rejectedText.includes('Failed to load recent logs: <img src=x onerror=1> upstream timeout')) throw new Error('expected resource page failed recent logs state for rejected fetches');
  const rejectedImages = collectNodes(rejectedRun.document.getElementById('accounts'), (node) => node.tagName === 'IMG');
  if (rejectedImages.length !== 0) throw new Error('expected rejected recent-log fetch errors to remain text content rather than parsed html');

  const populatedRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true }, entries: [{ timestamp: '2026-04-16T07:11:00Z', stream: 'codedeploy/bluegreen/1', message: '<img src=x onerror=1> rollback check passed' }] }) });
    throw new Error('unexpected populated fetch url: ' + url);
  });
  await settle();
  const populatedText = collectText(populatedRun.document.getElementById('accounts')).join(' | ');
  if (!populatedText.includes('Logs source')) throw new Error('expected populated recent logs section to show source');
  if (!populatedText.includes('cloudwatch')) throw new Error('expected populated recent logs section to show source value');
  if (!populatedText.includes('Lookup region')) throw new Error('expected populated recent logs section to label lookup region');
  if (!populatedText.includes('Control account ID')) throw new Error('expected populated recent logs section to label control account');
  if (!populatedText.includes('Target account ID')) throw new Error('expected populated recent logs section to label target account');
  if (!populatedText.includes('Cross-account lookup')) throw new Error('expected populated recent logs section to label cross-account lookup');
  if (!populatedText.includes('Observer role configured')) throw new Error('expected populated recent logs section to label observer role configuration');
  if (populatedText.includes('Backing resource type')) throw new Error('expected direct recent-log sources to avoid backing resource metadata labels');
  if (populatedText.includes('Backing resource name')) throw new Error('expected direct recent-log sources to avoid backing resource metadata values');
  if (!populatedText.includes('Log stream · codedeploy/bluegreen/1')) throw new Error('expected populated recent logs section to show stream');
  if (!populatedText.includes('Log timestamp · 2026-04-16T07:11:00Z')) throw new Error('expected populated recent logs section to show timestamp');
  if (!populatedText.includes('<img src=x onerror=1> rollback check passed')) throw new Error('expected populated recent logs section to render raw log message text safely');
  const images = collectNodes(populatedRun.document.getElementById('accounts'), (node) => node.tagName === 'IMG');
  if (images.length !== 0) throw new Error('expected log messages to remain text content rather than parsed html');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageLiveInspectionWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	ecsDetailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal ecs detail body: %v", err)
	}

	unsupportedDetailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				{"name": "payments-api-staging-blue", "resource": "blueTaskSetTargetGroup", "description": "Blue task-set target group"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal unsupported detail body: %v", err)
	}

	asgDetailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-prod",
		"accountName":  "prod-core",
		"awsAccountId": "123456789012",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-prod-candidate", "resource": "candidateAutoScalingGroup", "description": "Candidate capacity"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal asg detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const ecsDetailBody = ` + string(ecsDetailJSON) + `;
const unsupportedDetailBody = ` + string(unsupportedDetailJSON) + `;
const asgDetailBody = ` + string(asgDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }

const ecsResourceID = 'ecsService::payments-api-staging-service::Primary ECS service';
const summaryResourceID = 'codeDeployDeploymentGroup::payments-api-staging Deployment Group::Linear traffic shifting deployment group';
const blueTaskSetTargetGroupResourceID = 'blueTaskSetTargetGroup::payments-api-staging-blue::Blue task-set target group';
const weightedListenerRuleResourceID = 'productionListenerRule::payments-api-staging-prod-listener::Weighted listener rule';
const asgResourceID = 'candidateAutoScalingGroup::payments-api-prod-candidate::Candidate capacity';
const bootstrapResourceID = 'userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity';
const trafficShiftPlanResourceID = 'trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan';

async function renderWithFetch(search, fetchImpl, settleCount = 2) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  for (let i = 0; i < settleCount; i += 1) await settle();
  return { document, window };
}

(async () => {
  const ecsDeploymentURL = '/v1/overview/deployments/payments-api-staging';
  const ecsLogsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service';
  const ecsInspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service';

  let resolveInspection;
  const loadingFetchCalls = [];
  const loadingRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(ecsResourceID), (url) => {
    loadingFetchCalls.push(String(url));
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(ecsDetailBody) });
    if (url === ecsLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === ecsInspectionURL) return new Promise((resolve) => { resolveInspection = resolve; });
    throw new Error('unexpected loading fetch url: ' + url);
  }, 1);
  const loadingText = collectText(loadingRun.document.getElementById('accounts')).join(' | ');
  if (!loadingText.includes('Live inspection')) throw new Error('expected resource page to render live inspection section heading while loading');
  if (!loadingText.includes('Loading live inspection…')) throw new Error('expected resource page to render loading state for live inspection');
  if (!loadingFetchCalls.includes(ecsInspectionURL)) throw new Error('expected resource page to lazy-load live inspection for the selected resource');
  resolveInspection({ ok: true, json: () => Promise.resolve({ available: true, source: 'ecs_service', lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true }, ecsService: { clusterArn: 'arn:aws:ecs:ap-northeast-2:210987654321:cluster/payments-staging', serviceArn: 'arn:aws:ecs:ap-northeast-2:210987654321:service/payments-staging/payments-api-staging-service', serviceName: 'payments-api-staging-service', status: 'ACTIVE', launchType: 'FARGATE', desiredCount: 2, runningCount: 2, pendingCount: 0, minimumHealthyPercent: 100, maximumPercent: 200, healthCheckGracePeriodSeconds: 45, taskDefinition: 'payments-api:42', deployments: [], events: [] } }) });
  await settle();

  const summaryLogsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
  const summaryInspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
  const summaryRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent(summaryResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(unsupportedDetailBody) });
    if (url === summaryLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === summaryInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'rollout_execution_summary', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: false }, trafficRoutingResource: { kind: 'codeDeployDeploymentGroup', displayName: 'CodeDeploy deployment group', inspectionMode: 'summary_only', summary: 'Linear traffic shifting deployment group' } }) });
    throw new Error('unexpected summary fetch url: ' + url);
  });
  const summaryText = collectText(summaryRun.document.getElementById('accounts')).join(' | ');
  if (!summaryText.includes('Rollout summary')) throw new Error('expected traffic-routing inspection to render rollout summary heading');
  if (!summaryText.includes('CodeDeploy deployment group')) throw new Error('expected traffic-routing inspection to render display name');
  if (!summaryText.includes('Rollout summary only')) throw new Error('expected traffic-routing inspection to render summary inspection mode');
  if (!summaryText.includes('Linear traffic shifting deployment group')) throw new Error('expected traffic-routing inspection to render controller summary');
  if (!summaryText.includes('Lookup region')) throw new Error('expected traffic-routing inspection to label lookup context');
  if (!summaryText.includes('Observer role configured')) throw new Error('expected traffic-routing inspection to label observer role configuration');
  if (!summaryText.includes('False')) throw new Error('expected traffic-routing inspection to show missing observer role configuration');

  const blueTargetGroupLogsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue';
  const blueTargetGroupInspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue';
  const blueTargetGroupRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=' + encodeURIComponent(blueTaskSetTargetGroupResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(unsupportedDetailBody) });
    if (url === blueTargetGroupLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === blueTargetGroupInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'traffic_routing_aws',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true },
      trafficRoutingResource: {
        kind: 'blueTaskSetTargetGroup',
        displayName: 'Blue task-set target group',
        inspectionMode: 'live',
        summary: 'Blue replacement task-set target group receiving test traffic before promotion.',
        targetGroup: {
          targetGroupName: 'payments-api-staging-blue',
          targetGroupArn: 'arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:targetgroup/payments-api-staging-blue/abc123',
          protocol: 'HTTP',
          port: 8080,
          targetType: 'ip',
          healthCheckProtocol: 'HTTP',
          healthCheckPort: 'traffic-port',
          healthCheckPath: '/healthz',
          healthCheckIntervalSeconds: 30,
          healthCheckTimeoutSeconds: 5,
          healthyThresholdCount: 2,
          unhealthyThresholdCount: 3,
          loadBalancerArns: ['arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:loadbalancer/app/payments-api/abc'],
          targetHealthCounts: { healthy: 2, unhealthy: 1, initial: 1 },
          targets: [{ targetId: '10.0.3.12', port: 8080, state: 'healthy', reason: 'Target.ResponseCodeMismatch', description: '<script>not html</script> remains text' }],
        },
      },
    }) });
    throw new Error('unexpected blue target-group fetch url: ' + url);
  });
  const blueTargetGroupText = collectText(blueTargetGroupRun.document.getElementById('accounts')).join(' | ');
  if (!blueTargetGroupText.includes('Live AWS inspection')) throw new Error('expected live target-group inspection mode to render live copy');
  if (!blueTargetGroupText.includes('Target group live details')) throw new Error('expected live target-group details section');
  if (!blueTargetGroupText.includes('Target group name')) throw new Error('expected live target-group name label');
  if (!blueTargetGroupText.includes('payments-api-staging-blue')) throw new Error('expected live target-group name value');
  if (!blueTargetGroupText.includes('Protocol / port')) throw new Error('expected live target-group protocol/port label');
  if (!blueTargetGroupText.includes('HTTP / 8080')) throw new Error('expected live target-group protocol/port value');
  if (!blueTargetGroupText.includes('Health check path')) throw new Error('expected live target-group health-check label');
  if (!blueTargetGroupText.includes('/healthz')) throw new Error('expected live target-group health-check path value');
  if (!blueTargetGroupText.includes('Target health counts')) throw new Error('expected live target-group health-count label');
  if (!blueTargetGroupText.includes('healthy 2 · unhealthy 1 · initial 1')) throw new Error('expected live target-group health-count value');
  if (!blueTargetGroupText.includes('Registered target health')) throw new Error('expected live target-group registered targets section');
  if (!blueTargetGroupText.includes('Health state · healthy')) throw new Error('expected live target health row');
  if (!blueTargetGroupText.includes('<script>not html</script> remains text')) throw new Error('expected target description to remain literal text');
  const blueTargetGroupScripts = collectNodes(blueTargetGroupRun.document.getElementById('accounts'), (node) => node.tagName === 'SCRIPT');
  if (blueTargetGroupScripts.length !== 0) throw new Error('expected target-group live details to use safe text rendering');

  const weightedListenerRuleLogsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=productionListenerRule&resourceName=payments-api-staging-prod-listener';
  const weightedListenerRuleInspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-staging-prod-listener';
  const weightedListenerRuleRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=productionListenerRule&resourceId=' + encodeURIComponent(weightedListenerRuleResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(unsupportedDetailBody) });
    if (url === weightedListenerRuleLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'no_log_source', entries: [] }) });
    if (url === weightedListenerRuleInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'rollout_execution_summary', lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: false }, trafficRoutingResource: { kind: 'productionListenerRule', displayName: 'Production listener rule', inspectionMode: 'summary_only', summary: 'Weighted listener rule' } }) });
    throw new Error('unexpected weighted listener rule fetch url: ' + url);
  });
  const weightedListenerRuleText = collectText(weightedListenerRuleRun.document.getElementById('accounts')).join(' | ');
  if (!weightedListenerRuleText.includes('Rollout summary')) throw new Error('expected weighted listener rule inspection to render rollout summary heading');
  if (!weightedListenerRuleText.includes('Production listener rule')) throw new Error('expected weighted listener rule inspection to render display name');
  if (!weightedListenerRuleText.includes('Rollout summary only')) throw new Error('expected weighted listener rule inspection to render summary inspection mode');
  if (!weightedListenerRuleText.includes('Weighted listener rule')) throw new Error('expected weighted listener rule inspection to render controller summary');
  if (!weightedListenerRuleText.includes('This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.')) throw new Error('expected weighted listener rule recent logs to explain no-log-source state');

  const liveWeightedListenerRuleRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=productionListenerRule&resourceId=' + encodeURIComponent(weightedListenerRuleResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(unsupportedDetailBody) });
    if (url === weightedListenerRuleLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'no_log_source', entries: [] }) });
    if (url === weightedListenerRuleInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'traffic_routing_aws',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true },
      trafficRoutingResource: {
        kind: 'productionListenerRule',
        displayName: 'Production listener rule',
        inspectionMode: 'live',
        summary: 'Production listener rule operators inspect while Tenet updates weighted forwarding during rollout',
        listenerRule: {
          ruleArn: 'arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef',
          priority: '10',
          isDefault: false,
          conditions: [{ field: 'path-pattern', values: ['/api/*', '<script>still text</script>'] }],
          actions: [{ type: 'forward', order: 1, targetGroups: [
            { targetGroupArn: 'arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:targetgroup/payments-active/abc123', weight: 80 },
            { targetGroupArn: 'arn:aws:elasticloadbalancing:ap-northeast-2:210987654321:targetgroup/payments-candidate/def456', weight: 20 },
          ] }],
        },
      },
    }) });
    throw new Error('unexpected live weighted listener rule fetch url: ' + url);
  });
  const liveWeightedListenerRuleText = collectText(liveWeightedListenerRuleRun.document.getElementById('accounts')).join(' | ');
  if (!liveWeightedListenerRuleText.includes('Live AWS inspection')) throw new Error('expected live listener-rule inspection mode to render live copy');
  if (!liveWeightedListenerRuleText.includes('Listener rule live details')) throw new Error('expected live listener-rule details section');
  if (!liveWeightedListenerRuleText.includes('Rule ARN')) throw new Error('expected live listener-rule ARN label');
  if (!liveWeightedListenerRuleText.includes('Priority')) throw new Error('expected live listener-rule priority label');
  if (!liveWeightedListenerRuleText.includes('Default rule')) throw new Error('expected live listener-rule default label');
  if (!liveWeightedListenerRuleText.includes('False')) throw new Error('expected live listener-rule default value');
  if (!liveWeightedListenerRuleText.includes('Listener rule conditions')) throw new Error('expected live listener-rule conditions section');
  if (!liveWeightedListenerRuleText.includes('path-pattern')) throw new Error('expected live listener-rule condition field');
  if (!liveWeightedListenerRuleText.includes('Values · /api/*, <script>still text</script>')) throw new Error('expected live listener-rule condition values to render as text');
  if (liveWeightedListenerRuleText.includes('No listener-rule conditions are currently reported for this rule.')) throw new Error('expected a single live listener-rule condition to render instead of the empty state');
  const liveWeightedListenerRuleConditionItems = collectNodes(liveWeightedListenerRuleRun.document.getElementById('accounts'), (node) => node.tagName === 'LI' && collectText(node).join(' | ').includes('path-pattern'));
  if (liveWeightedListenerRuleConditionItems.length !== 1) throw new Error('expected exactly one live listener-rule condition row, got ' + liveWeightedListenerRuleConditionItems.length);
  if (!liveWeightedListenerRuleText.includes('Listener rule actions')) throw new Error('expected live listener-rule actions section');
  if (!liveWeightedListenerRuleText.includes('forward')) throw new Error('expected live listener-rule action type');
  if (!liveWeightedListenerRuleText.includes('Order · 1')) throw new Error('expected live listener-rule action order');
  if (!liveWeightedListenerRuleText.includes('targetgroup/payments-active/abc123 · weight 80')) throw new Error('expected live listener-rule active target weight');
  if (!liveWeightedListenerRuleText.includes('targetgroup/payments-candidate/def456 · weight 20')) throw new Error('expected live listener-rule candidate target weight');
  const liveWeightedListenerRuleScripts = collectNodes(liveWeightedListenerRuleRun.document.getElementById('accounts'), (node) => node.tagName === 'SCRIPT');
  if (liveWeightedListenerRuleScripts.length !== 0) throw new Error('expected listener-rule live details to use safe text rendering');

  const emptyRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(ecsResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(ecsDetailBody) });
    if (url === ecsLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === ecsInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'ecs_service', lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true } }) });
    throw new Error('unexpected empty fetch url: ' + url);
  });
  const emptyText = collectText(emptyRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No live inspection details are available for this resource yet.')) throw new Error('expected empty inspection state');
  if (!emptyText.includes('Inspection source')) throw new Error('expected empty inspection state to show inspection source');
  if (!emptyText.includes('ecs_service')) throw new Error('expected empty inspection state to show inspection source value');

  const ecsSupportedRun = await renderWithFetch('?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(ecsResourceID), (url) => {
    if (url === ecsDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(ecsDetailBody) });
    if (url === ecsLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === ecsInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'ecs_service',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true },
      ecsService: {
        clusterArn: 'arn:aws:ecs:ap-northeast-2:210987654321:cluster/payments-staging',
        serviceArn: 'arn:aws:ecs:ap-northeast-2:210987654321:service/payments-staging/payments-api-staging-service',
        serviceName: 'payments-api-staging-service',
        status: 'ACTIVE',
        launchType: 'FARGATE',
        desiredCount: 2,
        runningCount: 2,
        pendingCount: 0,
        minimumHealthyPercent: 100,
        maximumPercent: 200,
        healthCheckGracePeriodSeconds: 45,
        taskDefinition: 'payments-api:42',
        deployments: [{ id: 'ecs-svc/123', status: 'PRIMARY', rolloutState: 'COMPLETED', rolloutStateReason: 'steady state', taskDefinition: 'payments-api:42', desiredCount: 2, runningCount: 2, pendingCount: 0, createdAt: '2026-04-18T00:10:00Z', updatedAt: '2026-04-18T00:11:00Z' }],
        events: [{ createdAt: '2026-04-18T00:12:00Z', message: '<img src=x onerror=1> service reached steady state' }],
      },
    }) });
    throw new Error('unexpected ecs supported fetch url: ' + url);
  });
  const ecsSupportedText = collectText(ecsSupportedRun.document.getElementById('accounts')).join(' | ');
  if (!ecsSupportedText.includes('Service name')) throw new Error('expected ECS inspection to render service name label');
  if (!ecsSupportedText.includes('payments-api-staging-service')) throw new Error('expected ECS inspection to render service name value');
  if (!ecsSupportedText.includes('Minimum healthy percent')) throw new Error('expected ECS inspection to render minimum healthy percent label');
  if (!ecsSupportedText.includes('100%')) throw new Error('expected ECS inspection to render minimum healthy percent value');
  if (!ecsSupportedText.includes('Maximum percent')) throw new Error('expected ECS inspection to render maximum percent label');
  if (!ecsSupportedText.includes('200%')) throw new Error('expected ECS inspection to render maximum percent value');
  if (!ecsSupportedText.includes('Health check grace period')) throw new Error('expected ECS inspection to render health check grace period label');
  if (!ecsSupportedText.includes('45 seconds')) throw new Error('expected ECS inspection to render health check grace period value');
  if (!ecsSupportedText.includes('Service deployments')) throw new Error('expected ECS inspection to render deployments section');
  if (!ecsSupportedText.includes('Deployment status · PRIMARY')) throw new Error('expected ECS inspection to render deployment status');
  if (!ecsSupportedText.includes('Rollout state · COMPLETED')) throw new Error('expected ECS inspection to render deployment rollout state');
  if (!ecsSupportedText.includes('Desired/running/pending · 2 / 2 / 0')) throw new Error('expected ECS inspection to render deployment counts');
  if (!ecsSupportedText.includes('Recent ECS service events')) throw new Error('expected ECS inspection to render events section');
  if (!ecsSupportedText.includes('Event timestamp · 2026-04-18T00:12:00Z')) throw new Error('expected ECS inspection to render event timestamp');
  if (!ecsSupportedText.includes('<img src=x onerror=1> service reached steady state')) throw new Error('expected ECS inspection to render event text safely');
  const ecsImages = collectNodes(ecsSupportedRun.document.getElementById('accounts'), (node) => node.tagName === 'IMG');
  if (ecsImages.length !== 0) throw new Error('expected ECS inspection event text to remain text content rather than parsed html');

  const asgDeploymentURL = '/v1/overview/deployments/payments-api-prod';
  const asgLogsURL = '/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate';
  const asgInspectionURL = '/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate';
  const asgSupportedRun = await renderWithFetch('?page=resource&deployment=payments-api-prod&resourceType=candidateAutoScalingGroup&resourceId=' + encodeURIComponent(asgResourceID), (url) => {
    if (url === asgDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(asgDetailBody) });
    if (url === asgLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    if (url === asgInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'auto_scaling_group',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true },
      autoScalingGroup: {
        autoScalingGroupArn: 'arn:aws:autoscaling:ap-northeast-2:123456789012:autoScalingGroup:*:autoScalingGroupName/payments-api-prod-candidate',
        autoScalingGroupName: 'payments-api-prod-candidate',
        minSize: 2,
        maxSize: 8,
        desiredCapacity: 4,
        defaultCooldown: 300,
        healthCheckType: 'EC2',
        healthCheckGracePeriod: 120,
        launchTemplate: {
          id: 'lt-abc123',
          name: 'payments-api-prod-candidate',
          version: '$Default',
          imageId: 'ami-abc123',
          instanceType: 'm6i.large',
        },
        mixedInstancesPolicy: {
          launchTemplate: {
            id: 'lt-base123',
            name: 'payments-api-prod-candidate-base',
            version: '7',
          },
          overrides: [{
            instanceType: 'c7g.large',
            weightedCapacity: '2',
            launchTemplate: {
              id: 'lt-arm123',
              name: 'payments-api-prod-candidate-arm',
              version: '3',
            },
          }],
        },
        targetGroupArns: ['arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-candidate/abc123'],
        instances: [{ instanceId: 'i-abc123', availabilityZone: 'ap-northeast-2a', lifecycleState: 'InService', healthStatus: 'Healthy', protectedFromScaleIn: true, weightedCapacity: '1' }],
      },
    }) });
    throw new Error('unexpected asg supported fetch url: ' + url);
  });
  const asgSupportedText = collectText(asgSupportedRun.document.getElementById('accounts')).join(' | ');
  if (!asgSupportedText.includes('Auto Scaling Group name')) throw new Error('expected ASG inspection to render group name label');
  if (!asgSupportedText.includes('payments-api-prod-candidate')) throw new Error('expected ASG inspection to render group name value');
  if (!asgSupportedText.includes('Launch template')) throw new Error('expected ASG inspection to render launch template label');
  if (!asgSupportedText.includes('payments-api-prod-candidate · version $Default · id lt-abc123')) throw new Error('expected ASG inspection to render launch template summary');
  if (!asgSupportedText.includes('Mixed instances launch template')) throw new Error('expected ASG inspection to render mixed instances launch template label');
  if (!asgSupportedText.includes('payments-api-prod-candidate-base · version 7 · id lt-base123')) throw new Error('expected ASG inspection to render mixed instances launch template summary');
  if (!asgSupportedText.includes('Observed image ID')) throw new Error('expected ASG inspection to render observed image id label');
  if (!asgSupportedText.includes('ami-abc123')) throw new Error('expected ASG inspection to render observed image id value');
  if (!asgSupportedText.includes('Observed instance type')) throw new Error('expected ASG inspection to render observed instance type label');
  if (!asgSupportedText.includes('m6i.large')) throw new Error('expected ASG inspection to render observed instance type value');
  if (!asgSupportedText.includes('Target group ARNs')) throw new Error('expected ASG inspection to render target groups section');
  if (!asgSupportedText.includes('arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-candidate/abc123')) throw new Error('expected ASG inspection to render target group ARN');
  if (!asgSupportedText.includes('Mixed instances overrides')) throw new Error('expected ASG inspection to render mixed instances overrides section');
  if (!asgSupportedText.includes('payments-api-prod-candidate-arm · version 3 · id lt-arm123')) throw new Error('expected ASG inspection to render mixed instances override launch template');
  if (!asgSupportedText.includes('Instance type · c7g.large')) throw new Error('expected ASG inspection to render mixed instances override instance type');
  if (!asgSupportedText.includes('Instances')) throw new Error('expected ASG inspection to render instances section');
  if (!asgSupportedText.includes('Lifecycle state · InService')) throw new Error('expected ASG inspection to render lifecycle state');
  if (!asgSupportedText.includes('Protected from scale-in · True')) throw new Error('expected ASG inspection to render protected-from-scale-in');
  if (!asgSupportedText.includes('Weighted capacity · 1')) throw new Error('expected ASG inspection to render weighted capacity');

  const bootstrapLogsURL = '/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=file%3Auserdata%2Fpayments-api-prod.sh';
  const bootstrapInspectionURL = '/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=file%3Auserdata%2Fpayments-api-prod.sh';
  const bootstrapSupportedRun = await renderWithFetch('?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=' + encodeURIComponent(bootstrapResourceID), (url) => {
    if (url === asgDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(asgDetailBody) });
    if (url === bootstrapLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'ec2_console',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true },
      backingResourceType: 'candidateAutoScalingGroup',
      backingResourceName: 'payments-api-prod-candidate',
      entries: [
        { timestamp: '2026-04-16T07:11:30Z', message: 'bootstrap start' },
        { timestamp: '2026-04-16T07:11:45Z', message: 'bootstrap complete' },
      ],
    }) });
    if (url === bootstrapInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'auto_scaling_group',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true },
      userDataBootstrapSource: {
        desiredBootstrapSource: 'file:userdata/payments-api-prod.sh',
        observedBootstrapSource: 'file:userdata/payments-api-prod-legacy.sh',
        backingResourceType: 'candidateAutoScalingGroup',
        backingResourceName: 'payments-api-prod-candidate',
        launchTemplate: {
          id: 'lt-abc123',
          name: 'payments-api-prod-candidate',
          version: '$Default',
          imageId: 'ami-legacy123',
          instanceType: 'm6i.large',
        },
      },
    }) });
    throw new Error('unexpected bootstrap-source fetch url: ' + url);
  });
  const bootstrapSupportedText = collectText(bootstrapSupportedRun.document.getElementById('accounts')).join(' | ');
  const bootstrapRecentLogsSection = collectNodes(bootstrapSupportedRun.document.getElementById('accounts'), (node) => node?.id === 'resource-recent-logs')[0];
  if (!bootstrapRecentLogsSection) throw new Error('expected bootstrap-source resource page to render recent logs section');
  const bootstrapRecentLogsText = collectText(bootstrapRecentLogsSection).join(' | ');
  if (!bootstrapSupportedText.includes('Desired bootstrap source')) throw new Error('expected bootstrap-source inspection to render desired source label');
  if (!bootstrapSupportedText.includes('Logs source')) throw new Error('expected bootstrap-source recent logs to render logs source label');
  if (!bootstrapSupportedText.includes('ec2_console')) throw new Error('expected bootstrap-source recent logs to render logs source value');
  if (!bootstrapSupportedText.includes('bootstrap start')) throw new Error('expected bootstrap-source recent logs to render first console line');
  if (!bootstrapSupportedText.includes('bootstrap complete')) throw new Error('expected bootstrap-source recent logs to render second console line');
  if (!bootstrapRecentLogsText.includes('Backing resource type')) throw new Error('expected bootstrap-source recent logs to render backing resource type label');
  if (!bootstrapRecentLogsText.includes('candidateAutoScalingGroup')) throw new Error('expected bootstrap-source recent logs to render backing resource type value');
  if (!bootstrapRecentLogsText.includes('Backing resource name')) throw new Error('expected bootstrap-source recent logs to render backing resource name label');
  if (!bootstrapRecentLogsText.includes('payments-api-prod-candidate')) throw new Error('expected bootstrap-source recent logs to render backing resource name value');
  if (!bootstrapSupportedText.includes('file:userdata/payments-api-prod.sh')) throw new Error('expected bootstrap-source inspection to render desired source value');
  if (!bootstrapSupportedText.includes('Observed bootstrap source')) throw new Error('expected bootstrap-source inspection to render observed source label');
  if (!bootstrapSupportedText.includes('file:userdata/payments-api-prod-legacy.sh')) throw new Error('expected bootstrap-source inspection to render observed source value');
  if (!bootstrapSupportedText.includes('Backing resource type')) throw new Error('expected bootstrap-source inspection to render backing resource type label');
  if (!bootstrapSupportedText.includes('candidateAutoScalingGroup')) throw new Error('expected bootstrap-source inspection to render backing resource type value');
  if (!bootstrapSupportedText.includes('Observed launch template')) throw new Error('expected bootstrap-source inspection to render observed launch template label');
  if (!bootstrapSupportedText.includes('payments-api-prod-candidate · version $Default · id lt-abc123')) throw new Error('expected bootstrap-source inspection to render observed launch template value');
  if (!bootstrapSupportedText.includes('Observed image ID')) throw new Error('expected bootstrap-source inspection to render observed image id label');
  if (!bootstrapSupportedText.includes('ami-legacy123')) throw new Error('expected bootstrap-source inspection to render observed image id value');
  if (!bootstrapSupportedText.includes('Observed instance type')) throw new Error('expected bootstrap-source inspection to render observed instance type label');
  if (!bootstrapSupportedText.includes('m6i.large')) throw new Error('expected bootstrap-source inspection to render observed instance type value');
  if (!bootstrapSupportedText.includes('Resolved backing resource')) throw new Error('expected bootstrap-source inspection to render backing-resource handoff heading');
  if (!bootstrapSupportedText.includes('Bootstrap source observation')) throw new Error('expected bootstrap-source inspection to render observation summary heading');
  if (!bootstrapSupportedText.includes('Observed launch template bootstrap source differs from the normalized desired bootstrap source.')) throw new Error('expected bootstrap-source inspection to render mismatch summary');
  const bootstrapLinks = collectNodes(bootstrapSupportedRun.document.getElementById('accounts'), (node) => node.tagName === 'A');
  const bootstrapHandoffLink = bootstrapLinks.find((node) => node.textContent === 'Open payments-api-prod-candidate drill-down');
  if (!bootstrapHandoffLink) throw new Error('expected bootstrap-source inspection to render backing-resource handoff link');
  if (bootstrapHandoffLink.href !== '/?page=resource&deployment=payments-api-prod&resourceType=candidateAutoScalingGroup&resourceId=candidateAutoScalingGroup%3A%3Apayments-api-prod-candidate%3A%3ACandidate+capacity') throw new Error('expected bootstrap-source handoff link to reuse normalized resource detail route, got ' + bootstrapHandoffLink.href);
  if (bootstrapHandoffLink.target !== '_self') throw new Error('expected bootstrap-source handoff link to stay inside embedded UI');
  if (bootstrapHandoffLink.rel) throw new Error('expected bootstrap-source handoff link to avoid new-tab rel attributes');

  const bootstrapMissingMatchRun = await renderWithFetch('?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=' + encodeURIComponent(bootstrapResourceID), (url) => {
    if (url === asgDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(asgDetailBody) });
    if (url === bootstrapLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'ec2_console',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true },
      entries: [],
    }) });
    if (url === bootstrapInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'auto_scaling_group',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '123456789012', crossAccount: false, observerRoleConfigured: true },
      userDataBootstrapSource: {
        desiredBootstrapSource: 'file:userdata/payments-api-prod.sh',
        observedBootstrapSource: 'file:userdata/payments-api-prod.sh',
        backingResourceType: 'serviceAutoScalingGroup',
        backingResourceName: 'payments-api-prod-service',
      },
    }) });
    throw new Error('unexpected bootstrap-source mismatch fetch url: ' + url);
  });
  const bootstrapMissingMatchText = collectText(bootstrapMissingMatchRun.document.getElementById('accounts')).join(' | ');
  if (bootstrapMissingMatchText.includes('Resolved backing resource')) throw new Error('expected bootstrap-source inspection to skip handoff when no matching resource metadata is available');

  const trafficShiftPlanLogsURL = '/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing';
  const trafficShiftPlanInspectionURL = '/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=trafficShiftPlan&resourceName=payments-api-prod-weighted-routing';
  const trafficShiftPlanRun = await renderWithFetch('?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=' + encodeURIComponent(trafficShiftPlanResourceID), (url) => {
    if (url === asgDeploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(asgDetailBody) });
    if (url === trafficShiftPlanLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'no_log_source', entries: [] }) });
    if (url === trafficShiftPlanInspectionURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({
      available: true,
      source: 'rollout_execution_summary',
      lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '222222222222', crossAccount: true, observerRoleConfigured: false },
      trafficRoutingResource: {
        kind: 'trafficShiftPlan',
        displayName: 'Traffic shift plan',
        inspectionMode: 'summary_only',
        summary: 'Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s',
      },
    }) });
    throw new Error('unexpected traffic shift plan fetch url: ' + url);
  });
  const trafficShiftPlanText = collectText(trafficShiftPlanRun.document.getElementById('accounts')).join(' | ');
  if (!trafficShiftPlanText.includes('Traffic shift plan')) throw new Error('expected traffic shift plan inspection to render display name');
  if (!trafficShiftPlanText.includes('Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s')) throw new Error('expected traffic shift plan inspection to render summary text');
  if (!trafficShiftPlanText.includes('This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.')) throw new Error('expected traffic shift plan recent logs to explain no-log-source state');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailLazyPanelReadAuthGuidanceWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal resource detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => node ? [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean) : [];
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle(count = 1) { for (let i = 0; i < count; i += 1) { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); } }

const resourceID = 'ecsService::payments-api-staging-service::Primary ECS service';
const initialSearch = '?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(resourceID);
const deploymentURL = '/v1/overview/deployments/payments-api-staging';
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service';
const inspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service';
const unavailableLogsPayload = {
  available: false,
  source: '',
  lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true },
  unavailableReason: 'unsupported_resource_type',
  entries: [],
};
const unavailableInspectionPayload = {
  available: false,
  source: '',
  lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true },
  unavailableReason: 'unsupported_resource_type',
};

const scenarios = [
  {
    name: 'recent logs 401 keeps bookmarkable route and explains trusted front door retry',
    failingPanel: 'logs',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expectedHeading: 'Authenticated operator required',
    expectedMessage: 'Tenet rejected GET /v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service because authenticated identity is required on this operator read route. Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.',
    unexpectedText: 'Failed to load recent logs:',
    buttonLabel: 'Refresh recent logs',
  },
  {
    name: 'recent logs 403 keeps bookmarkable route and explains operator authorization failure',
    failingPanel: 'logs',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expectedHeading: 'Operator access denied',
    expectedMessage: 'Tenet rejected GET /v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    unexpectedText: 'Failed to load recent logs:',
    buttonLabel: 'Refresh recent logs',
  },
  {
    name: 'live inspection 401 keeps bookmarkable route and explains trusted front door retry',
    failingPanel: 'inspection',
    status: 401,
    body: 'unauthenticated: operator identity missing',
    expectedHeading: 'Authenticated operator required',
    expectedMessage: 'Tenet rejected GET /v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service because authenticated identity is required on this operator read route. Tenet does not start a built-in GitHub OAuth login or Tenet-managed browser session from this page; reopen or sign in through the trusted front door for this same bookmarkable URL so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet loads the view.',
    unexpectedText: 'Failed to load live inspection:',
    buttonLabel: 'Refresh live inspection',
  },
  {
    name: 'live inspection 403 keeps bookmarkable route and explains operator authorization failure',
    failingPanel: 'inspection',
    status: 403,
    body: 'forbidden: operator does not have access to this deployment',
    expectedHeading: 'Operator access denied',
    expectedMessage: 'Tenet rejected GET /v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
    unexpectedText: 'Failed to load live inspection:',
    buttonLabel: 'Refresh live inspection',
  },
];

(async () => {
  for (const scenario of scenarios) {
    const document = createDocument();
    const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
    const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
    let detailFetchCount = 0;
    let logsFetchCount = 0;
    let inspectionFetchCount = 0;
    const fetch = (url) => {
      if (url === deploymentURL) {
        detailFetchCount += 1;
        return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
      }
      if (url === logsURL) {
        logsFetchCount += 1;
        if (scenario.failingPanel === 'logs') {
          return Promise.resolve({ ok: false, status: scenario.status, text: () => Promise.resolve(scenario.body) });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve(unavailableLogsPayload) });
      }
      if (url === inspectionURL) {
        inspectionFetchCount += 1;
        if (scenario.failingPanel === 'inspection') {
          return Promise.resolve({ ok: false, status: scenario.status, text: () => Promise.resolve(scenario.body) });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve(unavailableInspectionPayload) });
      }
      throw new Error('unexpected fetch url for ' + scenario.name + ': ' + url);
    };

    new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
    await settle(4);

    const root = document.getElementById('accounts');
    let text = collectText(root).join(' | ');
    if (!text.includes(scenario.expectedHeading)) throw new Error('expected auth heading for ' + scenario.name + ': ' + text);
    if (!text.includes(scenario.expectedMessage)) throw new Error('expected auth guidance message for ' + scenario.name + ': ' + text);
    if (text.includes(scenario.unexpectedText)) throw new Error('expected auth guidance instead of generic panel failure for ' + scenario.name + ': ' + text);
    if (window.location.search !== initialSearch) throw new Error('expected initial resource auth guidance to preserve bookmarkable URL for ' + scenario.name);
    if (detailFetchCount !== 1) throw new Error('expected resource detail to load exactly once for ' + scenario.name + ', got ' + detailFetchCount);
    if (logsFetchCount !== 1) throw new Error('expected recent logs panel to load exactly once for ' + scenario.name + ', got ' + logsFetchCount);
    if (inspectionFetchCount !== 1) throw new Error('expected live inspection panel to load exactly once for ' + scenario.name + ', got ' + inspectionFetchCount);

    let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
    let refresh = buttons.find((node) => node.textContent === scenario.buttonLabel);
    if (!refresh) throw new Error('expected panel refresh button for ' + scenario.name);
    if (refresh.disabled) throw new Error('expected panel refresh button enabled after auth failure for ' + scenario.name);

    refresh.dispatch('click');
    await settle(4);

    text = collectText(root).join(' | ');
    if (!text.includes(scenario.expectedHeading)) throw new Error('expected refresh auth heading for ' + scenario.name + ': ' + text);
    if (!text.includes(scenario.expectedMessage)) throw new Error('expected refresh auth guidance message for ' + scenario.name + ': ' + text);
    if (text.includes(scenario.unexpectedText)) throw new Error('expected refresh auth guidance instead of generic panel failure for ' + scenario.name + ': ' + text);
    if (window.location.search !== initialSearch) throw new Error('expected panel refresh to preserve bookmarkable URL for ' + scenario.name);
    if (detailFetchCount !== 1) throw new Error('expected panel refresh not to reload deployment detail for ' + scenario.name + ', got ' + detailFetchCount);
    if (scenario.failingPanel === 'logs') {
      if (logsFetchCount !== 2) throw new Error('expected recent logs refresh to refetch only that panel for ' + scenario.name + ', got ' + logsFetchCount);
      if (inspectionFetchCount !== 1) throw new Error('expected recent logs refresh not to refetch live inspection for ' + scenario.name + ', got ' + inspectionFetchCount);
    } else {
      if (inspectionFetchCount !== 2) throw new Error('expected live inspection refresh to refetch only that panel for ' + scenario.name + ', got ' + inspectionFetchCount);
      if (logsFetchCount !== 1) throw new Error('expected live inspection refresh not to refetch recent logs for ' + scenario.name + ', got ' + logsFetchCount);
    }

    buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
    refresh = buttons.find((node) => node.textContent === scenario.buttonLabel);
    if (!refresh) throw new Error('expected panel refresh button after refresh for ' + scenario.name);
    if (refresh.disabled) throw new Error('expected panel refresh button re-enabled after refresh for ' + scenario.name);
  }
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageRefreshesInPlaceWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	initialDetailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Original deployment group description"},
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal initial detail body: %v", err)
	}

	refreshedDetailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Updated deployment group description after refresh"},
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal refreshed detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const initialDetailBody = ` + string(initialDetailJSON) + `;
const refreshedDetailBody = ` + string(refreshedDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) current.add(name); else current.delete(name);
        this.className = Array.from(current).join(' ');
      },
    };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION') {
        if (node.selected || this.value === '') this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle(count = 1) { for (let i = 0; i < count; i += 1) { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); } }
const originalResourceID = 'codeDeployDeploymentGroup::payments-api-staging Deployment Group::Original deployment group description';
const deploymentURL = '/v1/overview/deployments/payments-api-staging';
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
const inspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
const initialSearch = '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent(originalResourceID);
const fetchCalls = [];
let deploymentFetchCount = 0;
let resolveRefreshDetail;
const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === deploymentURL) {
    deploymentFetchCount += 1;
    if (deploymentFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetailBody) });
    if (deploymentFetchCount === 2) return new Promise((resolve) => { resolveRefreshDetail = resolve; });
    if (deploymentFetchCount === 3) return Promise.resolve({ ok: false, status: 403, json: () => Promise.resolve({}) });
    throw new Error('unexpected deployment fetch count: ' + deploymentFetchCount);
  }
  if (url === logsURL) {
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', lookupContext: {}, entries: [] }) });
  }
  if (url === inspectionURL) {
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'rollout_execution_summary', lookupContext: {}, trafficRoutingResource: { kind: 'CodeDeploy deployment group', displayName: 'payments-api-staging Deployment Group', inspectionMode: 'summary_only', summary: 'CodeDeploy deployment group summary' } }) });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle(4);
  const root = document.getElementById('accounts');
  let text = collectText(root).join(' | ');
  if (!text.includes('Original deployment group description')) throw new Error('expected initial resource detail to render original bookmarked resource');
  let buttons = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  let refresh = buttons.find((node) => node.textContent === 'Refresh resource detail');
  if (!refresh) throw new Error('expected resource detail refresh button in toolbar');
  if (refresh.disabled) throw new Error('expected resource detail refresh button enabled after initial load');

  refresh.dispatch('click');
  await settle(1);
  buttons = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refresh = buttons.find((node) => node.textContent === 'Refresh resource detail');
  if (!refresh) throw new Error('expected resource detail refresh button while refresh is loading');
  if (!refresh.disabled) throw new Error('expected resource detail refresh button disabled while deployment detail refresh is in flight');
  text = collectText(root).join(' | ');
  if (!text.includes('Refreshing resource detail from GET /v1/overview/deployments/{deploymentName}…')) throw new Error('expected resource detail loading copy during refresh');
  if (!text.includes('Original deployment group description')) throw new Error('expected current resource detail to remain visible during refresh');
  if (window.location.search !== initialSearch) throw new Error('expected resource detail refresh to preserve bookmarkable URL while loading');

  resolveRefreshDetail({ ok: true, json: () => Promise.resolve(refreshedDetailBody) });
  await settle(4);
  text = collectText(root).join(' | ');
  if (!text.includes('Updated deployment group description after refresh')) throw new Error('expected refreshed resource detail to re-resolve bookmarked resource from updated payload');
  if (text.includes('Original deployment group description')) throw new Error('expected refreshed resource detail to replace stale resource metadata');
  if (window.location.search !== initialSearch) throw new Error('expected successful resource detail refresh to preserve bookmarkable URL');
  buttons = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON');
  refresh = buttons.find((node) => node.textContent === 'Refresh resource detail');
  if (!refresh || refresh.disabled) throw new Error('expected resource detail refresh button re-enabled after success');

  refresh.dispatch('click');
  await settle(3);
  text = collectText(root).join(' | ');
  if (!text.includes('Operator access denied')) throw new Error('expected operator-read 403 heading after failed resource detail refresh');
  if (!text.includes('Current resource detail remains visible below.')) throw new Error('expected failed resource detail refresh to keep current detail visible');
  if (!text.includes('Updated deployment group description after refresh')) throw new Error('expected latest successful resource detail to remain visible after failed refresh');
  if (!text.includes('Tenet rejected GET /v1/overview/deployments/payments-api-staging after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.')) throw new Error('expected shared 403 operator-read guidance for resource detail refresh failures');
  if (window.location.search !== initialSearch) throw new Error('expected failed resource detail refresh to preserve bookmarkable URL');
  if (fetchCalls.filter((url) => url === deploymentURL).length !== 3) throw new Error('expected exactly three deployment-detail fetches');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageRefreshControlsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":         "payments-api-staging",
		"accountName":  "staging-core",
		"awsAccountId": "210987654321",
		"region":       "ap-northeast-2",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging-service", "resource": "ecsService", "description": "Primary ECS service"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal refresh detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle(count = 1) { for (let i = 0; i < count; i += 1) { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); } }
const resourceID = 'ecsService::payments-api-staging-service::Primary ECS service';
const deploymentURL = '/v1/overview/deployments/payments-api-staging';
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service';
const inspectionURL = '/v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=ecsService&resourceName=payments-api-staging-service';
const initialSearch = '?page=resource&deployment=payments-api-staging&resourceType=ecsService&resourceId=' + encodeURIComponent(resourceID);
const fetchCalls = [];
let logsFetchCount = 0;
let inspectionFetchCount = 0;
let resolveInitialLogs;
let resolveInitialInspection;
let resolveRefreshLogs;
let resolveRefreshInspection;
const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === deploymentURL) return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === logsURL) {
    logsFetchCount += 1;
    if (logsFetchCount === 1) return new Promise((resolve) => { resolveInitialLogs = resolve; });
    if (logsFetchCount === 2) return new Promise((resolve) => { resolveRefreshLogs = resolve; });
    throw new Error('unexpected logs fetch count: ' + logsFetchCount);
  }
  if (url === inspectionURL) {
    inspectionFetchCount += 1;
    if (inspectionFetchCount === 1) return new Promise((resolve) => { resolveInitialInspection = resolve; });
    if (inspectionFetchCount === 2) return new Promise((resolve) => { resolveRefreshInspection = resolve; });
    throw new Error('unexpected inspection fetch count: ' + inspectionFetchCount);
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle(1);
  const root = document.getElementById('accounts');
  let text = collectText(root).join(' | ');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  let refreshLogs = buttons.find((node) => node.textContent === 'Refresh recent logs');
  let refreshInspection = buttons.find((node) => node.textContent === 'Refresh live inspection');
  if (!refreshLogs || !refreshInspection) throw new Error('expected resource refresh buttons while sections are loading');
  if (!refreshLogs.disabled || !refreshInspection.disabled) throw new Error('expected resource refresh buttons disabled while initial fetches are in flight');
  if (!text.includes('Loading recent logs…')) throw new Error('expected initial loading state for recent logs');
  if (!text.includes('Loading live inspection…')) throw new Error('expected initial loading state for live inspection');

  resolveInitialLogs({ ok: true, json: () => Promise.resolve({
    available: true,
    source: 'cloudwatch',
    lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true },
    entries: [{ timestamp: '2026-04-18T00:15:00Z', stream: 'ecs/payments-api-staging/1', message: 'Initial recent log line' }],
  }) });
  resolveInitialInspection({ ok: false, status: 503, json: () => Promise.resolve({}) });
  await settle(3);

  text = collectText(root).join(' | ');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshLogs = buttons.find((node) => node.textContent === 'Refresh recent logs');
  refreshInspection = buttons.find((node) => node.textContent === 'Refresh live inspection');
  if (!refreshLogs || !refreshInspection) throw new Error('expected resource refresh buttons after initial responses');
  if (refreshLogs.disabled || refreshInspection.disabled) throw new Error('expected resource refresh buttons enabled after initial terminal states');
  if (!text.includes('Initial recent log line')) throw new Error('expected initial recent log payload before refresh');
  if (!text.includes('Failed to load live inspection: resource inspection request failed (503)')) throw new Error('expected initial live inspection error state before refresh');

  refreshLogs.dispatch('click');
  await settle(1);
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshLogs = buttons.find((node) => node.textContent === 'Refresh recent logs');
  refreshInspection = buttons.find((node) => node.textContent === 'Refresh live inspection');
  if (!refreshLogs || !refreshInspection) throw new Error('expected refresh buttons during recent-logs refresh');
  if (!refreshLogs.disabled) throw new Error('expected recent-logs refresh button disabled while its fetch is in flight');
  if (refreshInspection.disabled) throw new Error('expected live-inspection refresh button to stay enabled during recent-logs refresh');
  if (logsFetchCount !== 2) throw new Error('expected recent-logs refresh to bypass cached ready state');
  if (window.location.search !== initialSearch) throw new Error('expected recent-logs refresh to preserve bookmarkable URL');

  resolveRefreshLogs({ ok: true, json: () => Promise.resolve({
    available: true,
    source: 'cloudwatch',
    lookupContext: { region: 'ap-northeast-2', controlAccountId: '123456789012', targetAccountId: '210987654321', crossAccount: true, observerRoleConfigured: true },
    entries: [{ timestamp: '2026-04-18T00:16:00Z', stream: 'ecs/payments-api-staging/2', message: 'Updated recent log line' }],
  }) });
  await settle(3);

  text = collectText(root).join(' | ');
  if (!text.includes('Updated recent log line')) throw new Error('expected recent-logs refresh to render fresh payload');
  if (text.includes('Initial recent log line')) throw new Error('expected recent-logs refresh to replace stale payload');

  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshLogs = buttons.find((node) => node.textContent === 'Refresh recent logs');
  refreshInspection = buttons.find((node) => node.textContent === 'Refresh live inspection');
  if (!refreshLogs || !refreshInspection) throw new Error('expected refresh buttons before live-inspection refresh');
  refreshInspection.dispatch('click');
  await settle(1);
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshLogs = buttons.find((node) => node.textContent === 'Refresh recent logs');
  refreshInspection = buttons.find((node) => node.textContent === 'Refresh live inspection');
  if (!refreshLogs || !refreshInspection) throw new Error('expected refresh buttons during live-inspection refresh');
  if (!refreshInspection.disabled) throw new Error('expected live-inspection refresh button disabled while its fetch is in flight');
  if (refreshLogs.disabled) throw new Error('expected recent-logs refresh button to stay enabled during live-inspection refresh');
  if (inspectionFetchCount !== 2) throw new Error('expected live-inspection refresh to bypass cached error state');
  if (window.location.search !== initialSearch) throw new Error('expected live-inspection refresh to preserve bookmarkable URL');

  resolveRefreshInspection({ ok: true, json: () => Promise.resolve({
    available: true,
    source: 'ecs_service',
    lookupContext: { region: 'ap-northeast-2', controlAccountId: '210987654321', targetAccountId: '210987654321', crossAccount: false, observerRoleConfigured: true },
    ecsService: {
      clusterArn: 'arn:aws:ecs:ap-northeast-2:210987654321:cluster/payments-staging',
      serviceArn: 'arn:aws:ecs:ap-northeast-2:210987654321:service/payments-staging/payments-api-staging-service',
      serviceName: 'payments-api-staging-service',
      status: 'ACTIVE',
      launchType: 'FARGATE',
      desiredCount: 2,
      runningCount: 2,
      pendingCount: 0,
      taskDefinition: 'payments-api:43',
      deployments: [],
      events: [{ createdAt: '2026-04-18T00:17:00Z', message: 'Service reached steady state again' }],
    },
  }) });
  await settle(3);

  text = collectText(root).join(' | ');
  if (!text.includes('Service name')) throw new Error('expected refreshed live inspection payload to render supported fields');
  if (!text.includes('payments-api-staging-service')) throw new Error('expected refreshed live inspection payload to render service name');
  if (!text.includes('Service reached steady state again')) throw new Error('expected refreshed live inspection payload to render fresh event text');
  if (text.includes('Failed to load live inspection: resource inspection request failed (503)')) throw new Error('expected refreshed live inspection payload to replace stale error state');
  if (fetchCalls.filter((url) => url === deploymentURL).length !== 1) throw new Error('expected in-place refresh controls not to reload deployment detail');
  if (fetchCalls.filter((url) => url === logsURL).length !== 2) throw new Error('expected exactly two recent-logs fetches');
  if (fetchCalls.filter((url) => url === inspectionURL).length !== 2) throw new Error('expected exactly two live-inspection fetches');
  if (window.location.search !== initialSearch) throw new Error('expected bookmarkable URL to remain unchanged after both refreshes');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageWithNodeHarnessResolvesDuplicateSameTypeResources(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "staging-core",
		"awsAccountId": "210987654321",
		"deployments": []map[string]any{{
			"name":           "payments-api-staging",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":          "payments-api-staging",
		"accountName":   "staging-core",
		"awsAccountId":  "210987654321",
		"region":        "ap-northeast-2",
		"executionMode": "ecsCodeDeploy",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "prod listener", "resource": "productionListener", "description": "Routes production traffic"},
				{"name": "test listener", "resource": "productionListener", "description": "Routes verification traffic"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=productionListener&resourceId=productionListener%3A%3Atest%20listener%3A%3ARoutes%20verification%20traffic&q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=productionListener&resourceName=test+listener';
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
(async () => {
  await settle();
  await settle();
  if (!fetchCalls.includes('/v1/overview/deployments/payments-api-staging')) throw new Error('expected duplicate-resource page to load deployment detail');
  if (!fetchCalls.includes(logsURL)) throw new Error('expected duplicate-resource page to load recent logs for selected resource');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!summaryText.includes('test listener')) throw new Error('expected resource detail to resolve the intended duplicate resource');
  if (summaryText.includes('prod listener')) throw new Error('expected duplicate resource resolution not to fall back to the first same-type resource');
  if (!accountsText.includes('Resource id')) throw new Error('expected duplicate resource page to render resource id');
  if (!accountsText.includes('productionListener::test listener::Routes verification traffic')) throw new Error('expected duplicate resource page to surface the stable unique resource id');
  if (!accountsText.includes('Routes verification traffic')) throw new Error('expected duplicate resource page to render the matched duplicate resource execution description');
  if (!accountsText.includes('Recent logs are not supported for this resource type.')) throw new Error('expected duplicate resource page to render friendly recent-log unavailable text');
  if (accountsText.includes('Routes production traffic')) throw new Error('expected duplicate resource page not to render the wrong duplicate resource execution description');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceDetailPageWithNodeHarnessResolvesBookmarkedResourceAcrossReorderedExecutionPayloads(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "staging-core",
		"awsAccountId": "210987654321",
		"deployments": []map[string]any{{
			"name":           "payments-api-staging",
			"appRef":         "payments",
			"environmentRef": "staging-apne2",
			"region":         "ap-northeast-2",
			"targetType":     "ecs",
			"strategy":       "linear",
			"executionMode":  "ecsCodeDeploy",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	bookmarkedDetailJSON, err := json.Marshal(map[string]any{
		"name":          "payments-api-staging",
		"accountName":   "staging-core",
		"awsAccountId":  "210987654321",
		"region":        "ap-northeast-2",
		"executionMode": "ecsCodeDeploy",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "prod listener", "resource": "productionListener", "description": "Routes production traffic"},
				{"name": "test listener", "resource": "productionListener", "description": "Routes verification traffic"},
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal bookmarked detail body: %v", err)
	}

	reorderedDetailJSON, err := json.Marshal(map[string]any{
		"name":          "payments-api-staging",
		"accountName":   "staging-core",
		"awsAccountId":  "210987654321",
		"region":        "ap-northeast-2",
		"executionMode": "ecsCodeDeploy",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "Linear traffic shifting deployment group"},
				{"name": "prod listener", "resource": "productionListener", "description": "Routes production traffic"},
				{"name": "test listener", "resource": "productionListener", "description": "Routes verification traffic"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal reordered detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const bookmarkedDetail = ` + string(bookmarkedDetailJSON) + `;
const reorderedDetail = ` + string(reorderedDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
const resourceID = 'productionListener::test listener::Routes verification traffic';
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(search, detailBody) {
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=productionListener&resourceName=test+listener';
  const fetch = (url) => {
    if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  document.getElementById('toolbar').replaceChildren();
  document.getElementById('summary').replaceChildren();
  document.getElementById('accounts').replaceChildren();
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
}
(async () => {
  await render('?page=application&deployment=payments-api-staging', bookmarkedDetail);
  const resourceLinks = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedBookmarkedHref = '/?' + new URLSearchParams({
    page: 'resource',
    deployment: 'payments-api-staging',
    resourceType: 'productionListener',
    resourceId: resourceID,
  }).toString();
  const bookmarkedLink = resourceLinks.find((node) => node.href === expectedBookmarkedHref);
  if (!bookmarkedLink) throw new Error('expected application page to generate bookmarkable stable resource link before reorder');
  if (bookmarkedLink.target !== '_self') throw new Error('expected bookmarked resource link to stay in embedded UI flow');

  await render('?page=resource&deployment=payments-api-staging&resourceType=productionListener&resourceId=' + encodeURIComponent(resourceID), reorderedDetail);
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const expectedBackHref = '/?' + new URLSearchParams({
    page: 'application',
    deployment: 'payments-api-staging',
  }).toString();
  const backLink = toolbarLinks.find((node) => node.href === expectedBackHref);
  if (!backLink || backLink.target !== '_self' || backLink.rel) throw new Error('expected reordered resource page to preserve self-navigation back link');
  if (!summaryText.includes('test listener')) throw new Error('expected bookmarked resource detail to resolve the intended resource after reorder');
  if (summaryText.includes('prod listener')) throw new Error('expected bookmarked resource detail not to drift to a different resource after reorder');
  if (!accountsText.includes(resourceID)) throw new Error('expected reordered resource page to preserve stable resource identity');
  if (!accountsText.includes('Routes verification traffic')) throw new Error('expected reordered resource page to render the intended execution description');
  if (accountsText.includes('Routes production traffic')) throw new Error('expected reordered resource page not to render the wrong execution description');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestEmbeddedUIResourceDetailBookmarkResolutionToleratesDescriptionDriftWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	updatedDetailJSON, err := json.Marshal(map[string]any{
		"name":          "payments-api-staging",
		"accountName":   "staging-core",
		"awsAccountId":  "210987654321",
		"region":        "ap-northeast-2",
		"executionMode": "ecsCodeDeploy",
		"rollout": map[string]any{
			"execution": []map[string]any{
				{"name": "payments-api-staging Deployment Group", "resource": "codeDeployDeploymentGroup", "description": "CodeDeploy deployment group coordinating the latest linear traffic shift"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal updated detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(updatedDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const oldResourceID = 'codeDeployDeploymentGroup::payments-api-staging Deployment Group::Linear traffic shifting deployment group';
(async () => {
  const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent(oldResourceID), hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetchCalls = [];
  const expectedLogsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === expectedLogsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  if (!fetchCalls.includes('/v1/overview/deployments/payments-api-staging')) throw new Error('expected description-drift resource page to load deployment detail');
  if (!fetchCalls.includes(expectedLogsURL)) throw new Error('expected description-drift resource page to load recent logs using the canonical resource name');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!summaryText.includes('payments-api-staging Deployment Group')) throw new Error('expected description-drift bookmark to resolve the canonical resource name');
  if (!summaryText.includes('CodeDeploy deployment group coordinating the latest linear traffic shift')) throw new Error('expected description-drift bookmark to resolve the updated resource description');
  if (summaryText.includes('No linked resource metadata is available for this selection.')) throw new Error('expected description-drift bookmark not to fall back to placeholder metadata');
  if (!accountsText.includes('codeDeployDeploymentGroup::payments-api-staging Deployment Group::CodeDeploy deployment group coordinating the latest linear traffic shift')) throw new Error('expected description-drift bookmark to surface the current canonical resource id');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestResourceAndWorkspaceDetailPagesRenderMissingGenerationsAsEmDashWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	t.Run("resource detail treats zero generations as missing", func(t *testing.T) {
		accountsJSON, err := json.Marshal([]map[string]any{{
			"name":         "staging-core",
			"awsAccountId": "210987654321",
			"deployments": []map[string]any{{
				"name": "payments-api-staging",
			}},
		}})
		if err != nil {
			t.Fatalf("marshal sample accounts: %v", err)
		}

		detailJSON, err := json.Marshal(map[string]any{
			"name":             "payments-api-staging",
			"accountName":      "staging-core",
			"awsAccountId":     "210987654321",
			"region":           "ap-northeast-2",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"sync":             map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
			"health":           map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
			"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v2"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1"},
			"runtimeSummary": map[string]any{
				"source":             "runtime-agent",
				"timestamp":          "2026-04-16T07:10:00Z",
				"targetType":         "ecs",
				"strategy":           "linear",
				"executionMode":      "ecsCodeDeploy",
				"historyCount":       0,
				"desiredGeneration":  0,
				"observedGeneration": 0,
			},
			"rollout": map[string]any{
				"phase":              "Accepted",
				"currentStep":        "ShiftTrafficLinear",
				"desiredGeneration":  0,
				"observedGeneration": 0,
				"execution": []map[string]any{{
					"name":        "payments-api-staging Deployment Group",
					"resource":    "codeDeployDeploymentGroup",
					"description": "Linear traffic shifting deployment group",
				}},
			},
		})
		if err != nil {
			t.Fatalf("marshal resource detail body: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent('codeDeployDeploymentGroup::payments-api-staging Deployment Group::Linear traffic shifting deployment group'), hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
const fetch = (url) => {
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
(async () => {
  await settle();
  await settle();
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('Runtime generations')) throw new Error('expected resource page runtime generations label');
  if (!accountsText.includes('Rollout generations')) throw new Error('expected resource page rollout generations label');
  if (!accountsText.includes('desired — · observed —')) throw new Error('expected resource page to render missing generations as em dash');
  if (accountsText.includes('desired 0 · observed 0')) throw new Error('expected resource page not to render synthetic zero generations');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("workspace detail treats zero generations as missing", func(t *testing.T) {
		detailJSON, err := json.Marshal(map[string]any{
			"name":             "api-prod",
			"appRef":           "payments",
			"environmentRef":   "prod",
			"region":           "us-west-2",
			"accountName":      "acct-a",
			"awsAccountId":     "111111111111",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"sync":             map[string]any{"state": "Drifted", "reason": "Service target group differs from desired"},
			"health":           map[string]any{"overall": "Unhealthy", "summary": "traffic steady but target registration is inconsistent"},
			"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v9"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v7"},
			"runtimeSummary": map[string]any{
				"source":             "deployment-controller",
				"timestamp":          "2026-04-16T06:50:00Z",
				"targetType":         "ecs",
				"strategy":           "linear",
				"executionMode":      "ecsCodeDeploy",
				"desiredGeneration":  0,
				"observedGeneration": 0,
				"historyCount":       0,
				"changes":            []string{},
				"conditions":         []map[string]any{},
			},
			"rollout": map[string]any{
				"execution": []map[string]any{{
					"name":        "prod-router",
					"resource":    "aws_lb_listener_rule.payments_prod",
					"description": "Routes production traffic to the stable target group",
				}},
			},
			"conditions": []map[string]any{},
		})
		if err != nil {
			t.Fatalf("marshal workspace detail body: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '?page=drift&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
(async () => {
  await settle();
  await settle();
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  if (!summaryText.includes('Runtime generations')) throw new Error('expected workspace summary runtime generations label');
  if (!summaryText.includes('desired — · observed —')) throw new Error('expected workspace summary to render missing generations as em dash');
  if (summaryText.includes('desired 0 · observed 0')) throw new Error('expected workspace summary not to render synthetic zero generations');
})();
`

		runNodeHarness(t, nodeProgram)
	})
}

func TestDeploymentWorkspaceDriftViewWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	managedDetailJSON, err := json.Marshal(map[string]any{
		"name":               "api-managed",
		"appRef":             "payments",
		"environmentRef":     "prod",
		"region":             "us-west-2",
		"accountName":        "acct-a",
		"awsAccountId":       "111111111111",
		"targetType":         "ecs",
		"strategy":           "linear",
		"executionMode":      "ecsCodeDeploy",
		"desiredGeneration":  9,
		"observedGeneration": 7,
		"sync":               map[string]any{"state": "Drifted", "reason": "Managed drift rows were published from the deployment detail"},
		"health":             map[string]any{"overall": "Unhealthy", "summary": "weighted routing is still inconsistent"},
		"desiredArtifact":    map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v7"},
		"managedDrift": []map[string]any{
			{
				"field":    "listener.weight",
				"desired":  "stable=100 canary=0",
				"actual":   "<script>alert('xss')</script> stable=80 canary=20",
				"severity": "high",
			},
			{
				"field":    "service.taskDefinition",
				"desired":  "payments:42",
				"actual":   "payments:41",
				"severity": "medium",
			},
			{
				"field":       "runtimeContext.capacity.desired",
				"desired":     "6",
				"actual":      "<script>alert('capacity')</script> 4",
				"severity":    "low",
				"resourceRef": map[string]any{"resourceType": "candidateAutoScalingGroup", "resourceName": "api-managed-candidate"},
			},
			{
				"field":    "runtimeContext.launchTemplate.imageId",
				"desired":  "ami-1234567890abcdef0",
				"actual":   "<script>alert('ami')</script> ami-fffffffffffffffff",
				"severity": "high",
			},
			{
				"field":    "runtimeContext.launchTemplate.instanceType",
				"desired":  "t3.medium",
				"actual":   "<script>alert('instance-type')</script> m6i.large",
				"severity": "medium",
			},
		},
		"runtimeSummary": map[string]any{
			"source":             "deployment-controller",
			"timestamp":          "2026-04-16T06:48:00Z",
			"targetType":         "ec2",
			"strategy":           "canary",
			"executionMode":      "asgWeightedTargetGroups",
			"desiredGeneration":  9,
			"observedGeneration": 7,
			"historyCount":       2,
			"changes":            []string{"managed drift row emitted"},
			"deployedArtifact":   map[string]any{"binary": "s3://artifacts.example/payments/api-managed-r17"},
			"conditions":         []map[string]any{{"type": "DriftDetected", "status": true, "reason": "TargetGroupMismatch", "message": "Runtime summary still shows drift"}},
		},
		"rollout": map[string]any{
			"execution": []map[string]any{{
				"name":        "api-managed-candidate",
				"resource":    "candidateAutoScalingGroup",
				"description": "Candidate Auto Scaling Group Tenet creates for managed drift evidence",
			}, {
				"name":        "prod-router",
				"resource":    "aws_lb_listener_rule.payments_prod",
				"description": "Routes production traffic to the stable target group",
			}},
		},
		"conditions": []map[string]any{{
			"type":    "DriftDetected",
			"status":  true,
			"reason":  "TargetGroupMismatch",
			"message": "Fallback-derived drift rows should be suppressed when managedDrift is present",
		}, {
			"type":    "Healthy",
			"status":  false,
			"reason":  "TargetDeregistrationLag",
			"message": "Fallback-derived health drift rows should be suppressed when managedDrift is present",
		}},
	})
	if err != nil {
		t.Fatalf("marshal managed drift detail body: %v", err)
	}

	managedEmptyDetailJSON, err := json.Marshal(map[string]any{
		"name":               "api-managed-empty",
		"appRef":             "payments",
		"environmentRef":     "prod",
		"region":             "us-west-2",
		"accountName":        "acct-a",
		"awsAccountId":       "111111111111",
		"targetType":         "ecs",
		"strategy":           "linear",
		"executionMode":      "ecsCodeDeploy",
		"desiredGeneration":  9,
		"observedGeneration": 7,
		"sync":               map[string]any{"state": "Drifted", "reason": "Managed drift rows are empty for this deployment"},
		"health":             map[string]any{"overall": "Unhealthy", "summary": "weighted routing is still inconsistent"},
		"desiredArtifact":    map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v7"},
		"managedDrift":       []map[string]any{},
		"runtimeSummary": map[string]any{
			"source":             "deployment-controller",
			"timestamp":          "2026-04-16T06:49:00Z",
			"targetType":         "ec2",
			"strategy":           "canary",
			"executionMode":      "asgWeightedTargetGroups",
			"desiredGeneration":  9,
			"observedGeneration": 7,
			"historyCount":       2,
			"changes":            []string{"managed drift empty"},
			"deployedArtifact":   map[string]any{"binary": "s3://artifacts.example/payments/api-managed-empty-r17"},
			"conditions":         []map[string]any{{"type": "DriftDetected", "status": true, "reason": "TargetGroupMismatch", "message": "Fallback-derived drift rows should stay suppressed when managedDrift is empty"}},
		},
		"conditions": []map[string]any{{
			"type":    "DriftDetected",
			"status":  true,
			"reason":  "TargetGroupMismatch",
			"message": "Fallback-derived drift rows should stay suppressed when managedDrift is empty",
		}},
	})
	if err != nil {
		t.Fatalf("marshal managed empty drift detail body: %v", err)
	}

	driftDetailJSON, err := json.Marshal(map[string]any{
		"name":               "api-prod",
		"appRef":             "payments",
		"environmentRef":     "prod",
		"region":             "us-west-2",
		"accountName":        "acct-a",
		"awsAccountId":       "111111111111",
		"targetType":         "ecs",
		"strategy":           "linear",
		"executionMode":      "ecsCodeDeploy",
		"desiredGeneration":  9,
		"deployedGeneration": 7,
		"sync":               map[string]any{"state": "Drifted", "reason": "Service target group differs from desired"},
		"health":             map[string]any{"overall": "Unhealthy", "summary": "traffic steady but target registration is inconsistent"},
		"desiredArtifact":    map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v7"},
		"runtimeSummary": map[string]any{
			"source":             "deployment-controller",
			"timestamp":          "2026-04-16T06:50:00Z",
			"targetType":         "ec2",
			"strategy":           "canary",
			"executionMode":      "asgWeightedTargetGroups",
			"desiredGeneration":  9,
			"observedGeneration": 7,
			"historyCount":       3,
			"changes":            []string{"listener rule updated", "target group weight drift"},
			"deployedArtifact":   map[string]any{"binary": "s3://artifacts.example/payments/api-prod-r17"},
			"conditions":         []map[string]any{{"type": "DriftDetected", "status": true, "reason": "TargetGroupMismatch", "message": "Runtime summary mirrors the drift condition"}, {"type": "Healthy", "status": false, "reason": "TargetDeregistrationLag", "message": "Old targets are still draining"}},
		},
		"rollout": map[string]any{
			"execution": []map[string]any{{
				"name":        "prod-router",
				"resource":    "aws_lb_listener_rule.payments_prod",
				"description": "Routes production traffic to the stable target group",
			}},
		},
		"conditions": []map[string]any{{
			"type":    "DriftDetected",
			"status":  true,
			"reason":  "TargetGroupMismatch",
			"message": "<script>alert('xss')</script> ALB production listener still targets the legacy task set",
		}, {
			"type":    "Progressing",
			"status":  true,
			"reason":  "TrafficShiftInFlight",
			"message": "CodeDeploy is still shifting live traffic",
		}, {
			"type":    "Accepted",
			"status":  true,
			"reason":  "ReadModelReady",
			"message": "Read model accepted the deployment snapshot",
		}, {
			"type":    "Healthy",
			"status":  false,
			"reason":  "TargetDeregistrationLag",
			"message": "Old targets are still draining",
		}, {
			"type":    "DriftDetected",
			"status":  false,
			"reason":  "BoundaryRegressionGuard",
			"message": "False drift detection should stay out of drift rows",
		}, {
			"type":    "Healthy",
			"status":  true,
			"reason":  "BoundaryRegressionGuard",
			"message": "Healthy true should stay out of drift rows",
		}},
	})
	if err != nil {
		t.Fatalf("marshal drift detail body: %v", err)
	}

	emptyDetailJSON, err := json.Marshal(map[string]any{
		"name":             "api-steady",
		"appRef":           "payments",
		"environmentRef":   "prod",
		"region":           "us-west-2",
		"accountName":      "acct-a",
		"awsAccountId":     "111111111111",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"sync":             map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
		"health":           map[string]any{"overall": "Healthy", "summary": "service steady"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v9"},
		"runtimeSummary": map[string]any{
			"source":             "deployment-controller",
			"timestamp":          "2026-04-16T06:52:00Z",
			"targetType":         "ecs",
			"strategy":           "linear",
			"executionMode":      "ecsCodeDeploy",
			"desiredGeneration":  9,
			"observedGeneration": 9,
			"historyCount":       1,
			"changes":            []string{},
			"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v9"},
			"conditions":         []map[string]any{},
		},
		"conditions": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal empty detail body: %v", err)
	}

	noLinkedResourcesDetailJSON, err := json.Marshal(map[string]any{
		"name":             "api-no-links",
		"appRef":           "payments",
		"environmentRef":   "prod",
		"region":           "us-west-2",
		"accountName":      "acct-a",
		"awsAccountId":     "111111111111",
		"sync":             map[string]any{"state": "Synced", "reason": "latest desired generation applied"},
		"health":           map[string]any{"overall": "Healthy", "summary": "service steady"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v9"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v9"},
		"runtimeSummary": map[string]any{
			"source":           "deployment-controller",
			"timestamp":        "2026-04-16T06:53:00Z",
			"historyCount":     1,
			"changes":          []string{},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v9"},
			"conditions":       []map[string]any{},
		},
		"conditions": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal no linked resources detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const managedDetail = ` + string(managedDetailJSON) + `;
const managedEmptyDetail = ` + string(managedEmptyDetailJSON) + `;
const driftDetail = ` + string(driftDetailJSON) + `;
const emptyDetail = ` + string(emptyDetailJSON) + `;
const noLinkedResourcesDetail = ` + string(noLinkedResourcesDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function render(detail) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?page=drift&deployment=' + encodeURIComponent(detail.name), hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/deployments/' + encodeURIComponent(detail.name)) return Promise.resolve({ ok: true, json: () => Promise.resolve(detail) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const managedRun = await render(managedDetail);
  const managedText = collectText(managedRun.document.getElementById('accounts')).join(' | ');
  const managedRows = collectNodes(managedRun.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  const managedLinks = collectNodes(managedRun.document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (!managedText.includes('Active drift source')) throw new Error('expected managed drift workspace source callout label');
  if (!managedText.includes('Current source: controller-authored managedDrift rows.')) throw new Error('expected managed drift workspace source summary');
  if (!managedText.includes('managedDrift is present in the deployment detail payload, so fallback read-model heuristics stay suppressed for this deployment.')) throw new Error('expected managed drift workspace source explanation');
  if (!managedRows.some((row) => row.includes('listener.weight'))) throw new Error('expected backend-authored managed drift row');
  if (!managedRows.some((row) => row.includes('service.taskDefinition'))) throw new Error('expected second backend-authored managed drift row');
  if (!managedRows.some((row) => row.includes('runtimeContext.capacity.desired') && row.includes("<script>alert('capacity')</script> 4") && row.includes('Open resource evidence'))) throw new Error('expected managed drift capacity row to render safe text plus resource evidence handoff');
  if (managedRows.some((row) => row.includes('listener.weight') && row.includes('Open resource evidence'))) throw new Error('expected unmanaged drift row without resourceRef to keep no-handoff rendering');
  const managedEvidenceLink = managedLinks.find((node) => node.textContent === 'Open resource evidence');
  const expectedManagedEvidenceHref = '/?page=resource&deployment=api-managed&resourceType=candidateAutoScalingGroup&resourceId=candidateAutoScalingGroup%3A%3Aapi-managed-candidate%3A%3ACandidate+Auto+Scaling+Group+Tenet+creates+for+managed+drift+evidence';
  if (!managedEvidenceLink || managedEvidenceLink.href !== expectedManagedEvidenceHref || managedEvidenceLink.target !== '_self' || managedEvidenceLink.rel) throw new Error('expected managed drift resource evidence link to target normalized resource route, got ' + (managedEvidenceLink ? managedEvidenceLink.href + ' target=' + managedEvidenceLink.target + ' rel=' + managedEvidenceLink.rel : 'none'));
  if (!managedRows.some((row) => row.includes('runtimeContext.launchTemplate.imageId') && row.includes("<script>alert('ami')</script> ami-fffffffffffffffff"))) throw new Error('expected managed drift launch-template imageId row to render as safe text');
  if (!managedRows.some((row) => row.includes('runtimeContext.launchTemplate.instanceType') && row.includes("<script>alert('instance-type')</script> m6i.large"))) throw new Error('expected managed drift launch-template instanceType row to render as safe text');
  if (managedRows.some((row) => row.includes('artifact'))) throw new Error('expected managed drift rows to suppress fallback artifact derivation');
  if (managedRows.some((row) => row.includes('condition.DriftDetected'))) throw new Error('expected managed drift rows to suppress fallback condition derivation');
  if (!managedText.includes('Drift summary')) throw new Error('expected managed drift summary heading');
  if (!managedText.includes('Total rows')) throw new Error('expected managed drift total-row summary');
  if (!managedText.includes('Visible rows')) throw new Error('expected managed drift visible-row summary');
  if (!managedText.includes('Showing all 5 drift rows.')) throw new Error('expected managed drift summary copy');
  if (!/Visible high \| 2(?: \||$)/.test(managedText)) throw new Error('expected managed drift visible high-severity summary count');
  if (!/Visible medium \| 2(?: \||$)/.test(managedText)) throw new Error('expected managed drift visible medium-severity summary count');
  if (!/Visible low \| 1(?: \||$)/.test(managedText)) throw new Error('expected managed drift visible low-severity summary count');
  if (!managedText.includes("<script>alert('xss')</script> stable=80 canary=20")) throw new Error('expected managed drift value to render as safe text');
  const managedSeverityField = managedRun.document.querySelector('[data-toolbar-field="Severity"]');
  if (!managedSeverityField) throw new Error('expected managed drift severity filter control');
  managedSeverityField.value = 'high';
  managedSeverityField.dispatch('change', { target: managedSeverityField });
  await settle();
  const managedHighText = collectText(managedRun.document.getElementById('accounts')).join(' | ');
  const managedHighRows = collectNodes(managedRun.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  if (managedHighRows.length !== 2) throw new Error('expected managed drift severity filter to narrow to two rows, got ' + managedHighRows.length);
  if (!managedHighRows[0].includes('listener.weight')) throw new Error('expected managed drift severity filter to retain the high severity row');
  if (!managedHighRows.some((row) => row.includes('runtimeContext.launchTemplate.imageId'))) throw new Error('expected managed drift severity filter to retain the launch-template imageId row');
  if (!managedHighText.includes('Showing 2 drift rows out of 5 drift rows after filters.')) throw new Error('expected managed drift summary to reflect high filter impact');

  const managedEmptyRun = await render(managedEmptyDetail);
  const managedEmptyText = collectText(managedEmptyRun.document.getElementById('accounts')).join(' | ');
  const managedEmptyRows = collectNodes(managedEmptyRun.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1);
  if (!managedEmptyText.includes('No controller-authored field-level drift was reported for this deployment.')) throw new Error('expected explicit controller-authored empty drift message');
  if (!managedEmptyText.includes('No field-level drift rows are currently reported.')) throw new Error('expected empty managed drift summary copy');
  if (managedEmptyText.includes('condition.DriftDetected')) throw new Error('expected empty managed drift state to suppress fallback condition rows');
  if (managedEmptyRows.length !== 0) throw new Error('expected empty managed drift state to suppress fallback drift table rows');

  const run = await render(driftDetail);
  if (run.fetchCalls.length !== 1 || run.fetchCalls[0] !== '/v1/overview/deployments/api-prod') throw new Error('expected drift workspace to load only deployment detail, got ' + run.fetchCalls.join(', '));
  const text = collectText(run.document.getElementById('accounts')).join(' | ');
  const toolbarText = collectText(run.document.getElementById('toolbar')).join(' | ');
  const workspaceLinks = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'A');
  const workspaceChips = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const breadcrumbChips = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className.includes('breadcrumb-current'));
  const applicationsBreadcrumb = workspaceLinks.find((node) => node.href === '/?page=applications' && node.textContent === 'Applications' && node.target === '_self');
  const accountBreadcrumb = workspaceLinks.find((node) => node.href === '/?page=account&accountId=123456789012' && node.textContent.includes('123456789012') && node.target === '_self');
  const applicationBreadcrumb = workspaceLinks.find((node) => node.href === '/?page=application&deployment=api-prod' && node.textContent === 'api-prod' && node.target === '_self');
  const applicationsOverviewLink = workspaceLinks.find((node) => node.href === '/?page=applications' && node.textContent === 'Applications overview' && node.target === '_self');
  const workspaceBackLink = workspaceLinks.find((node) => node.href === '/?page=application&deployment=api-prod' && node.textContent === 'Back to application' && node.target === '_self');
  const workspaceDriftLink = workspaceLinks.find((node) => node.href === '/?page=drift&deployment=api-prod' && node.textContent === 'Drift view' && node.target === '_self');
  const workspacePlanLink = workspaceLinks.find((node) => node.href === '/?page=plan&deployment=api-prod' && node.textContent === 'Plan/apply' && node.target === '_self');
  const workspaceAuditLink = workspaceLinks.find((node) => node.href === '/?page=audit&deployment=api-prod' && node.textContent === 'Audit log' && node.target === '_self');
  const workspaceResourceLink = workspaceLinks.find((node) => node.href === '/?page=resource&deployment=api-prod&resourceType=aws_lb_listener_rule.payments_prod&resourceId=aws_lb_listener_rule.payments_prod%3A%3Aprod-router%3A%3ARoutes+production+traffic+to+the+stable+target+group' && node.textContent === 'prod-router' && node.target === '_self');
  if (!text.includes('Drift viewer')) throw new Error('expected drift workspace title');
  if (!toolbarText.includes('Back to application')) throw new Error('expected workspace back link');
  if (!toolbarText.includes('api-prod')) throw new Error('expected workspace active deployment title');
  if (!text.includes('Breadcrumbs')) throw new Error('expected drift workspace breadcrumb chrome');
  if (!applicationsBreadcrumb || applicationsBreadcrumb.rel) throw new Error('expected drift workspace applications breadcrumb link');
  if (accountBreadcrumb) throw new Error('expected drift workspace to omit account breadcrumb when awsAccountId is unavailable');
  if (!applicationBreadcrumb || applicationBreadcrumb.rel) throw new Error('expected drift workspace application breadcrumb link');
  if (!breadcrumbChips.find((node) => node.textContent === 'Drift view')) throw new Error('expected drift workspace current breadcrumb chip');
  if (!text.includes('Quick navigation')) throw new Error('expected workspace quick navigation strip');
  if (!text.includes('Hierarchy')) throw new Error('expected workspace hierarchy quick-nav group label');
  if (!text.includes('Current destination')) throw new Error('expected workspace current-destination quick-nav group label');
  if (!text.includes('Deployment workspaces')) throw new Error('expected workspace quick navigation group label');
  if (!text.includes('Current section')) throw new Error('expected drift workspace current-section group label');
  if (!text.includes('No section is currently targeted.')) throw new Error('expected drift workspace empty current-section copy without hash');
  if (!text.includes('Linked resources')) throw new Error('expected drift workspace linked-resources group label');
  if (!text.includes('api-prod · Drift view')) throw new Error('expected workspace quick navigation current-state label');
  if (!applicationsOverviewLink || applicationsOverviewLink.rel) throw new Error('expected drift workspace quick navigation applications overview link');
  if (!workspaceBackLink || workspaceBackLink.rel) throw new Error('expected drift workspace quick navigation back link');
  if (workspaceDriftLink) throw new Error('expected drift workspace quick navigation to omit a self-link for the active workspace');
  const currentDestinationChip = workspaceChips.find((node) => node.getAttribute('aria-label') === 'Current destination: Drift view');
  if (!currentDestinationChip) throw new Error('expected drift workspace current-destination chip');
  if (workspaceChips.find((node) => node.getAttribute('aria-label') === 'Current workspace: Drift view')) throw new Error('expected drift workspace to stop using current-workspace chip copy');
  if (workspaceChips.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '))) throw new Error('expected drift workspace without hash not to render a current-section chip');
  if (!workspacePlanLink || workspacePlanLink.rel) throw new Error('expected drift workspace quick navigation plan link');
  if (!workspaceAuditLink || workspaceAuditLink.rel) throw new Error('expected drift workspace quick navigation audit link');
  if (!workspaceResourceLink || workspaceResourceLink.rel) throw new Error('expected drift workspace quick navigation resource link');
  if (!text.includes('Latest runtime signals')) throw new Error('expected drift workspace runtime signals section');
  if (!text.includes('deployment-controller')) throw new Error('expected drift workspace runtime source value');
  if (!text.includes('Target ec2 · Strategy canary · Execution asgWeightedTargetGroups')) throw new Error('expected drift workspace runtime context value');
  if (!/Runtime history \| 3(?: \||$)/.test(text)) throw new Error('expected drift workspace runtime history count');
  if (!text.includes('s3://artifacts.example/payments/api-prod-r17')) throw new Error('expected drift workspace runtime artifact value');
  if (!text.includes('DriftDetected = True · TargetGroupMismatch')) throw new Error('expected drift workspace runtime condition summary');
  if (!text.includes('listener rule updated')) throw new Error('expected drift workspace runtime change entry');
  if (!text.includes('target group weight drift')) throw new Error('expected drift workspace second runtime change entry');
  if (!text.includes('Rollout execution')) throw new Error('expected drift workspace rollout execution section');
  if (!text.includes('prod-router')) throw new Error('expected drift workspace rollout execution name');
  if (!text.includes('Field-level drift')) throw new Error('expected drift workspace section heading');
  if (!text.includes('Current source: derived fallback read-model heuristics.')) throw new Error('expected fallback drift workspace source summary');
  if (!text.includes('managedDrift is absent in the deployment detail payload, so Tenet is deriving field-level drift from artifact, generation, runtime-context, and unhealthy-condition read-model signals.')) throw new Error('expected fallback drift workspace source explanation');
  if (!text.includes('Field path')) throw new Error('expected drift workspace field column header');
  if (!text.includes('Desired')) throw new Error('expected drift workspace desired column header');
  if (!text.includes('Actual')) throw new Error('expected drift workspace actual column header');
  if (!text.includes('Severity')) throw new Error('expected drift workspace severity column header');
  if (!text.includes('artifact')) throw new Error('expected artifact drift row');
  if (!text.includes('generation')) throw new Error('expected generation drift row');
  if (!text.includes('runtimeContext.targetType')) throw new Error('expected runtime target-type drift row');
  if (!text.includes('runtimeContext.strategy')) throw new Error('expected runtime strategy drift row');
  if (!text.includes('runtimeContext.executionMode')) throw new Error('expected runtime execution-mode drift row');
  if (!text.includes('condition.DriftDetected')) throw new Error('expected unhealthy condition drift row');
  if (!text.includes('condition.Healthy')) throw new Error('expected unhealthy health-condition drift row');
  if (text.includes('condition.Progressing')) throw new Error('expected benign progressing condition to stay out of drift rows');
  if (text.includes('condition.Accepted')) throw new Error('expected benign accepted condition to stay out of drift rows');
  if (!text.includes('Drift summary')) throw new Error('expected fallback drift summary heading');
  if (!text.includes('Showing all 7 drift rows.')) throw new Error('expected fallback drift summary copy');
  const allRows = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  if (allRows.some((row) => row.includes('BoundaryRegressionGuard') && row.includes('False drift detection should stay out of drift rows'))) throw new Error('expected DriftDetected=false boundary to stay out of drift rows');
  if (allRows.some((row) => row.includes('BoundaryRegressionGuard') && row.includes('Healthy true should stay out of drift rows'))) throw new Error('expected Healthy=true boundary to stay out of drift rows');
  if (!text.includes('ghcr.io/example/payments:v9')) throw new Error('expected desired artifact value in drift rows');
  if (!text.includes('ghcr.io/example/payments:v7')) throw new Error('expected actual artifact value in drift rows');
  if (!text.includes('high')) throw new Error('expected high severity indicator');
  if (!text.includes('medium')) throw new Error('expected medium severity indicator');
  if (!text.includes('low')) throw new Error('expected low severity indicator');
  if (text.includes('No derived fallback field-level drift was detected from the deployment read model.')) throw new Error('expected non-empty drift data to avoid empty state');
  if (!text.includes("<script>alert('xss')</script> ALB production listener still targets the legacy task set")) throw new Error('expected drift condition message to render as text');
  let buttons = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  const clearDriftFiltersButton = buttons.find((node) => node.textContent === 'Clear drift filters');
  if (!clearDriftFiltersButton) throw new Error('expected drift clear-filters control');
  if (!clearDriftFiltersButton.disabled) throw new Error('expected drift clear-filters control disabled with default controls');
  if (run.window.location.search !== '?page=drift&deployment=api-prod') throw new Error('expected default drift route to stay bookmarkable');

  const severityField = run.document.querySelector('[data-toolbar-field="Severity"]');
  if (!severityField) throw new Error('expected drift severity filter control');
  severityField.value = 'high';
  severityField.dispatch('change', { target: severityField });
  await settle();
  const highOnlyText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!highOnlyText.includes('artifact')) throw new Error('expected high severity filter to retain artifact row');
  if (highOnlyText.includes('condition.Healthy')) throw new Error('expected high severity filter to remove low-severity condition row');
  if (!highOnlyText.includes('Showing 2 drift rows out of 7 drift rows after filters.')) throw new Error('expected high severity summary to reflect visible fallback rows');
  if (clearDriftFiltersButton.disabled) throw new Error('expected drift clear-filters control enabled after severity change');
  if (run.window.location.search !== '?page=drift&deployment=api-prod') throw new Error('expected severity change not to mutate drift route');

  const queryField = run.document.querySelector('[data-toolbar-field="Filter drift rows"]');
  if (!queryField) throw new Error('expected drift query filter control');
  severityField.value = 'all';
  severityField.dispatch('change', { target: severityField });
  queryField.value = 'targetderegistrationlag';
  queryField.dispatch('input', { target: queryField });
  await settle();
  const queryRows = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  if (queryRows.length !== 1) throw new Error('expected drift query filter to narrow to a single row, got ' + queryRows.length);
  if (!queryRows[0].includes('condition.Healthy')) throw new Error('expected drift query filter to match condition context');
  if (queryRows[0].includes('artifact')) throw new Error('expected drift query filter not to include artifact row content');
  const queryText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!queryText.includes('Showing 1 drift row out of 7 drift rows after filters.')) throw new Error('expected query summary to reflect visible fallback rows');

  queryField.value = '';
  queryField.dispatch('input', { target: queryField });
  await settle();

  const sortField = run.document.querySelector('[data-toolbar-field="Sort by"]');
  if (!sortField) throw new Error('expected drift sort control');
  sortField.value = 'severity';
  sortField.dispatch('change', { target: sortField });
  await settle();
  const rowItems = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1);
  const rowTexts = rowItems.map((node) => collectText(node).join(' | '));
  if (!rowTexts.length) throw new Error('expected rendered drift table rows');
  if (!rowTexts[0].includes('artifact')) throw new Error('expected severity sort to place artifact drift first');
  if (!rowTexts[rowTexts.length - 1].includes('condition.Healthy')) throw new Error('expected severity sort to place low severity drift last');
  severityField.value = 'high';
  severityField.dispatch('change', { target: severityField });
  queryField.value = 'targetderegistrationlag';
  queryField.dispatch('input', { target: queryField });
  await settle();
  let controlsText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!controlsText.includes('Drift filters: search “targetderegistrationlag” · severity high · sort severity')) throw new Error('expected drift filter summary to reflect all active controls');
  clearDriftFiltersButton.dispatch('click');
  await settle();
  if (queryField.value !== '') throw new Error('expected clear drift filters to reset query control');
  if (severityField.value !== 'all') throw new Error('expected clear drift filters to reset severity control');
  if (sortField.value !== 'field') throw new Error('expected clear drift filters to reset sort control');
  if (!clearDriftFiltersButton.disabled) throw new Error('expected clear drift filters to disable after resetting defaults');
  controlsText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!controlsText.includes('Drift filters: defaults')) throw new Error('expected drift filter summary to return to defaults after clearing');
  if (!controlsText.includes('Showing all 7 drift rows.')) throw new Error('expected clear drift filters to restore all drift rows');
  const clearedRows = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  if (!clearedRows.length) throw new Error('expected cleared drift filters to restore table rows');
  if (!clearedRows[clearedRows.length - 1].includes('runtimeContext.targetType')) throw new Error('expected clear drift filters to restore default field sort');
  if (run.window.location.search !== '?page=drift&deployment=api-prod') throw new Error('expected clear drift filters to preserve bookmarkable drift route');
  queryField.value = 'missing-signal';
  queryField.dispatch('input', { target: queryField });
  await settle();
  const filteredEmptyText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!filteredEmptyText.includes('Filters currently hide all 7 drift rows.')) throw new Error('expected filtered-empty summary copy');
  if (!filteredEmptyText.includes('No field-level drift rows match the current filters. Adjust the query or severity filters to inspect reported drift.')) throw new Error('expected filtered-empty table message');

  const emptyRun = await render(emptyDetail);
  const emptyText = collectText(emptyRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('Field-level drift')) throw new Error('expected empty drift workspace section heading');
  if (!emptyText.includes('No field-level drift rows are currently reported.')) throw new Error('expected empty fallback drift summary copy');
  if (!emptyText.includes('No derived fallback field-level drift was detected from the deployment read model.')) throw new Error('expected empty drift workspace message');
  if (emptyText.includes('condition.')) throw new Error('expected empty drift workspace to omit condition rows');

  const noLinksRun = await render(noLinkedResourcesDetail);
  const noLinksText = collectText(noLinksRun.document.getElementById('accounts')).join(' | ');
  const noLinksResourceLinks = collectNodes(noLinksRun.document.getElementById('accounts'), (node) => node.tagName === 'A' && node.href.includes('?page=resource&deployment=api-no-links'));
  if (!noLinksText.includes('Linked resources')) throw new Error('expected no-links drift workspace quick navigation group label');
  if (!noLinksText.includes('No linked resources are available in this drift workspace yet.')) throw new Error('expected explicit no-links drift workspace copy');
  if (noLinksResourceLinks.length !== 0) throw new Error('expected drift workspace with no linked resources to suppress broken resource links');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestDeploymentWorkspaceDriftSectionLinksWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	driftDetailJSON, err := json.Marshal(map[string]any{
		"name":            "payments-api-prod",
		"appRef":          "payments-api",
		"environmentRef":  "prod",
		"accountName":     "payments-prod",
		"awsAccountId":    "210987654321",
		"region":          "ap-northeast-2",
		"targetType":      "ec2",
		"strategy":        "canary",
		"executionMode":   "asgWeightedTargetGroups",
		"sync":            map[string]any{"state": "Drifted", "reason": "deployment detail still reports drift"},
		"health":          map[string]any{"overall": "Unhealthy", "summary": "target registration still catching up"},
		"desiredArtifact": map[string]any{"image": "ghcr.io/example/payments-api:v19"},
		"deployedArtifact": map[string]any{
			"image": "ghcr.io/example/payments-api:v18",
		},
		"runtimeSummary": map[string]any{
			"source":        "deployment-controller",
			"timestamp":     "2026-04-20T01:15:00Z",
			"targetType":    "ec2",
			"strategy":      "canary",
			"executionMode": "asgWeightedTargetGroups",
			"historyCount":  2,
			"changes":       []string{"target registration still shifting"},
			"conditions":    []map[string]any{{"type": "DriftDetected", "status": true, "reason": "TargetGroupMismatch"}},
		},
		"rollout": map[string]any{
			"phase":       "ShiftTrafficLinear",
			"currentStep": "ShiftTrafficLinear",
			"execution": []map[string]any{{
				"name":        "prod-router",
				"resource":    "aws_lb_listener_rule.payments_api_prod",
				"description": "Routes production traffic to the active target group",
			}},
		},
		"conditions": []map[string]any{{
			"type":    "DriftDetected",
			"status":  true,
			"reason":  "TargetGroupMismatch",
			"message": "Weighted targets are still reconciling",
		}},
	})
	if err != nil {
		t.Fatalf("marshal drift detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const driftDetail = ` + string(driftDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.id = '';
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) current.add(name); else current.delete(name);
        this.className = Array.from(current).join(' ');
      },
    };
  }
  append(...nodes) {
    nodes.forEach((node) => {
      if (node == null) return;
      if (typeof node === 'string') {
        const textNode = new Element('#text', this.ownerDocument);
        textNode.textContent = node;
        node = textNode;
      }
      node.parentNode = this;
      this.children.push(node);
      if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) {
        this.value = node.value;
      }
    });
  }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange() {} }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const createWindow = (initialSearch, hash) => {
  const listeners = {};
  return {
    location: { pathname: '/console', search: initialSearch, hash },
    addEventListener(type, handler) {
      listeners[type] = listeners[type] || [];
      listeners[type].push(handler);
    },
    dispatchEvent(event) {
      const handlers = listeners[event?.type] || [];
      handlers.forEach((handler) => handler.call(this, event));
    },
    dispatchHashChange() {
      this.dispatchEvent({ type: 'hashchange', target: this });
    },
  };
};
const collectNodes = (root, predicate) => {
  const matches = [];
  const visit = (node) => {
    if (!node) return;
    if (predicate(node)) matches.push(node);
    (node.children || []).forEach(visit);
  };
  visit(root);
  return matches;
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const initialSearch = '?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod';
const window = createWindow(initialSearch, '#drift-status-conditions');
const history = { replaceState(_state, _title, url) {
  const [pathAndQuery, hash = ''] = String(url).split('#');
  const [pathname, query = ''] = pathAndQuery.split('?');
  window.location.pathname = pathname || '/console';
  window.location.search = query ? '?' + query : '';
  window.location.hash = hash ? '#' + hash : '';
} };
const fetchCalls = [];
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (String(url) === '/v1/overview/deployments/payments-api-prod') {
    return Promise.resolve({ ok: true, json: () => Promise.resolve(driftDetail) });
  }
  throw new Error('unexpected fetch url: ' + url);
};
(async () => {
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-prod') throw new Error('expected drift workspace hash route to load only deployment detail');
  if (window.location.pathname !== '/console') throw new Error('expected drift workspace pathname to stay stable, got ' + window.location.pathname);
  if (window.location.search !== initialSearch) throw new Error('expected drift workspace query to stay stable, got ' + window.location.search);
  if (window.location.hash !== '#drift-status-conditions') throw new Error('expected initial drift workspace hash to stay stable, got ' + window.location.hash);
  const accounts = document.getElementById('accounts');
  const quickNavChips = collectNodes(accounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const currentSectionChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current section: Status conditions');
  if (!currentSectionChip) throw new Error('expected drift hash to render the current-section chip');
  const sectionLinks = collectNodes(accounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#drift-'));
  const expectedLinks = [
    ['/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod#drift-runtime-signals', 'Runtime signals'],
    ['/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod#drift-rollout-execution', 'Rollout execution'],
    ['/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod#drift-field-level', 'Field-level drift'],
  ];
  if (sectionLinks.length !== expectedLinks.length) throw new Error('expected three remaining drift section links, got ' + sectionLinks.length);
  expectedLinks.forEach(([href, text]) => {
    const link = sectionLinks.find((node) => node.href === href && node.textContent === text);
    if (!link) throw new Error('missing drift section link ' + text + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected drift section link to stay in-shell for ' + text);
  });
  if (sectionLinks.find((node) => node.textContent === 'Status conditions')) throw new Error('expected active drift section to be removed from On this page links');
  const sectionIDs = collectNodes(accounts, (node) => typeof node.id === 'string' && node.id.startsWith('drift-')).map((node) => node.id).sort();
  const expectedSectionIDs = ['drift-field-level', 'drift-rollout-execution', 'drift-runtime-signals', 'drift-status-conditions'];
  if (JSON.stringify(sectionIDs) !== JSON.stringify(expectedSectionIDs)) throw new Error('unexpected drift section ids: ' + JSON.stringify(sectionIDs));
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'drift-status-conditions') throw new Error('expected initial drift workspace hash to scroll the matching section into view');
  window.location.hash = '#drift-field-level';
  window.dispatchHashChange();
  await settle();
  const updatedAccounts = document.getElementById('accounts');
  const updatedChips = collectNodes(updatedAccounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const fieldLevelChip = updatedChips.find((node) => node.getAttribute('aria-label') === 'Current section: Field-level drift');
  if (!fieldLevelChip) throw new Error('expected drift hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedAccounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#drift-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Field-level drift')) throw new Error('expected updated drift current section to be removed from On this page links');
  if (!updatedSectionLinks.find((node) => node.textContent === 'Status conditions')) throw new Error('expected previous drift current section to return to On this page links after hashchange');
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'drift-field-level') throw new Error('expected drift hashchange to scroll the new current section into view');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestDeploymentWorkspaceDriftRefreshWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	initialDetailJSON, err := json.Marshal(map[string]any{
		"name":             "api-refresh",
		"appRef":           "payments",
		"environmentRef":   "prod",
		"region":           "us-west-2",
		"accountName":      "acct-a",
		"awsAccountId":     "111111111111",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"sync":             map[string]any{"state": "Drifted", "reason": "listener weights diverged"},
		"health":           map[string]any{"overall": "Unhealthy", "summary": "legacy target still serving traffic"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v52"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v51"},
		"managedDrift": []map[string]any{{
			"field":    "listener.weight",
			"desired":  "stable=100 canary=0",
			"actual":   "stable=80 canary=20",
			"severity": "high",
		}},
		"runtimeSummary": map[string]any{
			"source":           "deployment-controller",
			"timestamp":        "2026-04-19T07:55:00Z",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"historyCount":     2,
			"changes":          []string{"initial drift snapshot"},
			"changeDetails":    []string{"Listener weight changed from stable=100/canary=0 to stable=80/canary=20"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v51"},
			"conditions":       []map[string]any{{"type": "DriftDetected", "status": true, "reason": "ListenerWeightMismatch", "message": "Stable traffic is only 80 percent"}},
		},
		"conditions": []map[string]any{{
			"type":    "DriftDetected",
			"status":  true,
			"reason":  "ListenerWeightMismatch",
			"message": "Stable traffic is only 80 percent",
		}},
	})
	if err != nil {
		t.Fatalf("marshal initial drift detail body: %v", err)
	}

	refreshedDetailJSON, err := json.Marshal(map[string]any{
		"name":             "api-refresh",
		"appRef":           "payments",
		"environmentRef":   "prod",
		"region":           "us-west-2",
		"accountName":      "acct-a",
		"awsAccountId":     "111111111111",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"sync":             map[string]any{"state": "Drifted", "reason": "task definition still lagging"},
		"health":           map[string]any{"overall": "Progressing", "summary": "traffic is stable while tasks recycle"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v52"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v51"},
		"managedDrift": []map[string]any{{
			"field":    "service.taskDefinition",
			"desired":  "payments:52",
			"actual":   "payments:51",
			"severity": "medium",
		}},
		"runtimeSummary": map[string]any{
			"source":           "deployment-controller",
			"timestamp":        "2026-04-19T07:57:00Z",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"historyCount":     3,
			"changes":          []string{"refreshed drift snapshot"},
			"changeDetails":    []string{"Task definition remains one revision behind desired"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v51"},
			"conditions":       []map[string]any{{"type": "Progressing", "status": true, "reason": "TaskRecycleInFlight", "message": "New task set is still scaling"}},
		},
		"conditions": []map[string]any{{
			"type":    "Progressing",
			"status":  true,
			"reason":  "TaskRecycleInFlight",
			"message": "New task set is still scaling",
		}},
	})
	if err != nil {
		t.Fatalf("marshal refreshed drift detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const initialDetail = ` + string(initialDetailJSON) + `;
const refreshedDetail = ` + string(refreshedDetailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
async function render(fetchImpl) {
  const document = createDocument();
  const fetchCalls = [];
  const window = { location: { pathname: '/', search: '?page=drift&deployment=api-refresh', hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetch = (url) => {
    fetchCalls.push(String(url));
    return fetchImpl(String(url));
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls, window };
}
(async () => {
  const detailURL = '/v1/overview/deployments/api-refresh';
  const initialSearch = '?page=drift&deployment=api-refresh';
  let refreshFetchCount = 0;
  let resolveRefresh;
  const loadingView = await render((url) => {
    if (url !== detailURL) throw new Error('unexpected fetch url: ' + url);
    refreshFetchCount += 1;
    if (refreshFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetail) });
    if (refreshFetchCount === 2) {
      return new Promise((resolve) => { resolveRefresh = resolve; });
    }
    throw new Error('unexpected detail fetch count: ' + refreshFetchCount);
  });
  let buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  let refreshButton = buttons.find((node) => node.textContent === 'Refresh drift workspace');
  if (!refreshButton) throw new Error('expected drift workspace refresh button after initial render');
  if (refreshButton.disabled) throw new Error('expected drift workspace refresh button enabled after initial render');
  let text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('listener.weight')) throw new Error('expected initial drift row before refresh');
  refreshButton.dispatch('click');
  await settle();
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh drift workspace');
  if (!refreshButton || !refreshButton.disabled) throw new Error('expected drift workspace refresh button disabled only while refresh is in flight');
  text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('Refreshing drift workspace from GET /v1/overview/deployments/{deploymentName}…')) throw new Error('expected operator-readable drift refresh loading state');
  if (!text.includes('listener.weight')) throw new Error('expected current drift workspace content to stay visible during refresh');
  if (loadingView.fetchCalls.length !== 2 || loadingView.fetchCalls[0] !== detailURL || loadingView.fetchCalls[1] !== detailURL) throw new Error('expected drift refresh to re-fetch only deployment detail');
  if (loadingView.window.location.search !== initialSearch) throw new Error('expected in-place drift refresh to preserve bookmarkable route');
  resolveRefresh({ ok: true, json: () => Promise.resolve(refreshedDetail) });
  await settle();
  await settle();
  text = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('service.taskDefinition')) throw new Error('expected refreshed drift detail to render in place');
  if (text.includes('listener.weight')) throw new Error('expected refreshed drift detail to replace stale row content');
  buttons = collectNodes(loadingView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh drift workspace');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected drift workspace refresh button re-enabled after refresh completes');

  let errorFetchCount = 0;
  const errorView = await render((url) => {
    if (url !== detailURL) throw new Error('unexpected fetch url: ' + url);
    errorFetchCount += 1;
    if (errorFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialDetail) });
    if (errorFetchCount === 2) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected error-view detail fetch count: ' + errorFetchCount);
  });
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh drift workspace');
  if (!refreshButton) throw new Error('expected drift workspace refresh button in error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  text = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!text.includes('Failed to refresh drift workspace: detail request failed (500): boom')) throw new Error('expected operator-readable drift refresh error state');
  if (!text.includes('listener.weight')) throw new Error('expected previous drift content to remain visible after refresh error');
  buttons = collectNodes(errorView.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh drift workspace');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected drift workspace refresh button re-enabled after refresh error');
  if (errorFetchCount !== 2) throw new Error('expected drift error refresh to retry deployment detail exactly once');
  if (errorView.window.location.search !== initialSearch) throw new Error('expected drift refresh error state to preserve bookmarkable route');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestDeploymentWorkspacePlanViewWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":             "api-prod",
		"appRef":           "payments",
		"environmentRef":   "prod",
		"region":           "us-west-2",
		"accountName":      "acct-a",
		"awsAccountId":     "111111111111",
		"sync":             map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":           map[string]any{"overall": "Healthy", "summary": "traffic shifting"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v2"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1"},
		"runtimeSummary": map[string]any{
			"source":           "runtime-agent",
			"timestamp":        "2026-04-16T07:10:00Z",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"historyCount":     5,
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1-runtime"},
			"conditions":       []map[string]any{{"type": "Progressing", "status": true, "reason": "TrafficShiftCanary", "message": "Runtime summary tracks the in-flight canary shift"}},
		},
		"rollout": map[string]any{
			"phase":       "Progressing",
			"currentStep": "ShiftTrafficCanary",
			"summary":     "Shifting 10% canary traffic to the candidate task set",
			"execution":   []map[string]any{},
			"steps":       []map[string]any{{"kind": "CreateCodeDeployDeployment", "status": "completed", "detail": "Created CodeDeploy deployment"}, {"kind": "ShiftTrafficCanary", "status": "current", "detail": "Moving 10% traffic to the green task set"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	planPreviewReadyJSON, err := json.Marshal(map[string]any{
		"id":             "plan_ready_123",
		"deploymentName": "api-prod",
		"status":         "ready",
		"createdAt":      "2026-04-17T09:00:00Z",
		"operations": []map[string]any{{
			"kind":    "ShiftTrafficCanary",
			"status":  "current",
			"phase":   "Promoting",
			"target":  "server-preview-target",
			"summary": "Server-authored preview summary",
		}},
		"risk": map[string]any{
			"level":    "medium",
			"label":    "medium risk",
			"guidance": "Server-authored risk guidance",
			"reasons":  []string{"server preview risk reason"},
		},
		"approval": map[string]any{
			"required": true,
			"summary":  "Server-authored approval summary",
			"reasons":  []string{"change window required"},
		},
		"source": map[string]any{
			"deploymentName":     "api-prod",
			"route":              "/v1/overview/deployments/api-prod",
			"fingerprint":        "fp_plan_ready",
			"runtimeSource":      "runtime-agent",
			"runtimeTimestamp":   "2026-04-16T07:10:00Z",
			"desiredGeneration":  7,
			"observedGeneration": 6,
		},
	})
	if err != nil {
		t.Fatalf("marshal ready plan preview: %v", err)
	}

	planPreviewFailedJSON, err := json.Marshal(map[string]any{
		"id":             "plan_failed_123",
		"deploymentName": "api-prod",
		"status":         "failed",
		"createdAt":      "2026-04-17T09:05:00Z",
		"operations":     []map[string]any{},
		"risk": map[string]any{
			"level":    "high",
			"label":    "high risk",
			"guidance": "Preview failed before approval",
			"reasons":  []string{"source fingerprint changed"},
		},
		"approval": map[string]any{
			"required": true,
			"summary":  "Recreate the preview from the latest deployment detail before approval.",
			"reasons":  []string{"stored preview is stale"},
		},
		"source": map[string]any{
			"deploymentName":     "api-prod",
			"route":              "/v1/overview/deployments/api-prod",
			"fingerprint":        "fp_plan_failed",
			"runtimeSource":      "runtime-agent",
			"runtimeTimestamp":   "2026-04-16T07:10:00Z",
			"desiredGeneration":  7,
			"observedGeneration": 6,
		},
		"errors": []string{"stale source fingerprint", "create again from latest deployment detail"},
	})
	if err != nil {
		t.Fatalf("marshal failed plan preview: %v", err)
	}

	applyRecordFailedJSON, err := json.Marshal(map[string]any{
		"id":             "apply_failed_123",
		"planId":         "plan_ready_123",
		"deploymentName": "api-prod",
		"status":         "failed",
		"createdAt":      "2026-04-18T08:00:00Z",
		"error":          "apply execution is disabled",
		"details": []string{
			"Tenet persisted this immutable apply request in read-only mode and did not contact AWS.",
			"Live execution is intentionally disabled by default for this backend slice.",
		},
		"operations": []map[string]any{{
			"status":  "pending",
			"phase":   "Promoting",
			"target":  "server-preview-target",
			"summary": "Server-authored preview summary",
		}},
		"source": map[string]any{
			"planId":        "plan_ready_123",
			"route":         "/v1/plans/plan_ready_123",
			"planStatus":    "ready",
			"planCreatedAt": "2026-04-17T09:00:00Z",
			"fingerprint":   "fp_plan_ready",
		},
	})
	if err != nil {
		t.Fatalf("marshal failed apply record: %v", err)
	}

	domHarness := `
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); await Promise.resolve(); }
async function waitForFetchCount(fetchCalls, expected) { for (let attempt = 0; attempt < 20 && fetchCalls.length < expected; attempt += 1) await settle(); }
`

	t.Run("create flow renders server authored preview", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyRecord = ` + string(applyRecordFailedJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
let createRequestOptions = null;
let createApplyRequestOptions = null;
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/deployments/api-prod') {
    createRequestOptions = options;
    return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  }
  if (url === '/v1/applies/plans/plan_ready_123') {
    createApplyRequestOptions = options;
    return Promise.resolve({ ok: true, json: () => Promise.resolve(applyRecord) });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/api-prod') throw new Error('expected only deployment detail before create, got ' + fetchCalls.join(', '));
  let text = collectText(root).join(' | ');
  const linksBeforeCreate = collectNodes(root, (node) => node.tagName === 'A');
  const chipsBeforeCreate = collectNodes(root, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const breadcrumbChipsBeforeCreate = collectNodes(root, (node) => node.tagName === 'SPAN' && node.className.includes('breadcrumb-current'));
  const applicationsBreadcrumb = linksBeforeCreate.find((node) => node.href === '/?page=applications' && node.textContent === 'Applications' && node.target === '_self');
  const accountBreadcrumb = linksBeforeCreate.find((node) => node.href === '/?page=account&accountId=123456789012' && node.textContent.includes('123456789012') && node.target === '_self');
  const applicationBreadcrumb = linksBeforeCreate.find((node) => node.href === '/?page=application&deployment=api-prod' && node.textContent === 'api-prod' && node.target === '_self');
  const applicationsOverviewLink = linksBeforeCreate.find((node) => node.href === '/?page=applications' && node.textContent === 'Applications overview' && node.target === '_self');
  const workspaceBackLink = linksBeforeCreate.find((node) => node.href === '/?page=application&deployment=api-prod' && node.textContent === 'Back to application' && node.target === '_self');
  const workspaceDriftLink = linksBeforeCreate.find((node) => node.href === '/?page=drift&deployment=api-prod' && node.textContent === 'Drift view' && node.target === '_self');
  const workspacePlanLink = linksBeforeCreate.find((node) => node.href === '/?page=plan&deployment=api-prod' && node.textContent === 'Plan/apply' && node.target === '_self');
  const workspaceAuditLink = linksBeforeCreate.find((node) => node.href === '/?page=audit&deployment=api-prod' && node.textContent === 'Audit log' && node.target === '_self');
  if (!text.includes('Plan and apply')) throw new Error('expected plan workspace title');
  if (!text.includes('Breadcrumbs')) throw new Error('expected plan workspace breadcrumb chrome');
  if (!applicationsBreadcrumb || applicationsBreadcrumb.rel) throw new Error('expected plan workspace applications breadcrumb link');
  if (accountBreadcrumb) throw new Error('expected plan workspace to omit account breadcrumb when awsAccountId is unavailable');
  if (!applicationBreadcrumb || applicationBreadcrumb.rel) throw new Error('expected plan workspace application breadcrumb link');
  if (!breadcrumbChipsBeforeCreate.find((node) => node.textContent === 'Plan/apply')) throw new Error('expected plan workspace current breadcrumb chip');
  if (!text.includes('Quick navigation')) throw new Error('expected plan workspace quick navigation strip');
  if (!text.includes('Hierarchy')) throw new Error('expected plan workspace hierarchy quick-nav group label');
  if (!text.includes('Current destination')) throw new Error('expected plan workspace current-destination quick-nav group label');
  if (!text.includes('Deployment workspaces')) throw new Error('expected plan workspace quick navigation group label');
  if (!text.includes('Current section')) throw new Error('expected plan workspace current-section group label before create');
  if (!text.includes('No section is currently targeted.')) throw new Error('expected plan workspace empty current-section copy without hash');
  if (!text.includes('api-prod · Plan/apply')) throw new Error('expected plan workspace quick navigation current-state label');
  if (!applicationsOverviewLink || applicationsOverviewLink.rel) throw new Error('expected plan workspace quick navigation applications overview link');
  if (!workspaceBackLink || workspaceBackLink.rel) throw new Error('expected plan workspace quick navigation back link');
  if (!workspaceDriftLink || workspaceDriftLink.rel) throw new Error('expected plan workspace quick navigation drift link');
  if (workspacePlanLink) throw new Error('expected plan workspace quick navigation to omit a self-link for the active workspace');
  const currentDestinationChip = chipsBeforeCreate.find((node) => node.getAttribute('aria-label') === 'Current destination: Plan/apply');
  if (!currentDestinationChip) throw new Error('expected plan workspace current-destination chip');
  if (chipsBeforeCreate.find((node) => node.getAttribute('aria-label') === 'Current workspace: Plan/apply')) throw new Error('expected plan workspace to stop using current-workspace chip copy');
  if (chipsBeforeCreate.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '))) throw new Error('expected plan workspace without hash not to render a current-section chip');
  if (!workspaceAuditLink || workspaceAuditLink.rel) throw new Error('expected plan workspace quick navigation audit link');
  const sectionLinksBeforeCreate = collectNodes(root, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#plan-'));
  const expectedSectionLinksBeforeCreate = [
    ['/?page=plan&deployment=api-prod#plan-runtime-signals', 'Runtime signals'],
    ['/?page=plan&deployment=api-prod#plan-rollout-execution', 'Rollout execution'],
    ['/?page=plan&deployment=api-prod#plan-approval-guidance', 'Derived approval guidance'],
    ['/?page=plan&deployment=api-prod#plan-preview-actions', 'Plan preview actions'],
    ['/?page=plan&deployment=api-prod#plan-apply-actions', 'Apply request actions'],
    ['/?page=plan&deployment=api-prod#plan-operations', 'Plan operations'],
  ];
  if (sectionLinksBeforeCreate.length !== expectedSectionLinksBeforeCreate.length) throw new Error('expected only rendered plan section links before create, got ' + sectionLinksBeforeCreate.length);
  expectedSectionLinksBeforeCreate.forEach(([href, label]) => {
    const link = sectionLinksBeforeCreate.find((node) => node.href === href && node.textContent === label);
    if (!link) throw new Error('missing pre-create plan section link ' + label + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected pre-create plan section link to stay in-shell for ' + label);
  });
  if (sectionLinksBeforeCreate.find((node) => node.href.includes('#plan-stored-preview') || node.href.includes('#plan-stored-apply') || node.href.includes('#plan-apply-operations'))) {
    throw new Error('did not expect plan quick navigation links for unrendered stored preview/apply sections before create');
  }
  if (!text.includes('No stored plan preview yet')) throw new Error('expected idle plan-preview heading');
  if (!text.includes('Create Plan will POST /v1/plans/deployments/api-prod')) throw new Error('expected idle create-plan guidance');
  if (!text.includes('Apply Plan activates only when a loaded persisted ready plan preview with a valid planId is present.')) throw new Error('expected idle apply gating guidance');
  if (!text.includes('No immutable plan preview has been created yet.')) throw new Error('expected derived preview intro before create');
  if (!text.includes('Plan operations')) throw new Error('expected plan operations section');
  if (!text.includes('CreateCodeDeployDeployment')) throw new Error('expected derived rollout operation before create');
  if (!text.includes('Moving 10% traffic to the green task set')) throw new Error('expected derived rollout detail before create');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  let createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  let applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  let refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  let refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlan || !applyPlan || !refreshPlan || !refreshApply) throw new Error('expected plan workspace action buttons before create');
  if (createPlan.disabled || !applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan disabled before create');
  if (!refreshPlan.disabled || !refreshApply.disabled) throw new Error('expected refresh buttons disabled before persisted records exist');
  createPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/deployments/api-prod') throw new Error('unexpected fetch sequence after create: ' + fetchCalls.join(', '));
  if (!createRequestOptions || createRequestOptions.method !== 'POST') throw new Error('expected Create Plan to POST');
  if (!createRequestOptions.headers || createRequestOptions.headers['Content-Type'] !== 'application/json') throw new Error('expected Create Plan content type header');
  const createBody = JSON.parse(createRequestOptions.body || '{}');
  if (!/^plan_[0-9a-f]{16}$/.test(createBody.ifMatch || '')) throw new Error('expected Create Plan to send derived ifMatch fingerprint, got ' + createRequestOptions.body);
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123') throw new Error('expected Create Plan to persist planId in URL, got ' + window.location.search);
  text = collectText(root).join(' | ');
  if (!text.includes('Plan preview ready')) throw new Error('expected ready plan-preview heading');
  if (!text.includes('Immutable plan preview plan_ready_123 is loaded')) throw new Error('expected ready state message');
  if (!text.includes('Plan preview approval guidance')) throw new Error('expected server-authored approval guidance heading');
  if (!text.includes('Server-authored approval summary')) throw new Error('expected server-authored approval summary');
  if (!text.includes('Approval required') || !text.includes('Yes')) throw new Error('expected approval required indicator');
  if (!text.includes('Stored plan preview')) throw new Error('expected stored plan preview section');
  if (!text.includes('fp_plan_ready')) throw new Error('expected stored source fingerprint');
  if (!text.includes('Server-authored plan preview loaded from GET /v1/plans/plan_ready_123.')) throw new Error('expected server-authored operations intro');
  if (!text.includes('server-preview-target')) throw new Error('expected server-authored target');
  if (!text.includes('Server-authored preview summary')) throw new Error('expected server-authored operation summary');
  const links = collectNodes(root, (node) => node.tagName === 'A');
  const sectionLinksAfterCreate = collectNodes(root, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#plan-'));
  const expectedSectionLinksAfterCreate = [
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-runtime-signals', 'Runtime signals'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-rollout-execution', 'Rollout execution'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-approval-guidance', 'Plan preview approval guidance'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-preview-actions', 'Plan preview actions'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-stored-preview', 'Stored plan preview'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-apply-actions', 'Apply request actions'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123#plan-operations', 'Plan operations'],
  ];
  if (sectionLinksAfterCreate.length !== expectedSectionLinksAfterCreate.length) throw new Error('expected only rendered plan section links after preview load, got ' + sectionLinksAfterCreate.length);
  expectedSectionLinksAfterCreate.forEach(([href, label]) => {
    const link = sectionLinksAfterCreate.find((node) => node.href === href && node.textContent === label);
    if (!link) throw new Error('missing post-preview plan section link ' + label + ' -> ' + href);
  });
  if (sectionLinksAfterCreate.find((node) => node.href.includes('#plan-stored-apply') || node.href.includes('#plan-apply-details') || node.href.includes('#plan-apply-operations'))) {
    throw new Error('did not expect apply section links before an apply record is rendered');
  }
  const workspaceLink = links.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123' && node.textContent === 'Open bookmarked workspace');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected bookmarked workspace self link after create');
  const planJSONLink = links.find((node) => node.href === '/v1/plans/plan_ready_123' && node.textContent === 'Plan JSON');
  if (!planJSONLink || planJSONLink.target !== '_blank' || planJSONLink.rel !== 'noreferrer noopener') throw new Error('expected plan json read-model link after create');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlan || !applyPlan || !refreshPlan || !refreshApply) throw new Error('expected plan workspace buttons after create');
  if (createPlan.disabled || applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan enabled after ready preview load');
  if (refreshPlan.disabled || !refreshApply.disabled) throw new Error('expected only plan refresh enabled after preview load');
  applyPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/deployments/api-prod, /v1/applies/plans/plan_ready_123') throw new Error('unexpected fetch sequence after apply create: ' + fetchCalls.join(', '));
  if (!createApplyRequestOptions || createApplyRequestOptions.method !== 'POST') throw new Error('expected Apply Plan to POST');
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123') throw new Error('expected Apply Plan to persist applyId in URL, got ' + window.location.search);
  text = collectText(root).join(' | ');
  if (!text.includes('Apply request failed')) throw new Error('expected apply failed heading after create');
  if (!text.includes('Immutable apply request apply_failed_123 was created and recorded as failed.')) throw new Error('expected apply failed create message');
  if (!text.includes('Stored apply request')) throw new Error('expected stored apply request section');
  if (!text.includes('Apply lifecycle summary')) throw new Error('expected apply lifecycle summary after create');
  if (!text.includes('0/1 terminal · 0 running · 1 queued')) throw new Error('expected queued lifecycle progress summary after create');
  if (text.includes('Apply lifecycle metadata')) throw new Error('expected failed/default apply record to omit lifecycle metadata section when lifecycle fields are missing');
  if (!text.includes('Apply operations')) throw new Error('expected apply operations section');
  if (!text.includes('Immutable apply request loaded from GET /v1/applies/apply_failed_123.')) throw new Error('expected apply operations intro');
  if (!text.includes('Apply step 1')) throw new Error('expected synthetic apply-step label');
  if (!text.includes('Queued apply operation')) throw new Error('expected queued apply-operation label');
  if (!text.includes('server-preview-target')) throw new Error('expected apply operations target');
  if (!text.includes('Server-authored preview summary')) throw new Error('expected apply operations summary');
  if (!text.includes('Apply request details')) throw new Error('expected apply request details section');
  if (!text.includes('apply execution is disabled')) throw new Error('expected apply failure reason');
  if (!text.includes('Tenet persisted this immutable apply request in read-only mode and did not contact AWS.')) throw new Error('expected immutable apply detail');
  if (!text.includes('Live execution is intentionally disabled by default for this backend slice.')) throw new Error('expected disabled-by-default apply detail');
  const applyLinks = collectNodes(root, (node) => node.tagName === 'A');
  const sectionLinksAfterApply = collectNodes(root, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#plan-'));
  const expectedSectionLinksAfterApply = [
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-runtime-signals', 'Runtime signals'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-rollout-execution', 'Rollout execution'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-approval-guidance', 'Plan preview approval guidance'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-preview-actions', 'Plan preview actions'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-stored-preview', 'Stored plan preview'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-apply-actions', 'Apply request actions'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-stored-apply', 'Stored apply request'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-apply-details', 'Apply request details'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-apply-operations', 'Apply operations'],
    ['/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-operations', 'Plan operations'],
  ];
  if (sectionLinksAfterApply.length !== expectedSectionLinksAfterApply.length) throw new Error('expected rendered plan/apply section links after apply create, got ' + sectionLinksAfterApply.length);
  expectedSectionLinksAfterApply.forEach(([href, label]) => {
    const link = sectionLinksAfterApply.find((node) => node.href === href && node.textContent === label);
    if (!link) throw new Error('missing post-apply plan section link ' + label + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected post-apply plan section link to stay in-shell for ' + label);
  });
  const updatedWorkspaceLink = applyLinks.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked workspace');
  if (!updatedWorkspaceLink || updatedWorkspaceLink.target !== '_self' || updatedWorkspaceLink.rel) throw new Error('expected bookmarked workspace self link to include applyId after apply create');
  const bookmarkedApplyLink = applyLinks.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked apply');
  if (!bookmarkedApplyLink || bookmarkedApplyLink.target !== '_self' || bookmarkedApplyLink.rel) throw new Error('expected bookmarked apply self link after apply create');
  const applyJSONLink = applyLinks.find((node) => node.href === '/v1/applies/apply_failed_123' && node.textContent === 'Apply JSON');
  if (!applyJSONLink || applyJSONLink.target !== '_blank' || applyJSONLink.rel !== 'noreferrer noopener') throw new Error('expected apply json read-model link after apply create');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!refreshPlan || !refreshApply) throw new Error('expected refresh buttons after apply create');
  if (refreshPlan.disabled || refreshApply.disabled) throw new Error('expected refresh buttons enabled after apply create');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("create plan unauthorized explains trusted front door auth requirement", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/deployments/api-prod') {
    if (options.method !== 'POST') throw new Error('expected create plan POST');
    return Promise.resolve({
      ok: false,
      status: 401,
      text: () => Promise.resolve('unauthenticated: authenticated identity is required for this operator write route'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  if (!createPlan) throw new Error('expected Create Plan button before unauthorized create');
  createPlan.dispatch('click');
  await waitForFetchCount(fetchCalls, 2);
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/deployments/api-prod') throw new Error('unexpected fetch sequence after unauthorized create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod') throw new Error('expected unauthorized create to preserve workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Authenticated operator required')) throw new Error('expected operator-auth heading for unauthorized create');
  if (!text.includes('Tenet rejected POST /v1/plans/deployments/api-prod because authenticated identity is required on operator write routes.')) throw new Error('expected operator-facing create auth explanation');
  if (!text.includes('retry Create Plan for this same bookmarkable workspace through the trusted front door so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet sends the write.')) throw new Error('expected trusted front door create guidance');
  if (text.includes('Failed to create a plan preview')) throw new Error('expected unauthorized create to avoid generic fetch-error copy');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlanAfter = buttons.find((node) => node.textContent === 'Create Plan');
  const applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  const refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  const refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlanAfter || !applyPlan || !refreshPlan || !refreshApply) throw new Error('expected plan workspace action buttons after unauthorized create');
  if (createPlanAfter.disabled || !applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan disabled after unauthorized create');
  if (!refreshPlan.disabled || !refreshApply.disabled) throw new Error('expected refresh buttons disabled without stored records after unauthorized create');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("create plan forbidden explains deployment access denial", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/deployments/api-prod') {
    if (options.method !== 'POST') throw new Error('expected create plan POST');
    return Promise.resolve({
      ok: false,
      status: 403,
      text: () => Promise.resolve('forbidden: operator does not have access to this deployment'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const createPlan = collectNodes(root, (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Create Plan');
  if (!createPlan) throw new Error('expected Create Plan button before forbidden create');
  createPlan.dispatch('click');
  await waitForFetchCount(fetchCalls, 2);
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/deployments/api-prod') throw new Error('unexpected fetch sequence after forbidden create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod') throw new Error('expected forbidden create to preserve workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Operator access denied')) throw new Error('expected access-denied heading for forbidden create');
  if (!text.includes('Tenet rejected POST /v1/plans/deployments/api-prod after recognizing the caller\'s identity, but this operator is not authorized to create plan for that deployment through this route.')) throw new Error('expected operator-facing create access-denied explanation');
  if (!text.includes('Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry Create Plan.')) throw new Error('expected exact-or-scoped grant recovery guidance for create');
  if (!text.includes('Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.')) throw new Error('expected project-environment fail-closed guidance for create');
  if (text.includes('Failed to create a plan preview')) throw new Error('expected forbidden create to avoid generic fetch-error copy');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("create plan forbidden explains operator role requirement", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/deployments/api-prod') {
    if (options.method !== 'POST') throw new Error('expected create plan POST');
    return Promise.resolve({
      ok: false,
      status: 403,
      text: () => Promise.resolve('forbidden: authenticated operator role is required for this operator write route'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const createPlan = collectNodes(root, (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Create Plan');
  if (!createPlan) throw new Error('expected Create Plan button before role-forbidden create');
  createPlan.dispatch('click');
  await waitForFetchCount(fetchCalls, 2);
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/deployments/api-prod') throw new Error('unexpected fetch sequence after role-forbidden create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod') throw new Error('expected role-forbidden create to preserve workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Operator role required')) throw new Error('expected role-required heading for role-forbidden create');
  if (!text.includes('Tenet rejected POST /v1/plans/deployments/api-prod after recognizing the caller\'s identity because this operator write route requires a trusted forwarded operator/admin role.')) throw new Error('expected operator-role explanation for create');
  if (!text.includes('forward X-Tenet-Authenticated-Roles for an operator-capable session.')) throw new Error('expected forwarded-role guidance for create');
  if (text.includes('Failed to create a plan preview')) throw new Error('expected role-forbidden create to avoid generic fetch-error copy');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("apply create unauthorized explains trusted front door auth requirement", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/applies/plans/plan_ready_123') {
    if (options.method !== 'POST') throw new Error('expected apply create POST');
    return Promise.resolve({
      ok: false,
      status: 401,
      text: () => Promise.resolve('unauthenticated: authenticated identity is required for this operator write route'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  if (!applyPlan || applyPlan.disabled) throw new Error('expected Apply Plan enabled before unauthorized apply create');
  applyPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/plans/plan_ready_123') throw new Error('unexpected fetch sequence after unauthorized apply create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123') throw new Error('expected unauthorized apply create to preserve plan workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Authenticated operator required')) throw new Error('expected operator-auth heading for unauthorized apply create');
  if (!text.includes('Auth required')) throw new Error('expected apply auth-required pill copy');
  if (!text.includes('Tenet rejected POST /v1/applies/plans/plan_ready_123 because authenticated identity is required on operator write routes.')) throw new Error('expected operator-facing apply auth explanation');
  if (!text.includes('retry Apply Plan for this same bookmarkable workspace through the trusted front door so it can authenticate you and forward X-Tenet-Authenticated-* identity headers before Tenet sends the write.')) throw new Error('expected trusted front door apply guidance');
  if (text.includes('Failed to create an apply request')) throw new Error('expected unauthorized apply create to avoid generic fetch-error copy');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  const applyPlanAfter = buttons.find((node) => node.textContent === 'Apply Plan');
  const refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  const refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlan || !applyPlanAfter || !refreshPlan || !refreshApply) throw new Error('expected plan workspace action buttons after unauthorized apply create');
  if (createPlan.disabled || applyPlanAfter.disabled) throw new Error('expected create/apply actions to stay enabled after unauthorized apply create');
  if (refreshPlan.disabled || !refreshApply.disabled) throw new Error('expected only plan refresh enabled after unauthorized apply create');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("apply create forbidden explains operator role requirement", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/applies/plans/plan_ready_123') {
    if (options.method !== 'POST') throw new Error('expected apply create POST');
    return Promise.resolve({
      ok: false,
      status: 403,
      text: () => Promise.resolve('forbidden: authenticated operator role is required for this operator write route'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const applyPlan = collectNodes(root, (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Apply Plan');
  if (!applyPlan || applyPlan.disabled) throw new Error('expected Apply Plan enabled before role-forbidden apply create');
  applyPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/plans/plan_ready_123') throw new Error('unexpected fetch sequence after role-forbidden apply create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123') throw new Error('expected role-forbidden apply create to preserve plan workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Operator role required')) throw new Error('expected role-required heading for role-forbidden apply create');
  if (!text.includes('Tenet rejected POST /v1/applies/plans/plan_ready_123 after recognizing the caller\'s identity because this operator write route requires a trusted forwarded operator/admin role.')) throw new Error('expected operator-role explanation for apply');
  if (!text.includes('forward X-Tenet-Authenticated-Roles for an operator-capable session.')) throw new Error('expected forwarded-role guidance for apply');
  if (text.includes('Failed to create an apply request')) throw new Error('expected role-forbidden apply create to avoid generic fetch-error copy');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("apply create forbidden explains deployment access denial", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/applies/plans/plan_ready_123') {
    if (options.method !== 'POST') throw new Error('expected apply create POST');
    return Promise.resolve({
      ok: false,
      status: 403,
      text: () => Promise.resolve('forbidden: operator does not have access to this deployment'),
    });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const applyPlan = collectNodes(root, (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Apply Plan');
  if (!applyPlan || applyPlan.disabled) throw new Error('expected Apply Plan enabled before forbidden apply create');
  applyPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/plans/plan_ready_123') throw new Error('unexpected fetch sequence after forbidden apply create: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123') throw new Error('expected forbidden apply create to preserve plan workspace URL, got ' + window.location.search);
  const text = collectText(root).join(' | ');
  if (!text.includes('Operator access denied')) throw new Error('expected access-denied heading for forbidden apply create');
  if (!text.includes('Tenet rejected POST /v1/applies/plans/plan_ready_123 after recognizing the caller\'s identity, but this operator is not authorized to apply plan for that deployment through this route.')) throw new Error('expected operator-facing apply access-denied explanation');
  if (!text.includes('Ask a front-door/RBAC admin to authorize this deployment through either X-Tenet-Authorized-Deployments or matching X-Tenet-Authorized-Projects plus X-Tenet-Authorized-Environments catalog placement, then retry Apply Plan.')) throw new Error('expected exact-or-scoped grant recovery guidance for apply');
  if (!text.includes('Partial, stale, or unavailable project/environment placement still fails closed, so use the Operator context banner to confirm the forwarded scopes match this deployment.')) throw new Error('expected project-environment fail-closed guidance for apply');
  if (text.includes('Failed to create an apply request')) throw new Error('expected forbidden apply create to avoid generic fetch-error copy');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("bookmarked plan id loads stored failed preview", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewFailedJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_failed_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_failed_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_failed_123') throw new Error('expected deployment detail plus bookmarked plan preview fetch, got ' + fetchCalls.join(', '));
  const root = document.getElementById('accounts');
  const text = collectText(root).join(' | ');
  if (!text.includes('Plan preview failed')) throw new Error('expected failed plan-preview heading');
  if (!text.includes('Immutable plan preview plan_failed_123 is recorded as failed.')) throw new Error('expected failed preview message');
  if (!text.includes('Plan preview approval guidance')) throw new Error('expected preview approval heading for bookmarked plan');
  if (!text.includes('Recreate the preview from the latest deployment detail before approval.')) throw new Error('expected failed preview approval summary');
  if (!text.includes('Stored plan preview')) throw new Error('expected stored preview metadata section');
  if (!text.includes('fp_plan_failed')) throw new Error('expected failed preview fingerprint');
  if (!text.includes('Plan preview errors')) throw new Error('expected failed preview errors section');
  if (!text.includes('stale source fingerprint')) throw new Error('expected failed preview error text');
  if (!text.includes('No server-authored plan operations are stored for this preview.')) throw new Error('expected empty stored operations message');
  const links = collectNodes(root, (node) => node.tagName === 'A');
  const workspaceLink = links.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_failed_123' && node.textContent === 'Open bookmarked workspace');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected failed preview bookmarked workspace self link');
  const planJSONLink = links.find((node) => node.href === '/v1/plans/plan_failed_123' && node.textContent === 'Plan JSON');
  if (!planJSONLink || planJSONLink.target !== '_blank' || planJSONLink.rel !== 'noreferrer noopener') throw new Error('expected failed preview plan json read-model link');
  const buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  const applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  const refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  const refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlan || !applyPlan || !refreshPlan || !refreshApply) throw new Error('expected plan workspace action buttons on bookmarked plan');
  if (createPlan.disabled || !applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan disabled on bookmarked failed plan');
  if (refreshPlan.disabled || !refreshApply.disabled) throw new Error('expected only plan refresh enabled on bookmarked failed plan');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("bookmarked apply id loads stored immutable apply record", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyRecord = ` + string(applyRecordFailedJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/applies/apply_failed_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(applyRecord) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_failed_123') throw new Error('expected deployment detail plus bookmarked plan/apply fetches, got ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123') throw new Error('expected bookmarked apply URL to stay stable, got ' + window.location.search);
  const root = document.getElementById('accounts');
  const text = collectText(root).join(' | ');
  if (!text.includes('Plan preview ready')) throw new Error('expected ready plan preview heading on bookmarked apply');
  if (!text.includes('Apply request failed')) throw new Error('expected failed apply heading on bookmarked apply');
  if (!text.includes('Immutable apply request apply_failed_123 is loaded and recorded as failed.')) throw new Error('expected loaded failed apply message');
  if (!text.includes('Stored apply request')) throw new Error('expected stored apply request section');
  if (!text.includes('Apply lifecycle summary')) throw new Error('expected apply lifecycle summary on bookmarked apply');
  if (!text.includes('0/1 terminal · 0 running · 1 queued')) throw new Error('expected queued lifecycle progress summary on bookmarked apply');
  if (text.includes('Apply lifecycle metadata')) throw new Error('expected bookmarked failed/default apply record to omit lifecycle metadata section when lifecycle fields are missing');
  if (!text.includes('Apply operations')) throw new Error('expected apply operations section on bookmarked apply');
  if (!text.includes('Immutable apply request loaded from GET /v1/applies/apply_failed_123.')) throw new Error('expected apply operations intro on bookmarked apply');
  if (!text.includes('Apply step 1')) throw new Error('expected synthetic apply-step label on bookmarked apply');
  if (!text.includes('Queued apply operation')) throw new Error('expected queued apply-operation label on bookmarked apply');
  if (!text.includes('server-preview-target')) throw new Error('expected apply operations target on bookmarked apply');
  if (!text.includes('Server-authored preview summary')) throw new Error('expected apply operations summary on bookmarked apply');
  if (!text.includes('Apply request details')) throw new Error('expected apply request details section');
  if (!text.includes('apply execution is disabled')) throw new Error('expected stored apply error');
  if (!text.includes('Tenet persisted this immutable apply request in read-only mode and did not contact AWS.')) throw new Error('expected immutable apply detail on bookmarked apply');
  const links = collectNodes(root, (node) => node.tagName === 'A');
  const workspaceLink = links.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked workspace');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected bookmarked workspace self link to retain applyId');
  const bookmarkedApplyLink = links.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked apply');
  if (!bookmarkedApplyLink || bookmarkedApplyLink.target !== '_self' || bookmarkedApplyLink.rel) throw new Error('expected bookmarked apply self link');
  const applyJSONLink = links.find((node) => node.href === '/v1/applies/apply_failed_123' && node.textContent === 'Apply JSON');
  if (!applyJSONLink || applyJSONLink.target !== '_blank' || applyJSONLink.rel !== 'noreferrer noopener') throw new Error('expected bookmarked apply json link');
  const buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  const applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  const refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  const refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!createPlan || !applyPlan || !refreshPlan || !refreshApply) throw new Error('expected plan workspace action buttons on bookmarked apply');
  if (createPlan.disabled || applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan enabled on bookmarked apply backed by ready preview');
  if (refreshPlan.disabled || refreshApply.disabled) throw new Error('expected refresh buttons enabled on bookmarked apply');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("bookmarked plan hash deep links re-scroll after async plan and apply sections render", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyRecord = ` + string(applyRecordFailedJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION' && (node.selected || this.value === '')) { this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) {
    if (this.byID.has(id)) return this.byID.get(id) || null;
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.id === id) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
  querySelector(selector) {
    const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/);
    if (!dataMatch) return null;
    const target = dataMatch[1];
    const stack = Array.from(this.byID.values());
    while (stack.length) {
      const node = stack.shift();
      if (node?.dataset?.toolbarField === target) return node;
      stack.push(...(node.children || []));
    }
    return null;
  }
}
const collectNodes = (root, predicate) => {
  const matches = [];
  const visit = (node) => {
    if (!node) return;
    if (predicate(node)) matches.push(node);
    (node.children || []).forEach(visit);
  };
  visit(root);
  return matches;
};
const createWindow = (initialSearch, hash) => {
  const listeners = {};
  return {
    location: { pathname: '/console', search: initialSearch, hash },
    addEventListener(type, handler) {
      listeners[type] = listeners[type] || [];
      listeners[type].push(handler);
    },
    dispatchEvent(event) {
      const handlers = listeners[event?.type] || [];
      handlers.forEach((handler) => handler.call(this, event));
    },
    dispatchHashChange() {
      this.dispatchEvent({ type: 'hashchange', target: this });
    },
  };
};
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const initialSearch = '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123';
const window = createWindow(initialSearch, '#plan-apply-operations');
const history = { replaceState(_state, _title, url) {
  const [pathAndQuery, hash = ''] = String(url).split('#');
  const [pathname, query = ''] = pathAndQuery.split('?');
  window.location.pathname = pathname || '/console';
  window.location.search = query ? '?' + query : '';
  window.location.hash = hash ? '#' + hash : '';
} };
const fetchCalls = [];
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (String(url) === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (String(url) === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (String(url) === '/v1/applies/apply_failed_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(applyRecord) });
  throw new Error('unexpected fetch url: ' + url);
};
(async () => {
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_failed_123') throw new Error('expected bookmarked plan/apply hash route to load deployment detail plus stored plan/apply records');
  if (window.location.pathname !== '/console') throw new Error('expected plan workspace pathname to stay stable, got ' + window.location.pathname);
  if (window.location.search !== initialSearch) throw new Error('expected plan workspace query to stay stable, got ' + window.location.search);
  if (window.location.hash !== '#plan-apply-operations') throw new Error('expected plan workspace hash to stay stable, got ' + window.location.hash);
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'plan-apply-operations') throw new Error('expected plan workspace hash to re-scroll matching apply section after async render');
  const accounts = document.getElementById('accounts');
  const quickNavChips = collectNodes(accounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const currentSectionChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current section: Apply operations');
  if (!currentSectionChip) throw new Error('expected bookmarked plan/apply hash route to render the current-section chip');
  const sectionIDs = collectNodes(accounts, (node) => typeof node.id === 'string' && node.id.startsWith('plan-')).map((node) => node.id).sort();
  const expectedSectionIDs = ['plan-apply-actions', 'plan-apply-details', 'plan-apply-operations', 'plan-approval-guidance', 'plan-operations', 'plan-preview-actions', 'plan-rollout-execution', 'plan-runtime-signals', 'plan-stored-apply', 'plan-stored-preview'];
  if (JSON.stringify(sectionIDs) !== JSON.stringify(expectedSectionIDs)) throw new Error('unexpected plan section ids: ' + JSON.stringify(sectionIDs));
  const sectionLinks = collectNodes(accounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#plan-'));
  const expectedLinks = [
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-runtime-signals', 'Runtime signals'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-rollout-execution', 'Rollout execution'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-approval-guidance', 'Plan preview approval guidance'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-preview-actions', 'Plan preview actions'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-stored-preview', 'Stored plan preview'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-apply-actions', 'Apply request actions'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-stored-apply', 'Stored apply request'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-apply-details', 'Apply request details'],
    ['/console?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123#plan-operations', 'Plan operations'],
  ];
  if (sectionLinks.length !== expectedLinks.length) throw new Error('expected bookmarked plan/apply route to omit the active section from On this page links, got ' + sectionLinks.length);
  expectedLinks.forEach(([href, text]) => {
    const link = sectionLinks.find((node) => node.href === href && node.textContent === text);
    if (!link) throw new Error('missing plan section link ' + text + ' -> ' + href);
    if (link.target !== '_self' || link.rel) throw new Error('expected plan section link to stay in-shell for ' + text);
  });
  if (sectionLinks.find((node) => node.textContent === 'Apply operations')) throw new Error('expected active plan section to be removed from On this page links');
  window.location.hash = '#plan-stored-apply';
  window.dispatchHashChange();
  await settle();
  const updatedAccounts = document.getElementById('accounts');
  const updatedChips = collectNodes(updatedAccounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const storedApplyChip = updatedChips.find((node) => node.getAttribute('aria-label') === 'Current section: Stored apply request');
  if (!storedApplyChip) throw new Error('expected plan/apply hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedAccounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#plan-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Stored apply request')) throw new Error('expected updated plan/apply current section to be removed from On this page links after hashchange');
  if (!updatedSectionLinks.find((node) => node.textContent === 'Apply operations')) throw new Error('expected previous plan/apply current section to return to On this page links after hashchange');
  if (!document.lastScrolledTarget || document.lastScrolledTarget.id !== 'plan-stored-apply') throw new Error('expected plan/apply hashchange to scroll the new current section into view');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("bookmarked plan and apply loads show operator read guidance for auth failures", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
` + domHarness + `
const scenarios = [
  {
    name: 'bookmarked plan preview 401',
    search: '?page=plan&deployment=api-prod&planId=plan_ready_123',
    expectedCalls: '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123',
    fetch(url) {
      if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
      if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: operator identity missing') });
      throw new Error('unexpected fetch url for ' + this.name + ': ' + url);
    },
    expected: [
      'Authenticated operator required',
      'Tenet rejected GET /v1/plans/plan_ready_123 because authenticated identity is required on this operator read route.',
      'Deployment-derived context is still shown below and Apply Plan stays disabled until a persisted ready preview is available.',
      'Derived approval guidance',
      'Plan preview actions',
    ],
    unexpected: 'Failed to load the selected plan preview',
  },
  {
    name: 'bookmarked apply request 403',
    search: '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123',
    expectedCalls: '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_failed_123',
    fetch(url) {
      if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
      if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
      if (url === '/v1/applies/apply_failed_123') return Promise.resolve({ ok: false, status: 403, text: () => Promise.resolve('forbidden: operator does not have access to this deployment') });
      throw new Error('unexpected fetch url for ' + this.name + ': ' + url);
    },
    expected: [
      'Operator access denied',
      'Tenet rejected GET /v1/applies/apply_failed_123 after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.',
      'Deployment and plan-derived context are still shown below.',
      'Stored plan preview',
      'Server-authored approval summary',
    ],
    unexpected: 'Failed to load the selected apply request',
  },
  {
    name: 'bookmarked apply request 403 explains viewer role requirement',
    search: '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123',
    expectedCalls: '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_failed_123',
    fetch(url) {
      if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
      if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
      if (url === '/v1/applies/apply_failed_123') return Promise.resolve({ ok: false, status: 403, text: () => Promise.resolve('forbidden: authenticated viewer-capable role is required for this operator read route') });
      throw new Error('unexpected fetch url for ' + this.name + ': ' + url);
    },
    expected: [
      'Viewer role required',
      'Tenet rejected GET /v1/applies/apply_failed_123 after recognizing the caller\'s identity because this operator read route requires a trusted forwarded viewer/operator/admin role.',
      'Deployment and plan-derived context are still shown below.',
      'Stored plan preview',
      'Server-authored approval summary',
    ],
    unexpected: 'Failed to load the selected apply request',
  },
];

(async () => {
  for (const scenario of scenarios) {
    const document = new Document();
    document.byID.set('toolbar', document.createElement('section'));
    document.byID.set('summary', document.createElement('section'));
    document.byID.set('accounts', document.createElement('section'));
    const fetchCalls = [];
    const initialSearch = scenario.search;
    const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
    const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
    const fetch = (url) => {
      fetchCalls.push(String(url));
      return scenario.fetch(url);
    };
    new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
    await settle();
    await settle();
    await settle();
    if (fetchCalls.join(', ') !== scenario.expectedCalls) throw new Error('unexpected fetch sequence for ' + scenario.name + ': ' + fetchCalls.join(', '));
    if (window.location.search !== initialSearch) throw new Error('expected bookmarkable URL to stay unchanged for ' + scenario.name + ', got ' + window.location.search);
    const root = document.getElementById('accounts');
    const text = collectText(root).join(' | ');
    scenario.expected.forEach((fragment) => {
      if (!text.includes(fragment)) throw new Error('expected fragment for ' + scenario.name + ': ' + fragment + '\nUI text: ' + text);
    });
    if (scenario.unexpected && text.includes(scenario.unexpected)) throw new Error('unexpected generic failure copy for ' + scenario.name + ': ' + text);
  }
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("refresh controls reload bookmarked plan and apply records without changing the URL", func(t *testing.T) {
		applyRecordQueuedJSON, err := json.Marshal(map[string]any{
			"id":             "apply_refresh_123",
			"planId":         "plan_ready_123",
			"deploymentName": "api-prod",
			"status":         "queued",
			"createdAt":      "2026-04-18T08:00:00Z",
			"operations": []map[string]any{
				{"status": "queued", "phase": "Preparing", "target": "queue-target", "summary": "Queued summary"},
			},
			"lifecycle": map[string]any{
				"updatedAt":   "2026-04-18T08:01:00Z",
				"currentStep": 1,
				"totalSteps":  2,
			},
			"source": map[string]any{
				"planId":        "plan_ready_123",
				"route":         "/v1/plans/plan_ready_123",
				"planStatus":    "ready",
				"planCreatedAt": "2026-04-17T09:00:00Z",
				"fingerprint":   "fp_plan_ready",
			},
		})
		if err != nil {
			t.Fatalf("marshal queued refresh apply record: %v", err)
		}

		applyRecordRunningJSON, err := json.Marshal(map[string]any{
			"id":             "apply_refresh_123",
			"planId":         "plan_ready_123",
			"deploymentName": "api-prod",
			"status":         "running",
			"createdAt":      "2026-04-18T08:00:00Z",
			"operations": []map[string]any{
				{"status": "succeeded", "phase": "Preparing", "target": "queue-target", "summary": "Queued summary"},
				{"status": "running", "phase": "Promoting", "target": "promote-target", "summary": "Promotion in progress"},
			},
			"lifecycle": map[string]any{
				"updatedAt":      "2026-04-18T08:03:00Z",
				"startedAt":      "2026-04-18T08:02:00Z",
				"currentStep":    2,
				"completedSteps": 1,
				"totalSteps":     2,
			},
			"source": map[string]any{
				"planId":        "plan_ready_123",
				"route":         "/v1/plans/plan_ready_123",
				"planStatus":    "ready",
				"planCreatedAt": "2026-04-17T09:00:00Z",
				"fingerprint":   "fp_plan_ready",
			},
		})
		if err != nil {
			t.Fatalf("marshal running refresh apply record: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyQueuedRecord = ` + string(applyRecordQueuedJSON) + `;
const applyRunningRecord = ` + string(applyRecordRunningJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
let applyFetchCount = 0;
let planFetchCount = 0;
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_refresh_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') {
    planFetchCount += 1;
    return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  }
  if (url === '/v1/applies/apply_refresh_123') {
    applyFetchCount += 1;
    return Promise.resolve({ ok: true, json: () => Promise.resolve(applyFetchCount === 1 ? applyQueuedRecord : applyRunningRecord) });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  let text = collectText(root).join(' | ');
  if (!text.includes('Apply request queued')) throw new Error('expected queued apply heading before refresh');
  if (!text.includes('Immutable apply request apply_refresh_123 is loaded from GET /v1/applies/apply_refresh_123 and recorded as queued.')) throw new Error('expected queued apply message before refresh');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  const refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!refreshPlan || !refreshApply) throw new Error('expected refresh buttons on bookmarked apply');
  if (refreshPlan.disabled || refreshApply.disabled) throw new Error('expected refresh buttons enabled on bookmarked apply');
  refreshPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (planFetchCount !== 2) throw new Error('expected plan refresh to re-fetch stored preview');
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_refresh_123') throw new Error('expected plan refresh to preserve workspace URL');
  refreshApply.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (applyFetchCount !== 2) throw new Error('expected apply refresh to re-fetch stored apply record');
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_refresh_123, /v1/plans/plan_ready_123, /v1/applies/apply_refresh_123') throw new Error('unexpected fetch sequence during refresh monitoring: ' + fetchCalls.join(', '));
  if (window.location.search !== '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_refresh_123') throw new Error('expected apply refresh to preserve workspace URL');
  text = collectText(root).join(' | ');
  if (!text.includes('Apply request running')) throw new Error('expected running apply heading after refresh');
  if (!text.includes('Immutable apply request apply_refresh_123 is loaded from GET /v1/applies/apply_refresh_123 and recorded as running.')) throw new Error('expected running apply message after refresh');
  if (!text.includes('1/2 terminal · 1 running · 0 queued')) throw new Error('expected updated lifecycle summary after refresh');
  if (!text.includes('Current step 2 of 2 · Completed 1 of 2')) throw new Error('expected updated lifecycle metadata after refresh');
  if (!text.includes('Running apply operation')) throw new Error('expected running apply operation after refresh');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("plan and apply refresh auth failures keep bookmarkable context visible", func(t *testing.T) {
		applyRecordQueuedJSON, err := json.Marshal(map[string]any{
			"id":             "apply_refresh_123",
			"planId":         "plan_ready_123",
			"deploymentName": "api-prod",
			"status":         "queued",
			"createdAt":      "2026-04-18T08:00:00Z",
			"operations": []map[string]any{
				{"status": "queued", "phase": "Preparing", "target": "queue-target", "summary": "Queued summary"},
			},
			"source": map[string]any{
				"planId":        "plan_ready_123",
				"route":         "/v1/plans/plan_ready_123",
				"planStatus":    "ready",
				"planCreatedAt": "2026-04-17T09:00:00Z",
				"fingerprint":   "fp_plan_ready",
			},
		})
		if err != nil {
			t.Fatalf("marshal queued refresh auth apply record: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyQueuedRecord = ` + string(applyRecordQueuedJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
process.on('unhandledRejection', () => {});
const fetchCalls = [];
let planFetchCount = 0;
let applyFetchCount = 0;
const initialSearch = '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_refresh_123';
const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') {
    planFetchCount += 1;
    if (planFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
    return Promise.resolve({ ok: false, status: 401, text: () => Promise.resolve('unauthenticated: operator identity missing') });
  }
  if (url === '/v1/applies/apply_refresh_123') {
    applyFetchCount += 1;
    if (applyFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(applyQueuedRecord) });
    return Promise.resolve({ ok: false, status: 403, text: () => Promise.resolve('forbidden: operator does not have access to this deployment') });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  let text = collectText(root).join(' | ');
  if (!text.includes('Immutable apply request apply_refresh_123 is loaded from GET /v1/applies/apply_refresh_123 and recorded as queued.')) throw new Error('expected queued apply message before refresh auth failures');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  let refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  let refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  let applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  if (!refreshPlan || !refreshApply || !applyPlan) throw new Error('expected workspace controls before refresh auth failures');
  if (refreshPlan.disabled || refreshApply.disabled || applyPlan.disabled) throw new Error('expected refresh and apply controls enabled before refresh auth failures');

  refreshPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  text = collectText(root).join(' | ');
  if (planFetchCount !== 2) throw new Error('expected plan refresh to retry GET /v1/plans/{planId}');
  if (window.location.search !== initialSearch) throw new Error('expected plan refresh auth failure to preserve bookmarkable URL');
  if (!text.includes('Authenticated operator required')) throw new Error('expected plan refresh auth heading');
  if (!text.includes('Tenet rejected GET /v1/plans/plan_ready_123 because authenticated identity is required on this operator read route.')) throw new Error('expected plan refresh auth guidance');
  if (!text.includes('Stored plan preview')) throw new Error('expected stored plan preview to remain visible after plan refresh auth failure');
  if (!text.includes('fp_plan_ready')) throw new Error('expected stored plan fingerprint to remain visible after plan refresh auth failure');
  if (text.includes('Failed to load the selected plan preview')) throw new Error('expected plan refresh auth failure to avoid generic load error copy');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  if (!refreshPlan || !refreshApply || !applyPlan) throw new Error('expected workspace controls after plan refresh auth failure');
  if (refreshPlan.disabled || refreshApply.disabled) throw new Error('expected refresh buttons re-enabled after plan refresh auth failure');
  if (!applyPlan.disabled) throw new Error('expected Apply Plan disabled while plan preview auth failure is active');

  refreshApply.dispatch('click');
  await settle();
  await settle();
  await settle();
  text = collectText(root).join(' | ');
  if (applyFetchCount !== 2) throw new Error('expected apply refresh to retry GET /v1/applies/{applyId}');
  if (window.location.search !== initialSearch) throw new Error('expected apply refresh auth failure to preserve bookmarkable URL');
  if (!text.includes('Operator access denied')) throw new Error('expected apply refresh access-denied heading');
  if (!text.includes('Tenet rejected GET /v1/applies/apply_refresh_123 after recognizing the caller\'s identity, but this operator is not authorized for that route or deployment. Ask an admin to grant the deployment, project, or environment scope for this route, then retry this same bookmarkable page.')) throw new Error('expected apply refresh access-denied guidance');
  if (!text.includes('Stored apply request')) throw new Error('expected stored apply request to remain visible after apply refresh auth failure');
  if (!text.includes('Immutable apply request loaded from GET /v1/applies/apply_refresh_123.')) throw new Error('expected stored apply operations to remain visible after apply refresh auth failure');
  if (text.includes('Failed to load the selected apply request')) throw new Error('expected apply refresh auth failure to avoid generic load error copy');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshPlan = buttons.find((node) => node.textContent === 'Refresh plan preview');
  refreshApply = buttons.find((node) => node.textContent === 'Refresh apply request');
  if (!refreshPlan || !refreshApply) throw new Error('expected refresh buttons after apply refresh auth failure');
  if (refreshPlan.disabled || refreshApply.disabled) throw new Error('expected refresh buttons re-enabled after apply refresh auth failure');
  const expectedCalls = '/v1/overview/deployments/api-prod, /v1/plans/plan_ready_123, /v1/applies/apply_refresh_123, /v1/plans/plan_ready_123, /v1/applies/apply_refresh_123';
  if (fetchCalls.join(', ') !== expectedCalls) throw new Error('unexpected fetch sequence for refresh auth failures: ' + fetchCalls.join(', '));
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("future apply lifecycle states render lifecycle-aware chrome and metadata", func(t *testing.T) {
		cases := []struct {
			name            string
			status          string
			expectedHeading string
			expectedMessage string
			lifecycle       map[string]any
			expectedSummary string
			expectedSteps   string
		}{
			{
				name:            "queued",
				status:          "queued",
				expectedHeading: "Apply request queued",
				expectedMessage: "recorded as queued",
				lifecycle: map[string]any{
					"updatedAt":   "2026-04-18T08:01:00Z",
					"currentStep": 1,
					"totalSteps":  5,
				},
				expectedSummary: "Current step 1 of 5 · Completed — of 5",
				expectedSteps:   "— / 5",
			},
			{
				name:            "running",
				status:          "running",
				expectedHeading: "Apply request running",
				expectedMessage: "recorded as running",
				lifecycle: map[string]any{
					"updatedAt":      "2026-04-18T08:02:00Z",
					"startedAt":      "2026-04-18T08:01:30Z",
					"currentStep":    2,
					"completedSteps": 1,
					"totalSteps":     5,
				},
				expectedSummary: "Current step 2 of 5 · Completed 1 of 5",
				expectedSteps:   "1 / 5",
			},
			{
				name:            "succeeded",
				status:          "succeeded",
				expectedHeading: "Apply request succeeded",
				expectedMessage: "recorded as succeeded",
				lifecycle: map[string]any{
					"updatedAt":      "2026-04-18T08:05:00Z",
					"startedAt":      "2026-04-18T08:01:30Z",
					"finishedAt":     "2026-04-18T08:05:00Z",
					"currentStep":    5,
					"completedSteps": 5,
					"totalSteps":     5,
				},
				expectedSummary: "Current step 5 of 5 · Completed 5 of 5",
				expectedSteps:   "5 / 5",
			},
			{
				name:            "cancelled",
				status:          "cancelled",
				expectedHeading: "Apply request cancelled",
				expectedMessage: "recorded as cancelled",
				lifecycle: map[string]any{
					"updatedAt":      "2026-04-18T08:04:00Z",
					"startedAt":      "2026-04-18T08:01:30Z",
					"finishedAt":     "2026-04-18T08:04:00Z",
					"currentStep":    3,
					"completedSteps": 2,
					"totalSteps":     5,
				},
				expectedSummary: "Current step 3 of 5 · Completed 2 of 5",
				expectedSteps:   "2 / 5",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				applyRecordLifecycleJSON, err := json.Marshal(map[string]any{
					"id":             "apply_lifecycle_123",
					"planId":         "plan_ready_123",
					"deploymentName": "api-prod",
					"status":         tc.status,
					"createdAt":      "2026-04-18T08:00:00Z",
					"operations": []map[string]any{
						{"status": "queued", "phase": "Preparing", "target": "target-queued", "summary": "Queued summary"},
						{"status": "running", "phase": "Promoting", "target": "target-running", "summary": "Running summary"},
						{"status": "succeeded", "phase": "Promoting", "target": "target-succeeded", "summary": "Succeeded summary"},
						{"status": "failed", "phase": "Verifying", "target": "target-failed", "summary": "Failed summary"},
						{"status": "cancelled", "phase": "Cleanup", "target": "target-cancelled", "summary": "Cancelled summary"},
					},
					"lifecycle": tc.lifecycle,
					"source": map[string]any{
						"planId":        "plan_ready_123",
						"route":         "/v1/plans/plan_ready_123",
						"planStatus":    "ready",
						"planCreatedAt": "2026-04-17T09:00:00Z",
						"fingerprint":   "fp_plan_ready",
					},
				})
				if err != nil {
					t.Fatalf("marshal lifecycle apply record: %v", err)
				}

				nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewReadyJSON) + `;
const applyRecord = ` + string(applyRecordLifecycleJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_lifecycle_123', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_ready_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/applies/apply_lifecycle_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(applyRecord) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const text = collectText(root).join(' | ');
  if (!text.includes('` + tc.expectedHeading + `')) throw new Error('expected lifecycle heading for ` + tc.name + `');
  if (!text.includes('` + tc.expectedMessage + `')) throw new Error('expected lifecycle message for ` + tc.name + `');
  if (!text.includes('Apply lifecycle summary')) throw new Error('expected apply lifecycle summary for ` + tc.name + `');
  if (!text.includes('3/5 terminal · 1 running · 1 queued')) throw new Error('expected lifecycle progress summary for ` + tc.name + `');
  if (!text.includes('Apply lifecycle metadata')) throw new Error('expected lifecycle metadata section for ` + tc.name + `');
  if (!text.includes('` + tc.expectedSummary + `')) throw new Error('expected lifecycle metadata summary for ` + tc.name + `');
  if (!/Lifecycle updated\s*\|\s*2026-04-18T08:0[1-5]:00Z/.test(text)) throw new Error('expected lifecycle updated timestamp for ` + tc.name + `');
  if (!/Current lifecycle step\s*\|\s*[1-5]/.test(text)) throw new Error('expected current lifecycle step metric for ` + tc.name + `');
  if (!text.includes('Completed / total steps')) throw new Error('expected completed/total lifecycle metric for ` + tc.name + `');
  if (!text.includes('` + tc.expectedSteps + `')) throw new Error('expected completed/total lifecycle value for ` + tc.name + `');
  if (!text.includes('Queued apply operation')) throw new Error('expected queued apply-operation label for ` + tc.name + `');
  if (!text.includes('Running apply operation')) throw new Error('expected running apply-operation label for ` + tc.name + `');
  if (!text.includes('Succeeded apply operation')) throw new Error('expected succeeded apply-operation label for ` + tc.name + `');
  if (!text.includes('Failed apply operation')) throw new Error('expected failed apply-operation label for ` + tc.name + `');
  if (!text.includes('Cancelled apply operation')) throw new Error('expected cancelled apply-operation label for ` + tc.name + `');
  if ('` + tc.name + `' === 'queued') {
    if (!text.includes('Lifecycle started') || !text.includes('Lifecycle finished')) throw new Error('expected queued lifecycle timing labels to render');
    if (!/Lifecycle started\s*\|\s*—/.test(text)) throw new Error('expected queued lifecycle started placeholder');
    if (!/Lifecycle finished\s*\|\s*—/.test(text)) throw new Error('expected queued lifecycle finished placeholder');
  }
  if ('` + tc.name + `' === 'running') {
    if (!/Lifecycle started\s*\|\s*2026-04-18T08:01:30Z/.test(text)) throw new Error('expected running lifecycle started timestamp');
    if (!/Lifecycle finished\s*\|\s*—/.test(text)) throw new Error('expected running lifecycle finished placeholder');
  }
  if (!/Queued operations\s*\|\s*1/.test(text)) throw new Error('expected queued count for ` + tc.name + `');
  if (!/Running operations\s*\|\s*1/.test(text)) throw new Error('expected running count for ` + tc.name + `');
  if (!/Succeeded operations\s*\|\s*1/.test(text)) throw new Error('expected succeeded count for ` + tc.name + `');
  if (!/Failed operations\s*\|\s*1/.test(text)) throw new Error('expected failed count for ` + tc.name + `');
  if (!/Cancelled operations\s*\|\s*1/.test(text)) throw new Error('expected cancelled count for ` + tc.name + `');
})();
`
				runNodeHarness(t, nodeProgram)
			})
		}
	})

	t.Run("create falls back to loaded preview fingerprint when client cannot derive current fingerprint", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
const planPreview = ` + string(planPreviewFailedJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
let createRequestOptions = null;
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod&planId=plan_failed_123', hash: '' }, crypto: {} };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url, options = {}) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  if (url === '/v1/plans/plan_failed_123') return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  if (url === '/v1/plans/deployments/api-prod') {
    createRequestOptions = options;
    return Promise.resolve({ ok: true, json: () => Promise.resolve(planPreview) });
  }
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  const buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  if (!createPlan) throw new Error('expected Create Plan button on bookmarked failed preview');
  createPlan.dispatch('click');
  await settle();
  await settle();
  await settle();
  if (fetchCalls.join(', ') !== '/v1/overview/deployments/api-prod, /v1/plans/plan_failed_123, /v1/plans/deployments/api-prod') throw new Error('unexpected fetch sequence after fallback create: ' + fetchCalls.join(', '));
  const createBody = JSON.parse((createRequestOptions && createRequestOptions.body) || '{}');
  if (createBody.ifMatch !== 'fp_plan_failed') throw new Error('expected Create Plan to fall back to loaded preview fingerprint, got ' + JSON.stringify(createBody));
})();
`
		runNodeHarness(t, nodeProgram)
	})
}

func TestAuditTimelinePageWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	auditLogsJSON, err := json.Marshal(map[string]any{
		"entries": []map[string]any{
			{
				"timestamp":      "2026-04-17T09:00:00Z",
				"action":         "plan.created",
				"deploymentName": "payments-api-prod",
				"appRef":         "payments-api",
				"environmentRef": "prod-apne2",
				"planId":         "plan_ready_123",
				"actor": map[string]any{
					"available": false,
					"reason":    "actor information is unavailable because no authenticated request context was available for this local request",
				},
				"metadata": map[string]any{
					"status":            "ready",
					"sourceFingerprint": "fp_plan_ready",
				},
				"source": map[string]any{
					"kind":        "planPreview",
					"recordRoute": "/v1/plans/plan_ready_123",
				},
			},
			{
				"timestamp":      "2026-04-18T08:00:00Z",
				"action":         "apply.requested",
				"deploymentName": "payments-api-prod",
				"appRef":         "payments-api",
				"environmentRef": "prod-apne2",
				"planId":         "plan_ready_123",
				"applyId":        "apply_failed_123",
				"actor": map[string]any{
					"available": true,
					"id":        "operator-123",
					"name":      "Jane Operator",
					"email":     "jane@example.com",
				},
				"metadata": map[string]any{
					"status":            "failed",
					"planStatus":        "ready",
					"sourceFingerprint": "fp_plan_ready",
					"error":             "apply execution is disabled",
					"details":           []string{"Tenet persisted this immutable apply request in read-only mode and did not contact AWS."},
				},
				"source": map[string]any{
					"kind":        "applyRequest",
					"recordRoute": "/v1/applies/apply_failed_123",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal audit logs payload: %v", err)
	}

	domHarness := `
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); await Promise.resolve(); }
`

	t.Run("bookmarkable audit page renders newest first with self links", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const auditLogs = ` + string(auditLogsJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/console', search: '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/console'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/audit-logs?deployment=payments-api-prod&limit=2') return Promise.resolve({ ok: true, json: () => Promise.resolve(auditLogs) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/audit-logs?deployment=payments-api-prod&limit=2') throw new Error('expected only filtered audit log fetch on audit page');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected audit page back link');
  if (!toolbarText.includes('Audit filters: deployment payments-api-prod · Limit cap: latest 2')) throw new Error('expected audit toolbar filters');
  if (!summaryText.includes('Operator audit log')) throw new Error('expected audit page summary title');
  if (!summaryText.includes('Deployment audit timeline')) throw new Error('expected deployment-scoped audit summary label');
  if (!summaryText.includes('Immutable plan preview and apply-request activity for deployment payments-api-prod rendered from GET /v1/audit-logs.')) throw new Error('expected deployment-scoped audit summary copy');
  if (!accountsText.includes('Audit timeline')) throw new Error('expected audit page body title');
  if (!accountsText.includes('Active deployment context')) throw new Error('expected deployment context callout');
  if (!accountsText.includes('payments-api-prod')) throw new Error('expected deployment context value');
  if (!accountsText.includes('Apply requested (apply.requested)')) throw new Error('expected newest audit entry label');
  if (!accountsText.includes('Plan preview created (plan.created)')) throw new Error('expected older audit entry label');
  if (!accountsText.includes('2 entries')) throw new Error('expected audit entry count copy');
  if (!accountsText.includes('deployment payments-api-prod')) throw new Error('expected audit deployment copy');
  if (!accountsText.includes('Limit cap: latest 2')) throw new Error('expected audit limit cap copy');
  if (!accountsText.includes('Actor detail')) throw new Error('expected explicit actor label');
  if (!accountsText.includes('Recorded')) throw new Error('expected recorded actor state');
  if (!accountsText.includes('Jane Operator')) throw new Error('expected recorded actor details');
  if (!accountsText.includes('Unavailable')) throw new Error('expected unavailable actor state');
  if (!accountsText.includes('apply execution is disabled')) throw new Error('expected apply metadata in audit row');
  const limitSelect = document.querySelector('[data-toolbar-field="Limit"]');
  if (!limitSelect || limitSelect.value !== '2') throw new Error('expected audit limit select to reflect bookmarkable URL state');
  const refreshButton = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected enabled audit refresh button after initial render');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink || backLink.rel) throw new Error('expected audit page back link to preserve filters');
  const rows = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'DIV' && node.className === 'row');
  if (rows.length < 2) throw new Error('expected audit page rows');
  const rowTexts = rows.map((node) => collectText(node).join(' | '));
  if (!rowTexts[0].includes('Apply requested (apply.requested)')) throw new Error('expected newest audit entry first, got ' + rowTexts.join(' || '));
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const breadcrumbChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className.includes('breadcrumb-current'));
  const applicationsBreadcrumb = links.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Applications' && node.target === '_self');
  const applicationBreadcrumb = links.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'payments-api-prod' && node.target === '_self');
  const quickNavBackLink = links.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Back to application');
  const quickNavDriftLink = links.find((node) => node.href === '/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Drift view');
  const quickNavPlanLink = links.find((node) => node.href === '/console?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Plan/apply');
  const quickNavAuditLink = links.find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2' && node.textContent === 'Audit log');
  const quickNavDeploymentContextLink = links.find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2#audit-deployment-context' && node.textContent === 'Deployment context');
  const quickNavTimelineLink = links.find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2#audit-timeline' && node.textContent === 'Audit timeline');
  const quickNavChips = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const quickNavChip = quickNavChips.find((node) => node.textContent === 'Audit log');
  const applicationLink = links.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Open application drill-down');
  if (!applicationLink || applicationLink.target !== '_self' || applicationLink.rel) throw new Error('expected audit application self link');
  const driftLink = links.find((node) => node.href === '/console?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Open drift view');
  if (!driftLink || driftLink.target !== '_self' || driftLink.rel) throw new Error('expected audit drift self link');
  const workspaceLink = links.find((node) => node.href === '/console?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open plan/apply workspace');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected audit plan/apply self link');
  if (!accountsText.includes('Breadcrumbs')) throw new Error('expected deployment-scoped audit breadcrumb chrome');
  if (!applicationsBreadcrumb || applicationsBreadcrumb.rel) throw new Error('expected audit applications breadcrumb link');
  if (!applicationBreadcrumb || applicationBreadcrumb.rel) throw new Error('expected audit application breadcrumb link');
  if (!breadcrumbChips.find((node) => node.textContent === 'Audit log')) throw new Error('expected audit current breadcrumb chip');
  if (!accountsText.includes('Quick navigation')) throw new Error('expected deployment-scoped audit quick navigation strip');
  if (!accountsText.includes('Deployment workspaces')) throw new Error('expected deployment-scoped audit quick navigation group label');
  if (!accountsText.includes('Current section')) throw new Error('expected deployment-scoped audit current-section quick-nav group');
  if (!accountsText.includes('No section is currently targeted.')) throw new Error('expected deployment-scoped audit empty current-section copy without hash');
  if (!accountsText.includes('On this page')) throw new Error('expected deployment-scoped audit section quick-nav group');
  if (!accountsText.includes('payments-api-prod · Audit log')) throw new Error('expected deployment-scoped audit quick navigation current-state label');
  if (!quickNavBackLink || quickNavBackLink.target !== '_self' || quickNavBackLink.rel) throw new Error('expected audit quick navigation back link');
  if (!quickNavDriftLink || quickNavDriftLink.target !== '_self' || quickNavDriftLink.rel) throw new Error('expected audit quick navigation drift link');
  if (!quickNavPlanLink || quickNavPlanLink.target !== '_self' || quickNavPlanLink.rel) throw new Error('expected audit quick navigation plan link');
  if (!quickNavDeploymentContextLink || quickNavDeploymentContextLink.target !== '_self' || quickNavDeploymentContextLink.rel) throw new Error('expected audit quick navigation deployment-context section link');
  if (!quickNavTimelineLink || quickNavTimelineLink.target !== '_self' || quickNavTimelineLink.rel) throw new Error('expected audit quick navigation timeline section link');
  if (quickNavAuditLink) throw new Error('expected audit quick navigation to render active workspace as a chip');
  if (!quickNavChip) throw new Error('expected audit quick navigation chip');
  if (quickNavChips.find((node) => String(node.getAttribute('aria-label') || '').startsWith('Current section: '))) throw new Error('expected audit route without hash not to render a current-section chip');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("filtered audit empty state explains active filters and clear recovery", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const emptyAuditLogs = { entries: [] };
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/console', search: '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&action=apply.requested&limit=1', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathAndQuery, hash = ''] = String(url).split('#'); const [pathname, query = ''] = pathAndQuery.split('?'); window.location.pathname = pathname || '/console'; window.location.search = query ? '?' + query : ''; window.location.hash = hash ? '#' + hash : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=1') return Promise.resolve({ ok: true, json: () => Promise.resolve(emptyAuditLogs) });
  if (url === '/v1/audit-logs') return Promise.resolve({ ok: true, json: () => Promise.resolve(emptyAuditLogs) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Audit filters: deployment payments-api-prod · action Apply requested (apply.requested) · Limit cap: latest 1')) throw new Error('expected active audit filters in toolbar');
  if (!accountsText.includes('No audit entries match the current filters (deployment payments-api-prod · action Apply requested (apply.requested); Limit cap: latest 1). Clear audit controls to return to the full audit timeline.')) throw new Error('expected filtered audit empty-state recovery copy, got ' + accountsText);
  if (accountsText.includes('No audit activity has been persisted yet for deployment payments-api-prod.')) throw new Error('expected filtered empty state to avoid unfiltered no-activity copy');
  const clearButton = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit filters');
  if (!clearButton) throw new Error('expected clear audit filters button');
  clearButton.dispatch('click');
  await settle();
  await settle();
  if (window.location.search !== '?page=audit&q=payments&sync=OutOfSync&account=210987654321') throw new Error('expected clear filters to preserve shell query state, got ' + window.location.search);
  if (fetchCalls[fetchCalls.length - 1] !== '/v1/audit-logs') throw new Error('expected clear filters to reload unfiltered audit log, got ' + fetchCalls.join(' | '));
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("audit hash route tracks the current section in quick navigation", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const auditLogs = ` + string(auditLogsJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const initialSearch = '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2';
const hashListeners = [];
const window = {
  location: { pathname: '/console', search: initialSearch, hash: '#audit-timeline' },
  addEventListener(type, handler) { if (type === 'hashchange') hashListeners.push(handler); },
  dispatchHashChange() { hashListeners.forEach((handler) => handler({ type: 'hashchange' })); },
};
const history = { replaceState(_state, _title, url) { const [pathAndQuery, hash = ''] = String(url).split('#'); const [pathname, query = ''] = pathAndQuery.split('?'); window.location.pathname = pathname || '/console'; window.location.search = query ? '?' + query : ''; window.location.hash = hash ? '#' + hash : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/audit-logs?deployment=payments-api-prod&limit=2') return Promise.resolve({ ok: true, json: () => Promise.resolve(auditLogs) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/audit-logs?deployment=payments-api-prod&limit=2') throw new Error('expected hash-routed audit page to fetch the current deployment route');
  if (window.location.pathname !== '/console') throw new Error('expected audit pathname to stay stable, got ' + window.location.pathname);
  if (window.location.search !== initialSearch) throw new Error('expected audit query to stay stable, got ' + window.location.search);
  if (window.location.hash !== '#audit-timeline') throw new Error('expected audit hash to stay stable, got ' + window.location.hash);
  const accounts = document.getElementById('accounts');
  const quickNavChips = collectNodes(accounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const currentDestinationChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current destination: Audit log');
  if (!currentDestinationChip) throw new Error('expected audit hash route to keep current-destination chip');
  const currentSectionChip = quickNavChips.find((node) => node.getAttribute('aria-label') === 'Current section: Audit timeline');
  if (!currentSectionChip) throw new Error('expected audit hash to render the current-section chip');
  const sectionLinks = collectNodes(accounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#audit-'));
  if (sectionLinks.length !== 1) throw new Error('expected only one remaining audit section link, got ' + sectionLinks.length);
  const deploymentContextLink = sectionLinks.find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&limit=2#audit-deployment-context' && node.textContent === 'Deployment context');
  if (!deploymentContextLink || deploymentContextLink.target !== '_self' || deploymentContextLink.rel) throw new Error('expected audit hash route deployment-context link to stay in-shell');
  if (sectionLinks.find((node) => node.textContent === 'Audit timeline')) throw new Error('expected active audit section to be removed from On this page links');
  window.location.hash = '#audit-deployment-context';
  window.dispatchHashChange();
  await settle();
  const updatedAccounts = document.getElementById('accounts');
  const updatedChips = collectNodes(updatedAccounts, (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip');
  const deploymentContextChip = updatedChips.find((node) => node.getAttribute('aria-label') === 'Current section: Deployment context');
  if (!deploymentContextChip) throw new Error('expected audit hashchange to update the current-section chip');
  const updatedSectionLinks = collectNodes(updatedAccounts, (node) => node.tagName === 'A' && typeof node.href === 'string' && node.href.includes('#audit-'));
  if (updatedSectionLinks.find((node) => node.textContent === 'Deployment context')) throw new Error('expected updated audit current section to be removed from On this page links');
  if (!updatedSectionLinks.find((node) => node.textContent === 'Audit timeline')) throw new Error('expected previous audit current section to return to On this page links after hashchange');
})();
`

		runNodeHarness(t, nodeProgram)
	})

	t.Run("audit refresh reloads the current route in place without changing bookmarkable state", func(t *testing.T) {
		refreshedAuditLogsJSON, err := json.Marshal(map[string]any{
			"entries": []map[string]any{
				{
					"timestamp":      "2026-04-19T09:00:00Z",
					"action":         "apply.requested",
					"deploymentName": "payments-api-prod",
					"appRef":         "payments-api",
					"environmentRef": "prod-apne2",
					"planId":         "plan_ready_123",
					"applyId":        "apply_running_123",
					"actor": map[string]any{
						"available": true,
						"id":        "operator-123",
						"name":      "Jane Operator",
						"email":     "jane@example.com",
					},
					"metadata": map[string]any{
						"status":            "running",
						"planStatus":        "ready",
						"sourceFingerprint": "fp_plan_ready",
					},
					"source": map[string]any{
						"kind":        "applyRequest",
						"recordRoute": "/v1/applies/apply_running_123",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal refreshed audit logs payload: %v", err)
		}

		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const initialAuditLogs = ` + string(auditLogsJSON) + `;
const refreshedAuditLogs = ` + string(refreshedAuditLogsJSON) + `;
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const route = '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=2';
const initialSearch = '?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2';
const window = { location: { pathname: '/', search: initialSearch, hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
let refreshFetchCount = 0;
let resolveRefresh;
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url !== route) throw new Error('unexpected fetch url: ' + url);
  refreshFetchCount += 1;
  if (refreshFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialAuditLogs) });
  if (refreshFetchCount === 2) return new Promise((resolve) => { resolveRefresh = resolve; });
  throw new Error('unexpected audit fetch count: ' + refreshFetchCount);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const root = document.getElementById('accounts');
  let text = collectText(root).join(' | ');
  if (!text.includes('apply_failed_123')) throw new Error('expected initial audit row before refresh');
  let buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  let refreshButton = buttons.find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected enabled audit refresh button after initial render');
  refreshButton.dispatch('click');
  await settle();
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton || !refreshButton.disabled) throw new Error('expected disabled audit refresh button only while fetch is in flight');
  text = collectText(root).join(' | ');
  if (!text.includes('Refreshing audit log from GET /v1/audit-logs…')) throw new Error('expected audit refresh loading copy');
  if (!text.includes('apply_failed_123')) throw new Error('expected current audit content to remain visible during refresh');
  if (fetchCalls.length !== 2 || fetchCalls[0] !== route || fetchCalls[1] !== route) throw new Error('expected audit refresh to re-fetch only the current audit route');
  if (window.location.search !== initialSearch) throw new Error('expected audit refresh to preserve bookmarkable route');
  resolveRefresh({ ok: true, json: () => Promise.resolve(refreshedAuditLogs) });
  await settle();
  await settle();
  text = collectText(root).join(' | ');
  if (!text.includes('apply_running_123')) throw new Error('expected refreshed audit content to render in place');
  if (text.includes('apply_failed_123')) throw new Error('expected refreshed audit content to replace stale row');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected audit refresh button re-enabled after refresh completes');

  let errorFetchCount = 0;
  const errorDocument = new Document();
  errorDocument.byID.set('toolbar', errorDocument.createElement('section'));
  errorDocument.byID.set('summary', errorDocument.createElement('section'));
  errorDocument.byID.set('accounts', errorDocument.createElement('section'));
  const errorWindow = { location: { pathname: '/', search: initialSearch, hash: '' } };
  const errorHistory = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); errorWindow.location.pathname = pathname || '/'; errorWindow.location.search = query ? '?' + query : ''; } };
  const errorFetch = (url) => {
    if (url !== route) throw new Error('unexpected error fetch url: ' + url);
    errorFetchCount += 1;
    if (errorFetchCount === 1) return Promise.resolve({ ok: true, json: () => Promise.resolve(initialAuditLogs) });
    if (errorFetchCount === 2) return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected error audit fetch count: ' + errorFetchCount);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(errorWindow, errorDocument, errorFetch, errorHistory, Element, InputElement);
  await settle();
  await settle();
  buttons = collectNodes(errorDocument.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton) throw new Error('expected audit refresh button in error scenario');
  refreshButton.dispatch('click');
  await settle();
  await settle();
  text = collectText(errorDocument.getElementById('accounts')).join(' | ');
  if (!text.includes('Failed to refresh audit log: audit log request failed (500): boom')) throw new Error('expected audit refresh error copy');
  if (!text.includes('apply_failed_123')) throw new Error('expected previous audit content to remain visible after refresh error');
  buttons = collectNodes(errorDocument.getElementById('accounts'), (node) => node.tagName === 'BUTTON');
  refreshButton = buttons.find((node) => node.textContent === 'Refresh audit log');
  if (!refreshButton || refreshButton.disabled) throw new Error('expected audit refresh button re-enabled after refresh error');
  if (errorFetchCount !== 2) throw new Error('expected audit refresh error to retry exactly once');
  if (errorWindow.location.search !== initialSearch) throw new Error('expected audit refresh error state to preserve bookmarkable route');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("audit page bookmarkable filters preserve deployment action and limit URL state", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const filteredAuditLogs = { entries: [` + `
  {
    "timestamp": "2026-04-18T08:00:00Z",
    "action": "apply.requested",
    "deploymentName": "payments-api-prod",
    "appRef": "payments-api",
    "environmentRef": "prod-apne2",
    "planId": "plan_ready_123",
    "applyId": "apply_failed_123",
    "actor": { "available": false, "reason": "actor information is unavailable because no authenticated request context was available for this local request" },
    "metadata": { "status": "failed", "planStatus": "ready", "sourceFingerprint": "fp_plan_ready", "error": "apply execution is disabled" },
    "source": { "kind": "applyRequest", "recordRoute": "/v1/applies/apply_failed_123" }
  }
] };
` + domHarness + `
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const fetchCalls = [];
const window = { location: { pathname: '/console', search: '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&action=apply.requested&limit=2', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/console'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=2') return Promise.resolve({ ok: true, json: () => Promise.resolve(filteredAuditLogs) });
  if (url === '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=5') return Promise.resolve({ ok: true, json: () => Promise.resolve(filteredAuditLogs) });
  if (url === '/v1/audit-logs?deployment=payments-api-prod&action=plan.created&limit=5') return Promise.resolve({ ok: true, json: () => Promise.resolve({ entries: [] }) });
  if (url === '/v1/audit-logs') return Promise.resolve({ ok: true, json: () => Promise.resolve(filteredAuditLogs) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=2') throw new Error('expected initial deployment and action filtered audit fetch');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  if (!toolbarText.includes('Audit filters: deployment payments-api-prod · action Apply requested (apply.requested) · Limit cap: latest 2')) throw new Error('expected deployment, action, and limit filters in toolbar');
  let clearFiltersButton = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit filters');
  if (!clearFiltersButton || clearFiltersButton.disabled) throw new Error('expected enabled clear audit filters recovery action while filters are active');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!accountsText.includes('1 entry · newest record first · deployment payments-api-prod · action Apply requested (apply.requested) · Limit cap: latest 2')) throw new Error('expected active deployment, action, and limit filter copy in audit timeline');
  const auditQuickNavLink = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A').find((node) => node.href === '/console?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&action=apply.requested&limit=2' && node.textContent === 'Audit log');
  const auditQuickNavChip = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'SPAN' && node.className === 'quick-nav-chip').find((node) => node.textContent === 'Audit log');
  if (auditQuickNavLink) throw new Error('expected audit quick navigation to render active workspace as a chip while preserving URL state');
  if (!auditQuickNavChip) throw new Error('expected audit quick navigation chip to preserve action and limit state');
  const actionSelect = document.querySelector('[data-toolbar-field="Action"]');
  if (!actionSelect || actionSelect.value !== 'apply.requested') throw new Error('expected action select to reflect URL state');
  const limitSelect = document.querySelector('[data-toolbar-field="Limit"]');
  if (!limitSelect || limitSelect.value !== '2') throw new Error('expected limit select to reflect URL state');
  limitSelect.value = '5';
  limitSelect.dispatch('change', { target: limitSelect });
  await settle();
  await settle();
  if (fetchCalls.length !== 2 || fetchCalls[1] !== '/v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=5') throw new Error('expected updated limit-filtered audit fetch');
  if (window.location.search !== '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&action=apply.requested&limit=5') throw new Error('expected updated bookmarkable audit URL after limit change, got ' + window.location.search);
  actionSelect.value = 'plan.created';
  actionSelect.dispatch('change', { target: actionSelect });
  await settle();
  await settle();
  if (fetchCalls.length !== 3 || fetchCalls[2] !== '/v1/audit-logs?deployment=payments-api-prod&action=plan.created&limit=5') throw new Error('expected updated action-filtered audit fetch');
  if (window.location.search !== '?page=audit&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&action=plan.created&limit=5') throw new Error('expected updated bookmarkable audit URL, got ' + window.location.search);
  clearFiltersButton = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit filters');
  if (!clearFiltersButton || clearFiltersButton.disabled) throw new Error('expected clear audit filters recovery action to stay available while any audit filter is active');
  clearFiltersButton.dispatch('click');
  await settle();
  await settle();
  if (fetchCalls.length !== 4 || fetchCalls[3] !== '/v1/audit-logs') throw new Error('expected clear audit filters to reload the unfiltered audit route');
  if (window.location.search !== '?page=audit&q=payments&sync=OutOfSync&account=210987654321') throw new Error('expected clear audit filters to preserve q/sync/account and reset audit filters, got ' + window.location.search);
  if (window.location.pathname !== '/console') throw new Error('expected clear audit filters to stay on the audit page shell');
  const clearedActionSelect = document.querySelector('[data-toolbar-field="Action"]');
  if (!clearedActionSelect || clearedActionSelect.value !== '') throw new Error('expected action select to reset after clearing audit filters');
  const clearedLimitSelect = document.querySelector('[data-toolbar-field="Limit"]');
  if (!clearedLimitSelect || clearedLimitSelect.value !== '') throw new Error('expected limit select to reset after clearing audit filters');
  if (collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit filters')) throw new Error('did not expect clear audit filters action after filters were reset');
  const clearedToolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  if (!clearedToolbarText.includes('Audit filters: none')) throw new Error('expected cleared audit toolbar copy');
  const clearedAccountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (clearedAccountsText.includes('Active deployment context')) throw new Error('did not expect deployment-scoped callout after clearing audit filters');
  if (clearedAccountsText.includes('Quick navigation')) throw new Error('did not expect deployment quick navigation after clearing audit filters');
  if (clearedAccountsText.includes('deployment payments-api-prod')) throw new Error('did not expect deployment filter copy after clearing audit filters');
  if (clearedAccountsText.includes('action Plan preview created (plan.created)')) throw new Error('did not expect action filter copy after clearing audit filters');
  if (clearedAccountsText.includes('Limit cap: latest 5')) throw new Error('did not expect limit cap copy after clearing audit filters');
})();
`
		runNodeHarness(t, nodeProgram)
	})

	t.Run("audit page renders explicit fetch states", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
` + domHarness + `
function createDocument() {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
}
function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}
async function renderWithFetch(fetchImpl, search = '?page=audit') {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetchImpl, history, Element, InputElement);
  await settle();
  return { document, window };
}
(async () => {
  const pending = deferred();
  const loadingRun = await renderWithFetch(() => pending.promise);
  const loadingText = collectText(loadingRun.document.getElementById('accounts')).join(' | ');
  if (!loadingText.includes('Loading audit log…')) throw new Error('expected audit loading state');
  const clearFiltersButton = collectNodes(loadingRun.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit filters');
  if (clearFiltersButton) throw new Error('did not expect clear audit filters action without active audit filters');
  const loadingRefreshButton = collectNodes(loadingRun.document.getElementById('accounts'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Refresh audit log');
  if (!loadingRefreshButton || !loadingRefreshButton.disabled) throw new Error('expected audit refresh button disabled during initial load');
  pending.resolve({ ok: true, json: () => Promise.resolve({ entries: [] }) });
  await settle();
  await settle();
  const emptyText = collectText(loadingRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No audit activity has been persisted yet.')) throw new Error('expected audit empty state after resolve');
  if (emptyText.includes('Quick navigation')) throw new Error('did not expect audit quick navigation without a deployment filter');

  const limitOnlyRun = await renderWithFetch((url) => {
    if (url !== '/v1/audit-logs?limit=2') throw new Error('unexpected limit-only audit fetch: ' + url);
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ entries: [] }) });
  }, '?page=audit&limit=2');
  await settle();
  await settle();
  const limitToolbarText = collectText(limitOnlyRun.document.getElementById('toolbar')).join(' | ');
  const limitOnlyText = collectText(limitOnlyRun.document.getElementById('accounts')).join(' | ');
  if (!limitToolbarText.includes('Audit filters: none · Limit cap: latest 2')) throw new Error('expected limit-only audit toolbar to separate filters from cap');
  if (!limitOnlyText.includes('No audit entries are visible with the current limit cap (Limit cap: latest 2). Increase the limit or clear audit controls to return to the full audit timeline.')) throw new Error('expected limit-only audit empty-state guidance, got ' + limitOnlyText);
  const clearControlsButton = collectNodes(limitOnlyRun.document.getElementById('toolbar'), (node) => node.tagName === 'BUTTON').find((node) => node.textContent === 'Clear audit controls');
  if (!clearControlsButton) throw new Error('expected limit-only audit page to offer clear audit controls');

  const unavailableRun = await renderWithFetch(() => Promise.resolve({
    ok: false,
    status: 503,
    text: () => Promise.resolve('audit logs unavailable'),
  }));
  await settle();
  await settle();
  const unavailableText = collectText(unavailableRun.document.getElementById('accounts')).join(' | ');
  if (!unavailableText.includes('Audit log is currently unavailable.')) throw new Error('expected audit unavailable state');
  if (!unavailableText.includes('audit log API unavailable (503): audit logs unavailable')) throw new Error('expected audit unavailable detail text');

  const errorRun = await renderWithFetch(() => Promise.resolve({
    ok: false,
    status: 500,
    text: () => Promise.resolve('boom'),
  }));
  await settle();
  await settle();
  const errorText = collectText(errorRun.document.getElementById('accounts')).join(' | ');
  if (!errorText.includes('Failed to load audit log: audit log request failed (500): boom')) throw new Error('expected audit fetch error state');
})();
`
		runNodeHarness(t, nodeProgram)
	})
}

func TestEmbeddedUIChangeDetailsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":             "payments-api-staging",
		"appRef":           "payments",
		"environmentRef":   "staging-apne2",
		"region":           "ap-northeast-2",
		"accountName":      "staging-core",
		"awsAccountId":     "210987654321",
		"targetType":       "ecs",
		"strategy":         "linear",
		"executionMode":    "ecsCodeDeploy",
		"sync":             map[string]any{"state": "OutOfSync", "reason": "traffic shift running"},
		"health":           map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v2"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1"},
		"runtimeSummary": map[string]any{
			"source":           "deployment-controller",
			"timestamp":        "2026-04-16T07:10:00Z",
			"targetType":       "ecs",
			"strategy":         "linear",
			"executionMode":    "ecsCodeDeploy",
			"historyCount":     2,
			"changes":          []string{"summary", "currentStep"},
			"changeDetails":    []string{"Summary changed to Shifted 10 percent of traffic", "Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear"},
			"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1-runtime"},
			"conditions":       []map[string]any{{"type": "Progressing", "status": true, "reason": "TrafficShiftLinear", "message": "runtime summary tracks the in-flight linear shift"}},
		},
		"history": []map[string]any{{
			"timestamp":          "2026-04-16T07:05:00Z",
			"summary":            "Created deployment group revision for v2",
			"source":             "deployment-controller",
			"targetType":         "ecs",
			"strategy":           "linear",
			"executionMode":      "ecsCodeDeploy",
			"changes":            []string{"summary", "currentStep"},
			"changeDetails":      []string{"Summary changed to Created deployment group revision for v2", "Current step changed from VerifyServiceStability to ShiftTrafficLinear"},
			"deployedArtifact":   map[string]any{"image": "ghcr.io/example/payments:v0"},
			"conditions":         []map[string]any{{"type": "Progressing", "status": true, "reason": "DeploymentStarted", "message": "CodeDeploy deployment created"}},
			"sync":               map[string]any{"state": "OutOfSync"},
			"health":             map[string]any{"overall": "Progressing", "summary": "warming"},
			"desiredGeneration":  12,
			"observedGeneration": 11,
			"rolloutPhase":       "Accepted",
			"currentStep":        "ShiftTrafficLinear",
		}},
		"conditions": []map[string]any{{"type": "Available", "status": true, "reason": "ServiceReady", "message": "Service is ready"}},
		"rollout": map[string]any{
			"phase":       "Accepted",
			"currentStep": "ShiftTrafficLinear",
			"execution": []map[string]any{{
				"name":        "payments-api-staging Deployment Group",
				"resource":    "codeDeployDeploymentGroup",
				"description": "Linear traffic shifting deployment group",
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal detail body: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.type = '';
    this.placeholder = '';
    this.autocomplete = '';
    this.selectionStart = null;
    this.selectionEnd = null;
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class InputElement extends Element { setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; } }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector(selector) { const dataMatch = selector.match(/^\[data-toolbar-field="(.+)"\]$/); if (!dataMatch) return null; const target = dataMatch[1]; const stack = Array.from(this.byID.values()); while (stack.length) { const node = stack.shift(); if (node?.dataset?.toolbarField === target) return node; stack.push(...(node.children || [])); } return null; }
}
const createDocument = () => {
  const document = new Document();
  document.byID.set('toolbar', document.createElement('section'));
  document.byID.set('summary', document.createElement('section'));
  document.byID.set('accounts', document.createElement('section'));
  return document;
};
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const resourceID = 'codeDeployDeploymentGroup::payments-api-staging Deployment Group::Linear traffic shifting deployment group';
async function render(search) {
  const document = createDocument();
  const window = { location: { pathname: '/', search, hash: '' } };
  const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
  const fetchCalls = [];
  const logsURL = '/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=codeDeployDeploymentGroup&resourceName=payments-api-staging+Deployment+Group';
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  };
  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  await settle();
  await settle();
  return { document, fetchCalls };
}
(async () => {
  const resourceRun = await render('?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=' + encodeURIComponent(resourceID));
  if (!resourceRun.fetchCalls.includes('/v1/overview/deployments/payments-api-staging')) throw new Error('expected resource route to load deployment detail');
  const resourceText = collectText(resourceRun.document.getElementById('accounts')).join(' | ');
  if (!resourceText.includes('Runtime change details')) throw new Error('expected resource route runtime change details label');
  if (!resourceText.includes('Summary changed to Shifted 10 percent of traffic; Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear')) throw new Error('expected resource route runtime change details value');
  if (!resourceText.includes('History change details')) throw new Error('expected resource route history change details label');
  if (!resourceText.includes('Summary changed to Created deployment group revision for v2; Current step changed from VerifyServiceStability to ShiftTrafficLinear')) throw new Error('expected resource route history change details value');

  const driftRun = await render('?page=drift&deployment=payments-api-staging');
  if (driftRun.fetchCalls.length !== 1 || driftRun.fetchCalls[0] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected drift route to load only deployment detail');
  const driftText = collectText(driftRun.document.getElementById('accounts')).join(' | ');
  if (!driftText.includes('summary')) throw new Error('expected drift route existing runtime change labels');
  if (!driftText.includes('Runtime change details')) throw new Error('expected drift route runtime change details label');
  if (!driftText.includes('Summary changed to Shifted 10 percent of traffic; Current step changed from CreateCodeDeployDeployment to ShiftTrafficLinear')) throw new Error('expected drift route runtime change details value');
})();
`

	runNodeHarness(t, nodeProgram)
}

func TestEmbeddedUIRolloutExecutionFallbackContractWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
class Element {
  constructor(tagName, ownerDocument) {
    this.tagName = String(tagName).toUpperCase();
    this.ownerDocument = ownerDocument;
    this.children = [];
    this.parentNode = null;
    this.className = '';
    this.textContent = '';
    this.dataset = {};
    this.listeners = {};
    this.attributes = {};
    this.hidden = false;
    this.value = '';
    this.disabled = false;
    this.classList = { add: () => {}, toggle: () => {} };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  scrollIntoView(options) { this.ownerDocument.lastScrolledTarget = { id: this.id || '', options }; }
}
class InputElement extends Element { setSelectionRange() {} }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); this.lastScrolledTarget = null; }
  createElement(tagName) { return String(tagName).toLowerCase() === 'input' ? new InputElement(tagName, this) : new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector() { return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '', hash: '' } };
const history = { replaceState() {} };
const fetch = (url) => {
  if (url === '/v1/overview/applications') return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
  throw new Error('unexpected fetch url: ' + url);
};
const exported = new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent + '; return { canonicalRolloutExecutionFallback };')(window, document, fetch, history, Element, InputElement);
const weighted = exported.canonicalRolloutExecutionFallback({
  name: 'api-prod',
  executionMode: 'asgWeightedTargetGroups',
  strategy: 'canary',
  userData: { type: 'file', path: 'userdata/api-prod.sh' },
  rolloutConfig: { capacity: { desired: 4, min: 2, max: 8 }, strategy: { warmupSeconds: 90 } },
});
const codeDeploy = exported.canonicalRolloutExecutionFallback({
  name: 'worker-prod',
  executionMode: 'ecsCodeDeploy',
  strategy: 'linear',
  rolloutConfig: { strategy: { minHealthyPercent: 80, warmupSeconds: 45 } },
});
const refresh = exported.canonicalRolloutExecutionFallback({
  name: 'api-prod',
  executionMode: 'asgInstanceRefresh',
  userData: { type: 'file', path: 'userdata/api-prod-refresh.sh' },
  rolloutConfig: { stacks: [{ rollingUpdateInstanceCount: 2 }] },
});
const weightedCandidate = weighted.find((item) => item.resource === 'candidateAutoScalingGroup');
const weightedTraffic = weighted.find((item) => item.resource === 'trafficShiftPlan');
const codeDeployService = codeDeploy.find((item) => item.resource === 'ecsService');
const refreshService = refresh.find((item) => item.resource === 'serviceAutoScalingGroup');
if (!weightedCandidate || weightedCandidate.description !== 'Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout') throw new Error('unexpected weighted fallback candidate description: ' + (weightedCandidate && weightedCandidate.description));
if (!weightedTraffic || weightedTraffic.description !== 'Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s') throw new Error('unexpected weighted fallback traffic description: ' + (weightedTraffic && weightedTraffic.description));
if (!codeDeployService || codeDeployService.description !== 'ECS service Tenet updates during CodeDeploy traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)') throw new Error('unexpected code deploy fallback service description: ' + (codeDeployService && codeDeployService.description));
if (!refreshService || refreshService.description !== 'Service Auto Scaling Group Tenet refreshes in place with rolling-update batch size 2') throw new Error('unexpected instance refresh fallback description: ' + (refreshService && refreshService.description));
`

	runNodeHarness(t, nodeProgram)
}
