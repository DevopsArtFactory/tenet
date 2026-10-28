# ECS Git reconciliation

Tenet can poll a configured GitHub ref, create immutable plans and optionally
approve native ECS Rolling, Blue-Green, Canary and Linear Applies. Each cycle resolves the current ref to a
commit SHA. Manifests and repository RBAC come from that SHA; webhook payloads
never select an execution revision. Polling also catches commits when a webhook
is delayed or missed. Invalid/unavailable source or authorization blocks writes.

## Plan and approve manually

Use the GitHub catalog settings in the [execution contract](../architecture/tenet-v1-ecs-execution-contract.md).
Set an explicit `TENET_CATALOG_GITHUB_REF` and `TENET_ECS_RECONCILE=true`.
The default interval is 30 seconds; `TENET_ECS_RECONCILE_INTERVAL` accepts a Go
duration of at least `1s`. Keep the plan and Apply paths on persistent storage.
The loop creates plans for supported native ECS deployments, but does not approve an
Apply unless all automatic gates below are enabled. Manual approval uses the
existing Plan UI/API and requires the live executor, trusted identity and RBAC.

## Opt in to automatic sync

Configure the existing live-execution login, deployer roles and IAM permissions,
then set:

```sh
TENET_ECS_RECONCILE=true
TENET_ECS_AUTO_SYNC=true
TENET_ECS_AUTO_SYNC_SUBJECT=ecs-git-reconciler
TENET_AWS_ENABLE_ECS_EXECUTION=true
TENET_AWS_ENABLE_APPLY=true
```

Grant that exact service subject operator access only to its deployments in
`rbac/policy.json` (or YAML), for example:

```json
{
  "version": "v1",
  "users": [
    {"id": "ecs-git-reconciler", "roles": ["operator"], "deployments": ["worker"]}
  ]
}
```

Add the policy under each deployment's existing `ecs` block:

```yaml
ecs:
  # Keep the existing explicit controller, cluster, service, base revision and container.
  syncPolicy:
    mode: automatic
    selfHeal: false
```

For native Blue-Green, Canary and Linear, additionally set
`TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=true`, retain the reviewed
`ecs.advanced` routing/traffic configuration, and grant the configured subject
operator scope on the exact advanced deployment names. The
[native support matrix and recovery guide](ecs-native-recovery.md) applies to
automatic execution as well: existing same-strategy IPv4/ALB services only.
Turning on global sync does not opt a manual deployment into it.
Advanced sync with missing traffic inputs is rejected. Set `selfHeal: true` only
when automatic restoration of the managed fields below is intended.

Absent policy or `mode: manual` requires manual approval. Automatic Git-change
sync and separately enabled self-heal support all four native strategies. The automation identity is
server configuration, not a request header or an attribution label. Its approval
is recorded separately from human approval. Current repository roles/scopes are
rechecked before mutations. Revoking access, changing the configured subject or
disabling `TENET_ECS_AUTO_SYNC` blocks queued automatic writes. Re-enabling
resumes eligible durable work; it does not silently discard uncertain outcomes.
Read-only reconciliation/verification of an already accepted update may finish
while automatic writes are paused, provided current RBAC still authorizes it.

## Execution and drift

A successful Apply requires registered revision, service acceptance and verified
runtime convergence. Repeated polls, restart and duplicate/out-of-order webhooks
do not reapply the same verified intent. A subsequent Git image or reviewed traffic-policy change creates new work.
All four native strategies support Git desired-count changes. The manifest field
is `capacity.desired`, compiled to the ECS intent's `desiredCount`; it must stay
positive and within `capacity.min`/`capacity.max`. For Blue-Green, Canary and
Linear, a count-only update that requires a service write sends the reviewed
count with `ForceNewDeployment` so it has a fresh native deployment and
revision to correlate and observe. A matching current deployment can instead
be adopted for read-only verification if AWS already has the Git goal; it does
not acquire rollback ownership. An image/count change sends both together.
The declared strategy and traffic policy remain in effect. Request acceptance
is not success: current service count, exact healthy task set, drained sources
and configured target traffic must all converge to the approved goal.
Returning the branch to a previously applied commit after an intervening
successful deployment also creates fresh execution evidence.

Only a positively unsent service update can be superseded by a newly approved
plan. A registered task revision may remain as reusable history. Once an update
might have been sent, the old Apply must reconcile and finish observing before
new work can proceed. Failure keeps the typed intent held; use an explicit retry
or a new desired intent. Existing [Rolling recovery](ecs-rolling-recovery.md)
handles manual/default-off automatic rollback as separate outcomes. Native
advanced failures use ECS alarm/hook rollback and the separate operator recovery
Apply; the reconciler never falls back to a Rolling/CodeDeploy rollback. A
running recovery owns the physical service, and source recovery success does not
become a successful deployment of the failed Git intent. Failure or an operator
handoff holds that typed intent even after a documentation-only commit. Explicit
retry or a different reviewed desired intent is required.

`selfHeal: false` is the default: external AWS changes are not automatically
reverted after a verified success. Existing controller/resource inspection and
Apply evidence remain available. Set `selfHeal: true` with automatic mode after a
verified forward Apply. Rolling restores managed task-definition ARN, desired
count and Rolling percentages. Blue-Green, Canary and Linear restore the verified
workload, Git capacity, reviewed bake/traffic policy, alarms/hooks and configured
production traffic to the current native revision's target group. Target-group
colors may rotate between deployments; an old color is not the recovery goal.

Advanced repair requires the same service, strategy and previously verified Git
capacity. A new image/policy commit or enabling self-heal can retain that capacity
proof. A Git capacity change uses ordinary reviewed execution even with
`selfHeal: false`; its successful verification establishes a new self-heal epoch.
Moving to a different service cannot inherit the old repair scope. A count or
traffic-only repair starts a fresh native ECS deployment, using the declared
strategy and health verification. Tenet never edits ALB weights directly. A
repair whose drift disappears before execution can verify the matching current
deployment without another service update.
Tenet reads the exact scoped service using its deployer role, waits out external
rollouts, and creates a distinct plan/Apply tied to the drift signature and last
verified Apply. Repeated observations of one drift do not queue duplicates;
the same external change recurring after a successful repair gets a new repair.
Advanced observation requires correlated terminal native deployment/revision
history and current ALB configuration. Active external deployments/rollbacks,
pending or undrained source tasks, ambiguous history and failed/stopped work
block repair. Controller/strategy changes, foreign routing bindings and mixed
Service Connect/VPC Lattice are outside the reviewed repair scope.
Unavailable/throttled/denied or incomplete reads cannot authorize self-heal.
Health-only degradation cannot force another deployment.

## Operational boundaries

- Run one desired owner per physical ECS service. Conflicting declarations block
  automatic sync. Coordinate other tools that write the same ECS service.
- API and executor processes must share the same plan/Apply files and stable
  sidecar locks on a local Linux/macOS filesystem. These locks are not a
  distributed backend or network-filesystem coordination mechanism.
- Git ECS approval fingerprints bind source commit, desired intent and placement.
  Runtime timestamp/health refreshes do not invalidate that approval. Live
  preflight, saved service baseline and final mutation guards validate AWS state.
  Recreate unstarted approvals made with the previous fingerprint format.
- Apply convergence is durable execution evidence in the Apply monitor. The loop
  does not write the controller-owned global status store or invent fresh health.
- This flow updates existing services. It does not provision/delete clusters,
  services, target groups or task-definition history.

Tests exercise gates, durable restart/deduplication, branch reversion, safe
supersession, failed-intent holds and drift epochs with fake clients. Run a
separately authorized isolated AWS integration before production rollout.

## Isolated Git-to-ECS verification

Use the same approved disposable fixture and assumed-role checks as the
[Rolling recovery scenario](ecs-rolling-recovery.md#isolated-integration-scenario).
Also require `TENET_RUN_ECS_GIT_INTEGRATION=true` for this separate run. Keep a
fixture-only Git branch, RBAC subject and persistent stores; verify the baseline
before enabling automatic sync. The deployer needs scoped ECS describe/list/task
reads, task-definition registration/tagging, service update and inherited-role
`iam:PassRole`; the preflight identity also needs IAM simulation. Use the exact
capability/resource checks in the execution contract rather than wildcard admin.

1. Commit a digest-pinned image with manual mode. Confirm a ready SHA-correlated
   plan exists and no revision/service write occurs before authorized approval.
2. Enable the automatic gates and fixture-only subject. Commit a new digest;
   verify exactly one forward Apply and its correlated healthy runtime evidence.
   Repeat refreshes and restart after acceptance; confirm no duplicate writes.
3. Commit the approved failing image with `rollbackOnFailure: true`. Verify the
   failed forward record remains failed, the separate recovery succeeds, and
   subsequent polls hold the failed intent. Commit a healthy new digest and
   verify that new commit converges.
4. Change a managed count in the designated fixture. Confirm report-only mode
   makes no repair; then explicitly enable self-heal and verify one repair for
   the observed drift. Revoke service-subject scope and confirm further writes
   block. Do not use an unavailable read as proof of drift or success.
5. Disable automatic sync and both live flags, restore the fixture branch and
   service to the recorded baseline using the approved fixture procedure, and
   remove the fixture-only RBAC subject/token. Preserve audit records; remove
   test-owned task revisions/resources only after confirming nothing uses them.

This scenario documents a manual, explicitly authorized AWS run; normal CI never
runs it and the implementation work did not execute it against a real account.

## Isolated advanced Git-change verification

Use the [gated native AWS runbook](ecs-native-recovery.md#gated-isolated-aws-verification)
in a disposable account, and add the global sync flags, scoped automation subject
and per-deployment `mode: automatic` above. Do not execute these steps against a
production service or with fixture account/role/resource identifiers.

For each native advanced strategy, verify a new digest commit produces one Apply
with the resolved commit, configured strategy and a new native deployment ID.
Repeat the poll/webhook and restart the worker; no extra traffic action should
occur. Push another image during traffic shift/bake and confirm accepted work
finishes observation before the newer intent starts. Only a positively unsent
service update may be replaced by the new commit. Also check branch reversion
creates fresh verification evidence rather than reusing an old successful Apply.

Pause auto sync and confirm queued writes stop while already accepted read-only
observation may finish under current RBAC. Revoke the subject's repository scope
and verify mutation is denied. Exercise an ECS-reported rollback: preserve the
forward failure, keep the same typed intent held across documentation-only
commits, and accept a new fixed digest only after recovery releases ownership.
Disable the flags and use the native runbook cleanup steps after testing.

Fake-SDK tests cover this integration; no actual AWS rollout was run for this
change.

## Isolated advanced capacity-change verification

Use the disposable native fixture and automatic Git settings above. For each of
Blue-Green, Canary and Linear:

1. Keep `selfHeal: false`. Commit `capacity.desired: 4` from a baseline of two,
   then one, within the reviewed min/max. Verify each SHA creates one Apply with
   the correct count, declared native strategy and fresh deployment/revision IDs.
2. Repeat polls and restart after acceptance/lost response. Confirm there is one
   service update; read-only correlation must match the requested count and
   verification must prove the approved count, task/ALB health and source drain.
3. Commit another count during active traffic/bake. Confirm it waits for accepted
   work, which verifies its own pinned count before the newer goal starts.
   Test an image/count change together and reverting to the prior count.
4. Pause sync, revoke scope or change Git before a queued update starts. Verify
   no stale/unauthorized count write occurs. A rejected/failed count goal stays
   held across documentation-only commits; uncertain writes never blindly resend.
5. Enable self-heal after success, introduce external count drift and verify it
   restores the new verified Git count. Coordinate Service Auto Scaling and other
   writers rather than letting multiple controllers fight over desired count.
6. The [operator rollback capacity limitation](ecs-native-recovery.md#recovery-outcome-and-retry)
   also applies to Git count changes. Verify the endpoint rejects mismatched
   source/target capacity; use a separately reviewed Git correction after the
   current native deployment/rollback finishes. Preserve the failed forward
   outcome. Disable flags and clean up using the native runbook.

These are manual, separately authorized AWS checks. Normal tests/CI use fake SDK
clients; this implementation did not execute an AWS rollout.

## Isolated advanced self-heal verification

In the same disposable fixture, complete one healthy verified forward Apply for
each strategy, then enable `syncPolicy.selfHeal: true` with automatic mode.

1. Confirm the default-off policy leaves external changes alone. Change only a
   managed field: same-family workload, desired count, bake/alarm/hook policy or
   configured weights within the reviewed ALB pair. Test each independently.
2. Confirm one drift-bound repair restores the Git goal through a new native
   deployment. Repeat polling and restart after acceptance; check one service
   update, exact revision/traffic/task health evidence and no direct ALB writes.
3. Repeat the same drift after success and confirm a new repair epoch. Make the
   drift disappear before execution and confirm read-only adoption is possible.
4. Confirm external active deployment/rollback, unavailable reads, changed
   routing, revoked scope or paused sync cannot authorize a repair write.
   Health-only degradation must not force deployment.
5. Exercise a rejected/failed repair: subsequent polls hold it. Explicit retry
   retains the verified repair scope; a documentation-only commit cannot retry.
   Review the [capacity-repair recovery limitation](ecs-native-recovery.md#recovery-outcome-and-retry)
   before testing rollback. Clean up with the native runbook procedure.

These are separately authorized manual AWS checks, not steps run by normal CI.
Tests cover these flows with fake clients; this implementation did not execute
an AWS rollout.
