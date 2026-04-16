package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/buildinfo"
	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/reconcile"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

type contextKey string

type recordingRuntimeStore struct {
	listContexts []context.Context
	saveContexts []context.Context
	snapshots    []statusstore.Snapshot
	closeCalls   int
}

func (s *recordingRuntimeStore) List(ctx context.Context) ([]statusstore.Snapshot, error) {
	s.listContexts = append(s.listContexts, ctx)
	return append([]statusstore.Snapshot(nil), s.snapshots...), nil
}

func (s *recordingRuntimeStore) Get(_ context.Context, deploymentName string) (statusstore.Snapshot, error) {
	return statusstore.Get(s.snapshots, deploymentName)
}

func (s *recordingRuntimeStore) Save(ctx context.Context, snapshots []statusstore.Snapshot) error {
	s.saveContexts = append(s.saveContexts, ctx)
	s.snapshots = append([]statusstore.Snapshot(nil), snapshots...)
	return nil
}

func (s *recordingRuntimeStore) Close() error {
	s.closeCalls++
	return nil
}

func TestRunControllerLogsReconcilePlansFromRepositoryAndStatusSnapshots(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	desiredGeneration := desiredGenerationForTest(t, root, "api-prod")
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, fmt.Sprintf(`[{"deploymentName":"api-prod","status":{"observedGeneration":%d,"sync":{"state":"Reconciling","reason":"warming up"},"health":{"overall":"Progressing","summary":"warming"},"conditions":[]}}]`, desiredGeneration))

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
	requireLogLine(t, output, "steps=ResolvePlacement,ObserveCurrentState,ReconcileCurrentState")

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

func TestEmitRuntimePlansPersistsRolloutSummaryIntoStatusSnapshotsForEC2CanaryDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[{"type":"Ready","status":false,"reason":"HealthPending","message":"candidate target group is still warming"}],"deployedArtifact":{"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout current step to be persisted, got %+v", status.Rollout)
	}
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning {
		t.Fatalf("expected weighted target group rollout phase %q, got %+v", domain.RolloutPhaseProvisioning, status.Rollout)
	}
	if status.Rollout.Summary != "Creating candidate capacity for weighted target groups" {
		t.Fatalf("expected weighted target group rollout summary, got %+v", status.Rollout)
	}
	wantSteps := []string{
		"ResolvePlacement",
		"ObserveCurrentState",
		"CreateCandidateCapacity",
		"VerifyCandidateCapacity",
		"ShiftTrafficCanary",
		"PromoteCandidateCapacity",
	}
	if got := rolloutStepKinds(status.Rollout.Steps); len(got) != len(wantSteps) {
		t.Fatalf("expected rollout steps %v, got %v", wantSteps, got)
	} else {
		for i := range wantSteps {
			if got[i] != wantSteps[i] {
				t.Fatalf("expected rollout steps %v, got %v", wantSteps, got)
			}
		}
	}
	wantStatuses := []string{"completed", "completed", "current", "pending", "pending", "pending"}
	if got := rolloutStepStatuses(status.Rollout.Steps); len(got) != len(wantStatuses) {
		t.Fatalf("expected rollout step statuses %v, got %v", wantStatuses, got)
	} else {
		for i := range wantStatuses {
			if got[i] != wantStatuses[i] {
				t.Fatalf("expected rollout step statuses %v, got %v", wantStatuses, got)
			}
		}
	}
	if status.Rollout.Steps[2].Kind != "CreateCandidateCapacity" || !status.Rollout.Steps[2].Current {
		t.Fatalf("expected actionable rollout step to be marked current, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[len(status.Rollout.Steps)-1].Current {
		t.Fatalf("expected final planned rollout step to remain non-current, got %+v", status.Rollout.Steps)
	}
	if got := status.DeployedArtifact.Binary; got != "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64" {
		t.Fatalf("expected deployed artifact to be preserved, got %q", got)
	}
	if len(status.Conditions) != 1 || status.Conditions[0] != (domain.Condition{Type: "Ready", Status: false, Reason: "HealthPending", Message: "candidate target group is still warming"}) {
		t.Fatalf("expected seeded conditions to be preserved, got %+v", status.Conditions)
	}
}

func TestDeriveRolloutSummaryLeavesNonActionablePlansPending(t *testing.T) {
	deployment := domain.Deployment{Generation: 3}
	status := domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStatePaused}}
	plan := reconcile.Plan{Steps: []reconcile.Step{{Kind: reconcile.StepResolvePlacement}, {Kind: reconcile.StepObserveCurrentState}}}

	rollout := deriveRolloutSummary(deployment, status, plan)
	if rollout == nil {
		t.Fatalf("expected rollout summary")
	}
	if rollout.CurrentStep != "" {
		t.Fatalf("expected no actionable current step for non-actionable plan, got %+v", rollout)
	}
	if rollout.Phase != domain.RolloutPhasePending {
		t.Fatalf("expected pending phase for non-actionable plan, got %+v", rollout)
	}
	if rollout.Summary != "Rollout pending" {
		t.Fatalf("expected pending summary for non-actionable plan, got %+v", rollout)
	}
	wantStatuses := []string{"pending", "pending"}
	if got := rolloutStepStatuses(rollout.Steps); len(got) != len(wantStatuses) {
		t.Fatalf("expected rollout step statuses %v, got %v", wantStatuses, got)
	} else {
		for i := range wantStatuses {
			if got[i] != wantStatuses[i] {
				t.Fatalf("expected rollout step statuses %v, got %v", wantStatuses, got)
			}
		}
	}
	for _, step := range rollout.Steps {
		if step.Current {
			t.Fatalf("expected no current step for non-actionable plan, got %+v", rollout.Steps)
		}
	}
}

func TestEmitRuntimePlansUsesControllerContextForStoreIO(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)

	store := &recordingRuntimeStore{}
	ctx := context.WithValue(context.Background(), contextKey("request-id"), "controller-loop")
	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root}

	if err := emitRuntimePlans(ctx, logger, config, store); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}
	if len(store.listContexts) == 0 {
		t.Fatalf("expected emitRuntimePlans to list snapshots")
	}
	if len(store.saveContexts) == 0 {
		t.Fatalf("expected emitRuntimePlans to save snapshots")
	}
	if got := store.listContexts[0].Value(contextKey("request-id")); got != "controller-loop" {
		t.Fatalf("expected List to receive controller context, got %#v", got)
	}
	if got := store.saveContexts[0].Value(contextKey("request-id")); got != "controller-loop" {
		t.Fatalf("expected Save to receive controller context, got %#v", got)
	}
}

func TestEmitRuntimePlansPersistsCodeDeployRolloutStepsForECSLinearDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"linear"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"task set missing"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.CurrentStep != "CreateCodeDeployDeployment" {
		t.Fatalf("expected ecs linear rollout current step, got %+v", status.Rollout)
	}
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning {
		t.Fatalf("expected ecs CodeDeploy rollout phase %q, got %+v", domain.RolloutPhaseProvisioning, status.Rollout)
	}
	if status.Rollout.Summary != "Creating CodeDeploy traffic shift deployment" {
		t.Fatalf("expected ecs CodeDeploy rollout summary, got %+v", status.Rollout)
	}
	if len(status.Rollout.Steps) != 5 || status.Rollout.Steps[2].Kind != "CreateCodeDeployDeployment" || status.Rollout.Steps[3].Kind != "ShiftTrafficLinear" {
		t.Fatalf("expected ecs codedeploy rollout steps, got %+v", status.Rollout.Steps)
	}
	wantStatuses := []string{"completed", "completed", "current", "pending", "pending"}
	if got := rolloutStepStatuses(status.Rollout.Steps); len(got) != len(wantStatuses) {
		t.Fatalf("expected ecs CodeDeploy rollout step statuses %v, got %v", wantStatuses, got)
	} else {
		for i := range wantStatuses {
			if got[i] != wantStatuses[i] {
				t.Fatalf("expected ecs CodeDeploy rollout step statuses %v, got %v", wantStatuses, got)
			}
		}
	}
}

func TestEmitRuntimePlansPersistsRolloutStepDetailsForEC2CanaryDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"canary"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if len(status.Rollout.Steps) != 6 {
		t.Fatalf("expected six rollout steps, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[2].Detail != "Create a fresh candidate Auto Scaling Group bootstrapped from userdata before attaching it to the candidate target group." {
		t.Fatalf("expected candidate-capacity step detail, got %+v", status.Rollout.Steps[2])
	}
	if status.Rollout.Steps[4].Detail != "Shift a small canary slice of production traffic to the candidate target group before full promotion." {
		t.Fatalf("expected canary traffic detail, got %+v", status.Rollout.Steps[4])
	}
}

func TestEmitRuntimePlansPersistsCodeDeployRolloutStepDetailsForECSLinearDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"linear"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"task set missing"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if len(status.Rollout.Steps) != 5 {
		t.Fatalf("expected five rollout steps, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[2].Detail != "Create a CodeDeploy deployment to coordinate blue/green ECS traffic shifting for the replacement task set." {
		t.Fatalf("expected CodeDeploy deployment step detail, got %+v", status.Rollout.Steps[2])
	}
	if status.Rollout.Steps[3].Detail != "Advance ECS traffic in linear increments through CodeDeploy until the replacement task set serves all traffic." {
		t.Fatalf("expected linear ECS traffic detail, got %+v", status.Rollout.Steps[3])
	}
}

func TestEmitRuntimePlansPersistsRolloutStepDetailsForECSRollingDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"service update pending"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if len(status.Rollout.Steps) != 4 {
		t.Fatalf("expected four rollout steps, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[2].Detail != "Update the ECS service to start a native rolling deployment that replaces running tasks in place." {
		t.Fatalf("expected ECS rolling update detail, got %+v", status.Rollout.Steps[2])
	}
	if status.Rollout.Steps[3].Detail != "Wait for the ECS service to report a stable native rolling deployment after replacing tasks." {
		t.Fatalf("expected ECS rolling stability detail, got %+v", status.Rollout.Steps[3])
	}
}

func TestEmitRuntimePlansPersistsCodeDeployRolloutStepDetailsForECSCanaryDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"canary"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"task set missing"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if len(status.Rollout.Steps) != 5 {
		t.Fatalf("expected five rollout steps, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[3].Detail != "Shift a small canary slice of ECS service traffic to the replacement task set through CodeDeploy before the full cutover." {
		t.Fatalf("expected ECS CodeDeploy canary detail, got %+v", status.Rollout.Steps[3])
	}
}

func TestEmitRuntimePlansPersistsRolloutStepDetailsForWeightedTargetGroupFullShiftDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"file","path":"userdata/api-prod.sh"}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if len(status.Rollout.Steps) != 6 {
		t.Fatalf("expected six rollout steps, got %+v", status.Rollout.Steps)
	}
	if status.Rollout.Steps[4].Detail != "Shift all production traffic from the active target group to the candidate target group in one blue/green cutover." {
		t.Fatalf("expected weighted target group full-shift detail, got %+v", status.Rollout.Steps[4])
	}
}

func TestDeriveRolloutSummaryMarkSyncedDoesNotInventStepDetail(t *testing.T) {
	deployment := domain.Deployment{Generation: 7}
	status := domain.DeploymentStatus{ObservedGeneration: 7, Sync: domain.Sync{State: domain.SyncStateSynced}, Health: domain.Health{Overall: domain.HealthHealthy}}
	plan := reconcile.Plan{Steps: []reconcile.Step{{Kind: reconcile.StepMarkSynced}}}

	rollout := deriveRolloutSummary(deployment, status, plan)
	if rollout == nil {
		t.Fatalf("expected rollout summary")
	}
	if len(rollout.Steps) != 1 {
		t.Fatalf("expected one rollout step, got %+v", rollout.Steps)
	}
	if rollout.Steps[0].Status != domain.RolloutStepStatusCompleted {
		t.Fatalf("expected MarkSynced step to be completed, got %+v", rollout.Steps[0])
	}
	if rollout.Steps[0].Detail != "" {
		t.Fatalf("expected MarkSynced step detail to remain empty, got %+v", rollout.Steps[0])
	}
}

func TestEmitRuntimePlansPersistsWeightedTargetGroupRolloutSummaryExecutionDetails(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1},"binary":"s3://releases.example.com/payments-api/prod/payments-api-linux-amd64","userdata":{"type":"file","path":"./userdata/bootstrap/../api-prod.sh"}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning || status.Rollout.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout phase/current step to reflect the first actionable weighted-target-group step, got %+v", status.Rollout)
	}
	want := []domain.RolloutExecutionDetail{{
		Resource:    "candidateAutoScalingGroup",
		Name:        "api-prod-candidate",
		Description: "Candidate Auto Scaling Group Tenet will create for weighted target group rollout",
	}, {
		Resource:    "activeTargetGroup",
		Name:        "api-prod-active",
		Description: "Currently active target group receiving production traffic",
	}, {
		Resource:    "candidateTargetGroup",
		Name:        "api-prod-candidate",
		Description: "Candidate target group Tenet will register and warm before traffic shifting",
	}, {
		Resource:    "productionListenerRule",
		Name:        "api-prod-prod-listener",
		Description: "Production listener rule Tenet updates to keep weighted target groups attached during rollout",
	}, {
		Resource:    "trafficShiftPlan",
		Name:        "api-prod-weighted-routing",
		Description: "Controller-authored weighted routing plan describing how production traffic shifts between target groups",
	}, {
		Resource:    "userDataBootstrapSource",
		Name:        "file:userdata/api-prod.sh",
		Description: "Normalized userdata bootstrap source for candidate EC2 capacity",
	}}
	assertRolloutExecutionDetails(t, status.Rollout.Execution, want)
}

func TestEmitRuntimePlansPersistsCodeDeployRolloutSummaryExecutionDetails(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"linear"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"task set missing"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning || status.Rollout.CurrentStep != "CreateCodeDeployDeployment" {
		t.Fatalf("expected rollout phase/current step to reflect the first actionable CodeDeploy step, got %+v", status.Rollout)
	}
	want := []domain.RolloutExecutionDetail{{
		Resource:    "ecsService",
		Name:        "worker-prod-service",
		Description: "ECS service Tenet updates during deployment",
	}, {
		Resource:    "codeDeployApplication",
		Name:        "worker-prod-codedeploy-app",
		Description: "CodeDeploy application coordinating ECS traffic shifting",
	}, {
		Resource:    "codeDeployDeploymentGroup",
		Name:        "worker-prod-codedeploy-group",
		Description: "CodeDeploy deployment group Tenet targets for ECS rollout",
	}, {
		Resource:    "productionListener",
		Name:        "worker-prod-prod-listener",
		Description: "Production listener CodeDeploy shifts during ECS blue/green rollout",
	}, {
		Resource:    "testListener",
		Name:        "worker-prod-test-listener",
		Description: "Test listener CodeDeploy uses to validate the replacement ECS task set before promotion",
	}, {
		Resource:    "blueTaskSetTargetGroup",
		Name:        "worker-prod-blue",
		Description: "Blue task set target group CodeDeploy treats as the live ECS traffic destination",
	}, {
		Resource:    "greenTaskSetTargetGroup",
		Name:        "worker-prod-green",
		Description: "Green task set target group CodeDeploy warms before shifting ECS traffic",
	}}
	assertRolloutExecutionDetails(t, status.Rollout.Execution, want)
}

func TestEmitRuntimePlansPersistsRollingECSExecutionDetails(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "worker-prod.json"), `{"name":"worker-prod","appRef":"payments-worker","environmentRef":"prod-apne2","target":{"type":"ecs"},"strategy":{"type":"rolling"},"traffic":{"port":8080},"capacity":{"desired":1,"min":1,"max":2},"image":"ghcr.io/devopsartfactory/payments-worker:1.2.3"}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"worker-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"service update pending"},"health":{"overall":"Progressing","summary":"deployment pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning || status.Rollout.CurrentStep != "UpdateECSService" {
		t.Fatalf("expected ecs rolling phase/current step to reflect the first actionable rolling step, got %+v", status.Rollout)
	}
	want := []domain.RolloutExecutionDetail{{
		Resource:    "ecsService",
		Name:        "worker-prod-service",
		Description: "ECS service Tenet updates with native rolling deployment",
	}}
	assertRolloutExecutionDetails(t, status.Rollout.Execution, want)
}

func TestEmitRuntimePlansPersistsInstanceRefreshRolloutSummaryForEC2RollingDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"rolling"},"traffic":{"port":443},"capacity":{"desired":2,"min":1,"max":3},"userdata":{"type":"file","path":"userdata/api-prod-refresh.sh"}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"refresh pending"},"health":{"overall":"Progressing","summary":"replacement capacity pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.CurrentStep != "StartInstanceRefresh" {
		t.Fatalf("expected instance refresh rollout current step, got %+v", status.Rollout)
	}
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning {
		t.Fatalf("expected instance refresh rollout phase %q, got %+v", domain.RolloutPhaseProvisioning, status.Rollout)
	}
	if status.Rollout.Summary != "Starting EC2 instance refresh" {
		t.Fatalf("expected instance refresh rollout summary, got %+v", status.Rollout)
	}
	if len(status.Rollout.Steps) != 4 || status.Rollout.Steps[2].Kind != "StartInstanceRefresh" || status.Rollout.Steps[3].Kind != "VerifyReplacementCapacity" {
		t.Fatalf("expected instance refresh rollout steps, got %+v", status.Rollout.Steps)
	}
	want := []domain.RolloutExecutionDetail{{
		Resource:    "serviceAutoScalingGroup",
		Name:        "api-prod-service",
		Description: "Service Auto Scaling Group Tenet refreshes in place",
	}, {
		Resource:    "userDataBootstrapSource",
		Name:        "file:userdata/api-prod-refresh.sh",
		Description: "Normalized userdata bootstrap source for refreshed EC2 instances",
	}}
	assertRolloutExecutionDetails(t, status.Rollout.Execution, want)
}

func TestEmitRuntimePlansBootstrapsMissingFileBackedStatusStore(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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

func TestEmitRuntimePlansPersistsWeightedTargetGroupRolloutStepsWhenDesiredGenerationIsAhead(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout current step CreateCandidateCapacity, got %+v", status.Rollout)
	}
	if status.Rollout.Phase != domain.RolloutPhaseProvisioning {
		t.Fatalf("expected rollout phase %q, got %+v", domain.RolloutPhaseProvisioning, status.Rollout)
	}
	if status.Rollout.Summary != "Creating candidate capacity for weighted target groups" {
		t.Fatalf("expected weighted-target-group rollout summary, got %+v", status.Rollout)
	}
	if len(status.Rollout.Steps) != 6 || status.Rollout.Steps[2].Kind != "CreateCandidateCapacity" || status.Rollout.Steps[4].Kind != "ShiftTrafficFull" {
		t.Fatalf("expected weighted-target-group rollout steps, got %+v", status.Rollout.Steps)
	}
}

func TestEmitRuntimePlansPersistsHealthySyncedRolloutSummaryWhenGenerationsMatch(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	desiredGeneration := desiredGenerationForTest(t, root, "api-prod")
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, fmt.Sprintf(`[{"deploymentName":"api-prod","status":{"observedGeneration":%d,"sync":{"state":"Synced","reason":"stable"},"health":{"overall":"Healthy","summary":"all checks passed"},"conditions":[]}}]`, desiredGeneration))

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.CurrentStep != "" {
		t.Fatalf("expected completed rollout to clear current step, got %+v", status.Rollout)
	}
	if status.Rollout.Summary != "Deployment is synced and healthy" {
		t.Fatalf("expected healthy synced rollout summary, got %+v", status.Rollout)
	}
	wantStatuses := []string{"completed"}
	if got := rolloutStepStatuses(status.Rollout.Steps); len(got) != len(wantStatuses) {
		t.Fatalf("expected completed rollout step statuses %v, got %v", wantStatuses, got)
	} else {
		for i := range wantStatuses {
			if got[i] != wantStatuses[i] {
				t.Fatalf("expected completed rollout step statuses %v, got %v", wantStatuses, got)
			}
		}
	}
	if len(status.Rollout.Steps) != 1 || status.Rollout.Steps[0].Kind != string(reconcile.StepMarkSynced) || status.Rollout.Steps[0].Current {
		t.Fatalf("expected completed rollout step to be non-current mark-synced step, got %+v", status.Rollout.Steps)
	}
}

func TestEmitRuntimePlansPersistsRuntimeHistory(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[],"history":[{"timestamp":"2026-04-16T05:00:00Z","source":"controller","summary":"Waiting for fresher desired state","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"AwaitFreshDesired"}]}}]`)

	oldNow := runtimeHistoryNow
	runtimeHistoryNow = func() time.Time { return time.Date(2026, 4, 16, 6, 0, 0, 0, time.UTC) }
	defer func() { runtimeHistoryNow = oldNow }()

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if len(status.History) != 2 {
		t.Fatalf("expected prior history entry plus appended controller entry, got %+v", status.History)
	}
	latest := status.History[len(status.History)-1]
	if latest.Timestamp != "2026-04-16T06:00:00Z" {
		t.Fatalf("expected deterministic timestamp, got %+v", latest)
	}
	if latest.Source != runtimeHistorySourceController {
		t.Fatalf("expected controller source, got %+v", latest)
	}
	if latest.Summary != "Creating candidate capacity for weighted target groups" {
		t.Fatalf("expected rollout summary in history entry, got %+v", latest)
	}
	if latest.Sync != (domain.Sync{State: domain.SyncStateOutOfSync, Reason: "candidate missing"}) {
		t.Fatalf("expected sync snapshot in history entry, got %+v", latest)
	}
	if latest.Health != (domain.Health{Overall: domain.HealthProgressing, Summary: "candidate pending"}) {
		t.Fatalf("expected health snapshot in history entry, got %+v", latest)
	}
	if latest.RolloutPhase != domain.RolloutPhaseProvisioning || latest.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout phase/current step in history entry, got %+v", latest)
	}
	if status.Rollout == nil || latest.Summary != status.Rollout.Summary || latest.RolloutPhase != status.Rollout.Phase || latest.CurrentStep != status.Rollout.CurrentStep {
		t.Fatalf("expected history entry to reflect persisted rollout summary, got history=%+v rollout=%+v", latest, status.Rollout)
	}
	if got := status.History[0].Timestamp; got != "2026-04-16T05:00:00Z" {
		t.Fatalf("expected seeded history to be preserved, got %+v", status.History)
	}
}

func TestEmitRuntimePlansBootstrapsRuntimeHistoryForLegacyRolloutOnlySnapshot(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[],"rollout":{"desiredGeneration":2681220996861709023,"observedGeneration":-1,"phase":"Provisioning","currentStep":"CreateCandidateCapacity","summary":"Creating candidate capacity for weighted target groups","steps":[{"kind":"ResolvePlacement"},{"kind":"ObserveCurrentState"},{"kind":"CreateCandidateCapacity","current":true},{"kind":"VerifyCandidateCapacity"},{"kind":"ShiftTrafficFull"},{"kind":"PromoteCandidateCapacity"}]},"history":[]}}]`)

	oldNow := runtimeHistoryNow
	runtimeHistoryNow = func() time.Time { return time.Date(2026, 4, 16, 6, 15, 0, 0, time.UTC) }
	defer func() { runtimeHistoryNow = oldNow }()

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if len(status.History) != 1 {
		t.Fatalf("expected legacy rollout-only snapshot to bootstrap first controller history entry, got %+v", status.History)
	}
	latest := status.History[0]
	if latest.Timestamp != "2026-04-16T06:15:00Z" {
		t.Fatalf("expected deterministic timestamp for bootstrapped history entry, got %+v", latest)
	}
	if latest.Source != runtimeHistorySourceController || latest.Summary != "Creating candidate capacity for weighted target groups" {
		t.Fatalf("expected controller-authored rollout summary in bootstrapped history entry, got %+v", latest)
	}
	if latest.Sync != (domain.Sync{State: domain.SyncStateOutOfSync, Reason: "candidate missing"}) {
		t.Fatalf("expected sync snapshot in bootstrapped history entry, got %+v", latest)
	}
	if latest.Health != (domain.Health{Overall: domain.HealthProgressing, Summary: "candidate pending"}) {
		t.Fatalf("expected health snapshot in bootstrapped history entry, got %+v", latest)
	}
	if latest.RolloutPhase != domain.RolloutPhaseProvisioning || latest.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected rollout phase/current step in bootstrapped history entry, got %+v", latest)
	}
}

func TestEmitRuntimePlansAvoidsDuplicateRuntimeHistory(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[],"rollout":{"desiredGeneration":2681220996861709023,"observedGeneration":-1,"phase":"Provisioning","currentStep":"CreateCandidateCapacity","summary":"Creating candidate capacity for weighted target groups","steps":[{"kind":"ResolvePlacement"},{"kind":"ObserveCurrentState"},{"kind":"CreateCandidateCapacity","current":true},{"kind":"VerifyCandidateCapacity"},{"kind":"ShiftTrafficFull"},{"kind":"PromoteCandidateCapacity"}]},"history":[{"timestamp":"2026-04-16T05:30:00Z","source":"controller","summary":"Creating candidate capacity for weighted target groups","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Provisioning","currentStep":"CreateCandidateCapacity"}]}}]`)

	oldNow := runtimeHistoryNow
	runtimeHistoryNow = func() time.Time { return time.Date(2026, 4, 16, 6, 30, 0, 0, time.UTC) }
	defer func() { runtimeHistoryNow = oldNow }()

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if len(status.History) != 1 {
		t.Fatalf("expected unchanged operator-facing status to avoid duplicate history entry, got %+v", status.History)
	}
	if got := status.History[0].Timestamp; got != "2026-04-16T05:30:00Z" {
		t.Fatalf("expected existing history entry to be preserved, got %+v", status.History)
	}
}

func TestEmitRuntimePlansCapsRuntimeHistory(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":-1,"sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"conditions":[],"history":[{"timestamp":"2026-04-16T05:00:00Z","source":"controller","summary":"entry-00","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-00"},{"timestamp":"2026-04-16T05:01:00Z","source":"controller","summary":"entry-01","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-01"},{"timestamp":"2026-04-16T05:02:00Z","source":"controller","summary":"entry-02","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-02"},{"timestamp":"2026-04-16T05:03:00Z","source":"controller","summary":"entry-03","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-03"},{"timestamp":"2026-04-16T05:04:00Z","source":"controller","summary":"entry-04","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-04"},{"timestamp":"2026-04-16T05:05:00Z","source":"controller","summary":"entry-05","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-05"},{"timestamp":"2026-04-16T05:06:00Z","source":"controller","summary":"entry-06","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-06"},{"timestamp":"2026-04-16T05:07:00Z","source":"controller","summary":"entry-07","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-07"},{"timestamp":"2026-04-16T05:08:00Z","source":"controller","summary":"entry-08","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-08"},{"timestamp":"2026-04-16T05:09:00Z","source":"controller","summary":"entry-09","sync":{"state":"OutOfSync","reason":"candidate missing"},"health":{"overall":"Progressing","summary":"candidate pending"},"rolloutPhase":"Accepted","currentStep":"step-09"}]} }]`)

	oldNow := runtimeHistoryNow
	runtimeHistoryNow = func() time.Time { return time.Date(2026, 4, 16, 6, 45, 0, 0, time.UTC) }
	defer func() { runtimeHistoryNow = oldNow }()

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
		t.Fatalf("emitRuntimePlans returned error: %v", err)
	}

	snapshots, err := statusstore.Load(statusPath)
	if err != nil {
		t.Fatalf("failed to load status snapshots: %v", err)
	}
	status := snapshots[0].Status
	if len(status.History) != runtimeHistoryMaxEntries {
		t.Fatalf("expected history to be capped at %d entries, got %+v", runtimeHistoryMaxEntries, status.History)
	}
	if got := status.History[0].Summary; got != "entry-01" {
		t.Fatalf("expected oldest entry to be trimmed when cap is exceeded, got %+v", status.History)
	}
	latest := status.History[len(status.History)-1]
	if latest.Timestamp != "2026-04-16T06:45:00Z" || latest.Summary != "Creating candidate capacity for weighted target groups" || latest.CurrentStep != "CreateCandidateCapacity" {
		t.Fatalf("expected newest controller-authored history entry to be retained after trimming, got %+v", latest)
	}
}

func TestAppendRuntimeHistoryCapsOversizedExistingHistoryWithoutMeaningfulChange(t *testing.T) {
	current := domain.DeploymentStatus{
		Sync:   domain.Sync{State: domain.SyncStateOutOfSync, Reason: "candidate missing"},
		Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate pending"},
		Rollout: &domain.RolloutSummary{
			Phase:       domain.RolloutPhaseProvisioning,
			CurrentStep: "CreateCandidateCapacity",
			Summary:     "Creating candidate capacity for weighted target groups",
		},
		History: make([]domain.StatusHistoryEntry, 0, runtimeHistoryMaxEntries+1),
	}
	for i := 0; i < runtimeHistoryMaxEntries; i++ {
		current.History = append(current.History, domain.StatusHistoryEntry{
			Timestamp:    fmt.Sprintf("2026-04-16T05:%02d:00Z", i),
			Source:       runtimeHistorySourceController,
			Summary:      fmt.Sprintf("entry-%02d", i),
			Sync:         current.Sync,
			Health:       current.Health,
			RolloutPhase: domain.RolloutPhaseAccepted,
			CurrentStep:  fmt.Sprintf("step-%02d", i),
		})
	}
	current.History = append(current.History, domain.StatusHistoryEntry{
		Timestamp:    "2026-04-16T05:10:00Z",
		Source:       runtimeHistorySourceController,
		Summary:      current.Rollout.Summary,
		Sync:         current.Sync,
		Health:       current.Health,
		RolloutPhase: current.Rollout.Phase,
		CurrentStep:  current.Rollout.CurrentStep,
	})

	got := appendRuntimeHistory(current, current)
	if len(got) != runtimeHistoryMaxEntries {
		t.Fatalf("expected oversized history to be trimmed to %d entries even without a new entry, got %+v", runtimeHistoryMaxEntries, got)
	}
	if got[0].Summary != "entry-01" {
		t.Fatalf("expected oldest persisted entry to be trimmed, got %+v", got)
	}
	latest := got[len(got)-1]
	if latest.Timestamp != "2026-04-16T05:10:00Z" || latest.Summary != current.Rollout.Summary || latest.CurrentStep != current.Rollout.CurrentStep {
		t.Fatalf("expected latest persisted entry to be preserved when no new history is appended, got %+v", latest)
	}
}

func TestAppendRuntimeHistoryDedupesAgainstLatestPersistedEntryForLegacySnapshot(t *testing.T) {
	previous := domain.DeploymentStatus{
		Sync:   domain.Sync{State: domain.SyncStateOutOfSync, Reason: "candidate missing"},
		Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate pending"},
	}
	current := domain.DeploymentStatus{
		Sync:   previous.Sync,
		Health: previous.Health,
		Rollout: &domain.RolloutSummary{
			Phase:       domain.RolloutPhaseProvisioning,
			CurrentStep: "CreateCandidateCapacity",
			Summary:     "Creating candidate capacity for weighted target groups",
		},
		History: []domain.StatusHistoryEntry{{
			Timestamp:    "2026-04-16T05:30:00Z",
			Source:       runtimeHistorySourceController,
			Summary:      "Creating candidate capacity for weighted target groups",
			Sync:         previous.Sync,
			Health:       previous.Health,
			RolloutPhase: domain.RolloutPhaseProvisioning,
			CurrentStep:  "CreateCandidateCapacity",
		}},
	}

	got := appendRuntimeHistory(previous, current)
	if len(got) != 1 {
		t.Fatalf("expected legacy snapshot dedupe to reuse the latest persisted entry instead of appending a duplicate, got %+v", got)
	}
	if got[0].Timestamp != "2026-04-16T05:30:00Z" {
		t.Fatalf("expected existing persisted history entry to be preserved, got %+v", got)
	}
}

func TestEmitRuntimePlansDoesNotPersistCompletedRolloutSummaryForSyncedButUnhealthyDeployment(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	desiredGeneration := desiredGenerationForTest(t, root, "api-prod")
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, fmt.Sprintf(`[{"deploymentName":"api-prod","status":{"observedGeneration":%d,"sync":{"state":"Synced","reason":"stable"},"health":{"overall":"Progressing","summary":"still warming"},"conditions":[]}}]`, desiredGeneration))

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if status.Rollout.Phase == domain.RolloutPhaseCompleted {
		t.Fatalf("expected synced but unhealthy deployment to avoid completed rollout phase, got %+v", status.Rollout)
	}
	if status.Rollout.CurrentStep == string(reconcile.StepMarkSynced) {
		t.Fatalf("expected synced but unhealthy deployment to avoid mark-synced rollout step, got %+v", status.Rollout)
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
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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

func TestEmitRuntimePlansPersistsNonZeroDesiredGenerationAndZeroObservedGenerationInJSON(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "accounts", "prod-core.json"), `{"name":"prod-core","awsAccountId":"123456789012","roleArn":"arn:aws:iam::123456789012:role/TenetDeployerRole","allowedRegions":["ap-northeast-2"]}`)
	writeJSON(t, filepath.Join(root, "environments", "prod-apne2.json"), `{"name":"prod-apne2","accountRef":"prod-core","region":"ap-northeast-2","networkRef":"prod-main"}`)
	writeJSON(t, filepath.Join(root, "deployments", "api-prod.json"), `{"name":"api-prod","appRef":"payments-api","environmentRef":"prod-apne2","target":{"type":"ec2"},"strategy":{"type":"blueGreen"},"traffic":{"port":443},"capacity":{"desired":1,"min":1,"max":1}}`)
	desiredGeneration := desiredGenerationForTest(t, root, "api-prod")
	statusPath := filepath.Join(root, "status", "deployments.json")
	writeJSON(t, statusPath, `[{"deploymentName":"api-prod","status":{"observedGeneration":0,"sync":{"state":"Synced","reason":"stable"},"health":{"overall":"Healthy","summary":"all checks passed"},"conditions":[]}}]`)

	logger := log.New(&bytes.Buffer{}, "", 0)
	config := startupConfig{RepoRoot: root, StatusPath: statusPath}
	if err := emitRuntimePlans(context.Background(), logger, config, nil); err != nil {
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
	if value, exists := rollout["desiredGeneration"]; !exists || value != float64(desiredGeneration) {
		t.Fatalf("expected desiredGeneration=%d to be persisted, got %#v", desiredGeneration, rollout)
	}
	if value, exists := rollout["observedGeneration"]; !exists || value != float64(0) {
		t.Fatalf("expected observedGeneration=0 to be persisted, got %#v", rollout)
	}
}

func rolloutStepKinds(steps []domain.RolloutStepSummary) []string {
	kinds := make([]string, 0, len(steps))
	for _, step := range steps {
		kinds = append(kinds, step.Kind)
	}
	return kinds
}

func rolloutStepStatuses(steps []domain.RolloutStepSummary) []string {
	statuses := make([]string, 0, len(steps))
	for _, step := range steps {
		statuses = append(statuses, string(step.Status))
	}
	return statuses
}

func assertRolloutExecutionDetails(t *testing.T, got []domain.RolloutExecutionDetail, want []domain.RolloutExecutionDetail) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected rollout execution details %v, got %+v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected rollout execution details %v, got %+v", want, got)
		}
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

func desiredGenerationForTest(t *testing.T, repoRoot string, deploymentName string) int64 {
	t.Helper()
	compiled, err := catalog.LoadFromRepository(repoRoot)
	if err != nil {
		t.Fatalf("failed to compile repository for desired generation lookup: %v", err)
	}
	for _, deployment := range compiled.Deployments {
		if deployment.Name == deploymentName {
			return deployment.Generation
		}
	}
	t.Fatalf("deployment %q not found in compiled repository", deploymentName)
	return 0
}
