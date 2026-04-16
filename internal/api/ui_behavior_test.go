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

	accountsJSON, err := json.Marshal([]map[string]any{
		{
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
		"sync":           map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":         map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy"},
	})
	if err != nil {
		t.Fatalf("marshal application detail: %v", err)
	}

	t.Run("applications overview entry page", func(t *testing.T) {
		nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
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
const window = { location: { pathname: '/', search: '', hash: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = (url) => {
  fetchCalls.push(String(url));
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
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
  if (fetchCalls.length !== 1 || fetchCalls[0] !== '/v1/overview/accounts') throw new Error('expected only overview accounts fetch on applications page');
  if (!toolbarText.includes('Search applications')) throw new Error('expected applications page toolbar');
  if (!summaryText.includes('Application status overview')) throw new Error('expected applications page overview title');
  if (!summaryText.includes('Last update')) throw new Error('expected applications page last update column label');
  if (!summaryText.includes('2026-04-16T07:10:00Z')) throw new Error('expected applications page last update value');
  if (!accountsText.includes('payments-api-staging')) throw new Error('expected applications page row text');
  if (accountsText.includes('Overview JSON')) throw new Error('applications page should not expose deployment read-model quick links');
  if (accountsText.includes('Status JSON')) throw new Error('applications page should not expose status quick links');
  if (accountsText.includes('Rollout execution')) throw new Error('applications page should not render deep detail sections');
  const links = collectNodes(document.getElementById('accounts'), (node) => node.tagName === 'A');
  const appLink = links.find((node) => node.href === '/?page=application&deployment=payments-api-staging');
  if (!appLink) throw new Error('expected applications page link to application detail route');
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
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging', hash: '' } };
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
  if (fetchCalls[0] !== '/v1/overview/accounts' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging') {
    throw new Error('expected application route to load overview then deployment detail, got ' + fetchCalls.join(', '));
  }
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Back to applications')) throw new Error('expected application route back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/?page=applications' && node.target === '_self');
  if (!backLink) throw new Error('expected application route self-navigation back link');
  if (!summaryText.includes('payments-api-staging')) throw new Error('expected application route summary title');
  if (!summaryText.includes('Application detail page: show only application-linked resources before the final resource-detail page.')) throw new Error('expected application route drill-down guidance');
  if (!accountsText.includes('Application resources')) throw new Error('expected application route resources body');
  if (!accountsText.includes('Workloads')) throw new Error('expected application route workloads group');
  if (!accountsText.includes('Traffic routing')) throw new Error('expected application route traffic routing group');
  if (!accountsText.includes('payments-api-staging ECS Service')) throw new Error('expected application route synthesized workload resource');
  if (!accountsText.includes('payments-api-staging Deployment Group')) throw new Error('expected application route synthesized deployment-group resource');
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
		"name":            "payments-api-staging",
		"accountName":     "staging-core",
		"awsAccountId":    "210987654321",
		"region":          "ap-northeast-2",
		"appRef":          "payments",
		"environmentRef":  "staging-apne2",
		"targetType":      "ecs",
		"strategy":        "linear",
		"executionMode":   "ecsCodeDeploy",
		"sync":            map[string]any{"state": "OutOfSync", "reason": "rollout in progress"},
		"health":          map[string]any{"overall": "Progressing", "summary": "1/2 tasks healthy while rollout continues"},
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
const window = { location: { pathname: '/', search: '?page=application&deployment=payments-api-staging', hash: '' } };
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
  if (fetchCalls[0] !== '/v1/overview/accounts' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected application page to load overview then detail');
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
  if (!resourceLink.href.includes('/?page=resource&deployment=payments-api-staging')) throw new Error('expected resource detail href to include deployment');
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
			"historyCount":       2,
			"desiredGeneration":  12,
			"observedGeneration": 11,
			"changes":            []string{"deployment group switched to linear-10-percent-every-minute"},
		},
		"history": []map[string]any{{
			"timestamp":          "2026-04-16T07:05:00Z",
			"summary":            "Created deployment group revision for v2",
			"source":             "controller",
			"desiredGeneration":  12,
			"observedGeneration": 10,
			"rolloutPhase":       "Progressing",
			"currentStep":        "CreateCodeDeployDeployment",
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
const window = { location: { pathname: '/', search: '?page=resource&deployment=payments-api-staging&resourceType=codeDeployDeploymentGroup&resourceId=codeDeployDeploymentGroup', hash: '' } };
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
  if (fetchCalls[0] !== '/v1/overview/accounts' || fetchCalls[1] !== '/v1/overview/deployments/payments-api-staging') throw new Error('expected resource page to load overview then detail');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  const summaryText = collectText(document.getElementById('summary')).join(' | ');
  const accountsText = collectText(document.getElementById('accounts')).join(' | ');
  if (!toolbarText.includes('Back to application')) throw new Error('expected resource page back link');
  const toolbarLinks = collectNodes(document.getElementById('toolbar'), (node) => node.tagName === 'A');
  const backLink = toolbarLinks.find((node) => node.href === '/?page=application&deployment=payments-api-staging' && node.target === '_self');
  if (!backLink) throw new Error('expected resource page self-navigation back link');
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
  if (!accountsText.includes('Runtime history')) throw new Error('expected resource page runtime history section');
  if (!accountsText.includes('Created deployment group revision for v2')) throw new Error('expected resource page history detail');
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
			"source":       "deployment-controller",
			"timestamp":    "2026-04-16T06:50:00Z",
			"historyCount": 3,
			"changes":      []string{"listener rule updated", "target group weight drift"},
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
  if (fetchCalls[0] !== '/v1/overview/accounts' || fetchCalls[1] !== '/v1/overview/deployments/api-prod') {
    throw new Error('expected drift workspace to load overview then deployment detail, got ' + fetchCalls.join(', '));
  }
  const text = collectText(document.getElementById('accounts')).join(' | ');
  const toolbarText = collectText(document.getElementById('toolbar')).join(' | ');
  if (!text.includes('Drift viewer')) throw new Error('expected drift workspace title');
  if (!toolbarText.includes('Back to overview')) throw new Error('expected workspace back link');
  if (!text.includes('Latest runtime signals')) throw new Error('expected drift workspace runtime signals section');
  if (!text.includes('Runtime source')) throw new Error('expected drift workspace runtime source label');
  if (!text.includes('deployment-controller')) throw new Error('expected drift workspace runtime source value');
  if (!text.includes('Runtime timestamp')) throw new Error('expected drift workspace runtime timestamp label');
  if (!text.includes('2026-04-16T06:50:00Z')) throw new Error('expected drift workspace runtime timestamp value');
  if (!text.includes('Runtime history')) throw new Error('expected drift workspace runtime history label');
  if (!/Runtime history \| 3(?: \||$)/.test(text)) throw new Error('expected drift workspace runtime history count');
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
			"sync":           map[string]any{"state": "Reconciling", "reason": "rollout in progress"},
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
		"sync":             map[string]any{"state": "Reconciling", "reason": "rollout in progress"},
		"health":           map[string]any{"overall": "Healthy", "summary": "traffic shifting"},
		"desiredArtifact":  map[string]any{"image": "ghcr.io/example/payments:v2"},
		"deployedArtifact": map[string]any{"image": "ghcr.io/example/payments:v1"},
		"runtimeSummary": map[string]any{
			"source":       "runtime-agent",
			"timestamp":    "2026-04-16T07:10:00Z",
			"historyCount": 5,
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
  if (fetchCalls[0] !== '/v1/overview/accounts' || fetchCalls[1] !== '/v1/overview/deployments/api-prod') {
    throw new Error('expected plan workspace to load overview then deployment detail, got ' + fetchCalls.join(', '));
  }
  const root = document.getElementById('accounts');
  const text = collectText(root).join(' | ');
  if (!text.includes('Plan and apply')) throw new Error('expected plan workspace title');
  if (!text.includes('Plan/apply controls are not wired to mutation APIs in this read-only slice yet.')) throw new Error('expected plan workspace placeholder guidance');
  if (!text.includes('Plan operations')) throw new Error('expected plan workspace section heading');
  if (!text.includes('Latest runtime signals')) throw new Error('expected plan workspace runtime signals section');
  if (!text.includes('Runtime source')) throw new Error('expected plan workspace runtime source label');
  if (!text.includes('runtime-agent')) throw new Error('expected plan workspace runtime source value');
  if (!text.includes('Runtime timestamp')) throw new Error('expected plan workspace runtime timestamp label');
  if (!text.includes('2026-04-16T07:10:00Z')) throw new Error('expected plan workspace runtime timestamp value');
  if (!text.includes('Runtime history')) throw new Error('expected plan workspace runtime history label');
  if (!/Runtime history \| 5(?: \||$)/.test(text)) throw new Error('expected plan workspace runtime history count');
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
