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

- `TENET_REPO_ROOT=/workspace/examples/local-repo`
- `TENET_STATUS_PATH=/workspace/examples/local-status/deployments.json`
- `TENET_CONTROLLER_REQUEUE_SECONDS=5` in the local Compose demo so controller planning logs refresh quickly

Both services mount `./examples` read-only. That keeps the runtime deterministic and aligned with the current repo-backed architecture.

## Developer flows

### Local smoke check without Docker

```bash
make smoke-local
```

This is the fast smoke layer. It boots `tenet-api` and `tenet-controller` directly against the committed example fixtures, checks `/healthz`, and validates representative JSON payloads from the catalog, overview, status, and embedded UI paths without involving Docker.

### Full Docker stack lifecycle

```bash
make docker-up
curl http://localhost:8080/v1/catalog/summary
docker compose logs controller --tail=20
make docker-down
```

`make docker-up` routes through `scripts/docker-up.sh`. That shared helper refuses to start if port `8080` is already listening, then runs `docker compose up --build -d` only when the host port is clear.

### Compose-backed smoke verification

```bash
make smoke-docker
```

This is the full-stack smoke layer. It:

- validates `docker compose config`
- starts the stack through the shared `docker-up` helper and port-`8080` preflight
- confirms `api`, `controller`, and `redis` are present
- checks `/healthz`, `/v1/catalog/summary`, `/v1/overview/accounts`, `/v1/overview/deployments/payments-api-prod`, `/v1/status/deployments/payments-api-prod`, and `/`
- captures useful controller/API startup and reconcile evidence from `docker compose logs`
- tears the stack down with `docker compose down --remove-orphans`

## Deferred work

This slice deliberately does not introduce live AWS reconciliation yet. The next layers still needed are:

- controller-owned reconciliation loops based on the abstract planner
- durable runtime history beyond a single status file
- write APIs
- AWS observation/execution adapters
