# Tenet

Tenet is a repository-driven deployment control plane for multi-account
environments. It gives operators one place to compare desired deployment intent,
runtime status, rollout progress, and resource-level evidence for EC2 and ECS
targets.

Tenet is early-stage and intentionally conservative. The current product is
mostly read-only: it focuses on catalog loading, runtime read models, immutable
plan/apply records, auditability, and an embedded operator UI before live AWS
mutation is enabled.

## Product Overview

Tenet helps operators:

- load desired state from a local repository checkout or a GitHub-backed
  manifest tree
- compile account, environment, deployment, artifact, and rollout intent into a
  validated catalog
- join desired state with runtime status, audit, plan/apply, recent-log, and
  resource-inspection read models
- inspect applications, accounts, deployments, rollout workspaces, and resources
  from a single embedded UI
- troubleshoot front-door auth, deployment access, desired-state refresh
  readiness, and runtime evidence with explicit diagnostics

The primary UI flow is intentionally simple, and the embedded shell's initial loading copy now names that Applications overview entry point: Applications overview → Application detail → Resource detail.

Additional operator workspaces are linked from that flow: account detail, drift,
plan/apply, audit timeline, deployment events, recent logs, and live inspection.
Drift clear actions count active query/severity/sort filters and name reset state in assistive labels/tooltips.
Resource-detail quick navigation keeps `_self` links back to the Applications
and Application detail pages before operators pivot into log or inspection
evidence, while destination/section chips and hash jumps keep keyboard and screen
reader orientation aligned with the visual shell.
The plan/apply workspace keeps disabled mutation controls explanatory: `Create Plan`
reviews the current deployment, `Apply Plan` / refreshes activate after stored
records load, and optional executor claim metadata (`claim.workerId`,
`claim.claimedAt`, `claim.lastHeartbeatAt`, `claim.expiresAt`, and legacy
`claim.freshness.*`) shows local-safe ownership and lease health without raw
JSON. Queued/running records without claim metadata show pending executor-claim copy; failed/default records omit that section. If a reviewed plan is stale, the
workspace preserves bookmarkable context and tells the operator to create a fresh
plan preview before retrying.

## Architecture

Tenet currently has four main layers.

### Desired-State Loading

`tenet-api` and `tenet-controller` read the same repository-shaped manifest tree.
Set `TENET_CATALOG_SOURCE=filesystem` with `TENET_REPO_ROOT` for local
checkouts, or `TENET_CATALOG_SOURCE=github` with `TENET_CATALOG_GITHUB_*` for
read-only GitHub catalog loading.

### Catalog Compilation

The loader validates manifests into a compiled catalog covering accounts,
environments, placements, deployments, rollout strategy, and desired artifacts.
The compiled catalog is the stable desired-state model behind `/v1/catalog/*`
and the overview read models.

### Runtime State

Runtime status is stored separately from desired configuration. Local runs can use
filesystem-backed status, plan, apply, and audit stores; Compose can also use the
Redis-backed local stack. Runtime state represents sync, health, rollout
progress, bounded history, audit entries, and immutable plan/apply records.

### API And Embedded UI

The Go API joins catalog and runtime state into operator read models. The
embedded UI is static HTML/CSS/JS served by `tenet-api`, so local development and
Compose smoke checks do not require a separate frontend build.

## Run Locally

Run `tenet-api` against the example repository:

```bash
TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Open:

```text
http://localhost:18083/
```

For a GitHub-backed desired-state catalog:

```bash
TENET_CATALOG_SOURCE=github \
TENET_CATALOG_GITHUB_REPOSITORY=acme/platform-config \
TENET_CATALOG_GITHUB_REF=main \
TENET_CATALOG_GITHUB_PATH=tenet \
TENET_CATALOG_GITHUB_TOKEN=*** \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Useful local probes:
```bash
curl -fsS http://localhost:18083/v1/catalog/summary
curl -fsS http://localhost:18083/v1/overview/applications
curl -fsS http://localhost:18083/v1/status/deployments
curl -fsS http://localhost:18083/v1/operator/context
```

## Run With Compose

Start the local stack:

```bash
make docker-up
```

Then open:

```text
http://localhost:8080/
```

Stop it with:

```bash
make docker-down
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already
owned by something outside this Compose project. Use:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
make docker-down
docker compose up --build -d
```

The local-safe apply executor remains opt-in:

```bash
TENET_AWS_ENABLE_APPLY=true make docker-up
```

`TENET_AWS_ENABLE_APPLY=true` queues local-safe apply handoff metadata only; queued apply creation requires no live AWS credentials and does not perform live AWS mutation. The plan/apply workspace and read-only applies monitor surface `Apply execution readiness` copy from backend readiness evidence when present, and pair it with a low-noise `Next safe action` cue for queued, running, stale-claim, blocked-preflight, and disabled/default apply records. The fallback copy calls out that disabled/default apply records are expected local-safe evidence, not a product failure, and prevents operators from confusing metadata-only handoffs with live AWS execution. The applies monitor also summarizes `Queued applies`, `Lifecycle signals`, `Operation signals`, and `Claim signals` for the selected queued/active/stale collection so pending or stale handoffs are visible before an operator opens a row. When a stored apply is open, the plan/apply workspace also exposes the standalone non-mutating `GET /v1/applies/{applyId}/preflight` read route with JSON/copy handoffs and a manual `Refresh apply preflight` action that preserves the bookmarkable `deployment` / `planId` / `applyId` URL. It also reads `GET /v1/applies/{applyId}/execution-preview` into a dedicated `Apply execution preview` section with read-only/no-AWS-mutation copy, first-action compatibility metadata plus the full ordered execution-preview `operations[]` inventory when present, AWS context/blockers, and a manual `Refresh execution preview` action for reviewer/operator evidence without claiming executor progress. When the target account has a configured deployer `roleArn`, that preflight read may run only a read-only STS identity probe for the configured role and reports `deployerRole.configured`, `checkedWithAws`, `ready`, `roleArn`, `awsAccountId`, `region`, and explicit `missing_deployer_role` / `aws_unavailable` blockers; it never creates CodeDeploy deployments, shifts ECS/ALB traffic, or mutates ASGs. Preflight capability rows may also include read-only IAM `SimulatePrincipalPolicy` permission evidence (`checkedWithAws`, `allowed`, and `allowed` / `denied` / `unavailable` states) when AWS provider access is available; local/no-AWS runs still keep the route successful with explicit unavailable evidence. A compact `Apply preflight next action` summary appears above the detailed rows after preflight evidence loads: blocked preflight chooses the first blocked operation and reuses its safe handoffs, while ready preflight shows calm local-safe handoff copy without implying live AWS mutation succeeded. Per-operation preflight rows include additive `capabilityChecks[]` matrix entries (`action`, `resourceType`, `resourceName`, `requiredRole`, `state`, `reason`, and `summary`) for deterministic CodeDeploy/ECS and ASG/EC2 readiness review; the embedded plan/apply workspace renders these as low-noise permission readiness labels (`Permission allowed`, `Permission denied`, `Permission unchecked`, or `Permission unavailable`) instead of requiring operators to open raw JSON, plus troubleshooting handoffs (`deploymentDetailRoute`, `planApplyWorkspaceRoute`, `resourceInspectionRoute`, and `resourceLogsRoute`) so blocked rows point operators to the read-only deployment, workspace, live-inspection, and recent-log evidence they should inspect next. These checks are readiness evidence only and stay `blocked` for live mutation until a future executor implements the named AWS actions. Known preflight blocker codes for missing deployer-role configuration, AWS/provider unavailability, and failed assume-role probes render as human next-step labels instead of raw machine reasons.

Local-safe apply executor release guard: reviewers should verify the active collection (`GET /v1/applies?status=active`), stale active-claim collection (`GET /v1/applies?status=active&claim=stale`), queued exact subset (`GET /v1/applies?status=queued`), running exact subset (`GET /v1/applies?status=running`), terminal exact subsets (`GET /v1/applies?status=failed`, `GET /v1/applies?status=succeeded`, and `GET /v1/applies?status=cancelled`), stored apply read (`GET /v1/applies/{applyId}`), non-mutating preflight read (`GET /v1/applies/{applyId}/preflight`), read-only execution preview (`GET /v1/applies/{applyId}/execution-preview`), claim (`POST /v1/applies/{applyId}/claim`), heartbeat (`POST /v1/applies/{applyId}/heartbeat`), and transition (`POST /v1/applies/{applyId}/transitions`) routes together.
Operation rows in `GET /v1/applies/{applyId}` now expose additive `executionAttempt` metadata only when a claim, heartbeat, transition, or local-safe executor event touches that operation. The object includes `workerId`, `status`, `startedAt`, `lastHeartbeatAt`, optional `finishedAt`, `reason`, and a safety `summary`; legacy rows omit `executionAttempt` entirely, and the metadata is Tenet control-plane evidence, not proof of live AWS mutation.
That mode advances Tenet apply metadata only; this local-safe record lifecycle does not perform CodeDeploy, ECS, ALB, ASG, or other live AWS mutation. Local and Docker smoke cover the active-vs-queued/running apply collection split, stale-claim refinement, and terminal exact subsets: `GET /v1/applies?status=active` returns queued plus running records, `GET /v1/applies?status=active&claim=stale` returns running records whose claim lease is stale or expired, `GET /v1/applies?status=running` returns only claimed/in-flight running records, a claimed heartbeat record carries freshness fields, `GET /v1/applies?status=queued` stays the queued-only subset, and `GET /v1/applies?status=succeeded` / `GET /v1/applies?status=cancelled` return only matching terminal records. Terminal lifecycle filters (`GET /v1/applies?status=succeeded|failed|cancelled`) return immutable records in that final state so operators can inspect completed or failed handoffs without widening the executor queue views. The read-only `/?page=applies` monitor defaults to the queued subset read, adds explicit queued/active/stale view controls (`?appliesView=active` for queued plus running handoffs, `?appliesView=stale` for stale active claims), preserves the manual `Refresh applies` behavior and bookmarkable URL, translates claim heartbeat/lease metadata into fresh/stale/pending operator copy, lets operators load a row-local non-mutating execution preview (`GET /v1/applies/{applyId}/execution-preview`) without changing the selected `appliesView`, links operators back to the plan/apply workspace for full review (the plan/apply workspace for detail review remains the primary handoff), and keeps in-shell Applications overview plus JSON/read-route copy recovery actions visible on populated, loading, empty, unavailable, and generic fetch-error monitor states. Empty monitor states are actionable: queued empty points operators back to plan/apply workspaces, active empty explains that work may be terminal or still unclaimed, and stale empty is called out as the healthy recovery-queue state.

Optional managed-login provider modes are non-secret values: use
`TENET_AUTH_PROVIDER=github`, `TENET_AUTH_PROVIDER=google_oidc`,
`TENET_AUTH_PROVIDER=okta_oidc`, or diagnostic-only
`TENET_AUTH_PROVIDER=okta_saml`. Keep credential-bearing settings such as OAuth
client secrets, SAML private material, session secrets, and tokens redacted.

## Operator UI Entry Points

Start at `/` for the applications overview. The overview keeps the first page
compact with application rows, status chips, filters, a count-aware `Clear N filters`
reset action with an active-filter tooltip that names the exact search, sync, health, and account state before reset, `Open application`, a self-describing
`More actions for {deploymentName}` disclosure with an accessible secondary-action
group label, and `Applications JSON` / `Copy read route` handoffs for `GET /v1/overview/applications`. Each row keeps compact `Project` and `Last update` fields
from the joined application/runtime summary, falling back to `Latest runtime status` when no timestamp is present, and adds a compact `Latest runtime change`
cue only when `runtimeSummary.changeDetails[]` carries controller-authored change copy. The secondary account
drill-down keeps its toolbar `Refresh account detail` action and surfaces `Current account read: GET /v1/overview/accounts/{awsAccountId}` in the account header; account/group JSON handoffs include adjacent `Copy account read route`
/ `Copy group read route` controls for exact scoped `GET /v1/overview/accounts/...`
handoffs, and deployment rows with meaningful runtime/event evidence add a `_self` `Open runtime events` link to `/?page=application&deployment={deploymentName}#application-runtime-events` while preserving `q` / `sync` / `account`. Expandable rows expose their controlled lazy-loaded detail panel with
`aria-controls`; if duplicate panels wait on the same deployment detail read, the shared result updates every waiting panel instead of leaving stale loading copy behind. The account drill-down header and each project/environment group header also show compact runtime trend cues plus the retained runtime-events cue from already-loaded `account.deployments[].runtimeSummary.historyCount` or `history[]` fields; trend dots summarize quiet, synced, progressing, paused, and error states without adding another backend read, while the retained-events cue names the newest deployment/timestamp/summary or calmly says `No recent runtime events`. Application detail pages show the exact backing read route, `Application JSON`, `Copy read route`, and a lightweight `resourceFilter=` search across linked-resource type, name, description, and source-cue text
with visible/total impact and section-specific empty guidance alongside the inline
`Clear resource filter`. Their summary also includes `Linked-resource source`, distinguishing controller-authored `rollout.execution[]` rows from canonical fallback rollout inventory when that array is missing or empty, plus compact resource-row `Evidence cues`:
`Live evidence available` / `Recent logs supported` `_self` handoffs to the final
resource page's `#resource-live-inspection` / `#resource-recent-logs` panels, and `Related events · N` handoffs to `#resource-related-runtime-events` whenever retained controller events reference that resource. Application resource rows seed those related-event cues from the already-loaded `resourceRelatedEvents[]` detail summary first, using operator-readable source labels such as `Controller history` while preserving custom source values, then refresh them from the focused deployment-events read when that lazy panel resolves, so useful retained-history signals are visible before the heavier events timeline finishes loading. The final resource page repeats that `Linked-resource source` cue in the selected-resource context, so operators can tell whether the focused resource came from controller-authored `rollout.execution[]` or from fallback inventory without navigating back to the application page. The drift and plan workspaces show `Current workspace read: GET /v1/overview/deployments/{deploymentName}.`
in their chrome and include top-level `Workspace JSON` plus `Copy workspace read route` handoffs to that
deployment-detail read model. Initial and refresh-time non-auth read failures, including the default `GET /v1/overview/applications` landing read, render the active page label, failed `GET ...` route, optional `Tenet request ID`, and same-URL `Retry current page` action instead of a generic overview-only error. Malformed drill-down bookmarks that omit the required `accountId` or `deployment` context render a missing-context empty state with a same-shell `Back to applications` recovery link that keeps existing `q` / `sync` / `health` / `account` filters intact.

Common bookmarkable routes:

- `/?page=application&deployment={deploymentName}`
- `/?page=account&accountId={awsAccountId}`
- `/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}` (canonical); older `resourceName={name}` bookmarks are still accepted as a compatibility fallback
- `/?page=drift&deployment={deploymentName}`; optional `driftQuery`, `driftSeverity=high|medium|low`, and `driftSort=severity` preserve field-level drift filters in shareable URLs, filtered-empty tables offer an inline `Clear drift filters` recovery action, and no-row / filtered-no-match drift table messages are announced as polite status updates for assistive technology
- `/?page=plan&deployment={deploymentName}`
- `/?page=audit` and `/?page=applies`
Primary operator APIs:
- `GET /v1/overview/applications`
- `GET /v1/overview/applications/{deploymentName}`; application-detail `rollout.execution[]` rows include additive `evidenceCapabilities` route hints for live inspection, recent logs, and related runtime events when Tenet knows a safe lazy evidence route. These hints do not perform live AWS/log reads and do not claim provider data is currently available; the linked evidence routes remain authoritative for `available`, `source`, `unavailableReason`, and empty states.
- `GET /v1/overview/deployments/{deploymentName}`
- `GET /v1/status/deployments` and `GET /v1/status/deployments/{deploymentName}` for raw persisted runtime snapshots
- `GET /v1/overview/deployments/{deploymentName}/events` (or `GET /v1/overview/deployments/{deploymentName}/events?source={source}&limit={positiveInteger}` for exact source-filtered, bounded controller-history reads)
- `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`
  (recent-log responses expose `resultScope.requestedLimit`, `effectiveLimit`,
  `returnedEntryCount`, `returnedCount`, `truncationStatus`, and `truncated`;
  compact aliases such as `limit`/`returned` keep bounded reads terse, while ECS
  task container, ASG instance, and ECS cluster selectors keep focused reads explicit)
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`
  (the route requires a concrete resource type/name; supported ECS, ASG, and
  traffic-routing reads can use focused selectors/caps documented in
  `docs/features/resource-inspection.md`; live ASG reads may add optional
  `ec2InstanceStatus` status-check evidence beside sampled/selected EC2 host
  details when AWS `DescribeInstanceStatus` is available)

Managed-drift rows remain backward-compatible. When Tenet knows the evidence path for a row, `managedDrift[].source` is an additive object with stable `kind`, optional `label` / `route`, and optional `resourceType` / `resourceName` identity; rows without known source evidence omit `source` rather than emitting placeholders. Existing `resourceRef` metadata remains unchanged.

Resource drill-down Related runtime events, Recent logs, and Live inspection panels summarize controller history, lookup context, bounded reads, request correlation, and unavailable states without duplicating the detailed resource field contract. The Related runtime events panel stays refresh-based, reads the resource-filtered `GET /v1/overview/deployments/{deploymentName}/events?resourceType={type}&resourceName={name}&limit={n}&source={source}` route, offers compact `Related event source` and `Related event window` controls, distinguishes empty retained history from source-filtered no-match states and route failures, provides a one-click `Show all related event sources` recovery action for source-filtered empty states, and provides a `Copy related events route` handoff while preserving the bookmarkable resource URL. Live inspection responses keep the resolved deployment/resource identity and accepted selectors (such as `clusterArn` or `instanceId`) in the top-level envelope even when a provider returns sparse evidence. Use `docs/features/resource-recent-logs.md`, `docs/features/resource-inspection.md`, and `docs/architecture/tenet-v1-overview-ui.md` for route selectors, response fields, labels, smoke expectations, and embedded UI behavior.
Local/no-AWS unavailable payloads and successful empty reads stay distinct:
operators see friendly next-step copy while scripts keep stable fields such as
`available`, `unavailableReason`, `emptyReason`, `lookupContext`, and
`resultScope`; route-stable no-log-source resources may also expose
`relatedLogRoutes[]` handoffs so operators can try sibling recent-log reads
without treating the selected resource as broken, and the resource-inspection guide maps each unavailable reason to a next operator action so `available=false`
evidence is not treated as a generic backend failure. Successful-empty Recent logs panels also tell operators to adjust
the Log limit, refresh after activity, and review lookup context before
escalating. The local and Docker smoke scripts keep a compact no-AWS resource-evidence matrix for ECS service/task, ASG/bootstrap, traffic-routing no-log-source, selector validation, successful-empty, and guarded-auth paths.
The embedded UI release guard keeps lazy Recent logs and Live inspection panels aligned so request routes, request IDs, lookup context, and bounded-scope metadata
stay visible even when AWS evidence is unavailable.
Use `Share ECS service event` or the adjacent `Copy ECS service event link` from a Live inspection event row to hand off one ECS service event without changing the resource-page selector state.
In the embedded resource page, log and inspection caps stay bookmarkable as `resourceLogLimit=` and `resourceInspectionLimit=` UI state; positive caps are sent only to the matching lazy `resource-logs` or `resource-inspection` read as backend `limit=` query parameters.
Recent logs and Live inspection lazy-panel state messages such as loading, empty, unavailable, and non-auth fetch failures are polite atomic status regions, so assistive technology announces focused panel state changes without making the full evidence payload live.
Capture the HTTP `Request-Id` response header with the provider lookup context
when escalating a failed or unavailable Recent logs or Live inspection read; the
embedded cards surface the same request-correlation token as metadata so
reviewers can match screenshots or copied JSON to Tenet's structured request
logs.

- `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` (`409 Conflict` means the reviewed plan preview is stale); `GET /v1/applies?status=active` returns queued plus running immutable apply handoff records newest-first without executor mutation or live AWS behavior, `GET /v1/applies?status=active&claim=stale` narrows that active collection to running records with stale/expired claim leases, `GET /v1/applies?status=running` returns the exact claimed/in-flight running subset, `GET /v1/applies?status=queued` preserves the exact queued-only subset, and terminal exact filters such as `GET /v1/applies?status=failed` list immutable records in that final state. `GET /v1/applies/{applyId}/execution-preview` returns a read-only next-action preview with `readOnly:true`, the deterministic first operation `action`/resource, target `awsContext`, and an explicit blocker such as `live_aws_mutation_not_implemented`; it derives from the immutable apply/preflight evidence and never mutates AWS. The embedded `/?page=applies` monitor defaults to queued rows with manual `Refresh applies`, then lets operators switch to bookmarkable active (`/?page=applies&appliesView=active`) and stale-claim (`/?page=applies&appliesView=stale`) views to see claimed/running handoffs, worker IDs, heartbeat/lease freshness, fresh/stale/pending claim copy, `_self` handoffs back to `/?page=plan&deployment={deploymentName}&applyId={applyId}`, and JSON handoff, `Copy read route`, refresh, and Applications overview recovery actions visible on populated, empty, loading, unavailable, and generic fetch-error monitor states. Empty monitor states are actionable: queued empty points operators back to plan/apply workspaces, active empty explains that work may be terminal or still unclaimed, and stale empty is called out as the healthy recovery-queue state. Local-safe executors claim queued work with `POST /v1/applies/{applyId}/claim` and a body such as `{"workerId":"worker-a"}`; successful responses preserve the stable `id`, set `status:"running"`, add `claim.workerId` / `claim.claimedAt` / `claim.lastHeartbeatAt` / `claim.expiresAt`, append transition history, and still perform no live AWS mutation. Explicit expired-lease recovery uses the same route with a body such as `{"workerId":"worker-b","allowExpiredClaim":true}` and is accepted only when the persisted `claim.expiresAt` is expired; fresh competing claims still return `409 Conflict`, and legacy stale claims without persisted `claim.expiresAt` remain manual-recovery conflicts. The currently claimed worker refreshes its local-safe lease with `POST /v1/applies/{applyId}/heartbeat` and the same body shape; the heartbeat response updates `claim.lastHeartbeatAt` / `claim.expiresAt` while preserving the stable apply ID, lifecycle state, operation state, details, and transition history. A claimed apply can still advance through `POST /v1/applies/{applyId}/transitions`; the transition response keeps `claim.workerId`, `claim.claimedAt`, `claim.lastHeartbeatAt`, `claim.expiresAt`, and additive lifecycle metadata. Competing workers get `409 Conflict` text that distinguishes an active lease from stale legacy claim metadata; stale conflicts are operator recovery guidance unless a caller explicitly sends `"allowExpiredClaim":true` and the persisted lease expiry has passed; wrong-worker, unclaimed, or terminal heartbeat attempts also return `409 Conflict`. This remains local-safe only: claim/heartbeat/transition records update Tenet's persisted apply lifecycle without live AWS mutation.
- `POST /v1/controller/refresh` to request the configured desired-state refresh handoff manually (`filesystem_noop` for filesystem catalogs, `202 Accepted` with `controllerRefresh` evidence when GitHub+Redis handoff is available). Accepted manual refresh requests are also projected through `GET /v1/audit-logs?action=controller.refresh.requested` with actor metadata plus safe route/status/result/request ID/delivery ID/channel or no-op evidence, and `GET /v1/controller/refresh/status` returns the latest receipt plus explicit `no_history` / `unavailable` states for read-after-write troubleshooting; from deployment-scoped application/resource/drift/plan pages, the embedded Operator context keeps a non-noisy `Last refresh requested` cue and links operators to `/?page=audit&deployment={deploymentName}&action=controller.refresh.requested` without dropping the current bookmarkable route. The embedded Operator context details expose a DOM-built `Refresh controller` control only when `GET /v1/operator/context.desiredStateSource.controllerRefresh` reports an available handoff; the control preserves the current bookmarkable URL, disables while the POST is in flight, reports success/no-op/unavailable failures in the banner live status, reuses the shared operator-write auth guidance for `401` / `403` responses, and after an accepted processed handoff re-runs the currently active embedded read model once in place without rewriting `window.location.search` or `window.location.hash`, hiding the banner announcement, or starting polling.
- `GET /v1/audit-logs` (bounded reads expose `resultScope.returnedEntryCount`, `resultScope.returnedCount`, `truncationStatus`, and `truncated`; exact `action` filters include `plan.created`, `apply.requested`, `apply.transitioned`, and `controller.refresh.requested`)
- `GET /v1/operator/context`

The bookmarkable audit timeline (`/?page=audit`) keeps a manual `Refresh audit log` control, an accessible `Last refreshed` cue, and clearable deployment/action/limit chips that remove only their own control while preserving the shell path, `q`/`sync`/`account`, and stripping stale `#audit-*` anchors. Action controls keep exact API values such as `apply.requested` in selectors and copied read routes while visible filter chips and audit row titles use concise operator labels such as `Apply requested`. Capped audit windows from `resultScope` still expose inline expand/clear actions that keep the current deployment/action and applications-shell query state. The plan/apply workspace also keeps immutable apply lifecycle evidence local to the bookmarked record: when `GET /v1/applies/{applyId}` includes `transitions[]`, `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}` renders a read-only `Apply transition timeline` with transition time, previous/new status, step, actor availability, and error/details; records without persisted transitions show explicit empty-state copy without implying live AWS mutation.

The bookmarkable resource page (`/?page=resource...`) keeps the top-level deployment-detail read visible before lazy Recent logs or Live inspection panels load in the selected-resource body callout as `Current resource read: GET /v1/overview/deployments/{deploymentName}` and exposes `Resource JSON` / `Copy read route` toolbar handoffs to that same read model. The lazy Recent logs and Live inspection panels also expose `Copy recent logs route` / `Copy live inspection route` controls next to their refresh and limit controls, so operators can copy the exact `resource-logs` or `resource-inspection` request (including the selected bounded read limit) without opening raw JSON; populated Recent logs rows add `Copy log entry link` beside `Share log entry`, reusing the same row anchor URL with optional `containerName`, `instanceId`, `clusterArn`, and `resourceLogLimit` state preserved. Their focused state messages are polite atomic status regions for assistive technology. Resource-detail rendered sections also include low-noise same-page `Section link` and `Copy section link` troubleshooting handoffs that preserve the current pathname and query state, including `containerName`, `instanceId`, `resourceLogLimit`, and `resourceInspectionLimit`, while changing only the hash. The same page now adds a compact `Troubleshooting summary` derived from the selected deployment/resource plus any selected lookup scope (`containerName`, `clusterArn`, or `instanceId`) and the latest lazy Recent logs and Live inspection panel states, including an Evidence freshness row for completed reads plus empty-log and unavailable-provider next-step guidance. When a selected resource type has no focused Recent logs or Live inspection handoff yet, the summary marks those rows as `Unsupported for this resource type` and points operators back to the current resource read, rollout context, status conditions, and related runtime events instead of asking them to refresh hidden panels.

Deployment runtime events are read from the controller-retained history window. `TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT` controls that per-deployment retention limit and defaults to `10`; invalid, zero, negative, or above-1000 values fail controller startup. This controller-history timeline is not the immutable audit log. Successful `GET /v1/overview/deployments/{deploymentName}/events` responses keep `events: []` for repo-known deployments before history exists, with zero-count `managedDriftSummary`, an additive `sourceSummary` array of `{source, count}` entries computed from the retained window before optional source filtering and before positive limit truncation (blank persisted sources are bucketed as `unknown`, and `?source=unknown` selects those legacy entries), plus zero retained-event counts in `retention` / `resultScope` (`retention.limit` may still be non-zero, e.g. `10`, and reports the effective controller limit). The application detail Controller runtime events panel renders the same backend metadata, including total returned/retained counts, retention limit/status, query-window status, exact source filter when selected, exact resource filter when `resourceType` / `resourceName` narrow events to matching generic `events[].resourceRefs[]` rollout/execution references or existing `managedDrift[]` resource evidence, and source-summary chips such as `Source mix · Controller history · 7 events` whenever `sourceSummary` has entries, so operators can distinguish an empty retained history from a source-filtered empty window, query-limited response, or controller-retention cap without opening raw JSON. The panel omits those chips when `sourceSummary` is empty, keeps `eventLimit` and optional `eventSource` in the application URL, fetches `GET /v1/overview/deployments/{deploymentName}/events?limit={n}&source={source}` for non-all sources, and backend callers can additionally request `?resourceType={type}&resourceName={name}` before limit truncation (for example `GET /v1/overview/deployments/payments-api-prod/events?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate`), exposes a `Copy runtime events route` handoff for the exact selected read, and preserves both controls in per-event share links; the loaded timeline also shows a compact `Runtime event navigation` strip with newest/oldest visible event timestamp/summary and `_self` jumps to those event anchors. The selector keeps backend values in the route while showing operator-readable labels such as `Controller history`, `Deployment controller history`, and `GitHub refresh`, while exact bookmarked custom source values remain visible instead of being discarded. Runtime-event loading, empty, unavailable, and fetch-error states are polite atomic status regions so assistive technology announces focused panel changes without making the full event timeline live. The `resultScope` fields distinguish optional exact `?source=` filtering (reported as `sourceFilter` and applied before any limit), `?limit=` query truncation (`limited`, `truncationStatus`, compact alias `truncated`), and the retained controller-history cap (`retentionLimit`, `retainedEventCount`, `retentionStatus`), report exact-cap bounded reads as `truncationStatus: "unknown"` with `truncated: false`, and expose `returnedCount` as a compact alias for `returnedEventCount`. Invalid source values return `400 Bad Request` with `invalid source query`; direct-process smoke covers that validation while Compose smoke keeps the bounded `?limit=1` runtime-events `resultScope` and `sourceSummary` contract exercised.

Deployment and application detail payloads also include additive `resourceRelatedEvents[]` rows for each controller-authored rollout execution resource. Each row summarizes retained controller-history evidence already associated with that resource (`resourceRefs[]` plus managed-drift `resourceRef` / `source.resourceType` evidence): `retainedEventCount`, `newestEventTimestamp`, `sourceSummary`, and `managedDriftSummary`. These summaries help operators decide which linked resources have useful history before opening focused resource panels; they are retained controller-history cues, not live AWS telemetry or durable audit counts.

Trusted front-door auth is additive and opt-in. Only enable the auth/RBAC
environment toggles when every request reaches Tenet through a front door that
authenticates the caller and strips spoofable trusted headers before forwarding
trusted values. The auth/RBAC toggles are boolean, non-secret settings, so use
operator-readable examples such as `TENET_API_TRUST_AUTHENTICATED_HEADERS=true`,
`TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`,
`TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true`, and
`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`. Filesystem catalog
sources may additionally opt into repository-backed RBAC grants with
`TENET_API_AUTHORIZE_WITH_REPOSITORY_RBAC_POLICY=true`; when enabled, the
existing guarded read/write routes keep the same identity requirements but use
`rbac/policy.json`, `rbac/policy.yaml`, or `rbac/policy.yml` grants evaluated
against trusted identity plus `X-Tenet-Authenticated-Groups` instead of trusted
role/deployment grant headers. The embedded Operator context banner adds a
policy-mode cue in that mode: compact source/availability status stays above the
toolbar, while expanded details show matched user/group grants and derived
roles, deployment grants, and project/environment scopes for denied-read
troubleshooting. Leave it unset to preserve the default trusted-header
authorization path. Keep credential-bearing
settings such as tokens redacted.

## Validation

Fast local checks:

```bash
go test ./internal/api
go test ./cmd/tenet-api
make smoke-local
```

Containerized local-stack checks:

```bash
make smoke-docker
```

General repository checks:

```bash
go test ./...
```

CI Smoke mirrors those checks in the hosted fast lane and also syntax-checks the operator smoke helpers before running local smoke. Hosted fast-lane order is `go test ./...`, smoke helper syntax checks, then `make smoke-local`. The repository-wide docs test also validates local Markdown file links and same-file/cross-file heading anchors so runbook handoffs do not drift after headings move. Smoke helper scripts must keep their executable bits because Makefile targets and runner-start docs invoke them directly by path. They should also remove every mktemp-backed helper file or directory during cleanup so repeated smoke runs do not leave stale request/response payloads in `/tmp`. For smoke-script or runner-start evidence changes, run:

```bash
bash -n scripts/smoke-local.sh
bash -n scripts/smoke-docker.sh
bash -n scripts/ci-smoke-runner-evidence.sh
```

The Docker smoke lane remains separate; use `make smoke-docker` for Compose, container startup, fixture, or Docker-stack runtime changes. In hosted PRs it is path-scoped: docs-only changes normally stay on the fast lane, while changes under Docker/Compose, front-door auth, API/controller, local fixtures, runtime read models, RBAC policy diagnostics, resource evidence, webhook, plan/apply, or audit smoke surfaces run the full Docker stack before merge.

For hosted CI Smoke failures that never start a runner, collect pasteable issue
evidence with:

```bash
scripts/ci-smoke-runner-evidence.sh --summary <run-id>
```

The detailed runner-start evidence bundle remains available by omitting `--summary`; see
`docs/runbooks/ci-smoke-runner-start.md`, including newer `startup_failure` runs where the Actions jobs API returns `jobs: []` before per-job runner records exist.

## Documentation Map

- `docs/architecture/tenet-v1-operator-contracts.md` preserves the detailed
  endpoint, read-model, auth, resource, and smoke-verification contracts formerly
  kept in this README.
- `docs/architecture/tenet-v1-overview.md` describes the v1 control-plane
  direction and desired-state/runtime model.
- `docs/architecture/tenet-v1-local-runtime.md` documents local process,
  Compose, auth/RBAC, smoke, and runtime-store behavior.
- `docs/architecture/tenet-v1-overview-ui.md` documents embedded UI navigation,
  route shapes, and operator-workspace behavior.
- `docs/architecture/tenet-v1-audit-log-api.md` documents immutable audit,
  plan/apply, actor, and authorization contracts.
- `docs/architecture/tenet-v1-aws-central-control-plane.md` documents the AWS
  Terraform bootstrap, public repo-sync, and `repo_sync_token_secret_arn`.
- `docs/architecture/tenet-v1-enterprise-sso.md` documents managed login,
  Google Workspace OIDC, Okta OIDC, Okta SAML diagnostics, and SSO boundaries.
- `docs/features/managed-login.md` documents managed-login activation, provider routes, cookie-security diagnostics, logout boundaries, and trusted-front-door/policy relationships.
- `docs/features/overview-dashboard.md` documents application/account/resource pages,
  drift, audit, and plan/apply workspaces.
- `docs/features/resource-recent-logs.md` documents recent-log resource contracts, including lookup context, selector echoes, and friendly empty/unavailable states.
- `docs/features/resource-inspection.md` documents the focused live resource inspection route, selectors, lookup context, unavailable states, and detailed field contracts.
- `docs/runbooks/operator-troubleshooting.md` documents the auth, route, evidence, state, and verification troubleshooting quick-reference.
- `docs/runbooks/ci-smoke-runner-start.md` documents GitHub Actions runner-start
  failure triage for CI smoke checks.

## Repository Layout

- `cmd/tenet-api` - API server and embedded UI entry point
- `cmd/tenet-controller` - controller/runtime loop entry point
- `internal/api` - routes, read models, auth enforcement, and UI assets
- `internal/catalog` - manifest loading and catalog compilation
- `internal/statusstore`, `internal/planpreview`, `internal/applyrequest`, and
  `internal/auditlog` - runtime status, plan/apply, and audit stores
- `examples/local-repo` - sample desired-state repository
- `examples/local-status` - sample runtime status fixtures
- `scripts` - smoke, Compose, and CI helper scripts
- `docs` - architecture, feature, plan, and runbook documentation

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
