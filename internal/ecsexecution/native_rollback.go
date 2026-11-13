package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// NativeRollbackTarget is derived from an owned, durably accepted forward update.
// It names ECS's actual source revision, which can differ from the service's
// desired task definition after a failed deployment. HTTP callers cannot select it.
type NativeRollbackTarget = applyrequest.ECSNativeRollback

func NativeRollbackEvidence(parent applyrequest.Record) (NativeRollbackTarget, error) {
	target, err := applyrequest.NativeRollbackEvidence(parent)
	if err != nil {
		return NativeRollbackTarget{}, err
	}
	return *target, nil
}

type NativeRollbackClient interface {
	NativeDeploymentClient
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	StopServiceDeployment(context.Context, *ecs.StopServiceDeploymentInput, ...func(*ecs.Options)) (*ecs.StopServiceDeploymentOutput, error)
}

// NativeRollback requests ECS-owned recovery; acceptance is not healthy recovery.
// The explicit advanced-execution gate controls production/API access.
type NativeRollback struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (NativeRollbackClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

// The target is supplied separately to keep it server-authored and avoid reusing
// Rolling recovery's task-definition update semantics.
func (h NativeRollback) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, deployment, err := h.observe(ctx, r, target)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if deployment.Status != types.ServiceDeploymentStatusPending && deployment.Status != types.ServiceDeploymentStatusInProgress && !nativeRollbackAccepted(deployment, target) {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("native deployment no longer accepts rollback")
	}
	if deployment.Status == types.ServiceDeploymentStatusPending || deployment.Status == types.ServiceDeploymentStatusInProgress {
		if err := h.CheckCapabilities(ctx, r.Role, nativeStopChecks(r, target)); err != nil {
			return applyrequest.ECSOperationResult{}, err
		}
	}
	return rollbackResult(target), nil
}
func rollbackResult(t NativeRollbackTarget) applyrequest.ECSOperationResult {
	return applyrequest.ECSOperationResult{ServiceDeploymentARN: t.DeploymentARN, ServiceRevisionARN: t.SourceRevisionARN, PreviousServiceRevisionARN: t.ForwardRevisionARN, TaskDefinitionARN: t.SourceTaskDefinitionARN, PreviousTaskDefinitionARN: t.ForwardTaskDefinitionARN}
}
func (h NativeRollback) Execute(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result := rollbackResult(target)
	p := r.PreparedResult
	if p.ServiceDeploymentARN != result.ServiceDeploymentARN || p.ServiceRevisionARN != result.ServiceRevisionARN || p.PreviousServiceRevisionARN != result.PreviousServiceRevisionARN || p.TaskDefinitionARN != result.TaskDefinitionARN || p.PreviousTaskDefinitionARN != result.PreviousTaskDefinitionARN {
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("durable native stop preparation required"))
	}
	client, deployment, err := h.observe(ctx, r, target)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if nativeRollbackAccepted(deployment, target) {
		return result, nil
	}
	if deployment.Status != types.ServiceDeploymentStatusPending && deployment.Status != types.ServiceDeploymentStatusInProgress {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSCallError{Err: fmt.Errorf("native deployment is no longer eligible for rollback; no stop was sent"), Rejected: true}
	}
	checks := nativeStopChecks(r, target)
	if err := h.CheckCapabilities(ctx, r.Role, checks); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if err := authorizeMutation(ctx, r); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	output, err := client.StopServiceDeployment(ctx, &ecs.StopServiceDeploymentInput{ServiceDeploymentArn: aws.String(target.DeploymentARN), StopType: types.StopServiceDeploymentStopTypeRollback}, func(o *ecs.Options) { o.Retryer = aws.NopRetryer{}; o.RetryMaxAttempts = 1 })
	if err != nil {
		return applyrequest.ECSOperationResult{}, ecsMutationError(err)
	}
	if output == nil || aws.ToString(output.ServiceDeploymentArn) != target.DeploymentARN {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("stop acceptance lacks the exact deployment ARN; reconcile without repeating")
	}
	result.AWSRequestID, _ = awsmiddleware.GetRequestIDMetadata(output.ResultMetadata)
	return result, nil
}
func (h NativeRollback) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSReconciliation, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, deployment, err := h.observe(ctx, r, target)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	// PENDING/IN_PROGRESS after a lost response does not prove the call was unsent.
	return applyrequest.ECSReconciliation{Complete: nativeRollbackAccepted(deployment, target), Result: rollbackResult(target)}, nil
}
func nativeRollbackAccepted(d *types.ServiceDeployment, t NativeRollbackTarget) bool {
	switch d.Status {
	case types.ServiceDeploymentStatusRollbackRequested, types.ServiceDeploymentStatusRollbackInProgress, types.ServiceDeploymentStatusRollbackSuccessful, types.ServiceDeploymentStatusRollbackFailed:
		return d.Rollback == nil && (d.Status == types.ServiceDeploymentStatusRollbackRequested || d.Status == types.ServiceDeploymentStatusRollbackInProgress) || d.Rollback != nil && aws.ToString(d.Rollback.ServiceRevisionArn) == t.SourceRevisionARN
	default:
		return false
	}
}
func (h NativeRollback) observe(ctx context.Context, r applyrequest.ECSOperationRequest, t NativeRollbackTarget) (NativeRollbackClient, *types.ServiceDeployment, error) {
	if err := verifiedNativeRole(r.Intent, r.Role); err != nil {
		return nil, nil, err
	}
	if err := t.Validate(r.Intent); err != nil {
		return nil, nil, err
	}
	if h.ClientFor == nil || h.CheckCapabilities == nil || r.Recovery != nil {
		return nil, nil, fmt.Errorf("native rollback read providers required")
	}
	checks := append(nativeReadChecks(r), applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeServices", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}})
	if err := h.CheckCapabilities(ctx, r.Role, checks); err != nil {
		return nil, nil, err
	}
	client, err := h.ClientFor(ctx, r.Role)
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, fmt.Errorf("native rollback client unavailable")
	}
	services, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(r.Intent.ClusterARN), Services: []string{r.Intent.ServiceARN}})
	if err != nil {
		return nil, nil, err
	}
	if services == nil || len(services.Failures) != 0 || len(services.Services) != 1 {
		return nil, nil, fmt.Errorf("native rollback service unavailable")
	}
	service := &services.Services[0]
	if aws.ToString(service.ServiceArn) != r.Intent.ServiceARN || aws.ToString(service.ClusterArn) != r.Intent.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.SchedulingStrategy != types.SchedulingStrategyReplica || (aws.ToString(service.TaskDefinition) != t.ForwardTaskDefinitionARN && aws.ToString(service.TaskDefinition) != t.SourceTaskDefinitionARN) || (service.DesiredCount != t.DesiredCount && int64(service.DesiredCount) != int64(r.Intent.DesiredCount)) || !nativeBindingMatches(service, r.Intent) || service.DeploymentConfiguration == nil || string(service.DeploymentConfiguration.Strategy) != r.Intent.DeploymentStrategy {
		return nil, nil, fmt.Errorf("native rollback service placement/capacity/routing changed")
	}
	deployment, err := describeNativeDeployment(ctx, client, t.DeploymentARN, r)
	if err != nil {
		return nil, nil, err
	}
	if deployment.CreatedAt == nil || aws.ToString(deployment.TargetServiceRevision.Arn) != t.ForwardRevisionARN || len(deployment.SourceServiceRevisions) != 1 || aws.ToString(deployment.SourceServiceRevisions[0].Arn) != t.SourceRevisionARN || !advancedConfigMatches(deployment.DeploymentConfiguration, r.Intent) {
		return nil, nil, fmt.Errorf("native rollback deployment no longer matches captured forward/source evidence")
	}
	if deployment.Rollback != nil && aws.ToString(deployment.Rollback.ServiceRevisionArn) != t.SourceRevisionARN {
		return nil, nil, fmt.Errorf("ECS rollback names a different source revision")
	}
	history, err := listNativeDeployments(ctx, client, r)
	if err != nil {
		return nil, nil, err
	}
	if err := checkNativeRollbackHistory(ctx, client, r, deployment, history, t); err != nil {
		return nil, nil, err
	}
	for _, ref := range []struct{ revision, task string }{{t.ForwardRevisionARN, t.ForwardTaskDefinitionARN}, {t.SourceRevisionARN, t.SourceTaskDefinitionARN}} {
		revision, err := describeNativeRevision(ctx, client, ref.revision, r)
		if err != nil {
			return nil, nil, err
		}
		if aws.ToString(revision.TaskDefinition) != ref.task {
			return nil, nil, fmt.Errorf("native rollback immutable workload reference changed")
		}
	}
	return client, deployment, nil
}

var errRollbackSuperseded = errors.New("intervening or ambiguous native deployment prevents rollback")

// A rollback may start a companion deployment of the prior immutable revision.
// Bind at most one such row to the rollback instant and the exact reverse
// revision pair. A later redeploy of even the same workload is unrelated.
func checkNativeRollbackHistory(ctx context.Context, client NativeDeploymentClient, r applyrequest.ECSOperationRequest, forward *types.ServiceDeployment, history []types.ServiceDeploymentBrief, target NativeRollbackTarget) error {
	found, companion := false, false
	for _, brief := range history {
		arn := aws.ToString(brief.ServiceDeploymentArn)
		if arn == target.DeploymentARN {
			found = true
			continue
		}
		if brief.CreatedAt == nil {
			return fmt.Errorf("native recovery deployment ordering unavailable")
		}
		if brief.CreatedAt.Before(*forward.CreatedAt) && nativeTerminal(brief.Status) {
			continue
		}
		rb := forward.Rollback
		if companion || !nativeRollbackAccepted(forward, target) || rb == nil || rb.StartedAt == nil || !brief.CreatedAt.Equal(*rb.StartedAt) || !brief.CreatedAt.After(*forward.CreatedAt) || aws.ToString(brief.TargetServiceRevisionArn) != target.SourceRevisionARN {
			return errRollbackSuperseded
		}
		other, err := describeNativeDeployment(ctx, client, arn, r)
		if err != nil {
			return err
		}
		if other.CreatedAt == nil || !other.CreatedAt.Equal(*rb.StartedAt) || other.Rollback != nil || aws.ToString(other.TargetServiceRevision.Arn) != target.SourceRevisionARN || len(other.SourceServiceRevisions) != 1 || aws.ToString(other.SourceServiceRevisions[0].Arn) != target.ForwardRevisionARN || (other.Status != types.ServiceDeploymentStatusPending && other.Status != types.ServiceDeploymentStatusInProgress && other.Status != types.ServiceDeploymentStatusSuccessful) {
			return errRollbackSuperseded
		}
		companion = true
	}
	if !found {
		return fmt.Errorf("rollback deployment missing from current history")
	}
	return nil
}

func nativeStopChecks(r applyrequest.ECSOperationRequest, target NativeRollbackTarget) []applyrequest.OperationExecutionCapabilityCheck {
	return []applyrequest.OperationExecutionCapabilityCheck{
		{Action: "ecs:StopServiceDeployment", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}},
		{Action: "ecs:StopServiceDeployment", ResourceName: target.DeploymentARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN, "ecs:service": r.Intent.ServiceARN}},
	}
}
