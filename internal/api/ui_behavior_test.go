package api

import (
	"encoding/json"
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
		"name":           "payments-api-staging",
		"appRef":         "payments",
		"environmentRef": "staging-apne2",
		"region":         "ap-northeast-2",
		"accountName":    "staging-core",
		"awsAccountId":   "210987654321",
		"targetType":     "ecs",
		"strategy":       "linear",
		"executionMode":  "ecsCodeDeploy",
		"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
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
const window = { location: { pathname: '/', search: '?q=payments&sync=OutOfSync&account=210987654321', hash: '' } };
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
  if (!toolbarText.includes('Search applications')) throw new Error('expected applications page toolbar');
  if (!summaryText.includes('Application status overview')) throw new Error('expected applications page overview title');
  if (!summaryText.includes('Last update')) throw new Error('expected applications page last update column label');
  if (!summaryText.includes('2026-04-16T07:10:00Z')) throw new Error('expected applications page last update value');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected applications page row text');
  if (accountsText.includes('Overview JSON')) throw new Error('applications page should not expose deployment read-model quick links');
  if (accountsText.includes('Status JSON')) throw new Error('applications page should not expose status quick links');
  if (accountsText.includes('Rollout execution')) throw new Error('applications page should not render deep detail sections');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const accountLink = links.find((node) => node.href === '/?page=account&q=payments&sync=OutOfSync&account=210987654321&accountId=210987654321');
  if (!accountLink) throw new Error('expected applications page link to account detail route');
  if (accountLink.target !== '_self') throw new Error('expected applications page account link to stay in embedded UI flow');
  if (accountLink.rel) throw new Error('expected applications page account link to avoid new-tab rel attributes');
  const appLink = links.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging');
  if (!appLink) throw new Error('expected applications page link to application detail route');
  if (appLink.target !== '_self') throw new Error('expected applications page link to stay in embedded UI flow');
  if (appLink.rel) throw new Error('expected applications page link to avoid new-tab rel attributes');
})();
`

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
})();
`

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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

  const accountView = await render('?page=account&accountId=210987654321&q=payments&sync=OutOfSync&account=210987654321', (url) => {
    if (url === '/v1/overview/accounts/210987654321') return Promise.resolve({ ok: true, json: () => Promise.resolve(accountDetail) });
    throw new Error('unexpected account fetch url: ' + url);
  });
  const toolbarLinks = collectNodes(accountView.document.getElementById('toolbar'), (node) => node.tagName === 'A');
  if (!toolbarLinks.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.textContent === 'Back to applications' && node.target === '_self')) {
    throw new Error('expected account detail back link to preserve /console pathname');
  }
  const accountLinks = collectNodes(accountView.document.getElementById('accounts'), (node) => node.tagName === 'A');
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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
  if (!toolbarText.includes('Back to applications')) throw new Error('expected account page back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink) throw new Error('expected account page back link to preserve filters');
  if (!summaryText.includes('Account deployment overview')) throw new Error('expected account page summary title');
  if (!summaryText.includes('staging-core')) throw new Error('expected account page account name');
  if (!accountsText.includes('payments · staging-apne2')) throw new Error('expected grouped project/environment heading');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected account page deployment row');
  if (!accountsText.includes('Account JSON')) throw new Error('expected account page account json link');
  if (!accountsText.includes('Group JSON')) throw new Error('expected account page group json link');
  if (!accountsText.includes('Open application')) throw new Error('expected account page application drill-down links');
  if (accountsText.includes('Drift view')) throw new Error('expected account page to stop exposing drift workspace shortcut');
  if (accountsText.includes('Plan/apply')) throw new Error('expected account page to stop exposing plan workspace shortcut');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  if (!links.find((node) => node.href === '/v1/overview/accounts/210987654321' && node.textContent === 'Account JSON')) throw new Error('expected account json href');
  if (!links.find((node) => node.href === '/v1/overview/accounts/210987654321/projects/payments/environments/staging-apne2' && node.textContent === 'Group JSON')) throw new Error('expected group json href');
  if (!links.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.textContent === 'Open application' && node.target === '_self')) throw new Error('expected account page open application self link');
})();
`

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected application route back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink) throw new Error('expected application route self-navigation back link');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected application route summary title');
  if (!summaryText.includes('Application detail page: show only application-linked resources before the final resource-detail page.')) throw new Error('expected application route drill-down guidance');
  if (!accountsText.includes('Application resources')) throw new Error('expected application route resources body');
  if (!accountsText.includes('Drift view')) throw new Error('expected application route drift workspace entry');
  if (!accountsText.includes('Plan/apply')) throw new Error('expected application route plan workspace entry');
  if (!accountsText.includes('Workloads')) throw new Error('expected application route workloads group');
  if (!accountsText.includes('Traffic routing')) throw new Error('expected application route traffic routing group');
  if (!accountsText.includes('payments-api-staging-service')) throw new Error('expected application route synthesized workload resource');
  if (!accountsText.includes('payments-api-staging-codedeploy-group')) throw new Error('expected application route synthesized deployment-group resource');
  if (accountsText.includes('Create Plan')) throw new Error('application route should not render plan workspace actions');
  if (accountsText.includes('Field-level drift')) throw new Error('application route should not render drift workspace sections');
})();
`

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'unsupported_resource_type', entries: [] }) });
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
  if (accountsText.includes('No rollout execution resources are currently reported for this selection.')) throw new Error('expected fallback resource page not to render empty execution state');
})();
`

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
		"name":           "payments-api-staging",
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
  const driftLink = links.find((node) => node.textContent === 'Drift view');
  const planLink = links.find((node) => node.textContent === 'Plan/apply');
  if (!resourceLink) throw new Error('expected resource drill-down link');
  if (!driftLink || !driftLink.href.includes('/?page=drift&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected drift workspace href to include deployment and preserved filters');
  if (!planLink || !planLink.href.includes('/?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected plan workspace href to include deployment and preserved filters');
  if (driftLink.target !== '_self' || planLink.target !== '_self') throw new Error('expected workspace drill-down links to stay in embedded UI flow');
  if (!resourceLink.href.includes('/?page=resource&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging')) throw new Error('expected resource detail href to include deployment and preserved filters');
  if (resourceLink.target !== '_self') throw new Error('expected resource drill-down link to stay in embedded UI flow');
  if (resourceLink.rel) throw new Error('expected resource drill-down link to avoid new-tab rel attributes');
  const resourceIDFragment = new URLSearchParams({ resourceId: 'ecsService::payments-api-staging ECS Service::Primary ECS service for the staging environment' }).toString();
  if (!resourceLink.href.includes(resourceIDFragment)) throw new Error('expected stable unique resource identifier in drill-down href');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}

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

		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
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
		"deploymentName": "payments-api-staging",
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

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const detailBody = ` + string(detailJSON) + `;
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
  return { document, fetchCalls };
}
(async () => {
  let resolveEvents;
  const loadingView = await render((url) => {
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') {
      return new Promise((resolve) => { resolveEvents = resolve; });
    }
    throw new Error('unexpected fetch url: ' + url);
  });
  const loadingText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!loadingText.includes('Loading controller runtime events…')) throw new Error('expected loading state while deployment events request is pending');
  if (loadingView.fetchCalls.length !== 2 || loadingView.fetchCalls[0] !== '/v1/overview/applications/payments-api-staging' || loadingView.fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging/events') throw new Error('expected application detail fetch followed by deployment events fetch');
  resolveEvents({ ok: true, json: () => Promise.resolve(eventsBody) });
  await settle();
  await settle();
  const readyText = collectText(loadingView.document.getElementById('accounts')).join(' | ');
  if (!readyText.includes('Controller runtime events')) throw new Error('expected controller runtime events section heading');
  if (!readyText.includes('Controller-derived runtime events from GET /v1/overview/deployments/{deploymentName}/events. Newest entries first. This is not audit/auth history.')) throw new Error('expected application page to distinguish controller runtime events from audit/auth history');
  if (!readyText.includes('2 events · newest controller event first')) throw new Error('expected event ordering guidance');
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

  const emptyView = await render((url) => {
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: true, json: () => Promise.resolve({ deploymentName: 'payments-api-staging', events: [] }) });
    throw new Error('unexpected fetch url: ' + url);
  });
  const emptyText = collectText(emptyView.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No controller runtime events have been persisted yet.')) throw new Error('expected empty deployment events state');

  const unavailableView = await render((url) => {
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: false, status: 503, text: () => Promise.resolve('overview deployment events unavailable') });
    throw new Error('unexpected fetch url: ' + url);
  });
  const unavailableText = collectText(unavailableView.document.getElementById('accounts')).join(' | ');
  if (!unavailableText.includes('Controller runtime events are currently unavailable.')) throw new Error('expected unavailable deployment events state');
  if (!unavailableText.includes('deployment events unavailable (503): overview deployment events unavailable')) throw new Error('expected unavailable deployment events detail');

  const errorView = await render((url) => {
    if (url === '/v1/overview/applications/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === '/v1/overview/deployments/payments-api-staging/events') return Promise.resolve({ ok: false, status: 500, text: () => Promise.resolve('boom') });
    throw new Error('unexpected fetch url: ' + url);
  });
  const errorText = collectText(errorView.document.getElementById('accounts')).join(' | ');
  if (!errorText.includes('Failed to load controller runtime events: deployment events request failed (500): boom')) throw new Error('expected generic deployment events fetch error state');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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
  const fallbackLinks = collectNodes(fallbackRun.document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResourceID = 'trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s';
  const resourceHref = '/?' + new URLSearchParams({
    page: 'resource',
    deployment: 'payments-api-prod',
    resourceType: 'trafficShiftPlan',
    resourceId: expectedResourceID,
  }).toString();
  const fallbackLink = fallbackLinks.find((node) => node.href === resourceHref);
  if (!fallbackLink) throw new Error('expected fallback application page to synthesize canonical trafficShiftPlan resource link');
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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
  const backLink = toolbarLinks.find((node) => node.href === '/?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-staging' && node.target === '_self');
  if (!backLink) throw new Error('expected resource page self-navigation back link');
  if (backLink.rel) throw new Error('expected resource page back link to avoid new-tab rel attributes');
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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
  resolveLogs({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', entries: [] }) });
  await settle();

  const unavailableRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false, source: '', unavailableReason: 'cross_account_not_configured', entries: [] }) });
    throw new Error('unexpected unavailable fetch url: ' + url);
  });
  const unavailableText = collectText(unavailableRun.document.getElementById('accounts')).join(' | ');
  if (!unavailableText.includes('Recent logs are unavailable for this resource.')) throw new Error('expected resource page unavailable recent logs state');
  if (!unavailableText.includes('Cross-account access is not configured for this deployment.')) throw new Error('expected unavailable recent logs state to surface friendly provider reason');

  const emptyRun = await renderWithFetch((url) => {
    if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', entries: [] }) });
    throw new Error('unexpected empty fetch url: ' + url);
  });
  const emptyText = collectText(emptyRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No recent logs are available for this resource.')) throw new Error('expected resource page empty recent logs state');

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
    if (url === logsURL) return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: true, source: 'cloudwatch', entries: [{ timestamp: '2026-04-16T07:11:00Z', stream: 'codedeploy/bluegreen/1', message: '<img src=x onerror=1> rollback check passed' }] }) });
    throw new Error('unexpected populated fetch url: ' + url);
  });
  await settle();
  const populatedText = collectText(populatedRun.document.getElementById('accounts')).join(' | ');
  if (!populatedText.includes('Logs source')) throw new Error('expected populated recent logs section to show source');
  if (!populatedText.includes('cloudwatch')) throw new Error('expected populated recent logs section to show source value');
  if (!populatedText.includes('Log stream · codedeploy/bluegreen/1')) throw new Error('expected populated recent logs section to show stream');
  if (!populatedText.includes('Log timestamp · 2026-04-16T07:11:00Z')) throw new Error('expected populated recent logs section to show timestamp');
  if (!populatedText.includes('<img src=x onerror=1> rollback check passed')) throw new Error('expected populated recent logs section to render raw log message text safely');
  const images = collectNodes(populatedRun.document.getElementById('accounts'), (node) => node.tagName === 'IMG');
  if (images.length !== 0) throw new Error('expected log messages to remain text content rather than parsed html');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestDeploymentWorkspaceDriftViewWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
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

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const driftDetail = ` + string(driftDetailJSON) + `;
const emptyDetail = ` + string(emptyDetailJSON) + `;
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
  const run = await render(driftDetail);
  if (run.fetchCalls.length !== 1 || run.fetchCalls[0] !== '/v1/overview/deployments/api-prod') throw new Error('expected drift workspace to load only deployment detail, got ' + run.fetchCalls.join(', '));
  const text = collectText(run.document.getElementById('accounts')).join(' | ');
  const toolbarText = collectText(run.document.getElementById('toolbar')).join(' | ');
  if (!text.includes('Drift viewer')) throw new Error('expected drift workspace title');
  if (!toolbarText.includes('Back to application')) throw new Error('expected workspace back link');
  if (!toolbarText.includes('api-prod')) throw new Error('expected workspace active deployment title');
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
  const allRows = collectNodes(run.document.getElementById('accounts'), (node) => node.tagName === 'TR').slice(1).map((node) => collectText(node).join(' | '));
  if (allRows.some((row) => row.includes('BoundaryRegressionGuard') && row.includes('False drift detection should stay out of drift rows'))) throw new Error('expected DriftDetected=false boundary to stay out of drift rows');
  if (allRows.some((row) => row.includes('BoundaryRegressionGuard') && row.includes('Healthy true should stay out of drift rows'))) throw new Error('expected Healthy=true boundary to stay out of drift rows');
  if (!text.includes('ghcr.io/example/payments:v9')) throw new Error('expected desired artifact value in drift rows');
  if (!text.includes('ghcr.io/example/payments:v7')) throw new Error('expected actual artifact value in drift rows');
  if (!text.includes('high')) throw new Error('expected high severity indicator');
  if (!text.includes('medium')) throw new Error('expected medium severity indicator');
  if (!text.includes('low')) throw new Error('expected low severity indicator');
  if (text.includes('No field-level drift is currently derived from the deployment read model.')) throw new Error('expected non-empty drift data to avoid empty state');
  if (!text.includes("<script>alert('xss')</script> ALB production listener still targets the legacy task set")) throw new Error('expected drift condition message to render as text');

  const severityField = run.document.querySelector('[data-toolbar-field="Severity"]');
  if (!severityField) throw new Error('expected drift severity filter control');
  severityField.value = 'high';
  severityField.dispatch('change', { target: severityField });
  await settle();
  const highOnlyText = collectText(run.document.getElementById('accounts')).join(' | ');
  if (!highOnlyText.includes('artifact')) throw new Error('expected high severity filter to retain artifact row');
  if (highOnlyText.includes('condition.Healthy')) throw new Error('expected high severity filter to remove low-severity condition row');

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

  const emptyRun = await render(emptyDetail);
  const emptyText = collectText(emptyRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('Field-level drift')) throw new Error('expected empty drift workspace section heading');
  if (!emptyText.includes('No field-level drift is currently derived from the deployment read model.')) throw new Error('expected empty drift workspace message');
  if (emptyText.includes('condition.')) throw new Error('expected empty drift workspace to omit condition rows');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
`

	runNode := func(t *testing.T, nodeProgram string) {
		t.Helper()
		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
	}

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
  if (!text.includes('Plan and apply')) throw new Error('expected plan workspace title');
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
  if (!createPlan || !applyPlan) throw new Error('expected plan workspace action buttons before create');
  if (createPlan.disabled || !applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan disabled before create');
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
  const workspaceLink = links.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123' && node.textContent === 'Open bookmarked workspace');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected bookmarked workspace self link after create');
  const planJSONLink = links.find((node) => node.href === '/v1/plans/plan_ready_123' && node.textContent === 'Plan JSON');
  if (!planJSONLink || planJSONLink.target !== '_blank' || planJSONLink.rel !== 'noreferrer noopener') throw new Error('expected plan json read-model link after create');
  buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  if (!createPlan || !applyPlan) throw new Error('expected plan workspace buttons after create');
  if (createPlan.disabled || applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan enabled after ready preview load');
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
  if (!text.includes('Apply request details')) throw new Error('expected apply request details section');
  if (!text.includes('apply execution is disabled')) throw new Error('expected apply failure reason');
  if (!text.includes('Tenet persisted this immutable apply request in read-only mode and did not contact AWS.')) throw new Error('expected immutable apply detail');
  if (!text.includes('Live execution is intentionally disabled by default for this backend slice.')) throw new Error('expected disabled-by-default apply detail');
  const applyLinks = collectNodes(root, (node) => node.tagName === 'A');
  const updatedWorkspaceLink = applyLinks.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked workspace');
  if (!updatedWorkspaceLink || updatedWorkspaceLink.target !== '_self' || updatedWorkspaceLink.rel) throw new Error('expected bookmarked workspace self link to include applyId after apply create');
  const bookmarkedApplyLink = applyLinks.find((node) => node.href === '/?page=plan&deployment=api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open bookmarked apply');
  if (!bookmarkedApplyLink || bookmarkedApplyLink.target !== '_self' || bookmarkedApplyLink.rel) throw new Error('expected bookmarked apply self link after apply create');
  const applyJSONLink = applyLinks.find((node) => node.href === '/v1/applies/apply_failed_123' && node.textContent === 'Apply JSON');
  if (!applyJSONLink || applyJSONLink.target !== '_blank' || applyJSONLink.rel !== 'noreferrer noopener') throw new Error('expected apply json read-model link after apply create');
})();
`
		runNode(t, nodeProgram)
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
  if (!createPlan || !applyPlan) throw new Error('expected plan workspace action buttons on bookmarked plan');
  if (createPlan.disabled || !applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan disabled on bookmarked failed plan');
})();
`
		runNode(t, nodeProgram)
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
  if (!createPlan || !applyPlan) throw new Error('expected plan workspace action buttons on bookmarked apply');
  if (createPlan.disabled || applyPlan.disabled) throw new Error('expected Create Plan enabled and Apply Plan enabled on bookmarked apply backed by ready preview');
})();
`
		runNode(t, nodeProgram)
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
		runNode(t, nodeProgram)
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
					"reason":    "actor information is unavailable because the immutable plan preview and apply request stores do not persist operator identity",
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
					"available": false,
					"reason":    "actor information is unavailable because the immutable plan preview and apply request stores do not persist operator identity",
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
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
`

	runNode := func(t *testing.T, nodeProgram string) {
		t.Helper()
		cmd := exec.Command("node", "-e", nodeProgram)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
		}
	}

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
const window = { location: { pathname: '/console', search: '?page=audit&q=payments&sync=OutOfSync&account=210987654321&limit=2', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/console'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/audit-logs?limit=2') return Promise.resolve({ ok: true, json: () => Promise.resolve(auditLogs) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
(async () => {
  await settle();
  await settle();
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/audit-logs?limit=2') throw new Error('expected only audit log fetch on audit page');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected audit page back link');
  if (!summaryText.includes('Operator audit log')) throw new Error('expected audit page summary title');
  if (!accountsText.includes('Audit timeline')) throw new Error('expected audit page body title');
  if (!accountsText.includes('apply.requested')) throw new Error('expected newest audit entry text');
  if (!accountsText.includes('plan.created')) throw new Error('expected older audit entry text');
  if (!accountsText.includes('2 entries')) throw new Error('expected audit entry count copy');
  if (!accountsText.includes('limit 2')) throw new Error('expected audit limit copy');
  if (!accountsText.includes('Actor detail')) throw new Error('expected explicit actor label');
  if (!accountsText.includes('Unavailable')) throw new Error('expected unavailable actor state');
  if (!accountsText.includes('apply execution is disabled')) throw new Error('expected apply metadata in audit row');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/console?page=applications&q=payments&sync=OutOfSync&account=210987654321' && node.target === '_self');
  if (!backLink || backLink.rel) throw new Error('expected audit page back link to preserve filters');
  const rows = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'DIV' && node.className === 'row');
  if (rows.length < 2) throw new Error('expected audit page rows');
  const rowTexts = rows.map((node) => collectText(node).join(' | '));
  if (!rowTexts[0].includes('apply.requested')) throw new Error('expected newest audit entry first, got ' + rowTexts.join(' || '));
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const applicationLink = links.find((node) => node.href === '/console?page=application&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod' && node.textContent === 'Open application');
  if (!applicationLink || applicationLink.target !== '_self' || applicationLink.rel) throw new Error('expected audit application self link');
  const workspaceLink = links.find((node) => node.href === '/console?page=plan&q=payments&sync=OutOfSync&account=210987654321&deployment=payments-api-prod&planId=plan_ready_123&applyId=apply_failed_123' && node.textContent === 'Open plan/apply');
  if (!workspaceLink || workspaceLink.target !== '_self' || workspaceLink.rel) throw new Error('expected audit plan/apply self link');
})();
`
		runNode(t, nodeProgram)
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
  pending.resolve({ ok: true, json: () => Promise.resolve({ entries: [] }) });
  await settle();
  await settle();
  const emptyText = collectText(loadingRun.document.getElementById('accounts')).join(' | ');
  if (!emptyText.includes('No audit activity has been persisted yet.')) throw new Error('expected audit empty state after resolve');

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
		runNode(t, nodeProgram)
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
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
}
class InputElement extends Element { setSelectionRange() {} }
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
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

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
