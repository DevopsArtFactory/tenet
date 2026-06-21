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
`GET /v1/overview/applications`, plus a `Copy read route` control for that
same method/path. Each row keeps a
compact `Last update` field from the joined runtime summary, falling back to
`Latest runtime status` when no timestamp is present. The secondary account
drill-down keeps its `Refresh account detail` action in the page toolbar so
operators can refresh account-scoped rows from the same place they refresh
application detail pages. Application detail pages show the exact backing read route,
`Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search
for grouped linked resources with an inline `Clear resource filter` action whenever
that filter is active, while rollout/history/log evidence stays on the final resource
page. The drift and plan workspaces show `Current workspace read: GET /v1/overview/deployments/{deploymentName}.`
in their chrome and include a top-level `Workspace JSON` handoff to that
deployment-detail read model.

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
- `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`
  (recent-log responses expose `resultScope.limit`, `resultScope.returned`,
  `resultScope.total`, and `resultScope.truncated`; ECS task container, ASG
  instance, and ECS cluster selectors keep focused troubleshooting reads explicit)
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`
  (the route requires a concrete resource type/name; supported ECS service and
  ASG reads accept optional bounded selectors/caps, and bounded ECS task / ASG
  instance samples expose `resultScope.returnedItemCount` and compact alias
  `resultScope.returnedCount`, plus `truncationStatus` and compact boolean alias
  `truncated`)

Resource drill-down Recent logs and Live inspection panels label returned `lookupContext` as `Provider lookup context`; Recent logs renders that path in a compact card so operators can confirm the region/account/observer-role path before troubleshooting unavailable AWS reads.
In the embedded resource page, log and inspection caps stay bookmarkable as `resourceLogLimit=` and `resourceInspectionLimit=` UI state; positive caps are sent only to the matching lazy `resource-logs` or `resource-inspection` read as backend `limit=` query parameters.
Capture the HTTP `Request-Id` response header with that lookup context when escalating a failed or unavailable Recent logs or Live inspection read; the embedded Recent logs and Live inspection cards label the same value as `Tenet request ID` metadata so reviewers can match the payload to Tenet's structured request logs.
ECS service Live inspection can include optional deployment-configuration evidence from `DescribeServices`: `deploymentControllerType`, `deploymentCircuitBreaker`, and `deploymentAlarmConfiguration`, rendered only when AWS returns those fields.
Auto Scaling Group Live inspection can also render optional
`autoScalingGroup.scalingActivities[]` evidence as `Recent scaling activities`
with `Activity ID`, `Status code`, `Progress`, `Start time`, `End time`,
`Cause`, `Description`, and `Status message` labels, and each populated row
includes a same-tab `Share scaling activity` anchor for handing off the exact
bounded activity evidence; an explicit empty array
renders `No scaling activities are reported in the bounded recent activity window for this Auto Scaling Group.`
Those rows come from a bounded read-only `DescribeScalingActivities` lookup
after the ASG itself is resolved, and local/no-AWS runtimes keep the existing
unavailable or summary-only behavior instead of requiring AWS access.
For CodeDeploy ECS traffic-routing resources, local and Compose runs do not need
live AWS access to keep the drill-down usable: `codeDeployDeploymentGroup`,
`productionListener`, and `testListener` can return
`available=true`, `source=rollout_execution_summary`, and a summary-only
`trafficRoutingResource` body derived from rollout execution metadata. When AWS
inspection is configured and succeeds, the same identifiers may instead return
`source=live` / `inspectionMode=live`; the resource-detail Live inspection
panel renders a compact `Traffic-routing status` block with operator-facing
`Resource`, `Inspection mode`, `Source`, `Summary`, and — only when the live AWS
payload includes target-group health counts — a target-health status pill,
next-action cue, and raw `Target group health` rows before the detailed
deployment-group status cues such as CodeDeploy application, deployment group,
deployment config, deployment style, ECS service bindings, and the latest
CodeDeploy deployment summary and target groups. When the live payload
includes `trafficRoutingResource.codeDeployDeploymentGroup.latestDeployment` or
`latestCodeDeployDeployment`, that same resource-detail panel also shows a
scannable latest-deployment status pill, an operator next-action cue, the
deployment id/status/create time/complete time/status message, and any additive
rollback/error/alarm/deployment-overview diagnostics from CodeDeploy
`GetDeployment` when the extra deployment lookup succeeds, without moving that
noise onto the applications overview. If only the deployment-group summary is
available, Tenet preserves the existing summary-only payload shape. When live
target-group/listener rows
match exactly one normalized rollout resource, they add `_self` `Open resource
detail` handoffs back into the bookmarkable resource drill-down; unmatched or
incomplete identifiers remain plain text so operators never get broken self-
links. Local no-AWS summary fallback stays
explicitly summary-only and does not invent latest CodeDeploy deployment status
or target-group health counts. Local smoke guards ensure traffic-routing fallback
routes do not regress to `aws_unavailable`.

- `POST /v1/plans/deployments/{deploymentName}`
- `POST /v1/applies/plans/{planId}` (`409 Conflict` means the reviewed plan
  preview is stale and must be recreated before applying)
- `GET /v1/audit-logs`
  (bounded and empty authorized audit reads expose `resultScope.returnedEntryCount`
  and compact alias `resultScope.returnedCount`, plus `truncationStatus` and
  compact boolean alias `truncated`)
- `GET /v1/operator/context`

The bookmarkable audit timeline (`/?page=audit`) keeps a manual `Refresh audit
log` action and an accessible `Last refreshed` status region so screen-reader
and keyboard users get the same freshness signal as visual operators. When the
API reports a capped audit window through `resultScope`, the timeline also shows
inline actions to expand to the next preset limit or clear the cap while
preserving the current deployment/action and applications-shell query state.

The bookmarkable resource page (`/?page=resource...`) keeps the top-level deployment-detail read visible as `Current resource read: GET /v1/overview/deployments/{deploymentName}` and exposes `Resource JSON` / `Copy read route` handoffs to that read model. The same page now adds a compact `Troubleshooting summary` derived from the selected deployment/resource plus any selected lookup scope (`containerName`, `clusterArn`, or `instanceId`) and the latest lazy Recent logs and Live inspection panel states, including empty-log and unavailable-provider next-step guidance.

Deployment runtime events are read from the controller-retained history window, currently 10 events per deployment. Successful `GET /v1/overview/deployments/{deploymentName}/events` responses keep `events: []` for repo-known deployments before history exists, with zero-count `managedDriftSummary` plus zero retained-event counts in `retention` / `resultScope` (`retention.limit` may still be non-zero, e.g. `10`). The application detail Controller runtime events panel renders the same backend metadata, including total returned/retained counts, retention limit/status, and query-window status, so operators can distinguish an empty retained history from a query-limited response or a controller-retention cap. The `resultScope` fields distinguish `?limit=` query truncation (`limited`, `truncationStatus`, compact alias `truncated`) from the retained controller-history cap (`retentionLimit`, `retainedEventCount`, `retentionStatus`), report exact-cap bounded reads as `truncationStatus: "unknown"` with `truncated: false`, and expose `returnedCount` as a compact alias for `returnedEventCount`.

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

CI Smoke mirrors those checks in the hosted fast lane and also syntax-checks the operator smoke helpers before running local smoke. For smoke-script or runner-start evidence changes, run:

```bash
bash -n scripts/smoke-local.sh
bash -n scripts/smoke-docker.sh
bash -n scripts/ci-smoke-runner-evidence.sh
```

The Docker smoke lane remains separate; use `make smoke-docker` for Compose, container startup, fixture, or Docker-stack runtime changes. In hosted PRs it is path-scoped: docs-only changes normally stay on the fast lane, while changes under Docker/Compose, API/controller, local fixtures, runtime read models, resource evidence, webhook, plan/apply, or audit smoke surfaces run the full Docker stack before merge.

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
- `docs/features/overview-dashboard.md` documents application/account/resource pages,
  drift, audit, and plan/apply workspaces.
- `docs/features/resource-recent-logs.md` documents recent-log resource contracts.
- `docs/features/resource-inspection.md` documents live resource inspection selectors, lookup context, and bounded sample metadata.
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
