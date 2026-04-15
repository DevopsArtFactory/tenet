package rollout

import (
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestAdvanceAllowsForwardProgress(t *testing.T) {
	sm := StateMachine{}
	phase, err := sm.Advance(domain.RolloutPhasePending, domain.RolloutPhaseAccepted)
	if err != nil {
		t.Fatalf("Advance returned error: %v", err)
	}
	if phase != domain.RolloutPhaseAccepted {
		t.Fatalf("unexpected phase: %s", phase)
	}
}

func TestAdvanceRejectsSkippedTransition(t *testing.T) {
	sm := StateMachine{}
	_, err := sm.Advance(domain.RolloutPhasePending, domain.RolloutPhaseCompleted)
	if err == nil {
		t.Fatalf("expected skipped transition to fail")
	}
}

func TestAdvanceRejectsBackwardTransition(t *testing.T) {
	sm := StateMachine{}
	_, err := sm.Advance(domain.RolloutPhasePlanning, domain.RolloutPhaseAccepted)
	if err == nil {
		t.Fatalf("expected backward transition to fail")
	}
}

func TestDeriveSyncState(t *testing.T) {
	sm := StateMachine{}
	state := sm.DeriveSyncState(domain.RolloutPhaseCompleted, false)
	if state != domain.SyncStateSynced {
		t.Fatalf("expected completed rollout to be synced, got %s", state)
	}

	state = sm.DeriveSyncState(domain.RolloutPhaseHealthChecking, false)
	if state != domain.SyncStateReconciling {
		t.Fatalf("expected active rollout to be reconciling, got %s", state)
	}

	state = sm.DeriveSyncState(domain.RolloutPhaseRolledBack, false)
	if state != domain.SyncStateOutOfSync {
		t.Fatalf("expected rolled back rollout to be out of sync, got %s", state)
	}

	state = sm.DeriveSyncState(domain.RolloutPhaseCancelled, false)
	if state != domain.SyncStateOutOfSync {
		t.Fatalf("expected cancelled rollout to be out of sync, got %s", state)
	}

	state = sm.DeriveSyncState(domain.RolloutPhaseCompleted, true)
	if state != domain.SyncStateDrifted {
		t.Fatalf("expected drift to override completed rollout, got %s", state)
	}
}
