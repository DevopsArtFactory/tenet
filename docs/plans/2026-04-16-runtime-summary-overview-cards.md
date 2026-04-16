# Runtime Summary Overview Cards Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Surface controller-authored runtime summary metadata in `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and the embedded `/` UI so operators can see the latest runtime event without expanding raw history first.

**Architecture:** Add a small reusable runtime-summary read model derived from bounded `status.history[]` plus the current deployment status. Thread that additive summary through the overview catalog builders and render it in the account-oriented UI as top-level metrics/searchable text, while preserving the existing detail history timeline.

**Tech Stack:** Go 1.25, stdlib HTTP/JSON, embedded HTML/JS UI, Go tests, Node syntax check for embedded script.

---

### Task 1: Add additive runtime summary read model to overview builders

**Objective:** Derive stable latest-runtime metadata from deployment status/history and expose it through the overview read models.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`
- Modify: `internal/domain/status.go` (only if a shared additive struct is needed)

**Step 1: Write failing tests**

Add tests that expect:
- `BuildAccountOverviews(...)` to surface per-deployment runtime summary fields derived from the newest `status.history[]` entry when available.
- `BuildDeploymentOverviewDetail(...)` to surface the same summary for the detail payload.
- Missing history to fall back to current status/rollout fields without mutating stored snapshots.

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuild(AccountOverviews|DeploymentOverviewDetail).*RuntimeSummary' -count=1`
Expected: FAIL because runtime summary fields do not exist yet.

**Step 3: Write minimal implementation**

Implement a small additive runtime-summary helper that exposes fields such as latest timestamp, summary text, sync state, health status, rollout phase/current step, and history count. Reuse it from both overview builders and clone defensively.

**Step 4: Run test to verify pass**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:/opt/homebrew/bin:$PATH go test ./internal/catalog -run 'TestBuild(AccountOverviews|DeploymentOverviewDetail).*RuntimeSummary' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/domain/status.go
git commit -m "Surface latest runtime summary in overview read models"
```

### Task 2: Expose runtime summary through API contracts and embedded UI

**Objective:** Render the additive runtime summary in the account overview UI and keep search/detail behavior aligned with the new payload fields.

**Files:**
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `internal/api/web/index.html`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add tests that expect:
- `/v1/overview/accounts` and `/v1/overview/deployments/{deploymentName}` JSON to include the new runtime-summary object.
- The embedded UI to render latest runtime summary/timestamp on overview cards using safe DOM construction.
- The search/filter harness to match runtime-summary text in addition to existing rollout/history fields.

**Step 2: Run test to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestOverview|TestUI' -count=1`
Expected: FAIL because runtime summary is not yet returned/rendered.

**Step 3: Write minimal implementation**

Thread the runtime summary through the router payloads and render concise latest-runtime metrics in each deployment card. Keep the detail panel history unchanged, extend the search haystack additively, and continue using `createElement`/`textContent` instead of `innerHTML`.

**Step 4: Run test to verify pass**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestOverview|TestUI' -count=1`
- `python3 - <<'PY'
from pathlib import Path
html = Path('internal/api/web/index.html').read_text()
start = html.index('<script>') + len('<script>')
end = html.index('</script>', start)
Path('/tmp/tenet-overview-runtime-summary.js').write_text(html[start:end])
PY
node --check /tmp/tenet-overview-runtime-summary.js`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/api/router_test.go internal/api/ui_test.go internal/api/ui_behavior_test.go internal/api/web/index.html README.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Render latest runtime summary in overview UI"
```

### Task 3: Full verification, review, and ship the slice

**Objective:** Verify the slice end-to-end, run independent review, then push and create/update PRs if auth is available.

**Files:**
- Review working tree diff only.

**Step 1: Run focused and full verification**

Run:
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
- `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`
- If UI/runtime contracts changed materially: `docker compose up --build -d`, verify `/v1/overview/accounts`, `/v1/overview/deployments/payments-api-prod`, and `/`, then `docker compose down --remove-orphans`

**Step 2: Independent review**

Run spec compliance review first, then code-quality review, and fix any gaps before proceeding.

**Step 3: Push and PR workflow**

Run:
- `git push -u origin HEAD`
- Detect `gh auth status`; if available, create/update a PR, monitor checks, and enable auto-merge or merge when green.

**Step 4: Final commit message**

Use Lore protocol trailers and include verification commands in `Tested:`.
