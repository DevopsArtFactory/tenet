# Tenet

Tenet is an in-progress multi-account control-plane foundation for repository-backed deployment intent.

The current bootstrap focuses on deterministic, test-backed core packages rather than live AWS execution. Desired state is loaded from a repo-shaped manifest tree, compiled into a validated catalog, and exposed through a small read-only API surface.

## Current status

Implemented today:
- architecture and implementation-plan docs for the first control-plane slice plus the local-runtime bootstrap plan
- Go module bootstrap with `tenet-api` and `tenet-controller`
- domain, placement, rollout, reconcile, spec, manifest, and catalog foundations
- parser-light repository loader for `accounts/`, `environments/`, and `deployments/` JSON or YAML manifests
- deployment manifests can now ingest a goployer-compatible EC2 stack shape while carrying desired binary/image artifact metadata
- validated catalog compilation and repository-backed providers
- repository-aware controller bootstrap that validates `TENET_REPO_ROOT`, logs catalog counts at startup, and stays alive as a long-running process
- controller runtime loop that reloads repository and status fixtures on a timer and logs per-deployment planner intents
- rollout intent now supports `blueGreen`, `rolling`, `canary`, and `linear`
- target-aware execution recommendations are exposed for EC2 (`asgWeightedTargetGroups`, `asgInstanceRefresh`) and ECS (`ecsRolling`, `ecsCodeDeploy`)
- pluggable deployment status storage with filesystem and Redis backends
- status stores can be seeded from committed fixture JSON when empty
- filesystem-backed deployment status storage for the first runtime-status slice
- status snapshots now carry sync reasons, health summaries, deployment conditions, deployed binary/image artifact metadata, and controller-derived rollout summaries
- rollout summaries now use strategy-aware step labels such as EC2 `CreateCandidateCapacity`/`ShiftTrafficCanary`/`PromoteCandidateCapacity`, ECS `CreateCodeDeployDeployment`/`ShiftTrafficLinear`, and native ECS rolling `UpdateECSService`/`VerifyServiceStability`
- rollout summaries now also surface `rollout.execution[]` details so operators can see the concrete execution resources Tenet intends to use, including EC2 candidate ASGs/target groups, EC2 instance-refresh ASGs and userdata bootstrap sources, ECS rolling services, and ECS CodeDeploy application/deployment-group resources
- committed local example fixtures under `examples/local-repo/` and `examples/local-status/`
- Docker packaging and `compose.yaml` so the current control plane can run locally without AWS credentials
- read-only API endpoints for:
  - `GET /`
  - `GET /healthz`
  - `GET /v1/info`
  - `GET /v1/catalog/summary`
  - `GET /v1/catalog/accounts`
  - `GET /v1/catalog/environments`
  - `GET /v1/catalog/deployments`
  - `GET /v1/catalog/placements`
  - `GET /v1/catalog/topology`
  - `GET /v1/overview/accounts`
  - `GET /v1/overview/deployments/{deploymentName}`
  - `GET /v1/status/deployments`
  - `GET /v1/status/deployments/{deploymentName}`
- embedded browser UI grouped by account and deployment at `/`, including additive account-header count pills from `/v1/overview/accounts` for deployment totals plus synced/progressing/paused/error runtime-state summaries, controller-authored rollout summary text alongside rollout phase, current step, and planned steps for EC2 candidate-capacity, weighted-target-group, ECS CodeDeploy, and ECS rolling executions, plus expandable deployment detail panels that lazy-load joined placement, sync reason, rollout generation, rollout execution resources/descriptions, rollout-step, and condition data from `/v1/overview/deployments/{deploymentName}`
- method guards for the read-only API surface
- fail-fast validation for invalid repository roots and incomplete manifest layouts
- optional status-file exposure through `TENET_STATUS_PATH`
- a local smoke test script that validates the fixture-backed API flow end to end

Not implemented yet:
- live AWS reconciliation/execution
- controller-owned persisted runtime deployment status/history beyond current snapshot storage
- write APIs
- a dedicated frontend application or mutation workflows

## Repository shape

Tenet currently expects a repository-like manifest tree:
- `accounts/*.{json,yaml,yml}`
- `environments/*.{json,yaml,yml}`
- `deployments/*.{json,yaml,yml}`

The local examples now use YAML deployment manifests with:
- a goployer-compatible `stacks` shape for EC2/ECS placement metadata
- `binary` for EC2 artifact intent
- `image` for ECS artifact intent

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
curl http://localhost:8080/v1/overview/accounts
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl http://localhost:8080/v1/status/deployments/payments-api-prod
```

When you run only `tenet-api` against the committed fixture file, the status endpoint still reflects the seeded snapshots and may not include controller-derived `rollout` summaries yet. To populate the execution-mode-aware rollout phase, current step, summary text, planned steps, and `rollout.execution[]` resource details, either run `tenet-controller` against the same repo/status store or use the Docker Compose stack below.

Execution detail examples surfaced through `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and `/v1/status/deployments/{deploymentName}`:
- EC2 weighted target groups: candidate Auto Scaling Group, active target group, candidate target group, userdata bootstrap source
- EC2 instance refresh: service Auto Scaling Group, userdata bootstrap source
- ECS rolling: ECS service
- ECS CodeDeploy: ECS service, CodeDeploy application, CodeDeploy deployment group

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
- `controller` as a long-running repo-validating process that now reloads fixtures and logs reconcile plans every few seconds
- `redis` as the active runtime status backend, seeded from `examples/local-status/deployments.json` when empty

Useful commands:

```bash
curl http://localhost:8080/
curl http://localhost:8080/healthz
curl http://localhost:8080/v1/catalog/topology
curl http://localhost:8080/v1/overview/accounts
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl http://localhost:8080/v1/status/deployments
docker compose logs controller --tail=20
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
- `docs/plans/2026-04-15-controller-runtime-loop.md`
- `docs/plans/2026-04-15-deployment-yaml-manifests.md`
- `docs/plans/2026-04-15-rollout-runtime-ui.md`
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
- `docs/architecture/tenet-v1-rollout-strategy-model.md`
- `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- `docs/architecture/tenet-v1-runtime-state-storage.md`
- `docs/architecture/tenet-v1-overview-ui.md`

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
