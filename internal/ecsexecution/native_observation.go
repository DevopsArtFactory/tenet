package ecsexecution

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type NativeObservationClient interface {
	ObservationClient
	NativeDeploymentClient
}

// NativeObservation has no mutation API. A successful AWS deployment must also
// match current task health, service counts and the resolved ALB target weight.
type NativeObservation struct {
	ClientFor           func(context.Context, applyrequest.DeployerRoleReadiness) (NativeObservationClient, error)
	CheckCapabilities   func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
	ObserveRouting      func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error)
	ObserveTargetHealth func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness, string, []types.Task) (NativeTargetHealthEvidence, error)
	Timeout             time.Duration
	Now                 func() time.Time
}

func (h NativeObservation) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}
func (h NativeObservation) Preflight(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	if err := verifiedNativeRole(r.Intent, r.Role); err != nil {
		return err
	}
	if h.ClientFor == nil || h.CheckCapabilities == nil || h.ObserveRouting == nil || h.ObserveTargetHealth == nil || r.Recovery != nil {
		return fmt.Errorf("native observation requires read-only providers and forward intent")
	}
	target := r.Intent.BaseTaskDefinitionARN
	if len(r.PriorResults) > 0 {
		target = r.PriorResults[0].TaskDefinitionARN
	}
	service, _ := arn.Parse(r.Intent.ServiceARN)
	cluster, _ := arn.Parse(r.Intent.ClusterARN)
	checks := nativeReadChecks(r)
	checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeServices", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}}, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeTaskDefinition", ResourceName: target, RequiredRole: r.Role.RoleARN}, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:ListTasks", ResourceName: "*", RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}}, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeTasks", ResourceName: fmt.Sprintf("arn:%s:ecs:%s:%s:task/%s/*", service.Partition, r.Intent.Region, r.Intent.AWSAccountID, strings.TrimPrefix(cluster.Resource, "cluster/")), RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}})
	return h.CheckCapabilities(ctx, r.Role, checks)
}
func (h NativeObservation) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	return h.prepare(ctx, r, nil)
}
func (h NativeObservation) prepare(_ context.Context, r applyrequest.ECSOperationRequest, rollback *NativeRollbackTarget) (applyrequest.ECSOperationResult, error) {
	var accepted applyrequest.ECSOperationResult
	if rollback != nil {
		if err := rollback.validate(r); err != nil {
			return accepted, err
		}
		accepted = rollbackResult(*rollback)
		if len(r.PriorResults) != 1 || !sameNativeRollbackResult(r.PriorResults[0], *rollback) {
			return applyrequest.ECSOperationResult{}, fmt.Errorf("native recovery requires its fixed stop acceptance checkpoint")
		}
		accepted = r.PriorResults[0].Clone()
	} else {
		if len(r.PriorResults) != 2 || r.PriorResults[0].TaskDefinitionARN == "" || r.PriorResults[1].TaskDefinitionARN != r.PriorResults[0].TaskDefinitionARN || !nativeScopedARN(r.PriorResults[1].ServiceDeploymentARN, "service-deployment", r.Intent) || !nativeScopedARN(r.PriorResults[1].ServiceRevisionARN, "service-revision", r.Intent) {
			return applyrequest.ECSOperationResult{}, fmt.Errorf("native observation requires registered workload and correlated deployment/revision checkpoints")
		}
		accepted = r.PriorResults[1].Clone()
	}
	result := r.PreparedResult.Clone()
	if result.Observation == nil {
		timeout := h.Timeout
		if timeout <= 0 {
			timeout = 36 * time.Hour
		}
		now := h.now()
		result = accepted.Clone()
		result.Observation = &applyrequest.ECSRolloutObservation{Source: "aws-ecs-native", State: "initializing", Reason: "awaiting_native_observation", Summary: "Awaiting native ECS lifecycle, configured traffic and task evidence.", StartedAt: now.Format(time.RFC3339Nano), DeadlineAt: now.Add(timeout).Format(time.RFC3339Nano), ClusterARN: r.Intent.ClusterARN, ServiceARN: r.Intent.ServiceARN, ServiceDeploymentARN: result.ServiceDeploymentARN, TaskDefinitionARN: result.TaskDefinitionARN, DesiredCount: int32(r.Intent.DesiredCount)}
	}
	if rollback != nil {
		result.Observation.Source = "aws-ecs-native-rollback"
	}
	start, startErr := time.Parse(time.RFC3339Nano, result.Observation.StartedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, result.Observation.DeadlineAt)
	if (rollback != nil && !sameNativeRollbackResult(result, *rollback)) || startErr != nil || deadlineErr != nil || !deadline.After(start) || result.TaskDefinitionARN != accepted.TaskDefinitionARN || result.ServiceDeploymentARN != accepted.ServiceDeploymentARN || result.ServiceRevisionARN != accepted.ServiceRevisionARN || result.PreviousServiceRevisionARN != accepted.PreviousServiceRevisionARN {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("persisted native observation identity/deadline is invalid")
	}
	return result, nil
}
func (h NativeObservation) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	result, err := h.Reconcile(ctx, r)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if result.Complete {
		return result.Result, nil
	}
	if result.Failure != "" {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationFailure{Result: result.Result}
	}
	return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationPending{Result: result.Result, RetryAfter: result.RetryAfter}
}
func (h NativeObservation) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	return h.reconcile(ctx, r, nil)
}
func (h NativeObservation) reconcile(ctx context.Context, r applyrequest.ECSOperationRequest, rollback *NativeRollbackTarget) (applyrequest.ECSReconciliation, error) {
	if err := ctx.Err(); err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if r.PreparedResult.Observation == nil {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("durable native deadline required before polling")
	}
	result, err := h.prepare(ctx, r, rollback)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	o := result.Observation
	o.Polls++
	o.TasksObserved = false
	o.ObservedAt = h.now().Format(time.RFC3339Nano)
	o.Native = &applyrequest.ECSNativeRolloutObservation{TargetRevisionARN: result.ServiceRevisionARN, SourceRevisionARN: result.PreviousServiceRevisionARN}
	deadline, _ := time.Parse(time.RFC3339Nano, o.DeadlineAt)
	if !h.now().Before(deadline) {
		return observedFailure(result, "observation_timeout", "Native ECS observation exceeded its persisted deadline; this does not stop the AWS deployment."), nil
	}
	duration := deadline.Sub(h.now())
	if duration > 30*time.Second {
		duration = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	if err := h.Preflight(ctx, r); err != nil {
		return observationUnavailable(result, err), nil
	}
	client, err := h.ClientFor(ctx, r.Role)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if client == nil {
		return observationUnavailable(result, fmt.Errorf("native observation client unavailable")), nil
	}
	deployment, err := describeNativeDeployment(ctx, client, result.ServiceDeploymentARN, r)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	o.Native.Status = string(deployment.Status)
	o.Native.Stage = string(deployment.LifecycleStage)
	if deployment.Alarms != nil {
		o.Native.TriggeredAlarms = append([]string(nil), deployment.Alarms.TriggeredAlarmNames...)
		slices.Sort(o.Native.TriggeredAlarms)
	}
	if deployment.Rollback != nil {
		o.Native.RollbackRevisionARN = aws.ToString(deployment.Rollback.ServiceRevisionArn)
	}
	expectedForwardRevision := result.ServiceRevisionARN
	if rollback != nil {
		expectedForwardRevision = rollback.ForwardRevisionARN
	}
	if aws.ToString(deployment.TargetServiceRevision.Arn) != expectedForwardRevision || !advancedConfigMatches(deployment.DeploymentConfiguration, r.Intent) {
		return observedFailure(result, "native_deployment_mismatch", "Correlated native deployment no longer matches its reviewed target revision/configuration."), nil
	}
	if rollback != nil {
		if len(deployment.SourceServiceRevisions) != 1 || aws.ToString(deployment.SourceServiceRevisions[0].Arn) != rollback.SourceRevisionARN || (deployment.Rollback != nil && aws.ToString(deployment.Rollback.ServiceRevisionArn) != rollback.SourceRevisionARN) {
			return observedFailure(result, "native_rollback_revision_mismatch", "ECS rollback does not match the fixed source revision."), nil
		}
		revisions := []struct{ revision, task string }{{rollback.ForwardRevisionARN, rollback.ForwardTaskDefinitionARN}, {rollback.SourceRevisionARN, rollback.SourceTaskDefinitionARN}}
		for _, ref := range revisions {
			revision, err := describeNativeRevision(ctx, client, ref.revision, r)
			if err != nil {
				return observationUnavailable(result, err), nil
			}
			if aws.ToString(revision.TaskDefinition) != ref.task {
				return observedFailure(result, "native_rollback_revision_mismatch", "Native recovery immutable workload reference changed."), nil
			}
		}
		if deployment.CreatedAt == nil {
			return observationUnavailable(result, fmt.Errorf("native recovery deployment creation time unavailable")), nil
		}
		history, err := listNativeDeployments(ctx, client, r)
		if err != nil {
			return observationUnavailable(result, err), nil
		}
		found := false
		for _, brief := range history {
			if aws.ToString(brief.ServiceDeploymentArn) == rollback.DeploymentARN {
				found = true
				continue
			}
			if brief.CreatedAt == nil {
				return observationUnavailable(result, fmt.Errorf("native recovery deployment ordering unavailable")), nil
			}
			if !brief.CreatedAt.Before(*deployment.CreatedAt) || !nativeTerminal(brief.Status) {
				return observedFailure(result, "native_rollback_superseded", "Intervening or concurrent native deployment prevents attributing current health to this recovery."), nil
			}
		}
		if !found {
			return observationUnavailable(result, fmt.Errorf("native recovery deployment missing from history")), nil
		}
	}
	if !validNativePercent(deployment.TargetServiceRevision.RequestedProductionTrafficWeight) {
		return observationUnavailable(result, fmt.Errorf("invalid native target traffic evidence")), nil
	}
	o.Native.TargetRequestedPercent = copyNativePercent(deployment.TargetServiceRevision.RequestedProductionTrafficWeight)
	sourcePercent := 0.0
	sourceKnown := true
	for _, source := range deployment.SourceServiceRevisions {
		if !nativeScopedARN(aws.ToString(source.Arn), "service-revision", r.Intent) || !validNativePercent(source.RequestedProductionTrafficWeight) {
			return observationUnavailable(result, fmt.Errorf("invalid native source traffic evidence")), nil
		}
		if source.RequestedProductionTrafficWeight == nil {
			sourceKnown = false
		} else {
			sourcePercent += *source.RequestedProductionTrafficWeight
		}
	}
	if sourceKnown && len(deployment.SourceServiceRevisions) > 0 {
		if sourcePercent > 100.000001 {
			return observationUnavailable(result, fmt.Errorf("native source traffic total exceeds 100")), nil
		}
		o.Native.SourceRequestedPercent = aws.Float64(sourcePercent)
	}
	if rollback != nil {
		o.Native.TargetRequestedPercent = copyNativePercent(deployment.SourceServiceRevisions[0].RequestedProductionTrafficWeight)
		o.Native.SourceRequestedPercent = copyNativePercent(deployment.TargetServiceRevision.RequestedProductionTrafficWeight)
	}
	switch deployment.Status {
	case types.ServiceDeploymentStatusStopped:
		return observedFailure(result, "native_deployment_stopped", "AWS stopped the native deployment: "+shortEvidence(aws.ToString(deployment.StatusReason))), nil
	case types.ServiceDeploymentStatusRollbackFailed:
		return observedFailure(result, "native_rollback_failed", "AWS reports native rollback failure: "+shortEvidence(aws.ToString(deployment.StatusReason))), nil
	case types.ServiceDeploymentStatusRollbackSuccessful:
		if rollback == nil {
			return observedFailure(result, "native_rollback_reported", "AWS reports a rollback to its source revision; the requested forward revision did not succeed."), nil
		}
		if deployment.Rollback == nil {
			return observationUnavailable(result, fmt.Errorf("completed rollback source identity unavailable")), nil
		}
	}
	target, err := describeNativeRevision(ctx, client, result.ServiceRevisionARN, r)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if aws.ToString(target.TaskDefinition) != result.TaskDefinitionARN {
		return observedFailure(result, "native_revision_mismatch", "Native target revision workload differs from the registered task definition."), nil
	}
	targetGroup, err := nativeRevisionTargetGroup(target, r.Intent)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	routing, err := h.ObserveRouting(ctx, r.Intent, r.Role)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	targetPercent, err := configuredNativePercent(routing, targetGroup, r.Intent)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	o.Native.TargetConfiguredPercent = aws.Float64(targetPercent)
	o.Native.SourceConfiguredPercent = aws.Float64(100 - targetPercent)
	services, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(r.Intent.ClusterARN), Services: []string{r.Intent.ServiceARN}})
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if services == nil || len(services.Failures) > 0 || len(services.Services) != 1 {
		return observationUnavailable(result, fmt.Errorf("current native service evidence unavailable")), nil
	}
	service := &services.Services[0]
	if aws.ToString(service.ServiceArn) != r.Intent.ServiceARN || aws.ToString(service.ClusterArn) != r.Intent.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.SchedulingStrategy != types.SchedulingStrategyReplica {
		return observationUnavailable(result, fmt.Errorf("native service placement/controller unavailable")), nil
	}
	if len(service.LoadBalancers) == 0 || len(service.Deployments) == 0 {
		return observationUnavailable(result, fmt.Errorf("current native service routing evidence unavailable")), nil
	}
	if !nativeBindingMatches(service, r.Intent) {
		return observedFailure(result, "native_routing_superseded", "Current service routing no longer matches the reviewed native ALB binding."), nil
	}
	o.ServiceRunningCount = service.RunningCount
	o.PendingCount = service.PendingCount
	definition, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(result.TaskDefinitionARN)})
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if definition == nil || definition.TaskDefinition == nil || aws.ToString(definition.TaskDefinition.TaskDefinitionArn) != result.TaskDefinitionARN {
		return observationUnavailable(result, fmt.Errorf("registered native task definition unavailable")), nil
	}
	if rollback == nil && definition.TaskDefinition.Status != types.TaskDefinitionStatusActive {
		return observedFailure(result, "requested_revision_inactive", "The registered native task definition was deactivated."), nil
	}
	tasks, err := runningTasks(ctx, client, r)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	running, healthy, old, pending := int32(0), int32(0), int32(0), int32(0)
	var intendedTasks []types.Task
	containerHealthy := map[string]bool{}
	for _, task := range tasks {
		if aws.ToString(task.ClusterArn) != r.Intent.ClusterARN || (task.Group != nil && aws.ToString(task.Group) != "service:"+shortServiceName(r.Intent.ServiceARN)) {
			return observationUnavailable(result, fmt.Errorf("native task evidence belongs to another service/cluster")), nil
		}
		if aws.ToString(task.LastStatus) != "RUNNING" {
			pending++
			continue
		}
		if aws.ToString(task.TaskDefinitionArn) != result.TaskDefinitionARN {
			old++
			continue
		}
		running++
		intendedTasks = append(intendedTasks, task)
		if healthyTask(task, *definition.TaskDefinition) {
			containerHealthy[aws.ToString(task.TaskArn)] = true
		}
	}
	targetHealth, err := h.ObserveTargetHealth(ctx, r.Intent, r.Role, targetGroup, intendedTasks)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	seenHealthy := map[string]bool{}
	for _, taskARN := range targetHealth.HealthyTaskARNs {
		if containerHealthy[taskARN] && !seenHealthy[taskARN] {
			healthy++
			seenHealthy[taskARN] = true
		}
	}
	o.TasksObserved = true
	o.RunningCount = running
	o.HealthyCount = healthy
	o.PreviousCount = old
	if pending > o.PendingCount {
		o.PendingCount = pending
	}
	desired := int32(r.Intent.DesiredCount)
	sourcesDrained := true
	for _, source := range deployment.SourceServiceRevisions {
		if source.RunningTaskCount != 0 || source.PendingTaskCount != 0 {
			sourcesDrained = false
		}
	}
	controlComplete := deployment.Status == types.ServiceDeploymentStatusSuccessful
	configComplete := advancedConfigMatches(service.DeploymentConfiguration, r.Intent)
	if rollback != nil {
		controlComplete = deployment.Status == types.ServiceDeploymentStatusRollbackSuccessful
		configComplete = service.DeploymentConfiguration != nil && string(service.DeploymentConfiguration.Strategy) == r.Intent.DeploymentStrategy
		sourcesDrained = deployment.TargetServiceRevision.RunningTaskCount == 0 && deployment.TargetServiceRevision.PendingTaskCount == 0
	}
	if controlComplete && targetPercent == 100 && sourcesDrained && aws.ToString(service.TaskDefinition) == result.TaskDefinitionARN && service.DesiredCount == desired && service.RunningCount == desired && service.PendingCount == 0 && running == desired && healthy == desired && targetHealth.UnexpectedTargets == 0 && old == 0 && pending == 0 && configComplete {
		o.State = "converged"
		o.Reason = "native_revision_healthy"
		o.Summary = fmt.Sprintf("Native ECS deployment verified: %d/%d healthy tasks, configured target traffic 100%%, source traffic/tasks drained.", healthy, desired)
		if rollback != nil {
			o.Reason = "native_source_recovered"
			o.Summary = fmt.Sprintf("Native ECS rollback verified: %d/%d source tasks healthy, configured source traffic 100%%, forward tasks drained.", healthy, desired)
		}
		return applyrequest.ECSReconciliation{Complete: true, Result: result}, nil
	}
	reason := "native_deployment_in_progress"
	if targetHealth.UnexpectedTargets > 0 {
		reason = "native_target_members_pending"
	} else if healthy < running {
		reason = "native_target_health_pending"
	}
	if deployment.Status == types.ServiceDeploymentStatusRollbackRequested || deployment.Status == types.ServiceDeploymentStatusRollbackInProgress {
		reason = "native_rollback_in_progress"
	} else if deployment.Status == types.ServiceDeploymentStatusStopRequested {
		reason = "native_stop_requested"
	} else if controlComplete {
		reason = "native_completion_evidence_pending"
	}
	traffic := "unknown"
	if o.Native.TargetConfiguredPercent != nil {
		traffic = fmt.Sprintf("%g%%", *o.Native.TargetConfiguredPercent)
	}
	return observationProgress(result, reason, fmt.Sprintf("ECS %s / %s: configured target traffic %s, %d/%d intended tasks healthy, %d previous tasks. %s", deployment.Status, deployment.LifecycleStage, traffic, healthy, desired, old, shortEvidence(aws.ToString(deployment.StatusReason)))), nil
}
func validNativePercent(value *float64) bool {
	return value == nil || !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 100
}
func copyNativePercent(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return aws.Float64(*value)
}
func nativeRevisionTargetGroup(revision *types.ServiceRevision, intent domain.ECSDeploymentIntent) (string, error) {
	if revision.ResolvedConfiguration == nil || len(revision.ResolvedConfiguration.LoadBalancers) != 1 {
		return "", fmt.Errorf("native target routing resolution unavailable")
	}
	binding := revision.ResolvedConfiguration.LoadBalancers[0]
	group := aws.ToString(binding.TargetGroupArn)
	if aws.ToString(binding.ProductionListenerRule) != intent.Advanced.Routing.ProductionListenerRuleARN || (group != intent.Advanced.Routing.TargetGroupARN && group != intent.Advanced.Routing.AlternateTargetGroupARN) {
		return "", fmt.Errorf("resolved native target routing is unreviewed")
	}
	return group, nil
}
func configuredNativePercent(routing ALBRoutingSnapshot, group string, intent domain.ECSDeploymentIntent) (float64, error) {
	if routing.Signature == "" || routing.Production.Primary < 0 || routing.Production.Primary > 999 || routing.Production.Alternate < 0 || routing.Production.Alternate > 999 {
		return 0, fmt.Errorf("configured native traffic evidence unavailable")
	}
	total := float64(routing.Production.Primary) + float64(routing.Production.Alternate)
	if total == 0 {
		return 0, fmt.Errorf("configured native traffic has no positive target")
	}
	weight := routing.Production.Primary
	if group == intent.Advanced.Routing.AlternateTargetGroupARN {
		weight = routing.Production.Alternate
	} else if group != intent.Advanced.Routing.TargetGroupARN {
		return 0, fmt.Errorf("configured native target is unreviewed")
	}
	return float64(weight) * 100 / total, nil
}
