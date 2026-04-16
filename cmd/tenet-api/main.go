package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")
	repoRoot := os.Getenv("TENET_REPO_ROOT")
	statusBackend := envOrDefault("TENET_STATUS_BACKEND", "file")
	statusPath := os.Getenv("TENET_STATUS_PATH")
	redisAddr := os.Getenv("TENET_REDIS_ADDR")
	statusSeedPath := os.Getenv("TENET_STATUS_SEED_PATH")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s repo_root=%s status_backend=%s status_path=%s redis_addr=%s", info.Version, info.Commit, addr, repoRoot, statusBackend, statusPath, redisAddr)

	store := mustOpenStatusStore(statusBackend, statusPath, redisAddr)
	if store != nil {
		if err := statusstore.SeedFromFileIfEmpty(context.Background(), store, statusSeedPath); err != nil {
			log.Fatalf("seed status store: %v", err)
		}
	}
	options := newAPIOptions(info, repoRoot, store)

	if err := http.ListenAndServe(addr, api.NewRouter(options)); err != nil {
		log.Fatal(err)
	}
}

func newAPIOptions(info buildinfo.Info, repoRoot string, store statusstore.Store) api.Options {
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
		loadSnapshots := func() ([]statusstore.Snapshot, error) {
			if store == nil {
				return nil, nil
			}
			return store.List(context.Background())
		}
		options.OverviewApplications = func() ([]catalog.ApplicationOverview, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots()
			if err != nil {
				return nil, err
			}
			return catalog.BuildApplicationOverviews(compiled, snapshots)
		}
		options.OverviewAccounts = func() ([]catalog.AccountOverview, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return nil, err
			}
			snapshots, err := loadSnapshots()
			if err != nil {
				return nil, err
			}
			return catalog.BuildAccountOverviews(compiled, snapshots)
		}
		options.OverviewAccount = catalog.RepositoryAccountOverviewDetailProvider(repoRoot, loadSnapshots)
		options.OverviewProjectEnvironment = catalog.RepositoryProjectEnvironmentOverviewDetailProvider(repoRoot, loadSnapshots)
		options.OverviewDeployment = func(name string) (catalog.DeploymentOverviewDetail, error) {
			compiled, err := catalog.LoadFromRepository(repoRoot)
			if err != nil {
				return catalog.DeploymentOverviewDetail{}, err
			}
			if store == nil {
				return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
			}
			snapshot, err := store.Get(context.Background(), name)
			if err != nil {
				if errors.Is(err, statusstore.ErrNotFound) {
					return catalog.BuildDeploymentOverviewDetail(compiled, nil, name)
				}
				return catalog.DeploymentOverviewDetail{}, err
			}
			return catalog.BuildDeploymentOverviewDetail(compiled, []statusstore.Snapshot{snapshot}, name)
		}
	}
	if store == nil {
		return options
	}
	options.DeploymentStatuses = func() ([]statusstore.Snapshot, error) {
		return store.List(context.Background())
	}
	options.DeploymentStatus = func(name string) (statusstore.Snapshot, error) {
		return store.Get(context.Background(), name)
	}
	return options
}

func mustOpenStatusStore(backend, path, redisAddr string) statusstore.Store {
	switch backend {
	case "", "file":
		if path == "" {
			return nil
		}
		return statusstore.NewFileStore(path)
	case "redis":
		if redisAddr == "" {
			log.Fatal("TENET_REDIS_ADDR is required when TENET_STATUS_BACKEND=redis")
		}
		client := redis.NewClient(&redis.Options{Addr: redisAddr})
		if err := client.Ping(context.Background()).Err(); err != nil {
			log.Fatalf("connect redis status store: %v", err)
		}
		return statusstore.NewRedisStore(client, os.Getenv("TENET_REDIS_KEY_PREFIX"))
	default:
		log.Fatalf("unsupported TENET_STATUS_BACKEND %q", backend)
		return nil
	}
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
