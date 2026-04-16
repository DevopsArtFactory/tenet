# Account Overview Drill-down Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add an embedded account-detail drill-down page at `/` so operators can move from the applications landing page into an account-oriented grouped deployment view backed by the existing joined overview APIs.

**Architecture:** Keep the current applications-first landing page intact, but add an additive `account` page mode in the embedded UI that fetches `/v1/overview/accounts/{awsAccountId}` and renders grouped project/environment sections using the existing client-side grouping helpers. Preserve the current drill-down flow by adding `Open account` links from the applications overview and by keeping `Open application` links, JSON quick links, and filter/query preservation consistent with the existing page-router helpers.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, existing `/v1/overview/accounts/{awsAccountId}` and `/v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` routes, Go tests plus Node-backed UI behavior tests.

---

### Task 1: Lock the account-page route contract with tests

**Objective:** Add failing tests that define the new `account` page query-param contract and embedded UI behavior before changing the HTML.

**Files:**
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Test: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**

Add assertions covering:
- `parsePageState()` accepts `page=account`
- applications overview rows render an `_self` `Open account` link built with `buildPageURL('account', { accountId: deployment.awsAccountId })`
- account page summary text includes `Account deployment overview`
- account page renders grouped project/environment sections and preserves `Account JSON`, `Group JSON`, and `Open application` links
- account-page back link returns to `/?page=applications...` while preserving existing toolbar filters

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1`
Expected: FAIL because `account` is not yet a supported page and the new links/page renderer do not exist.

**Step 3: Write minimal implementation**

Update the embedded UI router contract and behavior tests only enough to express the intended page shape and link contracts.

**Step 4: Run test to verify pass**

Run: `go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Add account drill-down UI test contract

Constraint: Keep the applications-first landing page while adding an account-oriented drill-down path
Confidence: medium
Scope-risk: narrow
Directive: Preserve filter/query params and `_self` drill-down navigation for embedded UI routes
Tested: go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1
```

### Task 2: Implement the embedded account-detail page

**Objective:** Add the new `account` page mode and link it from the applications overview without disturbing the existing application/resource/drift/plan pages.

**Files:**
- Modify: `internal/api/web/index.html`
- Test: `internal/api/ui_test.go`
- Test: `internal/api/ui_behavior_test.go`

**Step 1: Write failing test**

Use the failing tests from Task 1 as the red phase.

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1`
Expected: FAIL until the account page is implemented.

**Step 3: Write minimal implementation**

In `internal/api/web/index.html`:
- extend `parsePageState()` and `buildPageURL()` to carry an additive `accountId` param
- add a fetch helper for `/v1/overview/accounts/{awsAccountId}`
- add `renderAccountDetailPage(account)` that:
  - renders an account summary/header (`Account deployment overview`)
  - shows stable account/topology/status summary cards
  - groups deployments by app/environment/region with the existing grouping helper
  - exposes `Account JSON`, `Group JSON`, and `Open application` links outside toggle buttons
- add `Open account` links to applications overview rows
- preserve current safe DOM construction (`createElement`, `textContent`, `replaceChildren`) and filter/back-link behavior

**Step 4: Run test to verify pass**

Run: `go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Add account drill-down page to embedded UI

Constraint: Reuse existing joined overview account APIs instead of adding new server routes
Rejected: Make `/` account-first again | the current applications-first landing page is already the shipped default UX
Confidence: medium
Scope-risk: moderate
Directive: Keep account/group links outside interactive toggle buttons and preserve `_self` drill-down navigation
Tested: go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1
```

### Task 3: Update docs and smoke coverage, then verify end to end

**Objective:** Keep operator docs and smoke checks aligned with the new account drill-down route.

**Files:**
- Modify: `README.md`
- Modify: `docs/features/overview-dashboard.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `scripts/smoke-local.sh`

**Step 1: Write failing test/check**

Extend smoke assertions to require:
- `Open account` appears in `/`
- the embedded HTML includes the `buildPageURL('account', { accountId: deployment.awsAccountId })` route contract

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture -count=1 && ./scripts/smoke-local.sh`
Expected: smoke or UI contract assertions fail until docs/smoke are updated.

**Step 3: Write minimal implementation**

- Update README and overview docs to describe the additive account drill-down page and its relationship to the existing applications-first landing page
- extend `scripts/smoke-local.sh` to assert the new link text and route template in `/`

**Step 4: Run test to verify pass**

Run:
- `go test ./internal/api -run 'TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture|TestApplicationsOverviewPageWithNodeHarness' -count=1`
- `go test ./...`
- `make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl http://localhost:8080/v1/overview/accounts/123456789012`
- verify `curl 'http://localhost:8080/?page=account&accountId=123456789012'`
- `docker compose down --remove-orphans`

Expected: all commands pass.

**Step 5: Commit**

```bash
git add README.md docs/features/overview-dashboard.md docs/architecture/tenet-v1-overview-ui.md scripts/smoke-local.sh
git commit -m "Document account drill-down overview flow

Constraint: Smoke/docs must stay aligned with the embedded route contract because `/` is the operator entry point
Confidence: medium
Scope-risk: narrow
Directive: Update smoke/docs whenever page-router query params or quick-link text changes
Tested: go test ./...; make smoke-local; docker compose config; docker compose up --build -d; docker compose down --remove-orphans
```
