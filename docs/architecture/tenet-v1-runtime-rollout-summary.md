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

Each `steps[]` entry is a `domain.RolloutStepSummary`:
- `kind`
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
- `steps[].status` is additive controller-derived progress metadata for that ordered plan: `completed` means the step is before the current actionable step, `current` marks the controller's current actionable step, and `pending` marks later planned work.
- `execution[]` is an additive ordered list of concrete execution resources or bootstrap inputs that the controller derived for the active execution mode.
- `currentStep` and `steps[].kind` intentionally use strategy-aware operator labels rather than generic placeholder phases.
- `steps[].status` is derived from the current plan index that drives `currentStep`, `phase`, and `summary`; it is not live AWS execution telemetry and should not be interpreted as proof that AWS has already completed the underlying action.
- EC2 weighted-target-group summaries now read like `CreateCandidateCapacity -> ShiftTrafficCanary -> PromoteCandidateCapacity`.
- ECS CodeDeploy summaries now read like `CreateCodeDeployDeployment -> ShiftTrafficLinear -> VerifyServiceStability`, while native ECS rolling summaries use labels such as `UpdateECSService` and `VerifyServiceStability`.
- `steps[].current=true` remains as the legacy boolean marker for the same step whose `steps[].status` is `current`.
- `phase` and `summary` are derived from that same actionable step so the persisted read model describes what the controller would do next, not only the eventual tail of the current plan.
- EC2 weighted-target-group plans therefore surface `Provisioning` / `Creating candidate capacity for weighted target groups` first, then can advance to `HealthChecking` / `Verifying candidate capacity before traffic shifting`, and later to `Promoting` traffic-shift or promotion summaries as step-tracking becomes richer.
- EC2 weighted-target-group `execution[]` entries identify the candidate Auto Scaling Group, the active target group, the candidate target group, and the userdata bootstrap source used to initialize candidate instances.
- EC2 instance-refresh plans surface `Provisioning` / `Starting EC2 instance refresh` first, then `Stabilizing` / `Verifying replacement capacity during instance refresh` once verification becomes current.
- EC2 instance-refresh `execution[]` entries identify the service Auto Scaling Group and the userdata bootstrap source that replacement capacity will consume.
- ECS CodeDeploy plans surface `Provisioning` / `Creating CodeDeploy traffic shift deployment` first, then `Promoting` / `Shifting traffic with ECS CodeDeploy`, and later `Stabilizing` / `Stabilizing ECS service after CodeDeploy traffic shift` when the actionable step advances.
- ECS CodeDeploy `execution[]` entries identify the ECS service plus the CodeDeploy application and deployment group used for the traffic shift.
- Native ECS rolling plans surface `Provisioning` / `Updating ECS service deployment` first and `Stabilizing` / `Stabilizing ECS service after rolling update` once service verification is current.
- Native ECS rolling `execution[]` entries identify the ECS service Tenet will update in place.

## API/UI usage

The runtime summary is preserved in two read surfaces:
- `GET /v1/status/deployments`
- `GET /v1/status/deployments/{deploymentName}`

The account overview surface also copies the same summary into each deployment card payload:
- `GET /v1/overview/accounts`

The deployment overview detail surface preserves the same `rollout.execution[]` payload for expanded per-deployment inspection:
- `GET /v1/overview/deployments/{deploymentName}`

That lets the embedded overview UI show rollout phase, current step, summary text, and planned steps without needing additional API joins in the browser, while preserving the richer EC2 candidate-capacity and ECS CodeDeploy/native-rolling terminology from the planner. Expanded detail panels then render the concrete execution resources and descriptions with the same payload shape rather than inventing UI-only fields.

## Compatibility

This slice is additive and optional.

Existing clients that only read sync, health, conditions, or artifact metadata remain compatible. New clients can opportunistically consume `rollout` when present.
