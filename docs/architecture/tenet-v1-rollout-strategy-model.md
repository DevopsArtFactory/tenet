# Tenet Rollout Strategy Model

Tenet now accepts four deployment strategies in manifest intent:
- `blueGreen`
- `rolling`
- `canary`
- `linear`

The control plane keeps those user-facing names while deriving a target-specific execution recommendation.

## EC2 execution model

For EC2 workloads, Tenet assumes immutable instance replacement and a fresh Auto Scaling Group for candidate capacity.

Recommended execution modes:
- `blueGreen` -> `asgWeightedTargetGroups`
- `canary` -> `asgWeightedTargetGroups`
- `linear` -> `asgWeightedTargetGroups`
- `rolling` -> `asgInstanceRefresh`

Why this shape:
- `userdata.sh` can bootstrap a new candidate ASG cleanly.
- ALB weighted target groups make gradual traffic shifts practical for canary and linear delivery.
- Blue/green remains a cutover between old and new target groups once the candidate is healthy.
- The execution-resource contract can therefore keep a stable ordered identifier set while using config-aware descriptions to explain candidate userdata source, candidate sizing/warmup, and canary vs linear vs full-cutover routing semantics.

Representative runtime step labels now mirror that execution model:
- `canary` / `linear` / `blueGreen` with `asgWeightedTargetGroups`:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `CreateCandidateCapacity`
  - `VerifyCandidateCapacity`
  - `ShiftTrafficCanary` or `ShiftTrafficLinear` or `ShiftTrafficFull`
  - `PromoteCandidateCapacity`
- `rolling` with `asgInstanceRefresh`:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `StartInstanceRefresh`
  - `VerifyReplacementCapacity`

## ECS execution model

For ECS workloads, Tenet derives:
- `rolling` -> `ecsRolling`
- `blueGreen` -> `ecsCodeDeploy`
- `canary` -> `ecsCodeDeploy`
- `linear` -> `ecsCodeDeploy`

Why this shape:
- Native ECS rolling is the simplest execution mode for standard rolling updates.
- CodeDeploy provides the first-class traffic-shifting semantics required for blue/green, canary, and linear releases.

Representative runtime step labels now mirror that execution model:
- `rolling` with `ecsRolling`:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `UpdateECSService`
  - `VerifyServiceStability`
- `blueGreen` / `canary` / `linear` with `ecsCodeDeploy`:
  - `ResolvePlacement`
  - `ObserveCurrentState`
  - `CreateCodeDeployDeployment`
  - `ShiftTrafficCanary` or `ShiftTrafficLinear` or `ShiftTrafficFull`
  - `VerifyServiceStability`

## Current scope

This slice now models and exposes both the execution recommendation and the operator-facing runtime step vocabulary that downstream status summaries reuse.

Tenet does not yet execute AWS rollouts, but the API, controller logs, and UI now show the intended EC2 candidate-capacity / weighted-target-group and ECS CodeDeploy / native-rolling mapping so future controller work can follow a stable contract.
