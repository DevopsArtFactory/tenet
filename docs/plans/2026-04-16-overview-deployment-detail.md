# Overview Deployment Detail Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a joined deployment-detail overview API and wire the embedded account-oriented UI to reveal richer per-deployment detail without leaving `/`.

**Architecture:** Build a small `catalog` join helper that combines compiled deployment intent, placement/account metadata, and the current status snapshot for one deployment. Expose that joined view at `GET /v1/overview/deployments/{deploymentName}` and let the embedded UI fetch it on demand for an expandable detail panel, keeping the overview list fast while providing richer rollout/sync/condition context.

**Tech Stack:** Go 1.25, standard library `net/http`, embedded HTML/CSS/JS UI, existing manifest/catalog/statusstore packages.

---

### Task 1: Add a joined deployment-detail builder in `catalog`

**Objective:** Create a reusable detail view that returns one deployment with placement/account metadata plus current runtime status.

**Files:**
- Modify: `internal/catalog/overview.go`
- Test: `internal/catalog/overview_test.go`

**Step 1: Write failing test**

Add a test that calls a new helper with a catalog containing one deployment and one matching snapshot, then asserts the returned detail includes:
- deployment name, app/environment refs, region/account metadata
- target type, strategy, execution mode
- desired and deployed artifact
- observed generation
- sync reason and health summary
- rollout summary and conditions

Also add a not-found test that expects `statusstore.ErrNotFound` when the deployment does not exist in the compiled catalog.

**Step 2: Run test to verify failure**

Run: `go test ./internal/catalog -run 'TestBuildDeploymentOverviewDetail|TestBuildDeploymentOverviewDetailReturnsNotFound' -v`
Expected: FAIL because the detail helper/type does not exist yet.

**Step 3: Write minimal implementation**

In `internal/catalog/overview.go`:
- add a `DeploymentOverviewDetail` type
- add `BuildDeploymentOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, deploymentName string) (DeploymentOverviewDetail, error)`
- reuse the existing placement/status join logic patterns from `BuildAccountOverviews`
- clone rollout summary and conditions defensively, matching the overview behavior
- return `statusstore.ErrNotFound` when the deployment is missing from catalog or placement resolution cannot produce a matching deployment record

**Step 4: Run test to verify pass**

Run: `go test ./internal/catalog -run 'TestBuildDeploymentOverviewDetail|TestBuildDeploymentOverviewDetailReturnsNotFound' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/overview_test.go
git commit -m "Add joined deployment detail views for the overview API"
```

### Task 2: Expose the deployment-detail overview API

**Objective:** Add a read-only endpoint for joined deployment detail data.

**Files:**
- Modify: `internal/api/router.go`
- Modify: `cmd/tenet-api/main.go`
- Test: `internal/api/router_test.go`

**Step 1: Write failing test**

Add router tests for:
- `GET /v1/overview/deployments/api-prod` returns 200 with the joined detail payload
- missing `OverviewDeployment` option returns 503
- provider `statusstore.ErrNotFound` returns 404
- invalid names containing `/` return 400

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestRouterOverviewDeployment' -v`
Expected: FAIL because the option/route do not exist yet.

**Step 3: Write minimal implementation**

In `internal/api/router.go`:
- add `OverviewDeployment func(string) (catalog.DeploymentOverviewDetail, error)` to `Options`
- add `GET /v1/overview/deployments/{deploymentName}`
- follow the existing method-guard and not-found patterns used by `/v1/status/deployments/{deploymentName}`

In `cmd/tenet-api/main.go`:
- when both repo and store are configured, wire `options.OverviewDeployment`
- load the compiled repo and current snapshots, then call `catalog.BuildDeploymentOverviewDetail`

**Step 4: Run test to verify pass**

Run: `go test ./internal/api -run 'TestRouterOverviewDeployment' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go cmd/tenet-api/main.go
git commit -m "Expose joined deployment detail overview endpoint"
```

### Task 3: Add expandable deployment detail panels to the embedded UI and docs

**Objective:** Let operators inspect richer deployment detail directly from `/` and document the new overview endpoint.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing test**

Because the UI is embedded static HTML, add/adjust an API/router-level assertion proving the new route is discoverable from the running control plane, then add a smoke-level verification step for the UI behavior.

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestRouterOverviewDeployment' -v`
Expected: PASS for the API once Task 2 is done; UI behavior itself will be verified with the repository smoke/runtime checks below.

**Step 3: Write minimal implementation**

In `internal/api/web/index.html`:
- make each deployment row expandable/clickable
- lazily fetch `/v1/overview/deployments/{name}` when expanding a row
- render a safe detail panel using DOM APIs only (`createElement`, `textContent`, `replaceChildren`)
- include sync reason, account/region placement, rollout generations, conditions, and full rollout-step list
- show loading/error states per expanded row

In docs:
- add the new endpoint to `README.md`
- describe the expandable detail panel and joined deployment-detail API in `docs/architecture/tenet-v1-overview-ui.md`

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
git add internal/api/web/index.html README.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Surface deployment detail panels in the overview UI"
```
