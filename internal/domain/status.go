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

// ManagedDriftSeverity classifies how urgently operators should inspect a drift item.
type ManagedDriftSeverity string

const (
	ManagedDriftSeverityHigh   ManagedDriftSeverity = "high"
	ManagedDriftSeverityMedium ManagedDriftSeverity = "medium"
	ManagedDriftSeverityLow    ManagedDriftSeverity = "low"
)

// ManagedDriftResourceRef identifies the live resource that produced a managed drift item.
type ManagedDriftResourceRef struct {
	ResourceType string `json:"resourceType"`
	ResourceName string `json:"resourceName"`
}

// ManagedDriftItem captures one controller-authored field-level drift item for Tenet-managed runtime fields.
type ManagedDriftItem struct {
	Field       string                   `json:"field"`
	Desired     string                   `json:"desired,omitempty"`
	Actual      string                   `json:"actual,omitempty"`
	Severity    ManagedDriftSeverity     `json:"severity,omitempty"`
	ResourceRef *ManagedDriftResourceRef `json:"resourceRef,omitempty"`
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

// ControllerRefreshEvidence captures non-secret metadata from a controller refresh handoff.
type ControllerRefreshEvidence struct {
	RequestID    string `json:"requestId"`
	DeliveryID   string `json:"deliveryId"`
	Repository   string `json:"repository"`
	Ref          string `json:"ref"`
	After        string `json:"after,omitempty"`
	RequestedAt  string `json:"requestedAt"`
	ReceivedAt   string `json:"receivedAt,omitempty"`
	ReconciledAt string `json:"reconciledAt,omitempty"`
}

// StatusHistoryEntry captures one operator-facing runtime history event.
type StatusHistoryEntry struct {
	EventID            string                     `json:"eventId,omitempty"`
	Timestamp          string                     `json:"timestamp"`
	Source             string                     `json:"source"`
	Summary            string                     `json:"summary"`
	Changes            []string                   `json:"changes,omitempty"`
	ChangeDetails      []string                   `json:"changeDetails,omitempty"`
	ControllerRefresh  *ControllerRefreshEvidence `json:"controllerRefresh,omitempty"`
	ManagedDrift       []ManagedDriftItem         `json:"managedDrift,omitempty"`
	DeployedArtifact   DeploymentArtifact         `json:"deployedArtifact,omitempty"`
	Conditions         []Condition                `json:"conditions,omitempty"`
	Sync               Sync                       `json:"sync"`
	Health             Health                     `json:"health"`
	DesiredGeneration  int64                      `json:"desiredGeneration"`
	ObservedGeneration int64                      `json:"observedGeneration"`
	RolloutPhase       RolloutPhase               `json:"rolloutPhase,omitempty"`
	CurrentStep        string                     `json:"currentStep,omitempty"`
	TargetType         string                     `json:"targetType,omitempty"`
	Strategy           string                     `json:"strategy,omitempty"`
	ExecutionMode      string                     `json:"executionMode,omitempty"`
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
	ManagedDrift       []ManagedDriftItem `json:"managedDrift,omitempty"`
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
	ManagedDrift       []ManagedDriftItem   `json:"managedDrift,omitempty"`
	History            []StatusHistoryEntry `json:"history,omitempty"`
	DeployedArtifact   DeploymentArtifact   `json:"deployedArtifact,omitempty"`
	Rollout            *RolloutSummary      `json:"rollout,omitempty"`
}

// IsSynced reports whether the deployment is currently synced.
func (s DeploymentStatus) IsSynced() bool {
	return s.Sync.State == SyncStateSynced
}
