# Tenet v1 Catalog Summary API

`/v1/catalog/summary` is the first API bridge from repository-backed desired state into a UI-friendly response.

It deliberately returns only counts for now:
- number of accounts
- number of environments
- number of deployments

This keeps the endpoint stable while the richer status and topology APIs are still being built.

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, the summary is recomputed from the same filtered topology used by the guarded raw catalog routes. Stale or unmatched forwarded allowlists return zero counts rather than leaking aggregate catalog size.
