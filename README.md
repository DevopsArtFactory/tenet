# Tenet

Tenet is an in-progress multi-account control-plane foundation for repository-backed deployment intent.

The current bootstrap focuses on deterministic, test-backed core packages rather than live AWS execution. Desired state is loaded from a repo-shaped manifest tree, compiled into a validated catalog, and exposed through a small read-only API surface.

## Current status

Implemented today:
- architecture and implementation-plan docs for the first control-plane slice plus the local-runtime bootstrap plan
- Go module bootstrap with `tenet-api` and `tenet-controller`
- domain, placement, rollout, reconcile, spec, manifest, and catalog foundations
- parser-light repository loader for `accounts/`, `environments/`, and `deployments/` JSON manifests
- validated catalog compilation and repository-backed providers
- repository-aware controller bootstrap that validates `TENET_REPO_ROOT`, logs catalog counts at startup, and stays alive as a long-running process
- filesystem-backed deployment status storage for the first runtime-status slice
- status snapshots now carry sync reasons, health summaries, and deployment conditions
- committed local example fixtures under `examples/local-repo/` and `examples/local-status/`
- Docker packaging and `compose.yaml` so the current control plane can run locally without AWS credentials
- read-only API endpoints for:
  - `GET /healthz`
  - `GET /v1/info`
  - `GET /v1/catalog/summary`
  - `GET /v1/catalog/accounts`
  - `GET /v1/catalog/environments`
  - `GET /v1/catalog/deployments`
  - `GET /v1/catalog/placements`
  - `GET /v1/catalog/topology`
  - `GET /v1/status/deployments`
  - `GET /v1/status/deployments/{deploymentName}`
- method guards for the read-only API surface
- fail-fast validation for invalid repository roots and incomplete manifest layouts
- optional status-file exposure through `TENET_STATUS_PATH`
- a local smoke test script that validates the fixture-backed API flow end to end

Not implemented yet:
- live AWS reconciliation/execution
- controller-owned persisted runtime deployment status/history
- write APIs
- UI

## Repository shape

Tenet currently expects a repository-like manifest tree:
- `accounts/*.json`
- `environments/*.json`
- `deployments/*.json`

A runnable example lives in:
- `examples/local-repo/`
- `examples/local-status/deployments.json`

## Local development

Run the API directly against the committed fixtures:

```bash
TENET_REPO_ROOT=$(pwd)/examples/local-repo \
TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json \
go run ./cmd/tenet-api
```

In another terminal you can inspect the current control plane state:

```bash
curl http://localhost:8080/v1/catalog/summary
curl http://localhost:8080/v1/status/deployments/payments-api-prod
```

Run the local smoke check:

```bash
make smoke-local
```

## Docker runtime

Bring up the local stack with Docker Compose:

```bash
make docker-up
```

This starts:
- `api` on `http://localhost:8080`
- `controller` as a long-running repo-validating process

Useful commands:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/v1/catalog/topology
curl http://localhost:8080/v1/status/deployments
make docker-down
```


Use the repository test suite:

```bash
go test ./...
```

If your local Go toolchain needs explicit paths, the development flow in this repo has also been verified with:

```bash
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global go test ./...
```

## Documentation map

Architecture and plan docs live under `docs/`:
- `docs/plans/2026-04-15-control-plane-bootstrap.md`
- `docs/plans/2026-04-15-local-runtime-bootstrap.md`
- `docs/architecture/tenet-v1-overview.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-spec-shapes.md`
- `docs/architecture/tenet-v1-filesystem-loader.md`
- `docs/architecture/tenet-v1-catalog-loading-flow.md`
- `docs/architecture/tenet-v1-catalog-compilation.md`
- `docs/architecture/tenet-v1-summary-provider.md`
- `docs/architecture/tenet-v1-catalog-summary-api.md`
- `docs/architecture/tenet-v1-catalog-accounts-api.md`
- `docs/architecture/tenet-v1-catalog-environments-api.md`
- `docs/architecture/tenet-v1-catalog-deployments-api.md`
- `docs/architecture/tenet-v1-catalog-placements-api.md`
- `docs/architecture/tenet-v1-catalog-topology-api.md`
- `docs/architecture/tenet-v1-controller-repository-bootstrap.md`
- `docs/architecture/tenet-v1-deployment-status-store.md`
- `docs/architecture/tenet-v1-deployment-status-api.md`
- `docs/architecture/tenet-v1-local-runtime.md`

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
