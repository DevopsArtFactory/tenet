# Rollout Step Status Summary Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich controller-authored rollout summaries with per-step progress status so operators can distinguish completed, current, and pending work for EC2/ECS runtime plans.

**Architecture:** Extend `domain.RolloutStepSummary` with an additive status field derived from the controller's current actionable plan index, preserve the richer payload through overview/status APIs, and render the status in the embedded UI rollout-step detail list. Keep the model controller-derived rather than pretending to track live AWS execution cursors.

**Tech Stack:** Go 1.25, stdlib JSON/HTTP, embedded HTML/JS UI, existing Go/Node-backed tests.

---

### Task 1: Add rollout step status modeling in the controller runtime summary

**Objective:** Add an additive `status` field to rollout step summaries and derive `completed/current/pending` values from the controller plan.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Test: `cmd/tenet-controller/runtime_test.go`
- Test: `internal/api/router_test.go`

**Step 1: Write failing tests**
- Add controller/runtime assertions that weighted-target-group and ECS CodeDeploy summaries persist `rollout.steps[*].status` with preflight steps marked `completed`, actionable step marked `current`, and future steps marked `pending`.
- Add router raw-JSON assertions that `rollout.steps[*].status` is serialized for overview/detail responses.

**Step 2: Run targeted tests to verify failure**
- `go test ./cmd/tenet-controller ./internal/api -run 'Test.*Rollout.*'`
- Expected: FAIL because `status` is not yet modeled/serialized.

**Step 3: Write minimal implementation**
- Introduce `status` on `domain.RolloutStepSummary`.
- Derive per-step statuses in `deriveRolloutSummary` using the current actionable step index.
- Preserve existing `current` semantics and keep the change additive/backward-compatible.

**Step 4: Run tests to verify pass**
- `go test ./cmd/tenet-controller ./internal/api -run 'Test.*Rollout.*'`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/domain/status.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go internal/api/router_test.go`
- `git commit -m "feat: add rollout step statuses"`

### Task 2: Render rollout step status in the embedded UI

**Objective:** Show completed/current/pending step state in the deployment detail rollout-step list without unsafe DOM operations.

**Files:**
- Modify: `internal/api/web/index.html`
- Test: `internal/api/ui_test.go`
- Test: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**
- Add UI contract assertions for rollout-step status rendering text/class markers.
- Extend the Node-backed behavior harness fixture to include step statuses and assert the rendered rollout-step metadata distinguishes completed/current/pending states.

**Step 2: Run targeted tests to verify failure**
- `go test ./internal/api -run 'TestEmbeddedUI.*'`
- Expected: FAIL because the UI only renders `current` vs `planned`.

**Step 3: Write minimal implementation**
- Update embedded UI helpers so the detail rollout-step list renders operator-facing text derived from `step.status` with a safe fallback to the legacy `current` boolean.
- Keep the overview card summary compact; only the detail list needs richer status labels in this slice.

**Step 4: Run tests to verify pass**
- `go test ./internal/api -run 'TestEmbeddedUI.*'`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go`
- `git commit -m "feat: show rollout step progress in embedded UI"`

### Task 3: Update README and architecture docs for the additive API field

**Objective:** Document the new rollout-step status field and its controller-derived semantics.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write/update doc expectations**
- Describe `rollout.steps[*].status` and the meanings of `completed`, `current`, and `pending`.
- Note that the field remains controller-derived from the current plan, not live AWS execution telemetry.

**Step 2: Verify docs mention the new additive payload**
- `grep -n "status" README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md`
- Expected: matching mentions of rollout step status semantics.

**Step 3: Commit**
- `git add README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md docs/plans/2026-04-16-rollout-step-status-summary.md`
- `git commit -m "docs: describe rollout step status summaries"`
