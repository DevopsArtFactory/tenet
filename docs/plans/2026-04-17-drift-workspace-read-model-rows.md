# Drift Workspace Read-Model Rows Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Replace the drift workspace placeholder with a real read-only drift table derived from the deployment detail payload that Tenet already serves.

**Architecture:** Keep this slice narrowly scoped to the embedded UI plus focused tests/docs. Reuse `GET /v1/overview/deployments/{deploymentName}` and synthesize operator-facing drift rows from existing desired/runtime fields (`desiredArtifact`, `deployedArtifact`, `observedGeneration`, `runtimeSummary`, `rollout`, `conditions`, `userData`, and rollout execution metadata). Do not add mutation APIs or new backend endpoints in this slice.

**Tech Stack:** Embedded HTML/CSS/JS in `internal/api/web/index.html`, Go UI contract tests, Node-backed behavior tests, README and overview UI architecture docs.

---

### Task 1: Lock the drift-row contract with failing tests

**Objective:** Define the new drift workspace behavior before changing implementation.

**Files:**
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Verify: `internal/api/web/index.html`

**Step 1: Write failing tests**

Require the drift workspace to render:
- a `Field-level drift` section containing a table-like header with `Field path`, `Desired`, `Actual`, and `Severity`
- drift rows derived from existing deployment-detail fields, including at least:
  - desired vs deployed artifact
  - desired vs observed generation
  - desired rollout intent vs current runtime context when runtime summary disagrees
  - unhealthy / drifted status conditions
- severity pills (`high`, `medium`, `low`)
- sorting by field path and severity
- filtering by query text / severity inside the drift workspace
- empty-state copy when the deployment is effectively synced and no drift rows are derived

**Step 2: Run test to verify failure**

Run:
`PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./internal/api -run 'TestDeploymentWorkspaceDriftViewWithNodeHarness|TestEmbeddedUIIncludesDeploymentWorkspaceViewModes' -v`

Expected: FAIL because the drift workspace still renders only the placeholder.

**Step 3: Write minimal implementation**

Add drift-row derivation helpers plus drift-workspace controls/rendering inside `renderDeploymentWorkspace(...)`, keeping all rendering safe via `createElement`, `textContent`, and `replaceChildren`.

**Step 4: Run tests to verify pass**

Run the same command and expect PASS.

### Task 2: Implement the drift-workspace UI slice

**Objective:** Ship a usable read-only drift viewer without broadening scope into write APIs.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`

**Step 1: Write failing targeted assertions**

Add assertions that prove:
- drift rows sort by severity and field path deterministically
- toolbar/filter state stays local to the drift workspace and does not break the existing applications/account drill-down pages
- the empty state appears when all compared fields are aligned
- no `innerHTML` is introduced for drift values/messages

**Step 2: Run focused tests to verify failure**

Run:
`PATH=/opt/homebrew/bin:$PATH /opt/homebrew/bin/go test ./internal/api -run 'TestDeploymentWorkspaceDriftViewWithNodeHarness|TestEmbeddedUIIncludesApplicationsOverviewPageArchitecture' -v`

Expected: FAIL until the new drift viewer is wired in.

**Step 3: Write minimal implementation**

Implement helper functions that derive rows from the existing detail payload and render:
- context header / summary
- sort + severity filter controls
- the drift rows table/list
- empty states for synced/no-drift deployments
- existing status-conditions section beneath the drift table

**Step 4: Run focused tests to verify pass**

Run the same command and expect PASS.

### Task 3: Update docs and verify the slice end to end

**Objective:** Keep docs and smoke-level verification aligned with the new drift workspace behavior.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `scripts/smoke-local.sh` (only if needed for route/UI contract coverage)

**Step 1: Update docs**

Document that the drift workspace is now a read-only derived drift view sourced from existing deployment detail data, not a live field-by-field cloud diff. Be explicit about the current scope and limitations.

**Step 2: Run final verification**

Run exactly:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl -fsS http://127.0.0.1:8080/ | grep -F 'Field-level drift'`
- `docker compose down --remove-orphans`

Expected: PASS.

### Task 4: Push and run PR workflow

**Objective:** Publish the verified slice autonomously.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin <branch>`

**Step 2: Create or update PR if auth is available**

Use `gh pr create` / `gh pr edit` with a body file when authenticated.

**Step 3: Monitor checks and merge when ready**

Use `gh pr checks` and `gh pr merge --auto --squash --delete-branch` when CI/settings allow it.
