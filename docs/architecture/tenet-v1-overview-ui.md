# Tenet Overview UI

Tenet now serves a minimal embedded UI at `/`.

## Purpose

The UI is intended to make the current control-plane slice visible without introducing a separate frontend toolchain.

It groups deployments by AWS account and shows:
- account-header count pills for `deploymentCount`, `syncedCount`, `progressingCount`, `pausedCount`, and `errorCount`
- deployment name and environment
- target type
- rollout strategy
- derived execution mode
- desired artifact
- deployed artifact
- rollout phase
- current rollout step
- planned rollout steps
- sync and health summaries
- expandable on-demand deployment detail panels for placement, sync reasons, rollout generations, rollout execution resources/descriptions, conditions, and the full rollout-step list

## Backing API

The page consumes joined overview endpoints:
- `GET /v1/overview/accounts`
- `GET /v1/overview/deployments/{deploymentName}`

`/v1/overview/accounts` keeps the top-level account page fast by returning the summary slice needed to render the default cards.

Each account object now also includes additive per-account runtime counters:
- `deploymentCount`
- `syncedCount`
- `progressingCount`
- `pausedCount`
- `errorCount`

The `/` UI renders those counters directly in each account header. When any aggregate account-count field is absent in the overview payload, the embedded client derives the fallback value from `account.deployments[*].sync.state` instead of displaying zero for the status buckets. Deployments with missing or unknown runtime state still contribute to `deploymentCount`, so they remain visible in the total even when they do not increment a concrete runtime-state bucket.

`/v1/overview/deployments/{deploymentName}` combines catalog placement data with the current deployment status snapshot for one deployment, including:
- `sync.reason`
- account and region placement metadata
- rollout desired/observed generations
- `rollout.execution[*]`
- `conditions[*]`
- the complete `rollout.steps[*]` list

The embedded UI only fetches the deployment-detail endpoint when an operator expands a row.

The overview page also includes a client-side filter toolbar above the summary cards:
- a text search that matches account/deployment-visible fields from the `/v1/overview/accounts` payload
- a sync-state filter
- an account filter
- a `Clear filters` action that resets the toolbar to the default account-wide view

Toolbar state is URL-synced with `history.replaceState` using query params:
- `q` for free-text search
- `sync` for the selected sync-state bucket
- `account` for the selected stable account identifier (`awsAccountId` in the current embedded UI)

The page re-applies those query params on load so filtered views are shareable/bookmarkable without server-side filtering. Filtering remains client-side against the already-fetched overview payload; no additional overview requests are issued when controls change. Stale or invalid `sync`/`account` query values are normalized back to the default `all` selection before the toolbar renders, so bookmarked URLs cannot leave the control state and the active filter state out of sync. When the active filter combination removes every deployment, the UI renders the explicit empty state `No deployments match the current filters.`

Because the page re-renders the account/deployment cards as filters change, the embedded toolbar preserves focus and the active text-selection/caret position for the typed search field across incremental search re-renders. This keeps free-text filtering usable even though the view is rebuilt client-side.

Each deployment card still surfaces the most operator-useful rollout cues directly in the overview response/UI:
- `rollout.phase`
- `rollout.currentStep`
- `rollout.steps[*].kind`

The expandable detail panel extends that view without forcing operators to switch to the raw status endpoint for common troubleshooting.

The rollout execution section is intentionally execution-mode-aware:
- EC2 weighted target groups show the candidate Auto Scaling Group, active target group, candidate target group, and userdata bootstrap source.
- EC2 instance refresh shows the service Auto Scaling Group plus the userdata bootstrap source for replacement capacity.
- ECS rolling shows the ECS service that Tenet will update in place.
- ECS CodeDeploy shows the ECS service, CodeDeploy application, and CodeDeploy deployment group used for traffic shifting.

## Detail-panel rendering constraints

The UI remains static embedded HTML/CSS/JS and preserves safe DOM construction:
- detail content is rendered with `document.createElement`, `textContent`, and `replaceChildren`
- manifest/status-derived values are not injected with `innerHTML`
- loading and error states are scoped to the expanded deployment row

## Why account-first grouping

The product direction is a multi-account control plane. Grouping by account keeps the local demo aligned with the eventual operating model and makes it easy to compare deployments across environments within the same account boundary.

## Current scope

The UI is intentionally small:
- static HTML/CSS/JS embedded into the Go binary
- no Node build
- no mutations or write APIs

Future slices can replace or extend this with a richer frontend once the runtime API stabilizes.
