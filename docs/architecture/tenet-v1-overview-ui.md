# Tenet Overview UI

Tenet now serves a minimal embedded UI at `/`.

## Purpose

The UI is intended to make the current control-plane slice visible without introducing a separate frontend toolchain.

In the current drill-down transition slice, the default `/` page now behaves as an applications overview instead of the previous dense account/group dashboard. The landing page consumes `GET /v1/overview/applications`, a flattened joined read model that already carries application-level rows plus additive runtime/rollout/artifact metadata, and renders only:
- application name
- account
- project/app
- sync
- health
- last update
- a drill-down link to `/?page=account&accountId={awsAccountId}`
- a drill-down link to `/?page=application&deployment={deploymentName}`

The denser account/group/resource presentation is being split into deeper pages instead of remaining on the first screen. The next shipped page in that drill-down can now be the account-oriented `/?page=account&accountId={awsAccountId}` view, which reuses `GET /v1/overview/accounts/{awsAccountId}` to render grouped project/environment sections plus sibling `Account JSON`, `Group JSON`, and `Open application` links. The deployment-scoped `/?page=application&deployment={deploymentName}` page now reuses `/v1/overview/applications/{deploymentName}`, groups linked rollout resources into `Workloads`, `Traffic routing`, and `Config and access` sections, and acts as the entry point for the deeper `/?page=drift&deployment={deploymentName}` and `/?page=plan&deployment={deploymentName}` workspaces. Resource drill-down links use the stable route shape `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`, while the account-oriented joined overview routes remain available for account/group drill-downs and JSON quick links.

## Backing API

The page consumes joined overview endpoints:
- `GET /v1/overview/applications`
- `GET /v1/overview/applications/{deploymentName}`
- `GET /v1/overview/accounts`
- `GET /v1/overview/accounts/{awsAccountId}`
- `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`
- `GET /v1/overview/deployments/{deploymentName}`
- `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}`
- `POST /v1/plans/deployments/{deploymentName}`
- `GET /v1/plans/{planId}`

`/v1/overview/applications` now keeps the default applications landing page narrow while still preserving the richer additive joined fields (`runtimeSummary`, rollout metadata, artifacts, generations, conditions) already produced for deployment rows. `/v1/overview/accounts` remains the richer account-oriented joined route used for account/group drill-downs and sibling JSON quick links.

Each deployment object in that overview response now also includes an additive `runtimeSummary` object when recent runtime metadata exists. The current summary contract is intentionally compact and operator-facing:
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
- `runtimeSummary.changes[]` as additive latest-runtime change labels
- `runtimeSummary.changeDetails[]` as additive latest-runtime human-readable change explanations
- `runtimeSummary.deployedArtifact` as additive latest-runtime artifact context
- `runtimeSummary.conditions[]` as additive concise latest-runtime condition context
- `runtimeSummary.source` when the newest summary was copied from controller-authored history
- `runtimeSummary.rolloutPhase`
- `runtimeSummary.currentStep`
- `runtimeSummary.historyCount`

The embedded UI renders the latest runtime summary/timestamp directly on the applications landing page using `document.createElement`, `textContent`, and `replaceChildren`; it keeps that first screen focused on narrow application triage rather than adding dedicated runtime-generation, rollout-context, or latest-change rows per application. When present, `runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`, `runtimeSummary.source`, `runtimeSummary.changes[]`, `runtimeSummary.changeDetails[]`, `runtimeSummary.deployedArtifact`, and concise `runtimeSummary.conditions[]` summaries still contribute to compact runtime-summary/search text on the landing page. When absent, the search index skips placeholder `—` values so empty runtime context does not create noisy matches. Dedicated rollout-context, artifact, condition, and change-detail rows remain part of the deeper resource/workspace/history views. The UI does not inject runtime/account identifier strings with `innerHTML`.

Each account object now also includes additive per-account runtime counters:
- `deploymentCount`
- `syncedCount`
- `progressingCount`
- `pausedCount`
- `errorCount`

Each account overview object may also include additive topology counters:
- `projectCount`
- `environmentCount`

The account-oriented `/v1/overview/accounts` read model still carries those account-header runtime counters when present. The same account payload still exposes an `Account JSON` quick link to `GET /v1/overview/accounts/{awsAccountId}` and grouped project/environment headers can still surface sibling additive grouped status pills plus a `Group JSON` quick link to `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` on account-focused views. Those grouped pills use the same bucket semantics as the account header: `Synced` contributes to synced, `Reconciling` / `OutOfSync` / `Drifted` contribute to progressing, `Paused` contributes to paused, and `Error` contributes to error. Zero-value group buckets are intentionally omitted so compact grouped headers do not fill with empty counters. When any aggregate account-count field is absent in the overview payload, the embedded client derives the fallback value from `account.deployments[*].sync.state` instead of displaying zero for the status buckets. Deployments with missing or unknown runtime state still contribute to `deploymentCount`, so they remain visible in the total even when they do not increment a concrete runtime-state bucket.

For topology, the embedded client prefers authoritative account-level `projectCount` / `environmentCount` values when they are present on the overview payload. To preserve compatibility with older overview responses, it still falls back to deriving the unique project/environment counts from `account.deployments[*]` when either field is absent. Project/environment grouping for the nested deployment sections continues to come from deployment-level `appRef + environmentRef + region` data regardless of whether the authoritative account counters are present.

`/v1/overview/deployments/{deploymentName}` combines catalog placement data with the current deployment status snapshot for one deployment, including:
- additive `sourceFingerprint` derived from the same rollout/runtime/detail inputs used by plan-preview creation so operators can send optimistic `ifMatch` on the first create call without first creating a preview
- the same additive `runtimeSummary` object used by the overview cards so API consumers can read a single latest-runtime contract without scraping `history[*]`
- account and region placement metadata
- `sync.reason`
- dedicated UI rendering of `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` as additive controller-owned runtime freshness metadata
- dedicated UI rendering of `runtimeSummary.changes[]` as additive latest-runtime change labels
- dedicated UI rendering of `runtimeSummary.changeDetails[]` as additive latest-runtime change-detail text on detail/workspace surfaces
- dedicated UI rendering of `runtimeSummary.deployedArtifact` as an additive `Runtime artifact` metric on the resource-detail page and deployment workspaces
- dedicated UI rendering of concise `runtimeSummary.conditions[]` summaries as an additive `Runtime conditions` metric on the resource-detail page and deployment workspaces
- dedicated UI rendering of `runtimeSummary.source` as an additive `Runtime source` metric when the latest summary came from controller-authored history
- optional normalized `userData` metadata from the deployment catalog so EC2 fallback rollout resources keep stable identities when canonical `rollout.execution[]` data is missing or empty
- rollout desired/observed generations
- `history[*]` controller-authored runtime timeline entries (timestamp, summary, sync, health, desired/observed generations, optional `targetType` / `strategy` / `executionMode`, optional `changes[]`, optional `changeDetails[]`, optional source, optional rollout phase, optional current step)
- `rollout.execution[*]`
- the complete `rollout.steps[*]` list
- `conditions[*]`

The embedded UI fetches `GET /v1/overview/applications/{deploymentName}` for `?page=application`, while `?page=resource`, `?page=drift`, and `?page=plan` continue to fetch `GET /v1/overview/deployments/{deploymentName}`. The default `/` landing page is the only page that loads `/v1/overview/applications`; deeper drill-down pages no longer depend on `/v1/overview/accounts` just to render detail. The application-detail route intentionally preserves deployment-detail semantics for shared fields such as `sourceFingerprint`, so consumers can hand the same optimistic-match token into plan-preview creation regardless of which detail route they started from.

The current applications overview page includes a client-side filter toolbar above the application list:
- a text search matching application/account/project/runtime summary text from `/v1/overview/applications`
- a sync-state filter
- an account filter
- an `Open account` drill-down link on each application row
- an `Open application` drill-down link on each application row
- a `Clear filters` action that resets the toolbar

The new embedded account page at `/?page=account&accountId={awsAccountId}` fetches `GET /v1/overview/accounts/{awsAccountId}` directly, preserves the active toolbar query params in its back link to the applications page, and renders the existing account/group-oriented joined overview with grouped project/environment sections plus sibling `Account JSON`, `Group JSON`, and `Open application` links outside the expandable row toggle. The older direct workspace shortcuts are intentionally removed from these account rows so the deployment workspaces are entered from the application detail page instead.

The older account/group JSON quick links and denser deployment quick links are no longer part of the default first page; they remain available on the account-oriented drill-down page and other deeper deployment contexts.

Toolbar state is URL-synced with `history.replaceState` using query params:
- `q` for free-text search
- `sync` for the selected sync-state bucket
- `account` for the selected stable account identifier (`awsAccountId` in the current embedded UI)

Those deployment workspace links intentionally stay inside the same embedded HTML shell rather than introducing new server-rendered pages. `?page=drift` and `?page=plan` switch the client into deployment-scoped read-only workspaces after the application-detail/deployment fetch flow, then lazy-load `/v1/overview/deployments/{deploymentName}` to render deployment context plus read-only troubleshooting/action sections. Both workspaces now also render a dedicated `Latest runtime signals` section directly in the shell using `runtimeSummary.source`, `runtimeSummary.targetType`, `runtimeSummary.strategy`, `runtimeSummary.executionMode`, `runtimeSummary.timestamp`, `runtimeSummary.historyCount`, additive `runtimeSummary.deployedArtifact`, concise `runtimeSummary.conditions[]` summaries, and additive `runtimeSummary.changeDetails[]`, followed by explicit runtime change-signal rows sourced from `runtimeSummary.changes[]`. Both workspaces also render a dedicated `Rollout execution` section from `detail.rollout.execution[]`, preserving the same ordered execution-resource identifiers documented for the detail panel. The drift workspace now derives a real read-only `Field-level drift` table from the same deployment-detail payload: it compares desired artifact/generation/runtime intent against current runtime signals, emits severity-tagged rows for true unhealthy/drift conditions, and lets operators sort/filter the derived rows entirely client-side without introducing new backend APIs. The plan/apply workspace keeps `Apply Plan` disabled, but it now uses `Create Plan` to call `POST /v1/plans/deployments/{deploymentName}` with an `ifMatch` fingerprint derived from the currently reviewed deployment detail whenever the client can do so, falling back to the loaded preview `source.fingerprint` when it cannot, then renders the returned server-authored immutable preview and rewrites the workspace URL to `?page=plan&deployment={deploymentName}&planId={planId}`. When that `planId` query param is present, the same workspace loads `GET /v1/plans/{planId}` and renders ready/failed/unavailable/fetch-error states around the stored preview while still keeping deployment-derived risk/operation context visible whenever no preview exists or preview retrieval fails. The workspace back link now returns to the application detail page so the drill-down path stays linear while URLs remain bookmarkable.

Each plan-preview record is intentionally narrow and read-only:
- stable `id` / `planId` derived from the immutable preview payload
- `status` (`ready` / `failed` today, with `pending` reserved for later async generation)
- server-authored `operations[]` rows with step kind, step status, rollout phase, target resource, and operator-facing summary text
- derived `risk` and `approval` blocks that the UI can render directly without recomputing heuristics client-side
- `source` context containing the deployment-detail route, deterministic source fingerprint, runtime/rollout context, and desired/observed generations used to detect stale plan input
- explicit `errors[]` when the server persists a failed preview (for example an optimistic `ifMatch` fingerprint that no longer matches the current deployment detail)

The current `POST /v1/plans/deployments/{deploymentName}` request body is optional and supports one additive optimistic field:
- `ifMatch`: `sourceFingerprint` from a previously fetched deployment detail, application detail, or loaded plan preview; the embedded client prefers the currently reviewed detail fingerprint when it can derive one and otherwise falls back to the loaded preview fingerprint. When that token no longer matches the current deployment detail, Tenet persists a `failed` preview instead of a vague server error so operators and future UI polling can tell that the preview went stale

Plan-preview persistence is local, explicit, and compatible with fixture-backed runs: `tenet-api` stores immutable plan previews in `TENET_PLAN_STORE_PATH` (default `${TMPDIR:-/tmp}/tenet/plan-previews.json`) using an atomic filesystem JSON write. Repeating the same create call against unchanged source inputs returns the existing immutable record instead of mutating it in place.

The page re-applies those query params on load so filtered views are shareable/bookmarkable without server-side filtering. Filtering remains client-side against the already-fetched applications payload; no additional overview requests are issued when controls change. Stale or invalid `sync`/`account` query values are normalized back to the default `all` selection before the toolbar renders, so bookmarked URLs cannot leave the control state and the active filter state out of sync. Matching on `runtimeSummary` is additive; it narrows the visible application rows without changing deeper drill-down history behavior, and the matched summary text now includes `runtimeSummary.source` provenance plus `runtimeSummary.changes[]` labels when present. When the active filter combination removes every application, the UI renders the explicit empty state `No applications match the current filters.`

Because the page re-renders the application rows as filters change, the embedded toolbar preserves focus and the active text-selection/caret position for the typed search field across incremental search re-renders. This keeps free-text filtering usable even though the view is rebuilt client-side.

Each application row still surfaces the most operator-useful rollout/runtime cues directly in the landing-page read model/UI:
- `sync.state`
- `health.overall`
- `runtimeSummary.timestamp`
- `runtimeSummary.summary`
- drill-down navigation to `/?page=application&deployment={deploymentName}`

The deeper application/resource/workspace pages extend that view without forcing operators to switch to the raw status endpoint for common troubleshooting. The resource-detail page includes a safe runtime-generation row sourced from `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration`, a `Runtime change details` metric sourced from `runtimeSummary.changeDetails[]`, a `Runtime source` metric sourced from `runtimeSummary.source` when present, additive `Runtime artifact` / `Runtime conditions` metrics sourced from `runtimeSummary.deployedArtifact` / `runtimeSummary.conditions`, plus recent runtime-history rendering with safe DOM construction, showing each controller-authored entry's timestamp, summary, sync state, health, desired/observed generations, optional `History context`, optional `History changes`, optional `History change details`, optional `History artifact`, optional `History conditions`, optional `History source`, optional `History rollout phase`, and optional rollout step, plus an explicit `No recent runtime history is available yet.` empty state when no bounded history has been persisted yet. The same resource-detail page now also lazy-loads a `Recent logs` section from `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}` using the currently selected resource type/name pair from the route. That section keeps its own explicit `Loading recent logs…`, unavailable, empty, and populated states and is rendered with safe DOM construction only. The deployment workspaces reuse that same safe DOM construction pattern for their `Latest runtime signals` and `Rollout execution` sections, including dedicated rollout-context rows sourced from `runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`, additive artifact/condition rows sourced from `runtimeSummary.deployedArtifact` / `runtimeSummary.conditions`, additive `Runtime change details` rows sourced from `runtimeSummary.changeDetails[]`, and explicit empty states when `runtimeSummary.changes[]` or `rollout.execution[]` are absent.

The current resource-log route intentionally supports only the resource types backed by the new read-only provider:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`

The local verification path now explicitly exercises the ECS rolling route shape with `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`, asserting the same stable `available`, `entries`, `source`, and `unavailableReason` JSON contract used for other resource-log checks.

Other resource types remain visible as resource-detail pages, but their `Recent logs` section renders an explicit unavailable state rather than inventing log data. The same unavailable contract is used when AWS credentials, assume-role access, task/container log configuration, running tasks/instances, or EC2 console-output access are not available from the current API process.

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

To preserve the application/resource drill-down contract for older fixture/local payloads, the UI synthesizes fallback execution resources when `detail.rollout.execution[]` is missing or empty. When the detail payload already includes `rollout.execution[]` as a non-empty array, the controller-authored array is used verbatim.

The synthesized fallback set is intentionally restricted to the controller's canonical rollout contracts and is kept byte-for-byte aligned with the controller/API fallback text:
- `asgWeightedTargetGroups`
  - `candidateAutoScalingGroup` → `{deployment}-candidate` → config-aware text that preserves the same resource identity while naming the normalized userdata source plus candidate desired/min/max capacity and warmup when configured
  - `activeTargetGroup` → `{deployment}-active` → `Currently active target group receiving production traffic`
  - `candidateTargetGroup` → `{deployment}-candidate` → `Candidate target group Tenet will register and warm before traffic shifting`
  - `productionListenerRule` → `{deployment}-prod-listener` → `Production listener rule operators inspect while Tenet updates weighted forwarding during rollout`
  - `trafficShiftPlan` → `{deployment}-weighted-routing` → strategy-aware text that differentiates canary vs linear vs single-cutover behavior and appends rollout qualifiers when present
  - `userDataBootstrapSource` → normalized from `userData` as `{type}:{path}`, `{type}`, `{path}`, or `unspecified` → `Normalized userdata bootstrap source for candidate EC2 capacity`
- `asgInstanceRefresh`
  - `serviceAutoScalingGroup` → `{deployment}-service` → `Service Auto Scaling Group Tenet refreshes in place`, extended to `... with rolling-update batch size N` when `rolloutConfig.stacks[*].rollingUpdateInstanceCount` is present
  - `userDataBootstrapSource` → normalized from `userData` as `{type}:{path}`, `{type}`, `{path}`, or `unspecified` → `Normalized userdata bootstrap source for refreshed EC2 instances`
- `ecsRolling`
  - `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s` when `rolloutConfig.strategy.minHealthyPercent` and/or `rolloutConfig.strategy.warmupSeconds` are present on the detail payload
- `ecsCodeDeploy`
  - each fallback description keeps the same `{deployment}-...` resource name but now includes the active traffic-shift style (`blueGreen`, `canary`, or `linear`) and any rollout qualifiers that the detail payload carries

Those fallback names/descriptions intentionally match the controller-authored `rollout.execution[]` metadata so the UI computes the same canonical `resourceId` values and existing bookmarked resource-detail routes stay stable across controller-authored and fixture-backed responses even as description text becomes more config-aware.

## Detail-panel rendering constraints

The UI remains static embedded HTML/CSS/JS and preserves safe DOM construction:
- detail content is rendered with `document.createElement`, `textContent`, and `replaceChildren`
- recent-log content is rendered with `document.createElement`, `textContent`, and `replaceChildren`
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
