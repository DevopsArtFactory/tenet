# Runtime Rollout Context History Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich controller-authored runtime history and runtime summaries with rollout context so operators can see which strategy/execution model each runtime event represents.

**Architecture:** Add additive rollout-context fields to controller-owned runtime history and derived runtime summaries. Preserve backward compatibility by keeping the fields optional, populating them from the compiled deployment during controller writes, then surfacing them unchanged through joined overview APIs and the embedded UI.

**Tech Stack:** Go 1.25, standard library, embedded HTML/JS UI, existing overview/status read models.

---

### Task 1: Add rollout-context fields to runtime history and summaries

**Objective:** Persist strategy, target type, and execution mode in controller-authored runtime history entries and runtime summaries.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Test: `cmd/tenet-controller/runtime_test.go`
- Test: `internal/catalog/overview_test.go`

**Step 1: Write failing tests**

Add focused tests asserting that:
- `deriveRolloutSummary` / runtime-history derivation preserves deployment `strategy`, `targetType`, and `executionMode` in new history entries.
- `deriveRuntimeSummary` copies those fields from the latest history entry and falls back to current rollout/deployment context when history is absent.

**Step 2: Run test to verify failure**

Run: `PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./cmd/tenet-controller ./internal/catalog -run 'TestAppendRuntimeHistory|TestBuild(AccountOverviews|DeploymentOverviewDetail)RuntimeSummary' -v`
Expected: FAIL because runtime history/runtime summary fields do not exist yet.

**Step 3: Write minimal implementation**

Implement additive optional fields on `domain.StatusHistoryEntry` and `domain.RuntimeSummary`, then populate them from the compiled deployment in controller runtime/history derivation and catalog summary derivation.

**Step 4: Run test to verify pass**

Run: `PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./cmd/tenet-controller ./internal/catalog -run 'TestAppendRuntimeHistory|TestBuild(AccountOverviews|DeploymentOverviewDetail)RuntimeSummary' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go internal/catalog/overview_test.go
git commit -m "Capture rollout context in runtime history"
```

### Task 2: Surface rollout-context fields through API/UI/docs

**Objective:** Keep the overview API payloads, embedded UI, and operator docs aligned with the richer runtime-summary/history contract.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `internal/api/web/index.html`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/features/overview-dashboard.md`

**Step 1: Write failing tests**

Add/extend tests asserting that:
- overview/deployment raw JSON includes `runtimeSummary.strategy`, `runtimeSummary.targetType`, `runtimeSummary.executionMode`, and matching `history[*]` fields.
- embedded UI renders those fields in runtime-summary/history sections using safe DOM construction.
- embedded script still passes `node --check`.

**Step 2: Run test to verify failure**

Run: `PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./internal/api -run 'Test(OverviewAccountsResponse|OverviewDeploymentResponse|DeploymentOverviewDetailRoute|UI|DeploymentWorkspace)' -v`
Expected: FAIL because the API/UI does not yet surface the rollout-context fields.

**Step 3: Write minimal implementation**

Propagate the additive fields through the overview/read-model JSON and render concise runtime context rows in the embedded application/resource/workspace history sections.

**Step 4: Run test to verify pass**

Run: `PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./internal/api -run 'Test(OverviewAccountsResponse|OverviewDeploymentResponse|DeploymentOverviewDetailRoute|UI|DeploymentWorkspace)' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/api/router_test.go internal/api/ui_test.go internal/api/ui_behavior_test.go internal/api/web/index.html README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md docs/features/overview-dashboard.md
git commit -m "Expose rollout context in runtime summaries"
```

---

## Final verification

Run:
- `PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./...`
- `PATH=/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `/v1/overview/accounts`, `/v1/overview/deployments/payments-api-prod`, and `/`
- `docker compose down --remove-orphans`

Expected: all commands succeed and the runtime summary/history surfaces show rollout-context metadata without breaking existing overview/resource drill-down flows.
