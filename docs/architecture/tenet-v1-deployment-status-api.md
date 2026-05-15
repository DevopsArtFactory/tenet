# Tenet v1 Deployment Status API

`/v1/status/deployments` is the first runtime-status read endpoint family.

It currently supports:
- `GET /v1/status/deployments`
- `GET /v1/status/deployments/{deploymentName}`

The payloads now expose the current persisted runtime snapshot shape rather than only the original narrow status core:
- `GET /v1/status/deployments` returns an array of snapshot objects
- `GET /v1/status/deployments/{deploymentName}` returns one snapshot object
- each snapshot contains `deploymentName` plus a `status` object
- `status` always includes `observedGeneration`, `sync`, `health`, and `conditions`
- before the controller persists a snapshot, it now normalizes controller-owned runtime defaults when those fields are still structurally empty: completed `MarkSynced` plans default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Synced` / `Healthy` and backfill `observedGeneration=deployment.Generation` when it is still empty, actionable plans default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Reconciling` / `Progressing` with controller-authored reason/summary text derived from the current rollout step, and paused non-actionable plans default an entirely empty `sync` struct and/or an entirely empty `health` struct to `Paused` / `Unknown` while preserving any already-seeded sync, health, conditions, artifact, history, or observed-generation values already present in storage
- `status.deployedArtifact` carries deployed binary/image metadata when known
- `status.rollout` may include controller-derived rollout generations, phase/current-step/summary fields, ordered `steps[]` entries, per-step `status` (`completed`, `current`, `pending`), optional per-step `detail`, and additive `execution[]` resource details
- `status.history[]` may include the bounded controller-authored runtime timeline with timestamp, source, summary, additive `changes[]` string labels, sync, health, desired/observed generation metadata, rollout phase, optional current step, and optional `controllerRefresh` evidence when a Redis GitHub desired-state refresh handoff triggered the reconcile

The deployment overview read models now intentionally copy the latest artifact/condition context into their additive `runtimeSummary` contract as well:
- `/v1/overview/applications`, `/v1/overview/accounts`, and `/v1/overview/deployments/{deploymentName}` may expose `runtimeSummary.deployedArtifact`
- those same joined/detail routes may expose `runtimeSummary.conditions[]` as concise latest-runtime condition context copied from the newest runtime signal
- bounded `history[]` entries on `/v1/overview/deployments/{deploymentName}` may also carry additive `deployedArtifact` / `conditions` fields when the controller recorded them for that point-in-time runtime signal
- `/v1/overview/deployments/{deploymentName}` now also exposes additive `sourceFingerprint`, a deterministic hash derived from the same rollout/runtime/detail inputs Tenet uses for plan-preview optimistic matching
- `/v1/overview/applications/{deploymentName}` preserves that same `sourceFingerprint` field because the application detail route currently keeps deployment-detail semantics aligned

That additive copy keeps overview/detail consumers from having to diff the full `status` object or scrape the newest history entry just to show the latest runtime artifact/condition context in a compact UI row.

These payloads are still deterministic persisted read models, but they now document the richer runtime shape Tenet serves today. The bounded runtime-history retention cap remains 10 entries per deployment; `GET /v1/overview/deployments/{deploymentName}/events` exposes that retained-history cap separately from `?limit=` result-scope truncation metadata.

## Front-door auth

`TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` now covers this raw status family too.

- `GET /v1/status/deployments` requires trusted `X-Tenet-Authenticated-*` identity and filters returned snapshots through the union of exact `X-Tenet-Authorized-Deployments` grants and matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement.
- `GET /v1/status/deployments` returns `200 OK` with `[]` when trusted deployment names and project/environment scopes are omitted, stale, or match no persisted snapshots.
- `GET /v1/status/deployments/{deploymentName}` allows exact deployment membership in that same allowlist or matching project/environment catalog placement; partial project-only or environment-only scopes fail safe with the same deployment-access denial.
- when enabled, missing trusted authenticated identity returns `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route`.
- when `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, recognized identity without a trusted read-capable `X-Tenet-Authenticated-Roles` value from `viewer` / `operator` / `admin` returns `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route`.
- when enabled, recognized identity without access to the requested deployment returns `403 Forbidden` plus `forbidden: operator does not have access to this deployment`.
- when disabled, both status routes keep their previous unauthenticated local-runtime behavior unchanged.
- The same deployment-access model now covers immutable deployment-scoped writes: `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` require trusted identity plus either an exact `X-Tenet-Authorized-Deployments` grant for the target deployment or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` catalog placement.
- `make smoke-docker` restarts the Compose `api` service with trusted-header auth enabled and proves this exact route contract: unauthenticated `GET /v1/status/deployments` returns `401`, allowlisted collection reads return only authorized rows, matching project/environment scopes can authorize the same read surface without an exact deployment-name grant, and when both `payments-api-prod` and `worker-prod` are allowlisted, the Compose smoke proves the controller-populated raw status collection returns both rows; omitting all deployment/project/environment grants returns `[]`, out-of-scope `GET /v1/status/deployments/{deploymentName}` returns `403`, and one in-scope detail read succeeds.

Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values.
