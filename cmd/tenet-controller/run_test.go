package main

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
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

func TestRunControllerOpensRuntimeStatusStoreOnceAndClosesOnShutdown(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)

	store := &recordingRuntimeStore{}
	openCalls := 0
	previousOpen := openRuntimeStatusStore
	openRuntimeStatusStore = func(_ context.Context, _ *log.Logger, _ startupConfig) (runtimeStatusStore, error) {
		openCalls++
		return store, nil
	}
	defer func() {
		openRuntimeStatusStore = previousOpen
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newConcurrentLogSink()
	logger := log.New(output, "", 0)
	config := startupConfig{RepoRoot: root, RequeueInterval: 10 * time.Millisecond}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	requireLogLine(t, output, "controller reconcile deployment=api-prod")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
	if openCalls != 1 {
		t.Fatalf("expected runController to open the runtime status store once, got %d", openCalls)
	}
	if store.closeCalls != 1 {
		t.Fatalf("expected runController to close the runtime status store once, got %d", store.closeCalls)
	}
}

func TestRunControllerOpensRuntimeStatusStoreForGitHubCatalogSource(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)

	store := &recordingRuntimeStore{}
	openCalls := 0
	previousOpen := openRuntimeStatusStore
	openRuntimeStatusStore = func(_ context.Context, _ *log.Logger, _ startupConfig) (runtimeStatusStore, error) {
		openCalls++
		return store, nil
	}
	defer func() {
		openRuntimeStatusStore = previousOpen
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newConcurrentLogSink()
	logger := log.New(output, "", 0)
	config := startupConfig{
		CatalogSource: controllerCatalogSource{
			kind: controllerCatalogSourceGitHub,
			githubRequest: githubcontents.Request{
				Repository: "acme/platform-config",
				Ref:        "main",
				Path:       "tenet",
			},
			loadCatalog: func(_ context.Context) (manifest.Catalog, error) {
				return catalog.LoadFromRepository(root)
			},
		},
		RequeueInterval: 10 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	requireLogLine(t, output, "controller reconcile deployment=api-prod")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
	if openCalls != 1 {
		t.Fatalf("expected runController to open the runtime status store once, got %d", openCalls)
	}
	if store.closeCalls != 1 {
		t.Fatalf("expected runController to close the runtime status store once, got %d", store.closeCalls)
	}
}
