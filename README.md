# Tenet

Tenet is a repository-driven deployment control plane for multi-account environments.

The project is aimed at giving operators one place to understand what should be running, what is actually happening now, and how a deployment is progressing across EC2 and ECS targets. The current product direction is an ArgoCD-inspired workflow: start from a simple applications overview, drill into an application, then drill further into the concrete resource that matters during rollout or troubleshooting.

Today, Tenet is still early-stage and intentionally read-only. The focus is on a strong control-plane core, clear runtime read models, and an operator-friendly embedded UI before live execution and mutation workflows are added.

## What Tenet does

At a high level, Tenet:
- reads deployment intent from a repository-shaped manifest tree
- compiles that intent into a validated internal catalog
- joins desired state with runtime status snapshots
- exposes that combined view through a small control-plane API and embedded UI
- helps operators inspect rollout state, runtime history, linked resources, and recent operational context

The current UI flow is:
- Applications overview
- Application detail
- Resource detail

This keeps the first page simple and pushes deeper operational detail into drill-down pages where it is easier to understand.

## Architecture

Tenet is currently built from four main layers.

### 1. Repository-backed desired state
Tenet expects a repo-like manifest structure with accounts, environments, and deployments. That repository is treated as the source of deployment intent.

### 2. Catalog compilation
The manifest tree is loaded and validated into a compiled catalog. This gives Tenet a stable internal model for:
- account placement
- environment targeting
- deployment strategy
- desired artifacts
- rollout intent

### 3. Runtime status layer
Tenet keeps runtime status separately from desired configuration. Runtime snapshots can be stored in the filesystem or Redis and are used to represent things like:
- sync state
- health state
- rollout progress
- conditions
- bounded runtime history

### 4. Read models and operator UI
Tenet joins desired state and runtime state into read models that power:
- the embedded browser UI
- drill-down pages for applications and resources
- operational views such as rollout context, immutable plan previews, immutable apply requests, deployment events, and recent logs

The embedded UI is intentionally lightweight: static HTML/CSS/JS served directly by the Go API. That keeps the product easy to run locally while the control-plane model is still evolving.

## How to use Tenet

### Run locally against the example repository

```bash
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
go run ./cmd/tenet-api
```

Then open:

```bash
http://localhost:8080/
```

You should land on the applications overview page.

Every HTTP response from `tenet-api` also carries a `Request-Id` header. If your client already sends `request-id`, Tenet echoes it back; otherwise it generates one. Capture that value when an API call fails and use the same ID to correlate request-scoped logs and any handler/provider work that reads `api.RequestIDFromContext(ctx)`.

`tenet-api` now emits one structured JSON log line per API/UI request with stable fields including `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`. Panic recovery logs a separate correlated panic event, keeps the process alive, and returns a generic `500` response that includes the same request ID without leaking a stack trace to the client.

Use `TENET_API_REQUEST_TIMEOUT` to bound request handling. The default is `30s`; set `TENET_API_REQUEST_TIMEOUT=0` to disable the request timeout middleware. When a request times out, Tenet returns an explicit correlated `503` response and emits the matching request log with `result=timeout`.

For the current read-only plan/apply flow, a typical local operator path is:
- `GET /v1/overview/applications/{deploymentName}` or `GET /v1/overview/deployments/{deploymentName}` to inspect the deployment and capture its additive `sourceFingerprint`
- open `http://localhost:8080/?page=audit` at any point for the bookmarkable audit page backed by `GET /v1/audit-logs`; it renders newest-first immutable plan/apply activity with explicit loading, empty, unavailable, and fetch-error states plus `_self` links back into the application drill-down and plan/apply workspace. Add `&action=plan.created|apply.requested` to narrow the embedded timeline to one action, and keep using `&limit={n}` when you want a smaller newest-first window from the API.
- `GET /v1/audit-logs?deployment={deploymentName}` when you want only one deployment's immutable plan/apply entries without changing the route shape
- `GET /v1/audit-logs?action=plan.created` when you want only immutable plan-preview creation records
- `GET /v1/audit-logs?action=apply.requested` when you want only immutable apply-request records
- `GET /v1/audit-logs?action={action}` when you want only one immutable action type; `action` must be exactly `plan.created` or `apply.requested`
- `POST /v1/plans/deployments/{deploymentName}` with optional `{"ifMatch":"<sourceFingerprint>"}` to persist an immutable preview for the reviewed state
- open `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}` for the bookmarkable plan workspace; `Apply Plan` activates only after that persisted ready preview is loaded with a valid `planId`
- `POST /v1/applies/plans/{planId}` to persist an immutable apply request derived from that ready preview, then reopen the same workspace at `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}`
- `GET /v1/applies/{applyId}` to reopen the stored apply request directly; this backend slice is disabled by default for live execution, so failed immutable apply records are expected and useful because they preserve operator-reviewed intent plus explicit error/details without mutating AWS
- `GET /v1/audit-logs?limit={n}` to project those persisted immutable plan/apply records into a newest-first local audit timeline; `/v1/audit-logs` is the preferred v1 route shape for this local timeline, and actor information is unavailable because the immutable plan preview and apply request stores do not persist operator identity
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}` when you need one deployment plus one exact action without changing the route family
- `GET /v1/audit-logs?action={action}&limit={n}` to keep the same newest-first and limit behavior while narrowing the timeline to exactly `plan.created` or `apply.requested`
- `GET /v1/audit-logs?deployment={deploymentName}&limit={n}` to keep the same newest-first and limit behavior while narrowing the timeline to one deployment; when no persisted audit records match, the API still returns `200 OK` with `entries: []`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}&limit={n}` when you need the same deployment-scoped bookmarkability plus one exact action type

Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis. Use `make smoke-docker` when you need to validate `compose.yaml`, confirm the `api`/`controller`/`redis` service contract, and prove the containerized local stack plus representative `Request-Id` headers, correlated structured API request logs, controller logs, deployment events, resource-log routes, `GET /v1/audit-logs`, `GET /v1/audit-logs?action=apply.requested&limit=2`, the invalid-action `400 Bad Request` boundary, the bookmarkable `/?page=audit&action={action}&limit={n}` audit view, immutable plan/apply read models, and embedded operator drill-down labels are reachable end to end.

### Run with Docker

```bash
make docker-up
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project. Inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`. If you want to clear a prior Tenet stack before retrying, run `make docker-down`, then rerun `docker compose up --build -d`.

Run the dedicated Docker-stack smoke before or instead of manual exploration when you want a single command that validates the Compose config, boots the stack, waits for API health, checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches `GET /v1/audit-logs` plus `GET /v1/audit-logs?action=apply.requested&limit=2`, rejects `action=invalid.action` with `400 Bad Request`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the disabled-by-default immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, fetches the bookmarkable `/?page=audit&action={action}&limit={n}` audit workspace URL and the bookmarkable `/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}` workspace URL, confirms the audit page preserves active filter copy such as `Audit filters: action apply.requested · limit 2`, confirms the `api` container emits correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`, confirms the embedded UI still includes current drill-down labels such as `Plan preview actions`, `Apply request actions`, `Controller runtime events`, and `Recent logs`, confirms controller logs are useful, and tears the stack back down:

```bash
make smoke-docker
```

This starts the local stack:
- `api` on `http://localhost:8080`
- `controller` as a long-running repo-validating process that now reloads fixtures and logs reconcile plans every few seconds
- `redis` as the active runtime status backend, seeded from `examples/local-status/deployments.json` when empty

Open:

```bash
http://localhost:8080/
```

To stop it:

```bash
make docker-down
```

## Using the UI

The intended operator flow is:

### Applications overview
Use the first page to scan:
- which applications exist
- which account they belong to
- sync and health state
- last update / current runtime signal

The applications overview also exposes an `_self` `Open audit log` link to the dedicated bookmarkable `/?page=audit` slice.

### Audit timeline
Use the audit page when you need immutable operator activity instead of controller-derived runtime history.

The page is backed directly by `GET /v1/audit-logs` and currently focuses on:
- newest-first `plan.created` and `apply.requested` entries
- a bookmarkable action filter that narrows the page and fetch route to exactly `plan.created` or `apply.requested`
- active filter copy such as `Audit filters: action apply.requested · limit 2` when you reopen `/?page=audit&action=apply.requested&limit=2`
- deployment / app / environment context
- explicit actor-unavailable or actor-unknown labeling instead of fabricated identity
- `_self` links back into `/?page=application&deployment={deploymentName}` as `Open application drill-down` and `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}` as `Open plan/apply workspace` when those identifiers exist

### Application detail
Open one application to see the linked rollout resources grouped into clear sections such as:
- workloads
- traffic routing
- config and access

### Resource detail
Open a concrete resource to inspect the deeper operational context for that resource, including rollout and runtime information. Recent logs are surfaced here when the resource type supports them.

Canonical resource-log routes stay stable across rollout modes so operators can bookmark or script them:
- `asgWeightedTargetGroups`: `trafficShiftPlan` (`{deployment}-weighted-routing`)
- `ecsRolling`: `ecsService` (`{deployment}-service`)

Behind those routes, the built-in AWS-backed recent-log provider now reuses SDK config/client setup per `(region, target account, observer role)` key inside a running Tenet API process instead of rebuilding ECS / CloudWatch Logs / Auto Scaling / EC2 clients on every fetch.

The same drill-down also includes read-only workspace surfaces:
- `/?page=drift&deployment={deploymentName}` for derived field-level troubleshooting context
- `/?page=plan&deployment={deploymentName}` for immutable plan previews, `Apply Plan` activation once a persisted ready preview is loaded, bookmarkable `planId`/`applyId` workspace URLs, and stored-preview quick links to both the embedded workspace (`_self`) and immutable `Plan JSON` / `Apply JSON` read models

The backend now also exposes read-only immutable apply-request routes:
- `POST /v1/applies/plans/{planId}`
- `GET /v1/applies/{applyId}`

These routes are additive and local-friendly. They persist immutable records to `TENET_APPLY_STORE_PATH` and intentionally stay disabled by default for live execution in this slice.

The same immutable stores also back a read-only audit projection route:
- `GET /v1/audit-logs`
- `GET /v1/audit-logs?deployment={deploymentName}`
- `GET /v1/audit-logs?action={action}`
- `GET /v1/audit-logs?limit={n}`
- `GET /v1/audit-logs?action={action}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}&limit={n}`

This timeline is newest-first, projects persisted plan preview and apply request records, derives `appRef` / `environmentRef` from the local catalog when possible, and keeps actor identity explicit by returning `actor.available=false` because this slice does not persist operator auth data. `deployment` is optional and must name exactly one deployment when present. `action` is optional and must be exactly `plan.created` or `apply.requested` when present. A deployment filter with no matching persisted records still returns `200 OK` with `entries: []` so local operators can distinguish "no audit rows yet" from transport failure. The embedded UI now renders that route at `/?page=audit` with explicit loading, empty, unavailable, and generic fetch-error states, plus a safe action filter control that preserves bookmarkable `action` / `deployment` / `limit` URL state.

Tenet also exposes a read-only deployment events slice for the same drill-down workflow:
- `GET /v1/overview/deployments/{deploymentName}/events`

The embedded `/?page=application&deployment={deploymentName}` drill-down now renders a small `Controller runtime events` timeline from that route with explicit loading, empty, unavailable, and fetch-error states. This route returns controller-derived runtime events copied from persisted `status.history`. It is intentionally operator-facing and refresh-based, not a full durable audit system yet, and the UI labels it separately from any future audit/auth history work. When a deployment exists but no controller-authored runtime history has been persisted, the route still returns `200 OK` with `events: []`.

When fixture-backed or early controller payloads have not yet persisted `rollout.execution[]`, Tenet keeps the read-only drill-down contract stable by synthesizing the same fallback rollout resources the operator would otherwise see from the controller-authored payload:
- `ecsRolling`: `ecsService` (`{deployment}-service`)

For the ECS rolling fallback service, the recent-log drill-down stays on the same operator workflow. Example route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`

## Repository layout

Tenet currently expects manifests under:

- `accounts/`
- `environments/`
- `deployments/`

Example data for local development lives in:

- `examples/local-repo/`
- `examples/local-status/`

## Current project status

Implemented today:
- repository-backed desired-state loading
- catalog compilation and validation
- controller/runtime status foundations
- filesystem and Redis runtime state backends
- read-only API and embedded UI
- applications -> application -> resource drill-down flow
- rollout/resource context for EC2 and ECS
- immutable plan preview creation/readback with stable `planId` and optimistic `ifMatch` support
- immutable apply-request creation/readback with stable `applyId`, plan validation, and disabled-by-default live execution semantics
- deployment events overview derived from persisted runtime history
- recent resource-log support for supported resource types

Not implemented yet:
- live AWS execution
- write/mutation APIs
- full plan/apply workflows
- a dedicated standalone frontend

## Documentation

For deeper design details, see:
- `docs/architecture/tenet-v1-overview.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-overview-ui.md`
- `docs/features/resource-recent-logs.md`

For the standalone architecture diagram, open this file locally in a browser:
- `docs/architecture/tenet-v1-architecture-diagram.html`

## Development

Run the main verification suite with:

```bash
go test ./...
make smoke-local
```

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
