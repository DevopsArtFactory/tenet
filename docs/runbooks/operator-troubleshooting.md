# Operator Troubleshooting

Daily auth, refresh, audit, and drill-down runbook for Tenet operators.

Use this when an operator cannot see an application, needs to force or trace a
desired-state refresh, or needs to explain missing resource evidence. Keep the
flow in order: identity and access issues can make later refresh, audit, and
drill-down signals look stale or empty.

## 1. Confirm identity, roles, and deployment access

Start from the embedded Operator context banner or call:

```bash
curl -i "$TENET_API_BASE/v1/operator/context"
```

Confirm `GET /v1/operator/context` shows the expected authenticated identity,
trusted header provenance or managed-session runtime, read/write role capability,
and deployment/project/environment grants for the deployment being inspected.

For `401`, reopen the same route through the trusted front door or active
managed-login/enterprise-SSO sign-in route, then retry the original bookmark.
For `403`, stay on the blocked route and inspect Operator context details for
recognized roles, `roles.canRead` / write capability, deployment access summary,
unmatched forwarded grants, and access verdict route examples. Do not treat a
403 as a sign-in problem after identity is already recognized.

## 2. Confirm desired-state source and controller-refresh availability

In Operator context details, inspect `desiredStateSource` before requesting a
refresh. Filesystem catalogs are expected to report a no-op controller-refresh
path. GitHub desired-state sources need the configured GitHub source plus the
controller refresh handoff to be available before a manual refresh can enqueue
work.

The embedded UI exposes `Refresh controller` only when the handoff is available.
Backend callers can use:

```bash
curl -i -X POST "$TENET_API_BASE/v1/controller/refresh"
```

Treat `POST /v1/controller/refresh` auth failures with the same guidance as
other operator writes: `401` means identity/session recovery first, while `403`
means the recognized caller lacks a write-capable role or deployment scope.

## 3. Request or trace a refresh through audit

After a manual refresh request, stay on the same bookmarked page and use the
success/no-op/unavailable banner as the immediate handoff result. Then open the
bookmarkable audit timeline (`/?page=audit`):

```text
/?page=audit
```

Filter for `controller.refresh.requested` and the deployment when applicable.
The matching audit rows should carry actor metadata plus safe request, route,
status, result, request ID, delivery ID, channel, or filesystem no-op evidence.
If no row appears after an accepted request, check the controller-refresh status
route before assuming desired state changed:

```bash
curl -i "$TENET_API_BASE/v1/controller/refresh/status"
```

Use `latestReceipt` when the status is `published`, treat `no_history` as an
empty configured audit store rather than a failed refresh, and treat
`unavailable` as an audit read-model/configuration problem to fix before
retrying refresh requests.

## 4. Drill into application, resource evidence, events, live inspection, and logs

Use the application page first (`/?page=application`):

```text
/?page=application&deployment={deploymentName}
```

Confirm the application detail loaded the expected deployment, source
fingerprint, runtime summary, managed drift rows, and Controller runtime events.
If the events panel is empty, distinguish an actually empty retained window from
a source filter, query limit, or controller retention cap before escalating.

Then open the specific resource (`/?page=resource`):

```text
/?page=resource&deployment={deploymentName}&resourceType={type}&resourceId={stableResourceId}
```

On `/?page=resource`, keep the selected resource context visible while checking
the lazy Recent logs and Live inspection panels. Capture the HTTP `Request-Id`
shown as `Tenet request ID` with the provider lookup context whenever evidence is
empty, unavailable, or denied. Use the panel refresh controls to rerun only the
needed recent-log or live-inspection read without changing the bookmark.

## 5. Decode common unavailable states

Use these states as routing clues, not as generic failures:

- `aws_unavailable` means Tenet reached a supported route but could not complete
  the provider lookup. Check credentials, observer-role reachability,
  permissions, region/account placement, AWS API availability, and the captured
  `Tenet request ID` plus `lookupContext`.
- `no_log_source` means the selected resource is a valid drill-down target, but
  Tenet has no independent recent-log stream or live provider read for that
  exact resource yet. Use application events, sibling resources, rollout summary,
  audit rows, or backing-resource handoffs instead of changing the route.
- `source_mismatch` means the incoming webhook or refresh trigger did not match
  the configured desired-state source. Compare the configured repo/ref/path/base
  URL in Operator context with the webhook or refresh source that was observed.
- `no retained controller history` means the immutable audit log may still have
  plan/apply/refresh rows, but the deployment's controller-history timeline has
  no retained events in its bounded window. Use `/?page=audit` for durable audit
  evidence and wait for the controller to write fresh runtime events before
  expecting application-event rows.

If several states appear together, resolve auth/access first, then desired-state
source or refresh handoff, then resource-provider availability. That order keeps
operator action focused on the layer that can actually change the next signal.
