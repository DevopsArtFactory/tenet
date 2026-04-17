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
- operational views such as rollout context, immutable plan previews, deployment events, and recent logs

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

Every HTTP response from `tenet-api` also carries a `Request-Id` header. If your client already sends `request-id`, Tenet echoes it back; otherwise it generates one. Capture that value when an API call fails and use the same ID to correlate any request-scoped handler or provider logs that read `api.RequestIDFromContext(ctx)`.

For the current read-only plan preview flow, a typical local operator path is:
- `GET /v1/overview/applications/{deploymentName}` or `GET /v1/overview/deployments/{deploymentName}` to inspect the deployment and capture its additive `sourceFingerprint`
- `POST /v1/plans/deployments/{deploymentName}` with optional `{"ifMatch":"<sourceFingerprint>"}` to persist an immutable preview for the reviewed state
- `GET /v1/plans/{planId}` or open `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}` to reopen the stored preview

Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis. Use `make smoke-docker` when you need to validate `compose.yaml`, confirm the `api`/`controller`/`redis` service contract, and prove the containerized local stack plus controller logs are reachable end to end.

### Run with Docker

```bash
make docker-up
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project. Inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`. If you want to clear a prior Tenet stack before retrying, run `make docker-down`, then rerun `docker compose up --build -d`.

Run the dedicated Docker-stack smoke before or instead of manual exploration when you want a single command that validates the Compose config, boots the stack, waits for API health, checks representative endpoints, confirms controller logs are useful, and tears the stack back down:

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

The same drill-down also includes read-only workspace surfaces:
- `/?page=drift&deployment={deploymentName}` for derived field-level troubleshooting context
- `/?page=plan&deployment={deploymentName}` for immutable plan previews and shareable `planId` reopen URLs while apply remains disabled

Tenet also exposes a read-only deployment events slice for the same drill-down workflow:
- `GET /v1/overview/deployments/{deploymentName}/events`

This route returns controller-derived runtime events copied from persisted `status.history`. It is intentionally operator-facing and refresh-based, not a full durable audit system yet. When a deployment exists but no controller-authored runtime history has been persisted, the route still returns `200 OK` with `events: []`.

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
