# Tenet v1 Catalog Deployment Listing API

`/v1/catalog/deployments` extends the read-only API surface with a stable deployment list.

It deliberately returns only the desired-state identity fields for now:
- deployment name
- application reference
- environment reference

This keeps the API useful for basic topology views while richer status, placement, and rollout detail endpoints are still being built.
