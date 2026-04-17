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

The goal is fast triage, not completeness.

### 2. Application detail
The second page focuses on one application and the resources that matter for understanding its rollout.

Resources are grouped into human-friendly sections such as:
- Workloads
- Traffic routing
- Config and access

This page should help the operator decide which concrete resource to inspect next.

### 3. Resource detail
The final page is where deeper operational context belongs.

This page is where Tenet can show things like:
- rollout state
- runtime history
- conditions
- artifact context
- recent logs when available

The goal is to make troubleshooting possible without forcing the user to jump directly into raw AWS consoles for the first step.

When `rollout.execution[]` is still missing or empty in a fixture-backed or early controller-authored payload, the resource drill-down keeps the operator contract stable by synthesizing the same fallback rollout resources the controller would eventually publish. That contract includes:
- `ecsRolling`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

That same fallback identity keeps resource-log drill-downs bookmarkable. Example route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`

### Additional operator views
The current UI also still exposes supporting operator surfaces beyond the main drill-down path:
- account-oriented views for grouped project/environment context
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
- application detail drill-down
- resource detail drill-down
- recent operational context for supported resources

Future iterations should continue improving:
- clarity of drill-down paths
- operator-focused wording
- troubleshooting usefulness
- quality of empty/unavailable/error states
- overall usability under real deployment pressure
