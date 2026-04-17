package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

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
	if !RolloutPhaseRolledBack.IsTerminal() {
		t.Fatalf("rolled back phase should be terminal")
	}
	if !RolloutPhaseCancelled.IsTerminal() {
		t.Fatalf("cancelled phase should be terminal")
	}
	if RolloutPhaseFailed.IsTerminal() {
		t.Fatalf("failed phase should not be terminal because rollback may still proceed")
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
	status := DeploymentStatus{Sync: Sync{State: SyncStateSynced, Reason: "RolloutCompleted"}, Health: Health{Overall: HealthHealthy, Summary: "all checks passed"}, Conditions: []Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "candidate promoted"}}}
	if !status.IsSynced() {
		t.Fatalf("synced status should report synced")
	}
	if status.Sync.Reason != "RolloutCompleted" {
		t.Fatalf("unexpected sync reason: %+v", status.Sync)
	}
	if !status.Health.Overall.IsHealthy() {
		t.Fatalf("expected healthy overall status: %+v", status.Health)
	}
	if len(status.Conditions) != 1 || status.Conditions[0].Type != "Ready" {
		t.Fatalf("unexpected conditions: %+v", status.Conditions)
	}
}

func TestRuntimeSummaryMarshalJSONOmitsZeroArtifact(t *testing.T) {
	payload, err := json.Marshal(RuntimeSummary{
		Conditions: []Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "runtime context still available"}},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	encoded := string(payload)
	if strings.Contains(encoded, `"deployedArtifact"`) {
		t.Fatalf("expected zero-value deployedArtifact to be omitted, got %s", encoded)
	}
	if !strings.Contains(encoded, `"conditions":[`) {
		t.Fatalf("expected conditions to remain encoded, got %s", encoded)
	}
}

func TestStatusHistoryEntryMarshalJSONOmitsZeroArtifact(t *testing.T) {
	payload, err := json.Marshal(StatusHistoryEntry{
		Timestamp:  "2026-04-17T08:00:00Z",
		Source:     "controller",
		Summary:    "Runtime status updated",
		Conditions: []Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "history context still available"}},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	encoded := string(payload)
	if strings.Contains(encoded, `"deployedArtifact"`) {
		t.Fatalf("expected zero-value deployedArtifact to be omitted, got %s", encoded)
	}
	if !strings.Contains(encoded, `"conditions":[`) {
		t.Fatalf("expected conditions to remain encoded, got %s", encoded)
	}
}
