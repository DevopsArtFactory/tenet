package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func assertInternalServerErrorResponse(t *testing.T, resp *httptest.ResponseRecorder, leaked string) {
	t.Helper()
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	body := resp.Body.String()
	if body != http.StatusText(http.StatusInternalServerError)+"\n" {
		t.Fatalf("expected generic 500 body %q, got %q", http.StatusText(http.StatusInternalServerError)+"\n", body)
	}
	if strings.Contains(body, leaked) {
		t.Fatalf("expected 500 body to avoid leaking %q, got %q", leaked, body)
	}
}

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
	const upstreamErr = "boom: summary provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		CatalogSummary: func() (catalog.Summary, error) {
			return catalog.Summary{}, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/summary", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
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

func TestRouterDeploymentStatusesUseStableRolloutAndHistoryJSONFieldNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{
				ObservedGeneration: 3,
				Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
				Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  3,
					ObservedGeneration: 3,
					Phase:              domain.RolloutPhaseCompleted,
					Summary:            "Deployment is synced and healthy",
					Steps:              []domain.RolloutStepSummary{{Kind: "MarkSynced", Status: domain.RolloutStepStatusCompleted}},
				},
				History: []domain.StatusHistoryEntry{{
					Timestamp:    "2026-04-16T07:00:00Z",
					Source:       "controller",
					Summary:      "Deployment is synced and healthy",
					Sync:         domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"},
					Health:       domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"},
					RolloutPhase: domain.RolloutPhaseCompleted,
				}},
			}}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/status/deployments", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw status payload: %v", err)
	}
	status, ok := payload[0]["status"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested status object, got %#v", payload[0]["status"])
	}
	rollout, ok := status["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw status payload, got %#v", status["rollout"])
	}
	if rollout["phase"] != string(domain.RolloutPhaseCompleted) {
		t.Fatalf("expected rollout.phase field, got %#v", rollout)
	}
	if _, exists := rollout["currentStep"]; exists {
		t.Fatalf("expected completed rollout currentStep to be omitted, got %#v", rollout)
	}
	if _, exists := rollout["CurrentStep"]; exists {
		t.Fatalf("unexpected Go-style rollout field name in %#v", rollout)
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected rollout.steps array in raw status payload, got %#v", rollout["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if step["kind"] != "MarkSynced" || step["status"] != string(domain.RolloutStepStatusCompleted) {
		t.Fatalf("expected rollout.steps[0] field names/values, got %#v", step)
	}
	if _, exists := step["current"]; exists {
		t.Fatalf("expected completed rollout step current flag to be omitted, got %#v", step)
	}
	if _, exists := step["Current"]; exists {
		t.Fatalf("unexpected Go-style step field name in %#v", step)
	}
	history, ok := status["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw status payload, got %#v", status["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	if historyEntry["rolloutPhase"] != string(domain.RolloutPhaseCompleted) {
		t.Fatalf("expected history[0].rolloutPhase field, got %#v", historyEntry)
	}
	if _, exists := historyEntry["currentStep"]; exists {
		t.Fatalf("expected completed history currentStep to be omitted, got %#v", historyEntry)
	}
	if _, exists := historyEntry["RolloutPhase"]; exists {
		t.Fatalf("unexpected Go-style history field name in %#v", historyEntry)
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
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  4,
					ObservedGeneration: 3,
					Phase:              domain.RolloutPhaseProvisioning,
					CurrentStep:        "CreateCandidateCapacity",
					Summary:            "Creating candidate capacity for weighted target groups",
					Steps:              []domain.RolloutStepSummary{{Kind: "ResolvePlacement", Status: domain.RolloutStepStatusCompleted}, {Kind: "CreateCandidateCapacity", Current: true, Status: domain.RolloutStepStatusCurrent}},
				},
				History: []domain.StatusHistoryEntry{{
					Timestamp:    "2026-04-16T07:05:00Z",
					Source:       "controller",
					Summary:      "Creating candidate capacity for weighted target groups",
					Sync:         domain.Sync{State: domain.SyncStateReconciling, Reason: "CandidateProvisioning"},
					Health:       domain.Health{Overall: domain.HealthProgressing, Summary: "warming"},
					RolloutPhase: domain.RolloutPhaseProvisioning,
					CurrentStep:  "CreateCandidateCapacity",
				}},
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
	rollout, ok := status["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object, got %#v", status["rollout"])
	}
	if rollout["phase"] != string(domain.RolloutPhaseProvisioning) || rollout["currentStep"] != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout phase/currentStep fields, got %#v", rollout)
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected rollout.steps array, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "ResolvePlacement" || firstStep["status"] != string(domain.RolloutStepStatusCompleted) {
		t.Fatalf("expected rollout.steps[0] field names/values, got %#v", firstStep)
	}
	if _, exists := firstStep["current"]; exists {
		t.Fatalf("expected completed rollout step current flag to be omitted, got %#v", firstStep)
	}
	currentStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[1] object, got %#v", steps[1])
	}
	if currentStep["kind"] != "CreateCandidateCapacity" || currentStep["current"] != true || currentStep["status"] != string(domain.RolloutStepStatusCurrent) {
		t.Fatalf("expected rollout.steps[1] field names/values, got %#v", currentStep)
	}
	history, ok := status["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw payload, got %#v", status["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	if historyEntry["currentStep"] != "CreateCandidateCapacity" || historyEntry["rolloutPhase"] != string(domain.RolloutPhaseProvisioning) {
		t.Fatalf("expected history field names/values, got %#v", historyEntry)
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
			return []catalog.AccountOverview{{
				Name:             "prod-core",
				AWSAccountID:     "123456789012",
				DeploymentCount:  3,
				ProjectCount:     2,
				EnvironmentCount: 2,
				SyncedCount:      1,
				ProgressingCount: 1,
				PausedCount:      1,
				ErrorCount:       0,
				Deployments: []catalog.DeploymentOverview{
					{
						Name:          "api-prod",
						Strategy:      "canary",
						ExecutionMode: "asgWeightedTargetGroups",
						RuntimeSummary: &domain.RuntimeSummary{
							Timestamp:          "2026-04-16T06:49:00Z",
							Source:             "controller",
							Summary:            "Traffic shift is progressing",
							Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
							Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							RolloutPhase:       domain.RolloutPhaseAccepted,
							CurrentStep:        "PromoteCandidateCapacity",
							HistoryCount:       4,
							Changes:            []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"},
						},
						Rollout: &domain.RolloutSummary{
							DesiredGeneration:  4,
							ObservedGeneration: 3,
							Phase:              domain.RolloutPhaseAccepted,
							CurrentStep:        "PromoteCandidateCapacity",
							Summary:            "Observing current state",
							Steps: []domain.RolloutStepSummary{
								{Kind: "CreateCandidateCapacity", Status: "completed"},
								{Kind: "ShiftTrafficCanary", Status: "pending"},
								{Kind: "PromoteCandidateCapacity", Current: true, Status: "current"},
							},
						},
					},
					{
						Name:          "api-staging",
						Strategy:      "linear",
						ExecutionMode: "ecsCodeDeploy",
						RuntimeSummary: &domain.RuntimeSummary{
							Summary:            "Waiting for service stability",
							Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
							Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							RolloutPhase:       domain.RolloutPhaseAccepted,
							CurrentStep:        "VerifyServiceStability",
							HistoryCount:       0,
						},
						Rollout: &domain.RolloutSummary{
							DesiredGeneration:  8,
							ObservedGeneration: 7,
							Phase:              domain.RolloutPhaseAccepted,
							CurrentStep:        "VerifyServiceStability",
							Summary:            "Observing current state",
							Steps: []domain.RolloutStepSummary{
								{Kind: "CreateCodeDeployDeployment", Status: "completed"},
								{Kind: "ShiftTrafficLinear", Status: "pending"},
								{Kind: "VerifyServiceStability", Current: true, Status: "current"},
							},
						},
					},
				},
			}}, nil
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
	if accounts[0].DeploymentCount != 3 || accounts[0].ProjectCount != 2 || accounts[0].EnvironmentCount != 2 || accounts[0].SyncedCount != 1 || accounts[0].ProgressingCount != 1 || accounts[0].PausedCount != 1 || accounts[0].ErrorCount != 0 {
		t.Fatalf("expected additive account counts in overview response, got %+v", accounts[0])
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
	if accounts[0].Deployments[0].RuntimeSummary == nil || accounts[0].Deployments[0].RuntimeSummary.Timestamp != "2026-04-16T06:49:00Z" || accounts[0].Deployments[0].RuntimeSummary.Source != "controller" || accounts[0].Deployments[0].RuntimeSummary.Summary != "Traffic shift is progressing" || accounts[0].Deployments[0].RuntimeSummary.Sync.Reason != "Traffic shift running" || accounts[0].Deployments[0].RuntimeSummary.DesiredGeneration != 8 || accounts[0].Deployments[0].RuntimeSummary.ObservedGeneration != 7 || accounts[0].Deployments[0].RuntimeSummary.CurrentStep != "PromoteCandidateCapacity" || accounts[0].Deployments[0].RuntimeSummary.HistoryCount != 4 || !reflect.DeepEqual(accounts[0].Deployments[0].RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected runtime summary in overview accounts response, got %+v", accounts[0].Deployments[0])
	}
	if got := accounts[0].Deployments[1].Rollout.Steps; got[0].Kind != "CreateCodeDeployDeployment" || got[0].Current || got[1].Kind != "ShiftTrafficLinear" || got[1].Current || got[2].Kind != "VerifyServiceStability" || !got[2].Current {
		t.Fatalf("expected strategy-aware ECS rollout steps, got %+v", got)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	if payload[0]["deploymentCount"] != float64(3) || payload[0]["projectCount"] != float64(2) || payload[0]["environmentCount"] != float64(2) || payload[0]["syncedCount"] != float64(1) || payload[0]["progressingCount"] != float64(1) || payload[0]["pausedCount"] != float64(1) || payload[0]["errorCount"] != float64(0) {
		t.Fatalf("expected additive JSON count fields, got %+v", payload[0])
	}
	rollout, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw overview payload, got %#v", payload[0])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("expected rollout.steps array in raw overview payload, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "CreateCandidateCapacity" || firstStep["status"] != "completed" {
		t.Fatalf("expected rollout.steps[0] status field names, got %#v", firstStep)
	}
	lastStep, ok := steps[2].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[2] object, got %#v", steps[2])
	}
	if lastStep["kind"] != "PromoteCandidateCapacity" || lastStep["current"] != true || lastStep["status"] != "current" {
		t.Fatalf("expected rollout.steps[2] current/status field names, got %#v", lastStep)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw overview payload, got %#v", payload[0])
	}
	if runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "PromoteCandidateCapacity" || runtimeSummary["historyCount"] != float64(4) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary JSON fields in raw overview payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 7 || changes[0] != "summary" || changes[6] != "currentStep" {
		t.Fatalf("expected runtimeSummary.changes field in raw overview payload, got %#v", runtimeSummary["changes"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in raw overview payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in raw overview payload, got %#v", runtimeSummary["health"])
	}
}

func TestOverviewAccountsResponseIncludesRuntimeSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{
				Name:         "prod-core",
				AWSAccountID: "123456789012",
				Deployments: []catalog.DeploymentOverview{{
					Name: "api-prod",
					RuntimeSummary: &domain.RuntimeSummary{
						Timestamp:          "2026-04-16T06:49:00Z",
						Source:             "controller",
						Summary:            "Traffic shift is progressing",
						Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
						Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
						DesiredGeneration:  8,
						ObservedGeneration: 7,
						RolloutPhase:       domain.RolloutPhaseAccepted,
						CurrentStep:        "ShiftTrafficCanary",
						HistoryCount:       2,
						Changes:            []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"},
					},
				}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview accounts payload, got %#v", payload)
	}
	if runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficCanary" || runtimeSummary["historyCount"] != float64(2) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary fields in overview accounts payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 7 || changes[1] != "sync" || changes[5] != "rolloutPhase" {
		t.Fatalf("expected runtimeSummary.changes in overview accounts payload, got %#v", runtimeSummary["changes"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in overview accounts payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in overview accounts payload, got %#v", runtimeSummary["health"])
	}
}

func TestOverviewAccountsResponseOmitsRuntimeSummarySourceWithoutHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return []catalog.AccountOverview{{
				Name:         "prod-core",
				AWSAccountID: "123456789012",
				Deployments: []catalog.DeploymentOverview{{
					Name: "api-prod",
					RuntimeSummary: &domain.RuntimeSummary{
						Summary:            "Serving stable capacity",
						Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
						Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
						DesiredGeneration:  8,
						ObservedGeneration: 8,
						RolloutPhase:       domain.RolloutPhaseCompleted,
						CurrentStep:        "Complete",
						HistoryCount:       0,
					},
				}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview accounts payload: %v", err)
	}
	runtimeSummary, ok := payload[0]["deployments"].([]any)[0].(map[string]any)["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview accounts payload, got %#v", payload)
	}
	if _, ok := runtimeSummary["source"]; ok {
		t.Fatalf("expected runtimeSummary.source to be omitted without history, got %#v", runtimeSummary)
	}
}

func TestRouterOverviewApplications(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewApplications: func() ([]catalog.ApplicationOverview, error) {
			return []catalog.ApplicationOverview{{
				Name:               "payments-api-prod",
				AppRef:             "payments-api",
				EnvironmentRef:     "prod-apne2",
				AccountName:        "prod-core",
				AWSAccountID:       "123456789012",
				Region:             "ap-northeast-2",
				TargetType:         "ec2",
				Strategy:           "canary",
				ExecutionMode:      "asgWeightedTargetGroups",
				DesiredArtifact:    domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r7"},
				DeployedArtifact:   domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64-r6"},
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "new artifact"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "2/3 targets healthy"},
				RuntimeSummary: &domain.RuntimeSummary{
					Timestamp:          "2026-04-16T06:20:00Z",
					Source:             "controller",
					Summary:            "Shifting canary traffic",
					ObservedGeneration: 7,
					CurrentStep:        "ShiftTrafficCanary",
				},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhasePromoting,
					CurrentStep:        "ShiftTrafficCanary",
				},
				Conditions: []domain.Condition{{Type: "Progressing", Status: true, Reason: "RolloutInFlight", Message: "candidate receiving traffic"}},
			}}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var applications []catalog.ApplicationOverview
	if err := json.Unmarshal(resp.Body.Bytes(), &applications); err != nil {
		t.Fatalf("failed to decode overview applications: %v", err)
	}
	if len(applications) != 1 || applications[0].Name != "payments-api-prod" || applications[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview applications: %+v", applications)
	}
	if applications[0].RuntimeSummary == nil || applications[0].Rollout == nil || len(applications[0].Conditions) != 1 {
		t.Fatalf("expected joined status metadata in overview applications response, got %+v", applications[0])
	}
}

func TestRouterOverviewApplicationsUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewApplicationsPropagatesErrors(t *testing.T) {
	const upstreamErr = "boom: overview applications provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewApplications: func() ([]catalog.ApplicationOverview, error) {
			return nil, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/applications", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
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
	const upstreamErr = "boom: overview accounts provider details"
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccounts: func() ([]catalog.AccountOverview, error) {
			return nil, errors.New(upstreamErr)
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertInternalServerErrorResponse(t, resp, upstreamErr)
}

func TestRouterOverviewAccountDetail(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewAccount: func(awsAccountID string) (catalog.AccountOverviewDetail, error) {
			if awsAccountID != "123456789012" {
				t.Fatalf("unexpected lookup aws account id: %s", awsAccountID)
			}
			return catalog.AccountOverviewDetail{
				Name:             "prod-core",
				AWSAccountID:     "123456789012",
				AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
				DeploymentCount:  1,
				ProjectCount:     1,
				EnvironmentCount: 1,
				SyncedCount:      1,
				Deployments: []catalog.DeploymentOverview{{
					Name:           "api-prod",
					EnvironmentRef: "prod-apne2",
					Region:         "ap-northeast-2",
					Strategy:       "canary",
					ExecutionMode:  "asgWeightedTargetGroups",
					Sync:           domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
					Health:         domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.AccountOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview account detail: %v", err)
	}
	if detail.Name != "prod-core" || detail.AWSAccountID != "123456789012" || len(detail.Deployments) != 1 {
		t.Fatalf("unexpected overview account detail: %+v", detail)
	}
	if detail.ProjectCount != 1 || detail.EnvironmentCount != 1 {
		t.Fatalf("expected topology count fields in overview account detail: %+v", detail)
	}
	if detail.Deployments[0].Name != "api-prod" || detail.Deployments[0].ExecutionMode != "asgWeightedTargetGroups" {
		t.Fatalf("unexpected overview account deployment detail: %+v", detail.Deployments[0])
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview account detail payload: %v", err)
	}
	if payload["projectCount"] != float64(1) || payload["environmentCount"] != float64(1) {
		t.Fatalf("expected topology count fields in raw overview account detail payload, got %#v", payload)
	}
}

func TestOverviewAccountDetailRoute(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, statusstore.ErrNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/missing", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
		if body := resp.Body.String(); body != statusstore.ErrNotFound.Error()+"\n" {
			t.Fatalf("expected not-found body %q, got %q", statusstore.ErrNotFound.Error()+"\n", body)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		const upstreamErr = "boom: overview account detail provider details"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, errors.New(upstreamErr)
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, upstreamErr)
	})

	t.Run("rejects invalid names", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
				return catalog.AccountOverviewDetail{}, nil
			},
		})
		for _, path := range []string{"/v1/overview/accounts/", "/v1/overview/accounts/a/b"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		}
	})
}

func TestRouterProjectEnvironmentOverview(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewProjectEnvironment: func(accountID, appRef, environmentRef string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			if accountID != "123456789012" || appRef != "payments-api" || environmentRef != "prod" {
				t.Fatalf("unexpected grouped lookup: %s %s %s", accountID, appRef, environmentRef)
			}
			return catalog.ProjectEnvironmentOverviewDetail{
				AccountName:      "prod-core",
				AWSAccountID:     "123456789012",
				AllowedRegions:   []string{"ap-northeast-2", "us-west-2"},
				AppRef:           "payments-api",
				EnvironmentRef:   "prod",
				Region:           "ap-northeast-2",
				DeploymentCount:  2,
				SyncedCount:      1,
				ProgressingCount: 1,
				PausedCount:      0,
				ErrorCount:       0,
				Deployments:      []catalog.DeploymentOverview{{Name: "payments-api-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}, {Name: "payments-worker-prod", AppRef: "payments-api", EnvironmentRef: "prod", AWSAccountID: "123456789012", Region: "ap-northeast-2", ExecutionMode: "asgWeightedTargetGroups", Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "steady"}}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var detail catalog.ProjectEnvironmentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode grouped overview detail: %v", err)
	}
	if detail.AccountName != "prod-core" || detail.AppRef != "payments-api" || detail.EnvironmentRef != "prod" || detail.Region != "ap-northeast-2" || len(detail.Deployments) != 2 {
		t.Fatalf("unexpected grouped overview detail: %+v", detail)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw grouped overview payload: %v", err)
	}
	if payload["accountName"] != "prod-core" || payload["appRef"] != "payments-api" || payload["environmentRef"] != "prod" || payload["deploymentCount"] != float64(2) {
		t.Fatalf("expected grouped overview JSON fields, got %#v", payload)
	}
}

func TestOverviewProjectEnvironmentRoute(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", resp.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			return catalog.ProjectEnvironmentOverviewDetail{}, statusstore.ErrNotFound
		}})
		req := httptest.NewRequest(http.MethodGet, "/v1/overview/accounts/123456789012/projects/missing/environments/prod", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("invalid paths", func(t *testing.T) {
		router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}, OverviewProjectEnvironment: func(string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error) {
			return catalog.ProjectEnvironmentOverviewDetail{}, nil
		}})
		for _, path := range []string{"/v1/overview/accounts/", "/v1/overview/accounts/123456789012/projects", "/v1/overview/accounts/123456789012/projects/payments-api", "/v1/overview/accounts/123456789012/projects/payments-api/environments", "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod/extra"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
			}
		}
	})
}

func TestRouterOverviewDeployment(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name:               "api-prod",
				AppRef:             "payments-api",
				EnvironmentRef:     "prod-apne2",
				AccountName:        "prod-core",
				AWSAccountID:       "123456789012",
				AllowedRegions:     []string{"ap-northeast-2"},
				Region:             "ap-northeast-2",
				TargetType:         "ecs-service",
				Strategy:           "linear",
				ExecutionMode:      "ecsCodeDeploy",
				DesiredArtifact:    domain.DeploymentArtifact{Image: "payments-api:v2"},
				UserData:           domain.DeploymentUserData{Type: "file", Path: "userdata/payments-api-prod.sh"},
				DeployedArtifact:   domain.DeploymentArtifact{Image: "payments-api:v1"},
				ObservedGeneration: 7,
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
				RuntimeSummary:     &domain.RuntimeSummary{Timestamp: "2026-04-16T06:49:00Z", Source: "controller", Summary: "Traffic shift is progressing", Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "steady"}, DesiredGeneration: 8, ObservedGeneration: 7, RolloutPhase: domain.RolloutPhaseAccepted, CurrentStep: "ShiftTrafficLinear", HistoryCount: 3, Changes: []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Phase:              domain.RolloutPhaseStabilizing,
					CurrentStep:        "VerifyServiceStability",
					Summary:            "Stabilizing ECS service after CodeDeploy traffic shift",
					Steps:              []domain.RolloutStepSummary{{Kind: "CreateCodeDeployDeployment", Status: "completed"}, {Kind: "ShiftTrafficLinear", Status: "pending"}, {Kind: "VerifyServiceStability", Current: true, Status: "current"}},
					Execution:          []domain.RolloutExecutionDetail{{Resource: "ecsService", Name: "payments-api-prod-service", Description: "ECS service Tenet updates during deployment"}, {Resource: "codeDeployApplication", Name: "payments-api-prod-codedeploy-app", Description: "CodeDeploy application coordinating ECS traffic shifting"}, {Resource: "codeDeployDeploymentGroup", Name: "payments-api-prod-codedeploy-group", Description: "CodeDeploy deployment group Tenet targets for ECS rollout"}, {Resource: "productionListener", Name: "payments-api-prod-prod-listener", Description: "Production listener CodeDeploy shifts during ECS blue/green rollout"}, {Resource: "testListener", Name: "payments-api-prod-test-listener", Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion"}, {Resource: "blueTaskSetTargetGroup", Name: "payments-api-prod-blue", Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination"}, {Resource: "greenTaskSetTargetGroup", Name: "payments-api-prod-green", Description: "Green task set target group CodeDeploy warms before shifting ECS traffic"}},
				},
				Conditions: []domain.Condition{{Type: "Ready", Status: false, Reason: "Updating", Message: "service is progressing"}},
				History: []domain.StatusHistoryEntry{{
					Timestamp:          "2026-04-16T06:49:00Z",
					Source:             "controller",
					Summary:            "Shifted 10 percent of traffic",
					Sync:               domain.Sync{State: domain.SyncStateReconciling},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					Changes:            []string{"summary", "currentStep"},
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
				}},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.DeploymentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview deployment: %v", err)
	}
	if detail.Name != "api-prod" || detail.ExecutionMode != "ecsCodeDeploy" || detail.AccountName != "prod-core" {
		t.Fatalf("unexpected overview deployment detail: %+v", detail)
	}
	if detail.UserData.Type != "file" || detail.UserData.Path != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userdata metadata in deployment detail, got %+v", detail.UserData)
	}
	if detail.Rollout == nil || detail.Rollout.CurrentStep != "VerifyServiceStability" || len(detail.Rollout.Steps) != 3 {
		t.Fatalf("expected rollout summary in deployment detail, got %+v", detail)
	}
	if detail.RuntimeSummary == nil || detail.RuntimeSummary.Timestamp != "2026-04-16T06:49:00Z" || detail.RuntimeSummary.Source != "controller" || detail.RuntimeSummary.Summary != "Traffic shift is progressing" || detail.RuntimeSummary.Sync.Reason != "Traffic shift running" || detail.RuntimeSummary.DesiredGeneration != 8 || detail.RuntimeSummary.ObservedGeneration != 7 || detail.RuntimeSummary.CurrentStep != "ShiftTrafficLinear" || detail.RuntimeSummary.HistoryCount != 3 || !reflect.DeepEqual(detail.RuntimeSummary.Changes, []string{"summary", "sync", "health", "desiredGeneration", "observedGeneration", "rolloutPhase", "currentStep"}) {
		t.Fatalf("expected runtime summary in deployment detail, got %+v", detail)
	}
	if len(detail.History) != 1 || detail.History[0].Summary != "Shifted 10 percent of traffic" || detail.History[0].DesiredGeneration != 8 || detail.History[0].ObservedGeneration != 7 || detail.History[0].CurrentStep != "ShiftTrafficLinear" {
		t.Fatalf("expected runtime history in deployment detail, got %+v", detail.History)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	rollout, ok := payload["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw payload, got %#v", payload["rollout"])
	}
	if rollout["currentStep"] != "VerifyServiceStability" {
		t.Fatalf("expected rollout.currentStep field, got %#v", rollout)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	userData, ok := payload["userData"].(map[string]any)
	if !ok || userData["type"] != "file" || userData["path"] != "userdata/payments-api-prod.sh" {
		t.Fatalf("expected userData object in raw payload, got %#v", payload["userData"])
	}
	if runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["source"] != "controller" || runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficLinear" || runtimeSummary["historyCount"] != float64(3) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary JSON fields in raw payload, got %#v", runtimeSummary)
	}
	changes, ok := runtimeSummary["changes"].([]any)
	if !ok || len(changes) != 7 || changes[2] != "health" || changes[4] != "observedGeneration" {
		t.Fatalf("expected runtimeSummary.changes field in raw payload, got %#v", runtimeSummary["changes"])
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in raw payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in raw payload, got %#v", runtimeSummary["health"])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("expected rollout.steps array in raw payload, got %#v", rollout["steps"])
	}
	lastStep, ok := steps[2].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[2] object, got %#v", steps[2])
	}
	if lastStep["kind"] != "VerifyServiceStability" || lastStep["current"] != true || lastStep["status"] != "current" {
		t.Fatalf("expected rollout.steps[2] field names, got %#v", lastStep)
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["kind"] != "CreateCodeDeployDeployment" || firstStep["status"] != "completed" {
		t.Fatalf("expected rollout.steps[0] status field names, got %#v", firstStep)
	}
	execution, ok := rollout["execution"].([]any)
	if !ok || len(execution) != 7 {
		t.Fatalf("expected rollout.execution array in raw payload, got %#v", rollout["execution"])
	}
	firstExecution, ok := execution[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.execution[0] object, got %#v", execution[0])
	}
	if firstExecution["resource"] != "ecsService" || firstExecution["name"] != "payments-api-prod-service" || firstExecution["description"] != "ECS service Tenet updates during deployment" {
		t.Fatalf("expected rollout.execution[0] field names, got %#v", firstExecution)
	}
	lastExecution, ok := execution[6].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.execution[6] object, got %#v", execution[6])
	}
	if lastExecution["resource"] != "greenTaskSetTargetGroup" || lastExecution["name"] != "payments-api-prod-green" || lastExecution["description"] != "Green task set target group CodeDeploy warms before shifting ECS traffic" {
		t.Fatalf("expected rollout.execution[6] field names, got %#v", lastExecution)
	}
	history, ok := payload["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("expected history array in raw payload, got %#v", payload["history"])
	}
	historyEntry, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0] object, got %#v", history[0])
	}
	sync, ok := historyEntry["sync"].(map[string]any)
	if !ok {
		t.Fatalf("expected history[0].sync object, got %#v", historyEntry["sync"])
	}
	if sync["state"] != string(domain.SyncStateReconciling) {
		t.Fatalf("expected history[0].sync.state field, got %#v", sync)
	}
	historyChanges, ok := historyEntry["changes"].([]any)
	if !ok || len(historyChanges) != 2 || historyChanges[0] != "summary" || historyChanges[1] != "currentStep" {
		t.Fatalf("expected history[0].changes string array in raw payload, got %#v", historyEntry["changes"])
	}
	if historyEntry["currentStep"] != "ShiftTrafficLinear" || historyEntry["desiredGeneration"] != float64(8) || historyEntry["observedGeneration"] != float64(7) || historyEntry["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected history[0] field names, got %#v", historyEntry)
	}
}

func TestDeploymentOverviewDetailRouteOmitsRuntimeSummarySourceWithoutHistory(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				RuntimeSummary: &domain.RuntimeSummary{
					Summary:            "Serving stable capacity",
					Sync:               domain.Sync{State: domain.SyncStateSynced, Reason: "steady"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 8,
					RolloutPhase:       domain.RolloutPhaseCompleted,
					CurrentStep:        "Complete",
					HistoryCount:       0,
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in raw payload, got %#v", payload["runtimeSummary"])
	}
	if _, ok := runtimeSummary["source"]; ok {
		t.Fatalf("expected runtimeSummary.source to be omitted without history, got %#v", runtimeSummary)
	}
}

func TestDeploymentOverviewDetailRouteIncludesRolloutStepDetail(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected lookup name: %s", name)
			}
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				Rollout: &domain.RolloutSummary{
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCodeDeployDeployment", Detail: "Creates CodeDeploy deployment for service <api>", Status: domain.RolloutStepStatusCompleted},
						{Kind: "ShiftTrafficLinear", Detail: "Shifts 10% of production traffic", Current: true, Status: domain.RolloutStepStatusCurrent},
					},
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var detail catalog.DeploymentOverviewDetail
	if err := json.Unmarshal(resp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode overview deployment: %v", err)
	}
	if detail.Rollout == nil || len(detail.Rollout.Steps) != 2 {
		t.Fatalf("expected rollout steps in deployment detail, got %+v", detail)
	}
	if detail.Rollout.Steps[0].Detail != "Creates CodeDeploy deployment for service <api>" {
		t.Fatalf("expected rollout step detail to survive typed decode, got %+v", detail.Rollout.Steps)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	rollout, ok := payload["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in raw payload, got %#v", payload["rollout"])
	}
	steps, ok := rollout["steps"].([]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected rollout.steps array in raw payload, got %#v", rollout["steps"])
	}
	firstStep, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[0] object, got %#v", steps[0])
	}
	if firstStep["detail"] != "Creates CodeDeploy deployment for service <api>" {
		t.Fatalf("expected rollout.steps[0].detail field, got %#v", firstStep)
	}
	secondStep, ok := steps[1].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout.steps[1] object, got %#v", steps[1])
	}
	if secondStep["detail"] != "Shifts 10% of production traffic" || secondStep["current"] != true || secondStep["status"] != string(domain.RolloutStepStatusCurrent) {
		t.Fatalf("expected rollout.steps[1] detail/current/status fields, got %#v", secondStep)
	}
	if _, exists := firstStep["Detail"]; exists {
		t.Fatalf("unexpected Go-style detail field name in %#v", firstStep)
	}
}

func TestOverviewDeploymentResponseIncludesRuntimeSummary(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name: "api-prod",
				RuntimeSummary: &domain.RuntimeSummary{
					Timestamp:          "2026-04-16T06:49:00Z",
					Summary:            "Traffic shift is progressing",
					Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
					Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
					DesiredGeneration:  8,
					ObservedGeneration: 7,
					RolloutPhase:       domain.RolloutPhaseAccepted,
					CurrentStep:        "ShiftTrafficLinear",
					HistoryCount:       3,
				},
			}, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode raw overview deployment payload: %v", err)
	}
	runtimeSummary, ok := payload["runtimeSummary"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtimeSummary object in overview deployment payload, got %#v", payload["runtimeSummary"])
	}
	if runtimeSummary["summary"] != "Traffic shift is progressing" || runtimeSummary["timestamp"] != "2026-04-16T06:49:00Z" || runtimeSummary["desiredGeneration"] != float64(8) || runtimeSummary["observedGeneration"] != float64(7) || runtimeSummary["currentStep"] != "ShiftTrafficLinear" || runtimeSummary["historyCount"] != float64(3) || runtimeSummary["rolloutPhase"] != string(domain.RolloutPhaseAccepted) {
		t.Fatalf("expected runtimeSummary fields in overview deployment payload, got %#v", runtimeSummary)
	}
	syncSummary, ok := runtimeSummary["sync"].(map[string]any)
	if !ok || syncSummary["state"] != string(domain.SyncStateReconciling) || syncSummary["reason"] != "Traffic shift running" {
		t.Fatalf("expected runtimeSummary.sync fields in overview deployment payload, got %#v", runtimeSummary["sync"])
	}
	healthSummary, ok := runtimeSummary["health"].(map[string]any)
	if !ok || healthSummary["overall"] != string(domain.HealthHealthy) || healthSummary["summary"] != "steady" {
		t.Fatalf("expected runtimeSummary.health fields in overview deployment payload, got %#v", runtimeSummary["health"])
	}
}

func TestRouterOverviewDeploymentUnavailable(t *testing.T) {
	router := NewRouter(Options{BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"}})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentNotFound(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/missing", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentProviderErrorReturnsInternalServerError(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, errors.New("boom")
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}

func TestRouterOverviewDeploymentRejectsInvalidNames(t *testing.T) {
	router := NewRouter(Options{
		BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, nil
		},
	})
	for _, path := range []string{"/v1/overview/deployments/", "/v1/overview/deployments/a/b"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
		}
	}
}

func TestRouterOverviewDeploymentResourceLogs(t *testing.T) {
	t.Run("returns resource log payload", func(t *testing.T) {
		expected := resourcelogs.Result{
			DeploymentName: "api-prod",
			ResourceType:   "ecsService",
			ResourceName:   "payments-api-prod-service",
			Available:      true,
			Source:         resourcelogs.SourceCloudWatch,
			Entries: []resourcelogs.Entry{
				{Timestamp: "2026-04-16T07:10:00Z", Message: "service became healthy", Stream: "ecs/payments-api/service/abc123"},
				{Timestamp: "2026-04-16T07:09:00Z", Message: "registered 2/2 targets"},
			},
			UnavailableReason: "",
		}

		var gotRequest struct {
			deploymentName string
			resourceType   string
			resourceName   string
		}
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				gotRequest.deploymentName = deploymentName
				gotRequest.resourceType = resourceType
				gotRequest.resourceName = resourceName
				return expected, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !reflect.DeepEqual(gotRequest, struct {
			deploymentName string
			resourceType   string
			resourceName   string
		}{deploymentName: "api-prod", resourceType: "ecsService", resourceName: "payments-api-prod-service"}) {
			t.Fatalf("unexpected provider request: %+v", gotRequest)
		}

		var got resourcelogs.Result
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode resource logs payload: %v", err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("unexpected resource logs payload: %+v", got)
		}

		var raw map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to decode raw resource logs payload: %v", err)
		}
		for _, key := range []string{"deploymentName", "resourceType", "resourceName", "available", "source", "entries", "unavailableReason"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected %q field in payload, got %#v", key, raw)
			}
		}
		entries, ok := raw["entries"].([]any)
		if !ok || len(entries) != 2 {
			t.Fatalf("expected entries array in payload, got %#v", raw["entries"])
		}
		entry, ok := entries[0].(map[string]any)
		if !ok || entry["timestamp"] != "2026-04-16T07:10:00Z" || entry["message"] != "service became healthy" || entry["stream"] != "ecs/payments-api/service/abc123" {
			t.Fatalf("expected entry shape in payload, got %#v", entries[0])
		}
		entry, ok = entries[1].(map[string]any)
		if !ok {
			t.Fatalf("expected second entry shape in payload, got %#v", entries[1])
		}
		if _, ok := entry["stream"]; ok {
			t.Fatalf("expected optional stream to be omitted when empty, got %#v", entries[1])
		}
	})

	for _, tc := range []struct {
		name         string
		query        string
		wantBodyPart string
	}{
		{name: "missing resourceType", query: "resourceName=payments-api-prod-service", wantBodyPart: "resourceType"},
		{name: "missing resourceName", query: "resourceType=ecsService", wantBodyPart: "resourceName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Options{
				BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
				OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
					t.Fatalf("overview deployment lookup should not run for invalid query")
					return catalog.DeploymentOverviewDetail{}, nil
				},
				OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
					t.Fatalf("resource logs provider should not run for invalid query")
					return resourcelogs.Result{}, nil
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?"+tc.query, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d with body %q", resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), tc.wantBodyPart) {
				t.Fatalf("expected 400 body to mention %q, got %q", tc.wantBodyPart, resp.Body.String())
			}
		})
	}

	t.Run("returns 503 when provider is unavailable", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "resource logs") {
			t.Fatalf("expected 503 body to mention resource logs, got %q", resp.Body.String())
		}
	})

	t.Run("propagates deployment not found as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "missing" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				t.Fatalf("resource logs provider should not run when deployment lookup fails")
				return resourcelogs.Result{}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/missing/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), statusstore.ErrNotFound.Error()) {
			t.Fatalf("expected 404 body to include %q, got %q", statusstore.ErrNotFound.Error(), resp.Body.String())
		}
	})

	t.Run("propagates deployment resource binding failures as 404", func(t *testing.T) {
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected resource logs request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourcelogs.Result{}, statusstore.ErrNotFound
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d with body %q", resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), statusstore.ErrNotFound.Error()) {
			t.Fatalf("expected 404 body to include %q, got %q", statusstore.ErrNotFound.Error(), resp.Body.String())
		}
	})

	t.Run("returns generic 500 when resource logs provider errors", func(t *testing.T) {
		const providerErr = "boom: resource logs provider internals"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{Name: name}, nil
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				if deploymentName != "api-prod" || resourceType != "ecsService" || resourceName != "payments-api-prod-service" {
					t.Fatalf("unexpected resource logs request: deployment=%q type=%q name=%q", deploymentName, resourceType, resourceName)
				}
				return resourcelogs.Result{}, errors.New(providerErr)
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, providerErr)
	})

	t.Run("returns generic 500 when deployment lookup errors on resource logs route", func(t *testing.T) {
		const providerErr = "boom: deployment lookup internals"
		router := NewRouter(Options{
			BuildInfo: buildinfo.Info{Version: "dev", Commit: "abc123"},
			OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
				if name != "api-prod" {
					t.Fatalf("unexpected deployment lookup: %s", name)
				}
				return catalog.DeploymentOverviewDetail{}, errors.New(providerErr)
			},
			OverviewDeploymentResourceLogs: func(ctx context.Context, deploymentName, resourceType, resourceName string) (resourcelogs.Result, error) {
				t.Fatalf("resource logs provider should not run when deployment lookup errors")
				return resourcelogs.Result{}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assertInternalServerErrorResponse(t, resp, providerErr)
	})
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
	for _, fragment := range []string{"Rollout phase", "Current step", "Planned steps", "ShiftTrafficCanary", "CreateCodeDeployDeployment", "deployments", "synced", "progressing", "paused", "error"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to render rollout summary label %q, got %q", fragment, body)
		}
	}
	for _, unsafe := range []string{
		"root.innerHTML = accounts.map",
		"document.getElementById('accounts').innerHTML = `<div class=\"empty\">Failed to load overview: ${error.message}</div>`",
		"['Progressing', deployments.length - synced]",
		"const progressing = deployments.filter((deployment) => deployment.sync?.state === 'Reconciling' || deployment.sync?.state === 'OutOfSync').length;",
	} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("expected UI to avoid unsafe rendering or misclassified summary logic %q, got %q", unsafe, body)
		}
	}
	for _, fragment := range []string{
		"textContent",
		"createElement",
		"const progressingStates = new Set(['Reconciling', 'OutOfSync', 'Drifted']);",
		"progressingStates.has(deployment.sync?.state)",
		"case 'Reconciling':",
		"case 'OutOfSync':",
		"case 'Drifted':\n            return 'progress';",
		"case 'Paused':\n            return 'paused';",
		"case 'Error':\n            return 'error';",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("expected UI to include safe rendering and precise sync-state pill mapping %q, got %q", fragment, body)
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
		OverviewAccount: func(string) (catalog.AccountOverviewDetail, error) {
			return catalog.AccountOverviewDetail{}, nil
		},
		OverviewDeployment: func(name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, nil
		},
		OverviewDeploymentResourceLogs: func(context.Context, string, string, string) (resourcelogs.Result, error) {
			return resourcelogs.Result{}, nil
		},
		DeploymentStatuses: func() ([]statusstore.Snapshot, error) {
			return []statusstore.Snapshot{}, nil
		},
		DeploymentStatus: func(name string) (statusstore.Snapshot, error) {
			return statusstore.Snapshot{}, nil
		},
	})

	paths := []string{"/", "/healthz", "/v1/info", "/v1/catalog/summary", "/v1/catalog/deployments", "/v1/catalog/environments", "/v1/catalog/accounts", "/v1/catalog/placements", "/v1/catalog/topology", "/v1/overview/applications", "/v1/overview/accounts", "/v1/overview/accounts/123456789012", "/v1/overview/accounts/123456789012/projects/payments-api/environments/prod", "/v1/overview/deployments/api-prod", "/v1/overview/deployments/api-prod/resource-logs?resourceType=ecsService&resourceName=payments-api-prod-service", "/v1/status/deployments", "/v1/status/deployments/api-prod"}
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
