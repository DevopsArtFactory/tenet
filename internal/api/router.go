package api

import (
	"encoding/json"
	"net/http"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

// Options configures the API router.
type Options struct {
	BuildInfo          buildinfo.Info
	CatalogSummary     func() (catalog.Summary, error)
	CatalogDeployments func() ([]catalog.DeploymentView, error)
}

// NewRouter builds the API router for the Tenet control plane.
func NewRouter(options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(options.BuildInfo)
	})
	mux.HandleFunc("/v1/catalog/summary", func(w http.ResponseWriter, _ *http.Request) {
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
	mux.HandleFunc("/v1/catalog/deployments", func(w http.ResponseWriter, _ *http.Request) {
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

	return mux
}
