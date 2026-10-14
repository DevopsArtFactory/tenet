# Reviewed ECS native advanced traffic inputs

This is the input/plan prerequisite for #1801. Blue-Green, Canary and Linear can
now carry a validated `ecs.advanced` policy through compilation, desired
identity, plan persistence and Apply projection. Live execution, monitoring and
rollback for these strategies are still gated off; the production executor and
automatic reconciler continue to accept Rolling only.

Use the complete [advanced example repository](../../examples/ecs-native-advanced-repo)
for all three strategies. Its account, role and resource identifiers are fixtures;
replace them with explicitly selected existing resources before any future live
use. No resource is provisioned by reading or planning these examples.

## Initial input support

| Input | Tenet contract |
| --- | --- |
| Controller | Explicit `ECS`; native `BLUE_GREEN`, `CANARY` or `LINEAR` |
| Routing | One ALB binding, two distinct target-group ARNs, one production listener-rule ARN; optional distinct test rule on the same ALB |
| Placement | All referenced resources use the service's account, region and partition; IAM role ARNs have no region |
| Roles | Explicit infrastructure role for ECS routing; explicit role per Lambda hook |
| Blue-Green | Explicit post-traffic `bakeTimeInMinutes` |
| Canary | Explicit percentage 0.1–100 in 0.1 increments and phase bake minutes |
| Linear | Explicit step percentage 3–100 in 0.1 increments and minutes between steps |
| Time bounds | Each bake value is 0–1440 minutes; total configured traffic/bake schedule is at most 24 hours |
| Alarms | Optional enabled block with 1–10 unique CloudWatch alarm names and explicit ECS rollback choice |
| Hooks | Same-placement Lambda function pinned to a positive numeric version, explicit IAM role and supported stages |
| Automatic sync | Unsupported for advanced strategies until execution/monitoring/rollback integration is verified |

Omit alarms/hooks to review no alarms/hooks. Zero bake times are explicit values,
not missing settings. A Canary block is invalid on Blue-Green/Linear and vice
versa. Rolling percentage settings cannot be combined with `advanced`.
NLB listeners, Service Connect, VPC Lattice, headless routing and Pause/Continue
hooks are outside this initial contract. The new block rejects unknown fields
in JSON and YAML rather than silently dropping unsupported traffic settings.

Supported Lambda stages are `RECONCILE_SERVICE`, `PRE_SCALE_UP`, `POST_SCALE_UP`,
`TEST_TRAFFIC_SHIFT`, `POST_TEST_TRAFFIC_SHIFT`, `PRODUCTION_TRAFFIC_SHIFT` and
`POST_PRODUCTION_TRAFFIC_SHIFT`. Initial support allows one Lambda per stage.
Test-traffic hooks require a test listener rule. Mutable aliases and unversioned
Lambda references are rejected so the reviewed hook cannot change implicitly.

## Review and compatibility

Native plans persist strategy, percentage/bake settings, ALB ARNs, roles, alarm
policy and hooks in every operation's typed ECS payload. The service-update note
summarizes those inputs and the plan is high risk because it affects live
traffic. AWS deployment/revision evidence will be required to prove actual
traffic progress and convergence; request acceptance cannot supply that proof.
Nested data is cloned so modifying a returned record cannot change another
operation, the compiled catalog or a persisted approval.

Each policy change participates in desired generation, source fingerprints and
stable plan/operation identity. Existing manifests without `advanced` retain
intent-only advanced previews and remain ineligible for live execution. Existing
Rolling manifests and legacy manifests without the native `ecs` block keep their
previous interpretation. Native plans never require CodeDeploy/AppSpec.

The ECS SDK prerequisite separately verifies typed payload serialization and
native deployment/revision/rollback API coverage using a local HTTP fixture.
Execution preflight must additionally validate actual service/controller/routing,
concrete permissions and version availability before any future write. Static
ARN validation is not evidence that a resource exists or is healthy.

AWS references: [deployment configuration](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_DeploymentConfiguration.html),
[Canary](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CanaryConfiguration.html),
[Linear](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_LinearConfiguration.html),
and [ALB infrastructure role](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AmazonECSInfrastructureRolePolicyForLoadBalancers.html).

## Read-only ALB execution prerequisite

The native executor's ALB readiness helper now reads only the reviewed ALB,
listeners, rules and target-group ARNs using an assumed deployer role. It checks
that the ALB is active, both IP target groups use the ALB VPC and reviewed HTTP/S
port/protocol, and listener rules forward only to the reviewed pair. An optional
test rule is distinct and shares the ALB. Redirect/fixed-response routing,
foreign targets/attachments, missing evidence and weighted target-group
stickiness are rejected in this initial support matrix.

The helper returns configured target weights and a signature of routing,
listener and target health-check configuration for a later final mutation fence.
These weights describe ALB configuration, not measured HTTP traffic. This check
does not infer healthy tasks or network reachability. Native service-deployment
and task observation must supply convergence evidence.

The interface exposes no ALB mutation operation: ECS will own traffic changes.
ELB describe actions require `Resource: "*"` in IAM; each actual SDK request still
selects only reviewed ARNs. Permission denial, incomplete data and cancelled
contexts fail closed. Source/observer credentials are never reused as execution
credentials, including same-account preflight.

This helper is a prerequisite and remains unwired to production advanced writes
until service-update, observation and rollback handlers are complete. The
existing live flags still enable Rolling only.

References: [ALB prerequisites](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/alb-resources-for-blue-green.html)
and [ELB IAM action scopes](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elbv2.html).
