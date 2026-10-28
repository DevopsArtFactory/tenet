# Reviewed ECS native advanced traffic inputs

This is the input/plan prerequisite for #1801. Blue-Green, Canary and Linear can
now carry a validated `ecs.advanced` policy through compilation, desired
identity, plan persistence and Apply projection. Execution, monitoring and
operator rollback are connected behind the additional advanced-execution opt-in.
Git-change automatic sync also requires the global automation gates and a
per-deployment automatic policy; all native strategies support separately enabled
self-heal after a verified forward Apply. See the
[runtime and recovery runbook](../runbooks/ecs-native-recovery.md) for enablement and ownership semantics.

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
| Capacity | Positive `capacity.desired` within reviewed min/max; native Git count changes and explicit self-heal supported |
| Automatic sync | All native strategies with global opt-ins, scoped automation subject and explicit automatic mode; separate self-heal opt-in after verified forward execution |

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

The helper is connected to advanced execution. The existing live flags enable
Rolling; advanced strategies additionally require
`TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=true`.

References: [ALB prerequisites](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/alb-resources-for-blue-green.html)
and [ELB IAM action scopes](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elbv2.html).

## Native service-update handler prerequisite

The advanced handler now prepares a durable service/configuration/routing
baseline and known service-deployment IDs before one `UpdateService` request.
It passes the reviewed native strategy, traffic/bake policy, alarms and Lambda
hooks to ECS; it never calls CodeDeploy or mutates ALB rules itself. An omitted
alarm/hook policy explicitly disables/clears it in the SDK payload rather than
accidentally preserving an old policy. Task/network/runtime settings remain
inherited from the reviewed task revision and existing service.

The initial handler accepts an existing native service using the same declared
strategy and one ALB binding. It requires `awsvpc`/IP targets and rejects mixed
Service Connect/VPC Lattice routing. Strategy migration is outside this execution
matrix. Reviewed Git capacity changes send the new desired count together with
the native policy/workload. Count-only writes force one new native deployment;
correlation requires its target requested count to match the reviewed intent.
Separately enabled self-heal can restore external capacity drift to the same
Git count verified by a preceding forward Apply on this service/strategy. That
proof is derived from persisted server-created plans and complete checkpoints,
not an API request field. Count/traffic repairs start a fresh native deployment;
active/uncorrelated native work blocks repair. See the
[drift contract](../runbooks/ecs-git-reconciliation.md#execution-and-drift) and
[capacity-repair recovery limitation](../runbooks/ecs-native-recovery.md#recovery-outcome-and-retry).
A configuration-only update forces one new native deployment so the changed
policy has an observable lifecycle; a matching current native deployment can
instead be adopted for read-only verification without another update.

The returned service-deployment ARN and target service-revision ARN are acceptance
identities, not convergence. Correlation describes the new target and source
revisions and checks the reviewed task/configuration against the saved baseline.
Known source revision/task pairs are captured before sending. This distinguishes
actual AWS source workload from a failed desired task still present in service
metadata, and retains the accepted source revision for native recovery.
Existing IDs cannot satisfy a lost response. Missing or multiple matching
candidates remain uncertain; reconciliation only reads and never reports absence
as authority to send again. Current correlation bounds history to 1000 records
with checked pagination. Final observation must still prove lifecycle/traffic,
task health and any rollback outcome.

Concrete capability checks include native service/deployment/revision reads,
service update, inherited task roles, and infrastructure/hook `iam:PassRole` with
`iam:PassedToService=ecs.amazonaws.com` (task roles retain their task principal).
Current RBAC, durable ownership and the final mutation callback remain separate
execution gates. External writers must coordinate on the same service: AWS does
not expose an UpdateService compare-and-swap token.

Production advanced execution remains disabled by default and requires the
additional explicit advanced opt-in. These handlers are verified through fake SDK
clients and local HTTP serialization fixtures; no real AWS deployment was run.

## Native observation prerequisite

The read-only observer now persists a 36-hour default observation deadline and
resumes it across restarts. Each poll reads the exact correlated deployment and
service revision, records AWS lifecycle/status and alarm evidence, resolves the
revision's ALB target group, and checks current configured traffic weights.
Requested traffic percentages remain optional AWS snapshots; they are not
measurements of HTTP traffic or substitutes for current ALB configuration.

A successful forward result requires AWS `SUCCESSFUL`, 100% configured traffic
to the resolved reviewed target, drained source task counts, matching current
service/revision/capacity/configuration and the exact intended healthy task set.
Current ALB target health is matched to sampled task IPv4 addresses and container
ports. Unrelated healthy targets cannot satisfy the check; extra or draining
members prevent completion. The initial matrix therefore uses IPv4 awsvpc/IP
targets. Container-only health and a historical AWS completion status do not
replace current ALB health evidence.

Traffic at 100% during `BAKE_TIME` remains in progress. Missing/throttled/denied
reads, unresolved routing and incomplete task/target health never become healthy
counts. Pending polls back off from two to 30 seconds. The deadline is a Tenet
observation bound, not an AWS deployment timeout or a command to stop ECS.

AWS `STOPPED`, `ROLLBACK_FAILED` and `ROLLBACK_SUCCESSFUL` are distinct forward
failure outcomes. A reported source rollback does not turn the requested new
revision into a successful deployment. Native status, stage, configured traffic
and triggered alarms are durable evidence; meaningful changes enter audit while
timestamp-only polls do not. The existing monitor shows lifecycle and configured
traffic with unknown values preserved rather than rendered as zero.

Production advanced execution uses the opt-in and ownership protocol described
in the runtime runbook. Current tests use fake SDK/HTTP
clients and do not change a real AWS service.

## Native rollback request prerequisite

The native rollback handler prepares immutable recovery identities from an owned
running/failed forward Apply. Its completed registration and service-update
checkpoints must contain the actual pre-write source revision/task pair and a
persisted mutation authorization time. A read-only adopted deployment cannot
supply ownership for stopping someone else's rollout. The source revision is
not guessed from the service's desired task-definition field.

Immediately before a stop, the handler checks the exact current service,
capacity, routing binding, deployment/configuration, immutable forward/source
workloads and deployment history. Newer, concurrent or ambiguously ordered
work blocks the stop. Concrete IAM checks include both the service and exact
service-deployment resources for `ecs:StopServiceDeployment`. Current operator
policy and durable ownership must additionally pass the mutation callback.

Only `PENDING`/`IN_PROGRESS` cause one SDK call with `stopType: ROLLBACK` and SDK
mutation retries disabled. Existing rollback states are observed without a new
stop; incompatible terminal states cannot start recovery. A transport error or
missing/mismatched response remains uncertain. Reconciliation only reads the
fixed deployment; an unchanged active state never proves absence or permits a
blind resend. A rollback state naming a different source revision is rejected.
`STOP_REQUESTED` alone does not prove a rollback rather than an abort.

A stop response or reconciled rollback status confirms request acceptance only.
Even `ROLLBACK_SUCCESSFUL` must be combined with current source traffic, task and
ALB health by the recovery observer before an operator can call it recovered.
The rollback endpoint now creates a separately authorized recovery child with
durable ownership handoff and observation; see the runtime runbook. Rolling
recovery is unchanged.
Tests use injected SDK clients and local HTTP fixtures; no AWS mutation was run.

References: [StopServiceDeployment](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_StopServiceDeployment.html),
[eligible deployment states](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/stop-service-deployment.html)
and [ECS IAM resources](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ecs.html).

## Native source-recovery observation prerequisite

The shared native observer can also observe a separately prepared rollback
request. Recovery retains the original deployment ARN and forward/source
revision identities, but its health goal is the fixed source revision. Requested
and configured traffic are presented relative to that recovery goal; raw AWS
status/stage and rollback revision remain unchanged.

A recovered result requires AWS `ROLLBACK_SUCCESSFUL` naming the captured source,
100% current configured source traffic, drained forward revision counts, the
current service pointing to the exact source workload at the reviewed capacity,
and the complete source task set healthy in both container and ALB evidence.
Historical rollback success alone cannot pass. Missing ALB data, remaining
forward tasks, partial traffic, mismatched immutable revision/task references,
or newer/concurrent deployment history prevent completion. A stopped rollout,
failed rollback and observation timeout remain distinct failures.

ECS may restore the source's previous bake/alarm policy; recovery therefore does
not require the forward policy to remain applied. Existing source tasks can be
observed after task-definition deregistration, since this observer neither
registers that definition again nor launches replacement tasks. Exact workload
identity, current health, capacity and routing are still required.

The recovery observation has its own persisted deadline, which is not extended
on restart. It never modifies the original forward Apply or calls a mutation
API. Fake SDK tests cover all three strategies, source/forward attribution,
partial progress, missing health evidence, failures, supersession and restart.
Production advanced writes require the additional opt-in and use durable
child-Apply ownership handoff; no actual AWS rollout was performed.
