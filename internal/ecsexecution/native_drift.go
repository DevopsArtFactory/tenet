package ecsexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// NativeDriftClient has no mutation APIs. ECS owns native traffic repair through
// a newly approved deployment; the probe never writes ALB configuration.
type NativeDriftClient interface {
	DriftClient
	NativeDeploymentClient
}
type NativeManagedDrift struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (NativeDriftClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
	ObserveRouting    func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error)
}

func (p NativeManagedDrift) Observe(ctx context.Context, intent domain.ECSDeploymentIntent, role applyrequest.DeployerRoleReadiness, verified applyrequest.Record) (bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if err := verifiedNativeRole(intent, role); err != nil {
		return false, "", err
	}
	wanted, err := applyrequest.VerifiedNativeService(verified, intent)
	if err != nil {
		return false, "", err
	}
	if !nativeScopedARN(wanted.ServiceDeploymentARN, "service-deployment", intent) || !nativeScopedARN(wanted.ServiceRevisionARN, "service-revision", intent) {
		return false, "", fmt.Errorf("verified native deployment placement unavailable")
	}
	base, _ := arn.Parse(intent.BaseTaskDefinitionARN)
	family, _, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
	request := applyrequest.ECSOperationRequest{Intent: intent, Role: role}
	if !sameFamily(wanted.TaskDefinitionARN, request, family) || p.ClientFor == nil || p.CheckCapabilities == nil || p.ObserveRouting == nil {
		return false, "", fmt.Errorf("native drift requires reviewed workload and read providers")
	}
	checks := append(nativeReadChecks(request), applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeServices", ResourceName: intent.ServiceARN, RequiredRole: role.RoleARN, Context: map[string]string{"ecs:cluster": intent.ClusterARN}})
	if err := p.CheckCapabilities(ctx, role, checks); err != nil {
		return false, "", err
	}
	client, err := p.ClientFor(ctx, role)
	if err != nil {
		return false, "", err
	}
	if client == nil {
		return false, "", fmt.Errorf("native drift client unavailable")
	}
	output, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(intent.ClusterARN), Services: []string{intent.ServiceARN}})
	if err != nil {
		return false, "", err
	}
	if output == nil || len(output.Failures) != 0 || len(output.Services) != 1 {
		return false, "", fmt.Errorf("native service drift observation unavailable")
	}
	service := &output.Services[0]
	if aws.ToString(service.ServiceArn) != intent.ServiceARN || aws.ToString(service.ClusterArn) != intent.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.SchedulingStrategy != types.SchedulingStrategyReplica || service.DeploymentConfiguration == nil || string(service.DeploymentConfiguration.Strategy) != intent.DeploymentStrategy || !nativeBindingMatches(service, intent) || service.DesiredCount < 0 || !sameFamily(aws.ToString(service.TaskDefinition), request, family) {
		return false, "", fmt.Errorf("native service drift placement/routing is outside the reviewed strategy")
	}
	if service.PendingCount != 0 {
		return false, "", fmt.Errorf("native service tasks are still pending")
	}
	config, err := normalizedAdvancedConfig(service.DeploymentConfiguration)
	if err != nil {
		return false, "", err
	}
	history, err := listNativeDeployments(ctx, client, request)
	if err != nil {
		return false, "", err
	}
	var latest *types.ServiceDeploymentBrief
	for i := range history {
		d := &history[i]
		if !nativeTerminal(d.Status) {
			return false, "", fmt.Errorf("external native deployment/rollback is still active")
		}
		if d.CreatedAt == nil {
			return false, "", fmt.Errorf("native drift deployment ordering unavailable")
		}
		if latest == nil || d.CreatedAt.After(*latest.CreatedAt) {
			latest = d
		} else if d.CreatedAt.Equal(*latest.CreatedAt) && aws.ToString(d.ServiceDeploymentArn) != aws.ToString(latest.ServiceDeploymentArn) {
			return false, "", fmt.Errorf("native drift deployment ordering is ambiguous")
		}
	}
	if latest == nil {
		return false, "", fmt.Errorf("native drift deployment history unavailable")
	}
	deployment, err := describeNativeDeployment(ctx, client, aws.ToString(latest.ServiceDeploymentArn), request)
	if err != nil {
		return false, "", err
	}
	revisionARN := aws.ToString(deployment.TargetServiceRevision.Arn)
	switch deployment.Status {
	case types.ServiceDeploymentStatusSuccessful:
		for _, source := range deployment.SourceServiceRevisions {
			if source.RunningTaskCount != 0 || source.PendingTaskCount != 0 {
				return false, "", fmt.Errorf("native source tasks have not drained")
			}
		}
	case types.ServiceDeploymentStatusRollbackSuccessful:
		if deployment.Rollback == nil || len(deployment.SourceServiceRevisions) != 1 || aws.ToString(deployment.Rollback.ServiceRevisionArn) != aws.ToString(deployment.SourceServiceRevisions[0].Arn) || deployment.TargetServiceRevision.RunningTaskCount != 0 || deployment.TargetServiceRevision.PendingTaskCount != 0 {
			return false, "", fmt.Errorf("native rollback source/drain evidence unavailable")
		}
		revisionARN = aws.ToString(deployment.Rollback.ServiceRevisionArn)
	default:
		return false, "", fmt.Errorf("native drift requires a completed deployment or source rollback; failed/stopped work requires operator review")
	}
	revision, err := describeNativeRevision(ctx, client, revisionARN, request)
	if err != nil {
		return false, "", err
	}
	if (revision.ServiceConnectConfiguration != nil && revision.ServiceConnectConfiguration.Enabled) || len(revision.VpcLatticeConfigurations) > 0 {
		return false, "", fmt.Errorf("native drift revision uses unsupported mixed routing")
	}
	if aws.ToString(revision.TaskDefinition) != aws.ToString(service.TaskDefinition) {
		return false, "", fmt.Errorf("native service workload has no correlated terminal revision")
	}
	group, err := nativeRevisionTargetGroup(revision, intent)
	if err != nil {
		return false, "", err
	}
	routing, err := p.ObserveRouting(ctx, intent, role)
	if err != nil {
		return false, "", err
	}
	if routing.Signature == "" {
		return false, "", fmt.Errorf("current ALB drift evidence unavailable")
	}
	percent, err := configuredNativePercent(routing, group, intent)
	if err != nil {
		return false, "", err
	}
	if aws.ToString(service.TaskDefinition) == wanted.TaskDefinitionARN && int(service.DesiredCount) == intent.DesiredCount && advancedConfigMatches(service.DeploymentConfiguration, intent) && percent == 100 {
		return false, "", nil
	}
	snapshot := struct {
		Task                                       string
		Count                                      int32
		Configuration                              *types.DeploymentConfiguration
		Deployment, Revision, TargetGroup, Routing string
		TargetPercent                              float64
	}{aws.ToString(service.TaskDefinition), service.DesiredCount, config, aws.ToString(deployment.ServiceDeploymentArn), revisionARN, group, routing.Signature, percent}
	content, err := json.Marshal(snapshot)
	if err != nil {
		return false, "", err
	}
	sum := sha256.Sum256(content)
	return true, hex.EncodeToString(sum[:]), nil
}
