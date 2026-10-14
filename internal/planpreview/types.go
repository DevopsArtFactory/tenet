package planpreview

import (
	"context"
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/requestactor"
)

var ErrNotFound = fmt.Errorf("plan preview not found")

// Status captures the immutable plan preview lifecycle.
type Status string

const (
	StatusPending Status = "pending"
	StatusReady   Status = "ready"
	StatusFailed  Status = "failed"
)

// CreateRequest captures optional optimistic-staleness guards for plan creation.
type CreateRequest struct {
	IfMatch string `json:"ifMatch,omitempty"`
}

// Operation is one server-authored plan preview row.
type Operation struct {
	ECS     *domain.ECSDeploymentIntent `json:"ecs,omitempty"`
	ID      string                      `json:"id,omitempty"`
	Kind    string                      `json:"kind"`
	Status  string                      `json:"status"`
	Phase   string                      `json:"phase"`
	Target  string                      `json:"target"`
	Summary string                      `json:"summary"`
}

// Risk captures derived operator-facing risk metadata.
type Risk struct {
	Level    string   `json:"level"`
	Label    string   `json:"label"`
	Guidance string   `json:"guidance"`
	Reasons  []string `json:"reasons,omitempty"`
}

// Approval captures derived approval guidance.
type Approval struct {
	Required bool     `json:"required"`
	Summary  string   `json:"summary"`
	Reasons  []string `json:"reasons,omitempty"`
}

// SourceContext captures the deployment-detail inputs used to derive a preview.
type SourceContext struct {
	DeploymentName     string        `json:"deploymentName"`
	Route              string        `json:"route"`
	Fingerprint        string        `json:"fingerprint"`
	TargetType         string        `json:"targetType,omitempty"`
	Strategy           string        `json:"strategy,omitempty"`
	ExecutionMode      string        `json:"executionMode,omitempty"`
	RuntimeTimestamp   string        `json:"runtimeTimestamp,omitempty"`
	RuntimeSource      string        `json:"runtimeSource,omitempty"`
	CurrentStep        string        `json:"currentStep,omitempty"`
	DesiredGeneration  int64         `json:"desiredGeneration,omitempty"`
	ObservedGeneration int64         `json:"observedGeneration,omitempty"`
	Sync               domain.Sync   `json:"sync"`
	Health             domain.Health `json:"health"`
}

// Record is an immutable, deployment-scoped plan preview snapshot.
type Record struct {
	ID             string             `json:"id"`
	DeploymentName string             `json:"deploymentName"`
	Status         Status             `json:"status"`
	CreatedAt      string             `json:"createdAt"`
	Actor          requestactor.Actor `json:"actor"`
	Operations     []Operation        `json:"operations,omitempty"`
	Risk           Risk               `json:"risk"`
	Approval       Approval           `json:"approval"`
	Source         SourceContext      `json:"source"`
	Errors         []string           `json:"errors,omitempty"`
}

// Store persists immutable plan preview records.
type Store interface {
	Get(context.Context, string) (Record, error)
	Save(context.Context, Record) error
}
