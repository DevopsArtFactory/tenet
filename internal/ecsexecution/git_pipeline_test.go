package ecsexecution

import (
	"context"
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

// Exercise the Git reconciler through all three real handlers and durable stores.
// Only the AWS SDK boundary and already-resolved Git input are replaced by fakes.
func TestGitReconciliationRealHandlersDeployFailRollbackAndAcceptRecoveryCommit(t *testing.T) {
	ctx := context.Background()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-repo"))
	if err != nil {
		t.Fatal(err)
	}
	raw.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
	raw.Deployments[0].ECS.SyncPolicy = &spec.ECSSyncPolicy{Mode: "automatic"}
	raw.Deployments[0].ECS.RollbackOnFailure = true
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plans.json")
	plans := planpreview.NewFileStore(planPath)
	detail := func(ctx context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
		return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
	}
	backend, _, request := serviceFixture(t)
	for revision := range backend.definitions {
		if revision != compiled.Deployments[0].ECS.BaseTaskDefinitionARN {
			delete(backend.definitions, revision)
		}
	}
	backend.revisions = nil
	backend.registrationFake.writes = 0
	observer := &observationFake{serviceFake: backend, tasks: map[string]types.Task{}}
	p := &recoveryPipeline{ctx: ctx, backend: backend, observer: observer, now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	checks := func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
		return nil
	}
	update := RollingService{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ServiceClient, error) { return backend, nil }, CheckCapabilities: checks}
	service := applyrequest.Service{Store: applyrequest.NewFileStore(filepath.Join(dir, "applies.json")), EnableAWSApply: true, LoadPlan: plans.Get, CurrentFingerprint: func(ctx context.Context, name string) (string, error) {
		d, err := detail(ctx, name)
		if err != nil {
			return "", err
		}
		return catalog.DeriveDeploymentSourceFingerprint(d)
	}, Now: func() time.Time { return p.now }, DeployerRoleReadiness: func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		return request.Role, nil
	}, AuthorizeCreate: func(ctx context.Context, _ applyrequest.Record) (*applyrequest.ExecutionPrincipal, error) {
		identity := requestactor.AuthenticatedIdentityFromContext(ctx)
		if !identity.Authenticated || identity.ID != "ecs-sync" {
			return nil, applyrequest.ErrExecutionUnauthorized
		}
		return &applyrequest.ExecutionPrincipal{Identity: identity, Automatic: applyrequest.AutomaticApprovalFromContext(ctx)}, nil
	}, RollbackTarget: func(ctx context.Context, parent applyrequest.Record) (*domain.ECSDeploymentIntent, error) {
		return update.RollbackTarget(ctx, parent, request.Role)
	}}
	loop := ecsreconcile.Reconciler{Enabled: true, Automatic: true, LiveECS: true, SubjectID: "ecs-sync", LoadCatalog: func(context.Context) (manifest.Catalog, error) { return compiled, nil }, Plans: plans, ListPlans: func(context.Context) ([]planpreview.Record, error) {
		records, err := planpreview.Load(planPath)
		if err != nil {
			return nil, nil
		}
		return records, nil
	}, Applies: service}
	p.executor = applyrequest.Executor{Service: service, WorkerID: "git-pipeline", ECS: &applyrequest.ECSExecution{Enabled: true, Authorize: func(_ context.Context, r applyrequest.Record) error {
		if r.Principal == nil || r.Principal.Identity.ID != "ecs-sync" {
			return applyrequest.ErrExecutionUnauthorized
		}
		return nil
	}, CurrentIntent: func(context.Context, string) (*domain.ECSDeploymentIntent, error) {
		return compiled.Deployments[0].ECS.Clone(), nil
	}, Handlers: map[string]applyrequest.ECSOperationHandler{
		string(reconcile.StepRegisterECSTaskDefinition): Registration{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (RegistrationClient, error) {
			return backend.registrationFake, nil
		}, CheckCapabilities: checks},
		string(reconcile.StepUpdateECSService): update,
		string(reconcile.StepVerifyServiceStability): RollingObservation{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ObservationClient, error) {
			return observer, nil
		}, CheckCapabilities: checks, Now: func() time.Time { return p.now }},
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
			p.now = p.now.Add(31 * time.Second)
			if err := p.executor.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	records := func() []applyrequest.Record {
		t.Helper()
		rows, err := service.Store.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	tick()
	run(2)
	p.healthyRollback()
	run(1)
	rows := records()
	if len(rows) != 1 || rows[0].Status != applyrequest.StatusSucceeded || backend.writes != 1 || backend.registrationFake.writes != 1 {
		t.Fatal("first commit did not converge once")
	}
	firstTask := aws.ToString(backend.service.TaskDefinition)
	tick()
	if len(records()) != 1 {
		t.Fatal("duplicate Git poll deployed again")
	}
	raw.Git.Revision = strings.Repeat("b", 40)
	raw.Deployments[0].Image = "example/worker:broken"
	compiled, err = manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	tick()
	run(2)
	backend.service.Deployments[0].RolloutState = types.DeploymentRolloutStateFailed
	run(1)
	// Next scan schedules the separately authorized automatic recovery.
	run(3)
	p.healthyRollback()
	run(1)
	rows = records()
	failed, recovered := 0, 0
	for _, r := range rows {
		if r.Status == applyrequest.StatusFailed {
			failed++
		}
		if r.Recovery != nil && r.Status == applyrequest.StatusSucceeded {
			recovered++
			if !r.Principal.Automatic {
				t.Fatal("recovery lost automatic approval gate")
			}
		}
	}
	if failed != 1 || recovered != 1 || aws.ToString(backend.service.TaskDefinition) != firstTask {
		t.Fatalf("failed commit did not restore and verify previous service: failures=%d recoveries=%d", failed, recovered)
	}
	writes := backend.writes
	registrations := backend.registrationFake.writes
	tick()
	run(2)
	if backend.writes != writes || backend.registrationFake.writes != registrations || len(records()) != 3 {
		t.Fatal("failed desired commit looped after rollback")
	}
	raw.Git.Revision = strings.Repeat("c", 40)
	raw.Deployments[0].Image = "example/worker:fixed"
	compiled, err = manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	tick()
	run(2)
	p.healthyRollback()
	run(1)
	rows = records()
	fixed := false
	for _, r := range rows {
		if r.Source.Git != nil && r.Source.Git.Revision == raw.Git.Revision && r.Status == applyrequest.StatusSucceeded && r.ECSRuntime.State == "converged" {
			fixed = true
		}
	}
	if !fixed || len(rows) != 4 || backend.writes != writes+1 || backend.registrationFake.writes != registrations+1 {
		t.Fatal("recovery commit did not deploy exactly once")
	}
	loop.Enabled = false
	raw.Git.Revision = strings.Repeat("d", 40)
	raw.Deployments[0].Image = "example/worker:disabled"
	compiled, err = manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	tick()
	run(2)
	if backend.writes != writes+1 || backend.registrationFake.writes != registrations+1 {
		t.Fatal("disabled loop wrote AWS")
	}
}
