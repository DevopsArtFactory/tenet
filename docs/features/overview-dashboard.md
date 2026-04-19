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

Each row surfaces:
- application name
- account
- project/app
- sync
- health
- last update
- `_self` `Open audit log` link to `/?page=audit`
- `_self` `Open account` link to `/?page=account&accountId={awsAccountId}`
- `_self` `Open application` link to `/?page=application&deployment={deploymentName}`

The account drill-down page consumes `GET /v1/overview/accounts/{awsAccountId}` and keeps the denser account-oriented view available without making it the first page again. Current account-page behavior:
- back-link to the applications overview while preserving current filters
- `Account deployment overview` summary/header with account and topology context
- grouped project/environment sections derived from the account payload
- sibling `Account JSON`, `Group JSON`, and `_self` `Open application` links outside the expandable row toggle
- expandable deployment rows that still lazy-load `GET /v1/overview/deployments/{deploymentName}` for deep detail

The underlying `/v1/overview/applications` payload is still a richer joined row than the first page renders. It also preserves additive runtime summary, rollout summary, artifact, generation, condition, rollout-context fields (`runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`), and the latest human-readable `runtimeSummary.changeDetails[]` copied from controller-authored runtime history when present, so other UI helpers and future slices can reuse the same route without rejoining `/v1/overview/accounts`. Client-side search now also matches additive `runtimeSummary.deployedArtifact`, concise `runtimeSummary.conditions[]` summaries, and `runtimeSummary.changeDetails[]` when those fields are present, but it intentionally skips placeholder `—` text so missing runtime context does not create noisy matches.

### Application detail page

The application-detail route now loads `GET /v1/overview/applications/{deploymentName}` and renders grouped linked resources instead of deep deployment internals.

Current behavior:
- breadcrumb/back-link to the applications overview
- top-level application summary for account/region context
- `_self` `Drift view` and `_self` `Plan/apply` links so operators enter the deployment workspaces from the application detail page instead of the account rows
- `Controller runtime events` section backed by `GET /v1/overview/deployments/{deploymentName}/events`
- explicit `Loading controller runtime events…`, empty, unavailable, and generic fetch-error states for that section
- event ordering labeled as newest-first with readable time, summary, sync/health context, rollout phase/current step, and optional controller metadata
- explicitly labels these as controller-derived runtime events, not audit/auth history
- the `Controller runtime events` section header now includes an in-place `Refresh runtime events` button that re-runs just that deployment-events route in place, keeps the bookmarkable `/?page=application...` URL stable, and disables only while that section fetch is active
- grouped resource sections:
  - `Workloads`
  - `Traffic routing`
  - `Config and access`
- `_self` `Open resource` links targeting `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`
- keeps the deeper artifact/history/generation/log drill-down on the final resource page
- the recent-log card on that final resource page now surfaces the backend lookup path directly as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured`, even when the log lookup is empty or unavailable
- the `Recent logs` and `Live inspection` cards on that final resource page each now render their own explicit refresh button so operators can re-run just that backend route in place; each control disables only while its own fetch is active, preserves the bookmarkable `/?page=resource...` URL, and forces a fresh fetch even when the last panel state was already cached as ready or error
- the backend resource drill-down now also has a parallel read-only inspection route for supported live and local-friendly summaries: `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`
- that inspection payload is intentionally additive and explicit for operators/UI consumers: common fields stay `deploymentName`, `resourceType`, `resourceName`, `lookupContext`, `available`, `source`, `unavailableReason`, while supported resource-specific bodies are currently `ecsService`, `autoScalingGroup`, and `trafficRoutingResource`
- live `autoScalingGroup` payloads now surface additive launch-template metadata already present on `DescribeAutoScalingGroups`, including optional `launchTemplate` and `mixedInstancesPolicy` details when the backing Auto Scaling Group uses them
- `trafficRoutingResource` keeps the same typed body for `codeDeployDeploymentGroup`, `productionListener`, `testListener`, and `trafficShiftPlan`, but `codeDeployDeploymentGroup`, `productionListener`, and `testListener` now upgrade from `summary_only` fallback to `inspectionMode=live` when AWS inspection succeeds
- local/no-AWS runs still return the existing summary-only `trafficRoutingResource` fallback for those CodeDeploy traffic-routing resources, so bookmarkable fixture-backed drill-downs remain stable
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

The existing `Drift view` and `Plan/apply` workspace shells remain available as deeper read-only troubleshooting pages, but they are now entered from the application detail page instead of older account-row shortcuts.

### Resource detail quick navigation

The embedded `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` page now includes a compact quick-navigation strip above the deeper runtime/log/inspection sections.

Current behavior:
- keeps all links bookmarkable and `_self` inside the embedded shell
- includes `Back to application`, `Drift view`, `Plan/apply`, and deployment-scoped `Audit log` links
- derives sibling resource links only from the already loaded deployment-detail payload, so the strip does not introduce another fetch path
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
- renders explicit `Loading audit log…`, empty, unavailable (`503`), and generic fetch-error states
- makes an active deployment filter obvious in both the page chrome and body when `deployment={deploymentName}` is present
- adds the same quick-navigation strip used in the drift/plan workspaces when `deployment={deploymentName}` is present, with `_self` links for `Back to application`, `Drift view`, `Plan/apply`, and the current deployment-scoped `Audit log` route
- renders deployment-specific empty-state copy when one deployment is selected and no persisted audit entries match, while keeping the generic unfiltered empty state unchanged
- keeps the current `/?page=audit...` URL/query state stable while leaving the current audit content visible with explicit loading/error copy during an in-place refresh of the same route
- keeps entries newest-first even if the client receives an unsorted payload
- shows explicit labels for action, deployment/app/environment context, timestamp, actor availability, and plan/apply identifiers
- keeps actor identity explicit rather than fabricated by rendering `Unavailable` / `Unknown` plus the server-authored reason text
- exposes `_self` `Open application drill-down` and `_self` `Open plan/apply workspace` links when `deploymentName`, `planId`, and `applyId` are present

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
- adds a quick-navigation strip for `Back to application`, `Drift view`, `Plan/apply`, and deployment-scoped `Audit log`, keeping the current workspace obvious while preserving `_self` bookmarkable navigation inside the embedded shell
- prefers backend-authored additive `managedDrift[]` rows from the deployment-detail payload when present, normalizes them into the same safe read-only table, and falls back to the existing client-derived artifact/generation/runtime-intent comparisons for older payloads
- when the controller itself defaulted a completed deployment to `Synced` and then emitted non-empty `managedDrift[]`, the same deployment-detail payload now surfaces `sync.state=Drifted` instead of leaving a misleading controller-authored synced state; seeded sync evidence and active rollout states remain unchanged
- shows an explicit `Active drift source` callout so operators can see whether the table is rendering controller-authored `managedDrift[]` rows or the existing derived fallback heuristics
- adds a compact drift summary ahead of the table with total rows, visible rows after the current query/severity filters, and live `high` / `medium` / `low` counts so filter impact is obvious without scanning the full table
- uses source-specific empty-state copy so operators can distinguish "no controller-authored field-level drift was reported" from "no derived fallback field-level drift was detected"
- adds filtered-empty copy that tells operators the current query/severity controls are hiding all reported drift rows
- keeps client-side severity filtering, query filtering, and sorting consistent across both backend-authored and fallback-derived drift rows
- keeps the toolbar `Audit log` link deployment-scoped so operators land on `/?page=audit&deployment={name}` instead of the global audit slice

### Deployment-scoped audit workspace

Current deployment-scoped audit behavior:
- reuses the embedded shell and the same `GET /v1/audit-logs` route family
- leaves the global `/?page=audit` route unchanged when no deployment filter is present
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
- rewrites the page URL to `/?page=plan&deployment={deploymentName}&planId={planId}` after a successful create, then loads `GET /v1/plans/{planId}` whenever a `planId` query param is present and renders ready/failed/unavailable/fetch-error states without hiding the deployment-derived context
- replaces the stored-preview raw URL text with actionable links: `Open bookmarked workspace` stays in-app via `_self`, while `Plan JSON` remains a read-model route to `GET /v1/plans/{planId}`
- activates `Apply Plan` only when the page has loaded a persisted ready preview with a valid `planId`, then POSTs `POST /v1/applies/plans/{planId}` and rewrites the page URL to `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}`
- relies on the backend to reject stale reviewed plans with `409 Conflict` when the current deployment-detail fingerprint no longer matches the preview fingerprint, so operators must create a fresh preview before a new immutable apply record is written
- loads `GET /v1/applies/{applyId}` whenever an `applyId` query param is present and renders explicit loading, queued, running, succeeded, cancelled, failed, unavailable, and generic fetch-error states without hiding deployment-derived or plan-derived fallback context
- renders stored immutable apply metadata plus `Open bookmarked apply` (`_self`) and `Apply JSON` links, keeps the current disabled-by-default failed behavior explicit, documents the opt-in queued backend contract behind `TENET_AWS_ENABLE_APPLY=true`, adds a compact operator-facing apply lifecycle summary beside the stored record, and now surfaces stored lifecycle timing/progress metrics from `lifecycle.updatedAt` / `startedAt` / `finishedAt` / `currentStep` / `completedSteps` / `totalSteps` when present
- when a stored immutable apply record includes additive `operations[]`, renders progress counts in that lifecycle summary plus a dedicated `Apply operations` table whose status labels stay readable for queued/running/succeeded/failed/cancelled rows without relying on mutable preview/fallback context; the stored rows currently use pending statuses for the default disabled path or queued statuses for the opt-in enabled path, while missing lifecycle timing/step fields stay as placeholders instead of synthetic progress values
- keeps the same quick-navigation strip available inside the plan/apply workspace so operators can jump to `Back to application`, `Drift view`, `Plan/apply`, and deployment-scoped `Audit log` without relying on browser back or leaving the bookmarkable route
- returns to the application detail page through the embedded back link so the drill-down path stays tight
- keeps the toolbar `Audit log` link deployment-scoped so operators land on `/?page=audit&deployment={name}` instead of the global audit slice

## Quick links

The dashboard now exposes the following navigation affordances across the current drill-down flow:
- landing page: `_self` `Open audit log`, `_self` `Open account`, `_self` `Open application`
- account detail page: `Account JSON`, `Group JSON`, `_self` `Open application`
- application detail page: `_self` `Drift view`, `_self` `Plan/apply`, `_self` `Audit log`, `Open resource`
- application detail `Audit log` now routes to `/?page=audit&deployment={deploymentName}` so the selected deployment stays visible in the bookmarkable audit slice
- audit page: `_self` `Back to applications`, `_self` `Open application drill-down`, `_self` `Open plan/apply workspace`
- resource/workspace drill-down pages: embedded back links that keep operators in the same shell
- account-oriented JSON surfaces remain available through the API routes (`/v1/overview/accounts/{awsAccountId}`, `/v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, `/v1/overview/deployments/{deploymentName}`, `/v1/status/deployments/{deploymentName}`)

The landing page itself is backed by `GET /v1/overview/applications`; the application-detail page uses `GET /v1/overview/applications/{deploymentName}`, while the deeper resource/workspace drill-down pages continue to reuse `GET /v1/overview/deployments/{deploymentName}`.

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
