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

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, this collection uses the same trusted front-door read contract as the operator overview routes: callers need trusted authenticated identity, a viewer-capable role when role enforcement is enabled, and visibility through `X-Tenet-Authorized-Deployments` or matching project/environment scopes. Rows outside that visibility set are filtered out, and an unmatched allowlist returns an explicit empty `[]` rather than leaking raw desired-state metadata.
