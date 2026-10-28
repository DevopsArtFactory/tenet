package ecsexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func TestNativeAPIExecutorPipelineHandsOffAndRecoversAllStrategies(t *testing.T) {
	type scenario struct {
		count     int
		mode      string
		automatic bool
	}
	var scenarios []scenario
	for _, count := range []int{1, 2, 4} {
		for _, mode := range []string{"accepted", "lost-response", "ecs-already-rolling-back", "finished-before-stop", "capacity-lost-response", "capacity-rejected", "capacity-already-restored"} {
			if count != 2 || !strings.HasPrefix(mode, "capacity-") {
				scenarios = append(scenarios, scenario{count: count, mode: mode})
			}
		}
	}
	for _, count := range []int{1, 4} {
		for _, mode := range []string{"accepted", "lost-response", "ecs-already-rolling-back", "capacity-lost-response", "git-accepted"} {
			scenarios = append(scenarios, scenario{count, mode, true})
		}
	}
	scenarios = append(scenarios, scenario{2, "accepted", true})

	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		for _, test := range scenarios {
			count, mode := test.count, test.mode
			lostResponse := mode == "lost-response"
			expectedStops := 1
			if mode == "ecs-already-rolling-back" {
				expectedStops = 0
			}
			t.Run(fmt.Sprintf("%s/%d/%s/automatic=%v", strategy, count, mode, test.automatic), func(t *testing.T) {
				ctx := context.Background()
				backend, update, r := nativeFixture(t, strategy)
				r.Intent.DesiredCount = count
				r.Intent.RollbackOnFailure = test.automatic
				if mode == "git-accepted" {
					r.Intent.SyncMode = "automatic"
				}
				// Exercise actual registration rather than preloading its result.
				delete(backend.definitions, r.PriorResults[0].TaskDefinitionARN)
				backend.registrationFake.revisions = nil
				backend.registrationFake.writes = 0
				detail := catalog.DeploymentOverviewDetail{Name: "worker", TargetType: "ecs", Strategy: strings.ToLower(strategy), ExecutionMode: "ecsNative", ECS: r.Intent.Clone(), DesiredArtifact: domain.DeploymentArtifact{Image: r.Intent.Image}}
				if mode == "git-accepted" {
					detail.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
				}
				dir := t.TempDir()
				plans := planpreview.Service{Store: planpreview.NewFileStore(filepath.Join(dir, "plans.json")), LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return detail, nil }}
				plan, err := plans.Create(ctx, "worker", planpreview.CreateRequest{})
				if err != nil {
					t.Fatal(err)
				}
				now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
				actor := requestactor.AuthenticatedIdentity{ID: "operator", Authenticated: true}
				role := r.Role
				stopped := &nativeRollbackFake{nativeFake: backend}
				capacity := &nativeCapacityFake{nativeRollbackFake: stopped}
				stopHandler := NativeRollback{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeRollbackClient, error) {
					return capacity, nil
				}, CheckCapabilities: update.CheckCapabilities}
				allowed := true
				service := applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), LoadPlan: plans.Get, EnableAWSApply: true, Now: func() time.Time { return now }, CurrentFingerprint: func(context.Context, string) (string, error) { return plan.Record.Source.Fingerprint, nil }, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
					return role, nil
				}, AuthorizeCreate: func(ctx context.Context, _ applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
					identity := requestactor.AuthenticatedIdentityFromContext(ctx)
					if !allowed || !identity.Authenticated {
						return nil, applyrequest.ErrExecutionUnauthorized
					}
					return &applyrequest.ExecutionPrincipal{Identity: identity, Automatic: applyrequest.AutomaticApprovalFromContext(ctx)}, nil
				}}
				service.PrepareNativeRollback = func(ctx context.Context, parent applyrequest.Record, target applyrequest.ECSNativeRollback) error {
					_, err := stopHandler.Prepare(ctx, applyrequest.ECSOperationRequest{Intent: *parent.Operations[0].ECS, Role: role}, target)
					return err
				}
				obsBackend := &nativeObservationFake{nativeFake: backend, sampler: &observationFake{serviceFake: backend.serviceFake, tasks: map[string]types.Task{}}, routing: ALBRoutingSnapshot{Signature: "current", Production: ALBWeightPair{Primary: 95, Alternate: 5}}}
				observer := NativeObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeObservationClient, error) {
					return obsBackend, nil
				}, CheckCapabilities: update.CheckCapabilities, ObserveRouting: func(context.Context, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
					return obsBackend.routing, nil
				}, ObserveTargetHealth: func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
					e := NativeTargetHealthEvidence{}
					for _, task := range tasks {
						e.HealthyTaskARNs = append(e.HealthyTaskARNs, aws.ToString(task.TaskArn))
					}
					return e, nil
				}, Now: func() time.Time { return now }}
				executor := applyrequest.Executor{Service: service, WorkerID: "native-first", ECS: &applyrequest.ECSExecution{Enabled: true, AllowAdvanced: true, Authorize: func(context.Context, applyrequest.Record) error {
					if !allowed {
						return applyrequest.ErrExecutionUnauthorized
					}
					return nil
				}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) { return detail.ECS.Clone(), nil }, Handlers: map[string]applyrequest.ECSOperationHandler{
					string(reconcile.StepRegisterECSTaskDefinition): Registration{AllowAdvanced: true, ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
						return backend.registrationFake, nil
					}, CheckCapabilities: update.CheckCapabilities},
					string(reconcile.StepUpdateECSService):          StrategyHandler{Advanced: update},
					string(reconcile.StepVerifyServiceStability):    StrategyHandler{Advanced: observer},
					applyrequest.NativeRollbackOperationKind:        NativeRollbackOperation{NativeRollback: stopHandler},
					applyrequest.NativeCapacityRestoreOperationKind: NativeCapacityRestore{NativeRollback: stopHandler, ObserveRouting: observer.ObserveRouting, Now: func() time.Time { return now }},
				}}}
				router := api.NewRouter(api.Options{TrustAuthenticatedHeaders: true, RequireAuthenticatedIdentity: true, RequireOperatorRoleForWrite: true, RequireDeploymentReadAuthorization: true, CreateApplyRequest: service.Create, ApplyRequest: service.Get, RequestApplyRollback: service.RequestRollback})
				post := func(url string) *httptest.ResponseRecorder {
					request := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"taskDefinitionArn":"caller-cannot-select-source","desiredCount":99}`))
					request.Header.Set(requestactor.AuthenticatedHeaderID, actor.ID)
					request.Header.Set(requestactor.AuthenticatedRolesHeader, "operator")
					request.Header.Set(requestactor.AuthorizedDeploymentsHeader, "worker")
					out := httptest.NewRecorder()
					router.ServeHTTP(out, request)
					return out
				}
				approvalContext := requestactor.WithAuthenticatedIdentityContext(ctx, actor)
				if mode == "git-accepted" {
					approvalContext = applyrequest.WithAutomaticApproval(approvalContext)
				}
				created, err := service.Create(approvalContext, plan.Record.ID)
				if err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
				}
				parent, err := service.Get(ctx, created.Record.ID)
				if err != nil {
					t.Fatal(err)
				}
				if parent.Status != applyrequest.StatusRunning || backend.writes != 1 || backend.registrationFake.writes != 1 || parent.Operations[1].ECSCheckpoint == nil {
					t.Fatalf("real native forward did not reach accepted update: %+v", parent)
				}
				before := parent.Operations[1].ECSCheckpoint.Result
				if mode == "ecs-already-rolling-back" {
					d := backend.deployments[before.ServiceDeploymentARN]
					d.Status = types.ServiceDeploymentStatusRollbackInProgress
					d.Rollback = &types.Rollback{ServiceRevisionArn: aws.String(before.PreviousServiceRevisionARN)}
					backend.deployments[before.ServiceDeploymentARN] = d
				}

				var child applyrequest.Record
				if test.automatic {
					if mode != "ecs-already-rolling-back" {
						d := backend.deployments[before.ServiceDeploymentARN]
						d.Alarms = &types.ServiceDeploymentAlarms{Status: types.ServiceDeploymentRollbackMonitorsStatusTriggered, TriggeredAlarmNames: []string{r.Intent.Advanced.Alarms.Names[0]}}
						backend.deployments[before.ServiceDeploymentARN] = d
					}
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
					parent, err = service.Get(ctx, parent.ID)
					if err != nil || parent.Status != applyrequest.StatusFailed || parent.ECSRuntime == nil {
						t.Fatal("native failure signal did not preserve failed forward", parent, err)
					}
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
				} else {
					response := post("/v1/applies/" + parent.ID + "/rollback")
					if response.Code != http.StatusCreated {
						t.Fatalf("rollback API: %d %s", response.Code, response.Body.String())
					}
					if err := json.Unmarshal(response.Body.Bytes(), &child); err != nil {
						t.Fatal(err)
					}
				}
				// API response may use a wrapper; locate the authoritative child via store.
				all, err := service.Store.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, record := range all {
					if record.NativeRollback != nil {
						child = record
					}
				}
				if child.NativeRollback == nil || child.NativeRollback.Automatic != test.automatic || !reflect.DeepEqual(child.Principal, parent.Principal) && test.automatic || child.NativeRollback.SourceRevisionARN != before.PreviousServiceRevisionARN || child.NativeRollback.SourceTaskDefinitionARN != before.PreviousTaskDefinitionARN {
					t.Fatal("API accepted caller source or lost pre-write source evidence")
				}
				again := post("/v1/applies/" + parent.ID + "/rollback")
				if again.Code != http.StatusOK {
					t.Fatalf("duplicate recovery %d %s", again.Code, again.Body.String())
				}
				parent, _ = service.Get(ctx, parent.ID)
				expectedParentStatus := applyrequest.StatusCancelled
				if test.automatic {
					expectedParentStatus = applyrequest.StatusFailed
				}
				if parent.Status != expectedParentStatus || parent.NativeRollbackRequestID != child.ID {
					t.Fatal("forward ownership did not transfer")
				}
				if mode == "finished-before-stop" {
					d := backend.deployments[before.ServiceDeploymentARN]
					d.Status = types.ServiceDeploymentStatusSuccessful
					backend.deployments[before.ServiceDeploymentARN] = d
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
					failed, err := service.Get(ctx, child.ID)
					if err != nil || failed.Status != applyrequest.StatusFailed || stopped.stops != 0 || !strings.Contains(failed.Error, "no stop was sent") {
						t.Fatal("finished forward rollout left a stuck recovery owner", failed, err)
					}
					return
				}
				if lostResponse {
					stopped.stopError = errors.New("accepted but response lost")
				}
				if err := executor.RunOnce(ctx); err != nil && !lostResponse {
					t.Fatal(err)
				}
				if stopped.stops != expectedStops {
					t.Fatal("native recovery did not send exactly one stop")
				}
				// Restart against JSON state and current authorization, without resending Stop.
				executor.WorkerID = "native-restarted"
				now = now.Add(6 * time.Minute)
				allowed = false
				if err := executor.RunOnce(ctx); err == nil {
					t.Fatal("revoked operator resumed recovery")
				}
				allowed = true
				stopped.stopError = nil
				for i := 0; i < 2; i++ {
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
				}
				observing, err := service.Get(ctx, child.ID)
				if err != nil {
					t.Fatal(err)
				}
				if observing.Status != applyrequest.StatusRunning || observing.ECSRuntime == nil || observing.ECSRuntime.State == "converged" || stopped.stops != expectedStops {
					t.Fatalf("stop acceptance became recovered health: %+v", observing)
				}
				deadline := observing.ECSRuntime.DeadlineAt
				target := *child.NativeRollback
				d := backend.deployments[target.DeploymentARN]
				d.Status = types.ServiceDeploymentStatusRollbackSuccessful
				d.TargetServiceRevision.RunningTaskCount = 0
				d.SourceServiceRevisions[0].RunningTaskCount = target.DesiredCount
				backend.deployments[target.DeploymentARN] = d
				source := backend.revisions[target.SourceRevisionARN]
				source.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{ProductionListenerRule: aws.String(r.Intent.Advanced.Routing.ProductionListenerRuleARN), TargetGroupArn: aws.String(r.Intent.Advanced.Routing.TargetGroupARN)}}}
				backend.revisions[target.SourceRevisionARN] = source
				backend.service.TaskDefinition = aws.String(target.SourceTaskDefinitionARN)
				backend.service.RunningCount = target.DesiredCount
				backend.service.PendingCount = 0
				obsBackend.routing.Production = ALBWeightPair{Primary: 100}
				for i := int32(0); i < target.DesiredCount; i++ {
					key := "arn:aws:ecs:ap-northeast-2:123456789012:task/production/recovery-" + string(rune('a'+i))
					obsBackend.sampler.running = append(obsBackend.sampler.running, key)
					obsBackend.sampler.tasks[key] = types.Task{TaskArn: aws.String("arn:aws:ecs:ap-northeast-2:123456789012:task/production/recovery-" + string(rune('a'+i))), ClusterArn: aws.String(r.Intent.ClusterARN), TaskDefinitionArn: aws.String(target.SourceTaskDefinitionARN), Group: aws.String("service:worker"), LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
				}
				now = now.Add(time.Minute)
				expectedCapacityCalls := 0
				if count != 2 {
					expectedCapacityCalls = 1
					if mode == "capacity-already-restored" {
						backend.service.DesiredCount = 2
						expectedCapacityCalls = 0
					}
					if mode == "capacity-lost-response" {
						capacity.capacityError = errors.New("capacity accepted but response lost")
					}
					if mode == "capacity-rejected" {
						capacity.noCapacityEffect = true
						capacity.capacityError = &types.ClientException{Message: aws.String("confirmed capacity rejection")}
					}
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
					if mode == "capacity-rejected" {
						failed, err := service.Get(ctx, child.ID)
						if err != nil || failed.Status != applyrequest.StatusFailed || capacity.capacityCalls != 1 || stopped.stops != expectedStops {
							t.Fatal("rejected capacity recovery was hidden or repeated", failed, err)
						}
						return
					}
					if mode == "capacity-lost-response" {
						capacity.capacityError = nil
						executor.WorkerID = "capacity-restarted"
						now = now.Add(6 * time.Minute)
						if err := executor.RunOnce(ctx); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := executor.RunOnce(ctx); err != nil {
					t.Fatal(err)
				}
				done, err := service.Get(ctx, child.ID)
				if err != nil || done.Status != applyrequest.StatusSucceeded || done.ECSRuntime.Reason != "native_source_recovered" || done.ECSRuntime.DeadlineAt != deadline || done.ECSRuntime.DesiredCount != 2 || backend.service.DesiredCount != 2 || capacity.capacityCalls != expectedCapacityCalls || stopped.stops != expectedStops || backend.writes != 1 || backend.registrationFake.writes != 1 {
					t.Fatalf("native recovery pipeline did not converge without extra writes: %+v %v", done, err)
				}
				if test.automatic {
					if err := executor.RunOnce(ctx); err != nil {
						t.Fatal(err)
					}
					rows, err := service.Store.List(ctx)
					failed, loadErr := service.Get(ctx, parent.ID)
					if err != nil || loadErr != nil || len(rows) != 2 || failed.Status != applyrequest.StatusFailed || !reflect.DeepEqual(failed.Operations[1].ECSCheckpoint.Result, before) {
						t.Fatal("automatic recovery looped or rewrote forward evidence", rows, err)
					}
				}
			})
		}
	}
}
