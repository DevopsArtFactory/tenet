# Tenet

Tenet is a repository-driven deployment control plane for multi-account
environments. It gives operators one place to compare desired deployment intent,
runtime status, rollout progress, and resource-level evidence for EC2 and ECS
targets.

Tenet is early-stage and intentionally conservative. The current product is
mostly read-only: it focuses on catalog loading, runtime read models, immutable
plan/apply records, auditability, and an embedded operator UI before live AWS
mutation is enabled.

## Product Overview

Tenet helps operators:

- load desired state from a local repository checkout or a GitHub-backed
  manifest tree
- compile account, environment, deployment, artifact, and rollout intent into a
  validated catalog
- join desired state with runtime status, audit, plan/apply, recent-log, and
  resource-inspection read models
- inspect applications, accounts, deployments, rollout workspaces, and resources
  from a single embedded UI
- troubleshoot front-door auth, deployment access, and runtime evidence with
  explicit diagnostics

The primary UI flow is intentionally simple:

1. Applications overview
2. Application detail
3. Resource detail

Additional operator workspaces are linked from that flow: account detail, drift,
plan/apply, audit timeline, deployment events, recent logs, and live inspection.
The plan/apply workspace keeps disabled mutation controls explanatory: `Create Plan`
is available for the currently reviewed deployment, while `Apply Plan` and
record refreshes activate after persisted plan/apply records are loaded. If
`POST /v1/applies/plans/{planId}` returns `409 Conflict` because the reviewed
plan is stale, the workspace keeps the bookmarkable deployment/plan context
visible and tells the operator to create a fresh plan preview before retrying.

## Architecture

Tenet currently has four main layers.

### Desired-State Loading

`tenet-api` and `tenet-controller` read the same repository-shaped manifest tree.
Set `TENET_CATALOG_SOURCE=filesystem` with `TENET_REPO_ROOT` for local
checkouts, or `TENET_CATALOG_SOURCE=github` with `TENET_CATALOG_GITHUB_*` for
read-only GitHub catalog loading.

### Catalog Compilation

The loader validates manifests into a compiled catalog covering accounts,
environments, placements, deployments, rollout strategy, and desired artifacts.
The compiled catalog is the stable desired-state model behind `/v1/catalog/*`
and the overview read models.

### Runtime State

Runtime status is stored separately from desired configuration. Local runs can use
filesystem-backed status, plan, apply, and audit stores; Compose can also use the
Redis-backed local stack. Runtime state represents sync, health, rollout
progress, bounded history, audit entries, and immutable plan/apply records.

### API And Embedded UI

The Go API joins catalog and runtime state into operator read models. The
embedded UI is static HTML/CSS/JS served by `tenet-api`, so local development and
Compose smoke checks do not require a separate frontend build.

## Run Locally

Run `tenet-api` against the example repository:

```bash
TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Open:

```text
http://localhost:18083/
```

For a GitHub-backed desired-state catalog:

```bash
TENET_CATALOG_SOURCE=github \
TENET_CATALOG_GITHUB_REPOSITORY=acme/platform-config \
TENET_CATALOG_GITHUB_REF=main \
TENET_CATALOG_GITHUB_PATH=tenet \
TENET_CATALOG_GITHUB_TOKEN=*** \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Useful local probes:

```bash
curl -fsS http://localhost:18083/v1/catalog/summary
curl -fsS http://localhost:18083/v1/overview/applications
curl -fsS http://localhost:18083/v1/operator/context
```

## Run With Compose

Start the local stack:

```bash
make docker-up
```

Then open:

```text
http://localhost:8080/
```

Stop it with:

```bash
make docker-down
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already
owned by something outside this Compose project. Use:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
make docker-down
docker compose up --build -d
```

The local-safe apply executor remains opt-in:

```bash
TENET_AWS_ENABLE_APPLY=true make docker-up
```

That mode exercises the queued apply handoff locally; live AWS mutation is still
not implemented for this backend slice.

## Operator UI Entry Points

Start at `/` for the applications overview. The overview keeps the first page
compact with application rows, status chips, filters, a count-aware `Clear N filters`
reset action when filter state is active, `Open application`, a self-describing
`More actions for {deploymentName}` disclosure with an accessible secondary-action
group label, and an `Applications JSON` handoff to
`GET /v1/overview/applications`. Each row keeps a
compact `Last update` field from the joined runtime summary, falling back to
`Latest runtime status` when no timestamp is present. The secondary account
drill-down keeps its `Refresh account detail` action in the page toolbar so
operators can refresh account-scoped rows from the same place they refresh
application detail pages.

Common bookmarkable routes:

- `/?page=application&deployment={deploymentName}`
- `/?page=account&accountId={awsAccountId}`
- `/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}` (canonical); older `resourceName={name}` bookmarks are still accepted as a compatibility fallback
- `/?page=drift&deployment={deploymentName}`; optional `driftQuery`, `driftSeverity=high|medium|low`, and `driftSort=severity` preserve field-level drift filters in shareable URLs
- `/?page=plan&deployment={deploymentName}`
- `/?page=audit`

Primary operator APIs:

- `GET /v1/overview/applications`
- `GET /v1/overview/deployments/{deploymentName}`
- `GET /v1/overview/deployments/{deploymentName}/events`
- `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}]`
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection`
  (bounded ECS task / ASG instance samples expose `resultScope.returnedItemCount`
  and compact alias `resultScope.returnedCount`, plus `truncationStatus` and
  compact boolean alias `truncated`)

Resource drill-down Recent logs and Live inspection panels label returned
`lookupContext` as `Provider lookup context` so operators can confirm the
region/account/observer-role path before troubleshooting unavailable AWS reads.
- `POST /v1/plans/deployments/{deploymentName}`
- `POST /v1/applies/plans/{planId}` (`409 Conflict` means the reviewed plan
  preview is stale and must be recreated before applying)
- `GET /v1/audit-logs`
  (bounded audit reads expose `resultScope.returnedEntryCount` and compact alias
  `resultScope.returnedCount`, plus `truncationStatus` and compact boolean alias
  `truncated`)
- `GET /v1/operator/context`

The bookmarkable audit timeline (`/?page=audit`) keeps a manual `Refresh audit
log` action and an accessible `Last refreshed` status region so screen-reader
and keyboard users get the same freshness signal as visual operators.

Deployment runtime events are read from the controller-retained history window, currently 10 events per deployment. Successful `GET /v1/overview/deployments/{deploymentName}/events` responses keep `events: []` for repo-known deployments before history exists, with zero-count `managedDriftSummary` plus zero retained-event counts in `retention` / `resultScope` (`retention.limit` may still be non-zero, e.g. `10`). The application detail Controller runtime events panel renders the same backend metadata, including total returned/retained counts, retention limit/status, and query-window status, so operators can distinguish an empty retained history from a query-limited response or a controller-retention cap. The `resultScope` fields distinguish `?limit=` query truncation (`limited`, `truncationStatus`, compact alias `truncated`) from the retained controller-history cap (`retentionLimit`, `retainedEventCount`, `retentionStatus`), and expose `returnedCount` as a compact alias for `returnedEventCount`.

Trusted front-door auth is additive and opt-in. Only enable the auth/RBAC
environment toggles when every request reaches Tenet through a front door that
authenticates the caller and strips spoofable trusted headers before forwarding
trusted values.

## Validation

Fast local checks:

```bash
go test ./internal/api
go test ./cmd/tenet-api
make smoke-local
```

Containerized local-stack checks:

```bash
make smoke-docker
```

General repository checks:

```bash
go test ./...
```

## Documentation Map

- `docs/architecture/tenet-v1-operator-contracts.md` preserves the detailed
  endpoint, read-model, auth, resource, and smoke-verification contracts formerly
  kept in this README.
- `docs/architecture/tenet-v1-overview.md` describes the v1 control-plane
  direction and desired-state/runtime model.
- `docs/architecture/tenet-v1-local-runtime.md` documents local process,
  Compose, auth/RBAC, smoke, and runtime-store behavior.
- `docs/architecture/tenet-v1-overview-ui.md` documents embedded UI navigation,
  route shapes, and operator-workspace behavior.
- `docs/architecture/tenet-v1-audit-log-api.md` documents immutable audit,
  plan/apply, actor, and authorization contracts.
- `docs/architecture/tenet-v1-enterprise-sso.md` documents managed login,
  Google Workspace OIDC, Okta OIDC, Okta SAML diagnostics, and SSO boundaries.
- `docs/features/overview-dashboard.md` documents the applications overview,
  application/account/resource pages, drift, audit, and plan/apply workspaces.
- `docs/features/resource-recent-logs.md` documents recent-log and live
  inspection resource contracts.
- `docs/runbooks/ci-smoke-runner-start.md` documents GitHub Actions runner-start
  failure triage for CI smoke checks.

## Repository Layout

- `cmd/tenet-api` - API server and embedded UI entry point
- `cmd/tenet-controller` - controller/runtime loop entry point
- `internal/api` - routes, read models, auth enforcement, and UI assets
- `internal/catalog` - manifest loading and catalog compilation
- `internal/runtime` - runtime status, plan/apply, and audit stores
- `examples/local-repo` - sample desired-state repository
- `examples/local-status` - sample runtime status fixtures
- `scripts` - smoke, Compose, and CI helper scripts
- `docs` - architecture, feature, plan, and runbook documentation

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
