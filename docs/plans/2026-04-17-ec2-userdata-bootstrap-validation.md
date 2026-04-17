# EC2 Userdata Bootstrap Validation Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Fail fast when EC2 deployments omit the userdata bootstrap metadata Tenet needs to model immutable candidate capacity and instance refresh replacement capacity.

**Architecture:** Tighten manifest validation in `internal/spec` so EC2 manifests that imply new or replacement instances must carry normalized userdata metadata before compilation. Keep the change narrowly scoped to validation plus accompanying docs so the existing rollout execution/resource contract remains intact and better grounded.

**Tech Stack:** Go, standard library testing, existing README and architecture docs.

---

### Task 1: Add failing validation tests for EC2 userdata requirements

**Objective:** Lock the intended contract before changing validation logic.

**Files:**
- Modify: `internal/spec/validate_test.go`
- Test: `internal/spec/validate_test.go`

**Step 1: Write failing test**

Add focused tests covering:
- EC2 canary/blueGreen/linear/rolling deployments without top-level or stack fallback userdata must fail.
- EC2 deployments with stack-level userdata fallback still pass.
- ECS deployments without userdata still pass.
- EC2 deployments with incomplete userdata metadata (missing type or missing path) fail.

**Step 2: Run test to verify failure**

Run: `go test ./internal/spec -run 'TestValidateDeploymentSpec(RejectsEC2MissingUserDataBootstrap|RejectsEC2IncompleteUserDataBootstrap|AcceptsEC2CanaryManifestWithStackUserDataFallback)' -v`
Expected: FAIL because validation does not yet enforce the bootstrap contract.

**Step 3: Write minimal implementation**

No production changes in this task.

**Step 4: Run test to verify pass**

Defer until Task 2.

**Step 5: Commit**

Commit together with Task 2 after the implementation is green.

### Task 2: Enforce EC2 userdata bootstrap validation

**Objective:** Make `ValidateDeployment` reject EC2 manifests that cannot supply candidate/replacement userdata metadata.

**Files:**
- Modify: `internal/spec/validate.go`
- Test: `internal/spec/validate_test.go`

**Step 1: Implement minimal validation**

Add a helper that resolves effective userdata from either top-level `deployment.UserData` or the first stack fallback and enforce for EC2 targets:
- `Type` and `Path` are both required for EC2 deployments.
- Error text should explain that EC2 deployments require userdata bootstrap metadata for immutable capacity.
- Do not require userdata for ECS.

**Step 2: Run focused tests**

Run: `go test ./internal/spec -run 'TestValidateDeploymentSpec(RejectsEC2MissingUserDataBootstrap|RejectsEC2IncompleteUserDataBootstrap|AcceptsEC2CanaryManifestWithStackUserDataFallback|AcceptsECSImageManifest)' -v`
Expected: PASS.

**Step 3: Run broader regression tests**

Run: `go test ./internal/spec ./internal/manifest -v`
Expected: PASS.

**Step 4: Commit**

```bash
git add internal/spec/validate.go internal/spec/validate_test.go
git commit -m "Require EC2 userdata bootstrap metadata"
```

### Task 3: Update README and rollout architecture docs

**Objective:** Document the now-enforced EC2 bootstrap contract alongside the existing execution-model docs.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-rollout-strategy-model.md`

**Step 1: Update docs**

Document that:
- EC2 weighted-target-group and instance-refresh modeling assumes manifest-provided userdata bootstrap metadata.
- Validation now fails fast when EC2 deployments omit userdata type/path.
- ECS remains unaffected.

**Step 2: Verify doc strings and examples**

Run: `go test ./... -run TestValidateDeploymentSpecAcceptsEC2CanaryManifestWithStackUserDataFallback -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add README.md docs/architecture/tenet-v1-rollout-strategy-model.md docs/plans/2026-04-17-ec2-userdata-bootstrap-validation.md
git commit -m "Document EC2 userdata bootstrap requirement"
```

### Task 4: Final verification and PR workflow

**Objective:** Prove the slice is green end to end and publish it.

**Files:**
- Verify existing touched files only.

**Step 1: Run repository verification**

Run:
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; go test ./...`
- `unset GOROOT GOPATH; export PATH=/opt/homebrew/bin:$PATH; make smoke-local`
- `docker compose config`
- `docker compose up --build -d`
- verify `curl http://localhost:8080/v1/overview/accounts`
- verify `curl http://localhost:8080/`
- `docker compose down --remove-orphans`

**Step 2: Review changes**

Run: `git diff --stat main...HEAD`
Expected: narrow validation+docs slice.

**Step 3: Push and open/update PR**

Push branch and, if `gh auth status` is available, create a PR and enable auto-merge after checks.

**Step 4: Final commit if needed**

Use a Lore-style commit message that records the new EC2 bootstrap validation contract and verification commands.
