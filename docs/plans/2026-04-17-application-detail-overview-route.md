# Application Detail Overview Route Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a first-class `/v1/overview/applications/{deploymentName}` detail route and switch the embedded application page to use that route instead of the deployment-detail endpoint directly.

**Architecture:** Keep `/v1/overview/deployments/{deploymentName}` as the canonical deployment-detail compatibility route, but introduce an application-detail read model and router/provider contract that the applications-first UI can target explicitly. Reuse the existing deployment-detail builder for now so the new route stays additive and low-risk, then move the embedded `?page=application` flow plus docs/tests to the new route.

**Tech Stack:** Go 1.25, stdlib `net/http`, existing catalog overview builders/providers, embedded HTML/JS UI, Go router tests, Node-backed UI behavior tests, smoke-local shell verification.

---

### Task 1: Add an application-detail read model builder

**Objective:** Introduce a stable application-detail overview type/provider that mirrors the current deployment-backed application page contract.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/overview_test.go`

**Step 1: Write failing test**

Add tests that prove:
- `BuildApplicationOverviewDetail(...)` returns the same desired/runtime/history data currently needed by the application page
- runtime summary, rollout summary, history, and conditions are cloned defensively
- unknown deployments return `statusstore.ErrNotFound`

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/catalog -run 'TestBuildApplicationOverviewDetail' -v`
Expected: FAIL because the builder/type does not exist yet.

**Step 3: Write minimal implementation**

Add:
- `type ApplicationOverviewDetail struct { ... }`
- `func BuildApplicationOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, deploymentName string) (ApplicationOverviewDetail, error)`

Implementation should reuse `BuildDeploymentOverviewDetail(...)` internally, then clone/copy the fields into the application-detail type so the public application-page contract is explicit without changing the current detail semantics.

**Step 4: Run tests to verify pass**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/catalog -run 'TestBuildApplicationOverviewDetail' -v`
Expected: PASS

### Task 2: Expose `/v1/overview/applications/{deploymentName}` and wire `tenet-api`

**Objective:** Add a dedicated API/provider route for application detail.

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `cmd/tenet-api/main.go`
- Modify: `cmd/tenet-api/main_test.go`

**Step 1: Write failing tests**

Add tests that prove:
- `GET /v1/overview/applications/{deploymentName}` returns JSON from `Options.OverviewApplication`
- provider absence returns `503`
- not found propagates `404`
- method guards still return `405`
- `tenet-api` wires the provider via `catalog.BuildApplicationOverviewDetail(...)`

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api ./cmd/tenet-api -run 'TestRouterOverviewApplication|TestAPIOptionsOverviewApplication' -v`
Expected: FAIL because the route/provider does not exist yet.

**Step 3: Write minimal implementation**

Add:
- `OverviewApplication func(string) (catalog.ApplicationOverviewDetail, error)` to `api.Options`
- router handler for `GET /v1/overview/applications/{deploymentName}`
- `cmd/tenet-api/main.go` provider wiring using `catalog.BuildApplicationOverviewDetail(...)`

**Step 4: Run tests to verify pass**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api ./cmd/tenet-api -run 'TestRouterOverviewApplication|TestAPIOptionsOverviewApplication' -v`
Expected: PASS

### Task 3: Switch the embedded application page and docs to the new route

**Objective:** Make the applications-first UI use the application-detail route explicitly and keep docs/smoke aligned.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `scripts/smoke-local.sh`
- Modify: `README.md`
- Modify: `docs/features/overview-dashboard.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add/update tests that prove:
- the `?page=application` flow fetches `/v1/overview/applications/{deploymentName}`
- deeper `resource` / `drift` / `plan` routes still use deployment detail and remain stable
- raw HTML and smoke-local contract mention the new application-detail route
- docs describe the application page as backed by the new route

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestApplicationDetailPageWithNodeHarness|TestEmbeddedUIIncludesApplicationDetailRoute' -v`
Expected: FAIL because the application page still fetches `/v1/overview/deployments/{deploymentName}`.

**Step 3: Write minimal implementation**

Update the embedded script so:
- `page=application` fetches `/v1/overview/applications/{deploymentName}`
- `page=resource`, `page=drift`, and `page=plan` keep using `/v1/overview/deployments/{deploymentName}`
- safe DOM rendering is preserved
- smoke/docs reflect the dedicated route

**Step 4: Run tests to verify pass**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'TestApplicationDetailPageWithNodeHarness|TestEmbeddedUIIncludesApplicationDetailRoute' -v`
Expected: PASS

### Task 4: Final verification

**Objective:** Prove the slice is safe end-to-end.

**Files:**
- Verify working tree changes from Tasks 1-3

**Step 1: Run full verification**

Run exactly:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl -fsS http://127.0.0.1:8080/v1/overview/applications/payments-api-prod`
- verify `curl -fsS http://127.0.0.1:8080/ | grep -F '/v1/overview/applications/'`
- `docker compose down --remove-orphans`

**Step 2: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go internal/api/router.go internal/api/router_test.go cmd/tenet-api/main.go cmd/tenet-api/main_test.go internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go scripts/smoke-local.sh README.md docs/features/overview-dashboard.md docs/architecture/tenet-v1-overview-ui.md docs/plans/2026-04-17-application-detail-overview-route.md
git commit -m "Give the applications page its own overview route"
```

---

## Review checklist

- [ ] application detail has a dedicated catalog builder/type
- [ ] `/v1/overview/applications/{deploymentName}` is wired in router + `tenet-api`
- [ ] the embedded `?page=application` page uses the new route
- [ ] deeper resource/workspace routes stay on deployment detail
- [ ] README, overview docs, and smoke-local mention the new route
- [ ] `go test ./...` passes
- [ ] `make smoke-local` passes
- [ ] Docker verification passes
