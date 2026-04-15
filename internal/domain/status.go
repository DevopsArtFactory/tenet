package domain

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

// RolloutStepSummary captures one controller-derived step in the current rollout plan.
type RolloutStepSummary struct {
	Kind    string `json:"kind"`
	Current bool   `json:"current,omitempty"`
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
	Steps              []RolloutStepSummary     `json:"steps,omitempty"`
	Execution          []RolloutExecutionDetail `json:"execution,omitempty"`
}

// StatusHistoryEntry captures one operator-facing runtime history event.
type StatusHistoryEntry struct {
	Timestamp    string       `json:"timestamp"`
	Source       string       `json:"source"`
	Summary      string       `json:"summary"`
	Sync         Sync         `json:"sync"`
	Health       Health       `json:"health"`
	RolloutPhase RolloutPhase `json:"rolloutPhase,omitempty"`
	CurrentStep  string       `json:"currentStep,omitempty"`
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
