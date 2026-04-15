# Tenet v1 Catalog Environment Listing API

`/v1/catalog/environments` extends the read-only API surface with a stable environment list.

It deliberately returns only the desired-state identity and placement fields for now:
- environment name
- account reference
- region
- network reference

This keeps the API useful for early topology and placement views while richer status and rollout detail endpoints are still being built.
