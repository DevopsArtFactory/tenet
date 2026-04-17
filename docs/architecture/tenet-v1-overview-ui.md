# Tenet Overview UI

## Purpose

The Tenet UI is the operator-facing surface of the control plane.

It is intentionally designed to help someone answer a few practical questions quickly:
- What needs attention right now?
- Which application is affected?
- Which resource is involved?
- What operational context explains the current state?

Rather than starting from raw infrastructure objects, the UI starts from the application and lets the operator drill down only when they need more detail.

## Interaction model

### 1. Applications overview
This is the default landing page.

It is intentionally narrow and should stay easy to scan.

The first page focuses on:
- application name
- account
- project
- sync state
- health
- last update

The goal is fast triage, not completeness. The denser account/group deployment surface now lives on the secondary `/?page=account&accountId={awsAccountId}` drill-down, where the UI reuses `GET /v1/overview/accounts/{awsAccountId}` plus grouped project/environment sections instead of crowding the landing page. The landing page also exposes a small `_self` link to the dedicated bookmarkable audit route at `/?page=audit` (with optional `&limit={n}` for a smaller newest-first window).

### 1b. Audit timeline
The audit page is a sibling operator slice, not a replacement for the application drill-down.

It stays intentionally narrow:
- fetch `GET /v1/audit-logs`
- render immutable plan/apply activity newest-first
- keep actor identity explicit as unavailable or unknown instead of inventing names
- provide `_self` links back into the existing `application` and `plan` routes when deployment / plan / apply identifiers exist

This page is for immutable operator activity. It is separate from the controller-derived runtime events rendered on the application detail page.

### 2. Application detail
The second page focuses on one application and the resources that matter for understanding its rollout.

Resources are grouped into human-friendly sections such as:
- Workloads
- Traffic routing
- Config and access

The page is backed by `GET /v1/overview/applications/{deploymentName}` and keeps the deeper `resource`, `drift`, and `plan` routes anchored to `GET /v1/overview/deployments/{deploymentName}`. A narrow controller-runtime-events slice is also rendered here by calling `GET /v1/overview/deployments/{deploymentName}/events` directly so the bookmarkable application detail stays operator-useful without pulling the full deployment-detail payload. Both detail surfaces preserve additive `sourceFingerprint` so the same reviewed payload can feed the read-only plan-preview flow without inventing a second client-side identity model.

This page should help the operator decide which concrete resource to inspect next while still exposing the most recent controller-derived runtime timeline in a clearly non-audit section.

### 3. Resource detail
The final page is where deeper operational context belongs.

This page is where Tenet can still show the deeper operational slices:
- rollout state
- runtime history
- conditions
- artifact context
- recent logs when available

The goal is to make troubleshooting possible without forcing the user to jump directly into raw AWS consoles for the first step.

The deployment-events route is intentionally read-only and controller-derived. It reflects persisted runtime history entries, returns `events: []` when a deployment exists without history yet, and should not be treated as a full durable audit system.

Canonical fallback resource-detail bindings remain stable so operators can share or bookmark the exact resource page they need:
- `asgWeightedTargetGroups`
- `asgInstanceRefresh`
- `ecsRolling`
- `ecsCodeDeploy`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

The neighboring read-only plan-preview flow is also part of the operator contract now:
- `POST /v1/plans/deployments/{deploymentName}` persists an immutable preview derived from the current deployment detail
- `GET /v1/plans/{planId}` reopens the exact stored preview
- `POST /v1/applies/plans/{planId}` persists an immutable apply request derived from an existing ready preview
- `GET /v1/applies/{applyId}` reopens the exact stored apply request
- `GET /v1/audit-logs?deployment={deploymentName}` narrows the local audit timeline to one deployment without introducing a second route family
- `GET /v1/audit-logs` projects persisted plan/apply records into a local audit timeline; the route also accepts `?limit={n}` for a newest-first window
- the embedded `/?page=audit` route now renders that audit timeline directly with explicit loading, empty, unavailable (`503`), and generic fetch-error states, and it accepts optional `&limit={n}` to request a smaller newest-first window from the API
- the embedded `/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable and prefers the reviewed detail `sourceFingerprint` for optimistic `ifMatch` protection before falling back to a loaded preview fingerprint
- live execution remains intentionally disabled by default in this backend slice, so the apply-request route records a failed immutable contract with operator-facing error/details instead of contacting AWS
- actor identity is intentionally explicit rather than invented: `GET /v1/audit-logs` returns `actor.available=false` because the persisted immutable stores do not currently capture operator auth data
- a deployment-scoped audit query that finds no persisted records still returns `entries: []` so operators can distinguish an empty immutable timeline from an unavailable route

When `rollout.execution[]` is still missing or empty in a fixture-backed or early controller-authored payload, the resource drill-down keeps the operator contract stable by synthesizing the same fallback rollout resources the controller would eventually publish. That contract includes:
- `ecsRolling`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

That same fallback identity keeps resource-log drill-downs bookmarkable. Example route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`

### Additional operator views
The current UI also still exposes supporting operator surfaces beyond the main drill-down path:
- account-oriented views for grouped project/environment context
- audit timeline for immutable plan/apply history
- drift workspace for troubleshooting
- plan/apply workspace for read-only rollout planning context

These should remain secondary to the main applications -> application -> resource flow, but they are still important operator tools.

## Design principles

The UI should continue to follow these principles.

### Progressive disclosure
Do not put everything on the first page.
Start with application status, then reveal deeper information through drill-down.

### Operator-first clarity
Prefer names, groups, and labels that are understandable during a rollout or incident.
The UI should reduce cognitive load, not increase it.

### Explicit states
Loading, empty, unavailable, and error states should all be visible and understandable.
A blank area is not acceptable UX.

### Stable navigation
Pages and links should preserve context so operators can bookmark, share, and return to the exact view they need.

### Lightweight delivery
The UI remains embedded and framework-light for now. That keeps the product easy to run and iterate on while the control-plane model is still settling.

## Backing model

Conceptually, the UI is powered by joined read models built from:
- desired deployment state from the repository
- runtime status from Tenet's status layer
- resource and rollout context derived for operators

That means the UI is not just a thin log viewer or a direct mirror of raw provider APIs. It is intended to be a control-plane view designed around operational understanding.

## Current scope

Today the UI already supports:
- applications overview
- account drill-down for grouped project/environment context
- application detail drill-down
- resource detail drill-down
- read-only drift and plan-preview workspaces with bookmarkable URLs
- deployment events derived from persisted runtime history
- recent operational context for supported resources

Future iterations should continue improving:
- clarity of drill-down paths
- operator-focused wording
- troubleshooting usefulness
- quality of empty/unavailable/error states
- overall usability under real deployment pressure
