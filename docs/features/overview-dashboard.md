# Tenet Overview Dashboard

## Purpose

The embedded Tenet dashboard is the current operator-facing entry point for the control-plane prototype.

The UX direction is now a simpler ArgoCD-style drill-down instead of a single dense dashboard. The intended navigation is:
- applications overview first
- application-linked resources second
- concrete resource detail last

The implementation remains fully embedded HTML/CSS/JS with no separate frontend toolchain.

The dashboard routes are source-agnostic on the backend: `tenet-api` can now feed the same `/v1/catalog/*` and `/v1/overview/*` payloads from either a filesystem checkout or a GitHub-backed read-only desired-state catalog without changing the UI route contract.

## What shipped so far

### 1. Applications overview landing page

The dashboard at `/` now starts from `GET /v1/overview/applications`, a flattened joined read model that already returns one application row per deployment instead of making the client flatten `GET /v1/overview/accounts` just to render the default landing page. The route intentionally preserves the richer joined runtime/rollout/artifact fields already produced by the account overview builder, even though the landing page currently renders only the application-triage subset.

The first page now focuses on:
- application name
- account
- project/app
- sync
- health
- last update

This keeps the first page optimized for triage instead of deep inspection while leaving the joined route useful for search matching and future application-level summary extensions.

### 2. Argo-inspired visual shell

The current UI refresh adds a more opinionated operator-console layout:
- a top hero section with control-plane summary
- summary cards for filter scope, runtime signals, and workspace shortcuts
- account sidebars that separate account metadata from deployment sections
- denser, more readable deployment cards with stronger visual hierarchy
- preserved dark theme and compact pill-based status cues

This is still intentionally lightweight and framework-free.

### 3. Filter toolbar and URL state

The toolbar is client-side and keeps state in the URL for shareable focused views.

Managed query params:
- `q`
- `sync`
- `account`
- `page`
- `accountId`
- `deployment`

Current behavior:
- `account` uses stable `awsAccountId`
- invalid `sync` and `account` values normalize to `all`
- unrelated query params/hash fragments are preserved when updating managed filters
- search input focus and caret position survive rerenders
- when filters remove all results, the UI renders `No applications match the current filters.`

## Current pages

### Applications overview

The first page now shows only application-level rows plus search/filter controls, backed by `/v1/overview/applications`.

Current applications-overview behavior:
- keeps the existing bookmarkable `q` / `sync` / `account` query state as the primary first-page triage surface
- exposes an in-place `Refresh applications overview` control that re-runs only `GET /v1/overview/applications`, disables only while that overview fetch is active, and keeps the same bookmarkable applications URL/query state stable
- keeps the current applications summary plus visible rows rendered with explicit loading/error copy while that in-place refresh is running or fails
- refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance as the other embedded read paths instead of generic refresh-error copy

Each row surfaces:
- application name
- account
- project/app
- sync
- health
- last update
- `_self` `Open audit log` link to `/?page=audit`
- expanded deployment rows also add a deployment-scoped `_self` `Audit log` handoff to `/?page=audit&deployment={deploymentName}` beside `_self` `Open application`
- `_self` `Open account` link to `/?page=account&accountId={awsAccountId}`
- `_self` `Open application` link to `/?page=application&deployment={deploymentName}`

The account drill-down page consumes `GET /v1/overview/accounts/{awsAccountId}` and keeps the denser account-oriented view available without making it the first page again. Current account-page behavior:
- back-link to the applications overview while preserving current filters
- the page now renders the same compact `Quick navigation` chrome used by the deeper drill-downs, with an explicit `_self` `Applications overview` return link plus a non-click `Account detail` chip so the current bookmarkable account route stays obvious without dropping current `q`, `sync`, or `account` query state
- the same account quick-nav now adds a hash-aware `Current section` chip and an `On this page` group for `Account summary` plus the currently rendered project/environment groups, with bookmarkable anchors such as `/?page=account&accountId={awsAccountId}#account-summary`
- when the same project/environment pair appears in multiple regions, those account section links keep the region in the visible label so operators can tell the jump targets apart before leaving the current section
- when one of those account section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to `No section is currently targeted.`
- an in-place `Refresh account detail` control that re-runs only `GET /v1/overview/accounts/{awsAccountId}` for the current bookmarkable route, disables only while that refresh is active, and keeps the same query/hash state stable
- the current account detail remains visible with explicit loading/error copy while that in-place refresh is running or fails
- `Account deployment overview` summary/header with account and topology context
- grouped project/environment sections derived from the account payload
- sibling `Account JSON`, `Group JSON`, and `_self` `Open application` links outside the expandable row toggle
- expandable deployment rows that still lazy-load `GET /v1/overview/deployments/{deploymentName}` for deep detail
- initial `GET /v1/overview/accounts/{awsAccountId}` auth failures are status-aware on the bookmarkable `/?page=account&accountId={awsAccountId}` route: `401` tells operators to retry the same page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route or deployment
- refresh-time `401` / `403` responses reuse that same trusted-front-door / access-denied guidance instead of generic fetch-error copy
- when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, `GET /v1/overview/accounts`, `GET /v1/overview/accounts/{awsAccountId}`, and `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` require trusted `X-Tenet-Authenticated-*` identity
- when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, those same guarded account reads additionally require one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer` / `operator` / `admin`
- the account collection filters returned accounts down to deployments visible through trusted `X-Tenet-Authorized-Deployments` and returns `200 OK` with `[]` when the allowlist is omitted or matches nothing
- the account-detail and grouped-account routes filter their deployment lists to the visible subset, recompute additive aggregate counters from that subset, and return the existing `403 Forbidden` plus `forbidden: operator does not have access to this deployment` when the authenticated operator has no authorized deployments in that account or grouped scope

The underlying `/v1/overview/applications` payload is still a richer joined row than the first page renders. It also preserves additive runtime summary, rollout summary, artifact, generation, condition, rollout-context fields (`runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`), and the latest human-readable `runtimeSummary.changeDetails[]` copied from controller-authored runtime history when present, so other UI helpers and future slices can reuse the same route without rejoining `/v1/overview/accounts`. Client-side search now also matches additive `runtimeSummary.deployedArtifact`, concise `runtimeSummary.conditions[]` summaries, and `runtimeSummary.changeDetails[]` when those fields are present, but it intentionally skips placeholder `—` text so missing runtime context does not create noisy matches.

### Application detail page

The application-detail route now loads `GET /v1/overview/applications/{deploymentName}` and renders grouped linked resources instead of deep deployment internals.

Current behavior:
- breadcrumb/back-link to the applications overview, with an additive account step when `awsAccountId` is already known from the loaded deployment detail
- top-level application summary for account/region context
- compact `Current application status` block near the top of the page so operators can quickly scan sync, health, rollout context, artifact, linked-resource count, runtime-event availability, and last update before drilling further
- the compact summary stays read-only and uses the existing application-detail payload, preserving the applications-first drill-down instead of turning the page back into a dense dashboard
- `_self` `Drift view` and `_self` `Plan/apply` links so operators enter the deployment workspaces from the application detail page instead of the account rows
- when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` this route also requires trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments` to include the exact deployment name
- when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is also enabled, that same application-detail read additionally requires one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer` / `operator` / `admin`
- initial `GET /v1/overview/applications/{deploymentName}` auth failures are status-aware: `401` tells operators to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route/deployment
- the page now renders the same compact `Quick navigation` chrome used by the deeper workspaces, with an explicit `_self` `Applications overview` return link, a non-click `Application detail` chip to keep the current drill-down level obvious, and a hash-aware `Current section` group plus an `On this page` group for only the currently rendered non-empty linked-resource sections plus `Controller runtime events`, with bookmarkable anchors such as `/?page=application&deployment={name}#application-runtime-events`
- when one of those application section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the friendly empty state `No section is currently targeted.`
- the page toolbar now includes an in-place `Refresh application detail` control that re-runs only `GET /v1/overview/applications/{deploymentName}`, disables only while that application-detail fetch is active, keeps the bookmarkable `/?page=application...` URL stable, and leaves the current compact status summary, grouped-resource content, and runtime-events content visible with explicit loading/error copy while the refresh is running or if it fails
- refresh-time `401` / `403` responses on that application-detail read reuse the same trusted-front-door / access-denied guidance instead of falling back to generic refresh-error text
- `Controller runtime events` section backed by `GET /v1/overview/deployments/{deploymentName}/events`
- explicit `Loading controller runtime events…`, empty, unavailable, and generic fetch-error states for non-auth failures on that section
- event ordering labeled as newest-first with readable time, summary, sync/health context, rollout phase/current step, and optional controller metadata
- explicitly labels these as controller-derived runtime events, not audit/auth history
- the `Controller runtime events` section header now includes an in-place `Refresh runtime events` button that re-runs just that deployment-events route in place, keeps the bookmarkable `/?page=application...` URL stable, and disables only while that section fetch is active
- when that guarded lazy read returns `401` or `403`, the runtime-events panel keeps its refresh control and bookmarkable route stable but replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on the other embedded read paths
- grouped resource sections:
  - `Workloads`
  - `Traffic routing`
  - `Config and access`
- empty zero-resource groups are intentionally suppressed on the page and in quick navigation so the application detail stays focused on actionable resource groups; if an application truly has no linked resources yet, the page shows `No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.` while still keeping the runtime-events section available
- `_self` `Open resource` links targeting `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`
- when the selected resource type already supports the final resource page's evidence panels, the same row now also surfaces direct `_self` handoffs to `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}#resource-live-inspection` and `#resource-recent-logs`
- unsupported resource types intentionally suppress those shortcuts instead of advertising unavailable evidence surfaces on the application page
- keeps the deeper artifact/history/generation/log drill-down on the final resource page
- that final resource page now also exposes a top-level `Refresh resource detail` action that re-runs `GET /v1/overview/deployments/{deploymentName}` in place, preserves the bookmarkable `/?page=resource...` URL, re-resolves the selected resource from the refreshed payload, and keeps the last successful resource detail visible while refresh-time loading/error guidance renders above it
- the recent-log card on that final resource page now surfaces the backend lookup path directly as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured`, even when the log lookup is empty or unavailable
- when the `Recent logs` payload also carries additive `backingResourceType` plus `backingResourceName` for a bootstrap-source lookup, that same card now renders those values directly as `Backing resource type` and `Backing resource name`; direct log sources keep the card quiet and do not show empty backing-resource placeholders
- the `Recent logs` and `Live inspection` cards on that final resource page each now render their own explicit `Refresh recent logs` and `Refresh live inspection` controls so operators can re-run only that panel's backend route in place; each control disables only while its own fetch is active, preserves the bookmarkable `/?page=resource...` URL, leaves the sibling panel and the rest of the resource page visible, and forces a fresh fetch even when the last panel state was already cached as ready or error
- when those guarded lazy panel reads return `401` or `403`, each panel keeps its own refresh control and bookmarkable route stable but replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths
- the backend resource drill-down now also has a parallel read-only inspection route for supported live and local-friendly summaries: `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`
- that inspection payload is intentionally additive and explicit for operators/UI consumers: common fields stay `deploymentName`, `resourceType`, `resourceName`, `lookupContext`, `available`, `source`, `unavailableReason`, while supported resource-specific bodies are currently `ecsService`, `autoScalingGroup`, and `trafficRoutingResource`
- when a `userDataBootstrapSource` inspection payload also includes `backingResourceType` plus `backingResourceName`, the embedded resource page now renders an explicit `_self` handoff to the resolved backing resource drill-down using the same normalized `rolloutExecutionResources()` mapping Tenet already uses for bookmarkable `resourceId` values, and suppresses the handoff entirely when the current deployment detail cannot resolve that resource safely
- the compose smoke now exercises that shipped bootstrap-source drill-down directly by fetching the bookmarkable `userDataBootstrapSource` resource page plus the paired `resource-logs` and `resource-inspection` routes, and it accepts explicit `aws_unavailable` / `no_log_source` unavailable payloads as the expected local no-AWS contract
- live `ecsService` payloads now surface additive deployment-configuration fields already present on `DescribeServices`, including optional `minimumHealthyPercent`, `maximumPercent`, and `healthCheckGracePeriodSeconds`
- live `autoScalingGroup` payloads now surface additive live launch-template inspection fields `launchTemplate.imageId` and `launchTemplate.instanceType`, alongside the optional `launchTemplate` and `mixedInstancesPolicy` details already present on `DescribeAutoScalingGroups` when the backing Auto Scaling Group uses them
- `trafficRoutingResource` keeps the same typed body for `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `productionListenerRule`, `activeTargetGroup`, `candidateTargetGroup`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`, and `trafficShiftPlan`, but only `codeDeployDeploymentGroup`, `productionListener`, and `testListener` upgrade from `summary_only` fallback to `inspectionMode=live` when AWS inspection succeeds
- local/no-AWS runs still return the existing summary-only `trafficRoutingResource` fallback for those CodeDeploy traffic-routing resources, while the weighted-target-group resources `productionListenerRule`, `activeTargetGroup`, `candidateTargetGroup`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`, and `trafficShiftPlan` stay summary-only by contract so bookmarkable fixture-backed drill-downs remain stable
- repeated successful live inspections for `codeDeployDeploymentGroup`, `productionListener`, and `testListener` reuse a bounded in-process cache keyed by `(region, target account, observer role, resource type, resource name)` so back-to-back operator drill-downs do not repeat the same AWS lookups inside one API process
- `ecsService` lookups keep a bounded in-process cluster hint per `(region, target account, observer role, service name)` so repeated inspection requests try the last known cluster first; if that hint is stale, Tenet clears it, falls back to the full cluster scan, and refreshes the hint from the recovered service location
- unsupported or unavailable inspection lookups keep the route usable during troubleshooting by returning `available=false` plus an explicit `unavailableReason` instead of surfacing a raw provider failure

Resource-list fallback contract:
- the UI synthesizes fallback rollout resources when `detail.rollout.execution[]` is missing or empty in the application-detail payload
- the application-detail payload also carries normalized `userData` metadata so EC2 fallback resources can preserve stable `userDataBootstrapSource` identities before controller-authored `rollout.execution[]` rows appear
- when `detail.rollout.execution[]` is present and non-empty, the UI preserves the controller-authored list verbatim
- synthesized fallback resources are limited to the canonical controller rollout contracts:
  - `asgWeightedTargetGroups`: `candidateAutoScalingGroup`, `activeTargetGroup`, `candidateTargetGroup`, `productionListenerRule`, `trafficShiftPlan`, `userDataBootstrapSource`
  - `asgInstanceRefresh`: `serviceAutoScalingGroup`, `userDataBootstrapSource`
  - `ecsRolling`: `ecsService`
  - `ecsCodeDeploy`: `ecsService`, `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`
- fallback names/descriptions stay aligned with the controller-authored `rollout.execution[]` metadata so derived `resourceId` values remain stable and bookmarked resource-detail routes keep resolving across fixture-backed and controller-authored payloads

### Deployment workspaces

The existing `Drift view` and `Plan/apply` workspace shells remain available as deeper read-only troubleshooting pages, but they are now entered from the application detail page instead of older account-row shortcuts. Their shared quick-nav also now mirrors the higher-level drill-downs more closely by exposing explicit `_self` return paths for `Applications overview` and `Back to application`, then separating the active drift/plan/audit page into its own non-click `Current destination` chip.

The bookmarkable `/?page=resource&deployment={name}...` and `/?page=drift&deployment={name}` entry loads both depend on `GET /v1/overview/deployments/{deploymentName}` and now distinguish initial auth failures: `401` tells operators to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route or deployment. When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` the same narrow read gate also applies to `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, and `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, using the exact deployment allowlist from trusted `X-Tenet-Authorized-Deployments`. On the resource page, those two lazy panel routes now render the same status-aware `401` / `403` guidance in place without reloading deployment detail or mutating the bookmarkable URL.

### Resource detail quick navigation

The embedded `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` page now includes explicit breadcrumb chrome plus the compact quick-navigation strip above the deeper runtime/log/inspection sections.

Current behavior:
- renders breadcrumb chrome with `_self` links for prior drill-down levels, including the account step when the loaded deployment detail exposes `awsAccountId`
- keeps the current resource as a non-click breadcrumb chip instead of presenting it as a self link
- keeps all links bookmarkable and `_self` inside the embedded shell
- includes `Back to application`, `Drift view`, `Plan/apply`, and deployment-scoped `Audit log` links
- renders the current resource as an explicit non-click chip so the strip does not present the active destination as a lateral link
- adds a `Current section` group that derives a non-click chip from `window.location.hash` when the bookmarkable route targets one of the rendered resource sections
- keeps the `On this page` group as `_self` fragment links for the remaining rendered resource sections, using stable ids such as `resource-runtime-history`, `resource-rollout-execution`, `resource-status-conditions`, `resource-live-inspection`, and `resource-recent-logs` while preserving the current bookmarkable pathname/query
- when a bookmarked fallback-resource alias resolves to a canonical resource id instead (for example `userDataBootstrapSource` `file:userdata/...` resolving to canonical `git:userdata/...`), those same-page fragment links keep the operator-requested `resourceId` in the URL and the page adds a small alias cue while still using the resolved canonical resource for detail, inspection, and log lookups
- derives sibling resource links only from the already loaded deployment-detail payload, so the strip does not introduce another fetch path
- updates the `Current section` chip for initial deep links, later in-page hash changes, and lazy `Recent logs` / `Live inspection` repaints that preserve the same bookmarkable route
- re-applies hash scrolling after the lazy `Recent logs` and `Live inspection` panels repaint so shared deep links such as `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}#resource-live-inspection` reopen the intended section after async render
- keeps the anchors outside toggle buttons and other nested interactive controls

### Audit timeline page

The embedded UI now exposes a dedicated bookmarkable audit route:
- `/?page=audit`
- `/?page=audit&deployment={deploymentName}` to keep one deployment's audit context visible in the page chrome/body
- `/?page=audit&action={action}` to narrow the page to exactly `plan.created` or `apply.requested`
- `/?page=audit&limit={n}` to request a smaller newest-first window from `GET /v1/audit-logs`; the audit toolbar exposes the same bookmarkable limit through an explicit safe selector

Current behavior:
- fetches `GET /v1/audit-logs` directly
- preserves optional `deployment`, `action`, and `limit` URL state when present
- renders a safe action filter control for exactly `plan.created` and `apply.requested`
- renders a safe limit selector in the audit toolbar with preset newest-first windows while still honoring bookmarked positive integers such as `limit=2`
- renders an in-place `Refresh audit log` control that re-runs only `GET /v1/audit-logs` for the current bookmarkable audit route, disables only while that audit fetch is active, and preserves the same deployment/action/limit query state
- renders a conditional `Clear audit filters` recovery action whenever `deployment`, `action`, or `limit` are active, resets those audit filters back to the default `/?page=audit` slice, and preserves bookmarkable applications-shell state such as `q`, `sync`, and `account`
- renders explicit `Loading audit log…`, empty, unavailable (`503`), and generic fetch-error states for non-auth failures
- renders distinct initial auth guidance for `GET /v1/audit-logs`: `401` tells operators to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route or deployment
- that auth gate is still narrow: when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, `GET /v1/overview/applications` requires trusted `X-Tenet-Authenticated-*` identity and filters returned rows through the trusted `X-Tenet-Authorized-Deployments` allowlist and, when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, the same guarded read routes additionally require one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer` / `operator` / `admin` with `forbidden: authenticated viewer-capable role is required for this operator read route` for authenticated callers that lack one; unscoped `GET /v1/audit-logs` follows that same collection contract with trusted identity plus allowlist filtering and returns `200 OK` with `{"entries":[]}` when no visible audit rows remain, deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}` reads still require exact deployment membership, and when the applications allowlist is omitted or matches nothing, the API returns `200 OK` with `[]` so the overview stays read-only and explicit
- makes an active deployment filter obvious in both the page chrome and body when `deployment={deploymentName}` is present
- adds breadcrumb chrome for `Applications -> {deploymentName} -> Audit log` when a deployment filter is present, keeping prior levels as `_self` links and the current audit route as a non-click chip
- adds the same quick-navigation strip used in the drift/plan workspaces when `deployment={deploymentName}` is present, with a `Hierarchy` group for `_self` `Applications overview` and `Back to application`, a `Current destination` chip for the active deployment-scoped `Audit log` route, and sibling workspace links for `Drift view` / `Plan/apply`
- adds a hash-aware `Current section` group plus an `On this page` group to that deployment-scoped audit quick-navigation strip, using stable fragment links for `Deployment context` (`#audit-deployment-context`) when the deployment callout is present and `Audit timeline` (`#audit-timeline`) for the immutable entries section while preserving the current bookmarkable pathname/query
- when one of those audit section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the same explicit empty state `No section is currently targeted.`
- renders deployment-specific empty-state copy when one deployment is selected and no persisted audit entries match, while keeping the generic unfiltered empty state unchanged
- keeps the current `/?page=audit...` URL/query state stable while leaving the current audit content visible with explicit loading/error copy during an in-place refresh of the same route
- keeps entries newest-first even if the client receives an unsorted payload
- shows explicit labels for action, deployment/app/environment context, timestamp, actor availability, and plan/apply identifiers
- keeps actor identity explicit rather than fabricated by rendering `Unavailable` / `Unknown` plus the server-authored reason text
- exposes `_self` `Open application drill-down` and `_self` `Open drift view` links whenever `deploymentName` is present, plus `_self` `Open plan/apply workspace` when `planId` and `applyId` are present

### Deployment runtime events API

Operators can also fetch a read-only deployment event timeline from:
- `GET /v1/overview/deployments/{deploymentName}/events`

This payload is derived from persisted `status.history` entries the controller authored for runtime changes. It is meant to support operator drill-down and timeline rendering, not to act as a full durable audit system yet.

Separate from that controller-runtime timeline, the backend also exposes `GET /v1/audit-logs` as a local immutable audit projection over persisted plan preview and apply request records. The dedicated `/?page=audit` view now renders that route, showing recorded actor identity when explicit `X-Tenet-Actor-*` request headers or additive `X-User-*` aliases were persisted and keeping `actor.available=false` explicit when no authenticated request context was present.

Current behavior:
- returns `deploymentName` plus an explicit `events[]` array
- returns `200 OK` with `events: []` when the deployment exists but no runtime history has been persisted yet
- never calls live AWS APIs; it only reflects the stored controller/runtime read model
- the application detail page consumes the same route directly instead of reusing `history[]` from the deeper deployment-detail payload

## Workspace shells

The dashboard now supports lightweight workspace routing inside the same embedded shell.

Routes:
- `/?page=audit`
- `/?page=audit&deployment={name}`
- `/?page=audit&action={action}`
- `/?page=audit&limit={n}`
- `/?page=drift&deployment={name}`
- `/?page=plan&deployment={name}`
- `/?page=plan&deployment={name}&planId={planId}`
- `/?page=plan&deployment={name}&planId={planId}&applyId={applyId}`

### Drift workspace

Current drift workspace behavior:
- reuses the embedded shell
- lazy-loads deployment detail
- shows latest runtime context rows for target/strategy/execution mode
- shows additive runtime artifact/condition rows from `runtimeSummary.deployedArtifact` / `runtimeSummary.conditions`
- keeps machine-readable `runtimeSummary.changes[]` rows while also rendering human-readable `runtimeSummary.changeDetails[]`
- shows deployment context and status conditions
- adds an in-place `Refresh drift workspace` control that re-runs only `GET /v1/overview/deployments/{deploymentName}` in place, keeps the bookmarkable `/?page=drift...` URL stable, disables only while that refresh is active, and leaves the current read-only workspace content visible with explicit loading/error copy
- adds breadcrumb chrome for `Applications -> [Account] -> {deploymentName} -> Drift view`, omitting the account link when `awsAccountId` is missing and keeping `Drift view` as a non-click chip
- adds a quick-navigation strip with a `Hierarchy` group for `_self` `Applications overview` and `Back to application`, a `Current destination` chip for the active drift workspace, and sibling deployment-workspace links for `Plan/apply` and deployment-scoped `Audit log`, keeping the current workspace obvious while preserving `_self` bookmarkable navigation for the other destinations inside the embedded shell
- adds a hash-aware `Current section` group plus an `On this page` group to that drift quick-navigation strip with `_self` fragment links for `Runtime signals`, `Rollout execution`, `Field-level drift`, and `Status conditions`, preserving the current bookmarkable pathname/query while appending section hashes such as `/?page=drift&deployment={name}#drift-field-level`
- when one of those drift section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- adds direct `_self` resource drill-down links to that drift quick-navigation strip using the same grouped normalized resource inventory already used by the application/resource pages, with explicit `No linked resources are available in this drift workspace yet.` copy when no linked resources are available
- prefers backend-authored additive `managedDrift[]` rows from the deployment-detail payload when present, normalizes them into the same safe read-only table, and falls back to the existing client-derived artifact/generation/runtime-intent comparisons for older payloads
- controller-authored rows can now include live-backed EC2 Auto Scaling Group capacity evidence for `asgWeightedTargetGroups` and `asgInstanceRefresh` deployments when live AWS inspection succeeds, using stable field names under `runtimeContext.capacity.{desired,min,max}`
- the same EC2 path can also add `runtimeContext.launchTemplate.bootstrapSource`, `runtimeContext.launchTemplate.imageId`, and `runtimeContext.launchTemplate.instanceType` when read-only launch-template inspection can decode an explicit bootstrap/userdata-source marker and the observed primary launch-template `imageId` / `instanceType`, then compare them against the normalized desired deployment bootstrap source plus first-stack-region `ami_id` / `instance_type`
- controller-authored rows can also include live-backed ECS service desired-count, deployment-config, and task-definition image evidence for `ecsCodeDeploy` and `ecsRolling` deployments under `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, and `runtimeContext.taskDefinition.image` when read-only ECS inspection can resolve the live service desired count, deployment configuration, and a single live container image
- focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows
- when the controller itself defaulted a completed deployment to `Synced` and then emitted non-empty `managedDrift[]`, the same deployment-detail payload now surfaces `sync.state=Drifted` instead of leaving a misleading controller-authored synced state; seeded sync evidence and active rollout states remain unchanged
- shows an explicit `Active drift source` callout so operators can see whether the table is rendering controller-authored `managedDrift[]` rows or the existing derived fallback heuristics
- adds a compact drift summary ahead of the table with total rows, visible rows after the current query/severity filters, and live `high` / `medium` / `low` counts so filter impact is obvious without scanning the full table
- adds an explicit `Clear drift filters` control that resets the drift query, severity, and sort controls back to their defaults in one step without mutating the bookmarkable `/?page=drift...` route or re-running the deployment-detail fetch
- uses source-specific empty-state copy so operators can distinguish "no controller-authored field-level drift was reported" from "no derived fallback field-level drift was detected"; local runs, missing AWS access, and older payloads continue to land on the existing clean fallback path instead of inventing live-backed rows
- adds filtered-empty copy that tells operators the current query/severity controls are hiding all reported drift rows
- keeps client-side severity filtering, query filtering, and sorting consistent across both backend-authored and fallback-derived drift rows
- keeps the toolbar `Audit log` link deployment-scoped so operators land on `/?page=audit&deployment={name}` instead of the global audit slice

### Deployment-scoped audit workspace

Current deployment-scoped audit behavior:
- reuses the embedded shell and the same `GET /v1/audit-logs` route family
- leaves the global `/?page=audit` route unchanged when no deployment filter is present
- adds breadcrumb chrome that keeps the applications overview and deployment detail as bookmarkable `_self` steps when the deployment is known, while leaving the current audit destination as a non-click chip
- adds the same compact quick-navigation strip as the drift/plan workspaces when `deployment={name}` is present
- keeps that strip bookmarkable by routing through `buildPageURL(...)`, so `q`, `sync`, `account`, `deployment`, and current audit `action` / `limit` state survive `_self` navigation

### Plan/apply workspace

Current plan/apply workspace behavior:
- reuses the embedded shell
- shows latest runtime context rows for target/strategy/execution mode
- shows additive runtime artifact/condition rows from `runtimeSummary.deployedArtifact` / `runtimeSummary.conditions`
- keeps machine-readable `runtimeSummary.changes[]` rows while also rendering human-readable `runtimeSummary.changeDetails[]`
- derives a read-only `Plan operations` table from `rollout.steps[]`, execution-resource metadata, and the latest runtime summary in the deployment detail route, and keeps that deployment-derived operations/risk/approval context visible before any preview exists so the page stays useful before mutation work
- shows a visible derived `high` / `medium` / `low` risk badge plus concise approval guidance from the current read model before any future mutation wiring
- enables `Create Plan` to call the immutable plan-preview API (`POST /v1/plans/deployments/{deploymentName}`) with an `ifMatch` fingerprint derived from the currently reviewed deployment/application detail when possible, falling back to the loaded plan preview `source.fingerprint` when the client cannot derive the current one, and immediately renders the returned server-authored immutable preview in place
- rewrites the page URL to `/?page=plan&deployment={deploymentName}&planId={planId}` after a successful create, then loads `GET /v1/plans/{planId}` whenever a `planId` query param is present and renders ready/failed/unavailable/fetch-error states without hiding the deployment-derived context; bookmarked or refreshed `401` / `403` plan-preview reads now show explicit trusted-front-door / access-denied guidance instead of generic fetch-error copy
- replaces the stored-preview raw URL text with actionable links: `Open bookmarked workspace` stays in-app via `_self`, while `Plan JSON` remains a read-model route to `GET /v1/plans/{planId}`
- when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, stored `GET /v1/plans/{planId}` reloads reuse the same trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments` allowlist contract as the other deployment-scoped operator drill-down reads, using the persisted preview deployment
- when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is enabled, `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` also require trusted forwarded `X-Tenet-Authenticated-Roles` to include one operator-capable role from `operator` / `admin`, so authenticated callers without that role set fail fast with `forbidden: authenticated operator role is required for this operator write route`
- when that same deployment-read authorization setting is enabled, `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` also require the trusted `X-Tenet-Authorized-Deployments` allowlist to include the requested deployment (or persisted preview deployment for apply creation), so unauthorized write attempts fail with the same explicit deployment-access-denied `403 Forbidden` used by guarded reads
- activates `Apply Plan` only when the page has loaded a persisted ready preview with a valid `planId`, then POSTs `POST /v1/applies/plans/{planId}` and rewrites the page URL to `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}`

- loads `GET /v1/applies/{applyId}` whenever an `applyId` query param is present and renders explicit loading, queued, running, succeeded, cancelled, failed, unavailable, and generic fetch-error states without hiding deployment-derived or plan-derived fallback context; bookmarked or refreshed `401` / `403` apply-record reads now show explicit trusted-front-door / access-denied guidance instead of generic fetch-error copy
- when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, stored `GET /v1/applies/{applyId}` reloads use the same trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments` allowlist contract as other guarded operator reads, using the persisted apply deployment
- the bookmarkable workspace distinguishes the new role-based `403` from deployment access denial: Tenet authenticated the caller, but the trusted front door did not forward an operator-capable `X-Tenet-Authenticated-Roles` value for the write route
- relies on the backend to reject stale reviewed plans with `409 Conflict` when the current deployment-detail fingerprint no longer matches the preview fingerprint, so operators must create a fresh preview before a new immutable apply record is written
- renders stored immutable apply metadata plus `Open bookmarked apply` (`_self`) and `Apply JSON` links, keeps the current disabled-by-default failed behavior explicit, documents the opt-in queued backend contract behind `TENET_AWS_ENABLE_APPLY=true`, adds a compact operator-facing apply lifecycle summary beside the stored record, and now surfaces stored lifecycle timing/progress metrics from `lifecycle.updatedAt` / `startedAt` / `finishedAt` / `currentStep` / `completedSteps` / `totalSteps` when present
- when a stored immutable apply record includes additive `operations[]`, renders progress counts in that lifecycle summary plus a dedicated `Apply operations` table whose status labels stay readable for queued/running/succeeded/failed/cancelled rows without relying on mutable preview/fallback context; the stored rows currently use pending statuses for the default disabled path or queued statuses for the opt-in enabled path, while missing lifecycle timing/step fields stay as placeholders instead of synthetic progress values
- adds breadcrumb chrome for `Applications -> [Account] -> {deploymentName} -> Plan/apply`, omitting the account link when `awsAccountId` is missing and keeping `Plan/apply` as a non-click chip
- keeps the same quick-navigation strip available inside the plan/apply workspace so operators can jump through a `Hierarchy` group with `_self` `Applications overview` and `Back to application`, see the active `Plan/apply` page as a non-click `Current destination` chip, and still move laterally to sibling `Drift view` / deployment-scoped `Audit log` workspaces without relying on browser back or leaving the bookmarkable route
- adds a hash-aware `Current section` group plus an `On this page` group to that plan/apply quick-navigation strip, using `_self` fragment links only for the plan/apply sections currently rendered and stable ids such as `plan-runtime-signals`, `plan-preview-actions`, `plan-stored-preview`, `plan-stored-apply`, `plan-apply-operations`, and `plan-operations` while preserving the current bookmarkable pathname/query and appending hashes such as `/?page=plan&deployment={name}&planId={planId}&applyId={applyId}#plan-apply-operations`
- when one of those plan/apply section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`

- returns to the application detail page through the embedded back link so the drill-down path stays tight
- keeps the toolbar `Audit log` link deployment-scoped so operators land on `/?page=audit&deployment={name}` instead of the global audit slice

## Quick links

The dashboard now exposes the following navigation affordances across the current drill-down flow:
- landing page: `_self` `Open audit log`, expanded-row deployment-scoped `_self` `Audit log`, `_self` `Open account`, `_self` `Open application`
- account detail page: `Account JSON`, `Group JSON`, `_self` `Open application`
- application detail page: `_self` `Drift view`, `_self` `Plan/apply`, `_self` `Audit log`, `Open resource`, `Open live inspection`, `Open recent logs`
- application detail `Audit log` now routes to `/?page=audit&deployment={deploymentName}` so the selected deployment stays visible in the bookmarkable audit slice
- audit page: `_self` `Back to applications`, `_self` `Open application drill-down`, `_self` `Open drift view`, `_self` `Open plan/apply workspace`
- resource/workspace drill-down pages: embedded back links that keep operators in the same shell
- account-oriented JSON surfaces remain available through the API routes (`/v1/overview/accounts/{awsAccountId}`, `/v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, `/v1/overview/deployments/{deploymentName}`, `/v1/status/deployments/{deploymentName}`)

The landing page itself is backed by `GET /v1/overview/applications`; the application-detail page uses `GET /v1/overview/applications/{deploymentName}`, while the deeper resource/workspace drill-down pages continue to reuse `GET /v1/overview/deployments/{deploymentName}`.

The bookmarkable landing page and bookmarkable account detail page both use the shared embedded read-route helpers on initial load, so `401 Unauthorized` and `403 Forbidden` responses show the same trusted-front-door / access-denied guidance used by the other operator-read pages instead of a generic overview fetch failure.

Important implementation constraints:
- these links are rendered as sibling controls outside the expand-toggle button when they appear in expandable deployment contexts
- interactive elements are not nested inside the toggle button

## Runtime-summary contract in the UI

Overview cards consume additive `runtimeSummary` data when present.

Current operator-visible fields include:
- `runtimeSummary.timestamp`
- `runtimeSummary.summary`
- `runtimeSummary.sync.state`
- `runtimeSummary.sync.reason`
- `runtimeSummary.health.overall`
- `runtimeSummary.health.summary`
- `runtimeSummary.desiredGeneration`
- `runtimeSummary.observedGeneration`
- `runtimeSummary.targetType`
- `runtimeSummary.strategy`
- `runtimeSummary.executionMode`
- `runtimeSummary.changes[]`
- `runtimeSummary.changeDetails[]`
- `runtimeSummary.deployedArtifact`
- `runtimeSummary.conditions[]`
- `runtimeSummary.source`
- `runtimeSummary.rolloutPhase`
- `runtimeSummary.currentStep`
- `runtimeSummary.historyCount`

UI rendering rules:
- safe DOM construction only (`createElement`, `textContent`, `replaceChildren`)
- no `innerHTML` for manifest/status-backed values
- missing generation fields render as `—`, not `0`
- runtime context rows are rendered as concise `Target … · Strategy … · Execution …` text sourced from runtimeSummary/history fields, not reconstructed from HTML snippets
- runtime artifact/condition summaries are rendered only when present; search/index text never includes placeholder `—` entries for missing runtime artifact/condition context
- latest change labels/details are additive string arrays, not objects

## Testing and verification

Current coverage includes:
- Go string-fragment tests in `internal/api/ui_test.go`
- Node-backed behavior tests in `internal/api/ui_behavior_test.go`
- embedded-script syntax validation by extracting the `<script>` block from `internal/api/web/index.html` and running `node --check` on the extracted JavaScript
- `make smoke-local`
- `make smoke-docker`, including raw shell-contract checks for the root applications overview refresh hooks (`Refresh applications overview`, the `GET /v1/overview/applications` route-builder text, and the loading/error copy), the bookmarkable application-detail status summary shell (`Current application status`, rollout/runtime summary labels, and the summary mount point in the served HTML/JS), the bookmarkable resource page's top-level refresh shell (`Refresh resource detail`, the `GET /v1/overview/deployments/{deploymentName}` route-builder text, and the preserved-detail loading/error copy), plus the bookmarkable resource page's lazy-panel refresh hooks (`Refresh recent logs`, `Refresh live inspection`, their route-builder helpers, and the loading/error/auth-guidance copy) rather than runtime DOM assertions

When changing the overview UI, preserve all of the following:
- URL normalization for filters
- focus/caret restoration
- lazy detail loading contract
- safe DOM rendering
- sibling quick-link structure outside the toggle button
- account/group/deployment quick-link href contracts
- workspace shell routing behavior

## Files most relevant to this feature

- `internal/api/web/index.html`
- `internal/api/ui_test.go`
- `internal/api/ui_behavior_test.go`
- `internal/api/ui.go`
- `README.md`
- `scripts/smoke-local.sh`

## Near-term next steps

Likely next UI slices:
- richer runtime history navigation and event timelines
- real plan/apply mutations once write APIs exist
- account/group summary sparklines or event cues
- deeper drift diagnostics once the backend exposes richer live drift sources beyond the current derived read model
- eventual extraction into a dedicated frontend only if the current read-model/UI surface becomes too large for embedded HTML
