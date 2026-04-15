package statusstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create status store dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write status store: %w", err)
	}
	return nil
}
