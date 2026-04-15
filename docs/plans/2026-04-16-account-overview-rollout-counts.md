# Account Overview Rollout Counts Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add account-level deployment status counts to the joined overview API and embedded UI so operators can scan which accounts are healthy, progressing, paused, or errored without opening every deployment row.

**Architecture:** Extend `catalog.AccountOverview` with additive aggregate counters derived from the same joined deployment/status read model already used for `/v1/overview/accounts`. Reuse that API payload in the embedded `/` UI so account cards can show concise status pills without adding a new endpoint or duplicating grouping logic in the browser.

**Tech Stack:** Go 1.25, existing `internal/catalog` overview builder, `internal/api` router/UI, Go test suite, local smoke checks, Docker Compose.

---

### Task 1: Add account-level deployment status counts to the joined overview model

**Objective:** Derive stable per-account status counts from grouped deployment overviews and expose them through `/v1/overview/accounts`.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add tests that expect each `AccountOverview` to include additive count fields for:
- total deployments
- synced deployments
- progressing deployments (`Reconciling`, `OutOfSync`, `Drifted`)
- paused deployments
- errored deployments

Cover at least:
- one account with a mix of synced and progressing deployments
- one account with no status snapshot yet (should still count toward total deployments, but not toward a concrete runtime state bucket)
- `/v1/overview/accounts` JSON including the new additive fields

**Step 2: Run test to verify failure**

Run: `go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts' -v`

Expected: FAIL because `AccountOverview` does not yet include the status-count fields.

**Step 3: Write minimal implementation**

Add additive integer fields to `catalog.AccountOverview` such as:
- `deploymentCount`
- `syncedCount`
- `progressingCount`
- `pausedCount`
- `errorCount`

Populate them during `BuildAccountOverviews` while preserving the existing deployment grouping, rollout summary cloning, and sort order. Treat missing/zero-valued sync state as unknown: count the deployment toward `deploymentCount`, but not toward any runtime-state bucket.

**Step 4: Run tests to verify pass**

Run: `go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Expose account-level overview status counts"
```

### Task 2: Render the new account status counts in the embedded overview UI and docs

**Objective:** Show the new aggregate counts in the account-oriented UI and document the additive overview payload.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/router_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add assertions that the embedded UI source now includes labels and rendering logic for account-level count pills/cards such as synced/progressing/paused/error counts, and extend router/UI tests so fixture account payloads include the new count fields.

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestRouterRootServesUI|TestEmbeddedUI|TestRouterOverviewAccounts' -v`

Expected: FAIL because the embedded UI currently only shows the deployment total pill per account.

**Step 3: Write minimal implementation**

- Update the embedded UI to render account-level status count pills in each account header.
- Keep unknown-state deployments visible via the existing deployment total even if they do not belong to a concrete state bucket.
- Update README and overview UI architecture docs to describe the additive per-account status-count summary in `/v1/overview/accounts` and `/`.

**Step 4: Run tests to verify pass**

Run: `go test ./internal/api -run 'TestRouterRootServesUI|TestEmbeddedUI|TestRouterOverviewAccounts' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/router_test.go README.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Show account-level status counts in overview UI"
```