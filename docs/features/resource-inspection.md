# Resource Inspection

Tenet exposes a read-only live-inspection slice for resource-detail drill-down
pages. It complements Recent logs: Recent logs answers "what recent operator
events or log lines exist?", while Live inspection answers "what does the
selected AWS/control-plane resource currently look like?"

Runtime code is intentionally unchanged by this guide; it documents the existing read-only route contract.

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

`limit` is optional and applies only to bounded ECS service task samples and Auto Scaling Group instance samples. For `ecsService`, it caps the newest inspected
running/stopped task summary bridge. For `candidateAutoScalingGroup`,
`serviceAutoScalingGroup`, and `userDataBootstrapSource` after it resolves a
backing ASG, it caps current-instance samples. Invalid, zero, or negative values
return `400 Bad Request` / `invalid limit query` for resource types that support
bounded samples; resource types without a sample window ignore the query.

`instanceId` is optional and only meaningful for Auto Scaling Group-backed inspection reads. When present, Tenet validates the selector as an EC2 instance ID
(`i-...`) before provider lookup, narrows the inspection to that ASG member, and
echoes the trimmed selector as response-level `instanceId`. If the selected
instance is well formed but is not currently part of the resolved ASG, the route
returns `available=false` / `unavailableReason=resource_not_found` instead of
falling back to another host. Omit it to preserve the bounded ASG sample
behavior.

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
- `source`
- optional `resultScope`
- `unavailableReason` on unavailable responses

Unavailable responses keep the selected route explicit with `available=false`, `unavailableReason`, `lookupContext`, and any accepted selector echo such as `clusterArn` or `instanceId`. This makes local/no-AWS, cross-account, stale-resource,
and unsupported-resource states useful evidence instead of vague provider
failures.

Known unavailable reasons include:
- `cross_account_not_configured`
- `resource_not_found`
- `aws_unavailable`
- `unsupported_resource_type`
- `no_log_source`

Operators should capture the HTTP `Request-Id` response header together with the `lookupContext` object before escalating. The request ID anchors Tenet's structured
API logs, while the lookup context explains the attempted region, control
account, target account, and observer-role path. The embedded UI surfaces that
header as `Tenet request ID` metadata in the Live inspection card when the lazy
read provides it, including successful unavailable payloads and non-auth fetch
errors, so operators can copy the escalation token without opening browser
devtools.

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

For ECS service payloads, the same sample window is also described in
`ecsService.tasksScope`. For ASG payloads, the sibling sample window is described
in `autoScalingGroup.instancesScope`. The top-level `resultScope` exists so
generic clients and scripts can summarize the bounded read without parsing every
resource-specific body first.

ECS service payloads can also include additive deployment-configuration evidence
from `DescribeServices` when AWS returns it:
- `ecsService.deploymentControllerType`
- `ecsService.deploymentCircuitBreaker.enabled`
- `ecsService.deploymentCircuitBreaker.rollback`
- `ecsService.deploymentAlarmConfiguration.enabled`
- `ecsService.deploymentAlarmConfiguration.rollback`
- `ecsService.deploymentAlarmConfiguration.alarmNames[]`

Those fields are optional. Fixture-backed, local, no-AWS, and older provider
payloads omit them rather than emitting empty placeholders.

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
deployment configuration, deployments, recent service events, task definition
evidence, and a bounded task sample when available. The selected-cluster
`clusterArn` selector scopes the service lookup directly and is echoed on
successful and explicit unavailable payloads.

`ecsTask` reads exact task identity, status, stopped reason, placement evidence,
task definition, container rows, optional `awslogs` metadata, and optional EC2
host facts for ECS-on-EC2 tasks when Tenet can resolve and describe the backing
instance. The same `clusterArn` selector keeps task IDs unambiguous when a parent
service handoff selected a cluster.

`candidateAutoScalingGroup` and `serviceAutoScalingGroup` read ASG size, health
settings, target groups, launch-template metadata, current instances, optional
scaling activity evidence, and optional EC2 host facts. The `limit` query bounds
the current-instance sample, and `instanceId` focuses the payload on one current
ASG member.

`userDataBootstrapSource` resolves the backing Auto Scaling Group for a bootstrap
source resource and compares the desired bootstrap source to the observed launch
template marker. Selected-instance inspection can include the matched
`userDataBootstrapSource.selectedInstance` row and nested EC2 details when live
AWS evidence is available.

Traffic-routing resources return `trafficRoutingResource` payloads. In local or
no-AWS runs, Tenet may return `inspectionMode=summary_only` from rollout
execution metadata. When AWS inspection succeeds, the same route can upgrade to
`inspectionMode=live` with CodeDeploy, listener, listener-rule, target-group,
latest-deployment, rollback, error, alarm, deployment-overview, and target-health
evidence.

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

## Current Limitations

This slice is intentionally read-only and refresh-based:
- no streaming provider updates
- no live AWS mutation
- local fixture and Compose runs can return explicit unavailable or summary-only payloads without treating that as a route failure
- `resultScope` describes the returned bounded sample, not a durable inventory or provider-wide total
- unsupported resources return explicit unavailable payloads instead of generic provider errors
