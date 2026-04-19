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
- `TENET_CONTROLLER_REQUEUE_SECONDS=5` in the local Compose demo so controller planning logs refresh quickly
- `TENET_AWS_ENABLE_APPLY=false` by default in the local Compose demo; set it to `true` when you want queued-on-create immutable apply records plus a local-safe executor handoff on the same `applyId` without introducing AWS mutation

Both services mount `./examples` read-only. That keeps the runtime deterministic and aligned with the current repo-backed architecture.

For `tenet-api`, the local runtime also enables request-scoped JSON logs for every API/UI request. Operators can rely on the stable fields `request_id`, `method`, `path`, `status`, `duration_ms`, and `result`, plus an `error` field on timeout/panic paths. `TENET_API_REQUEST_TIMEOUT` defaults to `30s`; set it to `0` to disable the timeout wrapper for local debugging.

## Developer flows

### Local smoke check without Docker

```bash
make smoke-local
```

Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation. It boots `tenet-api` against the committed example fixtures, checks `/healthz`, validates representative JSON payloads from the catalog and status endpoints, proves the executor-facing `POST /v1/applies/{applyId}/transitions` boundary on the smoke-created terminal apply record (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), proves additive `X-User-*` alias support plus explicit `X-Tenet-Actor-*` precedence by sending both header families with conflicting identities on one `payments-api-staging` plan/apply flow and verifying the explicit actor survives the stored plan preview, stored apply request, and deployment-scoped audit lookup, fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route plus one representative bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, and asserts only the raw embedded application/resource shell strings needed to prove those drill-down pages can render and navigate, restarts `tenet-api` with `TENET_API_TRUST_AUTHENTICATED_HEADERS=*** plus `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=*** to prove unauthenticated operator writes fail with `401` and trusted `X-Tenet-Authenticated-*` headers project into persisted plan/apply/audit actor fields when explicit actor headers are absent, then restarts again with `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=*** to prove deployment-scoped reads enforce trusted `X-Tenet-Authenticated-*` identity plus `X-Tenet-Authorized-Deployments`, including one allowed guarded read, one denied guarded read, guarded immutable `GET /v1/plans/{planId}` / `GET /v1/applies/{applyId}` reloads, and one deployment-scoped audit read, while unscoped reads stay unchanged. The bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures. In other words, the bookmarkable `/?page=plan` workspace still includes operator-facing guidance for those auth-gated write failures. Do it only behind a front door that authenticates callers and strips inbound `X-Tenet-Authenticated-*` headers. To be explicit, do it only behind a front door that authenticates callers and strips inbound `X-Tenet-Authenticated-*` headers. The local read-route allowlist follows the same rule and strips inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments`.

Neither local smoke path claims to prove live-backed `managedDrift[]` rows under `runtimeContext.capacity.{desired,min,max}`, `runtimeContext.capacity.desired`, or `runtimeContext.taskDefinition.image`. Local runs may legitimately lack AWS access, so focused `go test ./internal/api` contract coverage locks that additive JSON/UI detail instead.

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

Use `make smoke-docker` when you need to validate the containerized local stack itself. The script runs `docker compose config`, asserts the `api`, `controller`, and `redis` services are present in `compose.yaml`, starts the stack, waits for API health, checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, validates non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the default disabled immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, mirrors the direct-process actor coverage by sending both header families with conflicting identities on a `payments-api-staging` plan/apply flow and verifying that the explicit `X-Tenet-Actor-*` actor survives the stored plan preview, stored apply request, and deployment-scoped `GET /v1/audit-logs?deployment=payments-api-staging&action=apply.requested&limit=1` retrieval, reuses that smoke-created terminal `applyId` to verify the executor-facing transition-route boundaries (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), fetches the bookmarkable `/?page=application&deployment=payments-api-prod` route plus one representative bookmarkable `/?page=resource&deployment=payments-api-prod&resourceType=trafficShiftPlan&resourceId=trafficShiftPlan::payments-api-prod-weighted-routing::Weighted routing plan` route, fetches the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace URL plus the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit workspace URL and the bookmarkable `/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}` workspace URL, checks raw embedded application/resource/drift/audit shell HTML/script fragments instead of runtime DOM text so the shipped operator read-auth guidance hooks for initial `401` / `403` failures remain present on those bookmarkable routes, confirms the drift page source still includes `Active drift source`, `Current source: controller-authored managedDrift rows.`, `Drift summary`, and field-level table headers such as `Field path` / `Desired` / `Actual` / `Severity`, confirms the audit page preserves active filter copy such as `Audit filters: action apply.requested · limit 2`, confirms the `api` container emits correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable drift workspace request and the deployment-scoped audit workspace request, restarts the Compose `api` service with `TENET_API_TRUST_AUTHENTICATED_HEADERS=true` plus `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` to prove unauthenticated `GET /v1/overview/applications` fails with `401`, trusted `X-Tenet-Authorized-Deployments` filters the returned application rows, and omitting that allowlist returns `[]`, then restarts the Compose `api` service with `TENET_AWS_ENABLE_APPLY=true` to verify the opt-in queued-on-create immutable apply contract and the follow-up local-safe executor handoff to failed on the same `applyId` while still avoiding AWS mutation, confirms the embedded UI still includes current drill-down labels such as `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured`, confirms controller logs contain useful startup/reconcile output, and then runs `docker compose down --remove-orphans` even on failure.

GitHub Actions uses the same split so contributors and operators can read CI failures in the same terms as local reproduction. `.github/workflows/ci-smoke.yml` runs a fast lane on every pull request plus pushes to `main`; it pins the Go toolchain from `go.mod`, then runs `go test ./...` and `make smoke-local`. A separate slower Docker lane runs `make smoke-docker` only on pushes to `main` and manual `workflow_dispatch` runs, which keeps the PR path fast while still exercising the full containerized stack in automation.

Both smoke scripts intentionally persist intermediate deployment-detail, plan-preview, and apply-record JSON to temp files through the shared `write_payload` helper before Python reads them back. Keeping those payloads file-backed avoids large environment-variable blobs and stays portable across Linux CI runners with tighter environment-size limits.

The same smoke flow also verifies that representative responses carry a `Request-Id` header and that the local API process emits structured request logs. Operators should capture that response ID when an API call fails; Tenet threads the same value through the request context, echoes it back in explicit timeout/panic responses, and logs it on both request-completion and panic-recovery paths so API failures are easy to correlate with backend/provider diagnostics.

### Full Docker stack

```bash
make docker-up
curl http://localhost:8080/v1/catalog/summary
docker compose logs controller --tail=20
make docker-down
```

The `make docker-up` preflight intentionally fails fast when `localhost:8080` is already occupied by something other than this Compose stack. When that happens, inspect the listener with `lsof -nP -iTCP:8080 -sTCP:LISTEN`, clear a prior Tenet stack with `make docker-down` if needed, then rerun `docker compose up --build -d`.

Leave `TENET_AWS_ENABLE_APPLY` unset for the default disabled immutable apply contract. Set `TENET_AWS_ENABLE_APPLY=true make docker-up` when you want the same local stack to create queued immutable apply records and then let the local-safe executor advance the same `applyId`; that opt-in remains read-only and does not contact AWS.

## Deferred work

This slice deliberately does not introduce live AWS reconciliation yet. The next layers still needed are:

- controller-owned reconciliation loops based on the abstract planner
- durable runtime history beyond a single status file
- write APIs
- AWS observation/execution adapters
