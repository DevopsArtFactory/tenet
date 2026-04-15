# Strategy-Aware Rollout Planner Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make Tenet derive target- and strategy-specific rollout steps so EC2 weighted-target-group and ECS CodeDeploy recommendations produce operator-meaningful runtime summaries.

**Architecture:** Keep the existing controller-owned rollout summary pipeline, but teach the reconcile planner to emit strategy-aware step sequences based on `deployment.TargetType`, `deployment.Strategy`, and `deployment.ExecutionMode`. Reuse the existing persisted `rollout` summary and embedded overview UI so richer step sequences flow through the current API surfaces without adding new endpoints.

**Tech Stack:** Go 1.25, existing `internal/reconcile` planner, controller runtime status persistence, account overview API/UI, Go test suite, local smoke checks, Docker Compose.

---

### Task 1: Add strategy-aware planner steps for EC2 and ECS execution modes

**Objective:** Replace the generic rollout plan with mechanism-specific steps that match EC2 immutable-host rollouts and ECS CodeDeploy/native rolling behavior.

**Files:**
- Modify: `internal/reconcile/planner.go`
- Modify: `internal/reconcile/planner_test.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add planner tests that expect concrete step sequences for:
- EC2 `canary` / `linear` / `blueGreen` with `asgWeightedTargetGroups`
- EC2 `rolling` with `asgInstanceRefresh`
- ECS `rolling` with `ecsRolling`
- ECS `blueGreen` / `canary` / `linear` with `ecsCodeDeploy`

Also add controller-runtime tests that assert persisted rollout summaries now include those richer step names for at least:
- one EC2 canary deployment
- one ECS linear deployment

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/reconcile ./cmd/tenet-controller -run 'TestPlanner|TestEmitRuntimePlans' -v`

Expected: FAIL because the planner still emits generic steps like `ResolvePlacement`, `ObserveCurrentState`, and `PlanCandidateGeneration`.

**Step 3: Write minimal implementation**

Update `Planner.Build` so when desired generation is ahead it emits strategy-aware sequences such as:
- EC2 weighted target groups:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `CreateCandidateCapacity`
  - `VerifyCandidateCapacity`
  - `ShiftTrafficCanary` or `ShiftTrafficLinear` or `ShiftTrafficFull`
  - `PromoteCandidateCapacity`
- EC2 rolling instance refresh:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `StartInstanceRefresh`
  - `VerifyReplacementCapacity`
- ECS rolling:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `UpdateECSService`
  - `VerifyServiceStability`
- ECS CodeDeploy-backed strategies:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `CreateCodeDeployDeployment`
  - `ShiftTrafficCanary` or `ShiftTrafficLinear` or `ShiftTrafficFull`
  - `VerifyServiceStability`

Preserve the existing same-generation repair, mark-synced, paused, and await-fresh-desired behavior.

**Step 4: Run tests to verify pass**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/reconcile ./cmd/tenet-controller -run 'TestPlanner|TestEmitRuntimePlans' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/reconcile/planner.go internal/reconcile/planner_test.go cmd/tenet-controller/runtime_test.go
git commit -m "Model rollout plans by EC2 and ECS execution mode"
```

### Task 2: Document the richer rollout summaries and verify overview surfaces still reflect them

**Objective:** Keep README and architecture docs aligned with the richer strategy-aware rollout plan output.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-rollout-strategy-model.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Step 1: Write failing tests**

Extend overview/API tests to expect example rollout steps that reflect the new strategy-aware planner terminology, such as `ShiftTrafficCanary` and `CreateCodeDeployDeployment`, in overview payloads and embedded UI HTML fixtures.

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts|TestRouterRootServesUI' -v`

Expected: FAIL because the current test fixtures and docs still describe generic runtime steps.

**Step 3: Write minimal implementation**

- Update tests to use the richer rollout step examples.
- Update README to explain that runtime summaries now reflect EC2 candidate-capacity and ECS CodeDeploy/native-rolling phases.
- Update the rollout strategy architecture doc with example step sequences for EC2 and ECS.
- Update the runtime rollout summary doc to explain that `steps[]` now intentionally carry strategy-aware operator labels.

**Step 4: Run tests to verify pass**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviews|TestRouterOverviewAccounts|TestRouterRootServesUI' -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add README.md docs/architecture/tenet-v1-rollout-strategy-model.md docs/architecture/tenet-v1-runtime-rollout-summary.md internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Document strategy-aware rollout summaries"
```