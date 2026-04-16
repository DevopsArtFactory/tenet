# Controller Runtime Write Paths Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Move controller runtime persistence toward backend-owned additive write paths and controller-owned batch applies instead of full-store snapshot rewrites.

**Architecture:** Extend `internal/statusstore` with additive per-deployment write/delete operations that preserve sorted listing semantics for both filesystem and Redis backends, then add a batched apply contract so `cmd/tenet-controller` can persist one reconcile result without exposing partial updates. The controller should prune stale entries, keep context propagation intact, and preserve existing runtime-history behavior.

**Tech Stack:** Go 1.25, stdlib JSON/filesystem, Redis via `go-redis/v9`, existing Go test suite.

---

### Task 1: Add per-deployment status-store write primitives

**Objective:** Introduce additive controller-owned write/delete operations in the status-store package with filesystem and Redis coverage.

**Files:**
- Modify: `internal/statusstore/store.go`
- Modify: `internal/statusstore/file.go`
- Modify: `internal/statusstore/redis.go`
- Modify: `internal/statusstore/file_test.go`
- Modify: `internal/statusstore/redis_test.go`
- Test: `internal/statusstore/list_test.go`

**Step 1: Write failing tests**

Add tests covering:
- filesystem upsert writes one deployment without clobbering another
- filesystem delete removes one deployment and keeps stable sorting
- Redis upsert writes one deployment without full-save setup
- Redis delete removes deployment/index membership and returns not found afterward

**Step 2: Run tests to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/statusstore -run 'Test(FileStore|RedisStore).*(Upsert|Delete)' -v`
Expected: FAIL because the new per-deployment methods do not exist yet.

**Step 3: Write minimal implementation**

Add an additive interface in `internal/statusstore/store.go`:

```go
type Writer interface {
    Store
    Upsert(context.Context, Snapshot) error
    Delete(context.Context, string) error
}
```

Implement `Upsert`/`Delete` for `FileStore` by loading current snapshots, replacing or removing a single deployment, and saving back through existing sorted persistence helpers. Implement `Upsert`/`Delete` for `RedisStore` with single-key writes plus index maintenance.

**Step 4: Run tests to verify pass**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./internal/statusstore -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/statusstore/store.go internal/statusstore/file.go internal/statusstore/redis.go internal/statusstore/file_test.go internal/statusstore/redis_test.go internal/statusstore/list_test.go
git commit -m "Add additive status-store write primitives"
```

### Task 2: Update the controller to use additive runtime writes and stale-deployment pruning

**Objective:** Persist controller-owned runtime state through the batch write contract when available so one reconcile result lands atomically, while preserving the plain-store `Save` fallback for backends that do not support batched runtime writes.

**Files:**
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests**

Add tests covering:
- controller uses `BatchWriter.Apply` for each reconcile result when the store supports batched runtime writes
- controller deletes stale deployment snapshots that no longer exist in the compiled repo
- controller still falls back to `Save` for plain `statusstore.Store` implementations
- controller preserves context propagation for batch apply/save store I/O

**Step 2: Run tests to verify failure**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans' -v`
Expected: FAIL because the controller still saves the entire snapshot set.

**Step 3: Write minimal implementation**

Use a runtime helper that prefers the new batch writer interface when available:

```go
if batchWriter, ok := store.(statusstore.BatchWriter); ok {
    // apply the compiled snapshots and stale deployment names in one batch
} else {
    // existing full Save fallback for plain statusstore.Store backends
}
```

Keep `SeedFromFileIfEmpty`, history appends, and rollout summary derivation unchanged. Do not add a naive controller loop of `Upsert`/`Delete` calls when `BatchWriter` is unavailable; that would expose partial runtime state if a later mutation failed mid-reconcile.

**Step 4: Run tests to verify pass**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./cmd/tenet-controller -run 'TestEmitRuntimePlans' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/tenet-controller/runtime.go cmd/tenet-controller/runtime_test.go
git commit -m "Use additive runtime persistence in controller"
```

### Task 3: Document additive runtime persistence behavior

**Objective:** Keep operator docs aligned with the new controller-owned write-path behavior.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-runtime-state-storage.md`

**Step 1: Write failing doc-oriented assertions or identify the exact sections to update**

Document that:
- controller now prefers batch applies when the backend supports the `BatchWriter` contract
- Redis no longer depends on whole-store rewrites for active runtime updates
- filesystem remains compatible via read-modify-write batch saves
- deployments removed from the compiled repo are pruned from persisted runtime state on the next controller reconcile

**Step 2: Update docs**

Add short, exact wording aligned with the implementation contract.

**Step 3: Verify**

Run: `GOROOT=/Users/kuma/.gvm/gos/go1.25 GOPATH=/Users/kuma/.gvm/pkgsets/go1.25/global PATH=/Users/kuma/.gvm/gos/go1.25/bin:/opt/homebrew/bin:$PATH go test ./...`
Expected: PASS.

**Step 4: Commit**

```bash
git add README.md docs/architecture/tenet-v1-runtime-state-storage.md docs/plans/2026-04-16-controller-runtime-write-paths.md
git commit -m "Document controller runtime write paths"
```