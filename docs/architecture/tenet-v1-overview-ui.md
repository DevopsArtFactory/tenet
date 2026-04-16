# Tenet Overview UI

Tenet now serves a minimal embedded UI at `/`.

## Purpose

The UI is intended to make the current control-plane slice visible without introducing a separate frontend toolchain.

It groups deployments by AWS account and then by project/environment within each account, and shows:
- account-header count pills for `deploymentCount`, `syncedCount`, `progressingCount`, `pausedCount`, and `errorCount`
- account topology summary text for unique project/environment counts, preferring additive account-level `projectCount` / `environmentCount` fields from `/v1/overview/accounts` when present and falling back to derivation from the visible deployments for older payloads
- per-group project/environment headers with the grouped region, deployment count, and additive grouped status pills derived from the deployments currently visible in that section
- deployment name and environment
- target type
- rollout strategy
- derived execution mode
- desired artifact
- deployed artifact
- additive latest-runtime summary text from `runtimeSummary.summary` plus runtime sync/health/rollout cues when present
- additive latest-runtime timestamp from `runtimeSummary.timestamp` with a `Latest runtime status` fallback label when no timestamp exists yet
- additive latest-runtime desired/observed generation metadata from `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration`
- additive latest-runtime change labels from `runtimeSummary.changes[]`
- additive latest-runtime provenance text from `runtimeSummary.source` when the summary was derived from controller-authored history
- rollout phase
- current rollout step
- planned rollout steps
- sync and health summaries
- expandable on-demand deployment detail panels for placement, sync reasons, rollout generations, rollout execution resources/descriptions, recent bounded runtime history, conditions, and the full rollout-step list

## Backing API

The page consumes joined overview endpoints:
- `GET /v1/overview/accounts`
- `GET /v1/overview/accounts/{awsAccountId}`
- `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`
- `GET /v1/overview/deployments/{deploymentName}`

`/v1/overview/accounts` keeps the top-level account page fast by returning the summary slice needed to render the default cards.

Each deployment object in that overview response now also includes an additive `runtimeSummary` object when recent runtime metadata exists. The current summary contract is intentionally compact and operator-facing:
- `runtimeSummary.timestamp`
- `runtimeSummary.summary`
- `runtimeSummary.sync.state`
- `runtimeSummary.sync.reason`
- `runtimeSummary.health.overall`
- `runtimeSummary.health.summary`
- `runtimeSummary.desiredGeneration`
- `runtimeSummary.observedGeneration`
- `runtimeSummary.changes[]` as additive latest-runtime change labels
- `runtimeSummary.source` when the newest summary was copied from controller-authored history
- `runtimeSummary.rolloutPhase`
- `runtimeSummary.currentStep`
- `runtimeSummary.historyCount`

The embedded UI renders that latest runtime summary/timestamp directly on the overview card using `document.createElement`, `textContent`, and `replaceChildren`; it also renders dedicated runtime-generation and latest-change rows from `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` and `runtimeSummary.changes[]` with the same safe DOM construction, includes `runtimeSummary.source` and latest change labels in the compact overview-card summary/search text when present, builds the account-header `Account JSON` link with `createElement('a')` plus an encoded `href` derived from `awsAccountId`, and does not inject runtime/account identifier strings with `innerHTML`.

Each account object now also includes additive per-account runtime counters:
- `deploymentCount`
- `syncedCount`
- `progressingCount`
- `pausedCount`
- `errorCount`

Each account overview object may also include additive topology counters:
- `projectCount`
- `environmentCount`

The `/` UI renders the account-header runtime counters directly from those fields when present. The same account header now also renders an `Account JSON` quick link to `GET /v1/overview/accounts/{awsAccountId}` as a sibling control next to the title/count summary rather than nesting it inside deployment-level expand toggles. Each grouped project/environment header likewise renders sibling additive grouped status pills plus a `Group JSON` quick link to `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` so operators can inspect the exact grouped scope already shown in the dashboard. Those grouped pills are derived from the deployments still visible in the section after client-side filtering and use the same bucket semantics as the account header: `Synced` contributes to synced, `Reconciling` / `OutOfSync` / `Drifted` contribute to progressing, `Paused` contributes to paused, and `Error` contributes to error. Zero-value group buckets are intentionally omitted so the compact grouped headers do not fill with empty counters. When any aggregate account-count field is absent in the overview payload, the embedded client derives the fallback value from `account.deployments[*].sync.state` instead of displaying zero for the status buckets. Deployments with missing or unknown runtime state still contribute to `deploymentCount`, so they remain visible in the total even when they do not increment a concrete runtime-state bucket.

For topology, the embedded client prefers authoritative account-level `projectCount` / `environmentCount` values when they are present on the overview payload. To preserve compatibility with older overview responses, it still falls back to deriving the unique project/environment counts from `account.deployments[*]` when either field is absent. Project/environment grouping for the nested deployment sections continues to come from deployment-level `appRef + environmentRef + region` data regardless of whether the authoritative account counters are present.

`/v1/overview/deployments/{deploymentName}` combines catalog placement data with the current deployment status snapshot for one deployment, including:
- the same additive `runtimeSummary` object used by the overview cards so API consumers can read a single latest-runtime contract without scraping `history[*]`
- account and region placement metadata
- `sync.reason`
- dedicated UI rendering of `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` as additive controller-owned runtime freshness metadata
- dedicated UI rendering of `runtimeSummary.changes[]` as additive latest-runtime change labels
- dedicated UI rendering of `runtimeSummary.source` as an additive `Runtime source` metric when the latest summary came from controller-authored history
- rollout desired/observed generations
- `history[*]` controller-authored runtime timeline entries (timestamp, summary, sync, health, desired/observed generations, optional `changes[]`, optional source, optional rollout phase, optional current step)
- `rollout.execution[*]`
- the complete `rollout.steps[*]` list
- `conditions[*]`

The embedded UI only fetches the deployment-detail endpoint when an operator expands a row.

The overview page also includes a client-side filter toolbar above the summary cards and per-deployment quick links under each card title:
- a text search that matches account/deployment-visible fields from the `/v1/overview/accounts` payload, including additive `runtimeSummary` text, latest change labels, provenance/source, and timestamp fields
- a sync-state filter
- an account filter
- an account-header `Account JSON` quick link that jumps to `/v1/overview/accounts/{awsAccountId}`
- a grouped-header `Group JSON` quick link that jumps to `/v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`
- direct `Overview JSON` and `Status JSON` links that jump to `/v1/overview/deployments/{deploymentName}` and `/v1/status/deployments/{deploymentName}`
- a `Clear filters` action that resets the toolbar to the default account-wide view

Toolbar state is URL-synced with `history.replaceState` using query params:
- `q` for free-text search
- `sync` for the selected sync-state bucket
- `account` for the selected stable account identifier (`awsAccountId` in the current embedded UI)

The page re-applies those query params on load so filtered views are shareable/bookmarkable without server-side filtering. Filtering remains client-side against the already-fetched overview payload; no additional overview requests are issued when controls change. Stale or invalid `sync`/`account` query values are normalized back to the default `all` selection before the toolbar renders, so bookmarked URLs cannot leave the control state and the active filter state out of sync. Matching on `runtimeSummary` is additive; it narrows the visible overview cards without changing lazy detail loading or detail-panel history behavior, and the matched summary text now includes `runtimeSummary.source` provenance plus `runtimeSummary.changes[]` labels when present. When the active filter combination removes every deployment, the UI renders the explicit empty state `No deployments match the current filters.`

Because the page re-renders the account/deployment cards as filters change, the embedded toolbar preserves focus and the active text-selection/caret position for the typed search field across incremental search re-renders. This keeps free-text filtering usable even though the view is rebuilt client-side.

Each deployment card still surfaces the most operator-useful rollout cues directly in the overview response/UI:
- `rollout.phase`
- `rollout.currentStep`
- `rollout.steps[*].kind`

The expandable detail panel extends that view without forcing operators to switch to the raw status endpoint for common troubleshooting. It now includes a safe runtime-generation row sourced from `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration`, a `Runtime changes` metric sourced from `runtimeSummary.changes[]`, a `Runtime source` metric sourced from `runtimeSummary.source` when present, plus a recent runtime-history section rendered with safe DOM construction, showing each controller-authored entry's timestamp, summary, sync state, health, desired/observed generations, optional `History changes`, optional `History source`, optional `History rollout phase`, and optional rollout step, plus an explicit `No recent runtime history is available yet.` empty state when no bounded history has been persisted yet.

The detail rollout-step list now also consumes additive `rollout.steps[*].status` metadata and controller-authored `rollout.steps[*].detail` text from the deployment-detail payload:
- `completed` marks controller-plan steps that are already behind the current plan view, including fully completed rollouts with no remaining current actionable step
- `current` marks the controller's current actionable step
- `pending` marks later planned steps that have not become current yet

When present, `rollout.steps[*].detail` is rendered alongside that status for the same row so the detail panel shows both the step's progress label and its explanatory detail text.

Those labels and detail texts are derived from the controller's current abstract rollout plan and execution mode and are intentionally not live AWS execution telemetry. The UI should present them as controller-authored progress cues for the planned rollout-step list, not as direct evidence that AWS resources have already finished the underlying operation.

The rollout-step detail text remains execution-mode-aware:
- EC2 canary/linear/full-shift rows explain candidate Auto Scaling Group bootstrap from userdata plus weighted target-group traffic progression, whether that progression is a small canary slice, linear percentage increases, or a single full cutover.
- ECS CodeDeploy rows explain CodeDeploy deployment creation, subsequent traffic shifting to the replacement task set, and the later ECS service-stability wait.
- Native ECS rolling rows explain the in-place ECS service update that creates the rolling deployment plus the later service-stability verification step.

The rollout execution section is intentionally execution-mode-aware:
- EC2 weighted target groups show the candidate Auto Scaling Group (`candidateAutoScalingGroup`), active target group (`activeTargetGroup`), candidate target group (`candidateTargetGroup`), production listener rule (`productionListenerRule`), traffic-shift plan (`trafficShiftPlan`), and userdata bootstrap source (`userDataBootstrapSource`).
- EC2 instance refresh shows the service Auto Scaling Group (`serviceAutoScalingGroup`) plus the userdata bootstrap source (`userDataBootstrapSource`) for replacement capacity.
- ECS rolling shows the ECS service that Tenet will update in place.
- ECS CodeDeploy shows the ECS service (`ecsService`), CodeDeploy application (`codeDeployApplication`), CodeDeploy deployment group (`codeDeployDeploymentGroup`), production listener (`productionListener`), test listener (`testListener`), blue target group (`blueTaskSetTargetGroup`), and green target group (`greenTaskSetTargetGroup`) used for blue/green traffic shifting.

As with rollout-step status/detail rendering, those execution-resource rows are controller-authored summaries of Tenet's intended rollout contract. The UI should not present them as live AWS telemetry; they describe the plan's concrete resource names and routing objects, not independently observed completion. The account overview and deployment-detail read models preserve the ordered `rollout.execution[]` payload without renaming resource identifiers, so the exact strings documented above stay aligned with what operators see in the UI and JSON routes.

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
