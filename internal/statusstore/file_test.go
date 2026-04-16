package statusstore

import (
	"context"
	"errors"
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

func TestSaveKeepsExistingFileWhenAtomicRenameFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	original := []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 1}}}
	if err := Save(path, original); err != nil {
		t.Fatalf("initial Save returned error: %v", err)
	}

	originalContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	previousRename := renameFile
	renameFile = func(oldPath, newPath string) error {
		return errors.New("rename failed")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = Save(path, []Snapshot{{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}}})
	if err == nil || !strings.Contains(err.Error(), "rename status store") {
		t.Fatalf("expected rename failure, got %v", err)
	}

	currentContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after failed Save returned error: %v", err)
	}
	if string(currentContent) != string(originalContent) {
		t.Fatalf("expected failed atomic Save to preserve original file, got %s", string(currentContent))
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".deployments.json.tmp-*"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected failed atomic Save to clean temp files, got %+v", matches)
	}
}

func TestSavePreservesExistingFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	if err := Save(path, []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 1}}}); err != nil {
		t.Fatalf("initial Save returned error: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("Chmod returned error: %v", err)
	}

	if err := Save(path, []Snapshot{{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}}}); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("expected Save to preserve existing file mode 0640, got %#o", got)
	}
}

func TestFileStoreUpsertCreatesAndReplacesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	store := NewFileStore(path)
	ctx := context.Background()

	if err := store.Upsert(ctx, Snapshot{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2, Sync: domain.Sync{State: domain.SyncStateReconciling, Reason: "warming"}}}); err != nil {
		t.Fatalf("first Upsert returned error: %v", err)
	}
	if err := store.Upsert(ctx, Snapshot{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}}}); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}
	if err := store.Upsert(ctx, Snapshot{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 4, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "rolled-forward"}, Health: domain.Health{Overall: domain.HealthHealthy, Summary: "all checks passed"}}}); err != nil {
		t.Fatalf("replacement Upsert returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("unexpected listed snapshots: %+v", listed)
	}
	if listed[1].Status.ObservedGeneration != 4 || listed[1].Status.Sync.Reason != "rolled-forward" || listed[1].Status.Health.Overall != domain.HealthHealthy {
		t.Fatalf("expected replaced worker snapshot, got %+v", listed[1])
	}
}

func TestFileStoreDeleteRemovesSnapshotAndMissingIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	store := NewFileStore(path)
	ctx := context.Background()

	if err := store.Save(ctx, []Snapshot{
		{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3}},
		{DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}},
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := store.Delete(ctx, "worker-prod"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := store.Delete(ctx, "missing"); err != nil {
		t.Fatalf("Delete for missing snapshot returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].DeploymentName != "api-prod" {
		t.Fatalf("unexpected listed snapshots after delete: %+v", listed)
	}
	_, err = store.Get(ctx, "worker-prod")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected worker-prod to be deleted, got %v", err)
	}
}

func TestFileStoreDeleteOnMissingFileDoesNotCreateStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	store := NewFileStore(path)

	if err := store.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected store file to remain absent, got %v", err)
	}
}

func TestFileStoreDeleteRequiresPath(t *testing.T) {
	store := NewFileStore("")

	if err := store.Delete(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected missing path error, got %v", err)
	}
}

func TestFileStoreApplyBatchesUpdatesAndDeletesIntoOneSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	store := NewFileStore(path)
	ctx := context.Background()
	if err := store.Save(ctx, []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 1}}, {DeploymentName: "legacy-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9}}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if err := store.Apply(ctx, []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3, Sync: domain.Sync{State: domain.SyncStateSynced, Reason: "stable"}}}, {DeploymentName: "worker-prod", Status: domain.DeploymentStatus{ObservedGeneration: 2}}}, []string{"legacy-prod"}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].DeploymentName != "api-prod" || listed[1].DeploymentName != "worker-prod" {
		t.Fatalf("unexpected listed snapshots after apply: %+v", listed)
	}
	if listed[0].Status.ObservedGeneration != 3 || listed[0].Status.Sync.Reason != "stable" {
		t.Fatalf("expected api-prod to be updated in apply batch, got %+v", listed[0])
	}
	if _, err := store.Get(ctx, "legacy-prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected legacy-prod to be removed by apply batch, got %v", err)
	}
}

func TestFileStoreApplyKeepsExistingFileWhenAtomicRenameFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status", "deployments.json")
	store := NewFileStore(path)
	ctx := context.Background()
	original := []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 1}}, {DeploymentName: "legacy-prod", Status: domain.DeploymentStatus{ObservedGeneration: 9}}}
	if err := store.Save(ctx, original); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	originalContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	previousRename := renameFile
	renameFile = func(oldPath, newPath string) error {
		return errors.New("rename failed")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.Apply(ctx, []Snapshot{{DeploymentName: "api-prod", Status: domain.DeploymentStatus{ObservedGeneration: 3}}}, []string{"legacy-prod"})
	if err == nil || !strings.Contains(err.Error(), "rename status store") {
		t.Fatalf("expected rename failure, got %v", err)
	}

	currentContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after failed Apply returned error: %v", err)
	}
	if string(currentContent) != string(originalContent) {
		t.Fatalf("expected failed Apply to preserve original file, got %s", string(currentContent))
	}
}
