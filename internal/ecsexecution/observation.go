package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
)

type ObservationClient interface {
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

// RollingObservation verifies actual intended-revision tasks without a mutation API.
type RollingObservation struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (ObservationClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
	Timeout           time.Duration
	Now               func() time.Time
}

func (h RollingObservation) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h RollingObservation) Preflight(ctx context.Context, request applyrequest.ECSOperationRequest) error {
	role := request.Role
	e := request.Intent
	service, err := arn.Parse(e.ServiceARN)
	cluster, clusterErr := arn.Parse(e.ClusterARN)
	deployer, roleErr := arn.Parse(role.RoleARN)
	if h.ClientFor == nil || h.CheckCapabilities == nil || err != nil || clusterErr != nil || roleErr != nil || service.Service != "ecs" || cluster.Service != "ecs" || service.AccountID != e.AWSAccountID || cluster.AccountID != e.AWSAccountID || service.Region != e.Region || cluster.Region != e.Region || service.Partition != cluster.Partition || shortServiceName(e.ServiceARN) == "" || deployer.Service != "iam" || deployer.AccountID != e.AWSAccountID || deployer.Partition != service.Partition || !strings.HasPrefix(deployer.Resource, "role/") || !role.Configured || !role.Ready || !role.CheckedWithAWS || role.AWSAccountID != e.AWSAccountID || role.Region != e.Region || e.DeploymentController != "ECS" || e.DeploymentStrategy != "ROLLING" {
		return fmt.Errorf("observation requires verified native Rolling placement, role and read providers")
	}
	target := e.BaseTaskDefinitionARN
	if len(request.PriorResults) > 0 {
		target = request.PriorResults[0].TaskDefinitionARN
	}
	return h.CheckCapabilities(ctx, role, []applyrequest.OperationExecutionCapabilityCheck{
		{Context: map[string]string{"ecs:cluster": e.ClusterARN}, Action: "ecs:DescribeServices", ResourceType: "ecsService", ResourceName: e.ServiceARN, RequiredRole: role.RoleARN},
		{Action: "ecs:DescribeTaskDefinition", ResourceType: "taskDefinition", ResourceName: target, RequiredRole: role.RoleARN},
		{Context: map[string]string{"ecs:cluster": e.ClusterARN}, Action: "ecs:ListTasks", ResourceName: "*", RequiredRole: role.RoleARN},
		{Context: map[string]string{"ecs:cluster": e.ClusterARN}, Action: "ecs:DescribeTasks", ResourceType: "ecsTask", ResourceName: fmt.Sprintf("arn:%s:ecs:%s:%s:task/*", service.Partition, e.Region, e.AWSAccountID), RequiredRole: role.RoleARN},
	})
}

func (h RollingObservation) Prepare(_ context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	if len(request.PriorResults) < 2 || request.PriorResults[0].TaskDefinitionARN == "" || request.PriorResults[1].TaskDefinitionARN != request.PriorResults[0].TaskDefinitionARN {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("registration and accepted service-update checkpoints are required for observation")
	}
	result := request.PreparedResult.Clone()
	if result.Observation == nil {
		timeout := h.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Minute
		}
		now := h.now()
		result = request.PriorResults[1].Clone()
		result.Observation = &applyrequest.ECSRolloutObservation{Source: "aws-ecs", State: "initializing", Reason: "awaiting_observation", Summary: "Awaiting ECS rollout evidence.", StartedAt: now.Format(time.RFC3339Nano), DeadlineAt: now.Add(timeout).Format(time.RFC3339Nano), ClusterARN: request.Intent.ClusterARN, ServiceARN: request.Intent.ServiceARN, TaskDefinitionARN: result.TaskDefinitionARN, DeploymentID: result.ServiceDeploymentID, ServiceDeploymentARN: result.ServiceDeploymentARN, DesiredCount: int32(request.Intent.DesiredCount)}
	}
	start, err := time.Parse(time.RFC3339Nano, result.Observation.StartedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, result.Observation.DeadlineAt)
	if err != nil || deadlineErr != nil || !deadline.After(start) || result.TaskDefinitionARN != request.PriorResults[0].TaskDefinitionARN {
		return applyrequest.ECSOperationResult{}, fmt.Errorf("persisted ECS observation deadline or revision is invalid")
	}
	return result, nil
}

func (h RollingObservation) Execute(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSOperationResult, error) {
	observed, err := h.Reconcile(ctx, request)
	if err != nil {
		return applyrequest.ECSOperationResult{}, err
	}
	if observed.Complete {
		return observed.Result, nil
	}
	if observed.Failure != "" {
		return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationFailure{Result: observed.Result}
	}
	return applyrequest.ECSOperationResult{}, &applyrequest.ECSObservationPending{Result: observed.Result, RetryAfter: observed.RetryAfter}
}

func (h RollingObservation) Reconcile(ctx context.Context, request applyrequest.ECSOperationRequest) (applyrequest.ECSReconciliation, error) {
	if err := ctx.Err(); err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	if request.PreparedResult.Observation == nil {
		return applyrequest.ECSReconciliation{}, fmt.Errorf("durable observation deadline is required before polling")
	}
	result, err := h.Prepare(ctx, request)
	if err != nil {
		return applyrequest.ECSReconciliation{}, err
	}
	observation := result.Observation
	observation.Polls++
	observation.TasksObserved = false
	observation.DesiredCount = int32(request.Intent.DesiredCount)
	observation.ObservedAt = h.now().Format(time.RFC3339Nano)
	deadline, _ := time.Parse(time.RFC3339Nano, observation.DeadlineAt)
	if !h.now().Before(deadline) {
		return observedFailure(result, "observation_timeout", "ECS rollout observation exceeded its persisted deadline."), nil
	}
	ctx, cancel := context.WithTimeout(ctx, deadline.Sub(h.now()))
	defer cancel()
	if err := h.Preflight(ctx, request); err != nil {
		return observationUnavailable(result, err), nil
	}
	client, err := h.ClientFor(ctx, request.Role)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if client == nil {
		return observationUnavailable(result, fmt.Errorf("observation client is unavailable")), nil
	}
	response, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: aws.String(request.Intent.ClusterARN), Services: []string{request.Intent.ServiceARN}})
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if response == nil || len(response.Failures) > 0 || len(response.Services) != 1 {
		return observationUnavailable(result, fmt.Errorf("reviewed service is missing from ECS evidence")), nil
	}
	service := &response.Services[0]
	if aws.ToString(service.ServiceArn) != request.Intent.ServiceARN || aws.ToString(service.ClusterArn) != request.Intent.ClusterARN {
		return observationUnavailable(result, fmt.Errorf("AWS returned evidence for another service or cluster")), nil
	}
	observation.ServiceRunningCount = service.RunningCount
	observation.PendingCount = service.PendingCount
	if aws.ToString(service.Status) != "ACTIVE" || service.DeploymentController == nil || service.DeploymentController.Type != types.DeploymentControllerTypeEcs || service.DeploymentConfiguration == nil || (service.DeploymentConfiguration.Strategy != "" && service.DeploymentConfiguration.Strategy != types.DeploymentStrategyRolling) {
		return observedFailure(result, "superseded_deployment", "The service controller or strategy changed during the reviewed rollout."), nil
	}
	var primary *types.Deployment
	for i := range service.Deployments {
		deployment := &service.Deployments[i]
		if aws.ToString(deployment.TaskDefinition) == result.TaskDefinitionARN && deployment.RolloutState == types.DeploymentRolloutStateFailed && ((result.ServiceDeploymentID != "" && aws.ToString(deployment.Id) == result.ServiceDeploymentID) || (result.ServiceDeploymentID == "" && aws.ToString(deployment.Status) == "PRIMARY")) {
			reason := "rollout_failed"
			message := "ECS rollout failed: " + shortEvidence(aws.ToString(deployment.RolloutStateReason))
			if aws.ToString(service.TaskDefinition) == request.PriorResults[1].PreviousTaskDefinitionARN {
				reason = "rollback_observed"
				message = "ECS rolled back the requested deployment: " + shortEvidence(aws.ToString(deployment.RolloutStateReason))
			}
			return observedFailure(result, reason, message), nil
		}
		if aws.ToString(deployment.Status) == "PRIMARY" {
			primary = deployment
		}
	}
	if aws.ToString(service.TaskDefinition) != result.TaskDefinitionARN {
		started := deploymentObservationAnchor(result)
		if primary != nil && primary.CreatedAt != nil && primary.CreatedAt.After(started) {
			if aws.ToString(service.TaskDefinition) == request.PriorResults[1].PreviousTaskDefinitionARN {
				return observedFailure(result, "rollback_observed", "A new ECS deployment rolled the service back to its previous revision."), nil
			}
			return observedFailure(result, "superseded_deployment", "A new ECS deployment superseded the requested task-definition revision."), nil
		}
		return observationProgress(result, "service_update_visibility_pending", "The requested revision is not visible in the service read yet; awaiting correlated deployment evidence."), nil
	}
	if service.DesiredCount != int32(request.Intent.DesiredCount) || aws.ToInt32(service.DeploymentConfiguration.MinimumHealthyPercent) != int32(request.Intent.MinimumHealthyPercent) || aws.ToInt32(service.DeploymentConfiguration.MaximumPercent) != int32(request.Intent.MaximumPercent) {
		return observationProgress(result, "service_intent_mismatch", fmt.Sprintf("Service reports desired count %d; reviewed count is %d. Waiting for matching count and rolling configuration.", service.DesiredCount, request.Intent.DesiredCount)), nil
	}

	if primary == nil || aws.ToString(primary.Id) == "" || aws.ToString(primary.TaskDefinition) != result.TaskDefinitionARN {
		return observationProgress(result, "deployment_identity_pending", "Waiting for the intended revision to become the primary ECS deployment."), nil
	}
	if result.ServiceDeploymentID != "" && aws.ToString(primary.Id) != result.ServiceDeploymentID {
		started := deploymentObservationAnchor(result)
		if primary.CreatedAt != nil && primary.CreatedAt.After(started) {
			return observedFailure(result, "superseded_deployment", "Another ECS deployment superseded the correlated deployment identity."), nil
		}
		return observationProgress(result, "deployment_identity_pending", "Waiting for the correlated ECS primary deployment identity to become visible."), nil
	}
	result.ServiceDeploymentID = aws.ToString(primary.Id)
	observation.DeploymentID = result.ServiceDeploymentID
	definition, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: aws.String(result.TaskDefinitionARN)})
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	if definition == nil || definition.TaskDefinition == nil || aws.ToString(definition.TaskDefinition.TaskDefinitionArn) != result.TaskDefinitionARN {
		return observationUnavailable(result, fmt.Errorf("intended task-definition evidence is unavailable")), nil
	}
	if definition.TaskDefinition.Status != types.TaskDefinitionStatusActive {
		return observedFailure(result, "requested_revision_inactive", "The requested task definition was deactivated during rollout."), nil
	}
	tasks, err := runningTasks(ctx, client, request)
	if err != nil {
		return observationUnavailable(result, err), nil
	}
	running, healthy, old, pending := int32(0), int32(0), int32(0), int32(0)
	for _, task := range tasks {
		if aws.ToString(task.ClusterArn) != request.Intent.ClusterARN || (task.Group != nil && aws.ToString(task.Group) != "service:"+shortServiceName(request.Intent.ServiceARN)) {
			return observationUnavailable(result, fmt.Errorf("task evidence belongs to another cluster")), nil
		}
		if aws.ToString(task.LastStatus) != "RUNNING" {
			pending++
			continue
		}
		if aws.ToString(task.TaskDefinitionArn) != result.TaskDefinitionARN {
			old++
			continue
		}
		if aws.ToString(task.StartedBy) != "" && aws.ToString(task.StartedBy) != result.ServiceDeploymentID {
			old++
			continue
		}
		running++
		if healthyTask(task, *definition.TaskDefinition) {
			healthy++
		}
	}
	observation.TasksObserved = true
	observation.RunningCount = running
	observation.PreviousCount = old
	if pending > observation.PendingCount {
		observation.PendingCount = pending
	}
	observation.HealthyCount = healthy
	desired := int32(request.Intent.DesiredCount)
	if running == desired && healthy == desired && old == 0 && pending == 0 && service.RunningCount == desired && service.PendingCount == 0 && primary.DesiredCount == desired && primary.RunningCount == desired && primary.PendingCount == 0 {
		if primary.RolloutState == types.DeploymentRolloutStateCompleted || (primary.RolloutState == "" && len(service.LoadBalancers) == 0) {
			observation.State = "converged"
			observation.Reason = "intended_revision_healthy"
			observation.Summary = fmt.Sprintf("Intended ECS revision verified: %d/%d healthy tasks, no pending or previous-revision tasks.", healthy, desired)
			return applyrequest.ECSReconciliation{Complete: true, Result: result}, nil
		}
		if primary.RolloutState == types.DeploymentRolloutStateInProgress {
			return observationProgress(result, "deployment_in_progress", "Intended tasks are healthy; waiting for ECS deployment completion."), nil
		}
		if primary.RolloutState == "" {
			return observationProgress(result, "health_evidence_unavailable", "Task counts match, but ECS does not expose load-balancer rollout completion evidence for this service."), nil
		}
	}
	reason := "tasks_pending"
	if running < desired && service.PendingCount == 0 {
		reason = "missing_tasks"
	} else if running == desired && healthy < desired {
		reason = "health_checks_pending"
	}
	message := fmt.Sprintf("ECS rollout: %d/%d intended tasks running, %d healthy, %d pending, %d previous-revision tasks.", running, desired, healthy, observation.PendingCount, old)
	if evidence := startupEvidence(ctx, client, request, result); evidence != "" {
		reason = "task_startup_error"
		message += " " + evidence
	}
	if reason != "task_startup_error" {
		started, _ := time.Parse(time.RFC3339Nano, observation.StartedAt)
		for _, event := range service.Events {
			text := aws.ToString(event.Message)
			lower := strings.ToLower(text)
			if event.CreatedAt != nil && !event.CreatedAt.Before(started) && (strings.Contains(lower, "unable") || strings.Contains(lower, "failed")) {
				reason = "service_event_warning"
				message += " ECS event: " + shortEvidence(text)
				break
			}
		}
	}
	return observationProgress(result, reason, message), nil
}

func runningTasks(ctx context.Context, client ObservationClient, request applyrequest.ECSOperationRequest) ([]types.Task, error) {
	var arns []string
	var token *string
	seenTokens := map[string]bool{}
	seenTasks := map[string]bool{}
	for {
		page, err := client.ListTasks(ctx, &ecs.ListTasksInput{Cluster: aws.String(request.Intent.ClusterARN), ServiceName: aws.String(shortServiceName(request.Intent.ServiceARN)), DesiredStatus: types.DesiredStatusRunning, MaxResults: aws.Int32(100), NextToken: token})
		if err != nil {
			return nil, err
		}
		if page == nil {
			return nil, fmt.Errorf("task listing returned no evidence")
		}
		for _, value := range page.TaskArns {
			if !seenTasks[value] {
				seenTasks[value] = true
				arns = append(arns, value)
			}
		}
		token = page.NextToken
		if aws.ToString(token) == "" {
			break
		}
		if seenTokens[*token] {
			return nil, fmt.Errorf("task pagination repeated a token")
		}
		seenTokens[*token] = true
	}
	var tasks []types.Task
	described := map[string]bool{}
	for offset := 0; offset < len(arns); offset += 100 {
		end := offset + 100
		if end > len(arns) {
			end = len(arns)
		}
		output, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(request.Intent.ClusterARN), Tasks: arns[offset:end]})
		if err != nil {
			return nil, err
		}
		if output == nil || len(output.Failures) > 0 || len(output.Tasks) != end-offset {
			return nil, fmt.Errorf("listed tasks are not yet available for complete observation")
		}
		for _, task := range output.Tasks {
			value := aws.ToString(task.TaskArn)
			if !seenTasks[value] || described[value] {
				return nil, fmt.Errorf("task evidence contains an unexpected or duplicate identity")
			}
			described[value] = true
		}
		tasks = append(tasks, output.Tasks...)
	}
	return tasks, nil
}

func healthyTask(task types.Task, definition types.TaskDefinition) bool {
	if aws.ToString(task.LastStatus) != "RUNNING" || task.HealthStatus == types.HealthStatusUnhealthy {
		return false
	}
	essential := 0
	for _, expected := range definition.ContainerDefinitions {
		if expected.Essential != nil && !*expected.Essential {
			continue
		}
		essential++
		found := false
		for _, container := range task.Containers {
			if aws.ToString(container.Name) == aws.ToString(expected.Name) && aws.ToString(container.LastStatus) == "RUNNING" {
				if container.HealthStatus == types.HealthStatusUnhealthy {
					return false
				}
				if expected.HealthCheck != nil && (container.HealthStatus != types.HealthStatusHealthy || task.HealthStatus != types.HealthStatusHealthy) {
					return false
				}
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return essential > 0
}

// A bounded stopped-task sample is diagnostic only; it never proves success.
func startupEvidence(ctx context.Context, client ObservationClient, request applyrequest.ECSOperationRequest, result applyrequest.ECSOperationResult) string {
	page, err := client.ListTasks(ctx, &ecs.ListTasksInput{Cluster: aws.String(request.Intent.ClusterARN), ServiceName: aws.String(shortServiceName(request.Intent.ServiceARN)), DesiredStatus: types.DesiredStatusStopped, MaxResults: aws.Int32(100)})
	if err != nil || page == nil || len(page.TaskArns) == 0 {
		return ""
	}
	output, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(request.Intent.ClusterARN), Tasks: page.TaskArns})
	if err != nil || output == nil {
		return ""
	}
	started, _ := time.Parse(time.RFC3339Nano, result.Observation.StartedAt)
	for _, task := range output.Tasks {
		if aws.ToString(task.TaskDefinitionArn) == result.TaskDefinitionARN && aws.ToString(task.StartedBy) == result.ServiceDeploymentID && task.StoppedAt != nil && !task.StoppedAt.Before(started) && task.StopCode == types.TaskStopCodeTaskFailedToStart {
			return "Task startup failed: " + shortEvidence(aws.ToString(task.StoppedReason))
		}
	}
	return ""
}

func observationProgress(result applyrequest.ECSOperationResult, reason, message string) applyrequest.ECSReconciliation {
	result.Observation.State = "progressing"
	result.Observation.Reason = reason
	result.Observation.Summary = shortEvidence(message)
	polls := result.Observation.Polls
	if polls > 5 {
		polls = 5
	}
	if polls < 1 {
		polls = 1
	}
	delay := time.Second * time.Duration(1<<polls)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return applyrequest.ECSReconciliation{Result: result, RetryAfter: delay}
}
func observedFailure(result applyrequest.ECSOperationResult, reason, message string) applyrequest.ECSReconciliation {
	result.Observation.State = "failed"
	result.Observation.Reason = reason
	result.Observation.Summary = shortEvidence(message)
	return applyrequest.ECSReconciliation{Failure: result.Observation.Summary, Result: result}
}
func observationUnavailable(result applyrequest.ECSOperationResult, cause error) applyrequest.ECSReconciliation {
	reason := "aws_unavailable"
	var api smithy.APIError
	if errors.As(cause, &api) {
		switch api.ErrorCode() {
		case "ThrottlingException", "TooManyRequestsException":
			reason = "aws_throttled"
		case "AccessDenied", "AccessDeniedException":
			reason = "observation_access_denied"
		}
	}
	pending := observationProgress(result, reason, "ECS observation unavailable: "+shortEvidence(cause.Error()))
	pending.Result.Observation.State = "unavailable"
	return pending
}
func shortEvidence(value string) string {
	runes := []rune(value)
	if len(runes) > 500 {
		return string(runes[:500]) + "…"
	}
	return value
}

func shortServiceName(value string) string {
	parsed, err := arn.Parse(value)
	if err != nil {
		return ""
	}
	parts := strings.Split(parsed.Resource, "/")
	if len(parts) != 3 || parts[0] != "service" {
		return ""
	}
	return parts[2]
}

func deploymentObservationAnchor(result applyrequest.ECSOperationResult) time.Time {
	if created, err := time.Parse(time.RFC3339Nano, result.ServiceDeploymentCreatedAt); err == nil {
		return created
	}
	started, _ := time.Parse(time.RFC3339Nano, result.Observation.StartedAt)
	return started
}
