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

Use `make smoke-local` for the fastest fixture-backed regression check when you only need direct-process validation. It boots `tenet-api` against the committed example fixtures, checks `/healthz`, validates representative JSON payloads from the catalog and status endpoints, proves the executor-facing `POST /v1/applies/{applyId}/transitions` boundary on the smoke-created terminal apply record (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), and in enabled apply mode proves that Tenet now creates a queued immutable apply record before the local-safe executor advances the same `applyId` to a failed stub boundary without manual transition API calls.

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

Use `make smoke-docker` when you need to validate the containerized local stack itself. The script runs `docker compose config`, asserts the `api`, `controller`, and `redis` services are present in `compose.yaml`, starts the stack, waits for API health, checks representative endpoints plus deployment-events/resource-logs/audit-log JSON shapes, verifies representative `Request-Id` response headers, fetches the full `GET /v1/audit-logs` collection and asserts the expected `plan.created` and `apply.requested` actions are present without depending on `limit=2`, still fetches `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering, rejects `action=invalid.action` with `400 Bad Request`, validates non-empty controller-authored `managedDrift[]` evidence in `GET /v1/overview/deployments/payments-api-prod` with stable `field` / `desired` / `actual` / `severity` rows and aligned `runtimeSummary.managedDrift`, creates and reloads an immutable plan preview for `payments-api-prod` via `POST /v1/plans/deployments/payments-api-prod` and `GET /v1/plans/{planId}`, creates and reloads the default disabled immutable apply request via `POST /v1/applies/plans/{planId}` and `GET /v1/applies/{applyId}`, reuses that smoke-created terminal `applyId` to verify the executor-facing transition-route boundaries (`400` malformed JSON, `404` unknown apply, `409` invalid terminal move with operator-facing text), fetches the bookmarkable `/?page=drift&deployment=payments-api-prod` workspace URL plus the bookmarkable `/?page=audit&deployment={deploymentName}&action={action}&limit={n}` audit workspace URL and the bookmarkable `/?page=plan&deployment=payments-api-prod&planId={planId}&applyId={applyId}` workspace URL, confirms the drift page source still includes `Active drift source`, `Current source: controller-authored managedDrift rows.`, `Drift summary`, and field-level table headers such as `Field path` / `Desired` / `Actual` / `Severity`, confirms the audit page preserves active filter copy such as `Audit filters: action apply.requested · limit 2`, confirms the `api` container emits correlated structured `http_request` logs with stable fields such as `request_id`, `method`, `path`, `status`, `duration_ms`, and `result` for the bookmarkable drift workspace request and the deployment-scoped audit workspace request, restarts the Compose `api` service with `TENET_AWS_ENABLE_APPLY=true` to verify the opt-in queued-on-create immutable apply contract and the follow-up local-safe executor handoff to failed on the same `applyId` while still avoiding AWS mutation, confirms the embedded UI still includes current drill-down labels such as `Plan preview actions`, `Apply request actions`, `Controller runtime events`, `Recent logs`, `Lookup region`, and `Observer role configured`, confirms controller logs contain useful startup/reconcile output, and then runs `docker compose down --remove-orphans` even on failure.

GitHub Actions uses the same split so contributors and operators can read CI failures in the same terms as local reproduction. `.github/workflows/ci-smoke.yml` runs a fast lane on every pull request plus pushes to `main`; it pins the Go toolchain from `go.mod`, then runs `go test ./...` and `make smoke-local`. A separate slower Docker lane runs `make smoke-docker` only on pushes to `main` and manual `workflow_dispatch` runs, which keeps the PR path fast while still exercising the full containerized stack in automation.

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
