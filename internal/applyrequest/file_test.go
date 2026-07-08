package applyrequest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadRoundTripsApplyRequestRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applies", "requests.json")
	input := []Record{
		{ID: "apply_b", PlanID: "plan_b", DeploymentName: "worker-prod", Status: StatusFailed, CreatedAt: "2026-04-18T08:00:00Z", Error: disabledError, Details: []string{"did not contact AWS"}},
		{ID: "apply_a", PlanID: "plan_a", DeploymentName: "api-prod", Status: StatusFailed, CreatedAt: "2026-04-18T07:59:00Z", Error: disabledError, Lifecycle: Lifecycle{UpdatedAt: "2026-04-18T07:59:00Z", FinishedAt: "2026-04-18T07:59:00Z", TotalSteps: 1}, Operations: []Operation{{Status: OperationStatusPending, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."}}, Source: SourceContext{PlanID: "plan_a", Route: "/v1/plans/plan_a", PlanStatus: "ready", Fingerprint: "plan_input"}},
	}

	if err := Save(path, input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected two apply requests, got %+v", loaded)
	}
	if loaded[0].ID != "apply_a" || loaded[1].ID != "apply_b" {
		t.Fatalf("expected saved apply requests to be sorted by ID, got %+v", loaded)
	}
	if loaded[0].Source.Route != "/v1/plans/plan_a" || loaded[0].Operations[0].Status != OperationStatusPending || loaded[0].Lifecycle.TotalSteps != 1 || loaded[1].Details[0] != "did not contact AWS" {
		t.Fatalf("unexpected loaded apply requests: %+v", loaded)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.json")
	if err := os.WriteFile(path, []byte("{not-json}"), 0o644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "decode apply request store") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFileStoreGetAndSaveAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applies", "requests.json")
	store := NewFileStore(path)
	ctx := context.Background()
	record := Record{ID: "apply_a", PlanID: "plan_a", DeploymentName: "api-prod", Status: StatusFailed}

	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("first Save returned error: %v", err)
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}
	got, err := store.Get(ctx, "apply_a")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ID != "apply_a" || got.PlanID != "plan_a" {
		t.Fatalf("unexpected stored apply request: %+v", got)
	}
}

func TestFileStoreListReturnsSortedRecordsAndHandlesMissingFile(t *testing.T) {
	t.Run("sorted records", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "applies", "requests.json")
		store := NewFileStore(path)
		ctx := context.Background()
		for _, record := range []Record{
			{ID: "apply_b", PlanID: "plan_b", DeploymentName: "worker-prod", Status: StatusFailed},
			{ID: "apply_a", PlanID: "plan_a", DeploymentName: "api-prod", Status: StatusQueued},
		} {
			if err := store.Save(ctx, record); err != nil {
				t.Fatalf("Save(%s) returned error: %v", record.ID, err)
			}
		}

		records, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(records) != 2 || records[0].ID != "apply_a" || records[1].ID != "apply_b" {
			t.Fatalf("expected sorted apply requests, got %+v", records)
		}
	})

	t.Run("missing file returns empty slice", func(t *testing.T) {
		store := NewFileStore(filepath.Join(t.TempDir(), "applies", "requests.json"))
		records, err := store.List(context.Background())
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(records) != 0 {
			t.Fatalf("expected empty list for missing file, got %+v", records)
		}
	})
}

func TestFileStoreUpdateReplacesExistingRecordAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applies", "requests.json")
	store := NewFileStore(path)
	ctx := context.Background()
	record := Record{
		ID:             "apply_a",
		PlanID:         "plan_a",
		DeploymentName: "api-prod",
		Status:         StatusQueued,
		Lifecycle: Lifecycle{
			UpdatedAt:  "2026-04-18T08:05:00Z",
			TotalSteps: 2,
		},
		Operations: []Operation{
			{Status: OperationStatusQueued, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."},
			{Status: OperationStatusQueued, Phase: "Promoting", Target: "api-prod listener", Summary: "Shift production traffic."},
		},
	}

	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	record.Status = StatusRunning
	record.Lifecycle = Lifecycle{
		UpdatedAt:      "2026-04-18T08:06:00Z",
		StartedAt:      "2026-04-18T08:06:00Z",
		CurrentStep:    1,
		CompletedSteps: 0,
		TotalSteps:     2,
	}
	record.Operations[0].Status = OperationStatusRunning
	if err := store.Update(ctx, record); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	got, err := store.Get(ctx, "apply_a")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Status != StatusRunning || got.Lifecycle.StartedAt != "2026-04-18T08:06:00Z" || got.Operations[0].Status != OperationStatusRunning {
		t.Fatalf("expected updated apply request to be stored, got %+v", got)
	}
}

func TestFileStoreUpdateReturnsNotFoundWhenMissing(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "applies", "requests.json"))

	err := store.Update(context.Background(), Record{ID: "missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreHeartbeatPersistsTransformedClaimAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applies", "requests.json")
	store := NewFileStore(path)
	ctx := context.Background()
	record := Record{
		ID:             "apply_claimed",
		PlanID:         "plan_ready",
		DeploymentName: "api-prod",
		Status:         StatusRunning,
		Lifecycle:      Lifecycle{UpdatedAt: "2026-04-18T08:10:00Z", StartedAt: "2026-04-18T08:10:00Z", CurrentStep: 1, TotalSteps: 1},
		Claim:          &Claim{WorkerID: "worker-a", ClaimedAt: "2026-04-18T08:10:00Z", LastHeartbeatAt: "2026-04-18T08:10:00Z", ExpiresAt: "2026-04-18T08:15:00Z"},
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	updated, err := store.Heartbeat(ctx, "apply_claimed", HeartbeatRequest{WorkerID: "worker-a"}, func(record Record) (Record, error) {
		record.Claim.LastHeartbeatAt = "2026-04-18T08:12:30Z"
		record.Claim.ExpiresAt = "2026-04-18T08:17:30Z"
		return record, nil
	})
	if err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	if updated.Claim == nil || updated.Claim.LastHeartbeatAt != "2026-04-18T08:12:30Z" || updated.Lifecycle.UpdatedAt != "2026-04-18T08:10:00Z" {
		t.Fatalf("expected heartbeat transform to preserve non-claim state, got %+v", updated)
	}
	stored, err := store.Get(ctx, "apply_claimed")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored.Claim == nil || stored.Claim.LastHeartbeatAt != "2026-04-18T08:12:30Z" || stored.Claim.ExpiresAt != "2026-04-18T08:17:30Z" {
		t.Fatalf("expected heartbeat claim metadata persisted, got %+v", stored.Claim)
	}
}

func TestFileStoreGetReturnsNotFoundWhenMissing(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "applies", "requests.json"))

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveClosesTempFileOnRenameError(t *testing.T) {
	originalCreateTempFile := createTempFile
	originalRenameFile := renameFile
	defer func() {
		createTempFile = originalCreateTempFile
		renameFile = originalRenameFile
	}()

	var createdTempFile *os.File
	createTempFile = func(dir, pattern string) (*os.File, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		createdTempFile = file
		return file, nil
	}
	renameFile = func(string, string) error {
		return fmt.Errorf("rename failed")
	}

	err := Save(filepath.Join(t.TempDir(), "applies", "requests.json"), []Record{{ID: "apply_a", PlanID: "plan_a", DeploymentName: "api-prod", Status: StatusFailed}})
	if err == nil || !strings.Contains(err.Error(), "rename apply request store") {
		t.Fatalf("expected rename error, got %v", err)
	}
	if createdTempFile == nil {
		t.Fatalf("expected temp file to be created")
	}
	if _, writeErr := createdTempFile.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("expected temp file to be closed after Save failure, got %v", writeErr)
	}
}
