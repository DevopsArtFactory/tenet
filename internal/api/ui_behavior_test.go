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
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "acct-a",
			"awsAccountId": "111111111111",
			"deployments": []map[string]any{{
				"name":           "api",
				"appRef":         "payments",
				"environmentRef": "prod",
				"region":         "us-west-2",
				"sync":           map[string]any{"state": "Reconciling", "reason": "Traffic shift running"},
				"runtimeSummary": map[string]any{"timestamp": "2026-04-16T06:50:00Z", "summary": "Traffic shift is progressing", "sync": map[string]any{"state": "Reconciling", "reason": "Traffic shift running"}, "health": map[string]any{"overall": "Healthy", "summary": "steady"}, "rolloutPhase": "Progressing", "currentStep": "ShiftTrafficCanary", "historyCount": 2},
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const root = document.getElementById('accounts');
  const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
  const beforeText = collectText(root).join(' | ');
  if (!beforeText.includes('Latest runtime')) throw new Error('expected overview card to render latest runtime label');
  if (!beforeText.includes('Traffic shift is progressing')) throw new Error('expected overview card to render runtime summary text');
  if (!beforeText.includes('2026-04-16T06:50:00Z')) throw new Error('expected overview card to render runtime summary timestamp');
  const searchInput = document.querySelector('[data-toolbar-field="Search deployments"]');
  if (!searchInput) throw new Error('expected search input');
  searchInput.value = 'Traffic shift running';
  searchInput.dispatch('input', { target: searchInput });
  await settle();
  if (window.location.search !== '?q=Traffic+shift+running') throw new Error('expected runtime-summary filter query to sync to URL');
  if (document.getElementById('accounts').children.length !== 1) throw new Error('expected runtime-summary text to match additively in search');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestUIProjectEnvironmentGroupingWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":           "acct-a",
			"awsAccountId":   "111111111111",
			"allowedRegions": []string{"us-west-2"},
			"deployments": []map[string]any{
				{"name": "api-prod", "appRef": "payments", "environmentRef": "prod", "region": "us-west-2", "sync": map[string]any{"state": "Synced"}},
				{"name": "api-staging", "appRef": "payments", "environmentRef": "staging", "region": "us-west-2", "sync": map[string]any{"state": "Reconciling"}},
				{"name": "worker-prod", "appRef": "jobs", "environmentRef": "prod", "region": "us-west-2", "sync": map[string]any{"state": "Paused"}},
				{"name": "worker-staging", "appRef": "jobs", "environmentRef": "staging", "region": "us-west-2", "sync": map[string]any{"state": "Error"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const root = document.getElementById('accounts');
  const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
  const collectNodes = (node, predicate) => [predicate(node) ? [node] : [], ...(node.children || []).map((child) => collectNodes(child, predicate))].flat();
  const hasClass = (node, className) => typeof node.className === 'string' && new RegExp('(^|\\s)' + className + '(\\s|$)').test(node.className);
  const text = collectText(root).join(' | ');
  if (!text.includes('2 projects · 2 environments')) throw new Error('expected account topology summary to show unique project/environment counts');
  if (!text.includes('payments · prod')) throw new Error('expected grouped project/environment title for payments prod');
  if (!text.includes('Project payments · Environment prod · Region us-west-2')) throw new Error('expected grouped project/environment metadata for payments prod');
  if (!text.includes('payments · staging')) throw new Error('expected grouped project/environment title for payments staging');
  if (!text.includes('jobs · prod')) throw new Error('expected grouped project/environment title for jobs prod');
  if (!text.includes('jobs · staging')) throw new Error('expected grouped project/environment title for jobs staging');
  const groupSections = collectNodes(root, (node) => hasClass(node, 'deployment-group'));
  if (groupSections.length !== 4) throw new Error('expected one project/environment section per derived group');

  const groupSummaryByTitle = new Map(groupSections.map((section) => {
    const titleNode = collectNodes(section, (node) => hasClass(node, 'deployment-group-title'))[0];
    const groupHead = collectNodes(section, (node) => hasClass(node, 'deployment-group-head'))[0];
    if (!titleNode || !groupHead) throw new Error('expected each project/environment section to render a titled group header');
    const deploymentCountPill = collectNodes(groupHead, (node) => hasClass(node, 'group-pill')).map((node) => node.textContent).filter(Boolean);
    const statusPills = collectNodes(groupHead, (node) => hasClass(node, 'pill') && /(synced|progress|paused|error)/.test(node.className)).map((node) => node.textContent).filter(Boolean);
    return [titleNode.textContent, { deploymentCountPill, statusPills }];
  }));

  const expectGroupPills = (title, expectedStatusPill) => {
    const summary = groupSummaryByTitle.get(title);
    if (!summary) throw new Error('expected grouped section for ' + title);
    if (summary.deploymentCountPill.length !== 1 || summary.deploymentCountPill[0] !== '1 deployment') throw new Error('expected ' + title + ' to render exactly one deployment-count pill in the group header');
    if (summary.statusPills.length !== 1 || summary.statusPills[0] !== expectedStatusPill) throw new Error('expected ' + title + ' header pills to be exactly ' + expectedStatusPill + ', got ' + (summary.statusPills.join(', ') || 'none'));
  };

  expectGroupPills('payments · prod', '1 synced');
  expectGroupPills('payments · staging', '1 progressing');
  expectGroupPills('jobs · prod', '1 paused');
  expectGroupPills('jobs · staging', '1 error');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestUIAccountTopologyUsesAuthoritativeCountsWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":             "acct-a",
			"awsAccountId":     "111111111111",
			"allowedRegions":   []string{"us-west-2"},
			"projectCount":     7,
			"environmentCount": 11,
			"deployments": []map[string]any{
				{"name": "api-prod", "appRef": "payments", "environmentRef": "prod", "region": "us-west-2", "sync": map[string]any{"state": "Synced"}},
				{"name": "api-staging", "appRef": "payments", "environmentRef": "staging", "region": "us-west-2", "sync": map[string]any{"state": "Reconciling"}},
				{"name": "worker-prod", "appRef": "jobs", "environmentRef": "prod", "region": "us-west-2", "sync": map[string]any{"state": "Paused"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState(_state, _title, url) { const [pathname, query = ''] = String(url).split('?'); window.location.pathname = pathname || '/'; window.location.search = query ? '?' + query : ''; } };
const fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const root = document.getElementById('accounts');
  const collectText = (node) => [node.textContent, ...(node.children || []).flatMap((child) => collectText(child))].filter(Boolean);
  const text = collectText(root).join(' | ');
  if (!text.includes('7 projects · 11 environments')) throw new Error('expected account topology summary to prefer authoritative account counts');
  if (text.includes('2 projects · 2 environments')) throw new Error('expected account topology summary to avoid fallback-derived counts when authoritative counts exist');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIRendersAccountJSONQuickLinkInAccountHeaderWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountID := `111111111111/<script>alert(1)</script>`
	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "acct-a",
			"awsAccountId": accountID,
			"deployments": []map[string]any{{
				"name":           "api",
				"appRef":         "payments",
				"environmentRef": "prod",
				"region":         "us-west-2",
				"targetType":     "ecs",
				"strategy":       "canary",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "Reconciling"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
    this.href = '';
    this.target = '';
    this.rel = '';
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
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector() { return null; }
}

function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}

function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}

const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState() {} };
const fetch = (url) => {
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, Element);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const link = findFirst(document, (node) => node.tagName === 'A' && node.textContent === 'Account JSON');
  if (!link) throw new Error('expected account quick link in account header');
  const expectedHref = '/v1/overview/accounts/' + encodeURIComponent(sampleAccounts[0].awsAccountId);
  if (link.href !== expectedHref) throw new Error('expected account quick link href to encode awsAccountId');
  if (link.target !== '_blank') throw new Error('expected account quick link target');
  if (link.rel !== 'noreferrer noopener') throw new Error('expected account quick link rel');
  const header = link.parentNode && link.parentNode.parentNode;
  if (!header || header.className !== 'account-head') throw new Error('expected account quick link container to live in account header');
  if (findFirst(document, (node) => node.tagName === 'BUTTON' && walk([node]).includes(link))) throw new Error('expected account quick link not to be nested inside deployment toggle buttons');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIRendersGroupQuickLinksInGroupHeadersWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{{
		"name":           "acct-a",
		"awsAccountId":   "111111111111",
		"allowedRegions": []string{"us-west-2"},
		"deployments": []map[string]any{{
			"name":           "api-prod",
			"appRef":         "payments/api?<script>alert(1)</script>",
			"environmentRef": "prod&blue",
			"region":         "us-west-2",
			"targetType":     "ecs",
			"strategy":       "canary",
			"executionMode":  "ecsCodeDeploy",
			"sync":           map[string]any{"state": "Reconciling"},
		}},
	}})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
    this.href = '';
    this.target = '';
    this.rel = '';
    this.type = '';
    this.classList = { add: (...names) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); names.forEach((name) => current.add(name)); this.className = Array.from(current).join(' '); }, toggle: (name, force) => { const current = new Set(this.className.split(/\s+/).filter(Boolean)); const shouldAdd = force === undefined ? !current.has(name) : Boolean(force); if (shouldAdd) current.add(name); else current.delete(name); this.className = Array.from(current).join(' '); } };
  }
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector() { return null; }
}
const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState() {} };
const fetch = (url) => {
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  throw new Error('unexpected fetch url: ' + url);
};
function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}
function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, Element);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const link = findFirst(document, (node) => node.tagName === 'A' && node.textContent === 'Group JSON');
  if (!link) throw new Error('expected group quick link in group header');
  const expectedHref = '/v1/overview/accounts/' + encodeURIComponent(sampleAccounts[0].awsAccountId) + '/projects/' + encodeURIComponent(sampleAccounts[0].deployments[0].appRef) + '/environments/' + encodeURIComponent(sampleAccounts[0].deployments[0].environmentRef);
  if (link.href !== expectedHref) throw new Error('expected group quick link href to encode account/project/environment values');
  if (link.target !== '_blank') throw new Error('expected group quick link target');
  if (link.rel !== 'noreferrer noopener') throw new Error('expected group quick link rel');
  const head = link.parentNode && link.parentNode.parentNode;
  if (!head || head.className !== 'deployment-group-head') throw new Error('expected group quick link to live inside the group header');
  if (findFirst(document, (node) => node.tagName === 'BUTTON' && walk([node]).includes(link))) throw new Error('expected group quick link not to be nested inside deployment toggle buttons');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIRendersDeploymentQuickLinksBesideExpandButtonWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	deploymentName := `api/prod?<script>alert(1)</script>`
	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "acct-a",
			"awsAccountId": "111111111111",
			"deployments": []map[string]any{{
				"name":           deploymentName,
				"appRef":         "payments",
				"environmentRef": "prod",
				"region":         "us-west-2",
				"targetType":     "ecs",
				"strategy":       "canary",
				"executionMode":  "ecsCodeDeploy",
				"sync":           map[string]any{"state": "Reconciling"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
    this.href = '';
    this.target = '';
    this.rel = '';
    this.type = '';
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
  append(...nodes) { nodes.forEach((node) => { if (node == null) return; if (typeof node === 'string') { const textNode = new Element('#text', this.ownerDocument); textNode.textContent = node; node = textNode; } node.parentNode = this; this.children.push(node); if (this.tagName === 'SELECT' && node.tagName === 'OPTION') { if (node.selected || this.value === '') this.value = node.value; } }); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  addEventListener(type, handler) { this.listeners[type] = handler; }
  dispatch(type, event = {}) { if (!event.target) event.target = this; const handler = this.listeners[type]; if (handler) handler(event); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  focus() { this.ownerDocument.activeElement = this; }
}
class Document {
  constructor() { this.activeElement = null; this.byID = new Map(); }
  createElement(tagName) { return new Element(tagName, this); }
  getElementById(id) { return this.byID.get(id) || null; }
  querySelector() { return null; }
}

function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}

function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}

const document = new Document();
document.byID.set('toolbar', document.createElement('section'));
document.byID.set('summary', document.createElement('section'));
document.byID.set('accounts', document.createElement('section'));
const window = { location: { pathname: '/', search: '' } };
const history = { replaceState() {} };
const fetch = (url) => {
  if (url === '/v1/overview/accounts') return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  throw new Error('unexpected fetch url: ' + url);
};
new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, Element);
async function settle() { await Promise.resolve(); await new Promise((resolve) => setTimeout(resolve, 0)); }
(async () => {
  await settle();
  const rowLinks = findFirst(document, (node) => node.tagName === 'DIV' && node.className === 'row-links row-links-shell');
  if (!rowLinks) throw new Error('expected deployment quick-link container');
  const overviewLink = (rowLinks.children || []).find((node) => node.tagName === 'A' && node.textContent === 'Overview JSON');
  const statusLink = (rowLinks.children || []).find((node) => node.tagName === 'A' && node.textContent === 'Status JSON');
  if (!overviewLink) throw new Error('expected Overview JSON quick link');
  if (!statusLink) throw new Error('expected Status JSON quick link');
  const expectedOverviewHref = '/v1/overview/deployments/' + encodeURIComponent(sampleAccounts[0].deployments[0].name);
  const expectedStatusHref = '/v1/status/deployments/' + encodeURIComponent(sampleAccounts[0].deployments[0].name);
  if (overviewLink.href !== expectedOverviewHref) throw new Error('expected Overview JSON quick link href to encode deployment name');
  if (statusLink.href !== expectedStatusHref) throw new Error('expected Status JSON quick link href to encode deployment name');
  for (const link of [overviewLink, statusLink]) {
    if (link.target !== '_blank') throw new Error('expected deployment quick link target');
    if (link.rel !== 'noreferrer noopener') throw new Error('expected deployment quick link rel');
    if (findFirst(document, (node) => node.tagName === 'BUTTON' && walk([node]).includes(link))) throw new Error('expected deployment quick link not to be nested inside expand button');
  }
  const row = rowLinks.parentNode;
  if (!row || row.className !== 'row') throw new Error('expected deployment quick-link container to live directly under the deployment row');
  const [toggle, links, detailPanel] = row.children || [];
  if (toggle?.tagName !== 'BUTTON') throw new Error('expected expand button to remain first child in row');
  if (links !== rowLinks) throw new Error('expected deployment quick links to remain outside the expand button as a sibling container');
  if (detailPanel?.className !== 'detail-panel') throw new Error('expected deployment detail panel to remain separate from quick links');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIFilterBehaviorWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":           "acct-a",
			"awsAccountId":   "111111111111",
			"allowedRegions": []string{"us-west-2"},
			"deployments": []map[string]any{
				{
					"name":             "api",
					"appRef":           "payments",
					"environmentRef":   "prod",
					"region":           "us-west-2",
					"targetType":       "ecs",
					"strategy":         "canary",
					"executionMode":    "ecsCodeDeploy",
					"desiredArtifact":  map[string]any{"image": "ghcr.io/example/api:v2"},
					"deployedArtifact": map[string]any{"image": "ghcr.io/example/api:v1"},
					"sync":             map[string]any{"state": "Reconciling", "reason": "Traffic shift running"},
					"runtimeSummary":   map[string]any{"timestamp": "2026-04-16T06:50:00Z", "summary": "Traffic shift is progressing", "sync": map[string]any{"state": "Reconciling", "reason": "Traffic shift running"}, "health": map[string]any{"overall": "Healthy", "summary": "steady"}, "desiredGeneration": 7, "observedGeneration": 6, "rolloutPhase": "Progressing", "currentStep": "ShiftTrafficCanary", "historyCount": 2},
					"rollout":          map[string]any{"phase": "Progressing", "currentStep": "ShiftTrafficCanary", "summary": "10 percent shifted", "steps": []map[string]any{{"kind": "ShiftTrafficCanary"}, {"kind": "BakeTraffic"}}},
					"health":           map[string]any{"overall": "Healthy", "summary": "steady"},
				},
			},
		},
		{
			"name":           "acct-b",
			"awsAccountId":   "222222222222",
			"allowedRegions": []string{"us-east-1"},
			"deployments": []map[string]any{
				{
					"name":               "worker",
					"appRef":             "jobs",
					"environmentRef":     "prod",
					"region":             "us-east-1",
					"targetType":         "ec2",
					"strategy":           "linear",
					"executionMode":      "asgWeightedTargetGroups",
					"desiredArtifact":    map[string]any{"binary": "worker-v3"},
					"deployedArtifact":   map[string]any{"binary": "worker-v2"},
					"sync":               map[string]any{"state": "Error", "reason": "Alarm triggered"},
					"runtimeSummary":     map[string]any{"summary": "Rollout blocked after alarm", "sync": map[string]any{"state": "Error", "reason": "Alarm triggered"}, "health": map[string]any{"overall": "Degraded", "summary": "alarm open"}, "desiredGeneration": 9, "observedGeneration": 4, "rolloutPhase": "Blocked", "currentStep": "AbortTrafficShift", "historyCount": 1},
					"rollout":            map[string]any{"phase": "Blocked", "currentStep": "AbortTrafficShift", "summary": "stopped after alarm", "steps": []map[string]any{{"kind": "AbortTrafficShift"}}},
					"health":             map[string]any{"overall": "Degraded", "summary": "alarm open"},
					"observedGeneration": 2,
				},
			},
		},
		{
			"name":           "acct-empty",
			"awsAccountId":   "333333333333",
			"allowedRegions": []string{"eu-west-1"},
			"deployments":    []map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

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
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) {
          current.add(name);
        } else {
          current.delete(name);
        }
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
  replaceChildren(...nodes) {
    this.children = [];
    this.append(...nodes);
  }
  addEventListener(type, handler) {
    this.listeners[type] = handler;
  }
  dispatch(type, event = {}) {
    if (!event.target) event.target = this;
    const handler = this.listeners[type];
    if (handler) handler(event);
  }
  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }
  getAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null;
  }
  focus() {
    this.ownerDocument.activeElement = this;
  }
}
class InputElement extends Element {
  setSelectionRange(start, end) {
    this.selectionStart = start;
    this.selectionEnd = end;
  }
}
class Document {
  constructor() {
    this.activeElement = null;
    this.byID = new Map();
  }
  createElement(tagName) {
    if (String(tagName).toLowerCase() === 'input') {
      return new InputElement(tagName, this);
    }
    return new Element(tagName, this);
  }
  getElementById(id) {
    return this.byID.get(id) || null;
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

function createEnvironment(search) {
  const document = new Document();
  const toolbar = document.createElement('section');
  const summary = document.createElement('section');
  const accounts = document.createElement('section');
  document.byID.set('toolbar', toolbar);
  document.byID.set('summary', summary);
  document.byID.set('accounts', accounts);

  const window = { location: { pathname: '/', search } };
  const history = {
    calls: [],
    replaceState(_state, _title, url) {
      this.calls.push(url);
      const [pathname, query = ''] = String(url).split('?');
      window.location.pathname = pathname || '/';
      window.location.search = query ? '?' + query : '';
    },
  };

  const fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
  const appFactory = new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent + '; return { getCurrentViewState: () => currentViewState };');
  const api = appFactory(window, document, fetch, history, Element, InputElement);
  return { document, history, window, api };
}

async function settle() {
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function findButtonByText(document, text) {
  const stack = Array.from(document.byID.values());
  while (stack.length) {
    const node = stack.shift();
    if (node && node.tagName === 'BUTTON' && node.textContent === text) return node;
    stack.push(...(node?.children || []));
  }
  return null;
}

function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}

function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}

function collectText(node) {
  return [node?.textContent, ...((node?.children || []).flatMap((child) => collectText(child)))].filter(Boolean);
}

(async () => {
  const defaultEnv = createEnvironment('');
  await settle();
  const defaultAccounts = defaultEnv.document.getElementById('accounts');
  if (defaultAccounts.children.length !== 3) {
    throw new Error('expected default overview to render accounts with and without deployments');
  }
  const defaultText = collectText(defaultAccounts).join(' | ');
  if (!defaultText.includes('Latest runtime')) throw new Error('expected overview cards to render a latest runtime metric');
  if (!defaultText.includes('Traffic shift is progressing')) throw new Error('expected overview cards to render runtime summary text');
  if (!defaultText.includes('2026-04-16T06:50:00Z')) throw new Error('expected overview cards to render runtime summary timestamp');
  if (!defaultText.includes('Runtime generations')) throw new Error('expected overview cards to render runtime generation label');
  if (!defaultText.includes('desired 7 · observed 6')) throw new Error('expected overview cards to render runtime generation values');
  const apiToggle = findFirst(defaultEnv.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
  if (!apiToggle) throw new Error('expected api deployment expand toggle in overview');
  const apiOverviewText = collectText(apiToggle).join(' | ');
  if (!apiOverviewText.includes('Observed generation | —')) throw new Error('expected missing overview observed generation to render as em dash');
  if (apiOverviewText.includes('Observed generation | 0')) throw new Error('expected missing overview observed generation to avoid zero fallback');

  const filteredEnv = createEnvironment('?sync=Unknown&account=missing&q=%20api%20');
  await settle();
  const normalizedState = filteredEnv.api.getCurrentViewState();
  if (normalizedState.query !== 'api') throw new Error('expected query to be trimmed during normalization');
  if (normalizedState.syncState !== 'all') throw new Error('expected invalid sync state to normalize to all');
  if (normalizedState.account !== 'all') throw new Error('expected invalid account to normalize to all');

  const searchInput = filteredEnv.document.querySelector('[data-toolbar-field="Search deployments"]');
  const syncSelect = filteredEnv.document.querySelector('[data-toolbar-field="Sync state"]');
  const accountSelect = filteredEnv.document.querySelector('[data-toolbar-field="Account"]');
  const clearButton = findButtonByText(filteredEnv.document, 'Clear filters');
  if (!searchInput || searchInput.value !== 'api') throw new Error('expected normalized query to be reflected in the toolbar input');
  if (!syncSelect || syncSelect.value !== 'all') throw new Error('expected normalized sync filter to be reflected in the toolbar select');
  if (!accountSelect || accountSelect.value !== 'all') throw new Error('expected normalized account filter to be reflected in the toolbar select');
  if (!clearButton || clearButton.disabled !== false) throw new Error('expected clear-filters button to be enabled for active filters');
	  if (filteredEnv.window.location.search !== '?q=api') throw new Error('expected normalized URL state to persist the trimmed free-text query');

	  searchInput.value = 'Traffic shift running';
	  searchInput.dispatch('input', { target: searchInput });
	  await settle();
	  if (filteredEnv.window.location.search !== '?q=Traffic+shift+running') throw new Error('expected runtime-summary search text to update the URL query string');
	  if (filteredEnv.document.getElementById('accounts').children.length !== 1) throw new Error('expected runtime-summary search to filter down to one account');

	  searchInput.focus();
	  searchInput.selectionStart = 2;
  searchInput.selectionEnd = 2;
  searchInput.value = 'BakeTraffic';
  searchInput.dispatch('input', { target: searchInput });
  await settle();

  const rerenderedInput = filteredEnv.document.querySelector('[data-toolbar-field="Search deployments"]');
  if (filteredEnv.document.activeElement !== rerenderedInput) throw new Error('expected search input focus to survive rerender');
  if (rerenderedInput.selectionStart !== 2 || rerenderedInput.selectionEnd !== 2) throw new Error('expected caret position to survive rerender');
  if (filteredEnv.window.location.search !== '?q=BakeTraffic') throw new Error('expected free-text search to update the URL query string');
  if (filteredEnv.document.getElementById('accounts').children.length !== 1) throw new Error('expected planned-step search to filter down to one account');

  syncSelect.value = 'Error';
  syncSelect.dispatch('change', { target: syncSelect });
  await settle();
  if (filteredEnv.window.location.search !== '?q=BakeTraffic&sync=Error') throw new Error('expected sync-state changes to update the URL query string');
  const emptyState = filteredEnv.document.getElementById('accounts').children[0];
  if (!emptyState || emptyState.textContent !== 'No deployments match the current filters.') throw new Error('expected combined filters to render the explicit empty state');

  accountSelect.value = '222222222222';
  accountSelect.dispatch('change', { target: accountSelect });
  await settle();
  if (filteredEnv.window.location.search !== '?q=BakeTraffic&sync=Error&account=222222222222') throw new Error('expected account-filter changes to update the URL query string');

  clearButton.dispatch('click');
  await settle();
  if (filteredEnv.window.location.search !== '') throw new Error('expected clear filters to reset the URL state');
  if (filteredEnv.document.getElementById('accounts').children.length !== 3) throw new Error('expected clear filters to restore the full overview');
  if (filteredEnv.document.querySelector('[data-toolbar-field="Search deployments"]').value !== '') throw new Error('expected clear filters to reset the search input');
  if (filteredEnv.document.querySelector('[data-toolbar-field="Sync state"]').value !== 'all') throw new Error('expected clear filters to reset the sync select');
  if (filteredEnv.document.querySelector('[data-toolbar-field="Account"]').value !== 'all') throw new Error('expected clear filters to reset the account select');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIBehaviorWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "acct-a",
			"awsAccountId": "111111111111",
			"deployments": []map[string]any{
				{
					"name":           "api",
					"appRef":         "payments",
					"environmentRef": "prod",
					"region":         "us-west-2",
					"targetType":     "ecs",
					"strategy":       "canary",
					"executionMode":  "ecsCodeDeploy",
					"sync":           map[string]any{"state": "Reconciling"},
					"runtimeSummary": map[string]any{"summary": "Traffic shift is progressing", "timestamp": "2026-04-16T06:50:00Z", "source": "controller-runtime", "desiredGeneration": 7, "observedGeneration": 6, "changes": []string{"traffic-shift <literal>", "capacity-ready"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":         "api",
		"accountName":  "acct-a",
		"awsAccountId": "111111111111",
		"region":       "us-west-2",
		"runtimeSummary": map[string]any{
			"summary":            "Traffic shift is progressing",
			"timestamp":          "2026-04-16T06:50:00Z",
			"source":             "controller-runtime",
			"desiredGeneration":  7,
			"observedGeneration": 6,
			"changes":            []string{"traffic-shift <literal>", "capacity-ready"},
		},
		"sync": map[string]any{"reason": "Traffic shift running"},
		"rollout": map[string]any{"steps": []map[string]any{
			{"kind": "CreateCodeDeployDeployment", "detail": "Creates deployment group <unsafe>", "status": "completed"},
			{"kind": "ShiftTrafficCanary", "detail": "Shifts 10 percent of traffic <script>alert(1)</script>", "status": "current"},
			{"kind": "BakeTraffic", "status": "pending"},
		}},
		"history": []map[string]any{
			{
				"timestamp":          "2026-04-16T06:50:00Z",
				"summary":            "Shifted 10 percent of traffic <script>alert(1)</script>",
				"source":             "runtime-cache<unsafe>",
				"sync":               map[string]any{"state": "Reconciling"},
				"health":             map[string]any{"overall": "Healthy", "summary": "steady"},
				"desiredGeneration":  7,
				"observedGeneration": 6,
				"changes":            []string{"traffic-shift <literal>", "alarm-cleared<script>"},
				"rolloutPhase":       "Progressing <literal>",
				"currentStep":        "ShiftTrafficCanary",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample detail: %v", err)
	}

	detailEmptyJSON, err := json.Marshal(map[string]any{
		"name":           "api",
		"accountName":    "acct-a",
		"awsAccountId":   "111111111111",
		"region":         "us-west-2",
		"runtimeSummary": map[string]any{},
		"rollout": map[string]any{"steps": []map[string]any{
			{"kind": "CreateCodeDeployDeployment", "current": true},
			{"kind": "ShiftTrafficCanary"},
		}},
		"history": []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal empty detail: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailPayload = ` + string(detailJSON) + `;
const detailEmptyPayload = ` + string(detailEmptyJSON) + `;

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
    this.href = '';
    this.target = '';
    this.rel = '';
    this.classList = {
      add: (...names) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        names.forEach((name) => current.add(name));
        this.className = Array.from(current).join(' ');
      },
      toggle: (name, force) => {
        const current = new Set(this.className.split(/\s+/).filter(Boolean));
        const shouldAdd = force === undefined ? !current.has(name) : Boolean(force);
        if (shouldAdd) {
          current.add(name);
        } else {
          current.delete(name);
        }
        this.className = Array.from(current).join(' ');
      },
      contains: (name) => new Set(this.className.split(/\s+/).filter(Boolean)).has(name),
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
  replaceChildren(...nodes) {
    this.children = [];
    this.append(...nodes);
  }
  addEventListener(type, handler) {
    this.listeners[type] = handler;
  }
  dispatch(type, event = {}) {
    if (!event.target) event.target = this;
    const handler = this.listeners[type];
    if (handler) handler(event);
  }
  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }
  getAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null;
  }
  focus() {
    this.ownerDocument.activeElement = this;
  }
}
class InputElement extends Element {
  setSelectionRange(start, end) {
    this.selectionStart = start;
    this.selectionEnd = end;
  }
}
class Document {
  constructor() {
    this.activeElement = null;
    this.byID = new Map();
  }
  createElement(tagName) {
    if (String(tagName).toLowerCase() === 'input') {
      return new InputElement(tagName, this);
    }
    return new Element(tagName, this);
  }
  getElementById(id) {
    return this.byID.get(id) || null;
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

function createEnvironment(detailResponse) {
  const document = new Document();
  const toolbar = document.createElement('section');
  const summary = document.createElement('section');
  const accounts = document.createElement('section');
  document.byID.set('toolbar', toolbar);
  document.byID.set('summary', summary);
  document.byID.set('accounts', accounts);

  const window = { location: { pathname: '/', search: '' } };
  const history = { replaceState() {} };
  const fetchCalls = [];
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/accounts') {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
    }
    if (url === '/v1/overview/deployments/api') {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(detailResponse) });
    }
    throw new Error('unexpected fetch url: ' + url);
  };

  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  return { document, fetchCalls };
}

async function settle() {
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}

function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}

function findByText(document, text) {
  return findFirst(document, (node) => node.textContent === text);
}

function findDetailPanelForToggle(toggle) {
  if (!toggle) return null;
  const row = toggle.parentNode;
  if (!row) return null;
  return (row.children || []).find((child) => child.className === 'detail-panel') || null;
}

function collectText(node) {
  if (!node) return [];
  return [node.textContent, ...node.children.flatMap((child) => collectText(child))].filter(Boolean);
}

(async () => {
  const env = createEnvironment(detailPayload);
  await settle();
  if (env.fetchCalls.length !== 1 || env.fetchCalls[0] !== '/v1/overview/accounts') {
    throw new Error('expected initial render to fetch only overview accounts');
  }

	  const toggle = findFirst(env.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
	  if (!toggle) throw new Error('expected deployment expand toggle');

	  const overviewText = collectText(env.document.getElementById('accounts')).join(' | ');
	  if (!overviewText.includes('Traffic shift is progressing · controller-runtime')) throw new Error('expected overview runtime summary text to include runtime source');
	  if (!overviewText.includes('Latest changes')) throw new Error('expected overview card to render latest change label heading');
	  if (!overviewText.includes('traffic-shift <literal>, capacity-ready')) throw new Error('expected overview card to render latest change labels literally');
	  const searchInput = env.document.querySelector('[data-toolbar-field="Search deployments"]');
	  if (!searchInput) throw new Error('expected search input in detail-history env');
	  searchInput.value = 'traffic-shift <literal>';
	  searchInput.dispatch('input', { target: searchInput });
	  await settle();
	  if (env.document.getElementById('accounts').children.length !== 1) throw new Error('expected runtime change labels to participate in additive overview search');
	  searchInput.value = 'controller-runtime';
	  searchInput.dispatch('input', { target: searchInput });
	  await settle();
	  if (env.document.getElementById('accounts').children.length !== 1) throw new Error('expected runtime source to participate in additive overview search');
	  searchInput.value = '';
	  searchInput.dispatch('input', { target: searchInput });
	  await settle();

	  const refreshedToggle = findFirst(env.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
	  if (!refreshedToggle) throw new Error('expected deployment expand toggle after clearing runtime-source search');
	  refreshedToggle.dispatch('click');
	  await settle();
  if (env.fetchCalls.length !== 2 || env.fetchCalls[1] !== '/v1/overview/deployments/api') {
    throw new Error('expected expand to fetch deployment detail exactly once');
  }
	  const historyHeading = findByText(env.document, 'Runtime history');
	  if (!historyHeading) throw new Error('expected runtime history section heading');

	  const detailText = collectText(findFirst(env.document, (node) => node.className === 'detail-grid')).join(' | ');
	  if (!detailText.includes('Runtime generations')) throw new Error('expected detail grid to render runtime generation label');
	  if (!detailText.includes('desired 7 · observed 6')) throw new Error('expected detail grid to render runtime generation values');
	  if (!detailText.includes('Runtime source | controller-runtime')) throw new Error('expected detail grid to render runtime source metric');
	  if (!detailText.includes('Runtime changes | traffic-shift <literal>, capacity-ready')) throw new Error('expected detail grid to render runtime change labels literally');

	  const historySection = historyHeading.parentNode;
	  const historyText = collectText(historySection).join(' | ');
	  if (!historyText.includes('2026-04-16T06:50:00Z')) throw new Error('expected history timestamp to render');
	  if (!historyText.includes('Shifted 10 percent of traffic <script>alert(1)</script>')) throw new Error('expected history summary text to render literally');
	  if (!historyText.includes('History sync · Reconciling')) throw new Error('expected history sync state to render');
	  if (!historyText.includes('History health · Healthy · steady')) throw new Error('expected history health to render');
	  if (!historyText.includes('History generations · desired 7 · observed 6')) throw new Error('expected history generations to render');
	  if (!historyText.includes('History changes · traffic-shift <literal>, alarm-cleared<script>')) throw new Error('expected history change labels to render literally');
	  if (!historyText.includes('History source · runtime-cache<unsafe>')) throw new Error('expected history source to render literally');
	  if (!historyText.includes('History rollout phase · Progressing <literal>')) throw new Error('expected history rollout phase to render literally');
	  if (!historyText.includes('History rollout step · ShiftTrafficCanary')) throw new Error('expected history rollout step to render');

	  const completedStep = findFirst(env.document, (node) => node.className === 'detail-step-meta detail-step-meta-completed');
	  const currentStep = findFirst(env.document, (node) => node.className === 'detail-step-meta detail-step-meta-current');
	  const pendingStep = findFirst(env.document, (node) => node.className === 'detail-step-meta detail-step-meta-pending');
	  if (!completedStep || completedStep.textContent !== 'Completed rollout step') throw new Error('expected completed rollout-step status label/class');
	  if (!currentStep || currentStep.textContent !== 'Current rollout step') throw new Error('expected current rollout-step status label/class');
	  if (!pendingStep || pendingStep.textContent !== 'Pending rollout step') throw new Error('expected pending rollout-step status label/class');

	  const rolloutHeading = findByText(env.document, 'Rollout steps');
	  if (!rolloutHeading) throw new Error('expected rollout steps section heading');
	  const rolloutText = collectText(rolloutHeading.parentNode).join(' | ');
	  if (!rolloutText.includes('Step detail · Creates deployment group <unsafe>')) throw new Error('expected first rollout-step detail to render literally');
	  if (!rolloutText.includes('Step detail · Shifts 10 percent of traffic <script>alert(1)</script>')) throw new Error('expected current rollout-step detail to render literally');

	  const emptyEnv = createEnvironment(detailEmptyPayload);
	  await settle();
	  const emptyToggle = findFirst(emptyEnv.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
	  if (!emptyToggle) throw new Error('expected deployment expand toggle in empty-history env');
  emptyToggle.dispatch('click');
	  await settle();
  const emptyCopy = findByText(emptyEnv.document, 'No recent runtime history is available yet.');
  if (!emptyCopy) throw new Error('expected explicit empty runtime history copy');
  const emptyDetailPanel = findDetailPanelForToggle(emptyToggle);
  if (!emptyDetailPanel) throw new Error('expected detail panel for empty-history env');
  const emptyDetailText = collectText(emptyDetailPanel).join(' | ');
  if (!emptyDetailText.includes('Runtime generations | desired — · observed —')) throw new Error('expected missing runtime generations to render as em dash');
  if (!emptyDetailText.includes('Rollout generations | desired — · observed —')) throw new Error('expected missing rollout generations to render as em dash');
  if (emptyDetailText.includes('Runtime generations | desired 0 · observed 0')) throw new Error('expected missing runtime generations to avoid zero fallback');
  if (emptyDetailText.includes('Rollout generations | desired 0 · observed 0')) throw new Error('expected missing rollout generations to avoid zero fallback');
  const legacyCurrentStep = findFirst(emptyEnv.document, (node) => node.className === 'detail-step-meta detail-step-meta-current');
const legacyPendingStep = findFirst(emptyEnv.document, (node) => node.className === 'detail-step-meta detail-step-meta-pending');
	  if (!legacyCurrentStep || legacyCurrentStep.textContent !== 'Current rollout step') throw new Error('expected legacy current rollout step to render as current');
	  if (!legacyPendingStep || legacyPendingStep.textContent !== 'Pending rollout step') throw new Error('expected legacy non-current rollout step to render as pending');
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}

func TestEmbeddedUIDetailFetchRerenderRaceWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptJSON, err := json.Marshal(embeddedUIScriptForTest(t))
	if err != nil {
		t.Fatalf("marshal embedded script: %v", err)
	}

	accountsJSON, err := json.Marshal([]map[string]any{
		{
			"name":         "acct-a",
			"awsAccountId": "111111111111",
			"deployments": []map[string]any{
				{
					"name":           "api",
					"appRef":         "payments",
					"environmentRef": "prod",
					"region":         "us-west-2",
					"targetType":     "ecs",
					"strategy":       "canary",
					"executionMode":  "ecsCodeDeploy",
					"sync":           map[string]any{"state": "Reconciling"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal sample accounts: %v", err)
	}

	detailJSON, err := json.Marshal(map[string]any{
		"name":         "api",
		"accountName":  "acct-a",
		"awsAccountId": "111111111111",
		"region":       "us-west-2",
		"sync":         map[string]any{"reason": "Traffic shift running"},
		"history": []map[string]any{{
			"timestamp": "2026-04-16T06:50:00Z",
			"summary":   "Shifted 10 percent of traffic",
		}},
	})
	if err != nil {
		t.Fatalf("marshal sample detail: %v", err)
	}

	nodeProgram := `
const scriptContent = ` + string(scriptJSON) + `;
const sampleAccounts = ` + string(accountsJSON) + `;
const detailPayload = ` + string(detailJSON) + `;

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
        if (shouldAdd) {
          current.add(name);
        } else {
          current.delete(name);
        }
        this.className = Array.from(current).join(' ');
      },
      contains: (name) => new Set(this.className.split(/\s+/).filter(Boolean)).has(name),
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
  replaceChildren(...nodes) {
    this.children = [];
    this.append(...nodes);
  }
  addEventListener(type, handler) {
    this.listeners[type] = handler;
  }
  dispatch(type, event = {}) {
    if (!event.target) event.target = this;
    const handler = this.listeners[type];
    if (handler) handler(event);
  }
  setAttribute(name, value) {
    this.attributes[name] = String(value);
  }
  getAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null;
  }
  focus() {
    this.ownerDocument.activeElement = this;
  }
}
class InputElement extends Element {
  setSelectionRange(start, end) {
    this.selectionStart = start;
    this.selectionEnd = end;
  }
}
class Document {
  constructor() {
    this.activeElement = null;
    this.byID = new Map();
  }
  createElement(tagName) {
    if (String(tagName).toLowerCase() === 'input') {
      return new InputElement(tagName, this);
    }
    return new Element(tagName, this);
  }
  getElementById(id) {
    return this.byID.get(id) || null;
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

function walk(rootNodes) {
  const stack = Array.from(rootNodes);
  const nodes = [];
  while (stack.length) {
    const node = stack.shift();
    if (!node) continue;
    nodes.push(node);
    stack.unshift(...(node.children || []));
  }
  return nodes;
}

function findFirst(document, predicate) {
  return walk(document.byID.values()).find(predicate) || null;
}

function collectText(node) {
  if (!node) return [];
  return [node.textContent, ...node.children.flatMap((child) => collectText(child))].filter(Boolean);
}

function findDetailPanelForToggle(toggle) {
  const row = toggle.parentNode;
  return row.children.find((child) => child.className === 'detail-panel') || null;
}

function createEnvironment() {
  const document = new Document();
  const toolbar = document.createElement('section');
  const summary = document.createElement('section');
  const accounts = document.createElement('section');
  document.byID.set('toolbar', toolbar);
  document.byID.set('summary', summary);
  document.byID.set('accounts', accounts);

  const window = { location: { pathname: '/', search: '' } };
  const history = {
    replaceState(_state, _title, url) {
      const [pathname, query = ''] = String(url).split('?');
      window.location.pathname = pathname || '/';
      window.location.search = query ? '?' + query : '';
    },
  };
  const fetchCalls = [];
  let resolveDetail;
  const detailBody = new Promise((resolve) => {
    resolveDetail = resolve;
  });
  const fetch = (url) => {
    fetchCalls.push(String(url));
    if (url === '/v1/overview/accounts') {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(sampleAccounts) });
    }
    if (url === '/v1/overview/deployments/api') {
      return Promise.resolve({ ok: true, json: () => detailBody });
    }
    throw new Error('unexpected fetch url: ' + url);
  };

  new Function('window', 'document', 'fetch', 'history', 'HTMLElement', 'HTMLInputElement', scriptContent)(window, document, fetch, history, Element, InputElement);
  return { document, fetchCalls, resolveDetail };
}

async function settle() {
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

(async () => {
  const env = createEnvironment();
  await settle();

  const initialToggle = findFirst(env.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
  if (!initialToggle) throw new Error('expected initial deployment expand toggle');
  initialToggle.dispatch('click');
  await settle();
  if (env.fetchCalls.filter((url) => url === '/v1/overview/deployments/api').length !== 1) {
    throw new Error('expected first expand to start exactly one deployment-detail fetch');
  }

  const initialPanel = findDetailPanelForToggle(initialToggle);
  if (!initialPanel) throw new Error('expected initial detail panel');
  if (!collectText(initialPanel).join(' | ').includes('Loading deployment details…')) {
    throw new Error('expected initial detail panel to show loading state while fetch is in flight');
  }

  const searchInput = env.document.querySelector('[data-toolbar-field="Search deployments"]');
  if (!searchInput) throw new Error('expected search input');
  searchInput.value = 'missing';
  searchInput.dispatch('input', { target: searchInput });
  await settle();
  if (env.document.getElementById('accounts').children[0].textContent !== 'No deployments match the current filters.') {
    throw new Error('expected rerender to replace deployment rows with empty state');
  }

  const rerenderedInput = env.document.querySelector('[data-toolbar-field="Search deployments"]');
  if (!rerenderedInput) throw new Error('expected rerendered search input');
  rerenderedInput.value = '';
  rerenderedInput.dispatch('input', { target: rerenderedInput });
  await settle();

  const replacementToggle = findFirst(env.document, (node) => node.tagName === 'BUTTON' && node.getAttribute('aria-label') === 'Expand details for api');
  if (!replacementToggle) throw new Error('expected replacement deployment expand toggle after rerender');
  if (replacementToggle === initialToggle) throw new Error('expected overview rerender to create a new deployment DOM node');
  replacementToggle.dispatch('click');
  await settle();
  if (env.fetchCalls.filter((url) => url === '/v1/overview/deployments/api').length !== 1) {
    throw new Error('expected replacement expand to reuse the in-flight deployment-detail fetch');
  }

  const replacementPanel = findDetailPanelForToggle(replacementToggle);
  if (!replacementPanel) throw new Error('expected replacement detail panel');
  if (!collectText(replacementPanel).join(' | ').includes('Loading deployment details…')) {
    throw new Error('expected replacement detail panel to show loading state before the shared fetch resolves');
  }

  env.resolveDetail(detailPayload);
  await settle();

  const replacementText = collectText(replacementPanel).join(' | ');
  if (replacementText.includes('Loading deployment details…')) {
    throw new Error('expected replacement detail panel to stop showing loading state after shared fetch resolves');
  }
  if (!replacementText.includes('Sync reason') || !replacementText.includes('Traffic shift running')) {
    throw new Error('expected replacement detail panel to render fetched deployment detail after rerender');
  }
})();
`

	cmd := exec.Command("node", "-e", nodeProgram)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
