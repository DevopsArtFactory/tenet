package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DevopsArtFactory/tenet/internal/domain"
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

// ListStatusActive is the collection-only filter for non-terminal local-safe
// apply handoff records. It is not a lifecycle status for persisted records.
const ListStatusActive Status = "active"

// ListClaim captures collection-only filters over local-safe executor claims.
type ListClaim string

const (
	ListClaimStale ListClaim = "stale"
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

// OperationExecutionAttempt captures local-safe executor evidence for one apply operation.
// It is Tenet-owned control-plane metadata only; it does not prove live AWS mutation.
type OperationExecutionAttempt struct {
	WorkerID        string          `json:"workerId,omitempty"`
	Status          OperationStatus `json:"status,omitempty"`
	StartedAt       string          `json:"startedAt,omitempty"`
	LastHeartbeatAt string          `json:"lastHeartbeatAt,omitempty"`
	FinishedAt      string          `json:"finishedAt,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Summary         string          `json:"summary,omitempty"`
}

// Operation is one additive operator-facing execution row projected from a ready plan preview.
type Operation struct {
	ECSCheckpoint    *ECSCheckpoint              `json:"ecsCheckpoint,omitempty"`
	Kind             string                      `json:"kind,omitempty"`
	ECS              *domain.ECSDeploymentIntent `json:"ecs,omitempty"`
	ID               string                      `json:"id,omitempty"`
	Status           OperationStatus             `json:"status"`
	Phase            string                      `json:"phase"`
	Target           string                      `json:"target"`
	Summary          string                      `json:"summary"`
	ExecutionAttempt *OperationExecutionAttempt  `json:"executionAttempt,omitempty"`
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
	RetryDeploymentID  string                 `json:"retryDeploymentId,omitempty"`
	Recovery           *ECSRecovery           `json:"recovery,omitempty"`
	RetryOf            string                 `json:"retryOf,omitempty"`
	Principal          *ExecutionPrincipal    `json:"principal,omitempty"`
	ECSRuntime         *ECSRolloutObservation `json:"ecsRuntime,omitempty"`
	ID                 string                 `json:"id"`
	PlanID             string                 `json:"planId"`
	DeploymentName     string                 `json:"deploymentName"`
	Status             Status                 `json:"status"`
	CreatedAt          string                 `json:"createdAt"`
	Actor              requestactor.Actor     `json:"actor"`
	Error              string                 `json:"error,omitempty"`
	Details            []string               `json:"details,omitempty"`
	Lifecycle          Lifecycle              `json:"lifecycle,omitempty"`
	Claim              *Claim                 `json:"claim,omitempty"`
	Transitions        []Transition           `json:"transitions,omitempty"`
	Operations         []Operation            `json:"operations,omitempty"`
	ExecutionReadiness *ExecutionReadiness    `json:"executionReadiness,omitempty"`
	Source             SourceContext          `json:"source"`
}

// ClaimRequest captures one executor's local-safe attempt to claim queued work.
type ClaimRequest struct {
	WorkerID          string `json:"workerId"`
	AllowExpiredClaim bool   `json:"allowExpiredClaim,omitempty"`
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
	Claim  ListClaim
}

// Matches reports whether record belongs in this list query.
func (q ListQuery) Matches(record Record, now time.Time) bool {
	switch q.Status {
	case "":
		return q.claimMatches(record, now)
	case ListStatusActive:
		if record.Status != StatusQueued && record.Status != StatusRunning {
			return false
		}
		return q.claimMatches(record, now)
	default:
		return record.Status == q.Status && q.claimMatches(record, now)
	}
}

// NewestFirst reports whether list results should use created-time descending order.
func (q ListQuery) NewestFirst() bool {
	return q.Status == ListStatusActive
}

func (q ListQuery) claimMatches(record Record, now time.Time) bool {
	switch q.Claim {
	case "":
		return true
	case ListClaimStale:
		if record.Status != StatusRunning || record.Claim == nil {
			return false
		}
		expired, _, _ := claimExpired(*record.Claim, now)
		return expired
	default:
		return false
	}
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

// MutationStore protects the read/check/write boundary for lifecycle changes.
// Live executors require this atomic boundary rather than Store.Update snapshots.
type MutationStore interface {
	Mutate(context.Context, string, func(Record) (Record, error)) (Record, error)
}

// ExecutionPrincipal is trusted approval evidence, separate from actor attribution.
// It contains no credentials. Workers must evaluate it against current repository policy.
type ExecutionPrincipal struct {
	Identity requestactor.AuthenticatedIdentity `json:"identity"`
	Groups   requestactor.AuthenticatedGroups   `json:"groups"`
}

func (p *ExecutionPrincipal) Clone() *ExecutionPrincipal {
	if p == nil {
		return nil
	}
	copied := *p
	copied.Groups.Names = append([]string(nil), p.Groups.Names...)
	return &copied
}

var ErrExecutionUnauthorized = errors.New("ECS execution requires an authenticated operator with deployment access")
