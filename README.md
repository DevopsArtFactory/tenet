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
- operational views such as rollout context and recent logs

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

### Run with Docker

```bash
make docker-up
```

This starts the local stack, including the API, controller, and Redis-backed runtime state.

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
