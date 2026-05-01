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

The goal is fast triage, not completeness. The landing page keeps bookmarkable `q`, `sync`, `health`, and `account` filters; `health` uses the same row labels operators see in the table (`Healthy`, `Progressing`, `Unhealthy`, `Unknown`) so a health-focused view can be shared without losing the broader shell state. Browser tab titles are also route-aware safe text, so a shared or reopened `/?page=application`, `/?page=resource`, `/?page=drift`, `/?page=plan`, `/?page=audit`, or account drill-down URL identifies the active deployment/resource/account in browser chrome while preserving the static `Tenet Control Plane` HTML title as the non-JS fallback. The denser account/group deployment surface now lives on the secondary `/?page=account&accountId={awsAccountId}` drill-down, where the UI reuses `GET /v1/overview/accounts/{awsAccountId}` plus grouped project/environment sections instead of crowding the landing page. The landing page also exposes a small `_self` link to the dedicated bookmarkable audit route at `/?page=audit` (with optional `&deployment={deploymentName}`, `&action=plan.created|apply.requested`, and `&limit={n}` for a narrower newest-first window).

Initial auth failures on the landing read now follow the same shared operator-read contract as the other bookmarkable operator pages. `401 Unauthorized` on `/` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403 Forbidden` explains that identity was recognized but the current operator is not authorized for that route.

The `GET /v1/operator/context` endpoint behind the embedded banner also returns a diagnostic `enforcement` object and may return diagnostic `deploymentAccess.summary` JSON when Tenet can compare forwarded deployment allowlists or project/environment scopes with catalog placements. The banner renders the server-authored enforcement mode/summary as operator-facing chips so a local-unrestricted or observed-only API process is obvious before an operator interprets route outcomes; read/write-guarded mode stays neutral, while partial or non-enforcing modes use the existing warning treatment. It also renders allowlist counts and project/environment evidence directly: all-matched summaries reassure operators that the front door forwarded catalog-known deployment names or matching project/environment scopes, unmatched summaries use the total `unmatchedCount` plus the bounded `unmatchedNames` sample (flagged with `unmatchedNamesTruncated` when incomplete) in a warning pill with explicit front-door guidance for stale/miswired names or scopes, `emptyReason`/`unavailableReason` keep empty or unavailable states explicit, and deterministic `projectEnvironmentExamples` show the bounded visible project/environment/region/account groups that the trusted scope contract can reason about. The banner also renders bounded `accessVerdicts[].exampleRoutes` and `accessVerdicts[].requiredTrustedHeaders` evidence in each verdict chip, keeping the diagnostic tied to concrete shipped route families such as applications collections, deployment drill-downs, immutable plan/apply writes, and audit reads plus the identity, role, and deployment-allowlist or project/environment scope headers each read route class needs. The intended troubleshooting path is to start in that banner, inspect identity/roles/deployment allowlist/project-environment scopes, inspect the access-verdict route examples and required headers, and then debug the specific guarded route class and its trusted-header prerequisites. Use that evidence to troubleshoot stale front-door allowlists, stale scope values, and auth configuration; the summary is not an authorization source and the UI must not make client-side access decisions from it. The verdict route examples and required headers are diagnostic-only too; the UI must not make client-side access decisions from them. The banner's `Refresh context` action re-checks only that diagnostic endpoint in place, leaves the current application/resource/workspace URL untouched, and keeps the last successful context visible with a refresh-failure note so operators can compare header changes without losing their drill-down.

Initial auth failures on the secondary account drill-down now follow that same shared operator-read contract. `401 Unauthorized` on `/?page=account&accountId={awsAccountId}` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403 Forbidden` explains that identity was recognized but the current operator is not authorized for that route or deployment.

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

Only non-empty linked-resource groups are rendered on the application page and in its quick-navigation section list. This keeps the drill-down focused on actionable operator surfaces instead of showing empty placeholders. If an application truly has no linked resources yet, the page should show the explicit empty state `No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.` while still keeping the controller runtime events section available.

The page is backed by `GET /v1/overview/applications/{deploymentName}` and keeps the deeper `resource`, `drift`, and `plan` routes anchored to `GET /v1/overview/deployments/{deploymentName}`. A narrow controller-runtime-events slice is also rendered here by calling `GET /v1/overview/deployments/{deploymentName}/events` directly so the bookmarkable application detail stays operator-useful without pulling the full deployment-detail payload. Both detail surfaces preserve additive `sourceFingerprint` so the same reviewed payload can feed the read-only plan-preview flow without inventing a second client-side identity model. The companion `_self` `Audit log` link now opens `/?page=audit&deployment={deploymentName}` so the operator keeps the same deployment context when switching into the immutable audit slice.

Initial auth failures on that application-detail read are now intentionally operator-specific: `401 Unauthorized` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403 Forbidden` explains that identity was recognized but the current operator is not authorized for that route/deployment. When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, the backend now enforces that narrow read gate using trusted `X-Tenet-Authenticated-*` identity plus either trusted `X-Tenet-Authorized-Deployments` membership or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement. When `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, the same guarded read path additionally requires one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin`, returning `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route` before deployment filtering. Other statuses keep the existing unavailable/generic fetch-error behavior.

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

The embedded resource page now includes a top-level `Refresh resource detail` action that re-runs `GET /v1/overview/deployments/{deploymentName}` in place, preserves the bookmarkable `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` URL, re-resolves the selected resource from the refreshed payload, and keeps the last successful resource detail visible while refresh-time loading/error guidance renders above it. The same page also includes a dedicated `Live inspection` panel that lazy-loads `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}` for the selected resource while keeping the bookmarkable `/?page=resource...` route unchanged. That panel should always render an explicit state:
- loading
- unavailable, with lookup context plus friendly copy mapped from `unavailableReason`
- empty, when the route is available but there is no typed payload to show yet
- supported, with operator-friendly fields for `ecsService`, Auto Scaling Group, `userDataBootstrapSource`, and traffic-routing summary payloads, including ECS deployment-configuration evidence and the launch-template bootstrap-source comparison Tenet already uses for managed drift
- a `Refresh live inspection` action that re-runs only the inspection route in place, disables while that inspection fetch is active, and preserves the bookmarkable resource URL

The application/resource/drift/plan/audit drill-down pages now also render explicit breadcrumb chrome before their deeper sections. Those breadcrumbs keep prior drill-down levels as `_self` links, keep the current destination as a non-click chip, include the account step when the loaded deployment detail already exposes `awsAccountId`, and omit links that Tenet cannot build from already-loaded data. Resource, drift, plan, and deployment-scoped audit pages still keep their compact quick-navigation strip above the deeper detail sections. That strip is intentionally data-local: it builds `_self` links for `Back to application`, `Drift view`, `Plan/apply`, `Audit log`, and sibling linked resources using only the already loaded deployment-detail payload plus preserved URL state. It should not introduce nested interactive controls or a second fetch path just to support lateral navigation.

For recent logs, the embedded card keeps backend lookup context visible with operator-facing labels such as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured`. Those fields stay visible in unavailable and empty states so operators can see where Tenet looked before switching to raw AWS troubleshooting. That card also exposes `Refresh recent logs` with the same in-place URL-preserving behavior and per-panel loading disablement.

The deployment-events route is intentionally read-only and controller-derived. It reflects persisted runtime history entries, returns `events: []` when a deployment exists without history yet, and should not be treated as a full durable audit system. The application drill-down renders the route's additive `managedDriftSummary` as `Managed drift summary · {total} total · {high} high · {medium} medium · {low} low` above the event list so operators can spot severity distribution before scanning individual rows, then shows an in-shell `Open field-level drift` handoff to `/?page=drift&deployment={deploymentName}#drift-field-level` when those counts report managed drift. The application drill-down now also exposes `Refresh runtime events` on that panel so operators can re-run only `GET /v1/overview/deployments/{deploymentName}/events` in place, preserve the bookmarkable `/?page=application...` URL, and disable just that panel action while the events fetch is in flight. Auth-gated `401` / `403` responses on that in-place refresh now reuse the same trusted-front-door / access-denied guidance as the other embedded read surfaces instead of generic fetch-error copy. When an event's aligned change label is `managedDrift`, the paired `changeDetails[]` text now stays single-slot but explains deterministic added, removed, and updated drift rows so the timeline remains readable without diffing raw `managedDrift[]` arrays.

The drift workspace stays read-only and now exposes `Refresh drift workspace` on the field-level drift section so operators can re-run only `GET /v1/overview/deployments/{deploymentName}` in place, keep the bookmarkable `/?page=drift...` route unchanged, and see explicit loading/error copy without clearing the current drift workspace content while the refresh is in flight. Its compact drift summary labels high/medium/low severity counters as visible-row counts so query or severity filters do not make filtered counts look like the total baseline. That same drift shell now surfaces direct `_self` resource drill-down links in its quick-navigation strip using the already-loaded grouped normalized resource inventory from the deployment detail payload, and it renders explicit `No linked resources are available in this drift workspace yet.` copy instead of broken links when the inventory is empty. The breadcrumb and quick-navigation layers deliberately complement each other here: breadcrumbs preserve the drill-down path, while quick-nav keeps lateral workspace/resource jumps available. The backing deployment-detail contract now also treats controller-owned completed sync defaults as `Drifted` when the same payload carries concrete controller-authored `managedDrift[]` rows, while preserving seeded sync evidence and active rollout states. For EC2 deployments using `asgWeightedTargetGroups` or `asgInstanceRefresh`, those controller-authored rows can include live-backed Auto Scaling Group capacity evidence under `runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max` when read-only AWS inspection succeeds. ECS `ecsCodeDeploy` and `ecsRolling` deployments can now also publish live ECS-backed rows under `runtimeContext.capacity.desired` and `runtimeContext.taskDefinition.image` from read-only ECS service inspection when the service desired count and a single task-definition container image can be resolved. Fixture-backed, older, or no-AWS cases keep the existing fallback path without synthetic live rows.

That same `GET /v1/overview/deployments/{deploymentName}` entry contract now distinguishes auth failures for the bookmarkable `/?page=resource...` and `/?page=drift...` routes: `401 Unauthorized` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403 Forbidden` explains that identity was recognized but the current operator is not authorized for that route or deployment. The same narrow read gate also covers `GET /v1/overview/applications`, which now requires trusted identity and filters returned rows through the union of trusted `X-Tenet-Authorized-Deployments` exact grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement, returning `200 OK` with `[]` when no grant matches anything. When `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, those same guarded read routes additionally require one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin` before Tenet evaluates the deployment allowlist or collection filtering. `GET /v1/overview/accounts` follows the same collection contract and filters returned account aggregates through the trusted allowlist. Unscoped `GET /v1/audit-logs` now follows that same collection contract under read enforcement: it requires trusted identity, filters returned audit entries through the trusted `X-Tenet-Authorized-Deployments` allowlist, and returns `200 OK` with `{"entries":[]}` when the allowlist is omitted or no audit rows remain visible. Scoped account JSON routes now follow the same front-door contract: `GET /v1/overview/accounts/{awsAccountId}` and `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` require trusted identity, filter their deployment lists to authorized deployments only, recompute additive counters from that visible subset, and return the existing `403 Forbidden` deployment-read denial when the authenticated operator has no authorized deployments left in that scope. The default `/` landing page and bookmarkable `/?page=account...` drill-down render the same shared operator-read `401` / `403` guidance when those overview routes are denied. The gate also covers `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, `GET /v1/status/deployments`, `GET /v1/status/deployments/{deploymentName}`, `GET /v1/plans/{planId}`, and `GET /v1/applies/{applyId}` under the same trusted identity + read-role + allowlist contract.

For the resource drill-down contract, Tenet now documents the exact lazy routes `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` alongside the same explicit `401`, `403`, and `available=false` plus `unavailableReason` behavior so operators can reason about the panel-specific auth/unavailable states without inferring them from the UI alone.

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
- `POST /v1/applies/{applyId}/transitions` advances executor-facing lifecycle state on that same persisted apply record with a JSON `applyrequest.TransitionRequest` body while preserving the immutable stable `applyId`; Tenet's local-safe executor now uses that same state model internally, so enabled-mode progress does not require manual transition API calls. Under front-door auth enforcement, the same route uses the operator-write `401 Unauthorized` body for unauthenticated callers, and under deployment-read authorization it looks up the persisted apply deployment before returning the existing `403 Forbidden` deployment-access-denied body
- `GET /v1/audit-logs?deployment={deploymentName}` narrows the local audit timeline to one deployment without introducing a second route family
- deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}` keeps the immutable audit drill-down aligned with the other bookmarkable operator workspaces
- `GET /v1/audit-logs?action={action}` narrows the same local audit timeline to exactly `plan.created` or `apply.requested`
- `GET /v1/audit-logs` projects persisted plan/apply records into a local audit timeline; the route also accepts `?limit={n}` for a newest-first window and optional `action={action}` for exact action filtering
- the embedded `/?page=audit` route now renders that audit timeline directly with explicit loading, empty, unavailable (`503`), and generic fetch-error states for non-auth failures, adds distinct initial-read guidance for `401 Unauthorized` vs `403 Forbidden`, accepts optional `&action={action}` / `&limit={n}` for action-only bookmarks and optional `&deployment={deploymentName}` / `&action={action}` / `&limit={n}` for deployment-scoped bookmarks, shows active filter copy such as `Audit filters: action apply.requested · limit 2` for action-only bookmarks and `Audit filters: deployment payments-api-prod · action apply.requested · limit 2` for deployment-scoped bookmarks, keeps deployment-scoped copy obvious in both the page chrome and body, exposes the same bookmarkable limit through a safe toolbar selector that still honors positive-integer URLs such as `?limit=2`, renders a conditional `Clear audit filters` recovery action that resets deployment/action/limit back to the default `/?page=audit` route without dropping bookmarkable applications-shell query state such as `q`, `sync`, or `account`, and for deployment-scoped audit routes adds the same quick-navigation hierarchy/current-destination chrome plus hash-aware `Current section` / `On this page` fragment links for `#audit-deployment-context` and `#audit-timeline`
- the embedded `/?page=plan&deployment={deploymentName}&planId={planId}` workspace keeps the URL shareable/bookmarkable, exposes explicit `Refresh plan preview` / `Refresh apply request` controls that re-run the current immutable reads without a full browser reload, adds compact `_self` fragment navigation for only the currently rendered plan/apply sections by reusing the existing `buildPageSectionURL(...)` + `scrollToCurrentHashTarget()` pattern with stable ids such as `plan-runtime-signals`, `plan-preview-actions`, `plan-stored-preview`, `plan-stored-apply`, `plan-apply-operations`, and `plan-operations`, prefers the reviewed detail `sourceFingerprint` for optimistic `ifMatch` protection before falling back to a loaded preview fingerprint, and preserves deployment/plan-derived fallback context when those immutable reloads fail
- when `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, or `POST /v1/applies/{applyId}/transitions` returns `401 Unauthorized` under auth enforcement, the embedded workspace explains that authenticated identity is required on operator write routes and tells operators to retry through the trusted front door instead of showing generic fetch-error copy
- when `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is also enabled, those same write routes require the trusted `X-Tenet-Authorized-Deployments` allowlist to include the requested deployment, the persisted preview deployment for apply creation, or the persisted apply deployment for transitions, and `403 Forbidden` keeps the same operator-facing deployment-access-denied wording as the guarded read routes
- when bookmarked or refreshed `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` immutable reads return `401 Unauthorized` or `403 Forbidden`, the same workspace now reuses the operator read-auth framing from the other bookmarkable drill-down pages: `401` tells the operator to retry that bookmarkable workspace through the trusted front door, while `403` explains that the authenticated operator is not authorized for that route or deployment instead of falling back to generic fetch-error text
- when in-place `Refresh runtime events` on `/?page=application&deployment={deploymentName}` hits `GET /v1/overview/deployments/{deploymentName}/events` and that lazy read returns `401 Unauthorized` or `403 Forbidden`, the `Controller runtime events` panel now reuses that same operator read-auth framing instead of generic fetch-error copy while keeping the bookmarkable application route stable
- live execution remains intentionally disabled by default in this backend slice, so the apply-request route records a failed immutable contract with operator-facing error/details instead of contacting AWS while still attaching additive ordered `operations[]` rows with `status=pending`, `phase`, `target`, and `summary` fields projected from the reviewed plan preview
- when that reviewed plan fingerprint is stale, `POST /v1/applies/plans/{planId}` fails with `409 Conflict`, tells the operator to create a fresh plan preview, and does not persist a new apply record from drifted input
- when `TENET_AWS_ENABLE_APPLY=true`, newly created apply records switch to a queued immutable apply request without contacting AWS; the stored contract drops the disabled error, changes those projected `operations[]` rows to `status=queued`, and adds explicit details that the same `applyId` will be handed to a local-safe executor because live AWS mutation is not implemented yet
- that local-safe executor then advances the same apply record through `running` and finally `failed`, keeping the stable `applyId` while recording operator-facing handoff details instead of inventing a second job identity
- the stored apply read model now also includes an additive `lifecycle` block so executor code can advance the same stable `applyId` through queued/running/terminal states while surfacing `updatedAt` / `startedAt` / `finishedAt` timestamps plus `currentStep` / `completedSteps` / `totalSteps` progress metadata for operators
- malformed apply-transition JSON bodies fail with `400 Bad Request`, unknown apply IDs fail with `404 Not Found`, and invalid lifecycle moves fail with `409 Conflict` so executor/worker callers get explicit operator-facing boundaries without changing the route family
- the embedded queued/running/succeeded/cancelled/failed apply chrome is still rendering immutable stored lifecycle state for operator review, not a live AWS executor in this slice
- actor identity is intentionally explicit rather than invented: `GET /v1/audit-logs` renders `Recorded` with persisted `id` / `name` / `email` when explicit `X-Tenet-Actor-*` request headers or additive `X-User-*` aliases were present at record creation time, and still renders `actor.available=false` plus an unavailable reason for unauthenticated or purely local requests; when both header families are present, the explicit `X-Tenet-Actor-*` values win
- invalid audit `action` query values fail at the API boundary with `400 Bad Request` instead of silently widening or ignoring the filter
- a deployment-scoped audit query that finds no persisted records still returns `entries: []` so operators can distinguish an empty immutable timeline from an unavailable route

When `rollout.execution[]` is still missing or empty in a fixture-backed or early controller-authored payload, the resource drill-down keeps the operator contract stable by synthesizing the same fallback rollout resources the controller would eventually publish. That contract includes:
- `ecsRolling`
- `ecsService` → `{deployment}-service` → `ECS service Tenet updates with native rolling deployment`, optionally extended with rollout-strategy qualifiers such as `while respecting min healthy 90% and warmup 30s`

That same fallback identity keeps both resource-log and resource-inspection drill-downs bookmarkable. Example routes:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`
- `GET /v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service`

The smoke scripts extend that same bookmarkable surface to representative bootstrap-source and CodeDeploy resource drill-downs: they fetch the bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` page, the bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication::payments-api-staging-codedeploy-app::CodeDeploy application coordinating ECS traffic shifting` page, the bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup::payments-api-staging-blue::Blue task set target group CodeDeploy treats as the live ECS traffic destination` page, and the bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule::payments-api-prod-prod-listener::Production listener rule operators inspect while Tenet updates weighted forwarding during rollout` page. The same smoke pairs those shell reads with canonical backend routes including `GET /v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh`, `GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh`, and `GET /v1/overview/deployments/payments-api-staging/resource-inspection?resourceType=blueTaskSetTargetGroup&resourceName=payments-api-staging-blue`, and `GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=payments-api-prod-prod-listener`, while treating summary-only traffic-routing fallbacks and explicit unavailable reasons such as `aws_unavailable` and `no_log_source` as expected local no-AWS outcomes so the local-friendly logs and inspection contracts stay locked alongside the bookmarkable embedded drill-down.

The additive inspection route stays read-only and local-friendly:
- supported always-live summaries currently start with `ecsService`, `candidateAutoScalingGroup`, and `serviceAutoScalingGroup`
- supported traffic-routing resources are `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `productionListenerRule`, `activeTargetGroup`, `candidateTargetGroup`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`, and `trafficShiftPlan`; `trafficShiftPlan` stays `summary_only`, while CodeDeploy application/deployment-group/listener resources, ELB listener-rule resources, and target-group resources upgrade from `summary_only` fallback to live payloads when AWS inspection succeeds
- weighted-target-group traffic-routing summaries intentionally stay local-friendly and read-only: `trafficShiftPlan` never requires live AWS inspection just to render operator context on the bookmarkable resource page; `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` preserve that same summary-only fallback when AWS is disabled/unavailable, but `productionListenerRule` can upgrade to `trafficRoutingResource.listenerRule` and target groups can upgrade to the shared live `trafficRoutingResource.targetGroup` payload when the backend can describe the selected AWS resource. ECS CodeDeploy task-set target groups use the same fallback/live target-group shape.
- responses always include `lookupContext` plus `available`
- supported ECS payloads add `ecsService`, including additive `minimumHealthyPercent`, `maximumPercent`, and `healthCheckGracePeriodSeconds` fields when `DescribeServices` returns them
- supported Auto Scaling payloads add `autoScalingGroup`, with optional `launchTemplate` and `mixedInstancesPolicy` metadata when the live Auto Scaling Group already exposes launch-template details through `DescribeAutoScalingGroups`, plus additive `launchTemplate.imageId` and `launchTemplate.instanceType` fields when the live EC2 launch-template lookup resolves them
- supported traffic-routing payloads add `trafficRoutingResource` with `kind`, `displayName`, `inspectionMode`, controller/fallback `summary`, additive live `codeDeployApplication`, `codeDeployDeploymentGroup`, `listener`, or `listenerRule` details when available, and optional CodeDeploy task-set target-group metadata under `targetGroup` / compatible shapes when live target-group inspection is available
- unsupported or unavailable resources return `available=false` with an explicit `unavailableReason` instead of bubbling a vague provider error into the operator workflow

In the embedded UI, the supported inspection payloads should stay intentionally operator-facing:
- `ecsService`: service status, desired/running/pending counts, deployment-configuration evidence (`minimumHealthyPercent`, `maximumPercent`, `healthCheckGracePeriodSeconds`) when available, task definition, deployments, and recent ECS service events
- `autoScalingGroup`: sizing and health-check settings, target group ARNs, current instances, and additive live launch-template inspection fields `launchTemplate.imageId` and `launchTemplate.instanceType` when the live EC2 launch-template lookup resolves them
- `trafficRoutingResource`: summary-only or live traffic-routing context with explicit resource kind, display name, inspection mode, rollout summary text, additive CodeDeploy application/deployment-group/listener details, additive ELB `listenerRule` details (`ruleArn`, `priority`, `isDefault`, `conditions`, `actions`) when present, and CodeDeploy task-set target-group live details such as protocol/port, target type, health-check settings, load-balancer ARNs, and registered target-health counts when present

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
