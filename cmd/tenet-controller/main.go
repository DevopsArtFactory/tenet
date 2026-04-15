package main

import (
	"log"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
)

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	workerCount := envOrDefault("TENET_CONTROLLER_WORKERS", "1")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-controller version=%s commit=%s workers=%s", info.Version, info.Commit, workerCount)
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
