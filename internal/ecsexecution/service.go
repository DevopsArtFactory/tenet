package ecsexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type ServiceClient interface {
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	UpdateService(context.Context, *ecs.UpdateServiceInput, ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
}

// RollingService updates only the reviewed task revision, count and rolling bounds.
// Prepare supplies the pre-update state for the executor's durable checkpoint.
type RollingService struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (ServiceClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

func (h RollingService) Preflight(ctx context.Context, request applyrequest.ECSOperationRequest) error {
	_, _, _, err := h.observe(ctx, request)
	return err
}

func (h RollingService) observe(ctx context.Context, request applyrequest.ECSOperationRequest) (ServiceClient, *types.Service, *types.TaskDefinition, error) {
	e := request.Intent
	serviceARN, err := arn.Parse(e.ServiceARN)
	clusterARN, clusterErr := arn.Parse(e.ClusterARN)
	roleARN, roleErr := arn.Parse(request.Role.RoleARN)
	if h.ClientFor == nil || h.CheckCapabilities == nil || err != nil || clusterErr != nil || roleErr != nil || serviceARN.Service != "ecs" || clusterARN.Service != "ecs" || serviceARN.AccountID != e.AWSAccountID || clusterARN.AccountID != e.AWSAccountID || serviceARN.Region != e.Region || clusterARN.Region != e.Region || serviceARN.Partition != clusterARN.Partition || !strings.HasPrefix(serviceARN.Resource, "service/") || !strings.HasPrefix(clusterARN.Resource, "cluster/") || roleARN.Service != "iam" || !strings.HasPrefix(roleARN.Resource, "role/") || roleARN.AccountID != e.AWSAccountID || roleARN.Partition != serviceARN.Partition || !request.Role.Configured || !request.Role.Ready || !request.Role.CheckedWithAWS || request.Role.AWSAccountID != e.AWSAccountID || request.Role.Region != e.Region || e.DeploymentController != "ECS" || e.DeploymentStrategy != "ROLLING" {
		return nil, nil, nil, fmt.Errorf("Rolling update requires reviewed service/cluster placement and verified deployer role")
	}
	client, err := h.ClientFor(ctx, request.Role)
	if err != nil {
		return nil, nil, nil, err
	}
	if client == nil {
		return nil, nil, nil, fmt.Errorf("service update client is unavailable")
	}
	output, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(e.ClusterARN), Services: []string{e.ServiceARN}})
	if err != nil {
		return nil, nil, nil, err
	}
	if output == nil || len(output.Failures) > 0 || len(output.Services) != 1 {
		return nil, nil, nil, fmt.Errorf("reviewed ECS service was not found or could not be described")
	}
	service := &output.Services[0]
	if aws.ToString(service.ServiceArn) != e.ServiceARN || aws.ToString(service.ClusterArn) != e.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.SchedulingStrategy != types.SchedulingStrategyReplica || service.DeploymentConfiguration == nil || (service.DeploymentConfiguration.Strategy != "" && service.DeploymentConfiguration.Strategy != types.DeploymentStrategyRolling) {
		return nil, nil, nil, fmt.Errorf("existing service must be ACTIVE with the ECS native Rolling REPLICA controller")
	}
	currentARN, currentErr := arn.Parse(aws.ToString(service.TaskDefinition))
	if currentErr != nil || currentARN.Service != "ecs" || currentARN.AccountID != e.AWSAccountID || currentARN.Region != e.Region || currentARN.Partition != serviceARN.Partition || !strings.HasPrefix(currentARN.Resource, "task-definition/") {
		return nil, nil, nil, fmt.Errorf("current service revision is unavailable for rollback capture")
	}
	currentFamily, _, _ := strings.Cut(strings.TrimPrefix(currentARN.Resource, "task-definition/"), ":")
	if !sameFamily(aws.ToString(service.TaskDefinition), request, currentFamily) {
		return nil, nil, nil, fmt.Errorf("current service task definition is not a pinned revision")
	}
	target := e.BaseTaskDefinitionARN
	if len(request.PriorResults) > 0 {
		result := request.PriorResults[0]
		base, _ := arn.Parse(e.BaseTaskDefinitionARN)
		family, _, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
		if result.TaskDefinitionIntent == "" || !sameFamily(result.TaskDefinitionARN, request, family) {
			return nil, nil, nil, fmt.Errorf("reviewed registration result is required before updating the service")
		}
		target = result.TaskDefinitionARN
	}
	targetOutput, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(target)})
	if err != nil {
		return nil, nil, nil, err
	}
	if targetOutput == nil || targetOutput.TaskDefinition == nil || aws.ToString(targetOutput.TaskDefinition.TaskDefinitionArn) != target || (len(request.PriorResults) > 0 && targetOutput.TaskDefinition.Status != types.TaskDefinitionStatusActive) {
		return nil, nil, nil, fmt.Errorf("reviewed task-definition revision is unavailable")
	}
	definition := targetOutput.TaskDefinition
	if len(request.PriorResults) > 0 {
		found := false
		for _, container := range definition.ContainerDefinitions {
			if aws.ToString(container.Name) == e.ContainerName && aws.ToString(container.Image) == e.Image {
				found = true
			}
		}
		if !found {
			return nil, nil, nil, fmt.Errorf("registered revision does not contain the reviewed image")
		}
	}
	if err := compatibleServiceTask(service, definition); err != nil {
		return nil, nil, nil, err
	}
	matchesImage := false
	for _, container := range definition.ContainerDefinitions {
		if aws.ToString(container.Name) == e.ContainerName && aws.ToString(container.Image) == e.Image {
			matchesImage = true
		}
	}
	targetIsFinal := len(request.PriorResults) > 0 || (matchesImage && definition.Status == types.TaskDefinitionStatusActive)
	if !targetIsFinal || !serviceMatches(service, target, request) {
		if service.PendingCount > 0 {
			return nil, nil, nil, fmt.Errorf("service has pending work from a conflicting rollout")
		}
		active := 0
		for _, deployment := range service.Deployments {
			if deployment.RunningCount > 0 || deployment.PendingCount > 0 {
				active++
			}
		}
		if active > 1 {
			return nil, nil, nil, fmt.Errorf("service has multiple active deployment revisions")
		}
		for _, deployment := range service.Deployments {
			if deployment.RolloutState == types.DeploymentRolloutStateInProgress {
				return nil, nil, nil, fmt.Errorf("service has a conflicting active rollout")
			}
		}
	}
	checks := []applyrequest.OperationExecutionCapabilityCheck{
		{Action: "ecs:DescribeServices", ResourceType: "ecsService", ResourceName: e.ServiceARN, RequiredRole: request.Role.RoleARN},
		{Action: "ecs:UpdateService", ResourceType: "ecsService", ResourceName: e.ServiceARN, RequiredRole: request.Role.RoleARN},
		{Action: "ecs:DescribeTaskDefinition", ResourceType: "taskDefinition", ResourceName: target, RequiredRole: request.Role.RoleARN},
	}
	roleChecks, err := passRoleChecks(definition.TaskRoleArn, definition.ExecutionRoleArn, request)
	if err != nil {
		return nil, nil, nil, err
	}
	checks = append(checks, roleChecks...)
	for i := range checks {
		if strings.HasPrefix(checks[i].Action, "ecs:") && checks[i].Action != "ecs:DescribeTaskDefinition" {
			checks[i].Context = map[string]string{"ecs:cluster": e.ClusterARN}
		}
	}
	if err := h.CheckCapabilities(ctx, request.Role, checks); err != nil {
		return nil, nil, nil, err
	}
	return client, service, definition, nil
}

func compatibleServiceTask(service *types.Service, definition *types.TaskDefinition) error {
	if definition.NetworkMode == types.NetworkModeAwsvpc && (service.NetworkConfiguration == nil || service.NetworkConfiguration.AwsvpcConfiguration == nil || len(service.NetworkConfiguration.AwsvpcConfiguration.Subnets) == 0) {
		return fmt.Errorf("awsvpc task requires existing reviewed service network configuration")
	}
	if service.NetworkConfiguration != nil && service.NetworkConfiguration.AwsvpcConfiguration != nil && definition.NetworkMode != types.NetworkModeAwsvpc {
		return fmt.Errorf("task network mode conflicts with existing awsvpc service configuration")
	}
	if family := aws.ToString(service.PlatformFamily); family != "" {
		requested := "LINUX"
		if definition.RuntimePlatform != nil && definition.RuntimePlatform.OperatingSystemFamily != "" {
			requested = string(definition.RuntimePlatform.OperatingSystemFamily)
		}
		if strings.ToUpper(strings.ReplaceAll(family, " ", "_")) != requested {
			return fmt.Errorf("task runtime OS does not match existing service platform family")
		}
	}
	fargate := service.LaunchType == types.LaunchTypeFargate
	for _, provider := range service.CapacityProviderStrategy {
		if aws.ToString(provider.CapacityProvider) == "FARGATE" || aws.ToString(provider.CapacityProvider) == "FARGATE_SPOT" {
			fargate = true
		}
	}
	if fargate {
		compatible := false
		for _, value := range definition.RequiresCompatibilities {
			if value == types.CompatibilityFargate {
				compatible = true
			}
		}
		if !compatible {
			return fmt.Errorf("task definition is incompatible with the existing Fargate service")
		}
	}
	for _, binding := range service.LoadBalancers {
		matched := false
		for _, container := range definition.ContainerDefinitions {
			if aws.ToString(container.Name) != aws.ToString(binding.ContainerName) {
				continue
			}
			for _, port := range container.PortMappings {
				if aws.ToInt32(port.ContainerPort) == aws.ToInt32(binding.ContainerPort) {
					matched = true
				}
			}
		}
		if !matched {
			return fmt.Errorf("task definition does not preserve an existing load-balancer container/port binding")
		}
	}
	if primaryDeployment(service) == "" {
		return fmt.Errorf("service has no identifiable primary deployment")
	}
	return nil
}

func primaryDeployment(service *types.Service) string {
	for _, deployment := range service.Deployments {
		if aws.ToString(deployment.Status) == "PRIMARY" {
			return aws.ToString(deployment.Id)
		}
	}
	return ""
}

func serviceSnapshot(service *types.Service) *applyrequest.ECSServiceSnapshot {
	if service == nil || service.DeploymentConfiguration == nil {
		return nil
	}
	return &applyrequest.ECSServiceSnapshot{TaskDefinitionARN: aws.ToString(service.TaskDefinition), DesiredCount: service.DesiredCount, MinimumHealthyPercent: aws.ToInt32(service.DeploymentConfiguration.MinimumHealthyPercent), MaximumPercent: aws.ToInt32(service.DeploymentConfiguration.MaximumPercent), PrimaryDeploymentID: primaryDeployment(service)}
}

func serviceMatches(service *types.Service, target string, request applyrequest.ECSOperationRequest) bool {
	if service == nil || service.DeploymentConfiguration == nil {
		return false
	}
	return aws.ToString(service.TaskDefinition) == target && service.DesiredCount == int32(request.Intent.DesiredCount) && aws.ToInt32(service.DeploymentConfiguration.MinimumHealthyPercent) == int32(request.Intent.MinimumHealthyPercent) && aws.ToInt32(service.DeploymentConfiguration.MaximumPercent) == int32(request.Intent.MaximumPercent)
}

func (h RollingService) Prepare(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	_, service, _, err := h.observe(ctx, request)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if len(request.PriorResults) == 0 {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("registration checkpoint is required before preparing an ECS update")
	}
	result := request.PreparedResult.Clone()
	if result.PreviousService == nil {
		result.PreviousService = serviceSnapshot(service)
	} else if !reflect.DeepEqual(result.PreviousService, serviceSnapshot(service)) && !serviceMatches(service, request.PriorResults[0].TaskDefinitionARN, request) {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("service changed outside the reviewed execution after its pre-update snapshot")
	}
	result.PreviousTaskDefinitionARN = result.PreviousService.TaskDefinitionARN
	result.TaskDefinitionARN = request.PriorResults[0].TaskDefinitionARN
	result.TaskDefinitionRevision = request.PriorResults[0].TaskDefinitionRevision
	result.TaskDefinitionIntent = request.PriorResults[0].TaskDefinitionIntent
	return result, nil
}

func (h RollingService) Execute(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if request.PreparedResult.PreviousService == nil || len(request.PriorResults) == 0 {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSCallError{Err: fmt.Errorf("durable pre-update service snapshot and registration result are required"), Rejected: true}
	}
	client, service, _, err := h.observe(ctx, request)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	target := request.PriorResults[0].TaskDefinitionARN
	if serviceMatches(service, target, request) {
		result := request.PreparedResult.Clone()
		result.ServiceDeploymentID = deploymentForTarget(service, target)
		result.ServiceDeploymentCreatedAt = deploymentCreatedForTarget(service, target)
		return result, nil
	}
	if !reflect.DeepEqual(request.PreparedResult.PreviousService, serviceSnapshot(service)) {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSCallError{Err: fmt.Errorf("service changed after its durable pre-update snapshot"), Rejected: true}
	}
	content, err := json.Marshal(service.DeploymentConfiguration)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	var config types.DeploymentConfiguration
	if err := json.Unmarshal(content, &config); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	config.MinimumHealthyPercent = aws.Int32(int32(request.Intent.MinimumHealthyPercent))
	config.MaximumPercent = aws.Int32(int32(request.Intent.MaximumPercent))
	if err := authorizeMutation(ctx, request); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	output, err := client.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: aws.String(request.Intent.ClusterARN), Service: aws.String(request.Intent.ServiceARN), TaskDefinition: aws.String(target), DesiredCount: aws.Int32(int32(request.Intent.DesiredCount)), DeploymentConfiguration: &config}, func(options *ecs.Options) { options.Retryer = aws.NopRetryer{}; options.RetryMaxAttempts = 1 })
	if err != nil {
		return applyrequest.ECSOperationResult{}, ecsMutationError(err)
	}
	if output == nil || output.Service == nil || aws.ToString(output.Service.ServiceArn) != request.Intent.ServiceARN || aws.ToString(output.Service.ClusterArn) != request.Intent.ClusterARN || !serviceMatches(output.Service, target, request) {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("ECS update response does not confirm reviewed acceptance; reconcile before retry")
	}
	result := request.PreparedResult.Clone()
	result.TaskDefinitionARN = target
	result.ServiceDeploymentID = deploymentForTarget(output.Service, target)
	result.ServiceDeploymentCreatedAt = deploymentCreatedForTarget(output.Service, target)
	result.AWSRequestID, _ = awsmiddleware.GetRequestIDMetadata(output.ResultMetadata)
	return result, nil
}

func (h RollingService) Reconcile(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	_, service, _, err := h.observe(ctx, request)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if request.PreparedResult.PreviousService == nil || len(request.PriorResults) == 0 {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("cannot reconcile update without its durable pre-update snapshot")
	}
	if serviceMatches(service, request.PriorResults[0].TaskDefinitionARN, request) {
		result := request.PreparedResult.Clone()
		result.ServiceDeploymentID = deploymentForTarget(service, request.PriorResults[0].TaskDefinitionARN)
		result.ServiceDeploymentCreatedAt = deploymentCreatedForTarget(service, request.PriorResults[0].TaskDefinitionARN)
		return applyrequest.ECSReconciliation{Complete: true, Result: result}, nil
	}
	for _, deployment := range service.Deployments {
		if aws.ToString(deployment.TaskDefinition) == request.PriorResults[0].TaskDefinitionARN && deployment.RolloutState == types.DeploymentRolloutStateFailed {
			return applyrequest.ECSReconciliation{Failure: "ECS deployment " + aws.ToString(deployment.Id) + " failed: " + aws.ToString(deployment.RolloutStateReason)}, nil
		}
	}
	// A stale service response cannot prove UpdateService was never accepted.
	return applyrequest.ECSReconciliation{}, nil
}

func deploymentForTarget(service *types.Service, target string) string {
	for _, deployment := range service.Deployments {
		if aws.ToString(deployment.Status) == "PRIMARY" && aws.ToString(deployment.TaskDefinition) == target {
			return aws.ToString(deployment.Id)
		}
	}
	return ""
}

func deploymentCreatedForTarget(service *types.Service, target string) string {
	for _, deployment := range service.Deployments {
		if aws.ToString(deployment.Status) == "PRIMARY" && aws.ToString(deployment.TaskDefinition) == target && deployment.CreatedAt != nil {
			return deployment.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return ""
}
