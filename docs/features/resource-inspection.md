# Resource Inspection

Tenet exposes a read-only live-inspection slice for resource-detail drill-down
pages. It complements Recent logs: Recent logs answers "what recent operator
events or log lines exist?", while Live inspection answers "what does the
selected AWS/control-plane resource currently look like?"

Runtime code remains read-only; this guide documents the shipped live-inspection route contract.

## Route

`GET /v1/overview/deployments/{deploymentName}/resource-inspection?resourceType={type}&resourceName={name}[&clusterArn={ecsClusterArn}][&limit={positiveInteger}][&instanceId={ec2InstanceId}]`

The route requires `resourceType` and `resourceName`. Tenet resolves the
deployment, builds the same provider lookup context used by other resource
evidence routes, and returns an explicit JSON envelope for supported, empty, and
unavailable states.

`clusterArn` is optional and only meaningful for `resourceType=ecsService` and `resourceType=ecsTask`. When present, Tenet trims and validates the selector as
an ECS cluster ARN with a 12-digit AWS account ID before provider lookup. Accepted
selectors are echoed as response-level `clusterArn` on successful and explicit
unavailable payloads, so saved evidence identifies the cluster-scoped lookup.
Omit it to preserve Tenet's default ECS cluster discovery behavior. Malformed
cluster ARNs fail with `400 Bad Request` / `invalid clusterArn query`.

`limit` is optional and applies only to bounded ECS service task samples and direct Auto Scaling Group instance samples. For `ecsService`, it caps the newest inspected
running/stopped task summary bridge. For `candidateAutoScalingGroup` and
`serviceAutoScalingGroup`, it caps current-instance samples. Invalid, zero, or
negative values return `400 Bad Request` / `invalid limit query` for resource
types that support bounded samples; resource types without a sample window,
including `userDataBootstrapSource`, ignore the query.

`instanceId` is optional and only meaningful for Auto Scaling Group-backed inspection reads, including `userDataBootstrapSource` after it resolves a backing ASG. When present, Tenet validates the selector as an EC2 instance ID
(`i-...`) before provider lookup, narrows the inspection to that ASG member, and
echoes the trimmed selector as response-level `instanceId`. If the selected
instance is well formed but is not currently part of the resolved ASG, the route
returns `available=false` / `unavailableReason=resource_not_found` instead of
falling back to another host. Omit it to preserve the bounded ASG sample
behavior on direct ASG resources, or the default bootstrap-source comparison on
`userDataBootstrapSource`.

Example selected-cluster service inspection:

```bash
curl 'http://localhost:8080/v1/overview/deployments/worker-prod/resource-inspection?resourceType=ecsService&resourceName=worker-prod-service&clusterArn=arn%3Aaws%3Aecs%3Aap-northeast-2%3A123456789012%3Acluster%2Fworker-prod&limit=5'
```

Example selected ASG instance inspection:

```bash
curl 'http://localhost:8080/v1/overview/deployments/payments-api-prod/resource-inspection?resourceType=candidateAutoScalingGroup&resourceName=payments-api-prod-candidate&instanceId=i-0abc123def4567890'
```

## Response Envelope

Like recent logs, every resource-inspection response includes `lookupContext`.
The object is present for both available and unavailable payloads:
- `lookupContext.region`
- `lookupContext.controlAccountId`
- `lookupContext.targetAccountId`
- `lookupContext.crossAccount`
- `lookupContext.observerRoleConfigured`

Common response fields:
- `deploymentName`
- `resourceType`
- `resourceName`
- optional `resourceDescription` when Tenet resolved rollout execution context
- optional `clusterArn` when an ECS service/task request supplied an accepted cluster selector
- optional `instanceId` when an ASG-backed request supplied an accepted instance selector
- optional `backingResourceType`
- optional `backingResourceName`
- `lookupContext`
- `available`
- `source` (stable API token; the embedded UI translates known values such as `aws_ecs`, `aws_cloudwatch_logs`, and `rollout_execution_summary` into operator-friendly labels)
- optional `resultScope`
- `unavailableReason` on unavailable responses

Unavailable responses keep the selected route explicit with `available=false`, `unavailableReason`, `lookupContext`, and any accepted selector echo such as `clusterArn` or `instanceId`. The API also backfills the resolved deployment/resource envelope (`deploymentName`, `resourceType`, `resourceName`, and rollout `resourceDescription` when known) after provider lookup so sparse provider results still remain self-describing evidence. This makes local/no-AWS, missing-provider, cross-account, stale-resource,
and unsupported-resource states useful evidence instead of vague provider
failures. If the API instance was started without resource-inspection provider
wiring, Tenet still validates the deployment/resource selectors first, preserves
`503 Service Unavailable`, and returns the same JSON envelope with
`unavailableReason="overview deployment resource inspection unavailable"` instead
of a plain-text body.

Known unavailable reasons include:
- `cross_account_not_configured`
- `resource_not_found`
- `aws_unavailable`
- `overview deployment resource inspection unavailable`
- `unsupported_resource_type`
- `no_log_source`

For operator triage, `cross_account_not_configured` means the target deployment needs observer-role wiring before Tenet can inspect the selected resource. `resource_not_found` means the selected AWS resource, task, instance, or rollout target was not found in the resolved lookup path. `aws_unavailable` means Tenet could not complete the provider lookup because credentials, permissions, region/account placement, or the AWS API were unavailable. `overview deployment resource inspection unavailable` means the API route is active but this process was launched without the resource-inspection provider wired. `unsupported_resource_type` means the route is stable but Tenet has not implemented live inspection for that resource family yet. `no_log_source` means the resource is valid, but inspection can only use summary or sibling evidence rather than a dedicated live provider read.

### Unavailable reason operator actions

Use the unavailable reason as a routing cue rather than treating every
`available=false` response as the same failure:

| Reason | Next operator action |
| --- | --- |
| `cross_account_not_configured` | Confirm the deployment's target account has an observer role and that the control-plane account can assume it. |
| `resource_not_found` | Re-check the selected resource name, cluster/instance selector, and rollout execution context before escalating to AWS access. |
| `aws_unavailable` | Capture `Request-Id` plus `lookupContext`, then verify credentials, region/account placement, IAM permissions, and AWS API reachability. |
| `overview deployment resource inspection unavailable` | Restart or reconfigure `tenet-api` with the resource-inspection provider wired before retrying the same bookmark. |
| `unsupported_resource_type` | Keep the bookmark as stable evidence, but use deployment detail, Recent logs, or sibling resource reads until Tenet adds a live inspector. |
| `no_log_source` | Treat the resource as route-stable and inspect summary or sibling evidence; there is no dedicated live provider read for that resource yet. |

### Local/no-AWS release-guard matrix

`make smoke-local` and `make smoke-docker` keep a representative no-AWS matrix for ECS service/task, ASG and bootstrap-source, traffic-routing, selector-validation, and guarded-auth paths. The matrix checks machine fields (`available`, `unavailableReason`, `lookupContext`, `resultScope`, `clusterArn`, `instanceId`) together with the embedded UI's friendly Live inspection copy so local operators get useful evidence even without AWS credentials.

Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating. The request ID anchors Tenet's structured
API logs, while the lookup context explains the attempted region, control
account, target account, and observer-role path. The embedded UI surfaces that
header as `Tenet request ID` metadata in the Live inspection card when the lazy
read provides it, including successful unavailable payloads and non-auth fetch
errors, so operators can copy the escalation token without opening browser
devtools.

For local development and no-AWS runs, treat unavailable live inspection as an
operator-friendly route envelope rather than a backend failure: the payload keeps
`available=false`, `unavailableReason`, `lookupContext`, optional selector echoes,
and any `resultScope` sample metadata for scripts. Missing provider wiring is a
separate release-guard row: after Tenet validates the deployment/resource
selectors, a `503 Service Unavailable` response still returns JSON with
`deploymentName`, `resourceType`, `resourceName`, accepted `clusterArn` or
`instanceId` echoes, `available=false`, and
`unavailableReason="overview deployment resource inspection unavailable"` so
operators can retry the same bookmark after wiring the provider instead of
changing selectors first. A successful empty inspection read instead stays
`available=true` with `source`, `lookupContext`, request metadata, and optional
`resultScope`, but no typed body such as `ecsService` or `autoScalingGroup`; the
embedded UI renders this as an empty details state without exposing raw
unavailable reason codes.

## Result Scope

`resultScope` is additive metadata for bounded live-inspection samples, not a pagination cursor or provider-wide total guarantee. Tenet reports the sample window
it returned without issuing extra provider calls solely to prove provider-wide
totals.

Fields:
- `resultScope.sampleType` (`ecsTasks` or `autoScalingInstances`)
- optional `resultScope.requestedLimit`
- `resultScope.effectiveLimit`
- `resultScope.returnedItemCount`
- `resultScope.returnedCount`
- `resultScope.truncationStatus`
- `resultScope.truncated`

`resultScope.returnedCount` mirrors `resultScope.returnedItemCount` for clients
that share compact count renderers across audit, logs, events, and inspection
routes. `resultScope.truncated` is true only when `resultScope.truncationStatus`
is `truncated`.

Truncation statuses are conservative: `not_truncated`, `truncated`, `unknown`, or `not_applicable`.
- `not_truncated` means Tenet has evidence the returned sample is the complete bounded result.
- `truncated` means more matching sample rows existed than Tenet returned.
- `unknown` means Tenet reached the cap without provider evidence to prove whether more rows exist.
- `not_applicable` means the optional sample bridge was unavailable or the inspected resource has no bounded sample.

For ECS service payloads, the same task sample window is also described in
`ecsService.tasksScope`. ECS service event evidence is separately described in
`ecsService.eventsScope`, an additive response-window summary with
`effectiveLimit`, `returnedEventCount`, `truncationStatus`, `truncated`, optional
`newestEventAt`, optional `oldestEventAt`, and optional `summary`. The event
scope is not a durable audit cursor; ECS service events are a rolling recent
provider list, so the timestamps identify the newest and oldest events retained
in the current inspection response. For ASG payloads, the sibling sample window
is described in `autoScalingGroup.instancesScope`. The top-level `resultScope`
exists so generic clients and scripts can summarize the bounded task/instance
read without parsing every resource-specific body first.

ECS service payloads can also include additive deployment-configuration evidence
from `DescribeServices` when AWS returns it:
- `ecsService.deploymentControllerType`
- `ecsService.deploymentCircuitBreaker.enabled`
- `ecsService.deploymentCircuitBreaker.rollback`
- `ecsService.deploymentAlarmConfiguration.enabled`
- `ecsService.deploymentAlarmConfiguration.rollback`
- `ecsService.deploymentAlarmConfiguration.alarmNames[]`

ECS service deployment rollout-state rows are exposed as
`ecsService.deployments[]` from `DescribeServices` and can include `id`,
`status`, `rolloutState`, `rolloutStateReason`, `desiredCount`,
`runningCount`, `pendingCount`, `taskDefinition`, `createdAt`, `updatedAt`,
`primary`, and `active`. Recent ECS service events remain in
`ecsService.events[]`; `ecsService.eventsScope` reports the bounded event window
included in the response.

Those fields are optional. Fixture-backed, local, no-AWS, and older provider
payloads omit them rather than emitting empty placeholders.
Smoke treats `ecsService.deployments[]` as optional, but validates row field types whenever AWS returns deployment rows.

When present, the embedded Live inspection card renders the same evidence with
the operator-facing labels `Deployment controller`, `Deployment circuit breaker`,
and `Deployment alarms`; deployment rollout rows use `Deployment ID`,
`Deployment status`, `Rollout state`, `Rollout reason`,
`Desired/running/pending`, `Task definition`, `Created at`, and `Updated at`
labels plus `Primary deployment` / `Active deployment` cues and a same-tab
`Share ECS deployment` row anchor that preserves the current resource page.
Service events use `Recent ECS service events`, `Event timestamp`, optional
`Event ID`, and `Event window scope` labels plus a same-tab
`Share ECS service event` row anchor. Event anchors prefer provider-authored
`eventId` values when present and otherwise use a deterministic timestamp/message
fallback so operators can share a specific ECS service event without losing the
current resource selector state such as `clusterArn` or `resourceInspectionLimit`.
When ECS service events are present, `ecsService.events[].eventId` is a stable row identifier for the live-inspection payload. `Share ECS service event` links to a same-page row anchor for one ECS service event, and `Copy ECS service event link` copies the same stable handoff URL without navigating away; eventId-backed anchors remain stable across refreshes while fallback anchors keep older payloads shareable.
When the fields are absent, those rows stay quiet.

## Supported Resource Types

Current resource-inspection payload families:
- `ecsService`
- `ecsTask`
- `candidateAutoScalingGroup`
- `serviceAutoScalingGroup`
- `userDataBootstrapSource`
- `codeDeployApplication`
- `codeDeployDeploymentGroup`
- `productionListener`
- `testListener`
- `productionListenerRule`
- `activeTargetGroup`
- `candidateTargetGroup`
- `blueTaskSetTargetGroup`
- `greenTaskSetTargetGroup`
- `trafficShiftPlan`

`ecsService` reads service identity, status, desired/running/pending counts,
deployment configuration, deployments, recent service events with
`ecsService.eventsScope`, task definition evidence, and a bounded task sample
when available. Task sample container rows
preserve status, health, exit/reason, optional `imageDigest`, optional
`runtimeId`, optional `awslogs`, and optional `networkBindings` evidence from
ECS `DescribeTasks`. The selected-cluster
`clusterArn` selector scopes the service lookup directly and is echoed on
successful and explicit unavailable payloads. The operator-facing ECS service
labels include `Deployment controller`, `Deployment circuit breaker`,
`Deployment alarms`, `Service deployments`, `Recent ECS tasks`, and
`Recent ECS service events` so docs, smoke evidence, and the embedded UI keep the
same review vocabulary.

`ecsTask` reads exact task identity, status, stopped reason, placement evidence,
task definition, container rows, optional `imageDigest`, optional `runtimeId`,
optional `awslogs` metadata, optional `networkBindings` evidence from ECS
`DescribeTasks`, and optional EC2 host facts for ECS-on-EC2 tasks when Tenet can
resolve and describe the backing instance.
The same `clusterArn` selector keeps task IDs unambiguous when a parent service
handoff selected a cluster. The operator-facing ECS task/container labels include
`Last/desired status`, `Health status`, `Stopped reason`, `Container {name} · status`,
`Container {name} image digest`, `Container {name} runtime ID`,
`Container {name} exit`, `Container {name} awslogs`, and
`Container {name} network binding`, which keeps status, failure, log-binding,
network, and container identity evidence discoverable without opening raw JSON.

`candidateAutoScalingGroup` and `serviceAutoScalingGroup` read ASG size, health
settings, target groups, launch-template metadata, current instances, optional
scaling activity evidence, optional EC2 host facts, and optional EC2 instance
status-check evidence under `instances[].ec2InstanceStatus`. The `limit` query
bounds the current-instance sample, and `instanceId` focuses the payload on one
current ASG member. The operator-facing ASG/EC2 labels include `Launch template`,
`Mixed instances launch template`, `Observed image ID`, `Observed instance type`,
`Instances`, `Recent scaling activities`, `EC2 state`, `EC2 instance type`,
`AMI image ID`, `EC2 instance status check`, `EC2 system status check`,
`Attached EBS status check`, and `EC2 scheduled event {n}`, plus bounded
activity labels such as `Activity ID`, `Status code`, `Progress`, `Cause`, and
`Status message`.

`userDataBootstrapSource` resolves the backing Auto Scaling Group for a bootstrap
source resource and compares the desired bootstrap source to the observed launch
template marker. Selected-instance inspection can include the matched
`userDataBootstrapSource.selectedInstance` row, nested EC2 details, and nested
EC2 instance status-check evidence under `selectedInstance.ec2InstanceStatus`
when live AWS evidence is available.

Traffic-routing resources return `trafficRoutingResource` payloads. In local or
no-AWS runs, Tenet may return `inspectionMode=summary_only` from rollout
execution metadata. `inspectionMode=summary_only` remains the local/no-AWS
fallback for rollout-derived traffic-routing resources so fixture and Compose
drill-downs stay useful without live AWS credentials. When AWS inspection
succeeds, the same route can upgrade to `inspectionMode=live` with CodeDeploy,
listener, listener-rule, target-group, latest-deployment, rollback, error, alarm,
deployment-overview, and target-health evidence. The traffic-routing inspection
labels include `Traffic-routing status`, `CodeDeploy deployment group status`,
`Target group live details`, `Registered target health`, `Target group health`,
and `Inspection mode`, covering CodeDeploy deployment-group, production/test
listener, listener-rule, task-set target-group, and weighted target-group
families without changing the route shape.

## Embedded UI Behavior

The resource-detail page at
`/?page=resource&deployment={name}&resourceType={type}&resourceId={id}` renders a
`Live inspection` panel and lazy-loads the resource-inspection route for the
selected resource. The bookmarkable UI route stays unchanged while only the lazy
panel refreshes.

The panel labels the provider context as `Provider lookup context` and renders a
compact `Inspection path · ...` sentence using `lookupContext`. It displays:
- `Lookup region`
- `Control account ID`
- `Target account ID`
- `Cross-account lookup`
- `Observer role configured`

When `resultScope` is present, the panel renders:
- `Sample type`
- `Requested sample cap`
- `Effective sample cap`
- `Returned sample items`
- `Scope status`

Rendered states:
- loading while the lazy read is in flight
- unavailable when `available=false`, with friendly copy mapped from `unavailableReason`
- empty when the route is available but has no typed body to render
- supported when the response includes `ecsService`, `ecsTask`, `autoScalingGroup`, `userDataBootstrapSource`, or `trafficRoutingResource`

Bookmarkable selector state stays scoped to the resource page:
- `clusterArn` persists for selected ECS service and task inspections.
- `instanceId` persists for selected ASG-backed inspections.
- `resourceInspectionLimit` persists the live-inspection sample cap without reusing audit or recent-log limit query state.

The resource toolbar exposes narrow reset actions where appropriate, such as
clearing only the selected cluster or only the selected instance while preserving
resource identity, limits, filters, and the current section hash.

## Release guard matrix

Local and no-AWS release guards intentionally cover representative live-inspection states instead of requiring provider credentials. Keep the matrix compact and tied to the operator page:

- ECS service/task inspections prove selector echoes (`clusterArn`), request-route metadata, `lookupContext`, bounded `resultScope`, friendly unavailable copy, and explicit reasons such as `aws_unavailable`, `no_running_tasks`, `resource_not_found`, or `cross_account_not_configured`.
- Auto Scaling and bootstrap-source inspections prove `instanceId` echoes, optional backing-resource metadata, bounded sample fields, malformed-selector rejection, and explicit local/no-AWS unavailable payloads.
- Traffic-routing resources prove the local-friendly summary path (`trafficRoutingResource.inspectionMode=summary_only`) remains useful when AWS live inspection is unavailable.
- Available-but-empty inspection envelopes keep route, request ID, lookup context, and result-scope metadata visible without inventing raw machine-code details for operators.
- The embedded UI release guard checks friendly copy and stable machine fields together so Live inspection stays aligned with Recent logs while preserving safe DOM construction.

## Current Limitations

This slice is intentionally read-only and refresh-based:
- no streaming provider updates
- no live AWS mutation
- local fixture and Compose runs can return explicit unavailable or summary-only payloads without treating that as a route failure
- `resultScope` describes the returned bounded sample, not a durable inventory or provider-wide total
- unsupported resources return explicit unavailable payloads instead of generic provider errors
