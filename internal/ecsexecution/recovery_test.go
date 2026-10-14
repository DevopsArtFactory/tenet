package ecsexecution

import (
	"context"
	"encoding/json"
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
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type recoveryServiceFake struct{ *serviceFake }

func (f recoveryServiceFake) UpdateService(ctx context.Context, input *ecs.UpdateServiceInput, options ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	out, err := f.serviceFake.UpdateService(ctx, input, options...)
	if f.writes > 1 && !f.noEffect {
		f.service.Deployments[0].Id = aws.String("ecs-svc/recovery")
	}
	return out, err
}

type recoveryPipeline struct {
	ctx      context.Context
	executor applyrequest.Executor
	backend  *serviceFake
	observer *observationFake
	parent   applyrequest.Record
	now      time.Time
}

func newRecoveryPipeline(t *testing.T, automatic bool, baselineCount ...int32) *recoveryPipeline {
	t.Helper()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	raw.Deployments[0].ECS.RollbackOnFailure = automatic
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, "worker")
	if err != nil {
		t.Fatal(err)
	}
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"})
	dir := t.TempDir()
	plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
	plan, err := plans.Create(ctx, "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	backend, _, fixture := serviceFixture(t)
	for value := range backend.definitions {
		if value != detail.ECS.BaseTaskDefinitionARN {
			delete(backend.definitions, value)
		}
	}
	backend.revisions = nil
	backend.registrationFake.writes = 0
	count := int32(2)
	if len(baselineCount) > 0 {
		count = baselineCount[0]
	}
	backend.service.DesiredCount = count
	backend.service.RunningCount = count
	backend.service.DeploymentConfiguration.MinimumHealthyPercent = aws.Int32(100)
	backend.service.DeploymentConfiguration.MaximumPercent = aws.Int32(200)
	backend.service.Deployments[0].DesiredCount = count
	backend.service.Deployments[0].RunningCount = count
	pipeline := &recoveryPipeline{ctx: ctx, backend: backend, now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	checks := func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}
	update := RollingService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ServiceClient, error) {
		return recoveryServiceFake{backend}, nil
	}, CheckCapabilities: checks}
	service := applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), LoadPlan: plans.Get, EnableAWSApply: true, Now: func() time.Time { return pipeline.now }, CurrentFingerprint: func(context.Context, string) (string, error) { return plan.Record.Source.Fingerprint, nil }, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		return fixture.Role, nil
	}, AuthorizeCreate: func(ctx context.Context, _ applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
		id := requestactor.AuthenticatedIdentityFromContext(ctx)
		if id.ID != "operator" || !id.Authenticated {
			return nil, applyrequest.ErrExecutionUnauthorized
		}
		return &applyrequest.ExecutionPrincipal{Identity: id}, nil
	}, RollbackTarget: func(ctx context.Context, parent applyrequest.Record) (*domain.ECSDeploymentIntent, error) {
		return update.RollbackTarget(ctx, parent, fixture.Role)
	}}
	service.CreateRetryPlan = func(ctx context.Context, name string) (planpreview.Record, error) {
		result, err := plans.Create(ctx, name, planpreview.CreateRequest{})
		return result.Record, err
	}
	created, err := service.Create(ctx, plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	pipeline.observer = &observationFake{serviceFake: backend, tasks: map[string]types.Task{}}
	pipeline.executor = applyrequest.Executor{Service: service, WorkerID: "recovery-pipeline", ECS: &applyrequest.ECSExecution{Enabled: true, Authorize: func(_ context.Context, record applyrequest.Record) error {
		if record.Principal == nil || record.Principal.Identity.ID != "operator" {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) { return detail.ECS.Clone(), nil }, Handlers: map[string]applyrequest.ECSOperationHandler{
		string(reconcile.StepRegisterECSTaskDefinition): Registration{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
			return backend.registrationFake, nil
		}, CheckCapabilities: checks},
		string(reconcile.StepUpdateECSService): update,
		string(reconcile.StepVerifyServiceStability): RollingObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ObservationClient, error) {
			return pipeline.observer, nil
		}, CheckCapabilities: checks, Now: func() time.Time { return pipeline.now }},
	}}}
	for i := 0; i < 2; i++ {
		if err := pipeline.executor.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	backend.service.Deployments[0].RolloutState = types.DeploymentRolloutStateFailed
	backend.service.Deployments[0].RolloutStateReason = aws.String("fixture startup failure")
	if err := pipeline.executor.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	pipeline.parent, err = service.Get(ctx, created.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.parent.Status != applyrequest.StatusFailed || pipeline.parent.ECSRuntime.Reason != "rollout_failed" {
		t.Fatalf("forward fixture did not fail: %+v", pipeline.parent)
	}
	return pipeline
}

func (p *recoveryPipeline) healthyRollback() {
	desired := p.backend.service.DesiredCount
	p.backend.service.RunningCount = desired
	p.backend.service.PendingCount = 0
	p.backend.service.Deployments[0].DesiredCount = desired
	p.backend.service.Deployments[0].RunningCount = desired
	p.backend.service.Deployments[0].PendingCount = 0
	p.backend.service.Deployments[0].RolloutState = types.DeploymentRolloutStateCompleted
	p.observer.running = nil
	p.observer.tasks = map[string]types.Task{}
	for i := 0; i < int(desired); i++ {
		value := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/recovered-%d", i)
		p.observer.running = append(p.observer.running, value)
		p.observer.tasks[value] = types.Task{TaskArn: aws.String(value), ClusterArn: p.backend.service.ClusterArn, Group: aws.String("service:worker"), TaskDefinitionArn: p.backend.service.TaskDefinition, StartedBy: p.backend.service.Deployments[0].Id, LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
	}
}

func TestRollingRollbackPipelineRestoresAndVerifiesWithoutDuplicateWrites(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		for _, awsRollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("automatic=%t,ecsRollback=%t", automatic, awsRollback), func(t *testing.T) {
				p := newRecoveryPipeline(t, automatic)
				parentBytes, _ := json.Marshal(p.parent)
				baseline := p.parent.Operations[1].ECSCheckpoint.Result.PreviousService
				if awsRollback {
					p.backend.service.TaskDefinition = aws.String(baseline.TaskDefinitionARN)
					p.backend.service.Deployments = []types.Deployment{{Id: aws.String("ecs-svc/aws-rollback"), Status: aws.String("PRIMARY"), TaskDefinition: p.backend.service.TaskDefinition, RolloutState: types.DeploymentRolloutStateInProgress}}
				}
				var created applyrequest.CreateResult
				var err error
				if automatic {
					if err = p.executor.RunOnce(p.ctx); err != nil {
						t.Fatal(err)
					}
					records, err := p.executor.Service.List(p.ctx, applyrequest.ListQuery{})
					if err != nil {
						t.Fatal(err)
					}
					for _, record := range records {
						if record.Recovery != nil {
							created.Record = record
						}
					}
					if created.Record.ID == "" || !created.Record.Recovery.Automatic {
						t.Fatal("opt-in failed rollout did not create a recovery")
					}
				} else {
					created, err = p.executor.Service.RequestRollback(p.ctx, p.parent.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 2; i++ {
					if err := p.executor.RunOnce(p.ctx); err != nil {
						t.Fatal(err)
					}
				}
				expected := 2
				if awsRollback {
					expected = 1
				}
				if p.backend.writes != expected || p.backend.registrationFake.writes != 1 || aws.ToString(p.backend.service.TaskDefinition) != baseline.TaskDefinitionARN {
					t.Fatalf("incorrect recovery mutation count: updates=%d registrations=%d", p.backend.writes, p.backend.registrationFake.writes)
				}
				p.healthyRollback()
				// Restart a fresh executor after mutation, retaining the durable checkpoints.
				p.now = p.now.Add(6 * time.Minute)
				p.executor.WorkerID = "recovery-after-restart"
				if err := p.executor.RunOnce(p.ctx); err != nil {
					t.Fatal(err)
				}
				result, err := p.executor.Service.Get(p.ctx, created.Record.ID)
				if err != nil {
					t.Fatal(err)
				}
				if result.Status != applyrequest.StatusSucceeded || result.ECSRuntime.State != "converged" || result.ECSRuntime.HealthyCount != 2 {
					t.Fatalf("previous revision was not verified healthy: %+v", result)
				}
				for i := 0; i < 3; i++ {
					if err := p.executor.RunOnce(p.ctx); err != nil {
						t.Fatal(err)
					}
				}
				parent, err := p.executor.Service.Get(p.ctx, p.parent.ID)
				if err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(parent)
				if string(parentBytes) != string(after) || parent.Status != applyrequest.StatusFailed || p.backend.writes != expected || p.backend.registrationFake.writes != 1 {
					t.Fatal("recovery changed failure audit or repeated AWS mutation")
				}
				if _, err := p.executor.Service.Create(p.ctx, p.parent.PlanID); !errors.Is(err, applyrequest.ErrFailedIntentHeld) {
					t.Fatal("recovery allowed failed desired intent to immediately redeploy")
				}
			})
		}
	}
}

func TestRollingRollbackRejectsInterveningDeploymentAndInactiveBaseline(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		t.Run(fmt.Sprint(inactive), func(t *testing.T) {
			p := newRecoveryPipeline(t, false)
			if inactive {
				baseline := p.parent.Operations[1].ECSCheckpoint.Result.PreviousService.TaskDefinitionARN
				p.backend.definitions[baseline].TaskDefinition.Status = types.TaskDefinitionStatusInactive
				if _, err := p.executor.Service.RequestRollback(p.ctx, p.parent.ID); err == nil {
					t.Fatal("inactive baseline was accepted")
				}
			} else {
				created, err := p.executor.Service.RequestRollback(p.ctx, p.parent.ID)
				if err != nil {
					t.Fatal(err)
				}
				p.backend.service.Deployments[0].Id = aws.String("ecs-svc/external-redeployment")
				if err := p.executor.RunOnce(p.ctx); err == nil {
					t.Fatal("rollback accepted intervening deployment")
				}
				result, err := p.executor.Service.Get(p.ctx, created.Record.ID)
				if err != nil {
					t.Fatal(err)
				}
				if result.Status != applyrequest.StatusQueued {
					t.Fatal("unsafe rollback claimed ownership")
				}
			}
			if p.backend.writes != 1 || p.backend.registrationFake.writes != 1 {
				t.Fatal("unsafe rollback called AWS mutation")
			}
		})
	}
}

func TestRollingRollbackRecoversLostUpdateResponseAndReportsRecoveryFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			p := newRecoveryPipeline(t, false)
			created, err := p.executor.Service.RequestRollback(p.ctx, p.parent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.executor.RunOnce(p.ctx); err != nil {
				t.Fatal(err)
			}
			p.backend.updateError = errors.New("connection lost after rollback acceptance")
			if err := p.executor.RunOnce(p.ctx); err != nil {
				t.Fatal(err)
			}
			interrupted, err := p.executor.Service.Get(p.ctx, created.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if interrupted.Operations[1].ECSCheckpoint.State != "in_flight" || p.backend.writes != 2 {
				t.Fatal("lost rollback response did not retain ambiguous checkpoint")
			}
			p.backend.updateError = nil
			p.now = p.now.Add(6 * time.Minute)
			p.executor.WorkerID = "recovered-owner"
			if err := p.executor.RunOnce(p.ctx); err != nil {
				t.Fatal(err)
			}
			if failure {
				p.backend.service.Deployments[0].RolloutState = types.DeploymentRolloutStateFailed
			} else {
				p.healthyRollback()
			}
			if err := p.executor.RunOnce(p.ctx); err != nil {
				t.Fatal(err)
			}
			result, err := p.executor.Service.Get(p.ctx, created.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := applyrequest.StatusSucceeded
			if failure {
				expected = applyrequest.StatusFailed
			}
			if result.Status != expected || p.backend.writes != 2 || p.backend.registrationFake.writes != 1 {
				t.Fatalf("rollback replayed or lost recovery outcome: %+v", result)
			}
			for i := 0; i < 3; i++ {
				if err := p.executor.RunOnce(p.ctx); err != nil {
					t.Fatal(err)
				}
			}
			records, err := p.executor.Service.List(p.ctx, applyrequest.ListQuery{})
			if err != nil || len(records) != 2 {
				t.Fatal("recovery failure recursively created another rollback")
			}
		})
	}
}

func TestRollingRollbackRestoresPreviouslyScaledToZeroService(t *testing.T) {
	p := newRecoveryPipeline(t, false, 0)
	created, err := p.executor.Service.RequestRollback(p.ctx, p.parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := p.executor.RunOnce(p.ctx); err != nil {
			t.Fatal(err)
		}
	}
	p.healthyRollback()
	if err := p.executor.RunOnce(p.ctx); err != nil {
		t.Fatal(err)
	}
	result, err := p.executor.Service.Get(p.ctx, created.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != applyrequest.StatusSucceeded || result.ECSRuntime.DesiredCount != 0 || result.ECSRuntime.RunningCount != 0 || p.backend.service.DesiredCount != 0 {
		t.Fatalf("zero baseline was not restored: %+v", result)
	}
}

func TestExplicitRetryReplacesCorrelatedFailedPrimaryAndSuppressesOldAutomaticRecovery(t *testing.T) {
	p := newRecoveryPipeline(t, true)
	retry, err := p.executor.Service.Retry(p.ctx, p.parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Record.RetryDeploymentID != "ecs-svc/new" {
		t.Fatal("explicit retry lost failed deployment correlation")
	}
	for i := 0; i < 2; i++ {
		if err := p.executor.RunOnce(p.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !p.backend.input.ForceNewDeployment || p.backend.writes != 2 || p.backend.registrationFake.writes != 1 {
		t.Fatal("retry did not start exactly one fresh deployment of the existing desired revision")
	}
	p.healthyRollback()
	if err := p.executor.RunOnce(p.ctx); err != nil {
		t.Fatal(err)
	}
	result, err := p.executor.Service.Get(p.ctx, retry.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	records, err := p.executor.Service.List(p.ctx, applyrequest.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != applyrequest.StatusSucceeded || len(records) != 2 {
		t.Fatal("explicit retry raced an automatic rollback or failed to converge")
	}
}
