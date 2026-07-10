# Operator Troubleshooting

Compact operator troubleshooting quick-reference for Tenet drill-downs, refresh
handoffs, audit, plan/apply records, recent logs, and live inspection.

Use this page for route/evidence/state/verification checks. Start with identity
and access evidence before interpreting empty, unavailable, or stale downstream
signals.

## Identity and Refresh Preflight

| Check | Route | Evidence to capture | Expected local/no-AWS state | Verify |
| --- | --- | --- | --- | --- |
| Operator identity and grants | `GET /v1/operator/context` | `identity.authenticated`, `identityProvenance`, `roles.canRead`, `roles.canWrite`, `deploymentAccess.summary`, `accessVerdicts`, `desiredStateSource` | Local unrestricted runs may show relaxed enforcement; guarded runs should explain `401` missing identity and `403` missing role/deployment scope before any route-specific debugging. | `curl -i "$TENET_API_BASE/v1/operator/context"` |
| Managed-login boundary | `GET /v1/operator/context` | `authRuntime.tenetManagedLogin`, login/callback/logout routes, cookie diagnostics | Managed login authenticates browser identity; it does not grant Tenet roles or deployment access. Use `docs/features/managed-login.md` when active login changes `401` recovery. | `curl -i "$TENET_API_BASE/v1/operator/context"` |
| Manual controller refresh | `POST /v1/controller/refresh` | HTTP status, `Request-Id`, returned `controllerRefresh`/receipt evidence, deployment-scoped `Last refresh requested` cue | Filesystem catalogs return `status=filesystem_noop`; GitHub+Redis handoff can return `202 Accepted`; `401`/`403` still mean identity/write-grant debugging first. | `curl -i -X POST "$TENET_API_BASE/v1/controller/refresh"` |
| Refresh audit/status follow-up | `GET /v1/audit-logs?action=controller.refresh.requested`; `GET /v1/controller/refresh/status` | actor, safe route/status/result metadata, request ID, delivery ID/channel or filesystem no-op evidence, `latestReceipt` | `status=no_history` means the configured audit store has no refresh receipts yet; `status=unavailable` means fix audit read-model configuration before retrying. | `curl -i "$TENET_API_BASE/v1/controller/refresh/status"` |

Deployment-scoped refresh audit handoff: `/?page=audit&deployment={deploymentName}&action=controller.refresh.requested`.

## Surface Matrix

| Surface | UI route | Backing route(s) | Request-Id / lookup-context evidence | Expected local/no-AWS state | Verify |
| --- | --- | --- | --- | --- | --- |
| Applications overview | `/` | `GET /v1/overview/applications` | `Request-Id` / `Tenet request ID` on failed guarded reads; Operator context shows identity, role, and deployment-filter evidence. | Empty `200 OK` can be valid when catalog rows are missing or deployment filters/grants hide every app; guarded `401`/`403` should point back to Operator context. | `curl -i "$TENET_API_BASE/v1/overview/applications"` |
| Application detail | `/?page=application&deployment={deploymentName}` | `GET /v1/overview/applications/{deploymentName}` | `Request-Id` on initial or refresh read failure; application page keeps deployment, source fingerprint, runtime summary, and events context visible. | Missing retained events can be a bounded controller-history window, not an audit loss; use audit for durable plan/apply/refresh evidence. | `curl -i "$TENET_API_BASE/v1/overview/applications/{deploymentName}"` |
| Resource detail | `/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}` | `GET /v1/overview/deployments/{deploymentName}` | `Request-Id` on top-level deployment-detail failures; selected resource context shows the current resource read before lazy panels load. | Resource aliases can resolve to canonical IDs; local fixture detail should still render selected resource context even when live evidence is unavailable. | `curl -i "$TENET_API_BASE/v1/overview/deployments/{deploymentName}"` |
| Recent logs | `#resource-recent-logs` | `GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}` | Capture `Request-Id` / `Tenet request ID`, `lookupContext.region`, `lookupContext.targetAccountId`, `lookupContext.crossAccount`, `lookupContext.observerRoleConfigured`, selectors, and `resultScope`. | `available=true` with `entries=[]` plus `emptyReason` is a successful empty read; `available=false` / `unavailableReason=aws_unavailable` means provider lookup failed; `available=false` / `unavailableReason=no_log_source` is expected for route-stable resources without an independent stream. | `curl -i "$TENET_API_BASE/v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}&limit=5"` |
| Live inspection | `#resource-live-inspection` | `GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}` | Capture `Request-Id` / `Tenet request ID`, `lookupContext`, accepted selectors such as `clusterArn` or `instanceId`, and `resultScope` when bounded. | Summary-only traffic-routing evidence can be valid locally; optional EC2/ECS/AWS facts may be absent; `aws_unavailable`, `cross_account_not_configured`, and `resource_not_found` route operators to provider/account wiring instead of bookmark changes. | `curl -i "$TENET_API_BASE/v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}"` |
| Audit | `/?page=audit[&deployment={deploymentName}][&action={action}][&limit={n}]` | `GET /v1/audit-logs` | `Request-Id`, `resultScope`, actor availability, source record/detail routes, `entryId`, and action/deployment filters. | Empty scoped audit is valid when no matching immutable records exist; `action` supports `plan.created`, `apply.requested`, `apply.transitioned`, and `controller.refresh.requested`. | `curl -i "$TENET_API_BASE/v1/audit-logs?deployment={deploymentName}&action=apply.requested&limit=5"` |
| Plan/apply | `/?page=plan&deployment={deploymentName}[&planId={planId}][&applyId={applyId}]` | `POST /v1/plans/deployments/{deploymentName}`; `GET /v1/plans/{planId}`; `POST /v1/applies/plans/{planId}`; `GET /v1/applies/{applyId}`; `POST /v1/applies/{applyId}/transitions` | `Request-Id`, persisted actor, `planId`, `applyId`, source fingerprint/`ifMatch`, operation IDs, lifecycle status/progress, and audit source routes. | Local writes persist immutable Tenet records but do not mutate AWS; stale reviewed plans return `409 Conflict`; guarded writes require identity, operator/admin role, and matching deployment or project/environment scope. | `curl -i -X POST "$TENET_API_BASE/v1/plans/deployments/{deploymentName}"` |
| Apply collections | `/?page=applies` for the queued monitor; `/?page=applies&appliesView=active` for active handoffs; `/?page=applies&appliesView=stale` for stale active claims | `GET /v1/applies?status=active`; `GET /v1/applies?status=active&claim=stale`; `GET /v1/applies?status=queued` | `Request-Id`, returned immutable records, deployment names, `status=queued` or `status=running`, queued/running operations, lifecycle fields, claim `lastHeartbeatAt` / `expiresAt` when present, and related audit `apply.requested` / `apply.transitioned` rows. | Default local/no-AWS runs may return `[]`; with `TENET_AWS_ENABLE_APPLY=true`, queued-on-create records exercise the local-safe executor handoff without live AWS mutation. The active API returns queued plus running records newest-first; the stale active API narrows to running records with stale or expired claim leases; the embedded monitor exposes queued/active/stale view controls, keeps all reads read-only, and links rows back to `/?page=plan&deployment={deploymentName}&applyId={applyId}` for review. | `curl -i "$TENET_API_BASE/v1/applies?status=active&claim=stale"` |

Local-safe apply executor release guard: reviewers should verify the
active collection (`GET /v1/applies?status=active`), stale active-claim collection
(`GET /v1/applies?status=active&claim=stale`), queued exact subset
(`GET /v1/applies?status=queued`), stored apply read (`GET /v1/applies/{applyId}`), claim (`POST /v1/applies/{applyId}/claim`),
heartbeat (`POST /v1/applies/{applyId}/heartbeat`), and transition
(`POST /v1/applies/{applyId}/transitions`) routes together. They persist and
advance Tenet apply metadata only; this local-safe record lifecycle does not
perform live AWS mutation. The read-only `/?page=applies` monitor defaults to
the queued subset read, adds explicit queued/active/stale view controls (`?appliesView=active`
for the active collection, `?appliesView=stale` for stale active claims), preserves the explicit `Refresh applies` action and
bookmarkable URL, translates claim heartbeat/lease metadata into fresh/stale/pending
operator copy, links operators back to the plan/apply workspace for detail review, and keeps an in-shell Applications overview escape link on populated monitor views.

## Unavailable-State Decoder

| State | Meaning | Next check |
| --- | --- | --- |
| `401` | Tenet did not receive trusted identity for a guarded route. | Reopen the same bookmark through the trusted front door or active managed-login route, then retry the same URL. |
| `403` | Identity was recognized, but role or deployment/project/environment scope is missing. | Inspect `GET /v1/operator/context` `roles`, `deploymentAccess.summary`, and `accessVerdicts`; do not treat this as sign-in failure. |
| `aws_unavailable` | The route and resource were valid, but Tenet could not complete the provider lookup. | Keep the bookmark; capture `Request-Id` plus `lookupContext`, then check credentials, observer-role reachability, permissions, region/account placement, and AWS API availability. |
| `no_log_source` | The selected resource is valid, but Tenet has no independent recent-log/live source for that exact resource yet. | Use application events, sibling resources, rollout summary, audit rows, or backing-resource handoffs. |
| `source_mismatch` | A webhook or refresh trigger did not match configured desired-state source. | Compare Operator context GitHub repo/ref/path/base URL with the observed webhook or refresh source. |
| `no retained controller history` | The bounded controller-history window is empty for that deployment. | Use `/?page=audit` for durable plan/apply/refresh evidence and wait for fresh controller runtime events before expecting application-event rows. |

## Verification Commands

Use the narrowest command that proves the slice you changed:

Fast-to-broad ladder: `go test ./internal/api -run TestOperatorTroubleshootingRunbookDocsContract -count=1`,
`make test`, `make smoke-local`, then `make smoke-docker`.

```bash
go test ./internal/api -run TestOperatorTroubleshootingRunbookDocsContract -count=1
make test
make smoke-local
make smoke-docker
```

For docs-only changes, the focused Go docs contract is the fast guard. Run the
smoke lanes when the text changes release-guard expectations, local/no-AWS
matrices, or route contracts that smoke scripts assert.
