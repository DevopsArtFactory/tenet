package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
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

func TestServiceCreatePersistsImmutableFailedApplyRequestWhenExecutionIsDisabled(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
		Source:         planpreview.SourceContext{DeploymentName: "api-prod", Route: "/v1/overview/deployments/api-prod", Fingerprint: "plan_input"},
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

func TestServiceCreatePersistsAvailableActorFromRequestContext(t *testing.T) {
	store := &memoryStore{}
	plan := planpreview.Record{
		ID:             "plan_ready_1234abcd",
		DeploymentName: "api-prod",
		Status:         planpreview.StatusReady,
		CreatedAt:      "2026-04-17T08:00:00Z",
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
}
