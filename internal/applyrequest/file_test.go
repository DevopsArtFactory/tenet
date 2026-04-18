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
		{ID: "apply_a", PlanID: "plan_a", DeploymentName: "api-prod", Status: StatusFailed, CreatedAt: "2026-04-18T07:59:00Z", Error: disabledError, Operations: []Operation{{Status: OperationStatusPending, Phase: "Provisioning", Target: "api-prod deployment", Summary: "Create replacement capacity for api-prod."}}, Source: SourceContext{PlanID: "plan_a", Route: "/v1/plans/plan_a", PlanStatus: "ready", Fingerprint: "plan_input"}},
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
	if loaded[0].Source.Route != "/v1/plans/plan_a" || loaded[0].Operations[0].Status != OperationStatusPending || loaded[1].Details[0] != "did not contact AWS" {
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
