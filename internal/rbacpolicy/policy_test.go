package rbacpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func TestEvaluateCombinesUserAndGroupGrants(t *testing.T) {
	policy := Policy{Version: VersionV1, Users: []Subject{{ID: "operator-123", Roles: []string{"viewer"}, Deployments: []string{"payments-api-prod"}}}, Groups: []GroupGrant{{Name: "platform-admins", Roles: []string{"operator"}, Projects: []string{"payments-api"}, Environments: []string{"prod-apne2"}}}}
	got, err := Evaluate(policy, requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator-123"}, requestactor.AuthenticatedGroups{Names: []string{"platform-admins"}})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !got.Available || !got.Configured || got.Source != SourceRepository || !got.CanRead || !got.CanWrite {
		t.Fatalf("unexpected evaluation flags: %+v", got)
	}
	if want := "repository RBAC policy matched 2 subject grant(s): roles=2 deployments=1 projects=1 environments=1"; got.Summary != want {
		t.Fatalf("summary = %q, want %q", got.Summary, want)
	}
	if gotRoles, want := got.Roles.Names, []string{"viewer", "operator"}; !reflect.DeepEqual(gotRoles, want) {
		t.Fatalf("roles = %v, want %v", gotRoles, want)
	}
	if gotDeployments, want := got.DeploymentAccess.Names, []string{"payments-api-prod"}; !reflect.DeepEqual(gotDeployments, want) {
		t.Fatalf("deployments = %v, want %v", gotDeployments, want)
	}
	if gotProjects, want := got.ProjectEnvironmentScopes.Projects, []string{"payments-api"}; !reflect.DeepEqual(gotProjects, want) {
		t.Fatalf("projects = %v, want %v", gotProjects, want)
	}
	if len(got.MatchedUsers) != 1 || len(got.MatchedGroups) != 1 {
		t.Fatalf("expected one user and one group match, got %+v", got)
	}
}

func TestEvaluateReportsNoPolicyOrUnauthenticatedIdentity(t *testing.T) {
	missing, err := Evaluate(Policy{}, requestactor.AuthenticatedIdentity{}, requestactor.AuthenticatedGroups{})
	if err != nil {
		t.Fatalf("missing policy Evaluate() error = %v", err)
	}
	if missing.Configured || missing.Available || missing.UnavailableReason == "" {
		t.Fatalf("expected not-configured diagnostic, got %+v", missing)
	}
	if want := "no repository RBAC policy is configured"; missing.Summary != want {
		t.Fatalf("missing policy summary = %q, want %q", missing.Summary, want)
	}

	policy := Policy{Version: VersionV1, Users: []Subject{{Email: "ops@example.com", Roles: []string{"admin"}}}}
	got, err := Evaluate(policy, requestactor.AuthenticatedIdentity{}, requestactor.AuthenticatedGroups{})
	if err != nil {
		t.Fatalf("unauthenticated Evaluate() error = %v", err)
	}
	if !got.Configured || !got.Available || got.CanRead || got.CanWrite || got.UnavailableReason == "" {
		t.Fatalf("expected unauthenticated diagnostic, got %+v", got)
	}
	if want := "repository RBAC policy is loaded but no authenticated identity was available to evaluate"; got.Summary != want {
		t.Fatalf("unauthenticated summary = %q, want %q", got.Summary, want)
	}

	noGrant, err := Evaluate(policy, requestactor.AuthenticatedIdentity{Authenticated: true, Email: "other@example.com"}, requestactor.AuthenticatedGroups{})
	if err != nil {
		t.Fatalf("no-grant Evaluate() error = %v", err)
	}
	if want := "repository RBAC policy matched no user or group grants for this authenticated identity"; noGrant.Summary != want {
		t.Fatalf("no-grant summary = %q, want %q", noGrant.Summary, want)
	}
}

func TestValidateRejectsUnsupportedRolesAndDuplicateGroups(t *testing.T) {
	if err := Validate(Policy{Version: VersionV1, Users: []Subject{{ID: "u", Roles: []string{"auditor"}}}}); err == nil {
		t.Fatalf("expected unsupported role error")
	}
	if err := Validate(Policy{Version: VersionV1, Groups: []GroupGrant{{Name: "g"}, {Name: "g"}}}); err == nil {
		t.Fatalf("expected duplicate group error")
	}
}

func TestLoadFromRepositoryReadsOptionalPolicy(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadFromRepository(root); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("missing policy err = %v, want ErrNotConfigured", err)
	}
	writeFile(t, filepath.Join(root, "rbac", "policy.json"), `{"version":"v1","users":[{"email":"ops@example.com","roles":["viewer"]}],"groups":[{"name":"platform-admins","roles":["admin"]}]}`)
	policy, err := LoadFromRepository(root)
	if err != nil {
		t.Fatalf("LoadFromRepository() error = %v", err)
	}
	if len(policy.Users) != 1 || len(policy.Groups) != 1 {
		t.Fatalf("unexpected policy: %+v", policy)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
