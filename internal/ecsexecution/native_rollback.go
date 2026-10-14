package ecsexecution

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// NativeRollbackTarget is derived from an owned, durably accepted forward update.
// It names ECS's actual source revision, which can differ from the service's
// desired task definition after a failed deployment. HTTP callers cannot select it.
type NativeRollbackTarget struct {
	ParentApplyID            string `json:"parentApplyId"`
	DeploymentARN            string `json:"deploymentArn"`
	ForwardRevisionARN       string `json:"forwardRevisionArn"`
	SourceRevisionARN        string `json:"sourceRevisionArn"`
	ForwardTaskDefinitionARN string `json:"forwardTaskDefinitionArn"`
	SourceTaskDefinitionARN  string `json:"sourceTaskDefinitionArn"`
	DesiredCount             int32  `json:"desiredCount"`
}

func NativeRollbackEvidence(parent applyrequest.Record) (NativeRollbackTarget, error) {
	var empty NativeRollbackTarget
	if parent.ID == "" || parent.Principal == nil || !parent.Principal.Identity.Authenticated || parent.Recovery != nil || (parent.Status != applyrequest.StatusRunning && parent.Status != applyrequest.StatusFailed) || len(parent.Operations) != 3 {
		return empty, fmt.Errorf("owned running/failed native forward apply required")
	}
	kinds := []reconcile.StepKind{reconcile.StepRegisterECSTaskDefinition, reconcile.StepUpdateECSService, reconcile.StepVerifyServiceStability}
	for i, operation := range parent.Operations {
		if operation.Kind != string(kinds[i]) || !reflect.DeepEqual(operation.ECS, parent.Operations[0].ECS) {
			return empty, fmt.Errorf("unaltered native forward operations required")
		}
	}
	first, update := parent.Operations[0], parent.Operations[1]
	if first.ECS == nil || update.ECS == nil || first.ECS.DeploymentStrategy == "ROLLING" || first.ECSCheckpoint == nil || first.ECSCheckpoint.State != "complete" || update.ECSCheckpoint == nil || update.ECSCheckpoint.State != "complete" || update.ECSCheckpoint.MutationAuthorizedAt == "" {
		return empty, fmt.Errorf("durable owned registration and update checkpoints required")
	}
	if _, err := time.Parse(time.RFC3339Nano, update.ECSCheckpoint.MutationAuthorizedAt); err != nil {
		return empty, fmt.Errorf("valid forward mutation authorization timestamp required")
	}
	result := update.ECSCheckpoint.Result
	if result.NativeService == nil || result.NativeService.NoOpDeploymentARN != "" || result.PreviousService == nil || first.ECSCheckpoint.Result.TaskDefinitionARN != result.TaskDefinitionARN {
		return empty, fmt.Errorf("adopted or incomplete deployment cannot authorize rollback")
	}
	target := NativeRollbackTarget{ParentApplyID: parent.ID, DeploymentARN: result.ServiceDeploymentARN, ForwardRevisionARN: result.ServiceRevisionARN, SourceRevisionARN: result.PreviousServiceRevisionARN, ForwardTaskDefinitionARN: result.TaskDefinitionARN, SourceTaskDefinitionARN: result.PreviousTaskDefinitionARN, DesiredCount: result.PreviousService.DesiredCount}
	known := false
	for _, source := range result.NativeService.SourceRevisions {
		known = known || source.ARN == target.SourceRevisionARN && source.TaskDefinitionARN == target.SourceTaskDefinitionARN
	}
	if !known {
		return empty, fmt.Errorf("rollback requires the actual source captured before the forward write")
	}
	if err := target.validate(applyrequest.ECSOperationRequest{Intent: *first.ECS}); err != nil {
		return empty, err
	}
	return target, nil
}

func (t NativeRollbackTarget) validate(r applyrequest.ECSOperationRequest) error {
	base, _ := arn.Parse(r.Intent.BaseTaskDefinitionARN)
	family, _, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
	if t.ParentApplyID == "" || t.DesiredCount != int32(r.Intent.DesiredCount) || !nativeScopedARN(t.DeploymentARN, "service-deployment", r.Intent) || !nativeScopedARN(t.ForwardRevisionARN, "service-revision", r.Intent) || !nativeScopedARN(t.SourceRevisionARN, "service-revision", r.Intent) || t.ForwardRevisionARN == t.SourceRevisionARN || !sameFamily(t.ForwardTaskDefinitionARN, r, family) || !sameFamily(t.SourceTaskDefinitionARN, r, family) {
		return fmt.Errorf("fixed native rollback identities and unchanged capacity required")
	}
	return nil
}

type NativeRollbackClient interface {
	NativeDeploymentClient
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	StopServiceDeployment(context.Context, *ecs.StopServiceDeploymentInput, ...func(*ecs.Options)) (*ecs.StopServiceDeploymentOutput, error)
}

// NativeRollback requests ECS-owned recovery; acceptance is not healthy recovery.
// Production/API wiring stays disabled until the separate recovery observer and
// durable child-apply handoff are connected.
type NativeRollback struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (NativeRollbackClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

// The target is supplied separately to keep it server-authored and avoid reusing
// Rolling recovery's task-definition update semantics.
func (h NativeRollback) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest, target NativeRollbackTarget) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, _, err := h.observe(ctx, r, target)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
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
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("native deployment is not eligible for a new rollback request"))
	}
	checks := []applyrequest.OperationExecutionCapabilityCheck{
		{Action: "ecs:StopServiceDeployment", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN}},
		{Action: "ecs:StopServiceDeployment", ResourceName: target.DeploymentARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": r.Intent.ClusterARN, "ecs:service": r.Intent.ServiceARN}},
	}
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
	if err := t.validate(r); err != nil {
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
	if aws.ToString(service.ServiceArn) != r.Intent.ServiceARN || aws.ToString(service.ClusterArn) != r.Intent.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.SchedulingStrategy != types.SchedulingStrategyReplica || (aws.ToString(service.TaskDefinition) != t.ForwardTaskDefinitionARN && aws.ToString(service.TaskDefinition) != t.SourceTaskDefinitionARN) || service.DesiredCount != t.DesiredCount || !nativeBindingMatches(service, r.Intent) || service.DeploymentConfiguration == nil || string(service.DeploymentConfiguration.Strategy) != r.Intent.DeploymentStrategy {
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
	found := false
	for _, brief := range history {
		if aws.ToString(brief.ServiceDeploymentArn) == t.DeploymentARN {
			found = true
			continue
		}
		if brief.CreatedAt == nil || !brief.CreatedAt.Before(*deployment.CreatedAt) || !nativeTerminal(brief.Status) {
			return nil, nil, fmt.Errorf("intervening or ambiguous native deployment prevents rollback")
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("rollback deployment missing from current history")
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
