# Runtime Summary Generation Metadata Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Surface desired/observed generation metadata in controller-authored runtime history and overview runtime summaries so operators can see rollout freshness without opening raw status snapshots.

**Architecture:** Extend the additive runtime-history/runtime-summary contract with generation fields, populate them from controller-owned rollout/status data, and thread them through the overview/detail JSON and embedded UI using safe DOM rendering. Keep the change additive and backward-compatible.

**Tech Stack:** Go 1.25, stdlib JSON/HTTP tests, embedded HTML/JS UI, Node syntax/behavior harness.

---

### Task 1: Add generation-aware runtime history and runtime summaries

**Objective:** Persist desired/observed generation values in controller-authored history entries and overview runtime summaries.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- `appendRuntimeHistory` appends a new entry when desired/observed generations change even if sync/health text is otherwise stable.
- `deriveRuntimeSummary` copies `desiredGeneration` and `observedGeneration` from the latest history entry.
- fallback runtime summaries without history still expose generation fields from current rollout/status data.
- raw overview/detail JSON payloads include `runtimeSummary.desiredGeneration`, `runtimeSummary.observedGeneration`, and `history[*].desiredGeneration` / `history[*].observedGeneration`.

**Step 2: Run tests to verify failure**

Run:
- `go test ./cmd/tenet-controller -run 'TestAppendRuntimeHistory|TestEmitRuntimePlans' -v`
- `go test ./internal/catalog ./internal/api -run 'TestBuildAccountOverviewsRuntimeSummary|TestRouterOverview' -v`

Expected: FAIL because generation fields do not exist yet.

**Step 3: Write minimal implementation**

- Add additive `desiredGeneration` / `observedGeneration` fields to `domain.StatusHistoryEntry` and `domain.RuntimeSummary`.
- Populate history entries from current rollout/status data in `deriveRuntimeHistoryEntry`.
- Include generation changes in runtime-history dedupe logic.
- Populate history-first and fallback runtime summaries in `internal/catalog/overview.go`.

**Step 4: Run tests to verify pass**

Run the same targeted commands and then `go test ./...`.

**Step 5: Commit**

```bash
git add internal/domain/status.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router_test.go
git commit -m "Improve runtime summaries with rollout generations"
```

### Task 2: Surface runtime generation metadata in the embedded overview UI and docs

**Objective:** Show the new runtime generation metadata in the deployment detail/history UI and document the API contract.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`

**Step 1: Write failing tests**

Add assertions that the embedded UI:
- renders runtime-summary generation metadata safely,
- renders history generation metadata safely,
- keeps using DOM construction (no `innerHTML`),
- and keeps the extracted script valid.

**Step 2: Run tests to verify failure**

Run:
- `go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`

Expected: FAIL because the new generation strings are not rendered yet.

**Step 3: Write minimal implementation**

- Add safe UI rows for runtime summary generations and history generations.
- Update docs to describe the additive fields and their semantics.

**Step 4: Run tests to verify pass**

Run:
- `go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML' -v`
- `node --check /tmp/tenet-index-script.js` (or equivalent extracted-script syntax check from tests)

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-runtime-rollout-summary.md
git commit -m "Show runtime generations in overview UI"
```

---

## Verification

Run from repo root with the Tenet Go 1.25 toolchain:

```bash
export GOROOT=/Users/kuma/.gvm/gos/go1.25
export GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global
export PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH

go test ./...
make smoke-local
docker compose config
docker compose up --build -d
curl -fsS http://127.0.0.1:8080/v1/overview/accounts >/tmp/tenet-overview.json
curl -fsS http://127.0.0.1:8080/v1/overview/deployments/api >/tmp/tenet-detail.json || true
curl -fsS http://127.0.0.1:8080/ >/tmp/tenet-root.html
docker compose down --remove-orphans
```

## Notes

- Keep the contract additive: existing clients should continue to work if they ignore the new fields.
- Preserve `runtimeSummary` nil/omitempty behavior for completely empty statuses.
- Treat desired/observed generation values as controller-owned read-model metadata, not live AWS execution telemetry.
