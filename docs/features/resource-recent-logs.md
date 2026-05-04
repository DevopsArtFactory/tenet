# Resource Recent Logs

Tenet now exposes a read-only recent-log slice for resource-detail drill-down pages.

## Route

`GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}]`

`limit` is optional. When present, Tenet threads the positive cap into the resource-log provider request and still applies router-level truncation to `entries[]` as a safety fallback while preserving the provider's order; invalid, zero, or negative values return `400 Bad Request` with `invalid limit query`. The built-in CloudWatch-backed `ecsService` provider passes that cap to `GetLogEvents` for each resolved task/container stream, the `ecsTask` provider uses it for the exact task stream(s) it resolves, and the CodeDeploy history provider uses it while listing deployment IDs; providers without a meaningful native cap can return their normal bounded result and rely on the router fallback. Both local and compose smoke coverage lock a representative `ecsService` bounded read (`limit=1`), an ECS task/container read (`resourceType=ecsTask&containerName=worker&limit=1`), plus the invalid `limit=0` rejection so the operator-facing contract does not drift. That release guard is intentionally shape-focused: it verifies the `resultScope` fields and conservative status vocabulary without requiring live AWS totals or a specific number of returned log lines.

`containerName` is optional and only meaningful for `resourceType=ecsTask`. When present, Tenet narrows the task lookup to one awslogs-backed container inside the selected ECS task and echoes the trimmed selector as response-level `containerName`, including explicit unavailable payloads. Omit it to read all awslogs-backed containers Tenet can resolve for that exact task. Successful ECS task log rows may also carry per-entry `entries[].containerName`; that nested field is the resolved log source for that individual row, while the response-level `containerName` remains only the requested selector and is omitted for task-wide reads.

Every successful route response also includes additive `resultScope` metadata. This is bounded-read evidence for operators and UI clients, not a pagination cursor or full log-total guarantee: Tenet reports the requested cap when one was provided, the effective cap it applied after defaulting, the number of entries returned in this response, and a conservative `truncationStatus` (`truncated`, `not_truncated`, `unknown`, or `not_applicable`). Tenet does not issue extra AWS calls just to prove total rows beyond the bounded read.

Example limited service request:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=5'
```

Example ECS task/container-scoped request:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&containerName=worker&limit=1'
```

The `resourceName` for `ecsTask` should be the exact task ARN or task identifier Tenet surfaced from the ECS service/task drill-down; URL-encode full ARNs when calling the route from a shell.

Response fields:
- `deploymentName`
- `resourceType`
- `resourceName`
- optional `resourceDescription` when Tenet resolved a rollout execution description for the selected resource
- optional `backingResourceType`
- optional `backingResourceName`
- optional `containerName` when an ECS task recent-log request supplied a container selector
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
  - optional `containerName` for ECS task log entries when Tenet resolved the source container for that row
- `resultScope`
  - optional `requestedLimit`
  - `effectiveLimit`
  - `returnedEntryCount`
  - `truncationStatus` (`truncated`, `not_truncated`, `unknown`, or `not_applicable`)
- optional `unavailableReason`

## Operator troubleshooting examples

Use the three response shapes below when deciding whether an empty log card is expected or needs AWS follow-up:

- Bounded successful read: `available=true`, `entries[]` may still be empty, and `resultScope.requestedLimit=5` / `effectiveLimit=5` / `returnedEntryCount={len(entries)}` / `truncationStatus=not_truncated|unknown|truncated` tells the operator which capped envelope Tenet returned. Treat `unknown` as "Tenet reached the cap and intentionally did not ask the provider for a total," not as an API failure.
- Supported resource, AWS lookup unavailable: `available=false`, `unavailableReason=aws_unavailable` (or another explicit provider reason) plus populated `lookupContext.region`, `lookupContext.targetAccountId`, `lookupContext.crossAccount`, and `lookupContext.observerRoleConfigured` means the route was valid but the API process could not complete the live lookup. Operators should check credentials, observer-role reachability, region/account placement, and CloudWatch/ECS/EC2 access rather than changing the bookmarked resource route. The embedded UI maps known reasons to friendly copy and uses generic troubleshooting guidance for unknown provider/internal codes so the operator page does not expose raw machine-code strings.
- Route-stable resource with no independent stream: `available=false`, `unavailableReason=no_log_source`, `entries=[]`, and `resultScope.truncationStatus=not_applicable` means the resource is a valid drill-down target but Tenet does not have a separate log stream for it yet. This is expected today for weighted-target-group traffic-routing resources such as `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup`.

## Supported resource types

Current provider-backed resource types:
- `ecsService`
- `ecsTask`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource`
- `codeDeployApplication`
- `codeDeployDeploymentGroup`
- `productionListener`
- `testListener`
- `blueTaskSetTargetGroup`
- `greenTaskSetTargetGroup`

Current route-stable rollout resources with explicit no-log-source responses:
- `trafficShiftPlan`
- `productionListenerRule`
- `activeTargetGroup`
- `candidateTargetGroup`

Behavior by type:
- `ecsService` reads recent CloudWatch log events from the newest running and recently stopped service task/container streams when the ECS task definition and awslogs configuration can be resolved. Recently stopped tasks are included in the same bounded newest-first result so failed rollouts and crash loops can still surface useful logs when running tasks are absent or older.
- `ecsTask` reads recent CloudWatch log events for the exact ECS task ARN or task identifier selected from an ECS service/task drill-down. When `containerName` is present, Tenet narrows the read to that one awslogs-backed container; when it is omitted, Tenet reads the awslogs-backed containers it can resolve for the task and returns the bounded newest-first result. For successful ECS task reads, `entries[].containerName` is additive per-row source metadata that lets operators and UI consumers distinguish merged multi-container output without parsing CloudWatch stream names. Local and smoke environments may still return explicit unavailable states such as `aws_unavailable`, `resource_not_found`, `cross_account_not_configured`, or `no_log_source` when credentials, task visibility, account/role wiring, or awslogs configuration are unavailable.
- `candidateAutoScalingGroup` and `serviceAutoScalingGroup` read recent EC2 instance console/bootstrap output from the newest instance in the Auto Scaling Group.
- `userDataBootstrapSource` reuses the backing Auto Scaling Group Tenet already resolved for bootstrap-source inspection and returns the newest EC2 instance console/bootstrap output for that backing group.
- `codeDeployApplication` reads the newest deployment records across the selected CodeDeploy application without narrowing to one deployment group, returns them newest-first, and projects each deployment into one operator-facing recent-log row with the deployment ID in `entries[].stream`.
- `codeDeployDeploymentGroup` reads the newest deployment records for the selected CodeDeploy deployment group, returns them newest-first, and projects each deployment into one operator-facing recent-log row with the deployment ID in `entries[].stream`.
- `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` now reuse that same CodeDeploy deployment-group history through additive `backingResourceType=codeDeployDeploymentGroup` and `backingResourceName={deployment}-codedeploy-group`, so the resource page can show rollout history for those drill-down siblings without inventing a second event route.
- weighted-target-group traffic-routing resources (`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup`) are first-class route resources even though they do not have an independent recent-log stream yet; the recent-log route returns `available=false` with `unavailableReason=no_log_source` instead of treating them as unsupported, while the lookup context stays present for operator troubleshooting.
- when a bootstrap-source or CodeDeploy sibling lookup resolves through a backing resource, the response also keeps the original `resourceType`/`resourceName` and adds the resolved `backingResourceType` plus `backingResourceName` so the payload stays self-describing without changing the route shape. When Tenet resolved a rollout execution description for the selected resource, the response also echoes it as `resourceDescription` on both available and unavailable payloads so operators can understand the selected resource without rejoining deployment detail.

Inside one Tenet API process, the built-in AWS-backed provider now reuses SDK config/client setup per `(region, targetAccountID, observerRoleARN)` key. That keeps repeated operator refreshes on the same deployment/resource from rebuilding ECS, CloudWatch Logs, Auto Scaling, and EC2 clients for every request.

Unsupported resource types do not hard-fail the API. They return `available=false` with a stable `unavailableReason` code so the embedded UI can show an explicit unavailable state.

When a bootstrap-source resource has no resolved backing Auto Scaling Group yet, the route stays read-only and returns `available=false` with `unavailableReason=no_log_source`. When AWS lookup fails for that backing group, the route preserves the existing explicit unavailable path such as `aws_unavailable`. When Tenet did resolve a backing resource before the downstream lookup failed, the same optional `backingResourceType` and `backingResourceName` fields remain present so operators can see which backing resource Tenet tried to inspect.

The additive `lookupContext` object is always present, including unavailable responses, so operators can see which region/account path Tenet tried to inspect before AWS troubleshooting continues elsewhere. `resultScope` is also present on the route response: unavailable/no-log-source responses use `returnedEntryCount=0` and `truncationStatus=not_applicable`, while available responses use `truncated` only when Tenet's router actually trimmed provider output, `not_truncated` when the bounded response is below the effective cap, and `unknown` when the response exactly meets the cap and Tenet deliberately avoids extra provider calls to prove whether more rows exist.

## Embedded UI behavior

The resource-detail page at `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` now renders a `Recent logs` section and lazy-loads this route after the deployment detail payload resolves. That section also renders an explicit `Refresh recent logs` button so operators can re-run only this route in place without changing the bookmarkable URL. When the payload reports `backingResourceType` plus `backingResourceName` and the loaded deployment detail can safely resolve that backing resource, the same section renders a `_self` `Resolved backing resource` handoff to the backing resource drill-down instead of forcing operators to copy IDs into a new URL. The section also renders `resultScope` as `Requested cap`, `Effective cap`, `Returned entries`, and `Scope status` so operators can distinguish capped, below-cap, at-cap/unknown-total, and unavailable/not-applicable responses without inferring exact provider totals from AWS. Current backend status values are `truncated`, `not_truncated`, `unknown`, and `not_applicable`; the UI also recognizes the compatibility aliases `capped` and `at_cap_unknown_total` for the same conservative copy.

When the payload arrives, the card first shows a compact `Lookup path · ...` sentence that summarizes the region, control/target account path, same-account versus cross-account mode, and observer-role status at a glance. It then surfaces the backend `lookupContext` directly with operator-friendly labels:
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
- unavailable: `Recent logs are unavailable for this resource.` plus the lookup-context fields above and friendly UI copy mapped from known provider `unavailableReason` codes such as `unsupported_resource_type`, `cross_account_not_configured`, `no_log_source`, `no_running_tasks`, `no_instances`, `resource_not_found`, and `aws_unavailable`. For route-stable rollout resources that intentionally have no independent stream yet, `no_log_source` renders as `This resource is a valid drill-down target, but Tenet has no independent recent-log stream for it yet.` so operators do not confuse a valid drill-down page with an unsupported resource family.
- empty: `No recent logs are available for this resource.` plus the same lookup-context fields when the backend returns `available=true` with `entries=[]`
- populated: source metadata, the same lookup-context fields, and per-entry timestamp, optional stream, and message text

## Current limitations

This slice is intentionally read-only and refresh-based:
- no streaming or tailing transport yet
- cross-account reads depend on the API process having usable AWS credentials and optional assume-role access
- local fixture/smoke environments usually show the section itself but may return unavailable data because no live AWS log source is configured, or an empty `entries[]` state when a supported resource simply has no recent lines
- the route describes recent operator-facing logs only; it is not a durable audit/event stream
- manual refresh still preserves the shared in-process dedupe path for concurrent consumers, but it bypasses stale cached ready/error UI state so operators can force a new read from the backend route
