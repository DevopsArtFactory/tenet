package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestAPIOptionsOverviewDeploymentUsesNamedSnapshotLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
				DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
			},
		},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	detail, err := options.OverviewDeployment("api-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "api-prod" {
		t.Fatalf("expected Get lookup for api-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if detail.Name != "api-prod" || detail.DeployedArtifact.Image != "payments-api:v1" || detail.AccountName != "prod-core" {
		t.Fatalf("unexpected deployment detail: %+v", detail)
	}
}

func TestAPIOptionsOverviewDeploymentReturnsCatalogOnlyDetailWhenSnapshotMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	detail, err := options.OverviewDeployment("worker-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "worker-prod" {
		t.Fatalf("expected Get lookup for worker-prod, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
	if detail.Name != "worker-prod" || detail.AccountName != "prod-core" || detail.DesiredArtifact.Image != "payments-worker:v2" {
		t.Fatalf("unexpected deployment detail: %+v", detail)
	}
	if detail.DeployedArtifact != (domain.DeploymentArtifact{}) {
		t.Fatalf("expected zero deployed artifact, got %+v", detail.DeployedArtifact)
	}
	if detail.ObservedGeneration != 0 {
		t.Fatalf("expected zero observed generation, got %d", detail.ObservedGeneration)
	}
	if detail.Sync != (domain.Sync{}) {
		t.Fatalf("expected zero sync, got %+v", detail.Sync)
	}
	if detail.Health != (domain.Health{}) {
		t.Fatalf("expected zero health, got %+v", detail.Health)
	}
	if detail.Rollout != nil {
		t.Fatalf("expected nil rollout, got %+v", detail.Rollout)
	}
	if len(detail.Conditions) != 0 {
		t.Fatalf("expected no conditions, got %+v", detail.Conditions)
	}
}

func TestAPIOptionsOverviewDeploymentReturnsNotFoundForRepoUnknownDeployment(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}

	_, err := options.OverviewDeployment("missing")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
	if store.getCalls != 1 {
		t.Fatalf("expected one Get call, got %d", store.getCalls)
	}
	if store.gotGetName != "missing" {
		t.Fatalf("expected Get lookup for missing, got %q", store.gotGetName)
	}
	if store.listCalls != 0 {
		t.Fatalf("expected no List calls, got %d", store.listCalls)
	}
}

func TestAPIOptionsOverviewAccountUsesSnapshotListLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		listSnapshots: []statusstore.Snapshot{{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Sync:   domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
				Health: domain.Health{Overall: domain.HealthHealthy, Summary: "healthy"},
			},
		}},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}

	detail, err := options.OverviewAccount("123456789012")
	if err != nil {
		t.Fatalf("OverviewAccount returned error: %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
	if detail.AWSAccountID != "123456789012" || detail.Name != "prod-core" || len(detail.Deployments) != 2 {
		t.Fatalf("unexpected account detail: %+v", detail)
	}
	if detail.Deployments[0].Name != "api-prod" {
		t.Fatalf("expected api-prod first deployment, got %+v", detail.Deployments)
	}
}

func TestAPIOptionsOverviewAccountPropagatesSnapshotListErrors(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	expected := errors.New("snapshots unavailable")
	store := &stubStatusStore{listErr: expected}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}

	_, err := options.OverviewAccount("123456789012")
	if !errors.Is(err, expected) {
		t.Fatalf("expected snapshot list error, got %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
}

func TestAPIOptionsOverviewApplicationsUsesSnapshotListLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		listSnapshots: []statusstore.Snapshot{{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Sync:             domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
				Health:           domain.Health{Overall: domain.HealthHealthy, Summary: "healthy"},
				DeployedArtifact: domain.DeploymentArtifact{Image: "payments-api:v1"},
			},
		}},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewApplications == nil {
		t.Fatalf("expected overview applications provider")
	}

	applications, err := options.OverviewApplications()
	if err != nil {
		t.Fatalf("OverviewApplications returned error: %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one List call, got %d", store.listCalls)
	}
	if store.getCalls != 0 {
		t.Fatalf("expected no Get calls, got %d", store.getCalls)
	}
	if len(applications) != 2 {
		t.Fatalf("expected one flattened row per deployment, got %+v", applications)
	}
	if applications[0].Name != "api-prod" || applications[0].DeployedArtifact.Image != "payments-api:v1" || applications[0].Sync.State != domain.SyncStateSynced {
		t.Fatalf("unexpected application overview row: %+v", applications[0])
	}
}

func TestAPIOptionsOverviewApplicationsFallsBackToCatalogWhenStoreMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.OverviewApplications == nil {
		t.Fatalf("expected overview applications provider")
	}

	applications, err := options.OverviewApplications()
	if err != nil {
		t.Fatalf("OverviewApplications returned error: %v", err)
	}
	if len(applications) != 2 {
		t.Fatalf("expected one flattened row per deployment, got %+v", applications)
	}
	if applications[0].Name != "api-prod" || applications[0].AWSAccountID != "123456789012" || applications[0].RuntimeSummary != nil {
		t.Fatalf("expected catalog-only overview row with zero runtime state, got %+v", applications[0])
	}
	if applications[1].Name != "worker-prod" || applications[1].Sync != (domain.Sync{}) || applications[1].Health != (domain.Health{}) {
		t.Fatalf("expected zero-valued runtime fields for repo-only row, got %+v", applications[1])
	}
}

func TestAPIOptionsOverviewDetailProvidersFallBackToCatalogWhenStoreMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.OverviewDeployment == nil {
		t.Fatalf("expected overview deployment provider")
	}
	if options.OverviewAccounts == nil {
		t.Fatalf("expected overview accounts provider")
	}
	if options.OverviewAccount == nil {
		t.Fatalf("expected overview account provider")
	}
	if options.OverviewProjectEnvironment == nil {
		t.Fatalf("expected overview project/environment provider")
	}

	deployment, err := options.OverviewDeployment("api-prod")
	if err != nil {
		t.Fatalf("OverviewDeployment returned error: %v", err)
	}
	if deployment.Name != "api-prod" || deployment.AccountName != "prod-core" || deployment.Sync != (domain.Sync{}) || deployment.Health != (domain.Health{}) {
		t.Fatalf("expected catalog-only deployment detail, got %+v", deployment)
	}

	accounts, err := options.OverviewAccounts()
	if err != nil {
		t.Fatalf("OverviewAccounts returned error: %v", err)
	}
	if len(accounts) != 1 || accounts[0].AWSAccountID != "123456789012" || len(accounts[0].Deployments) != 2 {
		t.Fatalf("expected catalog-only account overviews, got %+v", accounts)
	}

	account, err := options.OverviewAccount("123456789012")
	if err != nil {
		t.Fatalf("OverviewAccount returned error: %v", err)
	}
	if account.Name != "prod-core" || account.DeploymentCount != 2 {
		t.Fatalf("expected catalog-only account detail, got %+v", account)
	}
	if len(account.Deployments) == 0 {
		t.Fatalf("expected account deployments in catalog-only detail, got %+v", account)
	}

	appRef := account.Deployments[0].AppRef
	environmentRef := account.Deployments[0].EnvironmentRef
	group, err := options.OverviewProjectEnvironment("123456789012", appRef, environmentRef)
	if err != nil {
		t.Fatalf("OverviewProjectEnvironment returned error: %v", err)
	}
	if group.AppRef != appRef || group.EnvironmentRef != environmentRef || len(group.Deployments) == 0 {
		t.Fatalf("expected catalog-only project/environment detail, got %+v", group)
	}
}

type stubStatusStore struct {
	getSnapshot   statusstore.Snapshot
	listSnapshots []statusstore.Snapshot
	getErr        error
	listErr       error
	gotGetName    string
	getCalls      int
	listCalls     int
}

func (s *stubStatusStore) List(context.Context) ([]statusstore.Snapshot, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listSnapshots != nil {
		return s.listSnapshots, nil
	}
	return []statusstore.Snapshot{s.getSnapshot}, nil
}

func (s *stubStatusStore) Get(_ context.Context, name string) (statusstore.Snapshot, error) {
	s.getCalls++
	s.gotGetName = name
	if s.getErr != nil {
		return statusstore.Snapshot{}, s.getErr
	}
	return s.getSnapshot, nil
}

func (s *stubStatusStore) Save(context.Context, []statusstore.Snapshot) error {
	return nil
}

func writeRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","image":"payments-api:v2","target":{"type":"ecs"},"stacks":[{"stack":"api-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","image":"payments-worker:v2","target":{"type":"ecs"},"stacks":[{"stack":"worker-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	return root
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
