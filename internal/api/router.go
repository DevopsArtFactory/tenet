package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/resourcelogs"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	return false
}

func respondInternalError(w http.ResponseWriter) {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func respondLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, statusstore.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondInternalError(w)
}

// Options configures the API router.
type Options struct {
	BuildInfo                      buildinfo.Info
	CatalogSummary                 func() (catalog.Summary, error)
	CatalogDeployments             func() ([]catalog.DeploymentView, error)
	CatalogEnvironments            func() ([]catalog.EnvironmentView, error)
	CatalogAccounts                func() ([]catalog.AccountView, error)
	CatalogPlacements              func() ([]catalog.PlacementView, error)
	CatalogTopology                func() (catalog.TopologySnapshot, error)
	OverviewApplications           func() ([]catalog.ApplicationOverview, error)
	OverviewApplication            func(string) (catalog.ApplicationOverviewDetail, error)
	OverviewAccounts               func() ([]catalog.AccountOverview, error)
	OverviewAccount                func(string) (catalog.AccountOverviewDetail, error)
	OverviewProjectEnvironment     func(string, string, string) (catalog.ProjectEnvironmentOverviewDetail, error)
	OverviewDeployment             func(string) (catalog.DeploymentOverviewDetail, error)
	OverviewDeploymentEvents       func(string) (catalog.DeploymentOverviewEvents, error)
	OverviewDeploymentResourceLogs func(context.Context, string, string, string) (resourcelogs.Result, error)
	DeploymentStatuses             func() ([]statusstore.Snapshot, error)
	DeploymentStatus               func(string) (statusstore.Snapshot, error)
}

// NewRouter builds the API router for the Tenet control plane.
func NewRouter(options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		serveUI(w)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(options.BuildInfo)
	})
	mux.HandleFunc("/v1/catalog/summary", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogSummary == nil {
			http.Error(w, "catalog summary unavailable", http.StatusServiceUnavailable)
			return
		}
		summary, err := options.CatalogSummary()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summary)
	})
	mux.HandleFunc("/v1/catalog/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogDeployments == nil {
			http.Error(w, "catalog deployments unavailable", http.StatusServiceUnavailable)
			return
		}
		deployments, err := options.CatalogDeployments()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployments)
	})
	mux.HandleFunc("/v1/catalog/environments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogEnvironments == nil {
			http.Error(w, "catalog environments unavailable", http.StatusServiceUnavailable)
			return
		}
		environments, err := options.CatalogEnvironments()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(environments)
	})
	mux.HandleFunc("/v1/catalog/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogAccounts == nil {
			http.Error(w, "catalog accounts unavailable", http.StatusServiceUnavailable)
			return
		}
		accounts, err := options.CatalogAccounts()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(accounts)
	})
	mux.HandleFunc("/v1/catalog/placements", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogPlacements == nil {
			http.Error(w, "catalog placements unavailable", http.StatusServiceUnavailable)
			return
		}
		placements, err := options.CatalogPlacements()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(placements)
	})
	mux.HandleFunc("/v1/catalog/topology", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.CatalogTopology == nil {
			http.Error(w, "catalog topology unavailable", http.StatusServiceUnavailable)
			return
		}
		topology, err := options.CatalogTopology()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(topology)
	})
	mux.HandleFunc("/v1/overview/applications", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.OverviewApplications == nil {
			http.Error(w, "overview applications unavailable", http.StatusServiceUnavailable)
			return
		}
		applications, err := options.OverviewApplications()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(applications)
	})
	mux.HandleFunc("/v1/overview/applications/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.OverviewApplication == nil {
			http.Error(w, "overview application unavailable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/overview/applications/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "invalid deployment name", http.StatusBadRequest)
			return
		}
		detail, err := options.OverviewApplication(name)
		if err != nil {
			respondLookupError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	})
	mux.HandleFunc("/v1/overview/accounts", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.OverviewAccounts == nil {
			http.Error(w, "overview accounts unavailable", http.StatusServiceUnavailable)
			return
		}
		accounts, err := options.OverviewAccounts()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(accounts)
	})
	mux.HandleFunc("/v1/overview/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/overview/accounts/")
		segments := strings.Split(path, "/")
		switch {
		case len(segments) == 1 && segments[0] != "":
			if options.OverviewAccount == nil {
				http.Error(w, "overview account unavailable", http.StatusServiceUnavailable)
				return
			}
			detail, err := options.OverviewAccount(segments[0])
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		case len(segments) == 5 && segments[0] != "" && segments[1] == "projects" && segments[2] != "" && segments[3] == "environments" && segments[4] != "":
			if options.OverviewProjectEnvironment == nil {
				http.Error(w, "overview project environment unavailable", http.StatusServiceUnavailable)
				return
			}
			detail, err := options.OverviewProjectEnvironment(segments[0], segments[2], segments[4])
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		default:
			http.Error(w, "invalid overview account path", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/v1/overview/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/overview/deployments/")
		switch {
		case strings.HasSuffix(path, "/events"):
			if options.OverviewDeploymentEvents == nil {
				http.Error(w, "overview deployment events unavailable", http.StatusServiceUnavailable)
				return
			}
			name := strings.TrimSuffix(path, "/events")
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			events, err := options.OverviewDeploymentEvents(name)
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(events)
		case strings.HasSuffix(path, "/resource-logs"):
			if options.OverviewDeployment == nil {
				http.Error(w, "overview deployment unavailable", http.StatusServiceUnavailable)
				return
			}
			if options.OverviewDeploymentResourceLogs == nil {
				http.Error(w, "overview deployment resource logs unavailable", http.StatusServiceUnavailable)
				return
			}
			name := strings.TrimSuffix(path, "/resource-logs")
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			resourceType := strings.TrimSpace(r.URL.Query().Get("resourceType"))
			if resourceType == "" {
				http.Error(w, "resourceType is required", http.StatusBadRequest)
				return
			}
			resourceName := strings.TrimSpace(r.URL.Query().Get("resourceName"))
			if resourceName == "" {
				http.Error(w, "resourceName is required", http.StatusBadRequest)
				return
			}
			if _, err := options.OverviewDeployment(name); err != nil {
				respondLookupError(w, err)
				return
			}
			result, err := options.OverviewDeploymentResourceLogs(r.Context(), name, resourceType, resourceName)
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
		default:
			if options.OverviewDeployment == nil {
				http.Error(w, "overview deployment unavailable", http.StatusServiceUnavailable)
				return
			}
			name := path
			if name == "" || strings.Contains(name, "/") {
				http.Error(w, "invalid deployment name", http.StatusBadRequest)
				return
			}
			detail, err := options.OverviewDeployment(name)
			if err != nil {
				respondLookupError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
		}
	})
	mux.HandleFunc("/v1/status/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.DeploymentStatuses == nil {
			http.Error(w, "deployment statuses unavailable", http.StatusServiceUnavailable)
			return
		}
		snapshots, err := options.DeploymentStatuses()
		if err != nil {
			respondInternalError(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshots)
	})
	mux.HandleFunc("/v1/status/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if options.DeploymentStatus == nil {
			http.Error(w, "deployment status unavailable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/status/deployments/")
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "invalid deployment name", http.StatusBadRequest)
			return
		}
		snapshot, err := options.DeploymentStatus(name)
		if err != nil {
			respondLookupError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})

	return mux
}
