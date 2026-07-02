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
- troubleshoot front-door auth, deployment access, desired-state refresh
  readiness, and runtime evidence with explicit diagnostics

The primary UI flow is intentionally simple:

1. Applications overview
2. Application detail
3. Resource detail

Additional operator workspaces are linked from that flow: account detail, drift,
plan/apply, audit timeline, deployment events, recent logs, and live inspection.
Resource-detail quick navigation keeps `_self` links back to the Applications
overview and parent Application detail page before operators pivot into focused
log or inspection evidence.
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
curl -fsS http://localhost:18083/v1/status/deployments
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

Optional managed-login provider modes are non-secret values: use
`TENET_AUTH_PROVIDER=github`, `TENET_AUTH_PROVIDER=google_oidc`,
`TENET_AUTH_PROVIDER=okta_oidc`, or diagnostic-only
`TENET_AUTH_PROVIDER=okta_saml`. Keep credential-bearing settings such as OAuth
client secrets, SAML private material, session secrets, and tokens redacted.

## Operator UI Entry Points

Start at `/` for the applications overview. The overview keeps the first page
compact with application rows, status chips, filters, a count-aware `Clear N filters`
reset action when filter state is active, `Open application`, a self-describing
`More actions for {deploymentName}` disclosure with an accessible secondary-action
group label, and `Applications JSON` / `Copy read route` handoffs for
`GET /v1/overview/applications`. Each row keeps a compact `Last update` field
from the joined runtime summary, falling back to `Latest runtime status` when no timestamp is present. The secondary account
drill-down keeps its toolbar `Refresh account detail` action; account/group JSON handoffs include adjacent `Copy account read route`
/ `Copy group read route` controls for exact scoped `GET /v1/overview/accounts/...`
handoffs. Expandable rows expose their controlled lazy-loaded detail panel with
`aria-controls`. Application detail pages show the exact backing read route,
`Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search
for grouped linked resources. When that filter is active, each resource section shows
visible/total impact and section-specific empty guidance alongside the inline
`Clear resource filter` action, while rollout/history/log evidence stays on the final
resource page. The drift and plan workspaces show `Current workspace read: GET /v1/overview/deployments/{deploymentName}.`
in their chrome and include a top-level `Workspace JSON` handoff to that
deployment-detail read model. Non-auth read failures render the active page label, failed `GET ...` route, optional `Tenet request ID`, and same-URL `Retry current page` action instead of a generic overview-only error.

Common bookmarkable routes:

- `/?page=application&deployment={deploymentName}`
- `/?page=account&accountId={awsAccountId}`
- `/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}` (canonical); older `resourceName={name}` bookmarks are still accepted as a compatibility fallback
- `/?page=drift&deployment={deploymentName}`; optional `driftQuery`, `driftSeverity=high|medium|low`, and `driftSort=severity` preserve field-level drift filters in shareable URLs, and no-row / filtered-no-match drift table messages are announced as polite status updates for assistive technology
- `/?page=plan&deployment={deploymentName}`
- `/?page=audit`
Primary operator APIs:
- `GET /v1/overview/applications`
- `GET /v1/overview/deployments/{deploymentName}`
- `GET /v1/status/deployments` and `GET /v1/status/deployments/{deploymentName}` for raw persisted runtime snapshots
- `GET /v1/overview/deployments/{deploymentName}/events` (or `GET /v1/overview/deployments/{deploymentName}/events?source={source}&limit={positiveInteger}` for exact source-filtered, bounded controller-history reads)
- `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`
  (recent-log responses expose `resultScope.requestedLimit`, `effectiveLimit`,
  `returnedEntryCount`, `returnedCount`, `truncationStatus`, and `truncated`;
  compact aliases such as `limit`/`returned` keep bounded reads terse, while ECS
  task container, ASG instance, and ECS cluster selectors keep focused reads explicit)
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`
  (the route requires a concrete resource type/name; supported ECS, ASG, and
  traffic-routing reads can use focused selectors/caps documented in
  `docs/features/resource-inspection.md`; live ASG reads may add optional
  `ec2InstanceStatus` status-check evidence beside sampled/selected EC2 host
  details when AWS `DescribeInstanceStatus` is available)

Resource drill-down Recent logs and Live inspection panels summarize lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract. Use
`docs/features/resource-recent-logs.md` and
`docs/features/resource-inspection.md` for the exact route selectors, response
fields, operator labels, and smoke-test expectations; use
`docs/architecture/tenet-v1-overview-ui.md` for the embedded UI behavior.
Local/no-AWS unavailable payloads and successful empty reads stay distinct:
operators see friendly next-step copy while scripts keep stable fields such as
`available`, `unavailableReason`, `emptyReason`, `lookupContext`, and
`resultScope`. The local and Docker smoke scripts keep a compact no-AWS resource-evidence matrix for ECS service/task, ASG/bootstrap, traffic-routing no-log-source, selector validation, successful-empty, and guarded-auth paths.
The embedded UI release guard keeps lazy Recent logs and Live inspection panels
aligned so request routes, request IDs, lookup context, and bounded-scope metadata
stay visible even when AWS evidence is unavailable.
Use `Share ECS service event` from a Live inspection event row to hand off one ECS service event without changing the resource-page selector state.
In the embedded resource page, log and inspection caps stay bookmarkable as `resourceLogLimit=` and `resourceInspectionLimit=` UI state; positive caps are sent only to the matching lazy `resource-logs` or `resource-inspection` read as backend `limit=` query parameters.
Recent logs and Live inspection lazy-panel state messages such as loading, empty, unavailable, and non-auth fetch failures are polite atomic status regions, so assistive technology announces focused panel state changes without making the full evidence payload live.
Capture the HTTP `Request-Id` response header with the provider lookup context
when escalating a failed or unavailable Recent logs or Live inspection read; the
embedded cards surface the same request-correlation token as metadata so
reviewers can match screenshots or copied JSON to Tenet's structured request
logs.

- `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` (`409 Conflict` means the reviewed plan preview is stale)
- `POST /v1/controller/refresh` to request the configured desired-state refresh handoff manually (`filesystem_noop` for filesystem catalogs, `202 Accepted` with `controllerRefresh` evidence when GitHub+Redis handoff is available). Accepted manual refresh requests are also projected through `GET /v1/audit-logs?action=controller.refresh.requested` with actor metadata plus safe route/status/result/request ID/delivery ID/channel or no-op evidence, and `GET /v1/controller/refresh/status` returns the latest receipt plus explicit `no_history` / `unavailable` states for read-after-write troubleshooting; from deployment-scoped application/resource/drift/plan pages, the embedded Operator context keeps a non-noisy `Last refresh requested` cue and links operators to `/?page=audit&deployment={deploymentName}&action=controller.refresh.requested` without dropping the current bookmarkable route. The embedded Operator context details expose a DOM-built `Refresh controller` control only when `GET /v1/operator/context.desiredStateSource.controllerRefresh` reports an available handoff; the control preserves the current bookmarkable URL, disables while the POST is in flight, reports success/no-op/unavailable failures in the banner live status, reuses the shared operator-write auth guidance for `401` / `403` responses, and after an accepted processed handoff re-runs the currently active embedded read model once in place without rewriting `window.location.search` or `window.location.hash`, hiding the banner announcement, or starting polling.
- `GET /v1/audit-logs` (bounded reads expose `resultScope.returnedEntryCount`, `resultScope.returnedCount`, `truncationStatus`, and `truncated`; exact `action` filters include `plan.created`, `apply.requested`, `apply.transitioned`, and `controller.refresh.requested`)
- `GET /v1/operator/context`

The bookmarkable audit timeline (`/?page=audit`) keeps a manual `Refresh audit
log`, an accessible `Last refreshed` cue, and clearable deployment/action/limit
chips that remove only their own control while preserving the shell path,
`q`/`sync`/`account`, and stripping stale `#audit-*` anchors. Capped audit
windows from `resultScope` still expose inline expand/clear actions that keep the
current deployment/action and applications-shell query state.

The bookmarkable resource page (`/?page=resource...`) keeps the top-level deployment-detail read visible in the selected-resource body callout as `Current resource read: GET /v1/overview/deployments/{deploymentName}` and exposes `Resource JSON` / `Copy read route` toolbar handoffs to that same read model. The lazy Recent logs and Live inspection panels also expose `Copy recent logs route` / `Copy live inspection route` controls next to their refresh and limit controls, so operators can copy the exact `resource-logs` or `resource-inspection` request (including the selected bounded read limit) without opening raw JSON; their focused state messages are polite atomic status regions for assistive technology. Resource-detail rendered sections also include low-noise same-page `Section link` and `Copy section link` troubleshooting handoffs that preserve the current pathname and query state, including `containerName`, `instanceId`, `resourceLogLimit`, and `resourceInspectionLimit`, while changing only the hash. The same page now adds a compact `Troubleshooting summary` derived from the selected deployment/resource plus any selected lookup scope (`containerName`, `clusterArn`, or `instanceId`) and the latest lazy Recent logs and Live inspection panel states, including an Evidence freshness row for completed reads plus empty-log and unavailable-provider next-step guidance.

Deployment runtime events are read from the controller-retained history window, currently 10 events per deployment; this controller-history timeline is not the immutable audit log. Successful `GET /v1/overview/deployments/{deploymentName}/events` responses keep `events: []` for repo-known deployments before history exists, with zero-count `managedDriftSummary`, an additive `sourceSummary` array of `{source, count}` entries computed from the retained window before optional source filtering and before positive limit truncation (blank persisted sources are bucketed as `unknown`, and `?source=unknown` selects those legacy entries), plus zero retained-event counts in `retention` / `resultScope` (`retention.limit` may still be non-zero, e.g. `10`). The application detail Controller runtime events panel renders the same backend metadata, including total returned/retained counts, retention limit/status, query-window status, exact source filter when selected, and source-summary chips such as `Source mix · controller · 7 events` whenever `sourceSummary` has entries, so operators can distinguish an empty retained history from a source-filtered empty window, query-limited response, or controller-retention cap without opening raw JSON. The panel omits those chips when `sourceSummary` is empty, keeps `eventLimit` and optional `eventSource` in the application URL, fetches `GET /v1/overview/deployments/{deploymentName}/events?limit={n}&source={source}` for non-all sources, and preserves both controls in per-event share links; the selector offers `All sources`, `controller`, `deployment-controller`, and `github`, while exact bookmarked custom source values remain visible instead of being discarded. Runtime-event loading, empty, unavailable, and fetch-error states are polite atomic status regions so assistive technology announces focused panel changes without making the full event timeline live. The `resultScope` fields distinguish optional exact `?source=` filtering (reported as `sourceFilter` and applied before any limit), `?limit=` query truncation (`limited`, `truncationStatus`, compact alias `truncated`), and the retained controller-history cap (`retentionLimit`, `retainedEventCount`, `retentionStatus`), report exact-cap bounded reads as `truncationStatus: "unknown"` with `truncated: false`, and expose `returnedCount` as a compact alias for `returnedEventCount`. Invalid source values return `400 Bad Request` with `invalid source query`; direct-process smoke covers that validation while Compose smoke keeps the bounded `?limit=1` runtime-events `resultScope` and `sourceSummary` contract exercised.

Trusted front-door auth is additive and opt-in. Only enable the auth/RBAC
environment toggles when every request reaches Tenet through a front door that
authenticates the caller and strips spoofable trusted headers before forwarding
trusted values. The auth/RBAC toggles are boolean, non-secret settings, so use
operator-readable examples such as `TENET_API_TRUST_AUTHENTICATED_HEADERS=true`,
`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`,
`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`, and
`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`. Filesystem catalog
sources may additionally opt into repository-backed RBAC grants with
`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true`; when enabled, the
existing guarded read/write routes keep the same identity requirements but use
`rbac/policy.json`, `rbac/policy.yaml`, or `rbac/policy.yml` grants evaluated
against trusted identity plus `X-Tenet-Authenticated-Groups` instead of trusted
role/deployment grant headers. The embedded Operator context banner adds a
policy-mode cue in that mode: compact source/availability status stays above the
toolbar, while expanded details show matched user/group grants and derived
roles, deployment grants, and project/environment scopes for denied-read
troubleshooting. Leave it unset to preserve the default trusted-header
authorization path. Keep credential-bearing
settings such as tokens redacted.

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

CI Smoke mirrors those checks in the hosted fast lane and also syntax-checks the operator smoke helpers before running local smoke. Hosted fast-lane order is `go test ./...`, smoke helper syntax checks, then `make smoke-local`. Smoke helper scripts must keep their executable bits because Makefile targets and runner-start docs invoke them directly by path. They should also remove every mktemp-backed helper file or directory during cleanup so repeated smoke runs do not leave stale request/response payloads in `/tmp`. For smoke-script or runner-start evidence changes, run:

```bash
bash -n scripts/smoke-local.sh
bash -n scripts/smoke-docker.sh
bash -n scripts/ci-smoke-runner-evidence.sh
```

The Docker smoke lane remains separate; use `make smoke-docker` for Compose, container startup, fixture, or Docker-stack runtime changes. In hosted PRs it is path-scoped: docs-only changes normally stay on the fast lane, while changes under Docker/Compose, front-door auth, API/controller, local fixtures, runtime read models, RBAC policy diagnostics, resource evidence, webhook, plan/apply, or audit smoke surfaces run the full Docker stack before merge.

For hosted CI Smoke failures that never start a runner, collect pasteable issue
evidence with:

```bash
scripts/ci-smoke-runner-evidence.sh --summary <run-id>
```

The detailed runner-start evidence bundle remains available by omitting
`--summary`; see `docs/runbooks/ci-smoke-runner-start.md`.

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
- `docs/architecture/tenet-v1-aws-central-control-plane.md` documents the AWS
  Terraform bootstrap, public repo-sync, and `repo_sync_token_secret_arn`.
- `docs/architecture/tenet-v1-enterprise-sso.md` documents managed login,
  Google Workspace OIDC, Okta OIDC, Okta SAML diagnostics, and SSO boundaries.
- `docs/features/managed-login.md` documents managed-login activation, provider routes, cookie-security diagnostics, logout boundaries, and trusted-front-door/policy relationships.
- `docs/features/overview-dashboard.md` documents application/account/resource pages,
  drift, audit, and plan/apply workspaces.
- `docs/features/resource-recent-logs.md` documents recent-log resource contracts, including lookup context, selector echoes, and friendly empty/unavailable states.
- `docs/features/resource-inspection.md` documents the focused live resource inspection route, selectors, lookup context, unavailable states, and detailed field contracts.
- `docs/runbooks/operator-troubleshooting.md` documents daily auth, refresh, audit, and drill-down troubleshooting for operators.
- `docs/runbooks/ci-smoke-runner-start.md` documents GitHub Actions runner-start
  failure triage for CI smoke checks.

## Repository Layout

- `cmd/tenet-api` - API server and embedded UI entry point
- `cmd/tenet-controller` - controller/runtime loop entry point
- `internal/api` - routes, read models, auth enforcement, and UI assets
- `internal/catalog` - manifest loading and catalog compilation
- `internal/statusstore`, `internal/planpreview`, `internal/applyrequest`, and
  `internal/auditlog` - runtime status, plan/apply, and audit stores
- `examples/local-repo` - sample desired-state repository
- `examples/local-status` - sample runtime status fixtures
- `scripts` - smoke, Compose, and CI helper scripts
- `docs` - architecture, feature, plan, and runbook documentation

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
