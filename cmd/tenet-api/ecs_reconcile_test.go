package main

import (
	"context"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/rbacpolicy"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestECSGitSyncStartupRequiresExplicitSourceAndWriteGates(t *testing.T) {
	t.Setenv("TENET_ECS_RECONCILE", "")
	t.Setenv("TENET_ECS_AUTO_SYNC", "")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "")
	t.Setenv("TENET_ECS_RECONCILE_INTERVAL", "")
	source := apiCatalogSource{kind: apiCatalogSourceGitHub, githubRequest: githubcontents.Request{Ref: "main"}}
	if err := validateECSReconcileConfiguration(source); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	if err := validateECSReconcileConfiguration(source); err == nil {
		t.Fatal("automatic sync enabled without gates")
	}
	t.Setenv("TENET_ECS_RECONCILE", "true")
	t.Setenv("TENET_AWS_ENABLE_ECS_EXECUTION", "true")
	t.Setenv("TENET_AWS_ENABLE_APPLY", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "ecs-sync")
	if err := validateECSReconcileConfiguration(source); err != nil {
		t.Fatal(err)
	}
	source.githubRequest.Ref = ""
	if err := validateECSReconcileConfiguration(source); err == nil {
		t.Fatal("default branch accepted")
	}
	source.githubRequest.Ref = "main"
	source.kind = apiCatalogSourceFilesystem
	if err := validateECSReconcileConfiguration(source); err == nil {
		t.Fatal("unversioned source accepted")
	}
	source.kind = apiCatalogSourceGitHub
	t.Setenv("TENET_ECS_RECONCILE_INTERVAL", "0s")
	if err := validateECSReconcileConfiguration(source); err == nil {
		t.Fatal("busy-loop interval accepted")
	}
}
func TestAutomaticPrincipalIsRevokedWhenGlobalSyncOrServiceIdentityChanges(t *testing.T) {
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	t.Setenv("TENET_ECS_RECONCILE", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "ecs-sync")
	options := api.Options{RepositoryPolicy: func(_ context.Context, id requestactor.AuthenticatedIdentity, g requestactor.AuthenticatedGroups) (rbacpolicy.Evaluation, error) {
		return rbacpolicy.Evaluate(rbacpolicy.Policy{Version: "v1", Users: []rbacpolicy.Subject{{ID: "ecs-sync", Roles: []string{"operator"}, Deployments: []string{"worker"}}}}, id, g)
	}}
	principal := &applyrequest.ExecutionPrincipal{Automatic: true, Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "ecs-sync"}}
	if err := authorizeECSAutomaticApproval(principal); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TENET_ECS_AUTO_SYNC", "false")
	if err := authorizeECSAutomaticApproval(principal); err == nil {
		t.Fatal("queued automation ignored global disable")
	}
	t.Setenv("TENET_ECS_AUTO_SYNC", "true")
	t.Setenv("TENET_ECS_AUTO_SYNC_SUBJECT", "new-sync")
	if err := authorizeECSAutomaticApproval(principal); err == nil {
		t.Fatal("replaced automation identity retained approval")
	}
	principal.Automatic = false
	if err := authorizeECSPrincipal(context.Background(), options, principal, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := authorizeECSAutomaticApproval(principal); err != nil {
		t.Fatal("manual RBAC approval incorrectly required auto sync")
	}
}
