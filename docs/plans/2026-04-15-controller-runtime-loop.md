# Tenet Controller Runtime Loop Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Extend the current local Tenet control-plane demo so `tenet-controller` actively reloads desired state and status snapshots on a timer, computes per-deployment reconcile plans, and emits deterministic runtime logs that show what the controller would do next.

**Architecture:** Keep the controller side effect-free for now. Add a small runtime loop that periodically loads the compiled catalog from `TENET_REPO_ROOT`, optionally loads deployment snapshots from `TENET_STATUS_PATH`, and feeds each deployment through the existing abstract planner. Log one summarized reconcile decision per deployment on startup and every tick.

**Tech Stack:** Go standard library, existing catalog/statusstore/reconcile packages, Go tests, Markdown docs.

---

## Delivery principles

- Preserve deterministic behavior with no AWS SDK or write-side effects.
- Reuse the existing planner instead of inventing a second decision path.
- Keep runtime config explicit and environment-driven.
- Make logs stable enough for smoke checks and future controller assertions.
- Keep the slice runnable inside the existing Docker Compose stack.

## Phase map

### Phase 0 — Document the next runtime slice

**Objective:** Capture the runtime-loop behavior and local workflow updates in repo docs.

**Files:**
- Create: `docs/plans/2026-04-15-controller-runtime-loop.md`
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-local-runtime.md`

### Phase 1 — Add failing controller runtime tests

**Objective:** Lock the intended runtime behavior before implementation.

**Files:**
- Modify: `cmd/tenet-controller/bootstrap_test.go`
- Modify: `cmd/tenet-controller/run_test.go`
- Create if needed: `cmd/tenet-controller/runtime_test.go`

**Step 1: Write failing tests for config loading**
- Require `TENET_CONTROLLER_REQUEUE_SECONDS` default handling.
- Require optional `TENET_STATUS_PATH` propagation.

**Step 2: Write failing tests for runtime execution**
- Controller logs startup and shutdown as before.
- Controller logs a reconcile plan for each deployment when repo fixtures are present.
- Controller keeps running across ticks until context cancellation.

### Phase 2 — Implement runtime config and loop

**Objective:** Make the controller periodically evaluate the fixture-backed repo.

**Files:**
- Modify: `cmd/tenet-controller/bootstrap.go`
- Modify: `cmd/tenet-controller/run.go`
- Create if needed: `cmd/tenet-controller/runtime.go`

**Implementation outline:**
- Add controller config fields for `StatusPath` and `RequeueInterval`.
- On each loop iteration:
  - load compiled catalog from repo root
  - load snapshots from status file if configured
  - build a deployment-name index for status lookups
  - call `reconcile.Planner.Build` for every deployment
  - log a stable summary containing deployment name, generation, sync state, and step list
- If repo root is unset, keep the process alive and log that runtime planning is disabled.

### Phase 3 — Reuse local fixtures in smoke validation

**Objective:** Keep the local runtime demo verifiable after the controller becomes active.

**Files:**
- Modify: `compose.yaml`
- Modify: `scripts/smoke-local.sh`
- Modify: `Makefile` only if an extra target is useful

**Implementation outline:**
- Pass `TENET_STATUS_PATH` into the controller container as well.
- Optionally reduce controller requeue interval in compose for fast local feedback.
- Extend smoke coverage if needed to assert controller logs include planner output.

### Phase 4 — Verification and PR hygiene

**Objective:** Land the runtime-loop slice safely.

**Verification checklist:**
- `go test ./...`
- `make smoke-local`
- `docker compose up --build -d`
- `docker compose logs controller --tail=50`
- `docker compose down --remove-orphans`
- independent review
- lore-format commit message
- push branch, open PR, merge when green

## Next slices after this PR

1. Promote runtime loop logs into structured in-memory/runtime status updates.
2. Add controller-owned writes to the filesystem status store.
3. Expose controller runtime decisions through read APIs.
4. Introduce real observation/execution adapters behind interfaces.
