package main

import (
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

type startupConfig struct {
	Info        buildinfo.Info
	WorkerCount string
	RepoRoot    string
	Summary     *catalog.Summary
}

func loadStartupConfig(getenv func(string) string) (startupConfig, error) {
	version := envOrDefaultFrom(getenv, "TENET_VERSION", "dev")
	commit := envOrDefaultFrom(getenv, "TENET_COMMIT", "unknown")
	workerCount := envOrDefaultFrom(getenv, "TENET_CONTROLLER_WORKERS", "1")
	repoRoot := getenv("TENET_REPO_ROOT")

	config := startupConfig{
		Info:        buildinfo.Current(version, commit),
		WorkerCount: workerCount,
		RepoRoot:    repoRoot,
	}
	if repoRoot == "" {
		return config, nil
	}

	compiled, err := catalog.LoadFromRepository(repoRoot)
	if err != nil {
		return startupConfig{}, fmt.Errorf("load controller repository catalog: %w", err)
	}
	summary := catalog.Summarize(compiled)
	config.Summary = &summary
	return config, nil
}

func envOrDefaultFrom(getenv func(string) string, name, fallback string) string {
	value := getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
