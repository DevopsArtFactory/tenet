package statusstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestSaveAndLoadRoundTripsSnapshots(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "status", "deployments.json")
	input := []Snapshot{
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2, Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "CandidateHealthPending"}, Health: domain.Health{Overall: domain.HealthProgressing, Summary: "candidate is warming"}, Conditions: []domain.Condition{{Type: "Ready", Status: false, Reason: "HealthPending", Message: "candidate target group is still warming"}}, DeployedArtifact: domain.DeploymentArtifact{Image: "ghcr.io/devopsartfactory/payments-worker:1.2.3"}}},
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "RolloutCompleted"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"}, Conditions: []domain.Condition{{Type: "Ready", Status: true, Reason: "Stable", Message: "deployment is healthy"}}, DeployedArtifact: domain.DeploymentArtifact{Binary: "s3://releases.example.com/payments-api/prod/payments-api-linux-amd64"}}},
	}

	if err := Save(path, input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected two snapshots, got %+v", loaded)
	}
	if loaded[0].DeploymentName != "api-prod" || loaded[1].DeploymentName != "worker-prod" {
		t.Fatalf("expected sorted snapshots, got %+v", loaded)
	}
	if loaded[0].Status.Sync.Reason != "RolloutCompleted" || loaded[1].Status.Health.Overall != domain.HealthProgressing {
		t.Fatalf("unexpected loaded snapshots: %+v", loaded)
	}
	if len(loaded[0].Status.Conditions) != 1 || loaded[0].Status.Conditions[0].Type != "Ready" {
		t.Fatalf("unexpected loaded conditions: %+v", loaded[0].Status.Conditions)
	}
	if loaded[0].Status.DeployedArtifact.Binary == "" || loaded[1].Status.DeployedArtifact.Image == "" {
		t.Fatalf("expected deployed artifact metadata, got %+v", loaded)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployments.json")
	if err := os.WriteFile(path, []byte("{not-json}"), 0o644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected invalid JSON error")
	}
	if !strings.Contains(err.Error(), "decode status store") {
		t.Fatalf("unexpected error: %v", err)
	}
}
