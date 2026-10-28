package ecsexecution

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type NativeDeploymentClient interface {
	ListServiceDeployments(context.Context, *ecs.ListServiceDeploymentsInput, ...func(*ecs.Options)) (*ecs.ListServiceDeploymentsOutput, error)
	DescribeServiceDeployments(context.Context, *ecs.DescribeServiceDeploymentsInput, ...func(*ecs.Options)) (*ecs.DescribeServiceDeploymentsOutput, error)
	DescribeServiceRevisions(context.Context, *ecs.DescribeServiceRevisionsInput, ...func(*ecs.Options)) (*ecs.DescribeServiceRevisionsOutput, error)
}

type NativeServiceClient interface {
	ServiceClient
	NativeDeploymentClient
}

// AdvancedService requests one ECS-owned traffic deployment. It never writes to
// ALB or CodeDeploy. Production wiring remains gated until observation/recovery
// are implemented; this handler can be exercised with injected SDK clients.
type AdvancedService struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (NativeServiceClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
	ObserveRouting    func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error)
}
type nativeServiceObservation struct {
	client      NativeServiceClient
	service     *types.Service
	target      *types.TaskDefinition
	deployments []types.ServiceDeploymentBrief
	routing     ALBRoutingSnapshot
	noOp        *types.ServiceDeployment
	sources     []applyrequest.ECSNativeSourceRevision
}

func (h AdvancedService) Preflight(ctx context.Context, r applyrequest.ECSOperationRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := h.observe(ctx, r)
	return err
}
func (h AdvancedService) observe(ctx context.Context, r applyrequest.ECSOperationRequest) (nativeServiceObservation, error) {
	var empty nativeServiceObservation
	if err := verifiedNativeRole(r.Intent, r.Role); err != nil {
		return empty, err
	}
	if h.ClientFor == nil || h.CheckCapabilities == nil || h.ObserveRouting == nil || r.Recovery != nil {
		return empty, fmt.Errorf("native service/deployer/routing providers and a forward advanced request are required")
	}
	if r.ManagedRepair && (!r.Intent.SelfHeal || r.Intent.SyncMode != "automatic" || r.NativeRollback != nil) {
		return empty, fmt.Errorf("managed native repair requires explicit automatic self-heal intent")
	}
	if r.RetryDeploymentID != "" && !nativeScopedARN(r.RetryDeploymentID, "service-deployment", r.Intent) {
		return empty, fmt.Errorf("native retry requires a concrete failed service-deployment ARN")
	}
	client, err := h.ClientFor(ctx, r.Role)
	if err != nil {
		return empty, err
	}
	if client == nil {
		return empty, fmt.Errorf("native service client unavailable")
	}
	output, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(r.Intent.ClusterARN), Services: []string{r.Intent.ServiceARN}})
	if err != nil {
		return empty, err
	}
	if output == nil || len(output.Failures) > 0 || len(output.Services) != 1 {
		return empty, fmt.Errorf("reviewed native service unavailable")
	}
	service := &output.Services[0]
	e := r.Intent
	if aws.ToString(service.ServiceArn) != e.ServiceARN || aws.ToString(service.ClusterArn) != e.ClusterARN || aws.ToString(service.Status) != "ACTIVE" || service.SchedulingStrategy != types.SchedulingStrategyReplica || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.DeploymentConfiguration == nil || string(service.DeploymentConfiguration.Strategy) != e.DeploymentStrategy {
		return empty, fmt.Errorf("existing service must already use the reviewed ECS native advanced strategy")
	}
	if service.DesiredCount != int32(e.DesiredCount) && !r.ManagedRepair {
		return empty, fmt.Errorf("initial native advanced execution does not resize service capacity")
	}
	routing := e.Advanced.Routing
	if !nativeBindingMatches(service, e) {
		return empty, fmt.Errorf("existing service routing must match the reviewed native ALB-only binding, role and target pair")
	}
	base, _ := arn.Parse(e.BaseTaskDefinitionARN)
	family, _, _ := strings.Cut(strings.TrimPrefix(base.Resource, "task-definition/"), ":")
	if !sameFamily(aws.ToString(service.TaskDefinition), r, family) {
		return empty, fmt.Errorf("native source revision must belong to the reviewed task-definition family")
	}
	target := e.BaseTaskDefinitionARN
	if len(r.PriorResults) > 0 {
		registration := r.PriorResults[0]
		if registration.TaskDefinitionIntent == "" || !sameFamily(registration.TaskDefinitionARN, r, family) {
			return empty, fmt.Errorf("reviewed registration result required")
		}
		target = registration.TaskDefinitionARN
	}
	definition, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(target)})
	if err != nil {
		return empty, err
	}
	if definition == nil || definition.TaskDefinition == nil || aws.ToString(definition.TaskDefinition.TaskDefinitionArn) != target || definition.TaskDefinition.NetworkMode != types.NetworkModeAwsvpc {
		return empty, fmt.Errorf("reviewed native ALB target requires an awsvpc task definition")
	}
	task := definition.TaskDefinition
	found := 0
	for _, container := range task.ContainerDefinitions {
		if aws.ToString(container.Name) == e.ContainerName && aws.ToString(container.Image) == e.Image {
			found++
		}
	}
	targetIsFinal := task.Status == types.TaskDefinitionStatusActive && found == 1
	if len(r.PriorResults) > 0 && !targetIsFinal {
		return empty, fmt.Errorf("registered native revision must be ACTIVE with the reviewed image")
	}
	if err := compatibleServiceTask(service, task); err != nil {
		return empty, err
	}
	if _, err := normalizedAdvancedConfig(service.DeploymentConfiguration); err != nil {
		return empty, err
	}
	checks := nativeReadChecks(r)
	checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeServices", ResourceName: e.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": e.ClusterARN}})
	checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:UpdateService", ResourceType: "ecsService", ResourceName: e.ServiceARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"ecs:cluster": e.ClusterARN}}, applyrequest.OperationExecutionCapabilityCheck{Action: "ecs:DescribeTaskDefinition", ResourceName: target, RequiredRole: r.Role.RoleARN})
	roles, err := passRoleChecks(task.TaskRoleArn, task.ExecutionRoleArn, r)
	if err != nil {
		return empty, err
	}
	checks = append(checks, roles...)
	passed := map[string]bool{}
	for _, roleARN := range append([]string{routing.InfrastructureRoleARN}, nativeHookRoles(e)...) {
		if passed[roleARN] {
			continue
		}
		passed[roleARN] = true
		checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "iam:PassRole", ResourceType: "iamRole", ResourceName: roleARN, RequiredRole: r.Role.RoleARN, Context: map[string]string{"iam:PassedToService": "ecs.amazonaws.com"}})
	}
	if err := h.CheckCapabilities(ctx, r.Role, checks); err != nil {
		return empty, err
	}
	observedRouting, err := h.ObserveRouting(ctx, e, r.Role)
	if err != nil {
		return empty, err
	}
	if observedRouting.Signature == "" {
		return empty, fmt.Errorf("current ALB routing fence unavailable")
	}
	deployments, err := listNativeDeployments(ctx, client, r)
	if err != nil {
		return empty, err
	}
	observation := nativeServiceObservation{client: client, service: service, target: task, deployments: deployments, routing: observedRouting}
	var active []types.ServiceDeploymentBrief
	var latest *types.ServiceDeploymentBrief
	for i := range deployments {
		deployment := deployments[i]
		if !nativeTerminal(deployment.Status) {
			active = append(active, deployment)
		}
		if deployment.CreatedAt == nil {
			return empty, fmt.Errorf("native deployment creation evidence unavailable")
		}
		if latest == nil || deployment.CreatedAt.After(*latest.CreatedAt) {
			copy := deployment
			latest = &copy
		} else if deployment.CreatedAt.Equal(*latest.CreatedAt) && aws.ToString(deployment.ServiceDeploymentArn) != aws.ToString(latest.ServiceDeploymentArn) {
			return empty, fmt.Errorf("native deployment ordering is ambiguous")
		}
	}
	if latest == nil {
		return empty, fmt.Errorf("native deployment history is required to review source revisions")
	}
	latestDeployment, err := describeNativeDeployment(ctx, client, aws.ToString(latest.ServiceDeploymentArn), r)
	if err != nil {
		return empty, err
	}
	references := append([]types.ServiceRevisionSummary{*latestDeployment.TargetServiceRevision}, latestDeployment.SourceServiceRevisions...)
	seenSources := map[string]bool{}
	for _, reference := range references {
		value := aws.ToString(reference.Arn)
		if seenSources[value] {
			continue
		}
		seenSources[value] = true
		revision, err := describeNativeRevision(ctx, client, value, r)
		if err != nil {
			return empty, err
		}
		if (revision.ServiceConnectConfiguration != nil && revision.ServiceConnectConfiguration.Enabled) || len(revision.VpcLatticeConfigurations) > 0 {
			return empty, fmt.Errorf("native revision uses unsupported mixed routing")
		}
		if !sameFamily(aws.ToString(revision.TaskDefinition), r, family) {
			return empty, fmt.Errorf("native source history must retain the reviewed task-definition family")
		}
		observation.sources = append(observation.sources, applyrequest.ECSNativeSourceRevision{ARN: value, TaskDefinitionARN: aws.ToString(revision.TaskDefinition)})
	}
	// A matching current native deployment can be adopted for read-only verification.
	if targetIsFinal && aws.ToString(service.TaskDefinition) == target && advancedConfigMatches(service.DeploymentConfiguration, e) {
		candidate := latest
		if len(active) == 1 {
			candidate = &active[0]
		}
		if candidate != nil && (candidate.Status == types.ServiceDeploymentStatusSuccessful || candidate.Status == types.ServiceDeploymentStatusInProgress || candidate.Status == types.ServiceDeploymentStatusPending) {
			described, err := describeNativeDeployment(ctx, client, aws.ToString(candidate.ServiceDeploymentArn), r)
			if err != nil {
				return empty, err
			}
			if described.TargetServiceRevision != nil && advancedConfigMatches(described.DeploymentConfiguration, e) {
				revision, err := describeNativeRevision(ctx, client, aws.ToString(described.TargetServiceRevision.Arn), r)
				if err != nil {
					return empty, err
				}
				canAdopt := service.DesiredCount == int32(e.DesiredCount)
				if r.ManagedRepair && described.Status == types.ServiceDeploymentStatusSuccessful {
					group, err := nativeRevisionTargetGroup(revision, e)
					if err != nil {
						return empty, err
					}
					percent, err := configuredNativePercent(observedRouting, group, e)
					if err != nil {
						return empty, err
					}
					canAdopt = canAdopt && percent == 100
				}
				if aws.ToString(revision.TaskDefinition) == target && canAdopt {
					observation.noOp = described
				}
			}
		}
	}
	if len(active) > 0 && (len(active) != 1 || observation.noOp == nil || aws.ToString(active[0].ServiceDeploymentArn) != aws.ToString(observation.noOp.ServiceDeploymentArn)) {
		return empty, fmt.Errorf("another native service deployment is active; observe it before updating")
	}
	if latest != nil && (latest.Status == types.ServiceDeploymentStatusStopped || latest.Status == types.ServiceDeploymentStatusRollbackFailed) && aws.ToString(service.TaskDefinition) == target && observation.noOp == nil && r.RetryDeploymentID != aws.ToString(latest.ServiceDeploymentArn) {
		return empty, fmt.Errorf("failed native primary requires an explicit deployment-correlated retry")
	}
	return observation, nil
}
func (h AdvancedService) Prepare(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if len(r.PriorResults) != 1 {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("native registration checkpoint required")
	}
	observation, err := h.observe(ctx, r)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	configHash, err := nativeConfigurationHash(observation.service.DeploymentConfiguration)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	result := r.PreparedResult.Clone()
	if result.PreviousService != nil {
		if !reflect.DeepEqual(result.PreviousService, serviceSnapshot(observation.service)) || result.NativeService == nil || result.NativeService.ConfigurationHash != configHash || result.NativeService.RoutingSignature != observation.routing.Signature {
			return applyrequest.ECSOperationResult{}, fmt.Errorf("native service/routing changed after its durable pre-update snapshot")
		}
		return result, nil
	}
	result.PreviousService = serviceSnapshot(observation.service)
	result.PreviousTaskDefinitionARN = result.PreviousService.TaskDefinitionARN
	result.TaskDefinitionARN = r.PriorResults[0].TaskDefinitionARN
	result.TaskDefinitionRevision = r.PriorResults[0].TaskDefinitionRevision
	result.TaskDefinitionIntent = r.PriorResults[0].TaskDefinitionIntent
	result.NativeService = &applyrequest.ECSNativeServiceSnapshot{ConfigurationHash: configHash, RoutingSignature: observation.routing.Signature, KnownDeploymentARNs: []string{}, SourceRevisions: append([]applyrequest.ECSNativeSourceRevision(nil), observation.sources...)}
	for _, deployment := range observation.deployments {
		result.NativeService.KnownDeploymentARNs = append(result.NativeService.KnownDeploymentARNs, aws.ToString(deployment.ServiceDeploymentArn))
	}
	slices.Sort(result.NativeService.KnownDeploymentARNs)
	if observation.noOp != nil {
		result.NativeService.NoOpDeploymentARN = aws.ToString(observation.noOp.ServiceDeploymentArn)
		result.NativeService.NoOpRevisionARN = aws.ToString(observation.noOp.TargetServiceRevision.Arn)
	}
	return result, nil
}
func (h AdvancedService) Execute(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	prepared := r.PreparedResult
	if prepared.PreviousService == nil || prepared.NativeService == nil || len(prepared.NativeService.SourceRevisions) == 0 || len(prepared.NativeService.KnownDeploymentARNs) == 0 || len(r.PriorResults) != 1 {
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("durable native snapshot and registration are required"))
	}
	observation, err := h.observe(ctx, r)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if observation.noOp != nil && (prepared.NativeService.NoOpDeploymentARN == aws.ToString(observation.noOp.ServiceDeploymentArn) || r.ManagedRepair) {
		result := prepared.Clone()
		result.NativeService.NoOpDeploymentARN = aws.ToString(observation.noOp.ServiceDeploymentArn)
		result.NativeService.NoOpRevisionARN = aws.ToString(observation.noOp.TargetServiceRevision.Arn)
		result.ServiceDeploymentARN = aws.ToString(observation.noOp.ServiceDeploymentArn)
		result.ServiceRevisionARN = aws.ToString(observation.noOp.TargetServiceRevision.Arn)
		return result, nil
	}
	hash, err := nativeConfigurationHash(observation.service.DeploymentConfiguration)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if !reflect.DeepEqual(prepared.PreviousService, serviceSnapshot(observation.service)) || prepared.NativeService.ConfigurationHash != hash || prepared.NativeService.RoutingSignature != observation.routing.Signature {
		return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("native service/routing changed before mutation"))
	}
	for _, deployment := range observation.deployments {
		if !slices.Contains(prepared.NativeService.KnownDeploymentARNs, aws.ToString(deployment.ServiceDeploymentArn)) {
			return applyrequest.ECSOperationResult{}, beforeMutationError(fmt.Errorf("intervening native deployment appeared before mutation"))
		}
	}
	config, err := advancedAWSConfig(r.Intent)
	if err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	if err := authorizeMutation(ctx, r); err != nil {
		return applyrequest.ECSOperationResult{}, beforeMutationError(err)
	}
	// Config-only changes must start a native deployment as well; otherwise ECS can
	// accept settings without producing the lifecycle identity we need to observe.
	force := aws.ToString(observation.service.TaskDefinition) == prepared.TaskDefinitionARN
	input := &ecs.UpdateServiceInput{Cluster: aws.String(r.Intent.ClusterARN), Service: aws.String(r.Intent.ServiceARN), TaskDefinition: aws.String(prepared.TaskDefinitionARN), DeploymentConfiguration: config, ForceNewDeployment: force}
	if r.ManagedRepair && observation.service.DesiredCount != int32(r.Intent.DesiredCount) {
		input.DesiredCount = aws.Int32(int32(r.Intent.DesiredCount))
	}
	output, err := observation.client.UpdateService(ctx, input, func(options *ecs.Options) { options.Retryer = aws.NopRetryer{}; options.RetryMaxAttempts = 1 })
	if err != nil {
		return applyrequest.ECSOperationResult{}, ecsMutationError(err)
	}
	reconciliation, err := h.reconcileWithClient(ctx, observation.client, r)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if !reconciliation.Complete {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("native update acceptance has no correlated deployment identity yet; reconcile without repeating")
	}
	result := reconciliation.Result
	if output != nil {
		result.AWSRequestID, _ = awsmiddleware.GetRequestIDMetadata(output.ResultMetadata)
	}
	return result, nil
}
func (h AdvancedService) Reconcile(ctx context.Context, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := verifiedNativeRole(r.Intent, r.Role); err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if h.ClientFor == nil || h.CheckCapabilities == nil {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("native read providers required")
	}
	if err := h.CheckCapabilities(ctx, r.Role, nativeReadChecks(r)); err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	client, err := h.ClientFor(ctx, r.Role)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if client == nil {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("native read client unavailable")
	}
	return h.reconcileWithClient(ctx, client, r)
}
func (h AdvancedService) reconcileWithClient(ctx context.Context, client NativeDeploymentClient, r applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	prepared := r.PreparedResult
	if prepared.NativeService == nil || prepared.PreviousService == nil || prepared.TaskDefinitionARN == "" {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("native reconciliation requires durable pre-update evidence")
	}
	if prepared.NativeService.NoOpDeploymentARN != "" {
		deployment, err := describeNativeDeployment(ctx, client, prepared.NativeService.NoOpDeploymentARN, r)
		if err != nil {
			return applyrequest.ECSReconciliation{}, err
		}
		if deployment.TargetServiceRevision == nil || aws.ToString(deployment.TargetServiceRevision.Arn) != prepared.NativeService.NoOpRevisionARN || !advancedConfigMatches(deployment.DeploymentConfiguration, r.Intent) {
			return applyrequest.ECSReconciliation{}, fmt.Errorf("adopted native deployment identity/configuration changed")
		}
		revision, err := describeNativeRevision(ctx, client, prepared.NativeService.NoOpRevisionARN, r)
		if err != nil {
			return applyrequest.ECSReconciliation{}, err
		}
		if aws.ToString(revision.TaskDefinition) != prepared.TaskDefinitionARN {
			return applyrequest.ECSReconciliation{}, fmt.Errorf("adopted native revision workload differs")
		}
		result := prepared.Clone()
		result.ServiceDeploymentARN = prepared.NativeService.NoOpDeploymentARN
		result.ServiceRevisionARN = prepared.NativeService.NoOpRevisionARN
		return applyrequest.ECSReconciliation{Complete: true, Result: result}, nil
	}
	deployments, err := listNativeDeployments(ctx, client, r)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	var matched *types.ServiceDeployment
	for _, brief := range deployments {
		value := aws.ToString(brief.ServiceDeploymentArn)
		if slices.Contains(prepared.NativeService.KnownDeploymentARNs, value) {
			continue
		}
		deployment, err := describeNativeDeployment(ctx, client, value, r)
		if err != nil {
			return applyrequest.ECSReconciliation{}, err
		}
		if deployment.TargetServiceRevision == nil || !advancedConfigMatches(deployment.DeploymentConfiguration, r.Intent) || len(deployment.SourceServiceRevisions) != 1 {
			continue
		}
		target, err := describeNativeRevision(ctx, client, aws.ToString(deployment.TargetServiceRevision.Arn), r)
		if err != nil {
			return applyrequest.ECSReconciliation{}, err
		}
		if aws.ToString(target.TaskDefinition) != prepared.TaskDefinitionARN {
			continue
		}
		sourceARN := aws.ToString(deployment.SourceServiceRevisions[0].Arn)
		var knownSource *applyrequest.ECSNativeSourceRevision
		for i := range prepared.NativeService.SourceRevisions {
			if prepared.NativeService.SourceRevisions[i].ARN == sourceARN {
				knownSource = &prepared.NativeService.SourceRevisions[i]
				break
			}
		}
		if knownSource == nil {
			continue
		}
		source, err := describeNativeRevision(ctx, client, sourceARN, r)
		if err != nil {
			return applyrequest.ECSReconciliation{}, err
		}
		if aws.ToString(source.TaskDefinition) != knownSource.TaskDefinitionARN {
			return applyrequest.ECSReconciliation{}, fmt.Errorf("native source revision workload changed")
		}

		if matched != nil {
			return applyrequest.ECSReconciliation{}, fmt.Errorf("multiple new native deployments match reviewed intent; do not guess or resend")
		}
		matched = deployment
	}
	if matched == nil {
		return applyrequest.ECSReconciliation{}, nil
	} // Absence is never replay authority.
	result := prepared.Clone()
	result.ServiceDeploymentARN = aws.ToString(matched.ServiceDeploymentArn)
	result.ServiceRevisionARN = aws.ToString(matched.TargetServiceRevision.Arn)
	result.PreviousServiceRevisionARN = aws.ToString(matched.SourceServiceRevisions[0].Arn)
	for _, source := range prepared.NativeService.SourceRevisions {
		if source.ARN == result.PreviousServiceRevisionARN {
			result.PreviousTaskDefinitionARN = source.TaskDefinitionARN
			break
		}
	}
	if matched.CreatedAt != nil {
		result.ServiceDeploymentCreatedAt = matched.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return applyrequest.ECSReconciliation{Complete: true, Result: result}, nil
}
func nativeHookRoles(intent domain.ECSDeploymentIntent) []string {
	var roles []string
	for _, hook := range intent.Advanced.LifecycleHooks {
		roles = append(roles, hook.RoleARN)
	}
	return roles
}
func nativeReadChecks(r applyrequest.ECSOperationRequest) []applyrequest.OperationExecutionCapabilityCheck {
	service, _ := arn.Parse(r.Intent.ServiceARN)
	name := strings.TrimPrefix(service.Resource, "service/")
	deployment := service
	deployment.Resource = "service-deployment/" + name + "/*"
	revision := service
	revision.Resource = "service-revision/" + name + "/*"
	clusterContext := map[string]string{"ecs:cluster": r.Intent.ClusterARN}
	nativeContext := map[string]string{"ecs:cluster": r.Intent.ClusterARN, "ecs:service": r.Intent.ServiceARN}
	return []applyrequest.OperationExecutionCapabilityCheck{
		{Action: "ecs:ListServiceDeployments", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: clusterContext},
		{Action: "ecs:DescribeServiceDeployments", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: clusterContext},
		{Action: "ecs:DescribeServiceDeployments", ResourceName: deployment.String(), RequiredRole: r.Role.RoleARN, Context: nativeContext},
		{Action: "ecs:DescribeServiceRevisions", ResourceName: r.Intent.ServiceARN, RequiredRole: r.Role.RoleARN, Context: clusterContext},
		{Action: "ecs:DescribeServiceRevisions", ResourceName: revision.String(), RequiredRole: r.Role.RoleARN, Context: nativeContext},
	}
}
func nativeScopedARN(value, resource string, intent domain.ECSDeploymentIntent) bool {
	service, serviceErr := arn.Parse(intent.ServiceARN)
	parsed, err := arn.Parse(value)
	if err != nil || serviceErr != nil || parsed.Service != "ecs" || parsed.Partition != service.Partition || parsed.Region != intent.Region || parsed.AccountID != intent.AWSAccountID {
		return false
	}
	prefix := resource + "/" + strings.TrimPrefix(service.Resource, "service/") + "/"
	suffix := strings.TrimPrefix(parsed.Resource, prefix)
	return strings.HasPrefix(parsed.Resource, prefix) && suffix != "" && !strings.ContainsAny(suffix, "/*?")
}
func nativeTerminal(status types.ServiceDeploymentStatus) bool {
	return status == types.ServiceDeploymentStatusSuccessful || status == types.ServiceDeploymentStatusStopped || status == types.ServiceDeploymentStatusRollbackSuccessful || status == types.ServiceDeploymentStatusRollbackFailed
}
func nativeKnownStatus(status types.ServiceDeploymentStatus) bool {
	return nativeTerminal(status) || status == types.ServiceDeploymentStatusPending || status == types.ServiceDeploymentStatusInProgress || status == types.ServiceDeploymentStatusStopRequested || status == types.ServiceDeploymentStatusRollbackRequested || status == types.ServiceDeploymentStatusRollbackInProgress
}
func listNativeDeployments(ctx context.Context, client NativeDeploymentClient, r applyrequest.ECSOperationRequest) ([]types.ServiceDeploymentBrief, error) {
	var deployments []types.ServiceDeploymentBrief
	var token *string
	seenTokens := map[string]bool{}
	seen := map[string]types.ServiceDeploymentBrief{}
	for pages := 0; pages < 10; pages++ {
		output, err := client.ListServiceDeployments(ctx, &ecs.ListServiceDeploymentsInput{Cluster: aws.String(r.Intent.ClusterARN), Service: aws.String(r.Intent.ServiceARN), MaxResults: aws.Int32(100), NextToken: token})
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, fmt.Errorf("native deployment listing unavailable")
		}
		for _, d := range output.ServiceDeployments {
			value := aws.ToString(d.ServiceDeploymentArn)
			if !nativeScopedARN(value, "service-deployment", r.Intent) || aws.ToString(d.ServiceArn) != r.Intent.ServiceARN || aws.ToString(d.ClusterArn) != r.Intent.ClusterARN || !nativeKnownStatus(d.Status) || !nativeScopedARN(aws.ToString(d.TargetServiceRevisionArn), "service-revision", r.Intent) {
				return nil, fmt.Errorf("native deployment list contains incomplete or unreviewed identities/status")
			}
			if previous, exists := seen[value]; exists {
				if !reflect.DeepEqual(previous, d) {
					return nil, fmt.Errorf("native deployment pagination returned inconsistent duplicate evidence")
				}
				continue
			}
			seen[value] = d
			deployments = append(deployments, d)
			if len(deployments) > 1000 {
				return nil, fmt.Errorf("native deployment history exceeds 1000 records")
			}
		}
		if aws.ToString(output.NextToken) == "" {
			return deployments, nil
		}
		next := aws.ToString(output.NextToken)
		if seenTokens[next] {
			return nil, fmt.Errorf("native deployment pagination repeated a cursor")
		}
		seenTokens[next] = true
		token = aws.String(next)
	}
	return nil, fmt.Errorf("native deployment history exceeds the initial 1000-record correlation bound")
}
func describeNativeDeployment(ctx context.Context, client NativeDeploymentClient, value string, r applyrequest.ECSOperationRequest) (*types.ServiceDeployment, error) {
	if !nativeScopedARN(value, "service-deployment", r.Intent) {
		return nil, fmt.Errorf("unreviewed native deployment ARN")
	}
	output, err := client.DescribeServiceDeployments(ctx, &ecs.DescribeServiceDeploymentsInput{ServiceDeploymentArns: []string{value}})
	if err != nil {
		return nil, err
	}
	if output == nil || len(output.Failures) > 0 || len(output.ServiceDeployments) != 1 {
		return nil, fmt.Errorf("native deployment evidence unavailable")
	}
	d := &output.ServiceDeployments[0]
	if aws.ToString(d.ServiceDeploymentArn) != value || aws.ToString(d.ServiceArn) != r.Intent.ServiceARN || aws.ToString(d.ClusterArn) != r.Intent.ClusterARN || !nativeKnownStatus(d.Status) {
		return nil, fmt.Errorf("native deployment identity/status does not match reviewed service")
	}
	if d.TargetServiceRevision == nil || !nativeScopedARN(aws.ToString(d.TargetServiceRevision.Arn), "service-revision", r.Intent) {
		return nil, fmt.Errorf("native target service revision unavailable")
	}
	return d, nil
}
func describeNativeRevision(ctx context.Context, client NativeDeploymentClient, value string, r applyrequest.ECSOperationRequest) (*types.ServiceRevision, error) {
	if !nativeScopedARN(value, "service-revision", r.Intent) {
		return nil, fmt.Errorf("unreviewed native service revision ARN")
	}
	output, err := client.DescribeServiceRevisions(ctx, &ecs.DescribeServiceRevisionsInput{ServiceRevisionArns: []string{value}})
	if err != nil {
		return nil, err
	}
	if output == nil || len(output.Failures) > 0 || len(output.ServiceRevisions) != 1 || aws.ToString(output.ServiceRevisions[0].ServiceRevisionArn) != value || aws.ToString(output.ServiceRevisions[0].ServiceArn) != r.Intent.ServiceARN || aws.ToString(output.ServiceRevisions[0].ClusterArn) != r.Intent.ClusterARN {
		return nil, fmt.Errorf("native revision workload/placement evidence unavailable")
	}
	return &output.ServiceRevisions[0], nil
}
