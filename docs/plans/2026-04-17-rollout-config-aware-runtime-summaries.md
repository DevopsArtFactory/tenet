# Rollout Config-Aware Runtime Summaries Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Carry normalized rollout configuration from manifests into compiled deployments so controller-authored rollout summaries can describe EC2/ECS execution with concrete capacity, warmup, and update-shape context.

**Architecture:** Keep the slice additive by extending `domain.Deployment` with a small normalized rollout-config model compiled from existing manifest fields. Then use that config in `cmd/tenet-controller/runtime.go` to enrich controller-derived `rollout.summary` and `rollout.steps[*].detail` strings for weighted-target-group EC2, EC2 instance refresh, ECS rolling, and ECS CodeDeploy without changing router/UI contracts.

**Tech Stack:** Go 1.25, stdlib tests, existing manifest compiler, controller runtime read model, README and architecture docs.

---

### Task 1: Add normalized rollout-config compilation coverage

**Objective:** Prove compiled deployments preserve the rollout-shaping manifest fields needed by runtime summaries.

**Files:**
- Modify: `internal/domain/deployment.go`
- Modify: `internal/manifest/compiler.go`
- Modify: `internal/manifest/compiler_test.go`
- Modify: `internal/spec/types.go`

**Step 1: Write failing tests**

Add compiler tests that prove:
- top-level `strategy.warmupSeconds` and `strategy.minHealthyPercent` are preserved in compiled deployments
- top-level `capacity` is preserved in compiled deployments
- stack `termination_delay_rate` and `rolling_update_instance_count` are preserved in compiled deployments when present
- stack capacity/userdata fallback still works for goployer-style manifests

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./internal/manifest -run 'TestCompilePreservesNormalizedRolloutConfig|TestCompilePreservesNormalizedStackRolloutConfigFallback' -v`
Expected: FAIL because the compiled deployment shape does not yet carry rollout config.

**Step 3: Write minimal implementation**

Add a compact normalized rollout-config model to `domain.Deployment` and populate it in `Compile(...)` using existing manifest fields only. Keep it additive and deterministic.

**Step 4: Run tests to verify pass**

Run the same command and expect PASS.

**Step 5: Commit**

```bash
git add internal/domain/deployment.go internal/spec/types.go internal/manifest/compiler.go internal/manifest/compiler_test.go
git commit -m "Preserve normalized rollout config in compiled deployments"
```

### Task 2: Make controller rollout summaries config-aware

**Objective:** Use the new normalized config to make controller-authored rollout summaries more concrete for EC2/ECS execution modes.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add focused controller tests that prove:
- weighted-target-group EC2 candidate-capacity details mention candidate ASG capacity bounds, normalized userdata bootstrap source, and warmup seconds
- ECS CodeDeploy canary/linear/full details mention CodeDeploy traffic shifting plus warmup/minHealthyPercent when configured
- EC2 instance-refresh and ECS rolling summaries/details mention rolling replacement batch size / min healthy expectations when configured

**Step 2: Run test to verify failure**

Run: `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansPersists(ConfigAwareWeightedTargetGroupRolloutSummary|ConfigAwareECSCodeDeployRolloutSummary|ConfigAwareRollingRolloutSummary)' -v`
Expected: FAIL because runtime summary/detail strings are still generic.

**Step 3: Write minimal implementation**

Update rollout summary/detail helpers in `runtime.go` so they use the normalized rollout config for operator-facing text while preserving existing step ordering and execution resource contracts.

**Step 4: Run tests to verify pass**

Run the same command and expect PASS.

**Step 5: Commit**

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Make rollout summaries config-aware"
```

### Task 3: Update docs and verify the slice

**Objective:** Keep README and architecture docs aligned with the richer runtime-summary semantics and verify the full repo.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-rollout-summary.md`
- Modify: `docs/architecture/tenet-v1-domain-model.md`

**Step 1: Update docs**

Document that controller-authored rollout summaries now derive operator-facing detail from normalized deployment rollout config, including capacity, warmup, userdata bootstrap source, and rolling-update shape where available.

**Step 2: Run verification**

Run exactly:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`

Expected: PASS.

**Step 3: Review diff**

Run: `git diff --stat`
Expected: focused rollout-config/runtime-summary/docs slice.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-runtime-rollout-summary.md docs/architecture/tenet-v1-domain-model.md
git commit -m "Document config-aware rollout summaries"
```

### Task 4: Push and run PR workflow

**Objective:** Publish the verified slice without waiting for user input.

**Files:**
- Git only

**Step 1: Push branch**

Run: `git push -u origin <branch>`

**Step 2: Create or update PR if auth is available**

Use `gh pr create` / `gh pr edit` when authenticated; otherwise record the auth blocker in the run report and continue after pushing.

**Step 3: Monitor checks when a PR exists**

Run: `gh pr checks` or the curl fallback if credentials are available.

**Step 4: Merge when ready**

Use `gh pr merge --auto --squash --delete-branch` when CI/auth support it.
