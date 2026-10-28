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
two operations when capacity is unchanged: stop the exact owned ECS service
deployment, then observe source recovery. When the captured pre-update count
differs from the forward Git count, a third operation restores that captured
count between stop acceptance and final health verification. Its identity and
goal are derived from the saved forward checkpoint, never the HTTP body. A queued recovery also blocks competing Applies on the physical
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

Native Git count changes and count-restoring self-heal also support operator
rollback to the **captured pre-update desired count**. ECS owns source-revision
and traffic rollback; Tenet does not assume StopServiceDeployment restores count.
The capacity operation waits for `ROLLBACK_SUCCESSFUL` naming the captured source,
drained forward tasks, current source workload and 100% configured source traffic.
It then sends only `UpdateService(desiredCount: capturedCount)`, without a task
revision/configuration update, forced deployment or ALB write. A count already
restored by ECS is adopted without another update.

The capacity update has its own durable checkpoint and current operator/role/IAM
checks. Lost responses reconcile the fixed source and current count read-only;
an unchanged count never authorizes replay. Explicit AWS rejections follow the
existing bounded retry/failure protocol. Unknown/denied reads, unrelated counts,
newer deployments, changed routing or failed source rollback block capacity
writes. Accepted count reconciliation needs read permissions, not a renewed
UpdateService grant; current repository access is still required.

Capacity restoration and final source health share a persisted 36-hour deadline,
which restart does not extend. Recovery succeeds only after the captured count
of source tasks is healthy in current container/ALB evidence and forward tasks
have drained. A failed/rejected recovery remains separate from its cancelled or
failed parent. The failed forward Git intent stays held; explicit retry or a
different reviewed Git intent is required after ownership is released.

## Opt in to automatic failure recovery

Add `ecs.rollbackOnFailure: true` to the reviewed deployment manifest. The default
is false. This supports native Blue-Green, Canary and Linear as well as the
existing Rolling flow, using their separate recovery handlers. Native advanced
inputs and the three live execution flags above remain required.

```yaml
ecs:
  # Keep the existing reviewed placement and advanced routing/traffic inputs.
  rollbackOnFailure: true
```

Tenet acts on a correlated native deployment alarm reported as `TRIGGERED` with
a reviewed alarm name, or on ECS reporting source rollback requested/in progress/
completed. Configure native deployment alarms/hooks to supply AWS failure
signals. `ecs.advanced.alarms.rollback` controls ECS's own alarm rollback; the
separate `ecs.rollbackOnFailure` flag authorizes Tenet's recovery Apply, including
restoration of the captured pre-update count. If ECS already started rollback,
Tenet observes it without another Stop request.

The worker saves the failed forward outcome first, then approves one recovery
Apply using the original authenticated principal and its reviewed policy. The
recovery's `nativeRollback.automatic` marker is separate from
`principal.automatic`, which identifies Git automation approval. Current RBAC,
strategy/live flags, role and IAM remain mandatory. Git-approved recovery also
requires the global automatic-sync flags and configured service subject. Human-
approved deployment recovery inherits that human's explicit policy and scope.
A new/withdrawn desired intent, paused automation or revoked access blocks new
automatic recovery writes; already accepted read-only observation can finish
under current access. Restore the reviewed policy/gates to resume paused work.

Timeouts, unavailable reads, health-only degradation, foreign/unowned/adopted
updates, stopped deployments, rollback failures and newer/ambiguous history do
not authorize a fresh automatic rollback. Unsupported terminal states require
operator review; Tenet never falls back to Rolling/CodeDeploy execution. An
explicit retry or an existing manual recovery suppresses automatic scheduling.
Failed recovery never recursively creates another recovery. The failed Git
intent remains held after successful recovery until explicit retry or a new
reviewed desired intent.

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
8. Test both two-to-four and two-to-one forward count changes. Roll back each:
   confirm source traffic recovery precedes one count-only update back to two,
   with no new deployment/task registration/ALB mutation. Verify acceptance alone
   does not pass while source tasks are missing, excess or unhealthy.
9. Restart or lose the count-update response. Verify read-only reconciliation,
   no duplicate Stop/count write and an unchanged deadline. Test already-restored
   count, IAM/RBAC revocation, a conflicting count/newer deployment, rollback
   failure and an explicitly rejected capacity request.
10. After recovery is terminal, explicitly retry the reviewed Git intent. Confirm
   a new deployment identity and no concurrent recovery/update calls.
11. Enable `ecs.rollbackOnFailure` on each strategy and repeat scale-out/in
   with a reviewed alarm failure. Confirm one failed forward record and one
   automatic recovery, the original principal/automatic marker, source rollback
   and prior count/health verification. Repeat with ECS already rolling back;
   confirm no duplicate Stop, count write or automatic retry loop after restart.
12. Verify the default-off policy, unavailable/timeout signals, unreviewed alarms,
   changed desired intent, manual recovery/explicit retry, paused Git automation
   and revoked scope cannot authorize automatic recovery writes.
13. Disable the flags, restore the disposable service intentionally, and remove
   test tasks/revisions, ALB targets/rules, hooks and temporary roles only after
   verifying nothing still references them. Preserve Apply/audit evidence.

Local fake-SDK/API tests cover all three strategies, interrupted handoff,
duplicate requests, lost responses, role/RBAC gates and recovery observation.
They do not prove real-account IAM, ALB wiring or application readiness.

## Sandbox integration preparation

Use the [AWS integration scenarios](ecs-native-integration.md) and
[standalone IaC fixture](../../deploy/terraform/ecs-native-integration/README.md)
for future live validation. Normal CI validates this fixture with a mocked AWS
provider only; it never provisions resources or runs these live scenarios.
