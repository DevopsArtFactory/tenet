# Tenet v1 Domain Model

## Resource families

Tenet should model deployment state with a small number of durable core resources. All other records are derived from these resources, AWS observations, or event history.

## Core resources

### Account

Represents a target AWS account that Tenet can observe or reconcile.

```yaml
kind: Account
metadata:
  name: prod-core
spec:
  awsAccountId: "123456789012"
  roleArn: arn:aws:iam::123456789012:role/TenetDeployerRole
  observerRoleArn: arn:aws:iam::123456789012:role/TenetObserverRole
  externalIdSecretRef: secrets://tenet/accounts/prod-core/external-id
  allowedRegions:
    - ap-northeast-2
    - us-east-1
  capabilities:
    ec2: true
    ecs: true
status:
  phase: Ready
  conditions:
    - type: AssumeRoleReady
      status: "True"
```

### Environment

Represents a logical environment, bound to a placement.

```yaml
kind: Environment
metadata:
  name: prod-apne2
spec:
  accountRef: prod-core
  region: ap-northeast-2
  networkRef: prod-main
  policyRef: prod-default
status:
  phase: Ready
```

### Application

Represents a deployable service.

```yaml
kind: Application
metadata:
  name: payments-api
spec:
  owners:
    - platform@example.com
  artifact:
    type: container
  health:
    path: /health
    successCodes: "200-299"
```

### Deployment

Represents desired deployment intent for one application in one environment. In the current in-repo model, the compiler also preserves a normalized rollout-config view from this spec so runtime summaries can talk about concrete execution tuning without depending on raw manifest syntax differences. That normalized view currently carries top-level strategy/capacity settings, first-stack-region EC2 launch-template intent (`ami_id` and `instance_type`), and stack rollout knobs such as `termination_delay_rate` and `rolling_update_instance_count`.

```yaml
kind: Deployment
metadata:
  name: payments-api-prod-apne2
spec:
  appRef: payments-api
  environmentRef: prod-apne2
  target:
    type: ec2
  release:
    image: ghcr.io/example/payments-api:1.2.3
    imageDigest: sha256:abc123
    ami: ami-0123456789abcdef0
    configDigest: sha256:def456
  strategy:
    type: blueGreen
    warmupSeconds: 120
    minHealthyPercent: 100
  traffic:
    hostname: api.example.com
    port: 443
  capacity:
    desired: 4
    min: 2
    max: 8
  networking:
    manageLoadBalancer: true
    manageTargetGroups: true
    manageListenerRules: true
    manageSecurityGroups: true
```

### Rollout

Represents a concrete reconciliation attempt of one desired generation.

```yaml
kind: Rollout
metadata:
  name: rol-20260415-001
spec:
  deploymentRef: payments-api-prod-apne2
  desiredGeneration: 17
  desiredRelease:
    imageDigest: sha256:abc123
    ami: ami-0123456789abcdef0
status:
  phase: HealthChecking
  percentComplete: 55
  activeGeneration: gen-16
  candidateGeneration: gen-17
```

## Status schema

A deployment status should be materialized separately from desired spec.

```yaml
status:
  observedGeneration: 17
  placement:
    accountId: "123456789012"
    accountAlias: prod-core
    region: ap-northeast-2
    network: prod-main
  sync:
    state: Reconciling
    reason: CandidateTargetHealthPending
  rollout:
    phase: HealthChecking
    startedAt: 2026-04-15T09:00:00Z
    activeRelease: sha256:old
    candidateRelease: sha256:new
    percentComplete: 55
  health:
    overall: Progressing
    targetGroupHealthy: 3/4
    targetGroupUnhealthy: 1
    ecsTasksHealthy: 0/0
  resources:
    loadBalancerArn: arn:aws:elasticloadbalancing:...
    listenerArn: arn:aws:elasticloadbalancing:...
    activeTargetGroupArn: arn:aws:elasticloadbalancing:...:targetgroup/active
    candidateTargetGroupArn: arn:aws:elasticloadbalancing:...:targetgroup/candidate
    activeAutoScalingGroup: tenet-prod-payments-api-asg-gen16
    candidateAutoScalingGroup: tenet-prod-payments-api-asg-gen17
    securityGroups:
      - sg-aaa
      - sg-bbb
  drift:
    state: NotDrifted
    summary: no out-of-band changes detected
  conditions:
    - type: Ready
      status: "False"
      reason: CandidateNotHealthy
    - type: CandidateReady
      status: "False"
      reason: TargetGroupHealthPending
    - type: AccountReachable
      status: "True"
      reason: AssumeRoleSucceeded
    - type: Drifted
      status: "False"
      reason: NoDriftDetected
```

## Conditions

Recommended condition types:
- SpecValid
- PlacementResolved
- AccountReachable
- AssumeRoleReady
- FoundationReady
- CandidateCreated
- CandidateReady
- TrafficShifted
- Stable
- Ready
- Drifted
- Paused
- Failed

## AWS resource bindings

Tenet should treat AWS resources as bindings derived from durable core objects. A binding record should include:
- internal deployment ID
- rollout ID
- AWS account ID
- AWS region
- resource type
- resource ARN or stable name
- generation role (`active`, `candidate`, `retiring`)
- ownership mode (`managed`, `observed`)

## Driver model

A common deployment driver interface lets EC2 and ECS share orchestration semantics.

```text
type DeploymentDriver interface {
    Plan(spec DeploymentSpec, status DeploymentStatus) (Plan, error)
    Apply(ctx Context, step PlanStep) (StepResult, error)
    Observe(ctx Context, deployment DeploymentRef) (Observation, error)
    Assess(ctx Context, observation Observation) (HealthAssessment, error)
    Promote(ctx Context, deployment DeploymentRef) error
    Rollback(ctx Context, deployment DeploymentRef) error
    Cleanup(ctx Context, deployment DeploymentRef) error
}
```

## EC2-specific generation model

Each generation should be explicit.

Generation fields:
- launch template version
- Auto Scaling Group name
- target group ARN
- desired/min/max capacity
- instance IDs
- health summary
- promotedAt and retiredAt timestamps

## ECS-specific generation model

ECS generations should preserve the same conceptual lifecycle.

Generation fields:
- cluster ARN
- service ARN
- task definition ARN
- optional task set ARN
- desired and running task counts
- target group ARN
- health summary

## Database mapping

Recommended relational tables:
- accounts
- environments
- applications
- deployment_specs
- deployment_statuses
- rollouts
- rollout_steps
- resource_bindings
- resource_snapshots
- conditions
- drift_reports
- events
- audit_logs

## Event model

Tenet should emit events for every meaningful transition.

Examples:
- DesiredSpecDetected
- PlanCreated
- AssumeRoleSucceeded
- CandidateTargetGroupCreated
- CandidateAutoScalingGroupCreated
- TargetHealthUpdated
- TrafficShiftStarted
- TrafficShiftCompleted
- RolloutStabilized
- DriftDetected
- RollbackStarted
- RollbackCompleted

## Naming and tagging

Recommended naming:
- `tenet-{env}-{app}-alb`
- `tenet-{env}-{app}-tg-active`
- `tenet-{env}-{app}-tg-can-{generation}`
- `tenet-{env}-{app}-asg-{generation}`
- `tenet-{env}-{app}-lt`

Required tags:
- `managed-by=tenet`
- `tenet/app=<app>`
- `tenet/env=<env>`
- `tenet/account=<alias>`
- `tenet/rollout-id=<id>`
- `tenet/generation=<gen>`

## Policy hooks

The model should leave space for policy evaluation before dangerous transitions.

Potential policy gates:
- production promotion approval
- quota checks
- region allowlists
- deployment wave ordering
- drift remediation mode
