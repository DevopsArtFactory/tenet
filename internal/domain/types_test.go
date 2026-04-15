package domain

import "testing"

func TestSyncStateValid(t *testing.T) {
	if !SyncStateSynced.Valid() {
		t.Fatalf("expected %q to be valid", SyncStateSynced)
	}

	if SyncState("mystery").Valid() {
		t.Fatalf("expected arbitrary sync state to be invalid")
	}
}

func TestRolloutPhaseTerminal(t *testing.T) {
	if !RolloutPhaseCompleted.IsTerminal() {
		t.Fatalf("completed phase should be terminal")
	}

	if RolloutPhasePlanning.IsTerminal() {
		t.Fatalf("planning phase should not be terminal")
	}
}

func TestHealthStatusHelpers(t *testing.T) {
	if !HealthHealthy.IsHealthy() {
		t.Fatalf("healthy status should report healthy")
	}

	if HealthProgressing.IsHealthy() {
		t.Fatalf("progressing status should not report healthy")
	}
}

func TestDeploymentStatusIsSynced(t *testing.T) {
	status := DeploymentStatus{Sync: Sync{State: SyncStateSynced}}
	if !status.IsSynced() {
		t.Fatalf("synced status should report synced")
	}
}
