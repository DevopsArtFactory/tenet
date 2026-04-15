# Tenet Runtime State Storage

Tenet now supports a pluggable deployment-status store for the current status snapshot plus a bounded controller-authored runtime-history slice persisted alongside each deployment.

## Supported backends

### Filesystem
Environment:
- `TENET_STATUS_BACKEND=file`
- `TENET_STATUS_PATH=/path/to/deployments.json`

This preserves the original JSON snapshot workflow.

### Redis
Environment:
- `TENET_STATUS_BACKEND=redis`
- `TENET_REDIS_ADDR=host:6379`
- optional `TENET_REDIS_KEY_PREFIX=tenet:status`
- optional `TENET_STATUS_SEED_PATH=/path/to/deployments.json`

Redis stores snapshots per deployment name and keeps an index set for stable listing.

Because runtime history is additive JSON on the same deployment snapshot, both the filesystem and Redis backends persist the controller-authored bounded `history[]` entries without requiring a separate event-store schema in this slice.

## Seeding behavior

When `TENET_STATUS_SEED_PATH` is provided, Tenet seeds the selected store only if the store is empty.

This makes local Docker flows convenient:
- committed example JSON remains the bootstrap seed
- Redis becomes the active runtime store
- future controller write paths can stop depending on a mounted file

## Current scope

This slice persists the current deployment status snapshot and a bounded recent runtime-history timeline authored by the controller. Each history entry captures the latest operator-facing summary of timestamp, summary text, sync state, health, and optional rollout step so the overview deployment-detail read model can expose recent changes without requiring a separate event stream.

Long-lived runtime history, event streams, and audit logs are still future work.
