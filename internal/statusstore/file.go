package statusstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
)

// Load reads deployment status snapshots from a JSON file.
func Load(path string) ([]Snapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read status store: %w", err)
	}

	var snapshots []Snapshot
	if err := json.Unmarshal(content, &snapshots); err != nil {
		return nil, fmt.Errorf("decode status store: %w", err)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].DeploymentName < snapshots[j].DeploymentName
	})
	return snapshots, nil
}

// Save writes deployment status snapshots to a JSON file.
func Save(path string, snapshots []Snapshot) error {
	sorted := append([]Snapshot(nil), snapshots...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].DeploymentName < sorted[j].DeploymentName
	})

	content, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status store: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create status store dir: %w", err)
	}
	fileMode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		fileMode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat status store: %w", err)
	}

	tempFile, err := createTempFile(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp status store: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(fileMode); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod temp status store: %w", err)
	}

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temp status store: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp status store: %w", err)
	}
	if err := renameFile(tempPath, path); err != nil {
		return fmt.Errorf("rename status store: %w", err)
	}
	return nil
}

func upsertSnapshots(snapshots []Snapshot, snapshot Snapshot) []Snapshot {
	updated := append([]Snapshot(nil), snapshots...)
	for i := range updated {
		if updated[i].DeploymentName == snapshot.DeploymentName {
			updated[i] = snapshot
			return updated
		}
	}
	return append(updated, snapshot)
}

func deleteSnapshot(snapshots []Snapshot, deploymentName string) []Snapshot {
	updated := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.DeploymentName == deploymentName {
			continue
		}
		updated = append(updated, snapshot)
	}
	return updated
}
