package planpreview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/catalog"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
	"github.com/DevopsArtFactory/tenet/internal/statusstore"
)

type memoryStore struct {
	records map[string]Record
}

func (s *memoryStore) Get(_ context.Context, id string) (Record, error) {
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

func (s *memoryStore) Save(_ context.Context, record Record) error {
	if s.records == nil {
		s.records = map[string]Record{}
	}
	if _, exists := s.records[record.ID]; !exists {
		s.records[record.ID] = record
	}
	return nil
}

func TestServiceCreatePersistsImmutableReadyPlanWithStableID(t *testing.T) {
	store := &memoryStore{}
	detail := catalog.DeploymentOverviewDetail{
		Name:               "api-prod",
		TargetType:         "ecs",
		Strategy:           "linear",
		ExecutionMode:      "ecsCodeDeploy",
		DesiredArtifact:    domain.DeploymentArtifact{Image: "payments-api:v2"},
		ObservedGeneration: 7,
		Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "PendingPromotion"},
		Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming up"},
		RuntimeSummary: &domain.RuntimeSummary{
			Timestamp:          "2026-04-16T06:49:00Z",
			Source:             "controller",
			Summary:            "Traffic shift is progressing",
			Sync:               domain.Sync{State: domain.SyncStateReconciling, Reason: "Traffic shift running"},
			Health:             domain.Health{Overall: domain.HealthHealthy, Summary: "steady"},
			DesiredGeneration:  8,
			ObservedGeneration: 7,
			CurrentStep:        "ShiftTrafficLinear",
			HistoryCount:       3,
		},
		Rollout: &domain.RolloutSummary{
			DesiredGeneration:  8,
			ObservedGeneration: 7,
			CurrentStep:        "VerifyServiceStability",
			Steps: []domain.RolloutStepSummary{
				{Kind: "CreateCodeDeployDeployment", Status: domain.RolloutStepStatusCompleted},
				{Kind: "ShiftTrafficLinear", Status: domain.RolloutStepStatusPending},
				{Kind: "VerifyServiceStability", Current: true, Status: domain.RolloutStepStatusCurrent},
			},
			Execution: []domain.RolloutExecutionDetail{
				{Resource: "ecsService", Name: "api-prod-service"},
				{Resource: "codeDeployDeploymentGroup", Name: "api-prod-codedeploy-group"},
			},
		},
	}
	fingerprint, err := catalog.DeriveDeploymentSourceFingerprint(detail)
	if err != nil {
		t.Fatalf("DeriveDeploymentSourceFingerprint returned error: %v", err)
	}
	service := Service{
		LoadDetail: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			if name != "api-prod" {
				t.Fatalf("unexpected deployment lookup: %s", name)
			}
			return detail, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 17, 8, 0, 0, 0, time.UTC) },
	}

	first, err := service.Create(context.Background(), "api-prod", CreateRequest{IfMatch: fingerprint})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !first.Created {
		t.Fatalf("expected first create to persist a new preview")
	}
	if first.Record.ID == "" || first.Record.Status != StatusReady {
		t.Fatalf("unexpected created record: %+v", first.Record)
	}
	if len(first.Record.Operations) != 3 || first.Record.Operations[1].Target != "api-prod-codedeploy-group (codeDeployDeploymentGroup)" {
		t.Fatalf("unexpected derived operations: %+v", first.Record.Operations)
	}
	if first.Record.Risk.Level != "medium" || !first.Record.Approval.Required {
		t.Fatalf("expected derived medium-risk approval metadata, got %+v %+v", first.Record.Risk, first.Record.Approval)
	}
	if first.Record.Actor.Available || first.Record.Actor.Reason != requestactor.UnavailableReason {
		t.Fatalf("expected explicit unavailable actor, got %+v", first.Record.Actor)
	}
	if first.Record.Source.Fingerprint != fingerprint || first.Record.Source.DesiredGeneration != 8 || first.Record.Source.ObservedGeneration != 7 {
		t.Fatalf("unexpected source context: %+v", first.Record.Source)
	}

	second, err := service.Create(context.Background(), "api-prod", CreateRequest{IfMatch: fingerprint})
	if err != nil {
		t.Fatalf("second Create returned error: %v", err)
	}
	if second.Created {
		t.Fatalf("expected repeated create to reuse the immutable preview")
	}
	if second.Record.ID != first.Record.ID || second.Record.CreatedAt != "2026-04-17T08:00:00Z" {
		t.Fatalf("expected stable readback, got first=%+v second=%+v", first.Record, second.Record)
	}

	fetched, err := service.Get(context.Background(), first.Record.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if fetched.ID != first.Record.ID || fetched.Operations[2].Kind != "VerifyServiceStability" {
		t.Fatalf("unexpected fetched plan preview: %+v", fetched)
	}
}

func TestServiceCreatePersistsFailedPlanWhenClientFingerprintIsStale(t *testing.T) {
	store := &memoryStore{}
	service := Service{
		LoadDetail: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name:               name,
				TargetType:         "ec2",
				Strategy:           "canary",
				ExecutionMode:      "asgWeightedTargetGroups",
				DesiredArtifact:    domain.DeploymentArtifact{Binary: "s3://bucket/releases/api-prod"},
				ObservedGeneration: 4,
				Sync:               domain.Sync{State: domain.SyncStateOutOfSync, Reason: "CandidatePending"},
				Health:             domain.Health{Overall: domain.HealthProgressing, Summary: "warming"},
				Rollout: &domain.RolloutSummary{
					DesiredGeneration:  5,
					ObservedGeneration: 4,
					CurrentStep:        "ShiftTrafficCanary",
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCandidateCapacity", Status: domain.RolloutStepStatusCompleted},
						{Kind: "ShiftTrafficCanary", Current: true, Status: domain.RolloutStepStatusCurrent},
					},
				},
			}, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 17, 8, 5, 0, 0, time.UTC) },
	}

	result, err := service.Create(context.Background(), "api-prod", CreateRequest{IfMatch: "plan_deadbeef"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected stale preview creation to persist a new failed record")
	}
	if result.Record.Status != StatusFailed {
		t.Fatalf("expected failed stale plan preview, got %+v", result.Record)
	}
	if len(result.Record.Errors) != 1 || !strings.Contains(result.Record.Errors[0], "stale source fingerprint") {
		t.Fatalf("expected stale error details, got %+v", result.Record.Errors)
	}
	if !result.Record.Approval.Required || !strings.Contains(result.Record.Approval.Summary, "refreshes deployment detail") {
		t.Fatalf("expected blocking approval guidance, got %+v", result.Record.Approval)
	}
	fetched, err := service.Get(context.Background(), result.Record.ID)
	if err != nil {
		t.Fatalf("Get returned error for failed preview: %v", err)
	}
	if fetched.ID != result.Record.ID || fetched.Status != StatusFailed || len(fetched.Errors) != 1 {
		t.Fatalf("expected failed preview to be persisted, got %+v", fetched)
	}
}

func TestServiceCreatePropagatesDeploymentLookupErrors(t *testing.T) {
	service := Service{
		LoadDetail: func(_ context.Context, _ string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{}, statusstore.ErrNotFound
		},
		Store: &memoryStore{},
	}

	_, err := service.Create(context.Background(), "missing", CreateRequest{})
	if !errors.Is(err, statusstore.ErrNotFound) {
		t.Fatalf("expected deployment lookup error, got %v", err)
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
		LoadDetail: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name:          name,
				TargetType:    "ec2",
				Strategy:      "canary",
				ExecutionMode: "asgWeightedTargetGroups",
				Sync:          domain.Sync{State: domain.SyncStateSynced},
				Health:        domain.Health{Overall: domain.HealthHealthy},
				Rollout: &domain.RolloutSummary{
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCandidateCapacity", Status: domain.RolloutStepStatusCompleted},
					},
				},
			}, nil
		},
		Store: &memoryStore{},
		Now:   func() time.Time { return time.Date(2026, 4, 17, 8, 10, 0, 0, time.UTC) },
	}

	_, err := service.Create(context.Background(), "api-prod", CreateRequest{})
	if err == nil || !strings.Contains(err.Error(), "encode stable plan preview id") {
		t.Fatalf("expected stable id encoding error, got %v", err)
	}
}

func TestNormalizeRecordBackfillsStableOperationIDs(t *testing.T) {
	record := Record{Operations: []Operation{
		{Kind: "ShiftTraffic", Status: "pending", Phase: "Promoting", Target: "listener", Summary: "shift traffic"},
		{Kind: "ShiftTraffic", Status: "current", Phase: "Promoting", Target: "listener", Summary: "shift traffic"},
		{ID: "operator-authored-anchor", Kind: "Verify", Status: "pending", Phase: "Verifying", Target: "service", Summary: "verify service"},
	}}

	normalized := NormalizeRecord(record)
	if len(normalized.Operations) != 3 {
		t.Fatalf("expected normalized operations, got %+v", normalized.Operations)
	}
	firstID := normalized.Operations[0].ID
	if firstID == "" || !strings.HasPrefix(firstID, "operation-") {
		t.Fatalf("expected deterministic generated operation id, got %+v", normalized.Operations)
	}
	if normalized.Operations[1].ID != firstID+"-2" {
		t.Fatalf("expected duplicate operation id suffix, got %+v", normalized.Operations)
	}
	if normalized.Operations[2].ID != "operator-authored-anchor" {
		t.Fatalf("expected existing operation id to be preserved, got %+v", normalized.Operations)
	}
	if record.Operations[0].ID != "" {
		t.Fatalf("NormalizeRecord should not mutate caller-owned operation slices, got %+v", record.Operations)
	}
}

func TestStableRecordIDIgnoresAdditiveOperationIDs(t *testing.T) {
	base := Record{
		DeploymentName: "api-prod",
		Status:         StatusReady,
		Operations: []Operation{
			{Kind: "ShiftTraffic", Status: "current", Phase: "Promoting", Target: "listener", Summary: "shift traffic"},
		},
		Source: SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "fingerprint"},
	}
	withID := base
	withID.Operations = []Operation{
		{ID: "operation-custom-anchor", Kind: "ShiftTraffic", Status: "current", Phase: "Promoting", Target: "listener", Summary: "shift traffic"},
	}

	baseID, err := stableRecordID(base)
	if err != nil {
		t.Fatalf("stableRecordID(base) returned error: %v", err)
	}
	withIDValue, err := stableRecordID(withID)
	if err != nil {
		t.Fatalf("stableRecordID(withID) returned error: %v", err)
	}
	if baseID != withIDValue {
		t.Fatalf("expected additive operation ids to be excluded from stable plan id, got %s vs %s", baseID, withIDValue)
	}
}

func TestServiceCreatePersistsAvailableActorFromRequestContext(t *testing.T) {
	store := &memoryStore{}
	service := Service{
		LoadDetail: func(_ context.Context, name string) (catalog.DeploymentOverviewDetail, error) {
			return catalog.DeploymentOverviewDetail{
				Name:          name,
				TargetType:    "ec2",
				Strategy:      "canary",
				ExecutionMode: "asgWeightedTargetGroups",
				Sync:          domain.Sync{State: domain.SyncStateSynced},
				Health:        domain.Health{Overall: domain.HealthHealthy},
				Rollout: &domain.RolloutSummary{
					Steps: []domain.RolloutStepSummary{
						{Kind: "CreateCandidateCapacity", Status: domain.RolloutStepStatusCompleted},
					},
				},
			}, nil
		},
		Store: store,
		Now:   func() time.Time { return time.Date(2026, 4, 17, 8, 12, 0, 0, time.UTC) },
	}

	ctx := requestactor.WithContext(context.Background(), requestactor.Actor{
		Available: true,
		ID:        "operator-123",
		Name:      "Jane Operator",
		Email:     "jane@example.com",
	})

	result, err := service.Create(ctx, "api-prod", CreateRequest{})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !result.Record.Actor.Available {
		t.Fatalf("expected available actor, got %+v", result.Record.Actor)
	}
	if result.Record.Actor.ID != "operator-123" || result.Record.Actor.Name != "Jane Operator" || result.Record.Actor.Email != "jane@example.com" {
		t.Fatalf("unexpected available actor details: %+v", result.Record.Actor)
	}
	if result.Record.Actor.Reason != "" {
		t.Fatalf("expected no unavailable reason for available actor, got %+v", result.Record.Actor)
	}
}
