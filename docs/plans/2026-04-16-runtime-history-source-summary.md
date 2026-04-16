# Runtime History Source and Summary Surface Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich Tenet's runtime summary and deployment detail read models so operators can see where the latest summary came from and which rollout phase each history entry represents.

**Architecture:** Keep the slice additive. Extend `domain.RuntimeSummary` with an optional `source` field populated from the newest controller-authored history entry, then update catalog builders, router/raw JSON coverage, and the embedded overview UI/detail panel to render the new source/phase metadata with safe DOM construction. Reuse the existing bounded history model; do not introduce a new store or API route.

**Tech Stack:** Go 1.25, standard library tests, embedded HTML/JS UI, existing overview/detail API builders.

---

### Task 1: Add failing tests for runtime summary source derivation

**Objective:** Lock the additive `runtimeSummary.source` contract in the catalog and API read models before changing implementation.

**Files:**
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/domain/status.go`
- Modify: `internal/catalog/overview.go`

**Step 1: Write failing tests**

Add/extend tests so they expect:
- `runtimeSummary.source == "controller"` when built from the newest history entry
- raw JSON from `/v1/overview/accounts` and `/v1/overview/deployments/{deploymentName}` to include `runtimeSummary.source`
- fallback runtime summaries without history to omit the field

**Step 2: Run tests to verify failure**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuild(AccountOverviewsRuntimeSummaryUsesLatestHistory|DeploymentOverviewDetailRuntimeSummaryUsesLatestHistory|AccountOverviewsRuntimeSummaryFallsBackToCurrentStatus|DeploymentOverviewDetailRuntimeSummaryFallsBackToCurrentStatus)' -v`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestOverview(Accounts|Deployment)Route' -v`

Expected: FAIL because `runtimeSummary.source` is not yet modeled or asserted.

**Step 3: Write minimal implementation**

Update `domain.RuntimeSummary` and `deriveRuntimeSummary` so history-backed runtime summaries copy the latest `StatusHistoryEntry.Source`, while no-history fallback summaries keep the field empty/omitted.

**Step 4: Run tests to verify pass**

Re-run the same focused commands and expect PASS.

**Step 5: Commit**

```bash
git add internal/domain/status.go internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Expose runtime summary source in overview read models

Constraint: Runtime summary must stay additive and history-first without inventing synthetic sources for legacy snapshots
Rejected: Add a separate summary provenance endpoint | the existing read models should stay the single operator contract
Confidence: high
Scope-risk: narrow
Directive: Only populate runtimeSummary.source from persisted history-backed summaries unless the runtime model gains explicit provenance elsewhere
Tested: go test ./internal/catalog -run 'TestBuild(AccountOverviewsRuntimeSummaryUsesLatestHistory|DeploymentOverviewDetailRuntimeSummaryUsesLatestHistory|AccountOverviewsRuntimeSummaryFallsBackToCurrentStatus|DeploymentOverviewDetailRuntimeSummaryFallsBackToCurrentStatus)' -v; go test ./internal/api -run 'TestOverview(Accounts|Deployment)Route' -v
Not-tested: full repository suite"
```

### Task 2: Add failing tests for embedded UI history source/phase rendering

**Objective:** Surface the richer metadata in the embedded UI without unsafe rendering regressions.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**

Extend the UI string-fragment and Node-backed behavior tests so they expect:
- a `Runtime source` metric rendered from `detail.runtimeSummary.source` when present
- each history entry to render `History source · <source>` and `History rollout phase · <phase>` when present
- runtime summary text/search helpers to include the additive runtime source
- no `innerHTML` regressions while rendering the new text literally

**Step 2: Run tests to verify failure**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'Test(EmbeddedUIUsesSafeDetailRenderingAndPreservesFalsyMetrics|EmbeddedUIBehaviorWithNodeHarness)' -v`

Expected: FAIL because the new runtime source/history phase metadata is not yet rendered.

**Step 3: Write minimal implementation**

Update the embedded UI to:
- add a `Runtime source` detail metric
- include `runtimeSummary.source` in the overview-card summary text used for display/search
- render `History source` and optional `History rollout phase` rows in the runtime-history section
- keep using `createElement`, `textContent`, and `replaceChildren`

**Step 4: Run tests to verify pass**

Re-run the targeted `go test ./internal/api ...` command and expect PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Show runtime summary source and history phase in UI

Constraint: Embedded UI runtime metadata must keep using safe DOM construction because history content is persisted operator-facing text
Rejected: Render source/phase only in raw JSON | the dashboard detail view is the primary operator surface
Confidence: high
Scope-risk: narrow
Directive: Any future runtime-history fields added to the UI must use createElement/textContent and be covered by the Node harness
Tested: go test ./internal/api -run 'Test(EmbeddedUIUsesSafeDetailRenderingAndPreservesFalsyMetrics|EmbeddedUIBehaviorWithNodeHarness)' -v
Not-tested: full repository suite"
```

### Task 3: Update docs and verify the slice end to end

**Objective:** Keep README and architecture docs aligned with the enriched summary/history contract and verify the repo.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`

**Step 1: Update docs**

Document that:
- `runtimeSummary.source` is present when derived from controller-authored history
- deployment-detail runtime history renders additive source/phase metadata when present
- the embedded UI search/display now includes runtime-summary provenance text

**Step 2: Run full verification**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`

Expected: PASS.

**Step 3: Review diff**

Run: `git diff --stat`
Expected: only runtime summary model/builder tests, embedded UI tests/HTML, and docs for runtime summary/history metadata.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-runtime-rollout-summary.md internal/domain/status.go internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Surface runtime summary provenance in overview details

Constraint: README, runtime-summary docs, and overview UI docs must stay aligned with the additive operator-facing API contract
Rejected: Leave provenance implicit in history only | overview cards and detail metrics should expose the latest summary source directly
Confidence: high
Scope-risk: narrow
Directive: When runtimeSummary fields change, update builder tests, router raw JSON assertions, UI behavior coverage, and docs in the same slice
Tested: go test ./...; make smoke-local; docker compose config
Not-tested: live GitHub Actions CI"
```

### Task 4: Push and run the PR workflow

**Objective:** Publish the verified slice without waiting for user input.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin feat/runtime-history-phase-source`

**Step 2: Create or update PR if auth is available**

Use `gh pr create` / `gh pr edit` when authenticated.

**Step 3: Monitor checks when a PR exists**

Run: `gh pr checks --watch`

**Step 4: Merge when ready**

Use `gh pr merge --auto --squash --delete-branch` when CI allows it.
