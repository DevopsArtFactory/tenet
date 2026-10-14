package ecsexecution

import (
	"context"
	"fmt"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// RollbackTarget reads the immutable previous revision; it never registers a replacement.
func (h RollingService) RollbackTarget(ctx context.Context, parent applyrequest.Record, role applyrequest.DeployerRoleReadiness) (*domain.ECSDeploymentIntent, error) {
	if h.ClientFor == nil || h.CheckCapabilities == nil || len(parent.Operations) != 3 || parent.Operations[0].ECS == nil || parent.Operations[1].ECSCheckpoint == nil || parent.Operations[1].ECSCheckpoint.Result.PreviousService == nil {
		return nil, applyrequest.ErrRecoveryUnavailable
	}
	snapshot := parent.Operations[1].ECSCheckpoint.Result.PreviousService
	target := parent.Operations[0].ECS.Clone()
	parsed, err := arn.Parse(snapshot.TaskDefinitionARN)
	placement, placementErr := arn.Parse(target.ServiceARN)
	deployer, roleErr := arn.Parse(role.RoleARN)
	if err != nil || placementErr != nil || roleErr != nil || parsed.Service != "ecs" || parsed.AccountID != target.AWSAccountID || parsed.Region != target.Region || parsed.Partition != placement.Partition || deployer.Service != "iam" || !strings.HasPrefix(deployer.Resource, "role/") || deployer.AccountID != target.AWSAccountID || deployer.Partition != placement.Partition || role.AWSAccountID != target.AWSAccountID || role.Region != target.Region || !role.Ready || !role.Configured || !role.CheckedWithAWS {
		return nil, fmt.Errorf("rollback requires verified original placement and deployer role")
	}
	if err := h.CheckCapabilities(ctx, role, []applyrequest.OperationExecutionCapabilityCheck{{Action: "ecs:DescribeTaskDefinition", ResourceName: snapshot.TaskDefinitionARN, RequiredRole: role.RoleARN}}); err != nil {
		return nil, err
	}
	client, err := h.ClientFor(ctx, role)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, applyrequest.ErrRecoveryUnavailable
	}
	output, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(snapshot.TaskDefinitionARN)})
	if err != nil {
		return nil, err
	}
	if output == nil || output.TaskDefinition == nil || aws.ToString(output.TaskDefinition.TaskDefinitionArn) != snapshot.TaskDefinitionARN || output.TaskDefinition.Status != types.TaskDefinitionStatusActive {
		return nil, fmt.Errorf("rollback previous task definition is missing or inactive")
	}
	image := ""
	for _, container := range output.TaskDefinition.ContainerDefinitions {
		if aws.ToString(container.Name) == target.ContainerName {
			if image != "" {
				return nil, fmt.Errorf("ambiguous rollback container")
			}
			image = aws.ToString(container.Image)
		}
	}
	if image == "" {
		return nil, fmt.Errorf("rollback previous task definition lacks the reviewed container")
	}
	target.BaseTaskDefinitionARN = snapshot.TaskDefinitionARN
	target.Image = image
	target.DesiredCount = int(snapshot.DesiredCount)
	target.MinimumHealthyPercent = int(snapshot.MinimumHealthyPercent)
	target.MaximumPercent = int(snapshot.MaximumPercent)
	target.RollbackOnFailure = false
	return target, nil
}
