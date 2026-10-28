package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/ecsreconcile"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/spec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// The Git input and SDK boundary are fixtures. Planning, durable approvals,
// registration, native updates, correlation and observation use real handlers.
func TestNativeGitAutomaticPipelineWaitsReconcilesAndHoldsRollbackFailures(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			ctx := context.Background()
			backend, update, request := nativeFixture(t, strategy)
			raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
			if err != nil {
				t.Fatal(err)
			}
			raw.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
			raw.Deployments[0].Strategy.Type = map[string]spec.StrategyType{"BLUE_GREEN": spec.StrategyTypeBlueGreen, "CANARY": spec.StrategyTypeCanary, "LINEAR": spec.StrategyTypeLinear}[strategy]
			raw.Deployments[0].ECS.Advanced = (*spec.ECSAdvancedSpec)(request.Intent.Advanced.Clone())
			raw.Deployments[0].ECS.MinimumHealthyPercent = nil
			raw.Deployments[0].ECS.MaximumPercent = nil
			raw.Deployments[0].ECS.SyncPolicy = &spec.ECSSyncPolicy{Mode: "automatic"}
			raw.Deployments[0].Image = request.Intent.Image
			compiled, err := manifest.Compile(raw)
			if err != nil {
				t.Fatal(err)
			}
			delete(backend.definitions, request.PriorResults[0].TaskDefinitionARN)
			backend.registrationFake.revisions = nil
			backend.registrationFake.writes = 0
			dir := t.TempDir()
			planPath := filepath.Join(dir, "plans.json")
			plans := planpreview.NewFileStore(planPath)
			now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
			enabled, authorized := true, true
			sampler := &nativeObservationFake{nativeFake: backend, sampler: &observationFake{serviceFake: backend.serviceFake, tasks: map[string]types.Task{}}, routing: ALBRoutingSnapshot{Signature: "verified", Production: ALBWeightPair{Primary: 100}}}
			service := applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), LoadPlan: plans.Get, EnableAWSApply: true, Now: func() time.Time { return now }, CurrentFingerprint: func(ctx context.Context, name string) (string, error) {
				detail, err := catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
				if err != nil {
					return "", err
				}
				return catalog.DeriveDeploymentSourceFingerprint(detail)
			}, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
				return request.Role, nil
			}, AuthorizeCreate: func(ctx context.Context, r applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
				id := requestactor.AuthenticatedIdentityFromContext(ctx)
				if !authorized || !enabled || !id.Authenticated || id.ID != "ecs-sync" || !applyrequest.AutomaticApprovalFromContext(ctx) || r.Source.Git == nil {
					return nil, applyrequest.ErrExecutionUnauthorized
				}
				return &applyrequest.ExecutionPrincipal{Automatic: true, Identity: id}, nil
			}}
			loop := ecsreconcile.Reconciler{Enabled: true, Automatic: true, LiveECS: true, AllowAdvanced: true, SubjectID: "ecs-sync", LoadCatalog: func(context.Context) (manifest.Catalog, error) { return compiled, nil }, Plans: plans, ListPlans: func(context.Context) ([]planpreview.Record, error) {
				rows, err := planpreview.Load(planPath)
				if errors.Is(err, os.ErrNotExist) {
					return nil, nil
				}
				return rows, err
			}, Applies: service}
			observer := NativeObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeObservationClient, error) {
				return sampler, nil
			}, CheckCapabilities: update.CheckCapabilities, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
				return sampler.routing, nil
			}, ObserveTargetHealth: func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
				e := NativeTargetHealthEvidence{}
				for _, task := range tasks {
					e.HealthyTaskARNs = append(e.HealthyTaskARNs, aws.ToString(task.TaskArn))
				}
				return e, nil
			}, Now: func() time.Time { return now }}
			executor := applyrequest.Executor{Service: service, WorkerID: "native-git", ECS: &applyrequest.ECSExecution{Enabled: true, AllowAdvanced: true, Authorize: func(_ context.Context, r applyrequest.Record) error {
				if !authorized || r.Principal == nil || r.Principal.Identity.ID != "ecs-sync" {
					return applyrequest.ErrExecutionUnauthorized
				}
				return nil
			}, AuthorizeMutation: func(context.Context, applyrequest.Record) error {
				if !enabled {
					return applyrequest.ErrExecutionUnauthorized
				}
				return nil
			}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) {
				return compiled.Deployments[0].ECS.Clone(), nil
			}, Handlers: map[string]applyrequest.ECSOperationHandler{
				string(reconcile.StepRegisterECSTaskDefinition): Registration{AllowAdvanced: true, ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
					return backend.registrationFake, nil
				}, CheckCapabilities: update.CheckCapabilities},
				string(reconcile.StepUpdateECSService): StrategyHandler{Advanced: update}, string(reconcile.StepVerifyServiceStability): StrategyHandler{Advanced: observer},
			}}}
			tick := func() {
				t.Helper()
				if err := loop.RunOnce(ctx); err != nil {
					t.Fatal(err)
				}
			}
			run := func(n int) {
				t.Helper()
				for i := 0; i < n; i++ {
					now = now.Add(31 * time.Second)
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
				}
			}
			rows := func() []applyrequest.Record {
				t.Helper()
				r, err := service.Store.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			current := func(id string) applyrequest.Record {
				t.Helper()
				r, err := service.Get(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			changed := func(sha string) {
				t.Helper()
				raw.Git.Revision = strings.Repeat(sha, 40)
				raw.Deployments[0].Image = "example/worker@sha256:" + strings.Repeat(sha, 64)
				var err error
				compiled, err = manifest.Compile(raw)
				if err != nil {
					t.Fatal(err)
				}
			}
			healthy := func(r applyrequest.Record, rollback bool) {
				t.Helper()
				result := r.Operations[1].ECSCheckpoint.Result
				d := backend.deployments[result.ServiceDeploymentARN]
				task, revision, group := result.TaskDefinitionARN, result.ServiceRevisionARN, request.Intent.Advanced.Routing.AlternateTargetGroupARN
				if backend.sourceOverride != "" {
					source := backend.revisions[backend.sourceOverride]
					if source.ResolvedConfiguration != nil && aws.ToString(source.ResolvedConfiguration.LoadBalancers[0].TargetGroupArn) == group {
						group = request.Intent.Advanced.Routing.TargetGroupARN
					}
				}
				if rollback {
					task, revision = result.PreviousTaskDefinitionARN, result.PreviousServiceRevisionARN
					source := backend.revisions[revision]
					group = aws.ToString(source.ResolvedConfiguration.LoadBalancers[0].TargetGroupArn)
					d.Status = types.ServiceDeploymentStatusRollbackSuccessful
					d.Rollback = &types.Rollback{ServiceRevisionArn: aws.String(revision)}
					d.TargetServiceRevision.RunningTaskCount = 0
					d.SourceServiceRevisions[0].RunningTaskCount = int32(request.Intent.DesiredCount)
				} else {
					d.Status = types.ServiceDeploymentStatusSuccessful
					d.TargetServiceRevision.RunningTaskCount = int32(request.Intent.DesiredCount)
					d.SourceServiceRevisions[0].RunningTaskCount = 0
				}
				backend.deployments[result.ServiceDeploymentARN] = d
				for i := range backend.history {
					if aws.ToString(backend.history[i].ServiceDeploymentArn) == result.ServiceDeploymentARN {
						backend.history[i].Status = d.Status
					}
				}
				ref := backend.revisions[revision]
				ref.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{TargetGroupArn: aws.String(group), ProductionListenerRule: aws.String(request.Intent.Advanced.Routing.ProductionListenerRuleARN)}}}
				backend.revisions[revision] = ref
				backend.service.TaskDefinition = aws.String(task)
				backend.service.RunningCount = int32(request.Intent.DesiredCount)
				backend.service.PendingCount = 0
				sampler.routing.Production = ALBWeightPair{Alternate: 100}
				if group == request.Intent.Advanced.Routing.TargetGroupARN {
					sampler.routing.Production = ALBWeightPair{Primary: 100}
				}
				sampler.sampler.running = nil
				sampler.sampler.tasks = map[string]types.Task{}
				for i := 0; i < request.Intent.DesiredCount; i++ {
					key := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/git-%d", i)
					sampler.sampler.running = append(sampler.sampler.running, key)
					sampler.sampler.tasks[key] = types.Task{TaskArn: aws.String(key), ClusterArn: aws.String(request.Intent.ClusterARN), Group: aws.String("service:worker"), TaskDefinitionArn: aws.String(task), LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
				}
				backend.sourceOverride = revision
			}
			tick()
			tick()
			first := rows()[0]
			run(2)
			accepted := current(first.ID)
			if backend.writes != 1 || backend.registrationFake.writes != 1 || !accepted.Principal.Automatic {
				t.Fatal("first Git commit was not executed once")
			}
			// A newer commit waits while the accepted older deployment bakes/observes.
			changed("b")
			tick()
			if len(rows()) != 1 {
				t.Fatal("new Git interrupted an accepted traffic deployment")
			}
			enabled = false
			loop.Automatic = false
			healthy(accepted, false)
			run(1)
			if current(first.ID).Status != applyrequest.StatusSucceeded {
				t.Fatal("paused automation blocked read-only completion")
			}
			tick()
			if len(rows()) != 1 {
				t.Fatal("paused automation queued new work")
			}
			enabled = true
			loop.Automatic = true
			tick()
			second := rows()[0]
			if second.ID == first.ID {
				second = rows()[1]
			}
			run(1)
			backend.updateError = errors.New("update accepted but response lost")
			run(1)
			backend.updateError = nil
			if backend.writes != 2 {
				t.Fatal("second Git commit did not reach native update")
			}
			executor.WorkerID = "native-git-restarted"
			now = now.Add(6 * time.Minute)
			enabled = false
			// Uncertain accepted writes reconcile without another mutation, even paused.
			run(1)
			enabled = true
			healthy(current(second.ID), true)
			run(1)
			failed := current(second.ID)
			if failed.Status != applyrequest.StatusFailed || failed.ECSRuntime.Reason != "native_rollback_reported" || backend.writes != 2 {
				t.Fatal("rollback was hidden as forward success or update was replayed", failed)
			}
			raw.Git.Revision = strings.Repeat("c", 40)
			compiled, err = manifest.Compile(raw)
			if err != nil {
				t.Fatal(err)
			}
			tick()
			if len(rows()) != 2 {
				t.Fatal("documentation-only commit bypassed failed native intent hold")
			}
			changed("d")
			tick()
			var fixed applyrequest.Record
			for _, r := range rows() {
				if r.Source.Git.Revision == strings.Repeat("d", 40) {
					fixed = r
				}
			}
			authorized = false
			if err := executor.RunOnce(ctx); err == nil {
				t.Fatal("revoked RBAC started fixed commit")
			}
			if backend.writes != 2 || backend.registrationFake.writes != 2 {
				t.Fatal("revoked RBAC mutated AWS")
			}
			authorized = true
			run(2)
			healthy(current(fixed.ID), false)
			run(1)
			tick()
			tick()
			if current(fixed.ID).Status != applyrequest.StatusSucceeded || len(rows()) != 3 || backend.writes != 3 || backend.registrationFake.writes != 3 {
				t.Fatal("new reviewed recovery commit did not converge once")
			}
		})
	}
}
