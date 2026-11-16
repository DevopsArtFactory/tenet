# Terraform: AWS central Tenet bootstrap

This directory provides a one-touch Terraform bootstrap for a centralized Tenet control plane on AWS.

What it provisions:
- a bootstrap VPC
- public subnets for ECS/Fargate tasks; an internal ALB in private subnets by default
- private subnets for Redis and EFS mount targets
- an HTTPS ALB for the embedded Tenet UI/API
- one ECS cluster
- one ECS service for `tenet-api`
- one ECS service for `tenet-controller`
- one ElastiCache Redis cluster for shared runtime state
- one EFS filesystem/access point for persisted plan/apply stores
- one ECR repository
- IAM roles for ECS execution and runtime access
- a local `terraform apply` image build/push step that builds the current repo checkout and pushes it to ECR

This is a practical bootstrap, not the final production topology. It intentionally optimizes for one-touch deployment from a checked-out Tenet repo.

## How the one-touch path works

`terraform apply` does three things in one flow:
1. creates the AWS infrastructure
2. builds the current local Tenet checkout with Docker Buildx
3. pushes the image into the provisioned ECR repository and deploys ECS services from it

That means the machine running `terraform apply` must have:
- Terraform 1.5+
- Docker with Buildx
- AWS CLI v2 with working credentials for the target AWS account

## Preflight before `terraform apply`

Run this checklist on the same machine that will run `terraform apply`:
- `terraform apply` builds and pushes the current local Tenet checkout from that machine, so run it from the repo checkout you intend to ship
- Terraform 1.7+ is installed
- Docker Buildx is installed and working locally
- AWS CLI v2 is installed and your current shell has working AWS credentials for this deployment
- verify that `repo_url` and `repo_branch` are reachable from the current machine before apply, for example `git ls-remote --heads <repo_url> <repo_branch>`:

```bash
git ls-remote --heads <repo_url> <repo_branch>
```

- public HTTPS `repo_url` values work anonymously when `repo_sync_token_secret_arn` is unset
- for a private HTTPS desired-state repo, store the repo token in AWS Secrets Manager and set `repo_sync_token_secret_arn` to that secret ARN

## Repo-sync pattern

Both `tenet-api` and `tenet-controller` now support `TENET_CATALOG_SOURCE=filesystem|github` for desired-state reads. This Terraform bootstrap intentionally chooses the filesystem path and sets `TENET_REPO_ROOT` because the shared repo-sync volume works for both services today. Public HTTPS `repo_url` values work anonymously, and private HTTPS desired-state repos can use the optional Secrets Manager token ARN described below.

`repo_sync_token_secret_arn` is optional. Leave it unset for anonymously readable public HTTPS repos. When set, ECS injects the secret value as `TENET_REPO_SYNC_TOKEN` only into the two `repo-sync` sidecars, and the ECS execution role receives `secretsmanager:GetSecretValue` for that secret ARN.

The repo-sync script keeps using the plain `repo_url` when no token is present, so public anonymous clones continue to work. When `TENET_REPO_SYNC_TOKEN` is present, the script requires an `https://` `repo_url` and supplies the token through Git's HTTPS credential prompt path instead of rewriting the configured repo URL.

To satisfy that without introducing an external GitOps controller, each ECS task definition includes a lightweight `alpine/git` sidecar that:
- clones `repo_url` into a shared task volume
- hard-resets it to `repo_branch`
- refreshes it on a loop

That keeps both:
- `tenet-api`
- `tenet-controller`

reading a real repo tree at `/workspace/repo`.

## Current scope and honesty notes

Implemented well by this bootstrap:
- centralized `tenet-api` + `tenet-controller`
- shared Redis runtime state
- ALB ingress
- one-touch image build/push/deploy from the local checkout
- cross-account read-only setup via `observer_role_arns`

Still intentionally bootstrap-grade:
- HTTPS, GitHub managed login, repository RBAC, and explicit operator CIDRs are required
- ECS tasks get public IPs to avoid NAT complexity in the first slice
- the runtime role can only mount the state filesystem and assume explicitly listed observer roles; no account-wide `ReadOnlyAccess` attachment is granted
- this module does not implement live write-side execution; `roleArn` remains part of the Tenet manifest contract, not part of this Terraform runtime yet

## Secure ingress and login prerequisites

Before applying, provide an existing ACM certificate for `hostname`, create a GitHub OAuth application with callback `https://<hostname>/auth/callback/github`, and store its client secret plus a separate random session signing secret (at least 32 bytes) in Secrets Manager. Pass only their ARNs to Terraform. Customer-managed KMS keys also need an appropriate key policy and execution-role decrypt permission; the default Secrets Manager key works with the supplied policy.

The ALB is internal by default: operators need VPN/peering connectivity. Point your DNS hostname at `alb_dns_name`. Set `alb_internal=false` only when intentionally exposing HTTPS to the explicitly allowed operator CIDRs. Port 80 is not opened. The application enforces secure cookies, login and repository RBAC, and ignores caller-supplied authentication headers.

Commit an `rbac/policy.yaml` granting your GitHub subject (`github:<numeric-user-id>`) the required role and deployment/project/environment scopes before signing in. Login alone grants no deployment access. OAuth state and identities persist in the existing EFS state volume. This module configures GitHub login; other Tenet-supported providers require corresponding task environment and secret changes.

Upgrades require these new inputs and replace the HTTP listener. Existing HTTP bookmarks must change to the configured HTTPS hostname. Removal of account-wide read access requires explicit observer-role configuration for same-account inspection too.

## Quick start

From the Tenet repository root:

```bash
cd deploy/terraform/aws-central-control-plane
cp terraform.tfvars.example terraform.tfvars
$EDITOR terraform.tfvars
terraform init
terraform apply
```

When apply finishes, Terraform prints:
- the ALB DNS name
- the Tenet URL
- the ECS runtime role ARN
- the ECR repository URL

## Single-account usage

If you are deploying Tenet and workloads in the same AWS account:
- configure a least-privilege observer role even for same-account workload inspection
- list that role in `observer_role_arns` and in the desired-state account configuration
- limit `operator_cidr_blocks` to your VPN network; an empty observer list grants no workload inspection permissions

## Multi-account usage

If you are deploying Tenet in a central control-plane account:
- set `observer_role_arns` to the target-account `TenetObserverRole` ARNs
- use the `runtime_role_arn` output when writing those target-account trust policies
- keep workload resources in the target accounts

A minimal target-account trust shape is:
- principal: the `runtime_role_arn` output from this module
- action: `sts:AssumeRole`

## Important inputs

See `variables.tf`, but the main knobs are:
- `name_prefix`
- `repo_url` (public HTTPS works anonymously; private HTTPS can use `repo_sync_token_secret_arn`)
- `repo_sync_token_secret_arn` (optional Secrets Manager ARN containing an HTTPS repo token for the repo-sync sidecars)
- `repo_branch`
- `image_tag`
- `operator_cidr_blocks`
- `observer_role_arns`

## Example tfvars

```hcl
name_prefix          = "tenet-prod"
repo_url             = "https://github.com/DevopsArtFactory/tenet.git"
# repo_sync_token_secret_arn = "arn:aws:secretsmanager:ap-northeast-2:123456789012:secret:tenet/repo-sync-token-AbCdEf"
repo_branch          = "main"
image_tag            = "bootstrap"
operator_cidr_blocks = ["203.0.113.10/32"]
observer_role_arns = [
  "arn:aws:iam::210987654321:role/TenetObserverRole",
  "arn:aws:iam::210987654322:role/TenetObserverRole"
]
```

## Verification after apply

Use the printed ALB URL or DNS name.

```bash
curl -I https://<configured-hostname>/
curl https://<configured-hostname>/healthz
# Sign in through the browser before accessing guarded API routes.
```

For cross-account reads, also test a supported resource-detail API route and verify that it either:
- returns live data, or
- returns an explicit unavailable payload with lookup context

## Cleanup

```bash
terraform destroy
```

## Offline validation

With Terraform 1.15+ installed, run `terraform init -backend=false`, `terraform validate`, and `terraform test`. The security tests use mocked providers and plan-only runs, so they do not build images or create AWS resources. CI runs these checks for every PR.
