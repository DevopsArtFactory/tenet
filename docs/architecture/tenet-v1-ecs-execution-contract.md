# ECS native execution input contract

Tenet accepts an optional `ecs` block on ECS deployment manifests. It binds a
reviewed plan to an **existing** cluster/service and a **pinned base Task
Definition revision**. This is an input/planning contract, not a live executor.
Apply remains local-safe and stops before AWS mutation.

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
future registration/update, the executor must read the base revision and existing
service, confirm the container exists, validate inherited runtime/role/network
settings and service compatibility, and check authorization. These checks are
part of the subsequent executor/registration issues; a ready plan does not prove
that they passed.

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

This contract does not create clusters/services, build images, register Task
Definitions, update ECS, or execute rollback. The current Apply executor boundary
continues to prevent live AWS mutation, including when apply is enabled.

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
implementation is provided. Per-deployment execution ownership and AWS response
recovery remain follow-up work in #1795; file locking alone does not enable live
AWS changes.

## Opt-in executor boundary

`applyrequest.Executor.ECS` is a separate, disabled-by-default execution boundary.
It requires both `ECSExecution.Enabled` and `Service.EnableAWSApply`, an injected
handler for each reviewed Rolling operation, a trusted authorization provider,
current desired intent, the immutable plan, the initial source fingerprint and a
verified target-account deployer role. Actor attribution and IAM simulation do
not authorize execution. The authorization provider must re-evaluate an
**authenticated** operator's role and deployment/project/environment grants;
`Record.Actor` or user-supplied actor headers are insufficient evidence.

The API startup still installs no live ECS handlers. `TENET_AWS_ENABLE_APPLY`
alone continues to use the existing local-safe path. Concrete AWS clients,
registration, service updates and convergence observation are supplied by the
following implementation issues; this boundary is currently verified with fake
handlers. Advanced native strategies and legacy CodeDeploy operations are rejected
before any handler mutation.

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
with a stable request identity derived from Apply/operation IDs. Only identifiers
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
the executor compares current desired ECS intent to the reviewed intent, allowing
runtime observations from its own rollout to change without invalidating the
Apply. Every new mutation still requires current authorization and deployer-role
readiness. Completed operation results can be finalized locally after restart
without issuing another AWS call.

A handler may also classify an explicit AWS response as a confirmed, retryable
rejection. That evidence is persisted as `retryable_rejection` and permits a
bounded retry after the same 30-second backoff without guessing whether an
ambiguous registration is absent. Unclassified network/server failures always
remain `in_flight` and require reconciliation.
