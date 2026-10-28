package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
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
