# Rollout Phase and Summary Detail Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make controller-owned rollout summaries describe in-progress EC2/ECS execution more precisely, then surface that richer summary text in the account overview UI and docs.

**Architecture:** Keep the existing planner step list as the source of truth, but derive rollout `phase` and `summary` from the full plan shape instead of collapsing most in-progress plans to `Accepted` / `Observing current state`. Preserve the existing account overview API payload and extend the embedded UI to render the rollout summary text already present on each deployment.

**Tech Stack:** Go 1.25, standard library, embedded HTML/JS UI, existing `go test` + `make smoke-local` + Docker Compose verification.

---

### Task 1: Derive strategy-aware rollout phases and human-readable summaries in the controller

**Objective:** Update controller runtime rollout summary generation so EC2 weighted-target-group, EC2 rolling, ECS rolling, and ECS CodeDeploy plans persist more specific `phase` and `summary` values.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`
- Test: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- EC2 canary/blue-green weighted-target-group plans persist `phase=Promoting` and a summary describing ALB weighted-target-group traffic shifting / candidate promotion.
- ECS linear/canary/blue-green CodeDeploy plans persist `phase=Stabilizing` and a summary describing CodeDeploy traffic shifting and ECS service stabilization.
- EC2 rolling instance-refresh plans persist a non-`Accepted` phase with a summary mentioning instance refresh / replacement capacity.
- Same-generation repair plans keep their existing repair-oriented summary semantics.

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsWeightedTargetGroupRolloutSummaryDetails|PersistsCodeDeployRolloutSummaryDetails|PersistsInstanceRefreshRolloutSummaryDetails)' -count=1`
Expected: FAIL because rollout summaries still collapse to coarse defaults.

**Step 3: Write minimal implementation**

Implement plan-shape-aware helpers in `cmd/tenet-controller/runtime.go` that:
- classify phase from the ordered plan steps,
- emit human-readable summary text for weighted-target-group, instance-refresh, ECS rolling, and ECS CodeDeploy plans,
- preserve existing synced/healthy and repair behavior,
- avoid changing stored step ordering or current-step behavior.

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsWeightedTargetGroupRolloutSummaryDetails|PersistsCodeDeployRolloutSummaryDetails|PersistsInstanceRefreshRolloutSummaryDetails|PersistsHealthySyncedRolloutSummaryWhenGenerationsMatch|DoesNotPersistCompletedRolloutSummaryForSyncedButUnhealthyDeployment)' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "feat: enrich controller rollout phase summaries"
```

### Task 2: Surface rollout summary text in the overview UI and refresh docs

**Objective:** Show the richer controller-authored rollout summary in the account overview UI and document the refined semantics in README and architecture notes.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Test: `go test ./...`

**Step 1: Write failing test / verification target**

Use the existing controller/runtime tests from Task 1 as the automated proof that richer summary text exists in persisted rollout state. For UI/docs, add a focused browser-safe rendering change without changing API shapes.

**Step 2: Run verification target**

Run: `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsWeightedTargetGroupRolloutSummaryDetails|PersistsCodeDeployRolloutSummaryDetails|PersistsInstanceRefreshRolloutSummaryDetails)' -count=1`
Expected: PASS before the UI/docs change.

**Step 3: Write minimal implementation**

Update the embedded UI to render rollout summary text as a metric/card field using safe DOM construction. Update README and `docs/architecture/tenet-v1-runtime-rollout-summary.md` so they describe the refined per-execution-mode phase/summary behavior instead of the old coarse-value caveat.

**Step 4: Run broader verification**

Run:
- `go test ./...`
- `make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `http://localhost:8080/v1/overview/accounts` and `http://localhost:8080/`
- `docker compose down --remove-orphans`

Expected: PASS, overview endpoint still serves, UI renders, Docker stack stays healthy.

**Step 5: Commit**

```bash
git add internal/api/web/index.html README.md docs/architecture/tenet-v1-runtime-rollout-summary.md
git commit -m "docs(ui): surface detailed rollout summaries"
```