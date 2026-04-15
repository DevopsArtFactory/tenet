# Tenet v1 Catalog Account Listing API

`/v1/catalog/accounts` extends the read-only API surface with a stable account list.

It deliberately returns only the desired-state identity and placement guardrail fields for now:
- account name
- AWS account ID
- allowed regions

This keeps the API useful for early account topology and placement views while richer account health and credential-status endpoints are still being built.
