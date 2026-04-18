package main

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/controllerrefresh"
	"github.com/DevopsArtFactory/tenet/internal/githubcontents"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
)

type fakeControllerRefreshSubscription struct {
	channel  string
	messages chan controllerrefresh.Message
}

func (s *fakeControllerRefreshSubscription) C() <-chan controllerrefresh.Message {
	return s.messages
}

func (s *fakeControllerRefreshSubscription) Channel() string {
	return s.channel
}

func (s *fakeControllerRefreshSubscription) Close() error {
	close(s.messages)
	return nil
}

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

func TestRunControllerReconcilesImmediatelyWhenRefreshHandoffArrives(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := &recordingRuntimeStore{}
	previousOpen := openRuntimeStatusStore
	openRuntimeStatusStore = func(_ context.Context, _ *log.Logger, _ startupConfig) (runtimeStatusStore, error) {
		return store, nil
	}
	defer func() {
		openRuntimeStatusStore = previousOpen
	}()

	subscription := &fakeControllerRefreshSubscription{
		channel:  "tenet:test:controller-refresh",
		messages: make(chan controllerrefresh.Message, 1),
	}
	previousSubscriptionOpener := controllerRefreshSubscriptionOpener
	controllerRefreshSubscriptionOpener = func(_ context.Context, _ startupConfig, _ runtimeStatusStore) (controllerrefresh.Subscription, error) {
		return subscription, nil
	}
	defer func() {
		controllerRefreshSubscriptionOpener = previousSubscriptionOpener
	}()

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
		StatusBackend:   "redis",
		RequeueInterval: time.Hour,
	}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	requireLogLine(t, output, "controller refresh handoff enabled backend=redis channel=tenet:test:controller-refresh")
	requireLogLine(t, output, "controller reconcile deployment=api-prod")

	subscription.messages <- controllerrefresh.Message{Request: controllerrefresh.Request{
		RequestID:   "github-webhook:delivery-1:2222222",
		DeliveryID:  "delivery-1",
		Repository:  "acme/platform-config",
		Ref:         "refs/heads/main",
		After:       "2222222",
		RequestedAt: "2026-04-19T09:30:00Z",
	}}

	requireLogLine(t, output, "controller refresh handoff received request_id=github-webhook:delivery-1:2222222 delivery_id=delivery-1 repository=acme/platform-config ref=refs/heads/main after=2222222 requested_at=2026-04-19T09:30:00Z")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if strings.Count(output.String(), "controller reconcile deployment=api-prod") >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected controller refresh handoff to trigger a second reconcile, got logs %q", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
}
