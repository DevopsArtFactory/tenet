# Tenet Overview Dashboard

## Purpose

The embedded Tenet dashboard is the current operator-facing entry point for the control-plane prototype.

The UX direction is now a simpler ArgoCD-style drill-down instead of a single dense dashboard. The intended navigation is:
- applications overview first
- application-linked resources second
- concrete resource detail last

The implementation remains fully embedded HTML/CSS/JS with no separate frontend toolchain.

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
- grouped resource sections:
  - `Workloads`
  - `Traffic routing`
  - `Config and access`
- `Open resource` links targeting `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`
- keeps the deeper artifact/history/generation/log drill-down on the final resource page

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

### Audit timeline page

The embedded UI now exposes a dedicated bookmarkable audit route:
- `/?page=audit`
- `/?page=audit&limit={n}` to request a smaller newest-first window from `GET /v1/audit-logs`

Current behavior:
- fetches `GET /v1/audit-logs` directly
- renders explicit `Loading audit log…`, empty, unavailable (`503`), and generic fetch-error states
- keeps entries newest-first even if the client receives an unsorted payload
- shows explicit labels for action, deployment/app/environment context, timestamp, actor availability, and plan/apply identifiers
- keeps actor identity explicit rather than fabricated by rendering `Unavailable` / `Unknown` plus the server-authored reason text
- exposes `_self` `Open application` and `_self` `Open plan/apply` links when `deploymentName`, `planId`, and `applyId` are present

### Deployment runtime events API

Operators can also fetch a read-only deployment event timeline from:
- `GET /v1/overview/deployments/{deploymentName}/events`

This payload is derived from persisted `status.history` entries the controller authored for runtime changes. It is meant to support operator drill-down and timeline rendering, not to act as a full durable audit system yet.

Separate from that controller-runtime timeline, the backend also exposes `GET /v1/audit-logs` as a local immutable audit projection over persisted plan preview and apply request records. The dedicated `/?page=audit` view now renders that route and keeps `actor.available=false` explicit until operator identity is actually persisted.

Current behavior:
- returns `deploymentName` plus an explicit `events[]` array
- returns `200 OK` with `events: []` when the deployment exists but no runtime history has been persisted yet
- never calls live AWS APIs; it only reflects the stored controller/runtime read model
- the application detail page consumes the same route directly instead of reusing `history[]` from the deeper deployment-detail payload

## Workspace shells

The dashboard now supports lightweight workspace routing inside the same embedded shell.

Routes:
- `/?page=audit`
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
- derives a real read-only `Field-level drift` table from the deployment-detail payload, comparing desired artifact/generation/runtime intent against current runtime signals and surfacing unhealthy or drift conditions with client-side severity filtering/sorting

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
- loads `GET /v1/applies/{applyId}` whenever an `applyId` query param is present and renders explicit loading, created/ready, failed, unavailable, and generic fetch-error states without hiding deployment-derived or plan-derived fallback context
- renders stored immutable apply metadata plus `Open bookmarked apply` (`_self`) and `Apply JSON` links, and explicitly tells operators that live execution remains disabled by default so failed immutable apply records are expected and useful
- returns to the application detail page through the embedded back link so the drill-down path stays tight

## Quick links

The dashboard now exposes the following navigation affordances across the current drill-down flow:
- landing page: `_self` `Open audit log`, `_self` `Open account`, `_self` `Open application`
- account detail page: `Account JSON`, `Group JSON`, `_self` `Open application`
- application detail page: `_self` `Drift view`, `_self` `Plan/apply`, `_self` `Audit log`, `Open resource`
- audit page: `_self` `Back to applications`, `_self` `Open application`, `_self` `Open plan/apply`
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
