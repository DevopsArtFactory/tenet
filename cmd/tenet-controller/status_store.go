package main

import (
	"context"
	"log"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/statusstore"
	"github.com/redis/go-redis/v9"
)

func mustOpenStatusStore(logger *log.Logger, config startupConfig) statusstore.Store {
	switch config.StatusBackend {
	case "", "file":
		if config.StatusPath == "" {
			return nil
		}
		return statusstore.NewFileStore(config.StatusPath)
	case "redis":
		if config.RedisAddr == "" {
			logger.Fatal("TENET_REDIS_ADDR is required when TENET_STATUS_BACKEND=redis")
		}
		client := redis.NewClient(&redis.Options{Addr: config.RedisAddr})
		if err := client.Ping(context.Background()).Err(); err != nil {
			logger.Fatalf("connect redis status store: %v", err)
		}
		return statusstore.NewRedisStore(client, os.Getenv("TENET_REDIS_KEY_PREFIX"))
	default:
		logger.Fatalf("unsupported TENET_STATUS_BACKEND %q", config.StatusBackend)
		return nil
	}
}
