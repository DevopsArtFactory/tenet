package applyrequest

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

func failedRecoveryFixture(t *testing.T) (*ecsFixture, context.Context) {
	t.Helper()
	f := newECSFixture(t)
	f.executor.Service.Store = NewFileStore(filepath.Join(t.TempDir(), "applies.json"))
	f.record.Principal = &ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "operator"}}
	f.record.Status = StatusFailed
	f.record.Error = "correlated ECS rollout failed"
	var err error
	f.record.ID, err = stableRecordID(f.record)
	if err != nil {
		t.Fatal(err)
	}
	target := "arn:aws:ecs:ap-northeast-2:123456789012:task-definition/worker:18"
	for i := range f.record.Operations {
		cp := &ECSCheckpoint{RequestID: ecsRequest(f.record, i, DeployerRoleReadiness{}).RequestID, State: "complete", Attempts: 1, Result: ECSOperationResult{TaskDefinitionARN: target, TaskDefinitionIntent: "content-hash"}}
		f.record.Operations[i].ECSCheckpoint = cp
		f.record.Operations[i].Status = OperationStatusSucceeded
	}
	f.record.Operations[1].ECSCheckpoint.Result.PreviousService = &ECSServiceSnapshot{TaskDefinitionARN: f.intent.BaseTaskDefinitionARN, DesiredCount: 1, MinimumHealthyPercent: 100, MaximumPercent: 200, PrimaryDeploymentID: "ecs-svc/baseline"}
	f.record.Operations[1].ECSCheckpoint.Result.ServiceDeploymentID = "ecs-svc/failed"
	f.record.Operations[2].ECSCheckpoint.State = "failed"
	f.record.Operations[2].Status = OperationStatusFailed
	f.record.ECSRuntime = &ECSRolloutObservation{State: "failed", Reason: "rollout_failed", DeploymentID: "ecs-svc/failed"}
	if err := f.executor.Service.Store.Save(context.Background(), f.record); err != nil {
		t.Fatal(err)
	}
	f.executor.Service.AuthorizeCreate = func(ctx context.Context, _ Record) (*ExecutionPrincipal, error) {
		id := requestactor.AuthenticatedIdentityFromContext(ctx)
		if !id.Authenticated || id.ID != "operator" {
			return nil, ErrExecutionUnauthorized
		}
		return &ExecutionPrincipal{Identity: id}, nil
	}
	f.executor.Service.RollbackTarget = func(context.Context, Record) (*domain.ECSDeploymentIntent, error) {
		target := f.intent.Clone()
		target.BaseTaskDefinitionARN = f.record.Operations[1].ECSCheckpoint.Result.PreviousService.TaskDefinitionARN
		target.Image = "registry.example.com/worker:previous"
		target.DesiredCount = 1
		target.MinimumHealthyPercent = 100
		target.MaximumPercent = 200
		target.RollbackOnFailure = false
		return target, nil
	}
	f.executor.Service.CreateRetryPlan = func(ctx context.Context, _ string) (planpreview.Record, error) {
		return f.executor.Service.LoadPlan(ctx, f.record.PlanID)
	}
	return f, requestactor.WithAuthenticatedIdentityContext(context.Background(), f.record.Principal.Identity)
}

func TestECSRollbackIsSeparateIdempotentAndPreservesFailedForward(t *testing.T) {
	f, ctx := failedRecoveryFixture(t)
	before := f.stored(t)
	if _, err := f.executor.Service.RequestRollback(context.Background(), f.record.ID); !errors.Is(err, ErrExecutionUnauthorized) {
		t.Fatal("anonymous rollback was authorized")
	}
	created, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || again.Created || again.Record.ID != created.Record.ID || created.Record.ID == f.record.ID {
		t.Fatal("rollback requests were duplicated or overwrote forward Apply")
	}
	if !reflect.DeepEqual(before, f.stored(t)) {
		t.Fatal("rollback rewrote failed forward audit evidence")
	}
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	completed, err := f.executor.Service.Get(ctx, created.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusSucceeded || completed.Recovery.ApplyID != f.record.ID || completed.Operations[0].ECS.BaseTaskDefinitionARN != before.Operations[1].ECSCheckpoint.Result.PreviousService.TaskDefinitionARN {
		t.Fatalf("rollback pipeline did not finish: %+v", completed)
	}
	if !reflect.DeepEqual(before, f.stored(t)) {
		t.Fatal("rollback completion changed original failure")
	}
}

func TestECSRollbackRejectsMissingBaselineAndAlteredTarget(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ecsFixture)
	}{
		{"missing baseline", func(f *ecsFixture) { f.record.Operations[1].ECSCheckpoint.Result.PreviousService = nil }},
		{"not failed", func(f *ecsFixture) { f.record.Status = StatusSucceeded }},
		{"untrusted forward", func(f *ecsFixture) { f.record.Principal = nil }},
		{"wrong target", func(f *ecsFixture) {
			f.executor.Service.RollbackTarget = func(context.Context, Record) (*domain.ECSDeploymentIntent, error) { return f.intent.Clone(), nil }
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, ctx := failedRecoveryFixture(t)
			test.change(f)
			if err := f.executor.Service.Store.Update(ctx, f.record); err != nil {
				t.Fatal(err)
			}
			if _, err := f.executor.Service.RequestRollback(ctx, f.record.ID); err == nil {
				t.Fatal("unsafe recovery was accepted")
			}
			if f.handler.calls.Load() != 0 {
				t.Fatal("unsafe recovery called AWS")
			}
		})
	}
}

func TestFailedECSIntentRequiresExplicitRetryAndWaitsForRollback(t *testing.T) {
	f, ctx := failedRecoveryFixture(t)
	if _, err := f.executor.Service.Create(ctx, f.record.PlanID); !errors.Is(err, ErrFailedIntentHeld) {
		t.Fatalf("failed intent was redeployed without retry approval: %v", err)
	}
	rollback, err := f.executor.Service.RequestRollback(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.executor.Service.Retry(ctx, f.record.ID); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatal("retry raced an active rollback")
	}
	for i := 0; i < 3; i++ {
		if err := f.executor.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	result, err := f.executor.Service.Get(ctx, rollback.Record.ID)
	if err != nil || result.Status != StatusSucceeded {
		t.Fatal("rollback did not finish")
	}
	retry, err := f.executor.Service.Retry(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.executor.Service.Retry(ctx, f.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Created || again.Created || retry.Record.RetryOf != f.record.ID || retry.Record.ID == f.record.ID || retry.Record.ID != again.Record.ID {
		t.Fatal("explicit retry approval lost identity or duplicated work")
	}
}

func TestAutomaticECSRecoveryOnlyUsesOptInCorrelatedFailure(t *testing.T) {
	for _, reason := range []string{"rollout_failed", "rollback_observed", "observation_timeout", "aws_unavailable", "superseded_deployment"} {
		t.Run(reason, func(t *testing.T) {
			f, ctx := failedRecoveryFixture(t)
			f.record.ECSRuntime.Reason = reason
			f.record.Operations[0].ECS.RollbackOnFailure = true
			// Terminal fixture approval models the same flag in a reviewed source plan.
			if err := f.executor.Service.Store.Update(ctx, f.record); err != nil {
				t.Fatal(err)
			}
			f.intent.RollbackOnFailure = true
			if err := f.executor.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
			records, err := f.executor.Service.List(ctx, ListQuery{})
			if err != nil {
				t.Fatal(err)
			}
			expected := 1
			if reason == "rollout_failed" || reason == "rollback_observed" {
				expected = 2
			}
			if len(records) != expected {
				t.Fatalf("automatic recovery for %s: records=%d", reason, len(records))
			}
			for _, record := range records {
				if record.Recovery != nil && (!record.Recovery.Automatic || record.Principal.Identity.ID != "operator") {
					t.Fatal("automatic recovery lost reviewed approval")
				}
			}
		})
	}
}

func TestPreviouslyQueuedECSApplyCannotRedeployAnIntentThatLaterFailed(t *testing.T) {
	f, ctx := failedRecoveryFixture(t)
	candidate := f.record
	candidate.Status = StatusQueued
	candidate.Error = ""
	candidate.Principal = &ExecutionPrincipal{Identity: requestactor.AuthenticatedIdentity{Authenticated: true, ID: "second-operator"}}
	candidate.Operations = cloneOperations(f.record.Operations)
	for i := range candidate.Operations {
		candidate.Operations[i].ECSCheckpoint = nil
		candidate.Operations[i].Status = OperationStatusQueued
	}
	candidate.ID, _ = stableRecordID(candidate)
	if err := f.executor.Service.Store.Save(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if err := f.executor.RunOnce(ctx); !errors.Is(err, ErrFailedIntentHeld) {
		t.Fatalf("previously queued apply bypassed the failed intent hold: %v", err)
	}
	blocked, err := f.executor.Service.Get(ctx, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != StatusQueued || blocked.Claim != nil || blocked.Error == "" || blocked.ExecutionReadiness.State != "blocked" || f.handler.calls.Load() != 0 {
		t.Fatal("held intent mutated AWS or hid its blocker")
	}
}
