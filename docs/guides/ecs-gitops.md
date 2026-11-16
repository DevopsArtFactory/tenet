# Configure ECS GitOps

This guide takes an existing ECS service through manifest setup, authenticated
manual approval, automatic Git synchronization and failure recovery. Start with
the [local walkthrough](getting-started.md) to learn the UI.

The commands in the live-execution sections enable AWS changes. Select the
account and service you intend Tenet to manage before enabling those flags.

## Prepare the service and identities

Tenet updates an existing ECS service from a pinned base task definition.
Provision the cluster, service, networking, registry and any ALB resources through
your existing infrastructure workflow.

| Required input | Where it is used |
| --- | --- |
| AWS account ID and region | Account/environment manifests and resource placement validation |
| Existing cluster and full service ARNs | The `ecs` block's execution target |
| Base task definition ARN including revision | The template for registering the new workload |
| Container name and image digest | The container image Tenet replaces in that template |
| Deployer role ARN | Assumed by Tenet for preflight, writes and execution observation |
| Observer role ARN | Used for optional AWS resource inspection |
| GitHub repository and explicit ref | Source of desired manifests and repository RBAC |
| Login provider identity | Human approval and read/write authorization |
| Persistent storage directory | Plans, Applies and managed-login state across restarts |

Tenet inherits other containers, roles, runtime settings and secret references
from the pinned task definition. Choose the base revision deliberately when
changing those inherited settings.

The credentials available to the Tenet process must be able to assume the
target roles. The deployer role needs the ECS describe/list/task reads,
task-definition registration/tagging, service updates, inherited task-role
`iam:PassRole`, and IAM simulation permissions used by preflight. Native advanced
recovery also needs `ecs:StopServiceDeployment` and the reviewed ALB reads.
The target role trust policy must admit Tenet's runtime principal.

Use the [execution permission contract](../architecture/tenet-v1-ecs-execution-contract.md#enabling-reviewed-rolling-execution)
and [sandbox IAM example](../../deploy/terraform/ecs-native-integration/iam.tf)
to review actions and resource scopes. The sandbox IAM example is bound to its
Terraform resources and requires adaptation for your account.

## Create a desired-state repository

Copy `examples/ecs-native-repo` into a Git repository you control. Its three
manifest directories contain the following basic shape. Every identifier and
image below is illustrative.

`accounts/production.yaml`

```yaml
name: production
awsAccountId: '123456789012'
roleArn: arn:aws:iam::123456789012:role/TenetDeployerRole
observerRoleArn: arn:aws:iam::123456789012:role/TenetObserverRole
allowedRegions:
  - ap-northeast-2
```

`environments/prod.yaml`

```yaml
name: prod
accountRef: production
region: ap-northeast-2
```

`deployments/worker.yaml`

```yaml
name: worker
appRef: worker
environmentRef: prod
target:
  type: ecs
strategy:
  type: rolling
image: 123456789012.dkr.ecr.ap-northeast-2.amazonaws.com/worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
capacity:
  desired: 2
  min: 1
  max: 4
ecs:
  deploymentController: ECS
  clusterArn: arn:aws:ecs:ap-northeast-2:123456789012:cluster/production
  serviceArn: arn:aws:ecs:ap-northeast-2:123456789012:service/production/worker
  baseTaskDefinitionArn: arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:17
  containerName: worker
  minimumHealthyPercent: 100
  maximumPercent: 200
  syncPolicy:
    mode: manual
    selfHeal: false
  rollbackOnFailure: false
```

Replace the account, region, roles, resources, container and image digest. Keep
the service in the same declared strategy. Check that `capacity.desired` stays
positive and within its min/max.

Add `rbac/policy.yaml` to the same repository.

```yaml
version: v1
users:
  - id: your-authenticated-subject-id
    roles: [operator]
    deployments: [worker]
```

Replace the subject with Tenet's authenticated identity ID. For GitHub login,
this is the numeric GitHub user ID represented as a string. Confirm it in
`GET /v1/operator/context` after signing in. A display name or actor label does
not grant execution permission.

Commit and push the files. For a repository with manifests in a subdirectory,
set `TENET_CATALOG_GITHUB_PATH` to that directory, containing both manifests and
`rbac`. Leave it empty for the layout above.

## Configure GitHub access and login

Set these variables in the environment of the API process. Use your own
repository/ref values. Store credentials in your deployment's secret management
rather than in the manifest repository.

```sh
export TENET_CATALOG_SOURCE=github
export TENET_CATALOG_GITHUB_REPOSITORY=your-org/tenet-desired
export TENET_CATALOG_GITHUB_REF=main
export TENET_CATALOG_GITHUB_PATH=
```

For a private repository, also supply `TENET_CATALOG_GITHUB_TOKEN` with read access
to its contents. Clear `TENET_REPO_ROOT` when selecting the GitHub source.
The catalog token and the login OAuth client serve separate purposes.

For GitHub login, register an OAuth application with a callback matching
`https://your-tenet-host/auth/callback/github`. Supply the client ID and secret
as `TENET_GITHUB_OAUTH_CLIENT_ID` and `TENET_GITHUB_OAUTH_CLIENT_SECRET`.

```sh
export TENET_DATA_DIR=/var/lib/tenet
export TENET_AUTH_PROVIDER=github
export TENET_GITHUB_OAUTH_REDIRECT_URL=https://your-tenet-host/auth/callback/github
export TENET_AUTH_STATE_STORE_PATH="$TENET_DATA_DIR/auth-state.json"
export TENET_AUTH_IDENTITY_STORE_PATH="$TENET_DATA_DIR/auth-identities.json"
export TENET_AUTH_SESSION_COOKIE_SECURE=true
export TENET_PLAN_STORE_PATH="$TENET_DATA_DIR/plans.json"
export TENET_APPLY_STORE_PATH="$TENET_DATA_DIR/applies.json"
```

`TENET_DATA_DIR` is a shell variable used by this guide. Create that directory and
make it writable by the Tenet process before starting it. Mount persistent local
storage there and retain the same paths across restarts. Supply
`TENET_AUTH_SESSION_SECRET` as a stable high-entropy secret and retain it across
process restarts.

Serve the installation through HTTPS. Okta OIDC and Google OIDC are also supported;
Okta SAML currently provides configuration diagnostics. Provider settings and
trusted-front-door requirements are in the [managed-login guide](../features/managed-login.md).

## Approve a first deployment manually

Build the API from the source revision you have reviewed.

```sh
go build -o /tmp/tenet-api ./cmd/tenet-api
```

With the catalog, identity, AWS credentials and storage configured, start the API
behind your HTTPS endpoint.

```sh
TENET_API_ADDR=127.0.0.1:18083 \
TENET_AWS_ENABLE_APPLY=true \
TENET_AWS_ENABLE_ECS_EXECUTION=true \
TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=false \
TENET_ECS_RECONCILE=true \
TENET_ECS_AUTO_SYNC=false \
/tmp/tenet-api
```

The API process runs the executor and Git reconciliation loop. The separate
controller maintains the general runtime status view. Apply checkpoints and
convergence evidence are available in the Apply monitor.

1. Sign in through `/auth/login/github` and confirm access to `worker`.
2. Open its Plan/apply workspace. Git reconciliation can create a manual plan,
   or you can select **Create Plan**.
3. Review the Git SHA, account/region, service, image, count and operations.
4. Select **Apply Plan** when the plan matches the intended change.
5. Open the Apply monitor and follow registration, update acceptance and service
   stability verification. Completion requires current convergence evidence.

For an authenticated API client, the same sequence uses these endpoints.

| Action | Endpoint |
| --- | --- |
| Create a plan | `POST /v1/plans/deployments/worker` |
| Read the saved plan | `GET /v1/plans/{planId}` |
| Approve that plan | `POST /v1/applies/plans/{planId}` |
| Read execution and recovery evidence | `GET /v1/applies/{applyId}` |

Use an authenticated session for these operations. If the source changes before
approval, create a fresh plan. A ready preview establishes reviewed intent;
live preflight separately checks the actual service and permissions.

## Enable automatic Git sync

After verifying a manual deployment, grant a dedicated service subject access
to the specific deployment. Add this entry to the existing policy's `users` list.

```yaml
  - id: ecs-git-reconciler
    roles: [operator]
    deployments: [worker]
```

Change the existing deployment's `ecs.syncPolicy` block.

```yaml
syncPolicy:
  mode: automatic
  selfHeal: false
```

Restart the API with the previous live configuration and these additional
settings. `TENET_ECS_AUTO_SYNC_SUBJECT` must match the policy entry exactly.

```sh
export TENET_ECS_AUTO_SYNC=true
export TENET_ECS_AUTO_SYNC_SUBJECT=ecs-git-reconciler
export TENET_ECS_RECONCILE_INTERVAL=30s
```

Set `TENET_ECS_AUTO_SYNC=true` in the API invocation as well, replacing the manual
example's `false` assignment. Commit a new image digest or `capacity.desired`.
Tenet resolves the configured ref to a SHA, reviews that snapshot, records an
automatic approval and executes the Apply. Repeated polling of the same
completed intent creates no duplicate deployment.

One manifest should own each physical ECS service. Coordinate service autoscaling
and other tools that write desired count with this ownership.

## Use Blue-Green, Canary or Linear

Use the complete
[advanced example manifests](../../examples/ecs-native-advanced-repo/deployments/)
for these strategies. Each requires an existing ECS REPLICA service already
using the declared strategy, IPv4 `awsvpc` tasks, and one ALB binding with two
reviewed IP target groups. Replace the example routing/infrastructure role ARNs,
alarm names and any versioned Lambda hooks.

| Strategy | Manifest value | Additional traffic settings |
| --- | --- | --- |
| Blue-Green | `blueGreen` | Reviewed target pair/rule and bake time |
| Canary | `canary` | Canary percentage and canary bake time |
| Linear | `linear` | Step percentage and step bake time |

Remove the Rolling percentage fields when using `ecs.advanced`, and enable
`TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=true` alongside the two live flags.
ECS performs traffic shifts. Tenet observes deployment/revision history, task
health and configured ALB routing. Review the exact supported bindings and
exclusions in the [advanced input contract](../architecture/ecs-native-advanced-inputs.md).

## Recover, restart and pause

Add `rollbackOnFailure: true` inside the existing `ecs` block to opt into automatic
failure recovery. Advanced recovery requires correlated ECS alarm or rollback
evidence. Configure the reviewed alarms for the failure conditions you need.
A failed forward Apply keeps its failure status and links to a separate recovery
Apply. Recovery verifies the captured source revision, previous count and health.

For operator-requested recovery, use
`POST /v1/applies/{forwardApplyId}/rollback`. After terminal recovery, fix the Git
intent or explicitly request `POST /v1/applies/{forwardApplyId}/retry`.
Tenet holds a failed intent to prevent repeated automatic redeployment of it.
See the [native recovery](../runbooks/ecs-native-recovery.md) and
[Rolling recovery](../runbooks/ecs-rolling-recovery.md) guides for eligibility.

On restart, retain the original plan/Apply files and lock sidecars. A replacement
worker takes over after the stored claim expires; the current lease is five
minutes. It reconciles uncertain AWS outcomes before permitting another write.
A successful API response alone is insufficient to prove healthy recovery.

Set `TENET_ECS_AUTO_SYNC=false` and restart to pause automatic writes.
Authorized read-only observation of accepted work can continue. Disabling the
live execution flags pauses Tenet's processing; ECS can continue its already
accepted deployment independently.

Set `selfHeal: true` only with automatic mode when you want Tenet to repair
supported externally changed fields after a verified forward Apply. Exact fields,
capacity epochs and ambiguity handling are in the [Git reconciliation runbook](../runbooks/ecs-git-reconciliation.md).

## Validation and deployment limits

CI exercises Git-to-Apply, rollback, count restoration and four simulated worker
restarts per advanced strategy against AWS Emulator. IAM readiness and the
failure alarm are fixtures; restart recreates runtime objects and reloads disk
state. The [emulator guide](../../tests/ecs-native-integration/README.md#native-ecs-recovery-in-ci)
explains the commands and assertions.

The [sandbox E2E runbook](../runbooks/ecs-native-integration.md) covers live IAM,
CloudWatch and actual HTTP traffic validation. Live AWS validation is still
pending in the documented project evidence.

Plan/Apply execution uses local file locks and durable JSON files. Use one
persistent local filesystem for cooperating processes. Independently replicated
API instances or network filesystems need a separate coordination design.
Back up the plan/Apply state together and retain the matching desired repository.
