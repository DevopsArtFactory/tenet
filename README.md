# Tenet

Tenet is an in-progress multi-account control-plane foundation for repository-backed deployment intent.

The current bootstrap focuses on deterministic, test-backed core packages rather than live AWS execution. Desired state is loaded from a repo-shaped manifest tree, compiled into a validated catalog, and exposed through a small read-only API surface.

## Current status

Implemented today:
- architecture and implementation-plan docs for the first control-plane slice plus the local-runtime bootstrap plan
- Go module bootstrap with `tenet-api` and `tenet-controller`
- domain, placement, rollout, reconcile, spec, manifest, and catalog foundations
- parser-light repository loader for `accounts/`, `environments/`, and `deployments/` JSON or YAML manifests
- deployment manifests can now ingest a goployer-compatible EC2 stack shape while carrying desired binary/image artifact metadata; EC2 manifests now fail fast unless they provide userdata bootstrap metadata (`type` + normalized non-empty `path`) either at the top level or, when the top-level `userdata` block is omitted/blank, through the first stack fallback
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
- when the controller persists runtime state, it now normalizes controller-owned defaults before persisting rollout/history: completed `MarkSynced` plans backfill `observedGeneration` when it is empty and default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Synced`/`Healthy`; actionable plans default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Reconciling`/`Progressing` with controller-authored reason/summary text derived from the current rollout step; and paused non-actionable plans default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Paused`/`Unknown` without rewriting any already-seeded sync/health values
- rollout summaries now use strategy-aware step labels such as EC2 `CreateCandidateCapacity`/`ShiftTrafficCanary`/`PromoteCandidateCapacity`, ECS `CreateCodeDeployDeployment`/`ShiftTrafficLinear`, and native ECS rolling `UpdateECSService`/`VerifyServiceStability`
- rollout summaries now also annotate each `rollout.steps[*]` entry with controller-derived `status` values (`completed`, `current`, `pending`) plus optional controller-derived `detail` text so operators can distinguish already-finished plan steps, the current actionable step, and future planned work; `completed` also covers fully finished plans that no longer have a current actionable step. Both fields come from the controller's current abstract plan and are not live AWS execution telemetry
- compiled deployments now preserve a normalized rollout-config view derived from top-level strategy/capacity fields plus stack rollout knobs such as `termination_delay_rate` and `rolling_update_instance_count`, so controller-authored rollout summaries can describe candidate-capacity sizing, userdata bootstrap source, warmup, min-healthy expectations, and rolling batch size without relying on raw manifest syntax
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
  - `GET /v1/overview/applications`
  - `GET /v1/overview/applications/{deploymentName}`
  - `GET /v1/overview/accounts`
  - `GET /v1/overview/accounts/{awsAccountId}`
  - `GET /v1/overview/accounts/{awsAccountId}/projects/{appRef}/environments/{environmentRef}`
  - `GET /v1/overview/deployments/{deploymentName}`
  - `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}`
  - `POST /v1/plans/deployments/{deploymentName}`
  - `GET /v1/plans/{planId}`
  - `GET /v1/status/deployments`
  - `GET /v1/status/deployments/{deploymentName}`
- embedded browser UI at `/` now starts with an ArgoCD-inspired applications overview page backed directly by `GET /v1/overview/applications`, showing application-level status only (`account`, `project`, `sync`, `health`, `last update`) and routing operators into an application-detail drill-down page at `/?page=application&deployment={deploymentName}`; that page is now backed by `GET /v1/overview/applications/{deploymentName}` and still groups application-linked resources into `Workloads`, `Traffic routing`, and `Config and access` sections with `Open resource` drill-down links, while the existing resource-detail page and deployment-scoped `Drift view` / `Plan/apply` workspace shells remain available as deeper read-only drill-downs backed by `/v1/overview/deployments/{deploymentName}`. Resource-detail pages now also lazy-load a `Recent logs` section from `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}` for the selected resource, rendering loading, unavailable, empty, and populated states with safe DOM construction only. When a fixture/local detail payload omits `detail.rollout.execution[]` or provides it as an empty array, the application/resource drill-down synthesizes the same canonical execution resources the controller would normally author so resource pages still use stable, bookmarkable routes shaped as `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`. The same detail-driven UI now also surfaces additive runtime artifact and concise runtime condition context from `runtimeSummary.deployedArtifact` and `runtimeSummary.conditions` in the resource detail/history views, and the applications-page client-side search text now indexes those runtime-summary fields only when present instead of adding placeholder noise
- deployment workspace shells at `/?page=drift&deployment={deploymentName}` and `/?page=plan&deployment={deploymentName}` now surface latest runtime source/timestamp/history-count metadata, concise runtime rollout-context rows (`runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`), additive runtime artifact/condition rows sourced from `runtimeSummary.deployedArtifact` / `runtimeSummary.conditions`, `runtimeSummary.changes[]`-derived runtime change rows plus human-readable `runtimeSummary.changeDetails[]`, and `rollout.execution[]` resource rows directly in the workspace shell before operators open the generic detail panel. The drift workspace now also derives a real read-only `Field-level drift` table from the deployment detail payload, comparing desired artifact/generation/runtime intent against current runtime signals and surfacing unhealthy or drift conditions with sortable/filterable severity rows instead of only showing a placeholder. The plan/apply workspace still keeps `Create Plan` / `Apply Plan` disabled in the embedded UI, but the backend now exposes a first immutable read-only plan-preview contract through `POST /v1/plans/deployments/{deploymentName}` and `GET /v1/plans/{planId}`. That server-authored preview persists an immutable payload with stable `planId`, explicit `status` (`ready` / `failed` today, reserving `pending` for future async work), derived operations, risk/approval metadata, and source generations/fingerprint so operators and future UI wiring can detect stale inputs without inventing a client-only plan model.
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

The local examples now use YAML deployment manifests that demonstrate both supported manifest shapes:
- top-level desired-state fields such as `environmentRef`, `target`, `strategy`, `capacity`, `binary`, and `image`
- a goployer-compatible `stacks` shape for EC2/ECS placement metadata
- EC2 deployments must provide userdata bootstrap metadata via either top-level `userdata` or, when that top-level block is omitted/blank, the first stack fallback; Tenet trims and normalizes the path and rejects entries that collapse to an empty bootstrap source such as `./`
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
curl http://localhost:8080/v1/overview/applications
curl http://localhost:8080/v1/overview/applications/payments-api-prod
curl http://localhost:8080/v1/overview/accounts
curl http://localhost:8080/v1/overview/accounts/210987654321
curl http://localhost:8080/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl -X POST http://localhost:8080/v1/plans/deployments/payments-api-prod
curl http://localhost:8080/v1/plans/{planId}
curl http://localhost:8080/v1/status/deployments/payments-api-prod
```

When you run only `tenet-api` against the committed fixture file, the stored snapshots consumed by `/v1/status/deployments`, `/v1/status/deployments/{deploymentName}`, `/v1/overview/applications`, `/v1/overview/applications/{deploymentName}`, `/v1/overview/accounts`, and `/v1/overview/deployments/{deploymentName}` still reflect the seeded fixture data. Those payloads can already include additive `runtimeSummary` objects derived from the current snapshot `sync`/`health`/`rollout` fields, including controller-owned `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` freshness metadata plus additive rollout-context fields (`runtimeSummary.targetType`, `runtimeSummary.strategy`, `runtimeSummary.executionMode`), but they may not yet include controller-authored timestamps, history-backed summary text, additive `runtimeSummary.changes[]` / `runtimeSummary.changeDetails[]`, rollout summaries, or bounded `history[]` entries until the controller has written richer runtime state. When the newest persisted history entry authored by the controller is available, overview/detail payloads also surface additive `runtimeSummary.source` provenance copied from that history-backed summary plus additive `runtimeSummary.changes[]` / `runtimeSummary.changeDetails[]` copied from the latest history-backed runtime signal; matching `history[*]` timeline entries can also carry additive `targetType` / `strategy` / `executionMode` context copied from the controller-owned runtime view. The same additive read-model contract now also carries `runtimeSummary.deployedArtifact` plus concise `runtimeSummary.conditions[]` copies so overview/detail consumers can read the latest runtime artifact/condition context without scraping the full status object or latest history entry. Fallback summaries built only from the current snapshot intentionally omit `runtimeSummary.source` rather than inventing a synthetic source. To populate the execution-mode-aware latest runtime timestamp/history-backed summary, runtime-summary desired/observed generation fields, rollout step status/detail fields, changeDetails, and bounded history, run `tenet-controller` against the same backend/store instead of relying on the fixture file alone.

The new plan-preview endpoints reuse that same deployment-detail contract as their source of truth. `POST /v1/plans/deployments/{deploymentName}` derives a server-authored preview from `GET /v1/overview/deployments/{deploymentName}`, persists it in a local immutable JSON store (`TENET_PLAN_STORE_PATH`, defaulting to `${TMPDIR:-/tmp}/tenet/plan-previews.json`), and returns a stable `planId`. Repeating the same create request against the same source fingerprint returns the same immutable payload instead of mutating it. `GET /v1/plans/{planId}` reads back the exact persisted record so future UI work can poll/share a bookmarkable preview without recomputing it client-side. The record includes `source.fingerprint`, desired/observed generations, rollout/runtime context, derived operations, and operator-facing `risk` / `approval` guidance; stale optimistic requests can also send `{"ifMatch":"<source fingerprint>"}` to force a persisted `failed` preview instead of a vague server error when the deployment detail has changed.

The controller now prefers batch store applies when the backend implements that contract, so Redis-backed local runtime updates no longer depend on full-store replacements or rewrite unchanged deployments on every tick. Batch-capable stores receive only changed/new deployment snapshots plus stale deletes, still expressed as per-deployment `Upsert`/`Delete` mutations inside one transaction; when a reconcile is a complete no-op, the controller now skips the batch apply entirely. Plain `statusstore.Store` implementations still receive the full compiled snapshot set through `Save`, while the filesystem-backed compatibility path keeps its full-snapshot atomic read-modify-write behavior through `Apply` when real changes or deletes exist rather than becoming a low-write-volume incremental store. On each reconcile, deployments no longer present in the compiled repo are pruned from persisted runtime state alongside the current batch apply/save. The controller-authored runtime history remains bounded to the latest 10 entries per deployment. Those runtime-summary/history generation fields, provenance fields, and rollout-step status/detail fields are controller-owned read-model metadata derived from Tenet's current plan/state view rather than live AWS execution telemetry.

Resource recent-log examples surfaced through `GET /v1/overview/deployments/{deploymentName}/resource-logs` and the resource-detail page at `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`:
- ECS service logs: `resourceType=ecsService` with the selected ECS service name, returning recent CloudWatch log events when task/container log configuration can be resolved
- EC2 capacity logs: `resourceType=candidateAutoScalingGroup` or `resourceType=serviceAutoScalingGroup` with the selected Auto Scaling Group name, returning recent instance console/bootstrap lines from the newest instance when available

Current access limitations for this slice:
- unsupported resource types return `available=false` with a stable `unavailableReason` code instead of a hard API error
- missing runtime log sources, no recent running tasks/instances, unavailable bootstrap output, missing resources, and AWS-access failures also return `available=false` with a stable `unavailableReason` code
- valid supported resources with no recent entries return `available=true` with an empty `entries[]` payload so the embedded UI can render an explicit empty state
- cross-account access remains read-only and depends on AWS credentials plus optional assume-role configuration being available to the API process; local fixture/smoke runs may therefore show `Recent logs` as unavailable even though the section is present in the UI

Rollout `rollout.steps[*].detail` examples surfaced through `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and `/v1/status/deployments/{deploymentName}`:
- EC2 canary/linear/full-shift steps explain the candidate Auto Scaling Group bootstrap and the weighted target-group traffic progression Tenet intends to orchestrate, such as creating fresh candidate capacity from userdata, shifting a canary slice, advancing linear percentages, or cutting all traffic to the candidate target group.
- ECS CodeDeploy steps explain CodeDeploy deployment creation plus how traffic shifts to the replacement task set and then waits for post-shift ECS service stability.
- Native ECS rolling steps explain in-place ECS service deployment creation/update semantics plus the later service-stability verification step.

Execution detail examples surfaced through `/v1/overview/accounts`, `/v1/overview/deployments/{deploymentName}`, and `/v1/status/deployments/{deploymentName}`:
- EC2 weighted target groups: candidate Auto Scaling Group (`candidateAutoScalingGroup`), active target group (`activeTargetGroup`), candidate target group (`candidateTargetGroup`), production listener rule (`productionListenerRule`), traffic-shift plan (`trafficShiftPlan`), userdata bootstrap source (`userDataBootstrapSource`). The candidate ASG description now calls out the normalized userdata source plus candidate desired/min/max capacity and warmup when configured, while the traffic-shift plan text differentiates canary vs linear vs single-cutover semantics and appends rollout qualifiers such as warmup when present.
- EC2 instance refresh: service Auto Scaling Group (`serviceAutoScalingGroup`), userdata bootstrap source (`userDataBootstrapSource`). When stack rollout config includes `rolling_update_instance_count`, the service ASG description explicitly calls out the rolling-update batch size.
- ECS rolling: ECS service, optionally extended with rollout-strategy qualifiers such as `min healthy` and `warmup`.
- ECS CodeDeploy: ECS service (`ecsService`), CodeDeploy application (`codeDeployApplication`), CodeDeploy deployment group (`codeDeployDeploymentGroup`), production listener (`productionListener`), test listener (`testListener`), blue target group (`blueTaskSetTargetGroup`), green target group (`greenTaskSetTargetGroup`). Each description now includes the traffic-shift style (`blueGreen`, `canary`, or `linear`) and any rollout qualifiers when present.

Those `rollout.execution[]` entries remain controller-authored summaries of the intended execution contract. They describe the resources and routing objects the current abstract plan expects to use; they are not direct live AWS telemetry proving those resources already exist or have completed traffic movement. The overview APIs and embedded UI preserve that ordered execution-resource list verbatim so the exact identifier names above remain part of the shared read-model contract.

For older fixture/local payloads that still omit `detail.rollout.execution[]` or provide it as an empty array, the application/resource drill-down synthesizes fallback resources for that missing-or-empty-detail case; controller-authored execution arrays still win unchanged whenever they are present and non-empty. The fallback list is intentionally limited to the canonical controller-authored contracts already documented above:
- `asgWeightedTargetGroups`: `candidateAutoScalingGroup` (`{deployment}-candidate`), `activeTargetGroup` (`{deployment}-active`), `candidateTargetGroup` (`{deployment}-candidate`), `productionListenerRule` (`{deployment}-prod-listener`), `trafficShiftPlan` (`{deployment}-weighted-routing`), `userDataBootstrapSource` (`{type}:{path}` from `userData`, or `unspecified`)
- `asgInstanceRefresh`: `serviceAutoScalingGroup` (`{deployment}-service`), `userDataBootstrapSource` (`{type}:{path}` from `userData`, or `unspecified`)
- `ecsRolling`: `ecsService` (`{deployment}-service`)
- `ecsCodeDeploy`: `ecsService` (`{deployment}-service`), `codeDeployApplication` (`{deployment}-codedeploy-app`), `codeDeployDeploymentGroup` (`{deployment}-codedeploy-group`), `productionListener` (`{deployment}-prod-listener`), `testListener` (`{deployment}-test-listener`), `blueTaskSetTargetGroup` (`{deployment}-blue`), `greenTaskSetTargetGroup` (`{deployment}-green`)

Those fallback names/descriptions are kept byte-for-byte aligned with the canonical controller-authored `rollout.execution[]` metadata so only description text evolves while resource names/order stay fixed and previously shared/bookmarked resource-detail routes keep resolving across fixture-backed and controller-authored payloads.

The local smoke path now also verifies the ECS rolling resource-log route for the fixture deployment `worker-prod` via `GET /v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service`, using the same `available`/`entries`/`source` or `unavailableReason` JSON-shape assertions as the existing traffic-shift-plan check.

Controller-authored runtime history is surfaced through `/v1/overview/deployments/{deploymentName}` and rendered in the expandable deployment detail panel at `/`. The current slice keeps only a bounded recent timeline so operators can see the latest sync/health/rollout-step changes without introducing a full event log, and the detail renderer now adds `History changes`, `History source`, and `History rollout phase` rows when those additive fields are present on an entry. The overview cards and client-side search index also surface the latest `runtimeSummary.changes[]` / `runtimeSummary.changeDetails[]` so operators can spot and filter on the most recent runtime change signals without opening the detail panel.

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
curl http://localhost:8080/v1/overview/applications
curl http://localhost:8080/v1/overview/applications/payments-api-prod
curl http://localhost:8080/v1/overview/accounts
curl http://localhost:8080/v1/overview/accounts/210987654321
curl http://localhost:8080/v1/overview/accounts/123456789012/projects/payments-api/environments/prod-apne2
curl http://localhost:8080/v1/overview/deployments/payments-api-prod
curl -X POST http://localhost:8080/v1/plans/deployments/payments-api-prod
curl http://localhost:8080/v1/status/deployments
open 'http://localhost:8080/?sync=Reconciling'
open 'http://localhost:8080/?account=210987654321&q=codedeploy'
docker compose logs controller --tail=20
make docker-down
```

The embedded UI toolbar keeps its current search, sync-state, and account filters in the URL query string so operators can bookmark or share a focused view without adding server-side filter endpoints. The default `/` page is now an applications overview backed by `/v1/overview/applications`, so the landing page no longer has to flatten `/v1/overview/accounts` client-side just to render application rows. The landing page renders only application-level triage data (`application`, `account`, `project`, `sync`, `health`, `last update`) plus additive `_self` drill-down links for `Open account` (`/?page=account&accountId={awsAccountId}`) and `Open application` (`/?page=application&deployment={deploymentName}`), while the joined route intentionally still carries richer additive runtime/rollout/artifact fields reused from the account overview builder. Client-side search matching now also indexes `runtimeSummary.deployedArtifact` and concise `runtimeSummary.conditions[]` summaries when those fields are present, but it skips placeholder `—` values so empty runtime context does not create noisy matches. The account drill-down page consumes `/v1/overview/accounts/{awsAccountId}`, renders an `Account deployment overview` summary with grouped project/environment sections, and preserves sibling `Account JSON`, `Group JSON`, and `Open application` links without nesting them inside the expandable row toggle. The application-detail page now consumes `/v1/overview/applications/{deploymentName}`, while the deeper `resource`, `drift`, and `plan` pages continue to consume `/v1/overview/deployments/{deploymentName}`. The application-detail view still groups linked rollout resources into `Workloads`, `Traffic routing`, and `Config and access` sections, each with `Open resource` links targeting `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`. The resource-detail page now lazy-loads `Recent logs` from `/v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}` using the selected resource's canonical type/name pair and renders loading, unavailable, empty, and populated states wi...


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
- `docs/features/resource-recent-logs.md`

## Open Source
- [Contributing Guide](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)

## License
This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
