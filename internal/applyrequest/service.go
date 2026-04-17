package applyrequest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/planpreview"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

const disabledError = "apply execution is disabled"

var (
	marshalStableRecordPayload = json.Marshal
)

// PlanProvider loads one persisted immutable plan preview by ID.
type PlanProvider func(context.Context, string) (planpreview.Record, error)

// Service derives and persists immutable apply requests from ready plan previews.
type Service struct {
	LoadPlan PlanProvider
	Store    Store
	Now      func() time.Time
}

// CreateResult captures the stored apply request plus whether it was newly created.
type CreateResult struct {
	Record  Record
	Created bool
}

// Create persists one immutable apply request for a ready plan preview.
func (s Service) Create(ctx context.Context, planID string) (CreateResult, error) {
	if s.LoadPlan == nil {
		return CreateResult{}, fmt.Errorf("apply request plan provider is required")
	}
	if s.Store == nil {
		return CreateResult{}, fmt.Errorf("apply request store is required")
	}

	plan, err := s.LoadPlan(ctx, planID)
	if err != nil {
		return CreateResult{}, err
	}
	if plan.Status != planpreview.StatusReady {
		return CreateResult{}, fmt.Errorf("%w: %s is %s", ErrPlanNotReady, plan.ID, plan.Status)
	}

	record := Record{
		PlanID:         plan.ID,
		DeploymentName: plan.DeploymentName,
		Status:         StatusFailed,
		CreatedAt:      s.now().UTC().Format(time.RFC3339),
		Actor:          requestactor.FromContext(ctx),
		Error:          disabledError,
		Details: []string{
			"Tenet persisted this immutable apply request in read-only mode and did not contact AWS.",
			"Live execution is intentionally disabled by default for this backend slice.",
		},
		Source: SourceContext{
			PlanID:        plan.ID,
			Route:         "/v1/plans/" + plan.ID,
			PlanStatus:    string(plan.Status),
			PlanCreatedAt: plan.CreatedAt,
			Fingerprint:   plan.Source.Fingerprint,
		},
	}
	record.ID, err = stableRecordID(record)
	if err != nil {
		return CreateResult{}, err
	}

	existing, err := s.Store.Get(ctx, record.ID)
	if err == nil {
		return CreateResult{Record: existing, Created: false}, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return CreateResult{}, err
	}
	if err := s.Store.Save(ctx, record); err != nil {
		return CreateResult{}, err
	}
	stored, err := s.Store.Get(ctx, record.ID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Record: stored, Created: true}, nil
}

// Get returns one persisted immutable apply request by ID.
func (s Service) Get(ctx context.Context, id string) (Record, error) {
	if s.Store == nil {
		return Record{}, fmt.Errorf("apply request store is required")
	}
	return s.Store.Get(ctx, id)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func stableRecordID(record Record) (string, error) {
	payload := struct {
		PlanID         string        `json:"planId"`
		DeploymentName string        `json:"deploymentName"`
		Source         SourceContext `json:"source"`
	}{
		PlanID:         record.PlanID,
		DeploymentName: record.DeploymentName,
		Source:         record.Source,
	}
	content, err := marshalStableRecordPayload(payload)
	if err != nil {
		return "", fmt.Errorf("encode stable apply request id: %w", err)
	}
	sum := sha256.Sum256(content)
	return "apply_" + hex.EncodeToString(sum[:8]), nil
}
