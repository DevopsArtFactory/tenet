package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

type startupConfig struct {
	Info            buildinfo.Info
	WorkerCount     string
	RepoRoot        string
	StatusBackend   string
	StatusPath      string
	RedisAddr       string
	StatusSeedPath  string
	RequeueInterval time.Duration
	Summary         *catalog.Summary
}

func loadStartupConfig(getenv func(string) string) (startupConfig, error) {
	version := envOrDefaultFrom(getenv, "TENET_VERSION", "dev")
	commit := envOrDefaultFrom(getenv, "TENET_COMMIT", "unknown")
	workerCount := envOrDefaultFrom(getenv, "TENET_CONTROLLER_WORKERS", "1")
	repoRoot := getenv("TENET_REPO_ROOT")
	statusBackend := envOrDefaultFrom(getenv, "TENET_STATUS_BACKEND", "file")
	statusPath := getenv("TENET_STATUS_PATH")
	redisAddr := getenv("TENET_REDIS_ADDR")
	statusSeedPath := getenv("TENET_STATUS_SEED_PATH")
	requeueSeconds, err := controllerRequeueSeconds(getenv)
	if err != nil {
		return startupConfig{}, err
	}

	config := startupConfig{
		Info:            buildinfo.Current(version, commit),
		WorkerCount:     workerCount,
		RepoRoot:        repoRoot,
		StatusBackend:   statusBackend,
		StatusPath:      statusPath,
		RedisAddr:       redisAddr,
		StatusSeedPath:  statusSeedPath,
		RequeueInterval: time.Duration(requeueSeconds) * time.Second,
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

func controllerRequeueSeconds(getenv func(string) string) (int, error) {
	value := envOrDefaultFrom(getenv, "TENET_CONTROLLER_REQUEUE_SECONDS", "30")
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse TENET_CONTROLLER_REQUEUE_SECONDS: %w", err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("TENET_CONTROLLER_REQUEUE_SECONDS must be positive")
	}
	return seconds, nil
}

func envOrDefaultFrom(getenv func(string) string, name, fallback string) string {
	value := getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
