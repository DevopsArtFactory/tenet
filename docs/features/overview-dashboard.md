# Tenet Overview Dashboard

## Purpose

The embedded Tenet dashboard is the current operator-facing entry point for the control-plane prototype.

The UX direction is now a simpler ArgoCD-style drill-down instead of a single dense dashboard. The intended navigation is:
- applications overview first
- application-linked resources second
- concrete resource detail last

The implementation remains fully embedded HTML/CSS/JS with no separate frontend toolchain.

The implementation remains static embedded HTML/CSS/JS with no separate frontend toolchain.

## What shipped so far

### 1. Applications overview landing page

The dashboard at `/` now starts from `GET /v1/overview/accounts`, but the client flattens that payload into an applications overview instead of showing the full account/group dashboard by default.

The first page now focuses on:
- application name
- account
- project/app
- sync
- health
- last update

This keeps the first page optimized for triage instead of deep inspection.

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
- `view`
- `deployment`

Current behavior:
- `account` uses stable `awsAccountId`
- invalid `sync` and `account` values normalize to `all`
- unrelated query params/hash fragments are preserved when updating managed filters
- search input focus and caret position survive rerenders
- when filters remove all results, the UI renders `No deployments match the current filters.`

## Current pages

### Applications overview

The first page now shows only application-level rows plus search/filter controls.

Each row surfaces:
- application name
- account
- project/app
- sync
- health
- last update
- `Open application` link to `/?page=application&deployment={deploymentName}`

### Application detail placeholder

Slice 1 also introduces the application-detail route target so the applications page can drill down into the next layer.

Current behavior:
- loads `GET /v1/overview/deployments/{deploymentName}`
- renders a breadcrumb/back-link target
- intentionally keeps grouped resource lists for the next slice

### Deployment workspaces

The existing `Drift view` and `Plan/apply` workspace shells remain available as deeper read-only troubleshooting pages while the new application/resource drill-down flow is being introduced incrementally.

## Workspace shells

The dashboard now supports lightweight workspace routing inside the same embedded shell.

Routes:
- `/?view=drift&deployment={name}`
- `/?view=plan&deployment={name}`

### Drift workspace

Current drift workspace behavior:
- reuses the embedded shell
- lazy-loads deployment detail
- shows deployment context and status conditions
- explicitly states that field-level drift rows are not yet available in the read model

### Plan/apply workspace

Current plan/apply workspace behavior:
- reuses the embedded shell
- shows rollout-plan steps from the deployment detail route
- displays disabled `Create Plan` / `Apply Plan` buttons
- explicitly states that mutation APIs are not wired yet

## Quick links

The dashboard now exposes the following navigation affordances:
- `Account JSON`
- `Group JSON`
- `Overview JSON`
- `Status JSON`
- `Drift view`
- `Plan/apply`

Important implementation constraint:
- these links are rendered as sibling controls outside the expand-toggle button
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
- `runtimeSummary.changes[]`
- `runtimeSummary.source`
- `runtimeSummary.rolloutPhase`
- `runtimeSummary.currentStep`
- `runtimeSummary.historyCount`

UI rendering rules:
- safe DOM construction only (`createElement`, `textContent`, `replaceChildren`)
- no `innerHTML` for manifest/status-backed values
- missing generation fields render as `—`, not `0`
- latest change labels are additive string arrays, not objects

## Testing and verification

Current coverage includes:
- Go string-fragment tests in `internal/api/ui_test.go`
- Node-backed behavior tests in `internal/api/ui_behavior_test.go`
- script syntax validation with `node --check`
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
- richer drift rows once the read model exposes field-level drift
- real plan/apply mutations once write APIs exist
- account/group summary sparklines or event cues
- better runtime history navigation and event timelines
- eventual extraction into a dedicated frontend only if the current read-model/UI surface becomes too large for embedded HTML
