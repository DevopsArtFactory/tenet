# Tenet v1 Runtime Rollout Summary

`domain.DeploymentStatus` now carries an optional `rollout` summary slice derived by the controller.

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

Each `steps[]` entry is a `domain.RolloutStepSummary`:
- `kind`
- `current`

## Semantics

- `rollout` is omitted when the controller has no derived rollout view for the deployment.
- `phase` is the coarse lifecycle state for the desired generation.
- `currentStep` is the concise operator label for the step presently being executed or awaited.
- `steps[]` is the ordered runtime plan summary, suitable for overview/UI rendering.
- `currentStep` and `steps[].kind` intentionally use strategy-aware operator labels rather than generic placeholder phases.
- EC2 weighted-target-group summaries now read like `CreateCandidateCapacity -> ShiftTrafficCanary -> PromoteCandidateCapacity`.
- ECS CodeDeploy summaries now read like `CreateCodeDeployDeployment -> ShiftTrafficLinear -> VerifyServiceStability`, while native ECS rolling summaries use labels such as `UpdateECSService` and `VerifyServiceStability`.
- `steps[].current=true` marks the active step when one is known.

## API/UI usage

The runtime summary is preserved in two read surfaces:
- `GET /v1/status/deployments`
- `GET /v1/status/deployments/{deploymentName}`

The account overview surface also copies the same summary into each deployment card payload:
- `GET /v1/overview/accounts`

That lets the embedded overview UI show rollout phase, current step, and planned steps without needing additional API joins in the browser, while preserving the richer EC2 candidate-capacity and ECS CodeDeploy/native-rolling terminology from the planner.

## Compatibility

This slice is additive and optional.

Existing clients that only read sync, health, conditions, or artifact metadata remain compatible. New clients can opportunistically consume `rollout` when present.
