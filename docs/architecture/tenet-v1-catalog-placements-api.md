# Tenet v1 Catalog Placement Listing API

`/v1/catalog/placements` extends the read-only API surface with a stable deployment placement list.

It deliberately returns only resolved deployment-to-placement fields for now:
- deployment name
- application reference
- environment reference
- account name and AWS account ID
- region
- network reference

This keeps the API useful for early topology and placement inspection while richer runtime status and rollout views are still being built.
