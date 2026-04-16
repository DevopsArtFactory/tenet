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
- controller-owned runtime persistence now prefers batch store applies when the selected backend supports them, letting Redis persist only changed/new deployment snapshots plus stale deletes through transactional per-deployment mutations while preserving full-store save fallback for simpler stores; deployments removed from the repo are pruned from persisted runtime state on the next controller reconcile
- filesystem-backed deployment status storage for the first runtime-status slice
- status snapshots now carry sync reasons, health summaries, deployment conditions, deployed binary/image artifact metadata, bounded controller-authored runtime history entries, and controller-derived rollout summaries
- rollout summaries now use strategy-aware step labels such as EC2 `CreateCandidateCapacity`/`ShiftTrafficCanary`/`PromoteCandidateCapacity`, ECS `CreateCodeDeployDeployment`/`ShiftTrafficLinear`, and native ECS rolling `UpdateECSService`/`VerifyServiceStability`
- rollout summaries now also annotate each `rollout.steps[*]` entry with controller-derived `status` values (`completed`, `current`, `pending`) plus optional controller-derived `detail` text so operators can distinguish already-finished plan steps, the current actionable step, and future planned work; `completed` also covers fully finished plans that no longer have a current actionable step. Both fields come from the controller's current abstract plan and are not live AWS execution telemetry
- rollout summaries now also surface additive controller-derived `rollout.execution[]` details so operators can see the concrete execution resources Tenet intends to use, including EC2 weighted-target-group candidate capacity and traffic-routing resources, EC2 instance-refresh ASGs and userdata bootstrap sources, ECS rolling services, and ECS CodeDeploy listener/target-group resources alongside application and deployment-group resources
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
  - `GET /v1/overview/accounts/{awsAccountId}`
  - `GET /v1/overview/deployments/{deploymentName}`
  - `GET /v1/status/deployments`
  - `GET /v1/status/deployments/{deploymentName}`
- embedded browser UI grouped by account and then project/environment deployment sections at `/`, including additive account-header count pills from `/v1/overview/accounts` for deployment totals plus synced/progressing/paused/error runtime-state summaries, grouped project/environment header status pills derived from the visible deployments in that section using the same bucket semantics as the account header (`Synced` => synced, `Reconciling`/`OutOfSync`/`Drifted` => progressing, `Paused` => paused, `Error` => error) while intentionally omitting zero-value buckets to keep compact headers small, additive account-level topology summary fields `projectCount` / `environmentCount` when present with fallback derivation from visible deployments when older payloads omit them, an account-header `Account JSON` quick link to `/v1/overview/accounts/{awsAccountId}`, per-group `Group JSON` quick links to `/v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`, additive `runtimeSummary` cards on each deployment row (latest summary text plus latest timestamp fallback plus desired/observed generation metadata, latest runtime change labels from `runtimeSummary.changes[]`, and history-backed provenance text via `runtimeSummary.source` when present), controller-authored rollout summary text alongside rollout phase, current step, and planned steps for EC2 weighted-target-group and instance-refresh executions plus ECS CodeDeploy and native rolling executions, an operator filter toolbar with free-text search plus sync-state/account selectors and shareable URL query params, free-text matching that now also includes additive `runtimeSummary` provenance/source fields plus latest change labels, per-deployment quick links to the existing overview/status JSON detail read models, plus expandable deployment detail panels that lazy-load joined placement, sync reason, runtime-summary generations/source/change labels, rollout generation, rollout execution resources/descriptions, runtime history generation metadata plus additive history `changes[]`/source/rollout-phase rows when present, rollout-step status/detail pairs, and condition data from `/v1/overview/deployments/{deploymentName}`
- method guards for the read-only API surface
- fail-fast validation for invalid repository roots and incomplete manifest layouts
- optional status-file exposure through `TENET_STATUS_PATH`
- a local smoke test script that validates the fixture-backed API flow end to end

Not implemented yet:
- live AWS reconciliation/execution
- controller-owned persisted runtime deployment event streams/audit logs beyond the current bounded status-history timeline
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
curl http://localhost:8080/v1/overview/accounts/210987654321
curl http://localhost:8080/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl http://localhost:8080/v1/status/deployments/payments-api-prod
```

When you run only `tenet-api` against the committed fixture file, the stored snapshots consumed by `/v1/status/deployments`, `/v1/status/deployments/{deploymentName}`, `/v1/overview/accounts`, and `/v1/overview/deployments/{deploymentName}` still reflect the seeded fixture data. Those payloads can already include additive `runtimeSummary` objects derived from the current snapshot `sync`/`health`/`rollout` fields, including controller-owned `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` freshness metadata, but they may not yet include controller-authored timestamps, history-backed summary text, additive `runtimeSummary.changes[]` labels, rollout summaries, or bounded `history[]` entries until the controller has written richer runtime state. When the newest persisted history entry authored by the controller is available, overview/detail payloads also surface additive `runtimeSummary.source` provenance copied from that history-backed summary plus additive `runtimeSummary.changes[]` labels copied from the latest history-backed runtime signal; fallback summaries built only from the current snapshot intentionally omit `runtimeSummary.source` rather than inventing a synthetic source. To populate the execution-mode-aware latest runtime timestamp/history-backed summary, runtime-summary desired/observed generations, runtime-summary provenance, runtime-summary change labels, rollout phase, current step, summary text, planned steps, per-step `rollout.steps[*].status` values (`completed`, `current`, `pending`), per-step `rollout.steps[*].detail` explanations, `rollout.execution[]` resource details, and recent runtime history timeline (including additive `history[*].desiredGeneration` / `history[*].observedGeneration` fields plus optional `history[*].changes[]` / `history[*].source` / `history[*].rolloutPhase` metadata), either run `tenet-controller` against the same repo/status store or use the Docker Compose stack below.

The controller now prefers batch store applies when the backend implements that contract, so Redis-backed local runtime updates no longer depend on full-store replacements or rewrite unchanged deployments on every tick. Batch-capable stores receive only changed/new deployment snapshots plus stale deletes, still expressed as per-deployment `Upsert`/`Delete` mutations inside one transaction; when a reconcile is a complete no-op, the controller now skips the batch apply entirely. Plain `statusstore.Store` implementations still receive the full compiled snapshot set through `Save`, while the filesystem-backed compatibility path keeps its full-snapshot atomic read-modify-write behavior through `Apply` when real changes or deletes exist rather than becoming a low-write-volume incremental store. On each reconcile, deployments no longer present in the compiled repo are pruned from persisted runtime state alongside the current batch apply/save. The controller-authored runtime history remains bounded to the latest 10 entries per deployment. Those runtime-summary/history generation fields, provenance fields, and rollout-step status/detail fields are controller-owned read-model metadata derived from Tenet's current plan/state view rather than live AWS execution telemetry.

Rollout `rollout.steps[*].detail` examples surfaced through `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and `/v1/status/deployments/{deploymentName}`:
- EC2 canary/linear/full-shift steps explain the candidate Auto Scaling Group bootstrap and the weighted target-group traffic progression Tenet intends to orchestrate, such as creating fresh candidate capacity from userdata, shifting a canary slice, advancing linear percentages, or cutting all traffic to the candidate target group.
- ECS CodeDeploy steps explain CodeDeploy deployment creation plus how traffic shifts to the replacement task set and then waits for post-shift ECS service stability.
- Native ECS rolling steps explain in-place ECS service deployment creation/update semantics plus the later service-stability verification step.

Execution detail examples surfaced through `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and `/v1/status/deployments/{deploymentName}`:
- EC2 weighted target groups: candidate Auto Scaling Group (`candidateAutoScalingGroup`), active target group (`activeTargetGroup`), candidate target group (`candidateTargetGroup`), production listener rule (`productionListenerRule`), traffic-shift plan (`trafficShiftPlan`), userdata bootstrap source (`userDataBootstrapSource`)
- EC2 instance refresh: service Auto Scaling Group (`serviceAutoScalingGroup`), userdata bootstrap source (`userDataBootstrapSource`)
- ECS rolling: ECS service
- ECS CodeDeploy: ECS service (`ecsService`), CodeDeploy application (`codeDeployApplication`), CodeDeploy deployment group (`codeDeployDeploymentGroup`), production listener (`productionListener`), test listener (`testListener`), blue target group (`blueTaskSetTargetGroup`), green target group (`greenTaskSetTargetGroup`)

Those `rollout.execution[]` entries remain controller-authored summaries of the intended execution contract. They describe the resources and routing objects the current abstract plan expects to use; they are not direct live AWS telemetry proving those resources already exist or have completed traffic movement. The overview APIs and embedded UI preserve that ordered execution-resource list verbatim so the exact identifier names above remain part of the shared read-model contract.

Controller-authored runtime history is surfaced through `/v1/overview/deployments/{deploymentName}` and rendered in the expandable deployment detail panel at `/`. The current slice keeps only a bounded recent timeline so operators can see the latest sync/health/rollout-step changes without introducing a full event log, and the detail renderer now adds `History changes`, `History source`, and `History rollout phase` rows when those additive fields are present on an entry. The overview cards and client-side search index also surface the latest `runtimeSummary.changes[]` labels so operators can spot and filter on the most recent runtime change signals without opening the detail panel.

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
curl http://localhost:8080/v1/overview/accounts/210987654321
curl http://localhost:8080/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl http://localhost:8080/v1/status/deployments
open 'http://localhost:8080/?sync=Reconciling'
open 'http://localhost:8080/?account=210987654321&q=codedeploy'
docker compose logs controller --tail=20
make docker-down
```

The embedded UI toolbar keeps its current search, sync-state, and account filters in the URL query string so operators can bookmark or share a focused view without adding server-side filter endpoints. Each account card now also summarizes topology with additive account-level `projectCount` / `environmentCount` fields from `/v1/overview/accounts` when they are present, falls back to deriving those counts from the visible deployments for older payloads that omit them, exposes an `Account JSON` quick link to `/v1/overview/accounts/{awsAccountId}` directly in the account header, and groups deployment rows into project/environment sections before operators expand individual deployments. Those grouped project/environment headers now also show additive grouped status pills derived from the deployments still visible in that section, using the same runtime-state buckets as the account header (`Synced` => synced, `Reconciling`/`OutOfSync`/`Drifted` => progressing, `Paused` => paused, `Error` => error) and omitting any zero-value bucket so the compact group headers stay readable. Runtime-summary display/search text on those cards now includes provenance/source text when `runtimeSummary.source` is present, so controller-authored history-backed summaries remain visible and searchable from the overview without opening the detail panel. Each grouped project/environment header now also exposes a `Group JSON` quick link into the matching grouped read model, while each deployment card still exposes direct `Overview JSON` and `Status JSON` links for quick navigation into the existing read-only detail endpoints.


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
- `docs/plans/2026-04-16-runtime-change-only-writes.md`
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
