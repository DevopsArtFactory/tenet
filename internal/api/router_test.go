package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
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
	})

	paths := []string{"/healthz", "/v1/info", "/v1/catalog/summary", "/v1/catalog/deployments", "/v1/catalog/environments", "/v1/catalog/accounts", "/v1/catalog/placements", "/v1/catalog/topology"}
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
