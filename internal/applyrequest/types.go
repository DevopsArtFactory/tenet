package applyrequest

import (
	"context"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var (
	ErrNotFound     = fmt.Errorf("apply request not found")
	ErrPlanNotReady = fmt.Errorf("plan preview is not ready")
)

// Status captures the immutable apply-request lifecycle.
type Status string

const (
	StatusPending Status = "pending"
	StatusFailed  Status = "failed"
)

// SourceContext captures the immutable plan preview that an apply request references.
type SourceContext struct {
	PlanID        string `json:"planId"`
	Route         string `json:"route"`
	PlanStatus    string `json:"planStatus"`
	PlanCreatedAt string `json:"planCreatedAt,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

// Operation is one additive operator-facing execution row projected from a ready plan preview.
type Operation struct {
	Status  string `json:"status"`
	Phase   string `json:"phase"`
	Target  string `json:"target"`
	Summary string `json:"summary"`
}

// Record is one immutable apply request derived from a persisted ready plan preview.
type Record struct {
	ID             string             `json:"id"`
	PlanID         string             `json:"planId"`
	DeploymentName string             `json:"deploymentName"`
	Status         Status             `json:"status"`
	CreatedAt      string             `json:"createdAt"`
	Actor          requestactor.Actor `json:"actor"`
	Error          string             `json:"error,omitempty"`
	Details        []string           `json:"details,omitempty"`
	Operations     []Operation        `json:"operations,omitempty"`
	Source         SourceContext      `json:"source"`
}

// Store persists immutable apply-request records.
type Store interface {
	Get(context.Context, string) (Record, error)
	Save(context.Context, Record) error
}
