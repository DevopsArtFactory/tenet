package main

import (
	"log"
	"net/http"
	"os"

	"github.com/DevopsArtFactory/tenet/internal/api"
	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
)

func main() {
	version := envOrDefault("TENET_VERSION", "dev")
	commit := envOrDefault("TENET_COMMIT", "unknown")
	addr := envOrDefault("TENET_API_ADDR", ":8080")

	info := buildinfo.Current(version, commit)
	log.Printf("starting tenet-api version=%s commit=%s addr=%s", info.Version, info.Commit, addr)

	if err := http.ListenAndServe(addr, api.NewRouter(api.Options{BuildInfo: info})); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
