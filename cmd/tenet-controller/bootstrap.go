package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
)

type startupConfig struct {
	Info                buildinfo.Info
	WorkerCount         string
	RepoRoot            string
	CatalogSource       controllerCatalogSource
	StatusBackend       string
	StatusPath          string
	RedisAddr           string
	StatusSeedPath      string
	RuntimeHistoryLimit int
	RequeueInterval     time.Duration
	Summary             *catalog.Summary
}

func loadStartupConfig(getenv func(string) string) (startupConfig, error) {
	version := envOrDefaultFrom(getenv, "TENET_VERSION", "dev")
	commit := envOrDefaultFrom(getenv, "TENET_COMMIT", "unknown")
	workerCount := envOrDefaultFrom(getenv, "TENET_CONTROLLER_WORKERS", "1")
	statusBackend := envOrDefaultFrom(getenv, "TENET_STATUS_BACKEND", "file")
	statusPath := getenv("TENET_STATUS_PATH")
	redisAddr := getenv("TENET_REDIS_ADDR")
	statusSeedPath := getenv("TENET_STATUS_SEED_PATH")
	requeueSeconds, err := controllerRequeueSeconds(getenv)
	if err != nil {
		return startupConfig{}, err
	}
	runtimeHistoryLimit, err := controllerRuntimeHistoryLimit(getenv)
	if err != nil {
		return startupConfig{}, err
	}
	catalogSource, err := loadControllerCatalogSourceFromEnv(getenv)
	if err != nil {
		return startupConfig{}, err
	}

	config := startupConfig{
		Info:                buildinfo.Current(version, commit),
		WorkerCount:         workerCount,
		RepoRoot:            catalogSource.filesystemRoot,
		CatalogSource:       catalogSource,
		StatusBackend:       statusBackend,
		StatusPath:          statusPath,
		RedisAddr:           redisAddr,
		StatusSeedPath:      statusSeedPath,
		RuntimeHistoryLimit: runtimeHistoryLimit,
		RequeueInterval:     time.Duration(requeueSeconds) * time.Second,
	}
	if !catalogSource.configured() {
		return config, nil
	}

	compiled, err := catalogSource.loadCatalog(context.Background())
	if err != nil {
		return startupConfig{}, fmt.Errorf("load controller catalog: %w", err)
	}
	summary := catalog.Summarize(compiled)
	config.Summary = &summary
	return config, nil
}

func (c startupConfig) catalogSource() controllerCatalogSource {
	if c.CatalogSource.configured() {
		return c.CatalogSource
	}
	if c.RepoRoot == "" {
		return controllerCatalogSource{}
	}
	return newControllerFilesystemCatalogSource(c.RepoRoot)
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

func controllerRuntimeHistoryLimit(getenv func(string) string) (int, error) {
	value := envOrDefaultFrom(getenv, "TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT", strconv.Itoa(domain.RuntimeHistoryMaxEntries))
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT: %w", err)
	}
	if limit <= 0 {
		return 0, fmt.Errorf("TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT must be positive")
	}
	if limit > domain.RuntimeHistoryMaxConfigurableEntries {
		return 0, fmt.Errorf("TENET_CONTROLLER_RUNTIME_HISTORY_LIMIT must be <= %d", domain.RuntimeHistoryMaxConfigurableEntries)
	}
	return limit, nil
}

func envOrDefaultFrom(getenv func(string) string, name, fallback string) string {
	value := getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
