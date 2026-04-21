# Tenet

Tenet is a repository-driven deployment control plane for multi-account environments.

The project is aimed at giving operators one place to understand what should be running, what is actually happening now, and how a deployment is progressing across EC2 and ECS targets. The current product direction is an ArgoCD-inspired workflow: start from a simple applications overview, drill into an application, then drill further into the concrete resource that matters during rollout or troubleshooting.

Today, Tenet is still early-stage and intentionally read-only. The focus is on a strong control-plane core, clear runtime read models, and an operator-friendly embedded UI before live execution and mutation workflows are added.

## What Tenet does

At a high level, Tenet:
- reads deployment intent from either a local repository checkout or a GitHub-backed read-only manifest tree
- compiles that intent into a validated internal catalog
- joins desired state with runtime status snapshots
- exposes that combined view through a small control-plane API and embedded UI
- helps operators inspect rollout state, runtime history, linked resources, and recent operational context

The current UI flow is:
- Applications overview
- Application detail
- Resource detail

This keeps the first page simple and pushes deeper operational detail into drill-down pages where it is easier to understand. The embedded application/resource/drift/plan/audit pages now also keep explicit breadcrumb chrome in the same shell so prior drill-down levels stay bookmarkable `_self` links, the current destination stays visible as a non-click chip, and the account step appears when Tenet already knows the deployment's `awsAccountId`. The application, resource, drift, and plan pages now all expose compact quick-navigation with bookmarkable `On this page` fragment links, while the resource/drift/audit shells still preserve the existing lateral `_self` jumps to sibling workspaces or resources without backing out first.

## Architecture

Tenet is currently built from four main layers.

### 1. Repository-backed desired state
Tenet expects a repo-like manifest structure with accounts, environments, and deployments. Both `tenet-api` and `tenet-controller` can read that desired state from either:
- a local checkout selected with `TENET_CATALOG_SOURCE=filesystem` plus `TENET_REPO_ROOT`
- a GitHub-backed read-only source selected with `TENET_CATALOG_SOURCE=github` plus `TENET_CATALOG_GITHUB_*`

The same JSON/YAML manifest codec and catalog compiler sit behind both modes, so `/v1/catalog/*` and `/v1/overview/*` keep the same response contract regardless of where desired state was loaded from. `TENET_REPO_ROOT` remains the filesystem-mode input when you want a local checkout; it is not a universal runtime requirement.

### 2. Catalog compilation
The manifest tree is loaded and validated into a compiled catalog. This gives Tenet a stable internal model for:
- account placement
- environment targeting
- deployment strategy
- desired artifacts
- rollout intent

### 3. Runtime status layer
Tenet keeps runtime status separately from desired configuration. Runtime snapshots can be stored in the filesystem or Redis and are used to represent things like:
- sync state
- health state
- rollout progress
- conditions
- bounded runtime history

### 4. Read models and operator UI
Tenet joins desired state and runtime state into read models that power:
- the embedded browser UI
- drill-down pages for applications and resources
- operational views such as rollout context, immutable plan previews, immutable apply requests, deployment events, and recent logs

The embedded UI is intentionally lightweight: static HTML/CSS/JS served directly by the Go API. That keeps the product easy to run locally while the control-plane model is still evolving.

## How to use Tenet

### Run locally against the example repository

```bash
TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

For manual `go run ./cmd/tenet-api` flows, `tenet-api` listens on `TENET_API_ADDR`, not `TENET_API_PORT`. Use `TENET_API_ADDR=:18083` as a safe local example before opening the UI.

Then open:

```bash
http://localhost:18083/
```

You should land on the applications overview page.

### Run `tenet-api` against a GitHub-backed desired-state catalog

```bash
TENET_CATALOG_SOURCE=github \
TENET_CATALOG_GITHUB_REPOSITORY=acme/platform-config \
TENET_CATALOG_GITHUB_REF=main \
TENET_CATALOG_GITHUB_PATH=tenet \
TENET_CATALOG_GITHUB_TOKEN=$GITHUB_TOKEN \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Optional GitHub source settings:
- `TENET_CATALOG_GITHUB_BASE_URL` for GitHub Enterprise Server or tests
- `TENET_CATALOG_GITHUB_REF` to pin a branch, tag, or commit
- `TENET_CATALOG_GITHUB_PATH` to point Tenet at a repo-relative manifest root such as `tenet` or `catalog/tenet`
- `TENET_CATALOG_GITHUB_TOKEN` when the repo is private or rate limits require auth

Startup validation is explicit:
- Tenet rejects ambiguous config when `TENET_REPO_ROOT` and `TENET_CATALOG_GITHUB_*` are both set without `TENET_CATALOG_SOURCE`
- `TENET_CATALOG_SOURCE=filesystem` requires `TENET_REPO_ROOT`
- `TENET_CATALOG_SOURCE=github` requires `TENET_CATALOG_GITHUB_REPOSITORY`
- `TENET_AWS_ENABLE_APPLY=true` still requires `TENET_REPO_ROOT` because the local-safe executor only supports the filesystem source

The read-model contract stays the same in both modes. For example:

```bash
curl http://localhost:18083/v1/catalog/summary
```

```json
{"accountCount":1,"environmentCount":1,"deploymentCount":2}
```

Every HTTP response from `tenet-api` also carries a `Request-Id` header. If your client already sends `request-id`, Tenet echoes it back; otherwise it generates one. Capture that value when an API call fails and use the same ID to correlate request-scoped logs and any handler/provider work that reads `api.RequestIDFromContext(ctx)`.

`tenet-api` now emits one structured JSON log line per API/UI request with stable fields including `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`. Panic recovery logs a separate correlated panic event, keeps the process alive, and returns a generic `500` response that includes the same request ID without leaking a stack trace to the client.

Use `TENET_API_REQUEST_TIMEOUT` to bound request handling. The default is `30s`; set `TENET_API_REQUEST_TIMEOUT=0` to disable the request timeout middleware. When a request times out, Tenet returns an explicit correlated `503` response and emits the matching request log with `result=timeout`. The same request context now flows through the catalog, overview, and status read providers, so GitHub-backed desired-state loads and status-store joins can stop work promptly when the client disconnects or the request deadline expires.

Tenet now exposes the first concrete GitHub webhook ingress path at `POST /api/v1/github/webhook`. Configure `TENET_GITHUB_WEBHOOK_SECRET` to enable that route; the local Compose stack wires a safe default `smoke-webhook-secret` so `make smoke-docker` can prove the signed contract end to end without extra setup. The handler remains conservative: it requires `X-GitHub-Delivery`, `X-GitHub-Event`, and `X-Hub-Signature-256`, validates the HMAC-SHA256 signature, checks the JSON body shape, and returns an operator-friendly summary of a signed push payload including `repository`, `ref`, `before`, `after`, `headCommit`, additive `changedFiles` lists, and an additive `desiredStateSourceMatch` object. That object keeps stable UI/docs-facing fields such as `configured`, `kind`, `matches`, `matchStatus`, `reason`, `repository`, `ref`, and `path` so operators can distinguish `not_configured`, `filesystem_source`, `repository_mismatch`, `ref_mismatch`, `path_untouched`, and `matched`. GitHub matching compares repository, normalizes branch or tag refs such as `main` vs `refs/heads/main` and `v1.2.3` vs `refs/tags/v1.2.3`, and treats `TENET_CATALOG_GITHUB_PATH` as a repo-relative prefix against changed files; when `TENET_CATALOG_GITHUB_REF` is unset, the webhook summary compares the push ref against `repository.default_branch` from the signed push payload because the active source is following the repo default branch. In filesystem mode or any non-Redis API runtime, a validated push still returns `202 Accepted` with `status=accepted`, `processed=false`, and an explicit unavailable reason. When the active desired-state source is GitHub, the signed push matches that source, and the API is running with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis-backed controller refresh handoff and returns `processed=true` plus an additive `controllerRefresh` object with `backend`, `channel`, `requestId`, and `publishedAt` so operators can see that the handoff was actually published. Signed non-push events return `200 OK` with `status=ignored` and `processed=false`. Malformed headers, signatures, or JSON fail with `400 Bad Request`.

For the current read-only plan/apply flow, a typical local operator path is:
- `GET /v1/overview/applications/{deploymentName}` or `GET /v1/overview/deployments/{deploymentName}` to inspect the deployment and capture its additive `sourceFingerprint`
- open `http://localhost:8080/?page=audit` at any point for the bookmarkable audit page backed by `GET /v1/audit-logs`; application detail plus deployment-scoped drift/plan workspaces now link into `/?page=audit&deployment={deploymentName}` so the audit chrome/body stays explicitly scoped to the active deployment, and the page renders deployment-specific empty-state copy when that deployment has no persisted entries. When a deployment is selected, the same in-shell breadcrumb chrome plus quick-navigation strip used by the drift/plan workspaces appears in the audit body so operators can jump with `_self` links to the applications overview, application detail, drift view, or plan/apply workspace while the current deployment-scoped audit route stays visible as a non-click active chip without dropping bookmarkable `q` / `sync` / `account` / `deployment` / `action` / `limit` state. Add `&action=plan.created|apply.requested` to narrow the embedded timeline to one action, and keep using `&limit={n}` when you want a smaller newest-first window from the API. The audit toolbar now exposes the same limit as an explicit safe preset selector while still honoring bookmarked positive integers such as `?limit=2`, and `Refresh audit log` re-fetches only `GET /v1/audit-logs` in place while keeping the current bookmarkable `/?page=audit...` URL/query state stable.
- `GET /v1/audit-logs?deployment={deploymentName}` when you want only one deployment's immutable plan/apply entries without changing the route shape
- `GET /v1/audit-logs?action=plan.created` when you want only immutable plan-preview creation records
- `GET /v1/audit-logs?action=apply.requested` when you want only immutable apply-request records
- `GET /v1/audit-logs?action={action}` when you want only one immutable action type; `action` must be exactly `plan.created` or `apply.requested`
- `POST /v1/plans/deployments/{deploymentName}` with optional `{"ifMatch":"<sourceFingerprint>"}` to persist an immutable preview for the reviewed state; when a local proxy or test client has authenticated operator context, forward it explicitly with additive `X-Tenet-Actor-Id`, `X-Tenet-Actor-Name`, and/or `X-Tenet-Actor-Email` headers, or the issue-10-compatible aliases `X-User-ID`, `X-User-Name`, and/or `X-User-Email`; when both forms are present, Tenet prefers the explicit `X-Tenet-Actor-*` values without changing the route shape
- open `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}` for the bookmarkable plan workspace; `Apply Plan` activates only after that persisted ready preview is loaded with a valid `planId`, and the embedded `Refresh plan preview` / `Refresh apply request` controls re-fetch stored records in place without forcing a browser reload. Initial bookmarked `GET /v1/plans/{planId}` loads and later in-place refreshes now replace generic fetch-error copy for `401` / `403` with operator-facing retry/access guidance while keeping deployment-derived fallback context visible in the same workspace
- `POST /v1/applies/plans/{planId}` to persist an immutable apply request derived from that ready preview, then reopen the same workspace at `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}`; before Tenet persists anything it re-validates the current deployment-detail fingerprint against the reviewed plan fingerprint and returns `409 Conflict` with clear operator-facing guidance to create a fresh preview when the plan is stale, so no new apply record is written from drifted input. When the fingerprints still match, the same explicit `X-Tenet-Actor-*` headers or additive `X-User-*` aliases are persisted when present, with `X-Tenet-Actor-*` winning if both are supplied, and the stored apply record carries additive ordered `operations[]` rows with `phase`, `target`, and `summary` projected from the reviewed plan preview plus `status=pending` in the default disabled contract or `status=queued` on create in the opt-in local-safe executor contract, together with an additive `lifecycle` block that starts with `updatedAt` and `totalSteps`
- `GET /v1/applies/{applyId}` to reopen the stored apply request directly; this backend slice is disabled by default for live execution, so failed immutable apply records with explicit disabled error/details and pending operation rows are expected and useful because they preserve operator-reviewed intent without mutating AWS. When you opt in with `TENET_AWS_ENABLE_APPLY=true`, the backend still creates the same immutable apply record as `queued`, but a Tenet-owned local-safe executor loop now advances that same `applyId` through `running` and finally `failed` with operator-facing handoff details explaining that live AWS mutation is still not implemented. That opt-in currently requires the filesystem catalog source via `TENET_REPO_ROOT`; GitHub catalog mode remains read-only. The stored read model keeps additive `lifecycle` metadata for that executor handoff, including `updatedAt` / `startedAt` / `finishedAt` timestamps plus `currentStep` / `completedSteps` / `totalSteps`, while the embedded `/?page=plan` workspace still renders lifecycle-aware queued/running/succeeded/cancelled/failed chrome, the existing compact stored-apply lifecycle summary from persisted `operations[]`, and a separate stored-apply lifecycle metadata block for timing plus current/completed step progress when those fields are present, without changing the route shape or job identity. Initial bookmarked `GET /v1/applies/{applyId}` loads and later in-place refreshes now replace generic fetch-error copy for `401` / `403` with explicit operator guidance while keeping plan-derived fallback context visible
- `POST /v1/applies/{applyId}/transitions` remains available when executor or worker code needs to advance that persisted immutable apply record on the same stable `applyId`; Tenet's local-safe executor uses the same `applyrequest.TransitionRequest` state model internally, so operators do not need manual transition API calls to see enabled-mode lifecycle progress. When front-door auth enforcement is enabled, that same route now rejects unauthenticated callers with the existing operator-write `401 Unauthorized` body, and when deployment-read authorization is enabled Tenet loads the persisted apply record to discover its deployment before enforcing the trusted `X-Tenet-Authorized-Deployments` allowlist. `make smoke-local` and `make smoke-docker` both reuse a smoke-created terminal `applyId` to prove malformed JSON bodies fail with `400 Bad Request` (`invalid apply transition request body`), unknown `applyId` values fail with `404 Not Found` (`apply request not found`), and invalid lifecycle moves on that terminal record fail with `409 Conflict` plus clear operator-facing text instead of creating a replacement apply record
- `GET /v1/audit-logs?limit={n}` to project those persisted immutable plan/apply records into a newest-first local audit timeline; `/v1/audit-logs` is the preferred v1 route shape for this local timeline, and actor identity remains explicit in both directions: persisted request actor fields when available, or `actor.available=false` plus the unavailable reason `actor information is unavailable because no authenticated request context was available for this local request` when no authenticated request context was present
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}` when you need one deployment plus one exact action without changing the route family
- `GET /v1/audit-logs?action={action}&limit={n}` to keep the same newest-first and limit behavior while narrowing the timeline to exactly `plan.created` or `apply.requested`
- `GET /v1/audit-logs?deployment={deploymentName}&limit={n}` to keep the same newest-first and limit behavior while narrowing the timeline to one deployment; when no persisted audit records match, the API still returns `200 OK` with `entries: []`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}&limit={n}` when you need the same deployment-scoped bookmarkability plus one exact action type

In `compose.yaml`, the local `api` service pins `TENET_PLAN_STORE_PATH` and `TENET_APPLY_STORE_PATH` to `/tmp/tenet/plan-previews.json` and `/tmp/tenet/apply-requests.json`, then mounts a named volume on `/tmp` so immutable plan/apply records survive the Compose `api` service restart used by `make smoke-docker`.

Front-door auth remains additive and opt-in. Tenet still expects your real authentication layer to run before requests reach the API. Set `TENET_API_TRUST_AUTHENTICATED_HEADERS=***` when the `tenet-api` instance should trust front-door-forwarded `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, and/or `X-Tenet-Authenticated-Email` headers; when those explicit trusted fields are absent, the same trusted authenticated-identity path also accepts additive `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases. `X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for that trusted authenticated identity path. Set `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***` when `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` must reject unauthenticated requests with `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`; enabling enforcement also trusts those forwarded identity headers for that API instance. Set `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=***` when those same operator write routes must additionally require one trusted forwarded `X-Tenet-Authenticated-Roles` header containing normalized comma-separated roles and at least one operator-capable role from `operator` / `admin`; when authenticated identity exists but neither role is present, the route now returns `403 Forbidden` plus `forbidden: authenticated operator role is required for this operator write route` before any plan/apply lookup. Set `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***` when operator reads must require trusted authenticated identity plus one trusted forwarded `X-Tenet-Authorized-Deployments` header containing a comma-separated exact deployment allowlist. That same allowlist now also gates write creation for `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` using the requested deployment or persisted preview deployment, and it gates `POST /v1/applies/{applyId}/transitions` using the persisted apply deployment, so recognized identity without access returns `403 Forbidden` plus `forbidden: operator does not have access to this deployment` before Tenet persists or mutates an immutable plan/apply record. Under that same read-authorization mode, unauthenticated transition callers still fail with the existing operator-write `401` before any apply lookup. This read gate is intentionally narrow: `GET /v1/overview/applications` now requires trusted authenticated identity and filters the returned application rows to only deployments present in that allowlist, while `GET /v1/overview/applications` returns `200 OK` with an empty JSON array when the trusted allowlist is missing or none of the returned rows match it. `GET /v1/status/deployments` now follows the same collection contract and filters returned raw snapshots through that allowlist, also returning `200 OK` with `[]` when the trusted allowlist is missing or none of the returned rows match it. `GET /v1/overview/accounts` now follows the same collection contract and filters returned account aggregates down to authorized deployments only, also returning `200 OK` with `[]` when the trusted allowlist is missing or none of the returned rows match it. Unscoped `GET /v1/audit-logs` now follows that same collection contract under read enforcement: it requires trusted authenticated identity, filters returned audit entries down to deployments present in `X-Tenet-Authorized-Deployments`, and returns `200 OK` with `{"entries":[]}` when the trusted allowlist is missing or no audit rows are visible. Scoped overview reads now also include `GET /v1/overview/accounts/{awsAccountId}` and `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}` alongside `GET /v1/overview/applications/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, raw status detail reads `GET /v1/status/deployments/{deploymentName}`, deployment-scoped `GET /v1/audit-logs` when `deployment={deploymentName}` is present, and immutable stored reads `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` using the persisted record deployment. `GET /v1/status/deployments/{deploymentName}` now reuses the same scoped deployment-read authorization contract. Focused router coverage plus both smoke paths now explicitly probe `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` for `401 Unauthorized` unauthenticated reads, `403 Forbidden` authenticated-but-unauthorized reads, and `200 OK` authorized reads, while preserving the existing `available=false` plus explicit `unavailableReason` payload contract when live lookup data is unavailable. When read enforcement is enabled, missing trusted authenticated identity returns `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`; recognized identity without any authorized deployment visible inside one scoped account or grouped account response returns the existing `403 Forbidden` plus `forbidden: operator does not have access to this deployment`, while scoped deployment reads still require exact deployment membership in `X-Tenet-Authorized-Deployments`. Only enable either setting when every request reaches Tenet through a front door that authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*` headers. Do not expose a header-trusting `tenet-api` instance directly to untrusted clients. When that `401` happens in the embedded `/?page=plan` workspace, Tenet now explains it in operator-facing language: authenticated identity is required on operator write routes, and operators must retry through the trusted front door instead of sending the write directly to Tenet. When that new write-route `403` happens, the same workspace explains that the trusted front door authenticated the caller but did not forward an operator-capable `X-Tenet-Authenticated-Roles` value for this session. The embedded read-only pages now use the same front-door framing on initial page loads: `401 Unauthorized` on the default applications landing page `/`, `/?page=application`, `/?page=resource`, `/?page=drift`, `/?page=audit`, `/?page=account`, or bookmarked `/?page=plan&planId=...` / `/?page=plan&applyId=...` record loads tells operators to retry that bookmarkable page through the trusted front door so authenticated identity can be provided, while `403 Forbidden` explains that identity was recognized but the current operator is not authorized for that route or deployment. The plan/apply workspace uses the same status-aware guidance for in-place `Refresh plan preview` and `Refresh apply request` reloads while keeping deployment/plan-derived fallback context visible in the page body. The application-detail `Controller runtime events` panel now uses that same status-aware guidance for in-place `Refresh runtime events` reloads, so `401` / `403` replaces generic fetch-error copy without reloading the rest of the page or mutating the bookmarkable `/?page=application...` route. The resource-detail `Recent logs` and `Live inspection` panels now use that same status-aware guidance for in-place `401` / `403` lazy-panel failures while keeping the rest of the resource page, the sibling panel, and the bookmarkable `/?page=resource...` route stable. **Security constraint:** only enable any of these settings when every request reaches Tenet through a front door that authenticates the caller, strips any client-supplied `X-Tenet-Authenticated-*` headers before forwarding its own trusted identity values, strips any client-supplied `X-Tenet-Authenticated-Roles` header before forwarding trusted roles, and strips any client-supplied `X-Tenet-Authorized-Deployments` header before forwarding the trusted deployment allowlist. Do not expose a header-trusting `tenet-api` instance directly to untrusted clients, or they can spoof operator identity or deployment access by sending those headers themselves. Explicit `X-Tenet-Actor-*` or `X-User-*` actor headers remain backward-compatible for persisted operator context, and `X-Tenet-Actor-*` values still win over `X-User-*` aliases for that local actor path. For persisted operator context, the trusted authenticated identity path resolves from `X-Tenet-Authenticated-*` headers or additive `X-User-*` aliases, and when explicit actor headers are absent Tenet now projects that authenticated identity into the persisted actor fields on immutable plan/apply records and their projected audit entries.

Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis; it now also proves the executor-facing `POST /v1/applies/{applyId}/transitions` boundary on a smoke-created terminal apply record (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move) plus the request-actor header contract by sending conflicting `X-Tenet-Actor-*` and `X-User-*` identities together and verifying the stored plan preview, stored apply request, and deployment-scoped audit lookup keep the explicit `X-Tenet-Actor-*` values, fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route, the bookmarkable `/?page=account&accountId=123456789012` route, the existing bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, plus one representative bookmarkable ECS resource route at `/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService::worker-prod-service::ECS service Tenet updates with native rolling deployment`, and checks only raw embedded application/account/resource shell strings that prove those drill-down pages can render and navigate, plus the landing-page helper strings that keep `/`, the bookmarkable application drill-down, and the bookmarkable account drill-down on the shared operator-read auth guidance path under trusted front-door auth, and the front-door auth contract by restarting `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`, proving unauthenticated operator writes fail with `401`, verifying trusted `X-Tenet-Authenticated-*` headers project into the stored plan/apply/audit actor fields when explicit actor headers are absent, and proving unauthenticated `POST /v1/applies/{applyId}/transitions` fail with `401`, then restarting again with `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` to prove guarded application, raw-status collection, account-collection, unscoped audit collection, raw-status detail, account-detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes, all enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments`, while collection routes filter to visible deployments and return `[]` when the allowlist has no matches. The unscoped audit collection keeps the same read-only shape under that gate by returning `{"entries":[]}` when the allowlist is missing or no audit rows are visible. The authorized drill-down smoke keeps the existing route behavior explicit by asserting those two resource reads still return `200 OK` with their normal payload shape and preserve `available=false` plus `unavailableReason` when local/live lookup data is unavailable. Use `make smoke-docker` when you need to validate `compose.yaml`, confirm the `api`/`controller`/`redis` service contract, and prove the containerized local stack plus representative `Request-Id` headers, correlated structured API request logs for the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down shell request alongside the existing bookmarkable `/?page=drift&deployment=payments-api-prod` workspace request and the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit workspace request, controller logs, deployment events, resource-log routes, non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, the bookmarkable `/?page=application&deployment=payments-api-prod` route, the bookmarkable `/?page=account&accountId=123456789012` route, the existing bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, one representative bookmarkable ECS resource route at `/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService::worker-prod-service::ECS service Tenet updates with native rolling deployment`, the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace source/summary/table copy, the full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present, the exact-action `GET /v1/audit-logs?action=apply.requested&limit=2` smoke, the invalid-action `400 Bad Request` boundary, the same smoke-created `POST /v1/applies/{applyId}/transitions` route boundaries (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit view, checks raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present on those bookmarkable routes, including the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down, and on the default applications landing page, the default disabled immutable apply contract, additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence through immutable plan/apply/audit records, mirrors the direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval, then restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` to prove unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/status/deployments`, unauthenticated `GET /v1/overview/accounts`, and unauthenticated `GET /v1/audit-logs?action=apply.requested` fail with `401`, unauthenticated `POST /v1/applies/{applyId}/transitions` fail with `401`, the same exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes fail with `401` unauthenticated and `403` when the allowlist excludes that deployment, trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, raw status snapshots, account aggregates, and unscoped audit entries, authenticated but unauthorized `POST /v1/applies/{applyId}/transitions` fails with `403`, and authorized resource drill-down reads still preserve `available=false` plus `unavailableReason` when the compose-backed runtime lacks live lookup data. The same docs contract also means the smoke restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=***` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***` to prove unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/status/deployments`, unauthenticated `GET /v1/overview/accounts`, and unauthenticated `GET /v1/audit-logs?action=apply.requested` fail with `401`, while the bookmarkable account drill-down stays on that same shared operator-read guidance path.

GitHub Actions mirrors those checks in two lanes under `.github/workflows/ci-smoke.yml`. The fast lane runs on every pull request plus pushes to `main`, pins the Go toolchain from `go.mod`, runs `go test ./...`, syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`. The slower Docker lane stays outside the PR fast path and, on pushes to `main` plus manual `workflow_dispatch` runs, syntax-checks `scripts/smoke-docker.sh` with `bash -n` before running `make smoke-docker`.

Both smoke scripts keep intermediate deployment-detail, plan-preview, and apply-record payloads on temp files via the shared `write_payload` helper instead of passing full JSON blobs through environment variables. That keeps the shell/Python handoff portable on Linux CI runners where environment-size limits are tighter than local shells.

The same trusted front-door read contract also now explicitly covers `GET /v1/overview/deployments/{deploymentName}/events`: smoke/docs verification treat it like the other deployment-scoped drill-down routes, with `401 Unauthorized` unauthenticated reads, `403 Forbidden` authenticated-but-unauthorized reads, and `200 OK` authorized reads while preserving the existing `deploymentName` plus `events` list payload contract.

### Run with Docker

```bash
make docker-up
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project. Inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`. If you want to clear a prior Tenet stack before retrying, run `make docker-down`, then rerun `docker compose up --build -d`.

Leave `TENET_AWS_ENABLE_APPLY` unset for the default disabled immutable apply contract. Set `TENET_AWS_ENABLE_APPLY=true make docker-up` when you want the queued-on-create local-safe executor handoff contract in the local Compose stack on a fresh plan/apply pair; this still does not execute AWS mutations.

The local Compose stack already sets a safe default `TENET_GITHUB_WEBHOOK_SECRET=smoke-webhook-secret`, so `make smoke-docker` can send signed webhook requests immediately. Override `TENET_GITHUB_WEBHOOK_SECRET` when you want a different local secret, and set it explicitly for direct-process runs. The default local stack still proves the explicit unavailable path because it uses the filesystem desired-state source, so accepted webhook pushes stay `processed=false` there. In GitHub catalog mode with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis controller refresh handoff and the controller reconciles immediately on receipt instead of waiting only for the next ticker interval.

Run the dedicated Docker-stack smoke before or instead of manual exploration when you want a single command that validates the Compose config, boots the stack, waits for API health, checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, sends a signed GitHub `push` request, a signed non-`push` request, and an invalid-signature request to `POST /api/v1/github/webhook` and asserts the `202 Accepted` / `200 OK` / `400 Bad Request` contract plus the filesystem-mode `desiredStateSourceMatch`, validates non-empty controller-authored `managedDrift[]` evidence in the compose-backed `GET /v1/overview/deployments/payments-api-prod` payload with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the default disabled immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, mirrors the direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval, reuses that stable terminal `applyId` to prove `POST /v1/applies/{applyId}/transitions` rejects malformed JSON with `400`, unknown apply IDs with `404`, and invalid lifecycle moves with `409` plus operator-facing terminal-state text, fetches the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down shell URL plus the bookmarkable `/?page=account&accountId=123456789012` drill-down shell URL alongside the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace URL plus the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit workspace URL (and still supports the action-only `/?page=audit&action={action}&limit={n}` shape) and the bookmarkable `/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}` workspace URL, checks raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present on those bookmarkable routes, and also checks raw embedded application/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the same guidance stays visible on the bookmarkable plan workspace. The smoke now also locks the resource quick-navigation current-section contract (`Current section`, the non-click chip text, and the empty-state copy when no hash-targeted section is active) alongside the existing resource shell route builders. The smoke confirms the drift page source still includes `Active drift source`, an `On this page` drift-section navigator that preserves the current pathname/query while appending `_self` hashes for `Runtime signals`, `Rollout execution`, `Field-level drift`, and `Status conditions`, drift-workspace `Linked resources` navigation with `_self` resource drill-down links built from the existing normalized inventory (plus explicit empty-state copy when none exist), `Drift summary`, and field-level table headers such as `Field path` / `Desired` / `Actual` / `Severity`, confirms the audit page preserves active filter copy such as `Audit filters: action apply.requested · limit 2`, confirms the `api` container emits correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request, restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=***` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***` to prove unauthenticated `GET /v1/overview/applications` fails with `401`, reloads the same stored `GET /v1/plans/{planId}` and `GET /v1/applies/{applyId}` records through trusted front-door headers so they survive the Compose `api` service restart, trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, and omitting that allowlist returns `[]`, confirms the embedded UI still includes current drill-down labels such as `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured`, and finally restarts the Compose `api` service with `TENET_AWS_ENABLE_APPLY=true` to create a fresh queued immutable `worker-prod` apply record from a new plan preview so the earlier disabled `payments-api-prod` record stays unchanged before the local-safe executor handoff marks that new `applyId` failed with explicit no-live-mutation details. The smoke confirms controller logs are useful and tears the stack back down. For the controller-populated raw status collection, when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows. The same guarded Compose auth pass also covers `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` for `401` unauthenticated reads, `403` unauthorized reads, and allowed authorized reads while preserving the existing unavailable payload contract when live AWS access is absent. In exact contract terms, omitting that allowlist returns `[]` for those guarded collections. `GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`, and one allowed raw status detail read succeeds. The bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures.

```bash
make smoke-docker
```

This starts the local stack:
- `api` on `http://localhost:8080`
- `controller` as a long-running repo-validating process that now reloads fixtures and logs reconcile plans every few seconds
- `redis` as the active runtime status backend, seeded from `examples/local-status/deployments.json` when empty

Open:

```bash
http://localhost:8080/
```

To stop it:

```bash
make docker-down
```

## Using the UI

The intended operator flow is:

### Applications overview
Use the first page to scan:
- which applications exist
- which account they belong to
- sync and health state
- last update / current runtime signal

The applications overview also exposes an `_self` `Open audit log` link to the dedicated bookmarkable `/?page=audit` slice.

### Audit timeline
Use the audit page when you need immutable operator activity instead of controller-derived runtime history.

The page is backed directly by `GET /v1/audit-logs` and currently focuses on:
- newest-first `plan.created` and `apply.requested` entries
- a bookmarkable action filter that narrows the page and fetch route to exactly `plan.created` or `apply.requested`
- a bookmarkable limit control in the audit toolbar that updates the fetch route and URL in place while preserving deployment and action filters
- an explicit in-place `Refresh audit log` control that re-fetches only `GET /v1/audit-logs`, disables only while that audit fetch is active, and keeps the current bookmarkable `/?page=audit...` URL/query state stable
- active filter copy such as `Audit filters: action apply.requested · limit 2` when you reopen `/?page=audit&action=apply.requested&limit=2`
- deployment-scoped filter copy such as `Audit filters: deployment payments-api-prod · action apply.requested · limit 2` when you reopen `/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2`
- deployment / app / environment context
- deployment-scoped chrome/body copy plus a deployment-specific empty state when `deployment={deploymentName}` is present and no persisted audit entries match
- initial `GET /v1/audit-logs` auth failures now render operator-specific guidance instead of generic fetch-error copy: `401` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that the current authenticated operator is not authorized for that route or deployment
- keeps the current audit content visible with explicit loading/error copy while that in-place refresh is running or fails for the same bookmarkable route
- explicit actor-unavailable or actor-unknown labeling instead of fabricated identity
- `_self` links back into `/?page=application&deployment={deploymentName}` as `Open application drill-down` and `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}` as `Open plan/apply workspace` when those identifiers exist

### Account detail
Open `/?page=account&accountId={awsAccountId}` to get the denser account/group deployment slice without leaving the embedded shell. Its initial `GET /v1/overview/accounts/{awsAccountId}` load now uses the same operator-read auth handling as the other bookmarkable drill-downs: `401` tells the operator to reopen the same page through the trusted front door so identity can be forwarded, `403` explains that identity was recognized but this operator is not authorized for that route or deployment, and other statuses keep the existing unavailable/generic fetch-error behavior.

### Application detail
Open one application to see the linked rollout resources grouped into clear sections such as:
- workloads
- traffic routing
- config and access

On the initial `GET /v1/overview/applications/{deploymentName}` load, embedded auth failures are also status-aware: `401` tells the operator to reopen the same bookmarkable page through the trusted front door so identity can be forwarded, `403` explains that identity was recognized but this operator is not authorized for that route/deployment, and other statuses keep the existing unavailable/generic fetch-error behavior.

### Resource detail
Open a concrete resource to inspect the deeper operational context for that resource, including rollout and runtime information. The embedded resource page now includes a compact `Quick navigation` strip with a non-click `Current resource` chip, a hash-aware non-click `Current section` chip when the bookmarkable route targets one of the rendered resource sections, and an `On this page` group for the remaining stable section hashes such as `#resource-runtime-history`, `#resource-live-inspection`, and `#resource-recent-logs`. The existing `_self` jumps back to the application, drift, plan/apply, audit, and sibling resources remain unchanged. The resource page also includes a `Live inspection` panel that lazy-loads the resource-inspection route for the selected resource while keeping bookmarkable resource URLs unchanged. The panel renders explicit loading, unavailable, empty, and supported states, reuses the same safe DOM construction patterns as the rest of the embedded UI, and keeps lookup context visible when Tenet cannot return a live payload. Operators now also get an in-place `Refresh live inspection` control on that panel so they can re-run only that panel-local backend route in place; the control disables only while its own fetch is active, preserves the bookmarkable URL, and leaves the sibling panel and the rest of the resource page visible. When that lazy read returns `401` or `403`, the panel keeps the same in-place refresh behavior but replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths. EC2 `userDataBootstrapSource` resources now also render a dedicated live-inspection payload there, reusing the same launch-template bootstrap-source evidence Tenet already uses for controller-managed drift so operators can compare the normalized desired bootstrap source against the observed launch-template marker without leaving the existing resource drill-down route. When that bootstrap-source payload also includes `backingResourceType` plus `backingResourceName`, the same panel now renders an explicit `_self` handoff to the resolved backing resource drill-down using the existing normalized `rolloutExecutionResources()` resource identity mapping, so `resourceId` stays stable and the handoff simply disappears when Tenet cannot safely resolve the backing resource from the current deployment detail.

Recent logs are still surfaced here when the resource type supports them, and the recent-log card keeps the backend lookup path explicit with operator-facing labels for region, control account, target account, cross-account status, and observer-role configuration even when no lines are returned or the lookup is unavailable. That card now also includes its own `Refresh recent logs` control so operators can re-run only that panel-local backend route in place with the same per-panel loading disablement while leaving the sibling panel and the rest of the resource page visible, and it uses the same trusted-front-door / access-denied `401` / `403` guidance instead of generic fetch-error text when the guarded lazy read is denied. `userDataBootstrapSource` now participates in that same recent-log path by reusing the backing Auto Scaling Group Tenet already resolves for bootstrap-source inspection; when that happens, the payload stays read-only and additive by preserving the requested bootstrap-source identity while also surfacing optional `backingResourceType` and `backingResourceName` fields for the resolved Auto Scaling Group. When no backing group or AWS access is available, the route preserves the same explicit unavailable payload states instead of failing the page. Shared resource deep links keep the current query state, update the `Current section` chip after in-page hash changes, and reapply hash scrolling after the lazy `Recent logs` or `Live inspection` panel finishes repainting, so reopening `/?page=resource...#resource-recent-logs` or `#resource-live-inspection` lands back on the intended section. A parallel read-only resource inspection route now exposes explicit typed backend payloads for supported live AWS resources without changing the bookmarkable drill-down shape.

The compose-backed smoke now covers that shipped bootstrap-source drill-down directly: it fetches the bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` route plus the matching `GET /v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=file:userdata/payments-api-prod.sh` and `GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=file:userdata/payments-api-prod.sh` reads, and treats explicit unavailable reasons such as `aws_unavailable` and `no_log_source` as part of the expected local no-AWS contract instead of assuming live AWS data.

Canonical resource-log routes stay stable across rollout modes so operators can bookmark or script them:
- `asgWeightedTargetGroups`: `trafficShiftPlan` (`{deployment}-weighted-routing`)
- `ecsRolling`: `ecsService` (`{deployment}-service`)

Behind those routes, the built-in AWS-backed recent-log provider now reuses SDK config/client setup per `(region, target account, observer role)` key inside a running Tenet API process instead of rebuilding ECS / CloudWatch Logs / Auto Scaling / EC2 clients on every fetch.

Current provider-backed recent-log support includes:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource` via the resolved backing Auto Scaling Group

The same drill-down now has a parallel inspection route family:
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`

Supported always-live inspection currently starts with:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`

Supported traffic-routing inspection currently covers:
- `codeDeployDeploymentGroup`
- `productionListener`
- `testListener`
- `trafficShiftPlan`

Within one running Tenet API process, repeated successful live inspections for `codeDeployDeploymentGroup`, `productionListener`, and `testListener` now reuse a bounded in-process cache keyed by `(region, target account, observer role, resource type, resource name)`. Summary-only fallbacks and unavailable outcomes are still recomputed or returned through the existing non-cached path.

For `ecsService`, Tenet now keeps a bounded in-process hint of the last cluster that resolved a given `(region, target account, observer role, service name)` lookup so repeated drill-downs avoid brute-force cluster scans. If that hint is stale or the service moved, the lookup falls back to the full cluster scan path, refreshes the hint, and keeps the existing resource-not-found semantics unchanged.

The embedded `Live inspection` panel renders operator-friendly summaries from those payloads:
- `ecsService`: service identity/status, desired-running-pending counts, additive deployment-configuration fields (`minimumHealthyPercent`, `maximumPercent`, `healthCheckGracePeriodSeconds`) when ECS returns them, active deployments, and recent ECS service events
- `autoScalingGroup`: group sizing/health settings, additive launch-template metadata, attached target groups, and current instances
- `userDataBootstrapSource`: normalized desired bootstrap source, observed launch-template bootstrap-source marker, the backing Auto Scaling resource Tenet inspected, the launch template version Tenet used for that observation, and an additive in-app handoff to the resolved backing resource drill-down when that resource can be matched safely from the loaded deployment metadata
- `trafficRoutingResource`: summary-only or live traffic-routing context with explicit resource kind, display name, inspection mode, rollout summary text, and additive live fields for CodeDeploy deployment groups or listeners when AWS access is available

Resource-inspection responses keep the contract explicit for UI consumers:
- common fields: `deploymentName`, `resourceType`, `resourceName`, `lookupContext`, `available`, `source`, `unavailableReason`
- ECS payloads add `ecsService`, including additive deployment-configuration fields `minimumHealthyPercent`, `maximumPercent`, and `healthCheckGracePeriodSeconds` when `DescribeServices` returns them
- Auto Scaling payloads add `autoScalingGroup`, including optional `launchTemplate` and `mixedInstancesPolicy` metadata when `DescribeAutoScalingGroups` returns launch-template details
- EC2 bootstrap-source payloads add `userDataBootstrapSource`, including `desiredBootstrapSource`, optional `observedBootstrapSource`, the backing Auto Scaling resource identity, and the observed launch template summary Tenet used for that comparison
- traffic-routing payloads add `trafficRoutingResource`

Traffic-routing inspection stays local-friendly without changing the route shape:
- `trafficShiftPlan` remains `summary_only`
- `codeDeployDeploymentGroup`, `productionListener`, and `testListener` stay `summary_only` when AWS inspection is disabled, and switch to `inspectionMode=live` with additive `codeDeployDeploymentGroup` or `listener` details when live AWS access succeeds
- AWS-side traffic-routing failures still return `available=false` with an explicit `unavailableReason` and `lookupContext`

Like recent logs, `lookupContext` is present for both `available=true` and unavailable inspection responses:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

Unsupported or temporarily unavailable resource types do not surface vague 500s from the provider layer. They return `available=false` with an explicit `unavailableReason` plus `lookupContext` so operators can see which region/account path Tenet attempted before continuing AWS troubleshooting elsewhere.

The same drill-down also includes read-only workspace surfaces:
- `/?page=drift&deployment={deploymentName}` for field-level troubleshooting context that now calls out the active drift-row source: controller-authored `managedDrift[]` rows when present, otherwise the existing derived fallback read-model heuristics for older payloads. Controller-authored rows can now also include live-backed EC2 Auto Scaling Group capacity evidence under stable field names such as `runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max` for `asgWeightedTargetGroups` and `asgInstanceRefresh` deployments when live AWS inspection succeeds. Those same EC2 inspections can now also add `runtimeContext.launchTemplate.bootstrapSource` when Tenet can read the primary Auto Scaling launch template version and parse an explicit bootstrap/userdata-source marker from its user data, letting operators see drift between the normalized desired deployment bootstrap source and the observed launch-template bootstrap source without adding a separate route family. ECS `ecsCodeDeploy` and `ecsRolling` deployments can also add live-backed ECS service rows under `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, and `runtimeContext.taskDefinition.image` when read-only ECS inspection resolves the service desired count, deployment configuration, and a single task-definition container image. Older payloads, local runs, or unavailable/missing AWS access still fall back cleanly without fabricating controller drift rows. Focused `go test ./internal/api` contract coverage locks that additive JSON/UI detail because local smoke may legitimately lack AWS access and therefore does not claim to prove live-backed capacity/bootstrap-source/deployment-config/image rows. Empty-state copy also distinguishes "no controller-authored drift reported" from "no derived fallback drift detected." When the controller itself owns the completed-plan runtime defaults and publishes non-empty `managedDrift[]`, that same deployment detail now surfaces `sync.state=Drifted` and drift-aware controller-authored completed summary text instead of misleading `Synced` wording, while preserving seeded/operator-authored sync and health evidence and active rollout states.
- the drift workspace also includes a compact summary above the field-level table with total rows, visible rows after the current filters, and live `high` / `medium` / `low` counts so operators can see severity distribution and filter impact at a glance
- the drift workspace exposes an explicit `Clear drift filters` control that resets the drift query, severity, and sort controls back to their defaults in one step without changing the bookmarkable route or re-fetching deployment detail
- the drift workspace quick-navigation strip now also exposes a hash-aware `Current section` group plus an `On this page` group with `_self` fragment links for `Runtime signals`, `Rollout execution`, `Field-level drift`, and `Status conditions`, preserving the current bookmarkable pathname/query while appending drift section hashes such as `/?page=drift&deployment={deploymentName}#drift-field-level`
- when one of those drift section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- the drift workspace quick-navigation strip now also exposes direct `_self` resource drill-down links built from the same grouped normalized resource inventory used by the application/resource pages, and renders explicit `No linked resources are available in this drift workspace yet.` copy when that inventory is empty
- `/?page=application&deployment={deploymentName}`, `/?page=resource&deployment={deploymentName}&resourceType={resourceType}&resourceId={resourceId}`, `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render explicit breadcrumb chrome that links prior drill-down levels with `_self` anchors, includes the account step when `awsAccountId` is already known from the loaded detail payload, and keeps the current destination as a non-click chip instead of a self-link
- the bookmarkable `/?page=resource&deployment={name}...` and `/?page=drift&deployment={name}` entry loads both depend on `GET /v1/overview/deployments/{deploymentName}` and now distinguish auth failures on that initial read: `401` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route or deployment
- `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render the same embedded quick-navigation strip for `Back to application`, `Drift view`, `Plan/apply`, and deployment-scoped `Audit log`, but the active workspace is rendered as a non-click chip instead of a self-link so operators can move laterally between read-only workspaces without relying on browser back or leaving the bookmarkable shell
- the plan/apply workspace quick-navigation strip now also exposes a hash-aware `Current section` group plus an `On this page` group with `_self` fragment links only for the plan/apply sections currently rendered, using stable ids such as `plan-runtime-signals`, `plan-preview-actions`, `plan-stored-preview`, `plan-stored-apply`, `plan-apply-operations`, and `plan-operations` while preserving the current bookmarkable pathname/query and appending hashes such as `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}#plan-apply-operations`
- when one of those plan/apply section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- `/?page=plan&deployment={deploymentName}` for immutable plan previews, `Apply Plan` activation once a persisted ready preview is loaded, bookmarkable `planId`/`applyId` workspace URLs, explicit `Refresh plan preview` / `Refresh apply request` controls that re-run the current `GET /v1/plans/{planId}` and `GET /v1/applies/{applyId}` reads in place, stored-preview quick links to both the embedded workspace (`_self`) and immutable `Plan JSON` / `Apply JSON` read models, lifecycle-aware apply status copy/chrome for queued/running/succeeded/cancelled/failed records, a compact stored-apply lifecycle summary with progress counts from persisted `operations[]`, stored-apply lifecycle timing/progress metrics sourced from `lifecycle.updatedAt` / `startedAt` / `finishedAt` / `currentStep` / `completedSteps` / `totalSteps` when present, a dedicated `Apply operations` table so operators can review the persisted execution sequence directly in the workspace, and explicit operator guidance for `401` / `403` failures on bookmarked `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` loads or refreshes while keeping deployment/plan-derived fallback context visible

The backend now also exposes read-only immutable apply-request routes:
- `POST /v1/applies/plans/{planId}`
- `GET /v1/applies/{applyId}`

These routes are additive and local-friendly. They persist immutable records to `TENET_APPLY_STORE_PATH`, keep live execution intentionally disabled by default in this slice, attach additive ordered `operations[]` rows so operators can inspect the future executor-facing `status` / `phase` / `target` / `summary` path without mutating AWS, and expose an additive `lifecycle` block so later executor transitions can report timing and step progress on the same stable `applyId`. The embedded `/?page=plan` workspace now renders those lifecycle fields directly when present and leaves missing step/timing fields as placeholders instead of inventing synthetic progress. `POST /v1/applies/plans/{planId}` also re-validates the current deployment-detail fingerprint against the reviewed plan fingerprint before persisting; stale plans fail with `409 Conflict` and operator-facing refresh guidance instead of writing a new apply record. Set `TENET_AWS_ENABLE_APPLY=true` when you want the backend contract to create a queued immutable apply request on a fresh plan/apply pair and then hand that new `applyId` to the local-safe executor stub while keeping this slice mutation-free.

The same immutable stores also back a read-only audit projection route:
- `GET /v1/audit-logs`
- `GET /v1/audit-logs?deployment={deploymentName}`
- `GET /v1/audit-logs?action={action}`
- `GET /v1/audit-logs?limit={n}`
- `GET /v1/audit-logs?action={action}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}&limit={n}`

This timeline is newest-first, projects persisted plan preview and apply request records, derives `appRef` / `environmentRef` from the local catalog when possible, and keeps actor identity explicit: records created with additive `X-Tenet-Actor-Id`, `X-Tenet-Actor-Name`, and/or `X-Tenet-Actor-Email` headers, or the compatible `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases, surface those fields, while unauthenticated or purely local requests still return `actor.available=false` plus an unavailable reason. When both header families are present, the explicit `X-Tenet-Actor-*` values win. `deployment` is optional and must name exactly one deployment when present. `action` is optional and must be exactly `plan.created` or `apply.requested` when present. A deployment filter with no matching persisted records still returns `200 OK` with `entries: []` so local operators can distinguish "no audit rows yet" from transport failure. The embedded UI now renders that route at `/?page=audit` with explicit loading, empty, unavailable, and generic fetch-error states for non-auth failures, plus distinct operator guidance on initial `401` vs `403` reads and a safe action filter control that preserves bookmarkable `action` / `deployment` / `limit` URL state.

Tenet also exposes a read-only deployment events slice for the same drill-down workflow:
- `GET /v1/overview/deployments/{deploymentName}/events`

The embedded `/?page=application&deployment={deploymentName}` drill-down now renders a small `Controller runtime events` timeline from that route with explicit loading, empty, unavailable, and non-auth fetch-error states. This route returns controller-derived runtime events copied from persisted `status.history`. It is intentionally operator-facing and refresh-based, not a full durable audit system yet, and the UI labels it separately from any future audit/auth history work. The same application page now also exposes a compact `Quick navigation` strip with a hash-aware `Current section` group plus an `On this page` group for `Workloads`, `Traffic routing`, `Config and access`, and `Controller runtime events`, so operators can reopen anchors such as `/?page=application&deployment={deploymentName}#application-runtime-events` without changing the fetch contract or leaving the embedded shell. When one of those section hashes is active, Tenet renders it as a non-click current-section chip, removes it from the remaining `On this page` links, and otherwise shows the friendly empty state `No section is currently targeted.`. The section header now includes an in-place `Refresh runtime events` control that re-fetches just `GET /v1/overview/deployments/{deploymentName}/events` in place, keeps the bookmarkable application route stable, and disables only while that events fetch is active. When that refresh returns `401` or `403`, the panel now shows the same trusted-front-door / access-denied guidance used on the other embedded read surfaces instead of generic fetch-error text. When the aligned runtime change label is `managedDrift`, the paired `changeDetails[]` text now stays single-slot but explains deterministic added, removed, and updated drift rows so operators can see what changed without diffing the whole `managedDrift[]` array by hand. When a deployment exists but no controller-authored runtime history has been persisted, the route still returns `200 OK` with `events: []`.

When fixture-backed or early controller payloads have not yet persisted `rollout.execution[]`, Tenet keeps the read-only drill-down contract stable by synthesizing the same fallback rollout resources the operator would otherwise see from the controller-authored payload:
- `ecsRolling`: `ecsService` (`{deployment}-service`)

For the ECS rolling fallback service, the recent-log drill-down stays on the same operator workflow. Example route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`

Resource-log responses now include an additive `lookupContext` object so the payload stays self-describing during AWS troubleshooting:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

When a requested resource resolves through a backing resource, the same response can also include additive backing-resource identity:
- `backingResourceType`
- `backingResourceName`

`lookupContext` is present for both `available=true` and unavailable responses, so operators can still confirm the attempted lookup region/account path when `entries` is empty or `unavailableReason` is set. When a bootstrap-source request resolved through a backing Auto Scaling Group before the downstream lookup ran, `backingResourceType` and `backingResourceName` remain present on both success and explicit unavailable responses. The embedded UI renders the lookup-context fields directly as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured` inside the `Recent logs` card.

## Repository layout

Tenet currently expects manifests under:

- `accounts/`
- `environments/`
- `deployments/`

Example data for local development lives in:

- `examples/local-repo/`
- `examples/local-status/`

## Current project status

Implemented today:
- repository-backed desired-state loading
- catalog compilation and validation
- controller/runtime status foundations
- filesystem and Redis runtime state backends
- read-only API and embedded UI
- applications -> application -> resource drill-down flow
- rollout/resource context for EC2 and ECS
- immutable plan preview creation/readback with stable `planId` and optimistic `ifMatch` support
- immutable apply-request creation/readback with stable `applyId`, plan validation, and disabled-by-default live execution semantics
- deployment events overview derived from persisted runtime history
- recent resource-log support for supported resource types
- resource inspection support for ECS services and deployment-linked Auto Scaling Groups

Not implemented yet:
- live AWS execution
- write/mutation APIs
- full plan/apply workflows
- a dedicated standalone frontend

## Documentation

For deeper design details, see:
- `docs/architecture/tenet-v1-overview.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-overview-ui.md`
- `docs/architecture/tenet-v1-aws-central-control-plane.md`
- `docs/features/resource-recent-logs.md`
- `deploy/terraform/aws-central-control-plane/README.md`

The AWS central-control-plane docs and Terraform bootstrap README now explicitly call out that the shipped repo-sync bootstrap path works out of the box only with an anonymously readable `repo_url` (for example a public HTTPS clone URL). Private desired-state repo authentication for that module is still a follow-up.

If you plan to use the AWS central-control-plane Terraform bootstrap, do the preflight before `terraform apply`: it builds and pushes the current local Tenet checkout from the machine running apply, requires Terraform 1.5+, Docker Buildx, and AWS CLI v2 with working credentials, and expects you to verify that `repo_url` and `repo_branch` are reachable from that machine first, for example with `git ls-remote --heads <repo_url> <repo_branch>`.

For the standalone architecture diagram, open this file locally in a browser:
- `docs/architecture/tenet-v1-architecture-diagram.html`

## Development

Run the main verification suite with:

```bash
go test ./...
make smoke-local
```

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
