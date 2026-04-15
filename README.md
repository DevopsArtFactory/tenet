# Tenet

Tenet is an in-progress multi-account control-plane foundation for repository-backed deployment intent.

The current bootstrap focuses on deterministic, test-backed core packages rather than live AWS execution. Desired state is loaded from a repo-shaped manifest tree, compiled into a validated catalog, and exposed through a small read-only API surface.

## Current status

Implemented today:
- architecture and implementation-plan docs for the first control-plane slice
- Go module bootstrap with `tenet-api` and `tenet-controller`
- domain, placement, rollout, reconcile, spec, manifest, and catalog foundations
- parser-light repository loader for `accounts/`, `environments/`, and `deployments/` JSON manifests
- validated catalog compilation and repository-backed providers
- repository-aware controller bootstrap that validates `TENET_REPO_ROOT` and logs catalog counts at startup
- read-only API endpoints for:
  - `GET /healthz`
  - `GET /v1/info`
  - `GET /v1/catalog/summary`
  - `GET /v1/catalog/accounts`
  - `GET /v1/catalog/environments`
  - `GET /v1/catalog/deployments`
  - `GET /v1/catalog/placements`
  - `GET /v1/catalog/topology`
- method guards for the read-only API surface
- fail-fast validation for invalid repository roots and incomplete manifest layouts

Not implemented yet:
- live AWS reconciliation/execution
- persisted runtime deployment status/history
- write APIs
- UI

## Repository shape

Tenet currently expects a repository-like manifest tree:
- `accounts/*.json`
- `environments/*.json`
- `deployments/*.json`

## Running tests

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

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
