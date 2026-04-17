package planpreview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadRoundTripsPlanPreviewRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plans", "previews.json")
	input := []Record{
		{ID: "plan_b", DeploymentName: "worker-prod", Status: StatusFailed, CreatedAt: "2026-04-17T08:00:00Z", Errors: []string{"stale source fingerprint"}},
		{ID: "plan_a", DeploymentName: "api-prod", Status: StatusReady, CreatedAt: "2026-04-17T07:59:00Z", Operations: []Operation{{Kind: "CreateCodeDeployDeployment", Status: "completed", Phase: "Provisioning", Target: "api-prod-codedeploy-group (codeDeployDeploymentGroup)", Summary: "Create the CodeDeploy deployment Tenet will use for traffic shifting."}}},
	}

	if err := Save(path, input); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected two plan previews, got %+v", loaded)
	}
	if loaded[0].ID != "plan_a" || loaded[1].ID != "plan_b" {
		t.Fatalf("expected saved previews to be sorted by ID, got %+v", loaded)
	}
	if loaded[0].Operations[0].Kind != "CreateCodeDeployDeployment" || loaded[1].Errors[0] != "stale source fingerprint" {
		t.Fatalf("unexpected loaded previews: %+v", loaded)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "previews.json")
	if err := os.WriteFile(path, []byte("{not-json}"), 0o644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "decode plan preview store") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFileStoreGetAndSaveAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plans", "previews.json")
	store := NewFileStore(path)
	ctx := context.Background()
	record := Record{ID: "plan_a", DeploymentName: "api-prod", Status: StatusReady}

	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("first Save returned error: %v", err)
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}
	got, err := store.Get(ctx, "plan_a")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ID != "plan_a" || got.DeploymentName != "api-prod" {
		t.Fatalf("unexpected stored plan preview: %+v", got)
	}
}

func TestFileStoreGetReturnsNotFoundWhenMissing(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "plans", "previews.json"))

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

	err := Save(filepath.Join(t.TempDir(), "plans", "previews.json"), []Record{{ID: "plan_a", DeploymentName: "api-prod", Status: StatusReady}})
	if err == nil || !strings.Contains(err.Error(), "rename plan preview store") {
		t.Fatalf("expected rename error, got %v", err)
	}
	if createdTempFile == nil {
		t.Fatalf("expected temp file to be created")
	}
	if _, writeErr := createdTempFile.Write([]byte("x")); !errors.Is(writeErr, os.ErrClosed) {
		t.Fatalf("expected temp file to be closed after Save failure, got %v", writeErr)
	}
}
