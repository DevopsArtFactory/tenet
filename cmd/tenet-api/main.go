package main

import (
	"log"
	"net/http"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")
	repoRoot := os.Getenv("TENET_REPO_ROOT")
	statusPath := os.Getenv("TENET_STATUS_PATH")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s repo_root=%s status_path=%s", info.Version, info.Commit, addr, repoRoot, statusPath)

	options := api.Options{BuildInfo: info}
	if repoRoot != "" {
		if _, err := catalog.LoadFromRepository(repoRoot); err != nil {
			log.Fatalf("invalid TENET_REPO_ROOT: %v", err)
		}
		options.CatalogSummary = catalog.RepositorySummaryProvider(repoRoot)
		options.CatalogDeployments = catalog.RepositoryDeploymentProvider(repoRoot)
		options.CatalogEnvironments = catalog.RepositoryEnvironmentProvider(repoRoot)
		options.CatalogAccounts = catalog.RepositoryAccountProvider(repoRoot)
		options.CatalogPlacements = catalog.RepositoryPlacementProvider(repoRoot)
		options.CatalogTopology = catalog.RepositoryTopologyProvider(repoRoot)
	}
	if statusPath != "" {
		options.DeploymentStatuses = func() ([]statusstore.Snapshot, error) {
			loaded, err := statusstore.Load(statusPath)
			if err != nil {
				return nil, err
			}
			return statusstore.List(loaded), nil
		}
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
