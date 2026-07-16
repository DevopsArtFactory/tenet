package applyrequest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecutorRunOnceAdvancesQueuedApplyToRunningThenFailed(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle: Lifecycle{
					UpdatedAt:  "2026-04-18T08:05:00Z",
					TotalSteps: 2,
				},
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
		},
	}
	now := time.Date(2026, 4, 18, 8, 6, 0, 0, time.UTC)
	executor := Executor{
		Service: Service{
			Store: store,
			Now: func() time.Time {
				return now
			},
		},
	}

	if err := executor.RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce returned error: %v", err)
	}
	running := store.records["apply_queued"]
	if running.Status != StatusRunning {
		t.Fatalf("expected queued apply to advance to running, got %+v", running)
	}
	if running.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || running.Lifecycle.CurrentStep != 1 {
		t.Fatalf("unexpected running lifecycle metadata: %+v", running.Lifecycle)
	}
	if running.Error != "" || len(running.Details) != 1 || !strings.Contains(running.Details[0], "tenet-local-executor") {
		t.Fatalf("expected queued claim details without error, got %+v", running)
	}
	if running.Claim == nil || running.Claim.WorkerID != "tenet-local-executor" {
		t.Fatalf("expected executor claim metadata, got %+v", running.Claim)
	}
	if attempt := running.Operations[0].ExecutionAttempt; attempt == nil || attempt.WorkerID != "tenet-local-executor" || attempt.Status != OperationStatusRunning || attempt.StartedAt != "2026-04-18T08:06:00Z" || attempt.LastHeartbeatAt != "2026-04-18T08:06:00Z" || !strings.Contains(attempt.Summary, "no live AWS mutation") {
		t.Fatalf("expected executor claim to persist operation attempt evidence, got %+v", attempt)
	}
	if running.Operations[1].ExecutionAttempt != nil {
		t.Fatalf("expected queued executor operation to omit attempt evidence, got %+v", running.Operations[1].ExecutionAttempt)
	}

	now = now.Add(30 * time.Second)
	if err := executor.RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	failed := store.records["apply_queued"]
	if failed.ID != "apply_queued" || failed.Status != StatusFailed {
		t.Fatalf("expected same apply id to fail after handoff, got %+v", failed)
	}
	if failed.Error != executorFailedError {
		t.Fatalf("expected executor failure error, got %+v", failed)
	}
	if failed.Lifecycle.FinishedAt != "2026-04-18T08:06:30Z" || failed.Lifecycle.CurrentStep != 1 || failed.Lifecycle.CompletedSteps != 0 {
		t.Fatalf("unexpected failed lifecycle metadata: %+v", failed.Lifecycle)
	}
	if statuses := []OperationStatus{failed.Operations[0].Status, failed.Operations[1].Status}; statuses[0] != OperationStatusFailed || statuses[1] != OperationStatusQueued {
		t.Fatalf("unexpected failed operation statuses: %+v", failed.Operations)
	}
	if attempt := failed.Operations[0].ExecutionAttempt; attempt == nil || attempt.WorkerID != "tenet-local-executor" || attempt.Status != OperationStatusFailed || attempt.FinishedAt != "2026-04-18T08:06:30Z" || attempt.Reason != executorFailedError || !strings.Contains(attempt.Summary, executorFailedError) {
		t.Fatalf("expected executor failure to finish operation attempt evidence, got %+v", attempt)
	}
}

func TestExecutorRunOnceReturnsListError(t *testing.T) {
	executor := Executor{Service: Service{Store: failingStore{err: errors.New("boom")}}}

	err := executor.RunOnce(context.Background())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestExecutorRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (Executor{Service: Service{Store: &memoryStore{}}, PollInterval: time.Millisecond}).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

type failingStore struct {
	err error
}

func (s failingStore) Get(context.Context, string) (Record, error) {
	return Record{}, s.err
}

func (s failingStore) List(context.Context) ([]Record, error) {
	return nil, s.err
}

func (s failingStore) Save(context.Context, Record) error {
	return s.err
}

func (s failingStore) Update(context.Context, Record) error {
	return s.err
}
