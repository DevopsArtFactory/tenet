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
)

func TestRunControllerLogsReconcilePlansFromRepositoryAndStatusSnapshots(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":2,"sync":{"state":"Reconciling","reason":"warming up"},"health":{"overall":"Progressing","summary":"warming"},"conditions":[]}}]`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	config := startupConfig{
		Info:            buildinfo.Info{Version: "dev", Commit: "abc123"},
		WorkerCount:     "1",
		RepoRoot:        root,
		StatusPath:      statusPath,
		RequeueInterval: 10 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	requireLogLine(t, &output, "controller reconcile deployment=api-prod")
	requireLogLine(t, &output, "steps=ResolvePlacement,ObserveCurrentState,AwaitFreshDesired")

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
}

func TestRunControllerLogsDisabledRuntimeWhenRepositoryRootIsUnset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	config := startupConfig{
		Info:            buildinfo.Info{Version: "dev", Commit: "abc123"},
		WorkerCount:     "1",
		RequeueInterval: 10 * time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		runController(ctx, logger, config)
		close(done)
	}()

	requireLogLine(t, &output, "controller runtime disabled")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
}

func requireLogLine(t *testing.T, output *bytes.Buffer, expected string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected log line %q in %q", expected, output.String())
}
