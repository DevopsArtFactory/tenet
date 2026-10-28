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
automatic execution as well: existing same-strategy, fixed-capacity IPv4/ALB
services only. Turning on global sync does not opt a manual deployment into it.
Advanced sync with missing traffic inputs or `selfHeal: true` is rejected.

Absent policy or `mode: manual` requires manual approval. Automatic Git-change
sync supports all four native strategies; self-heal remains Rolling-only. The automation identity is
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
Rolling also supports desired-count changes; advanced service resizing is outside
the initial support matrix and fails preflight. Returning the branch to a previously applied commit after an
intervening successful deployment also creates fresh execution evidence.

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
Apply evidence remain available. For Rolling only, set `selfHeal: true` with automatic mode to
restore managed task-definition ARN, desired count and Rolling percentages.
Tenet reads the exact scoped service using its deployer role, waits out external
rollouts, and creates a distinct plan/Apply tied to the drift signature and last
verified Apply. Repeated observations of one drift do not queue duplicates;
the same external change recurring after a successful repair gets a new repair.
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
change. Automatic drift repair for advanced strategies remains unsupported.
