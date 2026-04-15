# Tenet Runtime State Storage

Tenet now supports a pluggable deployment-status store.

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

## Seeding behavior

When `TENET_STATUS_SEED_PATH` is provided, Tenet seeds the selected store only if the store is empty.

This makes local Docker flows convenient:
- committed example JSON remains the bootstrap seed
- Redis becomes the active runtime store
- future controller write paths can stop depending on a mounted file

## Current scope

This slice persists only the current deployment status snapshot.

Runtime history, event streams, and audit logs are still future work.
