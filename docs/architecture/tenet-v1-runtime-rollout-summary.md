# Tenet v1 Runtime Rollout Summary

`domain.DeploymentStatus` now carries an optional `rollout` summary object derived by the controller.

## Purpose

The runtime status snapshot already answers basic sync and health questions, but active rollout operations need a compact progress view that is more specific than `Sync` and cheaper to consume than controller-internal state.

The `rollout` summary exists to expose that operator-facing progress view in a stable read model.

## Shape

`domain.RolloutSummary` currently contains:
- `desiredGeneration`
- `observedGeneration`
- `phase`
- `currentStep`
- `summary`
- `steps[]`
- `execution[]`

The additive runtime-summary/history view that the UI consumes also preserves controller-owned generation metadata alongside the rollout summary:
- `runtimeSummary.desiredGeneration`
- `runtimeSummary.observedGeneration`
- `runtimeSummary.targetType`
- `runtimeSummary.strategy`
- `runtimeSummary.executionMode`
- `runtimeSummary.changes[]` as additive latest-runtime change labels copied into overview/detail read models
- `runtimeSummary.changeDetails[]` as additive latest-runtime change-detail text copied into overview/detail read models
- `runtimeSummary.source` when the latest runtime summary was copied from controller-authored history
- `history[*].desiredGeneration`
- `history[*].observedGeneration`
- `history[*].targetType`
- `history[*].strategy`
- `history[*].executionMode`
- `history[*].changes[]` as additive per-entry runtime change labels
- `history[*].changeDetails[]` as additive per-entry runtime change-detail text
- `history[*].source`
- `history[*].rolloutPhase`

Each `steps[]` entry is a `domain.RolloutStepSummary`:
- `kind`
- `detail`
- `current`
- `status`

Each `execution[]` entry is a `domain.RolloutExecutionDetail`:
- `resource`
- `name`
- `description`

## Semantics

- `rollout` is omitted when the controller has no derived rollout view for the deployment.
- `phase` is the coarse lifecycle state for the desired generation.
- `currentStep` is derived from the first actionable step in the current abstract plan, skipping planner preflight steps such as `ResolvePlacement` and `ObserveCurrentState` when later execution work exists.
- `steps[]` is the ordered runtime plan summary, suitable for overview/UI rendering.
- `steps[].detail` is optional controller-derived explanatory text for an ordered plan step. When present it describes what Tenet intends to do next in operator-facing terms, but some terminal steps such as `MarkSynced` intentionally omit it. It is not live AWS execution telemetry.
- `steps[].status` is additive controller-derived progress metadata for that ordered plan: `completed` means the step is already behind the controller's current view of the plan, including fully finished rollouts that no longer have a current actionable step; `current` marks the controller's current actionable step; and `pending` marks later planned work.
- `runtimeSummary.desiredGeneration` / `runtimeSummary.observedGeneration` and `history[*].desiredGeneration` / `history[*].observedGeneration` are additive controller-owned freshness markers copied into the overview/detail read models so operators can compare the intended generation with the latest controller-observed generation without opening raw status snapshots.
- before the controller persists `rollout` and `history`, it now normalizes controller-owned runtime fields on the snapshot itself only when those fields are still structurally empty: a completed `MarkSynced` plan backfills an empty `observedGeneration` and defaults an entirely empty `sync` struct and/or an entirely empty `health` struct to `Synced` / `Healthy`; an actionable plan defaults an entirely empty `sync` struct and/or an entirely empty `health` struct to `Reconciling` / `Progressing` with controller-authored reason/summary text derived from the current rollout step; and a paused non-actionable plan defaults an entirely empty `sync` struct and/or an entirely empty `health` struct to `Paused` / `Unknown` while preserving any already-seeded sync/health values verbatim.
- controller-authored `managedDrift[]` remains additive and history-backed. For EC2 `asgWeightedTargetGroups` and `asgInstanceRefresh` deployments, the controller may append live-backed Auto Scaling Group capacity drift rows under `runtimeContext.capacity.desired`, `runtimeContext.capacity.min`, and `runtimeContext.capacity.max` when read-only AWS inspection succeeds. For ECS `ecsCodeDeploy` and `ecsRolling` deployments, the controller may append live ECS-backed rows under `runtimeContext.capacity.desired`, `runtimeContext.deploymentConfig.minimumHealthyPercent`, `runtimeContext.deploymentConfig.maximumPercent`, `runtimeContext.deploymentConfig.healthCheckGracePeriodSeconds`, and `runtimeContext.taskDefinition.image` by comparing the desired rollout capacity, desired minimum healthy percent, the implicit ECS maximum percent default, the desired warmup-derived health-check grace period, and the desired image artifact against the inspected ECS service desired count, deployment configuration, and a single task-definition container image. Live-backed rows may include optional `resourceRef` metadata (`resourceType` / `resourceName`) for the inspected Auto Scaling Group or ECS service that produced the evidence, plus optional additive `source` metadata (`kind`, optional `label` / `route`, optional `resourceType` / `resourceName`) pointing at the evidence route. The controller can also consume existing live traffic-routing inspection payloads to append optional high-severity rows under `runtimeContext.trafficRouting.weightedTargetGroups`, `runtimeContext.trafficRouting.codeDeployTargetGroups`, or `runtimeContext.trafficRouting.{blueTaskSetTargetGroup,greenTaskSetTargetGroup}.healthyTargets` with `resourceRef` and `source` pointing at the inspected listener/deployment-group/target-group resource; traffic-routing payloads with `inspectionMode=summary_only`, unavailable AWS access, or missing live detail are intentionally ignored. Artifact/read-model-only rows and rows without live resource context omit `resourceRef` and `source` entirely for backward compatibility. Unavailable/missing live access keeps reconciliation successful and omits those rows instead of inventing fallback live evidence.
- when a completed plan also carries controller-authored `managedDrift[]`, the controller rewrites its own completed-runtime text to match that drift signal: the controller-owned `sync` default becomes `Drifted`, the controller-owned default health summary becomes drift-aware, and the controller-authored completed rollout/history summary switches away from `synced` wording. Seeded/operator-authored `sync` and `health` evidence still wins unchanged, and non-completed active rollout summaries keep their existing wording.
- `runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode` and `history[*].targetType` / `history[*].strategy` / `history[*].executionMode` are additive rollout-context markers copied from the same controller-owned runtime view. They let overview/detail consumers render concise runtime context rows without guessing from catalog placement alone.
- `runtimeSummary.changes[]`, `runtimeSummary.changeDetails[]`, `history[*].changes[]`, and `history[*].changeDetails[]` are additive ordered change-signal fields. `runtimeSummary.changes[]` / `runtimeSummary.changeDetails[]` mirror the newest history-backed runtime signal when one exists so overview/search consumers can surface the latest labels and human-readable explanations without traversing `history[*]`, while `history[*].changes[]` / `history[*].changeDetails[]` preserve the per-entry labels and detail text for timeline/detail rendering. When the aligned change label is `managedDrift`, the paired detail text now stays single-slot but describes deterministic added, removed, and updated managed-drift rows instead of flattening the full row set into one coarse snapshot string.
- `runtimeSummary.source` is only populated when the latest summary was copied from persisted controller-authored history; fallback summaries built only from the current snapshot intentionally omit it instead of inventing synthetic provenance.
- `history[*].source` and `history[*].rolloutPhase` remain additive optional metadata on controller-authored history entries so detail consumers can show where an entry came from and which rollout lifecycle phase it represented.
- `execution[]` is an additive ordered list of concrete execution resources or bootstrap inputs that the controller derived for the active execution mode.
- `currentStep` and `steps[].kind` intentionally use strategy-aware operator labels rather than generic placeholder phases.
- `steps[].status` and `steps[].detail` are derived from the current plan index and execution mode that drive `currentStep`, `phase`, and `summary`; they are not live AWS execution telemetry and should not be interpreted as proof that AWS has already completed the underlying action.
- EC2 weighted-target-group summaries now read like `CreateCandidateCapacity -> ShiftTrafficCanary -> PromoteCandidateCapacity`.
- ECS CodeDeploy summaries now read like `CreateCodeDeployDeployment -> ShiftTrafficLinear -> VerifyServiceStability`, while native ECS rolling summaries use labels such as `UpdateECSService` and `VerifyServiceStability`.
- `steps[].current=true` remains as the legacy boolean marker for the same step whose `steps[].status` is `current`.
- `phase` and `summary` are derived from that same actionable step so the persisted read model describes what the controller would do next, not only the eventual tail of the current plan.
- EC2 weighted-target-group plans therefore surface `Provisioning` / `Creating candidate capacity for weighted target groups` first, then can advance to `HealthChecking` / `Verifying candidate capacity before traffic shifting`, and later to `Promoting` traffic-shift or promotion summaries as step-tracking becomes richer.
- EC2 canary/linear/full-shift `steps[].detail` text explains the same weighted-target-group flow in more concrete operator language: bootstrap a fresh candidate Auto Scaling Group from userdata, attach/warm it behind the candidate target group, and then progress traffic by canary slice, linear increments, or a single full cutover depending on strategy. When the compiled deployment carries richer rollout config, the candidate-capacity detail also names the normalized userdata bootstrap source, candidate desired/min/max capacity, and warmup duration.
- EC2 weighted-target-group `execution[]` entries identify the candidate Auto Scaling Group (`candidateAutoScalingGroup`), the active target group (`activeTargetGroup`), the candidate target group (`candidateTargetGroup`), the production listener rule (`productionListenerRule`) operators inspect while Tenet updates weighted forwarding during rollout, the weighted routing plan (`trafficShiftPlan`), and the userdata bootstrap source (`userDataBootstrapSource`) used to initialize candidate instances. The candidate ASG description is config-aware: it preserves the same resource identifier/order while naming the normalized userdata source plus candidate desired/min/max and warmup when configured. The traffic-shift-plan description is also strategy-aware, distinguishing canary, linear, and single-cutover semantics and appending rollout qualifiers when present.
- When live AWS inspection is available for those EC2 flows, the controller can now add best-effort managed-drift rows at `runtimeContext.launchTemplate.bootstrapSource`, `runtimeContext.launchTemplate.imageId`, and `runtimeContext.launchTemplate.instanceType` by reading the primary Auto Scaling launch template version and comparing any explicit bootstrap/userdata-source marker plus the observed `imageId` / `instanceType` against the normalized desired deployment bootstrap source and first-stack-region `ami_id` / `instance_type` intent. Missing AWS access, missing launch-template access, or unparseable user data stay additive and simply omit those rows; focused controller/API contract tests cover them because local smoke may legitimately lack AWS access.
- EC2 instance-refresh plans surface `Provisioning` / `Starting EC2 instance refresh` first, then `Stabilizing` / `Verifying replacement capacity during instance refresh` once verification becomes current. When stack rollout config includes `rolling_update_instance_count`, the provisioning summary and ASG execution detail also call out that batch size explicitly.
- EC2 instance-refresh `execution[]` entries identify the service Auto Scaling Group (`serviceAutoScalingGroup`) and the userdata bootstrap source (`userDataBootstrapSource`) that replacement capacity will consume.
- ECS CodeDeploy plans surface `Provisioning` / `Creating CodeDeploy traffic shift deployment` first, then `Promoting` / `Shifting traffic with ECS CodeDeploy`, and later `Stabilizing` / `Stabilizing ECS service after CodeDeploy traffic shift` when the actionable step advances.
- ECS CodeDeploy `steps[].detail` text explains creation of the CodeDeploy deployment resource, subsequent traffic-shift semantics for the replacement task set, and the final ECS service stability wait after traffic has moved. When `minHealthyPercent` and/or `warmupSeconds` are configured, the traffic-shift detail includes those rollout expectations directly.
- ECS CodeDeploy `execution[]` entries identify the ECS service (`ecsService`), CodeDeploy application (`codeDeployApplication`), CodeDeploy deployment group (`codeDeployDeploymentGroup`), production listener (`productionListener`), test listener (`testListener`), blue target group (`blueTaskSetTargetGroup`), and green target group (`greenTaskSetTargetGroup`) used for the traffic-shift contract. Each description now includes the active traffic-shift style (`blueGreen`, `canary`, or `linear`) plus rollout qualifiers such as `min healthy` and `warmup` when they are present in compiled rollout config.
- Native ECS rolling plans surface `Provisioning` / `Updating ECS service deployment` first and `Stabilizing` / `Stabilizing ECS service after rolling update` once service verification is current. When rollout config includes `minHealthyPercent` and/or `warmupSeconds`, the provisioning summary/detail and ECS service execution detail include those guardrails instead of leaving them implicit.
- Native ECS rolling `steps[].detail` text explains the in-place ECS service update that creates the rolling deployment and the later wait for service stability after task replacement.
- Native ECS rolling `execution[]` entries identify the ECS service Tenet will update in place.

## API/UI usage

The runtime summary is preserved in two read surfaces:
- `GET /v1/status/deployments`
- `GET /v1/status/deployments/{deploymentName}`

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` the raw status collection requires trusted authenticated identity and filters snapshots to the authorized deployment subset from exact trusted `X-Tenet-Authorized-Deployments` grants plus matching `X-Tenet-Authorized-Projects` / `X-Tenet-Authorized-Environments` catalog placement, returning `200 OK` with `[]` when no forwarded deployment name or project/environment scope matches persisted snapshots. When `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION=true` is also enabled, those same guarded status reads additionally require one trusted read-capable `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin` before allowlist filtering continues. The raw status detail route follows the same deployment-scoped `401 Unauthorized` / `403 Forbidden` contract as other guarded operator reads: missing trusted identity returns `unauthenticated: authenticated identity is required for this operator read route`, while a recognized operator without exact deployment membership or matching project/environment placement gets `forbidden: operator does not have access to this deployment`.

The overview read models also copy the same additive runtime summary into deployment-card payloads:
- `GET /v1/overview/applications`
- `GET /v1/overview/accounts`

The deployment overview detail surface preserves the same `rollout.execution[]` payload for expanded per-deployment inspection:
- `GET /v1/overview/deployments/{deploymentName}`

The deployment events surface exposes the same persisted controller/runtime timeline as an explicit operator-facing payload:
- `GET /v1/overview/deployments/{deploymentName}/events`

That lets the embedded overview UI keep the applications landing page focused on narrow triage while still consuming summary text and search/indexable rollout context from the same payload, and lets deeper detail/workspace views show rollout phase, current step, runtime-summary desired/observed generations, explicit runtime-context rows, runtime-summary change labels plus human-readable change details, runtime-summary provenance, and planned steps without needing additional API joins in the browser, while preserving the richer EC2 candidate-capacity and ECS CodeDeploy/native-rolling terminology from the planner. Expanded detail panels now render the same runtime-summary generation pair, an additive `Runtime context` metric sourced from `runtimeSummary.targetType` / `runtimeSummary.strategy` / `runtimeSummary.executionMode`, additive `Runtime changes` / `Runtime change details` metrics when latest labels/details are present, an additive `Runtime source` metric when available, and each history entry's desired/observed generations plus additive `History context` / `changes[]` / `changeDetails[]` / source / rollout-phase metadata when available, each rollout step's status together with its optional controller-derived `detail` text, and the concrete execution resources and descriptions, all with the same payload shape rather than inventing UI-only fields.

The `execution[]` list remains a controller-authored read-model summary, not a stream of live AWS observations. Clients should treat the resource names and descriptions as Tenet's intended execution contract for the current plan rather than definitive evidence that AWS has already created, attached, or shifted those resources. The overview builders and API routes preserve the ordered list and resource identifiers as-is while allowing only description text to evolve, so the exact identifier strings above are part of the stable read-model contract rather than documentation-only examples.

Deployment and application detail routes also include an additive top-level `managedDriftSummary` object next to the existing `managedDrift[]` rows so API/UI consumers can read current total/high/medium/low severity counts without recounting row details; empty row sets still serialize zero counts while preserving `managedDrift[]` `omitempty` compatibility. Populated managed-drift rows now also carry an additive backend-authored opaque `driftId` derived from stable row inputs plus duplicate-row ordinal, and the same ID is projected consistently through current status, `runtimeSummary.managedDrift`, and `history[*].managedDrift` without adding separate persistence. The deployment events route follows the same counting semantics across persisted `status.history`: it is copied from persisted history, returns `events: []` when no runtime history exists yet for a known deployment, includes an additive `managedDriftSummary` object with explicit `total`, `high`, `medium`, and `low` counts across the returned history rows, and is not a full durable audit/event system at this stage. For example, a drifted detail or history response can include `"managedDrift":[{"driftId":"drift_...","field":"runtimeContext.capacity.desired","desired":"6","actual":"4","severity":"high"}]` plus `"managedDriftSummary":{"total":1,"high":1,"medium":0,"low":0}` next to its preserved drift evidence.

## Compatibility

This slice is additive and optional.

Existing clients that only read sync, health, conditions, or artifact metadata remain compatible. New clients can opportunistically consume `rollout` when present.

`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true` is the concrete toggle example for that guarded raw-status collection/detail contract.
