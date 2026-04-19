# Tenet v1 audit log API

Tenet now exposes a small read-only audit timeline derived from the immutable operator-action records it already persists locally.

## Route shape

- `GET /v1/audit-logs`
- `GET /v1/audit-logs?deployment={deploymentName}`
- `GET /v1/audit-logs?action={action}`
- `GET /v1/audit-logs?limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}`
- `GET /v1/audit-logs?action={action}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&limit={n}`
- `GET /v1/audit-logs?deployment={deploymentName}&action={action}&limit={n}`

`/v1/audit-logs` is the preferred v1 route shape for this slice because the rest of the API already favors direct collection nouns such as `/v1/plans`, `/v1/applies`, and `/v1/catalog/...` instead of introducing an extra `/v1/audit/logs` hierarchy.

`limit` is optional and must be a positive integer when present.
`action` is optional and must be exactly `plan.created` or `apply.requested` when present.
`deployment` is optional and must name exactly one deployment when present.
The response is always newest-first.

The compose-backed smoke keeps this operator contract grounded by fetching the full `GET /v1/audit-logs` collection, asserting the expected `plan.created` and `apply.requested` actions are present without depending on a two-entry window, then fetching `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering and rejecting `action=invalid.action` with `400 Bad Request` instead of silently widening the query.

## Source of truth

This route is additive and read-only. It does not invent a new store.

The timeline is projected from:
- immutable plan preview records persisted to `TENET_PLAN_STORE_PATH`
- immutable apply request records persisted to `TENET_APPLY_STORE_PATH`

When the local repository catalog is available, the projection also enriches entries with `appRef` and `environmentRef` by matching the persisted `deploymentName` against the compiled deployment listing.

Empty filter results stay explicit: the route returns `entries: []` with `200 OK` when the selected deployment exists but has no persisted audit records yet.

## Response contract

The response shape is:

```json
{
  "entries": [
    {
      "timestamp": "2026-04-18T08:00:00Z",
      "action": "apply.requested",
      "deploymentName": "payments-api-prod",
      "appRef": "payments-api",
      "environmentRef": "prod-apne2",
      "planId": "plan_1234abcd",
      "applyId": "apply_5678efgh",
      "actor": {
        "available": true,
        "id": "operator-123",
        "name": "Jane Operator",
        "email": "jane@example.com"
      },
      "metadata": {
        "status": "failed",
        "planStatus": "ready",
        "sourceFingerprint": "plan_abcd1234",
        "error": "apply execution is disabled",
        "details": [
          "Tenet persisted this immutable apply request in read-only mode and did not contact AWS."
        ]
      },
      "source": {
        "kind": "applyRequest",
        "store": "applyRequestStore",
        "recordRoute": "/v1/applies/apply_5678efgh",
        "sourceRoute": "/v1/plans/plan_1234abcd"
      }
    }
  ]
}
```

Important details:
- `action` is stable and action-oriented (`plan.created`, `apply.requested`)
- the optional query `action` filter only supports those two exact values and rejects anything else with `400 Bad Request`
- `timestamp` is copied from the persisted immutable record
- `deploymentName`, `appRef`, and `environmentRef` help local operators and a future UI group related rows without reopening every record
- `planId` / `applyId` link the timeline back to the immutable read models Tenet already exposes
- `metadata` carries explicit action-specific details such as status, risk, fingerprint, error, and details
- `source` points to both the immutable record route and the parent route the record was derived from
- apply-request entries default to `metadata.status="failed"` plus the disabled error/details shown above; when `TENET_AWS_ENABLE_APPLY=true`, newly created apply-request entries persist as `metadata.status="queued"` without an error while still remaining mutation-free in this backend slice

## Auth and actor data

This slice does not introduce fake auth data.

When request-scoped operator context is available, Tenet persists it additively from explicit local-friendly headers:
- `X-Tenet-Actor-Id`
- `X-Tenet-Actor-Name`
- `X-Tenet-Actor-Email`
- compatible aliases `X-User-ID`, `X-User-Name`, and `X-User-Email`

Those fields appear unchanged on the immutable `plan` / `apply` records and on their projected `GET /v1/audit-logs` entries. When both header families are present, Tenet prefers the explicit `X-Tenet-Actor-*` values over `X-User-*` aliases. When no authenticated request context was present, Tenet keeps the contract explicit by returning `actor.available=false` plus an unavailable reason instead of inventing an identity.
