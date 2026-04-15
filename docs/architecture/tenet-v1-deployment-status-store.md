# Tenet v1 Deployment Status Store

The first status-persistence slice uses a filesystem-backed JSON store for deployment status snapshots.

Current behavior:
- snapshots are stored as a JSON array keyed by deployment name
- reads and writes are deterministic and sorted for reviewable diffs
- snapshots can now persist observed generation, sync reason, health summary, and conditions
- the store is intentionally small and local so status semantics can stabilize before introducing a database

This gives the control plane a concrete persistence seam for runtime deployment status without committing yet to a relational schema or external service.
