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

## ECS execution model

For ECS workloads, Tenet derives:
- `rolling` -> `ecsRolling`
- `blueGreen` -> `ecsCodeDeploy`
- `canary` -> `ecsCodeDeploy`
- `linear` -> `ecsCodeDeploy`

Why this shape:
- Native ECS rolling is the simplest execution mode for standard rolling updates.
- CodeDeploy provides the first-class traffic-shifting semantics required for blue/green, canary, and linear releases.

## Current scope

This slice only models and exposes the execution recommendation.

Tenet does not yet execute AWS rollouts, but the API, controller logs, and UI now show the intended runtime mapping so future controller work can follow a stable contract.
