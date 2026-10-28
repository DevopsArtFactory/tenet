# Native ECS advanced Apply and recovery

Blue-Green, Canary and Linear use the native ECS controller with one reviewed
ALB target pair. Tenet registers the reviewed workload, submits the strategy to
ECS and observes native deployment/revision IDs, lifecycle, configured ALB
traffic, container health and current target health. ECS owns traffic shifting;
Tenet does not write ALB rules or call CodeDeploy.

## Explicit enablement

Keep default flags off for inspection/demo environments. For an intentionally
selected test installation, configure all three:

```sh
TENET_AWS_ENABLE_APPLY=true
TENET_AWS_ENABLE_ECS_EXECUTION=true
TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=true
```

The advanced flag requires the other two. Startup still requires managed login
or a trusted authenticated front door and versioned repository RBAC. Approvals
and each operation recheck the stored authenticated subject's **current**
operator permission and deployment scope. Actor attribution headers do not
provide execution authority. All ECS and ALB clients assume the verified target
account deployer role, including same-account calls.

Advanced execution accepts explicit operator approval or separately enabled
[Git-change automatic sync](ecs-git-reconciliation.md#opt-in-to-automatic-sync).
Automation additionally requires the configured scoped service subject and each
deployment's `syncPolicy.mode: automatic`. All native strategies also support
separately enabled `syncPolicy.selfHeal: true` after verified forward execution;
see [drift semantics](ecs-git-reconciliation.md#execution-and-drift).
The advanced flag enables both new work
and recovery/observation for these strategies; disabling live execution pauses
Tenet's processing and does not stop a deployment already running in AWS.

The initial support matrix is existing ECS REPLICA services already using the
same declared strategy, a pinned task-definition family, IPv4
`awsvpc` tasks, and one ALB binding with two reviewed IP target groups. Strategy
migration, NLB, Service Connect, VPC Lattice and Pause/Continue hooks
are unsupported. Use the [input contract](../architecture/ecs-native-advanced-inputs.md)
and replace example account/region/role/resource identifiers before live use.

## Operator rollback and ownership transfer

Send `POST /v1/applies/{forwardApplyId}/rollback` with your authenticated operator
session. The request body cannot select a task definition, source revision,
account or deployment ARN. The response is the separate recovery Apply, with a
`Location` header. Repeated requests return that same child.

The forward Apply must be running or failed with complete registration and
owned service-update checkpoints. Its actual source revision/task pair must
have been captured before the update, and the mutation authorization timestamp
must be present. A read-only adopted deployment cannot authorize a stop. Before
creating recovery, current AWS service, capacity, routing, immutable revisions,
deployment history and permissions are checked. Unsupported terminal states,
newer/concurrent deployments and missing evidence block the request.

Under the same deployment/physical-service OS lock used by workers, Tenet saves
the immutable recovery intent first, then links the original Apply through
`nativeRollbackRequestId`. A running forward Apply becomes **locally cancelled**
and releases its worker claim. This means its tracking was handed to recovery;
it does not claim ECS stopped or recovered. An already failed forward Apply
keeps its failure and operation evidence.

If the process stops between the two file writes, the saved recovery reserves
the service. The old owner cannot resume forward work. A subsequent request or
worker repairs the parent link before claiming the child. The child contains
two operations: stop the exact owned ECS service deployment, then observe source
recovery. A queued recovery also blocks competing Applies on the physical
service. Native recovery records cannot be manually transitioned around the
executor. Use durable storage with working local OS file locks; independent
copies of the Apply store cannot coordinate ownership.

The stop operation issues one `StopServiceDeployment(ROLLBACK)` with SDK retries
disabled, immediately after a fresh authorization/role/claim check. A lost or
mismatched response enters read-only reconciliation against that fixed ARN.
An unchanged active deployment never proves the request was unsent and never
authorizes blind replay. If the forward deployment finishes before an unsent stop
can execute, recovery fails explicitly without sending Stop or retaining a stuck
queued owner. Existing ECS rollback states can be observed without
another stop. Fixed-ARN recovery can continue after the Git intent changes;
it still requires current repository policy, a matching verified deployer role
and no intervening AWS deployment.

## Recovery outcome and retry

Stop acceptance is not healthy recovery. The child succeeds only when AWS
reports `ROLLBACK_SUCCESSFUL` to the captured source, configured production
traffic is 100% on that source, forward tasks have drained, the current service
references the exact source workload/capacity and the complete source task set
is healthy in both container and ALB evidence. Missing/denied data remains
unavailable; it is never converted into healthy counts. Rollback failure,
stopped deployment, supersession and the persisted observation deadline produce
distinct outcomes. Restarting does not extend the deadline.

Inspect `GET /v1/applies/{recoveryApplyId}` or the Apply monitor/workspace. The
original record links to the recovery record, which links back to its parent.
A successful recovery does not rewrite the requested forward deployment as a
success. After the recovery is terminal, `POST /v1/applies/{forwardApplyId}/retry`
requests a fresh plan and explicit approval of the current Git intent. Active
recovery blocks retries; the original failed/handed-off intent remains held
until explicit retry or a different reviewed intent.

Native Git sync supports reviewed `capacity.desired` changes, and self-heal may
restore external drift to the last verified Git count. The current operator
rollback endpoint requires equal pre-update and reviewed capacity, so a Git count
change or count-restoring repair with different source capacity cannot use that
endpoint. ECS owns alarm/hook rollback; inspect its actual
capacity and outcome before further action. A failed repair stays held until an
explicit retry or different reviewed intent. A separately reviewed Git count
correction can run after the current deployment/rollback releases ownership;
Tenet does not assume StopServiceDeployment restores the old task count.

## Gated isolated AWS verification

This runbook is intentionally **not executed by tests or CI**. Select a disposable
account/service, reviewed digest and least-privilege roles before enabling the
flags above. Confirm no independent actor writes this service during the test.
Do not use example production-looking ARNs as real targets.

1. For each strategy, create a plan and inspect its task/image, traffic/bake
   settings, ALB pair/rules, infrastructure role, alarm and hook policy.
2. Approve once. Verify the registered task revision and returned service
   deployment/revision identities in AWS. Confirm no CodeDeploy resource exists
   as a prerequisite and no unmanaged service/network field changed.
3. During an active traffic/bake phase, request rollback. Verify one recovery
   Apply, the parent link and preserved forward checkpoints. Submit the same
   request again and confirm the same child ID.
4. Restart the worker during accepted stop/recovery. Confirm it reclaims the
   durable child without another stop and preserves its observation deadline.
5. Check native lifecycle, source revision, current ALB configuration, exact
   source task health and target registration. Confirm `ROLLBACK_SUCCESSFUL`
   alone is insufficient when current task/ALB evidence is incomplete.
6. Exercise an alarm/hook failure and already-started ECS rollback. Confirm the
   forward failure remains visible and operator recovery observes the existing
   AWS rollback without issuing another stop.
7. Verify revoked RBAC, disabled flags, wrong account/role, changed routing,
   newer deployment and missing AWS evidence prevent further mutation. Re-enable
   only after reviewing the current service and explicit recovery approval.
8. After recovery is terminal, explicitly retry the reviewed Git intent. Confirm
   a new deployment identity and no concurrent recovery/update calls.
9. Disable the flags, restore the disposable service intentionally, and remove
   test tasks/revisions, ALB targets/rules, hooks and temporary roles only after
   verifying nothing still references them. Preserve Apply/audit evidence.

Local fake-SDK/API tests cover all three strategies, interrupted handoff,
duplicate requests, lost responses, role/RBAC gates and recovery observation.
They do not prove real-account IAM, ALB wiring or application readiness.
