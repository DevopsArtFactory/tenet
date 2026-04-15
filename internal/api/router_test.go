package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func TestRouterHealthz(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestRouterInfo(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var info buildinfo.Info
	if err := json.Unmarshal(resp.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to decode info: %v", err)
	}

	if info.Commit != "abc123" {
		t.Fatalf("unexpected commit: %s", info.Commit)
	}
}

func TestRouterCatalogSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{AccountCount: 1, EnvironmentCount: 2, DeploymentCount: 3}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var summary catalog.Summary
	if err := json.Unmarshal(resp.Body.Bytes(), &summary); err != nil {
		t.Fatalf("failed to decode summary: %v", err)
	}
	if summary.DeploymentCount != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestRouterCatalogSummaryUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogSummaryPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogDeployments(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{{Name: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var deployments []catalog.DeploymentView
	if err := json.Unmarshal(resp.Body.Bytes(), &deployments); err != nil {
		t.Fatalf("failed to decode deployments: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Name != "api-prod" {
		t.Fatalf("unexpected deployments: %+v", deployments)
	}
}

func TestRouterCatalogDeploymentsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogDeploymentsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogEnvironments(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{{Name: "prod-apne2", AccountRef: "prod-core", Region: "ap-northeast-2", NetworkRef: "prod-main"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var environments []catalog.EnvironmentView
	if err := json.Unmarshal(resp.Body.Bytes(), &environments); err != nil {
		t.Fatalf("failed to decode environments: %v", err)
	}
	if len(environments) != 1 || environments[0].Name != "prod-apne2" {
		t.Fatalf("unexpected environments: %+v", environments)
	}
}

func TestRouterCatalogEnvironmentsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogEnvironmentsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/environments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogAccounts(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return []catalog.AccountView{{Name: "prod-core", AWSAccountID: "123456789012", AllowedRegions: []string{"ap-northeast-2"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var accounts []catalog.AccountView
	if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("failed to decode accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "prod-core" {
		t.Fatalf("unexpected accounts: %+v", accounts)
	}
}

func TestRouterCatalogAccountsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogAccountsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogPlacements(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{{DeploymentName: "api-prod", AppRef: "payments-api", EnvironmentRef: "prod-apne2", AccountName: "prod-core", AWSAccountID: "123456789012", Region: "ap-northeast-2", NetworkRef: "prod-main"}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var placements []catalog.PlacementView
	if err := json.Unmarshal(resp.Body.Bytes(), &placements); err != nil {
		t.Fatalf("failed to decode placements: %v", err)
	}
	if len(placements) != 1 || placements[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected placements: %+v", placements)
	}
}

func TestRouterCatalogPlacementsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogPlacementsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/placements", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterCatalogTopology(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{Summary: catalog.Summary{AccountCount: 1, EnvironmentCount: 1, DeploymentCount: 1}, Accounts: []catalog.AccountView{{Name: "prod-core"}}, Environments: []catalog.EnvironmentView{{Name: "prod-apne2"}}, Deployments: []catalog.DeploymentView{{Name: "api-prod"}}, Placements: []catalog.PlacementView{{DeploymentName: "api-prod", AccountName: "prod-core"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var topology catalog.TopologySnapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &topology); err != nil {
		t.Fatalf("failed to decode topology: %v", err)
	}
	if topology.Summary.DeploymentCount != 1 || len(topology.Placements) != 1 {
		t.Fatalf("unexpected topology: %+v", topology)
	}
}

func TestRouterCatalogTopologyUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterCatalogTopologyPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/topology", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatuses(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"}, Conditions: []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}}}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var snapshots []statusstore.Snapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("failed to decode snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected snapshots: %+v", snapshots)
	}
	if snapshots[0].Status.Sync.Reason != "RolloutCompleted" || len(snapshots[0].Status.Conditions) != 1 {
		t.Fatalf("unexpected status payload: %+v", snapshots[0].Status)
	}
}

func TestRouterDeploymentStatusesUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusesPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByName(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return statusstore.Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var snapshot statusstore.Snapshot
	if err := json.Unmarshal(resp.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}
	if snapshot.DeploymentName != "api-prod" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestRouterDeploymentStatusByNameUsesStableJSONFieldNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
				Conditions:         []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw payload: %v", err)
	}
	status, ok := payload["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested status object, got %#v", payload["status"])
	}
	if _, ok := status["observedGeneration"]; !ok {
		t.Fatalf("expected observedGeneration field in %#v", status)
	}
	sync, ok := status["sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected sync object, got %#v", status["sync"])
	}
	if sync["state"] != string(domain.SyncStateSynced) {
		t.Fatalf("unexpected sync payload: %#v", sync)
	}
	if _, exists := status["ObservedGeneration"]; exists {
		t.Fatalf("unexpected Go-style field name in %#v", status)
	}
}

func TestRouterDeploymentStatusByNameUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByNameNotFound(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, statusstore.ErrNotFound
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/missing", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestRouterDeploymentStatusByNameRejectsInvalidNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})
	for _, path := range []string{"/v1/status/deployments/", "/v1/status/deployments/a/b"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
		}
	}
}

func TestRouterDeploymentStatusByNamePropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterOverviewAccounts(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{Name: "prod-core", AWSAccountID: "123456789012", Deployments: []catalog.DeploymentOverview{{Name: "api-prod", Strategy: "canary", ExecutionMode: "asgWeightedTargetGroups", Rollout: &domain.RolloutSummary{DesiredGeneration: 4, ObservedGeneration: 3, Phase: domain.RolloutPhaseAccepted, CurrentStep: "PromoteCandidateCapacity", Summary: "Observing current state", Steps: []domain.RolloutStepSummary{{Kind: "CreateCandidateCapacity"}, {Kind: "ShiftTrafficCanary"}, {Kind: "PromoteCandidateCapacity", Current: true}}}}, {Name: "api-staging", Strategy: "linear", ExecutionMode: "ecsCodeDeploy", Rollout: &domain.RolloutSummary{DesiredGeneration: 8, ObservedGeneration: 7, Phase: domain.RolloutPhaseAccepted, CurrentStep: "VerifyServiceStability", Summary: "Observing current state", Steps: []domain.RolloutStepSummary{{Kind: "CreateCodeDeployDeployment"}, {Kind: "ShiftTrafficLinear"}, {Kind: "VerifyServiceStability", Current: true}}}}}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var accounts []catalog.AccountOverview
	if err := json.Unmarshal(resp.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("failed to decode overview accounts: %v", err)
	}
	if len(accounts) != 1 || len(accounts[0].Deployments) != 2 || accounts[0].Deployments[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview accounts: %+v", accounts)
	}
	if accounts[0].Deployments[0].Rollout == nil || accounts[0].Deployments[0].Rollout.Phase != domain.RolloutPhaseAccepted || accounts[0].Deployments[0].Rollout.CurrentStep != "PromoteCandidateCapacity" || len(accounts[0].Deployments[0].Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary in overview accounts response, got %+v", accounts)
	}
	if got := accounts[0].Deployments[0].Rollout.Steps; got[0].Kind != "CreateCandidateCapacity" || got[0].Current || got[1].Kind != "ShiftTrafficCanary" || got[1].Current || got[2].Kind != "PromoteCandidateCapacity" || !got[2].Current {
		t.Fatalf("expected strategy-aware EC2 rollout payload, got %+v", got)
	}
	if accounts[0].Deployments[1].Rollout == nil || accounts[0].Deployments[1].ExecutionMode != "ecsCodeDeploy" || accounts[0].Deployments[1].Rollout.Phase != domain.RolloutPhaseAccepted || accounts[0].Deployments[1].Rollout.CurrentStep != "VerifyServiceStability" {
		t.Fatalf("expected strategy-aware ECS rollout payload, got %+v", accounts[0].Deployments[1])
	}
	if got := accounts[0].Deployments[1].Rollout.Steps; got[0].Kind != "CreateCodeDeployDeployment" || got[0].Current || got[1].Kind != "ShiftTrafficLinear" || got[1].Current || got[2].Kind != "VerifyServiceStability" || !got[2].Current {
		t.Fatalf("expected strategy-aware ECS rollout steps, got %+v", got)
	}
}

func TestRouterOverviewAccountsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewAccountsPropagatesErrors(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return nil, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterRootServesUI(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "/v1/overview/accounts") {
		t.Fatalf("expected UI to reference overview endpoint, got %q", body)
	}
	for _, fragment := range []string{"Rollout phase", "Current step", "Planned steps", "ShiftTrafficCanary", "CreateCodeDeployDeployment"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to render rollout summary label %q, got %q", fragment, body)
		}
	}
	for _, unsafe := range []string{
		"root.innerHTML = accounts.map",
		"document.getElementById('accounts').innerHTML = `<div class=\"empty\">Failed to load overview: ${error.message}</div>`",
		"['Progressing', deployments.length - synced]",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected UI to avoid unsafe rendering or misclassified summary logic %q, got %q", unsafe, body)
		}
	}
	for _, fragment := range []string{"textContent", "createElement", "deployment.sync?.state === 'Reconciling' || deployment.sync?.state === 'OutOfSync'"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to include safe rendering and precise progressing-state logic %q, got %q", fragment, body)
		}
	}
}

func TestRouterRejectsNonGETMethods(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{}, nil
		},
		CatalogDeployments: func() ([]catalog.DeploymentView, error) {
			return []catalog.DeploymentView{}, nil
		},
		CatalogEnvironments: func() ([]catalog.EnvironmentView, error) {
			return []catalog.EnvironmentView{}, nil
		},
		CatalogAccounts: func() ([]catalog.AccountView, error) {
			return []catalog.AccountView{}, nil
		},
		CatalogPlacements: func() ([]catalog.PlacementView, error) {
			return []catalog.PlacementView{}, nil
		},
		CatalogTopology: func() (catalog.TopologySnapshot, error) {
			return catalog.TopologySnapshot{}, nil
		},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{}, nil
		},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{}, nil
		},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})

	paths := []string{"/", "/healthz", "/v1/info", "/v1/catalog/summary", "/v1/catalog/deployments", "/v1/catalog/environments", "/v1/catalog/accounts", "/v1/catalog/placements", "/v1/catalog/topology", "/v1/overview/accounts", "/v1/status/deployments", "/v1/status/deployments/api-prod"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", path, resp.Code)
		}
		if allow := resp.Header().Get("Allow"); allow != http.MethodGet {
			t.Fatalf("expected Allow header for %s, got %q", path, allow)
		}
	}
}
