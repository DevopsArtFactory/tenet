# ECS native execution input contract

Tenet accepts an optional `ecs` block on ECS deployment manifests. It binds a
reviewed plan to an **existing** cluster/service and a **pinned base Task
Definition revision**. Native Rolling execution is available behind a separate,
explicit opt-in. Default/local demo execution remains local-safe.

See [the complete example repository](../../examples/ecs-native-repo/) and the
[execution tracking issue](https://github.com/DevopsArtFactory/tenet/issues/1793).

```yaml
name: worker
appRef: worker
environmentRef: prod
target: {type: ecs}
strategy: {type: rolling}
image: registry.example.com/worker:v2
capacity: {desired: 2, min: 1, max: 4}
ecs:
  deploymentController: ECS
  clusterArn: arn:aws:ecs:ap-northeast-2:123456789012:cluster/production
  serviceArn: arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker
  baseTaskDefinitionArn: arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17
  containerName: worker
  minimumHealthyPercent: 100
  maximumPercent: 200
```

JSON and YAML have identical field names. `minimumHealthyPercent` defaults to
100 and `maximumPercent` to 200. An explicit minimum of zero is retained. Rolling
bounds must permit at least one task to start or stop at the desired count,
including ECS percentage rounding. Existing positive capacity constraints remain
in effect; scale-to-zero is not introduced by this contract.

## Validation and inheritance

The controller must be `ECS`. All ARNs must identify the same partition, account
and region; the account/region must also match the environment. The service ARN
must include the configured cluster name. Task Definition ARNs must include a
positive, bounded revision number; unqualified family names and implicit latest
revisions are rejected. A container name and an image reference are required.
URLs containing credentials, malformed digests, invalid bounds and conflicting
minimum-health settings are rejected before a plan can be compiled.

The first supported source is a pinned base revision, not an arbitrary inline
Task Definition document. The requested mutation replaces only the named
container's image and inherits other containers, task/execution roles, networking
mode, runtime resources and secret references from that revision. No secret
values or environment-variable bodies are accepted in the execution contract.

Compilation validates references and syntax without contacting AWS. Before any
registration/update, the executor must read the base revision and existing
service, confirm the container exists, validate inherited runtime/role/network
settings and service compatibility, and check authorization. The executor repeats these checks; a ready plan does not prove that they passed.

Digest-pinned images are preferred. Tag references remain accepted for
compatibility, but the registration operation explicitly warns that their content
is mutable. A plan fingerprint commits to the reference string, not to content
behind a mutable tag; use a digest for reproducible reviewed content.

## Native strategy mapping and compatibility

| Manifest strategy | Native strategy | Execution mode |
| --- | --- | --- |
| `rolling` | `ROLLING` | `ecsNative` |
| `blueGreen` | `BLUE_GREEN` | `ecsNative` |
| `canary` | `CANARY` | `ecsNative` |
| `linear` | `LINEAR` | `ecsNative` |

Adding the explicit `ecs` block selects native ECS intent. Native non-rolling
strategies are never implicitly mapped to CodeDeploy. Their traffic settings,
alarms/hooks, routing prerequisites and execution support are tracked in
[#1801](https://github.com/DevopsArtFactory/tenet/issues/1801); recording their
strategy intent here does not implement or authorize a traffic shift.

Manifests without the block retain their legacy read-only interpretation:
`ecsRolling` for Rolling and `ecsCodeDeploy` for the other ECS strategies. Existing
stored plans remain unchanged. To migrate, add validated explicit ECS inputs and
create a new plan. A `CODE_DEPLOY` controller is not accepted in this new block;
legacy CodeDeploy execution is not silently converted or enabled.

## Plan, Apply and staleness

Native plans contain three reviewed operations: `RegisterECSTaskDefinition`,
`UpdateECSService`, and `VerifyServiceStability`. Each operation stores a copied,
secret-free `ecs` payload with account/region, concrete targets, controller,
strategy, pinned base revision, container/image, desired count and rolling bounds.
The registration summary shows the previously observed image and requested image;
an absent observation is explicitly unknown. Existing service configuration is
also unknown until observed, rather than invented as a before-state.

The same payload and operation kind survive plan persistence, Apply projection,
and lifecycle transitions. Input changes participate in desired generation,
source fingerprints and stable operation/plan identities. Apply uses the existing
source-fingerprint guard to reject stale reviewed intent. Native plans take
precedence over old runtime CodeDeploy step metadata.

This contract does not create clusters/services or build images. Explicitly
enabled Rolling execution registers Task Definitions and updates existing
services. Explicit/approved failure-triggered Rolling recovery is described in the
[recovery runbook](../runbooks/ecs-rolling-recovery.md).

## Apply persistence before live execution

Apply records currently use the JSON `FileStore`; Redis backs other controller
state, not Apply execution. On Linux and macOS all file-store writers share a
persistent `.lock` sidecar using OS advisory locking. The JSON file is replaced
atomically after syncing its contents, and the containing directory is synced
before acknowledging persistence. The sidecar must remain in place while any
process is using the store. A worker exit releases its lock automatically.

Claims, heartbeats and lifecycle transitions reload and validate the latest
record inside the same critical section. New execution checkpoints must use
`MutationStore.Mutate`, not replace an earlier `Store.Update` snapshot. Mutation
callbacks only change local records and must not make network calls while
holding the store lock. Lock waits in context-aware methods are cancellable.

This protects cooperating processes sharing one local filesystem. It does not
provide distributed coordination across independent hosts or network filesystems.
Other operating systems reject file-store writes until an equivalent locking
implementation is provided. Per-deployment execution ownership and AWS response recovery are described below;
file locking alone does not enable live AWS changes.

## Opt-in executor boundary

`applyrequest.Executor.ECS` is a separate, disabled-by-default execution boundary.
It requires both `ECSExecution.Enabled` and `Service.EnableAWSApply`, an injected
handler for each reviewed Rolling operation, a trusted authorization provider,
current desired intent, the immutable plan, the initial source fingerprint and a
verified target-account deployer role. Actor attribution and IAM simulation do
not authorize execution. The authorization provider must re-evaluate an
**authenticated** operator's role and deployment/project/environment grants;
`Record.Actor` or user-supplied actor headers are insufficient evidence.

The API installs the concrete registration, update and observation handlers only
for the separately enabled native ECS executor. `TENET_AWS_ENABLE_APPLY=true`
alone retains the local-safe stub. Advanced native strategies and legacy
CodeDeploy operations are rejected before mutation.

### Enabling reviewed Rolling execution

Use a filesystem desired-state checkout or configured GitHub repository/ref, a
persistent local disk for the plan and Apply files, a valid `rbac/policy.yaml` (or JSON) policy, and either managed
login or an explicitly trusted authenticating front door. The live startup gate
requires both flags:

```bash
TENET_AWS_ENABLE_APPLY=true
TENET_AWS_ENABLE_ECS_EXECUTION=true
```

For a front door, also set `TENET_API_TRUST_AUTHENTICATED_HEADERS=true`; restrict
API network access to that front door and strip incoming identity headers before
setting verified claims. Managed login can instead supply authenticated session
identities without trusting forwarded headers. Live mode always enforces
identity, operator role, deployment read/write scope and repository policy,
regardless of the optional enforcement flags used by the local demo.

For example, bind the authenticated subject ID from your login provider to one
concrete deployment:

```yaml
version: v1
users:
  - id: your-authenticated-subject-id
    roles: [operator]
    deployments: [worker]
```

A new Apply records a trusted `principal` separately from user-editable `actor`
attribution. Its immutable ID binds that principal to the reviewed source plan;
legacy IDs are unchanged when the field is absent. Historical local-safe records
are never upgraded into trusted approvals. Create a fresh reviewed Apply after
enabling live mode. Workers re-read repository policy and placement on each
execution gate; removing the operator role or scope blocks subsequent execution.
Persisted group claims represent the approval-time login, not a live IdP group
membership query; revoke an outstanding approval through repository policy.

The target account must configure a deployer role. Tenet always assumes it,
including same-account calls. Its policy must allow the necessary ECS reads,
registration/tagging, service update, scoped inherited `iam:PassRole`, and
`iam:SimulatePrincipalPolicy` for itself. Permission checks use the concrete
service/family/task-role targets, `ecs:cluster` context where relevant,
`iam:PassedToService=ecs-tasks.amazonaws.com`, and create-time tagging context.
`DescribeTaskDefinition` requires `Resource: "*"` under the ECS IAM action
reference. Missing simulation results, denials, truncation or missing condition
context block execution. Policies with additional request/resource-tag conditions
may require more context than this gate currently supplies; they fail closed.
Simulation is additional evidence; AWS still authorizes every actual API call.
See [IAM simulation](https://docs.aws.amazon.com/IAM/latest/APIReference/API_SimulatePrincipalPolicy.html)
and [ECS action scopes](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ecs.html).

GitHub-backed live execution requires `TENET_CATALOG_GITHUB_REF` and reads all
manifests and repository RBAC from one resolved immutable commit SHA. The branch
head is re-resolved at each gate; only immutable commit content is cached. New
policy revocations are therefore visible without a branch TTL cache, and failed
loads never fall back to an older cached commit. Plans/Applies preserve the
repository/ref/path/revision, and the commit participates in the reviewed source
fingerprint. A moving branch cannot mix account, environment, deployment or RBAC
files from different revisions. Explicit manual approval is still required;
automatic Git reconciliation remains tracked by #1800.
No real AWS mutation is required for local/unit/smoke tests.

A cooperating executor holds OS execution locks for both the deployment name and
physical ECS service throughout a call. Atomic claim/state changes also prevent a
second Apply from becoming running against that deployment/service. A running
Apply keeps that reservation between polls, including after its lease expires:
lease expiry is not evidence that an AWS call failed. Claims and heartbeats use
the existing five-minute lease; a replacement worker recovers expired ownership
under the execution lock. Losing ownership cancels the call context and prevents
subsequent checkpoint or AWS writes. Independent hosts/network filesystems remain
unsupported by this file-backed implementation.

Each operation persists an `in_flight` checkpoint **before** invoking its handler,
with a stable request identity derived from Apply/operation IDs. Concrete handlers must
pass the executor ownership/authorization guard immediately before their SDK
mutation, after discovery reads. The guard durably records mutation authorization.
A new guarded checkpoint without that marker proves that no write was sent and
can resume preparation without inferring absence from eventually consistent AWS
reads. Older checkpoints remain conservative. A crash after authorization but
before the call still requires reconciliation. Only identifiers
are persisted in operation results. A crash after the call but before completion
storage therefore resumes through a read-only `Reconcile`, never blind replay.
Handlers must disable automatic mutation retries in their AWS client, use the
assumed deployer role and perform read-only preflight of concrete inputs/actions,
including scoped `iam:PassRole` for inherited roles. All handlers are preflighted
before the first call; the next handler is checked again before execution.

Confirmed permanent rejection fails the Apply. Other errors and unknown outcomes
retain ownership and reconcile after 30 seconds. A retry requires **positive
evidence of absence**, uses the same request identity, and is limited to three
attempts. Unresolved outcomes remain running with their error/checkpoint visible;
operators must restore authorization/desired intent or resolve the outstanding
AWS operation rather than start an overlapping Apply. Completion and error
transitions retain audit evidence. Manual lifecycle transitions cannot overwrite
an executor checkpoint, and disabling the boundary preserves active checkpoints
for later recovery.

The full preview fingerprint is checked before the first operation. Afterward,
the executor compares current desired ECS intent before each new mutation, allowing
runtime observations from its own rollout to change without invalidating the
Apply. Every new mutation still requires current authorization and deployer-role
readiness. Read-only recovery and rollout observation can finish the previously
approved target after Git intent changes. If Git changes between registration
and an unsent service update, further writes stop; automatic supersession of that
Apply is part of #1800. Completed operation results can be finalized locally after restart
without issuing another AWS call.

A handler may also classify an explicit AWS response as a confirmed, retryable
rejection. That evidence is persisted as `retryable_rejection` and permits a
bounded retry after the same 30-second backoff without guessing whether an
ambiguous registration is absent. Unclassified network/server failures always
remain `in_flight` and require reconciliation.

## Task-definition registration handler

`internal/ecsexecution.Registration` implements the registration operation using
an injected client and a mandatory concrete capability check. Its default client
factory always assumes the deployer role, including same-account execution.
The handler's client interface contains no service-update API. The explicitly enabled API
executor wires it alongside update/convergence handlers.

The handler reads the exact pinned base revision and deep-copies registration
fields, changes only the uniquely named container's image, and preserves other
containers, roles, secrets references, environment, logging, runtime platform,
volumes and resource settings. Response-only metadata is excluded. Inherited
`aws:` tags are omitted, user tags are preserved in deterministic order, and two
tag slots are required for `tenet:request-id` and
`tenet:task-definition-intent`. No task-definition body or secret material is
written to the Apply checkpoint; only ARN, revision, content hash and AWS request
identity are returned.

Preflight checks the pinned base and concrete `ecs:DescribeTaskDefinition`,
`ecs:ListTaskDefinitions`, `ecs:RegisterTaskDefinition`, `ecs:TagResource` and
inherited `iam:PassRole` targets. Both task roles must be in the reviewed account.
The capability checker must account for create-time tagging and
`iam:PassedToService=ecs-tasks.amazonaws.com` conditions; IAM simulation remains
additional evidence rather than a guarantee of AWS acceptance.

Before registering, the handler paginates active revisions in the exact family,
uses recovery tags to find known matching content, and verifies the actual
registration fields before reuse. Equivalent content can be reused by later
Apply requests; a copied tag alone cannot prove equivalence. SDK mutation retries
are disabled. An interrupted response is resolved by the executor's read-only
reconciliation path. A missing list result is treated as uncertain, so delayed
visibility cannot trigger another non-idempotent registration. Confirmed
read failures before registration are safe to retry; confirmed mutation
rejections use the executor's bounded retry policy.

References: [RegisterTaskDefinition](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_RegisterTaskDefinition.html),
[ListTaskDefinitions](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ListTaskDefinitions.html),
[create-time tagging permissions](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/supported-iam-actions-tagging.html),
and [ECS IAM action reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ecs.html).

Discovery scans newest-first and stops at the pinned base revision: a registration
for that request cannot predate its base. Remaining candidate descriptions are
paced to five per second per lookup, with cancellable waits and normal SDK
read retries. Very old pinned bases can still require a long history scan;
no absence is inferred from a truncated scan. Known failures before the mutation
are classified as safe, bounded retries rather than ambiguous registration.

## Rolling service update handler

`internal/ecsexecution.RollingService` updates an existing ACTIVE ECS/REPLICA
service using the exact ARN returned by registration. Native advanced strategies,
CodeDeploy/external controllers, missing services, conflicting active rollouts,
and incompatible network/platform/load-balancer settings fail preflight. The
handler checks service-scoped describe/update capabilities and the registered
revision's inherited PassRole ARNs again before a write.

Before the executor permits `UpdateService`, the handler's read-only `Prepare`
returns the old task-definition ARN, desired count, rolling bounds and primary
deployment ID. The executor persists that snapshot in the in-flight checkpoint.
The handler re-observes the service immediately before updating and rejects a
changed baseline. It sends only the reviewed task-definition/count/rolling bounds,
preserving the existing deployment alarms and circuit breaker, and leaves
networking, load balancers, placement, capacity providers and other service fields
untouched. Existing matching intent returns without forcing another rollout.

Update responses record AWS request/deployment IDs and the old service snapshot.
They complete only the update operation; Apply stays running until the separate
convergence operation verifies the intended revision. Lost responses reconcile
against observed service intent without repeating an uncertain write. A known
failed deployment records a terminal failure; a stale response does not prove
absence. AWS offers no conditional UpdateService comparison, so an external writer
can still race after the final observation. Cooperating Tenet applies are
serialized; external deployment controllers must not manage the same service.

This handler and its rollback snapshot are wired only in the explicitly enabled
native ECS executor. No real AWS updates are used in tests.

Reference: [UpdateService](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_UpdateService.html).


## Restartable rollout observation

`RollingObservation` uses a read-only client to verify the registered revision
and the correlated ECS deployment. It persists its start/deadline before polling
(default 15 minutes), then backs off from two to 30 seconds. Executor scans during
backoff do not contact AWS or IAM; they only refresh owned local leases when
needed. Restarting resumes the same checkpoint and deadline without another
registration or service update. Read calls also inherit the remaining deadline.

Success requires the intended primary deployment, matching service count and
rolling configuration, no pending/previous-deployment tasks, and all intended
RUNNING tasks with their essential containers running. Configured container
health checks must be healthy. AWS rollout completion supplies load-balancer
health evidence; absent completion evidence for a load-balanced service remains
pending. Without configured container/load-balancer health checks, RUNNING task
and scheduler evidence is the available resource-health signal. Zero requested
count is observed as converged only after all running/pending tasks drain; the
current desired-spec validation still requires a positive configured count.

Running-task listing is paginated and descriptions are batched at 100 tasks.
Missing/incomplete task evidence cannot count as success. A bounded stopped-task
sample provides correlated startup failure details without treating a recoverable
individual task failure as a terminal service rollout. Unavailable/throttled reads,
missing tasks, health checks, startup errors, explicit rollback, supersession,
ECS circuit-breaker failure and timeout have distinct runtime reasons. Historical
failures of another deployment using the same revision are ignored.

Old service views can lag an accepted update. They remain pending until there is
correlated failure or a newer primary deployment, rather than being mislabeled
as rollback immediately. The update result preserves the AWS deployment creation
time when available, so supersession can also be recognized before observation
starts. A newer/unrelated deployment is never accepted as success of the
requested deployment.

Latest secret-free evidence is stored in `Record.ECSRuntime` and the operation
checkpoint, projected into the existing Apply monitor and plan/apply workspace.
Counts appear directly; reviewed intent and detailed evidence use the existing
disclosures. Unavailable evidence is not rendered as a healthy task count. Audit
transitions retain meaningful progress/reason changes and terminal proof without
adding repeated timestamp-only progress entries. This does not overwrite the
controller-owned global status store. Production live execution still requires
the trusted approval/authorization startup wiring; the existing Apply flag alone
remains local-safe.

References: [DescribeServices](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_DescribeServices.html),
[ListTasks](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_ListTasks.html),
and [DescribeTasks](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_DescribeTasks.html).


The composed-handler integration also verifies a full no-op: an ACTIVE pinned
base that already contains the reviewed image is reusable without registration,
and a matching service is observed without UpdateService. Initial service
preflight does not mistake an unchanged base ARN for the final revision when the
reviewed image still differs; active conflicting rollout checks still apply.


## Rolling recovery and failed-intent holds

`ecs.rollbackOnFailure` is an explicit, default-off Rolling policy included in
reviewed intent. A separate recovery Apply binds the failed forward Apply,
registered revision/deployment identity, previous service snapshot and exact
previous ACTIVE target. The original failed record remains unchanged. Recovery
uses the same authorization, service lock, durable mutation guard and observer;
registration is prohibited in this mode. An already matching ECS rollback is
observed without another update.

Normal Apply creation and mutation gates hold previously failed typed intent even
when runtime fingerprints/new plan IDs change. An explicitly approved retry gets
its own `retryOf` identity and a fresh source plan; a new Git intent can instead be
reviewed normally. Recovery and retry creation share the service execution lock.
Read [ECS Rolling recovery](../runbooks/ecs-rolling-recovery.md) for API routes,
policy/failure criteria, concurrency, audit and the gated integration scenario.
