# Account Overview Detail API Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a joined account-detail overview endpoint and matching UI/docs updates so operators can inspect one account's overview payload directly from the account-oriented homepage.

**Architecture:** Extend the existing overview read model with an account-detail shape derived from the same account/deployment join logic already used by `/v1/overview/accounts`. Add a focused router endpoint at `/v1/overview/accounts/{awsAccountId}` and expose a direct `Account JSON` link in the embedded UI account header, keeping all rendering safe and preserving the existing account/deployment summary semantics.

**Tech Stack:** Go stdlib HTTP/router/json, existing `internal/catalog` overview builders/providers, embedded HTML/vanilla JS UI, Go tests, Node-backed UI harness where needed.

---

### Task 1: Add account-detail overview read model and builder

**Objective:** Create a dedicated account-detail shape and builder/provider logic keyed by AWS account ID.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/catalog/provider.go`
- Modify: `internal/catalog/overview_test.go`

**Step 1: Write failing tests**

Add tests that assert:
- `BuildAccountOverviewDetail` returns the selected account with cloned deployments and existing counts.
- Lookup is keyed by `AWSAccountID` and returns `statusstore.ErrNotFound` for unknown accounts.
- `RepositoryAccountOverviewDetailProvider` loads the repo and returns the selected account detail.

**Step 2: Run test to verify failure**

Run: `go test ./internal/catalog -run 'TestBuildAccountOverviewDetail|TestRepositoryAccountOverviewDetailProvider' -v`
Expected: FAIL — missing builder/provider symbols.

**Step 3: Write minimal implementation**

Implement:
- `type AccountOverviewDetail struct { ... }`
- `func BuildAccountOverviewDetail(c manifest.Catalog, snapshots []statusstore.Snapshot, awsAccountID string) (AccountOverviewDetail, error)`
- `func RepositoryAccountOverviewDetailProvider(root string, snapshots func() ([]statusstore.Snapshot, error)) func(string) (AccountOverviewDetail, error)`

Reuse `BuildAccountOverviews` or shared helpers instead of duplicating account/deployment join logic.

**Step 4: Run test to verify pass**

Run: `go test ./internal/catalog -run 'TestBuildAccountOverviewDetail|TestRepositoryAccountOverviewDetailProvider' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/catalog/provider.go internal/catalog/overview_test.go
git commit -m "Add account overview detail builder"
```

### Task 2: Expose `/v1/overview/accounts/{awsAccountId}` in the API

**Objective:** Add the joined account-detail route to the read-only API and wire it from `tenet-api`.

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `cmd/tenet-api/main.go`

**Step 1: Write failing tests**

Add router tests that assert:
- `GET /v1/overview/accounts/123456789012` returns the account detail payload.
- Unknown account IDs return `404` using `statusstore.ErrNotFound`.
- Invalid nested paths return `400`.
- The route is included in method-guard coverage.

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestRouterOverviewAccountDetail|TestOverviewAccountDetailRoute' -v`
Expected: FAIL — route/options not implemented.

**Step 3: Write minimal implementation**

Add:
- `OverviewAccount func(string) (catalog.AccountOverviewDetail, error)` to `api.Options`
- Router handler for `/v1/overview/accounts/{awsAccountId}`
- Wiring in `cmd/tenet-api/main.go` that loads the repo, lists snapshots, and builds the account detail by AWS account ID

**Step 4: Run test to verify pass**

Run: `go test ./internal/api -run 'TestRouterOverviewAccountDetail|TestOverviewAccountDetailRoute' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go cmd/tenet-api/main.go
git commit -m "Expose account overview detail route"
```

### Task 3: Add account JSON quick link and refresh docs

**Objective:** Surface the new endpoint from `/` and document the joined account-detail contract.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing tests**

Add tests that assert:
- Each account header renders an `Account JSON` link to `/v1/overview/accounts/{awsAccountId}`.
- The link is a sibling control in the account header (not nested inside deployment toggle buttons).
- Docs mention the new endpoint.

**Step 2: Run test to verify failure**

Run: `go test ./internal/api -run 'TestIndexHTML|TestEmbeddedUI' -v`
Expected: FAIL — account link missing.

**Step 3: Write minimal implementation**

Update the embedded UI account header to include the link using safe DOM construction, then update README and overview UI architecture docs to list/document the new endpoint.

**Step 4: Run test to verify pass**

Run: `go test ./internal/api -run 'TestIndexHTML|TestEmbeddedUI' -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Document and link account overview detail"
```

### Task 4: Verify the full slice

**Objective:** Confirm the new account overview detail flow works across tests and local runtime checks.

**Files:**
- Verify only

**Step 1: Run focused and full verification**

Run:
- `go test ./...`
- `make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- `curl http://localhost:8080/v1/overview/accounts/210987654321`
- `curl http://localhost:8080/`
- `docker compose down --remove-orphans`

Expected: PASS, valid compose config, API responds with the new account detail JSON, UI still loads.

**Step 2: Commit final verification/docs touch-ups if needed**

```bash
git add -A
git commit -m "Polish account overview detail slice"
```
