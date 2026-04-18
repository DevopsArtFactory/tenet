package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

type memoryStore struct {
	records   map[string]Record
	saveCalls int
}

func (s *memoryStore) Get(_ context.Context, id string) (Record, error) {
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

func (s *memoryStore) Save(_ context.Context, record Record) error {
	s.saveCalls++
	if s.records == nil {
		s.records = map[string]Record{}
	}
	if _, exists := s.records[record.ID]; !exists {
		s.records[record.ID] = record
	}
	return nil
}

func (s *memoryStore) Update(_ context.Context, record Record) error {
	if s.records == nil {
		s.records = map[string]Record{}
	}
	if _, exists := s.records[record.ID]; !exists {
		return ErrNotFound
	}
	s.records[record.ID] = record
	return nil
}

func TestServiceCreatePersistsImmutableFailedApplyRequestWhenExecutionIsDisabled(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "completed", Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", Summary: "Create the CodeDeploy deployment for api-prod."},
			{Status: "current", Phase: "Promoting", Target: "api-prod-prod-listener (productionListener)", Summary: "Shift production traffic toward the replacement capacity for api-prod."},
		},
		Source: planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(ctx context.Context, planID string) (planpreview.Record, error) {
			if planID != plan.ID {
				t.Fatalf("unexpected plan lookup: %s", planID)
			}
			return plan, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC) },
	}

	first, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !first.Created {
		t.Fatalf("expected first create to persist a new apply request")
	}
	if first.Record.ID == "" || first.Record.Status != StatusFailed {
		t.Fatalf("unexpected created apply request: %+v", first.Record)
	}
	if first.Record.PlanID != plan.ID || first.Record.Source.Route != "/v1/plans/"+plan.ID {
		t.Fatalf("unexpected apply source context: %+v", first.Record)
	}
	if first.Record.Actor.Available || first.Record.Actor.Reason != requestactor.UnavailableReason {
		t.Fatalf("expected explicit unavailable actor, got %+v", first.Record.Actor)
	}
	if first.Record.Error != disabledError || len(first.Record.Details) != 2 {
		t.Fatalf("expected disabled error/details, got %+v", first.Record)
	}
	if first.Record.Lifecycle.UpdatedAt != "2026-04-18T08:00:00Z" || first.Record.Lifecycle.FinishedAt != "2026-04-18T08:00:00Z" || first.Record.Lifecycle.TotalSteps != 2 {
		t.Fatalf("expected additive lifecycle metadata on create, got %+v", first.Record.Lifecycle)
	}
	if len(first.Record.Operations) != 2 {
		t.Fatalf("expected projected operations, got %+v", first.Record.Operations)
	}
	if first.Record.Operations[0].Status != OperationStatusPending || first.Record.Operations[0].Phase != "Provisioning" {
		t.Fatalf("expected pending projected operation rows, got %+v", first.Record.Operations)
	}
	if first.Record.Operations[1].Target != "api-prod-prod-listener (productionListener)" || first.Record.Operations[1].Summary != "Shift production traffic toward the replacement capacity for api-prod." {
		t.Fatalf("expected ordered plan projection into apply operations, got %+v", first.Record.Operations)
	}

	second, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("second Create returned error: %v", err)
	}
	if second.Created {
		t.Fatalf("expected repeated create to reuse the immutable apply request")
	}
	if second.Record.ID != first.Record.ID || second.Record.CreatedAt != "2026-04-18T08:00:00Z" {
		t.Fatalf("expected stable readback, got first=%+v second=%+v", first.Record, second.Record)
	}

	fetched, err := service.Get(context.Background(), first.Record.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.ID != first.Record.ID || fetched.PlanID != plan.ID || fetched.Status != StatusFailed {
		t.Fatalf("unexpected fetched apply request: %+v", fetched)
	}
}

func TestServiceCreatePersistsImmutableQueuedApplyRequestWhenExecutionIsEnabled(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "completed", Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
			{Status: "current", Phase: "Promoting", Target: "api-prod-prod-listener (productionListener)", Summary: "Shift production traffic toward the replacement capacity for api-prod."},
		},
		Source: planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		Store:          store,
		Now:            func() time.Time { return time.Date(2026, 4, 18, 8, 5, 0, 0, time.UTC) },
		EnableAWSApply: true,
	}

	result, err := service.Create(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected first create to persist a queued apply request")
	}
	if result.Record.Status != StatusQueued || result.Record.Error != "" {
		t.Fatalf("expected queued apply request without disabled error, got %+v", result.Record)
	}
	if result.Record.Lifecycle.UpdatedAt != "2026-04-18T08:05:00Z" || result.Record.Lifecycle.FinishedAt != "" || result.Record.Lifecycle.TotalSteps != 2 {
		t.Fatalf("expected queued lifecycle metadata, got %+v", result.Record.Lifecycle)
	}
	if len(result.Record.Details) != 2 || !strings.Contains(result.Record.Details[0], "TENET_AWS_ENABLE_APPLY=true") {
		t.Fatalf("expected queued details, got %+v", result.Record.Details)
	}
	if len(result.Record.Operations) != 2 {
		t.Fatalf("expected projected operations, got %+v", result.Record.Operations)
	}
	if result.Record.Operations[0].Status != OperationStatusQueued || result.Record.Operations[1].Status != OperationStatusQueued {
		t.Fatalf("expected queued projected operation rows, got %+v", result.Record.Operations)
	}
}

func TestServiceCreateReturnsNotFoundWhenPlanIsMissing(t *testing.T) {
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{}, planpreview.ErrNotFound
		},
		Store: &memoryStore{},
	}

	_, err := service.Create(context.Background(), "plan_missing")
	if !errors.Is(err, planpreview.ErrNotFound) {
		t.Fatalf("expected plan not found error, got %v", err)
	}
}

func TestServiceCreateRequiresReadyPlan(t *testing.T) {
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{
				ID:             "plan_failed_1234abcd",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusFailed,
			}, nil
		},
		Store: &memoryStore{},
	}

	_, err := service.Create(context.Background(), "plan_failed_1234abcd")
	if !errors.Is(err, ErrPlanNotReady) {
		t.Fatalf("expected ErrPlanNotReady, got %v", err)
	}
}

func TestServiceCreateRejectsStalePlanFingerprintBeforePersisting(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Source:         planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		CurrentFingerprint: func(_ context.Context, deploymentName string) (string, error) {
			if deploymentName != plan.DeploymentName {
				t.Fatalf("unexpected fingerprint lookup: %s", deploymentName)
			}
			return "plan_newer", nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 7, 0, 0, time.UTC) },
	}

	_, err := service.Create(context.Background(), plan.ID)
	if !errors.Is(err, ErrSourceFingerprintConflict) {
		t.Fatalf("expected ErrSourceFingerprintConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "create a new plan preview before applying") {
		t.Fatalf("expected operator-facing refresh guidance, got %v", err)
	}
	if store.saveCalls != 0 || len(store.records) != 0 {
		t.Fatalf("expected stale plan conflict to avoid persistence, got %+v", store.records)
	}
}

func TestServiceCreateReturnsStableRecordIDEncodingErrors(t *testing.T) {
	originalMarshal := marshalStableRecordPayload
	marshalStableRecordPayload = func(any) ([]byte, error) {
		return nil, fmt.Errorf("boom")
	}
	t.Cleanup(func() {
		marshalStableRecordPayload = originalMarshal
	})

	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return planpreview.Record{
				ID:             "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         planpreview.StatusReady,
			}, nil
		},
		Store: &memoryStore{},
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 10, 0, 0, time.UTC) },
	}

	_, err := service.Create(context.Background(), "plan_ready_1234abcd")
	if err == nil || !strings.Contains(err.Error(), "encode stable apply request id") {
		t.Fatalf("expected stable id encoding error, got %v", err)
	}
}

func TestStableRecordIDIgnoresAdditiveOperationsProjection(t *testing.T) {
	base := Record{
		PlanID:         "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Source: SourceContext{
			PlanID:        "plan_ready_1234abcd",
			Route:         "/v1/plans/plan_ready_1234abcd",
			PlanStatus:    "ready",
			PlanCreatedAt: "2026-04-17T08:00:00Z",
			Fingerprint:   "plan_input",
		},
	}

	first, err := stableRecordID(base)
	if err != nil {
		t.Fatalf("stableRecordID returned error: %v", err)
	}

	base.Operations = []Operation{{
		Status:  OperationStatusPending,
		Phase:   "Provisioning",
		Target:  "api-prod deployment",
		Summary: "Create replacement capacity for api-prod.",
	}}

	second, err := stableRecordID(base)
	if err != nil {
		t.Fatalf("stableRecordID with additive operations returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable apply ID to ignore additive operations projection, got %q and %q", first, second)
	}
}

func TestServiceCreatePersistsAvailableActorFromRequestContext(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Operations: []planpreview.Operation{
			{Status: "pending", Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
		},
	}
	service := Service{
		LoadPlan: func(context.Context, string) (planpreview.Record, error) {
			return plan, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 12, 0, 0, time.UTC) },
	}

	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
		Email:     "jane@example.com",
	})

	result, err := service.Create(ctx, plan.ID)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Record.Actor.Available {
		t.Fatalf("expected available actor, got %+v", result.Record.Actor)
	}
	if result.Record.Actor.ID != "operator-123" || result.Record.Actor.Name != "Jane Operator" || result.Record.Actor.Email != "jane@example.com" {
		t.Fatalf("unexpected available actor details: %+v", result.Record.Actor)
	}
	if len(result.Record.Operations) != 1 || result.Record.Operations[0].Status != OperationStatusPending {
		t.Fatalf("expected additive projected operations for available actor record, got %+v", result.Record.Operations)
	}
}

func TestServiceTransitionAdvancesQueuedApplyRequestToRunningAndSucceeded(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_1234abcd": {
				ID:             "apply_1234abcd",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Details: []string{
					"Tenet persisted this immutable apply request as queued because TENET_AWS_ENABLE_APPLY=true.",
					"No AWS mutation has been implemented for this backend slice yet.",
				},
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
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 6, 0, 0, time.UTC) },
	}

	running, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusRunning,
		Step:   1,
	})
	if err != nil {
		t.Fatalf("Transition to running returned error: %v", err)
	}
	if running.ID != "apply_1234abcd" || running.Status != StatusRunning {
		t.Fatalf("expected stable apply id with running status, got %+v", running)
	}
	if running.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || running.Lifecycle.UpdatedAt != "2026-04-18T08:06:00Z" || running.Lifecycle.CurrentStep != 1 || running.Lifecycle.CompletedSteps != 0 || running.Lifecycle.TotalSteps != 2 {
		t.Fatalf("unexpected running lifecycle metadata: %+v", running.Lifecycle)
	}
	if statuses := []OperationStatus{running.Operations[0].Status, running.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusRunning, OperationStatusQueued}) {
		t.Fatalf("unexpected running operations: %+v", running.Operations)
	}

	service.Now = func() time.Time { return time.Date(2026, 4, 18, 8, 7, 0, 0, time.UTC) }
	succeeded, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("Transition to succeeded returned error: %v", err)
	}
	if succeeded.ID != "apply_1234abcd" || succeeded.Status != StatusSucceeded {
		t.Fatalf("expected stable apply id with succeeded status, got %+v", succeeded)
	}
	if succeeded.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || succeeded.Lifecycle.FinishedAt != "2026-04-18T08:07:00Z" || succeeded.Lifecycle.UpdatedAt != "2026-04-18T08:07:00Z" || succeeded.Lifecycle.CurrentStep != 2 || succeeded.Lifecycle.CompletedSteps != 2 || succeeded.Lifecycle.TotalSteps != 2 {
		t.Fatalf("unexpected succeeded lifecycle metadata: %+v", succeeded.Lifecycle)
	}
	if statuses := []OperationStatus{succeeded.Operations[0].Status, succeeded.Operations[1].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusSucceeded, OperationStatusSucceeded}) {
		t.Fatalf("unexpected succeeded operations: %+v", succeeded.Operations)
	}
}

func TestServiceTransitionMarksRunningStepAsFailed(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_1234abcd": {
				ID:             "apply_1234abcd",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusRunning,
				CreatedAt:      "2026-04-18T08:05:00Z",
				Lifecycle: Lifecycle{
					UpdatedAt:   "2026-04-18T08:06:00Z",
					StartedAt:   "2026-04-18T08:06:00Z",
					CurrentStep: 2,
					TotalSteps:  3,
				},
				Operations: []Operation{
					{Status: OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusRunning, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
					{Status: OperationStatusQueued, Phase: "Finalizing", Target: "api-prod alarms", Summary: "Verify final health."},
				},
			},
		},
	}
	service := Service{
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 18, 8, 8, 0, 0, time.UTC) },
	}

	result, err := service.Transition(context.Background(), "apply_1234abcd", TransitionRequest{
		Status: StatusFailed,
		Step:   2,
		Error:  "promotion failed",
	})
	if err != nil {
		t.Fatalf("Transition to failed returned error: %v", err)
	}
	if result.Status != StatusFailed || result.Error != "promotion failed" {
		t.Fatalf("expected failed apply record with propagated error, got %+v", result)
	}
	if result.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || result.Lifecycle.FinishedAt != "2026-04-18T08:08:00Z" || result.Lifecycle.CurrentStep != 2 || result.Lifecycle.CompletedSteps != 1 || result.Lifecycle.TotalSteps != 3 {
		t.Fatalf("unexpected failed lifecycle metadata: %+v", result.Lifecycle)
	}
	if statuses := []OperationStatus{result.Operations[0].Status, result.Operations[1].Status, result.Operations[2].Status}; !reflect.DeepEqual(statuses, []OperationStatus{OperationStatusSucceeded, OperationStatusFailed, OperationStatusQueued}) {
		t.Fatalf("unexpected failed operations: %+v", result.Operations)
	}
}

func TestServiceTransitionRejectsInvalidLifecycleUpdates(t *testing.T) {
	store := &memoryStore{
		records: map[string]Record{
			"apply_queued": {
				ID:             "apply_queued",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusQueued,
				Operations: []Operation{
					{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
					{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
				},
			},
			"apply_done": {
				ID:             "apply_done",
				PlanID:         "plan_ready_1234abcd",
				DeploymentName: "api-prod",
				Status:         StatusSucceeded,
				Operations: []Operation{
					{Status: OperationStatusSucceeded, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
				},
			},
		},
	}
	service := Service{Store: store}

	for _, tc := range []struct {
		name    string
		id      string
		request TransitionRequest
	}{
		{
			name: "unsupported status",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: Status("mystery"),
			},
		},
		{
			name: "running requires valid step",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: StatusRunning,
				Step:   3,
			},
		},
		{
			name: "succeeded rejects partial step",
			id:   "apply_queued",
			request: TransitionRequest{
				Status: StatusSucceeded,
				Step:   1,
			},
		},
		{
			name: "terminal record cannot advance",
			id:   "apply_done",
			request: TransitionRequest{
				Status: StatusRunning,
				Step:   1,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Transition(context.Background(), tc.id, tc.request)
			if !errors.Is(err, ErrInvalidState) {
				t.Fatalf("expected ErrInvalidState, got %v", err)
			}
		})
	}
}

func TestApplyLifecycleStatusHelpers(t *testing.T) {
	if !StatusQueued.Valid() || !StatusFailed.Valid() || !StatusCancelled.IsTerminal() {
		t.Fatalf("expected queued/failed/cancelled lifecycle helpers to report supported states")
	}
	if StatusRunning.IsTerminal() {
		t.Fatalf("running apply status should not be terminal")
	}
	if Status("mystery").Valid() {
		t.Fatalf("unexpected unknown apply status to validate")
	}
	if !OperationStatusQueued.Valid() || !OperationStatusSucceeded.IsTerminal() {
		t.Fatalf("expected queued/succeeded operation helpers to report supported states")
	}
	if OperationStatusRunning.IsTerminal() {
		t.Fatalf("running apply operation status should not be terminal")
	}
	if OperationStatus("mystery").Valid() {
		t.Fatalf("unexpected unknown apply operation status to validate")
	}
}
