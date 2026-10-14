package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestReviewedECSPipelineRegistersUpdatesAndResumesRealObservationHandlers(t *testing.T) {
	for _, alreadyMatching := range []bool{false, true} {
		t.Run(fmt.Sprintf("alreadyMatching=%t", alreadyMatching), func(t *testing.T) {
			ctx := context.Background()
			raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
			if err != nil {
				t.Fatal(err)
			}
			compiled, err := manifest.Compile(raw)
			if err != nil {
				t.Fatal(err)
			}
			detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "worker")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
			plan, err := plans.Create(ctx, "worker", planpreview.CreateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			backend, _, fixtureRequest := serviceFixture(t)
			for value := range backend.definitions {
				if value != detail.ECS.BaseTaskDefinitionARN {
					delete(backend.definitions, value)
				}
			}
			backend.revisions = nil
			backend.registrationFake.writes = 0
			expectedWrites := 1
			if alreadyMatching {
				expectedWrites = 0
				backend.definitions[detail.ECS.BaseTaskDefinitionARN].TaskDefinition.ContainerDefinitions[0].Image = aws.String(detail.ECS.Image)
				backend.service.DesiredCount = int32(detail.ECS.DesiredCount)
				backend.service.RunningCount = int32(detail.ECS.DesiredCount)
				backend.service.DeploymentConfiguration.MinimumHealthyPercent = aws.Int32(int32(detail.ECS.MinimumHealthyPercent))
				backend.service.DeploymentConfiguration.MaximumPercent = aws.Int32(int32(detail.ECS.MaximumPercent))
				backend.service.Deployments[0].DesiredCount = int32(detail.ECS.DesiredCount)
				backend.service.Deployments[0].RunningCount = int32(detail.ECS.DesiredCount)
			}
			now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
			store := applyrequest.NewFileStore(filepath.Join(dir, "applies.json"))
			service := applyrequest.Service{Store: store, LoadPlan: plans.Get, EnableAWSApply: true, Now: func() time.Time { return now }, CurrentFingerprint: func(context.Context, string) (string, error) { return plan.Record.Source.Fingerprint, nil }, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
				return fixtureRequest.Role, nil
			}}
			created, err := service.Create(ctx, plan.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			// This explicit fixture allowlist represents a trusted authorization provider;
			// actor-attribution headers are never used to authorize execution.
			allowedApplyID := created.Record.ID
			checks := func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
				return nil
			}
			observeBackend := &observationFake{serviceFake: backend, tasks: map[string]types.Task{}}
			executor := applyrequest.Executor{Service: service, WorkerID: "pipeline-worker", ECS: &applyrequest.ECSExecution{Enabled: true, Authorize: func(_ context.Context, record applyrequest.Record) error {
				if record.ID != allowedApplyID {
					return errors.New("not in trusted fixture approval set")
				}
				return nil
			}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) { return detail.ECS.Clone(), nil }, Handlers: map[string]applyrequest.ECSOperationHandler{
				string(reconcile.StepRegisterECSTaskDefinition): Registration{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
					return backend.registrationFake, nil
				}, CheckCapabilities: checks},
				string(reconcile.StepUpdateECSService): RollingService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ServiceClient, error) { return backend, nil }, CheckCapabilities: checks},
				string(reconcile.StepVerifyServiceStability): RollingObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ObservationClient, error) {
					return observeBackend, nil
				}, CheckCapabilities: checks, Now: func() time.Time { return now }},
			}}}
			for i := 0; i < 3; i++ {
				if err := executor.RunOnce(ctx); err != nil {
					t.Fatal(err)
				}
			}
			pending, err := service.Get(ctx, created.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if pending.Status != applyrequest.StatusRunning || pending.ECSRuntime == nil || pending.ECSRuntime.Reason != "missing_tasks" || backend.writes != expectedWrites || backend.registrationFake.writes != expectedWrites {
				t.Fatalf("pipeline failed to stop at real observation: %+v", pending)
			}
			beforeDeadline := pending.ECSRuntime.DeadlineAt
			target := pending.Operations[0].ECSCheckpoint.Result.TaskDefinitionARN
			if pending.Operations[1].ECSCheckpoint.Result.PreviousService == nil {
				t.Fatal("pipeline omitted pre-update rollback snapshot")
			}
			backend.service.RunningCount = 2
			backend.service.PendingCount = 0
			backend.service.Deployments[0].DesiredCount = 2
			backend.service.Deployments[0].RunningCount = 2
			backend.service.Deployments[0].PendingCount = 0
			backend.service.Deployments[0].RolloutState = types.DeploymentRolloutStateCompleted
			for i := 0; i < 2; i++ {
				value := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/converged-%d", i)
				observeBackend.running = append(observeBackend.running, value)
				observeBackend.tasks[value] = types.Task{TaskArn: aws.String(value), ClusterArn: aws.String(detail.ECS.ClusterARN), Group: aws.String("service:worker"), TaskDefinitionArn: aws.String(target), StartedBy: backend.service.Deployments[0].Id, LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
			}
			now = now.Add(6 * time.Minute)
			executor.WorkerID = "pipeline-after-restart"
			if err := executor.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
			completed, err := service.Get(ctx, created.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if completed.Status != applyrequest.StatusSucceeded || completed.ECSRuntime.State != "converged" || completed.ECSRuntime.HealthyCount != 2 || completed.ECSRuntime.DeadlineAt != beforeDeadline || backend.writes != expectedWrites || backend.registrationFake.writes != expectedWrites {
				t.Fatalf("restart replayed mutation or lost verified convergence: %+v", completed)
			}

		})
	}
}
