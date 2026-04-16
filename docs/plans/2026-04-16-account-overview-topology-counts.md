# Account Overview Topology Counts Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add additive `projectCount` and `environmentCount` fields to account overview APIs so the `/` account-oriented UI can use server-authored topology counts while preserving a client-side fallback for older payloads.

**Architecture:** Extend `internal/catalog/overview.go` to derive unique app/environment counts per account and carry them through both the list and detail overview shapes. Update router and UI tests plus the embedded UI to prefer the new fields when present while keeping the existing derived fallback path for compatibility.

**Tech Stack:** Go 1.25, standard library tests, embedded HTML/JS UI, existing overview API/router tests.

---

### Task 1: Add failing tests and server-side topology counts to overview APIs

**Objective:** Make `/v1/overview/accounts` and `/v1/overview/accounts/{awsAccountId}` return additive server-authored topology counts.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add/extend tests so they assert:
- `AccountOverview` includes `projectCount` and `environmentCount` with unique counts per account.
- `AccountOverviewDetail` carries the same two fields.
- Raw JSON from the router includes `projectCount` and `environmentCount` for account overview list/detail payloads.

**Step 2: Run tests to verify failure**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuildAccountOverviews|TestBuildAccountOverviewDetail' -v`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestRouterOverviewAccounts|TestOverviewAccountDetailRoute' -v`

Expected: FAIL because the new fields are not yet part of the catalog/router contract.

**Step 3: Write minimal implementation**

Update `internal/catalog/overview.go` to:
- add `projectCount` and `environmentCount` fields to `AccountOverview` and `AccountOverviewDetail`
- derive unique `AppRef` and `EnvironmentRef` counts per account while building account overviews
- copy those fields into the account detail response

**Step 4: Run tests to verify pass**

Re-run the same focused commands and expect PASS.

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Expose account topology counts in overview APIs

Constraint: The new fields must be additive so older UI payload consumers keep working
Rejected: Compute topology counts only in the browser | overview APIs should expose joined account summaries directly
Confidence: high
Scope-risk: narrow
Directive: Keep projectCount/environmentCount aligned between list and detail overview routes
Tested: go test ./internal/catalog -run 'TestBuildAccountOverviews|TestBuildAccountOverviewDetail' -v; go test ./internal/api -run 'TestRouterOverviewAccounts|TestOverviewAccountDetailRoute' -v
Not-tested: full repository suite"
```

### Task 2: Update the embedded UI/docs to prefer authoritative counts and verify the slice

**Objective:** Keep `/` and the docs aligned with the additive account topology counts contract.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add/extend tests so they assert:
- the embedded UI prefers `account.projectCount` / `account.environmentCount` when present
- the UI still falls back to deriving counts from deployments when the new fields are absent
- the account header topology text and filtering behavior remain intact

**Step 2: Run tests to verify failure**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestUI|TestEmbeddedUI' -v`

Expected: FAIL because the UI/tests do not yet cover the new authoritative count contract.

**Step 3: Write minimal implementation**

Update `internal/api/web/index.html` so the account header topology label uses `projectCount` and `environmentCount` when present and only falls back to `deriveProjectEnvironmentGroups(account)` when the fields are absent.

Update README + overview UI architecture docs to describe the new additive account-level topology count fields.

**Step 4: Run full verification**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-overview-ui.md internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Surface authoritative account topology counts in the UI

Constraint: The embedded UI must preserve compatibility with older overview payloads that omit the new additive fields
Rejected: Remove the client-side fallback | static fixtures and older payloads still need to render correctly
Confidence: high
Scope-risk: narrow
Directive: When changing account header summary fields, keep router JSON, embedded UI behavior, and docs aligned in the same slice
Tested: go test ./...; make smoke-local; docker compose config
Not-tested: live GitHub Actions CI"
```

### Task 3: Push and run PR workflow

**Objective:** Publish the verified slice without waiting for user input.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin feat/account-overview-topology-counts`

**Step 2: Create or update PR if auth is available**

Use `gh pr create` / `gh pr edit` when authenticated.

**Step 3: Monitor checks and merge when ready**

Run `gh pr checks --watch` and use `gh pr merge --auto --squash --delete-branch` when CI/auth support it.
