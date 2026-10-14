package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
)

type observationFake struct {
	duplicate bool
	*serviceFake
	tasks         map[string]types.Task
	running       []string
	stopped       []string
	listError     error
	describeError error
	pages         bool
	listCalls     int
	batches       []int
	serviceFilter string
}

func (f *observationFake) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	f.listCalls++
	f.serviceFilter = aws.ToString(input.ServiceName)
	if f.listError != nil {
		return nil, f.listError
	}
	if input.DesiredStatus == types.DesiredStatusStopped {
		return &ecs.ListTasksOutput{TaskArns: f.stopped}, nil
	}
	if f.pages && input.NextToken == nil && len(f.running) > 1 {
		return &ecs.ListTasksOutput{TaskArns: f.running[:1], NextToken: aws.String("remaining")}, nil
	}
	if f.pages && input.NextToken != nil {
		return &ecs.ListTasksOutput{TaskArns: f.running[1:]}, nil
	}
	return &ecs.ListTasksOutput{TaskArns: f.running}, nil
}
func (f *observationFake) DescribeTasks(_ context.Context, input *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	f.batches = append(f.batches, len(input.Tasks))
	if f.describeError != nil {
		return nil, f.describeError
	}
	var result ecs.DescribeTasksOutput
	for _, value := range input.Tasks {
		if task, ok := f.tasks[value]; ok {
			result.Tasks = append(result.Tasks, task)
		} else {
			result.Failures = append(result.Failures, types.Failure{Arn: aws.String(value), Reason: aws.String("MISSING")})
		}
	}
	if f.duplicate && len(result.Tasks) > 1 {
		result.Tasks[1] = result.Tasks[0]
	}
	return &result, nil
}

func observationFixture(t *testing.T) (*observationFake, RollingObservation, applyrequest.ECSOperationRequest, *time.Time) {
	t.Helper()
	fake, update, request := serviceFixture(t)
	prepared, err := update.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparedResult = prepared
	accepted, err := update.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.PriorResults = append(request.PriorResults, accepted)
	request.OperationID = "verify"
	request.RequestID = "tenet-observe"
	request.PreparedResult = applyrequest.ECSOperationResult{}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	observerFake := &observationFake{serviceFake: fake, tasks: map[string]types.Task{}}
	observer := RollingObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ObservationClient, error) {
		return observerFake, nil
	}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}, Timeout: 10 * time.Minute, Now: func() time.Time { return now }}
	request.PreparedResult, err = observer.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fake.service.RunningCount = 2
	fake.service.Deployments[0].DesiredCount = 2
	fake.service.Deployments[0].RunningCount = 2
	fake.service.Deployments[0].RolloutState = types.DeploymentRolloutStateCompleted
	for i := 0; i < 2; i++ {
		value := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/task-%d", i)
		observerFake.running = append(observerFake.running, value)
		observerFake.tasks[value] = types.Task{TaskArn: aws.String(value), ClusterArn: aws.String(request.Intent.ClusterARN), Group: aws.String("service:worker"), TaskDefinitionArn: aws.String(accepted.TaskDefinitionARN), StartedBy: aws.String(accepted.ServiceDeploymentID), LastStatus: aws.String("RUNNING"), HealthStatus: types.HealthStatusUnknown, Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
	}
	return observerFake, observer, request, &now
}

func TestRollingObservationVerifiesIntendedTasksAndZeroDesiredCount(t *testing.T) {
	for _, zero := range []bool{false, true} {
		fake, observer, request, _ := observationFixture(t)
		fake.pages = true
		if zero {
			request.Intent.DesiredCount = 0
			request.PreparedResult.Observation.DesiredCount = 0
			fake.service.DesiredCount = 0
			fake.service.RunningCount = 0
			fake.service.Deployments[0].RunningCount = 0
			fake.service.Deployments[0].DesiredCount = 0
			fake.running = nil
		}
		result, err := observer.Reconcile(context.Background(), request)
		if err != nil || !result.Complete || result.Failure != "" || result.Result.Observation.State != "converged" || result.Result.Observation.HealthyCount != int32(request.Intent.DesiredCount) || fake.writes != 1 || fake.serviceFilter != "worker" {
			t.Fatalf("failed verified convergence: %+v %v", result, err)
		}
	}
}

func TestRollingObservationDoesNotConfuseMissingOrUnhealthyTasksWithSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		change       func(*observationFake, applyrequest.ECSOperationRequest)
	}{
		{"missing tasks", "missing_tasks", func(f *observationFake, _ applyrequest.ECSOperationRequest) { f.running = nil }},
		{"health checks pending", "health_checks_pending", func(f *observationFake, _ applyrequest.ECSOperationRequest) {
			task := f.tasks[f.running[0]]
			task.HealthStatus = types.HealthStatusUnhealthy
			f.tasks[f.running[0]] = task
		}},
		{"missing essential container", "health_checks_pending", func(f *observationFake, _ applyrequest.ECSOperationRequest) {
			task := f.tasks[f.running[0]]
			task.Containers = nil
			f.tasks[f.running[0]] = task
		}},
		{"previous deployment task", "missing_tasks", func(f *observationFake, _ applyrequest.ECSOperationRequest) {
			task := f.tasks[f.running[0]]
			task.StartedBy = aws.String("ecs-svc/old")
			f.tasks[f.running[0]] = task
		}},
		{"pending service task", "tasks_pending", func(f *observationFake, _ applyrequest.ECSOperationRequest) {
			f.service.PendingCount = 1
			f.service.Deployments[0].PendingCount = 1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, h, r, _ := observationFixture(t)
			tc.change(f, r)
			result, err := h.Reconcile(context.Background(), r)
			if err != nil || result.Complete || result.Failure != "" || result.Result.Observation.Reason != tc.reason {
				t.Fatalf("bad pending evidence: %+v %v", result, err)
			}
		})
	}
}

func TestRollingObservationPersistsDeadlineAndDistinguishesReadFailures(t *testing.T) {
	fake, observer, request, now := observationFixture(t)
	deadline := request.PreparedResult.Observation.DeadlineAt
	fake.listError = &smithy.GenericAPIError{Code: "ThrottlingException", Message: "limited"}
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || result.Complete || result.Failure != "" || result.Result.Observation.State != "unavailable" || result.Result.Observation.Reason != "aws_throttled" {
		t.Fatalf("read failure became rollout failure/success: %+v %v", result, err)
	}
	request.PreparedResult = result.Result
	fake.listError = nil
	*now = now.Add(time.Minute)
	resumed, err := observer.Reconcile(context.Background(), request)
	if err != nil || !resumed.Complete || resumed.Result.Observation.DeadlineAt != deadline || resumed.Result.Observation.Polls != 2 || fake.writes != 1 {
		t.Fatalf("restart lost deadline or replayed update: %+v %v", resumed, err)
	}
	*now = now.Add(20 * time.Minute)
	timedOut, err := observer.Reconcile(context.Background(), request)
	if err != nil || timedOut.Failure == "" || timedOut.Result.Observation.Reason != "observation_timeout" {
		t.Fatalf("deadline was reset across restart: %+v %v", timedOut, err)
	}
}

func TestRollingObservationSeparatesStaleReadsFromRollbackAndSupersession(t *testing.T) {
	fake, observer, request, now := observationFixture(t)
	original := fake.service.Deployments[0]
	fake.service.TaskDefinition = aws.String(request.PriorResults[1].PreviousTaskDefinitionARN)
	before := now.Add(-time.Minute)
	fake.service.Deployments = []types.Deployment{{Id: aws.String("ecs-svc/old"), Status: aws.String("PRIMARY"), TaskDefinition: fake.service.TaskDefinition, RolloutState: types.DeploymentRolloutStateCompleted, CreatedAt: &before}}
	stale, err := observer.Reconcile(context.Background(), request)
	if err != nil || stale.Failure != "" || stale.Complete || stale.Result.Observation.Reason != "service_update_visibility_pending" {
		t.Fatalf("stale service read became rollback: %+v %v", stale, err)
	}
	after := now.Add(time.Second)
	*now = now.Add(2 * time.Second)
	fake.service.Deployments[0].CreatedAt = &after
	fake.service.Deployments[0].Id = aws.String("ecs-svc/rollback")
	rollback, err := observer.Reconcile(context.Background(), request)
	if err != nil || rollback.Failure == "" || rollback.Result.Observation.Reason != "rollback_observed" {
		t.Fatalf("explicit rollback was not distinguished: %+v %v", rollback, err)
	}
	fake.service.TaskDefinition = aws.String(request.PriorResults[0].TaskDefinitionARN)
	fake.service.Deployments = []types.Deployment{original}
	fake.service.Deployments[0].RolloutState = types.DeploymentRolloutStateFailed
	fake.service.Deployments[0].RolloutStateReason = aws.String("circuit breaker failed")
	failed, err := observer.Reconcile(context.Background(), request)
	if err != nil || failed.Failure == "" || failed.Result.Observation.Reason != "rollout_failed" {
		t.Fatalf("correlated rollout failure was not distinguished: %+v %v", failed, err)
	}
	fake.service.Deployments[0].Id = aws.String("ecs-svc/another")
	fake.service.Deployments[0].RolloutState = types.DeploymentRolloutStateCompleted
	fake.service.Deployments[0].CreatedAt = &after
	superseded, err := observer.Reconcile(context.Background(), request)
	if err != nil || superseded.Failure == "" || superseded.Result.Observation.Reason != "superseded_deployment" {
		t.Fatalf("new deployment identity was accepted: %+v %v", superseded, err)
	}
}

func TestRollingObservationReportsCorrelatedStartupEvidenceWithoutPrematureFailure(t *testing.T) {
	fake, observer, request, now := observationFixture(t)
	fake.running = nil
	fake.service.RunningCount = 0
	fake.service.Deployments[0].RunningCount = 0
	fake.service.Deployments[0].RolloutState = types.DeploymentRolloutStateInProgress
	value := "arn:aws:ecs:ap-northeast-2:123456789012:task/production/stopped"
	fake.stopped = []string{value}
	fake.tasks[value] = types.Task{TaskArn: aws.String(value), TaskDefinitionArn: aws.String(request.PriorResults[0].TaskDefinitionARN), StartedBy: aws.String(request.PriorResults[1].ServiceDeploymentID), StoppedAt: now, StopCode: types.TaskStopCodeTaskFailedToStart, StoppedReason: aws.String("CannotPullContainerError: unavailable image")}
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || result.Complete || result.Failure != "" || result.Result.Observation.Reason != "task_startup_error" {
		t.Fatalf("startup error was hidden or prematurely terminal: %+v %v", result, err)
	}
}

func TestRollingObservationBatchesLargeServicesAndRejectsIncompleteEvidence(t *testing.T) {
	fake, observer, request, _ := observationFixture(t)
	template := fake.tasks[fake.running[0]]
	fake.running = nil
	for i := 0; i < 205; i++ {
		value := fmt.Sprintf("task-%d", i)
		task := template
		task.TaskArn = aws.String(value)
		fake.tasks[value] = task
		fake.running = append(fake.running, value)
	}
	request.Intent.DesiredCount = 205
	fake.service.DesiredCount = 205
	fake.service.RunningCount = 205
	fake.service.Deployments[0].DesiredCount = 2
	fake.service.Deployments[0].DesiredCount = 205
	fake.service.Deployments[0].RunningCount = 205
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || !result.Complete || len(fake.batches) != 3 || fake.batches[0] != 100 || fake.batches[2] != 5 {
		t.Fatalf("large-service evidence was truncated: %+v %v batches=%v", result, err, fake.batches)
	}
	fake.describeError = errors.New("temporary task evidence outage")
	unavailable, err := observer.Reconcile(context.Background(), request)
	if err != nil || unavailable.Complete || unavailable.Failure != "" || unavailable.Result.Observation.State != "unavailable" {
		t.Fatal("incomplete task evidence became healthy")
	}
}

func TestRollingObservationIgnoresFailedHistoryForAnotherDeploymentOfSameRevision(t *testing.T) {
	fake, observer, request, _ := observationFixture(t)
	fake.service.Deployments = append(fake.service.Deployments, types.Deployment{Id: aws.String("ecs-svc/older-failed"), Status: aws.String("INACTIVE"), TaskDefinition: aws.String(request.PriorResults[0].TaskDefinitionARN), RolloutState: types.DeploymentRolloutStateFailed})
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || !result.Complete || result.Failure != "" {
		t.Fatalf("historical failure was confused with intended deployment: %+v %v", result, err)
	}
}

func TestRollingObservationRejectsDuplicateTaskEvidence(t *testing.T) {
	fake, observer, request, _ := observationFixture(t)
	fake.duplicate = true
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || result.Complete || result.Result.Observation.State != "unavailable" || result.Result.Observation.TasksObserved {
		t.Fatalf("duplicate task response falsely verified desired count: %+v %v", result, err)
	}
}

func TestRollingObservationUsesAWSDeploymentTimeForEarlySupersession(t *testing.T) {
	fake, observer, request, now := observationFixture(t)
	request.PreparedResult.ServiceDeploymentCreatedAt = now.Add(-10 * time.Second).Format(time.RFC3339Nano)
	created := now.Add(-5 * time.Second)
	fake.service.Deployments[0].Id = aws.String("ecs-svc/newer-before-observer-started")
	fake.service.Deployments[0].CreatedAt = &created
	result, err := observer.Reconcile(context.Background(), request)
	if err != nil || result.Failure == "" || result.Result.Observation.Reason != "superseded_deployment" {
		t.Fatalf("deployment superseded before observer start was missed: %+v %v", result, err)
	}
}
