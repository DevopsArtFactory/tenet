package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

type runtimeStatusStore interface {
	statusstore.Store
	Close() error
}

type fileRuntimeStatusStore struct {
	statusstore.FileStore
}

func (fileRuntimeStatusStore) Close() error {
	return nil
}

type redisRuntimeStatusStore struct {
	statusstore.RedisStore
	client redis.UniversalClient
}

func (s redisRuntimeStatusStore) Close() error {
	return s.client.Close()
}

var openRuntimeStatusStore = openStatusStore

func openStatusStore(ctx context.Context, _ *log.Logger, config startupConfig) (runtimeStatusStore, error) {
	switch config.StatusBackend {
	case "", "file":
		if config.StatusPath == "" {
			return nil, nil
		}
		return fileRuntimeStatusStore{FileStore: statusstore.NewFileStore(config.StatusPath)}, nil
	case "redis":
		if config.RedisAddr == "" {
			return nil, fmt.Errorf("TENET_REDIS_ADDR is required when TENET_STATUS_BACKEND=redis")
		}
		client := redis.NewClient(&redis.Options{Addr: config.RedisAddr})
		if err := client.Ping(ctx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("connect redis status store: %w", err)
		}
		return redisRuntimeStatusStore{
			RedisStore: statusstore.NewRedisStore(client, os.Getenv("TENET_REDIS_KEY_PREFIX")),
			client:     client,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported TENET_STATUS_BACKEND %q", config.StatusBackend)
	}
}
