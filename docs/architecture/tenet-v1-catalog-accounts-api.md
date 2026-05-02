# Tenet v1 Catalog Account Listing API

`/v1/catalog/accounts` extends the read-only API surface with a stable account list.

It deliberately returns only the desired-state identity and placement guardrail fields for now:
- account name
- AWS account ID
- allowed regions

This keeps the API useful for early account topology and placement views while richer account health and credential-status endpoints are still being built.

When `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` is enabled, this collection uses the same trusted front-door read contract as the operator overview routes. Accounts are derived from visible deployment placements only, so stale or unmatched forwarded allowlists fail closed with an empty `[]` rather than exposing every account in the raw catalog.
