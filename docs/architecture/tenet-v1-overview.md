# Tenet v1 Architecture Overview

## What Tenet is

Tenet is a repository-driven deployment control plane for multi-account AWS environments.

Its job is to sit between declared deployment intent and day-to-day operations:
- teams describe what should run in Git-like manifests
- Tenet compiles that intent into a validated internal model
- Tenet joins that desired state with runtime status
- operators use Tenet to understand rollout state, health, linked resources, and recent operational context

The current product direction is intentionally operator-first. The system is being shaped around an ArgoCD-like drill-down experience rather than a dense dashboard full of low-level raw data.

## Core idea

Tenet separates three concerns:

1. Desired state
- What should exist
- Which account and region it belongs to
- Which strategy and artifact should be deployed

2. Runtime state
- What appears to be happening now
- Whether the deployment is synced, progressing, drifted, paused, or failed
- What rollout and health signals have been observed recently

3. Operator read models
- What a human needs to see to understand the situation quickly
- Applications overview, application detail, and resource detail views
- Context that helps troubleshooting without forcing the operator to inspect raw infrastructure objects first

## Architecture at a glance

Tenet currently has four practical layers.

### 1. Repository-backed desired state
Tenet reads manifests from a repo-shaped tree:
- accounts
- environments
- deployments

That repository is the source of deployment intent.

### 2. Catalog compilation
The manifest tree is loaded, normalized, and validated into a compiled catalog.

This catalog gives Tenet a consistent internal model for:
- placement
- deployment strategy
- desired artifacts
- rollout intent
- account and environment relationships

### 3. Runtime status storage
Runtime state is stored separately from desired configuration.

This layer tracks things like:
- sync state
- health state
- rollout progress
- conditions
- bounded runtime history

In local development, this can be backed by the filesystem or Redis.

### 4. Read-only control-plane surface
Tenet exposes joined read models through:
- the API
- the embedded UI
- a first safe GitHub webhook ingestion primitive at `POST /api/v1/github/webhook`

The embedded UI is intentionally lightweight and served directly by the Go API. That keeps the system easy to run locally while the control-plane model is still stabilizing. Request timeout and cancellation now propagate through the catalog, overview, and status provider path as well, so slow GitHub-backed catalog reads or runtime-status joins can stop when the originating HTTP request is cancelled or times out. The webhook surface stays conservative but is no longer inert: with `TENET_GITHUB_WEBHOOK_SECRET` configured, it validates signed GitHub requests, decodes push payloads into an operator-friendly summary, returns explicit `accepted` or `ignored` responses, reports deleted-ref pushes as `status=ignored` with `deleted=true`, and reports additive `desiredStateSourceMatch` state for the currently configured desired-state source. Filesystem mode and non-Redis runtimes still return `processed=false` with an explicit unavailable reason, while matched GitHub-source pushes in `TENET_STATUS_BACKEND=redis` publish an additive `controllerRefresh` handoff, return `processed=true`, and let the controller reconcile immediately instead of waiting only for the ticker.

## UI architecture

The current UI follows a progressive drill-down model.

### Applications overview
This is the starting point.

It answers:
- which applications exist
- which account they belong to
- whether they look healthy or not
- which ones deserve attention first

### Application detail
This page narrows the view to one application and its linked resources.

It is meant to answer:
- what resources are involved in this rollout
- where traffic routing lives
- what the operator should inspect next

### Resource detail
This is the operational page.

It is meant to answer:
- what is happening to this specific resource
- what rollout/runtime context exists right now
- what logs, history, or conditions help explain the current state

## Operating model

Tenet is designed as a central control plane that can reason about multiple AWS accounts while keeping workload ownership in the target accounts.

At a conceptual level:
- Tenet owns control-plane logic, validation, read models, and operator surfaces
- target accounts own the actual workload resources
- Tenet needs a trust/access model that allows observation and, later, controlled execution

## Why this shape

This architecture is meant to solve a common operator problem:
raw infrastructure data is often too low-level, while high-level deployment dashboards are often too shallow.

Tenet tries to bridge that gap by:
- keeping desired state explicit
- keeping runtime state explicit
- turning both into views that make operational sense to humans

## Current scope

Tenet today is still early and intentionally conservative.

Included now:
- repository-backed desired-state loading
- catalog compilation and validation
- runtime status foundations
- read-only control-plane API
- GitHub webhook validation, push-payload summarization, desired-state source relevance reporting, and Redis-backed controller refresh handoff in GitHub+Redis mode
- local-safe apply executor handoff for immutable apply requests when explicitly enabled
- embedded UI with applications -> application -> resource drill-down

Not included yet:
- full live AWS execution
- write/mutation workflows
- complete plan/apply automation
- a standalone frontend application

## Diagram

Open the architecture diagram here:
- `docs/architecture/tenet-v1-architecture-diagram.html`

## Related docs

- `docs/architecture/tenet-v1-overview-ui.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-aws-central-control-plane.md`
- `docs/features/resource-recent-logs.md`
- `deploy/terraform/aws-central-control-plane/README.md`
