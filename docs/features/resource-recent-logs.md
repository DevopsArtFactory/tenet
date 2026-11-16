# Resource Recent Logs

Tenet now exposes a read-only recent-log slice for resource-detail drill-down pages.

Operator troubleshooting quick-reference: use `docs/runbooks/operator-troubleshooting.md`
for the route/evidence/state/verification matrix when comparing Recent logs with
resource detail, Live inspection, audit, plan/apply, and queued apply evidence.

## Route

`GET /v1/overview/deployments/{deploymentName}/resource-logs?resourceType={type}&resourceName={name}[&limit={positiveInteger}][&containerName={container}][&instanceId={ec2InstanceId}][&clusterArn={ecsClusterArn}]`

`clusterArn` is optional for ECS reads. For `ecsService`, it selects a cluster;
when omitted, the existing service discovery behavior applies. Malformed cluster
ARNs return `400 Bad Request` before provider lookup.

For `ecsTask`, the deployment must declare trusted `ecs.clusterArn` and
`ecs.serviceArn` values. Use the full task ARN containing the cluster name;
bare task IDs and legacy short ARNs are rejected. Tenet checks the ARN's account,
region and cluster, then verifies the task's live `group=service:<service name>`
with ECS `DescribeTasks` before reading details or logs. A supplied `clusterArn`
must match the configured cluster; omitting it uses that configured cluster.
Missing scope, mismatched selectors, standalone tasks and tasks from another
service return `404`, without revealing task details. AWS verification failures
also stop the read. Both running and stopped service tasks can pass this check.
Legacy deployments without explicit ECS service intent must configure it before
using task drill-downs.

`limit` is optional. When present, Tenet threads the positive cap into the resource-log provider request and still applies router-level truncation to `entries[]` as a safety fallback while preserving the provider's order; invalid, zero, or negative values return `400 Bad Request` with `invalid limit query`. The built-in CloudWatch-backed `ecsService` provider passes that cap to `GetLogEvents` for each resolved task/container stream, the `ecsTask` provider uses it for the exact task stream(s) it resolves, and the CodeDeploy history provider uses it while listing deployment IDs; providers without a meaningful native cap can return their normal bounded result and rely on the router fallback. Both local and compose smoke coverage lock a representative `ecsService` bounded read (`limit=1`), denial of an unverified ECS task/container/cluster read, plus the invalid `limit=0` rejection so the operator-facing contract does not drift. That release guard is intentionally shape-focused: it verifies the `resultScope` fields and conservative status vocabulary without requiring live AWS totals or a specific number of returned log lines.

`containerName` is optional and only meaningful for `resourceType=ecsTask`. When present, Tenet narrows the task lookup to one awslogs-backed container inside the selected ECS task and echoes the trimmed selector as response-level `containerName`, including explicit unavailable payloads. Omit it to read all awslogs-backed containers Tenet can resolve for that exact task. Successful ECS task log rows may also carry per-entry `entries[].containerName`; that nested field is the resolved log source for that individual row, while the response-level `containerName` remains only the requested selector and is omitted for task-wide reads. The local and compose release guards intentionally lock the response-level selector separately from per-entry/source metadata and separately from rollout `resourceDescription`, so clients can render each operator clue in the right place.

`instanceId` is optional and only meaningful for Auto Scaling Group-backed EC2 console-log reads (`candidateAutoScalingGroup`, `serviceAutoScalingGroup`, and `userDataBootstrapSource` after it resolves a backing ASG). When present, Tenet validates the selector as an EC2 instance ID (`i-...`) before provider lookups, narrows the read to that ASG member, and echoes the trimmed selector as response-level `instanceId`, including explicit unavailable payloads. If the selected instance is not currently part of the resolved ASG, the route stays successful and explicit with `available=false` / `unavailableReason=resource_not_found`; omit `instanceId` to preserve the default newest-instance console-log behavior. Successful EC2 console rows also carry additive `entries[].instanceId` metadata for the source ASG member, including default reads where no response-level selector was supplied.

Every successful route response also includes additive `resultScope` metadata. This is bounded-read evidence for operators and UI clients, not a pagination cursor or full log-total guarantee: Tenet reports the requested cap when one was provided, the effective cap it applied after defaulting as both `effectiveLimit` and compact `limit`, the number of entries returned in this response as `returned`, `returnedCount`, and compatibility `returnedEntryCount={len(entries)}`, the renderable entry count before Tenet's response cap as `total` / `totalEntryCount`, optional `newestEntryTimestamp` / `oldestEntryTimestamp` values derived from the returned rows, a conservative `truncationStatus` (`truncated`, `not_truncated`, `unknown`, or `not_applicable`), and the boolean `truncated` alias that is true only when Tenet actually trimmed provider output. Tenet does not issue extra AWS calls just to prove total rows or provider-wide time ranges beyond the bounded read.

Each returned recent-log row now carries additive `entries[].entryId`, a backend-authored stable row identifier derived from the deployment/resource route, provider source, normalized row metadata, and a duplicate-row ordinal. It is intentionally opaque (`rlog_...`), requires no persistence layer, and lets operators/reviewers hand off one specific row without depending on mutable UI formatting around timestamps, messages, streams, containers, instance IDs, or optional CloudWatch log-group evidence. The embedded resource page uses that backend identifier for `#resource-log-...` row anchors, `Share log entry` links, and adjacent `Copy log entry link` buttons when present, while retaining its deterministic timestamp/message-derived fallback for older payloads that omit `entryId`. Empty and unavailable payloads keep the existing `entries=[]` semantics and therefore have no row IDs to render.

Example limited service request:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&limit=5'
```

Example selected-cluster service request when duplicate service names exist across ECS clusters:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5'
```

The selected-cluster form is a backend route contract, and local/compose smoke now lock the response-level `clusterArn` echo on both available and explicit unavailable ECS service recent-log payloads. The embedded resource page also preserves bookmarkable `clusterArn` URL state for ECS service and ECS task resources so saved drill-down links, section jumps, and lazy-panel refreshes keep the same selected cluster scope. On ECS task pages, the UI carries that selected-cluster state alongside `containerName` for the task-specific route shape. When operators need to compare against a broader read, the resource toolbar exposes **Clear selected container** to remove only `containerName` while preserving task/cluster identity, and **Clear selected cluster** to remove only `clusterArn` while preserving the resource identity, task container, filters, limits, and current section hash.

Example ECS task/container-scoped request:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-logs?resourceType=ecsTask&resourceName=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Atask%2Fworker%2Fabc123&containerName=worker&limit=1'
```

The `resourceName` for `ecsTask` should be the exact task ARN containing the configured cluster name that Tenet surfaced from the ECS service/task drill-down; URL-encode full ARNs when calling the route from a shell.

Response fields:
- `deploymentName`
- `resourceType`
- `resourceName`
- optional `resourceDescription` when Tenet resolved a rollout execution description for the selected resource
- optional `backingResourceType`
- optional `backingResourceName`
- optional `containerName` when an ECS task recent-log request supplied a container selector
- optional `instanceId` when an Auto Scaling Group-backed EC2 console-log request supplied an instance selector
- optional `clusterArn` when an ECS service/task recent-log request supplied an accepted cluster selector
- `lookupContext`
  - `region`
  - `controlAccountId`
  - `targetAccountId`
  - `crossAccount`
  - `observerRoleConfigured`
- `available`
- `source` (stable API token; embedded UI evidence panels translate known values such as `aws_cloudwatch_logs`, `cloudwatch`, `ecs_service`, and `ec2_console` into operator-friendly labels)
- `entries[]`
  - `entryId` (`rlog_...`, backend-authored stable row identifier for row-level handoffs)
  - `timestamp`
  - `message`
  - optional `stream`
  - optional `logGroup` for CloudWatch-backed ECS rows when Tenet knows the resolved log group; older/static payloads and non-CloudWatch providers may omit it
  - optional `containerName` for ECS task log entries when Tenet resolved the source container for that row
- `resultScope`
  - optional `requestedLimit`
  - `effectiveLimit`
  - `limit`
  - `returnedEntryCount`
  - `returnedCount`
  - `returned`
  - `totalEntryCount`
  - `total`
  - optional `newestEntryTimestamp` for the newest timestamp present in this returned `entries[]` window
  - optional `oldestEntryTimestamp` for the oldest timestamp present in this returned `entries[]` window
  - `truncationStatus` (`truncated`, `not_truncated`, `unknown`, or `not_applicable`)
  - `truncated`
- optional `emptyReason` on `available=true` responses with `entries=[]` (`no_recent_events` when the provider returned no recent events/consoles in the bounded window, or `no_formatted_entries` when raw provider records were present but had no renderable log message after Tenet trimmed/formatted them)
- optional `relatedLogRoutes[]` on `unavailableReason=no_log_source` responses when Tenet can route operators to sibling resources that do have recent-log sources
  - `label`
  - `route` (route-stable `GET /v1/overview/deployments/{deploymentName}/resource-logs?...` handoff)
  - `resourceType`
  - `resourceName`
- optional `unavailableReason`

## Operator troubleshooting examples

Use the three response shapes below when deciding whether an empty log card is expected or needs AWS follow-up:

- Bounded successful read: `available=true`, `entries[]` may still be empty, and `resultScope.requestedLimit=5` / `effectiveLimit=5` / compact `limit=5` / `returned={len(entries)}` / `total={pre-cap renderable entries}` / compatibility `returnedCount` and `returnedEntryCount` / optional `newestEntryTimestamp` and `oldestEntryTimestamp` from the returned rows / `truncationStatus=not_truncated|unknown|truncated` plus `truncated={true|false}` tells the operator which capped envelope Tenet returned. When `entries=[]`, the additive `emptyReason` distinguishes `no_recent_events` from `no_formatted_entries`; treat either as a successful provider read rather than an unavailable AWS lookup. Treat `unknown` as "Tenet reached the cap and intentionally did not ask the provider for a provider-wide total," not as an API failure.
- Supported resource, AWS lookup unavailable: `available=false`, `unavailableReason=aws_unavailable` (or another explicit provider reason) plus populated `lookupContext.region`, `lookupContext.targetAccountId`, `lookupContext.crossAccount`, and `lookupContext.observerRoleConfigured` means the route was valid but the API process could not complete the live lookup. Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating; that pair gives reviewers the exact Tenet request log correlation key plus the attempted region/account/observer-role path. The embedded UI now surfaces that header as `Tenet request ID` metadata in the Recent logs card when the lazy read provides it, including successful unavailable/empty payloads and non-auth fetch errors, so operators can copy the escalation token without opening browser devtools. Operators should check credentials, observer-role reachability, region/account placement, and CloudWatch/ECS/EC2 access rather than changing the bookmarked resource route. The embedded UI maps known reasons to friendly next-step copy for missing cross-account observer-role wiring, missing ECS tasks, missing ASG instances, stale resource selections, and temporary AWS/provider access failures, and it uses generic troubleshooting guidance for unknown provider/internal codes so the operator page does not expose raw machine-code strings.
- Route-stable resource with no independent stream: `available=false`, `unavailableReason=no_log_source`, `entries=[]`, and `resultScope.truncationStatus=not_applicable` means the resource is a valid drill-down target but Tenet does not have a separate log stream for it yet. These unavailable/no-log-source responses use `returnedCount=0`, compatibility `returnedEntryCount=0`, `truncated=false`, and `truncationStatus=not_applicable`. When Tenet can derive sibling log sources from the same deployment detail, the payload also includes additive `relatedLogRoutes[]` handoffs so operators can try sibling recent-log reads without treating the selected resource as broken. For example a weighted `trafficShiftPlan` response can point at `candidateAutoScalingGroup` and `userDataBootstrapSource` recent-log routes so operators have an actionable EC2/bootstrap path without changing the selected resource route.
- Missing provider wiring: `available=false`, `entries=[]`, `unavailableReason="overview deployment resource logs unavailable"`, and the selected deployment/resource plus accepted selector echoes means the route is active but this API process was launched without the Recent logs provider wired. Tenet still validates the deployment/resource/selectors first and preserves `503 Service Unavailable`, so operators can distinguish a local wiring problem from malformed bookmarks, missing deployments, or AWS lookup failures.

For local development and no-AWS runs, keep the same distinction: an unavailable
provider lookup is still a successful route envelope with `available=false`,
`unavailableReason`, `lookupContext`, and not-applicable `resultScope`; a
successful empty log read stays `available=true`, `entries=[]`, and carries
`emptyReason` for machines while the embedded UI renders operator-friendly copy
instead of raw reason codes.

### Local/no-AWS release-guard matrix

The local and Docker smoke scripts keep a compact no-AWS matrix so route owners can update one checklist when unavailable-state behavior changes. The embedded Recent logs card should preserve the exact lazy-read request route, Request-Id metadata, lookup context, bounded-scope fields, and friendly operator copy for each state.

| Route family | Representative resource | Expected local/no-AWS evidence |
| --- | --- | --- |
| ECS service recent logs | `ecsService` with optional `clusterArn` | `lookupContext`, selector echo, bounded `resultScope`, and either successful-empty `emptyReason` or explicit `unavailableReason` |
| ECS task recent logs | `ecsTask` with `containerName` and `clusterArn` | unverified fixture returns `404`; unit tests cover authorized task envelopes and container/cluster selectors |
| ASG console logs | `candidateAutoScalingGroup` / `userDataBootstrapSource` with optional `instanceId` | selected-instance echo, backing-ASG metadata for bootstrap sources, malformed `instanceId` rejected with `400`, and stale member selections returned as `resource_not_found` |
| Traffic-routing no-log-source | `trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, `candidateTargetGroup` | `available=false`, `unavailableReason=no_log_source`, `entries=[]`, populated `lookupContext`, `resultScope.truncationStatus=not_applicable`, and `relatedLogRoutes[]` handoffs to sibling recent-log sources when Tenet can derive them |
| Successful-empty provider read | any supported log source with no returned rows | `available=true`, `entries=[]`, `emptyReason=no_recent_events` or `no_formatted_entries`, and no `unavailableReason` |

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
- `ecsService` reads recent CloudWatch log events from the newest running and recently stopped service task/container streams when the ECS task definition and awslogs configuration can be resolved. Recently stopped tasks are included in the same bounded newest-first result so failed rollouts and crash loops can still surface useful logs when running tasks are absent or older. Add `clusterArn={arn:aws:ecs:...:cluster/name}` when two clusters in the same account/region expose the same service name; Tenet trims and validates the ECS cluster ARN, uses it as the direct service lookup scope, and keeps the default all-cluster scan when it is omitted.
- `ecsTask` reads recent CloudWatch log events only after the deployment-bound membership check above succeeds. `containerName` narrows the read to one awslogs-backed container; omitting it reads all resolved awslogs-backed containers for that task. Successful rows include `entries[].containerName` to identify their source. Provider failures after authorization still use explicit unavailable states.
- `candidateAutoScalingGroup` and `serviceAutoScalingGroup` read recent EC2 instance console/bootstrap output from the newest instance in the Auto Scaling Group by default. Add `instanceId={i-...}` to select a specific current ASG member; malformed IDs fail with `400 Bad Request`, and well-formed IDs that are not part of the resolved ASG return `available=false` / `unavailableReason=resource_not_found` instead of reading arbitrary instance console output. Successful rows include `entries[].instanceId` so default ASG reads and selected-instance reads both identify which EC2 member produced each console line. In the embedded resource page, **Clear selected instance** removes only the ASG `instanceId` selector from the bookmarkable URL and refreshes Recent logs / Live inspection with the default unscoped ASG lookup while preserving the resource route, limits, filters, and current section hash. The sibling Live inspection panel can also render optional selected-host `autoScalingGroup.instances[].ec2Instance` facts from live `DescribeInstances` as `EC2 state`, `EC2 instance type`, `AMI image ID`, IP, subnet/VPC, and launch-time labels; local/no-AWS runs may omit those host facts while keeping the membership row and console-log selector useful.

Example selected-instance ASG console request:

```bash
curl 'http://localhost:8080/v1/overview/deployments/payments-api-prod/resource-logs?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890'
```

Successful selected-instance responses echo the selector so operators can tell a narrow EC2 console lookup from the default newest-instance read:

```json
{
  "deploymentName": "payments-api-prod",
  "resourceType": "candidateAutoScalingGroup",
  "resourceName": "payments-api-prod-candidate",
  "instanceId": "i-0abc123def4567890",
  "lookupContext": {
    "region": "ap-northeast-2",
    "controlAccountId": "123456789012",
    "targetAccountId": "123456789012",
    "crossAccount": false,
    "observerRoleConfigured": false
  },
  "available": true,
  "source": "ec2_console",
  "entries": [
    {
      "timestamp": "2026-04-16T07:11:30Z",
      "message": "selected instance bootstrap output",
      "instanceId": "i-0abc123def4567890"
    }
  ],
  "resultScope": {
    "effectiveLimit": 100,
    "limit": 100,
    "returnedEntryCount": 1,
    "returnedCount": 1,
    "returned": 1,
    "totalEntryCount": 1,
    "total": 1,
    "newestEntryTimestamp": "2026-04-16T07:11:30Z",
    "oldestEntryTimestamp": "2026-04-16T07:11:30Z",
    "truncationStatus": "not_truncated",
    "truncated": false
  }
}
```
- `userDataBootstrapSource` reuses the backing Auto Scaling Group Tenet already resolved for bootstrap-source inspection and returns the newest EC2 instance console/bootstrap output for that backing group by default; the same optional `instanceId={i-...}` selector can narrow the backing-ASG console read to a specific current member.
- `codeDeployApplication` reads the newest deployment records across the selected CodeDeploy application without narrowing to one deployment group, returns them newest-first, and projects each deployment into one operator-facing recent-log row with the deployment ID in `entries[].stream`.
- `codeDeployDeploymentGroup` reads the newest deployment records for the selected CodeDeploy deployment group, returns them newest-first, and projects each deployment into one operator-facing recent-log row with the deployment ID in `entries[].stream`.
- `productionListener`, `testListener`, `blueTaskSetTargetGroup`, and `greenTaskSetTargetGroup` now reuse that same CodeDeploy deployment-group history through additive `backingResourceType=codeDeployDeploymentGroup` and `backingResourceName={deployment}-codedeploy-group`, so the resource page can show rollout history for those drill-down siblings without inventing a second event route.
- weighted-target-group traffic-routing resources (`trafficShiftPlan`, `productionListenerRule`, `activeTargetGroup`, and `candidateTargetGroup`) are first-class route resources even though they do not have an independent recent-log stream yet; the recent-log route returns `available=false` with `unavailableReason=no_log_source` instead of treating them as unsupported, while the lookup context stays present for operator troubleshooting. When the deployment detail exposes sibling log-capable resources, `relatedLogRoutes[]` points at the candidate Auto Scaling Group and userdata bootstrap-source recent-log routes so the embedded resource-detail Recent logs panel can show `Related recent-log reads` links that pivot operators to EC2/bootstrap evidence without changing the validity of the selected traffic-routing route.
- when a bootstrap-source or CodeDeploy sibling lookup resolves through a backing resource, the response also keeps the original `resourceType`/`resourceName` and adds the resolved `backingResourceType` plus `backingResourceName` so the payload stays self-describing without changing the route shape. When Tenet resolved a rollout execution description for the selected resource, the response also echoes it as `resourceDescription` on both available and unavailable payloads so operators can understand the selected resource without rejoining deployment detail.

Inside one Tenet API process, the built-in AWS-backed provider now reuses SDK config/client setup per `(region, targetAccountID, observerRoleARN)` key. That keeps repeated operator refreshes on the same deployment/resource from rebuilding ECS, CloudWatch Logs, Auto Scaling, and EC2 clients for every request.

Unsupported resource types do not hard-fail the API. They return `available=false` with a stable `unavailableReason` code so the embedded UI can show an explicit unavailable state.

When a bootstrap-source resource has no resolved backing Auto Scaling Group yet, the route stays read-only and returns `available=false` with `unavailableReason=no_log_source`. When AWS lookup fails for that backing group, the route preserves the existing explicit unavailable path such as `aws_unavailable`. When Tenet did resolve a backing resource before the downstream lookup failed, the same optional `backingResourceType` and `backingResourceName` fields remain present so operators can see which backing resource Tenet tried to inspect.

`make smoke-local` and `make smoke-docker` now release-guard the selector and successful-empty reason contracts without requiring live EC2 console output. They fetch representative recent-log routes and assert that `available=true` / `entries=[]` payloads carry `emptyReason` (`no_recent_events` or `no_formatted_entries`), populated successful payloads omit `emptyReason`, and unavailable payloads keep using explicit `unavailableReason`. They also fetch the bootstrap-source recent-log route with `instanceId=i-0abc123def4567890`, assert the response echoes that selector plus backing-ASG metadata even when the local environment returns an explicit unavailable payload, keep the unselected bootstrap-source read covered for newest-instance default behavior and optional `entries[].instanceId` evidence, and reject malformed `instanceId` values before provider lookup.

The additive `lookupContext` object is always present, including unavailable responses, so operators can see which region/account path Tenet tried to inspect before AWS troubleshooting continues elsewhere. `resultScope` is also present on the route response: unavailable/no-log-source responses use `limit={effective cap}`, `returned=0`, `total=0`, compatibility `returnedCount=0` / `returnedEntryCount=0`, `truncated=false`, and `truncationStatus=not_applicable`, while available responses use `truncated=true` only when Tenet trimmed provider output, `not_truncated` when the bounded response is below the effective cap, and `unknown` when the response exactly meets the cap and Tenet deliberately avoids extra provider calls to prove whether more rows exist.

## Release guard matrix

Local and no-AWS release guards intentionally cover representative unavailable and successful-empty states instead of requiring live provider credentials. Keep the matrix compact and operator-facing:

- ECS service reads prove selector echoes (`clusterArn`, `containerName`), `lookupContext`, `resultScope`, and explicit unavailable reasons such as `aws_unavailable`, `no_running_tasks`, `resource_not_found`, or `no_log_source` without treating them as route failures.
- Unverified ECS task fixtures must return `404` before any detail or log read; authorization tests cover live service membership with a local fake AWS endpoint.
- Auto Scaling and bootstrap-source reads prove `instanceId` echoes, backing-resource metadata, `lookupContext`, bounded `resultScope`, malformed-selector rejection, and explicit local/no-AWS unavailable payloads.
- Traffic-routing resources prove valid route-stable `no_log_source` unavailable responses for resources without independent log streams yet.
- Successful empty responses stay `available=true` with `entries=[]` plus `emptyReason` (`no_recent_events` or `no_formatted_entries`), so the UI can show a calm empty state while scripts still assert machine-readable evidence.
- The embedded UI release guard checks friendly copy and stable machine fields together: request route, request ID, lookup context, result scope, `unavailableReason`/`emptyReason`, and safe DOM rendering must stay aligned.

## Embedded UI behavior

The resource-detail page at `/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` now renders a `Recent logs` section and lazy-loads this route after the deployment detail payload resolves. For ECS task container handoffs, the same embedded shell preserves the optional `containerName` selector in URL state (`/?page=resource&deployment={name}&resourceType=ecsTask&resourceId={id}&containerName={container}#resource-recent-logs`) so bookmarks/reloads reopen the task resource and fetch the container-scoped logs without sending operators to raw JSON in a new tab. For ASG resources, the Live inspection header's `_self` `Open EC2 console logs` handoff also preserves the selected `instanceId` plus the current `resourceLogLimit` when the operator is reviewing one EC2 member before jumping to Recent logs. That section also renders an explicit `Refresh recent logs` button so operators can re-run only this route in place, and its `Log limit` selector persists as the resource-page query param `resourceLogLimit={n}` rather than reusing the audit page's `limit` param. `Copy recent logs route` sits beside those controls and copies the exact `GET /v1/overview/deployments/{deploymentName}/resource-logs?...` lazy-read route for the currently selected resource, selector, and log cap; it is a copy-to-clipboard handoff for tickets or terminal reproduction, not a raw-JSON navigation that changes the embedded page. Bookmarkable URLs continue to preserve `deployment`, `resourceType`, `resourceId`, optional `containerName` / `instanceId`, and the `#resource-recent-logs` hash while the lazy API read still sends backend `limit={n}` only to the recent-log route. When the payload reports `backingResourceType` plus `backingResourceName` and the loaded deployment detail can safely resolve that backing resource, the same section renders a `_self` `Resolved backing resource` handoff to the backing resource drill-down instead of forcing operators to copy IDs into a new URL. The section also renders `resultScope` as `Requested cap`, `Effective cap`, `Returned entries`, and `Scope status` so operators can distinguish capped, below-cap, at-cap/unknown-total, and unavailable/not-applicable responses without inferring exact provider totals from AWS. Current backend status values are `truncated`, `not_truncated`, `unknown`, and `not_applicable`; the UI also recognizes the compatibility aliases `capped` and `at_cap_unknown_total` for the same conservative copy.

When a Recent logs or Live inspection payload includes `lookupContext`, the panel labels it as `Provider lookup context`. Recent logs shows a compact lookup-context card with a `Lookup path · ...` summary plus chips for returned region, control account, target account, cross-account, and observer-role-configured values. Live inspection keeps its compact `Inspection path · ...` sentence. The panels then surface the backend `resourceDescription` and response-level ECS task `containerName` selector when present, followed by `lookupContext` with operator-friendly labels:
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
- empty: `No recent logs are available for this resource.` plus the same lookup-context fields when the backend returns `available=true` with `entries=[]`; known `emptyReason` values render friendly details (`no_recent_events` explains that the provider had no events in the bounded window, while `no_formatted_entries` explains that provider records existed but none produced operator-visible rows) without exposing the raw machine code in the UI. When no backend `emptyReason` is present, the embedded panel still gives operator next steps: increase the Log limit, refresh after deployment activity, and use lookup context before escalating.
- populated: source metadata, rollout `Resource description` / ECS task `Selected container` metadata when present, the same lookup-context fields, and per-entry timestamp, optional stream, optional CloudWatch log group, optional container source, and message text. Each rendered log row also gets a stable `#resource-log-...` anchor plus an `_self` `Share log entry` link and `Copy log entry link` button that both preserve the current `/?page=resource&deployment=...&resourceType=...&resourceId=...` route context plus optional `containerName`, `instanceId`, `clusterArn`, and selected `resourceLogLimit`, so operators can hand off the exact row they investigated without opening a raw JSON route. Loading, unavailable, denied, and successful-empty states intentionally omit row share/copy controls. ECS task live-inspection container rows with awslogs evidence expose an `_self` `Open container recent logs` handoff to the embedded resource route with `containerName={name}` and `#resource-recent-logs`; the recent-log API request still receives the same trimmed `containerName` selector through `URLSearchParams`.

## Current limitations

This slice is intentionally read-only and refresh-based:
- no streaming or tailing transport yet
- cross-account reads depend on the API process having usable AWS credentials and optional assume-role access
- local fixture/smoke environments usually show the section itself but may return unavailable data because no live AWS log source is configured, or an empty `entries[]` state when a supported resource simply has no recent lines
- the route describes recent operator-facing logs only; it is not a durable audit/event stream
- manual refresh still preserves the shared in-process dedupe path for concurrent consumers, but it bypasses stale cached ready/error UI state so operators can force a new read from the backend route
