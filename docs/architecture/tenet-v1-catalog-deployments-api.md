# Tenet v1 Catalog Deployment Listing API

`/v1/catalog/deployments` extends the read-only API surface with a stable deployment list.

It deliberately returns the desired-state identity and artifact fields for now:
- deployment name
- application reference
- environment reference
- target type
- desired `binary` artifact for EC2 when declared
- desired `image` artifact for ECS when declared

This keeps the API useful for basic topology views while richer status, placement, and rollout detail endpoints are still being built.
