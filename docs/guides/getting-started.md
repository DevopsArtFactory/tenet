# Getting started

This walkthrough starts Tenet with sample deployments, explains the main screens,
and creates a plan. It takes you to a running UI before configuring an AWS
account or a login provider.

Tenet reads desired state from a manifest repository and presents applications,
runtime evidence and deployment actions together. Live deployment execution
supports native ECS Rolling, Blue-Green, Canary and Linear. EC2 manifests support
catalog, planning and inspection workflows.

## Prerequisites

Use Linux or macOS with Git and Go matching the version in
[go.mod](../../go.mod), currently 1.27.4. The UI is embedded in the Go API server.
A separate Node.js build is unnecessary.

The alternative Compose setup requires Docker, Docker Compose v2 and Make.
The demo uses bundled manifests and recorded status. AWS credentials are needed
when you choose live inspection or configure live ECS execution.

## Start the demo

Clone the repository and work from its root. Before public release, GitHub
requires repository access for this clone.

```sh
git clone https://github.com/DevopsArtFactory/tenet.git
cd tenet
go version
```

Use a fresh terminal for the demo so configuration from another Tenet installation
does not select its catalog or authentication provider. Give this session its own
temporary plan and Apply files.

```sh
TENET_DEMO_STATE=$(mktemp -d)

TENET_CATALOG_SOURCE=filesystem \
TENET_REPO_ROOT="$PWD/examples/local-repo" \
TENET_STATUS_BACKEND=file \
TENET_STATUS_PATH="$PWD/examples/local-status/deployments.json" \
TENET_PLAN_STORE_PATH="$TENET_DEMO_STATE/plans.json" \
TENET_APPLY_STORE_PATH="$TENET_DEMO_STATE/applies.json" \
TENET_AWS_ENABLE_APPLY=false \
TENET_AWS_ENABLE_ECS_EXECUTION=false \
TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION=false \
TENET_ECS_RECONCILE=false \
TENET_API_ADDR=127.0.0.1:18083 \
go run ./cmd/tenet-api
```

Keep that terminal running. Open [the application list](http://127.0.0.1:18083/?page=applications)
in your browser. The first run downloads Go dependencies and compiles the API.

In a second terminal, check that the process and catalog are available.

```sh
curl --fail http://127.0.0.1:18083/healthz
curl --fail http://127.0.0.1:18083/v1/catalog/summary
```

The health endpoint returns `ok`. The catalog endpoint returns JSON with the
sample accounts, environments and deployments. The status shown in this setup
comes from the bundled status file. Its timestamps describe recorded fixtures.

## Explore the UI

| Screen | What to look for |
| --- | --- |
| Applications | Find a deployment by name and compare its desired state with the recorded runtime state. |
| Application detail | Open `payments-api-prod` to inspect its artifact, capacity, rollout and resources. |
| Resource detail | Inspect a resource's identity and available inspection/log actions. AWS-backed actions need matching credentials and observer permissions. |
| Account detail | See which environments and applications belong to an AWS account. |
| Drift | Review differences between declared intent and available observations. |
| Plan/apply | Create a stored preview and review the operations before approving an Apply. |
| Applies monitor | Follow queued/running/terminal Apply records, operation checkpoints and recovery links. |
| Audit | Inspect retained action records when diagnosing who requested a change and what happened. |

Resource inspection may report `aws_unavailable` with this sample setup.
That means Tenet lacks usable AWS evidence for the example resource. A resource
without an independent log stream can report `no_log_source`.

## Create your first plan

1. Open `payments-api-prod`, then its Plan/apply workspace. You can also open
   [the sample plan workspace](http://127.0.0.1:18083/?page=plan&deployment=payments-api-prod).
2. Select **Create Plan**.
3. Review the target, source fingerprint, operations and any readiness errors.
   The URL now includes a `planId`, which identifies this saved preview.
4. Keep the URL to return to the same plan. A changed manifest requires a fresh
   plan because approval is tied to the reviewed source.

The equivalent preview request is available for scripts.

```sh
curl --fail -X POST \
  -H 'Content-Type: application/json' \
  -d '{}' \
  http://127.0.0.1:18083/v1/plans/deployments/payments-api-prod
```

Apply execution is disabled by the demo command. A plan can be inspected with
that setting. In a live ECS installation, **Apply Plan** creates an execution
record, and the executor registers the requested task definition, updates the
existing service and observes convergence.

`TENET_AWS_ENABLE_APPLY=true` alone selects the local executor stub. Actual ECS
execution also requires `TENET_AWS_ENABLE_ECS_EXECUTION=true`, authenticated
authorization, repository RBAC and a verified deployer role. Follow the
[ECS GitOps setup](ecs-gitops.md) when moving to an AWS account.

## Use your own manifests

Tenet loads JSON or YAML files from this layout.

```text
desired-state/
  accounts/
    production.yaml
  environments/
    prod.yaml
  deployments/
    worker.yaml
  rbac/
    policy.yaml
```

An account declares its AWS account ID, observer/deployer roles and allowed
regions. An environment selects an account and region. A deployment selects an
environment and declares its image, strategy and capacity. RBAC grants identities
access to named deployments or supported project/environment scopes.

Copy a sample to a separate directory before editing it.

```sh
cp -R examples/ecs-native-repo ../tenet-desired
```

This sample contains placeholder AWS identifiers and image digests. Replace them
using the [ECS setup guide](ecs-gitops.md). Restart the API with
`TENET_REPO_ROOT` pointing to the new directory and the live flags still false
to inspect its compiled catalog first.

The `examples/local-repo` catalog illustrates the UI and legacy rollout models.
The explicit `ecs` blocks in `examples/ecs-native-repo` and
`examples/ecs-native-advanced-repo` are the starting points for native execution.

## Run the API, controller and Redis together

From the source repository root, start the bundled development stack.

```sh
make docker-up
docker compose ps
curl --fail http://localhost:8080/healthz
docker compose logs --tail=30 api controller
```

Open [localhost:8080](http://localhost:8080/). The API serves the UI and
plan/Apply endpoints. The controller periodically refreshes the runtime status
stored in Redis. Native ECS Apply execution and Git reconciliation run in the
API process when their flags are enabled.

The Compose stack publishes ports 8080 and 6379 on the host and has demo defaults.
Use it on a development machine. Plan/Apply files are in the named
`tenet-api-tmp` volume. Redis has no configured persistent data volume in this
stack, so its status is seeded from fixtures when its container is recreated.

Stop the stack with `make docker-down`. This preserves the named API volume.
Deleting that volume also deletes its saved plan/Apply state.

## Stop and troubleshoot

Stop the Go process with **Ctrl+C**. Keep the same plan/Apply paths when testing
resume behavior. The temporary directory used above is disposable demo storage;
a live installation needs persistent storage.

| Symptom | Check |
| --- | --- |
| Go rejects the toolchain version | Compare `go version` with `go.mod`; configure toolchain installation/downloads for your environment. |
| Address already in use | Choose another `TENET_API_ADDR` for the Go demo. Compose also needs ports 8080 and 6379 available. |
| Catalog startup error | Check the repository root, YAML/JSON syntax, environment/account references and allowed region. |
| Empty catalog | Confirm `TENET_REPO_ROOT` points at the directory containing `accounts`, `environments` and `deployments`. |
| Old status timestamps | The Go demo loads saved fixtures. Check the controller/status backend for a running installation. |
| Plan rejected after an edit | Create a fresh plan against the current manifest. |
| 401 or 403 | Check managed login and the authenticated subject's repository RBAC grant. |
| Apply disabled | Use the live ECS setup and its explicit execution flags when you intend AWS changes. |

Continue with [ECS GitOps setup](ecs-gitops.md), the
[operator troubleshooting runbook](../runbooks/operator-troubleshooting.md), or
[contributing](../../CONTRIBUTING.md).
