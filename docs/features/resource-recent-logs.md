# Resource Recent Logs

Tenet now exposes a read-only recent-log slice for resource-detail drill-down pages.

## Route

`GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}`

Response fields:
- `deploymentName`
- `resourceType`
- `resourceName`
- `lookupContext`
  - `region`
  - `controlAccountId`
  - `targetAccountId`
  - `crossAccount`
  - `observerRoleConfigured`
- `available`
- `source`
- `entries[]`
  - `timestamp`
  - `message`
  - optional `stream`
- optional `unavailableReason`

## Supported resource types

Current provider-backed resource types:
- `ecsService`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource`

Behavior by type:
- `ecsService` reads recent CloudWatch log events from the newest running task/container streams when the ECS task definition and awslogs configuration can be resolved.
- `candidateAutoScalingGroup` and `serviceAutoScalingGroup` read recent EC2 instance console/bootstrap output from the newest instance in the Auto Scaling Group.
- `userDataBootstrapSource` reuses the backing Auto Scaling Group Tenet already resolved for bootstrap-source inspection and returns the newest EC2 instance console/bootstrap output for that backing group.

Inside one Tenet API process, the built-in AWS-backed provider now reuses SDK config/client setup per `(region, targetAccountID, observerRoleARN)` key. That keeps repeated operator refreshes on the same deployment/resource from rebuilding ECS, CloudWatch Logs, Auto Scaling, and EC2 clients for every request.

Unsupported resource types do not hard-fail the API. They return `available=false` with a stable `unavailableReason` code so the embedded UI can show an explicit unavailable state.

When a bootstrap-source resource has no resolved backing Auto Scaling Group yet, the route stays read-only and returns `available=false` with `unavailableReason=no_log_source`. When AWS lookup fails for that backing group, the route preserves the existing explicit unavailable path such as `aws_unavailable`.

The additive `lookupContext` object is always present, including unavailable responses, so operators can see which region/account path Tenet tried to inspect before AWS troubleshooting continues elsewhere.

## Embedded UI behavior

The resource-detail page at `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` now renders a `Recent logs` section and lazy-loads this route after the deployment detail payload resolves. That section also renders an explicit `Refresh recent logs` button so operators can re-run only this route in place without changing the bookmarkable URL.

When the payload arrives, the card surfaces the backend `lookupContext` directly with operator-friendly labels:
- `Lookup region`
- `Control account ID`
- `Target account ID`
- `Cross-account lookup`
- `Observer role configured`

The UI keeps the logs slice narrowly scoped to the selected resource and uses safe DOM construction only:
- `document.createElement`
- `textContent`
- `replaceChildren`
- no `innerHTML` for log messages
- the refresh button disables only while the recent-log fetch is in flight; it does not block the neighboring `Live inspection` panel

Rendered states:
- loading: `Loading recent logs…`
- unavailable: `Recent logs are unavailable for this resource.` plus the lookup-context fields above and friendly UI copy mapped from known provider `unavailableReason` codes such as `unsupported_resource_type`, `cross_account_not_configured`, `no_log_source`, `no_running_tasks`, `no_instances`, `resource_not_found`, and `aws_unavailable`
- empty: `No recent logs are available for this resource.` plus the same lookup-context fields when the backend returns `available=true` with `entries=[]`
- populated: source metadata, the same lookup-context fields, and per-entry timestamp, optional stream, and message text

## Current limitations

This slice is intentionally read-only and refresh-based:
- no streaming or tailing transport yet
- cross-account reads depend on the API process having usable AWS credentials and optional assume-role access
- local fixture/smoke environments usually show the section itself but may return unavailable data because no live AWS log source is configured, or an empty `entries[]` state when a supported resource simply has no recent lines
- the route describes recent operator-facing logs only; it is not a durable audit/event stream
- manual refresh still preserves the shared in-process dedupe path for concurrent consumers, but it bypasses stale cached ready/error UI state so operators can force a new read from the backend route
