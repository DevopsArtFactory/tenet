# Tenet v1 Architecture Overview

## Goal

Tenet is a multi-account GitOps deployment control plane for AWS workloads. It reconciles Git-declared desired state into EC2 and ECS resources across multiple AWS accounts and regions, then exposes deployment progress, health, drift, and topology through a central API and UI.

## Product principles

- Git is the source of truth for desired state.
- Tenet continuously reconciles desired state to actual AWS state.
- Spec and status are separate and first-class.
- EC2 and ECS share a common rollout model.
- Multi-account operation uses a central control plane with cross-account role assumption.
- UI is a state and topology view, not a thin wrapper around logs.

## Operating model

### Control plane account

The central Tenet account runs:
- Git sync and spec ingestion
- Validation and planning
- Reconciliation workers
- Status aggregation
- API and web UI
- Audit/event storage

### Target accounts

Target AWS accounts own workload resources and expose cross-account IAM roles for Tenet to assume.

Tenet manages or observes:
- EC2 Auto Scaling Groups, Launch Templates, instances
- ECS clusters, services, task definitions, task sets
- Application Load Balancers, listeners, listener rules, target groups
- Security groups and deployment-scoped network resources
- Health and rollout telemetry

## High-level architecture

```text
Git repository
  -> Git sync / webhook ingest
    -> Spec validator
      -> Placement resolver (account / region / network)
        -> Rollout planner
          -> Reconciliation engine
            -> EC2 driver
            -> ECS driver
            -> ELBv2 / ASG / EC2 / ECS / IAM integrations
          -> Status aggregator
            -> Postgres
            -> Event stream / audit log
              -> API server
              -> UI
```

## Multi-account topology

```text
Central Tenet Control Plane Account
  - API/UI
  - Controller
  - Workers
  - Postgres
  - Event storage

    AssumeRole + ExternalId
       |
       +--> Shared services account
       +--> Dev account
       +--> Staging account
       +--> Prod account (region A)
       +--> Prod account (region B)
```

## Resource ownership model

Tenet should separate resource ownership into two layers.

### Foundation layer

Usually managed by platform teams or bootstrap flows:
- VPCs and baseline networking
- shared subnets
- DNS zones and ACM certificates
- shared ECS clusters where applicable
- organization guardrails and SCPs

### Workload layer

Managed by Tenet during reconciliation:
- deployment target groups
- listener rules or listener actions
- deployment Auto Scaling Groups
- launch template versions
- ECS service/task definition revisions
- deployment-scoped security groups when allowed

This boundary avoids state conflicts with external IaC systems and reduces blast radius.

## Deployment models

### EC2 deployment model

Recommended strategy: immutable blue/green style rollout.

1. Detect a new desired release.
2. Create or select a candidate target group.
3. Create a new launch template version.
4. Create a candidate Auto Scaling Group.
5. Wait for target health and warmup to succeed.
6. Shift listener traffic from active to candidate target group.
7. Stabilize and confirm health.
8. Mark synced status.
9. Drain and retire prior generation.

Active and candidate generations remain explicit in status and topology views.

### ECS deployment model

Common lifecycle, different driver implementation:
- release maps to task definition revision or image digest
- candidate maps to new service revision or task set
- health combines ECS deployment health and target group health
- promotion shifts listener traffic or finalizes rolling deployment

MVP should support ECS rolling deployment first, while preserving a status schema that can support full blue/green later.

## Reconciliation loop

Tenet behaves like a controller, not a one-shot script.

1. Observe desired spec from Git.
2. Resolve target placement and credentials.
3. Load current persisted status and observed AWS state.
4. Compute diff and rollout plan.
5. Execute the next safe idempotent action.
6. Persist fresh status, conditions, resource bindings, and events.
7. Repeat until synced, failed, paused, or rolled back.

## Status model

Status answers four questions:
- What generation is desired?
- What generation is actually active?
- Is rollout in progress, healthy, failed, or drifted?
- Why is the deployment not yet synced?

Recommended top-level fields:
- observedGeneration
- sync.state
- rollout.phase
- placement
- activeRelease and candidateRelease
- resource bindings
- health summary
- conditions[]
- lastAppliedCommit
- drift summary

## Sync states

- `Synced`: desired revision is active and healthy, with no blocking diff.
- `OutOfSync`: desired state differs from active state and no rollout is in progress.
- `Reconciling`: Tenet is actively moving actual state toward desired state.
- `Drifted`: managed resources differ from desired state due to out-of-band changes.
- `Error`: Tenet cannot safely proceed.
- `Paused`: rollout paused by policy or operator action.

## Rollout phases

Recommended rollout phases:
- Pending
- Accepted
- Planning
- Provisioning
- CandidateCreated
- HealthChecking
- ReadyToPromote
- Promoting
- Stabilizing
- Completed
- Failed
- RollingBack
- RolledBack
- Cancelled

## Multi-account trust model

Tenet should use STS AssumeRole with ExternalId and session tagging.

Per target account:
- `TenetDeployerRole`: write path for managed resources
- optional `TenetObserverRole`: read-only path for discovery and health observation

Recommended controls:
- trust only the central control plane role
- require external ID
- scope regions and services where possible
- enforce tagging policy for Tenet-managed resources
- use session tags for app, env, deployment, and rollout identifiers

## Observability model

Tenet aggregates:
- internal controller events
- ASG activities
- ELB target health
- ECS service events
- drift detection findings
- rollout milestones

Every event should be keyed by:
- app
- environment
- account
- region
- rollout ID
- resource ARN
- severity
- timestamp

## API surface

Primary read APIs:
- list applications, environments, and placements
- read deployment spec, status, and conditions
- read rollout timeline and resource topology
- read drift reports and audit history

Limited write APIs:
- pause or resume rollout
- rollback active rollout
- acknowledge policy gates

Desired-state edits should still flow through Git whenever possible.

## UI information architecture

Core screens:
- Global fleet overview
- Account and region health view
- Deployment detail page
- Rollout timeline
- Topology graph showing active and candidate generations
- Spec vs actual diff and drift view

## Delivery strategy

Phase 1 should establish:
- domain model and schema definitions
- central controller skeleton
- multi-account credential resolution
- EC2 blue/green planner and status model
- API read surface
- basic UI health and rollout pages

Later phases add:
- ECS support
- advanced drift remediation
- approval gates and policy engine
- richer topology and event ingestion
