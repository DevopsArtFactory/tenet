# Rollout Step Detail Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add controller-derived rollout step detail so EC2 weighted-target-group and ECS CodeDeploy/native rolling deployments expose operator-meaningful step descriptions and traffic-shift semantics in APIs and the embedded overview UI.

**Architecture:** Extend the domain rollout step summary with additive detail/metadata fields, derive those fields from the existing planner/controller rollout plan, and render them in the joined overview detail API and embedded UI without changing the existing step ordering or execution-mode mapping. Keep the change controller-owned and deterministic so file-backed and Redis-backed runtime stores preserve the same payload shape.

**Tech Stack:** Go 1.25, embedded HTML/JS UI, existing planner/controller runtime summary pipeline, Go tests, Node syntax/behavior harness.

---

### Task 1: Add rollout step detail fields and controller derivation

**Objective:** Persist additive step detail for rollout steps so API consumers can see what each current/pending/completed step means, especially for EC2 weighted traffic shifting and ECS CodeDeploy flows.

**Files:**
- Modify: `internal/domain/status.go`
- Modify: `internal/reconcile/planner.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Test: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add tests covering:
- EC2 canary rollout step details include an explicit traffic-shift detail for `ShiftTrafficCanary` and candidate-capacity detail for `CreateCandidateCapacity`.
- ECS linear rollout step details include a CodeDeploy detail for `CreateCodeDeployDeployment` and a linear traffic detail for `ShiftTrafficLinear`.
- Completed `MarkSynced` plans still mark steps completed without inventing extra detail.

Example assertions:

```go
if status.Rollout.Steps[2].Detail != "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group." {
    t.Fatalf("expected candidate-capacity step detail, got %+v", status.Rollout.Steps[2])
}
if status.Rollout.Steps[4].Detail != "Shift a small canary slice of production traffic to the candidate target group before full promotion." {
    t.Fatalf("expected canary traffic detail, got %+v", status.Rollout.Steps[4])
}
```

**Step 2: Run test to verify failure**

Run: `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansPersists(RolloutStepDetailsForEC2CanaryDeployment|CodeDeployRolloutStepDetailsForECSLinearDeployment)' -count=1`
Expected: FAIL because `RolloutStepSummary` does not yet expose detail fields.

**Step 3: Write minimal implementation**

Implement:
- additive `detail` field on `domain.RolloutStepSummary`
- controller helper(s) that derive deterministic step detail strings from `deployment`, `plan`, and `step.Kind`
- no change to rollout step ordering, phase selection, or execution-mode selection

Representative detail strings:

```go
case reconcile.StepCreateCandidateCapacity:
    return "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group."
case reconcile.StepShiftTrafficCanary:
    return "Shift a small canary slice of production traffic to the candidate target group before full promotion."
case reconcile.StepShiftTrafficLinear:
    if planContainsStep(plan, reconcile.StepCreateCodeDeployDeployment) {
        return "Advance ECS traffic in linear increments through CodeDeploy until the replacement task set serves all traffic."
    }
    return "Advance ALB traffic in linear increments toward the candidate target group until promotion is safe."
```

**Step 4: Run tests to verify pass**

Run:
- `go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansPersists(RolloutStepDetailsForEC2CanaryDeployment|CodeDeployRolloutStepDetailsForECSLinearDeployment)' -count=1`
- `go test ./cmd/tenet-controller -count=1`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/domain/status.go internal/reconcile/planner.go cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Document rollout step detail in controller summaries"
```

### Task 2: Render rollout step detail in overview detail API/UI

**Objective:** Surface the new step detail field through joined overview responses and the embedded UI deployment detail panel.

**Files:**
- Modify: `internal/catalog/overview.go`
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**

Add tests covering:
- overview deployment detail JSON includes `rollout.steps[*].detail`
- embedded UI detail panel renders a step detail row under each rollout step when present
- UI behavior/syntax coverage keeps the detail panel rendering valid JavaScript

Example assertion:

```go
if firstStep["detail"] != "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group." {
    t.Fatalf("expected rollout step detail in JSON, got %+v", firstStep)
}
```

**Step 2: Run test to verify failure**

Run:
- `go test ./internal/api -run 'TestDeploymentOverviewDetailRouteIncludesRolloutStepDetail|TestIndexHTMLIncludesRolloutStepDetailRendering' -count=1`
- `go test ./internal/api -run TestEmbeddedUIFilterBehaviorWithNodeHarness -count=1`

Expected: FAIL because the route fixtures/UI do not yet assert or render rollout step detail.

**Step 3: Write minimal implementation**

Implement:
- rollout summary cloning that preserves step detail fields
- embedded UI rendering for `detail.rollout.steps[*].detail` using safe DOM construction and a muted detail text element
- keep step status/current rendering intact

Representative UI snippet:

```js
if (step.detail) {
  stepItem.append(createElement('div', 'detail-step-description', step.detail));
}
```

**Step 4: Run tests to verify pass**

Run:
- `go test ./internal/api -run 'TestDeploymentOverviewDetailRouteIncludesRolloutStepDetail|TestIndexHTMLIncludesRolloutStepDetailRendering|TestEmbeddedUIFilterBehaviorWithNodeHarness' -count=1`
- `node --check /tmp/tenet-ui-rollout-step-detail.js`

For the syntax check, extract the embedded `<script>` block before running `node --check`.

**Step 5: Commit**

```bash
git add internal/catalog/overview.go internal/api/web/index.html internal/api/router_test.go internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "Show rollout step detail in overview UI"
```

### Task 3: Update docs for rollout step detail semantics

**Objective:** Keep README and architecture docs aligned with the new operator-facing rollout detail contract.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Write failing doc-oriented checks**

Use targeted grep-like assertions in tests only if an existing doc test exists; otherwise verify manually by reading the updated sections after editing.

**Step 2: Update docs**

Document that:
- `rollout.steps[*].detail` is controller-derived explanatory text, not live AWS execution telemetry
- EC2 canary/linear detail explains candidate ASG bootstrap + weighted target-group progression
- ECS CodeDeploy detail explains deployment creation and traffic-shift semantics
- the overview detail panel now renders both step status and step description

**Step 3: Verify docs**

Read back the changed sections to confirm wording and examples are aligned with the API/UI behavior.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md
git commit -m "Document rollout step detail semantics"
```

---

## Final verification

Run from repo root with the Go 1.25 toolchain in PATH:

```bash
go test ./...
make smoke-local
docker compose config
docker compose up --build -d
curl -s http://localhost:8080/v1/overview/deployments/payments-api-prod | jq '.rollout.steps'
curl -s http://localhost:8080/ | head -40
docker compose down --remove-orphans
```

Expected results:
- all Go tests pass
- smoke-local passes
- Docker Compose config/build succeeds
- overview detail JSON includes `rollout.steps[*].detail`
- embedded UI loads and renders rollout detail text without JavaScript errors

## Notes
- Do not touch unrelated untracked files such as `.codex/`, `.gitignore`, or `AGENTS.md`.
- Preserve existing execution-mode recommendations and rollout step statuses.
- Keep the new fields additive and backwards-compatible for existing fixture snapshots and Redis/file status stores.
