package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestAdvancedInputSupportDoesNotEnableProductionExecution(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "false")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	source := newFilesystemCatalogSource(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plans.json")
	applyPath := filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Current("test", "test"), source, nil, planPath, applyPath)
	plans := planpreview.Service{Store: planpreview.NewFileStore(planPath), LoadDetail: options.OverviewDeployment}
	service := newApplyService(source, options, planPath, applyPath)
	for _, name := range []string{"web-blue-green", "web-canary", "web-linear"} {
		plan, err := plans.Create(context.Background(), name, planpreview.CreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(context.Background(), plan.Record.ID); !errors.Is(err, applyrequest.ErrPlanNotReady) {
			t.Fatalf("%s advanced inputs enabled live execution: %v", name, err)
		}
	}
	records, err := service.List(context.Background(), applyrequest.ListQuery{})
	if err != nil || len(records) != 0 {
		t.Fatal("unsupported production strategy queued live work")
	}
}

func TestAdvancedManualExecutionRequiresExplicitFlagAndCurrentOperatorPolicy(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "true")
	source := newFilesystemCatalogSource(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	dir := t.TempDir()
	planPath, applyPath := filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Current("test", "test"), source, nil, planPath, applyPath)
	allowed := true
	options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, groups requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		role := "operator"
		if !allowed {
			role = "viewer"
		}
		return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "operator", Roles: []string{role}, Deployments: []string{"web-blue-green", "web-canary", "web-linear"}}}}, id, groups)
	}
	plans := planpreview.Service{Store: planpreview.NewFileStore(planPath), LoadDetail: options.OverviewDeployment}
	service := newApplyService(source, options, planPath, applyPath)
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	t.Setenv("TENET_ECS_RECONCILE", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "operator")
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{ID: "operator", Authenticated: true})
	for _, name := range []string{"web-blue-green", "web-canary", "web-linear"} {
		plan, err := plans.Create(ctx, name, planpreview.CreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(context.Background(), plan.Record.ID); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("attribution-only request authorized advanced execution", err)
		}
		if _, err := service.Create(applyrequest.WithAutomaticApproval(ctx), plan.Record.ID); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("advanced automation bypassed explicit manual approval", err)
		}
		created, err := service.Create(ctx, plan.Record.ID)
		if err != nil || created.Record.Status != applyrequest.StatusQueued {
			t.Fatal("manual advanced approval did not queue", created, err)
		}
		executor := newNativeECSExecutor(source, options, planPath, applyPath, nil)
		if !executor.ECS.AllowAdvanced || executor.Service.PrepareNativeRollback == nil || executor.ECS.Handlers[applyrequest.NativeRollbackOperationKind] == nil {
			t.Fatal("advanced runtime missing recovery wiring")
		}
		allowed = false
		if err := executor.ECS.Authorize(ctx, created.Record); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("revoked repository operator remained authorized", err)
		}
		allowed = true
	}
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "false")
	executor := newNativeECSExecutor(source, options, planPath, applyPath, nil)
	if executor.ECS.AllowAdvanced || executor.Service.PrepareNativeRollback != nil {
		t.Fatal("advanced execution leaked through existing Rolling flag")
	}
	roleCalls := 0
	executor.Service.DeployerRoleReadiness = func(context.Context, applyrequest.DeployerRoleTarget) (applyrequest.DeployerRoleReadiness, error) {
		roleCalls++
		return applyrequest.DeployerRoleReadiness{}, errors.New("must not contact AWS")
	}
	if err := executor.RunOnce(ctx); err == nil {
		t.Fatal("disabled advanced approvals were not blocked")
	}
	if roleCalls != 0 {
		t.Fatal("disabled advanced executor contacted AWS readiness")
	}
}
func TestAdvancedFlagAloneCannotStartLiveExecution(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "false")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "false")
	if err := validateECSExecutionConfiguration(newFilesystemCatalogSource("unused")); err == nil {
		t.Fatal("advanced-only flag bypassed main execution boundary")
	}
}

func TestAdvancedAutomaticGitApprovalAndWriteGates(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "true")
	t.Setenv("TENET_ECS_RECONCILE", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "ecs-sync")
	base := newFilesystemCatalogSource(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	source := base
	source.kind = apiCatalogSourceGitHub
	source.githubRequest.Ref = "main"
	source.loadCatalog = func(ctx context.Context) (manifest.Catalog, error) {
		c, err := base.loadCatalog(ctx)
		if err != nil {
			return c, err
		}
		c.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: strings.Repeat("a", 40)}
		for i := range c.Deployments {
			c.Deployments[i].ECS.SyncMode = "automatic"
			c.Deployments[i].ECS.SelfHeal = true
		}
		return c, nil
	}
	dir := t.TempDir()
	planPath, applyPath := filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Info{}, source, nil, planPath, applyPath)
	authorized := true
	options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, g requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		role := "operator"
		if !authorized {
			role = "viewer"
		}
		return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "ecs-sync", Roles: []string{role}, Deployments: []string{"web-blue-green", "web-canary", "web-linear"}}}}, id, g)
	}
	service := newApplyService(source, options, planPath, applyPath)
	plans := planpreview.Service{Store: planpreview.NewFileStore(planPath), LoadDetail: options.OverviewDeployment}
	ctx := applyrequest.WithAutomaticApproval(requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{Authenticated: true, ID: "ecs-sync"}))
	var approved []applyrequest.Record
	for _, name := range []string{"web-blue-green", "web-canary", "web-linear"} {
		plan, err := plans.Create(ctx, name, planpreview.CreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		created, err := service.Create(ctx, plan.Record.ID)
		if err != nil || !created.Record.Principal.Automatic || created.Record.Source.Git == nil {
			t.Fatal("pinned automatic approval missing", created, err)
		}
		approved = append(approved, created.Record)
	}
	executor := newNativeECSExecutor(source, options, planPath, applyPath, nil)
	loop := newECSGitReconciler(source, options, nil, planPath, applyPath, nil)
	if !loop.AllowAdvanced {
		t.Fatal("production reconciler did not receive advanced opt-in")
	}
	for _, r := range approved {
		if err := executor.ECS.Authorize(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := executor.ECS.AuthorizeMutation(ctx, r); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TENET_ECS_AUTO_SYNC", "false")
		if err := executor.ECS.AuthorizeMutation(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("paused automation retained write authority", err)
		}
		if err := executor.ECS.Authorize(ctx, r); err != nil {
			t.Fatal("automation pause blocked accepted read-only observation", err)
		}
		t.Setenv("TENET_ECS_AUTO_SYNC", "true")
		t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "replacement-subject")
		if err := executor.ECS.AuthorizeMutation(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("replaced automation subject retained write authority", err)
		}
		t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "ecs-sync")
		authorized = false
		if err := executor.ECS.Authorize(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("revoked RBAC was ignored", err)
		}
		authorized = true
		original := r.Operations[0].ECS.SyncMode
		r.Operations[0].ECS.SyncMode = "manual"
		if err := executor.ECS.AuthorizeMutation(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("manual intent acquired automatic approval", err)
		}
		r.Operations[0].ECS.SyncMode = original
		savedGit := r.Source.Git
		r.Source.Git = nil
		if err := executor.ECS.AuthorizeMutation(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("unpinned source acquired automatic approval", err)
		}
		r.Source.Git = savedGit
	}
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "false")
	for _, r := range approved {
		if err := executor.ECS.Authorize(ctx, r); !errors.Is(err, applyrequest.ErrExecutionUnauthorized) {
			t.Fatal("disabled advanced strategy remained executable", err)
		}
	}
}

func TestSwitchToAdvancedAutomaticSyncCanRetireUnsentManualApproval(t *testing.T) {
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_ADVANCED_EXECUTION", "true")
	t.Setenv("TENET_ECS_RECONCILE", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "ecs-sync")
	base := newFilesystemCatalogSource(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	mode, revision := "manual", strings.Repeat("a", 40)
	source := base
	source.loadCatalog = func(ctx context.Context) (manifest.Catalog, error) {
		c, err := base.loadCatalog(ctx)
		if err != nil {
			return c, err
		}
		c.Git = &domain.GitSource{Repository: "org/desired", Ref: "main", Revision: revision}
		for i := range c.Deployments {
			c.Deployments[i].ECS.SyncMode = mode
		}
		return c, nil
	}
	dir := t.TempDir()
	planPath, applyPath := filepath.Join(dir, "plans.json"), filepath.Join(dir, "applies.json")
	options := newAPIOptionsWithCatalogSourceAndStores(buildinfo.Info{}, source, nil, planPath, applyPath)
	options.RepositoryPolicy = func(_ context.Context, id requestactor.AuthenticatedIdentity, g requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "ecs-sync", Roles: []string{"operator"}, Deployments: []string{"web-canary"}}}}, id, g)
	}
	service := newApplyService(source, options, planPath, applyPath)
	plans := planpreview.Service{Store: planpreview.NewFileStore(planPath), LoadDetail: options.OverviewDeployment}
	ctx := requestactor.WithAuthenticatedIdentityContext(context.Background(), requestactor.AuthenticatedIdentity{ID: "ecs-sync", Authenticated: true})
	firstPlan, err := plans.Create(ctx, "web-canary", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Create(ctx, firstPlan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	mode, revision = "automatic", strings.Repeat("b", 40)
	nextPlan, err := plans.Create(ctx, "web-canary", planpreview.CreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	next, err := service.Create(applyrequest.WithAutomaticApproval(ctx), nextPlan.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := service.Store.(applyrequest.ECSExecutionStore).LockExecution(ctx, "web-canary", next.Record.Operations[0].ECS.ServiceARN)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	previous, err := service.SupersedeECS(applyrequest.WithAutomaticApproval(ctx), first.Record.ID, next.Record.ID)
	if err != nil || previous.Status != applyrequest.StatusCancelled {
		t.Fatal("manual policy trapped the newly opted-in automatic approval", previous, err)
	}
	if !next.Record.Principal.Automatic || first.Record.Principal.Automatic {
		t.Fatal("cancellation rewrote approval provenance")
	}
}
