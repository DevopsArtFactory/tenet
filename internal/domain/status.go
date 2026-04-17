package domain

import "encoding/json"

// SyncState summarizes whether desired and actual state are aligned.
type SyncState string

const (
	SyncStateSynced      SyncState = "Synced"
	SyncStateOutOfSync   SyncState = "OutOfSync"
	SyncStateReconciling SyncState = "Reconciling"
	SyncStateDrifted     SyncState = "Drifted"
	SyncStateError       SyncState = "Error"
	SyncStatePaused      SyncState = "Paused"
)

// Valid reports whether the sync state is one of the supported values.
func (s SyncState) Valid() bool {
	switch s {
	case SyncStateSynced, SyncStateOutOfSync, SyncStateReconciling, SyncStateDrifted, SyncStateError, SyncStatePaused:
		return true
	default:
		return false
	}
}

// HealthStatus summarizes deployment health.
type HealthStatus string

const (
	HealthHealthy     HealthStatus = "Healthy"
	HealthProgressing HealthStatus = "Progressing"
	HealthUnhealthy   HealthStatus = "Unhealthy"
	HealthUnknown     HealthStatus = "Unknown"
)

// IsHealthy reports whether the status is fully healthy.
func (h HealthStatus) IsHealthy() bool {
	return h == HealthHealthy
}

// Sync captures the current sync view.
type Sync struct {
	State  SyncState `json:"state"`
	Reason string    `json:"reason"`
}

// Health captures the current health summary.
type Health struct {
	Overall HealthStatus `json:"overall"`
	Summary string       `json:"summary"`
}

// RolloutStepStatus captures the lifecycle state of a rollout step.
type RolloutStepStatus string

const (
	RolloutStepStatusCompleted RolloutStepStatus = "completed"
	RolloutStepStatusCurrent   RolloutStepStatus = "current"
	RolloutStepStatusPending   RolloutStepStatus = "pending"
)

// RolloutStepSummary captures one controller-derived step in the current rollout plan.
type RolloutStepSummary struct {
	Kind    string            `json:"kind"`
	Detail  string            `json:"detail,omitempty"`
	Current bool              `json:"current,omitempty"`
	Status  RolloutStepStatus `json:"status,omitempty"`
}

// RolloutExecutionDetail captures one operator-facing execution resource detail.
type RolloutExecutionDetail struct {
	Resource    string `json:"resource"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// RolloutSummary captures the controller-owned rollout view for a deployment.
type RolloutSummary struct {
	DesiredGeneration  int64                    `json:"desiredGeneration"`
	ObservedGeneration int64                    `json:"observedGeneration"`
	Phase              RolloutPhase             `json:"phase,omitempty"`
	CurrentStep        string                   `json:"currentStep,omitempty"`
	Summary            string                   `json:"summary,omitempty"`
	TargetType         string                   `json:"targetType,omitempty"`
	Strategy           string                   `json:"strategy,omitempty"`
	ExecutionMode      string                   `json:"executionMode,omitempty"`
	Steps              []RolloutStepSummary     `json:"steps,omitempty"`
	Execution          []RolloutExecutionDetail `json:"execution,omitempty"`
}

// StatusHistoryEntry captures one operator-facing runtime history event.
type StatusHistoryEntry struct {
	Timestamp          string             `json:"timestamp"`
	Source             string             `json:"source"`
	Summary            string             `json:"summary"`
	Changes            []string           `json:"changes,omitempty"`
	ChangeDetails      []string           `json:"changeDetails,omitempty"`
	DeployedArtifact   DeploymentArtifact `json:"deployedArtifact,omitempty"`
	Conditions         []Condition        `json:"conditions,omitempty"`
	Sync               Sync               `json:"sync"`
	Health             Health             `json:"health"`
	DesiredGeneration  int64              `json:"desiredGeneration"`
	ObservedGeneration int64              `json:"observedGeneration"`
	RolloutPhase       RolloutPhase       `json:"rolloutPhase,omitempty"`
	CurrentStep        string             `json:"currentStep,omitempty"`
	TargetType         string             `json:"targetType,omitempty"`
	Strategy           string             `json:"strategy,omitempty"`
	ExecutionMode      string             `json:"executionMode,omitempty"`
}

// MarshalJSON omits zero-value deployedArtifact while preserving additive status fields.
func (e StatusHistoryEntry) MarshalJSON() ([]byte, error) {
	type alias StatusHistoryEntry
	payload := struct {
		alias
		DeployedArtifact *DeploymentArtifact `json:"deployedArtifact,omitempty"`
	}{alias: alias(e)}
	if e.DeployedArtifact != (DeploymentArtifact{}) {
		artifact := e.DeployedArtifact
		payload.DeployedArtifact = &artifact
	}
	return json.Marshal(payload)
}

// RuntimeSummary captures the latest stable operator-facing runtime metadata.
type RuntimeSummary struct {
	Timestamp          string             `json:"timestamp,omitempty"`
	Source             string             `json:"source,omitempty"`
	Summary            string             `json:"summary,omitempty"`
	Changes            []string           `json:"changes,omitempty"`
	ChangeDetails      []string           `json:"changeDetails,omitempty"`
	DeployedArtifact   DeploymentArtifact `json:"deployedArtifact,omitempty"`
	Conditions         []Condition        `json:"conditions,omitempty"`
	Sync               Sync               `json:"sync"`
	Health             Health             `json:"health"`
	DesiredGeneration  int64              `json:"desiredGeneration"`
	ObservedGeneration int64              `json:"observedGeneration"`
	RolloutPhase       RolloutPhase       `json:"rolloutPhase,omitempty"`
	CurrentStep        string             `json:"currentStep,omitempty"`
	TargetType         string             `json:"targetType,omitempty"`
	Strategy           string             `json:"strategy,omitempty"`
	ExecutionMode      string             `json:"executionMode,omitempty"`
	HistoryCount       int                `json:"historyCount"`
}

// MarshalJSON omits zero-value deployedArtifact while preserving additive runtime summary fields.
func (s RuntimeSummary) MarshalJSON() ([]byte, error) {
	type alias RuntimeSummary
	payload := struct {
		alias
		DeployedArtifact *DeploymentArtifact `json:"deployedArtifact,omitempty"`
	}{alias: alias(s)}
	if s.DeployedArtifact != (DeploymentArtifact{}) {
		artifact := s.DeployedArtifact
		payload.DeployedArtifact = &artifact
	}
	return json.Marshal(payload)
}

// DeploymentStatus captures observed state separate from desired spec.
type DeploymentStatus struct {
	ObservedGeneration int64                `json:"observedGeneration"`
	Sync               Sync                 `json:"sync"`
	Health             Health               `json:"health"`
	Conditions         []Condition          `json:"conditions"`
	History            []StatusHistoryEntry `json:"history,omitempty"`
	DeployedArtifact   DeploymentArtifact   `json:"deployedArtifact,omitempty"`
	Rollout            *RolloutSummary      `json:"rollout,omitempty"`
}

// IsSynced reports whether the deployment is currently synced.
func (s DeploymentStatus) IsSynced() bool {
	return s.Sync.State == SyncStateSynced
}
