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
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Valid reports whether the apply-request lifecycle status is supported.
func (s Status) Valid() bool {
	switch s {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the apply-request lifecycle can no longer advance.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// OperationStatus captures the operator-facing lifecycle for one projected apply operation.
type OperationStatus string

const (
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusQueued    OperationStatus = "queued"
	OperationStatusRunning   OperationStatus = "running"
	OperationStatusSucceeded OperationStatus = "succeeded"
	OperationStatusFailed    OperationStatus = "failed"
	OperationStatusCancelled OperationStatus = "cancelled"
)

// Valid reports whether the projected operation lifecycle status is supported.
func (s OperationStatus) Valid() bool {
	switch s {
	case OperationStatusPending, OperationStatusQueued, OperationStatusRunning, OperationStatusSucceeded, OperationStatusFailed, OperationStatusCancelled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the projected operation lifecycle can no longer advance.
func (s OperationStatus) IsTerminal() bool {
	switch s {
	case OperationStatusSucceeded, OperationStatusFailed, OperationStatusCancelled:
		return true
	default:
		return false
	}
}

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
	Status  OperationStatus `json:"status"`
	Phase   string          `json:"phase"`
	Target  string          `json:"target"`
	Summary string          `json:"summary"`
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
