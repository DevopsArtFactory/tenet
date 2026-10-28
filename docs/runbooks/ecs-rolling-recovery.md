# ECS Rolling recovery

Recovery restores the exact Task Definition ARN, desired count and rolling bounds
captured before a failed native Rolling update. It creates a separate Apply with
`recovery.applyId` pointing to the failed forward Apply. The forward result stays
failed even when recovery succeeds. Task-definition history, services and clusters
are never deleted.

This requires the [live ECS execution configuration](../architecture/tenet-v1-ecs-execution-contract.md#enabling-reviewed-rolling-execution).
The default local demo does not enable AWS mutation.

## Explicit recovery

Open the failed Apply in its plan/apply workspace. Review the previous revision,
task count and rolling bounds, then choose **Restore previous revision**. The
button is unavailable without a persisted pre-update baseline. The API equivalent
is `POST /v1/applies/{failedApplyId}/rollback`, using the authenticated operator's
normal session or trusted front-door identity. The server derives the target from
durable failure evidence; callers cannot supply a replacement ARN or count.

The requesting operator must currently have repository write permission and scope
for the deployment. The executor repeats authorization, desired-intent, concrete
capability and lease checks before writing. The recovery target must remain
ACTIVE and include the reviewed container. Mutable image tags remain mutable;
use digest-pinned image references for reproducible image content.

Recovery reuses the exact previous revision without registering a replacement.
Before `UpdateService`, it verifies that the primary deployment still has the
failed Apply's task-definition ARN **and deployment ID**. Tasks belonging to an
intervening revision block the update. A known failed rollout's pending work can
be replaced; unrelated active work cannot. The pre-recovery snapshot is persisted
and checked again immediately before the call.

If ECS already restored the exact previous task/count/bounds, recovery performs
no additional update and verifies that revision's health instead. If ECS restored
only part of the configuration, or another actor changed the primary deployment,
Tenet blocks further mutation rather than guessing whether it may overwrite that
change. Missing deployment identity also blocks a new rollback write. Investigate
the current service and its deployment history before changing desired intent.

Acceptance of an update is not recovery success. The existing restartable
observer verifies the exact recovery deployment, essential-container health,
service/task counts, absence of old/pending tasks and ECS rollout completion.
A previous count of zero is preserved and verified after tasks drain.

## Approved automatic recovery

Set this in the reviewed native ECS manifest before creating the forward Apply:

```yaml
ecs:
  # Existing explicit controller/cluster/service/base/container fields still apply.
  rollbackOnFailure: true
```

It defaults to `false`. This section describes Rolling; Blue-Green, Canary and
Linear use the [native automatic recovery flow](ecs-native-recovery.md#opt-in-to-automatic-failure-recovery).
The reviewed plan shows the automatic-recovery policy and binds it into the intent/fingerprint.
A fresh repository policy evaluation must still authorize the approving principal
when recovery is scheduled and executed. The manifest's current intent must still
match the failed forward intent.

Automatic recovery runs only for correlated ECS `rollout_failed` or
`rollback_observed` evidence. Unavailable/throttled AWS reads, task startup warnings,
observation timeout and external supersession do **not** authorize an automatic
rollback. An operator can explicitly request recovery after investigating a
failure or timeout if its correlation and baseline are still valid.

Per-service locks make simultaneous operator/automatic requests resolve to one
recovery Apply. Restart/lost responses reconcile stored checkpoints without
repeating ambiguous updates. A failed recovery is terminal and never recursively
triggers another rollback. Both failure and recovery outcomes remain visible in
Apply Monitor, the workspace and audit metadata (`recoveryOf`, `recoveryAutomatic`).

## Failed desired intent and explicit retry

Recovery does not rewrite Git to the previous image. The failed forward intent
is held: a fresh runtime observation or new plan ID cannot automatically redeploy
the same typed ECS intent. This check runs at creation and again before mutation,
so a request queued before the failure cannot bypass it.

Either change Git deployment intent and review a new plan, or use **Approve retry
of Git intent** (`POST /v1/applies/{failedApplyId}/retry`). An explicit retry loads a
fresh plan, requires the same failed typed intent, re-authorizes the operator and
creates a separate Apply linked by `retryOf`. Duplicate requests return the same
retry; retrying a subsequently failed attempt uses that attempt's ID. If the
requested revision is already current but its correlated primary deployment
failed, the explicit retry may issue one `ForceNewDeployment` update. It waits
for a new deployment identity; the old failed primary cannot prove acceptance.
A healthy/matching service does not get an unnecessary forced rollout. A running or
queued recovery must finish before a retry can be approved. Normal Apply creation
returns `409` with an explicit retry requirement for held intent.

## Isolated integration scenario

All automated unit, race and smoke tests use fake clients; they do not change AWS.
Run a real integration scenario only in an explicitly designated disposable AWS
account/service after approving its account, role, service and failure fixture.
Never point the scenario at a production service.

Before enabling either live flag, gate the run on all of these concrete checks:

1. `TENET_RUN_ECS_RECOVERY_INTEGRATION=true` is explicitly set for this run, and the
   target account/service ARN exactly matches the approved isolated fixture.
2. Record `sts:GetCallerIdentity`, the assumed deployer role, current service
   revision/count/bounds and one healthy task's image digest. Use persistent,
   isolated plan/Apply paths and an operator scoped only to this service.
3. Confirm the healthy pinned baseline remains ACTIVE. Prepare a separate,
   deliberately failing image revision and enable `rollbackOnFailure` only in the
   fixture manifest. Keep external deployment writers disabled for the fixture.
4. Review and Apply the failing intent. Verify the forward Apply becomes failed,
   the linked recovery restores the saved revision/count/bounds, and its observer
   confirms the baseline is healthy. The forward audit row must remain failed.
5. Repeat with recovery policy disabled and use explicit rollback. Restart the
   executor after acceptance to verify recovery without another update. Test an
   already ECS-restored baseline and confirm no extra rollback update is sent.
6. Change the primary deployment in a separate approved fixture case and verify
   Tenet blocks overwrite. Revoke operator scope and verify no mutation. Observe a
   recovery failure separately and verify it creates no recursive recovery.
7. Verify held intent cannot redeploy through a fresh plan. Approve an explicit
   retry only after restoring the fixture's prerequisites, then return Git to the
   healthy digest and disable both live flags when the scenario ends.

The real-AWS scenario is intentionally gated and is not part of normal CI.
