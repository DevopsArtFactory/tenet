# Tenet Runtime State Storage

Tenet now supports a pluggable deployment-status store for the current status snapshot plus a bounded controller-authored runtime-history slice persisted alongside each deployment.

## Supported backends

### Filesystem
Environment:
- `TENET_STATUS_BACKEND=file`
- `TENET_STATUS_PATH=/path/to/deployments.json`

This preserves the original JSON snapshot workflow while still allowing the controller to apply one reconcile result through a batch read-modify-write save when the filesystem backend is active. That compatibility path still rewrites the underlying JSON file, so it is not the same write profile as Redis-backed transactional batch mutations.

### Redis
Environment:
- `TENET_STATUS_BACKEND=redis`
- `TENET_REDIS_ADDR=host:6379`
- optional `TENET_REDIS_KEY_PREFIX=tenet:status`
- optional `TENET_STATUS_SEED_PATH=/path/to/deployments.json`

Redis stores snapshots per deployment name and keeps an index set for stable listing. When the controller opens a Redis-backed runtime store, it now prefers a single batch apply instead of rewriting the full snapshot set on every reconcile tick. Inside that Redis batch, Tenet still expresses the reconcile result as per-deployment `Upsert`/`Delete` mutations, and the current controller loop still includes every compiled deployment on each tick, so the optimization is transactional per-deployment persistence rather than change-only persistence.

Because runtime history is additive JSON on the same deployment snapshot, both the filesystem and Redis backends persist the controller-authored bounded `history[]` entries without requiring a separate event-store schema in this slice.

## Seeding behavior

When `TENET_STATUS_SEED_PATH` is provided, Tenet seeds the selected store only if the store is empty.

This makes local Docker flows convenient:
- committed example JSON remains the bootstrap seed
- Redis becomes the active runtime store
- controller-owned additive write paths can stop depending on mounted full-store rewrites even though the seed file remains available for bootstrap

## Current scope

This slice persists the current deployment status snapshot and a bounded recent runtime-history timeline authored by the controller. Each history entry captures the latest operator-facing summary of timestamp, summary text, sync state, health, additive desired/observed generation metadata, rollout phase, and optional rollout step so the overview deployment-detail read model can expose recent changes without requiring a separate event stream. The controller now prefers batch write primitives when available: Redis applies the full reconcile result in one transactional batch while still using per-deployment `Upsert`/`Delete` operations internally, and the filesystem backend keeps compatibility through one read-modify-write batch save. The current controller still includes every compiled deployment in each batch, and it prunes persisted snapshots for deployments removed from the compiled repo as part of that same reconcile write. The current runtime-history retention cap is 10 entries per deployment.

Long-lived runtime history, event streams, and audit logs are still future work.
