package statusstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/domain"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	input := []Snapshot{
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2, Sync: domain.Sync{State: domain.SyncStateReconciling}}},
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced}}},
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
