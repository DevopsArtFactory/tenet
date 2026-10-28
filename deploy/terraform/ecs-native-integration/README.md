# Native ECS integration fixture

Preparation only: nothing in the normal smoke tests or CI creates AWS resources.
This standalone Terraform root bootstraps three **existing-strategy** ECS Fargate
services (`BLUE_GREEN`, `CANARY`, `LINEAR`) in a dedicated sandbox VPC, with one
IPv4 ALB, separate IP target pairs/rules, container + ALB health checks, a custom
CloudWatch alarm per service, short-retention logs and separate observer/deployer
roles. It does not provision Tenet, its login/RBAC, a Git server or an image registry.

Requires Terraform 1.7+, AWS provider **6.35.1** (separate from the older central
control-plane root), a commercial AWS region supporting all three native strategies,
and two explicitly selected AZs. Only backend-free initialization, validation and
**mocked** Terraform tests are needed for development:

```sh
terraform -chdir=deploy/terraform/ecs-native-integration init -backend=false
terraform -chdir=deploy/terraform/ecs-native-integration fmt -check -recursive
terraform -chdir=deploy/terraform/ecs-native-integration validate
terraform -chdir=deploy/terraform/ecs-native-integration test
python3 -m unittest discover -s tests/ecs-native-integration -v
```

`tests/fixture.tftest.hcl` replaces the entire AWS provider with a mock. It runs
plans only, and needs neither AWS credentials nor an AWS account connection.
Initialization downloads the pinned provider; these checks are not live E2E results.

## Future sandbox bootstrap (separate authorization required)

Do not run this section as part of preparation. Creation incurs charges for an
ALB, at least six Fargate tasks/public IPv4 addresses, alarms and logs. Rollouts
can temporarily double task capacity. No NAT gateway or autoscaler is created.
Public task IPv4 addresses enable HTTPS image pulls/log delivery, but inbound
port 8080 is allowed only from the ALB security group. ALB port 80 is restricted
to the operator CIDR (/24 or narrower); this fixture serves synthetic data only.

1. Build and publish baseline, candidate and health-failing image digests using
   the [test app instructions](../../../tests/ecs-native-integration/README.md).
   Use a private registry accessible to the task execution role; private ECR in
   the same account/region works with the attached task execution policy.
2. Copy `terraform.tfvars.example` to `terraform.tfvars`. Select a sandbox account,
   two AZs, operator public IPv4, the **existing** Tenet runner role and baseline
   digest. The provider's `allowed_account_ids` prevents using another account.
   Use unique `name` values and separate Terraform state directories per test run.
3. The bootstrap operator needs VPC/ALB/ECS/IAM/log/alarm creation and PassRole
   permissions. The account needs the ECS service-linked role (bootstrap may
   require `iam:CreateServiceLinkedRole`). The existing Tenet runner additionally
   needs an identity policy permitting `sts:AssumeRole` on the two output roles;
   this module sets their trust but deliberately does not edit the runner role.
4. With separately authorized credentials, review a saved Terraform plan before
   applying it. Inspect native deployment status after bootstrap; Terraform's
   service creation response does not prove healthy native convergence.
5. Save `terraform output -json` to a local ignored file, then export Git inputs:

   ```sh
   python3 tests/ecs-native-integration/fixture.py export \
     --outputs /tmp/tenet-fixture-outputs.json --destination /tmp/tenet-e2e-repo
   ```

   The exporter works offline, accepts only five fixed file paths and refuses an
   existing destination. The `.yaml` files contain JSON, a YAML subset supported
   by Tenet. Commit them to a separate test Git repository and configure its real
   RBAC, authenticated login and an isolated durable Tenet state directory.
   Do not copy production RBAC or actor-header bypasses. Exported automation
   policies default **off**; all runtime execution flags remain operator-owned.
6. Follow the [E2E scenarios](../../../docs/runbooks/ecs-native-integration.md).

Terraform owns bootstrap infrastructure. `ignore_changes` cedes service task
revision/count/native policy/alarms/load-balancer binding and rule actions to ECS
and Tenet after creation. Reapplying Terraform is **not** a reset or self-heal
mechanism. Inspect unexpected infrastructure drift separately, and never run
Terraform updates concurrently with a Tenet scenario. Recreate a unique fixture
for a clean baseline rather than reusing a failed run's state.

The deployer can update/stop only fixture services/deployments and pass only its
fixture execution/routing roles. `RegisterTaskDefinition` and several read/list
APIs require `Resource: "*"`; this is a sandbox policy, not an organizational
production IAM template. The observer has no ECS mutation or PassRole grant.
The ECS routing role uses AWS's infrastructure managed policy and is the only
role intended to change ALB routing. No Lambda hook/test listener is configured.

## Future cleanup

Stop the isolated Tenet reconciler/worker first; retain redacted plans, Applies,
audit and AWS snapshots. Wait for native deployments/rollback to finish before
removing infrastructure roles. With separate authorization, review a destroy
plan in the **same account/state** and destroy this root. Verify services/tasks,
ALB, ENIs, roles, alarms and logs are removed. Tenet-created task-definition
revisions are outside Terraform state: inventory and deregister/delete only
revisions in this run's three exact families after tasks are gone. Separately
remove test images/repository and any external test metric data expires by AWS
retention. Never destroy using another run's state or manually delete its roles
while ECS is still routing traffic.

References: [AWS ALB requirements](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/alb-resources-for-blue-green.html),
[ECS infrastructure role](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AmazonECSInfrastructureRolePolicyForLoadBalancers.html),
[pinned Terraform service schema](https://registry.terraform.io/providers/hashicorp/aws/6.35.1/docs/resources/ecs_service).
