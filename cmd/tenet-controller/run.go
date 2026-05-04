package main

import (
	"context"
	"log"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
)

func runController(ctx context.Context, logger *log.Logger, config startupConfig) {
	catalogSource := config.catalogSource()
	catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL := catalogSource.logFields()
	if config.Summary != nil {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s catalog_source=%s repo_root=%s github_repository=%s github_ref=%s github_path=%s github_base_url=%s status_backend=%s status_path=%s redis_addr=%s requeue_interval=%s accounts=%d environments=%d deployments=%d", config.Info.Version, config.Info.Commit, config.WorkerCount, catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL, config.StatusBackend, config.StatusPath, config.RedisAddr, config.RequeueInterval, config.Summary.AccountCount, config.Summary.EnvironmentCount, config.Summary.DeploymentCount)
	} else {
		logger.Printf("starting tenet-controller version=%s commit=%s workers=%s catalog_source=%s repo_root=%s github_repository=%s github_ref=%s github_path=%s github_base_url=%s status_backend=%s status_path=%s redis_addr=%s requeue_interval=%s", config.Info.Version, config.Info.Commit, config.WorkerCount, catalogSourceKind, repoRoot, githubRepository, githubRef, githubPath, githubBaseURL, config.StatusBackend, config.StatusPath, config.RedisAddr, config.RequeueInterval)
	}

	var store runtimeStatusStore
	if catalogSource.configured() {
		var err error
		store, err = openRuntimeStatusStore(ctx, logger, config)
		if err != nil {
			logger.Fatalf("open runtime status store: %v", err)
		}
		if store != nil {
			defer func() {
				if err := store.Close(); err != nil {
					logger.Printf("close runtime status store: %v", err)
				}
			}()
		}
	}

	if err := emitRuntimePlans(ctx, logger, config, store); err != nil {
		logger.Printf("controller runtime error: %v", err)
	}

	refreshSubscription, err := controllerRefreshSubscriptionOpener(ctx, config, store)
	if err != nil {
		logger.Printf("controller refresh handoff unavailable: %v", err)
	}
	if refreshSubscription != nil {
		defer func() {
			if err := refreshSubscription.Close(); err != nil {
				logger.Printf("close controller refresh handoff: %v", err)
			}
		}()
		logger.Printf("controller refresh handoff enabled backend=redis channel=%s", refreshSubscription.Channel())
	}

	interval := config.RequeueInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var refreshMessages <-chan controllerrefresh.Message
	if refreshSubscription != nil {
		refreshMessages = refreshSubscription.C()
	}

	for {
		select {
		case <-ctx.Done():
			logger.Printf("stopping tenet-controller")
			return
		case message, ok := <-refreshMessages:
			if !ok {
				refreshMessages = nil
				continue
			}
			if message.Err != nil {
				logger.Printf("controller refresh handoff ignored: %v", message.Err)
				continue
			}
			receivedAt := time.Now().UTC()
			logger.Printf(
				"controller refresh handoff received request_id=%s delivery_id=%s repository=%s ref=%s after=%s requested_at=%s",
				message.Request.RequestID,
				message.Request.DeliveryID,
				message.Request.Repository,
				message.Request.Ref,
				message.Request.After,
				message.Request.RequestedAt,
			)
			if err := emitRuntimePlansForRefresh(ctx, logger, config, store, &message.Request, receivedAt); err != nil {
				logger.Printf("controller runtime error: %v", err)
			}
		case <-ticker.C:
			if err := emitRuntimePlans(ctx, logger, config, store); err != nil {
				logger.Printf("controller runtime error: %v", err)
			}
		}
	}
}
