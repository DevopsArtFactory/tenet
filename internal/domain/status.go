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

// DeploymentStatus captures observed state separate from desired spec.
type DeploymentStatus struct {
	ObservedGeneration int64       `json:"observedGeneration"`
	Sync               Sync        `json:"sync"`
	Health             Health      `json:"health"`
	Conditions         []Condition `json:"conditions"`
}

// IsSynced reports whether the deployment is currently synced.
func (s DeploymentStatus) IsSynced() bool {
	return s.Sync.State == SyncStateSynced
}
