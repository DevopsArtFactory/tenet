package main

import (
	"log"
	"os"
)

func main() {
	config, err := loadStartupConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	if config.Summary != nil {
		log.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s accounts=%d environments=%d deployments=%d", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot, config.Summary.AccountCount, config.Summary.EnvironmentCount, config.Summary.DeploymentCount)
		return
	}
	log.Printf("starting tenet-controller version=%s commit=%s workers=%s repo_root=%s", config.Info.Version, config.Info.Commit, config.WorkerCount, config.RepoRoot)
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
