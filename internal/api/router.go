package api

import (
	"encoding/json"
	"net/http"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
)

// NewRouter builds the API router for the Tenet control plane.
func NewRouter(info buildinfo.Info) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})

	return mux
}
