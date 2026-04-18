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

The goal is fast triage, not completeness. The denser account/group deployment surface now lives on the secondary `/?page=account&accountId={awsAccountId}` drill-down, where the UI reuses `GET /v1/overview/accounts/{awsAccountId}` plus grouped project/environment sections instead of crowding the landing page. The landing page also exposes a small `_self` link to the dedicated bookmarkable audit route at `/?page=audit` (with optional `&deployment={deploymentName}`, `&action=plan.created|apply.requested`, and `&limit={n}` for a narrower newest-first window).

### 1b. Audit timeline
The audit page is a sibling operator slice, not a replacement for the application drill-down.

It stays intentionally narrow:
- fetch `GET /v1/audit-logs`
- preserve optional bookmarkable `action`, `deployment`, and `limit` URL state on the audit route
- render a safe action filter control for exactly `plan.created` and `apply.requested`
- render immutable plan/apply activity newest-first
- make a selected deployment obvious in both the audit chrome and the audit body when `deployment={deploymentName}` is present
- swap to deployment-specific empty-state copy when one deployment is selected and no audit entries match, while keeping the generic unfiltered empty state unchanged
- keep actor identity explicit as unavailable or unknown instead of inventing names
- provide `_self` links back into the existing `application` drill-down and `plan` workspace when deployment / plan / apply identifiers exist

This page is for immutable operator activity. It is separate from the controller-derived runtime events rendered on the application detail page.

### 2. Application detail
The second page focuses on one application and the resources that matter for understanding its rollout.

Resources are grouped into human-friendly sections such as:
- Workloads
- Traffic routing
- Config and access

The page is backed by `GET /v1/overview/applications/{deploymentName}` and keeps the deeper `resource`, `drift`, and `plan` routes anchored to `GET /v1/overview/deployments/{deploymentName}`. A narrow controller-runtime-events slice is also rendered here by calling `GET /v1/overview/deployments/{deploymentName}/events` directly so the bookmarkable application detail stays operator-useful without pulling the full deployment-detail payload. Both detail surfaces preserve additive `sourceFingerprint` so the same reviewed payload can feed the read-only plan-preview flow without inventing a second client-side identity model. The companion `_self` `Audit log` link now opens `/?page=audit&deployment={deploymentName}` so the operator keeps the same deployment context when switching into the immutable audit slice.

This page should help the operator decide which concrete resource to inspect next while still exposing the most recent controller-derived runtime timeline in a clearly non-audit section.

### 3. Resource detail
The final page is where deeper operational context belongs.

This page is where Tenet can still show the deeper operational slices:
- rollout state
- runtime history
- conditions
- live inspection
- artifact context
- recent logs plus the explicit lookup path Tenet used

The goal is to make troubleshooting possible without forcing the user to jump directly into raw AWS consoles for the first step.

The embedded resource page now includes a dedicated `Live inspection` panel that lazy-loads `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}` for the selected resource while keeping the bookmarkable `/?page=resource...` route unchanged. That panel should always render an explicit state:
- loading
- unavailable, with lookup context plus friendly copy mapped from `unavailableReason`
- empty, when the route is available but there is no typed payload to show yet
- supported, with operator-friendly fields for `ecsService`, Auto Scaling Group, and traffic-routing summary payloads
- a `Refresh live inspection` action that re-runs only the inspection route in place, disables while that inspection fetch is active, and preserves the bookmarkable resource URL

The same embedded resource route now also renders a compact quick-navigation strip above the deeper detail sections. That strip is intentionally data-local: it builds `_self` links for `Back to application`, `Drift view`, `Plan/apply`, `Audit log`, and sibling linked resources using only the already loaded deployment-detail payload plus preserved URL state. It should not introduce nested interactive controls or a second fetch path just to support lateral navigation.

For recent logs, the embedded card keeps backend lookup context visible with operator-facing labels such as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured`. Those fields stay visible in unavailable and empty states so operators can see where Tenet looked before switching to raw AWS troubleshooting. That card also exposes `Refresh recent logs` with the same in-place URL-preserving behavior and per-panel loading disablement.

The deployment-events route is intentionally read-only and controller-derived. It reflects persisted runtime history entries, returns `events: []` when a deployment exists without history yet, and should not be treated as a full durable audit system. The application drill-down now also exposes `Refresh runtime events` on that panel so operators can re-run only `GET /v1/overview/deployments/{deploymentName}/events` in place, preserve the bookmarkable `/?page=application...` URL, and disable just that panel action while the events fetch is in flight.

The drift workspace stays read-only and now exposes `Refresh drift workspace` on the field-level drift section so operators can re-run only `GET /v1/overview/deployments/{deploymentName}` in place, keep the bookmarkable `/?page=drift...` route unchanged, and see explicit loading/error copy without clearing the current drift workspace content while the refresh is in flight.

Canonical fallback resource-detail bindings remain stable so operators can share or bookmark the exact resource page they need:
- `asgWeightedTargetGroups`
- `asgInstanceRefresh`
- `ecsRolling`
- `ecsCodeDeploy`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

The neighboring read-only plan-preview flow is also part of the operator contract now:
- `POST /v1/plans/deployments/{deploymentName}` persists an immutable preview derived from the current deployment detail
- `GET /v1/plans/{planId}` reopens the exact stored preview
- `POST /v1/applies/plans/{planId}` persists an immutable apply request derived from an existing ready preview, but only after re-validating that the current deployment-detail fingerprint still matches the reviewed plan fingerprint
- `GET /v1/applies/{applyId}` reopens the exact stored apply request
- `POST /v1/applies/{applyId}/transitions` advances executor-facing lifecycle state on that same persisted apply record with a JSON `applyrequest.TransitionRequest` body while preserving the immutable stable `applyId`; Tenet's local-safe executor now uses that same state model internally, so enabled-mode progress does not require manual transition API calls
- `GET /v1/audit-logs?deployment={deploymentName}` narrows the local audit timeline to one deployment without introducing a second route family
- `GET /v1/audit-logs?action={action}` narrows the same local audit timeline to exactly `plan.created` or `apply.requested`
- `GET /v1/audit-logs` projects persisted plan/apply records into a local audit timeline; the route also accepts `?limit={n}` for a newest-first window and optional `action={action}` for exact action filtering
- the embedded `/?page=audit` route now renders that audit timeline directly with explicit loading, empty, unavailable (`503`), and generic fetch-error states, accepts optional `&action={action}` / `&limit={n}` for action-only bookmarks and optional `&deployment={deploymentName}` / `&action={action}` / `&limit={n}` for deployment-scoped bookmarks, shows active filter copy such as `Audit filters: action apply.requested · limit 2` for action-only bookmarks and `Audit filters: deployment payments-api-prod · action apply.requested · limit 2` for deployment-scoped bookmarks, keeps deployment-scoped copy obvious in both the page chrome and body, and exposes the same bookmarkable limit through a safe toolbar selector that still honors positive-integer URLs such as `?limit=2`
- the embedded `/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable, exposes explicit `Refresh plan preview` / `Refresh apply request` controls that re-run the current immutable reads without a full browser reload, and prefers the reviewed detail `sourceFingerprint` for optimistic `ifMatch` protection before falling back to a loaded preview fingerprint
- live execution remains intentionally disabled by default in this backend slice, so the apply-request route records a failed immutable contract with operator-facing error/details instead of contacting AWS while still attaching additive ordered `operations[]` rows with `status=pending`, `phase`, `target`, and `summary` fields projected from the reviewed plan preview
- when that reviewed plan fingerprint is stale, `POST /v1/applies/plans/{planId}` fails with `409 Conflict`, tells the operator to create a fresh plan preview, and does not persist a new apply record from drifted input
- when `TENET_AWS_ENABLE_APPLY=true`, newly created apply records switch to a queued immutable apply request without contacting AWS; the stored contract drops the disabled error, changes those projected `operations[]` rows to `status=queued`, and adds explicit details that the same `applyId` will be handed to a local-safe executor because live AWS mutation is not implemented yet
- that local-safe executor then advances the same apply record through `running` and finally `failed`, keeping the stable `applyId` while recording operator-facing handoff details instead of inventing a second job identity
- the stored apply read model now also includes an additive `lifecycle` block so executor code can advance the same stable `applyId` through queued/running/terminal states while surfacing `updatedAt` / `startedAt` / `finishedAt` timestamps plus `currentStep` / `completedSteps` / `totalSteps` progress metadata for operators
- malformed apply-transition JSON bodies fail with `400 Bad Request`, unknown apply IDs fail with `404 Not Found`, and invalid lifecycle moves fail with `409 Conflict` so executor/worker callers get explicit operator-facing boundaries without changing the route family
- the embedded queued/running/succeeded/cancelled/failed apply chrome is still rendering immutable stored lifecycle state for operator review, not a live AWS executor in this slice
- actor identity is intentionally explicit rather than invented: `GET /v1/audit-logs` renders `Recorded` with persisted `id` / `name` / `email` when explicit `X-Tenet-Actor-*` request headers were present at record creation time, and still renders `actor.available=false` plus an unavailable reason for unauthenticated or purely local requests
- invalid audit `action` query values fail at the API boundary with `400 Bad Request` instead of silently widening or ignoring the filter
- a deployment-scoped audit query that finds no persisted records still returns `entries: []` so operators can distinguish an empty immutable timeline from an unavailable route

When `rollout.execution[]` is still missing or empty in a fixture-backed or early controller-authored payload, the resource drill-down keeps the operator contract stable by synthesizing the same fallback rollout resources the controller would eventually publish. That contract includes:
- `ecsRolling`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

That same fallback identity keeps both resource-log and resource-inspection drill-downs bookmarkable. Example routes:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`
- `GET /v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service`

The additive inspection route stays read-only and local-friendly:
- supported always-live summaries currently start with `ecsService`, `candidateAutoScalingGroup`, and `serviceAutoScalingGroup`
- supported traffic-routing resources are `codeDeployDeploymentGroup`, `productionListener`, `testListener`, and `trafficShiftPlan`; `trafficShiftPlan` stays `summary_only`, while the CodeDeploy deployment-group/listener resources upgrade to `inspectionMode=live` when AWS inspection succeeds and fall back to the existing summary payload when it does not
- `trafficShiftPlan` stays summary-only, while `codeDeployDeploymentGroup`, `productionListener`, and `testListener` preserve that summary fallback locally and add live AWS inspection details on the same `trafficRoutingResource` body when live lookup succeeds
- responses always include `lookupContext` plus `available`
- supported ECS payloads add `ecsService`
- supported Auto Scaling payloads add `autoScalingGroup`, with optional `launchTemplate` and `mixedInstancesPolicy` metadata when the live Auto Scaling Group already exposes launch-template details through `DescribeAutoScalingGroups`
- supported traffic-routing payloads add `trafficRoutingResource` with `kind`, `displayName`, `inspectionMode`, controller/fallback `summary`, and additive live `codeDeployDeploymentGroup` or `listener` details when available
- unsupported or unavailable resources return `available=false` with an explicit `unavailableReason` instead of bubbling a vague provider error into the operator workflow

In the embedded UI, the supported inspection payloads should stay intentionally operator-facing:
- `ecsService`: service status, desired/running/pending counts, task definition, deployments, and recent ECS service events
- `autoScalingGroup`: sizing and health-check settings, additive launch-template metadata, target group ARNs, and current instances
- `trafficRoutingResource`: summary-only or live traffic-routing context with explicit resource kind, display name, inspection mode, rollout summary text, and additive CodeDeploy deployment-group or listener details

### Additional operator views
The current UI also still exposes supporting operator surfaces beyond the main drill-down path:
- account-oriented views for grouped project/environment context
- audit timeline for immutable plan/apply history
- drift workspace for troubleshooting, explicitly labeling whether field-level drift is rendering from deployment-detail `managedDrift[]` rows or from the current derived fallback read-model heuristics during mixed-mode migration
- plan/apply workspace for read-only rollout planning context, including immutable plan-preview actions, explicit in-place refresh controls for bookmarkable `planId` / `applyId` records, and a persisted apply-operations table when a bookmarkable `applyId` is loaded

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
