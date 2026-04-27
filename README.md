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

This keeps the first page simple and pushes deeper operational detail into drill-down pages where it is easier to understand. The embedded application/resource/drift/plan/audit pages now also keep explicit breadcrumb chrome in the same shell so prior drill-down levels stay bookmarkable `_self` links, the current destination stays visible as a non-click chip, and the account step appears when Tenet already knows the deployment's `awsAccountId`. The application page quick-nav now makes the overview-to-detail hierarchy explicit with an `_self` `Applications overview` return link plus a non-click `Application detail` chip, while the bookmarkable account page now mirrors that same orientation pattern with an `_self` `Applications overview` return path plus a non-click `Account detail` chip. The deployment workspaces now use that same hierarchy pattern too: their shared quick-nav exposes `_self` `Applications overview` and `Back to application` return paths, keeps the active drift/plan/audit destination as a non-click chip, and still preserves compact bookmarkable `On this page` fragment links plus the existing lateral `_self` jumps to sibling workspaces or resources without backing out first.

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
TENET_CATALOG_GITHUB_TOKEN=*** \
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

Tenet now exposes the first concrete GitHub webhook ingress path at `POST /api/v1/github/webhook`. Configure `TENET_GITHUB_WEBHOOK_SECRET` to enable that route; the local Compose stack wires a safe default `smoke-webhook-secret` so `make smoke-docker` can prove the signed contract end to end without extra setup. The handler remains conservative: it requires `X-GitHub-Delivery`, `X-GitHub-Event`, and `X-Hub-Signature-256`, validates the HMAC-SHA256 signature, checks the JSON body shape, and returns an operator-friendly summary of a signed push payload. Signed non-`push` deliveries intentionally stay `status=ignored` so operators can distinguish an authenticated no-op from signature or payload failures. including `repository`, `ref`, `before`, `after`, `headCommit`, additive `changedFiles` lists, and an additive `desiredStateSourceMatch` object. That object keeps stable UI/docs-facing fields such as `configured`, `kind`, `matches`, `matchStatus`, `reason`, `repository`, `ref`, and `path` so operators can distinguish `not_configured`, `filesystem_source`, `repository_mismatch`, `ref_mismatch`, `path_untouched`, and `matched`. GitHub matching compares repository, normalizes branch or tag refs such as `main` vs `refs/heads/main` and `v1.2.3` vs `refs/tags/v1.2.3`, and treats `TENET_CATALOG_GITHUB_PATH` as a repo-relative prefix against changed files; when `TENET_CATALOG_GITHUB_REF` is unset, the webhook summary compares the push ref against `repository.default_branch` from the signed push payload because the active source is following the repo default branch. In filesystem mode or any non-Redis API runtime, a validated push still returns `202 Accepted` with `status=accepted`, `processed=false`, and an explicit unavailable reason. When the active desired-state source is GitHub, the signed push matches that source, and the API is running with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis-backed controller refresh handoff and returns `processed=true` plus an additive `controllerRefresh` object with `backend`, `channel`, `requestId`, and `publishedAt` so operators can see that the handoff was actually publish... [truncated]

For the current read-only plan/apply flow, a typical local operator path is:
- `GET /v1/overview/applications/{deploymentName}` or `GET /v1/overview/deployments/{deploymentName}` to inspect the deployment and capture its additive `sourceFingerprint`
- open `http://localhost:8080/?page=audit` at any point for the bookmarkable audit page backed by `GET /v1/audit-logs`; application detail plus deployment-scoped drift/plan workspaces now link into `/?page=audit&deployment={deploymentName}` so the audit chrome/body stays explicitly scoped to the active deployment, and the page renders deployment-specific empty-state copy when that deployment has no persisted entries. When a deployment is selected, the same in-shell breadcrumb chrome plus quick-navigation strip used by the drift/plan workspaces appears in the audit body so operators can jump with `_self` links to the applications overview, application detail, drift view, or plan/apply workspace while the current deployment-scoped audit route stays visible as a non-click active chip without dropping bookmarkable `q` / `sync` / `account` / `deployment` / `action` / `limit` state. Add `&action=plan.created|apply.requested` to narrow the embedded timeline to one action, and keep using `&limit={n}` when you want a smaller newest-first window from the API. The audit toolbar now exposes the same limit as an explicit safe preset selector while still honoring bookmarked positive integers such as `?limit=2`; `Refresh audit log` re-fetches only `GET /v1/audit-logs` in place while keeping the current bookmarkable `/?page=audit...` URL/query state stable; and `Clear audit filters` appears only when deployment/action/limit are active so operators can recover to the unfiltered `/?page=audit` slice without dropping bookmarkable `q` / `sync` / `account` state.
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

### Trusted front-door auth

Front-door auth remains additive and opt-in. Tenet still expects your real authentication layer to run before requests reach the API.

Use these settings only when every request reaches Tenet through a front door that authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, and `X-Tenet-Authorized-Deployments` headers before forwarding trusted values. Do not expose a header-trusting `tenet-api` instance directly to untrusted clients.

- `TENET_API_TRUST_AUTHENTICATED_HEADERS` trusts front-door-forwarded `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, and `X-Tenet-Authenticated-Email`. When those explicit trusted fields are absent, the same trusted authenticated-identity path also accepts additive `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases. `X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for that trusted authenticated identity path.
- `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY` requires authenticated identity on `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions`. Unauthenticated requests fail with `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`. When explicit actor headers are absent, the trusted authenticated identity path projects that identity into persisted plan/apply/audit actor fields.
- In the embedded plan/apply workspace, the same operator guidance stays explicit: authenticated identity is required on operator write routes, so operators should retry through the trusted front door instead of sending those writes directly to Tenet.
- `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` trusts `X-Tenet-Authenticated-Roles`, normalizes comma-separated roles, and requires one operator-capable `operator` / `admin` role on operator write routes. Authenticated callers without one of those roles get `403 Forbidden` plus `forbidden: authenticated operator role is required for this operator write route` before any plan/apply lookup.
- `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` trusts `X-Tenet-Authorized-Deployments` as a comma-separated exact deployment allowlist. That same allowlist gates `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions`, and it also filters `GET /v1/overview/applications`, `GET /v1/status/deployments`, `GET /v1/overview/accounts`, and unscoped `GET /v1/audit-logs` down to authorized deployments. Missing or non-matching allowlists return `200 OK` with `[]` or `{"entries":[]}` on those collection routes, while scoped reads still require exact deployment membership.
- When both `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` are enabled, guarded read routes additionally require one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin`. Authenticated callers without one now fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route` before Tenet evaluates deployment visibility.
- The same trusted front-door read contract covers `GET /v1/overview/accounts/{awsAccountId}`, `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, `GET /v1/overview/applications/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, `GET /v1/status/deployments/{deploymentName}`, deployment-scoped `GET /v1/audit-logs`, and immutable stored reads `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` using the persisted record deployment.
- The same trusted front-door read contract also covers `GET /v1/overview/accounts/{awsAccountId}` for the bookmarkable account-detail workspace and its project/environment drill-down.

### Smoke verification scope

Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis.

It verifies:
- API health plus representative catalog and status payloads.
- The executor-facing `POST /v1/applies/{applyId}/transitions` boundary on a smoke-created terminal apply record: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.
- The request-actor header contract by sending conflicting `X-Tenet-Actor-*` and `X-User-*` identities together and verifying the stored plan preview, stored apply request, and deployment-scoped audit lookup keep the explicit `X-Tenet-Actor-*` values.
- Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, one representative ECS resource route, and representative CodeDeploy `codeDeployApplication` / `blueTaskSetTargetGroup` resource routes, checking only raw embedded application/account/resource/drift/audit/plan shell strings needed to prove those drill-down pages can render and navigate.
- Trusted front-door auth by restarting `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS` plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY`, proving unauthenticated operator writes fail with `401`, verifying trusted `X-Tenet-Authenticated-*` headers project into stored plan/apply/audit actor fields when explicit actor headers are absent, and keeping the persisted `POST /v1/applies/{applyId}/transitions` write route guarded.
- Trusted front-door auth by restarting again with `TENET_API_TRUST_AUTHENTICATED_HEADERS`, `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`, and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`, proving unauthenticated read collections fail with `401`, authenticated callers without a read-capable role fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route`, deployment-filtered collections still return `[]` / `{"entries":[]}` when nothing remains visible, and deployment-scoped reads keep the existing deployment-access `403` when the operator lacks access.
- The same shared operator-read guidance path on `/`, the bookmarkable application drill-down, the bookmarkable account drill-down, the resource/drift/audit shells, and the bookmarkable `/?page=plan` workspace, but only behind a trusted front door that strips inbound trust headers before forwarding its own values.

Use `make smoke-docker` when you need to validate the containerized local stack itself.

It verifies:
- `compose.yaml` plus the `api`/`controller`/`redis` service contract.
- API health, representative endpoints, deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope and `testListener` backing-resource metadata, and representative `Request-Id` response headers.
- Correlated structured API request logs, controller logs, resource-log routes, and non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`.
- The same compose-backed verification also keeps correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace aligned with the documented request-correlation surface.
- The same bookmarkable application, account, traffic-shift resource, ECS resource, drift, audit, and plan routes locked by direct-process smoke, still checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present. In that compose-backed verification surface, the root applications shell now also locks the shipped `Refresh applications overview` route-builder/loading/error hooks, while the bookmarkable application shell still exposes the `_self` `Applications overview` return path, the non-click `Application detail` destination chip, and the bookmarkable application-detail status summary shell. The same route family now includes the CodeDeploy application and task-set target-group resource shells in both smoke lanes.
- Both smoke lanes now cover the shipped CodeDeploy application and task-set target-group bookmarkable resource shells directly. CodeDeploy application and task-set target-group resource shell coverage uses the same file-backed raw HTML/JS contract as the other resource pages. The same compose-backed shell coverage now also locks the bookmarkable resource page's top-level `Refresh resource detail` shell (`GET /v1/overview/deployments/{deploymentName}` route-builder text plus the preserved-detail loading/error copy) alongside the existing `Refresh recent logs` and `Refresh live inspection` lazy-panel route-builder/loading/error/auth-guidance hooks from the served HTML/JS shell instead of runtime DOM text, so release-gate smoke stays aligned with the shipped resource troubleshooting surface.
- The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Resolved backing resource`, `Lookup region`, `Observer role configured`, and the runtime-events `Open drift view` handoff present so bookmarkable operator drill-down pages stay aligned with the compose-backed verification surface.
- The full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present, exact-action `GET /v1/audit-logs?action=apply.requested&limit=2`, and the invalid-action `400 Bad Request` boundary.
- The same smoke-created `POST /v1/applies/{applyId}/transitions` route boundaries: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.
- The same actor-header precedence check used by direct-process smoke, plus the default disabled immutable apply contract.
- Trusted front-door auth by restarting the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS`, `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY`, `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`, and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`, proving unauthenticated guarded reads fail with `401`, authenticated callers without a read-capable `viewer` / `operator` / `admin` role fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route`, trusted `X-Tenet-Authenticated-Roles: viewer` permits the same guarded read surface, trusted `X-Tenet-Authorized-Deployments` filters returned application rows, raw status snapshots, account aggregates, and unscoped audit entries, unauthenticated `POST /v1/applies/{applyId}/transitions` fails with `401`, authenticated but unauthorized transitions fail with `403`, and authorized resource drill-down reads still preserve `available=false` plus `unavailableReason` when live lookup data is unavailable.
- Resource drill-down auth and smoke wording keep the `available=false` plus explicit `unavailableReason` payload contract visible for authorized reads when live lookup data is unavailable.

GitHub Actions mirrors those checks in two lanes under `.github/workflows/ci-smoke.yml`. The fast lane runs on every pull request plus pushes to `main`, pins the Go toolchain from `go.mod`, runs `go test ./...`, syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`. The slower Docker lane stays outside the PR fast path and, on pushes to `main` plus manual `workflow_dispatch` runs, syntax-checks `scripts/smoke-docker.sh` with `bash -n` before running `make smoke-docker`.

Both smoke scripts keep intermediate deployment-detail, plan-preview, and apply-record payloads on temp files via the shared `write_payload` helper instead of passing full JSON blobs through environment variables. That keeps the shell/Python handoff portable on Linux CI runners where environment-size limits are tighter than local shells. The same smoke/docs contract now explicitly says it checks raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text, including the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down, its bookmarkable application-detail status summary shell, and bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication::payments-api-staging-codedeploy-app::CodeDeploy application coordinating ECS traffic shifting` plus `/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup::payments-api-staging-blue::Blue task set target group CodeDeploy treats as the live ECS traffic destination` shells, so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present. The documented smoke surface still includes additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence through immutable plan/apply/audit records.

The same trusted front-door read contract also now explicitly covers `GET /v1/overview/deployments/{deploymentName}/events`: smoke/docs verification treat it like the other deployment-scoped drill-down routes, with `401 Unauthorized` unauthenticated reads, `403 Forbidden` authenticated-but-unauthorized reads, and `200 OK` authorized reads while preserving the existing `deploymentName` plus `events` list payload contract and the additive `managedDriftSummary` severity counts. That same read-contract wording now also keeps the exact operator-facing read body `unauthenticated: authenticated identity is required for this operator read route` visible in the README so the documented `401` surface matches the guarded routes, it keeps the matching deployment-visibility denial `forbidden: operator does not have access to this deployment` visible for scoped `403` cases, and it keeps the embedded write-route framing phrase `authenticated identity is required on operator write routes` visible for the bookmarkable plan/apply workspace guidance.

That compose-backed smoke description is intentionally explicit: it checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope and `testListener` backing-resource metadata, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, and then locks the matching bookmarkable shell contracts.

README auth/verification anchors for the current front-door slice:
- write creation for `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` now evaluates the requested deployment, the persisted preview deployment, and the persisted apply deployment on the matching guarded write/read paths
- `GET /v1/overview/applications` now requires trusted authenticated identity and filters the returned application rows to only deployments present in that allowlist
- `GET /v1/status/deployments` now follows the same collection contract, and `GET /v1/status/deployments/{deploymentName}` now reuses the same scoped deployment-read authorization contract
- Unscoped `GET /v1/audit-logs` now follows that same collection contract under read enforcement and returns `200 OK` with an empty JSON array or `{"entries":[]}` when the trusted allowlist is missing or none of the returned rows match it; stated exactly for the audit collection route, it returns `200 OK` with `{"entries":[]}` when the trusted allowlist is missing or no audit rows are visible
- `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.
- the unscoped audit collection still returns `{"entries":[]}` when no audit rows are visible
- the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace source/summary/table copy remains part of the documented compose smoke surface, and the same smoke reuses that stable terminal `applyId` to prove `POST /v1/applies/{applyId}/transitions` rejects malformed JSON with `400`, unknown apply IDs with `404`, and invalid lifecycle moves with `409` plus operator-facing terminal-state text; authenticated but unauthorized `POST /v1/applies/{applyId}/transitions` fails with `403`
- security wording stays explicit: only enable any of these settings when every request reaches Tenet through a front door that authenticates the caller, strips any client-supplied `X-Tenet-Authenticated-*` headers, strips any client-supplied `X-Tenet-Authorized-Deployments` header, and forwards only trusted values; operators must retry through the trusted front door when Tenet needs authenticated identity or trusted forwarded roles
- local actor precedence stays explicit: `X-Tenet-Actor-*` values still win over `X-User-*` aliases for that local actor path, and the trusted authenticated identity path resolves from `X-Tenet-Authenticated-*` headers or additive `X-User-*` aliases when explicit actor headers are absent; the trusted authenticated identity path resolves from `X-Tenet-Authenticated-*` headers or additive `X-User-*` aliases, and when explicit actor headers are absent Tenet now projects that authenticated identity into the persisted actor fields
- the documented bookmarkable shell coverage explicitly includes the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit view and correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request
- smoke/docs auth probes stay concrete: unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/status/deployments`, unauthenticated `GET /v1/overview/accounts`, and `GET /v1/audit-logs` all keep explicit `401` coverage
- raw shell/docs anchors remain literal: `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Resolved backing resource`, `Lookup region`, `Observer role configured`, and the runtime-events `Open drift view` handoff
- when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows, omitting that allowlist returns `[]` for those guarded collections, `GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`, one trusted `X-Tenet-Authenticated-Roles: viewer` raw status detail read succeeds, and the same compose restart keeps read-role denial and write-route allowlist denial distinct
- `make smoke-docker` also locks non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`

### Run with Docker

```bash
make docker-up
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project. Inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`. If you want to clear a prior Tenet stack before retrying, run `make docker-down`, then rerun `docker compose up --build -d`.

Leave `TENET_AWS_ENABLE_APPLY` unset for the default disabled immutable apply contract. Set `TENET_AWS_ENABLE_APPLY=true make docker-up` when you want the queued-on-create local-safe executor handoff contract in the local Compose stack on a fresh plan/apply pair; this still does not execute AWS mutations.

The local Compose stack already sets a safe default `TENET_GITHUB_WEBHOOK_SECRET=smoke-...et`, so `make smoke-docker` can send signed webhook requests immediately. Override `TENET_GITHUB_WEBHOOK_SECRET` when you want a different local secret, and set it explicitly for direct-process runs. The default local stack still proves the explicit unavailable path because it uses the filesystem desired-state source, so accepted webhook pushes stay `processed=false` there. In GitHub catalog mode with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis controller refresh handoff and the controller reconciles immediately on receipt instead of waiting only for the next ticker interval.

Use `make smoke-docker` when you need to validate `compose.yaml`, confirm the `api`/`controller`/`redis` service contract, boot the stack, wait for API health, check representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope and `testListener` backing-resource metadata, verify representative `Request-Id` response headers, and fetch the full `GET /v1/audit-logs` collection to assert the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`; the full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present is part of the documented smoke contract, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, sends a signed GitHub `push` request, a signed non-`push` request, and an invalid-signature request to `POST /api/v1/github/webhook` and asserts the `202 Accepted` / `200 OK` / `400 Bad Request` contract plus the filesystem-mode `desiredStateSourceMatch`, validates non-empty controller-authored `managedDrift[]` evidence in the compose-backed `GET /v1/overview/deployments/payments-api-prod` payload with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the default disabled immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, mirrors the direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval, reuses that stable terminal `applyId` to prove `POST /v1/applies/{applyId}/transitions` rejects malformed JSON with `400`, unknown apply IDs with `404`, and invalid lifecycle moves with... [truncated]

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

The applications overview also exposes an `_self` `Open audit log` link to the dedicated bookmarkable `/?page=audit` slice. Expanded deployment rows now also add deployment-scoped `_self` `Drift view`, `Plan/apply`, and `Audit log` handoffs beside `Open application`, so operators can jump straight from top-level triage into `/?page=drift&deployment={deploymentName}` for field-level troubleshooting, `/?page=plan&deployment={deploymentName}` for immutable preview/apply review, or `/?page=audit&deployment={deploymentName}` for immutable activity on the selected deployment. The landing page still includes an in-place `Refresh applications overview` control that re-runs only `GET /v1/overview/applications`, disables only while that overview fetch is active, keeps the current bookmarkable `q` / `sync` / `account` query state stable, and leaves the current application rows visible with explicit loading/error copy while the refresh runs or fails. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance as the deeper bookmarkable drill-downs instead of falling back to generic refresh-error text. When an active `q` search matches hidden runtime-only evidence (for example controller change details, conditions, rollout step text, or artifact values), the matching row adds a concise `Search match` context line so the operator can see why it matched without turning the landing page into a dense runtime view.

### Audit timeline
Use the audit page when you need immutable operator activity instead of controller-derived runtime history.

The page is backed directly by `GET /v1/audit-logs` and currently focuses on:
- newest-first `plan.created` and `apply.requested` entries
- a bookmarkable action filter that narrows the page and fetch route to exactly `plan.created` or `apply.requested`
- a bookmarkable limit control in the audit toolbar that updates the fetch route and URL in place while preserving deployment and action filters
- an explicit in-place `Refresh audit log` control that re-fetches only `GET /v1/audit-logs`, disables only while that audit fetch is active, and keeps the current bookmarkable `/?page=audit...` URL/query state stable
- an explicit in-page `Clear audit filters` control that appears only when deployment/action/limit filters are active and resets those audit filters back to the default unscoped `/?page=audit` route while preserving bookmarkable `q` / `sync` / `account` state
- active filter copy such as `Audit filters: action apply.requested · limit 2` when you reopen `/?page=audit&action=apply.requested&limit=2`
- deployment-scoped filter copy such as `Audit filters: deployment payments-api-prod · action apply.requested · limit 2` when you reopen `/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2`
- deployment / app / environment context
- deployment-scoped chrome/body copy plus a deployment-specific empty state when `deployment={deploymentName}` is present and no persisted audit entries match
- deployment-scoped quick navigation that keeps `Applications overview`, `Back to application`, the current non-click `Audit log` destination, sibling `Drift view` / `Plan/apply` workspace links, and a hash-aware `Current section` / `On this page` pair for `Deployment context` and `Audit timeline` fragments such as `/?page=audit&deployment=payments-api-prod&limit=2#audit-timeline`
- the current audit section becomes a non-click chip when that hash is active, while the remaining audit sections stay as `_self` fragment links and otherwise fall back to `No section is currently targeted.`
- initial `GET /v1/audit-logs` auth failures now render operator-specific guidance instead of generic fetch-error copy: `401` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that the current authenticated operator is not authorized for that route or deployment
- keeps the current audit content visible with explicit loading/error copy while that in-place refresh is running or fails for the same bookmarkable route
- explicit actor-unavailable or actor-unknown labeling instead of fabricated identity
- `_self` links back into `/?page=application&deployment={deploymentName}` as `Open application drill-down`, into `/?page=drift&deployment={deploymentName}` as `Open drift view`, and into `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}` as `Open plan/apply workspace` when those identifiers exist

### Account detail
Open `/?page=account&accountId={awsAccountId}` to get the denser account/group deployment slice without leaving the embedded shell. Its initial `GET /v1/overview/accounts/{awsAccountId}` load now uses the same operator-read auth handling as the other bookmarkable drill-downs: `401` tells the operator to reopen the same page through the trusted front door so identity can be forwarded, `403` explains that identity was recognized but this operator is not authorized for that route or deployment, and other statuses keep the existing unavailable/generic fetch-error behavior. The same account page now also renders compact `Quick navigation` chrome with an `_self` `Applications overview` return link plus a non-click `Account detail` chip so operators can stay oriented inside the bookmarkable account route without losing current `q`, `sync`, or `account` query state. That quick-nav now also adds a hash-aware non-click `Current section` chip plus an `On this page` group covering the `Account summary` and rendered project/environment groups, with a friendly fallback of `No section is currently targeted.` until the bookmarkable route includes a matching `#account-*` fragment such as `#account-summary`. When the same project/environment pair appears in multiple regions, those section links now keep the region in the label so the operator can tell the target groups apart before jumping. The same page now exposes an in-place `Refresh account detail` control that re-runs only `GET /v1/overview/accounts/{awsAccountId}`, disables only while that refresh is active, keeps the current bookmarkable `/?page=account...` route and hash stable, and leaves the stale account content visible with explicit loading/error copy while the refresh is running or if it fails. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance instead of falling back to generic error text.

### Application detail
Open one application to see the linked rollout resources grouped into clear sections such as:
- workloads
- traffic routing
- config and access

The top of that page now also includes a compact `Current application status` block so operators can answer “what is happening with this app right now?” before scanning resource cards. The summary stays intentionally small and read-only: it surfaces the current sync state, health, rollout context, artifact, linked-resource count, runtime-event availability, and last update using the existing application-detail payload instead of reintroducing the older dense dashboard.

Each linked resource row keeps the generic `_self` `Open resource` drill-down, and resource types that already support the final page’s existing evidence panels now also expose direct `_self` handoffs into those stable section anchors: `Open live inspection` jumps straight to `#resource-live-inspection` (including CodeDeploy application rows), and `Open recent logs` jumps straight to `#resource-recent-logs` only for resource types backed by the logs route. Unsupported resource types stay honest by omitting those shortcuts instead of advertising evidence surfaces the final page cannot currently honor.

On the initial `GET /v1/overview/applications/{deploymentName}` load, embedded auth failures are also status-aware: `401` tells the operator to reopen the same bookmarkable page through the trusted front door so identity can be forwarded, `403` explains that identity was recognized but this operator is not authorized for that route/deployment, and other statuses keep the existing unavailable/generic fetch-error behavior. After the page is open, the same drill-down now exposes an in-place `Refresh application detail` control that re-runs only `GET /v1/overview/applications/{deploymentName}`, disables only while that refresh is active, keeps the bookmarkable `/?page=application...` route stable, and leaves the current compact status summary, grouped-resource content, and controller-runtime-events content visible with explicit loading/error copy. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance instead of falling back to generic refresh-error text.

### Resource detail
Open a concrete resource to inspect the deeper operational context for that resource, including rollout and runtime information. The generic read-only route families remain `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection`. Supported-but-unavailable reads keep the route usable with `available=false` plus explicit `unavailableReason` payload contract instead of vague server errors. The embedded resource page now includes a top-level `Refresh resource detail` action that re-runs `GET /v1/overview/deployments/{deploymentName}` in place, preserves the bookmarkable `deployment` / `resourceType` / `resourceId` URL, re-resolves the selected resource from the refreshed payload, and keeps the last successful resource detail visible while refresh-time loading/error guidance renders above it. It also includes a compact `Quick navigation` strip with a non-click `Current resource` chip, a hash-aware non-click `Current section` chip when the bookmarkable route targets one of the rendered resource sections, and an `On this page` group for the remaining stable section hashes such as `#resource-runtime-history`, `#resource-live-inspection`, and `#resource-recent-logs`. The existing `_self` jumps back to the application, drift, plan/apply, audit, and sibling resources remain unchanged. The resource page also includes a `Live inspection` panel that lazy-loads the resource-inspection route for the selected resource while keeping bookmarkable resource URLs unchanged. The panel renders explicit loading, unavailable, empty, and supported states, reuses the same safe DOM construction patterns as the rest of the embedded UI, and keeps lookup context visible when Tenet cannot return a live payload. Operators now also get an in-place `Refresh live inspection` control on that panel so they can re-run only that panel-local backend route in place; the control disables only while its own fetch is active, preserves the bookmarkable URL, and leaves the sibling panel and the rest of the resource page visible. When that lazy read returns `401` or `403`, the panel keeps the same in-place refresh behavior but replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths. EC2 `userDataBootstrapSource` resources now also render a dedicated live-inspection payload there, reusing the same launch-template bootstrap-source evidence Tenet already uses for controller-managed drift so operators can compare the normalized desired bootstrap source against the observed launch-template marker without leaving the existing resource drill-down route. When that bootstrap-source payloa... [truncated]

Recent logs are still surfaced here when the resource type supports them, and the recent-log card keeps the backend lookup path explicit with operator-facing labels for region, control account, target account, cross-account status, and observer-role configuration even when no lines are returned or the lookup is unavailable. That card now also includes its own `Refresh recent logs` control so operators can re-run only that panel-local backend route in place with the same per-panel loading disablement while leaving the sibling panel and the rest of the resource page visible, and it uses the same trusted-front-door / access-denied `401` / `403` guidance instead of generic fetch-error text when the guarded lazy read is denied. `userDataBootstrapSource` now participates in that same recent-log path by reusing the backing Auto Scaling Group Tenet already resolves for bootstrap-source inspection; CodeDeploy listener and task-set target-group sibling resources likewise reuse the backing deployment-group history. When that happens, the payload stays read-only and additive by preserving the requested resource identity while also surfacing optional `backingResourceType` and `backingResourceName` fields for the resolved backing resource. The embedded `Recent logs` card renders those additive fields directly as `Backing resource type` and `Backing resource name` when they are present, and when the loaded deployment detail can safely resolve that backing resource it also renders a `_self` `Resolved backing resource` handoff to the backing resource drill-down. Direct log sources stay quiet and do not show placeholder backing-resource labels. When no backing group or AWS access is available, the route preserves the same explicit unavailable payload states instead of failing the page. Shared resource deep links keep the current query state, update the `Current section` chip after in-page hash changes, and reapply hash scrolling after the lazy `Recent logs` or `Live inspection` panel finishes repainting, so reopening `/?page=resource...#resource-recent-logs` or `#resource-live-inspection` lands back on the intended section. A parallel read-only resource inspection route now exposes explicit typed backend payloads for supported live AWS resources without ch... [truncated]

The compose-backed smoke now covers that shipped bootstrap-source drill-down directly: it fetches the bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` route plus the matching canonical backend reads `GET /v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh` and `GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh`, and treats explicit unavailable reasons such as `aws_unavailable` and `no_log_source` as part of the expected local no-AWS contract instead of assuming live AWS data.

Canonical resource-log routes stay stable across rollout modes so operators can bookmark or script them:
- `asgWeightedTargetGroups`: `trafficShiftPlan` (`{deployment}-weighted-routing`)
- `ecsRolling`: `ecsService` (`{deployment}-service`)
- `ecsCodeDeploy`: `codeDeployApplication` (`{deployment}-codedeploy-app`) for application-wide CodeDeploy deployment history, `codeDeployDeploymentGroup` (`{deployment}-codedeploy-group`) for deployment-group-scoped history, plus the sibling `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` routes that reuse that same deployment-group history through additive backing-resource metadata

Behind those routes, the built-in AWS-backed recent-log provider now reuses SDK config/client setup per `(region, target account, observer role)` key inside a running Tenet API process instead of rebuilding ECS / CloudWatch Logs / Auto Scaling / EC2 clients on every fetch, and it reuses that same AWS config for signed CodeDeploy deployment-history lookups. For `ecsService`, the provider inspects both newest running tasks and recently stopped service tasks, then returns the same bounded newest-first `entries[]` envelope so failed rollouts and crash loops can still expose container logs even when no task is currently running.

Current provider-backed recent-log support includes:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource` via the resolved backing Auto Scaling Group
- `codeDeployDeploymentGroup` via newest-first CodeDeploy deployment history for the selected deployment group
- `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` via the backing CodeDeploy deployment-group history for the same rollout

The same drill-down now has a parallel inspection route family:
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`

Supported always-live inspection currently starts with:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`

Supported traffic-routing inspection currently covers:
- `codeDeployApplication`
- `codeDeployDeploymentGroup`
- `productionListener`
- `testListener`
- `productionListenerRule`
- `activeTargetGroup`
- `candidateTargetGroup`
- `blueTaskSetTargetGroup`
- `greenTaskSetTargetGroup`
- `trafficShiftPlan`

Within one running Tenet API process, repeated successful live inspections for `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, and `testListener` now reuse a bounded in-process cache keyed by `(region, target account, observer role, resource type, resource name)`. Summary-only fallbacks and unavailable outcomes are still recomputed or returned through the existing non-cached path.

For `ecsService`, Tenet now keeps a bounded in-process hint of the last cluster that resolved a given `(region, target account, observer role, service name)` lookup so repeated drill-downs avoid brute-force cluster scans. If that hint is stale or the service moved, the lookup falls back to the full cluster scan path, refreshes the hint, and keeps the existing resource-not-found semantics unchanged.

The embedded `Live inspection` panel renders operator-friendly summaries from those payloads:
- `ecsService`: service identity/status, desired-running-pending counts, additive deployment-configuration fields (`minimumHealthyPercent`, `maximumPercent`, `healthCheckGracePeriodSeconds`) when ECS returns them, active deployments, and recent ECS service events
- `autoScalingGroup`: group sizing/health settings, attached target groups, current instances, and additive launch-template details including `launchTemplate.imageId` and `launchTemplate.instanceType` when the live EC2 launch-template lookup resolves them
- `userDataBootstrapSource`: normalized desired bootstrap source, observed launch-template bootstrap-source marker, the backing Auto Scaling resource Tenet inspected, the launch template version Tenet used for that observation, and an additive in-app handoff to the resolved backing resource drill-down when that resource can be matched safely from the loaded deployment metadata
- `trafficRoutingResource`: summary-only or live traffic-routing context with explicit resource kind, display name, inspection mode, rollout summary text, additive live fields for CodeDeploy applications/deployment groups/listeners when AWS access is available, and target-group live details for `blueTaskSetTargetGroup` / `greenTaskSetTargetGroup` payloads that include protocol/port, health-check settings, load-balancer ARNs, and registered target-health counts

Resource-inspection responses keep the contract explicit for UI consumers:
- common fields: `deploymentName`, `resourceType`, `resourceName`, `lookupContext`, `available`, `source`, `unavailableReason`
- ECS payloads add `ecsService`, including additive deployment-configuration fields `minimumHealthyPercent`, `maximumPercent`, and `healthCheckGracePeriodSeconds` when `DescribeServices` returns them
- Auto Scaling payloads add `autoScalingGroup`, including optional `launchTemplate` and `mixedInstancesPolicy` metadata when `DescribeAutoScalingGroups` returns launch-template details plus additive observed launch-template runtime fields such as `launchTemplate.imageId` and `launchTemplate.instanceType` when the live EC2 launch-template lookup resolves them
- EC2 bootstrap-source payloads add `userDataBootstrapSource`, including `desiredBootstrapSource`, optional `observedBootstrapSource`, the backing Auto Scaling resource identity, and the observed launch template summary Tenet used for that comparison, including additive `launchTemplate.imageId` and `launchTemplate.instanceType` when live EC2 launch-template data is available
- traffic-routing payloads add `trafficRoutingResource`; when live CodeDeploy task-set target-group metadata is present, the embedded UI reads it from `trafficRoutingResource.targetGroup` (or compatible `targetGroupInspection` / `loadBalancerTargetGroup` shapes) and renders the target group name/ARN, protocol/port, target type, health-check settings, load-balancer ARNs, target-health counts, and registered target rows without requiring operators to open raw JSON

Traffic-routing inspection stays local-friendly without changing the route shape:
- `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` remain `summary_only`; `blueTaskSetTargetGroup` and `greenTaskSetTargetGroup` still render summary-only local/no-AWS payloads without empty live-field placeholders, and upgrade in place when the backend returns `inspectionMode=live` plus target-group metadata
- `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, and `testListener` stay `summary_only` when AWS inspection is disabled, and switch to `inspectionMode=live` with additive `codeDeployApplication`, `codeDeployDeploymentGroup`, or `listener` details when live AWS access succeeds
- AWS-side traffic-routing failures still return `available=false` with an explicit `unavailableReason` and `lookupContext`

Like recent logs, `lookupContext` is present for both `available=true` and unavailable inspection responses:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

Unsupported or temporarily unavailable resource types do not surface vague 500s from the provider layer. They return `available=false` with an explicit `unavailableReason` plus `lookupContext` so operators can see which region/account path Tenet attempted before continuing AWS troubleshooting elsewhere.

The same drill-down also includes read-only workspace surfaces:
- `/?page=drift&deployment={deploymentName}` for field-level troubleshooting context that now calls out the active drift-row source: controller-authored `managedDrift[]` rows when present, otherwise the existing derived fallback read-model heuristics for older payloads. Controller-authored rows can now also include live-backed EC2 Auto Scaling Group capacity evidence under stable field names such as `runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max` for `asgWeightedTargetGroups` and `asgInstanceRefresh` deployments when live AWS inspection succeeds. Those same EC2 inspections can now also add `runtimeContext.launchTemplate.bootstrapSource`, `runtimeContext.launchTemplate.imageId`, and `runtimeContext.launchTemplate.instanceType` by comparing the normalized desired deployment bootstrap source plus first-stack-region `ami_id` / `instance_type` intent against the observed primary launch-template values without adding a separate route family. ECS `ecsCodeDeploy` and `ecsRolling` deployments can also add live-backed ECS service rows under `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, and `runtimeContext.taskDefinition.image` when read-only ECS inspection resolves the service desired count, deployment configuration, and a single task-definition container image. Older payloads, local runs, or unavailable/missing AWS access still fall back cleanly without fabricating controller drift rows. Focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows. Empty-state copy also distinguishes "no controller-authored drift reported" from "no derived fallback drift detected." When ... [truncated]
- the drift workspace also includes a compact summary above the field-level table with total rows, visible rows after the current filters, and live `high` / `medium` / `low` counts so operators can see severity distribution and filter impact at a glance
- the drift workspace exposes an explicit `Clear drift filters` control that resets the drift query, severity, and sort controls back to their defaults in one step without changing the bookmarkable route or re-fetching deployment detail
- the drift workspace quick-navigation strip now also exposes a hash-aware `Current section` group plus an `On this page` group with `_self` fragment links for `Runtime signals`, `Rollout execution`, `Field-level drift`, and `Status conditions`, preserving the current bookmarkable pathname/query while appending drift section hashes such as `/?page=drift&deployment={deploymentName}#drift-field-level`
- when one of those drift section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- the drift workspace quick-navigation strip now also exposes direct `_self` resource drill-down links built from the same grouped normalized resource inventory used by the application/resource pages, and renders explicit `No linked resources are available in this drift workspace yet.` copy when that inventory is empty
- `/?page=application&deployment={deploymentName}`, `/?page=resource&deployment={deploymentName}&resourceType={resourceType}&resourceId={resourceId}`, `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render explicit breadcrumb chrome that links prior drill-down levels with `_self` anchors, includes the account step when `awsAccountId` is already known from the loaded detail payload, and keeps the current destination as a non-click chip instead of a self-link
- the bookmarkable `/?page=resource&deployment={name}...` and `/?page=drift&deployment={name}` entry loads both depend on `GET /v1/overview/deployments/{deploymentName}` and now distinguish auth failures on that initial read: `401` tells the operator to retry the same bookmarkable page through the trusted front door so authenticated identity can be provided, while `403` explains that identity was recognized but the current operator is not authorized for that route or deployment
- `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render the same embedded quick-navigation strip with a `Hierarchy` group for `_self` `Applications overview` and `Back to application`, a `Current destination` group that keeps the active workspace as a non-click chip, and a `Deployment workspaces` group for the remaining sibling workspaces so operators can move laterally between read-only workspaces without relying on browser back or leaving the bookmarkable shell
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

The embedded `/?page=application&deployment={deploymentName}` drill-down now renders a small `Controller runtime events` timeline from that route with explicit loading, empty, unavailable, and non-auth fetch-error states. This route returns controller-derived runtime events copied from persisted `status.history` plus an additive `managedDriftSummary` with explicit `total`, `high`, `medium`, and `low` counts across those returned history rows (zeroes are present when `events` is empty); the application page renders those counts as `Managed drift summary · {total} total · {high} high · {medium} medium · {low} low` above the event list so operators can triage drift severity before opening individual event rows, and shows an in-shell `Open drift view` handoff whenever those counts report managed drift so operators can jump directly into the field-level drift workspace. Example payload fragment: `{"deploymentName":"payments-api-prod","events":[{"summary":"Deployment has managed drift","managedDrift":[{"field":"runtimeContext.capacity.desired","severity":"high"}]}],"managedDriftSummary":{"total":1,"high":1,"medium":0,"low":0}}`. It is intentionally operator-facing and refresh-based, not a full durable audit system yet, and the UI labels it separately from any future audit/auth history work. The same application page now also exposes a compact `Quick navigation` strip with a hash-aware `Current section` group plus an `On this page` group for only the currently rendered non-empty linked-resource sections and `Controller runtime events`, so operators can reopen anchors such as `/?page=application&deployment={deploymentName}#application-runtime-events` without changing the fetch contract or leaving the embedded shell. Empty zero-resource groups are suppressed on the page and in quick navigation; if an application truly has no linked resources yet, the page shows the explicit operator-facing empty state `No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.` while still keeping the runtime-events section available. When one of those section hashes is active, Tenet renders it as a non-click current-section chip, removes it from the remaining `On this page` links, and otherwise shows the friendly empty state `No section is currently targeted.`. The section header now includes an in-place `Refresh runtime events` control that re-fetches just `GET /v1/overview/deployments/{deploymentName}/events` in place, keeps the bookmarkable application route stable, and disables only while that events fetch is active. When that refresh returns `401` or `403`, the panel now shows the same trusted-front-door / access-denied guidance used on the other embedded read surfaces instead of generic fetch-error text. When the aligned runtime change label is `managedDrift`, the paired `changeDetails[]` text now stays single-slot but explains deterministic added, removed, and updated drift rows so operators can see what changed without diffing the whole `managedDrift[]` array by hand. When a deployment exists but no controller-authored runtime history has been persisted, the route still returns `200 OK` with `events: []`, so the UI can keep the page stable and explicit during early runtime bootstrap.

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

`lookupContext` is present for both `available=true` and unavailable responses, so operators can still confirm the attempted lookup region/account path when `entries` is empty or `unavailableReason` is set. When a bootstrap-source request resolved through a backing Auto Scaling Group before the downstream lookup ran, `backingResourceType` and `backingResourceName` remain present on both success and explicit unavailable responses. The embedded UI renders the lookup-context fields directly as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured` inside the `Recent logs` card, and when those additive backing-resource fields are present it also renders them there as `Backing resource type` and `Backing resource name`.

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
