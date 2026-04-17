package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
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

func TestRequestTimeoutFromEnv(t *testing.T) {
	t.Run("defaults to thirty seconds", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 30*time.Second {
			t.Fatalf("expected 30s default, got %s", timeout)
		}
	})

	t.Run("accepts explicit disable", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "0")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 0 {
			t.Fatalf("expected disabled timeout, got %s", timeout)
		}
	})

	t.Run("parses durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "45s")

		timeout, err := requestTimeoutFromEnv()
		if err != nil {
			t.Fatalf("requestTimeoutFromEnv returned error: %v", err)
		}
		if timeout != 45*time.Second {
			t.Fatalf("expected 45s timeout, got %s", timeout)
		}
	})

	t.Run("rejects invalid durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "not-a-duration")

		if _, err := requestTimeoutFromEnv(); err == nil {
			t.Fatalf("expected invalid duration error")
		}
	})

	t.Run("rejects negative durations", func(t *testing.T) {
		t.Setenv("TENET_API_REQUEST_TIMEOUT", "-1s")

		if _, err := requestTimeoutFromEnv(); err == nil {
			t.Fatalf("expected negative duration error")
		}
	})
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

func TestAPIOptionsPlanPreviewPersistsAndReadsBack(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.PlanPreview == nil {
		t.Fatalf("expected plan preview providers to be wired")
	}

	created, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	if !created.Created || created.Record.ID == "" {
		t.Fatalf("expected a persisted immutable plan preview, got %+v", created)
	}
	if created.Record.DeploymentName != "api-prod" || created.Record.Status != planpreview.StatusReady {
		t.Fatalf("unexpected created plan preview: %+v", created.Record)
	}
	if len(created.Record.Operations) == 0 {
		t.Fatalf("expected derived plan operations, got %+v", created.Record)
	}

	fetched, err := options.PlanPreview(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("PlanPreview returned error: %v", err)
	}
	if fetched.ID != created.Record.ID || fetched.Source.Route != "/v1/overview/deployments/api-prod" {
		t.Fatalf("unexpected fetched immutable plan preview: %+v", fetched)
	}
}

func TestAPIOptionsApplyRequestPersistsDisabledRecordAndReadsBack(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	t.Setenv("TENET_PLAN_STORE_PATH", filepath.Join(t.TempDir(), "plan-previews.json"))
	t.Setenv("TENET_APPLY_STORE_PATH", filepath.Join(t.TempDir(), "apply-requests.json"))

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	if options.CreateDeploymentPlanPreview == nil || options.PlanPreview == nil || options.CreateApplyRequest == nil || options.ApplyRequest == nil {
		t.Fatalf("expected plan/apply providers to be wired")
	}

	plan, err := options.CreateDeploymentPlanPreview(context.Background(), "api-prod", planpreview.CreateRequest{})
	if err != nil {
		t.Fatalf("CreateDeploymentPlanPreview returned error: %v", err)
	}
	created, err := options.CreateApplyRequest(context.Background(), plan.Record.ID)
	if err != nil {
		t.Fatalf("CreateApplyRequest returned error: %v", err)
	}
	if !created.Created || created.Record.ID == "" {
		t.Fatalf("expected a persisted immutable apply request, got %+v", created)
	}
	if created.Record.PlanID != plan.Record.ID || created.Record.Status != applyrequest.StatusFailed {
		t.Fatalf("unexpected created apply request: %+v", created.Record)
	}
	if created.Record.Error != "apply execution is disabled" || len(created.Record.Details) == 0 {
		t.Fatalf("expected disabled apply contract, got %+v", created.Record)
	}

	fetched, err := options.ApplyRequest(context.Background(), created.Record.ID)
	if err != nil {
		t.Fatalf("ApplyRequest returned error: %v", err)
	}
	if fetched.ID != created.Record.ID || fetched.Source.Route != "/v1/plans/"+plan.Record.ID {
		t.Fatalf("unexpected fetched immutable apply request: %+v", fetched)
	}
}

func TestAPIOptionsOverviewDeploymentEventsUsesNamedSnapshotLookup(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{
		getSnapshot: statusstore.Snapshot{
			DeploymentName: "api-prod",
			Status: domain.DeploymentStatus{
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Shifted 10 percent of traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
				}},
			},
		},
	}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentEvents == nil {
		t.Fatalf("expected overview deployment events provider")
	}

	events, err := options.OverviewDeploymentEvents("api-prod")
	if err != nil {
		t.Fatalf("OverviewDeploymentEvents returned error: %v", err)
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
	if events.DeploymentName != "api-prod" || len(events.Events) != 1 || events.Events[0].Source != "controller" {
		t.Fatalf("unexpected deployment events: %+v", events)
	}
}

func TestAPIOptionsOverviewDeploymentEventsReturnsEmptyListWhenSnapshotMissing(t *testing.T) {
	repoRoot := writeRepositoryFixture(t)
	store := &stubStatusStore{getErr: statusstore.ErrNotFound}

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
	if options.OverviewDeploymentEvents == nil {
		t.Fatalf("expected overview deployment events provider")
	}

	events, err := options.OverviewDeploymentEvents("worker-prod")
	if err != nil {
		t.Fatalf("OverviewDeploymentEvents returned error: %v", err)
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
	if events.DeploymentName != "worker-prod" || events.Events == nil || len(events.Events) != 0 {
		t.Fatalf("expected explicit empty events payload, got %+v", events)
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

func TestAPIOptionsOverviewApplication(t *testing.T) {
	t.Run("uses named snapshot lookup", func(t *testing.T) {
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
		if options.OverviewApplication == nil {
			t.Fatalf("expected overview application provider")
		}

		detail, err := options.OverviewApplication("api-prod")
		if err != nil {
			t.Fatalf("OverviewApplication returned error: %v", err)
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
			t.Fatalf("unexpected application detail: %+v", detail)
		}
	})

	t.Run("returns catalog only detail when snapshot missing", func(t *testing.T) {
		repoRoot := writeRepositoryFixture(t)
		store := &stubStatusStore{getErr: statusstore.ErrNotFound}

		options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, store)
		if options.OverviewApplication == nil {
			t.Fatalf("expected overview application provider")
		}

		detail, err := options.OverviewApplication("worker-prod")
		if err != nil {
			t.Fatalf("OverviewApplication returned error: %v", err)
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
			t.Fatalf("unexpected application detail: %+v", detail)
		}
		if detail.DeployedArtifact != (domain.DeploymentArtifact{}) || detail.ObservedGeneration != 0 || detail.Sync != (domain.Sync{}) || detail.Health != (domain.Health{}) {
			t.Fatalf("expected zero runtime fields in catalog-only detail, got %+v", detail)
		}
	})
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

func TestAPIOptionsOverviewDeploymentResourceLogsUsesECSRollingFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeRollingECSRepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", "ecsService", "api-prod-service")
	if err != nil {
		t.Fatalf("expected ecs rolling fallback resource binding to work, got %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider to run once for ecs rolling fallback resource, got %d calls", provider.calls)
	}
	if provider.lastRequest.ResourceType != "ecsService" || provider.lastRequest.ResourceName != "api-prod-service" {
		t.Fatalf("unexpected resource log request for ecs rolling fallback: %+v", provider.lastRequest)
	}
}

func TestAPIOptionsOverviewDeploymentResourceLogsUsesWeightedTargetGroupFallbackWhenExecutionIsMissing(t *testing.T) {
	repoRoot := writeCanaryEC2RepositoryFixture(t)
	provider := &fakeResourceLogProvider{result: resourcelogs.Result{Available: true}}
	restore := stubResourceLogProviderFactory(provider)
	defer restore()

	options := newAPIOptions(buildinfo.Info{Version: "dev", Commit: "abc123"}, repoRoot, nil)
	for _, resource := range []struct {
		resourceType string
		resourceName string
	}{
		{resourceType: "candidateAutoScalingGroup", resourceName: "api-prod-candidate"},
		{resourceType: "trafficShiftPlan", resourceName: "api-prod-weighted-routing"},
		{resourceType: "userDataBootstrapSource", resourceName: "file:userdata/api-prod.sh"},
	} {
		provider.calls = 0
		provider.lastRequest = resourcelogs.Request{}

		_, err := options.OverviewDeploymentResourceLogs(context.Background(), "api-prod", resource.resourceType, resource.resourceName)
		if err != nil {
			t.Fatalf("expected weighted target group fallback resource binding for %s/%s, got %v", resource.resourceType, resource.resourceName, err)
		}
		if provider.calls != 1 {
			t.Fatalf("expected provider to run once for %s/%s, got %d calls", resource.resourceType, resource.resourceName, provider.calls)
		}
		if provider.lastRequest.ResourceType != resource.resourceType || provider.lastRequest.ResourceName != resource.resourceName {
			t.Fatalf("unexpected resource log request for %s/%s: %+v", resource.resourceType, resource.resourceName, provider.lastRequest)
		}
	}
}

func TestCanonicalRolloutExecutionFallback(t *testing.T) {
	t.Run("weighted target groups", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "asgWeightedTargetGroups",
			Strategy:      "canary",
			UserData:      domain.DeploymentUserData{Type: "file", Path: "userdata/api-prod.sh"},
			RolloutConfig: domain.DeploymentRolloutConfig{Capacity: domain.DeploymentRolloutCapacity{Desired: 4, Min: 2, Max: 8}, Strategy: domain.DeploymentRolloutStrategy{WarmupSeconds: 90}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "candidateAutoScalingGroup",
			Name:        "api-prod-candidate",
			Description: "Candidate Auto Scaling Group Tenet creates from normalized userdata source file:userdata/api-prod.sh with desired 4, min 2, and max 8 instances and warmup 90s for weighted target group rollout",
		}, {
			Resource:    "activeTargetGroup",
			Name:        "api-prod-active",
			Description: "Currently active target group receiving production traffic",
		}, {
			Resource:    "candidateTargetGroup",
			Name:        "api-prod-candidate",
			Description: "Candidate target group Tenet will register and warm before traffic shifting",
		}, {
			Resource:    "productionListenerRule",
			Name:        "api-prod-prod-listener",
			Description: "Production listener rule operators inspect while Tenet updates weighted forwarding during rollout",
		}, {
			Resource:    "trafficShiftPlan",
			Name:        "api-prod-weighted-routing",
			Description: "Weighted routing plan Tenet follows to shift a canary slice of production traffic from the active target group to the candidate target group before full promotion while respecting warmup 90s",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        "file:userdata/api-prod.sh",
			Description: "Normalized userdata bootstrap source for candidate EC2 capacity",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected weighted target group fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("ecs code deploy", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "worker-prod",
			ExecutionMode: "ecsCodeDeploy",
			Strategy:      "linear",
			RolloutConfig: domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{MinHealthyPercent: 80, WarmupSeconds: 45}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        "worker-prod-service",
			Description: "ECS service Tenet updates during CodeDeploy traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "codeDeployApplication",
			Name:        "worker-prod-codedeploy-app",
			Description: "CodeDeploy application coordinating ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "codeDeployDeploymentGroup",
			Name:        "worker-prod-codedeploy-group",
			Description: "CodeDeploy deployment group Tenet targets for ECS rollout (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "productionListener",
			Name:        "worker-prod-prod-listener",
			Description: "Production listener CodeDeploy shifts during ECS traffic shifting (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "testListener",
			Name:        "worker-prod-test-listener",
			Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "blueTaskSetTargetGroup",
			Name:        "worker-prod-blue",
			Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}, {
			Resource:    "greenTaskSetTargetGroup",
			Name:        "worker-prod-green",
			Description: "Green task set target group CodeDeploy warms before shifting ECS traffic (style: linear; qualifiers: min healthy 80% and warmup 45s)",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected ecs code deploy fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("ecs rolling", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "ecsRolling",
			RolloutConfig: domain.DeploymentRolloutConfig{Strategy: domain.DeploymentRolloutStrategy{MinHealthyPercent: 90, WarmupSeconds: 30}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "ecsService",
			Name:        "api-prod-service",
			Description: "ECS service Tenet updates with native rolling deployment while respecting min healthy 90% and warmup 30s",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected ecs rolling fallback resources: got %+v want %+v", got, want)
		}
	})

	t.Run("asg instance refresh", func(t *testing.T) {
		detail := catalog.DeploymentOverviewDetail{
			Name:          "api-prod",
			ExecutionMode: "asgInstanceRefresh",
			UserData:      domain.DeploymentUserData{Type: "file", Path: "userdata/api-prod-refresh.sh"},
			RolloutConfig: domain.DeploymentRolloutConfig{Stacks: []domain.DeploymentRolloutStack{{RollingUpdateInstanceCount: 2}}},
		}
		want := []domain.RolloutExecutionDetail{{
			Resource:    "serviceAutoScalingGroup",
			Name:        "api-prod-service",
			Description: "Service Auto Scaling Group Tenet refreshes in place with rolling-update batch size 2",
		}, {
			Resource:    "userDataBootstrapSource",
			Name:        "file:userdata/api-prod-refresh.sh",
			Description: "Normalized userdata bootstrap source for refreshed EC2 instances",
		}}
		if got := canonicalRolloutExecutionFallback(detail); !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected asg instance refresh fallback resources: got %+v want %+v", got, want)
		}
	})
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

func writeCanaryEC2RepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary","warmupSeconds":90},"traffic":{"port":443},"capacity":{"desired":4,"min":2,"max":8},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)
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

func writeRollingECSRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","observerRoleArn":"arn:aws:iam::123456789012:role/TenetObserverRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling","minHealthyPercent":90,"warmupSeconds":30},"capacity":{"desired":2,"min":1,"max":2}}`)
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
