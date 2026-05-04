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

This keeps the first page simple and pushes deeper operational detail into drill-down pages where it is easier to understand. When the applications list is empty, the UI now distinguishes between filters hiding known rows and `GET /v1/overview/applications` returning no visible applications at all, so operators know whether to clear filters or check catalog/status/auth inputs. The embedded application/resource/drift/plan/audit pages now also keep explicit breadcrumb chrome in the same shell so prior drill-down levels stay bookmarkable `_self` links, the current destination stays visible as a non-click chip, and the account step appears when Tenet already knows the deployment's `awsAccountId`. The application page quick-nav now makes the overview-to-detail hierarchy explicit with an `_self` `Applications overview` return link plus a non-click `Application detail` chip, while the bookmarkable account page now mirrors that same orientation pattern with an `_self` `Applications overview` return path plus a non-click `Account detail` chip. The deployment workspaces now use that same hierarchy pattern too: their shared quick-nav exposes `_self` `Applications overview` and `Back to application` return paths, keeps the active drift/plan/audit destination as a non-click chip, and still preserves compact bookmarkable `On this page` fragment links plus the existing lateral `_self` jumps to sibling workspaces or resources without backing out first. Browser tab titles follow the active route as safe text, so multiple open Tenet tabs identify the current application, account, resource, drift view, plan/apply workspace, or audit feed without changing the bookmarkable URL.

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

Tenet now exposes the first concrete GitHub webhook ingress path at `POST /api/v1/github/webhook`. Configure `TENET_GITHUB_WEBHOOK_SECRET` to enable that route; the local Compose stack wires a safe default `smoke-webhook-secret` so `make smoke-docker` can prove the signed contract end to end without extra setup. The handler remains conservative: it requires `X-GitHub-Delivery`, `X-GitHub-Event`, and `X-Hub-Signature-256`, validates the HMAC-SHA256 signature, checks the JSON body shape, and returns an operator-friendly summary of a signed push payload. Signed non-`push` deliveries and deleted-ref push payloads intentionally stay `status=ignored` so operators can distinguish an authenticated no-op from signature or payload failures; deleted-ref summaries include `deleted=true` and skip controller refresh because there is no new desired-state revision to load. Push summaries include `repository`, `ref`, `before`, `after`, `headCommit`, additive `changedFiles` lists, and an additive `desiredStateSourceMatch` object. That object keeps stable UI/docs-facing fields such as `configured`, `kind`, `matches`, `matchStatus`, `reason`, `repository`, `ref`, and `path` so operators can distinguish `not_configured`, `filesystem_source`, `repository_mismatch`, `ref_mismatch`, `path_untouched`, and `matched`. GitHub matching compares repository, normalizes branch or tag refs such as `main` vs `refs/heads/main` and `v1.2.3` vs `refs/tags/v1.2.3`, and treats `TENET_CATALOG_GITHUB_PATH` as a repo-relative prefix against changed files; when `TENET_CATALOG_GITHUB_REF` is unset, the webhook summary compares the push ref against `repository.default_branch` from the signed push payload because the active source is following the repo default branch. In filesystem mode or any non-Redis API runtime, a validated push still returns `202 Accepted` with `status=accepted`, `processed=false`, and an explicit unavailable reason. When the active desired-state source is GitHub, the signed push matches that source, and the API is running with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis-backed controller refresh handoff and returns `processed=true` plus an additive `controllerRefresh` object with `backend`, `channel`, `requestId`, and `publishedAt` so operators can see that the handoff was actually publish... [truncated]

For the current read-only plan/apply flow, a typical local operator path is:
- `GET /v1/overview/applications/{deploymentName}` or `GET /v1/overview/deployments/{deploymentName}` to inspect the deployment and capture its additive `sourceFingerprint`
- open `http://localhost:8080/?page=audit` at any point for the bookmarkable audit page backed by `GET /v1/audit-logs`; application detail plus deployment-scoped drift/plan workspaces now link into `/?page=audit&deployment={deploymentName}` so the audit chrome/body stays explicitly scoped to the active deployment, and the page renders filter-aware empty-state copy when the selected deployment/action filters or a limit cap match no persisted entries. When a deployment is selected, the same in-shell breadcrumb chrome plus quick-navigation strip used by the drift/plan workspaces appears in the audit body so operators can jump with `_self` links to the applications overview, application detail, drift view, or plan/apply workspace while the current deployment-scoped audit route stays visible as a non-click active chip without dropping bookmarkable `q` / `sync` / `account` / `deployment` / `action` / `limit` state. Add `&action=plan.created|apply.requested` to narrow the embedded timeline to one action, and keep using `&limit={n}` when you want a smaller newest-first window from the API. The audit toolbar now exposes the same limit as an explicit safe preset selector while still honoring bookmarked positive integers such as `?limit=2`; `Refresh audit log` re-fetches only `GET /v1/audit-logs` in place while keeping the current bookmarkable `/?page=audit...` URL/query state stable; and `Clear audit filters` / `Clear audit controls` appears when deployment/action filters or the limit cap are active so operators can recover to the unfiltered `/?page=audit` slice without dropping bookmarkable `q` / `sync` / `account` state.
- `GET /v1/audit-logs?deployment={deploymentName}` when you want only one deployment's immutable plan/apply entries without changing the route shape
- `GET /v1/audit-logs?action=plan.created` when you want only immutable plan-preview creation records
- `GET /v1/audit-logs?action=apply.requested` when you want only immutable apply-request records
- `GET /v1/audit-logs?action={action}` when you want only one immutable action type; `action` must be exactly `plan.created` or `apply.requested`
- `POST /v1/plans/deployments/{deploymentName}` with optional `{"ifMatch":"<sourceFingerprint>"}` to persist an immutable preview for the reviewed state; when a local proxy or test client has authenticated operator context, forward it explicitly with additive `X-Tenet-Actor-Id`, `X-Tenet-Actor-Name`, and/or `X-Tenet-Actor-Email` headers, or the issue-10-compatible aliases `X-User-ID`, `X-User-Name`, and/or `X-User-Email`; when both forms are present, Tenet prefers the explicit `X-Tenet-Actor-*` values without changing the route shape
- open `http://localhost:8080/?page=plan&deployment=payments-api-prod&planId={planId}` for the bookmarkable plan workspace; `Apply Plan` activates only after that persisted ready preview is loaded with a valid `planId`, the embedded `Refresh plan workspace` control re-fetches the mutable deployment-derived context from `GET /v1/overview/deployments/{deploymentName}` without changing the bookmarkable `planId` / `applyId` URL, and the embedded `Refresh plan preview` / `Refresh apply request` controls re-fetch stored records in place without forcing a browser reload. Initial bookmarked `GET /v1/plans/{planId}` loads and later in-place refreshes now replace generic fetch-error copy for `401` / `403` with operator-facing retry/access guidance while keeping deployment-derived fallback context visible in the same workspace
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


Front-door auth remains additive and opt-in. Tenet still supports the existing trusted-header mode for installations that authenticate operators before requests reach the API. The provider-neutral enterprise SSO boundary is documented in `docs/architecture/tenet-v1-enterprise-sso.md`; that boundary keeps GitHub OAuth, Google Workspace OIDC, and Okta OIDC as active managed-login providers only when explicitly configured, keeps Okta SAML diagnostic-only, keeps Google Workspace/Gmail identity-only, uses local fake IdP tests instead of live credentials, and leaves repository-backed RBAC policy evaluation to follow-up issue #663. The API can now also run explicitly configured Tenet-managed GitHub OAuth, Google Workspace OIDC, or Okta OIDC browser login: `GET /auth/login/github`, `GET /auth/login/google`, or `GET /auth/login/okta` creates one-time callback state, redirects to the configured provider, the matching callback route exchanges the code, persists the normalized identity, and issues a signed `tenet_session` cookie (`HttpOnly`, `SameSite=Lax`) that later requests can project into the authenticated request actor. When that runtime is active, `GET /auth/logout` and `POST /auth/logout` clear only the Tenet-managed `tenet_session` cookie with the same cookie attributes and redirect to a validated in-app `returnUrl` (default `/`). Clearing this cookie does not sign the operator out of GitHub, does not clear an upstream front-door session, and does not change trusted front-door header precedence. The callback marks that cookie `Secure` whenever the request is already HTTPS, when a trusted front door forwards `X-Forwarded-Proto: https` / `Forwarded: proto=https`, or when operators explicitly enable the `TENET_AUTH_SESSION_COOKIE_SECURE` override; local HTTP development keeps the cookie non-`Secure` so `make smoke-local`-style workflows remain usable. `GET /v1/operator/context.authRuntime.tenetManagedLogin` reports the same non-secret cookie-security evidence through `sessionCookieSecureOverride`, `sessionCookieSecureForRequest`, and `sessionCookieSecureSources` so operators can confirm whether the current diagnostic request arrived through a secure-cookie path without inspecting `Set-Cookie` manually. The internal `authprovider` package provides GitHub OAuth, Google Workspace OIDC, and Okta OIDC authorization-code primitives for managed-login integration work, the internal `authidentity` package provides a small JSON-backed identity-store contract for normalized provider identities, and the internal `authstate` package provides a JSON-backed OAuth callback state store with expiry and one-time consume semantics for future CSRF/state validation. When the runtime variables are complete for GitHub (`TENET_AUTH_PROVIDER=github`, `TENET_GITHUB_OAUTH_CLIENT_ID`, `TENET_GITHUB_OAUTH_CLIENT_SECRET`, `TENET_GITHUB_OAUTH_REDIRECT_URL`, shared state/identity stores, and `TENET_AUTH_SESSION_SECRET`), `tenet-api` wires the GitHub provider plus JSON-backed state/identity stores and reports `authRuntime.tenetManagedLogin.active=true` from `GET /v1/operator/context`. When the runtime variables are complete for Google (`TENET_AUTH_PROVIDER=google`, the `TENET_GOOGLE_OIDC_*` client/issuer/redirect settings, shared state/identity stores, and `TENET_AUTH_SESSION_SECRET`), `tenet-api` wires the Google provider plus the same JSON-backed state/identity stores and reports the active enterprise SSO diagnostic without storing or returning provider access tokens. The runtime stays disabled when any setting is absent, keeps non-secret `requiredSettings` / `missingSettings` diagnostics, and never logs or returns configured secrets. The shipped managed-login runtime supports GitHub OAuth, Google Workspace OIDC, and Okta OIDC browser login today; Okta SAML provider selection remains represented as inactive non-secret diagnostics under `authRuntime.enterpriseSso`, while provider-token refresh/revocation, upstream provider/front-door logout, and long-lived token lifecycle management remain future work. The legacy `/api/v1/auth/*` route family remains unregistered. The same diagnostic returns non-secret `requiredSettings` and `missingSettings` arrays so operators can tell whether managed login is disabled because configuration is absent, partial, or names an unsupported provider without exposing secret values.


`GET /v1/operator/context.authRuntime.enterpriseSso` is the provider-neutral diagnostic surface for the first enterprise SSO runtime slice. For `TENET_AUTH_PROVIDER=google_oidc` (or the `google` alias), the managed-login runtime is active when the Google client, redirect, issuer, state-store, identity-store, and session-secret settings are complete. For `TENET_AUTH_PROVIDER=okta_oidc`, the same managed-login runtime is active when the Okta issuer, client, redirect, state-store, identity-store, and session-secret settings are complete. Okta SAML remains diagnostic-only and inactive; complete Okta SAML preflight now also validates that metadata/ACS values are absolute HTTP(S) URLs and that `TENET_OKTA_SAML_ACS_URL` points at `/auth/callback/okta/saml` before reporting `configured=true`. The diagnostic reports the canonical provider, sign-in/callback/logout routes, `requiredSettings`, `missingSettings`, `invalidSettings`, shared state/identity/session-secret booleans, and `active` state without returning secrets, raw tokens, SAML assertions, authorization codes, or private keys. Google Workspace/Gmail means identity login only; it does not grant Tenet Gmail mailbox access. Example configured Google diagnostic:

```json
{
  "authRuntime": {
    "enterpriseSso": {
      "configured": true,
      "active": true,
      "provider": "google",
      "requiredSettings": ["TENET_AUTH_PROVIDER", "TENET_GOOGLE_OIDC_CLIENT_ID", "TENET_GOOGLE_OIDC_CLIENT_SECRET", "TENET_GOOGLE_OIDC_ISSUER_URL", "TENET_GOOGLE_OIDC_REDIRECT_URL", "TENET_AUTH_STATE_STORE_PATH", "TENET_AUTH_IDENTITY_STORE_PATH", "TENET_AUTH_SESSION_SECRET"],
      "missingSettings": [],
      "loginRoute": "/auth/login/google",
      "callbackRoute": "/auth/callback/google",
      "logoutRoute": "/auth/logout",
      "stateStoreConfigured": true,
      "identityStoreConfigured": true,
      "sessionSecretConfigured": true
    }
  }
}
```

`make smoke-local` and `make smoke-docker` restart the API with fake local Google OIDC, Okta OIDC, and Okta SAML settings to prove Google and Okta OIDC report active managed-login/enterprise-SSO runtimes while Okta SAML remains configured-but-inactive, non-secret, credential-free, and explicit about route placeholders plus required/missing/invalid settings. The smoke guard now also exercises `GET /auth/logout` and `POST /auth/logout` on the configured Google path to prove logout redirects only to safe in-app `returnUrl` values, clears the Tenet `tenet_session` cookie, and does not claim provider-token/front-door-session revocation. That smoke coverage does not contact Google or Okta; focused Go tests cover the Google and Okta OIDC fake-provider login/callback paths, missing-setting combinations, and router JSON field names.

`GET /v1/operator/context.authRuntime.tenetManagedLogin` also exposes lifecycle boundaries without returning secrets: `oauthStateTtlSeconds` is the one-time callback-state lifetime, `sessionTtlSeconds` is the signed session-cookie lifetime, `sessionExpiryEnforced=true` confirms expired signed cookies stop projecting an authenticated identity, `sessionCookiePath=/`, `sessionCookieHttpOnly=true`, and `sessionCookieSameSite=Lax` mirror the cookie attributes used by login and logout responses, `providerAccessTokenStored=false` and `providerAccessTokenReturned=false` confirm provider access tokens are used only for any callback/profile lookup and never returned to operators, and `providerTokenRefreshSupported=false` / `providerTokenRevocationSupported=false` make clear that Tenet does not refresh or revoke upstream provider tokens. The embedded Operator context banner translates provider identifiers such as `github`, `google_oidc`, `okta_saml`, and `okta_oidc` into human-readable labels and keeps provider boundaries explicit: Google Workspace OIDC is identity-claim-only and does not grant Gmail mailbox access, while Okta OIDC maps profile claims into Tenet session identity and Okta SAML remains diagnostic-only until a SAML runtime lands; both leave authorization to trusted roles plus deployment grants. `logoutClearsSessionCookie=true` only when the local logout route is active, while `logoutRevokesProviderToken=false` reminds operators to handle upstream provider/front-door sign-out separately.

Reviewer verification for managed-login work should include `go test ./internal/api ./cmd/tenet-api` for route/session/env wiring and `go test ./internal/authprovider ./internal/authidentity ./internal/authstate` for provider, normalized identity persistence, and callback-state helpers. Local and Docker smoke still cover the default disabled runtime unless those OAuth environment variables are explicitly supplied. The release guard is deliberately split: smoke proves a default API does not advertise active managed login and still reports the non-secret lifecycle/token/logout boundary fields (`oauthStateTtlSeconds`, `sessionTtlSeconds`, `sessionCookiePath`, `sessionCookieHttpOnly`, `sessionCookieSameSite`, provider-token stored/returned/refresh/revocation false flags, and inactive Tenet-local logout), while focused `internal/api` router/session tests and the Node-backed embedded UI harness use local fakes to prove the configured GitHub, Google, and Okta OIDC login/callback/session/logout paths, the expanded Operator context sign-out handoff, and same-shell managed-login handoffs without live GitHub, Google, or Okta credentials. The same harness also proves provider-neutral active enterprise SSO handoffs such as `Sign in with Google Workspace SSO` and `Sign out of Google Workspace SSO session` without live provider credentials, including the Google Workspace case where the API reports both `tenetManagedLogin.active=true` and `enterpriseSso.active=true`; the embedded recovery copy deliberately prefers the enterprise SSO label so operators do not mistake Google Workspace SSO for the GitHub managed-login lane, and the embedded sign-out copy uses that same provider label when an active enterprise `logoutRoute` is present. The configured fake path also locks that `GET /auth/logout` links use `/auth/logout?returnUrl=...`, stay in the same shell, and explicitly say they clear only the Tenet-managed session cookie; the enterprise sign-out coverage additionally proves the current bookmarkable route is preserved and upstream provider/front-door logout or provider-token revocation are not claimed.

Use these settings only when every request reaches Tenet through a front door that authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values. Do not expose a header-trusting `tenet-api` instance directly to untrusted clients.

Tenet currently has two intentionally separate identity header families:

| Header family | Trusted source | Current use |
| --- | --- | --- |
| `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments` | Authenticating front door only | Request-scoped route authorization, `GET /v1/operator/context` diagnostics, and optional projection into persisted plan/apply/audit actor fields when no explicit actor headers are present. |
| `X-Tenet-Actor-*` plus compatible `X-User-*` aliases | Local proxy, smoke, or trusted integration creating immutable records | Persisted actor metadata on plan previews, apply requests, and audit projections; this family does not authorize routes by itself. |

Keep that split intact when integrating an external front door: the provider should authenticate the user, strip spoofable inbound trust headers including `X-User-*` aliases, and then forward only the trusted authorization headers Tenet is configured to consume. Tenet-managed GitHub, Google Workspace OIDC, and Okta OIDC sessions are signed cookies minted only by their configured callback routes; `/auth/logout` only expires that Tenet-managed cookie and redirects back to a safe in-app URL. Tenet does not treat raw browser-supplied `X-User-*` headers as a login session, and clearing the Tenet cookie does not remove trusted front-door identity headers from a request.

- `TENET_API_TRUST_AUTHENTICATED_HEADERS` trusts front-door-forwarded `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, and `X-Tenet-Authenticated-Email`. When those explicit trusted fields are absent, the same trusted authenticated-identity path also accepts additive `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases. `X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for that trusted authenticated identity path.
- `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY` requires authenticated identity on `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions`. Unauthenticated requests fail with `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`. When explicit actor headers are absent, the trusted authenticated identity path projects that identity into persisted plan/apply/audit actor fields.
- In the embedded plan/apply workspace, the same operator guidance stays explicit: authenticated identity is required on operator write routes. When Tenet-managed login is inactive, unauthenticated writes tell operators to retry the same bookmarkable plan/apply action through the trusted front door instead of sending those writes directly to Tenet; active enterprise SSO can only replace that recovery path when the enterprise diagnostic also reports an active same-origin `loginRoute`. When the optional managed-login or enterprise SSO diagnostic reports an active same-origin login route, guarded read recovery can offer the configured Tenet-managed GitHub or provider-neutral enterprise SSO sign-in handoff while preserving the current `returnUrl`. For the GitHub managed-login runtime specifically, guarded read recovery can offer the configured Tenet-managed GitHub sign-in handoff while preserving the current `returnUrl`. If a write reaches Tenet but returns deployment-access `403`, the failure card tells operators to request either an exact `X-Tenet-Authorized-Deployments` grant for that deployment or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes that prove the catalog placement for the deployment being written.
- `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` trusts `X-Tenet-Authenticated-Roles`, normalizes comma-separated roles, and requires one operator-capable `operator` / `admin` role on operator write routes. Role forwarding intentionally has no `X-User-*` compatibility alias; `roles.provenance.source=absent` means the front door must forward `X-Tenet-Authenticated-Roles` before role-gated reads or writes can pass. Authenticated callers without one of those roles get `403 Forbidden` plus `forbidden: authenticated operator role is required for this operator write route` before any plan/apply lookup.
- `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` trusts `X-Tenet-Authorized-Deployments` as a comma-separated exact deployment allowlist for existing exact grants. It also accepts additive `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes for guarded deployment reads and immutable writes; Tenet only widens access when it can prove the deployment's catalog placement matches both a forwarded project and environment. Exact deployment grants continue to win for deployment-specific access. Guarded collection reads (`GET /v1/overview/applications`, raw catalog collections such as `GET /v1/catalog/deployments`, `GET /v1/status/deployments`, `GET /v1/overview/accounts`, and unscoped `GET /v1/audit-logs`) filter to the union of exact deployment grants and project/environment catalog matches. Missing, stale, or non-matching grants return `200 OK` with `[]` or `{"entries":[]}` on collection routes, while scoped reads return the existing operator-read `403` unless exact deployment membership or project/environment placement proves access.
- When both `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` are enabled, guarded read routes additionally require one trusted forwarded read-capable `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin`. Authenticated callers without one now fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route` before Tenet evaluates deployment visibility.
- For local filesystem desired-state repositories, Tenet now also looks for optional repository RBAC policy diagnostics at `rbac/policy.json`, `rbac/policy.yaml`, or `rbac/policy.yml`. Missing policy files do not block startup or change route enforcement. When present, `GET /v1/operator/context` evaluates the current trusted identity and `X-Tenet-Authenticated-Groups` group claims against that policy and returns a diagnostic-only `repositoryPolicy` object showing matched user/group grants, derived roles, deployment grants, project/environment scopes, and whether those derived grants would read or write. This is groundwork for repository-backed RBAC; route authorization still uses the trusted front-door role/deployment headers above until a later slice explicitly opts into policy-backed enforcement.
- `GET /v1/operator/context` is a read-only diagnostic endpoint for trusted front-door troubleshooting. It reports the normalized trusted identity, additive `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases` lists for the accepted trusted identity header families, additive `identityProvenance` source metadata for the `id`, `name`, and `email` identity fields (`canonical`, `alias`, `managed_session_cookie`, or `absent`), additive persisted actor-attribution diagnostics (`actor`, `actorHeaderSupport`, `actorProvenance`, and `actorAttribution`) showing the effective plan/apply/audit actor, supported `X-Tenet-Actor-*` headers, compatible `X-User-*` actor aliases, and whether attribution came from explicit actor headers, trusted authenticated identity projection, or remained unavailable, plus the stable `actorAttribution.precedence` order (`explicit_actor_headers`, `actor_alias_headers`, `trusted_authenticated_identity`, `unavailable`) that explains how Tenet chooses persisted actor evidence, forwarded roles with read/write capability booleans, additive `roles.supportedNames` (currently `viewer`, `operator`, `admin`), additive `roles.supportedHeaderNames` (currently only `X-Tenet-Authenticated-Roles`, because role forwarding intentionally has no `X-User-*` compatibility alias), additive `roles.recognizedNames` / `roles.unrecognizedNames` diagnostics for forwarded values, and additive `roles.provenance.source` metadata (`canonical` for a normalized `X-Tenet-Authenticated-Roles` value, or `absent` when no trusted role value was forwarded), forwarded deployment allowlist, project/environment scopes, additive non-secret `desiredStateSource` diagnostics (filesystem versus GitHub catalog source, webhook match/ignored status vocabulary, and controller-refresh handoff availability such as Redis backend/channel without tokens or connection strings), and which auth/RBAC enforcement toggles are active for the request. Its additive `enforcement` object summarizes the API process mode (for example `trusted_front_door_read_write`) plus whether guarded read/write routes, deployment-scoped exact allowlists, and collection filtering are active. Its additive `accessVerdicts.deploymentScopedRead.acceptsProjectEnvironmentScopes` and `accessVerdicts.deploymentScopedWrite.acceptsProjectEnvironmentScopes` fields explicitly tell UI clients when matching project/environment scopes can satisfy those route classes in addition to exact deployment grants. Its additive `accessVerdicts` block summarizes whether common operator read, operator write, deployment-scoped read/write, filtered collection-read, and raw catalog-read paths have the request ingredients they need, including concrete `exampleRoutes` such as `GET /v1/overview/applications`, `GET /v1/overview/deployments/{deploymentName}`, and `POST /v1/plans/deployments/{deploymentName}` plus `requiredTrustedHeaders` such as `X-Tenet-Authenticated-Id`, `X-User-ID`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments` so operators can map each verdict class to shipped routes and forwarded header prerequisites without guessing. The verdicts also include clear reason strings for missing identity, missing read/write roles, missing exact deployment allowlist membership, and collection reads that are likely empty because no visible deployments were forwarded or catalog placement data is unavailable to match project/environment scopes. When deployment-read authorization is enforced and a catalog placement provider is available, `deploymentAccess.summary` also reports forwarded, matched/visible, and total unmatched deployment-name counts plus bounded unmatched-name evidence (`unmatchedNames`, with `unmatchedNamesTruncated` when the sample is incomplete), forwarded project/environment scope counts, bounded stale scope evidence (`unmatchedProjects` / `unmatchedEnvironments`, with matching truncation flags), and deterministic `projectEnvironmentExamples` (project, environment, region, AWS account, deployment count, and bounded example deployment names) so operators can spot stale front-door allowlists and project/environment coverage gaps. If the forwarded allowlist is valid but no catalog deployments are visible, the summary stays available and includes `emptyReason`; if catalog placement data cannot be loaded, the summary stays present with `available=false` and `unavailableReason`. That summary is diagnostic-only: it does not mint sessions, replace OAuth/RBAC, or authorize routes by itself. The enforcement summary and access verdicts are troubleshooting-only too; guarded routes still evaluate the request-scoped identity, roles, and exact deployment allowlist independently.
- The same trusted front-door read contract covers `GET /v1/overview/accounts/{awsAccountId}`, raw desired-state catalog reads under `/v1/catalog/*`, `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, `GET /v1/overview/applications/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, `GET /v1/status/deployments/{deploymentName}`, deployment-scoped `GET /v1/audit-logs`, and immutable stored reads `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` using the persisted record deployment.

Operator troubleshooting checklist for guarded reads and writes:
1. Start in the embedded `Operator context` banner, or call `GET /v1/operator/context` directly, before changing role or deployment headers. In the embedded banner, open `/#operator-context-access-details-panel` (or expand `Show access details` manually); those expanded details include the actor-attribution diagnostic (`actor`, `actorHeaderSupport`, `actorProvenance`, and `actorAttribution`), the `Supported role headers: X-Tenet-Authenticated-Roles` diagnostic, the read-only `desiredStateSource` diagnostic (filesystem/GitHub source metadata, controller-refresh handoff readiness, and webhook match/ignored reasons), the read-only `authRuntime.tenetManagedLogin` managed-login runtime diagnostic (configured/active/provider label/provider boundary/missing settings/storage/session), the read-only `authRuntime.enterpriseSso` diagnostic (configured/active/provider label/missing settings/storage/session), and the `Supported deployment access headers: X-Tenet-Authorized-Deployments, X-Tenet-Authorized-Projects, X-Tenet-Authorized-Environments` diagnostic, making clear that `X-Tenet-Actor-*` wins over compatible `X-User-*` actor aliases for persisted audit/plan/apply records, role forwarding has no `X-User-*` compatibility alias, Google Workspace OIDC does not imply Gmail mailbox access, Okta SSO maps only identity into the Tenet session, and deployment access can come from exact deployment grants or matching project/environment scopes. If that banner fetch is itself rejected with `401` / `403`, the shell reuses the same guarded-read recovery copy as the rest of the drill-down UI: default or inactive managed-login `401` recovery points operators back through the trusted front door that forwards `X-Tenet-Authenticated-*` identity headers, while an active same-origin Tenet-managed login or enterprise SSO diagnostic can switch that `401` card to a `_self` provider sign-in handoff that preserves the current URL as `returnUrl`; role and deployment `403`s continue to point operators to `Show access details` so they can inspect role provenance, `roles.supportedHeaderNames`, `roles.recognizedNames` / `roles.unrecognizedNames`, `roles.canRead`, `deploymentAccess.supportedHeaderNames`, `deploymentAccess.summary`, access verdicts, unmatched grants, and the required trusted headers for the rejected route.
2. Inspect `identity.authenticated`, `identityHeaderSupport`, `identityProvenance`, `actor`, `actorHeaderSupport`, `actorProvenance`, `actorAttribution`, `roles.provenance.source`, `roles.supportedHeaderNames`, `roles.supportedNames`, `roles.recognizedNames` / `roles.unrecognizedNames`, `roles.canRead` / `roles.canWrite`, `deploymentAccess.supportedHeaderNames`, and `deploymentAccess.summary` first; these explain `401`, which canonical and alias identity header families Tenet accepts, whether the request used canonical `X-Tenet-Authenticated-*` fields, compatibility `X-User-*` aliases, or the Tenet-managed `managed_session_cookie` source (rendered in the banner as `Tenet-managed browser session cookie`), whether persisted plan/apply/audit actor attribution will use canonical `X-Tenet-Actor-*`, compatible `X-User-*`, or trusted authenticated identity projection, and the exact `actorAttribution.precedence` order Tenet applies before persisting records, whether a trusted `X-Tenet-Authenticated-Roles` value was forwarded, that there is intentionally no `X-User-*` role alias, the exact role labels Tenet supports, which forwarded values Tenet recognized versus ignored, which deployment/project/environment access headers this API process understands, viewer-role `403`, and likely-empty collection reads before route-specific debugging. When placement data is available, `deploymentAccess.summary` reports both `exactDeploymentMatchedCount` and `projectEnvironmentMatchedCount`, and `visibleDeploymentExamples` gives a bounded set of concrete visible deployments with `matchSources` (`exactDeployment`, `projectEnvironment`, or both) so operators can see whether exact grants or project/environment scopes are actually responsible for access.
3. Inspect the `accessVerdicts` chips and their bounded route examples to map the blocked or likely-empty verdict to the concrete guarded route family, such as applications collections, raw catalog reads, deployment drill-downs, immutable plan/apply writes, or audit reads. Each verdict now includes `acceptedRoleNames`: read route families list `viewer`, `operator`, and `admin`, while write route families list `operator` and `admin`. Collection-read chips show `Collection scope: filtered to visible deployments` when Tenet will return only rows visible through the forwarded deployment/project/environment grants, add `Collection result: likely empty` when those grants are expected to produce an empty collection, and deployment-scoped read/write chips show `Scope grants: exact deployments or matching project/environment scopes` when either grant shape can satisfy that route class.
4. Debug the specific guarded route class by checking the banner's full `Required trusted headers:` fragment: identity, read/write role, and/or exact `X-Tenet-Authorized-Deployments` membership or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes. For project/environment-scoped reads and deployment-scoped plan/apply writes, both project and environment headers must be forwarded and must match catalog placement data; partial, stale, or unavailable placement data intentionally diagnose as likely `403`/empty instead of widening access.
5. If an operator write returns `403 Forbidden`, confirm `roles.canWrite=true` (`operator` or `admin`) plus either exact deployment membership or matching project/environment catalog placement; writes fail before plan/apply lookup when either prerequisite is missing.

A minimal diagnostic request mirrors the headers your front door should forward:

```bash
curl -fsS http://localhost:8080/v1/operator/context \
  -H 'X-Tenet-Authenticated-Id: operator-123' \
  -H 'X-Tenet-Authenticated-Roles: viewer,operator' \
  -H 'X-Tenet-Authorized-Deployments: payments-api-prod,worker-prod' \
  -H 'X-Tenet-Authorized-Projects: payments-api' \
  -H 'X-Tenet-Authorized-Environments: prod-apne2'
```

To evaluate repository policy diagnostics during local filesystem runs, add an optional `rbac/policy.json` file to the desired-state repository root:

```json
{
  "version": "v1",
  "users": [
    {
      "id": "operator-123",
      "roles": ["viewer"],
      "deployments": ["payments-api-prod"]
    }
  ],
  "groups": [
    {
      "name": "platform-admins",
      "roles": ["operator"],
      "projects": ["payments-api"],
      "environments": ["prod-apne2"]
    }
  ]
}
```

Then include the group claim alongside the normal front-door diagnostic headers:

```bash
curl -fsS http://localhost:8080/v1/operator/context \
  -H 'X-Tenet-Authenticated-Id: operator-123' \
  -H 'X-Tenet-Authenticated-Roles: viewer,operator' \
  -H 'X-Tenet-Authenticated-Groups: platform-admins' \
  -H 'X-Tenet-Authorized-Deployments: payments-api-prod,worker-prod' \
  -H 'X-Tenet-Authorized-Projects: payments-api' \
  -H 'X-Tenet-Authorized-Environments: prod-apne2'
```

To test whether a front door is forwarding unsupported role labels, send the same request with `X-Tenet-Authenticated-Roles: viewer,auditor`. The diagnostic should show `roles.recognizedNames=["viewer"]`, `roles.unrecognizedNames=["auditor"]`, `roles.canRead=true`, and `roles.canWrite=false`; `auditor` is reported so operators can fix the front-door/RBAC mapping, but it does not grant Tenet read or write capability.

Example diagnostic summary shape (including identity and role source provenance, where identity `canonical` means `X-Tenet-Authenticated-*`, identity `alias` means compatibility `X-User-*`, and role `canonical` means a normalized `X-Tenet-Authenticated-Roles` value was forwarded):

```json
{
  "identityHeaderSupport": {
    "canonical": [
      "X-Tenet-Authenticated-Id",
      "X-Tenet-Authenticated-Name",
      "X-Tenet-Authenticated-Email"
    ],
    "aliases": ["X-User-ID", "X-User-Name", "X-User-Email"]
  },
  "identityProvenance": {
    "idSource": "canonical",
    "nameSource": "alias",
    "emailSource": "absent"
  },
  "actorHeaderSupport": {
    "canonical": [
      "X-Tenet-Actor-Id",
      "X-Tenet-Actor-Name",
      "X-Tenet-Actor-Email"
    ],
    "aliases": ["X-User-ID", "X-User-Name", "X-User-Email"]
  },
  "actorProvenance": {
    "idSource": "canonical",
    "nameSource": "alias",
    "emailSource": "absent"
  },
  "actorAttribution": {
    "available": true,
    "source": "explicit_actor_headers",
    "precedence": [
      "explicit_actor_headers",
      "actor_alias_headers",
      "trusted_authenticated_identity",
      "unavailable"
    ],
    "summary": "persisted plan/apply/audit actor fields will use explicit X-Tenet-Actor-* headers, with X-User-* actor aliases only filling missing actor fields"
  },
  "roles": {
    "names": ["viewer", "operator", "auditor"],
    "canRead": true,
    "canWrite": true,
    "supportedNames": ["viewer", "operator", "admin"],
    "supportedHeaderNames": ["X-Tenet-Authenticated-Roles"],
    "recognizedNames": ["viewer", "operator"],
    "unrecognizedNames": ["auditor"],
    "provenance": {
      "source": "canonical"
    }
  },
  "deploymentAccess": {
    "supportedHeaderNames": [
      "X-Tenet-Authorized-Deployments",
      "X-Tenet-Authorized-Projects",
      "X-Tenet-Authorized-Environments"
    ],
    "summary": {
      "available": true,
      "forwardedCount": 2,
      "forwardedProjectCount": 1,
      "forwardedEnvironmentCount": 1,
      "matchedCount": 2,
      "exactDeploymentMatchedCount": 2,
      "projectEnvironmentMatchedCount": 1,
      "projectCount": 2,
      "environmentCount": 1,
      "visibleDeploymentExamples": [
        {
          "deploymentName": "payments-api-prod",
          "appRef": "payments-api",
          "environmentRef": "prod-apne2",
          "region": "ap-northeast-2",
          "awsAccountId": "123456789012",
          "matchSources": ["exactDeployment", "projectEnvironment"]
        },
        {
          "deploymentName": "worker-prod",
          "appRef": "worker",
          "environmentRef": "prod-apne2",
          "region": "ap-northeast-2",
          "awsAccountId": "123456789012",
          "matchSources": ["exactDeployment"]
        }
      ],
      "projectEnvironmentExamples": [
        {
          "appRef": "payments-api",
          "environmentRef": "prod-apne2",
          "region": "ap-northeast-2",
          "awsAccountId": "123456789012",
          "deploymentCount": 1,
          "exampleDeployments": ["payments-api-prod"]
        }
      ]
    }
  },
  "repositoryPolicy": {
    "configured": true,
    "available": true,
    "source": "repository",
    "version": "v1",
    "diagnosticOnly": true,
    "supportedGroupHeaderNames": ["X-Tenet-Authenticated-Groups"],
    "matchedUsers": [{"kind": "user", "name": "operator-123", "match": "id"}],
    "matchedGroups": [{"kind": "group", "name": "platform-admins", "match": "name"}],
    "roles": {"names": ["viewer", "operator"]},
    "deploymentAccess": {"names": ["payments-api-prod"]},
    "projectEnvironmentScopes": {"projects": ["payments-api"], "environments": ["prod-apne2"]},
    "canRead": true,
    "canWrite": true,
    "summary": "repository RBAC policy matched 2 subject grant(s): roles=2 deployments=1 projects=1 environments=1"
  },
  "accessVerdicts": {
    "operatorRead": {
      "acceptedRoleNames": ["viewer", "operator", "admin"]
    },
    "operatorWrite": {
      "acceptedRoleNames": ["operator", "admin"]
    }
  }
}
```

`deploymentAccess.supportedHeaderNames` (`supportedHeaderNames`) lists the exact trusted front-door deployment access headers Tenet recognizes so RBAC operators can compare their proxy/header-forwarding configuration with the active diagnostic payload.

If a front door forwards stale project/environment scopes, the same summary keeps the response diagnostic rather than silently widening access. Operators should look for the stale scope fields before debugging individual routes:

```json
{
  "deploymentAccess": {
    "summary": {
      "available": true,
      "forwardedProjectCount": 2,
      "forwardedEnvironmentCount": 2,
      "matchedCount": 1,
      "projectEnvironmentMatchedCount": 1,
      "unmatchedProjectCount": 1,
      "unmatchedProjects": ["archived-payments"],
      "unmatchedEnvironmentCount": 1,
      "unmatchedEnvironments": ["stale-env"]
    }
  }
}
```

Those fields are troubleshooting evidence only: stale project/environment scope diagnostics do not authorize deployment-scoped routes, mint a session, or replace the guarded route checks.

### Smoke verification scope

Use `make smoke-local` for the fastest fixture-backed API/UI regression check when you only need direct-process validation without Docker or Redis.

It verifies:
- API health plus representative catalog and status payloads.
- The executor-facing `POST /v1/applies/{applyId}/transitions` boundary on a smoke-created terminal apply record: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.
- The request-actor header contract by sending conflicting `X-Tenet-Actor-*` and `X-User-*` identities together and verifying the stored plan preview, stored apply request, and deployment-scoped audit lookup keep the explicit `X-Tenet-Actor-*` values.
- Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, one representative ECS resource route, and representative CodeDeploy `codeDeployApplication` / `blueTaskSetTargetGroup` resource routes, checking only raw embedded application/account/resource/drift/audit/plan shell strings needed to prove those drill-down pages can render and navigate.
- Trusted front-door auth by restarting `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS` plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY`, proving unauthenticated operator writes fail with `401`, verifying trusted `X-Tenet-Authenticated-*` headers project into stored plan/apply/audit actor fields when explicit actor headers are absent, and keeping the persisted `POST /v1/applies/{applyId}/transitions` write route guarded.
- Trusted front-door auth by restarting again with `TENET_API_TRUST_AUTHENTICATED_HEADERS`, `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`, and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`, proving unauthenticated read collections fail with `401`, authenticated callers without a read-capable role fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route`, deployment-filtered collections still return `[]` / `{"entries":[]}` when nothing remains visible, `GET /v1/operator/context` reports diagnostic `authRuntime.tenetManagedLogin` disabled-mode settings evidence, `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases`, `actorHeaderSupport.canonical` / `actorHeaderSupport.aliases`, alias-only `actorProvenance`, canonical `X-Tenet-Actor-*` precedence over conflicting `X-User-*` aliases, `actorAttribution.source`, `actorAttribution.precedence`, `accessVerdicts` for allowed guarded reads/writes plus their `requiredTrustedHeaders` and `acceptedRoleNames`, canonical `roles.provenance.source` evidence when trusted `X-Tenet-Authenticated-Roles` is forwarded, mixed forwarded-role diagnostics where `viewer, auditor` yields `roles.recognizedNames=["viewer"]` plus `roles.unrecognizedNames=["auditor"]`, absent role provenance when the role header is missing, missing-allowlist empty-collection risk, and stale project/environment scope evidence (`forwardedProjectCount`, `forwardedEnvironmentCount`, `unmatchedProjects`, and `unmatchedEnvironments`), and deployment-scoped reads keep the existing deployment-access `403` when the operator lacks access.
- The same shared operator-read guidance path on `/`, the bookmarkable application drill-down, the bookmarkable account drill-down, the resource/drift/audit shells, and the bookmarkable `/?page=plan` workspace; unauthenticated `401` copy defaults to the trusted-front-door retry path and switches to a same-shell sign-in handoff only when `/v1/operator/context` reports an active same-origin Tenet-managed login or enterprise SSO route (for example `authRuntime.enterpriseSso.provider=okta_oidc`, `active=true`, and `loginRoute=/auth/login/okta` produces `Sign in with Okta SSO` while preserving the current bookmarkable URL as `returnUrl`; configured-but-inactive Okta diagnostics remain non-clickable).

Use `make smoke-docker` when you need to validate the containerized local stack itself.

It verifies:
- `compose.yaml` plus the `api`/`controller`/`redis` service contract.
- API health, representative endpoints, deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope, `testListener` backing-resource metadata, weighted `activeTargetGroup` / `candidateTargetGroup` `no_log_source` recent-log payloads, ECS task drill-down `resource-logs?resourceType=ecsTask&containerName=worker&limit=1` plus paired live-inspection envelope coverage, bounded `resource-logs?limit=1` truncation/resultScope metadata plus `limit=0` rejection, and representative `Request-Id` response headers.
- Correlated structured API request logs, controller logs, resource-log routes, and non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows, additive detail-level `managedDriftSummary` severity counts, validates `resourceRef` shape when backend-authored metadata is present, and keeps the local-friendly no-resourceRef fallback explicit while aligned `runtimeSummary.managedDrift` remains present.
- The same compose-backed verification also keeps correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace aligned with the documented request-correlation surface.
- The same bookmarkable application, account, traffic-shift resource, ECS resource, drift, audit, and plan routes locked by direct-process smoke, still checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present. In that compose-backed verification surface, the root applications shell now also locks the shipped `Refresh applications overview` route-builder/loading/error hooks, while the bookmarkable application shell still exposes the `_self` `Applications overview` return path, the non-click `Application detail` destination chip, and the bookmarkable application-detail status summary shell. The same route family now includes the CodeDeploy application, task-set target-group, and weighted production listener-rule resource shells in both smoke lanes.
Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, one representative ECS resource route, representative CodeDeploy `codeDeployApplication` / `blueTaskSetTargetGroup` resource routes, and the weighted `productionListenerRule` resource route remain part of the same shell-level smoke contract.
- Both smoke lanes now cover the shipped CodeDeploy application, task-set target-group, and weighted `productionListenerRule` bookmarkable resource shells directly. CodeDeploy application, task-set target-group, and listener-rule resource shell coverage uses the same file-backed raw HTML/JS contract as the other resource pages. The local-friendly backend checks still treat `aws_unavailable` and `no_log_source` as part of the expected local no-AWS contract, including explicit `no_log_source` recent-log payloads for weighted `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` resources. The same compose-backed shell coverage now also locks the bookmarkable resource page's top-level `Refresh resource detail` shell (`GET /v1/overview/deployments/{deploymentName}` route-builder text plus the preserved-detail loading/error copy) alongside the existing `Refresh recent logs` and `Refresh live inspection` lazy-panel route-builder/loading/error/auth-guidance hooks from the served HTML/JS shell instead of runtime DOM text, so release-gate smoke stays aligned with the shipped resource troubleshooting surface.
- The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Resolved backing resource`, `Lookup path ·`, `Lookup region`, `Observer role configured`, and the runtime-events `Open field-level drift` handoff present so bookmarkable operator drill-down pages stay aligned with the compose-backed verification surface.
- The full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present, exact-action `GET /v1/audit-logs?action=apply.requested&limit=2`, and the invalid-action `400 Bad Request` boundary.
- The same smoke-created `POST /v1/applies/{applyId}/transitions` route boundaries: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.
- The same actor-header precedence check used by direct-process smoke, including `actorAttribution.precedence`, plus the default disabled immutable apply contract.
- Trusted front-door auth by restarting the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS`, `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY`, `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`, and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`, proving unauthenticated guarded reads fail with `401`, authenticated callers without a read-capable `viewer` / `operator` / `admin` role fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route`, trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment, stale project/environment scopes return empty guarded collections, trusted `X-Tenet-Authorized-Deployments` still filters returned application rows, raw status snapshots, account aggregates, and unscoped audit entries, `GET /v1/operator/context` reports `authRuntime.tenetManagedLogin` disabled-mode settings evidence, `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases`, canonical forwarded-role provenance, missing-role `roles.provenance.source=absent` diagnostics while preserving canonical identity provenance, and reports all-matched, combined exact-plus-project/environment, and stale/unmatched forwarded deployment allowlist diagnostics through `deploymentAccess.summary`, including `visibleDeploymentExamples` rows with deployment name, project, environment, region, AWS account, and `matchSources` evidence, plus stale project/environment scope evidence (`forwardedProjectCount`, `forwardedEnvironmentCount`, `unmatchedProjects`, and `unmatchedEnvironments`) and the project/environment example region, AWS account, deployment count, and bounded deployment-name evidence, unauthenticated `POST /v1/applies/{applyId}/transitions` fails with `401`, authenticated but unauthorized transitions fail with `403`, and authorized resource drill-down reads still preserve `available=false` plus `unavailableReason` when live lookup data is unavailable.
- Resource drill-down auth and smoke wording keep the `available=false` plus explicit `unavailableReason` payload contract visible for authorized reads when live lookup data is unavailable.

GitHub Actions mirrors those checks in two lanes under `.github/workflows/ci-smoke.yml`. The fast lane runs on every pull request plus pushes to `main`, pins the Go toolchain from `go.mod`, runs `go test ./...`, syntax-checks both `scripts/smoke-local.sh` and `scripts/smoke-docker.sh` with `bash -n`, then runs `make smoke-local`. The slower Docker lane still runs on pushes to `main` plus manual `workflow_dispatch` runs; on pull requests, a cheap `docker-smoke-scope` job first checks changed paths and runs the full `make smoke-docker` container stack only when the PR touches Compose/front-door/auth/API/controller, GitHub webhook, resource evidence/logs/inspection, or plan/apply/audit persistence smoke surfaces such as `compose.yaml`, `Dockerfile`, `Makefile`, `scripts/smoke-docker.sh`, `scripts/docker-up.sh`, `cmd/tenet-api/**`, `cmd/tenet-controller/**`, `internal/api/**`, `internal/githubwebhook/**`, managed-login provider/session persistence packages (`internal/authprovider/**`, reserved Okta SAML/enterprise runtime paths `internal/authsaml/**` and `internal/authsso/**`, `internal/authidentity/**`, `internal/authstate/**`), `internal/requestactor/**`, `internal/statusstore/**`, `internal/resourcelogs/**`, `internal/resourceinspection/**`, `internal/planpreview/**`, `internal/applyrequest/**`, `internal/auditlog/**`, or `.github/workflows/ci-smoke.yml`. Unrelated docs-only PRs keep the cheap fast lane without the Docker stack.

Both smoke scripts keep intermediate deployment-detail, plan-preview, and apply-record payloads on temp files via the shared `write_payload` helper instead of passing full JSON blobs through environment variables. That keeps the shell/Python handoff portable on Linux CI runners where environment-size limits are tighter than local shells. The same smoke/docs contract now explicitly says it checks raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text, including the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down, its bookmarkable application-detail status summary shell, and bookmarkable `/?page=resource&deployment=payments-api-staging&resourceType=codeDeployApplication&resourceId=codeDeployApplication::payments-api-staging-codedeploy-app::CodeDeploy application coordinating ECS traffic shifting`, `/?page=resource&deployment=payments-api-staging&resourceType=blueTaskSetTargetGroup&resourceId=blueTaskSetTargetGroup::payments-api-staging-blue::Blue task set target group CodeDeploy treats as the live ECS traffic destination`, and `/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule::payments-api-prod-prod-listener::Production listener rule operators inspect while Tenet updates weighted forwarding during rollout` shells, so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present. The documented smoke surface still includes additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence through immutable plan/apply/audit records.

The same trusted front-door read contract also now explicitly covers `GET /v1/overview/deployments/{deploymentName}/events`: smoke/docs verification treat it like the other deployment-scoped drill-down routes, with `401 Unauthorized` unauthenticated reads, `403 Forbidden` authenticated-but-unauthorized reads, and `200 OK` authorized reads while preserving the existing `deploymentName` plus `events` list payload contract and the additive `managedDriftSummary` severity counts. That same read-contract wording now also keeps the exact operator-facing read body `unauthenticated: authenticated identity is required for this operator read route` visible in the README so the documented `401` surface matches the guarded routes, it keeps the matching deployment-visibility denial `forbidden: operator does not have access to this deployment` visible for scoped `403` cases, and it keeps the embedded write-route framing phrase `authenticated identity is required on operator write routes` visible for the bookmarkable plan/apply workspace guidance.

That compose-backed smoke description is intentionally explicit: it checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, including the compose-backed `codeDeployApplication` recent-log envelope, `testListener` backing-resource metadata, weighted target-group `no_log_source` recent-log payloads, the ECS task drill-down `resource-logs` request with a task ARN plus `containerName=worker` and the paired `resource-inspection?resourceType=ecsTask` envelope, bounded `resource-logs?limit=1` truncation/resultScope metadata, and invalid `resource-logs?limit=0` `400 Bad Request` handling, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, and then locks the matching bookmarkable shell contracts.

README auth/verification anchors for the current front-door slice:
- write creation for `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` now evaluates the requested deployment, the persisted preview deployment, and the persisted apply deployment on the matching guarded write/read paths.
- `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes. It also gates raw catalog collections such as `GET /v1/catalog/deployments` through the same trusted identity, viewer-role, and deployment-visibility checks.
- collection routes keep explicit empty-success semantics when nothing is visible: `GET /v1/overview/applications`, raw catalog list routes such as `GET /v1/catalog/deployments`, `GET /v1/status/deployments`, and account collections return `[]`, while raw catalog `GET /v1/catalog/summary` / `GET /v1/catalog/topology` recompute counts from visible catalog rows and the unscoped audit collection still returns `{"entries":[]}` when no audit rows are visible.
- security wording stays explicit: only enable any of these settings when every request reaches Tenet through a front door that authenticates the caller, strips any client-supplied `X-Tenet-Authenticated-*` headers, strips any client-supplied `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments` headers, strips any client-supplied trusted-alias `X-User-*` headers, and forwards only trusted values; operators must retry through the trusted front door when Tenet needs authenticated identity or trusted forwarded roles.
- local actor precedence stays explicit: `X-Tenet-Actor-*` values still win over `X-User-*` aliases for that local actor path, while the trusted authenticated identity path resolves from `X-Tenet-Authenticated-*` headers or additive `X-User-*` aliases; when explicit actor headers are absent, Tenet now projects that authenticated identity into the persisted actor fields.
- smoke/docs auth probes stay concrete: unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/catalog/deployments`, unauthenticated `GET /v1/status/deployments`, unauthenticated `GET /v1/overview/accounts`, and `GET /v1/audit-logs` all keep explicit `401` coverage; `GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`, trusted `X-Tenet-Authenticated-Roles: viewer` deployment detail/status/events and immutable plan/apply reads succeed through matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes even when `X-Tenet-Authorized-Deployments` does not name the deployment, stale project/environment scopes return empty guarded collections, both smoke lanes assert `GET /v1/operator/context` `identityHeaderSupport.canonical` / `identityHeaderSupport.aliases`, `actorAttribution.precedence`, `roles.provenance.source` for canonical forwarded roles and missing-role `absent` cases while preserving canonical `identityProvenance` in the missing-role case, and both smoke lanes prove a mixed `viewer, auditor` forwarded-role header yields `roles.recognizedNames=["viewer"]` plus `roles.unrecognizedNames=["auditor"]` while keeping the embedded shell's `Recognized capability roles:`, `Unsupported forwarded roles ignored for capability checks:`, and read-versus-write capability guidance locked, plus `deploymentAccess.summary` for all-matched, combined exact-plus-project/environment, and stale/unmatched forwarded allowlists, including `visibleDeploymentExamples` rows with deployment name, project, environment, region, AWS account, and `matchSources` evidence plus region/AWS account/deployment-count fields on `projectEnvironmentExamples`, and the compose restart keeps read-role denial and write-route allowlist denial distinct.
- bookmarkable shell coverage still includes `/?page=drift&deployment=payments-api-prod`, `/?page=audit&deployment={deploymentName}&action={action}&limit={n}`, the plan/apply workspace transition checks, and correlated structured `http_request` logs with stable `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` fields.
- raw shell/docs anchors remain literal: `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Resolved backing resource`, `Lookup path ·`, `Lookup region`, `Observer role configured`, and the runtime-events `Open field-level drift` handoff.
- `make smoke-docker` also locks non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows, the additive detail-level `managedDriftSummary` severity counts, validates `resourceRef` shape when backend-authored metadata is present, and keeps the local-friendly no-resourceRef fallback explicit while aligned `runtimeSummary.managedDrift` remains present.

### Run with Docker

```bash
make docker-up
```

If `make docker-up` exits before starting the stack, `localhost:8080` is already owned by something outside this Compose project. Inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`. If you want to clear a prior Tenet stack before retrying, run `make docker-down`, then rerun `docker compose up --build -d`.

Leave `TENET_AWS_ENABLE_APPLY` unset for the default disabled immutable apply contract. Set `TENET_AWS_ENABLE_APPLY=true make docker-up` when you want the queued-on-create local-safe executor handoff contract in the local Compose stack on a fresh plan/apply pair; this still does not execute AWS mutations.

Both smoke lanes now send signed accepted-push, deleted-ref push, ignored-ping, and invalid-signature webhook requests; the deleted-ref probe keeps `status=ignored`, `processed=false`, `deleted=true`, retained repository/ref/before/after summary fields, and a reason that says controller refresh handoff was skipped.

The local Compose stack already sets a safe default `TENET_GITHUB_WEBHOOK_SECRET=smoke-...et`, so `make smoke-docker` can send signed webhook requests immediately. Override `TENET_GITHUB_WEBHOOK_SECRET` when you want a different local secret, and set it explicitly for direct-process runs. The default local stack still proves the explicit unavailable path because it uses the filesystem desired-state source, so accepted webhook pushes stay `processed=false` there. In GitHub catalog mode with `TENET_STATUS_BACKEND=redis`, the same route publishes a Redis controller refresh handoff and the controller reconciles immediately on receipt instead of waiting only for the next ticker interval.

Use `make smoke-docker` when you need the containerized release gate rather than the direct-process smoke. It validates `compose.yaml`, the `api` / `controller` / `redis` service contract, representative API health and JSON shapes, request-correlation headers/logs, webhook signature handling, immutable plan/apply/audit records, default disabled apply behavior, direct-process actor precedence parity, trusted front-door read/write auth probes, raw embedded shell contracts, desired-state source / controller-refresh Operator context diagnostics, and local-friendly no-AWS unavailable states. The full `GET /v1/audit-logs` collection with expected `plan.created` and `apply.requested` actions present remains part of that gate, along with exact-action filtering and invalid-action `400 Bad Request` coverage.

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

The applications overview keeps only one primary row action visible: `_self` `Open application` to `/?page=application&deployment={deploymentName}`. Secondary handoffs move behind a `More actions` disclosure per row: `_self` `Open account`, deployment-scoped `_self` `Drift view`, `_self` `Plan/apply`, and `_self` `Audit log` links remain bookmarkable without crowding first-page triage. Application rows include a compact optional `Latest runtime signal` line when the already-loaded runtime summary contains meaningful evidence, preferring controller change details, then runtime summary text, then concise condition or rollout-step cues; rows without real runtime evidence omit the line instead of rendering placeholder dashes. That line includes an `_self` `Open runtime events` handoff to `/?page=application&deployment={deploymentName}#application-runtime-events` so operators can jump from first-page triage to the deeper controller timeline without making the landing page dense. The landing page still includes an in-place `Refresh applications overview` control that re-runs only `GET /v1/overview/applications`, disables only while that overview fetch is active, keeps the current bookmarkable `q` / `sync` / `health` / `account` query state stable, and leaves the current application rows visible with explicit loading/error copy while the refresh runs or fails. The `health` query parameter accepts the same row labels operators see in the table (`Healthy`, `Progressing`, `Unhealthy`, or `Unknown`) so filtered links can be shared for health-focused triage. When filters are active, the summary card shows visible applications as `N of M`, echoes the active filter labels, and scopes `Last update` to the visible subset so operators do not mistake a narrowed result for the whole estate. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance as the deeper bookmarkable drill-downs instead of falling back to generic refresh-error text. When an active `q` search matches hidden runtime-only evidence (for example controller change details, conditions, rollout step text, or artifact values), the matching row adds a concise `Search match` context line so the operator can see why it matched without turning the landing page into a dense runtime view.

The embedded shell also renders a compact `Operator context` banner above the toolbar. It calls `GET /v1/operator/context` and keeps the always-visible row to one status line: trusted front-door identity, trusted-header mode, read/write capability, and deployment-grant summary plus refresh/error status. After the first successful load, the banner also shows a concise `Last checked` timestamp, preserves it while `Refresh context` is in flight, and keeps the last successful timestamp visible if a refresh fails so operators can tell whether a long-lived tab is showing stale-but-known-good trusted-front-door diagnostics. A small status-only live region announces checking/loading, ready identity, refresh, unavailable, and error states; the surrounding `Operator context` section is not a live region, so screen readers are not asked to announce the verbose diagnostics payload when the banner rerenders. Guarded deployment-scoped plan/apply writes require both `X-Tenet-Authorized-Projects` and `X-Tenet-Authorized-Environments` to match catalog placement when no exact deployment grant is forwarded. Verbose diagnostics now sit behind an explicit `Show access details` / `Hide access details` disclosure that stays in the operator's chosen expanded/collapsed state across `Refresh context` rerenders. The same expanded details now render non-secret `desiredStateSource` diagnostics so operators can see whether this instance is using filesystem or GitHub desired state, which GitHub repo/ref/path/base URL is configured when present, whether Redis controller-refresh handoff is available, and the webhook match/ignored reason vocabulary (`matched`, `repository_mismatch`, `ref_mismatch`, `path_untouched`, `deleted_ref_ignored`, and `controller_refresh_unavailable`) without exposing secrets or sending a test webhook. The authentication-boundary reminder distinguishes the default trusted-front-door path from an explicitly active Tenet-managed browser login: when `authRuntime.tenetManagedLogin.active=false`, guarded-read `401` recovery keeps the front-door retry copy; when it is active and reports a same-origin `loginRoute`, the recovery actions offer a `_self` `Sign in with Tenet-managed login` handoff that preserves the current bookmarkable path/query/hash as `returnUrl`. The same disclosure includes managed-login runtime diagnostics from `authRuntime.tenetManagedLogin` (configured/active/provider, required or missing settings, login/callback/session-cookie metadata, cookie attributes (`path`, `HttpOnly`, `SameSite`, configured secure override, current-request secure decision, and secure evidence such as HTTPS or forwarded-proto headers), and state/identity/session storage readiness), identity and role header provenance, supported identity header families from `identityHeaderSupport` (canonical `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, `X-Tenet-Authenticated-Email`; compatibility aliases `X-User-ID`, `X-User-Name`, `X-User-Email`, with canonical values preferred when both are present), supported Tenet role labels from `roles.supportedNames`, the canonical supported role-header list from `roles.supportedHeaderNames` (`X-Tenet-Authenticated-Roles`, with no `X-User-*` role alias), the deployment access header list from `deploymentAccess.supportedHeaderNames` (`X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, and `X-Tenet-Authorized-Environments`), bounded route examples, required trusted headers, project/environment visibility examples, stale-scope warnings, visible deployment examples, and access-verdict details remain available without pushing the main applications workflow down the page. Unsupported forwarded-role guidance uses the same capability split: forward `viewer`, `operator`, or `admin` for read capability, and forward only `operator` or `admin` for write capability. Operators can still immediately tell whether this API process is local-unrestricted, observed-only, read-only guarded, write-only guarded, or read/write guarded before interpreting guarded-route denials or likely-empty collection reads.

### Audit timeline
Use the audit page when you need immutable operator activity instead of controller-derived runtime history.

The page is backed directly by `GET /v1/audit-logs` and currently focuses on:
- newest-first `plan.created` and `apply.requested` entries
- a bookmarkable action filter that narrows the page and fetch route to exactly `plan.created` or `apply.requested`
- a bookmarkable limit control in the audit toolbar that updates the fetch route and URL in place while preserving deployment and action filters
- an explicit in-place `Refresh audit log` control that re-fetches only `GET /v1/audit-logs`, disables only while that audit fetch is active, and keeps the current bookmarkable `/?page=audit...` URL/query state stable
- an explicit in-page `Clear audit filters` / `Clear audit controls` control that appears when deployment/action filters or a limit cap are active and resets those audit controls back to the default unscoped `/?page=audit` route while preserving bookmarkable `q` / `sync` / `account` state
- active filter copy such as `Audit filters: action Apply requested (apply.requested) · Limit cap: latest 2` when you reopen `/?page=audit&action=apply.requested&limit=2`
- deployment-scoped filter copy such as `Audit filters: deployment payments-api-prod · action Apply requested (apply.requested) · Limit cap: latest 2` when you reopen `/?page=audit&deployment=payments-api-prod&action=apply.requested&limit=2`
- deployment / app / environment context
- the exact current audit read route, such as `GET /v1/audit-logs?deployment=payments-api-prod&action=apply.requested&limit=2`, rendered near the timeline intro so operators can confirm which scoped collection request backs the page before troubleshooting auth, empty states, or stale bookmarks
- a compact visible-entry summary that counts total visible audit rows, `plan.created` rows, `apply.requested` rows, recorded actors, and unavailable/unknown actors for the current deployment/action/limit result set
- deployment-scoped chrome/body copy plus a deployment-specific empty state when `deployment={deploymentName}` is present and no persisted audit entries match
- deployment-scoped quick navigation that keeps `Applications overview`, `Back to application`, the current non-click `Audit log` destination, sibling `Drift view` / `Plan/apply` workspace links, and a hash-aware `Current section` / `On this page` pair for `Deployment context` and `Audit timeline` fragments such as `/?page=audit&deployment=payments-api-prod&limit=2#audit-timeline`
- the current audit section becomes a non-click chip when that hash is active, while the remaining audit sections stay as `_self` fragment links and otherwise fall back to `No section is currently targeted.`
- initial `GET /v1/audit-logs` auth failures now render operator-specific guidance instead of generic fetch-error copy: `401` tells the operator to reopen the same bookmarkable page through the trusted front door so authenticated identity can be provided, read-role `403` points admins at `Show access details` for role provenance, `roles.canRead`, and `X-Tenet-Authenticated-Roles`, and deployment-access `403` points at `deploymentAccess.summary`, access verdicts, unmatched grants, and the matching deployment, project, or environment scope
- keeps the current audit content visible with explicit loading/error copy while that in-place refresh is running or fails for the same bookmarkable route
- explicit actor-unavailable or actor-unknown labeling instead of fabricated identity
- `_self` links back into `/?page=application&deployment={deploymentName}` as `Open application drill-down`, into `/?page=drift&deployment={deploymentName}` as `Open drift view`, and into `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}` as `Open plan/apply workspace` when those identifiers exist

### Account detail
Open `/?page=account&accountId={awsAccountId}` to get the denser account/group deployment slice without leaving the embedded shell. Its initial `GET /v1/overview/accounts/{awsAccountId}` load now uses the same operator-read auth handling as the other bookmarkable drill-downs: `401` tells the operator to reopen the same page through the trusted front door so identity can be forwarded, role-gated `403` asks operators to check `Show access details` for role provenance, `roles.canRead`, and the forwarded `X-Tenet-Authenticated-Roles` value, deployment-access `403` asks them to compare `deploymentAccess.summary`, access verdicts, and unmatched deployment/project/environment grants, and other statuses keep the existing unavailable/generic fetch-error behavior. The same account page now also renders compact `Quick navigation` chrome with an `_self` `Applications overview` return link plus a non-click `Account detail` chip so operators can stay oriented inside the bookmarkable account route without losing current `q`, `sync`, or `account` query state. That quick-nav now also adds a hash-aware non-click `Current section` chip plus an `On this page` group covering the `Account summary` and rendered project/environment groups, with a friendly fallback of `No section is currently targeted.` until the bookmarkable route includes a matching `#account-*` fragment such as `#account-summary`. When the same project/environment pair appears in multiple regions, those section links now keep the region in the label so the operator can tell the target groups apart before jumping. The same page now exposes an in-place `Refresh account detail` control that re-runs only `GET /v1/overview/accounts/{awsAccountId}`, disables only while that refresh is active, keeps the current bookmarkable `/?page=account...` route and hash stable, and leaves the stale account content visible with explicit loading/error copy while the refresh is running or if it fails. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance instead of falling back to generic error text.

### Application detail
Open one application to see the linked rollout resources grouped into clear sections such as:
- workloads
- traffic routing
- config and access

The top of that page now also includes a compact `Current application status` block so operators can answer “what is happening with this app right now?” before scanning resource cards. The summary stays intentionally small and read-only: it surfaces the current sync state, health, rollout context, artifact, linked-resource count, managed-drift severity totals from `managedDriftSummary` (falling back to current `managedDrift[]` rows, or `No managed drift reported`), runtime-event availability, and last update using the existing application-detail payload instead of reintroducing the older dense dashboard.

Each linked resource row keeps the generic `_self` `Open resource` drill-down, and resource types that already support the final page’s existing evidence panels now also expose direct `_self` handoffs into those stable section anchors: `Open live inspection` jumps straight to `#resource-live-inspection`, and `Open recent logs` jumps straight to `#resource-recent-logs` for route-stable log resources, including CodeDeploy application/deployment-group/listener/task-set rows plus weighted traffic-shift plan, listener-rule, and target-group rows that may legitimately resolve to friendly `no_log_source` evidence. Unsupported resource types stay honest by omitting those shortcuts instead of advertising evidence surfaces the final page cannot currently honor.

On the initial `GET /v1/overview/applications/{deploymentName}` load, embedded auth failures are also status-aware: `401` tells the operator to reopen the same bookmarkable page through the trusted front door so identity can be forwarded, role-gated `403` points at `#operator-context-access-details-panel` / `Show access details` for role provenance, `roles.canRead`, and the forwarded `X-Tenet-Authenticated-Roles` value, deployment-access `403` points at `deploymentAccess.summary`, access verdicts, stale/unmatched grants, and the deployment/project/environment scope, and other statuses keep the existing unavailable/generic fetch-error behavior. After the page is open, the same drill-down now exposes an in-place `Refresh application detail` control that re-runs only `GET /v1/overview/applications/{deploymentName}`, disables only while that refresh is active, keeps the bookmarkable `/?page=application...` route stable, and leaves the current compact status summary, grouped-resource content, and controller-runtime-events content visible with explicit loading/error copy. Refresh-time `401` / `403` responses reuse the same trusted-front-door / access-denied guidance instead of falling back to generic refresh-error text.

### Resource detail
Open a concrete resource to inspect the deeper operational context for that resource, including rollout and runtime information. The generic read-only route families remain `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection`. Supported-but-unavailable reads keep the route usable with `available=false` plus explicit `unavailableReason` payload contract instead of vague server errors. The embedded resource page now includes a top-level `Refresh resource detail` action that re-runs `GET /v1/overview/deployments/{deploymentName}` in place, preserves the bookmarkable `deployment` / `resourceType` / `resourceId` URL, re-resolves the selected resource from the refreshed payload, and keeps the last successful resource detail visible while refresh-time loading/error guidance renders above it. It also includes a compact `Quick navigation` strip with a non-click `Current resource` chip, a hash-aware non-click `Current section` chip when the bookmarkable route targets one of the rendered resource sections, and an `On this page` group for the remaining stable section hashes such as `#resource-runtime-history`, `#resource-live-inspection`, and `#resource-recent-logs`. The existing `_self` jumps back to the application, drift, plan/apply, audit, and sibling resources remain unchanged. The resource page also includes a `Live inspection` panel that lazy-loads the resource-inspection route for the selected resource while keeping bookmarkable resource URLs unchanged. The panel renders explicit loading, unavailable, empty, and supported states, reuses the same safe DOM construction patterns as the rest of the embedded UI, and keeps lookup context visible when Tenet cannot return a live payload. Operators now also get an in-place `Refresh live inspection` control on that panel so they can re-run only that panel-local backend route in place; the control disables only while its own fetch is active, preserves the bookmarkable URL, and leaves the sibling panel and the rest of the resource page visible. When that lazy read returns `401` or `403`, the panel keeps the same in-place refresh behavior but replaces generic fetch-error copy with the same trusted-front-door / access-denied guidance used on other embedded read paths. EC2 `userDataBootstrapSource` resources now also render a dedicated live-inspection payload there, reusing the same launch-template bootstrap-source evidence Tenet already uses for controller-managed drift so operators can compare the normalized desired bootstrap source against the observed launch-template marker without leaving the existing resource drill-down route. When that bootstrap-source payloa... [truncated]

Recent logs are still surfaced here when the resource type supports them, and the recent-log card keeps the backend lookup path explicit with a compact `Lookup path · ...` summary plus operator-facing labels for region, control account, target account, cross-account status, and observer-role configuration even when no lines are returned or the lookup is unavailable. Known `unavailableReason` values render friendly operator copy, and unknown provider/internal codes fall back to generic troubleshooting guidance instead of being shown literally. Route-stable traffic-routing resources with `unavailableReason=no_log_source` render friendly copy explaining that the resource is a valid drill-down target but has no independent recent-log stream yet, rather than exposing the raw machine code to operators. That card now also includes its own `Log limit` selector (`All available logs`, `5 entries`, `10 entries`, and `25 entries`) plus a `Refresh recent logs` control. Selecting a positive cap adds `limit={n}` only to the lazy recent-log backend read, threads the cap into provider-aware lookups such as ECS CloudWatch `GetLogEvents` while keeping router truncation as a fallback, renders visible `Log entry cap` copy in the card, preserves the bookmarkable `/?page=resource...` URL, and leaves the sibling `Live inspection` panel untouched; refresh reuses the selected cap with the same per-panel loading disablement. The card also renders backend `resultScope` evidence as `Requested cap`, `Effective cap`, `Returned entries`, and conservative `Scope status` copy so operators can see whether Tenet returned a capped slice, a below-cap response, an at-cap response with unknown provider totals, or an unavailable/not-applicable lookup without implying exact AWS totals. The card still uses the same trusted-front-door / access-denied `401` / `403` guidance instead of generic fetch-error text when the guarded lazy read is denied. `userDataBootstrapSource` now participates in that same recent-log path by reusing the backing Auto Scaling Group Tenet already resolves for bootstrap-source inspection; CodeDeploy listener and task-set target-group sibling resources likewise reuse the backing deployment-group history. When that happens, the payload stays read-only and additive by preserving the requested resource identity while also surfacing optional `backingResourceType` and `backingResourceName` fields for the resolved backing resource. The embedded `Recent logs` card renders those additive fields directly as `Backing resource type` and `Backing resource name` without changing the route shape.

The compose-backed smoke now covers shipped resource drill-down shells directly: it fetches the bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=userDataBootstrapSource&resourceId=userDataBootstrapSource::file:userdata/payments-api-prod.sh::Normalized userdata bootstrap source for candidate EC2 capacity` route plus the matching canonical backend reads `GET /v1/overview/deployments/payments-api-prod/resource-logs?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh` and `GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=userDataBootstrapSource&resourceName=git:userdata/payments-api-prod.sh`. It also fetches the bookmarkable weighted listener-rule shell `/?page=resource&deployment=payments-api-prod&resourceType=productionListenerRule&resourceId=productionListenerRule::payments-api-prod-prod-listener::Production listener rule operators inspect while Tenet updates weighted forwarding during rollout` and its `resource-inspection` fallback payload without requiring live AWS credentials. Local smoke treats explicit unavailable reasons such as `aws_unavailable` plus summary-only `trafficRoutingResource` fallbacks as the expected no-AWS contract; live `trafficRoutingResource.listenerRule` fields appear only when AWS lookup succeeds and the selected resource name is an ELB listener-rule ARN.

Canonical resource-log routes stay stable across rollout modes so operators can bookmark or script them:
- `asgWeightedTargetGroups`: `trafficShiftPlan` (`{deployment}-weighted-routing`), `productionListenerRule` (`{deployment}-prod-listener`), `activeTargetGroup` (`{deployment}-active`), and `candidateTargetGroup` (`{deployment}-candidate`)
- `ecsRolling`: `ecsService` (`{deployment}-service`)
- `ecsCodeDeploy`: `codeDeployApplication` (`{deployment}-codedeploy-app`) for application-wide CodeDeploy deployment history, `codeDeployDeploymentGroup` (`{deployment}-codedeploy-group`) for deployment-group-scoped history, plus the sibling `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` routes that reuse that same deployment-group history through additive backing-resource metadata

Behind those routes, the built-in AWS-backed recent-log provider now reuses SDK config/client setup per `(region, target account, observer role)` key inside a running Tenet API process instead of rebuilding ECS / CloudWatch Logs / Auto Scaling / EC2 clients on every fetch, and it reuses that same AWS config for signed CodeDeploy deployment-history lookups. For `ecsService`, the provider inspects both newest running tasks and recently stopped service tasks, then returns the same bounded newest-first `entries[]` envelope so failed rollouts and crash loops can still expose container logs even when no task is currently running.

Current provider-backed recent-log support includes:
- `ecsService` across newest running and recently stopped service tasks
- `ecsTask` for an exact task ARN, with optional `containerName={container}` to narrow the bounded CloudWatch Logs read to one awslogs-backed container; responses echo that selector as `containerName` when supplied so scripted clients can distinguish container-scoped task lookups from task-wide reads
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource` via the resolved backing Auto Scaling Group
- `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` as route-stable weighted traffic-routing resources that may return explicit `no_log_source` evidence in local/no-AWS runs
- `codeDeployApplication` via newest-first CodeDeploy deployment history for the selected application
- `codeDeployDeploymentGroup` via newest-first CodeDeploy deployment history for the selected deployment group
- `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` via the backing CodeDeploy deployment-group history for the same rollout

Recent-log payloads also echo the selected rollout execution description as additive `resourceDescription` when Tenet resolved one, including available and explicit unavailable responses. That keeps scripted troubleshooting output self-describing without forcing clients to re-fetch deployment detail just to explain the selected resource.

The same drill-down now has a parallel inspection route family:
- `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}`

Supported always-live inspection currently starts with:
- `ecsService`
- `ecsTask` for an exact ECS task ARN or task identifier, returning task status plus container and awslogs stream metadata when ECS task-definition data is available
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

Within one running Tenet API process, repeated successful live inspections for `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `productionListenerRule`, `activeTargetGroup`, `candidateTargetGroup`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` now reuse a bounded in-process cache keyed by `(region, target account, observer role, resource type, resource name)`. Summary-only fallbacks and unavailable outcomes are still recomputed or returned through the existing non-cached path.

For `ecsService`, Tenet now keeps a bounded in-process hint of the last cluster that resolved a given `(region, target account, observer role, service name)` lookup so repeated drill-downs avoid brute-force cluster scans. If that hint is stale or the service moved, the lookup falls back to the full cluster scan path, refreshes the hint, and keeps the existing resource-not-found semantics unchanged.

The embedded `Live inspection` panel renders operator-friendly summaries from those payloads:
- `ecsService`: service identity/status, desired-running-pending counts, additive deployment-configuration fields (`minimumHealthyPercent`, `maximumPercent`, `healthCheckGracePeriodSeconds`) when ECS returns them, active deployments, recent ECS service events, and a bounded newest-first `tasks[]` bridge for exact running/stopped ECS task drill-downs when task discovery succeeds; those task rows include task/container/awslogs evidence plus `_self` handoffs into task live-inspection/recent-log anchors, while sibling `tasksScope` metadata reports the effective cap, returned row count, and conservative truncation status (`not_truncated`, `truncated`, or `unknown`)
- `autoScalingGroup`: group sizing/health settings, attached target groups, current instances, and additive launch-template details including `launchTemplate.imageId` and `launchTemplate.instanceType` when the live EC2 launch-template lookup resolves them
- `userDataBootstrapSource`: normalized desired bootstrap source, observed launch-template bootstrap-source marker, the backing Auto Scaling resource Tenet inspected, the launch template version Tenet used for that observation, and an additive in-app handoff to the resolved backing resource drill-down when that resource can be matched safely from the loaded deployment metadata
- `trafficRoutingResource`: summary-only or live traffic-routing context with explicit resource kind, display name, inspection mode, rollout summary text, additive live fields for CodeDeploy applications/deployment groups/listeners and ELB listener rules when AWS access is available, and target-group live details for `activeTargetGroup` / `candidateTargetGroup` / `blueTaskSetTargetGroup` / `greenTaskSetTargetGroup` payloads that include protocol/port, health-check settings, load-balancer ARNs, and registered target-health counts

Resource-inspection responses keep the contract explicit for UI consumers:
- common fields: `deploymentName`, `resourceType`, `resourceName`, `lookupContext`, `available`, `source`, `unavailableReason`
- ECS service payloads add `ecsService`, including additive deployment-configuration fields `minimumHealthyPercent`, `maximumPercent`, and `healthCheckGracePeriodSeconds` when `DescribeServices` returns them, plus additive `tasks[]` summaries for up to 10 newest inspected running/stopped tasks when `ListTasks` / `DescribeTasks` evidence is available. Each task row keeps the existing `taskArn`, `taskId`, status/health/launch fields, timestamps, stopped reason, and container rows with optional `awslogs.group` / `awslogs.stream` metadata when task-definition lookup succeeds; sibling `tasksScope` reports `effectiveLimit`, `returnedTaskCount`, and `truncationStatus` so operators can tell whether Recent ECS tasks is complete, explicitly truncated, or conservatively unknown at the cap; the embedded UI renders those rows with in-app task inspection/log handoffs.
- ECS task payloads add `ecsTask`, including task ARN/id/status, launch/health/stopped context, task definition ARN, and container rows with optional `awslogs.group` / `awslogs.stream` metadata; bookmarked `/?page=resource&resourceType=ecsTask...` pages render this evidence directly instead of falling through to generic empty live-inspection copy
- Auto Scaling payloads add `autoScalingGroup`, including optional `launchTemplate` and `mixedInstancesPolicy` metadata when `DescribeAutoScalingGroups` returns launch-template details plus additive observed launch-template runtime fields such as `launchTemplate.imageId` and `launchTemplate.instanceType` when the live EC2 launch-template lookup resolves them
- EC2 bootstrap-source payloads add `userDataBootstrapSource`, including `desiredBootstrapSource`, optional `observedBootstrapSource`, the backing Auto Scaling resource identity, and the observed launch template summary Tenet used for that comparison, including additive `launchTemplate.imageId` and `launchTemplate.instanceType` when live EC2 launch-template data is available
- traffic-routing payloads add `trafficRoutingResource`; when live weighted or CodeDeploy task-set target-group metadata is present, the embedded UI reads it from `trafficRoutingResource.targetGroup` (or compatible `targetGroupInspection` / `loadBalancerTargetGroup` shapes) and renders the target group name/ARN, protocol/port, target type, health-check settings, load-balancer ARNs, target-health counts, and registered target rows without requiring operators to open raw JSON. Live production listener-rule payloads add `trafficRoutingResource.listenerRule` with `ruleArn`, `priority`, `isDefault`, `conditions`, `actions`; the embedded UI renders those fields as rule identity, condition values, action order/type, and weighted target-group ARNs so operators can verify the active traffic split without opening raw JSON.

Traffic-routing inspection stays local-friendly without changing the route shape:
- `trafficShiftPlan` remains `summary_only` because it is a controller-authored rollout summary rather than a direct live AWS resource. `productionListenerRule` keeps the same summary-only local/no-AWS fallback, and can upgrade to `inspectionMode=live` with additive `listenerRule` metadata when the resource name is an ELB listener-rule ARN and AWS access succeeds.
- `activeTargetGroup`, `candidateTargetGroup`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` still render summary-only local/no-AWS payloads without empty live-field placeholders, and upgrade in place when the backend returns `inspectionMode=live` plus `targetGroup` metadata. For example:

  ```http
  GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=activeTargetGroup&resourceName=payments-api-prod-active
  ```

  ```json
  {
    "deploymentName": "payments-api-prod",
    "resourceType": "activeTargetGroup",
    "resourceName": "payments-api-prod-active",
    "available": true,
    "source": "traffic_routing_aws",
    "trafficRoutingResource": {
      "kind": "activeTargetGroup",
      "displayName": "Active target group",
      "inspectionMode": "live",
      "targetGroup": {
        "targetGroupName": "payments-api-prod-active",
        "targetGroupArn": "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-api-prod-active/abc123",
        "protocol": "HTTP",
        "port": 8080,
        "targetType": "instance",
        "targetHealthCounts": {"healthy": 3}
      }
    }
  }
  ```

  The candidate path uses the same schema with `resourceType=candidateTargetGroup` and `resourceName={candidate-target-group-name}`. A listener-rule ARN can also upgrade the weighted production-listener-rule resource to live metadata without changing the route family:

  ```http
  GET /v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=productionListenerRule&resourceName=arn%3Aaws%3Aelasticloadbalancing%3Aap-northeast-2%3A123456789012%3Alistener-rule%2Fapp%2Fpayments%2F50dc6c495c0c9188%2Fabcdef0123456789%2F1234567890abcdef
  ```

  ```json
  {
    "deploymentName": "payments-api-prod",
    "resourceType": "productionListenerRule",
    "available": true,
    "source": "traffic_routing_aws",
    "trafficRoutingResource": {
      "kind": "productionListenerRule",
      "displayName": "Production listener rule",
      "inspectionMode": "live",
      "summary": "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
      "listenerRule": {
        "ruleArn": "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:listener-rule/app/payments/50dc6c495c0c9188/abcdef0123456789/1234567890abcdef",
        "priority": "10",
        "isDefault": false,
        "conditions": [{"field": "path-pattern", "values": ["/api/*"]}],
        "actions": [{"type": "forward", "order": 1, "targetGroups": [{"targetGroupArn": "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-active/abc123", "weight": 80}, {"targetGroupArn": "arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:targetgroup/payments-candidate/def456", "weight": 20}]}]
      }
    }
  }
  ```
- `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, and `testListener` stay `summary_only` when AWS inspection is disabled, and switch to `inspectionMode=live` with additive `codeDeployApplication`, `codeDeployDeploymentGroup`, or `listener` details when live AWS access succeeds; `productionListenerRule` follows the same local fallback pattern and can switch to additive `listenerRule` details when the selected resource name is an ELB listener-rule ARN
- AWS-side traffic-routing failures still return `available=false` with an explicit `unavailableReason` and `lookupContext`

Like recent logs, `lookupContext` is present for both `available=true` and unavailable inspection responses:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

Unsupported or temporarily unavailable resource types do not surface vague 500s from the provider layer. They return `available=false` with an explicit `unavailableReason` plus `lookupContext` so operators can see which region/account path Tenet attempted before continuing AWS troubleshooting elsewhere.

The same drill-down also includes read-only workspace surfaces:
- `/?page=drift&deployment={deploymentName}` for field-level troubleshooting context that now calls out the active drift-row source: controller-authored `managedDrift[]` rows when present, otherwise the existing derived fallback read-model heuristics for older payloads. Controller-authored rows can now also include live-backed EC2 Auto Scaling Group capacity evidence under stable field names such as `runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max` for `asgWeightedTargetGroups` and `asgInstanceRefresh` deployments when live AWS inspection succeeds, with optional `resourceRef` metadata naming the inspected `candidateAutoScalingGroup` or `serviceAutoScalingGroup`. Those same EC2 inspections can now also add `runtimeContext.launchTemplate.bootstrapSource`, `runtimeContext.launchTemplate.imageId`, and `runtimeContext.launchTemplate.instanceType` by comparing the normalized desired deployment bootstrap source plus first-stack-region `ami_id` / `instance_type` intent against the observed primary launch-template values without adding a separate route family, and those live-backed launch-template rows carry the same Auto Scaling Group `resourceRef`. ECS `ecsCodeDeploy` and `ecsRolling` deployments can also add live-backed ECS service rows under `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, and `runtimeContext.taskDefinition.image` when read-only ECS inspection resolves the service desired count, deployment configuration, and a single task-definition container image; those live-backed ECS rows can include `resourceRef` metadata naming the inspected `ecsService`. Traffic-routing inspections can add optional high-severity rows such as `runtimeContext.trafficRouting.weightedTargetGroups`, `runtimeContext.trafficRouting.codeDeployTargetGroups`, or `runtimeContext.trafficRouting.{blueTaskSetTargetGroup,greenTaskSetTargetGroup}.healthyTargets` only when an existing read-only traffic-routing inspection returns `inspectionMode=live`; `summary_only`, unavailable, or AWS-missing traffic-routing payloads stay quiet instead of fabricating rows from static rollout summaries. Older payloads, local runs, or unavailable/missing AWS access still fall back cleanly without fabricating controller drift rows. Focused `go test ./internal/api` coverage locks the live-backed EC2 launch-template drift rows for `runtimeContext.launchTemplate.imageId` and `runtimeContext.launchTemplate.instanceType` because local smoke may legitimately lack AWS access and therefore does not claim to prove those rows. Empty-state copy also distinguishes "no controller-authored drift reported" from "no derived fallback drift detected." When ... [truncated]
- the drift workspace also includes a compact summary above the field-level table with total rows, visible rows after the current filters, and visible `high` / `medium` / `low` counts so operators can see severity distribution and filter impact at a glance without confusing filtered counts for the total baseline
- when a controller-authored field-level drift row carries `resourceRef`, the table renders an `_self` `Open resource evidence` action that resolves the referenced resource through the same normalized application/resource inventory and opens the existing bookmarkable `/?page=resource&deployment={deploymentName}&resourceType={resourceType}&resourceId={resourceId}` evidence page; rows without metadata keep the no-handoff fallback cell
- the drift workspace exposes an explicit `Clear drift filters` control that resets the drift query, severity, and sort controls back to their defaults in one step without changing the bookmarkable route or re-fetching deployment detail
- the drift workspace quick-navigation strip now also exposes a hash-aware `Current section` group plus an `On this page` group with `_self` fragment links for `Runtime signals`, `Rollout execution`, `Field-level drift`, and `Status conditions`, preserving the current bookmarkable pathname/query while appending drift section hashes such as `/?page=drift&deployment={deploymentName}#drift-field-level`
- when one of those drift section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- the drift workspace quick-navigation strip now also exposes direct `_self` resource drill-down links built from the same grouped normalized resource inventory used by the application/resource pages, and renders explicit `No linked resources are available in this drift workspace yet.` copy when that inventory is empty
- `/?page=application&deployment={deploymentName}`, `/?page=resource&deployment={deploymentName}&resourceType={resourceType}&resourceId={resourceId}`, `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render explicit breadcrumb chrome that links prior drill-down levels with `_self` anchors, includes the account step when `awsAccountId` is already known from the loaded detail payload, and keeps the current destination as a non-click chip instead of a self-link
- the bookmarkable `/?page=resource&deployment={name}...` and `/?page=drift&deployment={name}` entry loads both depend on `GET /v1/overview/deployments/{deploymentName}` and now distinguish auth failures on that initial read: `401` tells the operator to reopen the same bookmarkable page through the trusted front door so authenticated identity can be provided, role-gated `403` points at `#operator-context-access-details-panel` / `Show access details` for role provenance, `roles.canRead`, and forwarded `X-Tenet-Authenticated-Roles`, and deployment-access `403` points at `deploymentAccess.summary`, access verdicts, stale/unmatched grants, and the matching deployment, project, or environment scope
- `/?page=drift&deployment={deploymentName}`, `/?page=plan&deployment={deploymentName}`, and deployment-scoped `/?page=audit&deployment={deploymentName}` now render the same embedded quick-navigation strip with a `Hierarchy` group for `_self` `Applications overview` and `Back to application`, a `Current destination` group that keeps the active workspace as a non-click chip, and a `Deployment workspaces` group for the remaining sibling workspaces so operators can move laterally between read-only workspaces without relying on browser back or leaving the bookmarkable shell
- the plan/apply workspace quick-navigation strip now also exposes a hash-aware `Current section` group plus an `On this page` group with `_self` fragment links only for the plan/apply sections currently rendered, using stable ids such as `plan-runtime-signals`, `plan-preview-actions`, `plan-stored-preview`, `plan-stored-apply`, `plan-apply-operations`, and `plan-operations` while preserving the current bookmarkable pathname/query and appending hashes such as `/?page=plan&deployment={deploymentName}&planId={planId}&applyId={applyId}#plan-apply-operations`
- when one of those plan/apply section hashes is active, the current section becomes a non-click chip, disappears from the remaining `On this page` links, and otherwise falls back to the explicit empty state `No section is currently targeted.`
- `/?page=plan&deployment={deploymentName}` for immutable plan previews, `Apply Plan` activation once a persisted ready preview is loaded, bookmarkable `planId`/`applyId` workspace URLs, an explicit `Refresh plan workspace` control that re-runs `GET /v1/overview/deployments/{deploymentName}` in place without changing those bookmarkable query params, explicit `Refresh plan preview` / `Refresh apply request` controls that re-run the current `GET /v1/plans/{planId}` and `GET /v1/applies/{applyId}` reads in place, stored-preview quick links to both the embedded workspace (`_self`) and immutable `Plan JSON` / `Apply JSON` read models, lifecycle-aware apply status copy/chrome for queued/running/succeeded/cancelled/failed records, a compact stored-apply lifecycle summary with progress counts from persisted `operations[]`, stored-apply lifecycle timing/progress metrics sourced from `lifecycle.updatedAt` / `startedAt` / `finishedAt` / `currentStep` / `completedSteps` / `totalSteps` when present, a dedicated `Apply operations` table so operators can review the persisted execution sequence directly in the workspace, and explicit operator guidance for `401` / `403` failures on bookmarked `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` loads or refreshes while keeping deployment/plan-derived fallback context visible

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

The embedded `/?page=application&deployment={deploymentName}` drill-down now renders a small `Controller runtime events` timeline from that route with explicit loading, empty, unavailable, and non-auth fetch-error states. This route returns controller-derived runtime events copied from persisted `status.history` plus an additive `managedDriftSummary` with explicit `total`, `high`, `medium`, and `low` counts across those returned history rows (zeroes are present when `events` is empty). When a Redis controller-refresh handoff from an accepted GitHub desired-state push triggers reconciliation, the controller appends a non-secret `controllerRefresh` object to the persisted event with `requestId`, `deliveryId`, `repository`, `ref`, optional `after`, `requestedAt`, `receivedAt`, and `reconciledAt`; the application-page runtime-events timeline renders those fields as a concise `GitHub refresh evidence` cue with a shortened revision, delivery/request IDs, and request/receive/reconcile timestamps when present. Normal ticker reconciles omit that object and render as ordinary runtime events without placeholder GitHub labels, and deleted-ref webhook pushes still publish no refresh handoff and therefore create no runtime event. `make smoke-local` now locks the complementary default path by asserting fixture-empty events stay empty and ticker/default controller events do not include `controllerRefresh`. The application page renders those counts as `Managed drift summary · {total} total · {high} high · {medium} medium · {low} low` above the event list so operators can triage drift severity before opening individual event rows, and shows an in-shell `Open field-level drift` handoff to `/?page=drift&deployment={deploymentName}#drift-field-level` whenever those counts report managed drift so operators can jump directly into the field-level drift workspace. Example payload fragment: `{"deploymentName":"payments-api-prod","events":[{"summary":"Deployment has managed drift","managedDrift":[{"field":"runtimeContext.capacity.desired","severity":"high"}]}],"managedDriftSummary":{"total":1,"high":1,"medium":0,"low":0}}`. It is intentionally operator-facing and refresh-based, not a full durable audit system yet, and the UI labels it separately from any future audit/auth history work. The same application page now also exposes a compact `Quick navigation` strip with a hash-aware `Current section` group plus an `On this page` group for only the currently rendered non-empty linked-resource sections and `Controller runtime events`, so operators can reopen anchors such as `/?page=application&deployment={deploymentName}#application-runtime-events` without changing the fetch contract or leaving the embedded shell. Empty zero-resource groups are suppressed on the page and in quick navigation; if an application truly has no linked resources yet, the page shows the explicit operator-facing empty state `No linked resources are currently reported for this application. Use controller runtime events below to inspect the latest rollout signals.` while still keeping the runtime-events section available. When one of those section hashes is active, Tenet renders it as a non-click current-section chip, removes it from the remaining `On this page` links, and otherwise shows the friendly empty state `No section is currently targeted.`. The section header now includes an in-place `Refresh runtime events` control that re-fetches just `GET /v1/overview/deployments/{deploymentName}/events` in place, keeps the bookmarkable application route stable, and disables only while that events fetch is active. When that refresh returns `401` or `403`, the panel now shows the same trusted-front-door / access-denied guidance used on the other embedded read surfaces instead of generic fetch-error text. When the aligned runtime change label is `managedDrift`, the paired `changeDetails[]` text now stays single-slot but explains deterministic added, removed, and updated drift rows so operators can see what changed without diffing the whole `managedDrift[]` array by hand. When a deployment exists but no controller-authored runtime history has been persisted, the route still returns `200 OK` with `events: []`, so the UI can keep the page stable and explicit during early runtime bootstrap.

When fixture-backed or early controller payloads have not yet persisted `rollout.execution[]`, Tenet keeps the read-only drill-down contract stable by synthesizing the same fallback rollout resources the operator would otherwise see from the controller-authored payload:
- `ecsRolling`: `ecsService` (`{deployment}-service`)

For the ECS rolling fallback service, the recent-log drill-down stays on the same operator workflow. Example route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`

When a service inspection identifies a concrete ECS task, operators and UI consumers can reuse the same route families with `resourceType=ecsTask`. The task resource name may be an ECS task ARN, and recent-log reads accept optional `containerName` to narrow the CloudWatch Logs lookup to a single container stream:
- `GET /v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123`
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&containerName=api&limit=5`

The ECS task inspection payload keeps the same explicit envelope as other resource-inspection reads and adds an `ecsTask` object, for example:

```json
{
  "deploymentName": "worker-prod",
  "resourceType": "ecsTask",
  "resourceName": "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123",
  "available": true,
  "source": "ecs_service",
  "ecsTask": {
    "taskArn": "arn:aws:ecs:ap-northeast-2:123456789012:task/worker/abc123",
    "taskId": "abc123",
    "lastStatus": "STOPPED",
    "stoppedReason": "Essential container in task exited",
    "containers": [{"name": "api", "lastStatus": "STOPPED", "awslogs": {"group": "/ecs/worker", "stream": "ecs/api/abc123"}}]
  }
}
```

Operators can add an optional positive `limit` query parameter when they need a smaller, shareable recent-log envelope; Tenet applies it after the backend lookup while preserving the provider's newest-first order and returns `400 Bad Request` for non-positive or non-numeric values. Successful responses include additive `resultScope` metadata with the optional `requestedLimit`, the `effectiveLimit`, `returnedEntryCount`, and conservative `truncationStatus` (`truncated`, `not_truncated`, `unknown`, or `not_applicable`) so operators can tell which bounded read Tenet actually returned. Tenet does not issue extra provider calls solely to prove exact totals beyond the bounded read. Example limited route:
- `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=5`

Resource-log responses now include an additive `lookupContext` object so the payload stays self-describing during AWS troubleshooting:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

When a requested resource resolves through a backing resource, the same response can also include additive backing-resource identity:
- `backingResourceType`
- `backingResourceName`

`lookupContext` is present for both `available=true` and unavailable responses, so operators can still confirm the attempted lookup region/account path when `entries` is empty or `unavailableReason` is set. When a bootstrap-source request resolved through a backing Auto Scaling Group before the downstream lookup ran, `backingResourceType` and `backingResourceName` remain present on both success and explicit unavailable responses. Weighted-target-group traffic-routing resources such as `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup` also preserve the same route and lookup-context envelope but now return `available=false` with `unavailableReason=no_log_source` because they do not have an independent log stream yet. See `docs/features/resource-recent-logs.md` for operator troubleshooting examples that distinguish bounded successful reads, AWS-unavailable supported-resource lookups, and route-stable no-log-source resources. The embedded UI renders the lookup-context fields directly as `Lookup region`, `Control account ID`, `Target account ID`, `Cross-account lookup`, and `Observer role configured` inside the `Recent logs` card, and when those additive backing-resource fields are present it also renders them there as `Backing resource type` and `Backing resource name`.

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
- trusted front-door auth plus optional Tenet-managed GitHub login groundwork

Not implemented yet:
- live AWS execution
- Okta SAML activation and repository-backed provider group authorization
- repository-backed RBAC policy evaluation for users/groups
- full plan/apply workflows
- a dedicated standalone frontend

## Documentation

For deeper design details, see:
- `docs/architecture/tenet-v1-overview.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-overview-ui.md`
- `docs/architecture/tenet-v1-enterprise-sso.md`
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
