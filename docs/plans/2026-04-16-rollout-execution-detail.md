# Rollout Execution Detail Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add controller-derived rollout execution detail for EC2 and ECS deployments so operators can see which candidate/runtime resources Tenet intends to use, then surface that detail in the overview UI and docs.

**Architecture:** Extend the compiled deployment/runtime model with the minimal metadata needed to describe EC2 userdata bootstrap and deterministic execution resource names. Have the controller derive additive rollout execution details from the deployment plus the current abstract plan, persist them in the rollout summary, and expose them unchanged through the existing overview/status read models and embedded UI.

**Tech Stack:** Go 1.25, standard library, existing manifest/catalog/statusstore/controller packages, embedded HTML/CSS/JS UI.

---

### Task 1: Add deployment/runtime fields and rollout execution-detail derivation

**Objective:** Persist additive rollout execution detail describing EC2 candidate ASGs/target groups and ECS service/CodeDeploy resources.

**Files:**
- Modify: `internal/domain/deployment.go`
- Modify: `internal/domain/status.go`
- Modify: `internal/manifest/compiler.go`
- Modify: `internal/manifest/compiler_test.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- compiled EC2 deployments preserve a normalized userdata path from manifest intent
- controller-derived rollout summaries for EC2 weighted-target-group plans include execution detail for candidate ASG, active/candidate target groups, and userdata bootstrap path
- controller-derived rollout summaries for ECS CodeDeploy plans include execution detail for ECS service, CodeDeploy application, and deployment group
- existing rollout summaries remain additive and keep phase/current-step semantics unchanged

**Step 2: Run tests to verify failure**

Run:
- `go test ./internal/manifest -run 'TestCompile.*UserData' -count=1`
- `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsWeightedTargetGroupRolloutSummaryExecutionDetails|PersistsCodeDeployRolloutSummaryExecutionDetails)' -count=1`

Expected: FAIL because the deployment/runtime fields and rollout execution details do not exist yet.

**Step 3: Write minimal implementation**

Implement:
- a deployment field carrying normalized userdata path/type for EC2 workloads
- additive rollout summary fields for execution detail (resource names + operator-facing descriptions)
- controller helpers that derive deterministic execution detail by execution mode:
  - EC2 weighted target groups: candidate ASG, active target group, candidate target group, userdata bootstrap source
  - EC2 instance refresh: service ASG, userdata bootstrap source
  - ECS CodeDeploy: ECS service, CodeDeploy application, deployment group
  - ECS rolling: ECS service
- preserve cloning/copy behavior for rollout summaries in catalog joins

**Step 4: Run tests to verify pass**

Run:
- `go test ./internal/manifest -run 'TestCompile.*UserData' -count=1`
- `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans(PersistsWeightedTargetGroupRolloutSummaryExecutionDetails|PersistsCodeDeployRolloutSummaryExecutionDetails|PersistsInstanceRefreshRolloutSummaryForEC2RollingDeployment|PersistsHealthySyncedRolloutSummaryWhenGenerationsMatch)' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/domain/deployment.go internal/domain/status.go internal/manifest/compiler.go internal/manifest/compiler_test.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Expose rollout execution resource details"
```

### Task 2: Surface rollout execution detail through overview joins, UI, and docs

**Objective:** Keep the joined overview surfaces and embedded UI aligned with the richer rollout execution details.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add tests/assertions that:
- overview/detail builders clone and preserve rollout execution-detail fields
- embedded UI renders a rollout execution-detail section using safe DOM APIs only
- UI copy references the new execution-detail payload without regressing lazy-load/detail rendering contracts

**Step 2: Run tests to verify failure**

Run:
- `go test ./internal/catalog -run 'TestBuild(AccountOverviews|DeploymentOverviewDetail)' -count=1`
- `go test ./internal/api -run 'TestEmbeddedUI(UsesSafeDetailRenderingAndPreservesFalsyMetrics|LazyLoadsDeploymentDetailsOnExpandOnly)' -count=1`

Expected: FAIL because joins/UI do not yet surface rollout execution detail.

**Step 3: Write minimal implementation**

Implement:
- rollout-summary cloning updates in catalog join helpers for new execution-detail fields
- a safe UI detail section that renders execution resources/descriptions from `rollout.execution`
- README and architecture docs describing the new rollout execution-detail semantics and examples for EC2 weighted-target-group, EC2 instance-refresh, ECS rolling, and ECS CodeDeploy plans

**Step 4: Run tests to verify pass**

Run:
- `go test ./...`
- `make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl -sf http://localhost:8080/v1/overview/accounts >/dev/null`
- verify `curl -sf http://localhost:8080/v1/overview/deployments/payments-api-prod >/dev/null`
- verify `curl -sf http://localhost:8080/ >/dev/null`
- `docker compose down --remove-orphans`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/api/web/index.html internal/api/ui_test.go README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Surface rollout execution details in overview UI"
```