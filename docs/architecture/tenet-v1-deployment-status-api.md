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
- `status.history[]` may include the bounded controller-authored runtime timeline with timestamp, source, summary, additive `changes[]` string labels, sync, health, desired/observed generation metadata, rollout phase, and optional current step

The deployment overview read models now intentionally copy the latest artifact/condition context into their additive `runtimeSummary` contract as well:
- `/v1/overview/applications`, `/v1/overview/accounts`, and `/v1/overview/deployments/{deploymentName}` may expose `runtimeSummary.deployedArtifact`
- those same joined/detail routes may expose `runtimeSummary.conditions[]` as concise latest-runtime condition context copied from the newest runtime signal
- bounded `history[]` entries on `/v1/overview/deployments/{deploymentName}` may also carry additive `deployedArtifact` / `conditions` fields when the controller recorded them for that point-in-time runtime signal
- `/v1/overview/deployments/{deploymentName}` now also exposes additive `sourceFingerprint`, a deterministic hash derived from the same rollout/runtime/detail inputs Tenet uses for plan-preview optimistic matching
- `/v1/overview/applications/{deploymentName}` preserves that same `sourceFingerprint` field because the application detail route currently keeps deployment-detail semantics aligned

That additive copy keeps overview/detail consumers from having to diff the full `status` object or scrape the newest history entry just to show the latest runtime artifact/condition context in a compact UI row.

These payloads are still deterministic persisted read models, but they now document the richer runtime shape Tenet serves today. The bounded runtime-history retention cap remains 10 entries per deployment.
