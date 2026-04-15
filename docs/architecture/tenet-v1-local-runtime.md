# Tenet v1 Local Runtime

The local runtime packages the current read-only control-plane foundation as a Docker-friendly stack.

## What is implemented in this slice

- `tenet-api` serves the catalog and deployment-status read APIs.
- `tenet-controller` validates the repository catalog at startup and stays alive as a long-running control-plane process.
- a sample repository-shaped desired-state tree lives under `examples/local-repo/`.
- a sample filesystem-backed deployment-status file lives under `examples/local-status/deployments.json`.
- `compose.yaml` wires both processes together so the repo can be explored locally without AWS credentials.

## Runtime contract

The local stack uses these inputs:

- `TENET_REPO_ROOT=/workspace/examples/local-repo`
- `TENET_STATUS_PATH=/workspace/examples/local-status/deployments.json`

Both services mount `./examples` read-only. That keeps the runtime deterministic and aligned with the current repo-backed architecture.

## Developer flows

### Local smoke check without Docker

```bash
make smoke-local
```

This boots `tenet-api` against the committed example fixtures, checks `/healthz`, and validates representative JSON payloads from the catalog and status endpoints.

### Full Docker stack

```bash
make docker-up
curl http://localhost:8080/v1/catalog/summary
curl http://localhost:8080/v1/status/deployments/payments-api-prod
make docker-down
```

## Deferred work

This slice deliberately does not introduce live AWS reconciliation yet. The next layers still needed are:

- controller-owned reconciliation loops based on the abstract planner
- durable runtime history beyond a single status file
- write APIs
- AWS observation/execution adapters
