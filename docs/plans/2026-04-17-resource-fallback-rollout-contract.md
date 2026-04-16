# Resource Fallback Rollout Contract Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Keep the embedded application/resource drill-down usable even when `/v1/overview/deployments/{name}` omits `rollout.execution[]`, while preserving the exact rollout resource contract for EC2 weighted-target-group and ECS CodeDeploy execution modes.

**Architecture:** Extend the embedded UI's synthesized fallback resource list so it mirrors the controller-authored rollout execution identifiers and stable ordering already emitted by `tenet-controller`. Lock the behavior with focused string-contract and Node-backed behavior tests, then update the operator docs to describe that the UI synthesizes the same resource contract when fixture payloads have not yet persisted execution resources.

**Tech Stack:** Go, embedded HTML/JavaScript UI, Go test + Node harness tests, Markdown docs.

---

### Task 1: Lock fallback resource expectations with failing UI tests

**Objective:** Add test coverage proving fallback application/resource pages synthesize the exact rollout resource identifiers and stable links for EC2 weighted target groups and ECS CodeDeploy.

**Files:**
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**

Add assertions for these exact embedded UI strings/behaviors:
- ECS CodeDeploy synthesized resources include `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`.
- EC2 weighted-target-group synthesized resources include `candidateAutoScalingGroup`, `activeTargetGroup`, `candidateTargetGroup`, `productionListenerRule`, `trafficShiftPlan`, `userDataBootstrapSource`.
- The resource drill-down should resolve a synthesized `trafficShiftPlan` resource through a stable `resourceId`-based link even when `detail.rollout.execution` is absent.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/api -run 'TestIndexHTML|TestApplicationsOverviewPageWithNodeHarness|TestResourceDetailPageUsesStableResourceIDsWithNodeHarness' -v`
Expected: FAIL because the current synthesized fallback omits `trafficShiftPlan`, `testListener`, and `codeDeployApplication`.

**Step 3: Commit**

```bash
git add internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "test: lock fallback rollout resource contract"
```

### Task 2: Implement exact synthesized fallback resources in the embedded UI

**Objective:** Update `groupResourceSections` so fallback resources match controller-authored rollout execution identifiers and ordering for ECS CodeDeploy and EC2 weighted target groups.

**Files:**
- Modify: `internal/api/web/index.html`
- Test: `internal/api/ui_test.go`
- Test: `internal/api/ui_behavior_test.go`

**Step 1: Write minimal implementation**

Update the `synthesize()` branch inside `groupResourceSections(detail)` to emit:
- ECS CodeDeploy: `ecsService`, `codeDeployApplication`, `codeDeployDeploymentGroup`, `productionListener`, `testListener`, `blueTaskSetTargetGroup`, `greenTaskSetTargetGroup`
- EC2 weighted target groups: `candidateAutoScalingGroup`, `activeTargetGroup`, `candidateTargetGroup`, `productionListenerRule`, `trafficShiftPlan`, `userDataBootstrapSource`

Keep the existing resource grouping logic and safe DOM construction unchanged.

**Step 2: Run tests to verify pass**

Run: `go test ./internal/api -run 'TestIndexHTML|TestApplicationsOverviewPageWithNodeHarness|TestResourceDetailPageUsesStableResourceIDsWithNodeHarness' -v`
Expected: PASS

**Step 3: Run regression tests**

Run: `go test ./internal/api -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go
git commit -m "feat(ui): align synthesized rollout resources with controller contract"
```

### Task 3: Update docs for synthesized rollout resource fallback behavior

**Objective:** Document that the application/resource drill-down preserves the controller rollout resource contract even when fixture/local payloads omit `rollout.execution[]`.

**Files:**
- Modify: `README.md`
- Modify: `docs/features/overview-dashboard.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`

**Step 1: Update docs**

Document:
- the exact synthesized fallback resources for `asgWeightedTargetGroups` and `ecsCodeDeploy`
- that the UI uses those resources only when `rollout.execution[]` is absent
- that the stable resource-detail route remains `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}`

**Step 2: Verify docs strings**

Run: `go test ./internal/api -run 'TestIndexHTML' -v`
Expected: PASS (embedded UI contract still present after doc-aligned implementation)

**Step 3: Commit**

```bash
git add README.md docs/features/overview-dashboard.md docs/architecture/tenet-v1-overview-ui.md docs/plans/2026-04-17-resource-fallback-rollout-contract.md
git commit -m "docs: describe synthesized rollout resource fallback"
```
