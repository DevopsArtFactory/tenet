# Tenet v1 Deployment Status API

`/v1/status/deployments` is the first runtime-status read endpoint.

It currently returns filesystem-backed deployment status snapshots containing:
- deployment name
- observed generation
- sync state and reason
- overall health summary
- conditions

This keeps the first status API narrow and deterministic while richer persisted rollout, health, condition, and drift detail are still being built.
