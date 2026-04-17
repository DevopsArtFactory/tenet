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

	t.Run("application detail route reuses deployment detail fetch", func(t *testing.T) {
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
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-staging') {
    throw new Error('expected application route to load only deployment detail, got ' + fetchCalls.join(', '));
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
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected ecs fallback page to load only deployment detail');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-staging-codedeploy-app', 'codeDeployApplication', 'CodeDeploy application coordinating ECS traffic shifting'],
    ['payments-api-staging-codedeploy-group', 'codeDeployDeploymentGroup', 'CodeDeploy deployment group Tenet targets for ECS rollout'],
    ['payments-api-staging-prod-listener', 'productionListener', 'Production listener CodeDeploy shifts during ECS blue/green rollout'],
    ['payments-api-staging-test-listener', 'testListener', 'Test listener CodeDeploy uses to validate the replacement ECS task set before promotion'],
    ['payments-api-staging-blue', 'blueTaskSetTargetGroup', 'Blue task set target group CodeDeploy treats as the live ECS traffic destination'],
    ['payments-api-staging-green', 'greenTaskSetTargetGroup', 'Green task set target group CodeDeploy warms before shifting ECS traffic'],
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
  if (url === '/v1/overview/deployments/payments-api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-prod') throw new Error('expected asg fallback page to load only deployment detail');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-prod-candidate', 'candidateAutoScalingGroup', 'Candidate Auto Scaling Group Tenet will create for weighted target group rollout'],
    ['payments-api-prod-active', 'activeTargetGroup', 'Currently active target group receiving production traffic'],
    ['payments-api-prod-candidate', 'candidateTargetGroup', 'Candidate target group Tenet will register and warm before traffic shifting'],
    ['payments-api-prod-prod-listener', 'productionListenerRule', 'Production listener rule operators inspect while Tenet updates weighted forwarding during rollout'],
    ['payments-api-prod-weighted-routing', 'trafficShiftPlan', 'Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group'],
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
  if (url === '/v1/overview/deployments/payments-api-refresh') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-refresh') throw new Error('expected instance refresh fallback page to load only deployment detail');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResources = [
    ['payments-api-refresh-service', 'serviceAutoScalingGroup', 'Service Auto Scaling Group Tenet refreshes in place'],
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
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
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
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
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
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected application page to load only deployment detail');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected application page back link');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected application summary title');
  if (!accountsText.includes('Application resources')) throw new Error('expected application resources heading');
  if (!accountsText.includes('Workloads')) throw new Error('expected workloads group');
  if (!accountsText.includes('Traffic routing')) throw new Error('expected traffic routing group');
  if (!accountsText.includes('Config and access')) throw new Error('expected config and access group');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected workload resource');
  if (!accountsText.includes('prod listener')) throw new Error('expected traffic routing resource');
  if (!accountsText.includes('bootstrap source')) throw new Error('expected config/access resource');
  if (accountsText.includes('Runtime history')) throw new Error('application page should not expose deep runtime history yet');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const resourceLink = links.find((node) => node.textContent === 'Open resource');
  if (!resourceLink) throw new Error('expected resource drill-down link');
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
  if (url === '/v1/overview/deployments/payments-api-staging') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected nameless application page to load only deployment detail');
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
				{"name": "payments-api-prod-candidate", "resource": "candidateAutoScalingGroup", "description": "Candidate Auto Scaling Group Tenet will create for weighted target group rollout"},
				{"name": "payments-api-prod-active", "resource": "activeTargetGroup", "description": "Currently active target group receiving production traffic"},
				{"name": "payments-api-prod-candidate", "resource": "candidateTargetGroup", "description": "Candidate target group Tenet will register and warm before traffic shifting"},
				{"name": "payments-api-prod-prod-listener", "resource": "productionListenerRule", "description": "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout"},
				{"name": "payments-api-prod-weighted-routing", "resource": "trafficShiftPlan", "description": "Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group"},
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
    if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
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
  if (fallbackRun.fetchCalls.length !== 1 || fallbackRun.fetchCalls[0] !== '/v1/overview/deployments/payments-api-prod') throw new Error('expected fallback application page to load only deployment detail');
  const fallbackLinks = collectNodes(fallbackRun.document.getElementById('accounts'), (node) => node.tagName === 'A' && node.textContent === 'Open resource');
  const expectedResourceID = 'trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group';
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
  if (!summaryText.includes('Weighted routing plan Tenet follows to move production traffic from the active target group to the candidate target group')) throw new Error('expected canonical resource page to resolve trafficShiftPlan resource description');
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

func TestDeploymentWorkspaceDriftViewWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "acct-a",
		"awsAccountId": "111111111111",
		"deployments": []map[string]any{{
			"name":           "api-prod",
			"appRef":         "payments",
			"environmentRef": "prod",
			"region":         "us-west-2",
			"sync":           map[string]any{"state": "Drifted", "reason": "Service target group differs from desired"},
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":           "api-prod",
		"appRef":         "payments",
		"environmentRef": "prod",
		"region":         "us-west-2",
		"accountName":    "acct-a",
		"awsAccountId":   "111111111111",
		"sync":           map[string]any{"state": "Drifted", "reason": "Service target group differs from desired"},
		"health":         map[string]any{"overall": "Healthy", "summary": "traffic steady"},
		"runtimeSummary": map[string]any{
			"source":           "deployment-controller",
			"timestamp":        "2026-04-16T06:50:00Z",
			"targetType":       "ec2",
			"strategy":         "canary",
			"executionMode":    "asgWeightedTargetGroups",
			"historyCount":     3,
			"changes":          []string{"listener rule updated", "target group weight drift"},
			"deployedArtifact": map[string]any{"binary": "s3://artifacts.example/payments/api-prod-r17"},
			"conditions":       []map[string]any{{"type": "DriftDetected", "status": true, "reason": "TargetGroupMismatch", "message": "Runtime summary mirrors the drift condition"}},
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
			"message": "ALB production listener still targets the legacy task set",
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
const window = { location: { pathname: '/', search: '?page=drift&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/api-prod') {
    throw new Error('expected drift workspace to load only deployment detail, got ' + fetchCalls.join(', '));
  }
  const text = collectText(document.getElementById('accounts')).join(' | ');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  if (!text.includes('Drift viewer')) throw new Error('expected drift workspace title');
  if (!toolbarText.includes('Back to overview')) throw new Error('expected workspace back link');
  if (!text.includes('Latest runtime signals')) throw new Error('expected drift workspace runtime signals section');
  if (!text.includes('Runtime source')) throw new Error('expected drift workspace runtime source label');
  if (!text.includes('deployment-controller')) throw new Error('expected drift workspace runtime source value');
  if (!text.includes('Runtime context')) throw new Error('expected drift workspace runtime context label');
  if (!text.includes('Target ec2 · Strategy canary · Execution asgWeightedTargetGroups')) throw new Error('expected drift workspace runtime context value');
  if (!text.includes('Runtime timestamp')) throw new Error('expected drift workspace runtime timestamp label');
  if (!text.includes('2026-04-16T06:50:00Z')) throw new Error('expected drift workspace runtime timestamp value');
  if (!text.includes('Runtime history')) throw new Error('expected drift workspace runtime history label');
  if (!/Runtime history \| 3(?: \||$)/.test(text)) throw new Error('expected drift workspace runtime history count');
  if (!text.includes('Runtime artifact')) throw new Error('expected drift workspace runtime artifact label');
  if (!text.includes('s3://artifacts.example/payments/api-prod-r17')) throw new Error('expected drift workspace runtime artifact value');
  if (!text.includes('Runtime conditions')) throw new Error('expected drift workspace runtime conditions label');
  if (!text.includes('DriftDetected = True · TargetGroupMismatch')) throw new Error('expected drift workspace runtime condition summary');
  if (!text.includes('listener rule updated')) throw new Error('expected drift workspace runtime change entry');
  if (!text.includes('target group weight drift')) throw new Error('expected drift workspace second runtime change entry');
  if (!text.includes('Rollout execution')) throw new Error('expected drift workspace rollout execution section');
  if (!text.includes('prod-router')) throw new Error('expected drift workspace rollout execution name');
  if (!text.includes('aws_lb_listener_rule.payments_prod')) throw new Error('expected drift workspace rollout execution resource');
  if (!text.includes('Routes production traffic to the stable target group')) throw new Error('expected drift workspace rollout execution description');
  if (!text.includes('Field-level drift')) throw new Error('expected drift workspace section heading');
  if (!text.includes('Field-level drift is not yet available from the current read model.')) throw new Error('expected drift workspace empty-state guidance');
  if (!text.includes('Status conditions')) throw new Error('expected drift workspace to render status conditions section');
  if (!text.includes('TargetGroupMismatch')) throw new Error('expected drift workspace to surface condition reason');
  if (!text.includes('ALB production listener still targets the legacy task set')) throw new Error('expected drift workspace to surface condition message');
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

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":         "acct-a",
		"awsAccountId": "111111111111",
		"deployments": []map[string]any{{
			"name":           "api-prod",
			"appRef":         "payments",
			"environmentRef": "prod",
			"region":         "us-west-2",
			"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
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
const window = { location: { pathname: '/', search: '?page=plan&deployment=api-prod', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  if (url === '/v1/overview/deployments/api-prod') return Promise.resolve({ ok: true, json: () => Promise.resolve(detailBody) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
(async () => {
  await settle();
  await settle();
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/deployments/api-prod') {
    throw new Error('expected plan workspace to load only deployment detail, got ' + fetchCalls.join(', '));
  }
  const root = document.getElementById('accounts');
  const text = collectText(root).join(' | ');
  if (!text.includes('Plan and apply')) throw new Error('expected plan workspace title');
  if (!text.includes('Plan/apply controls are not wired to mutation APIs in this read-only slice yet.')) throw new Error('expected plan workspace placeholder guidance');
  if (!text.includes('Plan operations')) throw new Error('expected plan workspace section heading');
  if (!text.includes('Latest runtime signals')) throw new Error('expected plan workspace runtime signals section');
  if (!text.includes('Runtime source')) throw new Error('expected plan workspace runtime source label');
  if (!text.includes('runtime-agent')) throw new Error('expected plan workspace runtime source value');
  if (!text.includes('Runtime context')) throw new Error('expected plan workspace runtime context label');
  if (!text.includes('Target ecs · Strategy linear · Execution ecsCodeDeploy')) throw new Error('expected plan workspace runtime context value');
  if (!text.includes('Runtime timestamp')) throw new Error('expected plan workspace runtime timestamp label');
  if (!text.includes('2026-04-16T07:10:00Z')) throw new Error('expected plan workspace runtime timestamp value');
  if (!text.includes('Runtime history')) throw new Error('expected plan workspace runtime history label');
  if (!/Runtime history \| 5(?: \||$)/.test(text)) throw new Error('expected plan workspace runtime history count');
  if (!text.includes('Runtime artifact')) throw new Error('expected plan workspace runtime artifact label');
  if (!text.includes('ghcr.io/example/payments:v1-runtime')) throw new Error('expected plan workspace runtime artifact value');
  if (!text.includes('Runtime conditions')) throw new Error('expected plan workspace runtime conditions label');
  if (!text.includes('Progressing = True · TrafficShiftCanary')) throw new Error('expected plan workspace runtime condition summary');
  if (!text.includes('No runtime change signals are currently reported.')) throw new Error('expected plan workspace runtime change empty state');
  if (!text.includes('Rollout execution')) throw new Error('expected plan workspace rollout execution section');
  if (!text.includes('No rollout execution resources are currently reported.')) throw new Error('expected plan workspace rollout execution empty state');
  if (!text.includes('CreateCodeDeployDeployment')) throw new Error('expected plan workspace to surface rollout plan steps');
  if (!text.includes('ShiftTrafficCanary')) throw new Error('expected plan workspace to surface current rollout step');
  const buttons = collectNodes(root, (node) => node.tagName === 'BUTTON');
  const createPlan = buttons.find((node) => node.textContent === 'Create Plan');
  const applyPlan = buttons.find((node) => node.textContent === 'Apply Plan');
  if (!createPlan || !applyPlan) throw new Error('expected plan workspace action buttons');
  if (!createPlan.disabled || !applyPlan.disabled) throw new Error('expected read-only plan workspace buttons to remain disabled');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
