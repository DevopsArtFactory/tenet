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
`action` is optional and must be exactly `plan.created`, `apply.requested`, or `apply.transitioned` when present.
`deployment` is optional and must name exactly one deployment when present.
The response is always newest-first. When trusted front-door deployment-read authorization is enabled on an unscoped collection request, Tenet applies action filtering first, filters entries through the caller's authorized deployment set, and only then applies `limit`; this preserves `limit={n}` as a window over entries the operator is allowed to see instead of over hidden deployments.

The compose-backed smoke keeps this operator contract grounded by fetching the full `GET /v1/audit-logs` collection, asserting the expected `plan.created`, `apply.requested`, and lifecycle `apply.transitioned` action support is present without depending on a two-entry window, then fetching `GET /v1/audit-logs?action=apply.requested&limit=2` to verify exact-action filtering and rejecting `action=invalid.action` with `400 Bad Request` instead of silently widening the query. When a trusted front-door collection filter legitimately removes every row, Tenet still returns `entries: []` with `resultScope.action`, count, limit, and truncation metadata so operators can distinguish an empty authorized view from a route failure. Under trusted front-door auth, the same read surface documents `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator read route` for missing trusted identity and the existing deployment/role-aware `403` cases for authenticated-but-disallowed callers. Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values. A directly exposed header-trusting API instance is not a safe deployment shape. `X-Tenet-Actor-*` values still win over `X-User-*` aliases for local actor context. Tenet projects that authenticated identity into the persisted actor fields for immutable plan/apply records and therefore into their projected audit entries.

## Source of truth

This route is additive and read-only. It does not invent a new store.

The timeline is projected from:
- immutable plan preview records persisted to `TENET_PLAN_STORE_PATH`
- immutable apply request records persisted to `TENET_APPLY_STORE_PATH`

When the local repository catalog is available, the projection also enriches entries with `appRef` and `environmentRef` by matching the persisted `deploymentName` against the compiled deployment listing.

Empty filter results stay explicit: the route returns `entries: []` with `200 OK` when the selected deployment exists but has no persisted audit records yet. The sibling `resultScope` still reports the applied deployment/action filters and zero returned/total entries so callers can distinguish an empty matching timeline from a missing metadata field.

## Response contract

The response shape is:

```json
{
  "entries": [
    {
      "entryId": "audit_apply_4f7a1b2c3d4e5f60",
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
  ],
  "resultScope": {
    "returnedEntryCount": 1,
    "returnedCount": 1,
    "totalEntryCount": 3,
    "limited": true,
    "limit": 1,
    "requestedLimit": 1,
    "effectiveLimit": 1,
    "truncationStatus": "truncated",
    "truncated": true,
    "deploymentName": "payments-api-prod",
    "action": "apply.requested"
  }
}
```

Important details:
- `resultScope` is server-authored bounded-read metadata for the exact response: `returnedEntryCount` is the number of rows in `entries`, `returnedCount` is its compact alias, `totalEntryCount` is the number of matching rows after deployment/action/authorization filtering but before the requested `limit`, `limited` is true only when the returned window is capped, legacy `limit` and additive `requestedLimit` echo the positive requested cap when present, `effectiveLimit` is the cap Tenet applied (or `0` for uncapped reads), `truncationStatus` is `truncated` or `not_truncated` for known-complete local reads, `truncated` is the boolean alias for that status, and `deploymentName` / `action` echo the normalized filters that Tenet applied
- `entryId` is a backend-authored opaque stable row identifier derived from the immutable plan/apply source identity; it is unique across `plan.created`, `apply.requested`, and `apply.transitioned` rows and remains stable across filtering, sorting, and `limit` windows so UI anchors can prefer it over client-derived fallbacks
- `action` is stable and action-oriented (`plan.created`, `apply.requested`, `apply.transitioned`)
- the optional query `action` filter only supports those three exact values and rejects anything else with `400 Bad Request`
- `timestamp` is copied from the persisted immutable record; for `apply.transitioned` rows it is copied from the persisted apply transition entry
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

These fields appear unchanged on the immutable `plan` / `apply` records and on their projected `GET /v1/audit-logs` entries. When both header families are present, Tenet prefers the explicit `X-Tenet-Actor-*` values over `X-User-*` aliases. When no authenticated request context was present, Tenet keeps the contract explicit by returning `actor.available=false` plus an unavailable reason instead of inventing an identity.

`X-Tenet-Actor-*` values still win over `X-User-*` aliases for local actor context.

Tenet can also consume trusted front-door identity headers when the API instance is configured for them. Set `TENET_API_TRUST_AUTHENTICATED_HEADERS` to trust `X-Tenet-Authenticated-Id`, `X-Tenet-Authenticated-Name`, and/or `X-Tenet-Authenticated-Email`; when those explicit trusted fields are absent, that same trusted authenticated-identity path also accepts additive `X-User-ID`, `X-User-Name`, and/or `X-User-Email` aliases. `X-Tenet-Authenticated-*` values still win over `X-User-*` aliases for the trusted authenticated identity path. Set `TENET_API_ENFORCE_AUTHENTICATED_IDENTITY` when `POST /v1/plans/deployments/{deploymentName}` and `POST /v1/applies/plans/{planId}` must reject unauthenticated requests with `401 Unauthorized` plus `unauthenticated: authenticated identity is required for this operator write route`; enabling enforcement also trusts those forwarded identity headers for that API instance. Set `TENET_API_ENFORCE_DEPLOYMENT_READ_AUTHORIZATION` when deployment-scoped reads and immutable writes must also require either a trusted `X-Tenet-Authorized-Deployments` header containing a comma-separated exact deployment allowlist or matching `X-Tenet-Authorized-Projects` plus `X-Tenet-Authorized-Environments` scopes backed by catalog placement. When `TENET_API_ENFORCE_OPERATOR_ROLE_AUTHORIZATION` is also enabled, those same guarded read routes additionally require one trusted forwarded `X-Tenet-Authenticated-Roles` value from `viewer`, `operator`, or `admin`; authenticated callers without a read-capable role now fail with `403 Forbidden` plus `forbidden: authenticated viewer-capable role is required for this operator read route` before any deployment allowlist filtering happens. That same deployment-access check also gates `POST /v1/plans/deployments/{deploymentName}`, `POST /v1/applies/plans/{planId}`, and `POST /v1/applies/{applyId}/transitions` using the requested deployment or persisted plan/apply deployment before Tenet creates, reuses, or advances immutable records; project/environment scoped writes fail closed when catalog placement data is unavailable. Deployment-scoped `GET /v1/audit-logs?deployment={deploymentName}` and its action/limit variants still require exact deployment membership and return `403 Forbidden` plus `forbidden: operator does not have access to this deployment` when that deployment is absent from `X-Tenet-Authorized-Deployments`. Unscoped `GET /v1/audit-logs` now follows the same collection-style contract as the guarded overview/status collections under read enforcement: it requires trusted authenticated identity, filters returned `entries` down to deployments present in `X-Tenet-Authorized-Deployments`, and returns `200 OK` with `{"entries":[]}` when no visible audit rows remain.

Tenet projects that authenticated identity into the persisted actor fields for immutable plan/apply records and therefore into their projected audit entries.

Only enable that trust when a front door authenticates the caller and strips any client-supplied `X-Tenet-Authenticated-*`, `X-Tenet-Authenticated-Roles`, `X-Tenet-Authorized-Deployments`, `X-Tenet-Authorized-Projects`, `X-Tenet-Authorized-Environments`, and trusted-alias `X-User-*` headers before forwarding trusted values. A directly exposed header-trusting API instance is not a safe deployment shape.
