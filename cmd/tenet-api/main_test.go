package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
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

func TestAPIOptionsOverviewDeploymentResourceLogsWiresObserverRoleAndPlacement(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentResourceLogs == nil {
		t.Fatalf("expected overview deployment resource logs provider")
	}

	result, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-live-service")
	if err != nil {
		t.Fatalf("OverviewDeploymentResourceLogs returned error: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected wired provider result, got %+v", result)
	}
	want := resourcelogs.Request{
		DeploymentName:  "api-prod",
		ResourceType:    "ecsService",
		ResourceName:    "api-prod-live-service",
		TargetAccountID: "123456789012",
		Region:          "ap-northeast-2",
		ObserverRoleARN: "arn:aws:iam::123456789012:role/TenetObserverRole",
	}
	if !reflect.DeepEqual(provider.lastRequest, want) {
		t.Fatalf("unexpected resource log request: %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsRejectsResourcesOutsideDeploymentExecution(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				Rollout: &domain.RolloutSummary{Execution: []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "api-prod-live-service", Description: "live ECS service"}}},
			},
		},
	}
	provider := &fakeResourceLogProvider{}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "other-service")
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected not found for unbound resource, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("expected provider not to run for unbound resource, got %d calls", provider.calls)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesCanonicalFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-service")
	if err != nil {
		t.Fatalf("expected canonical fallback resource binding to work, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to run once for fallback-bound resource, got %d calls", provider.calls)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesASGInstanceRefreshFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRollingEC2RepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType string
		resourceName string
	}{
		{resourceType: "serviceAutoScalingGroup", resourceName: "api-prod-service"},
		{resourceType: "userDataBootstrapSource", resourceName: "file:userdata/api-prod-refresh.sh"},
	} {
		provider.calls = 0
		provider.lastRequest = resourcelogs.Request{}

		_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", resource.resourceType, resource.resourceName)
		if err != nil {
			t.Fatalf("expected asg instance refresh fallback resource binding for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName {
			t.Fatalf("unexpected resource log request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
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

type fakeResourceLogProvider struct {
	result      resourcelogs.Result
	err         error
	lastRequest resourcelogs.Request
	calls       int
}

func (p *fakeResourceLogProvider) Recent(_ context.Context, req resourcelogs.Request) (resourcelogs.Result, error) {
	p.calls++
	p.lastRequest = req
	return p.result, p.err
}

func stubResourceLogProviderFactory(provider resourceLogProvider) func() {
	original := newResourceLogProvider
	newResourceLogProvider = func() resourceLogProvider { return provider }
	return func() { newResourceLogProvider = original }
}

func writeRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","image":"payments-api:v2","target":{"type":"ecs"},"stacks":[{"stack":"api-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","image":"payments-worker:v2","target":{"type":"ecs"},"stacks":[{"stack":"worker-prod","account":"prod-core","env":"prod-apne2","replacement_type":"linear","capacity":{"desired":1,"min":1,"max":1},"regions":[{"region":"ap-northeast-2"}]}]}`)
	return root
}

func writeRollingEC2RepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"rolling"},"traffic":{"port":443},"capacity":{"desired":2,"min":1,"max":3},"userdata":{"type":"file","path":"userdata/api-prod-refresh.sh"}}`)
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
