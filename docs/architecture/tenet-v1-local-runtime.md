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

## Developer flows

### Local smoke check without Docker

```bash
make smoke-local
```

Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation. It boots `tenet-api` against the committed example fixtures, checks `/healthz`, validates representative JSON payloads from the catalog and status endpoints, proves the executor-facing `POST /v1/applies/{applyId}/transitions` boundary on the smoke-created terminal apply record (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), proves additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities on one `payments-api-staging` plan/apply flow and verifying the explicit actor survives the stored plan preview, stored apply request, and deployment-scoped audit lookup, fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route, the bookmarkable `/?page=account&accountId=123456789012` route, the existing bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, plus one representative bookmarkable ECS resource route at `/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService::worker-prod-service::ECS service Tenet updates with native rolling deployment`, and asserts only the raw embedded application/account/resource shell strings needed to prove those drill-down pages can render and navigate. It then restarts `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS=***` plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=***` to prove unauthenticated operator writes fail with `401` and trusted `X-Tenet-Authenticated-*` headers project into persisted plan/apply/audit actor fields when explicit actor headers are absent, including the persisted `POST /v1/applies/{applyId}/transitions` write route. The same local runtime can also expose `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=***`, which trusts `X-Tenet-Authenticated-Roles`, normalizes comma-separated forwarded roles, and requires one operator-capable `operator` / `admin` role before those operator write routes proceed; recognized identity without one of those roles returns `forbidden: authenticated operator role is required for this operator write route` before any persisted plan/apply lookup. It then restarts again with `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=***` to prove unauthorized `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` fail with the same explicit `403 Forbidden` deployment-access-denied response, while unauthenticated transition callers still short-circuit with the operator-write `401` before any apply lookup, and that deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments`. That same smoke pass now explicitly covers unscoped `GET /v1/audit-logs?action=apply.requested` for `401` unauthenticated reads, allowlist-filtered `200 OK` reads, and `{"entries":[]}` when authenticated identity exists but no audit rows are authorized, plus `GET /v1/overview/deployments/{deploymentName}/resource-logs` and `GET /v1/overview/deployments/{deploymentName}/resource-inspection` for `401` unauthenticated reads, `403` authenticated-but-unauthorized reads, and `200 OK` authorized reads, while preserving the existing `available=false` plus `unavailableReason` payload contract when live lookup data is unavailable. Under that same trusted front-door auth contract, the bookmarkable application drill-down `/?page=application&deployment=payments-api-prod` and `/?page=account&accountId=123456789012` drill-down both stay on the shared operator-read guidance path alongside `/`, resource, drift, audit, and plan workspace bookmarks, the bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures, and do it only behind a front door that authenticates callers and strips inbound `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, and `X-Tenet-Authorized-Deployments` before forwarding trusted values. The local read-route allowlist follows the same rule and strips inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments`. Local smoke still does not claim to prove the new `userDataBootstrapSource` live-inspection payload because that payload depends on the same read-only launch-template bootstrap-source evidence Tenet uses for managed drift and may legitimately be unavailable without AWS access; focused service/router/UI tests lock that additive contract instead.

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

Use `make smoke-docker` when you need to validate the containerized local stack itself. The script runs `docker compose config`, asserts the `api`, `controller`, and `redis` services are present in `compose.yaml`, starts the stack, waits for API health, checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, validates non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the default disabled immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, mirrors the direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval, reuses that smoke-created terminal `applyId` to verify the executor-facing transition-route boundaries (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route, the bookmarkable `/?page=account&accountId=123456789012` route, the existing bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, plus one representative bookmarkable ECS resource route at `/?page=resource&deployment=worker-prod&resourceType=ecsService&resourceId=ecsService::worker-prod-service::ECS service Tenet updates with native rolling deployment`, fetches the bookmarkable `/?page=application&deployment=payments-api-prod` drill-down shell URL plus the bookmarkable `/?page=account&accountId=123456789012` drill-down shell URL alongside the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace URL plus the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit workspace URL and the bookmarkable `/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}` workspace URL, checks raw embedded application/account/resource/drift/audit/plan shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present on those bookmarkable routes, confirms the drift page source still includes `Active drift source`, `Current source: controller-authored managedDrift rows.`, `Drift summary`, and field-level table headers such as `Field path` / `Desired` / `Actual` / `Severity`, confirms the audit page preserves active filter copy such as `Audit filters: action apply.requested · limit 2`, confirms the `api` container emits correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable application drill-down shell request, bookmarkable account drill-down shell request, bookmarkable drift workspace request, the deployment-scoped audit workspace request, and the bookmarkable plan/apply workspace shell request, restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` to prove unauthenticated `GET /v1/overview/applications`, unauthenticated `GET /v1/status/deployments`, unauthenticated `GET /v1/overview/accounts`, and unauthenticated `GET /v1/audit-logs?action=apply.requested` fail with `401`, and unauthenticated `POST /v1/applies/{applyId}/transitions` fail with `401`, and now also to prove `GET /v1/overview/deployments/{deploymentName}/resource-logs` plus `GET /v1/overview/deployments/{deploymentName}/resource-inspection` fail with `401` unauthenticated and `403` when the allowlist excludes that deployment while allowed reads still preserve `available=false` plus `unavailableReason` when live lookup data is unavailable. Under that same trusted front-door auth slice, the bookmarkable application drill-down shell `/?page=application&deployment=payments-api-prod` keeps the shared operator-read guidance contract in the raw served HTML alongside the landing page, account, resource, drift, and audit shells. The smoke then reloads the same stored `GET /v1/plans/{planId}` and `GET /v1/applies/{applyId}` records through trusted front-door headers so they survive the Compose `api` service restart, proves trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, raw status snapshots, account aggregates, and unscoped audit entries, and finally restarts the Compose `api` service with `TENET_AWS_ENABLE_APPLY=true` to verify the opt-in queued-on-create immutable apply contract on a fresh `worker-prod` plan/apply pair so the earlier disabled `payments-api-prod` record remains unchanged before the local-safe executor handoff marks that new `applyId` failed while still avoiding AWS mutation. When the smoke reenables trusted front-door auth, it also proves authenticated but unauthorized `POST /v1/applies/{applyId}/transitions` fails with `403`, trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, raw status snapshots, and account aggregates, and when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows. Omitted allowlists still return `[]` for those guarded collections and `{"entries":[]}` for the unscoped audit collection, and one allowed raw status detail read plus authorized `GET /v1/overview/deployments/{deploymentName}/resource-logs` / `GET /v1/overview/deployments/{deploymentName}/resource-inspection` reads succeed while preserving their existing unavailable payload semantics when AWS access is absent. In exact contract terms, omitting that allowlist returns `[]` for those guarded collections. `GET /v1/status/deployments/{deploymentName}` outside the allowlist fails with `403`, and one allowed raw status detail read succeeds. The bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures; do it only behind a front door that authenticates callers and strips inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments` before forwarding trusted values.

That compose smoke surface now also fetches one representative bookmarkable bootstrap-source resource page plus the paired `userDataBootstrapSource` `resource-logs` and `resource-inspection` routes, keeping those assertions file-backed and treating explicit `aws_unavailable` / `no_log_source` unavailable payloads as expected local-runtime outcomes instead of assuming live AWS-backed success.

GitHub Actions uses the same split so contributors and operators can read CI failures in the same terms as local reproduction. `.github/workflows/ci-smoke.yml` runs a fast lane on every pull request plus pushes to `main`; it pins the Go toolchain from `go.mod`, runs `go test ./...`, syntax-checks `scripts/smoke-local.sh` with `bash -n`, then runs `make smoke-local`. A separate slower Docker lane runs only on pushes to `main` and manual `workflow_dispatch` runs, syntax-checks `scripts/smoke-docker.sh` with `bash -n`, then runs `make smoke-docker`, which keeps the PR path fast while still exercising the full containerized stack in automation.

Both smoke scripts intentionally persist intermediate deployment-detail, plan-preview, and apply-record JSON to temp files through the shared `write_payload` helper before Python reads them back. The compose smoke now uses that same file-backed pattern for one representative bookmarkable bootstrap-source resource page plus its paired `resource-logs` and `resource-inspection` payloads, and accepts explicit `aws_unavailable` / `no_log_source` unavailable outcomes instead of requiring live AWS-backed success. Keeping those payloads file-backed avoids large environment-variable blobs and stays portable across Linux CI runners with tighter environment-size limits.

The same smoke flow also verifies that representative responses carry a `Request-Id` header and that the local API process emits structured request logs, including correlated bookmarkable shell reads for the application drill-down, account drill-down, drift workspace, deployment-scoped audit workspace, and bookmarkable plan/apply workspace. The raw embedded shell/docs contract also keeps the labels `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured` present so bookmarkable operator drill-down pages stay aligned with the compose-backed verification surface. Operators should capture that response ID when an API call fails; Tenet threads the same value through the request context, echoes it back in explicit timeout/panic responses, and logs it on both request-completion and panic-recovery paths so API failures are easy to correlate with backend/provider diagnostics.

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
