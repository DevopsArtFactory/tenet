# Runtime History Change Signals Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add additive controller-authored runtime history change signals so overview APIs, runtime summaries, and the embedded UI can explain what changed between history entries.

**Architecture:** Extend the runtime status history read model with compact additive change labels derived by the controller whenever it appends a new history entry. Preserve the history-first runtime summary behavior by surfacing the latest change labels through catalog joins, router JSON, and the embedded UI without changing existing rollout execution semantics.

**Tech Stack:** Go 1.25, standard library tests, embedded HTML/JavaScript UI, Tenet controller runtime/status read models.

---

### Task 1: Add controller-derived runtime history change labels

**Objective:** Persist additive change labels on status history entries and the derived runtime summary so operators can see what changed in the latest runtime update.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `internal/catalog/overview.go`
- Test: `cmd/tenet-controller/runtime_test.go`
- Test: `internal/catalog/overview_test.go`
- Test: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add tests that expect:
- `appendRuntimeHistory` to record change labels such as `summary`, `sync`, `health`, `desiredGeneration`, `observedGeneration`, `rolloutPhase`, and `currentStep` when those values change.
- the first controller-authored history entry to expose `changes=["initial"]`.
- `runtimeSummary` to clone and surface the latest `changes` field through account overview and deployment-detail JSON.

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/tenet-controller ./internal/catalog ./internal/api -run 'TestEmitRuntimePlans|TestBuild|TestOverview' -v`
Expected: FAIL because status history entries and runtime summaries do not yet carry `changes`.

**Step 3: Write minimal implementation**

Implement additive `changes []string` fields on `domain.StatusHistoryEntry` and `domain.RuntimeSummary`, derive them in `appendRuntimeHistory`, and preserve them through overview/detail builders and router payloads.

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/tenet-controller ./internal/catalog ./internal/api -run 'TestEmitRuntimePlans|TestBuild|TestOverview' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Clarify runtime history changes for operators"
```

### Task 2: Surface runtime history change labels in the embedded UI and docs

**Objective:** Keep the embedded overview/detail UI plus README/architecture docs aligned with the richer runtime-history contract.

**Files:**
- Modify: `internal/api/web/index.html`
- Test: `internal/api/ui_test.go`
- Test: `internal/api/ui_behavior_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add tests that expect:
- the embedded UI script to render latest runtime change labels and per-history-entry change labels using safe DOM construction.
- search/index text to include the latest change labels.
- raw docs to describe the additive `changes[]` contract for `history[]` and `runtimeSummary`.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`
Expected: FAIL because the UI does not yet render history/runtime change labels.

**Step 3: Write minimal implementation**

Render change-label pills/metadata in the embedded deployment cards and history detail section without using `innerHTML`, and document the additive `changes[]` semantics in README plus the overview/runtime architecture docs.

**Step 4: Run tests to verify pass**

Run: `go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Surface runtime history change signals in UI"
```

### Task 3: Verify the full slice end-to-end

**Objective:** Confirm the new runtime-history slice is green across unit, smoke, and Docker-backed checks.

**Files:**
- Modify: `docs/plans/2026-04-16-runtime-history-change-signals.md` (only if verification notes need correction)

**Step 1: Run focused verification**

Run:
- `go test ./...`
- `make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- `curl -sf http://127.0.0.1:8080/v1/overview/accounts >/tmp/tenet-overview.json`
- `curl -sf http://127.0.0.1:8080/v1/overview/deployments/payments-api-prod >/tmp/tenet-detail.json`
- `docker compose down --remove-orphans`

Expected:
- all tests pass
- smoke-local passes
- Docker Compose starts cleanly and both overview endpoints respond with JSON

**Step 2: Review diff**

Run: `git diff --stat`
Expected: only runtime history change-signal code/tests/docs for this slice.

**Step 3: Commit**

```bash
git add docs/plans/2026-04-16-runtime-history-change-signals.md
git commit -m "Document runtime history change-signal verification"
```
