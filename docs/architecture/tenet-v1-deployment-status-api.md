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
- `status.deployedArtifact` carries deployed binary/image metadata when known
- `status.rollout` may include controller-derived rollout generations, phase/current-step/summary fields, ordered `steps[]` entries, per-step `status` (`completed`, `current`, `pending`), optional per-step `detail`, and additive `execution[]` resource details
- `status.history[]` may include the bounded controller-authored runtime timeline with timestamp, source, summary, sync, health, desired/observed generation metadata, rollout phase, and optional current step

These payloads are still deterministic persisted read models, but they now document the richer runtime shape Tenet serves today. The bounded runtime-history retention cap remains 10 entries per deployment.
