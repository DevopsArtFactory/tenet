package main

import (
	"context"
	"log"
	"time"
)

func runController(ctx context.Context, logger *log.Logger, config startupConfig) {
	if config.Summary != nil {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s status_backend=%s status_path=%s redis_addr=%s requeue_interval=%s accounts=%d environments=%d deployments=%d", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot, config.StatusBackend, config.StatusPath, config.RedisAddr, config.RequeueInterval, config.Summary.AccountCount, config.Summary.EnvironmentCount, config.Summary.DeploymentCount)
	} else {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s status_backend=%s status_path=%s redis_addr=%s requeue_interval=%s", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot, config.StatusBackend, config.StatusPath, config.RedisAddr, config.RequeueInterval)
	}

	if err := emitRuntimePlans(logger, config); err != nil {
		logger.Printf("controller runtime error: %v", err)
	}

	interval := config.RequeueInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Printf("stopping tenet-controller")
			return
		case <-ticker.C:
			if err := emitRuntimePlans(logger, config); err != nil {
				logger.Printf("controller runtime error: %v", err)
			}
		}
	}
}
