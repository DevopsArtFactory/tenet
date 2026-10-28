package ecsexecution

import (
	"context"
	"fmt"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// NativeCapacityRestore scales only the captured source after ECS finishes rollback.
// It neither starts another deployment nor changes task/configuration/ALB fields.
type NativeCapacityRestore struct {
	NativeRollback
	ObserveRouting func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error)
	Now            func() time.Time
}

func (h NativeCapacityRestore) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}
func (h NativeCapacityRestore) Preflight(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := h.validate(ctx, r); err != nil {
		return err
	}
	return h.CheckCapabilities(ctx, r.Role, []applyrequest.OperationExecutionCapabilityCheck{{Action: "ecs:UpdateService", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}}})
}
func (h NativeCapacityRestore) validate(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	if _, err := (NativeRollbackOperation{}).Prepare(ctx, r); err != nil {
		return err
	}
	if h.ClientFor == nil || h.CheckCapabilities == nil || h.ObserveRouting == nil || int64(r.NativeRollback.DesiredCount) == int64(r.Intent.DesiredCount) {
		return fmt.Errorf("captured capacity change and native recovery providers required")
	}
	return nil
}
func (h NativeCapacityRestore) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if err := h.validate(ctx, r); err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if len(r.PriorResults) != 1 || !sameNativeRollbackResult(r.PriorResults[0], *r.NativeRollback) {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("capacity restore requires fixed stop acceptance")
	}
	result := r.PreparedResult.Clone()
	if result.Observation == nil {
		result = rollbackResult(*r.NativeRollback)
		now := h.now()
		result.Observation = &applyrequest.ECSRolloutObservation{Source: "aws-ecs-native-rollback", State: "initializing", Reason: "awaiting_native_source_rollback", Summary: "Waiting for source rollback before restoring captured capacity.", StartedAt: now.Format(time.RFC3339Nano), DeadlineAt: now.Add(36 * time.Hour).Format(time.RFC3339Nano), ClusterARN: r.Intent.ClusterARN, ServiceARN: r.Intent.ServiceARN, ServiceDeploymentARN: result.ServiceDeploymentARN, TaskDefinitionARN: result.TaskDefinitionARN, DesiredCount: r.NativeRollback.DesiredCount}
	}
	start, startErr := time.Parse(time.RFC3339Nano, result.Observation.StartedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, result.Observation.DeadlineAt)
	if !sameNativeRollbackResult(result, *r.NativeRollback) || startErr != nil || deadlineErr != nil || !deadline.After(start) || result.Observation.DesiredCount != r.NativeRollback.DesiredCount {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("persisted capacity recovery identity/deadline changed")
	}
	return result, nil
}

// ready is true only for the exact completed source rollback at forward capacity.
func (h NativeCapacityRestore) state(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, NativeRollbackClient, bool, error) {
	result, err := h.Prepare(ctx, r)
	if err != nil {
		return applyrequest.ECSReconciliation{}, nil, false, err
	}
	o := result.Observation
	o.Polls++
	o.ObservedAt = h.now().Format(time.RFC3339Nano)
	o.TasksObserved = false
	deadline, _ := time.Parse(time.RFC3339Nano, o.DeadlineAt)
	if !h.now().Before(deadline) {
		return observedFailure(result, "observation_timeout", "Native source/capacity recovery exceeded its persisted deadline."), nil, false, nil
	}
	duration := deadline.Sub(h.now())
	if duration > 30*time.Second {
		duration = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	client, deployment, err := h.NativeRollback.observe(ctx, r, *r.NativeRollback)
	if err != nil {
		return observationUnavailable(result, err), nil, false, nil
	}
	t := r.NativeRollback
	o.Native = &applyrequest.ECSNativeRolloutObservation{Status: string(deployment.Status), Stage: string(deployment.LifecycleStage), TargetRevisionARN: t.SourceRevisionARN, SourceRevisionARN: t.ForwardRevisionARN}
	if deployment.Rollback != nil {
		o.Native.RollbackRevisionARN = aws.ToString(deployment.Rollback.ServiceRevisionArn)
	}
	switch deployment.Status {
	case types.ServiceDeploymentStatusRollbackFailed, types.ServiceDeploymentStatusStopped, types.ServiceDeploymentStatusSuccessful:
		return observedFailure(result, "native_capacity_rollback_failed", "ECS did not complete the captured source rollback; capacity was not changed."), client, false, nil
	case types.ServiceDeploymentStatusRollbackSuccessful:
	default:
		return observationProgress(result, "awaiting_native_source_rollback", "Waiting for ECS to finish source rollback before restoring capacity."), client, false, nil
	}
	if deployment.Rollback == nil || aws.ToString(deployment.Rollback.ServiceRevisionArn) != t.SourceRevisionARN || deployment.TargetServiceRevision.RunningTaskCount != 0 || deployment.TargetServiceRevision.PendingTaskCount != 0 {
		return observationUnavailable(result, fmt.Errorf("captured rollback source/drain evidence unavailable")), client, false, nil
	}
	source, err := describeNativeRevision(ctx, client, t.SourceRevisionARN, r)
	if err != nil {
		return observationUnavailable(result, err), client, false, nil
	}
	group, err := nativeRevisionTargetGroup(source, r.Intent)
	if err != nil {
		return observationUnavailable(result, err), client, false, nil
	}
	routing, err := h.ObserveRouting(ctx, r.Intent, r.Role)
	if err != nil || routing.Signature == "" {
		return observationUnavailable(result, fmt.Errorf("source ALB configuration unavailable: %v", err)), client, false, nil
	}
	percent, err := configuredNativePercent(routing, group, r.Intent)
	if err != nil {
		return observationUnavailable(result, err), client, false, nil
	}
	o.Native.TargetConfiguredPercent = aws.Float64(percent)
	if percent != 100 {
		return observationProgress(result, "awaiting_native_source_traffic", "Waiting for configured source traffic to reach 100% before capacity recovery."), client, false, nil
	}
	output, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(r.Intent.ClusterARN), Services: []string{r.Intent.ServiceARN}})
	if err != nil || output == nil || len(output.Failures) != 0 || len(output.Services) != 1 {
		return observationUnavailable(result, fmt.Errorf("current source service unavailable: %v", err)), client, false, nil
	}
	s := output.Services[0]
	if aws.ToString(s.ServiceArn) != r.Intent.ServiceARN || aws.ToString(s.ClusterArn) != r.Intent.ClusterARN || aws.ToString(s.Status) != "ACTIVE" || s.DeploymentController == nil || s.DeploymentController.Type != types.DeploymentControllerTypeEcs || s.SchedulingStrategy != types.SchedulingStrategyReplica || aws.ToString(s.TaskDefinition) != t.SourceTaskDefinitionARN || !nativeBindingMatches(&s, r.Intent) || s.DeploymentConfiguration == nil || string(s.DeploymentConfiguration.Strategy) != r.Intent.DeploymentStrategy || (s.DesiredCount != t.DesiredCount && int64(s.DesiredCount) != int64(r.Intent.DesiredCount)) {
		return observationUnavailable(result, fmt.Errorf("source workload/capacity/routing changed before recovery")), client, false, nil
	}
	o.ServiceRunningCount = s.RunningCount
	o.PendingCount = s.PendingCount
	if s.DesiredCount == t.DesiredCount {
		o.State = "capacity_restored"
		o.Reason = "native_source_capacity_restored"
		o.Summary = fmt.Sprintf("Source desired count restored to %d; task and ALB health verification remains pending.", t.DesiredCount)
		return applyrequest.ECSReconciliation{Complete: true, Result: result}, client, false, nil
	}
	return observationProgress(result, "native_capacity_restore_pending", "Captured source rollback is complete; previous capacity still needs restoration."), client, true, nil
}
func (h NativeCapacityRestore) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	outcome, client, ready, err := h.state(ctx, r)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if outcome.Complete {
		return outcome.Result, nil
	}
	if outcome.Failure != "" {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationFailure{Result: outcome.Result}
	}
	if !ready {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationPending{Result: outcome.Result, RetryAfter: outcome.RetryAfter}
	}
	updater, ok := client.(interface {
		UpdateService(context.Context, *ecs.UpdateServiceInput, ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	})
	if !ok {
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("native capacity update provider unavailable"))
	}
	if err := h.Preflight(ctx, r); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if err := authorizeMutation(ctx, r); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	deadline, _ := time.Parse(time.RFC3339Nano, outcome.Result.Observation.DeadlineAt)
	if !h.now().Before(deadline) || ctx.Err() != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("capacity recovery deadline/context expired before mutation"))
	}
	out, err := updater.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(r.Intent.ClusterARN), Service: aws.String(r.Intent.ServiceARN), DesiredCount: aws.Int32(r.NativeRollback.DesiredCount)}, func(o *ecs.Options) { o.Retryer = aws.NopRetryer{}; o.RetryMaxAttempts = 1 })
	if err != nil {
		return applyrequest.ECSOperationResult{}, ecsMutationError(err)
	}
	if out == nil || out.Service == nil || aws.ToString(out.Service.ServiceArn) != r.Intent.ServiceARN || aws.ToString(out.Service.ClusterArn) != r.Intent.ClusterARN || aws.ToString(out.Service.TaskDefinition) != r.NativeRollback.SourceTaskDefinitionARN || out.Service.DesiredCount != r.NativeRollback.DesiredCount {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("capacity acceptance unavailable; reconcile without repeating")
	}
	result := outcome.Result
	result.Observation.State = "capacity_update_accepted"
	result.Observation.Reason = "native_source_capacity_accepted"
	result.Observation.Summary = fmt.Sprintf("ECS accepted source desired count %d; current task and ALB health remain unverified.", r.NativeRollback.DesiredCount)
	result.AWSRequestID, _ = awsmiddleware.GetRequestIDMetadata(out.ResultMetadata)
	return result, nil
}
func (h NativeCapacityRestore) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	// Once a count update may have been sent, unchanged count is never replay authority.
	outcome, _, _, err := h.state(ctx, r)
	return outcome, err
}
