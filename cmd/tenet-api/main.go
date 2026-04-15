package main

import (
	"log"
	"net/http"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")
	repoRoot := os.Getenv("TENET_REPO_ROOT")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s repo_root=%s", info.Version, info.Commit, addr, repoRoot)

	options := api.Options{BuildInfo: info}
	if repoRoot != "" {
		options.CatalogSummary = catalog.RepositorySummaryProvider(repoRoot)
		options.CatalogDeployments = catalog.RepositoryDeploymentProvider(repoRoot)
	}

	if err := http.ListenAndServe(addr, api.NewRouter(options)); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
