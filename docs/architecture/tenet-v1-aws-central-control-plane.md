# Tenet v1 AWS Central Control Plane

This document explains how to run Tenet as a centrally managed control plane on AWS and how that model changes between single-account and multi-account environments.

It is intentionally practical:
- where Tenet itself should live
- what should connect to what
- how target accounts should trust the central Tenet runtime
- what is already implemented today versus what is still future execution work

## Truth in advertising

Tenet today is still read-heavy and intentionally conservative.

Implemented today:
- repository-backed desired-state loading
- catalog compilation and validation
- `tenet-api` and the embedded operator UI
- `tenet-controller` runtime/status reconciliation loops
- filesystem or Redis runtime-state storage
- read-only AWS-backed inspection surfaces such as recent logs for supported resource types
- cross-account read access through the API process when AWS credentials plus `observerRoleArn` are configured

Not implemented yet:
- full live AWS mutation/execution
- a production-hardened write path that assumes `roleArn` and performs rollout changes in target accounts
- complete apply/job orchestration across accounts

So when this document talks about `roleArn`, treat it as the stable account contract for future mutation/execution. When it talks about `observerRoleArn`, that is relevant to the current read-only AWS inspection path today.

## Recommended top-level shape

The default production model should be a central Tenet deployment.

At a high level:
- one AWS account hosts the Tenet control plane
- that account runs `tenet-api`, `tenet-controller`, and Redis
- target workload accounts keep owning ECS services, EC2 Auto Scaling Groups, load balancers, and related resources
- Tenet reads desired state from a repo and joins that with runtime status plus optional AWS observations
- operators access one central UI/API instead of logging into each target account separately

## Central control-plane components

A practical AWS deployment should contain these pieces.

### 1. Control-plane runtime

Run these centrally:
- `tenet-api`
- `tenet-controller`
- Redis

Typical AWS placements:
- ECS on Fargate for the first production deployment
- EKS if you already standardize on Kubernetes
- EC2 only if you already have a strong VM-based platform pattern and do not want another scheduler

For a first centralized deployment, ECS/Fargate is the simplest operational default.

### 2. Desired-state source

Tenet needs one repository-shaped source of truth containing:
- `accounts/`
- `environments/`
- `deployments/`

Tenet selects desired-state reads with `TENET_CATALOG_SOURCE`:
- `filesystem` uses `TENET_REPO_ROOT` to point `tenet-api` and `tenet-controller` at the same checked-out repo or synced repo tree
- `github` uses `TENET_CATALOG_GITHUB_*` so both services read the same desired-state manifests directly from GitHub in read-only mode

The shipped Terraform/bootstrap path in this doc still uses repo-sync plus shared filesystem delivery today because that works for both services now. That repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL. Private desired-state repo authentication is not wired into this module yet.

### 3. Runtime state store

Use Redis for the central deployment.

Recommended production setting:
- `TENET_STATUS_BACKEND=redis`
- `TENET_REDIS_ADDR=<redis endpoint>`

Keep filesystem-backed status only for local/demo paths. A central AWS deployment should treat Redis as the shared runtime-state backend between `tenet-api` and `tenet-controller`.

### 4. Operator ingress

Expose only the API/UI entry point to operators.

Recommended pattern:
- internal or internet-facing ALB in front of `tenet-api`
- TLS termination at the ALB
- authentication handled by your chosen front door or identity layer before the request reaches Tenet

Tenet's shipped control-plane runtime consumes trusted forwarded identity headers from that layer. It does not currently run a built-in GitHub OAuth browser login, issue Tenet-managed browser sessions, or manage an OAuth/OIDC provider lifecycle inside `tenet-api`.

The UI is embedded in `tenet-api`, so you do not need a separate frontend service.

If you enable `TENET_API_TRUST_AUTHENTICATED_HEADERS=true`, `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY=true`, and/or `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION=true`, keep that `tenet-api` instance behind the same authenticating front door. The proxy/load balancer layer must strip any inbound `X-Tenet-Authenticated-*` headers and `X-Tenet-Authorized-Deployments` from untrusted clients and then add its own trusted forwarded identity plus deployment-allowlist values, or callers can spoof operator identity or deployment access by sending those headers directly.

### 5. Secrets and credentials

The central runtime needs AWS credentials through the normal AWS SDK default chain.

In practice, that means one of:
- task role on ECS
- IRSA/service account role on EKS
- instance profile on EC2

For cross-account read access today, that base runtime identity must be able to assume each target account's `observerRoleArn` when the target account differs from the control-plane account.

## Connection model

The easiest way to think about Tenet on AWS is as five connection paths.

### A. Operator -> Tenet

- browser or API client connects to ALB
- ALB forwards to `tenet-api`
- `tenet-api` serves both JSON APIs and the embedded UI

### B. Tenet API/controller -> desired-state source

- both services read the same desired-state manifests, either from a shared repo tree in filesystem mode or from the same GitHub source envs in GitHub mode
- the shipped Terraform bootstrap below uses the shared repo tree path at `/workspace/repo`

### C. Tenet API/controller -> Redis

- `tenet-controller` writes runtime status/history to Redis
- `tenet-api` reads the same Redis-backed runtime state to render the UI/API

### D. Tenet API -> target AWS accounts for read-only inspection

This is the currently implemented AWS connection path.

For supported inspection features such as recent logs:
- `tenet-api` loads AWS config via the AWS SDK default chain
- if the target account is the same as the runtime's caller account, it can use the base runtime identity directly
- if the target account differs, Tenet attempts to assume `observerRoleArn`
- if cross-account access is needed but `observerRoleArn` is missing, Tenet returns an explicit unavailable state instead of pretending inspection succeeded

### E. Future Tenet controller/executor -> target AWS accounts for mutations

This is the intended future path.

When Tenet grows live rollout execution:
- the central controller or a dedicated executor should assume `roleArn` in the target account
- mutations should happen in the target account while Tenet itself remains centralized
- the control-plane account should keep the orchestration logic, audit surface, and operator UI

## Single-account environment

A single-account environment means the Tenet runtime and the workload resources live in the same AWS account.

Example shape:
- account `platform-prod` hosts `tenet-api`, `tenet-controller`, Redis, ALB, and the workloads Tenet is observing

### Why use this mode

Use single-account mode when:
- you are starting with one AWS account
- you want the simplest production rollout for Tenet itself
- your platform team does not need cross-account trust yet

### Recommended wiring

- deploy Tenet into the same VPC or a peered/shared-services VPC in the same account
- give the Tenet runtime one AWS identity with read access to the services it needs to inspect
- keep `observerRoleArn` optional; same-account reads can usually use the runtime identity directly
- still populate `roleArn` in account specs so the account contract stays stable when write-side execution is introduced later

### Single-account request flow

- operator opens the central Tenet URL
- `tenet-api` reads desired state from the repo
- `tenet-api` reads runtime state from Redis
- for AWS-backed inspection, `tenet-api` uses its own runtime identity directly against ECS / EC2 / CloudWatch Logs in the same account

### Suggested account spec

```json
{
  "name": "platform-prod",
  "awsAccountId": "123456789012",
  "roleArn": "arn:aws:iam::123456789012:role/TenetDeployerRole",
  "observerRoleArn": "arn:aws:iam::123456789012:role/TenetObserverRole",
  "allowedRegions": ["ap-northeast-2", "us-west-2"]
}
```

In a pure single-account deployment, `observerRoleArn` can be the same account and may even be omitted for today's read-only inspection paths if the runtime identity already has the needed permissions. Keeping it present is still a good long-term contract.

### What to provision

- one ECS/Fargate service or equivalent for `tenet-api`
- one ECS/Fargate service or equivalent for `tenet-controller`
- one Redis deployment (ElastiCache/MemoryDB or self-managed Redis)
- one ALB
- one task role / runtime IAM role for Tenet
- one repo delivery mechanism into the runtime filesystem

## Multi-account environment

A multi-account environment means Tenet lives in one central account and observes or later reconciles workloads in other AWS accounts.

This should be the default long-term operating model.

Example shape:
- `tools-control` account hosts Tenet itself
- `prod-core`, `staging-core`, `sandbox-core` accounts host workloads

### Why use this mode

Use multi-account mode when:
- you already separate prod/staging/shared services by account
- you want one operator surface across many workload accounts
- you want Tenet to remain centralized while application resources stay owned by target accounts

### Recommended wiring

- deploy Tenet in a dedicated control-plane account
- give the Tenet runtime a base IAM identity in that control-plane account
- in each target account, create a read-only `TenetObserverRole`
- later, in each target account, create or keep a separate `TenetDeployerRole` for future mutation/execution
- trust the control-plane runtime identity to assume those roles
- keep account manifests in the repo aligned with the real role ARNs and account IDs

### Multi-account request flow today

- operator opens the central Tenet URL in the control-plane account
- `tenet-api` serves the UI/API from the control-plane account
- `tenet-api` reads desired state from the central repo checkout
- `tenet-controller` writes runtime status into central Redis
- for AWS-backed inspection against a target account, `tenet-api` uses its base runtime credentials, sees that the target account differs, and assumes that account's `observerRoleArn`

### Multi-account execution flow later

- Tenet keeps orchestration and audit in the control-plane account
- an execution-capable controller or executor assumes `roleArn` in the selected target account
- the workload resources remain in that target account
- the operator still uses the same central UI/API

### Suggested account spec

```json
{
  "name": "prod-core",
  "awsAccountId": "210987654321",
  "roleArn": "arn:aws:iam::210987654321:role/TenetDeployerRole",
  "observerRoleArn": "arn:aws:iam::210987654321:role/TenetObserverRole",
  "allowedRegions": ["ap-northeast-2", "us-west-2"]
}
```

### Target-account IAM split

Use two roles conceptually.

#### `TenetObserverRole`

Purpose:
- read-only inspection from the centralized Tenet API
- ECS/EC2/CloudWatch Logs/STS read paths used by resource-detail troubleshooting

Trust policy:
- trusted by the central Tenet runtime identity in the control-plane account

Permissions:
- only the read actions required for the inspection features you enable
- no mutation permissions

#### `TenetDeployerRole`

Purpose:
- reserved for future live rollout execution in the target account

Trust policy:
- trusted by the central Tenet execution identity in the control-plane account

Permissions:
- only the mutation permissions needed for the rollout/resource types Tenet will actually manage
- should stay separate from the observer role so read-only operator flows do not need broad write access

## Recommended account boundary model

For production, prefer this split.

### Control-plane account

Owns:
- `tenet-api`
- `tenet-controller`
- Redis
- ingress / ALB
- repo sync path
- secrets/config for the control plane
- central logs/metrics/alerts for Tenet itself

### Workload accounts

Own:
- ECS services
- EC2 Auto Scaling Groups
- ALBs/NLBs/listener rules/target groups
- CodeDeploy resources
- application logs and workload-local alarms
- IAM roles Tenet assumes from the control-plane account

This keeps the blast radius cleaner:
- central operators get one Tenet surface
- workload ownership stays in workload accounts
- read-only and write-side trust can evolve independently

## Production deployment checklist

### 1. Choose the control-plane account

Decide which AWS account will host:
- `tenet-api`
- `tenet-controller`
- Redis
- ingress
- repo sync storage/path

### 2. Deploy the runtime

Set at least these environment variables:

```bash
TENET_API_ADDR=:8080
TENET_CATALOG_SOURCE=filesystem
TENET_REPO_ROOT=/workspace/repo
TENET_STATUS_BACKEND=redis
TENET_REDIS_ADDR=redis:6379
TENET_STATUS_SEED_PATH=/workspace/bootstrap/deployments.json
TENET_API_REQUEST_TIMEOUT=30s
TENET_CONTROLLER_WORKERS=1
TENET_CONTROLLER_REQUEUE_SECONDS=30
```

Use `TENET_STATUS_SEED_PATH` only when you want Redis to bootstrap from an existing snapshot file. In a steady production deployment, Redis is the live shared runtime store.

If you want the API to accept signed GitHub webhooks, also inject `TENET_GITHUB_WEBHOOK_SECRET` from your control-plane secret store. That enables `POST /api/v1/github/webhook`, which validates required GitHub headers, HMAC-SHA256 signatures, and push-payload shape, then returns an accepted-or-ignored summary with additive `desiredStateSourceMatch` details. In filesystem mode or other non-Redis API runtimes, accepted pushes stay `processed=false` with an explicit unavailable reason. In GitHub catalog mode with `TENET_STATUS_BACKEND=redis`, a matched push publishes an additive `controllerRefresh` handoff over Redis so the controller can reconcile immediately. The local Compose smoke uses a safe default `smoke-webhook-secret` only to exercise the signed contract end to end; treat that value as local-only and override it in any real environment.

`tenet-api` can alternatively read desired state directly from GitHub:

```bash
TENET_API_ADDR=:8080
TENET_CATALOG_SOURCE=github
TENET_CATALOG_GITHUB_REPOSITORY=acme/platform-config
TENET_CATALOG_GITHUB_REF=main
TENET_CATALOG_GITHUB_PATH=tenet
TENET_STATUS_BACKEND=redis
TENET_REDIS_ADDR=redis:6379
TENET_STATUS_SEED_PATH=/workspace/bootstrap/deployments.json
TENET_API_REQUEST_TIMEOUT=30s
```

That GitHub mode is read-only for desired state. Both `tenet-api` and `tenet-controller` can now use those GitHub envs for desired-state reads, while `TENET_REPO_ROOT` remains the filesystem-mode contract when you want a local checkout. The opt-in local-safe apply executor still requires the filesystem source.

### 3. Choose desired-state delivery

`tenet-api` and `tenet-controller` may either share the same checkout via `TENET_REPO_ROOT` or use the same read-only GitHub source envs above for desired-state reads. The shipped Terraform/bootstrap path still uses the shared checkout plus repo-sync pattern today. That repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL. Keep the local checkout when you want filesystem mode or when the API-side local-safe apply executor is enabled.

If you use the shipped Terraform bootstrap, run this operator preflight on the same machine before `terraform apply`:
- `terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship
- Terraform 1.5+ is installed
- Docker Buildx is installed and working locally
- AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment
- verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`
- the shipped repo-sync path works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL
- private desired-state repo authentication is not wired into this module yet

### 4. Add account manifests

For each managed AWS account, add an account manifest containing at least:
- `name`
- `awsAccountId`
- `roleArn`
- optional `observerRoleArn`
- `allowedRegions`

### 5. Configure AWS credentials for the runtime

Use the AWS SDK default chain through the runtime environment:
- ECS task role
- EKS IRSA
- EC2 instance profile

For multi-account read paths, that runtime identity must be allowed to assume each configured `observerRoleArn`.

### 6. Expose the operator surface

Publish the central URL through an ALB and your preferred auth front door.

If you expose `POST /api/v1/github/webhook`, route GitHub to that exact path and keep the same shared secret in both GitHub and `TENET_GITHUB_WEBHOOK_SECRET`. Validated push events return `202 Accepted` with a useful summary plus `desiredStateSourceMatch` states such as `repository_mismatch`, `ref_mismatch`, `path_untouched`, and `matched`; signed non-push events return `200 OK` with `status=ignored`; invalid signatures fail with `400 Bad Request`. When the active desired-state source is GitHub, the push matches it, and the API/controller runtime is using Redis, the response flips to `processed=true`, adds a `controllerRefresh` object, and the controller reconciles immediately from that Redis handoff. Filesystem mode, mismatched pushes, and non-Redis runtimes keep explicit `processed=false` reasons instead of pretending the handoff happened. The ref comparison normalizes both branch and tag forms and, when `TENET_CATALOG_GITHUB_REF` is unset, compares the push ref against the signed payload's `repository.default_branch` so default-branch-following GitHub catalog mode does not report misleading false mismatches.

### 7. Verify same-account or cross-account reads

Verify these in order:
- the UI loads at `/`
- `/v1/overview/applications` returns application rows
- resource-detail pages load
- for supported resources, recent-log fetches either return real data or an explicit unavailable state
- cross-account resources report useful lookup context rather than failing silently

## Concrete AWS deployment examples

The examples below are not the only valid production topologies, but they are realistic first deployments that match Tenet's current runtime contract.

### Example A: single-account ECS/Fargate deployment

Use this when you want the fastest path from local/demo to a real AWS deployment.

#### Topology

- one VPC in account `platform-prod`
- one internal or internet-facing ALB
- one ECS cluster
- one ECS service for `tenet-api`
- one ECS service for `tenet-controller`
- one Redis deployment reachable from both services
- one shared repo mount/sync path inside the tasks

#### Suggested AWS resources

- ALB: `tenet-prod-alb`
- ECS cluster: `tenet-prod-cluster`
- ECS service: `tenet-api`
- ECS service: `tenet-controller`
- Redis: ElastiCache for Redis or MemoryDB
- CloudWatch Logs groups:
  - `/aws/ecs/tenet-api`
  - `/aws/ecs/tenet-controller`
- ECS task role: `TenetRuntimeRole`
- ECS execution role: standard image-pull/log-write execution role

#### Network layout

- ALB in public subnets if operators need internet access
- ECS tasks in private subnets
- Redis in private subnets
- security groups:
  - ALB -> `tenet-api` on 8080
  - `tenet-api` -> Redis on 6379
  - `tenet-controller` -> Redis on 6379
  - `tenet-api` -> AWS public APIs through NAT or VPC endpoints as needed

#### Runtime configuration

`tenet-api` task:

```bash
TENET_API_ADDR=:8080
TENET_CATALOG_SOURCE=filesystem
TENET_REPO_ROOT=/workspace/repo
TENET_STATUS_BACKEND=redis
TENET_REDIS_ADDR=tenet-redis.example.cache.amazonaws.com:6379
TENET_STATUS_SEED_PATH=/workspace/bootstrap/deployments.json
TENET_API_REQUEST_TIMEOUT=30s
```

`tenet-controller` task:

```bash
TENET_CATALOG_SOURCE=filesystem
TENET_REPO_ROOT=/workspace/repo
TENET_STATUS_BACKEND=redis
TENET_REDIS_ADDR=tenet-redis.example.cache.amazonaws.com:6379
TENET_STATUS_SEED_PATH=/workspace/bootstrap/deployments.json
TENET_CONTROLLER_WORKERS=1
TENET_CONTROLLER_REQUEUE_SECONDS=30
```

#### Filesystem-mode repo delivery options

Pick one practical pattern:
- mount EFS at `/workspace/repo` and have a separate sync job update it from Git
- build a sidecar/init step that clones the repo into ephemeral task storage on startup
- publish a repo snapshot artifact and unpack it into the task filesystem during deploy

For a first ECS deployment, EFS plus a simple repo-sync job is the most straightforward long-lived option because both services can read the same tree without custom image rebuilds. In the shipped Terraform/bootstrap module, that repo-sync job works out of the box only when `repo_url` is anonymously readable, for example a public HTTPS clone URL. Private desired-state repo authentication is not wired into this module yet.

#### IAM expectations

`TenetRuntimeRole` should have:
- read access to the specific AWS APIs Tenet uses for same-account inspection
- permission to write application logs to CloudWatch through the task execution/runtime setup
- no broad mutation permissions unless you intentionally prepare for future write-side execution

#### Operator URL flow

- operator opens `https://tenet.example.com/`
- ALB forwards to `tenet-api`
- `tenet-api` serves the embedded UI and JSON APIs
- `tenet-controller` keeps Redis updated in the background

#### Good first verification commands

```bash
curl -I https://tenet.example.com/
curl https://tenet.example.com/v1/overview/applications
curl 'https://tenet.example.com/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service'
```

### Example B: multi-account ECS/Fargate deployment

Use this when you already separate workloads across accounts and want Tenet in a dedicated control-plane account.

#### Accounts

- control-plane account: `tools-control`
- workload account: `prod-core`
- workload account: `staging-core`

#### Topology

In `tools-control`:
- one VPC
- one ALB
- one ECS cluster
- one `tenet-api` service
- one `tenet-controller` service
- one Redis deployment
- one shared repo path

In each workload account:
- the actual ECS/EC2/ALB/CodeDeploy resources
- one `TenetObserverRole`
- optionally one `TenetDeployerRole` reserved for future execution

#### Cross-account trust model

- `tenet-api` runs with base credentials from `TenetRuntimeRole` in `tools-control`
- account manifests point `observerRoleArn` at each target account's `TenetObserverRole`
- when the selected deployment belongs to another account, Tenet assumes that role for read-only inspection

#### Example account manifest set

Control-plane repo entry for `prod-core`:

```json
{
  "name": "prod-core",
  "awsAccountId": "210987654321",
  "roleArn": "arn:aws:iam::210987654321:role/TenetDeployerRole",
  "observerRoleArn": "arn:aws:iam::210987654321:role/TenetObserverRole",
  "allowedRegions": ["ap-northeast-2", "us-west-2"]
}
```

Control-plane repo entry for `staging-core`:

```json
{
  "name": "staging-core",
  "awsAccountId": "210987654322",
  "roleArn": "arn:aws:iam::210987654322:role/TenetDeployerRole",
  "observerRoleArn": "arn:aws:iam::210987654322:role/TenetObserverRole",
  "allowedRegions": ["ap-northeast-2"]
}
```

#### Example target-account trust relationship

A practical trust shape for `TenetObserverRole` is:
- principal: the ECS task role used by `tenet-api` in the `tools-control` account
- action: `sts:AssumeRole`
- optional external ID or condition keys if your platform standard requires them

#### Example runtime path for one cross-account request

When an operator opens a resource page for `payments-api-prod` in `prod-core`:
- browser -> ALB in `tools-control`
- ALB -> `tenet-api`
- `tenet-api` reads deployment/account metadata from the central repo and Redis
- `tenet-api` detects target account `210987654321`
- AWS SDK in `tenet-api` assumes `arn:aws:iam::210987654321:role/TenetObserverRole`
- Tenet performs the read-only inspection in `prod-core`
- UI shows either live data or an explicit unavailable state with lookup context

#### Recommended operational boundaries

Keep these split cleanly:
- control-plane observability in the `tools-control` account for Tenet itself
- workload observability in each workload account for the applications themselves
- no direct operator dependency on logging into workload accounts for first-line drill-down

#### Good first verification commands

```bash
curl https://tenet.example.com/v1/overview/applications
curl 'https://tenet.example.com/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate'
curl 'https://tenet.example.com/v1/overview/deployments/payments-api-staging/resource-logs?resourceType=ecsService&resourceName=payments-api-staging-service'
```

If these return explicit unavailable states, check in this order:
- the deployment's `awsAccountId` and region in the compiled catalog
- the account manifest's `observerRoleArn`
- whether the control-plane runtime identity is allowed to assume the observer role
- whether the observer role has the read permissions needed by the selected inspection path

## Connection guidance by environment type

### If you are starting with single-account

Do this first:
- deploy one central Tenet runtime in that account
- run Redis centrally
- use the runtime IAM identity directly for read-only AWS inspection
- still keep `roleArn` and `observerRoleArn` in manifests so the repo contract already matches the future multi-account shape

Then, when you later split accounts:
- move Tenet to a dedicated control-plane account if needed
- keep the same manifest model
- swap same-account access for assume-role access through `observerRoleArn`

### If you already have multi-account AWS

Do this first:
- deploy Tenet in a dedicated control-plane account
- create `TenetObserverRole` in every target account
- keep `TenetDeployerRole` separate even if execution is not enabled yet
- point every account manifest at those exact target-account role ARNs

This avoids repainting the architecture later.

## What “centralized Tenet” should mean here

A centralized Tenet deployment should mean:
- one UI/API for operators
- one controller/runtime state backend
- one repo-backed desired-state source of truth
- many target accounts behind explicit trust boundaries
- no need for operators to jump account-to-account for first-line deployment troubleshooting

It should not mean:
- moving workloads into the control-plane account
- flattening all IAM into one overpowered runtime role
- pretending current read-only inspection is already live rollout execution

## Recommended default decision

If there is no strong constraint otherwise, use this default:
- deploy Tenet centrally in a dedicated control-plane AWS account
- run `tenet-api`, `tenet-controller`, and Redis there
- keep workload resources in single or multiple target accounts
- use `observerRoleArn` for today's cross-account read-only inspection
- reserve `roleArn` for future controlled write-side execution

That model works for both:
- a simple single-account first rollout
- a real multi-account production platform

## Related docs

- `docs/architecture/tenet-v1-overview.md`
- `docs/architecture/tenet-v1-domain-model.md`
- `docs/architecture/tenet-v1-local-runtime.md`
- `docs/features/resource-recent-logs.md`
- `deploy/terraform/aws-central-control-plane/README.md`
