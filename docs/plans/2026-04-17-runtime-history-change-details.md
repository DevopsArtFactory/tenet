# Runtime History Change Details Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich controller-authored runtime history with operator-facing change details so history and runtime summaries explain what changed instead of only listing coarse field keys.

**Architecture:** Keep the slice additive by extending the runtime history/read-model contract with optional `changeDetails` entries derived by the controller from the latest meaningful status transition. Preserve the existing `changes` field for stable machine filtering while adding human-readable detail strings that flow through API JSON, overview/detail joins, and the embedded UI.

**Tech Stack:** Go 1.25, controller/runtime read models, embedded HTML/JS UI, Go tests, Node-backed UI behavior tests.

---

### Task 1: Add failing backend contract tests for additive history change details

**Objective:** Lock the desired controller and API behavior before implementation.

**Files:**
- Modify: `cmd/tenet-controller/runtime_test.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`
- Test: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add focused tests that assert:
- controller-authored history entries include `changeDetails` for changed sync/health/rollout/generation/artifact/condition context;
- runtime summaries derived from latest controller history preserve the same `changeDetails` when history exists;
- raw overview/detail JSON includes `runtimeSummary.changeDetails` and `history[*].changeDetails`.

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller ./internal/catalog ./internal/api -run 'Test.*ChangeDetails|TestRouterOverview.*ChangeDetails' -v`
Expected: FAIL because `changeDetails` does not exist yet.

**Step 3: Commit**

```bash
git add cmd/tenet-controller/runtime_test.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "docs: capture failing tests for runtime history change details"
```

### Task 2: Implement controller-derived change details in runtime history and joined read models

**Objective:** Add additive `changeDetails` support end-to-end in controller/runtime/catalog layers.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write minimal implementation**

Extend `domain.StatusHistoryEntry` and `domain.RuntimeSummary` with optional `changeDetails []string` fields.

Implement controller derivation rules in `cmd/tenet-controller/runtime.go` so details are concise and deterministic, for example:

```go
[]string{
    "Sync changed to Reconciling: Traffic shift running",
    "Health changed to Progressing: candidate fleet bootstrapping",
    "Desired generation changed from 7 to 8",
    "Current step changed from CreateCandidateCapacity to ShiftTrafficCanary",
}
```

Preserve `changes` as the stable machine-readable key list.

Update overview cloning/join helpers so `runtimeSummary.changeDetails` and `history[*].changeDetails` are copied defensively.

**Step 2: Run targeted tests to verify pass**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller ./internal/catalog ./internal/api -run 'Test.*ChangeDetails|TestRouterOverview.*ChangeDetails' -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go internal/catalog/overview.go cmd/tenet-controller/runtime_test.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "feat: add controller-derived runtime history change details"
```

### Task 3: Surface change details in the embedded UI, smoke coverage, and docs

**Objective:** Make the new additive runtime change details visible to operators and document the contract.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `scripts/smoke-local.sh`
- Modify: `README.md`
- Modify: `docs/features/overview-dashboard.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`

**Step 1: Write failing UI tests**

Add focused assertions for safe rendering of runtime/history `changeDetails` in deployment/resource/workspace sections.

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML|Test.*ChangeDetails' -v`
Expected: FAIL because the UI does not render `changeDetails` yet.

**Step 3: Implement minimal UI/docs updates**

Render additive change-detail text with safe DOM APIs (`createElement`, `textContent`) in runtime summary/history sections. Keep existing `changes` label rows, but add human-readable detail rows or lists when present.

Update smoke/docs examples to mention and verify `changeDetails` on overview/detail payloads.

**Step 4: Run targeted tests to verify pass**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML|Test.*ChangeDetails' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go scripts/smoke-local.sh README.md docs/features/overview-dashboard.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-runtime-rollout-summary.md
git commit -m "docs: surface runtime history change details across UI and docs"
```

### Task 4: Full verification and PR handoff

**Objective:** Prove the slice is green end-to-end and ready to land.

**Files:**
- Review only

**Step 1: Run verification**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `/v1/overview/accounts`, `/v1/overview/deployments/payments-api-prod`, and `/`
- `docker compose down --remove-orphans`

**Step 2: Review diff and commit**

```bash
git add -A
git commit -m "feat: enrich runtime history with operator-facing change details"
```

**Step 3: Push and open PR if auth is available**

```bash
git push -u origin HEAD
# then use gh pr create / gh pr checks / gh pr merge --auto when authenticated
```
