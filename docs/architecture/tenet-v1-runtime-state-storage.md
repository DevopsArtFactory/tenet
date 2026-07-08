# Tenet Runtime State Storage

Tenet now supports a pluggable deployment-status store for the current status snapshot plus a bounded controller-authored runtime-history slice persisted alongside each deployment.

## Supported backends

### Filesystem
Environment:
- `TENET_STATUS_BACKEND=file`
- `TENET_STATUS_PATH=/path/to/deployments.json`

This preserves the original JSON snapshot workflow while still allowing the controller to apply one reconcile result through a batch read-modify-write save when the filesystem backend is active. When a reconcile introduces changes or stale deletes, that compatibility path still rewrites the underlying JSON file from the full merged snapshot set, so it is not the same write profile as Redis-backed transactional batch mutations; when nothing changed, the controller now skips the no-op batch entirely.

### Redis
Environment:
- `TENET_STATUS_BACKEND=redis`
- `TENET_REDIS_ADDR=host:6379`
- optional `TENET_REDIS_KEY_PREFIX=tenet:status`
- optional `TENET_STATUS_SEED_PATH=/path/to/deployments.json`

Redis stores snapshots per deployment name and keeps an index set for stable listing. When the controller opens a Redis-backed runtime store, it now prefers a single batch apply instead of rewriting the full snapshot set on every reconcile tick. Batch-capable stores now receive only changed/new deployment snapshots plus stale deletes, and Redis still applies that reduced mutation set transactionally through per-deployment `Upsert`/`Delete` operations inside one transaction.

Because runtime history is additive JSON on the same deployment snapshot, both the filesystem and Redis backends persist the controller-authored bounded `history[]` entries without requiring a separate event-store schema in this slice.

## Seeding behavior

When `TENET_STATUS_SEED_PATH` is provided, Tenet seeds the selected store only if the store is empty.

This makes local Docker flows convenient:
- committed example JSON remains the bootstrap seed
- Redis becomes the active runtime store
- controller-owned additive write paths can stop depending on mounted full-store rewrites even though the seed file remains available for bootstrap

## Current scope

This slice persists the current deployment status snapshot and a bounded recent runtime-history timeline authored by the controller. Each history entry captures the latest operator-facing summary of timestamp, summary text, sync state, health, additive desired/observed generation metadata, rollout phase, and optional rollout step so the overview deployment-detail read model can expose recent changes without requiring a separate event stream. The controller now prefers batch write primitives when available: batch-capable stores receive only changed/new snapshots plus stale deletes, Redis applies that reduced reconcile result in one transactional batch while still using per-deployment `Upsert`/`Delete` operations internally, and when nothing changed the controller skips the batch write entirely. Plain `Store.Save` backends still receive the full compiled snapshot set, while the filesystem compatibility path continues to rewrite the full merged JSON snapshot set when changes or deletes exist so atomic compatibility is preserved. Persisted snapshots for deployments removed from the compiled repo are pruned as part of the same reconcile write. `TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT` controls the per-deployment controller runtime-history retention limit, defaults to `10`, and invalid, zero, negative, or above-1000 values fail controller startup; deployment-events retention metadata reports the effective controller limit so operators can distinguish a full retained controller-history window from a smaller `?limit=` query window.

Long-lived runtime history and event streams are still future work. A small local read-only audit projection now exists at `GET /v1/audit-logs`, but comprehensive durable audit storage remains future work.
