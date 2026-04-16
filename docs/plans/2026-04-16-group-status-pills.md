# Group Status Pills Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Improve the `/` embedded account overview by showing per-project/environment status pills so operators can scan grouped deployment health without expanding each deployment card.

**Architecture:** Keep the slice client-side and additive inside `internal/api/web/index.html`, deriving grouped status counts from the already-loaded `/v1/overview/accounts` payload. Protect the behavior with both string-contract tests and the existing Node-backed embedded UI harness, then update README and architecture docs to describe the new grouped status summary.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, existing Go string-fragment tests, Node-backed behavior harness, README + architecture docs.

---

### Task 1: Add failing UI tests for grouped status pills

**Objective:** Lock the intended grouped-status rendering contract before changing the embedded UI.

**Files:**
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Verify: `go test ./internal/api -run 'TestEmbeddedUIGroupsDeploymentsByProjectAndEnvironment|TestUIProjectEnvironmentGroupingWithNodeHarness' -v`

**Step 1: Write failing test**

Update `TestEmbeddedUIGroupsDeploymentsByProjectAndEnvironment` to require:
- a `deriveGroupCounts(group)` helper
- grouped pills for `synced`, `progressing`, `paused`, and `error`
- grouped count labels such as `1 synced` and `1 paused`

Update `TestUIProjectEnvironmentGroupingWithNodeHarness` so the rendered text must include grouped status pills for the sample groups:
- `1 synced` for `payments · prod`
- `1 progressing` for `payments · staging`
- `1 paused` for `jobs · prod`

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestEmbeddedUIGroupsDeploymentsByProjectAndEnvironment|TestUIProjectEnvironmentGroupingWithNodeHarness' -v`
Expected: FAIL because grouped status pills are not yet rendered.

**Step 3: Write minimal implementation**

In `internal/api/web/index.html`:
- add a `deriveGroupCounts(group)` helper mirroring the account-level sync bucketing semantics
- append grouped status pills beside the existing deployment-count pill
- keep the rendering safe with `createElement` / `textContent`

**Step 4: Run test to verify pass**

Run the same focused command and expect PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Improve grouped deployment status scanning

Constraint: Group summaries must stay purely additive on top of /v1/overview/accounts without adding new API fetches
Rejected: Lazy-load grouped detail API for counts | the list payload already contains enough deployment sync data
Confidence: high
Scope-risk: narrow
Directive: Keep grouped sync bucketing aligned with account-level summary semantics
Tested: go test ./internal/api -run 'TestEmbeddedUIGroupsDeploymentsByProjectAndEnvironment|TestUIProjectEnvironmentGroupingWithNodeHarness' -v
Not-tested: full repository suite"
```

### Task 2: Update docs and run full verification

**Objective:** Keep the operator docs aligned with the new grouped status summary and verify the repository slice end-to-end.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Verify: `go test ./...`
- Verify: `make smoke-local`
- Verify: `docker compose config`

**Step 1: Update docs**

Describe that each project/environment group on `/` now shows additive grouped status pills derived from the visible deployments using the same sync-state bucketing semantics as the account header.

**Step 2: Run verification**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`

Expected: PASS.

**Step 3: Review diff**

Run: `git diff --stat`
Expected: embedded UI/tests plus doc updates only.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-overview-ui.md internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Document grouped status pills in overview UI

Constraint: README and architecture docs must stay aligned with the embedded UI contract operators see at /
Rejected: Doc-only follow-up later | UI affordance changes are easy to miss without same-slice documentation
Confidence: high
Scope-risk: narrow
Directive: When group-header summary semantics change, update both README and overview-ui architecture docs in the same slice
Tested: go test ./...; make smoke-local; docker compose config
Not-tested: live GitHub Actions CI"
```

### Task 3: Push and run PR workflow

**Objective:** Publish the verified slice without waiting for user input.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin feat/group-status-pills`

**Step 2: Create or update PR**

Use authenticated `gh` flow to create a PR against `main` with a concise summary and test plan.

**Step 3: Monitor checks and merge**

Use `gh pr checks` / `gh pr merge --auto --squash --delete-branch` when CI permits.
