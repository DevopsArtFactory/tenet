# Native ECS sandbox E2E scenarios

**Prepared, not live-validated.** The fixture, test app and offline checks do not
prove AWS rollout/rollback behavior. Live steps below require a separately
approved sandbox account, resource creation and mutation window. Normal CI never
runs them. Use all three strategies independently; a passing Canary run is not
evidence for Blue-Green or Linear.

## Setup and evidence

Bootstrap the [dedicated Terraform fixture](../../deploy/terraform/ecs-native-integration/README.md)
after authorization. Build the three [test image digests](../../tests/ecs-native-integration/README.md).
Export the five Git manifests and configure a separate authenticated Tenet
installation with versioned, deployment-scoped operator RBAC and durable local
Apply/plan/audit storage with OS locks. The existing runner role must assume the
output observer/deployer roles. Use a pinned test Git branch and no other AWS
writer/autoscaler for these services. Match region, account and exact service
ARNs to saved Terraform outputs before any live step.

Initially keep automation off. Explicitly enable these flags only on the test
installation when live execution is authorized:

```sh
TENET_AWS_ENABLE_APPLY=true
TENET_AWS_ENABLE_ECS_EXECUTION=true
TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=true
```

Use the existing authenticated UI to preview and approve each manual Apply.
Equivalent API sequence: read deployment detail/source fingerprint; then
`POST /v1/plans/deployments/{name}` with `{"ifMatch":"<fingerprint>"}`;
inspect `GET /v1/plans/{id}`; approve with `POST /v1/applies/plans/{id}`.
Use a real authenticated operator session; actor headers are not authorization.
Do not use `/transitions` to manufacture successful execution.

For every scenario record:

- Run name, strategy, account/region/service, Git SHA and image manifest digests.
- Immutable plan and forward Apply, its operation checkpoints, mutation authorization
  timestamps, registered task definition, native deployment/target/source revision IDs.
- Before/during/after AWS snapshots using `fixture.py collect`, plus relevant
  CloudTrail events (allow for delivery delay), authenticated Tenet audit and logs.
- Any recovery child, parent link, captured source task definition/count and
  terminal outcome. Redact sessions/tokens; retain resource identities for correlation.

Start with two healthy baseline tasks and a terminal successful native revision.
Do not infer this from Terraform completion or `runningCount` alone. If bootstrap
does not yield a usable native source revision, record that as a failed prerequisite
and stop rather than bypassing executor ownership checks.

A scenario passes only after **both Tenet and current AWS evidence agree**:
exact intended/source task definition and count, container health, target health
for each task IP/port, no extra/draining targets/tasks, fully drained previous
revision, matching current service and 100% configured traffic to the resolved
revision's target group. AWS `SUCCESSFUL` or `ROLLBACK_SUCCESSFUL` alone is
insufficient. Target colors rotate: derive the goal group from its service
revision rather than assuming blue is always production. Fetch the marker via
`http://<output ALB DNS>/<strategy>/` from the allowed operator IP as an additional
traffic smoke check; it does not replace target/task evidence.

Use a bounded scenario window (for example 30 minutes for this short-bake fixture).
On expiry record failure/uncertainty and retain evidence; do not extend or reset
Tenet's persisted 36-hour observation deadline to force a pass.

## Scenario matrix

Run each row for **BLUE_GREEN, CANARY, LINEAR** with fresh evidence directories.
Reset to a verified healthy baseline between rows using an explicitly approved
Git change/Retry; never reapply Terraform to erase failed deployment evidence.

| ID | Action | Required result |
| --- | --- | --- |
| N01 | Manual healthy baseline → candidate image, count 2 | One owned native update; strategy-specific traffic progression; candidate healthy at 2; forward Apply succeeds only after bake/drain |
| N02 | Enable Git automation; commit candidate digest | No manual approval; pinned Git SHA and automation subject recorded; one update; healthy success |
| N03 | Commit `capacity.desired: 4`, then separately `1` | Each count change creates/correlates one native deployment with that requested count; exact healthy task/target count and eventual success |
| N04 | Enable selfHeal after verified forward; externally change count 2 → 3 | One owned repair to reviewed 2, native lifecycle and healthy final state; repeated polls create no duplicate repair |
| N05 | Request operator rollback while count-changing forward 2 → 4 is active | One recovery child; fixed captured source revision and count 2 restored; parent handed off, never falsely succeeded |
| N06 | Enable Tenet automatic rollback; trigger reviewed alarm during count-changing forward | Failed parent retained; exactly one automatic child; previous revision and count 2 restored |
| N07 | Enable AWS alarm rollback too; trigger reviewed alarm | AWS rollback adopted by Tenet recovery; no duplicate Stop; captured count restored and verified |
| N08 | Restart worker during forward, then during recovery/count restoration | Same persisted identities/deadlines; no repeated owned mutation; eventual healthy source or explicit uncertainty |
| N09 | Withdraw automatic recovery policy or operator permission before next write | New writes blocked; no unauthorized stop/count restore; accepted read reconciliation may finish under current access |
| N10 | Replay Git notification/polls, then docs-only commit after recovered failure | No duplicate forward/recovery and no recursive recovery; failed typed intent stays held until explicit Retry or different reviewed goal |

## Git automatic sync, counts and self-heal (N02–N04)

Use the [Git reconciliation runbook](ecs-git-reconciliation.md) to configure the
scoped automation subject in repository RBAC. Set on the isolated installation:

```sh
TENET_ECS_RECONCILE=true
TENET_ECS_AUTO_SYNC=true
TENET_ECS_AUTO_SYNC_SUBJECT=<the-reviewed-test-service-subject>
```

Commit `ecs.syncPolicy.mode: automatic` before a reviewed healthy image/count
change. For N04 additionally commit `ecs.syncPolicy.selfHeal: true`, wait for a
verified forward, then use separately authorized sandbox-admin credentials to
change **only** service `desiredCount` to 3. Record that external mutation and
exclude it from Tenet's mutation count. Keep desired Git count 2. Self-heal must
restore 2 with one repair, not report success because an old deployment succeeded.
Stop interference from any other writer. Health-only degradation is not managed
intent drift and must not spawn deployments.

For N03 only change `capacity.desired` (within generated min 1/max 4), leave the
image/config fixed, and wait for each run to terminate before the next commit.
The expected native target `requestedTaskCount` is respectively 4 and 1; a
historical deployment with another count cannot satisfy correlation.

## Source/count rollback and failure injection (N05–N07)

Before each forward save its actual source revision, task definition and
pre-update count 2. Ensure a **new owned** service update occurs; read-only
adoption cannot authorize stopping somebody else's deployment.

For N05 use `POST /v1/applies/{forwardId}/rollback` during `PENDING`/`IN_PROGRESS`.
Repeat the same request to test deduplication. Inspect the child's immutable
source identities and third `RestoreECSServiceCapacity` operation when count
changed. A manual rollback can cancel a running parent's local tracking; that
is not proof AWS recovered.

For N06 first review/commit `ecs.rollbackOnFailure: true`, keeping
`ecs.advanced.alarms.enable: true` and **`alarms.rollback: false`** to distinguish
Tenet's approval from AWS's rollback policy. Then commit the unhealthy image
digest and count 4. The image's health failure alone does **not** authorize Tenet
automatic rollback. During the correlated forward's `PENDING`/`IN_PROGRESS`,
explicitly publish the fixture's custom failure metric with separately authorized
sandbox-admin credentials:

```sh
aws --profile sandbox-admin --region "$REGION" cloudwatch put-metric-data \
  --namespace Tenet/Integration \
  --metric-data "MetricName=Failure,Dimensions=[{Name=Run,Value=$RUN},{Name=Strategy,Value=$STRATEGY}],Value=1,Unit=Count"
```

Use the exact output run/strategy/alarm, confirm the alarm is `ALARM`, and capture
the native deployment's `alarms.status: TRIGGERED` with that reviewed alarm name.
If ECS reaches an unsupported terminal state before Tenet observes an eligible
signal, record the timing/result as a **failed experiment**, not a successful
auto recovery. Do not manually transition a record or stop the deployment to
manufacture N06. Increase reviewed bake durations in a fresh scenario if the
signal window is too short. Missing metric data is non-breaching; before another
run publish Value=0, wait for alarm `OK` and confirm the preceding maximum has
left its period. Do not reuse a still-triggered alarm as a fresh test signal.

For N07 first reset healthy, explicitly review `alarms.rollback: true` as well
as `rollbackOnFailure: true`, and repeat the injection. Native rollback states
are valid Tenet signals; existing AWS rollback must be adopted without another
Stop. Capture a read-only AWS timeline to distinguish this from N06.

For both, require a **failed** forward parent and one automatic recovery child
with the original authenticated principal (Git automatic provenance preserved
when applicable). Final source health/count 2 and exact source revision must
agree in Tenet/AWS. CloudTrail should show at most one accepted owned Stop when
needed, then a count-only UpdateService only if AWS has not already restored 2.
The count update must not force a deployment, register a task definition or
write ALB configuration. Preserve the initial failed desired count 4 in parent
checkpoints. A failed recovery must not recursively schedule another recovery.
Repeat with forward count 1 and unchanged count 2 to cover downscale and the
no-capacity-operation recovery path.

## Restart, authority and ambiguous outcomes (N08–N10)

Restart only the isolated worker, retaining its exact state directory and OS
locks. Record the checkpoint/claim epoch before/after. Test a restart after
accepted Stop and after accepted capacity restoration; exact original deployment
and recovery IDs/deadlines must remain unchanged. Inspect CloudTrail for duplicate
writes. A normal restart is **not** evidence for a deliberately lost SDK response;
transport-loss fault injection remains a separate test requiring an approved
local fault proxy/harness. Do not introduce a production endpoint override here.

For N09 revoke the scoped subject or withdraw `rollbackOnFailure` in Git before
the next unaccepted auto-recovery mutation. Confirm blocked readiness and no
new AWS write. A queued child may keep service ownership; restore the exact
reviewed policy/access to resume rather than deleting its state files. Test
revocation of the fixture deployer's UpdateService permission separately from
already accepted read-only reconciliation. Manual fixed-source recovery has
different Git-change semantics; see the [recovery contract](ecs-native-recovery.md).

For N10 verify active service ownership blocks competing Applies/retries. After
recovery, repeat reconciliation and a docs-only Git commit; parent failure stays
held. Then explicitly request `POST /v1/applies/{forwardId}/retry` with current
reviewed intent and access, or commit a different healthy typed intent to resume.
A read timeout, throttling/permission-denied observation or health-only warning
must not be accepted as an alarm/native-rollback auto-recovery signal. These
negative cases also have offline fake-SDK coverage; do not broaden AWS IAM or
force mutations just to turn an uncertain observation green.

## Result and teardown

Save a result per strategy/scenario: PASS / FAIL / BLOCKED / NOT RUN, start/end
UTC, exact Git SHA and immutable identities, source/forward/final counts, links
to evidence, failure reason and reviewer. `complete.json` is a collection marker,
not a test verdict. The [result sheet](../../tests/ecs-native-integration/results.csv) starts every
strategy/scenario as **NOT RUN**.

After evidence retention, follow the fixture's cleanup instructions under separate
authorization. Stop automation before destroy, wait for native work to terminate,
retain AWS routing role until ECS finishes and inventory task-definition revisions
created by Tenet outside Terraform state. Check costs/resources after cleanup.
