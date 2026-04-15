# Tenet Control Plane Bootstrap Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Build the first maintainable slice of Tenet: a documented multi-account architecture, a small Go control-plane skeleton, and test-backed domain/state foundations for future EC2 and ECS rollout work.

**Architecture:** Start with a narrowly scoped control-plane core rather than a fake end-to-end deployment. Define domain types, spec/status schemas, placement resolution, and rollout state transitions in small packages with tests. Keep AWS interactions behind interfaces so the first milestones are deterministic and easy to extend.

**Tech Stack:** Go standard library, Go testing package, Markdown docs.

---

## Delivery principles

- Keep diffs small and reviewable.
- Prefer package boundaries over giant files.
- Add tests before production code for behavior-bearing packages.
- Avoid external dependencies until the design hardens.
- Preserve room for EC2 and ECS drivers without prematurely implementing both.

## Phase map

### Phase 0 — Architecture and plan documents

**Objective:** Capture the control-plane shape and implementation roadmap in-repo.

**Files:**
- Create: `docs/architecture/tenet-v1-overview.md`
- Create: `docs/architecture/tenet-v1-domain-model.md`
- Create: `docs/plans/2026-04-15-control-plane-bootstrap.md`

**Verification:**
- Read the docs for consistency.
- Ensure multi-account, spec/status, rollout phases, and EC2/ECS models are all covered.

### Phase 1 — Repository bootstrap

**Objective:** Establish a minimal Go module and command layout for the control plane.

**Files:**
- Create: `go.mod`
- Create: `cmd/tenet-api/main.go`
- Create: `cmd/tenet-controller/main.go`
- Create: `internal/buildinfo/buildinfo.go`
- Create: `Makefile`

**Step 1: Write failing build expectation**
- Run `go test ./...`
- Expected: fail because module and packages do not exist.

**Step 2: Add minimal module and entrypoints**
- `tenet-api` should start an HTTP server placeholder.
- `tenet-controller` should load and log a placeholder controller configuration.

**Step 3: Run tests**
- Run `go test ./...`
- Expected: pass.

**Step 4: Run formatting and build checks**
- Run `gofmt -w $(find . -name '*.go')`
- Run `go test ./...`

### Phase 2 — Domain foundations

**Objective:** Model durable core types for account, environment, deployment, rollout, sync state, and conditions.

**Files:**
- Create: `internal/domain/account.go`
- Create: `internal/domain/environment.go`
- Create: `internal/domain/application.go`
- Create: `internal/domain/deployment.go` (include `AppRef` so deployments do not encode app identity only in names)
- Create: `internal/domain/rollout.go`
- Create: `internal/domain/status.go`
- Create: `internal/domain/condition.go`
- Create: `internal/domain/types_test.go`

**Step 1: Write failing tests**
- Add tests for zero-value safety, enum validation helpers, and rollout progress semantics.
- Run `go test ./internal/domain/...`
- Expected: fail due to missing types.

**Step 2: Implement minimal domain types**
- Keep files focused by resource family.
- Add helper methods like `IsTerminal`, `IsHealthy`, `SyncState.Valid()`.

**Step 3: Re-run tests**
- Run `go test ./internal/domain/...`
- Expected: pass.

### Phase 3 — Placement resolution

**Objective:** Add account/region-aware placement resolution detached from AWS APIs.

**Files:**
- Create: `internal/placement/resolver.go`
- Create: `internal/placement/resolver_test.go`

**Step 1: Write failing tests**
- Test valid account+region resolution.
- Test missing account failure.
- Test disallowed region failure.

**Step 2: Implement resolver**
- Inputs: account records, environment bindings, deployment spec.
- Output: resolved account/region/network context.

**Step 3: Run tests**
- Run `go test ./internal/placement/...`
- Expected: pass.

### Phase 4 — Rollout state machine

**Objective:** Encode a deterministic rollout phase machine and sync-state transitions.

**Files:**
- Create: `internal/rollout/statemachine.go`
- Create: `internal/rollout/statemachine_test.go`

**Step 1: Write failing tests**
- Valid transition from Pending -> Accepted -> Planning.
- Invalid backward transition rejection.
- Failed and RolledBack terminal states.
- Sync state derivation from rollout phase + condition summary.

**Step 2: Implement state machine**
- No AWS logic yet.
- Keep rule tables explicit and test-backed.

**Step 3: Run tests**
- Run `go test ./internal/rollout/...`
- Expected: pass.

### Phase 5 — Controller planning skeleton

**Objective:** Create the first planner shape that turns desired deployment and status into abstract reconciliation steps.

**Files:**
- Create: `internal/reconcile/plan.go`
- Create: `internal/reconcile/planner.go`
- Create: `internal/reconcile/planner_test.go`

**Step 1: Write failing tests**
- New desired generation should emit `ResolvePlacement`, `ObserveCurrentState`, and `PlanCandidateGeneration` steps.
- Healthy synced state should emit no-op or `MarkSynced` result.

**Step 2: Implement minimal planner**
- Keep steps abstract; no AWS SDK calls.

**Step 3: Run tests**
- Run `go test ./internal/reconcile/...`
- Expected: pass.

### Phase 6 — Read-only HTTP surface

**Objective:** Expose a minimal API for health and static architecture metadata.

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/router_test.go`
- Modify: `cmd/tenet-api/main.go`

**Step 1: Write failing tests**
- `GET /healthz` returns 200.
- `GET /v1/info` returns version/build metadata.

**Step 2: Implement handlers**
- Keep router package small and dependency-free.

**Step 3: Run tests**
- Run `go test ./internal/api/... ./cmd/tenet-api/... ./...`
- Expected: pass.

### Phase 7 — Verification and PR hygiene

**Objective:** Keep each landable slice reviewable and safe.

**Files:**
- Modify only files touched by each phase.

**Verification checklist per slice:**
- `gofmt -w` on changed Go files.
- `go test ./...`
- independent diff review
- lore-format commit message
- push branch and open or update PR

## Initial file layout target

```text
cmd/
  tenet-api/
  tenet-controller/
internal/
  api/
  buildinfo/
  domain/
  placement/
  reconcile/
  rollout/
docs/
  architecture/
  plans/
```

## Risks and follow-up

- AWS SDK integration is intentionally deferred to keep the first slices deterministic.
- ECS-specific behavior should not leak into the shared rollout model too early.
- Persistence and UI implementation should wait until status semantics stabilize.

## Progress update

Implemented so far:
- Phases 0 through 6 foundations are in place in repo form.
- Read-only API surface now includes summary, account, environment, deployment, placement, topology, and deployment-status endpoints.
- Controller bootstrap now validates and summarizes the repository catalog at startup when `TENET_REPO_ROOT` is set, then stays alive as a signal-aware long-running process.
- The first runtime-status persistence seam exists as a filesystem-backed deployment status store.
- Independent logic-review fixes have already been applied for planner behavior, rollout semantics, HTTP method guards, and manifest root validation.

Next likely slices:
- richer topology/status read models once runtime status persistence exists
- controller-side orchestration beyond the current abstract planner skeleton
- eventual AWS execution drivers once the data model stabilizes

## First execution target

Land Phases 0 through 4 as the first PR series:
1. docs and architecture
2. repo bootstrap
3. domain + placement + rollout state machine

This keeps the repository usable after every PR while building toward the full control plane.
