package rollout

import (
	"fmt"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

// StateMachine validates rollout transitions and derives sync state summaries.
type StateMachine struct{}

var allowedTransitions = map[domain.RolloutPhase]map[domain.RolloutPhase]struct{}{
	domain.RolloutPhasePending: {
		domain.RolloutPhasePending:   {},
		domain.RolloutPhaseAccepted:  {},
		domain.RolloutPhaseCancelled: {},
	},
	domain.RolloutPhaseAccepted: {
		domain.RolloutPhaseAccepted:  {},
		domain.RolloutPhasePlanning:  {},
		domain.RolloutPhaseCancelled: {},
	},
	domain.RolloutPhasePlanning: {
		domain.RolloutPhasePlanning:     {},
		domain.RolloutPhaseProvisioning: {},
		domain.RolloutPhaseCancelled:    {},
	},
	domain.RolloutPhaseProvisioning: {
		domain.RolloutPhaseProvisioning:     {},
		domain.RolloutPhaseCandidateCreated: {},
		domain.RolloutPhaseFailed:           {},
		domain.RolloutPhaseRollingBack:      {},
	},
	domain.RolloutPhaseCandidateCreated: {
		domain.RolloutPhaseCandidateCreated: {},
		domain.RolloutPhaseHealthChecking:   {},
		domain.RolloutPhaseFailed:           {},
		domain.RolloutPhaseRollingBack:      {},
	},
	domain.RolloutPhaseHealthChecking: {
		domain.RolloutPhaseHealthChecking: {},
		domain.RolloutPhaseReadyToPromote: {},
		domain.RolloutPhaseFailed:         {},
		domain.RolloutPhaseRollingBack:    {},
	},
	domain.RolloutPhaseReadyToPromote: {
		domain.RolloutPhaseReadyToPromote: {},
		domain.RolloutPhasePromoting:      {},
		domain.RolloutPhaseCancelled:      {},
	},
	domain.RolloutPhasePromoting: {
		domain.RolloutPhasePromoting:   {},
		domain.RolloutPhaseStabilizing: {},
		domain.RolloutPhaseFailed:      {},
		domain.RolloutPhaseRollingBack: {},
	},
	domain.RolloutPhaseStabilizing: {
		domain.RolloutPhaseStabilizing: {},
		domain.RolloutPhaseCompleted:   {},
		domain.RolloutPhaseFailed:      {},
		domain.RolloutPhaseRollingBack: {},
	},
	domain.RolloutPhaseCompleted: {
		domain.RolloutPhaseCompleted: {},
	},
	domain.RolloutPhaseFailed: {
		domain.RolloutPhaseFailed:      {},
		domain.RolloutPhaseRollingBack: {},
	},
	domain.RolloutPhaseRollingBack: {
		domain.RolloutPhaseRollingBack: {},
		domain.RolloutPhaseRolledBack:  {},
		domain.RolloutPhaseFailed:      {},
	},
	domain.RolloutPhaseRolledBack: {
		domain.RolloutPhaseRolledBack: {},
	},
	domain.RolloutPhaseCancelled: {
		domain.RolloutPhaseCancelled: {},
	},
}

// Advance validates the requested transition.
func (StateMachine) Advance(from, to domain.RolloutPhase) (domain.RolloutPhase, error) {
	next, ok := allowedTransitions[from]
	if !ok {
		return "", fmt.Errorf("unknown phase %q", from)
	}
	if _, ok := allowedTransitions[to]; !ok {
		return "", fmt.Errorf("unknown phase %q", to)
	}
	if _, ok := next[to]; !ok {
		return "", fmt.Errorf("invalid transition from %q to %q", from, to)
	}

	return to, nil
}

// DeriveSyncState maps rollout progress and drift into a deployment sync state.
func (StateMachine) DeriveSyncState(phase domain.RolloutPhase, drifted bool) domain.SyncState {
	if drifted {
		return domain.SyncStateDrifted
	}

	switch phase {
	case domain.RolloutPhaseCompleted:
		return domain.SyncStateSynced
	case domain.RolloutPhaseFailed:
		return domain.SyncStateError
	case domain.RolloutPhaseRolledBack, domain.RolloutPhaseCancelled:
		return domain.SyncStateOutOfSync
	case domain.RolloutPhasePending, domain.RolloutPhaseAccepted, domain.RolloutPhasePlanning,
		domain.RolloutPhaseProvisioning, domain.RolloutPhaseCandidateCreated,
		domain.RolloutPhaseHealthChecking, domain.RolloutPhaseReadyToPromote,
		domain.RolloutPhasePromoting, domain.RolloutPhaseStabilizing,
		domain.RolloutPhaseRollingBack:
		return domain.SyncStateReconciling
	default:
		return domain.SyncStateOutOfSync
	}
}
