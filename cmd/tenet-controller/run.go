package main

import (
	"context"
	"log"
)

func runController(ctx context.Context, logger *log.Logger, config startupConfig) {
	if config.Summary != nil {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s accounts=%d environments=%d deployments=%d", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot, config.Summary.AccountCount, config.Summary.EnvironmentCount, config.Summary.DeploymentCount)
	} else {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot)
	}

	<-ctx.Done()
	logger.Printf("stopping tenet-controller")
}
