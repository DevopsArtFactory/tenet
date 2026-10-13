# Tenet

Tenet is a repository-driven deployment control plane for multi-account AWS environments. It brings declared deployment intent, runtime status, rollout progress, and resource evidence into one operator UI for **EC2 and ECS**.

**Status: early-stage, primarily read-only.** Tenet can inspect AWS resources, create deployment plans, and record apply requests. Live AWS deployment mutation is not implemented. Enabling apply advances local handoff metadata only; the current executor stops before changing AWS resources.

## What it does

- Loads and validates account, environment, and deployment manifests from a local checkout or GitHub.
- Compares desired configuration with runtime status and supported AWS observations to surface drift, health, and rollout context.
- Provides an **Applications → Application → Resource** workflow, with account, drift, plan/apply, audit, log, and live-inspection views.
- Records deployment plans, apply lifecycles, and audit evidence for review.
- Supports optional managed login, repository-backed RBAC, and signed GitHub webhook refresh handoffs in GitHub + Redis mode.

## Run locally

Requires Go 1.27.2 or later. From the repository root:

```bash
TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT="$PWD/examples/local-repo" \
TENET_STATUS_PATH="$PWD/examples/local-status/deployments.json" \
TENET_API_ADDR=127.0.0.1:18083 \
go run ./cmd/tenet-api
```

Open [localhost:18083](http://localhost:18083/). This starts the API and embedded UI using sample manifests and saved runtime fixtures.

Live inspection and logs require AWS credentials and the appropriate observer-role access.

For a controller and Redis-backed local stack, use Docker Compose:

```bash
make docker-up
# Open http://localhost:8080/
make docker-down
```

The controller refreshes runtime state periodically. See the [local runtime guide](docs/architecture/tenet-v1-local-runtime.md) for controller, storage, GitHub catalog, and auth configuration.

## How it works

```text
Local repository / GitHub manifests
                 ↓
       Validate and compile catalog
                 ↓
Controller → Runtime status store
                 ↓
       Go API + embedded operator UI
```

Desired configuration and runtime state are stored separately. The API joins both into operator views. Runtime status supports file and Redis backends; plan/apply records use file-backed stores.

The UI is static HTML/CSS/JavaScript embedded in the Go server and needs no separate frontend build.

## Development

```bash
go test ./...
make smoke-local
```

Use `make smoke-docker` when changing the Compose stack or its runtime behavior. CI scope, helper syntax checks, and runner troubleshooting are documented in the [operator reference](docs/architecture/tenet-v1-operator-reference.md#validation). See [CONTRIBUTING](CONTRIBUTING.md) for contribution guidelines.

## Documentation

| Guide | Covers |
| --- | --- |
| [Architecture](docs/architecture/tenet-v1-overview.md) | Desired state, runtime state, and control-plane scope |
| [Local runtime](docs/architecture/tenet-v1-local-runtime.md) | API/controller setup, Compose, storage, and configuration |
| [Operator UI](docs/features/overview-dashboard.md) | Applications, accounts, resources, drift, audit, and plan/apply |
| [Operator reference](docs/architecture/tenet-v1-operator-reference.md) | Detailed UI behavior, auth settings, apply boundaries, and validation |
| [API contracts](docs/architecture/tenet-v1-operator-contracts.md) | Endpoint and read-model contracts |
| [Resource inspection](docs/features/resource-inspection.md) | Live AWS resource evidence and selectors |
| [Recent logs](docs/features/resource-recent-logs.md) | Log sources, lookup context, and bounded reads |
| [Managed login](docs/features/managed-login.md) | Login providers, sessions, and authorization boundaries |
| [Troubleshooting](docs/runbooks/operator-troubleshooting.md) | Operational diagnostics and evidence handoffs |
| [AWS bootstrap](deploy/terraform/aws-central-control-plane/README.md) | Central control-plane Terraform deployment |

## Repository layout

| Path | Purpose |
| --- | --- |
| `cmd/tenet-api` | API server and embedded UI |
| `cmd/tenet-controller` | Runtime reconciliation loop |
| `internal/catalog`, `internal/manifest*` | Manifest loading, validation, and compilation |
| `internal/reconcile`, `internal/rollout` | Deployment planning and rollout models |
| `internal/statusstore`, `internal/planpreview`, `internal/applyrequest`, `internal/auditlog` | Runtime state, plans, apply records, and audit |
| `internal/resourceinspection`, `internal/resourcelogs` | AWS resource and log reads |
| `examples` | Sample manifests and runtime fixtures |
| `docs`, `scripts`, `deploy` | Guides, smoke checks, and deployment configuration |

## License

[Apache License 2.0](LICENSE).
