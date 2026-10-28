package ecsexecution

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
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

type nativeHealPipeline struct {
	f                   *nativeObservationFake
	probe               NativeManagedDrift
	req                 applyrequest.ECSOperationRequest
	service             applyrequest.Service
	executor            applyrequest.Executor
	loop                ecsreconcile.Reconciler
	catalog             manifest.Catalog
	raw                 manifest.CatalogSpec
	now                 time.Time
	enabled, authorized bool
	parent              string
}

func setupNativeHealPipeline(t *testing.T, strategy string, heal bool) *nativeHealPipeline {
	t.Helper()
	f, probe, req, verified := nativeDriftFixture(t, strategy)
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	raw.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
	raw.Deployments[0].Strategy.Type = map[string]spec.StrategyType{"BLUE_GREEN": spec.StrategyTypeBlueGreen, "CANARY": spec.StrategyTypeCanary, "LINEAR": spec.StrategyTypeLinear}[strategy]
	raw.Deployments[0].Image = req.Intent.Image
	raw.Deployments[0].ECS.Advanced = (*spec.ECSAdvancedSpec)(req.Intent.Advanced.Clone())
	raw.Deployments[0].ECS.MinimumHealthyPercent = nil
	raw.Deployments[0].ECS.MaximumPercent = nil
	raw.Deployments[0].ECS.SyncPolicy = &spec.ECSSyncPolicy{Mode: "automatic", SelfHeal: heal}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	req.Intent.SelfHeal = heal
	if !reflect.DeepEqual(compiled.Deployments[0].ECS, &req.Intent) {
		t.Fatalf("fixture/catalog intents differ: %+v %+v", compiled.Deployments[0].ECS, req.Intent)
	}
	p := &nativeHealPipeline{f: f, probe: probe, req: req, catalog: compiled, raw: raw, now: time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC), enabled: true, authorized: true}
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plans.json")
	plans := planpreview.NewFileStore(planPath)
	detail := func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
		return catalog.BuildDeploymentOverviewDetail(p.catalog, nil, name)
	}
	planService := planpreview.Service{Store: plans, LoadDetail: detail}
	p.service = applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), LoadPlan: plans.Get, EnableAWSApply: true, Now: func() time.Time { return p.now }, CurrentFingerprint: func(ctx context.Context, name string) (string, error) {
		d, err := detail(ctx, name)
		if err != nil {
			return "", err
		}
		return catalog.DeriveDeploymentSourceFingerprint(d)
	}, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		return req.Role, nil
	}, AuthorizeCreate: func(ctx context.Context, _ applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
		id := requestactor.AuthenticatedIdentityFromContext(ctx)
		if !p.authorized || !id.Authenticated || id.ID != "ecs-sync" {
			return nil, applyrequest.ErrExecutionUnauthorized
		}
		return &applyrequest.ExecutionPrincipal{Identity: id, Automatic: applyrequest.AutomaticApprovalFromContext(ctx)}, nil
	}}
	plan, err := planService.Create(context.Background(), "worker", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := applyrequest.WithAutomaticApproval(requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{ID: "ecs-sync", Authenticated: true}))
	first, err := p.service.Create(ctx, plan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.service.Store.(applyrequest.MutationStore).Mutate(ctx, first.Record.ID, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.Status = applyrequest.StatusSucceeded
		r.ECSRuntime = verified.ECSRuntime.Clone()
		for i := range r.Operations {
			r.Operations[i].Status = applyrequest.OperationStatusSucceeded
			cp := *verified.Operations[i].ECSCheckpoint
			cp.Result = cp.Result.Clone()
			r.Operations[i].ECSCheckpoint = &cp
		}
		return r, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p.parent = first.Record.ID
	p.loop = ecsreconcile.Reconciler{Enabled: true, Automatic: true, LiveECS: true, AllowAdvanced: true, SubjectID: "ecs-sync", LoadCatalog: func(context.Context) (manifest.Catalog, error) { return p.catalog, nil }, Plans: plans, ListPlans: func(context.Context) ([]planpreview.Record, error) { return planpreview.Load(planPath) }, Applies: p.service, ObserveDrift: func(ctx context.Context, d domain.Deployment, r applyrequest.Record) (ecsreconcile.Drift, error) {
		found, sig, err := p.probe.Observe(ctx, *d.ECS, req.Role, r)
		return ecsreconcile.Drift{Detected: found, Signature: sig}, err
	}}
	update := AdvancedService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeServiceClient, error) {
		return f.nativeFake, nil
	}, CheckCapabilities: func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}, ObserveRouting: probe.ObserveRouting}
	observer := NativeObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (NativeObservationClient, error) {
		return f, nil
	}, CheckCapabilities: probe.CheckCapabilities, ObserveRouting: probe.ObserveRouting, ObserveTargetHealth: func(_ context.Context, _ domain.ECSDeploymentIntent, _ applyrequest.DeployerRoleReadiness, _ string, tasks []types.Task) (NativeTargetHealthEvidence, error) {
		e := NativeTargetHealthEvidence{}
		for _, task := range tasks {
			e.HealthyTaskARNs = append(e.HealthyTaskARNs, aws.ToString(task.TaskArn))
		}
		return e, nil
	}, Now: func() time.Time { return p.now }}
	p.executor = applyrequest.Executor{Service: p.service, WorkerID: "native-heal", ECS: &applyrequest.ECSExecution{Enabled: true, AllowAdvanced: true, Authorize: func(context.Context, applyrequest.Record) error {
		if !p.authorized {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}, AuthorizeMutation: func(context.Context, applyrequest.Record) error {
		if !p.enabled {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) {
		return p.catalog.Deployments[0].ECS.Clone(), nil
	}, Handlers: map[string]applyrequest.ECSOperationHandler{
		string(reconcile.StepRegisterECSTaskDefinition): Registration{AllowAdvanced: true, ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
			return f.registrationFake, nil
		}, CheckCapabilities: update.CheckCapabilities},
		string(reconcile.StepUpdateECSService): update, string(reconcile.StepVerifyServiceStability): observer,
	}}}
	p.f.sourceOverride = verified.Operations[1].ECSCheckpoint.Result.ServiceRevisionARN
	return p
}
func (p *nativeHealPipeline) tick(t *testing.T) {
	t.Helper()
	if err := p.loop.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func (p *nativeHealPipeline) rows(t *testing.T) []applyrequest.Record {
	t.Helper()
	rows, err := p.service.Store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func (p *nativeHealPipeline) run(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		p.now = p.now.Add(31 * time.Second)
		if err := p.executor.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
func (p *nativeHealPipeline) healthy(t *testing.T, id string) {
	t.Helper()
	r, err := p.service.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	result := r.Operations[1].ECSCheckpoint.Result
	d := p.f.deployments[result.ServiceDeploymentARN]
	d.Status = types.ServiceDeploymentStatusSuccessful
	d.TargetServiceRevision.RunningTaskCount = int32(r.Operations[0].ECS.DesiredCount)
	for i := range d.SourceServiceRevisions {
		d.SourceServiceRevisions[i].RunningTaskCount = 0
		d.SourceServiceRevisions[i].PendingTaskCount = 0
	}
	p.f.deployments[result.ServiceDeploymentARN] = d
	for i := range p.f.history {
		if aws.ToString(p.f.history[i].ServiceDeploymentArn) == result.ServiceDeploymentARN {
			p.f.history[i].Status = d.Status
		}
	}
	group := p.req.Intent.Advanced.Routing.TargetGroupARN
	if p.f.routing.Production.Primary == 100 {
		group = p.req.Intent.Advanced.Routing.AlternateTargetGroupARN
	}
	ref := p.f.revisions[result.ServiceRevisionARN]
	ref.ResolvedConfiguration = &types.ResolvedConfiguration{LoadBalancers: []types.ServiceRevisionLoadBalancer{{TargetGroupArn: aws.String(group), ProductionListenerRule: aws.String(p.req.Intent.Advanced.Routing.ProductionListenerRuleARN)}}}
	p.f.revisions[result.ServiceRevisionARN] = ref
	p.f.routing.Production = ALBWeightPair{Primary: 100}
	if group == p.req.Intent.Advanced.Routing.AlternateTargetGroupARN {
		p.f.routing.Production = ALBWeightPair{Alternate: 100}
	}
	p.f.service.RunningCount = int32(r.Operations[0].ECS.DesiredCount)
	p.f.service.PendingCount = 0
	p.f.sampler.running = nil
	p.f.sampler.tasks = map[string]types.Task{}
	for i := 0; i < r.Operations[0].ECS.DesiredCount; i++ {
		key := fmt.Sprintf("arn:aws:ecs:ap-northeast-2:123456789012:task/production/heal-%d-%d", p.f.writes, i)
		p.f.sampler.running = append(p.f.sampler.running, key)
		p.f.sampler.tasks[key] = types.Task{TaskArn: aws.String(key), ClusterArn: aws.String(p.req.Intent.ClusterARN), Group: aws.String("service:worker"), TaskDefinitionArn: aws.String(result.TaskDefinitionARN), LastStatus: aws.String("RUNNING"), Containers: []types.Container{{Name: aws.String("worker"), LastStatus: aws.String("RUNNING")}}}
	}
	p.f.sourceOverride = result.ServiceRevisionARN
}
func TestNativeSelfHealRepairsCountAndTrafficOncePerVerifiedEpoch(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			p := setupNativeHealPipeline(t, strategy, true)
			p.tick(t)
			if len(p.rows(t)) != 1 {
				t.Fatal("healthy service queued repair")
			}
			for epoch := 0; epoch < 2; epoch++ {
				before := p.f.writes
				previous := p.parent
				p.f.service.DesiredCount = 3
				p.f.routing.Production = ALBWeightPair{Primary: 20, Alternate: 80}
				p.tick(t)
				p.tick(t)
				rows := p.rows(t)
				if len(rows) != epoch+2 {
					t.Fatal("repeated drift poll queued duplicates")
				}
				var repair applyrequest.Record
				for _, r := range rows {
					if r.Status == applyrequest.StatusQueued {
						repair = r
					}
				}
				if repair.Source.LastVerifiedApplyID != previous {
					t.Fatal("repair did not bind latest verified epoch", repair)
				}
				p.run(t, 2)
				if p.f.writes != before+1 || p.f.input.DesiredCount == nil || *p.f.input.DesiredCount != 2 || !p.f.input.ForceNewDeployment {
					t.Fatal("native self-heal did not restore capacity with one fresh deployment")
				}
				p.executor.WorkerID = "native-heal-restarted"
				p.now = p.now.Add(6 * time.Minute)
				p.healthy(t, repair.ID)
				p.enabled = false
				p.run(t, 1)
				p.enabled = true
				done, err := p.service.Get(context.Background(), repair.ID)
				if err != nil || done.Status != applyrequest.StatusSucceeded || done.ECSRuntime.State != "converged" {
					t.Fatal("repair did not verify current task/ALB health", done, err)
				}
				p.parent = done.ID
				p.tick(t)
				if len(p.rows(t)) != epoch+2 {
					t.Fatal("verified repair looped")
				}
			}
		})
	}
}
func TestNativeSelfHealOffUnavailableAndRevokedCannotWrite(t *testing.T) {
	for _, name := range []string{"off", "active external", "pending", "unavailable", "RBAC revoked", "paused", "drift disappeared"} {
		t.Run(name, func(t *testing.T) {
			p := setupNativeHealPipeline(t, "CANARY", name != "off")
			p.f.service.DesiredCount = 3
			switch name {
			case "active external":
				p.f.history[len(p.f.history)-1].Status = types.ServiceDeploymentStatusInProgress
			case "pending":
				p.f.service.PendingCount = 1
			case "unavailable":
				p.f.readError = errors.New("denied")
			}
			err := p.loop.RunOnce(context.Background())
			if name == "active external" || name == "pending" || name == "unavailable" {
				if err == nil || len(p.rows(t)) != 1 || p.f.writes != 1 {
					t.Fatal("unavailable drift authorized repair", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "off" {
				if len(p.rows(t)) != 1 || p.f.writes != 1 {
					t.Fatal("default-off self-heal changed AWS")
				}
				return
			}
			if name == "drift disappeared" {
				p.f.service.DesiredCount = 2
				p.run(t, 3)
				if p.f.writes != 1 {
					t.Fatal("resolved drift forced another deployment")
				}
				return
			}
			if name == "paused" {
				p.enabled = false
			} else {
				p.authorized = false
			}
			if err := p.executor.RunOnce(context.Background()); err == nil {
				t.Fatal("untrusted repair executed")
			}
			if p.f.writes != 1 {
				t.Fatal("disabled/unauthorized repair wrote AWS")
			}
		})
	}
}

func TestNativeRepairAuthorityRejectsUnverifiedPredecessors(t *testing.T) {
	p := setupNativeHealPipeline(t, "CANARY", true)
	p.f.service.DesiredCount = 3
	p.tick(t)
	if _, err := p.service.Store.(applyrequest.MutationStore).Mutate(context.Background(), p.parent, func(r applyrequest.Record) (applyrequest.Record, error) {
		r.ECSRuntime.TasksObserved = false
		return r, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.executor.RunOnce(context.Background()); err == nil || p.f.writes != 1 {
		t.Fatal("unverified predecessor authorized repair", err)
	}
}

func TestNativeRepairExplicitRetryRetainsDriftAuthorityAndFailureHold(t *testing.T) {
	p := setupNativeHealPipeline(t, "LINEAR", true)
	p.f.service.DesiredCount = 3
	p.tick(t)
	var repair applyrequest.Record
	for _, r := range p.rows(t) {
		if r.Status == applyrequest.StatusQueued {
			repair = r
		}
	}
	p.f.noEffect = true
	p.f.updateError = &types.ClientException{Message: aws.String("fixture confirmed rejection")}
	p.run(t, 2)
	failed, err := p.service.Get(context.Background(), repair.ID)
	if err != nil || failed.Status != applyrequest.StatusFailed {
		t.Fatal("repair failure did not stay separate", failed, err)
	}
	p.tick(t)
	if len(p.rows(t)) != 2 {
		t.Fatal("failed repair retried without approval")
	}
	p.f.updateError = nil
	p.f.noEffect = false
	p.service.CreateRetryPlan = func(ctx context.Context, name string) (planpreview.Record, error) {
		d, err := catalog.BuildDeploymentOverviewDetail(p.catalog, nil, name)
		if err != nil {
			return planpreview.Record{}, err
		}
		s := planpreview.Service{Store: p.loop.Plans, LoadDetail: func(context.Context, string) (catalog.DeploymentOverviewDetail, error) { return d, nil }}
		r, err := s.Create(ctx, name, planpreview.CreateRequest{})
		return r.Record, err
	}
	p.executor.Service = p.service
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{ID: "ecs-sync", Authenticated: true})
	retry, err := p.service.Retry(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.run(t, 2)
	p.healthy(t, retry.Record.ID)
	p.run(t, 1)
	done, err := p.service.Get(ctx, retry.Record.ID)
	if err != nil || done.Status != applyrequest.StatusSucceeded || done.RetryOf != failed.ID || p.f.service.DesiredCount != 2 {
		t.Fatal("explicit retry lost managed capacity authority", done, err)
	}
}

func TestNativeSelfHealCanRestoreVerifiedCapacityAlongsideNewGitImage(t *testing.T) {
	p := setupNativeHealPipeline(t, "CANARY", false)
	// Opt in after a previously verified manual/no-heal goal, without changing its capacity.
	p.catalog.Git.Revision = strings.Repeat("b", 40)
	intent := p.catalog.Deployments[0].ECS
	intent.SelfHeal = true
	intent.Image = "example/worker@sha256:" + strings.Repeat("b", 64)
	p.catalog.Deployments[0].DesiredArtifact.Image = intent.Image
	p.f.service.DesiredCount = 3
	p.tick(t)
	var approved applyrequest.Record
	for _, r := range p.rows(t) {
		if r.Status == applyrequest.StatusQueued {
			approved = r
		}
	}
	p.run(t, 2)
	if p.f.input.DesiredCount == nil || *p.f.input.DesiredCount != 2 || p.f.service.DesiredCount != 2 {
		t.Fatal("new Git image lost verified-capacity repair authority")
	}
	p.healthy(t, approved.ID)
	p.run(t, 1)
	done, err := p.service.Get(context.Background(), approved.ID)
	if err != nil || done.Status != applyrequest.StatusSucceeded {
		t.Fatal("new image and drift repair did not converge", done, err)
	}
}

func TestNativeSelfHealRestoresReviewedPolicyThroughNativeDeployment(t *testing.T) {
	for _, strategy := range []string{"BLUE_GREEN", "CANARY", "LINEAR"} {
		t.Run(strategy, func(t *testing.T) {
			p := setupNativeHealPipeline(t, strategy, true)
			p.f.service.DeploymentConfiguration.BakeTimeInMinutes = aws.Int32(29)
			p.tick(t)
			var repair applyrequest.Record
			for _, r := range p.rows(t) {
				if r.Status == applyrequest.StatusQueued {
					repair = r
				}
			}
			if repair.ID == "" {
				t.Fatal("policy drift did not queue a repair")
			}
			before := p.f.writes
			p.run(t, 2)
			if p.f.writes != before+1 || !p.f.input.ForceNewDeployment || p.f.input.DesiredCount != nil || !advancedConfigMatches(p.f.input.DeploymentConfiguration, p.req.Intent) {
				t.Fatal("policy repair did not submit the reviewed configuration through ECS")
			}
			p.healthy(t, repair.ID)
			p.run(t, 1)
			done, err := p.service.Get(context.Background(), repair.ID)
			if err != nil || done.Status != applyrequest.StatusSucceeded {
				t.Fatal(done, err)
			}
			p.tick(t)
			if len(p.rows(t)) != 2 {
				t.Fatal("verified policy repair repeated")
			}
		})
	}
}
