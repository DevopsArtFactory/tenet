package api

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedUIFilterBehaviorWithNodeHarness(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	scriptBlock := regexp.MustCompile(`(?s)<script>(.*)</script>`)
	matches := scriptBlock.FindSubmatch(indexHTML)
	if len(matches) != 2 {
		t.Fatal("expected embedded UI HTML to contain exactly one script block")
	}

	scriptJSON, err := json.Marshal(string(matches[1]))
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
					"name":               "api",
					"appRef":             "payments",
					"environmentRef":     "prod",
					"region":             "us-west-2",
					"targetType":         "ecs",
					"strategy":           "canary",
					"executionMode":      "ecsCodeDeploy",
					"desiredArtifact":    map[string]any{"image": "ghcr.io/example/api:v2"},
					"deployedArtifact":   map[string]any{"image": "ghcr.io/example/api:v1"},
					"sync":               map[string]any{"state": "Reconciling", "reason": "Traffic shift running"},
					"rollout":            map[string]any{"phase": "Progressing", "currentStep": "ShiftTrafficCanary", "summary": "10 percent shifted", "steps": []map[string]any{{"kind": "ShiftTrafficCanary"}, {"kind": "BakeTraffic"}}},
					"health":             map[string]any{"overall": "Healthy", "summary": "steady"},
					"observedGeneration": 1,
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

(async () => {
  const defaultEnv = createEnvironment('');
  await settle();
  const defaultAccounts = defaultEnv.document.getElementById('accounts');
  if (defaultAccounts.children.length !== 3) {
    throw new Error('expected default overview to render accounts with and without deployments');
  }

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
