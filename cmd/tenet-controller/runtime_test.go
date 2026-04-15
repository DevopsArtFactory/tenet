package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
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

	output := newConcurrentLogSink()
	logger := log.New(output, "", 0)
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

	requireLogLine(t, output, "controller reconcile deployment=api-prod")
	requireLogLine(t, output, "steps=ResolvePlacement,ObserveCurrentState,AwaitFreshDesired")

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

	output := newConcurrentLogSink()
	logger := log.New(output, "", 0)
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

	requireLogLine(t, output, "controller runtime disabled")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runController did not stop after cancellation")
	}
}

func TestEmitRuntimePlansPersistsRolloutSummaryIntoStatusSnapshots(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[{"type":"Ready","status":false,"reason":"HealthPending","message":"candidate target group is still warming"}],"deployedArtifact":{"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	status := snapshots[0].Status
	if status.Rollout == nil {
		t.Fatalf("expected rollout summary to be persisted")
	}
	if status.Rollout.CurrentStep != "PlanCandidateGeneration" {
		t.Fatalf("expected rollout current step to be persisted, got %+v", status.Rollout)
	}
	if got := status.DeployedArtifact.Binary; got != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64" {
		t.Fatalf("expected deployed artifact to be preserved, got %q", got)
	}
	if len(status.Conditions) != 1 || status.Conditions[0] != (domain.Condition{Type: "Ready", Status: false, Reason: "HealthPending", Message: "candidate target group is still warming"}) {
		t.Fatalf("expected seeded conditions to be preserved, got %+v", status.Conditions)
	}
}

func TestEmitRuntimePlansBootstrapsMissingFileBackedStatusStore(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load bootstrapped status snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 bootstrapped snapshot, got %d", len(snapshots))
	}
	if snapshots[0].DeploymentName != "api-prod" {
		t.Fatalf("expected api-prod bootstrapped snapshot, got %+v", snapshots)
	}
	if snapshots[0].Status.Rollout == nil {
		t.Fatalf("expected bootstrapped snapshot to persist rollout summary")
	}
}

func TestEmitRuntimePlansPersistsCandidatePlanningCurrentStepWhenDesiredGenerationIsAhead(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if status.Rollout == nil {
		t.Fatalf("expected rollout summary to be persisted")
	}
	if status.Rollout.CurrentStep != "PlanCandidateGeneration" {
		t.Fatalf("expected rollout current step PlanCandidateGeneration, got %+v", status.Rollout)
	}
	if status.Rollout.Phase != domain.RolloutPhasePlanning {
		t.Fatalf("expected rollout phase %q, got %+v", domain.RolloutPhasePlanning, status.Rollout)
	}
}

func TestEmitRuntimePlansPersistsHealthySyncedRolloutSummaryWhenGenerationsMatch(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":0,"sync":{"state":"Synced","reason":"stable"},"health":{"overall":"Healthy","summary":"all checks passed"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if status.Rollout == nil {
		t.Fatalf("expected rollout summary to be persisted")
	}
	if status.Rollout.Phase != domain.RolloutPhaseCompleted {
		t.Fatalf("expected rollout phase %q, got %+v", domain.RolloutPhaseCompleted, status.Rollout)
	}
	if status.Rollout.Summary != "Deployment is synced and healthy" {
		t.Fatalf("expected healthy synced rollout summary, got %+v", status.Rollout)
	}
}

func TestEmitRuntimePlansRemovesSnapshotsForDeletedDeployments(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[
  {"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[{"type":"Ready","status":false,"reason":"HealthPending","message":"candidate target group is still warming"}],"deployedArtifact":{"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}}},
  {"deploymentName":"old-prod","status":{"observedGeneration":7,"sync":{"state":"Synced","reason":"stale snapshot"},"health":{"overall":"Healthy","summary":"should be removed"},"conditions":[{"type":"Ready","status":true,"reason":"Persisted","message":"stale deployment"}]}}
]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected stale snapshots to be removed, got %d snapshots", len(snapshots))
	}
	if snapshots[0].DeploymentName != "api-prod" {
		t.Fatalf("expected only api-prod snapshot to remain, got %+v", snapshots)
	}
	if got := snapshots[0].Status.DeployedArtifact.Binary; got != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64" {
		t.Fatalf("expected current deployment artifact to be preserved, got %q", got)
	}
	if len(snapshots[0].Status.Conditions) != 1 || snapshots[0].Status.Conditions[0].Message != "candidate target group is still warming" {
		t.Fatalf("expected current deployment conditions to be preserved, got %+v", snapshots[0].Status.Conditions)
	}
}

func TestEmitRuntimePlansPersistsZeroRolloutGenerationsInJSON(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":0,"sync":{"state":"Synced","reason":"stable"},"health":{"overall":"Healthy","summary":"all checks passed"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(logger, config); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("failed to read persisted status snapshots: %v", err)
	}

	var persisted []map[string]any
	if err := json.Unmarshal(content, &persisted); err != nil {
		t.Fatalf("failed to decode persisted status snapshots: %v", err)
	}
	rollout, ok := persisted[0]["status"].(map[string]any)["rollout"].(map[string]any)
	if !ok {
		t.Fatalf("expected rollout object in persisted status, got %s", string(content))
	}
	if value, exists := rollout["desiredGeneration"]; !exists || value != float64(0) {
		t.Fatalf("expected desiredGeneration=0 to be persisted, got %#v", rollout)
	}
	if value, exists := rollout["observedGeneration"]; !exists || value != float64(0) {
		t.Fatalf("expected observedGeneration=0 to be persisted, got %#v", rollout)
	}
}

func requireLogLine(t *testing.T, output interface{ String() string }, expected string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains([]byte(output.String()), []byte(expected)) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected log line %q in %q", expected, output.String())
}
