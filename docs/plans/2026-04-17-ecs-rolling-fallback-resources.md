# ECS Rolling Fallback Resources Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Preserve resource drill-down and resource-log routing for ECS rolling deployments even when deployment-detail payloads omit `rollout.execution[]`.

**Architecture:** Extend the canonical fallback rollout-execution inventory used by both the API backend and embedded UI so native ECS rolling deployments synthesize the same `ecsService` resource the controller already authors. Keep the fallback contract byte-for-byte aligned across backend binding, UI link generation, smoke/docs text, and tests so bookmarked resource pages remain stable.

**Tech Stack:** Go 1.25, embedded HTML/JS, existing router/UI Node harness tests, smoke-local script, README and feature docs.

---

### Task 1: Lock the missing ECS rolling fallback behavior with failing tests

**Objective:** Add focused tests that fail until ECS rolling fallback resources exist in both the API and embedded UI.

**Files:**
- Modify: `cmd/tenet-api/main_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `internal/api/ui_test.go`

**Step 1: Write failing test**

Add backend coverage requiring `deploymentHasExecutionResource` fallback binding to accept:

```go
resourceType := "ecsService"
resourceName := "worker-prod-service"
```

for a deployment detail shaped as:

```go
detail := catalog.DeploymentOverviewDetail{
    Name:          "worker-prod",
    ExecutionMode: "ecsRolling",
}
```

Add UI coverage requiring the application detail page to synthesize this fallback resource and render an `Open resource` link for an ECS rolling deployment whose `rollout.execution` is missing or empty.

Add string-contract coverage in `internal/api/ui_test.go` for the embedded fallback branch:

```js
if (detail.executionMode === 'ecsRolling') {
  return [
    { resource: 'ecsService', name: `${detail.name}-service`, description: 'ECS service Tenet updates with native rolling deployment' },
  ];
}
```

**Step 2: Run test to verify failure**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-api -run 'TestNewAPIOptionsOverviewDeploymentResourceLogsBindsECSRollingFallbackResource|TestCanonicalRolloutExecutionFallback' -v`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`

Expected: FAIL because ECS rolling fallback resources are not synthesized everywhere yet.

**Step 3: Commit**

Do not commit yet.

---

### Task 2: Implement the ECS rolling fallback contract and align docs/smoke

**Objective:** Ship the fallback in backend + UI and keep docs/smoke coverage aligned with the new resource contract.

**Files:**
- Modify: `cmd/tenet-api/main.go`
- Modify: `internal/api/web/index.html`
- Modify: `scripts/smoke-local.sh`
- Modify: `README.md`
- Modify: `docs/features/overview-dashboard.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write minimal implementation**

Implement the same canonical fallback resource in both backend and UI:

```go
{Resource: "ecsService", Name: detail.Name + "-service", Description: "ECS service Tenet updates with native rolling deployment"}
```

```js
{ resource: 'ecsService', name: `${detail.name}-service`, description: 'ECS service Tenet updates with native rolling deployment' }
```

Update smoke/docs so the fallback inventory explicitly includes `ecsRolling`.

**Step 2: Run targeted tests**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-api -run 'TestNewAPIOptionsOverviewDeploymentResourceLogsBindsECSRollingFallbackResource|TestCanonicalRolloutExecutionFallback' -v`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`

Expected: PASS.

**Step 3: Final verification**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl -fsS "http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service"` returns JSON
- verify `curl -fsS "http://localhost:8080/" | grep -q "Open resource"`
- `docker compose down --remove-orphans`

**Step 4: Commit**

```bash
git add cmd/tenet-api/main.go cmd/tenet-api/main_test.go internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go scripts/smoke-local.sh README.md docs/features/overview-dashboard.md docs/architecture/tenet-v1-overview-ui.md docs/plans/2026-04-17-ecs-rolling-fallback-resources.md
git commit -m "Keep ECS rolling drill-downs reachable without rollout execution rows

Constraint: Older fixture/local detail payloads can omit rollout.execution even though the resource detail and recent-log flows still need a stable ECS service identity
Rejected: Treat missing execution rows as no linked resources | breaks native ECS rolling drill-down and resource-log routing for valid deployments
Confidence: high
Scope-risk: narrow
Reversibility: clean
Directive: Keep backend and UI fallback rollout inventories byte-for-byte aligned with controller-authored execution metadata
Tested: targeted cmd/tenet-api and internal/api tests, go test ./..., make smoke-local, docker compose config, docker compose up --build -d, docker compose down --remove-orphans
Not-tested: live AWS-backed recent-log retrieval for a real ECS rolling deployment"
```
