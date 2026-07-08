package applyrequest

import (
	"context"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var (
	ErrNotFound                  = fmt.Errorf("apply request not found")
	ErrPlanNotReady              = fmt.Errorf("plan preview is not ready")
	ErrSourceFingerprintConflict = fmt.Errorf("apply request plan source fingerprint conflict")
	ErrInvalidState              = fmt.Errorf("apply request lifecycle transition is invalid")
	ErrInvalidClaimRequest       = fmt.Errorf("apply request claim request is invalid")
	ErrAlreadyClaimed            = fmt.Errorf("apply request is already claimed by another worker")
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
	ID      string          `json:"id,omitempty"`
	Status  OperationStatus `json:"status"`
	Phase   string          `json:"phase"`
	Target  string          `json:"target"`
	Summary string          `json:"summary"`
}

// Lifecycle captures additive operator-facing progress metadata for one apply request.
type Lifecycle struct {
	UpdatedAt      string `json:"updatedAt,omitempty"`
	StartedAt      string `json:"startedAt,omitempty"`
	FinishedAt     string `json:"finishedAt,omitempty"`
	CurrentStep    int    `json:"currentStep,omitempty"`
	CompletedSteps int    `json:"completedSteps,omitempty"`
	TotalSteps     int    `json:"totalSteps,omitempty"`
}

// Claim captures the local-safe executor lease metadata for one queued apply request.
type Claim struct {
	WorkerID        string `json:"workerId"`
	ClaimedAt       string `json:"claimedAt"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	ExpiresAt       string `json:"expiresAt,omitempty"`
}

// Transition captures one successful lifecycle advance for immutable audit projection.
type Transition struct {
	TransitionedAt string             `json:"transitionedAt"`
	PreviousStatus Status             `json:"previousStatus"`
	Status         Status             `json:"status"`
	Step           int                `json:"step,omitempty"`
	Actor          requestactor.Actor `json:"actor"`
	Error          string             `json:"error,omitempty"`
	Details        []string           `json:"details,omitempty"`
	Lifecycle      Lifecycle          `json:"lifecycle,omitempty"`
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
	Lifecycle      Lifecycle          `json:"lifecycle,omitempty"`
	Claim          *Claim             `json:"claim,omitempty"`
	Transitions    []Transition       `json:"transitions,omitempty"`
	Operations     []Operation        `json:"operations,omitempty"`
	Source         SourceContext      `json:"source"`
}

// ClaimRequest captures one executor's local-safe attempt to claim queued work.
type ClaimRequest struct {
	WorkerID string `json:"workerId"`
}

// HeartbeatRequest captures one claimed executor's local-safe lease refresh.
type HeartbeatRequest struct {
	WorkerID string `json:"workerId"`
}

// TransitionRequest captures one validated lifecycle advance for a persisted apply request.
type TransitionRequest struct {
	Status  Status   `json:"status"`
	Step    int      `json:"step,omitempty"`
	Error   string   `json:"error,omitempty"`
	Details []string `json:"details,omitempty"`
}

// ListQuery captures the narrow read filters supported for apply requests.
type ListQuery struct {
	Status Status
}

// Store persists immutable apply-request records.
type Store interface {
	Get(context.Context, string) (Record, error)
	List(context.Context) ([]Record, error)
	Save(context.Context, Record) error
	Update(context.Context, Record) error
}

// ClaimStore atomically claims one queued apply request when the backend can
// protect the read/check/write boundary. FileStore implements this for the
// local-safe executor handoff path.
type ClaimStore interface {
	Claim(context.Context, string, ClaimRequest, func(Record) (Record, error)) (Record, error)
}

// HeartbeatStore atomically refreshes one claimed apply request when the backend
// can protect the read/check/write boundary. FileStore implements this for the
// local-safe executor lease path.
type HeartbeatStore interface {
	Heartbeat(context.Context, string, HeartbeatRequest, func(Record) (Record, error)) (Record, error)
}
