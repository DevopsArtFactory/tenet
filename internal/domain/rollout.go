package domain

// RolloutPhase describes lifecycle progress for a desired generation.
type RolloutPhase string

const (
	RolloutPhasePending          RolloutPhase = "Pending"
	RolloutPhaseAccepted         RolloutPhase = "Accepted"
	RolloutPhasePlanning         RolloutPhase = "Planning"
	RolloutPhaseProvisioning     RolloutPhase = "Provisioning"
	RolloutPhaseCandidateCreated RolloutPhase = "CandidateCreated"
	RolloutPhaseHealthChecking   RolloutPhase = "HealthChecking"
	RolloutPhaseReadyToPromote   RolloutPhase = "ReadyToPromote"
	RolloutPhasePromoting        RolloutPhase = "Promoting"
	RolloutPhaseStabilizing      RolloutPhase = "Stabilizing"
	RolloutPhaseCompleted        RolloutPhase = "Completed"
	RolloutPhaseFailed           RolloutPhase = "Failed"
	RolloutPhaseRollingBack      RolloutPhase = "RollingBack"
	RolloutPhaseRolledBack       RolloutPhase = "RolledBack"
	RolloutPhaseCancelled        RolloutPhase = "Cancelled"
)

// IsTerminal reports whether no forward progress remains in the current phase.
func (p RolloutPhase) IsTerminal() bool {
	switch p {
	case RolloutPhaseCompleted, RolloutPhaseFailed, RolloutPhaseRolledBack, RolloutPhaseCancelled:
		return true
	default:
		return false
	}
}
