# Tenet v1 Deployment Status API

`/v1/status/deployments` is the first runtime-status read endpoint family.

It currently supports:
- `GET /v1/status/deployments`
- `GET /v1/status/deployments/{deploymentName}`

The payloads are filesystem-backed deployment status snapshots containing:
- deployment name
- observed generation
- sync state and reason
- overall health summary
- conditions
- deployed artifact metadata (`binary` for EC2 or `image` for ECS when known)

This keeps the first status API narrow and deterministic while richer persisted rollout, health, condition, and drift detail are still being built.
