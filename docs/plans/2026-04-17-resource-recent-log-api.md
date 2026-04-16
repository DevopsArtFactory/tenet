# Resource Recent Log API Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a small real backend/UI slice that lets Tenet resource detail pages show recent ECS and EC2 operational logs.

**Architecture:** Introduce a read-only resource-log provider behind the API router. Resolve resource logs from the deployment detail context plus the selected resource type/name: ECS services read recent CloudWatch Logs events from the newest running task/container streams; EC2 Auto Scaling Group resources read recent instance bootstrap output from the newest instance console output. Keep the contract refresh-based and explicitly unavailable/empty when the resource type is unsupported or cross-account access is not configured.

**Tech Stack:** Go 1.25 toolchain, net/http, embedded UI JS, AWS SDK for Go v2 (config, sts, ecs, cloudwatchlogs, autoscaling, ec2), existing router/UI tests, Node UI harness, smoke-local script.

---

### Task 1: Define the resource-log API contract with failing tests

**Objective:** Lock the new route, JSON shape, and unavailable/error semantics before implementation.

**Files:**
- Modify: `internal/api/router_test.go`
- Create: `internal/resourcelogs/service_test.go`
- Modify: `internal/api/router.go`
- Create: `internal/resourcelogs/service.go`

**Step 1: Write failing tests**

Add router tests covering:
- `GET /v1/overview/deployments/payments-api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod`
- 200 JSON response with:
  - `deploymentName`
  - `resourceType`
  - `resourceName`
  - `available`
  - `source`
  - `entries[]` (`timestamp`, `message`, optional `stream`)
  - optional `unavailableReason`
- 400 on missing `resourceType` or `resourceName`
- 503 when the provider is not wired
- 404 passthrough when the deployment lookup fails

Add resource-log service tests covering:
- ECS service happy path: selects newest running task streams and returns recent CloudWatch log events
- EC2 ASG happy path: selects newest instance from the ASG and returns recent console-output/bootstrap lines
- unsupported resource type returns `available=false` with a stable unavailable reason instead of a hard error
- cross-account access without assume-role config returns `available=false` with a stable unavailable reason

**Step 2: Run tests to verify failure**

Run:
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestRouterOverviewDeploymentResourceLogs' -v`
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/resourcelogs -run 'TestService' -v`

Expected: FAIL because the route/provider/types do not exist yet.

**Step 3: Commit**

Do not commit yet; proceed to Task 2 after the failures are observed.

---

### Task 2: Implement the AWS-backed read-only resource-log provider and route

**Objective:** Ship the backend/API slice with real ECS/EC2 log retrieval and graceful unavailable behavior.

**Files:**
- Create: `internal/resourcelogs/service.go`
- Modify: `internal/api/router.go`
- Modify: `cmd/tenet-api/main.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Write minimal implementation**

Implement a provider that:
- loads AWS config from the default chain for the deployment region
- optionally assumes a role when configured and the deployment account differs from the caller account
- for `ecsService`:
  - finds the ECS service by name
  - finds recent running tasks
  - resolves awslogs driver streams from the task definition/container config
  - returns recent CloudWatch log events in reverse-chronological operator-friendly order
- for `candidateAutoScalingGroup` and `serviceAutoScalingGroup`:
  - describes the ASG by name
  - resolves the newest instance
  - reads latest console output/bootstrap lines
  - returns recent non-empty lines as log entries
- returns `available=false` and a stable `unavailableReason` for unsupported resource types, missing AWS access assumptions, missing running tasks/instances, or no configured log source

Wire the provider into `cmd/tenet-api/main.go` and expose the new route from `internal/api/router.go`.

**Step 2: Run targeted tests**

Run:
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/resourcelogs -v`
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestRouterOverviewDeploymentResourceLogs' -v`

Expected: PASS.

**Step 3: Commit**

Do not commit yet; continue to Task 3 for UI/docs.

---

### Task 3: Show recent logs on the resource detail page and update docs/smoke coverage

**Objective:** Make the backend slice visible in the embedded UI and keep docs/smoke coverage aligned.

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`
- Modify: `scripts/smoke-local.sh`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Create: `docs/features/resource-recent-logs.md`

**Step 1: Write failing UI/docs tests**

Add tests that require:
- resource detail page renders a `Recent logs` section
- logs load lazily from `/v1/overview/deployments/${deployment}/resource-logs?...`
- loading, unavailable, empty, and populated states are rendered safely via `textContent` / `replaceChildren`
- no `innerHTML` usage for log messages
- `scripts/smoke-local.sh` asserts the embedded HTML contains `Recent logs`

**Step 2: Run tests to verify failure**

Run:
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestEmbeddedUI|TestResourceDetailPage' -v`

Expected: FAIL before the UI code exists.

**Step 3: Implement minimal UI/docs changes**

Update the resource detail page to lazy-load and render recent logs for the currently selected resource, including explicit unavailable and empty-state copy. Document the endpoint, supported resource types, and current access limitations in README + architecture doc + feature doc.

**Step 4: Run tests to verify pass**

Run:
- `PATH=/opt/homebrew/bin:$PATH go test ./internal/api -run 'TestEmbeddedUI|TestResourceDetailPage' -v`
- `node --check <(python3 - <<'PY'
from pathlib import Path
html = Path('internal/api/web/index.html').read_text()
start = html.index('<script>') + len('<script>')
end = html.index('</script>', start)
print(html[start:end])
PY
)`

Expected: PASS.

**Step 5: Final verification**

Run from repo root:
- `PATH=/opt/homebrew/bin:$PATH go test ./...`
- `PATH=/opt/homebrew/bin:$PATH make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate` returns JSON (empty/unavailable is acceptable for local fixture data)
- verify `/` contains `Recent logs`
- `docker compose down --remove-orphans`

**Step 6: Commit**

```bash
git add go.mod go.sum internal/api/router.go internal/api/router_test.go internal/resourcelogs/service.go internal/resourcelogs/service_test.go internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go scripts/smoke-local.sh README.md docs/architecture/tenet-v1-overview-ui.md docs/features/resource-recent-logs.md docs/plans/2026-04-17-resource-recent-log-api.md cmd/tenet-api/main.go
git commit -m "Add recent operational logs to resource detail pages

Constraint: Keep this slice read-only and refresh-based; no streaming transport yet
Rejected: Shelling out to aws CLI from the API process | harder to test and weaker than typed SDK clients
Confidence: medium
Scope-risk: moderate
Reversibility: clean
Directive: Keep resource-log contract explicit about unavailable states and do not silently invent ECS/EC2 log sources
Tested: targeted router/resource-log/UI tests, go test ./..., make smoke-local, docker compose config, docker compose up --build -d, docker compose down --remove-orphans
Not-tested: live cross-account assume-role access against production AWS accounts"
```
