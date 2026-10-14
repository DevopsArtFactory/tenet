package ecsexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// DriftClient intentionally exposes no AWS mutation API.
type DriftClient interface {
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
}
type ManagedDrift struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (DriftClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

// Observe compares only fields managed by Tenet, after a verified successful Apply.
// Unavailable/in-progress observations never authorize self-healing.
func (p ManagedDrift) Observe(ctx context.Context, intent domain.ECSDeploymentIntent, role applyrequest.DeployerRoleReadiness, verified applyrequest.Record) (bool, string, error) {
	service, se := arn.Parse(intent.ServiceARN)
	cluster, ce := arn.Parse(intent.ClusterARN)
	principal, pe := arn.Parse(role.RoleARN)
	if p.ClientFor == nil || p.CheckCapabilities == nil || se != nil || ce != nil || pe != nil || service.Service != "ecs" || !strings.HasPrefix(service.Resource, "service/") || cluster.Service != "ecs" || !strings.HasPrefix(cluster.Resource, "cluster/") || service.AccountID != intent.AWSAccountID || cluster.AccountID != intent.AWSAccountID || service.Region != intent.Region || cluster.Region != intent.Region || service.Partition != cluster.Partition || principal.Partition != service.Partition || principal.Service != "iam" || !strings.HasPrefix(principal.Resource, "role/") || principal.AccountID != intent.AWSAccountID || !role.Configured || !role.Ready || !role.CheckedWithAWS || role.AWSAccountID != intent.AWSAccountID || role.Region != intent.Region || intent.DeploymentController != "ECS" || intent.DeploymentStrategy != "ROLLING" {
		return false, "", fmt.Errorf("managed drift requires verified Rolling service placement and deployer role")
	}
	if verified.Status != applyrequest.StatusSucceeded || verified.ECSRuntime == nil || verified.ECSRuntime.State != "converged" || len(verified.Operations) != 3 || verified.Operations[0].ECS == nil || *verified.Operations[0].ECS != intent || verified.Operations[0].ECSCheckpoint == nil {
		return false, "", fmt.Errorf("managed drift requires the last verified reviewed intent")
	}
	target := verified.Operations[0].ECSCheckpoint.Result.TaskDefinitionARN
	if target == "" {
		return false, "", fmt.Errorf("verified Apply omitted registered revision")
	}
	checks := []applyrequest.OperationExecutionCapabilityCheck{{Action: "ecs:DescribeServices", ResourceType: "ecsService", ResourceName: intent.ServiceARN, RequiredRole: role.RoleARN, Context: map[string]string{"ecs:cluster": intent.ClusterARN}}}
	if err := p.CheckCapabilities(ctx, role, checks); err != nil {
		return false, "", err
	}
	client, err := p.ClientFor(ctx, role)
	if err != nil {
		return false, "", err
	}
	if client == nil {
		return false, "", fmt.Errorf("drift client unavailable")
	}
	output, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(intent.ClusterARN), Services: []string{intent.ServiceARN}})
	if err != nil {
		return false, "", err
	}
	if output == nil || len(output.Failures) != 0 || len(output.Services) != 1 {
		return false, "", fmt.Errorf("service drift observation unavailable")
	}
	observed := output.Services[0]
	config := observed.DeploymentConfiguration
	if aws.ToString(observed.ServiceArn) != intent.ServiceARN || aws.ToString(observed.ClusterArn) != intent.ClusterARN || aws.ToString(observed.Status) != "ACTIVE" || observed.DeploymentController == nil || observed.DeploymentController.Type != types.DeploymentControllerTypeEcs || observed.SchedulingStrategy != types.SchedulingStrategyReplica || config == nil || (config.Strategy != "" && config.Strategy != types.DeploymentStrategyRolling) || config.MinimumHealthyPercent == nil || config.MaximumPercent == nil || aws.ToString(observed.TaskDefinition) == "" {
		return false, "", fmt.Errorf("service drift placement/configuration unavailable")
	}
	primary := 0
	for _, d := range observed.Deployments {
		if d.RolloutState == types.DeploymentRolloutStateInProgress || d.PendingCount > 0 || (aws.ToString(d.Status) != "PRIMARY" && d.RunningCount > 0) {
			return false, "", fmt.Errorf("external service rollout still active")
		}
		if aws.ToString(d.Status) == "PRIMARY" {
			primary++
		}
	}
	if primary != 1 || observed.PendingCount > 0 {
		return false, "", fmt.Errorf("service drift deployment observation unavailable")
	}
	matches := aws.ToString(observed.TaskDefinition) == target && int(observed.DesiredCount) == intent.DesiredCount && int(aws.ToInt32(config.MinimumHealthyPercent)) == intent.MinimumHealthyPercent && int(aws.ToInt32(config.MaximumPercent)) == intent.MaximumPercent
	if matches {
		return false, "", nil
	}
	snapshot := applyrequest.ECSServiceSnapshot{TaskDefinitionARN: aws.ToString(observed.TaskDefinition), DesiredCount: observed.DesiredCount, MinimumHealthyPercent: aws.ToInt32(config.MinimumHealthyPercent), MaximumPercent: aws.ToInt32(config.MaximumPercent)}
	content, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(content)
	return true, hex.EncodeToString(sum[:]), nil
}
