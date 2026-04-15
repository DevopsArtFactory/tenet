# Tenet Deployment Manifest Compatibility Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make Tenet deployment manifests YAML-first and compatible with the goployer EC2 manifest shape while carrying deployable artifact metadata for EC2 binaries and ECS images.

**Architecture:** Extend `internal/manifestfs` so it loads JSON or YAML manifests, then expand `spec.DeploymentSpec` to accept a goployer-style `stacks` structure with stack/account/env/replacement_type/capacity/regions data. Keep compilation centralized so Tenet can normalize that richer manifest into the current domain model while also exposing desired and deployed artifact metadata.

**Tech Stack:** Go standard library, `gopkg.in/yaml.v3`, existing manifest/catalog/statusstore packages, Go tests, Markdown docs.

---

## Phase 0 — Document the compatibility slice

**Objective:** Capture the manifest-format and artifact-model change in repo docs.

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture/tenet-v1-filesystem-loader.md`
- Create: `docs/plans/2026-04-15-deployment-yaml-manifests.md`

## Phase 1 — Add failing loader and spec tests

**Objective:** Lock the new compatibility expectations before implementation.

**Files:**
- Modify: `internal/manifestfs/loader_test.go`
- Modify: `internal/spec/validate_test.go`
- Modify: `internal/manifest/compiler_test.go`
- Modify: `internal/catalog/deployments_test.go`
- Modify: `internal/statusstore/file_test.go`

**Coverage target:**
- YAML and YML deployment manifests load correctly.
- Invalid YAML is rejected.
- goployer-style EC2 manifests validate and compile.
- ECS manifests can carry an image artifact.
- status snapshots round-trip deployed binary/image metadata.

## Phase 2 — Implement YAML + goployer-compatible deployment normalization

**Objective:** Accept the richer manifest shape without rewriting downstream components.

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/manifestfs/loader.go`
- Modify: `internal/spec/types.go`
- Modify: `internal/spec/validate.go`
- Modify: `internal/domain/deployment.go`
- Modify: `internal/domain/status.go`
- Modify: `internal/manifest/compiler.go`
- Modify: `internal/catalog/deployments.go`

**Implementation outline:**
- Load `.json`, `.yaml`, and `.yml` manifest files.
- Add goployer-compatible stack/region/userdata structures to `DeploymentSpec`.
- Normalize app/environment/target/strategy/capacity from either Tenet-native or goployer-style fields.
- Track desired artifact metadata on the deployment domain object.
- Track deployed artifact metadata on deployment status.

## Phase 3 — Migrate local examples to YAML-first deployment manifests

**Objective:** Make the committed demo reflect the preferred authoring model.

**Files:**
- Remove: `examples/local-repo/deployments/payments-api-prod.json`
- Remove: `examples/local-repo/deployments/payments-api-staging.json`
- Create: `examples/local-repo/deployments/payments-api-prod.yaml`
- Create: `examples/local-repo/deployments/payments-api-staging.yaml`
- Modify: `examples/local-status/deployments.json`

**Example intent:**
- EC2 deployment uses a goployer-style stack manifest plus a `binary` artifact.
- ECS deployment uses the same stack shape plus an `image` artifact.
- Status snapshots carry the currently deployed binary or image.

## Phase 4 — Verification and PR hygiene

**Objective:** Prove the local stack still works end to end.

**Verification checklist:**
- `go test ./...`
- `make smoke-local`
- `docker compose up --build -d`
- `curl http://localhost:8080/v1/catalog/deployments`
- `curl http://localhost:8080/v1/status/deployments/payments-api-prod`
- `docker compose down --remove-orphans`
- independent review
- lore-format commit message
- push branch, open PR, merge when green
