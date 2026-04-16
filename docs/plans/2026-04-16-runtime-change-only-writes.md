# Change-Only Controller Runtime Writes Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Reduce controller-owned runtime persistence churn by applying only changed deployment snapshots to batch-capable status stores while preserving atomic full-save behavior for plain stores.

**Architecture:** Keep the existing controller-owned runtime read model and `statusstore.BatchWriter` contract, but teach `emitRuntimePlans` to diff the newly derived per-deployment statuses against the currently persisted snapshots before calling `BatchWriter.Apply`. Batch-capable backends should receive only changed upserts plus stale deployment deletes, and the controller should skip `Apply` entirely when a reconcile is a complete no-op. Plain `statusstore.Store` implementations must continue receiving the full compiled snapshot set through `Save`, while the filesystem compatibility path may still rewrite the full merged snapshot set when its batch apply receives real changes or deletes so atomic behavior stays intact.

**Tech Stack:** Go 1.25, standard library, existing `internal/statusstore` contracts, `cmd/tenet-controller`, repository docs.

---

### Task 1: Lock the batch-writer diff contract with failing controller tests

**Objective:** Prove that batch-capable stores receive only changed snapshots while plain stores still receive the full snapshot set.

**Files:**
- Modify: `cmd/tenet-controller/runtime_test.go`
- Test: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add focused tests that cover:
- unchanged snapshots are omitted from `BatchWriter.Apply` upserts
- changed snapshots are included in `BatchWriter.Apply` upserts
- newly compiled deployments are included in `BatchWriter.Apply` upserts
- stale persisted deployments are still passed in the delete list
- complete no-op batch reconciles skip `Apply` entirely
- plain stores still receive the full compiled snapshot set through `Save`

Suggested test names:
```go
func TestEmitRuntimePlansBatchWriterSkipsUnchangedSnapshots(t *testing.T) {}
func TestEmitRuntimePlansBatchWriterIncludesChangedAndNewSnapshots(t *testing.T) {}
```

**Step 2: Run test to verify failure**

Run:
```bash
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansBatchWriter' -v
```

Expected: FAIL because the current implementation always sends every compiled deployment through `BatchWriter.Apply`.

**Step 3: Write minimal implementation**

Update `emitRuntimePlans` to:
- compute the fully derived `currentStatuses` as it already does
- build the full sorted snapshot set once
- when `runtimeStatusBatchWriter(store)` is available, diff the full sorted snapshot set against the currently loaded persisted `statuses`
- send only changed/new snapshots to `Apply`
- skip `Apply` entirely when both the changed snapshot set and stale delete list are empty
- continue sending stale deployment names in sorted order
- keep `Save` fallback behavior unchanged for plain stores

Prefer a small helper such as:
```go
func changedRuntimeSnapshots(current []statusstore.Snapshot, previous map[string]domain.DeploymentStatus) []statusstore.Snapshot
```

**Step 4: Run test to verify pass**

Run:
```bash
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlansBatchWriter' -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Reduce controller batch writes to changed snapshots only"
```

---

### Task 2: Document the new change-only batch-write behavior

**Objective:** Keep the public docs aligned with the actual controller persistence semantics.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-state-storage.md`
- Modify: `docs/architecture/tenet-v1-deployment-status-api.md`

**Step 1: Write failing documentation-oriented assertions (if appropriate)**

No new code-level docs test is required for this slice; instead, update the docs immediately after the implementation is green.

**Step 2: Update docs**

Document that:
- batch-capable stores now receive only changed/new snapshots plus stale deletes
- no-op batch reconciles skip `Apply` entirely
- Redis remains transactional but no longer rewrites unchanged deployments every tick
- plain stores still receive the full compiled snapshot set through `Save`
- `/v1/status/deployments*` now documents the richer current payload shape instead of the original narrow snapshot description

Key wording to preserve:
- change-only delta application applies to batch-capable stores, with no-op batches skipped entirely
- filesystem/plain-store compatibility paths still preserve full-snapshot atomic behavior when real changes or deletes exist
- bounded runtime history remains capped at 10 entries

**Step 3: Verify docs are updated coherently**

Run:
```bash
git diff -- README.md docs/architecture/tenet-v1-runtime-state-storage.md docs/architecture/tenet-v1-deployment-status-api.md
```

Expected: docs consistently describe change-only batch writes and the current status API payload.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-runtime-state-storage.md docs/architecture/tenet-v1-deployment-status-api.md
git commit -m "Document change-only runtime batch writes"
```

---

### Task 3: Run full verification and publish the slice

**Objective:** Prove the slice works across unit, smoke, and Docker-backed local verification before pushing.

**Files:**
- Review only: `cmd/tenet-controller/runtime.go`
- Review only: `cmd/tenet-controller/runtime_test.go`
- Review only: `README.md`
- Review only: `docs/architecture/tenet-v1-runtime-state-storage.md`
- Review only: `docs/architecture/tenet-v1-deployment-status-api.md`

**Step 1: Run targeted and full tests**

Run:
```bash
GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...
make smoke-local
```

Expected: PASS.

**Step 2: Run Docker verification**

Run:
```bash
docker compose config
docker compose up --build -d
curl -fsS http://localhost:8080/v1/overview/accounts >/tmp/tenet-overview.json
curl -fsS http://localhost:8080/v1/status/deployments/payments-api-prod >/tmp/tenet-status.json
docker compose down --remove-orphans
```

Expected:
- Compose config is valid
- stack starts successfully
- overview API responds
- status detail API responds

**Step 3: Review change summary**

Run:
```bash
git diff --stat origin/main...HEAD
```

Expected: focused controller + docs slice.

**Step 4: Commit/push if needed**

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go README.md docs/architecture/tenet-v1-runtime-state-storage.md docs/architecture/tenet-v1-deployment-status-api.md
git commit -m "Reduce controller runtime store churn"
git push -u origin HEAD
```

**Step 5: PR workflow**

If GitHub auth is available:
```bash
gh auth status
gh pr create --title "Reduce controller runtime store churn" --body-file /tmp/tenet-pr.md
gh pr checks --watch
gh pr merge --auto --squash --delete-branch
```

If GitHub auth is unavailable, stop after push and report that PR creation was blocked by auth.
