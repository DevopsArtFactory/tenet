# Controller-Derived Runtime Status Defaults Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make `tenet-controller` write useful controller-owned `sync`, `health`, and generation defaults when a deployment has no persisted runtime snapshot yet, while preserving seeded runtime fields when they already exist.

**Architecture:** Keep the slice inside `cmd/tenet-controller/runtime.go` so the controller remains the single author of additive runtime write-model fields. Derive the minimal default status from the already-built reconcile plan, then let existing rollout/history/runtime-summary code consume that normalized status without schema changes.

**Tech Stack:** Go 1.25, stdlib, existing `reconcile.Planner`, `domain.DeploymentStatus`, `statusstore`, controller/unit tests, README + architecture docs.

---

### Task 1: Add failing tests for controller-derived default status fields

**Objective:** Lock the intended behavior before implementation.

**Files:**
- Modify: `cmd/tenet-controller/runtime_test.go`
- Test: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add focused tests that assert:
- a deployment with no prior status gets controller-authored defaults:
  - `observedGeneration` stays `0`
  - `sync.state` becomes `Reconciling`
  - `sync.reason` follows the derived rollout summary/current step
  - `health.overall` becomes `Progressing`
  - `health.summary` explains that Tenet is driving the rollout
- an already-synced healthy deployment keeps or derives `Synced` + `Healthy`
- seeded non-empty `sync` / `health` fields are preserved instead of being overwritten

Suggested test names:
- `TestEmitRuntimePlansDerivesDefaultSyncAndHealthForNewSnapshots`
- `TestCompiledRuntimeStatusForTestPreservesSeededSyncAndHealth`

**Step 2: Run test to verify failure**

Run:
`unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansDerivesDefaultSyncAndHealthForNewSnapshots|TestCompiledRuntimeStatusForTestPreservesSeededSyncAndHealth' -v`

Expected: FAIL because the controller currently writes rollout/history without filling empty `sync` / `health` defaults.

**Step 3: Commit**

Do not commit yet.

---

### Task 2: Implement controller-owned default status derivation

**Objective:** Normalize empty runtime status before rollout/history persistence.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write minimal implementation**

Add a helper in `cmd/tenet-controller/runtime.go` that:
- accepts `deployment`, prior `status`, and computed `plan`
- fills only missing controller-owned fields (`sync`, `health`, optionally `observedGeneration` when synced)
- preserves seeded `deployedArtifact`, `conditions`, `history`, and any already-populated `sync` / `health`
- uses the current plan/rollout summary to derive readable defaults, for example:
  - `MarkSynced` => `Synced` / `Healthy`
  - actionable rollout plans => `Reconciling` / `Progressing`
  - non-actionable paused plans => `Paused` / `Unknown`

Call that helper before `deriveRolloutSummary(...)` / `appendRuntimeHistory(...)`, then recompute rollout/history from the normalized status.

**Step 2: Run focused tests to verify pass**

Run:
`unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansDerivesDefaultSyncAndHealthForNewSnapshots|TestCompiledRuntimeStatusForTestPreservesSeededSyncAndHealth' -v`

Expected: PASS.

**Step 3: Run broader controller tests**

Run:
`unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller -v`

Expected: PASS.

---

### Task 3: Update docs for the controller-authored runtime contract

**Objective:** Keep operator-facing docs aligned with the new runtime defaults.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-deployment-status-api.md`

**Step 1: Update docs**

Document that when the controller has no prior runtime snapshot, it now writes controller-authored default `sync` / `health` values alongside rollout/history:
- `Reconciling` + `Progressing` for active rollout plans
- `Synced` + `Healthy` for completed `MarkSynced` plans
- seeded non-empty runtime fields still win

**Step 2: Verify docs mention the real behavior**

Check the changed docs for consistency with code and tests.

---

### Task 4: Full verification and commit

**Objective:** Prove the slice is safe end-to-end and record it cleanly.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-deployment-status-api.md`

**Step 1: Run full verification**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- if affected by the slice: `docker compose up --build -d`, verify `/v1/overview/applications`, `/v1/overview/deployments/{deploymentName}`, and then `docker compose down --remove-orphans`

**Step 2: Review diff**

Run:
`git diff --stat`

**Step 3: Commit**

Use a Lore-style commit message, for example:

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-deployment-status-api.md docs/plans/2026-04-17-controller-derived-runtime-status-defaults.md
git commit -m "Derive controller-owned runtime defaults for new snapshots

The controller now fills empty sync/health runtime fields before writing
rollout/history so fresh Redis/file snapshots carry actionable status
without requiring a seeded status file.

Constraint: Preserve seeded runtime fields when they already exist
Rejected: Overwrite all sync/health fields on every reconcile | would erase external/seeded runtime evidence
Confidence: medium
Scope-risk: narrow
Directive: Keep rollout/history derivation downstream of normalized controller status defaults
Tested: unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...; unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local; docker compose config
Not-tested: live AWS-backed reconcile execution"
```
