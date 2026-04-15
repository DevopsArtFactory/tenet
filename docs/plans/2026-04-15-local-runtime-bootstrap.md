# Tenet Local Runtime Bootstrap Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make the current Tenet control-plane foundation runnable on a local machine with Docker, sample repository data, and a documented developer workflow while continuing to extend the platform in small reviewable slices.

**Architecture:** Keep the existing repo-backed control plane read-only, but package it as a two-process local stack: `tenet-api` serves catalog/status endpoints and `tenet-controller` validates the repo catalog and runs as a long-lived control-plane process. Mount a sample manifest repository and sample status data into both containers so local users can boot the stack without AWS credentials.

**Tech Stack:** Go standard library, Go test package, Docker multi-stage builds, Docker Compose, Markdown docs.

---

## Delivery principles

- Keep the slice landable without introducing live AWS dependencies.
- Preserve deterministic startup by continuing to validate repo and status inputs up front.
- Add sample data that exercises the existing catalog and status APIs end-to-end.
- Keep docs and local commands first-class so the repo is easy to run and inspect.
- Land Docker/runtime support before deeper reconciliation work.

## Phase map

### Phase 0 — Document the next execution shape

**Objective:** Record how the repo moves from foundations to a locally runnable control plane and what remains after that.

**Files:**
- Create: `docs/plans/2026-04-15-local-runtime-bootstrap.md`
- Modify: `README.md`
- Create: `docs/architecture/tenet-v1-local-runtime.md`

**Verification:**
- README startup commands match committed files exactly.
- Architecture doc clearly separates implemented local runtime vs deferred AWS execution.

### Phase 1 — Add example repository and runtime status fixtures

**Objective:** Provide a valid repo-shaped input tree and deployment status file that local runs and tests can share.

**Files:**
- Create: `examples/local-repo/accounts/prod-core.json`
- Create: `examples/local-repo/accounts/staging-core.json`
- Create: `examples/local-repo/environments/prod-apne2.json`
- Create: `examples/local-repo/environments/staging-apne2.json`
- Create: `examples/local-repo/deployments/payments-api-prod.json`
- Create: `examples/local-repo/deployments/payments-api-staging.json`
- Create: `examples/local-status/deployments.json`

**Step 1: Validate fixture shape with existing tests/commands**
- Run `go test ./internal/manifestfs ./internal/catalog ./internal/statusstore`
- Expected: pass before fixture use.

**Step 2: Add realistic fixture data**
- Include at least two accounts/environments and two deployments.
- Include status snapshots that expose health, sync, and condition data in the API.

**Step 3: Verify fixtures load through the existing binaries**
- Run `TENET_REPO_ROOT=$(pwd)/examples/local-repo TENET_STATUS_PATH=$(pwd)/examples/local-status/deployments.json go run ./cmd/tenet-api`
- Expected: process starts and serves read-only endpoints.

### Phase 2 — Add Docker packaging and local stack orchestration

**Objective:** Make Tenet runnable with `docker compose up` using the example fixtures.

**Files:**
- Create: `Dockerfile`
- Create: `compose.yaml`
- Modify: `Makefile`

**Step 1: Write executable/runtime expectation**
- Run `docker compose config`
- Expected: fail before Docker files exist.

**Step 2: Add a multi-stage build**
- Build both `tenet-api` and `tenet-controller` in one Dockerfile.
- Use separate compose services with explicit commands and environment.
- Mount the example repo and status fixtures read-only.

**Step 3: Add developer make targets**
- `make docker-build`
- `make docker-up`
- `make docker-down`
- `make smoke-local`

**Step 4: Verify the local stack**
- Run `docker compose up --build -d`
- Run `curl http://localhost:8080/v1/catalog/summary`
- Run `curl http://localhost:8080/v1/status/deployments/payments-api-prod`
- Expected: JSON responses with fixture-backed data.

### Phase 3 — Add automated smoke coverage for the runnable slice

**Objective:** Lock the local-runtime workflow with deterministic tests or scripts.

**Files:**
- Create: `scripts/smoke-local.sh`
- Modify: `Makefile`
- Add/modify tests only if application code changes are required.

**Step 1: Write failing smoke expectation**
- Run the smoke script before creation.
- Expected: command not found / file missing.

**Step 2: Add the smoke script**
- Validate API binary behavior without Docker when possible.
- Exercise `/healthz`, `/v1/catalog/summary`, and `/v1/status/deployments/{name}`.

**Step 3: Verify**
- Run `make smoke-local`
- Expected: pass locally and against the example fixtures.

### Phase 4 — PR hygiene and follow-on execution roadmap

**Objective:** Keep the slice small, reviewed, and clearly connected to the next implementation series.

**Files:**
- Modify only files touched above.

**Verification checklist:**
- `gofmt -w` on changed Go files if any Go code changes are made.
- `go test ./...`
- `docker compose config`
- `make smoke-local`
- independent diff review
- lore-format commit message
- push branch, open PR, monitor checks, merge when green

## Next slices after this PR

1. Controller runtime loop that periodically loads the catalog and emits structured reconciliation intents from the existing planner.
2. Persisted status-history model beyond the current single-file snapshot list.
3. Write-side APIs and controller-owned status updates.
4. AWS AssumeRole observation/execution adapters once the local runtime contract is stable.

## Progress update

Implemented before this plan:
- Read-only catalog and deployment-status APIs.
- Repository-backed manifest loading and validation.
- Controller startup validation and long-lived process behavior.
- Filesystem-backed deployment status store.

This plan’s target outcome:
- Anyone can clone the repo, run `docker compose up --build`, and inspect a working Tenet control-plane demo locally.
