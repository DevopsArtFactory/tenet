# Rollout Execution Config Contract Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make Tenet's rollout execution resources more operator-meaningful by deriving config-aware execution descriptions for EC2 weighted target groups, EC2 instance refresh, ECS rolling, and ECS CodeDeploy while keeping backend/UI fallback contracts aligned.

**Architecture:** Extend the controller-owned `rollout.execution[]` description generation so it includes normalized rollout-config context already preserved on compiled deployments, then mirror the exact fallback descriptions in `tenet-api` and the embedded UI. Lock the shared contract with focused Go and Node-backed tests so resource-detail bookmarks remain stable across controller-authored and synthesized execution resources.

**Tech Stack:** Go, embedded HTML/JavaScript, Go test, Node-backed UI harness, make smoke-local.

---

### Task 1: Lock the new execution-description contract with failing tests

**Objective:** Define the exact operator-facing description strings before changing implementation.

**Files:**
- Modify: `cmd/tenet-controller/runtime_test.go`
- Modify: `cmd/tenet-api/main_test.go`
- Modify: `internal/api/ui_test.go`
- Modify: `internal/api/ui_behavior_test.go`

**Step 1: Write failing tests**

Add assertions for these behaviors:
- `asgWeightedTargetGroups` candidate capacity description mentions normalized userdata source plus desired/min/max and warmup when configured.
- `trafficShiftPlan` description mentions canary/linear/blue-green semantics and rollout qualifiers when present.
- `asgInstanceRefresh` service ASG description includes rolling-update batch size when configured.
- `ecsCodeDeploy` deployment-group/listener descriptions mention the active traffic-shift style (`blueGreen`, `canary`, `linear`) and qualifiers when relevant.
- Synthesized fallback resources in `cmd/tenet-api` and `internal/api/web/index.html` match the controller-authored descriptions byte-for-byte.

**Step 2: Run tests to verify failure**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansPersists(ConfigAwareWeightedTargetGroupRolloutSummary|ConfigAwareCodeDeployExecutionDetails|InstanceRefreshExecutionDetails)' -v`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-api -run TestCanonicalRolloutExecutionFallback -v`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/api -run 'Test(IndexHTMLIncludesSynthesizedRolloutFallbackContract|EmbeddedUI.*RolloutExecution|EmbeddedUIFallback.*|EmbeddedUIDetail.*|EmbeddedUIRendersWorkspaceRuntimeSignals)' -v`

Expected: FAIL because the current description helpers still emit generic descriptions.

### Task 2: Implement config-aware rollout execution descriptions

**Objective:** Make controller-authored and synthesized execution resources reflect rollout configuration and strategy semantics.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-api/main.go`
- Modify: `internal/api/web/index.html`

**Step 1: Write minimal implementation**

Implement helpers that preserve current resource identifiers/order while upgrading descriptions:
- weighted-target-group candidate ASG description uses normalized userdata source and candidate capacity/warmup context.
- traffic-shift plan description differentiates canary, linear, and full cutover semantics.
- instance-refresh service ASG description includes configured batch size.
- ECS CodeDeploy descriptions mention deployment style and strategy qualifiers where appropriate.
- Keep exact fallback alignment between controller, API fallback, and UI fallback.

**Step 2: Run focused tests to verify pass**

Run the same focused commands from Task 1.

Expected: PASS.

### Task 3: Update docs and verification coverage

**Objective:** Document the refined execution-resource contract and keep local verification aligned.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-overview-ui.md`
- Modify: `docs/architecture/tenet-v1-rollout-strategy-model.md`

**Step 1: Update docs**

Document that execution descriptions are config-aware/controller-authored and give representative examples for:
- EC2 candidate ASG bootstrap + capacity/warmup
- EC2 traffic-shift plan semantics for canary/linear/blue-green
- ECS CodeDeploy traffic-shift contract
- EC2 rolling batch-size qualifiers

**Step 2: Run project verification**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`

Expected: PASS.

### Task 4: Commit and ship the slice

**Objective:** Commit the verified slice with docs.

**Files:**
- Modify: only files touched above plus this plan if desired

**Step 1: Review diff**

Run:
- `git diff --stat`
- `git status --short`

**Step 2: Commit**

Run:
```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go cmd/tenet-api/main.go cmd/tenet-api/main_test.go internal/api/web/index.html internal/api/ui_test.go internal/api/ui_behavior_test.go README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-overview-ui.md docs/architecture/tenet-v1-rollout-strategy-model.md docs/plans/2026-04-17-rollout-execution-config-contract.md
git commit
```

Commit subject should describe why the execution contract became config-aware, and the message body should follow the repo Lore trailer protocol.
