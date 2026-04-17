# Runtime Summary Artifact and Condition Context Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich controller-authored runtime history and joined runtime summaries with deployed-artifact and condition context so operators can understand what changed without opening the raw status payload.

**Architecture:** Extend the additive read model instead of inventing a new API. The controller should write richer `status.history[]` entries, the catalog join should project the newest history/current snapshot into `runtimeSummary`, and the embedded UI should render/search that additive context safely.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, Go tests, Node script syntax check via existing UI tests.

---

### Task 1: Add failing controller/catalog tests for artifact and condition runtime context

**Objective:** Lock the desired runtime-history and runtime-summary behavior before changing production code.

**Files:**
- Modify: `cmd/tenet-controller/runtime_test.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- controller-authored `status.history[]` entries capture the latest `deployedArtifact` and `conditions`
- history `changes[]` includes `deployedArtifact` and `conditions` when those runtime facts change
- `runtimeSummary` prefers the latest history-backed artifact/condition context and falls back to current snapshot data when history is absent
- raw overview/detail JSON includes nested `runtimeSummary.deployedArtifact` and `runtimeSummary.conditions`

**Step 2: Run tests to verify failure**

Run:
- `go test ./cmd/tenet-controller -run 'Test(EmitRuntimePlans|AppendRuntimeHistory)' -v`
- `go test ./internal/catalog -run 'Test(BuildDeploymentOverviewDetail|BuildApplicationOverviews|DeriveRuntimeSummary)' -v`
- `go test ./internal/api -run 'TestOverview|TestDeploymentOverview' -v`

Expected: FAIL with missing artifact/condition runtime-summary or history assertions.

**Step 3: Commit**

```bash
git add cmd/tenet-controller/runtime_test.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Lock runtime artifact and condition summary expectations"
```

### Task 2: Implement runtime-history and runtime-summary artifact/condition context

**Objective:** Teach the controller and joined overview builder to preserve and project artifact/condition runtime context.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `internal/catalog/overview.go`

**Step 1: Write minimal implementation**

Implement additive fields on:
- `domain.StatusHistoryEntry`
- `domain.RuntimeSummary`

Update controller/runtime logic so:
- `deriveRuntimeHistoryEntry(...)` copies `status.DeployedArtifact` and `status.Conditions`
- history-change detection treats artifact/condition changes as meaningful changes
- legacy/latest history normalization backfills missing artifact/condition context from the current controller view
- `deriveRuntimeSummary(...)` carries history-first artifact/condition context and current-status fallback context
- clone helpers defensively copy artifact/condition slices

**Step 2: Run focused tests to verify pass**

Run:
- `go test ./cmd/tenet-controller -run 'Test(EmitRuntimePlans|AppendRuntimeHistory)' -v`
- `go test ./internal/catalog -run 'Test(BuildDeploymentOverviewDetail|BuildApplicationOverviews|DeriveRuntimeSummary)' -v`
- `go test ./internal/api -run 'TestOverview|TestDeploymentOverview' -v`

Expected: PASS

**Step 3: Commit**

```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go internal/catalog/overview.go
git commit -m "Add artifact and condition context to runtime summaries"
```

### Task 3: Render and document runtime artifact/condition context in the embedded UI

**Objective:** Make the new runtime-summary fields operator-visible in the UI and docs.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-deployment-status-api.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/features/overview-dashboard.md`

**Step 1: Write failing UI/docs tests**

Add assertions that:
- resource/detail/workspace/history surfaces runtime artifact and condition context when present
- client-side runtime-summary search text includes the new artifact/condition context without noisy placeholders when absent
- the embedded script still passes `node --check`

**Step 2: Implement the minimal UI/docs updates**

Update the embedded UI to safely render:
- runtime artifact context (`runtimeSummary.deployedArtifact`)
- concise runtime condition context (`runtimeSummary.conditions`)
- history artifact/condition context for history rows

Keep safe DOM construction only.

Update README/architecture/docs to describe the additive read-model contract and where operators see it.

**Step 3: Run focused UI verification**

Run:
- `go test ./internal/api -run 'TestEmbeddedUI|TestDeploymentWorkspace|TestResourcePage' -v`
- extracted script syntax check through the existing UI tests

Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-deployment-status-api.md docs/architecture/tenet-v1-overview-ui.md docs/features/overview-dashboard.md
git commit -m "Show runtime artifact and condition context in overview UI"
```

---

## Final verification

Run from repo root:

```bash
/opt/homebrew/bin/go test ./...
make smoke-local
docker compose config
docker compose up --build -d
curl -sf http://localhost:8080/v1/overview/deployments/payments-api-prod | jq '.runtimeSummary, .history[-1]'
curl -sf http://localhost:8080/ | grep -q 'Runtime artifact'
docker compose down --remove-orphans
```

Expected:
- all Go tests pass
- smoke-local passes
- Docker Compose starts successfully
- overview/detail payloads expose runtime artifact/condition context
- embedded UI contains the new runtime context labels
