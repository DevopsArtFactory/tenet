package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
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

// Options configures the API router.
type Options struct {
	BuildInfo           buildinfo.Info
	CatalogSummary      func() (catalog.Summary, error)
	CatalogDeployments  func() ([]catalog.DeploymentView, error)
	CatalogEnvironments func() ([]catalog.EnvironmentView, error)
	CatalogAccounts     func() ([]catalog.AccountView, error)
	CatalogPlacements   func() ([]catalog.PlacementView, error)
	CatalogTopology     func() (catalog.TopologySnapshot, error)
	DeploymentStatuses  func() ([]statusstore.Snapshot, error)
	DeploymentStatus    func(string) (statusstore.Snapshot, error)
}

// NewRouter builds the API router for the Tenet control plane.
func NewRouter(options Options) http.Handler {
	mux := http.NewServeMux()
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(topology)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			if errors.Is(err, statusstore.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})

	return mux
}
