# Tenet Rollout Runtime and UI Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Extend Tenet toward a real multi-account deployment control plane by adding richer rollout intent, a Redis-backed runtime-state option, and a minimal account-oriented UI.

**Architecture:** Keep Tenet read-model-first for this slice. Normalize richer deployment strategies in `spec` and `manifest`, expose a joined overview API that combines desired catalog data with runtime status, and introduce a pluggable status-store interface with filesystem and Redis backends. Use the new overview payload as the single source of truth for a minimal embedded UI grouped by account and deployment.

**Tech Stack:** Go standard library, embedded static assets, `github.com/redis/go-redis/v9`, `github.com/alicebob/miniredis/v2`, Docker Compose, Markdown docs.

---

## Rollout design decisions for this slice

**EC2:**
- Blue/green remains the default immutable-host strategy.
- Canary and linear should be modeled as progressive traffic shifts over a newly created Auto Scaling Group plus a new target group, fronted by ALB weighted forwarding.
- `userdata.sh` remains the bootstrap mechanism for the candidate ASG.

**ECS:**
- Rolling remains native ECS rolling.
- Blue/green, canary, and linear should map to ECS services that use the CodeDeploy deployment controller.

**Control-plane modeling:**
- Keep user-facing strategy names (`blueGreen`, `rolling`, `canary`, `linear`).
- Expose a target-specific execution recommendation (`asgWeightedTargetGroups`, `ecsRolling`, `ecsCodeDeploy`) so the UI and future controller runtime can explain how Tenet will execute the intent.

## Phase 0 — Document the direction

**Objective:** Capture the rollout/runtime/UI direction in repo docs before implementation drifts.

**Files:**
- Modify: `README.md`
- Create: `docs/architecture/tenet-v1-rollout-strategy-model.md`
- Create: `docs/architecture/tenet-v1-runtime-state-storage.md`
- Create: `docs/architecture/tenet-v1-overview-ui.md`
- Create: `docs/plans/2026-04-15-rollout-runtime-ui.md`

## Phase 1 — Add failing tests for richer rollout modeling

**Objective:** Lock the desired rollout and overview payloads before implementation.

**Files:**
- Modify: `internal/spec/validate_test.go`
- Modify: `internal/manifest/compiler_test.go`
- Modify: `internal/catalog/deployments_test.go`
- Create: `internal/catalog/overview_test.go`
- Modify: `internal/api/router_test.go`

**Coverage target:**
- `canary` and `linear` deployment strategies validate.
- EC2 strategy recommendations resolve to weighted ASG/target-group execution.
- ECS strategy recommendations resolve to rolling or CodeDeploy execution.
- overview payload joins account/environment/deployment/status data.
- root UI route and overview API route both respond.

## Phase 2 — Add a pluggable status store with Redis support

**Objective:** Replace the file-only assumption with a storage seam that still works locally.

**Files:**
- Create: `internal/statusstore/store.go`
- Modify: `internal/statusstore/file.go`
- Modify: `internal/statusstore/file_test.go`
- Modify: `internal/statusstore/list.go`
- Modify: `internal/statusstore/list_test.go`
- Create: `internal/statusstore/redis.go`
- Create: `internal/statusstore/redis_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `cmd/tenet-api/main.go`
- Modify: `cmd/tenet-controller/runtime.go`
- Modify: `compose.yaml`

**Implementation outline:**
- Define a `Store` interface with `List`, `Get`, and `Save` methods.
- Keep the filesystem implementation compatible with the current JSON snapshot format.
- Add a Redis implementation keyed by deployment name and a sorted index for stable listing.
- Allow runtime selection with env such as `TENET_STATUS_BACKEND`, `TENET_STATUS_PATH`, and `TENET_REDIS_ADDR`.

## Phase 3 — Add rollout execution recommendations and joined overview API

**Objective:** Surface the deployment intent in a way a future controller and current UI can both consume.

**Files:**
- Modify: `internal/spec/types.go`
- Modify: `internal/spec/validate.go`
- Modify: `internal/domain/deployment.go`
- Modify: `internal/manifest/compiler.go`
- Modify: `internal/catalog/deployments.go`
- Create: `internal/catalog/overview.go`
- Modify: `internal/catalog/provider.go`
- Modify: `internal/api/router.go`

**Implementation outline:**
- Add `canary` and `linear` strategy support.
- Add deployment rollout recommendation fields such as strategy, execution mode, and target summary.
- Introduce `/v1/overview/accounts` as a grouped read model for the UI.

## Phase 4 — Add a minimal embedded UI

**Objective:** Make the local Docker stack show a real browser UI grouped by account and deployment.

**Files:**
- Create: `internal/api/ui.go`
- Create: `web/index.html`
- Modify: `scripts/smoke-local.sh`
- Modify: `examples/local-repo/deployments/payments-api-prod.yaml`
- Modify: `examples/local-repo/deployments/payments-api-staging.yaml`
- Modify: `examples/local-status/deployments.json`

**UI intent:**
- Account cards with nested deployment rows.
- Strategy, target, desired artifact, deployed artifact, sync, and health shown together.
- Explanation chips for EC2 weighted canary / ECS CodeDeploy-backed traffic shifts.

## Phase 5 — Verification and PR hygiene

**Objective:** Prove the local runtime still works end to end.

**Verification checklist:**
- `go test ./...`
- `make smoke-local`
- `docker compose up --build -d`
- `curl http://localhost:8080/v1/overview/accounts`
- `curl http://localhost:8080/`
- `docker compose down --remove-orphans`
- independent review
- conventional commit messages
- push branch, open PR, merge when green
