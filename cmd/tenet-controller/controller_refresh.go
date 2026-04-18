package main

import (
	"context"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/redis/go-redis/v9"
)

type redisControllerRefreshClientProvider interface {
	Client() redis.UniversalClient
}

var controllerRefreshSubscriptionOpener = openControllerRefreshSubscription

func openControllerRefreshSubscription(ctx context.Context, config startupConfig, store runtimeStatusStore) (controllerrefresh.Subscription, error) {
	source := config.catalogSource()
	if source.kind != controllerCatalogSourceGitHub || config.StatusBackend != "redis" || store == nil {
		return nil, nil
	}

	provider, ok := store.(redisControllerRefreshClientProvider)
	if !ok || provider.Client() == nil {
		return nil, nil
	}
	return controllerrefresh.NewRedisHandoff(provider.Client(), os.Getenv("TENET_REDIS_KEY_PREFIX")).Subscribe(ctx)
}
