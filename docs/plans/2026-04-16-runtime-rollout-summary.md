# Runtime Rollout Summary Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add controller-owned runtime rollout summaries that persist into the active status store and surface in the account-oriented overview API/UI.

**Architecture:** Derive a lightweight rollout summary from the reconcile plan on each controller loop, merge it into deployment status snapshots, persist the snapshots through the existing file/Redis abstraction, and expose the new runtime fields through the joined overview model and embedded UI. Keep the slice additive and backwards-compatible with existing seeded status fixtures.

**Tech Stack:** Go 1.25, existing `internal/reconcile`, `internal/statusstore`, embedded HTML UI, Go test suite, Docker Compose smoke checks.

---

### Task 1: Persist controller-owned rollout summaries into deployment status snapshots

**Objective:** Make the controller derive and save a runtime summary for every deployment on each reconcile loop.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**
- Add a domain/runtime test that expects a persisted status to include:
  - a rollout summary with a phase and ordered step list
  - a current step for candidate planning when desired generation is ahead
  - a healthy synced summary when desired and observed generations match
- Add a controller runtime test that runs `emitRuntimePlans` or `runController` against a temporary repo/status path and asserts the status file is rewritten with the derived rollout summary.

**Step 2: Run test to verify failure**
Run: `go test ./cmd/tenet-controller -run 'TestRunController.*|TestEmitRuntime.*' -v`
Expected: FAIL because rollout summary fields/derivation do not exist yet.

**Step 3: Write minimal implementation**
- Add additive rollout summary structs on `domain.DeploymentStatus` (phase/current step/steps/message).
- Derive summaries from `reconcile.Plan` in `cmd/tenet-controller/runtime.go`.
- Merge derived runtime data with seeded status snapshots and persist the full snapshot set via the existing store.
- Preserve existing deployed artifact metadata and existing richer condition/history-compatible fields.

**Step 4: Run tests to verify pass**
Run: `go test ./cmd/tenet-controller -run 'TestRunController.*|TestEmitRuntime.*' -v`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Persist runtime rollout summaries in controller status snapshots"
```

### Task 2: Surface rollout summaries in overview responses, embedded UI, and docs

**Objective:** Expose the new runtime rollout fields through the account overview API and render them in the browser UI.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/web/index.html`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Create: `docs/architecture/tenet-v1-runtime-rollout-summary.md`

**Step 1: Write failing tests**
- Extend overview tests to expect rollout summary fields copied from deployment status.
- Extend router/UI tests to expect the embedded UI HTML to reference rollout summary rendering labels.

**Step 2: Run test to verify failure**
Run: `go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts|TestRouterRootServesUI' -v`
Expected: FAIL because overview/UI do not expose rollout summary fields yet.

**Step 3: Write minimal implementation**
- Add rollout summary fields to `catalog.DeploymentOverview`.
- Copy runtime rollout summary data from status snapshots inside `BuildAccountOverviews`.
- Update the embedded UI to show rollout phase, current step, and planned steps in each deployment card.
- Document the new controller-owned runtime summary slice in README and architecture docs.

**Step 4: Run tests to verify pass**
Run: `go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts|TestRouterRootServesUI' -v`
Expected: PASS.

**Step 5: Commit**
```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go internal/api/web/index.html README.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-runtime-rollout-summary.md
git commit -m "Expose runtime rollout summaries in overview UI"
```
