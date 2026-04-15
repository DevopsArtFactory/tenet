# Runtime History Overview Detail Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add controller-authored runtime history entries to persisted deployment status and surface recent history through the joined overview detail API and embedded UI.

**Architecture:** Extend `domain.DeploymentStatus` with additive history metadata that the controller appends during reconcile writes without breaking existing file/Redis stores. Expose the history through `/v1/overview/deployments/{deploymentName}` and render a compact recent-history timeline in the expandable deployment detail panel at `/`.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, existing file/Redis status stores, Go tests plus Node script syntax/behavior checks already used by the UI tests.

---

### Task 1: Add additive runtime history types and overview/detail plumbing

**Objective:** Model persisted runtime history on deployment status snapshots and pass it through the overview detail read model.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go` (if joined detail JSON contract assertions need coverage)

**Step 1: Write failing tests**
- Add a catalog overview/detail test proving `BuildDeploymentOverviewDetail` clones and returns recent history entries from persisted status.
- Add a clone-safety assertion proving callers cannot mutate the original stored history slice through the returned detail struct.

**Step 2: Run test to verify failure**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuildDeploymentOverviewDetail' -count=1`
Expected: FAIL because the detail model does not yet carry history.

**Step 3: Write minimal implementation**
- Add `StatusHistoryEntry` to `internal/domain/status.go` with timestamp, source, summary, sync, health, rollout phase/current step fields.
- Add `History []domain.StatusHistoryEntry` to `domain.DeploymentStatus`.
- Add `History []domain.StatusHistoryEntry` to `catalog.DeploymentOverviewDetail`.
- Clone history entries when building overview detail/read models.

**Step 4: Run test to verify pass**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuildDeploymentOverviewDetail' -count=1`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/domain/status.go internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Add runtime history to overview detail read models"
```

### Task 2: Make the controller append bounded runtime history entries when it writes status

**Objective:** Persist a concise controller-owned runtime history timeline during reconcile writes.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**
- Add a runtime test that seeds a prior history entry and verifies a new controller reconcile appends a bounded latest entry containing timestamp/source/summary/phase/current step.
- Add a runtime test that verifies no duplicate history entry is appended when the controller writes an unchanged status snapshot.
- Add a runtime test that verifies history length is capped (for example latest 10 entries) when repeated state changes occur.

**Step 2: Run test to verify failure**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsRuntimeHistory|AvoidsDuplicateRuntimeHistory|CapsRuntimeHistory)' -count=1`
Expected: FAIL because runtime history is not yet appended.

**Step 3: Write minimal implementation**
- Add a helper that derives a controller history entry from the newly computed status/rollout summary.
- Append a new entry only when the operator-facing runtime status meaningfully changes versus the prior snapshot.
- Cap history length to the most recent bounded window.
- Preserve additive compatibility with file and Redis stores by relying on existing JSON marshaling.

**Step 4: Run test to verify pass**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsRuntimeHistory|AvoidsDuplicateRuntimeHistory|CapsRuntimeHistory)' -count=1`
Expected: PASS.

**Step 5: Commit**
```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Persist controller-authored runtime history entries"
```

### Task 3: Render recent runtime history in the embedded overview UI and document it

**Objective:** Show recent runtime history in the expandable deployment detail panel and update repo docs.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go` (if behavior assertions change)
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/architecture/tenet-v1-runtime-state-storage.md`

**Step 1: Write failing tests**
- Add UI string/contract tests asserting the detail renderer includes a runtime history section, safe DOM rendering, and empty-state copy.
- Add/extend a behavior test if needed to verify history rows render from fetched detail data.

**Step 2: Run test to verify failure**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestUIScript|TestUIBehavior' -count=1`
Expected: FAIL because the UI does not yet render runtime history.

**Step 3: Write minimal implementation**
- Add a recent-history section to the lazy-loaded detail panel using safe DOM construction only.
- Render timestamp, summary, sync state, health, and rollout step when present.
- Keep an explicit empty state when no history exists.
- Update README and architecture docs to describe controller-authored runtime history and its current bounded scope.

**Step 4: Run test to verify pass**
Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestUIScript|TestUIBehavior' -count=1`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-runtime-state-storage.md
git commit -m "Show runtime history in the overview detail UI"
```

### Task 4: Final verification and integration review

**Objective:** Verify the whole slice is green and documented.

**Files:**
- Review all changed files from Tasks 1-3.

**Step 1: Run targeted and full validation**
Run:
```bash
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local
docker compose config
```
If API/UI/runtime behavior changed materially, also run:
```bash
docker compose up --build -d
curl http://localhost:8080/v1/overview/accounts
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl http://localhost:8080/
docker compose down --remove-orphans
```

**Step 2: Review diff**
Run: `git diff --stat`
Expected: focused status/history slice touching controller, overview detail, UI, and docs.

**Step 3: Final commit**
```bash
git add -A
git commit -m "Add runtime history timeline to deployment detail views"
```
