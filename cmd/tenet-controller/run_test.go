package main

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
)

func TestRunControllerLogsStartupAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newConcurrentLogSink()
	logger := log.New(output, "", 0)
	config := startupConfig{
		Info:        buildinfo.Info{Version: "dev", Commit: "abc123"},
		WorkerCount: "2",
		RepoRoot:    "/tmp/repo",
		Summary:     &catalog.Summary{AccountCount: 1, EnvironmentCount: 2, DeploymentCount: 3},
	}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}

	logs := output.String()
	if !bytes.Contains([]byte(logs), []byte("starting tenet-controller")) {
		t.Fatalf("expected startup log, got %q", logs)
	}
	if !bytes.Contains([]byte(logs), []byte("stopping tenet-controller")) {
		t.Fatalf("expected shutdown log, got %q", logs)
	}
}
