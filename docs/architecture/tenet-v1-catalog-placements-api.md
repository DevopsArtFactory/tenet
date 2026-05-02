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

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, this collection uses the same trusted front-door read contract as the operator overview routes. Placement rows are filtered to deployments visible through exact deployment grants or matching project/environment scopes; stale or unmatched forwarded scopes return an empty `[]` instead of exposing unrelated placement metadata.
