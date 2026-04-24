# Tenet v1 Local Runtime

The local runtime packages the current read-only control-plane foundation as a Docker-friendly stack.

## What is implemented in this slice

- `tenet-api` serves the catalog and deployment-status read APIs.
- `tenet-controller` validates the repository catalog at startup, then reloads the repo and status snapshots on a timer to log abstract reconcile plans per deployment.
- a sample repository-shaped desired-state tree lives under `examples/local-repo/`, including YAML deployment manifests that can carry `binary` or `image` artifact intent.
- a sample filesystem-backed deployment-status file lives under `examples/local-status/deployments.json`, including the currently deployed binary/image metadata.
- `compose.yaml` wires both processes together so the repo can be explored locally without AWS credentials.

## Runtime contract

The local stack uses these inputs:

- `TENET_CATALOG_SOURCE=filesystem`
- `TENET_REPO_ROOT=/workspace/examples/local-repo`
- `TENET_STATUS_PATH=/workspace/examples/local-status/deployments.json`
- `TENET_PLAN_STORE_PATH=/tmp/tenet/plan-previews.json`
- `TENET_APPLY_STORE_PATH=/tmp/tenet/apply-requests.json`
- `TENET_CONTROLLER_REQUEUE_SECONDS=5` in the local Compose demo so controller planning logs refresh quickly
- `TENET_AWS_ENABLE_APPLY=false` by default in the local Compose demo; set it to `true` when you want queued-on-create immutable apply records plus a local-safe executor handoff on the same `applyId` without introducing AWS mutation

Both services mount `./examples` read-only. The `api` service also mounts a named volume on `/tmp`, which keeps `/tmp/tenet/plan-previews.json` and `/tmp/tenet/apply-requests.json` restart-safe so immutable plan/apply records survive the Compose `api` service restart used by `make smoke-docker`. That keeps the runtime deterministic and aligned with the current repo-backed architecture.

For `tenet-api`, the local runtime also enables request-scoped JSON logs for every API/UI request. Operators can rely on the stable fields `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`, plus an `error` field on timeout/panic paths. `TENET_API_REQUEST_TIMEOUT` defaults to `30s`; set it to `0` to disable the timeout wrapper for local debugging.

## Trusted front-door auth

Front-door auth remains additive and opt-in. Tenet still expects a real authentication layer to run before requests reach the API.

Use these settings only when every request reaches Tenet through a front door that authenticates callers and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, and `X-Tenet-Authorized-Deployments` headers before forwarding trusted values. Do not expose a header-trusting `tenet-api` instance directly to untrusted clients.

- `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` trusts front-door-forwarded `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, and `X-Tenet-Authenticated-Email`. When those explicit trusted fields are absent, the same trusted authenticated-identity path also accepts additive `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases, and `X-Tenet-Authenticated-*` still wins over `X-User-*`.
- `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true` requires authenticated identity on `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions`. Unauthenticated requests fail with `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`. When explicit actor headers are absent, the trusted authenticated identity path projects that identity into persisted plan/apply/audit actor fields.
- `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` trusts `X-Tenet-Authenticated-Roles`, normalizes comma-separated roles, and requires one operator-capable `operator` or `admin` role on those same operator write routes. Authenticated callers without one of those roles get `403 Forbidden` plus `forbidden: authenticated operator role is required for this operator write route` before any plan/apply lookup.
- `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` trusts `X-Tenet-Authorized-Deployments` as a comma-separated exact deployment allowlist. That same allowlist gates `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions`, including the persisted `POST /v1/applies/{applyId}/transitions` write route, and it also filters `GET /v1/overview/applications`, `GET /v1/status/deployments`, `GET /v1/overview/accounts`, and unscoped `GET /v1/audit-logs` down to authorized deployments. Missing or non-matching allowlists return `200 OK` with `[]` or `{"entries":[]}` on those collection routes, while scoped reads still require exact deployment membership and return `401 Unauthorized` without trusted identity or `403 Forbidden` when the recognized operator is not authorized.
- deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments` when read authorization is enabled.
- The same trusted front-door read contract also covers `GET /v1/overview/accounts/{awsAccountId}`, `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, `GET /v1/overview/applications/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}`, `GET /v1/overview/deployments/{deploymentName}/events`, `GET /v1/overview/deployments/{deploymentName}/resource-logs`, `GET /v1/overview/deployments/{deploymentName}/resource-inspection`, `GET /v1/status/deployments/{deploymentName}`, deployment-scoped `GET /v1/audit-logs`, and immutable stored reads `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` using the persisted record deployment.

## Developer flows

### Local smoke check without Docker

```bash
make smoke-local
```

Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation. It boots `tenet-api` against the committed example fixtures, checks `/healthz`, validates representative JSON payloads from the catalog and status endpoints, proves the executor-facing `POST /v1/applies/{applyId}/transitions` boundary on the smoke-created terminal apply record (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), and proves additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities on one `payments-api-staging` plan/apply flow and verifying the explicit actor survives the stored plan preview, stored apply request, and deployment-scoped audit lookup. It also fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route, the bookmarkable `/?page=account&accountId=123456789012` route, the existing bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, plus one representative bookmarkable ECS resource route at `/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService::worker-prod-service::ECS service Tenet updates with native rolling deployment`, and asserts only the raw embedded application/account/resource/drift/audit/plan shell strings needed to prove those drill-down pages can render and navigate. It then restarts `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS` plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY` to prove unauthenticated operator writes fail with `401` and trusted `X-Tenet-Authenticated-*` headers project into persisted plan/apply/audit actor fields when explicit actor headers are absent, including the persisted `POST /v1/applies/{applyId}/transitions` write route. It restarts again with `TENET_API_TRUST_AUTHENTICATED_HEADERS`, `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION`, and `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` so guarded read collections reject missing identity with `401`, reject authenticated callers without a read-capable trusted `viewer` / `operator` / `admin` role with `403`, keep deployment-scoped reads on the existing deployment-access `403`, and continue returning `[]` or `{"entries":[]}` for filtered-empty guarded collections.

Additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities remains part of that direct-process smoke surface.

Bookmarkable `/?page=application&deployment=payments-api-prod`, `/?page=account&accountId=123456789012`, the existing traffic-shift resource route, and one representative ECS resource route remain part of the same shell-level smoke contract.

Trusted front-door auth by restarting `tenet-api` remains part of the same smoke contract.

`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` gates guarded application, raw-status collection, account collection, unscoped audit collection, raw-status detail, account detail, grouped-account, immutable plan/apply, deployment-scoped audit reads, plus the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` drill-down routes.

Local smoke still does not claim to prove the new `userDataBootstrapSource` live-inspection payload because that payload depends on the same read-only launch-template bootstrap-source evidence Tenet uses for managed drift and may legitimately be unavailable without AWS access; focused service/router/UI tests lock that additive contract instead.

The same guarded local smoke now also proves `GET /v1/overview/deployments/{deploymentName}/events` follows the deployment-scoped `401` / `403` / `200 OK` contract and keeps the existing `deploymentName` plus `events` list shape for authorized reads.

Neither local smoke path claims to prove live-backed `managedDrift[]` rows under `runtimeContext.capacity.{desired,min,max}`, `runtimeContext.launchTemplate.bootstrapSource`, `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, or `runtimeContext.taskDefinition.image`. Local runs may legitimately lack AWS access, so focused `go test ./internal/api` contract coverage locks that additive JSON/UI detail instead.

This local smoke path now sets `TENET_CATALOG_SOURCE=filesystem` explicitly so the fixture-backed contract is unambiguous. `tenet-controller` now honors the same `TENET_CATALOG_SOURCE=filesystem|github` contract as `tenet-api` for desired-state reads, but the local stack keeps the mounted checkout path because that is still the simplest deterministic demo shape.

### Manual `tenet-api` run without Docker

For manual `go run ./cmd/tenet-api` flows, `tenet-api` listens on `TENET_API_ADDR`, not `TENET_API_PORT`. Use `TENET_API_ADDR=:18083` as a safe local example before opening the UI so you do not collide with the default Docker stack on `localhost:8080`.

```bash
TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
TENET_API_ADDR=:18083 \
go run ./cmd/tenet-api
```

Then open:

```bash
http://localhost:18083/
```

### Docker stack smoke check

```bash
make smoke-docker
```

Use `make smoke-docker` when you need to validate the containerized local stack itself.

It verifies:
- `docker compose config` plus the `api`, `controller`, and `redis` services present in `compose.yaml`.
- API health, representative endpoints, deployment-events/resource-logs/audit-log JSON shapes, and representative `Request-Id` response headers.
- The full `GET /v1/audit-logs` collection, exact-action filtering with `GET /v1/audit-logs?action=apply.requested&limit=2`, and `400 Bad Request` on `action=invalid.action`.
- Non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`.
- Immutable `payments-api-prod` plan/apply creation and reload through `POST /v1/plans/deployments/payments-api-prod`, `GET /v1/plans/{planId}`, `POST /v1/applies/plans/{planId}`, and `GET /v1/applies/{applyId}`.
- The same direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval.
- The same smoke-created terminal `applyId` transition boundaries: `400` malformed JSON, `404` unknown apply, and `409` invalid terminal move with operator-facing text.
- Bookmarkable application, account, drift, audit, and plan workspace URLs, checking raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present on those bookmarkable routes. In that compose-backed verification surface, the bookmarkable application shell still exposes the `_self` `Applications overview` return path plus the non-click `Application detail` destination chip.
- Drift page source/summary/table copy, audit filter copy such as `Audit filters: action apply.requested · limit 2`, the `Clear audit filters` recovery affordance, and correlated `http_request` logs for the bookmarkable application, account, drift, audit, and plan/apply shell requests.
- Trusted front-door auth by restarting the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION`, proving unauthenticated guarded reads fail with `401`, trusted `X-Tenet-Authorized-Deployments` filters returned application rows, raw status snapshots, account aggregates, and unscoped audit entries, unauthenticated `POST /v1/applies/{applyId}/transitions` fails with `401`, authenticated but unauthorized transitions fail with `403`, and authorized resource drill-down reads still preserve `available=false` plus `unavailableReason` when live lookup data is unavailable.
- The opt-in `TENET_AWS_ENABLE_APPLY=true` queued-on-create immutable apply contract on a fresh `worker-prod` plan/apply pair, keeping the earlier disabled `payments-api-prod` record unchanged and still avoiding AWS mutation.

Omitted allowlists still return `[]` for guarded collections and `{"entries":[]}` for the unscoped audit collection. `GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`, and one allowed raw status detail read succeeds, and the bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures, so use it only behind a front door that authenticates callers and strips inbound trust headers before forwarding its own values.

The bookmarkable `/?page=plan` workspace, but only behind a trusted front door that strips inbound trust headers before forwarding its own values, remains part of the documented guarded-read contract and operator-facing read surface.

That compose smoke surface now also fetches one representative bookmarkable bootstrap-source resource page plus the paired `userDataBootstrapSource` `resource-logs` and `resource-inspection` routes, keeping those assertions file-backed and treating explicit `aws_unavailable` / `no_log_source` unavailable payloads as expected local-runtime outcomes instead of assuming live AWS-backed success.

GitHub Actions uses the same split so contributors and operators can read CI failures in the same terms as local reproduction. `.github/workflows/ci-smoke.yml` runs a fast lane on every pull request plus pushes to `main`; it pins the Go toolchain from `go.mod`, runs `go test ./...`, syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`. A separate slower Docker lane runs only on pushes to `main` and manual `workflow_dispatch` runs, syntax-checks `scripts/smoke-docker.sh` with `bash -n`, then runs `make smoke-docker`, which keeps the PR path fast while still exercising the full containerized stack in automation.

Both smoke scripts intentionally persist intermediate deployment-detail, plan-preview, and apply-record JSON to temp files through the shared `write_payload` helper before Python reads them back. The compose smoke now uses that same file-backed pattern for one representative bookmarkable bootstrap-source resource page plus its paired `resource-logs` and `resource-inspection` payloads, and accepts explicit `aws_unavailable` / `no_log_source` unavailable outcomes instead of requiring live AWS-backed success. Keeping those payloads file-backed avoids large environment-variable blobs and stays portable across Linux CI runners with tighter environment-size limits.

The same smoke flow also verifies that representative responses carry a `Request-Id` header and that the local API process emits structured `http_request` logs, including correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace. The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured` present so bookmarkable operator drill-down pages stay aligned with the compose-backed verification surface. For the read-role gate specifically, the documented trusted read-capable roles remain `viewer` / `operator` / `admin`. Operators should capture that response ID when an API call fails; Tenet threads the same value through the request context, echoes it back in explicit timeout/panic responses, and logs it on both request-completion and panic-recovery paths so API failures are easy to correlate with backend/provider diagnostics.

The same smoke flow also verifies correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request.

The guarded Compose smoke also now locks `GET /v1/overview/deployments/{deploymentName}/events` to the same deployment-scoped `401` / `403` / `200 OK` contract as the other drill-down routes while preserving the existing `deploymentName` plus `events` list shape for authorized reads.

### Full Docker stack

```bash
make docker-up
curl http://localhost:8080/v1/catalog/summary
docker compose logs controller --tail=20
make docker-down
```

The `make docker-up` preflight intentionally fails fast when `localhost:8080` is already occupied by something other than this Compose stack. When that happens, inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`, clear a prior Tenet stack with `make docker-down` if needed, then rerun `docker compose up --build -d`.

Leave `TENET_AWS_ENABLE_APPLY` unset for the default disabled immutable apply contract. Set `TENET_AWS_ENABLE_APPLY=true make docker-up` when you want the same local stack to create queued immutable apply records on a fresh plan/apply pair and then let the local-safe executor advance that new `applyId`; that opt-in remains read-only and does not contact AWS.

## Deferred work

This slice deliberately does not introduce live AWS reconciliation yet. The next layers still needed are:

- controller-owned reconciliation loops based on the abstract planner
- durable runtime history beyond a single status file
- write APIs
- AWS observation/execution adapters


The smoke helper still runs `docker compose down --remove-orphans` even on failure so local cleanup stays deterministic.
