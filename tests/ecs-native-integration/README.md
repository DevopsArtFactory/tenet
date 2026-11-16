# ECS native E2E preparation

See the [IaC root](../../deploy/terraform/ecs-native-integration) and
[scenario runbook](../../docs/runbooks/ecs-native-integration.md).
No helper here creates AWS resources or submits Tenet Applies.

## Test images

`app/server.py` serves a version marker on every path and `/health` with 200.
A separate image baked with `FAIL_HEALTH=true` returns 503 for paths ending in
`/health`; it remains reachable for diagnostics. Both ECS container health and
ALB health use that endpoint. No external dependency, credentials or data is needed.

After separate authorization to publish images, use a reviewed Linux amd64
Python 3.12+ slim **digest** for `BASE_IMAGE`, and build three distinct images:

```sh
docker build --platform linux/amd64 --build-arg BASE_IMAGE="$BASE_IMAGE" \
  --build-arg VERSION=baseline -t tenet-e2e:baseline tests/ecs-native-integration/app
docker build --platform linux/amd64 --build-arg BASE_IMAGE="$BASE_IMAGE" \
  --build-arg VERSION=candidate -t tenet-e2e:candidate tests/ecs-native-integration/app
docker build --platform linux/amd64 --build-arg BASE_IMAGE="$BASE_IMAGE" \
  --build-arg VERSION=unhealthy --build-arg FAIL_HEALTH=true \
  -t tenet-e2e:unhealthy tests/ecs-native-integration/app
```

Publish to your test ECR using separately approved credentials; use the registry
**manifest digest**, not local image ID or tag, in Terraform and Git. Keep the
same base digest for all three images. No build/push happens in standard CI.

## Evidence helper

`fixture.py export` reads saved Terraform JSON and writes a new test repository.
`fixture.py collect` explicitly invokes AWS CLI v2 **read operations only** using
a required named profile and the fixture region. It checks caller account before
creating the destination, saves service/native deployment + revision history,
running tasks, both target health snapshots, rule weights and alarm state. It
batches Describe requests and uses CLI pagination for lists. It does not call
Tenet, approve/retry/rollback, inject failures or alter AWS state.

```sh
python3 tests/ecs-native-integration/fixture.py collect \
  --outputs /tmp/tenet-fixture-outputs.json --strategy canary \
  --profile sandbox-observer --destination /tmp/tenet-e2e/canary-before
```

Use a profile with read permissions for the fixture (and STS caller identity);
prefer the output observer role. Do not archive credentials or authenticated
headers. Store snapshots outside Git. The `complete.json` marker means collection
finished, **not** that a deployment passed. Snapshots are sequential, not atomic;
collect again after quiescence if they disagree. A transitional deployment may
lack a target revision; its raw response is retained and only identified revisions
are described. Missing revision evidence remains unknown, never a passing result. CLI errors and account mismatch
exit unsuccessfully; partial directories must not be treated as passing evidence.
Save Tenet's immutable plan, Git SHA, forward/child Applies and audit separately
using the authenticated UI/API; these are required to interpret AWS evidence.

Offline checks:

```sh
python3 -m unittest discover -s tests/ecs-native-integration -v
```

## Native ECS recovery in CI

The **Native ECS rollback (AWS Emulator)** job runs two tests for blue/green,
canary and linear. The SDK test checks ECS deployment history, rollback and
count-only updates directly. The Git test creates a temporary repository and
an emulated Fargate service with two healthy baseline tasks. A second commit
enables automatic sync, selects a missing image and sets the count to four
(blue/green and linear) or one (canary).

Tenet reconciles that commit, registers the task definition and applies the
service update. After ECS reports an image pull failure on the candidate, the
test injects the configured alarm into the deployment observation. Tenet then
creates the recovery Apply, requests rollback and restores the previous count.
The test checks the failed parent and successful child, the original service
revision and task definition, two healthy source tasks, and configured ALB
routing. A further reconcile must leave exactly those two Applies.

Each strategy runs uninterrupted and with four worker restarts. The restart
scenario interrupts immediately after ECS accepts task definition registration,
service update, rollback, and capacity restoration, before Tenet saves the
response. It verifies an authorized in-flight checkpoint on disk, advances the
test clock past the abandoned claim's expiry, and rebuilds the reconciler,
executor, file stores and SDK clients with a new worker ID. Recovery uses the
saved checkpoint and fresh emulator reads. Each mutation must be called exactly
once, with only two task definitions and one forward deployment in ECS history.

This simulates worker termination inside the test process using a dedicated
panic recovered by the harness. It exercises disk recovery and lease takeover;
OS process signals, startup configuration and filesystem crash durability need
separate process-level tests.

The Git checkout, IAM readiness fixture and injected alarm keep this test
independent of GitHub and AWS accounts. ECS operations and ALB observations use
the emulator's HTTPS loopback endpoint, CA and credentials. Live IAM policy
evaluation, CloudWatch metric delivery and HTTP traffic distribution are covered
by the separate sandbox scenarios.

With Git, Python 3, AWS CLI v2 and Go installed, start a fresh emulator on port
19297 and set `TENET_EMULATOR_STATE` to its directory containing `ca.pem` and
`emulator_credentials`. Run the following from the repository root.

```sh
go test -tags awsemulator -run '^TestNativeEmulator' -count=1 -v ./internal/ecsexecution
```
