# Tenet v1 Catalog Topology API

`/v1/catalog/topology` extends the read-only API surface with a single topology snapshot of the current desired catalog.

It deliberately bundles the existing lightweight read models:
- summary counts
- accounts
- environments
- deployments
- resolved placements

This keeps early UI and inspection clients simple while richer runtime status, history, and rollout-detail endpoints are still being built.

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, this snapshot uses the same trusted front-door read contract as the operator overview routes. Deployments and placements are filtered to visible deployments, accounts/environments are derived from those visible placements, and summary counts are recomputed from the filtered topology so stale or unmatched scopes return an explicit empty topology instead of leaking raw desired-state inventory.
