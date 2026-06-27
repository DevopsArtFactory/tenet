# Tenet v1 Catalog Environment Listing API

`/v1/catalog/environments` extends the read-only API surface with a stable environment list.

It deliberately returns only the desired-state identity and placement fields for now:
- environment name
- account reference
- region
- network reference

This keeps the API useful for early topology and placement views while richer status and rollout detail endpoints are still being built.

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, this collection uses the same trusted front-door read contract as the operator overview routes. Environments are derived from visible deployment placements only, so stale or unmatched forwarded allowlists fail closed with `200 OK` and an empty `[]` rather than exposing every environment in the raw catalog. If placement data is missing or unavailable, Tenet uses that same explicit empty collection because it cannot prove which raw environments are visible to the caller.
