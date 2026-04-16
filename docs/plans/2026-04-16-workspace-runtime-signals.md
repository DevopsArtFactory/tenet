# Workspace Runtime Signals Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Enrich the embedded deployment workspaces at `/` so the read-only drift and plan/apply shells surface rollout execution resources plus latest runtime change signals without requiring operators to expand the generic detail panel first.

**Architecture:** Keep the slice additive and UI-focused. Reuse the existing `/v1/overview/deployments/{deploymentName}` payload, extend the shared `renderDeploymentWorkspace` path in `internal/api/web/index.html`, and lock the behavior down with Go string-fragment tests plus the Node-backed UI behavior harness. Update README and overview architecture docs in the same slice.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, Go tests, Node syntax/behavior checks, existing overview detail API.

---

### Task 1: Surface rollout execution and latest runtime signals in deployment workspaces

**Objective:** Render operator-facing rollout execution resources and runtime summary metadata directly inside the drift and plan/apply workspace shells.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Verify: `go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML|TestDeploymentWorkspace' -v`

**Step 1: Write failing tests**

Add expectations that the workspace renderer:
- shows a `Latest runtime signals` section with runtime summary source/timestamp/history count metadata plus literal `runtimeSummary.changes[]` entries when present
- shows a `Rollout execution` section in both drift and plan/apply workspaces using `detail.rollout.execution[]`
- keeps empty states explicit when change signals or execution resources are absent
- continues to use safe DOM construction (`createElement`, `textContent`) rather than `innerHTML`

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML|TestDeploymentWorkspace' -v`
Expected: FAIL because the workspaces do not yet render those sections.

**Step 3: Write minimal implementation**

In `renderDeploymentWorkspace`, add small shared helpers/sections that:
- render latest runtime metadata from `detail.runtimeSummary`
- render execution-resource rows from `detail.rollout.execution`
- place those sections in both the drift and plan/apply workspace modes without changing the existing read-only placeholder semantics

**Step 4: Run test to verify pass**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestEmbeddedUI|TestIndexHTML|TestDeploymentWorkspace' -v`
- `python3 - <<'PY'
from pathlib import Path
html = Path('internal/api/web/index.html').read_text()
start = html.index('<script>') + len('<script>')
end = html.index('</script>', start)
Path('/tmp/tenet-workspace-runtime-signals.js').write_text(html[start:end])
PY
node --check /tmp/tenet-workspace-runtime-signals.js`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Clarify deployment workspace runtime signals"
```

### Task 2: Update docs and verify the full slice

**Objective:** Keep operator docs aligned with the richer workspace shells and verify the repository end to end.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Verify: `go test ./...`
- Verify: `make smoke-local`
- Verify: `docker compose config`

**Step 1: Update docs**

Document that deployment workspaces now surface:
- rollout execution resources directly in the workspace shell
- latest runtime summary source/timestamp/history-count metadata
- runtime change-signal rows derived from `runtimeSummary.changes[]`

**Step 2: Run full verification**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`
- If workspace rendering changed materially: `docker compose up --build -d`, verify `/`, `/?page=drift&deployment=payments-api-prod`, and `/?page=plan&deployment=payments-api-prod`, then `docker compose down --remove-orphans`

Expected: PASS.

**Step 3: Review diff**

Run: `git diff --stat`
Expected: only workspace UI/tests/docs and the plan document for this slice.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-overview-ui.md docs/plans/2026-04-16-workspace-runtime-signals.md
git commit -m "Document deployment workspace runtime signals"
```

### Task 3: Push and run PR workflow

**Objective:** Publish the verified slice autonomously.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin feat/workspace-runtime-signals`

**Step 2: Create/update PR**

Use `gh pr create` / `gh pr edit` if authenticated.

**Step 3: Monitor checks and merge**

Use `gh pr checks` and `gh pr merge --auto --squash --delete-branch` when ready.
